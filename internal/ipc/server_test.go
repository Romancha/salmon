package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockProvider implements StatusProvider for tests.
type mockProvider struct {
	mu             sync.Mutex
	status         StatusResponse
	logs           []LogEntry
	queueItems     []QueueStatusItem
	syncCalled     bool
	shutdownCalled bool
}

func (m *mockProvider) GetStatus() StatusResponse {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

func (m *mockProvider) TriggerSync() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.syncCalled = true
}

func (m *mockProvider) GetLogs(n int) []LogEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	if n > len(m.logs) {
		n = len(m.logs)
	}
	result := make([]LogEntry, n)
	copy(result, m.logs[len(m.logs)-n:])
	return result
}

func (m *mockProvider) GetQueueStatus() QueueStatusResponse {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := make([]QueueStatusItem, len(m.queueItems))
	copy(items, m.queueItems)
	return QueueStatusResponse{Items: items}
}

func (m *mockProvider) RequestShutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shutdownCalled = true
}

func testSocketPath(t *testing.T) string {
	t.Helper()
	// macOS limits Unix socket paths to 104 chars. Use /tmp with a short unique name.
	f, err := os.CreateTemp("/tmp", "ipc-*.sock")
	require.NoError(t, err)
	path := f.Name()
	_ = f.Close()
	_ = os.Remove(path) // socket will be created by server
	t.Cleanup(func() { _ = os.Remove(path) })
	return path
}

func dialSocket(t *testing.T, path string) net.Conn {
	t.Helper()
	var d net.Dialer
	conn, err := d.DialContext(context.Background(), "unix", path)
	require.NoError(t, err)
	return conn
}

func sendCmd(t *testing.T, conn net.Conn, cmd any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(cmd)
	require.NoError(t, err)
	data = append(data, '\n')
	_, err = conn.Write(data)
	require.NoError(t, err)

	scanner := bufio.NewScanner(conn)
	require.True(t, scanner.Scan(), "expected response, got error: %v", scanner.Err())
	return json.RawMessage(scanner.Bytes())
}

func startTestServer(t *testing.T, provider StatusProvider) string {
	t.Helper()
	sockPath := testSocketPath(t)
	srv := NewServer(sockPath, provider, testLogger())
	require.NoError(t, srv.Start(context.Background()))
	t.Cleanup(func() { _ = srv.Stop() })
	return sockPath
}

func TestServer_StatusCommand(t *testing.T) {
	provider := &mockProvider{
		status: StatusResponse{
			State:     "idle",
			LastSync:  "2026-03-04T12:00:00Z",
			LastError: "",
			Stats: SyncStats{
				NotesSynced:    100,
				TagsSynced:     10,
				QueueProcessed: 5,
				LastDurationMs: 1200,
			},
		},
	}

	sockPath := startTestServer(t, provider)
	conn := dialSocket(t, sockPath)
	defer conn.Close() //nolint:errcheck,gosec // test cleanup

	resp := sendCmd(t, conn, Request{Cmd: "status"})

	var status StatusResponse
	require.NoError(t, json.Unmarshal(resp, &status))
	assert.Equal(t, "idle", status.State)
	assert.Equal(t, "2026-03-04T12:00:00Z", status.LastSync)
	assert.Equal(t, 100, status.Stats.NotesSynced)
	assert.Equal(t, int64(1200), status.Stats.LastDurationMs)
}

func TestServer_SyncNowCommand(t *testing.T) {
	provider := &mockProvider{}
	sockPath := startTestServer(t, provider)

	conn := dialSocket(t, sockPath)
	defer conn.Close() //nolint:errcheck,gosec // test cleanup

	resp := sendCmd(t, conn, Request{Cmd: "sync_now"})

	var ok OkResponse
	require.NoError(t, json.Unmarshal(resp, &ok))
	assert.True(t, ok.Ok)

	provider.mu.Lock()
	assert.True(t, provider.syncCalled)
	provider.mu.Unlock()
}

