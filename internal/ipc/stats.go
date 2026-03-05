package ipc

import (
	"sync"
	"time"
)

// defaultLogBufferSize is the number of log entries retained in the ring buffer.
const defaultLogBufferSize = 500

// StatsTracker tracks sync statistics and state for the IPC status provider.
type StatsTracker struct {
	mu             sync.RWMutex
	state          string // "idle", "syncing", "error"
	lastSync       time.Time
	lastError      string
	notesSynced    int
	tagsSynced     int
	queueProcessed int
	lastDurationMs int64
	version        string

	logBuf  []LogEntry
	logSize int

	queueItems []QueueStatusItem

	syncTrigger chan struct{}
	shutdownCh  chan struct{}
}

// NewStatsTracker creates a new StatsTracker with the given log buffer size.
func NewStatsTracker(logBufferSize int) *StatsTracker {
	if logBufferSize <= 0 {
		logBufferSize = defaultLogBufferSize
	}
	return &StatsTracker{
		state:       "idle",
		logBuf:      make([]LogEntry, 0, logBufferSize),
		logSize:     logBufferSize,
		syncTrigger: make(chan struct{}, 1),
		shutdownCh:  make(chan struct{}, 1),
	}
}

// SetVersion sets the bridge version string reported in status responses.
func (st *StatsTracker) SetVersion(v string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.version = v
}

// GetStatus returns the current status for IPC clients.
func (st *StatsTracker) GetStatus() StatusResponse {
	st.mu.RLock()
	defer st.mu.RUnlock()

	var lastSyncStr string
	if !st.lastSync.IsZero() {
		lastSyncStr = st.lastSync.UTC().Format(time.RFC3339)
	}

	return StatusResponse{
		State:     st.state,
		LastSync:  lastSyncStr,
		LastError: st.lastError,
		Stats: SyncStats{
			NotesSynced:    st.notesSynced,
			TagsSynced:     st.tagsSynced,
			QueueProcessed: st.queueProcessed,
			LastDurationMs: st.lastDurationMs,
		},
		Version: st.version,
	}
}

// TriggerSync sends a non-blocking signal to trigger an immediate sync.
func (st *StatsTracker) TriggerSync() {
	select {
	case st.syncTrigger <- struct{}{}:
	default:
		// Already triggered, skip.
	}
}

// SyncTriggered returns the channel that receives sync trigger signals.
func (st *StatsTracker) SyncTriggered() <-chan struct{} {
	return st.syncTrigger
}

// GetLogs returns the last n log entries from the ring buffer.
func (st *StatsTracker) GetLogs(n int) []LogEntry {
	st.mu.RLock()
	defer st.mu.RUnlock()

	if n <= 0 || len(st.logBuf) == 0 {
		return []LogEntry{}
	}

	if n > len(st.logBuf) {
		n = len(st.logBuf)
	}

	start := len(st.logBuf) - n
	result := make([]LogEntry, n)
	copy(result, st.logBuf[start:])
	return result
}

// SetSyncing marks the bridge as currently syncing.
func (st *StatsTracker) SetSyncing() {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.state = "syncing"
}

// SetIdle marks the bridge as idle after a successful sync.
func (st *StatsTracker) SetIdle() {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.state = "idle"
	st.lastError = ""
	st.lastSync = time.Now()
}

// SetError marks the bridge as in error state.
func (st *StatsTracker) SetError(errMsg string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.state = "error"
	st.lastError = errMsg
}

// RecordSync records stats from the most recent sync cycle.
func (st *StatsTracker) RecordSync(notesSynced, tagsSynced, queueProcessed int, durationMs int64) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.notesSynced = notesSynced
	st.tagsSynced = tagsSynced
	st.queueProcessed = queueProcessed
	st.lastDurationMs = durationMs
}

// GetQueueStatus returns the current queue items snapshot for IPC clients.
func (st *StatsTracker) GetQueueStatus() QueueStatusResponse {
	st.mu.RLock()
	defer st.mu.RUnlock()

	items := make([]QueueStatusItem, len(st.queueItems))
	copy(items, st.queueItems)
	return QueueStatusResponse{Items: items}
}

// SetQueueItems replaces the current queue items snapshot (called when items are leased).
func (st *StatsTracker) SetQueueItems(items []QueueStatusItem) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.queueItems = make([]QueueStatusItem, len(items))
	copy(st.queueItems, items)
}

// UpdateQueueItemStatus updates the status of a specific queue item by ID.
func (st *StatsTracker) UpdateQueueItemStatus(queueID int64, status string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	for i := range st.queueItems {
		if st.queueItems[i].ID == queueID {
			st.queueItems[i].Status = status
			return
		}
	}
}

// ClearQueueItems removes all queue items (called after sync cycle completes).
func (st *StatsTracker) ClearQueueItems() {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.queueItems = nil
}

// RequestShutdown sends a non-blocking signal to request daemon shutdown.
func (st *StatsTracker) RequestShutdown() {
	select {
	case st.shutdownCh <- struct{}{}:
	default:
	}
}

// ShutdownRequested returns the channel that receives shutdown request signals.
func (st *StatsTracker) ShutdownRequested() <-chan struct{} {
	return st.shutdownCh
}

// AddLog appends a log entry to the ring buffer.
func (st *StatsTracker) AddLog(entry LogEntry) {
	st.mu.Lock()
	defer st.mu.Unlock()

	if len(st.logBuf) >= st.logSize {
		// Shift buffer: drop oldest entry.
		copy(st.logBuf, st.logBuf[1:])
		st.logBuf[len(st.logBuf)-1] = entry
	} else {
		st.logBuf = append(st.logBuf, entry)
	}
}
