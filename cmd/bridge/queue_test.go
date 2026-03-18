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

	"github.com/romancha/salmon/internal/beardb"
	"github.com/romancha/salmon/internal/ipc"
	"github.com/romancha/salmon/internal/models"
	"github.com/romancha/salmon/internal/xcallback"
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
	bridge := newTestBridge(db, hub, statePath) // nil bear-xcall

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	// No ack because bear-xcall is nil — queue processing skipped entirely.
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
				ConsumerID:     "testapp",
			},
		},
	}
	xcall := &mockXCallback{createBearID: "bear-uuid-1"}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	// Verify bear-xcall was called.
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

	// bear-xcall should NOT be called — body already matches.
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
				ConsumerID:     "myapp",
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

	// bear-xcall should NOT be called — tag already exists.
	assert.Empty(t, xcall.calls)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "applied", hub.ackItems[0].Status)
}

func TestProcessQueue_TrashAction(t *testing.T) { //nolint:dupl // trash test mirrors archive test by design
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

	// bear-xcall should NOT be called — note already trashed.
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
	xcall := &mockXCallback{createErr: fmt.Errorf("bear-xcall failed")}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "failed", hub.ackItems[0].Status)
	assert.Contains(t, hub.ackItems[0].Error, "bear-xcall failed")
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

func TestProcessQueue_ConflictCreatesConflictNote(t *testing.T) {
	db := &mockBearDB{
		notesByUUID: map[string]*beardb.NoteBasicInfo{
			"bear-note-1": {UUID: "bear-note-1", Title: "Original Note", Body: "user body"},
		},
	}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             20,
				IdempotencyKey: "idem-20",
				Action:         "update",
				NoteID:         "bear-note-1",
				Payload:        `{"body":"consumer body","bear_id":"bear-note-1"}`,
				NoteSyncStatus: "conflict",
			},
		},
	}
	xcall := &mockXCallback{createBearID: "conflict-bear-id"}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	// Should have called bear-xcall Create with conflict title.
	require.Len(t, xcall.calls, 1)
	assert.Equal(t, "create", xcall.calls[0].action)
	assert.Equal(t, "[Conflict] Original Note", xcall.calls[0].title)
	assert.Equal(t, "consumer body", xcall.calls[0].body)

	// Should have acked as applied with ConflictResolved set.
	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "applied", hub.ackItems[0].Status)
	assert.Equal(t, "", hub.ackItems[0].BearID)
	assert.True(t, hub.ackItems[0].ConflictResolved, "conflict resolution must be signalled to hub")
}

func TestProcessQueue_ConflictWithTitleInPayload(t *testing.T) {
	db := &mockBearDB{}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             21,
				IdempotencyKey: "idem-21",
				Action:         "update",
				NoteID:         "hub-note-1",
				Payload:        `{"title":"New Note","body":"consumer body"}`,
				NoteSyncStatus: "conflict",
			},
		},
	}
	xcall := &mockXCallback{createBearID: "conflict-bear-id"}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	require.Len(t, xcall.calls, 1)
	assert.Equal(t, "[Conflict] New Note", xcall.calls[0].title)
	assert.Equal(t, "consumer body", xcall.calls[0].body)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "applied", hub.ackItems[0].Status)
	assert.True(t, hub.ackItems[0].ConflictResolved, "conflict resolution must be signalled to hub")
}

func TestProcessQueue_ConflictXCallFails(t *testing.T) {
	db := &mockBearDB{
		notesByUUID: map[string]*beardb.NoteBasicInfo{
			"bear-note-1": {UUID: "bear-note-1", Title: "Note", Body: "body"},
		},
	}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             22,
				IdempotencyKey: "idem-22",
				Action:         "update",
				NoteID:         "bear-note-1",
				Payload:        `{"body":"consumer body"}`,
				NoteSyncStatus: "conflict",
			},
		},
	}
	xcall := &mockXCallback{createErr: fmt.Errorf("bear-xcall unavailable")}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "failed", hub.ackItems[0].Status)
	assert.Contains(t, hub.ackItems[0].Error, "create conflict note")
}

