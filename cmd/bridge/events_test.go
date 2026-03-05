package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/romancha/bear-sync/internal/beardb"
	"github.com/romancha/bear-sync/internal/mapper"
	"github.com/romancha/bear-sync/internal/models"
)

func TestEventEmitter_NilSafe(_ *testing.T) {
	var e *EventEmitter
	// Must not panic on nil receiver.
	e.Emit(&SyncEvent{Event: "sync_start"})
}

func TestEventEmitter_EmitsJSON(t *testing.T) {
	var buf bytes.Buffer
	e := NewEventEmitter(&buf)

	e.Emit(&SyncEvent{Event: "sync_start"})

	events := parseEvents(t, buf.String())
	require.Len(t, events, 1)
	assert.Equal(t, "sync_start", events[0].Event)
	assert.NotEmpty(t, events[0].Time)
}

func TestEventEmitter_FieldPresence(t *testing.T) {
	var buf bytes.Buffer
	e := NewEventEmitter(&buf)

	e.Emit(&SyncEvent{Event: "sync_start"})

	line := strings.TrimSpace(buf.String())
	// String fields with omitempty are omitted when empty.
	assert.NotContains(t, line, `"phase"`)
	assert.NotContains(t, line, `"error"`)
	// Integer fields are always present (no omitempty) so the Swift app can distinguish 0 from absent.
	assert.Contains(t, line, `"notes_synced"`)
	assert.Contains(t, line, `"duration_ms"`)
	assert.Contains(t, line, `"tags_synced"`)
	assert.Contains(t, line, `"queue_items"`)
}

func TestSyncEvents_InitialSync(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")
	var buf bytes.Buffer

	db := &mockBearDB{
		notes: []mapper.BearNoteRow{
			{ZPK: 1, ZUNIQUEIDENTIFIER: strPtr("note-1"), ZTITLE: strPtr("Note 1"), ZMODIFICATIONDATE: floatPtr(100)},
			{ZPK: 2, ZUNIQUEIDENTIFIER: strPtr("note-2"), ZTITLE: strPtr("Note 2"), ZMODIFICATIONDATE: floatPtr(200)},
		},
		tags: []mapper.BearTagRow{
			{ZPK: 1, ZUNIQUEIDENTIFIER: strPtr("tag-1"), ZTITLE: strPtr("Tag 1")},
		},
		noteTags: []beardb.NoteTagPair{
			{NoteUUID: "note-1", TagUUID: "tag-1"},
		},
		noteUUIDs: []string{"note-1", "note-2"},
		tagUUIDs:  []string{"tag-1"},
	}
	hub := &mockHubClient{}
	bridge := newTestBridge(db, hub, statePath)
	bridge.events = NewEventEmitter(&buf)

	err := bridge.Run(context.Background())
	require.NoError(t, err)

	events := parseEvents(t, buf.String())
	require.GreaterOrEqual(t, len(events), 4)

	// First event: sync_start.
	assert.Equal(t, "sync_start", events[0].Event)

	// Find reading_bear and pushing_hub progress events.
	readingBear := findEventByPhase(events, "reading_bear")
	pushingHub := findEventByPhase(events, "pushing_hub")
	require.NotNil(t, readingBear)
	require.NotNil(t, pushingHub)
	assert.Equal(t, 2, readingBear.Notes)
	assert.Equal(t, 2, pushingHub.Notes)

	// Last event: sync_complete with correct counts.
	last := events[len(events)-1]
	assert.Equal(t, "sync_complete", last.Event)
	assert.Equal(t, 2, last.NotesSynced)
	assert.Equal(t, 1, last.TagsSynced)
	assert.GreaterOrEqual(t, last.DurationMs, int64(0))
}

