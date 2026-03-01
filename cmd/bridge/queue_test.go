package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/romancha/bear-sync/internal/beardb"
	"github.com/romancha/bear-sync/internal/models"
)

// newQueueBridge creates a Bridge configured for queue processing tests.
func newQueueBridge(
	db *mockBearDB, hub *mockHubClient, xcall *mockXCallback, statePath string,
) *Bridge {
	b := NewBridge(db, hub, xcall, "test-bear-token", statePath, "", testLogger())
	b.sleepFn = func(_ time.Duration) {} // no-op sleep for tests
	return b
}

func TestProcessQueue_NoItems(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")
	require.NoError(t, saveState(statePath, &BridgeState{LastSyncAt: 100}))

	db := &mockBearDB{}
	hub := &mockHubClient{}
	xcall := &mockXCallback{}
	bridge := newQueueBridge(db, hub, xcall, statePath)

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	assert.Empty(t, hub.ackItems)
	assert.Empty(t, xcall.calls)
}

func TestProcessQueue_NilXCallback(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")
	require.NoError(t, saveState(statePath, &BridgeState{LastSyncAt: 100}))

	db := &mockBearDB{}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{ID: 1, Action: "create", Payload: `{"title":"Test"}`},
		},
	}
	bridge := newTestBridge(db, hub, statePath) // nil xcall

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	// No ack because xcall is nil — queue processing skipped entirely.
	assert.Empty(t, hub.ackItems)
}

func TestProcessQueue_CreateAction(t *testing.T) {
	db := &mockBearDB{
		notesByUUID: map[string]*beardb.NoteBasicInfo{
			"bear-uuid-1": {UUID: "bear-uuid-1", Title: "New Note", Body: "Hello"},
		},
	}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             1,
				IdempotencyKey: "idem-1",
				Action:         "create",
				NoteID:         "hub-uuid-1",
				Payload:        `{"title":"New Note","body":"Hello","tags":["tag1"]}`,
			},
		},
	}
	xcall := &mockXCallback{createBearID: "bear-uuid-1"}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	// Verify xcall was called.
	require.Len(t, xcall.calls, 1)
	assert.Equal(t, "create", xcall.calls[0].action)
	assert.Equal(t, "New Note", xcall.calls[0].title)
	assert.Equal(t, "Hello", xcall.calls[0].body)
	assert.Equal(t, []string{"tag1"}, xcall.calls[0].tags)

	// Verify ack.
	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, int64(1), hub.ackItems[0].QueueID)
	assert.Equal(t, "idem-1", hub.ackItems[0].IdempotencyKey)
	assert.Equal(t, "applied", hub.ackItems[0].Status)
	assert.Equal(t, "bear-uuid-1", hub.ackItems[0].BearID)
}

func TestProcessQueue_CreateFallback(t *testing.T) {
	db := &mockBearDB{
		recentNotes: []beardb.NoteBasicInfo{
			{UUID: "fallback-uuid", Title: "Fallback Note"},
		},
	}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             2,
				IdempotencyKey: "idem-2",
				Action:         "create",
				Payload:        `{"title":"Fallback Note","body":"body"}`,
			},
		},
	}
	xcall := &mockXCallback{createBearID: ""} // empty UUID triggers fallback
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "applied", hub.ackItems[0].Status)
	assert.Equal(t, "fallback-uuid", hub.ackItems[0].BearID)
}

func TestProcessQueue_CreateFallbackAmbiguous(t *testing.T) {
	db := &mockBearDB{
		recentNotes: []beardb.NoteBasicInfo{
			{UUID: "uuid-1", Title: "Same Title"},
			{UUID: "uuid-2", Title: "Same Title"},
		},
	}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             3,
				IdempotencyKey: "idem-3",
				Action:         "create",
				Payload:        `{"title":"Same Title","body":"body"}`,
			},
		},
	}
	xcall := &mockXCallback{createBearID: ""}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "failed", hub.ackItems[0].Status)
	assert.Contains(t, hub.ackItems[0].Error, "ambiguous")
}