func TestProcessQueue_NonConflictProcessedNormally(t *testing.T) {
	db := &mockBearDB{
		notesByUUID: map[string]*beardb.NoteBasicInfo{
			"bear-note-1": {UUID: "bear-note-1", Title: "Note", Body: "old"},
		},
	}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             23,
				IdempotencyKey: "idem-23",
				Action:         "update",
				NoteID:         "bear-note-1",
				Payload:        `{"body":"new body"}`,
				NoteSyncStatus: "synced",
			},
		},
	}
	xcall := &mockXCallback{}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	// Should process normally (update, not create conflict note).
	require.Len(t, xcall.calls, 1)
	assert.Equal(t, "update", xcall.calls[0].action)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "applied", hub.ackItems[0].Status)
}

func TestProcessQueue_ConsumerIDDoesNotAffectProcessing(t *testing.T) {
	db := &mockBearDB{
		notesByUUID: map[string]*beardb.NoteBasicInfo{
			"bear-note-1": {UUID: "bear-note-1", Title: "Note 1", Body: "old body 1"},
			"bear-note-2": {UUID: "bear-note-2", Title: "Note 2", Body: "old body 2"},
			"bear-note-3": {UUID: "bear-note-3", Title: "Note 3", Trashed: 0},
		},
	}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             30,
				IdempotencyKey: "idem-30",
				Action:         "update",
				NoteID:         "bear-note-1",
				Payload:        `{"body":"new body 1"}`,
				ConsumerID:     "testapp",
			},
			{
				ID:             31,
				IdempotencyKey: "idem-31",
				Action:         "update",
				NoteID:         "bear-note-2",
				Payload:        `{"body":"new body 2"}`,
				ConsumerID:     "myapp",
			},
			{
				ID:             32,
				IdempotencyKey: "idem-32",
				Action:         "trash",
				NoteID:         "bear-note-3",
				Payload:        `{"action":"trash"}`,
				ConsumerID:     "", // empty consumer_id (legacy or bridge-created)
			},
		},
	}
	xcall := &mockXCallback{}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	// All items processed regardless of consumer_id value (including empty).
	require.Len(t, xcall.calls, 3)
	assert.Equal(t, "update", xcall.calls[0].action)
	assert.Equal(t, "bear-note-1", xcall.calls[0].bearID)
	assert.Equal(t, "update", xcall.calls[1].action)
	assert.Equal(t, "bear-note-2", xcall.calls[1].bearID)
	assert.Equal(t, "trash", xcall.calls[2].action)
	assert.Equal(t, "bear-note-3", xcall.calls[2].bearID)

	// All acked as applied.
	require.Len(t, hub.ackItems, 3)
	assert.Equal(t, "applied", hub.ackItems[0].Status)
	assert.Equal(t, "applied", hub.ackItems[1].Status)
	assert.Equal(t, "applied", hub.ackItems[2].Status)
}

// --- add_file tests ---

func TestProcessQueue_AddFileAction(t *testing.T) {
	db := &mockBearDB{
		notesByUUID: map[string]*beardb.NoteBasicInfo{
			"bear-note-1": {UUID: "bear-note-1", Title: "Note"},
		},
	}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             40,
				IdempotencyKey: "idem-40",
				Action:         "add_file",
				NoteID:         "bear-note-1",
				Payload:        `{"attachment_id":"att-1","filename":"photo.jpg","bear_id":"bear-note-1"}`,
			},
		},
		downloadData: map[string][]byte{
			"att-1": []byte("image-data-here"),
		},
	}
	xcall := &mockXCallback{}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	require.Len(t, xcall.calls, 1)
	assert.Equal(t, "add_file", xcall.calls[0].action)
	assert.Equal(t, "bear-note-1", xcall.calls[0].bearID)
	assert.Equal(t, "photo.jpg", xcall.calls[0].filename)
	assert.Equal(t, []byte("image-data-here"), xcall.calls[0].fileData)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "applied", hub.ackItems[0].Status)
}