func TestSyncEvents_DeltaSync(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")
	var buf bytes.Buffer

	state := &BridgeState{
		LastSyncAt:   100,
		KnownNoteIDs: []string{"note-1"},
	}
	require.NoError(t, saveState(statePath, state))

	db := &mockBearDB{
		notes: []mapper.BearNoteRow{
			{ZPK: 2, ZUNIQUEIDENTIFIER: strPtr("note-2"), ZTITLE: strPtr("New Note"), ZMODIFICATIONDATE: floatPtr(150)},
		},
		tags: []mapper.BearTagRow{
			{ZPK: 1, ZUNIQUEIDENTIFIER: strPtr("tag-1"), ZTITLE: strPtr("Tag 1")},
		},
		noteUUIDs: []string{"note-1", "note-2"},
		tagUUIDs:  []string{"tag-1"},
	}
	hub := &mockHubClient{}
	bridge := newTestBridge(db, hub, statePath)
	bridge.events = NewEventEmitter(&buf)

	err := bridge.Run(context.Background())
	require.NoError(t, err)

	events := parseEvents(t, buf.String())
	require.GreaterOrEqual(t, len(events), 4)

	assert.Equal(t, "sync_start", events[0].Event)

	readingBear := findEventByPhase(events, "reading_bear")
	pushingHub := findEventByPhase(events, "pushing_hub")
	require.NotNil(t, readingBear)
	require.NotNil(t, pushingHub)
	assert.Equal(t, 1, readingBear.Notes)
	assert.Equal(t, 1, pushingHub.Notes)

	last := events[len(events)-1]
	assert.Equal(t, "sync_complete", last.Event)
	assert.Equal(t, 1, last.NotesSynced)
	assert.Equal(t, 1, last.TagsSynced)
}

func TestSyncEvents_DeltaNoChanges(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")
	var buf bytes.Buffer

	state := &BridgeState{
		LastSyncAt:   100,
		KnownNoteIDs: []string{"note-1"},
	}
	require.NoError(t, saveState(statePath, state))

	db := &mockBearDB{
		noteUUIDs: []string{"note-1"},
	}
	hub := &mockHubClient{}
	bridge := newTestBridge(db, hub, statePath)
	bridge.events = NewEventEmitter(&buf)

	err := bridge.Run(context.Background())
	require.NoError(t, err)

	events := parseEvents(t, buf.String())
	require.GreaterOrEqual(t, len(events), 3)

	assert.Equal(t, "sync_start", events[0].Event)

	// reading_bear emitted with 0 notes (no changes).
	readingBear := findEventByPhase(events, "reading_bear")
	require.NotNil(t, readingBear)

	// No pushing_hub event (empty push skipped).
	pushingHub := findEventByPhase(events, "pushing_hub")
	assert.Nil(t, pushingHub)

	last := events[len(events)-1]
	assert.Equal(t, "sync_complete", last.Event)
}

func TestSyncEvents_SyncError(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")
	var buf bytes.Buffer

	db := &eventsFailingDB{}
	hub := &mockHubClient{}
	bridge := newTestBridge(db, hub, statePath)
	bridge.events = NewEventEmitter(&buf)

	err := bridge.Run(context.Background())
	require.Error(t, err)

	events := parseEvents(t, buf.String())
	require.GreaterOrEqual(t, len(events), 2)

	assert.Equal(t, "sync_start", events[0].Event)
	last := events[len(events)-1]
	assert.Equal(t, "sync_error", last.Event)
	assert.NotEmpty(t, last.Error)
}

