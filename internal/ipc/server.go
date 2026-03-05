package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"
)

// Request is a JSON command sent by the client.
type Request struct {
	Cmd   string `json:"cmd"`
	Lines int    `json:"lines,omitempty"` // for "logs" command
}

// StatusResponse is the response to a "status" command.
type StatusResponse struct {
	State     string    `json:"state"`
	LastSync  string    `json:"last_sync"`
	LastError string    `json:"last_error"`
	Stats     SyncStats `json:"stats"`
	Version   string    `json:"version,omitempty"`
	Error     string    `json:"error,omitempty"`
}

// SyncStats holds cumulative stats about the bridge's sync activity.
type SyncStats struct {
	NotesSynced    int   `json:"notes_synced"`
	TagsSynced     int   `json:"tags_synced"`
	QueueProcessed int   `json:"queue_processed"`
	LastDurationMs int64 `json:"last_duration_ms"`
}

// OkResponse is a simple acknowledgment response.
type OkResponse struct {
	Ok    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// LogEntry represents a single log entry stored in the ring buffer.
type LogEntry struct {
	Time  string `json:"time"`
	Level string `json:"level"`
	Msg   string `json:"msg"`
}

// LogsResponse is the response to a "logs" command.
type LogsResponse struct {
	Entries []LogEntry `json:"entries"`
	Error   string     `json:"error,omitempty"`
}

// QueueStatusItem represents a write queue item for IPC status display.
type QueueStatusItem struct {
	ID        int64  `json:"id"`
	Action    string `json:"action"`
	NoteTitle string `json:"note_title"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at,omitempty"`
}

// QueueStatusResponse is the response to a "queue_status" command.
type QueueStatusResponse struct {
	Items []QueueStatusItem `json:"items"`
	Error string            `json:"error,omitempty"`
}

// StatusProvider provides current bridge status to the IPC server.
type StatusProvider interface {
	GetStatus() StatusResponse
	TriggerSync()
	GetLogs(n int) []LogEntry
	GetQueueStatus() QueueStatusResponse
	RequestShutdown()
}

// Server is a Unix socket IPC server for the bridge daemon.
type Server struct {
	socketPath string
	provider   StatusProvider
	logger     *slog.Logger
	listener   net.Listener
	cancelFn   context.CancelFunc

	mu      sync.Mutex
	started bool
}

// NewServer creates a new IPC server.
func NewServer(socketPath string, provider StatusProvider, logger *slog.Logger) *Server {
	return &Server{
		socketPath: socketPath,
		provider:   provider,
		logger:     logger,
	}
}

// Start begins listening for connections. It removes any stale socket file before binding.
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return fmt.Errorf("ipc server already started")
	}
	s.started = true
	s.mu.Unlock()

	// Remove stale socket file.
	if err := os.Remove(s.socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.revertStarted()
		return fmt.Errorf("remove stale socket: %w", err)
	}

	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "unix", s.socketPath)
	if err != nil {
		s.revertStarted()
		return fmt.Errorf("listen on %s: %w", s.socketPath, err)
	}

	// Set socket permissions to owner-only.
	if err := os.Chmod(s.socketPath, 0o600); err != nil {
		ln.Close() //nolint:errcheck,gosec // closing on error path
		s.revertStarted()
		return fmt.Errorf("chmod socket: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)

	s.mu.Lock()
	s.listener = ln
	s.cancelFn = cancel
	s.mu.Unlock()

	go s.acceptLoop(ctx)

	s.logger.Info("ipc server started", "socket", s.socketPath)
	return nil
}

func (s *Server) revertStarted() {
	s.mu.Lock()
	s.started = false
	s.mu.Unlock()
}

// Stop shuts down the IPC server and removes the socket file.
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return nil
	}

	s.started = false
	if s.cancelFn != nil {
		s.cancelFn()
	}

	var closeErr error
	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			closeErr = fmt.Errorf("close ipc listener: %w", err)
		}
	}

	// Best-effort cleanup of socket file.
	os.Remove(s.socketPath) //nolint:errcheck,gosec // best-effort cleanup

	s.logger.Info("ipc server stopped")
	return closeErr
}

const (
	connReadTimeout  = 5 * time.Second
	connWriteTimeout = 5 * time.Second
	maxRequestSize   = 4096
)

func (s *Server) acceptLoop(ctx context.Context) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				s.logger.Warn("ipc accept error, continuing", "error", err)
				continue
			}
		}

		go s.handleConn(ctx, conn)
	}
}

func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close() //nolint:errcheck // best-effort close

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, maxRequestSize), maxRequestSize)

	if err := conn.SetReadDeadline(time.Now().Add(connReadTimeout)); err != nil {
		s.logger.Debug("ipc set read deadline failed", "error", err)
		return
	}

	if !scanner.Scan() {
		if scanner.Err() != nil {
			s.logger.Debug("ipc read error", "error", scanner.Err())
		}
		return
	}

	line := scanner.Bytes()

	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		s.writeJSON(conn, OkResponse{Ok: false, Error: "invalid JSON"})
		return
	}

	s.dispatch(ctx, conn, &req)
}

func (s *Server) dispatch(_ context.Context, conn net.Conn, req *Request) {
	switch req.Cmd {
	case "status":
		status := s.provider.GetStatus()
		s.writeJSON(conn, status)
	case "sync_now":
		s.provider.TriggerSync()
		s.writeJSON(conn, OkResponse{Ok: true})
	case "logs":
		lines := req.Lines
		if lines <= 0 {
			lines = 50
		}
		entries := s.provider.GetLogs(lines)
		s.writeJSON(conn, LogsResponse{Entries: entries})
	case "queue_status":
		queueStatus := s.provider.GetQueueStatus()
		s.writeJSON(conn, queueStatus)
	case "quit":
		s.writeJSON(conn, OkResponse{Ok: true})
		s.provider.RequestShutdown()
	default:
		s.writeJSON(conn, OkResponse{Ok: false, Error: fmt.Sprintf("unknown command: %s", req.Cmd)})
	}
}

func (s *Server) writeJSON(conn net.Conn, v any) {
	if err := conn.SetWriteDeadline(time.Now().Add(connWriteTimeout)); err != nil {
		s.logger.Debug("ipc set write deadline failed", "error", err)
		return
	}

	data, err := json.Marshal(v)
	if err != nil {
		s.logger.Warn("ipc marshal response failed", "error", err)
		return
	}

	data = append(data, '\n')
	if _, err := conn.Write(data); err != nil {
		s.logger.Debug("ipc write error", "error", err)
	}
}