func TestProcessQueue_AddFileDownloadError(t *testing.T) {
	db := &mockBearDB{
		notesByUUID: map[string]*beardb.NoteBasicInfo{
			"bear-note-1": {UUID: "bear-note-1", Title: "Note"},
		},
	}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             41,
				IdempotencyKey: "idem-41",
				Action:         "add_file",
				NoteID:         "bear-note-1",
				Payload:        `{"attachment_id":"att-missing","filename":"photo.jpg","bear_id":"bear-note-1"}`,
			},
		},
		downloadErr: fmt.Errorf("hub unavailable"),
	}
	xcall := &mockXCallback{}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "failed", hub.ackItems[0].Status)
	assert.Contains(t, hub.ackItems[0].Error, "download attachment")
}

func TestProcessQueue_AddFileXCallError(t *testing.T) {
	db := &mockBearDB{
		notesByUUID: map[string]*beardb.NoteBasicInfo{
			"bear-note-1": {UUID: "bear-note-1", Title: "Note"},
		},
	}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             42,
				IdempotencyKey: "idem-42",
				Action:         "add_file",
				NoteID:         "bear-note-1",
				Payload:        `{"attachment_id":"att-1","filename":"photo.jpg","bear_id":"bear-note-1"}`,
			},
		},
		downloadData: map[string][]byte{
			"att-1": []byte("image-data"),
		},
	}
	xcall := &mockXCallback{addFileErr: fmt.Errorf("bear-xcall add-file failed")}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "failed", hub.ackItems[0].Status)
	assert.Contains(t, hub.ackItems[0].Error, "bear-xcall add-file")
}

func TestProcessQueue_AddFileInvalidPayload(t *testing.T) {
	db := &mockBearDB{}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             43,
				IdempotencyKey: "idem-43",
				Action:         "add_file",
				Payload:        `{invalid json`,
			},
		},
	}
	xcall := &mockXCallback{}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "failed", hub.ackItems[0].Status)
	assert.Contains(t, hub.ackItems[0].Error, "parse add_file payload")
}

func TestProcessQueue_AddFileTooLarge(t *testing.T) {
	db := &mockBearDB{
		notesByUUID: map[string]*beardb.NoteBasicInfo{
			"bear-note-1": {UUID: "bear-note-1", Title: "Note"},
		},
	}
	// 6 MB data — exceeds 5 MB limit.
	bigData := make([]byte, 6*1024*1024)
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             44,
				IdempotencyKey: "idem-44",
				Action:         "add_file",
				NoteID:         "bear-note-1",
				Payload:        `{"attachment_id":"att-big","filename":"huge.bin","bear_id":"bear-note-1"}`,
			},
		},
		downloadData: map[string][]byte{
			"att-big": bigData,
		},
	}
	xcall := &mockXCallback{}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "failed", hub.ackItems[0].Status)
	assert.Contains(t, hub.ackItems[0].Error, "too large")
	assert.Empty(t, xcall.calls) // xcall should NOT be called.
}

func TestProcessQueue_AddFileAlreadyApplied(t *testing.T) {
	db := &mockBearDB{
		notesByUUID: map[string]*beardb.NoteBasicInfo{
			"bear-note-1": {UUID: "bear-note-1", Title: "Note"},
		},
		filesByNote: map[string][]string{
			"bear-note-1": {"photo.jpg"},
		},
	}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             45,
				IdempotencyKey: "idem-45",
				Action:         "add_file",
				NoteID:         "bear-note-1",
				Payload:        `{"attachment_id":"att-1","filename":"photo.jpg","bear_id":"bear-note-1"}`,
			},
		},
	}
	xcall := &mockXCallback{}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	// xcall should NOT be called — file already exists on note.
	assert.Empty(t, xcall.calls)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "applied", hub.ackItems[0].Status)
}

// --- archive tests ---

func TestProcessQueue_ArchiveAction(t *testing.T) { //nolint:dupl // archive test mirrors trash test by design
	db := &mockBearDB{
		notesByUUID: map[string]*beardb.NoteBasicInfo{
			"bear-note-1": {UUID: "bear-note-1", Title: "Note", Archived: 0},
		},
	}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             50,
				IdempotencyKey: "idem-50",
				Action:         "archive",
				NoteID:         "bear-note-1",
				Payload:        `{"bear_id":"bear-note-1"}`,
			},
		},
	}
	xcall := &mockXCallback{}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	require.Len(t, xcall.calls, 1)
	assert.Equal(t, "archive", xcall.calls[0].action)
	assert.Equal(t, "bear-note-1", xcall.calls[0].bearID)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "applied", hub.ackItems[0].Status)
}