func TestSyncEvents_WithQueueProcessing(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")
	var buf bytes.Buffer

	state := &BridgeState{
		LastSyncAt:   100,
		KnownNoteIDs: []string{"note-1"},
	}
	require.NoError(t, saveState(statePath, state))

	db := &mockBearDB{
		noteUUIDs: []string{"note-1"},
		notesByUUID: map[string]*beardb.NoteBasicInfo{
			"bear-1": {UUID: "bear-1", Title: "Test", Body: "old body"},
		},
	}
	hub := &mockHubClient{
		queueItems: []models.WriteQueueItem{
			{
				ID:      1,
				Action:  "update",
				NoteID:  "bear-1",
				Payload: `{"bear_id":"bear-1","body":"new body"}`,
			},
		},
	}
	xcall := &mockXCallback{}
	bridge := NewBridge(db, hub, xcall, "token", statePath, "", testLogger())
	bridge.sleepFn = func(_ time.Duration) {}
	bridge.events = NewEventEmitter(&buf)

	err := bridge.Run(context.Background())
	require.NoError(t, err)

	events := parseEvents(t, buf.String())
	queueProgress := findEventByPhase(events, "processing_queue")
	require.NotNil(t, queueProgress)
	assert.Equal(t, 1, queueProgress.Items)

	last := events[len(events)-1]
	assert.Equal(t, "sync_complete", last.Event)
	assert.Equal(t, 1, last.QueueItems)
}

func TestSyncEvents_NilEmitterNoEvents(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	db := &mockBearDB{noteUUIDs: []string{"note-1"}}
	hub := &mockHubClient{}
	bridge := newTestBridge(db, hub, statePath) // events is nil

	// Must not panic and must complete successfully.
	err := bridge.Run(context.Background())
	require.NoError(t, err)
}

func TestSyncEvents_EventOrder(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")
	var buf bytes.Buffer

	db := &mockBearDB{
		notes: []mapper.BearNoteRow{
			{ZPK: 1, ZUNIQUEIDENTIFIER: strPtr("note-1"), ZTITLE: strPtr("Note 1"), ZMODIFICATIONDATE: floatPtr(100)},
		},
		noteUUIDs: []string{"note-1"},
	}
	hub := &mockHubClient{}
	bridge := newTestBridge(db, hub, statePath)
	bridge.events = NewEventEmitter(&buf)

	err := bridge.Run(context.Background())
	require.NoError(t, err)

	events := parseEvents(t, buf.String())
	require.GreaterOrEqual(t, len(events), 4)

	// Verify strict event ordering: sync_start -> reading_bear -> pushing_hub -> sync_complete.
	eventTypes := make([]string, len(events))
	for i, e := range events {
		if e.Phase != "" {
			eventTypes[i] = e.Phase
		} else {
			eventTypes[i] = e.Event
		}
	}

	startIdx := indexOf(eventTypes, "sync_start")
	readIdx := indexOf(eventTypes, "reading_bear")
	pushIdx := indexOf(eventTypes, "pushing_hub")
	completeIdx := indexOf(eventTypes, "sync_complete")

	assert.Less(t, startIdx, readIdx, "sync_start should precede reading_bear")
	assert.Less(t, readIdx, pushIdx, "reading_bear should precede pushing_hub")
	assert.Less(t, pushIdx, completeIdx, "pushing_hub should precede sync_complete")
}

// parseEvents parses newline-delimited JSON events from output.
func parseEvents(t *testing.T, output string) []SyncEvent {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	events := make([]SyncEvent, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		var e SyncEvent
		require.NoError(t, json.Unmarshal([]byte(line), &e))
		events = append(events, e)
	}
	return events
}

// findEventByPhase returns the first event with the given phase, or nil.
func findEventByPhase(events []SyncEvent, phase string) *SyncEvent {
	for i := range events {
		if events[i].Phase == phase {
			return &events[i]
		}
	}
	return nil
}

// indexOf returns the index of the first occurrence of target in slice, or -1.
func indexOf(slice []string, target string) int {
	for i, s := range slice {
		if s == target {
			return i
		}
	}
	return -1
}

// eventsFailingDB always fails on Notes() for testing sync_error events.
type eventsFailingDB struct {
	mockBearDB
}

func (f *eventsFailingDB) Notes(_ context.Context, _ float64) ([]mapper.BearNoteRow, error) {
	return nil, fmt.Errorf("simulated notes read failure")
}
