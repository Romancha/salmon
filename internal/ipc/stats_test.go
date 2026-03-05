package ipc

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatsTracker_InitialState(t *testing.T) {
	st := NewStatsTracker(0)
	status := st.GetStatus()
	assert.Equal(t, "idle", status.State)
	assert.Empty(t, status.LastSync)
	assert.Empty(t, status.LastError)
	assert.Equal(t, 0, status.Stats.NotesSynced)
}

func TestStatsTracker_SetSyncing(t *testing.T) {
	st := NewStatsTracker(0)
	st.SetSyncing()
	assert.Equal(t, "syncing", st.GetStatus().State)
}

func TestStatsTracker_SetIdle(t *testing.T) {
	st := NewStatsTracker(0)
	st.SetSyncing()
	st.SetIdle()
	status := st.GetStatus()
	assert.Equal(t, "idle", status.State)
	assert.NotEmpty(t, status.LastSync)
}

func TestStatsTracker_SetError(t *testing.T) {
	st := NewStatsTracker(0)
	st.SetError("connection failed")
	status := st.GetStatus()
	assert.Equal(t, "error", status.State)
	assert.Equal(t, "connection failed", status.LastError)
}

func TestStatsTracker_RecordSync(t *testing.T) {
	st := NewStatsTracker(0)

	st.RecordSync(10, 5, 2, 1200)
	status := st.GetStatus()
	assert.Equal(t, 10, status.Stats.NotesSynced)
	assert.Equal(t, 5, status.Stats.TagsSynced)
	assert.Equal(t, 2, status.Stats.QueueProcessed)
	assert.Equal(t, int64(1200), status.Stats.LastDurationMs)

	// Per-cycle (overwrites previous values).
	st.RecordSync(3, 1, 0, 800)
	status = st.GetStatus()
	assert.Equal(t, 3, status.Stats.NotesSynced)
	assert.Equal(t, 1, status.Stats.TagsSynced)
	assert.Equal(t, 0, status.Stats.QueueProcessed)
	assert.Equal(t, int64(800), status.Stats.LastDurationMs)
}

func TestStatsTracker_AddLog(t *testing.T) {
	st := NewStatsTracker(3)

	st.AddLog(LogEntry{Time: "t1", Level: "info", Msg: "first"})
	st.AddLog(LogEntry{Time: "t2", Level: "warn", Msg: "second"})

	logs := st.GetLogs(10)
	assert.Len(t, logs, 2)
	assert.Equal(t, "first", logs[0].Msg)
	assert.Equal(t, "second", logs[1].Msg)
}

func TestStatsTracker_AddLog_RingBuffer(t *testing.T) {
	st := NewStatsTracker(3)

	st.AddLog(LogEntry{Msg: "a"})
	st.AddLog(LogEntry{Msg: "b"})
	st.AddLog(LogEntry{Msg: "c"})
	st.AddLog(LogEntry{Msg: "d"}) // Should evict "a".

	logs := st.GetLogs(10)
	require.Len(t, logs, 3)
	assert.Equal(t, "b", logs[0].Msg)
	assert.Equal(t, "c", logs[1].Msg)
	assert.Equal(t, "d", logs[2].Msg)
}

func TestStatsTracker_GetLogs_LimitN(t *testing.T) {
	st := NewStatsTracker(10)

	for i := range 5 {
		st.AddLog(LogEntry{Msg: string(rune('a' + i))})
	}

	logs := st.GetLogs(2)
	require.Len(t, logs, 2)
	assert.Equal(t, "d", logs[0].Msg)
	assert.Equal(t, "e", logs[1].Msg)
}

func TestStatsTracker_GetLogs_Empty(t *testing.T) {
	st := NewStatsTracker(10)
	logs := st.GetLogs(5)
	assert.Nil(t, logs)
}

func TestStatsTracker_GetLogs_ZeroN(t *testing.T) {
	st := NewStatsTracker(10)
	st.AddLog(LogEntry{Msg: "a"})
	logs := st.GetLogs(0)
	assert.Nil(t, logs)
}

func TestStatsTracker_TriggerSync(t *testing.T) {
	st := NewStatsTracker(0)

	// First trigger should succeed.
	st.TriggerSync()

	select {
	case <-st.SyncTriggered():
		// Expected.
	default:
		t.Fatal("expected sync trigger signal")
	}
}

func TestStatsTracker_TriggerSync_NonBlocking(t *testing.T) {
	st := NewStatsTracker(0)

	// Double trigger should not block (channel buffer of 1).
	st.TriggerSync()
	st.TriggerSync()

	select {
	case <-st.SyncTriggered():
		// Expected.
	default:
		t.Fatal("expected sync trigger signal")
	}
}

