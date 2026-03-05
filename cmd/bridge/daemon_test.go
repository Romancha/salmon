package main

import (
	"context"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/romancha/salmon/internal/beardb"
	"github.com/romancha/salmon/internal/mapper"
)

func TestRunDaemon_RunsInitialSyncAndStops(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	db := &mockBearDB{noteUUIDs: []string{"note-1"}}
	hub := &mockHubClient{}
	bridge := newTestBridge(db, hub, statePath)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := runDaemon(ctx, bridge, 1*time.Hour, testLogger())
	require.NoError(t, err)

	// Initial sync should have run (state file created).
	state, err := loadState(statePath)
	require.NoError(t, err)
	require.NotNil(t, state)
}

func TestRunDaemon_RunsMultipleCycles(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	var notesCallCount atomic.Int32
	db := &daemonCountingDB{notesCallCount: &notesCallCount}
	hub := &mockHubClient{}
	bridge := newTestBridge(db, hub, statePath)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(250 * time.Millisecond)
		cancel()
	}()

	err := runDaemon(ctx, bridge, 50*time.Millisecond, testLogger())
	require.NoError(t, err)

	// Notes() is called each sync cycle. Initial sync + ticker cycles = at least 3.
	count := notesCallCount.Load()
	assert.GreaterOrEqual(t, count, int32(3), "expected at least 3 Notes() calls, got %d", count)
}

func TestRunDaemon_ContinuesOnSyncError(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	var syncCount atomic.Int32
	db := &daemonFailingDB{failUntil: 2, syncCount: &syncCount}
	hub := &mockHubClient{}
	bridge := newTestBridge(db, hub, statePath)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(250 * time.Millisecond)
		cancel()
	}()

	err := runDaemon(ctx, bridge, 50*time.Millisecond, testLogger())
	require.NoError(t, err)

	// Daemon should have continued running despite errors.
	count := syncCount.Load()
	assert.GreaterOrEqual(t, count, int32(2), "daemon should continue despite sync errors")
}

func TestRunDaemon_GracefulShutdown(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	db := &mockBearDB{noteUUIDs: []string{"note-1"}}
	hub := &mockHubClient{}
	bridge := newTestBridge(db, hub, statePath)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- runDaemon(ctx, bridge, 1*time.Hour, testLogger())
	}()

	// Give time for initial sync.
	time.Sleep(50 * time.Millisecond)

	// Cancel context (simulates SIGTERM/SIGINT).
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not shut down within timeout")
	}
}

func TestLoadConfig_SyncInterval_Default(t *testing.T) {
	t.Setenv("BRIDGE_HUB_URL", "http://localhost:8080")
	t.Setenv("BRIDGE_HUB_TOKEN", "test-token")
	t.Setenv("BEAR_TOKEN", "bear-token")
	t.Setenv("BRIDGE_SYNC_INTERVAL", "")

	cfg, err := loadConfig()
	require.NoError(t, err)
	assert.Equal(t, defaultSyncInterval, cfg.syncInterval)
}

func TestLoadConfig_SyncInterval_Custom(t *testing.T) {
	t.Setenv("BRIDGE_HUB_URL", "http://localhost:8080")
	t.Setenv("BRIDGE_HUB_TOKEN", "test-token")
	t.Setenv("BEAR_TOKEN", "bear-token")
	t.Setenv("BRIDGE_SYNC_INTERVAL", "60")

	cfg, err := loadConfig()
	require.NoError(t, err)
	assert.Equal(t, 60*time.Second, cfg.syncInterval)
}

func TestLoadConfig_SyncInterval_Invalid(t *testing.T) {
	t.Setenv("BRIDGE_HUB_URL", "http://localhost:8080")
	t.Setenv("BRIDGE_HUB_TOKEN", "test-token")
	t.Setenv("BEAR_TOKEN", "bear-token")

	tests := []struct {
		name  string
		value string
	}{
		{"non-numeric", "abc"},
		{"zero", "0"},
		{"negative", "-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("BRIDGE_SYNC_INTERVAL", tt.value)
			_, err := loadConfig()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "BRIDGE_SYNC_INTERVAL")
		})
	}
}

// daemonCountingDB embeds mockBearDB and counts Notes() calls.
type daemonCountingDB struct {
	mockBearDB
	notesCallCount *atomic.Int32
}

func (d *daemonCountingDB) Notes(_ context.Context, _ float64) ([]mapper.BearNoteRow, error) {
	d.notesCallCount.Add(1)
	return nil, nil
}

// daemonFailingDB embeds mockBearDB but overrides Notes() to fail.
type daemonFailingDB struct {
	mockBearDB
	failUntil int32
	syncCount *atomic.Int32
}

func (f *daemonFailingDB) Notes(_ context.Context, _ float64) ([]mapper.BearNoteRow, error) {
	count := f.syncCount.Add(1)
	if count <= f.failUntil {
		return nil, fmt.Errorf("simulated db error")
	}
	return nil, nil
}

func (f *daemonFailingDB) AllNoteUUIDs(_ context.Context) ([]string, error) {
	return nil, nil
}

func (f *daemonFailingDB) AllTagUUIDs(_ context.Context) ([]string, error) {
	return nil, nil
}

func (f *daemonFailingDB) AllAttachmentUUIDs(_ context.Context) ([]string, error) {
	return nil, nil
}

func (f *daemonFailingDB) AllBacklinkUUIDs(_ context.Context) ([]string, error) {
	return nil, nil
}

func (f *daemonFailingDB) NoteTags(_ context.Context) ([]beardb.NoteTagPair, error) {
	return nil, nil
}

func (f *daemonFailingDB) PinnedNoteTags(_ context.Context) ([]beardb.NoteTagPair, error) {
	return nil, nil
}