func TestProcessQueue_ArchiveAlreadyArchived(t *testing.T) {
	db := &mockBearDB{
		notesByUUID: map[string]*beardb.NoteBasicInfo{
			"bear-note-1": {UUID: "bear-note-1", Title: "Note", Archived: 1},
		},
	}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             51,
				IdempotencyKey: "idem-51",
				Action:         "archive",
				NoteID:         "bear-note-1",
				Payload:        `{"bear_id":"bear-note-1"}`,
			},
		},
	}
	xcall := &mockXCallback{}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	// bear-xcall should NOT be called — note already archived.
	assert.Empty(t, xcall.calls)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "applied", hub.ackItems[0].Status)
}

func TestProcessQueue_ArchiveXCallError(t *testing.T) {
	db := &mockBearDB{
		notesByUUID: map[string]*beardb.NoteBasicInfo{
			"bear-note-1": {UUID: "bear-note-1", Title: "Note", Archived: 0},
		},
	}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             52,
				IdempotencyKey: "idem-52",
				Action:         "archive",
				NoteID:         "bear-note-1",
				Payload:        `{"bear_id":"bear-note-1"}`,
			},
		},
	}
	xcall := &mockXCallback{archiveErr: fmt.Errorf("bear-xcall archive failed")}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "failed", hub.ackItems[0].Status)
	assert.Contains(t, hub.ackItems[0].Error, "bear-xcall archive")
}

func TestProcessQueue_ArchiveInvalidPayload(t *testing.T) {
	db := &mockBearDB{}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             53,
				IdempotencyKey: "idem-53",
				Action:         "archive",
				NoteID:         "nonexistent",
				Payload:        `{"bear_id":"nonexistent"}`,
			},
		},
	}
	xcall := &mockXCallback{}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "failed", hub.ackItems[0].Status)
	assert.Contains(t, hub.ackItems[0].Error, "cannot resolve bear UUID")
}

// --- rename_tag tests ---

func TestProcessQueue_RenameTagAction(t *testing.T) {
	db := &mockBearDB{}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             60,
				IdempotencyKey: "idem-60",
				Action:         "rename_tag",
				Payload:        `{"name":"old/tag","new_name":"new/tag"}`,
			},
		},
	}
	xcall := &mockXCallback{}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	require.Len(t, xcall.calls, 1)
	assert.Equal(t, "rename_tag", xcall.calls[0].action)
	assert.Equal(t, "old/tag", xcall.calls[0].oldName)
	assert.Equal(t, "new/tag", xcall.calls[0].newName)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "applied", hub.ackItems[0].Status)
}

func TestProcessQueue_RenameTagXCallError(t *testing.T) {
	db := &mockBearDB{}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             61,
				IdempotencyKey: "idem-61",
				Action:         "rename_tag",
				Payload:        `{"name":"old/tag","new_name":"new/tag"}`,
			},
		},
	}
	xcall := &mockXCallback{renameTagErr: fmt.Errorf("bear error: tag locked")}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "failed", hub.ackItems[0].Status)
	assert.Contains(t, hub.ackItems[0].Error, "bear-xcall rename-tag")
}

func TestProcessQueue_RenameTagInvalidPayload(t *testing.T) {
	db := &mockBearDB{}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             62,
				IdempotencyKey: "idem-62",
				Action:         "rename_tag",
				Payload:        `{invalid json`,
			},
		},
	}
	xcall := &mockXCallback{}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "failed", hub.ackItems[0].Status)
	assert.Contains(t, hub.ackItems[0].Error, "parse rename_tag payload")
}

// --- delete_tag tests ---

func TestProcessQueue_DeleteTagAction(t *testing.T) {
	db := &mockBearDB{}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             70,
				IdempotencyKey: "idem-70",
				Action:         "delete_tag",
				Payload:        `{"name":"old/tag"}`,
			},
		},
	}
	xcall := &mockXCallback{}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	require.Len(t, xcall.calls, 1)
	assert.Equal(t, "delete_tag", xcall.calls[0].action)
	assert.Equal(t, "old/tag", xcall.calls[0].tagName)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "applied", hub.ackItems[0].Status)
}