func TestServer_LogsCommand(t *testing.T) {
	provider := &mockProvider{
		logs: []LogEntry{
			{Time: "t1", Level: "info", Msg: "first"},
			{Time: "t2", Level: "warn", Msg: "second"},
			{Time: "t3", Level: "error", Msg: "third"},
		},
	}

	sockPath := startTestServer(t, provider)
	conn := dialSocket(t, sockPath)
	defer conn.Close() //nolint:errcheck,gosec // test cleanup

	resp := sendCmd(t, conn, Request{Cmd: "logs", Lines: 2})

	var logsResp LogsResponse
	require.NoError(t, json.Unmarshal(resp, &logsResp))
	assert.Len(t, logsResp.Entries, 2)
	assert.Equal(t, "second", logsResp.Entries[0].Msg)
	assert.Equal(t, "third", logsResp.Entries[1].Msg)
}

func TestServer_LogsCommand_DefaultLines(t *testing.T) {
	logs := make([]LogEntry, 60)
	for i := range 60 {
		logs[i] = LogEntry{Time: "t", Level: "info", Msg: "msg"}
	}
	provider := &mockProvider{logs: logs}

	sockPath := startTestServer(t, provider)
	conn := dialSocket(t, sockPath)
	defer conn.Close() //nolint:errcheck,gosec // test cleanup

	// Lines=0 should default to 50.
	resp := sendCmd(t, conn, Request{Cmd: "logs"})

	var logsResp LogsResponse
	require.NoError(t, json.Unmarshal(resp, &logsResp))
	assert.Len(t, logsResp.Entries, 50)
}

func TestServer_QuitCommand(t *testing.T) {
	provider := &mockProvider{}
	sockPath := startTestServer(t, provider)

	conn := dialSocket(t, sockPath)
	defer conn.Close() //nolint:errcheck,gosec // test cleanup

	resp := sendCmd(t, conn, Request{Cmd: "quit"})

	var ok OkResponse
	require.NoError(t, json.Unmarshal(resp, &ok))
	assert.True(t, ok.Ok)

	assert.Eventually(t, func() bool {
		provider.mu.Lock()
		defer provider.mu.Unlock()
		return provider.shutdownCalled
	}, time.Second, 10*time.Millisecond)
}

func TestServer_UnknownCommand(t *testing.T) {
	provider := &mockProvider{}
	sockPath := startTestServer(t, provider)

	conn := dialSocket(t, sockPath)
	defer conn.Close() //nolint:errcheck,gosec // test cleanup

	resp := sendCmd(t, conn, Request{Cmd: "invalid_cmd"})

	var ok OkResponse
	require.NoError(t, json.Unmarshal(resp, &ok))
	assert.False(t, ok.Ok)
	assert.Contains(t, ok.Error, "unknown command")
}

func TestServer_MalformedJSON(t *testing.T) {
	provider := &mockProvider{}
	sockPath := startTestServer(t, provider)

	conn := dialSocket(t, sockPath)
	defer conn.Close() //nolint:errcheck,gosec // test cleanup

	// Send malformed JSON.
	_, err := conn.Write([]byte("not json\n"))
	require.NoError(t, err)

	scanner := bufio.NewScanner(conn)
	require.True(t, scanner.Scan())

	var ok OkResponse
	require.NoError(t, json.Unmarshal(scanner.Bytes(), &ok))
	assert.False(t, ok.Ok)
	assert.Contains(t, ok.Error, "invalid JSON")
}