func TestStatsTracker_ConcurrentAccess(t *testing.T) {
	st := NewStatsTracker(100)

	var wg sync.WaitGroup
	wg.Add(4)

	go func() {
		defer wg.Done()
		for range 100 {
			st.SetSyncing()
			st.SetIdle()
		}
	}()

	go func() {
		defer wg.Done()
		for range 100 {
			st.RecordSync(1, 1, 1, 100)
		}
	}()

	go func() {
		defer wg.Done()
		for range 100 {
			st.AddLog(LogEntry{Msg: "test"})
		}
	}()

	go func() {
		defer wg.Done()
		for range 100 {
			st.GetStatus()
			st.GetLogs(10)
		}
	}()

	wg.Wait()

	status := st.GetStatus()
	assert.Equal(t, 1, status.Stats.NotesSynced)
}

func TestStatsTracker_QueueStatus_Empty(t *testing.T) {
	st := NewStatsTracker(0)
	resp := st.GetQueueStatus()
	assert.Empty(t, resp.Items)
	assert.Empty(t, resp.Error)
}

func TestStatsTracker_SetQueueItems(t *testing.T) {
	st := NewStatsTracker(0)
	items := []QueueStatusItem{
		{ID: 1, Action: "create", NoteTitle: "Note 1", Status: "processing", CreatedAt: "2026-03-04T12:00:00Z"},
		{ID: 2, Action: "update", NoteTitle: "Note 2", Status: "processing"},
	}
	st.SetQueueItems(items)

	resp := st.GetQueueStatus()
	require.Len(t, resp.Items, 2)
	assert.Equal(t, int64(1), resp.Items[0].ID)
	assert.Equal(t, "create", resp.Items[0].Action)
	assert.Equal(t, "Note 1", resp.Items[0].NoteTitle)
	assert.Equal(t, "processing", resp.Items[0].Status)
	assert.Equal(t, "2026-03-04T12:00:00Z", resp.Items[0].CreatedAt)
}

func TestStatsTracker_SetQueueItems_ReplacesExisting(t *testing.T) {
	st := NewStatsTracker(0)
	st.SetQueueItems([]QueueStatusItem{{ID: 1, Action: "create"}})
	st.SetQueueItems([]QueueStatusItem{{ID: 2, Action: "update"}, {ID: 3, Action: "trash"}})

	resp := st.GetQueueStatus()
	require.Len(t, resp.Items, 2)
	assert.Equal(t, int64(2), resp.Items[0].ID)
	assert.Equal(t, int64(3), resp.Items[1].ID)
}

func TestStatsTracker_UpdateQueueItemStatus(t *testing.T) {
	st := NewStatsTracker(0)
	st.SetQueueItems([]QueueStatusItem{
		{ID: 1, Action: "create", Status: "processing"},
		{ID: 2, Action: "update", Status: "processing"},
	})

	st.UpdateQueueItemStatus(1, "applied")

	resp := st.GetQueueStatus()
	assert.Equal(t, "applied", resp.Items[0].Status)
	assert.Equal(t, "processing", resp.Items[1].Status)
}

func TestStatsTracker_UpdateQueueItemStatus_NotFound(t *testing.T) {
	st := NewStatsTracker(0)
	st.SetQueueItems([]QueueStatusItem{{ID: 1, Action: "create", Status: "processing"}})

	// Should not panic when ID not found.
	st.UpdateQueueItemStatus(999, "applied")

	resp := st.GetQueueStatus()
	assert.Equal(t, "processing", resp.Items[0].Status)
}

func TestStatsTracker_ClearQueueItems(t *testing.T) {
	st := NewStatsTracker(0)
	st.SetQueueItems([]QueueStatusItem{{ID: 1, Action: "create"}})
	st.ClearQueueItems()

	resp := st.GetQueueStatus()
	assert.Empty(t, resp.Items)
}

func TestStatsTracker_QueueStatus_IsCopy(t *testing.T) {
	st := NewStatsTracker(0)
	st.SetQueueItems([]QueueStatusItem{{ID: 1, Action: "create", Status: "processing"}})

	resp := st.GetQueueStatus()
	resp.Items[0].Status = "mutated"

	// Original should be unchanged.
	resp2 := st.GetQueueStatus()
	assert.Equal(t, "processing", resp2.Items[0].Status)
}

func TestNewStatsTracker_DefaultBufferSize(t *testing.T) {
	st := NewStatsTracker(0)
	// Fill beyond default buffer size.
	for i := range defaultLogBufferSize + 10 {
		st.AddLog(LogEntry{Msg: string(rune('a' + (i % 26)))})
	}
	logs := st.GetLogs(defaultLogBufferSize + 10)
	assert.Len(t, logs, defaultLogBufferSize)
}