func TestProcessQueue_DeleteTagNotFound(t *testing.T) {
	db := &mockBearDB{}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             71,
				IdempotencyKey: "idem-71",
				Action:         "delete_tag",
				Payload:        `{"name":"nonexistent"}`,
			},
		},
	}
	// Bear returns "not found" error for non-existent tag — bridge should skip (ack as applied).
	xcall := &mockXCallback{deleteTagErr: fmt.Errorf("bear-xcall delete-tag: %w", &xcallback.BearError{Code: 1, Msg: "tag not found"})}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "applied", hub.ackItems[0].Status)
}

func TestProcessQueue_DeleteTagXCallError(t *testing.T) {
	db := &mockBearDB{}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             72,
				IdempotencyKey: "idem-72",
				Action:         "delete_tag",
				Payload:        `{"name":"some/tag"}`,
			},
		},
	}
	xcall := &mockXCallback{deleteTagErr: fmt.Errorf("bear-xcall unavailable")}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "failed", hub.ackItems[0].Status)
	assert.Contains(t, hub.ackItems[0].Error, "bear-xcall delete-tag")
}

func TestProcessQueue_DeleteTagInvalidPayload(t *testing.T) {
	db := &mockBearDB{}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             73,
				IdempotencyKey: "idem-73",
				Action:         "delete_tag",
				Payload:        `{invalid json`,
			},
		},
	}
	xcall := &mockXCallback{}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "failed", hub.ackItems[0].Status)
	assert.Contains(t, hub.ackItems[0].Error, "parse delete_tag payload")
}

// --- BearModifiedAt in ack tests ---

func TestProcessQueue_UpdateAckIncludesBearModifiedAt(t *testing.T) {
	db := &mockBearDB{
		notesByUUID: map[string]*beardb.NoteBasicInfo{
			"bear-note-1": {UUID: "bear-note-1", Title: "Note", Body: "old body", ModifiedAt: 726842700.5},
		},
	}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             80,
				IdempotencyKey: "idem-80",
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

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "applied", hub.ackItems[0].Status)
	// The mock returns the same note on re-read (NoteByUUID), so BearModifiedAt reflects its ModifiedAt.
	assert.NotEmpty(t, hub.ackItems[0].BearModifiedAt)
	assert.Equal(t, "2024-01-13T12:45:00Z", hub.ackItems[0].BearModifiedAt)
}

func TestProcessQueue_CreateAckIncludesBearModifiedAt(t *testing.T) {
	db := &mockBearDB{
		notesByUUID: map[string]*beardb.NoteBasicInfo{
			"bear-uuid-1": {UUID: "bear-uuid-1", Title: "New Note", Body: "Hello", ModifiedAt: 726842700.0},
		},
	}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             81,
				IdempotencyKey: "idem-81",
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

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "applied", hub.ackItems[0].Status)
	assert.Equal(t, "bear-uuid-1", hub.ackItems[0].BearID)
	assert.NotEmpty(t, hub.ackItems[0].BearModifiedAt)
}

func TestProcessQueue_UpdateBearReadFailsGracefulDegradation(t *testing.T) {
	// First call to NoteByUUID succeeds (for findBearNoteForItem), but we test the scenario
	// where the verification read returns nil (note not found after apply).
	db := &mockBearDB{
		notesByUUID: map[string]*beardb.NoteBasicInfo{
			"bear-note-1": {UUID: "bear-note-1", Title: "Note", Body: "old body", ModifiedAt: 726842700.0},
		},
	}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             82,
				IdempotencyKey: "idem-82",
				Action:         "update",
				NoteID:         "bear-note-1",
				Payload:        `{"body":"new body"}`,
			},
		},
	}
	xcall := &mockXCallback{}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	// Make the mock return an error on the second call (verification read).
	callCount := 0
	origNotes := db.notesByUUID
	db.noteByUUIDFn = func(_ context.Context, bearUUID string) (*beardb.NoteBasicInfo, error) {
		callCount++
		if callCount == 1 {
			// First call: findBearNoteForItem
			return origNotes[bearUUID], nil
		}
		// Second call: verification read — simulate failure
		return nil, fmt.Errorf("sqlite busy")
	}

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	require.Len(t, hub.ackItems, 1)
	assert.Equal(t, "applied", hub.ackItems[0].Status)
	// BearModifiedAt should be empty due to failed verification read.
	assert.Empty(t, hub.ackItems[0].BearModifiedAt)
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