func TestProcessQueue_UpdateAction(t *testing.T) {
	db := &mockBearDB{
		notesByUUID: map[string]*beardb.NoteBasicInfo{
			"bear-note-1": {UUID: "bear-note-1", Title: "Note", Body: "old body"},
		},
	}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             4,
				IdempotencyKey: "idem-4",
				Action:         "update",
				NoteID:         "bear-note-1",
				Payload:        `{"body":"new body"}`,
			},
		},
	}
	xcall := &mockXCallback{}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	require.Len(t, xcall.calls, 1)
	assert.Equal(t, "update", xcall.calls[0].action)
	assert.Equal(t, "bear-note-1", xcall.calls[0].bearID)
	assert.Equal(t, "new body", xcall.calls[0].body)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "applied", hub.ackItems[0].Status)
}

func TestProcessQueue_UpdateDuplicateSafe(t *testing.T) {
	db := &mockBearDB{
		notesByUUID: map[string]*beardb.NoteBasicInfo{
			"bear-note-1": {UUID: "bear-note-1", Title: "Note", Body: "desired body"},
		},
	}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             5,
				IdempotencyKey: "idem-5",
				Action:         "update",
				NoteID:         "bear-note-1",
				Payload:        `{"body":"desired body"}`,
			},
		},
	}
	xcall := &mockXCallback{}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	// xcall should NOT be called — body already matches.
	assert.Empty(t, xcall.calls)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "applied", hub.ackItems[0].Status)
}

func TestProcessQueue_AddTagAction(t *testing.T) {
	db := &mockBearDB{
		notesByUUID: map[string]*beardb.NoteBasicInfo{
			"bear-note-1": {UUID: "bear-note-1", Title: "Note"},
		},
		tagsByNote: map[string][]string{
			"bear-note-1": {"existing-tag"},
		},
	}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             6,
				IdempotencyKey: "idem-6",
				Action:         "add_tag",
				NoteID:         "bear-note-1",
				Payload:        `{"tag":"new-tag"}`,
			},
		},
	}
	xcall := &mockXCallback{}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	require.Len(t, xcall.calls, 1)
	assert.Equal(t, "add_tag", xcall.calls[0].action)
	assert.Equal(t, "bear-note-1", xcall.calls[0].bearID)
	assert.Equal(t, "new-tag", xcall.calls[0].tag)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "applied", hub.ackItems[0].Status)
}

func TestProcessQueue_AddTagDuplicateSafe(t *testing.T) {
	db := &mockBearDB{
		notesByUUID: map[string]*beardb.NoteBasicInfo{
			"bear-note-1": {UUID: "bear-note-1", Title: "Note"},
		},
		tagsByNote: map[string][]string{
			"bear-note-1": {"already-has-tag"},
		},
	}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             7,
				IdempotencyKey: "idem-7",
				Action:         "add_tag",
				NoteID:         "bear-note-1",
				Payload:        `{"tag":"already-has-tag"}`,
			},
		},
	}
	xcall := &mockXCallback{}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	// xcall should NOT be called — tag already exists.
	assert.Empty(t, xcall.calls)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "applied", hub.ackItems[0].Status)
}

func TestProcessQueue_TrashAction(t *testing.T) {
	db := &mockBearDB{
		notesByUUID: map[string]*beardb.NoteBasicInfo{
			"bear-note-1": {UUID: "bear-note-1", Title: "Note", Trashed: 0},
		},
	}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             8,
				IdempotencyKey: "idem-8",
				Action:         "trash",
				NoteID:         "bear-note-1",
				Payload:        `{"action":"trash"}`,
			},
		},
	}
	xcall := &mockXCallback{}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	require.Len(t, xcall.calls, 1)
	assert.Equal(t, "trash", xcall.calls[0].action)
	assert.Equal(t, "bear-note-1", xcall.calls[0].bearID)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "applied", hub.ackItems[0].Status)
}