func TestServer_ConcurrentConnections(t *testing.T) {
	provider := &mockProvider{
		status: StatusResponse{State: "idle"},
	}
	sockPath := startTestServer(t, provider)

	const numClients = 10
	var wg sync.WaitGroup
	wg.Add(numClients)

	errs := make(chan error, numClients)

	for range numClients {
		go func() {
			defer wg.Done()
			var d net.Dialer
			conn, err := d.DialContext(context.Background(), "unix", sockPath)
			if err != nil {
				errs <- err
				return
			}
			defer conn.Close() //nolint:errcheck,gosec // test goroutine cleanup

			data, _ := json.Marshal(Request{Cmd: "status"}) //nolint:errcheck,errchkjson // test helper
			data = append(data, '\n')
			if _, err := conn.Write(data); err != nil {
				errs <- err
				return
			}

			scanner := bufio.NewScanner(conn)
			if !scanner.Scan() {
				errs <- scanner.Err()
				return
			}

			var status StatusResponse
			if err := json.Unmarshal(scanner.Bytes(), &status); err != nil {
				errs <- err
				return
			}

			if status.State != "idle" {
				errs <- assert.AnError
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
}

func TestServer_RemovesStaleSocket(t *testing.T) {
	sockPath := testSocketPath(t)

	// Create a real stale Unix socket by starting and stopping a listener.
	lc := net.ListenConfig{}
	ln, err := lc.Listen(context.Background(), "unix", sockPath)
	require.NoError(t, err)
	ln.(*net.UnixListener).SetUnlinkOnClose(false)
	require.NoError(t, ln.Close())
	// Socket file remains on disk after listener close (unlink-on-close disabled).

	srv := NewServer(sockPath, &mockProvider{}, testLogger())
	require.NoError(t, srv.Start(context.Background()))
	t.Cleanup(func() { _ = srv.Stop() })

	// Should be able to connect (stale socket was removed and replaced).
	conn := dialSocket(t, sockPath)
	_ = conn.Close()
}

func TestServer_RejectsNonSocketPath(t *testing.T) {
	sockPath := testSocketPath(t)

	// Create a regular file at the socket path.
	require.NoError(t, os.WriteFile(sockPath, []byte("not a socket"), 0o600))

	srv := NewServer(sockPath, &mockProvider{}, testLogger())
	err := srv.Start(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "not a Unix socket")
}

func TestServer_StopRemovesSocket(t *testing.T) {
	sockPath := testSocketPath(t)

	srv := NewServer(sockPath, &mockProvider{}, testLogger())
	require.NoError(t, srv.Start(context.Background()))

	// Socket file should exist.
	_, err := os.Stat(sockPath)
	require.NoError(t, err)

	require.NoError(t, srv.Stop())

	// Socket file should be removed.
	_, err = os.Stat(sockPath)
	assert.True(t, os.IsNotExist(err))
}

func TestServer_DoubleStartFails(t *testing.T) {
	sockPath := testSocketPath(t)

	srv := NewServer(sockPath, &mockProvider{}, testLogger())
	require.NoError(t, srv.Start(context.Background()))
	t.Cleanup(func() { _ = srv.Stop() })

	err := srv.Start(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already started")
}

func TestServer_QueueStatusCommand(t *testing.T) {
	provider := &mockProvider{
		queueItems: []QueueStatusItem{
			{ID: 1, Action: "create", NoteTitle: "My Note", Status: "processing", CreatedAt: "2026-03-04T12:00:00Z"},
			{ID: 2, Action: "add_tag", NoteTitle: "work", Status: "applied"},
		},
	}

	sockPath := startTestServer(t, provider)
	conn := dialSocket(t, sockPath)
	defer conn.Close() //nolint:errcheck,gosec // test cleanup

	resp := sendCmd(t, conn, Request{Cmd: "queue_status"})

	var queueResp QueueStatusResponse
	require.NoError(t, json.Unmarshal(resp, &queueResp))
	require.Len(t, queueResp.Items, 2)
	assert.Equal(t, int64(1), queueResp.Items[0].ID)
	assert.Equal(t, "create", queueResp.Items[0].Action)
	assert.Equal(t, "My Note", queueResp.Items[0].NoteTitle)
	assert.Equal(t, "processing", queueResp.Items[0].Status)
	assert.Equal(t, "2026-03-04T12:00:00Z", queueResp.Items[0].CreatedAt)
	assert.Equal(t, int64(2), queueResp.Items[1].ID)
	assert.Equal(t, "add_tag", queueResp.Items[1].Action)
	assert.Equal(t, "applied", queueResp.Items[1].Status)
}

func TestServer_QueueStatusCommand_Empty(t *testing.T) {
	provider := &mockProvider{}
	sockPath := startTestServer(t, provider)

	conn := dialSocket(t, sockPath)
	defer conn.Close() //nolint:errcheck,gosec // test cleanup

	resp := sendCmd(t, conn, Request{Cmd: "queue_status"})

	var queueResp QueueStatusResponse
	require.NoError(t, json.Unmarshal(resp, &queueResp))
	assert.Empty(t, queueResp.Items)
}

func TestServer_SocketPermissions(t *testing.T) {
	sockPath := testSocketPath(t)

	srv := NewServer(sockPath, &mockProvider{}, testLogger())
	require.NoError(t, srv.Start(context.Background()))
	t.Cleanup(func() { _ = srv.Stop() })

	info, err := os.Stat(sockPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}