// --- buildQueueStatusItems and extractNoteTitle tests ---

func TestBuildQueueStatusItems(t *testing.T) {
	items := []models.WriteQueueItem{
		{ID: 1, Action: "create", Payload: `{"title":"My Note","body":"content"}`, Status: "processing", CreatedAt: "2026-03-04T12:00:00Z"},
		{ID: 2, Action: "add_tag", Payload: `{"tag":"work","bear_id":"note-1"}`, Status: "processing"},
		{ID: 3, Action: "rename_tag", Payload: `{"name":"old","new_name":"new"}`, Status: "pending"},
	}

	result := buildQueueStatusItems(items)

	require.Len(t, result, 3)

	assert.Equal(t, int64(1), result[0].ID)
	assert.Equal(t, "create", result[0].Action)
	assert.Equal(t, "My Note", result[0].NoteTitle)
	assert.Equal(t, "processing", result[0].Status)
	assert.Equal(t, "2026-03-04T12:00:00Z", result[0].CreatedAt)

	assert.Equal(t, int64(2), result[1].ID)
	assert.Equal(t, "add_tag", result[1].Action)
	assert.Equal(t, "work", result[1].NoteTitle)

	assert.Equal(t, int64(3), result[2].ID)
	assert.Equal(t, "rename_tag", result[2].Action)
	assert.Equal(t, "old", result[2].NoteTitle)
}

func TestExtractNoteTitle(t *testing.T) {
	tests := []struct {
		name     string
		payload  string
		action   string
		expected string
	}{
		{"create with title", `{"title":"My Note","body":"content"}`, "create", "My Note"},
		{"update with title", `{"title":"Updated","body":"content"}`, "update", "Updated"},
		{"add_tag extracts tag", `{"tag":"work"}`, "add_tag", "work"},
		{"rename_tag extracts name", `{"name":"old","new_name":"new"}`, "rename_tag", "old"},
		{"delete_tag extracts name", `{"name":"tag/to/delete"}`, "delete_tag", "tag/to/delete"},
		{"trash no title", `{"bear_id":"note-1"}`, "trash", ""},
		{"invalid json", `{invalid`, "create", ""},
		{"empty payload", `{}`, "create", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractNoteTitle(tt.payload, tt.action)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessQueue_StatsTrackerUpdated(t *testing.T) {
	db := &mockBearDB{
		notesByUUID: map[string]*beardb.NoteBasicInfo{
			"bear-note-1": {UUID: "bear-note-1", Title: "Note", Body: "old"},
		},
	}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:             100,
				IdempotencyKey: "idem-100",
				Action:         "update",
				NoteID:         "bear-note-1",
				Payload:        `{"title":"My Note","body":"new body"}`,
				Status:         "processing",
				CreatedAt:      "2026-03-04T12:00:00Z",
			},
		},
	}
	xcall := &mockXCallback{}
	bridge := newQueueBridge(db, hub, xcall, filepath.Join(t.TempDir(), "state.json"))

	// Set a stats tracker to verify queue tracking.
	stats := ipc.NewStatsTracker(0)
	bridge.stats = stats

	err := bridge.processQueue(context.Background())
	require.NoError(t, err)

	// Stats tracker should have the processed item with "applied" status.
	queueResp := stats.GetQueueStatus()
	require.Len(t, queueResp.Items, 1)
	assert.Equal(t, int64(100), queueResp.Items[0].ID)
	assert.Equal(t, "update", queueResp.Items[0].Action)
	assert.Equal(t, "My Note", queueResp.Items[0].NoteTitle)
	assert.Equal(t, "applied", queueResp.Items[0].Status)
}