func TestProcessQueue_TrashDuplicateSafe(t *testing.T) {
	db := &mockBearDB{
		notesByUUID: map[string]*beardb.NoteBasicInfo{
			"bear-note-1": {UUID: "bear-note-1", Title: "Note", Trashed: 1},
		},
	}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             9,
				IdempotencyKey: "idem-9",
				Action:         "trash",
				NoteID:         "bear-note-1",
				Payload:        `{"action":"trash"}`,
			},
		},
	}
	xcall := &mockXCallback{}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	// xcall should NOT be called — note already trashed.
	assert.Empty(t, xcall.calls)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "applied", hub.ackItems[0].Status)
}

func TestProcessQueue_FailedItemDoesNotBlockOthers(t *testing.T) {
	db := &mockBearDB{
		notesByUUID: map[string]*beardb.NoteBasicInfo{
			"bear-note-2": {UUID: "bear-note-2", Title: "Note 2", Body: "old"},
		},
	}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             10,
				IdempotencyKey: "idem-10",
				Action:         "update",
				NoteID:         "nonexistent", // Will fail — can't find note.
				Payload:        `{"body":"new"}`,
			},
			{
				ID:             11,
				IdempotencyKey: "idem-11",
				Action:         "update",
				NoteID:         "bear-note-2",
				Payload:        `{"body":"updated"}`,
			},
		},
	}
	xcall := &mockXCallback{}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	require.Len(t, hub.ackItems, 2)

	// First item failed.
	assert.Equal(t, "failed", hub.ackItems[0].Status)
	assert.Contains(t, hub.ackItems[0].Error, "cannot resolve bear UUID")

	// Second item should still succeed.
	assert.Equal(t, "applied", hub.ackItems[1].Status)
}

func TestProcessQueue_UnknownAction(t *testing.T) {
	db := &mockBearDB{}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             12,
				IdempotencyKey: "idem-12",
				Action:         "unknown_action",
				Payload:        `{}`,
			},
		},
	}
	xcall := &mockXCallback{}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "failed", hub.ackItems[0].Status)
	assert.Contains(t, hub.ackItems[0].Error, "unknown action")
}

func TestProcessQueue_CreateWithXCallError(t *testing.T) {
	db := &mockBearDB{}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             13,
				IdempotencyKey: "idem-13",
				Action:         "create",
				Payload:        `{"title":"Test","body":"body"}`,
			},
		},
	}
	xcall := &mockXCallback{createErr: fmt.Errorf("xcall failed")}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "failed", hub.ackItems[0].Status)
	assert.Contains(t, hub.ackItems[0].Error, "xcall failed")
}

func TestProcessQueue_BearIDFromPayload(t *testing.T) {
	db := &mockBearDB{
		notesByUUID: map[string]*beardb.NoteBasicInfo{
			"payload-bear-id": {UUID: "payload-bear-id", Title: "Note", Body: "old"},
		},
	}

	payloadBytes, _ := json.Marshal(map[string]string{"body": "new", "bear_id": "payload-bear-id"})
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             14,
				IdempotencyKey: "idem-14",
				Action:         "update",
				NoteID:         "hub-uuid",
				Payload:        string(payloadBytes),
			},
		},
	}
	xcall := &mockXCallback{}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	// Should have used bear_id from payload.
	require.Len(t, xcall.calls, 1)
	assert.Equal(t, "payload-bear-id", xcall.calls[0].bearID)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "applied", hub.ackItems[0].Status)
}

func TestCountByStatus(t *testing.T) {
	items := []models.SyncAckItem{
		{Status: "applied"},
		{Status: "applied"},
		{Status: "failed"},
	}
	assert.Equal(t, 2, countByStatus(items, "applied"))
	assert.Equal(t, 1, countByStatus(items, "failed"))
	assert.Equal(t, 0, countByStatus(items, "pending"))
}
