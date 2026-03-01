package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/romancha/bear-sync/internal/beardb"
	"github.com/romancha/bear-sync/internal/hubclient"
	"github.com/romancha/bear-sync/internal/mapper"
	"github.com/romancha/bear-sync/internal/models"
)

// mockBearDB implements beardb.BearDB for testing.
type mockBearDB struct {
	notes       []mapper.BearNoteRow
	tags        []mapper.BearTagRow
	attachments []mapper.BearAttachmentRow
	backlinks   []mapper.BearBacklinkRow
	noteTags    []beardb.NoteTagPair
	pinnedTags  []beardb.NoteTagPair
	noteUUIDs   []string
	tagUUIDs    []string
	attUUIDs    []string
	blUUIDs     []string

	// Verification data for write queue tests.
	notesByUUID   map[string]*beardb.NoteBasicInfo
	tagsByNote    map[string][]string // bearUUID -> tag titles
	recentNotes   []beardb.NoteBasicInfo
}

func (m *mockBearDB) Notes(_ context.Context, _ float64) ([]mapper.BearNoteRow, error) {
	return m.notes, nil
}

func (m *mockBearDB) Tags(_ context.Context, _ float64) ([]mapper.BearTagRow, error) {
	return m.tags, nil
}

func (m *mockBearDB) Attachments(_ context.Context, _ float64) ([]mapper.BearAttachmentRow, error) {
	return m.attachments, nil
}

func (m *mockBearDB) Backlinks(_ context.Context, _ float64) ([]mapper.BearBacklinkRow, error) {
	return m.backlinks, nil
}

func (m *mockBearDB) NoteTags(_ context.Context) ([]beardb.NoteTagPair, error) {
	return m.noteTags, nil
}

func (m *mockBearDB) PinnedNoteTags(_ context.Context) ([]beardb.NoteTagPair, error) {
	return m.pinnedTags, nil
}

func (m *mockBearDB) NoteTagsForNotes(_ context.Context, noteUUIDs []string) ([]beardb.NoteTagPair, error) {
	noteSet := make(map[string]bool, len(noteUUIDs))
	for _, u := range noteUUIDs {
		noteSet[u] = true
	}
	var result []beardb.NoteTagPair
	for _, p := range m.noteTags {
		if noteSet[p.NoteUUID] {
			result = append(result, p)
		}
	}
	return result, nil
}

func (m *mockBearDB) PinnedNoteTagsForNotes(_ context.Context, noteUUIDs []string) ([]beardb.NoteTagPair, error) {
	noteSet := make(map[string]bool, len(noteUUIDs))
	for _, u := range noteUUIDs {
		noteSet[u] = true
	}
	var result []beardb.NoteTagPair
	for _, p := range m.pinnedTags {
		if noteSet[p.NoteUUID] {
			result = append(result, p)
		}
	}
	return result, nil
}

func (m *mockBearDB) AllNoteUUIDs(_ context.Context) ([]string, error)       { return m.noteUUIDs, nil }
func (m *mockBearDB) AllTagUUIDs(_ context.Context) ([]string, error)        { return m.tagUUIDs, nil }
func (m *mockBearDB) AllAttachmentUUIDs(_ context.Context) ([]string, error) { return m.attUUIDs, nil }
func (m *mockBearDB) AllBacklinkUUIDs(_ context.Context) ([]string, error)   { return m.blUUIDs, nil }
func (m *mockBearDB) Close() error                                           { return nil }

func (m *mockBearDB) NoteByUUID(_ context.Context, bearUUID string) (*beardb.NoteBasicInfo, error) {
	if m.notesByUUID == nil {
		return nil, nil
	}
	info, ok := m.notesByUUID[bearUUID]
	if !ok {
		return nil, nil
	}
	return info, nil
}

func (m *mockBearDB) NoteTagTitles(_ context.Context, bearUUID string) ([]string, error) {
	if m.tagsByNote == nil {
		return nil, nil
	}
	return m.tagsByNote[bearUUID], nil
}

func (m *mockBearDB) FindRecentNotesByTitle(_ context.Context, _ string, _ float64) ([]beardb.NoteBasicInfo, error) {
	return m.recentNotes, nil
}

// mockHubClient implements hubclient.HubClient for testing.
type mockHubClient struct {
	pushes      []models.SyncPushRequest
	queueItems  []models.WriteQueueItem
	ackItems    []models.SyncAckItem
	uploadedIDs []string // attachment IDs that were uploaded
}

func (m *mockHubClient) SyncPush(_ context.Context, req models.SyncPushRequest) error { //nolint:gocritic // interface match
	m.pushes = append(m.pushes, req)
	return nil
}

func (m *mockHubClient) LeaseQueue(_ context.Context, _ string) ([]models.WriteQueueItem, error) {
	return m.queueItems, nil
}

func (m *mockHubClient) AckQueue(_ context.Context, items []models.SyncAckItem) error {
	m.ackItems = append(m.ackItems, items...)
	return nil
}

func (m *mockHubClient) UploadAttachment(_ context.Context, attachmentID string, _ io.Reader) error {
	m.uploadedIDs = append(m.uploadedIDs, attachmentID)
	return nil
}

func (m *mockHubClient) GetSyncStatus(_ context.Context) (*hubclient.SyncStatus, error) {
	return nil, nil
}

// mockXCallback implements xcallback.XCallback for testing.
type mockXCallback struct {
	createBearID string
	createErr    error
	updateErr    error
	addTagErr    error
	trashErr     error
	calls        []xcallCall
}

type xcallCall struct {
	action string
	bearID string
	title  string
	body   string
	tags   []string
	tag    string
}

func (m *mockXCallback) Create(_ context.Context, _, title, body string, tags []string) (string, error) {
	m.calls = append(m.calls, xcallCall{action: "create", title: title, body: body, tags: tags})
	return m.createBearID, m.createErr
}

func (m *mockXCallback) Update(_ context.Context, _, bearID, body string) error {
	m.calls = append(m.calls, xcallCall{action: "update", bearID: bearID, body: body})
	return m.updateErr
}

func (m *mockXCallback) AddTag(_ context.Context, _, bearID, tag string) error {
	m.calls = append(m.calls, xcallCall{action: "add_tag", bearID: bearID, tag: tag})
	return m.addTagErr
}

func (m *mockXCallback) Trash(_ context.Context, _, bearID string) error {
	m.calls = append(m.calls, xcallCall{action: "trash", bearID: bearID})
	return m.trashErr
}

func strPtr(s string) *string    { return &s }
func floatPtr(f float64) *float64 { return &f }

// newTestBridge creates a Bridge for testing with nil xcallback (no queue processing).
func newTestBridge(db beardb.BearDB, hub hubclient.HubClient, statePath string) *Bridge {
	b := NewBridge(db, hub, nil, "", statePath, "", testLogger())
	b.sleepFn = func(_ time.Duration) {} // no-op sleep for tests
	return b
}

func TestInitialSync(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

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

	err := bridge.Run(context.Background())
	require.NoError(t, err)

	// Should have pushed: 1 batch for notes (2 < 50) + 1 batch for tags/attachments/backlinks.
	require.Len(t, hub.pushes, 2)

	// First push: notes.
	assert.Len(t, hub.pushes[0].Notes, 2)

	// Second push: tags + junction tables + initial_sync_complete meta.
	assert.Len(t, hub.pushes[1].Tags, 1)
	assert.Len(t, hub.pushes[1].NoteTags, 1)
	assert.Equal(t, "true", hub.pushes[1].Meta["initial_sync_complete"])

	// State file should exist.
	state, err := loadState(statePath)
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Greater(t, state.LastSyncAt, float64(0))
	assert.Len(t, state.KnownNoteIDs, 2)
	assert.Len(t, state.KnownTagIDs, 1)
	assert.Len(t, state.KnownNoteTagPairs, 1)
}

func TestInitialSync_BatchedNotes(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	// Create 120 notes to test batching (50 per batch = 3 batches).
	notes := make([]mapper.BearNoteRow, 120)
	noteUUIDs := make([]string, 120)
	for i := range 120 {
		uuid := "note-" + string(rune('A'+i%26)) + string(rune('0'+i/26))
		notes[i] = mapper.BearNoteRow{
			ZPK:               int64(i + 1),
			ZUNIQUEIDENTIFIER: strPtr(uuid),
			ZTITLE:            strPtr("Note " + uuid),
		}
		noteUUIDs[i] = uuid
	}

	db := &mockBearDB{
		notes:     notes,
		noteUUIDs: noteUUIDs,
	}
	hub := &mockHubClient{}
	bridge := newTestBridge(db, hub, filepath.Join(tmpDir, "state.json"))

	err := bridge.Run(context.Background())
	require.NoError(t, err)

	// 3 note batches + 1 metadata-only push with initial_sync_complete=true.
	require.Len(t, hub.pushes, 4)
	assert.Len(t, hub.pushes[0].Notes, 50)
	assert.Len(t, hub.pushes[1].Notes, 50)
	assert.Len(t, hub.pushes[2].Notes, 20)
	assert.Equal(t, "true", hub.pushes[3].Meta["initial_sync_complete"])

	state, err := loadState(statePath)
	require.NoError(t, err)
	assert.Len(t, state.KnownNoteIDs, 120)
}

func TestDeltaSync_NoChanges(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	// Pre-create state (simulating previous sync).
	state := &BridgeState{
		LastSyncAt:   100,
		KnownNoteIDs: []string{"note-1"},
		KnownTagIDs:  []string{"tag-1"},
	}
	require.NoError(t, saveState(statePath, state))

	db := &mockBearDB{
		noteUUIDs: []string{"note-1"},
		tagUUIDs:  []string{"tag-1"},
	}
	hub := &mockHubClient{}
	bridge := newTestBridge(db, hub, statePath)

	err := bridge.Run(context.Background())
	require.NoError(t, err)

	// No changes detected — no push should happen.
	assert.Empty(t, hub.pushes)

	// Counter should increment.
	updatedState, err := loadState(statePath)
	require.NoError(t, err)
	assert.Equal(t, 1, updatedState.JunctionFullScanCounter)
}

func TestDeltaSync_WithChanges(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	state := &BridgeState{
		LastSyncAt:   100,
		KnownNoteIDs: []string{"note-1"},
		KnownTagIDs:  []string{"tag-1"},
	}
	require.NoError(t, saveState(statePath, state))

	db := &mockBearDB{
		notes: []mapper.BearNoteRow{
			{ZPK: 2, ZUNIQUEIDENTIFIER: strPtr("note-2"), ZTITLE: strPtr("New Note"), ZMODIFICATIONDATE: floatPtr(150)},
		},
		noteTags: []beardb.NoteTagPair{
			{NoteUUID: "note-2", TagUUID: "tag-1"},
		},
		noteUUIDs: []string{"note-1", "note-2"},
		tagUUIDs:  []string{"tag-1"},
	}
	hub := &mockHubClient{}
	bridge := newTestBridge(db, hub, statePath)

	err := bridge.Run(context.Background())
	require.NoError(t, err)

	require.Len(t, hub.pushes, 1)
	assert.Len(t, hub.pushes[0].Notes, 1)
	assert.Len(t, hub.pushes[0].NoteTags, 1) // Junction delta for changed note.

	updatedState, err := loadState(statePath)
	require.NoError(t, err)
	assert.Greater(t, updatedState.LastSyncAt, state.LastSyncAt)
	assert.Len(t, updatedState.KnownNoteIDs, 2)
}

func TestDeltaSync_DeletionDetection(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	state := &BridgeState{
		LastSyncAt:         100,
		KnownNoteIDs:       []string{"note-1", "note-2", "note-3"},
		KnownTagIDs:        []string{"tag-1", "tag-2"},
		KnownAttachmentIDs: []string{"att-1"},
		KnownBacklinkIDs:   []string{"bl-1"},
	}
	require.NoError(t, saveState(statePath, state))

	// note-2 and tag-2 have been deleted from Bear.
	db := &mockBearDB{
		noteUUIDs: []string{"note-1", "note-3"},
		tagUUIDs:  []string{"tag-1"},
		attUUIDs:  []string{"att-1"},
		blUUIDs:   []string{"bl-1"},
	}
	hub := &mockHubClient{}
	bridge := newTestBridge(db, hub, statePath)

	err := bridge.Run(context.Background())
	require.NoError(t, err)

	require.Len(t, hub.pushes, 1)
	assert.Equal(t, []string{"note-2"}, hub.pushes[0].DeletedNoteIDs)
	assert.Equal(t, []string{"tag-2"}, hub.pushes[0].DeletedTagIDs)
	assert.Empty(t, hub.pushes[0].DeletedAttachmentIDs)
	assert.Empty(t, hub.pushes[0].DeletedBacklinkIDs)
}

func TestDeltaSync_JunctionFullScan(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	// Counter at 12 — triggers full scan.
	state := &BridgeState{
		LastSyncAt:              100,
		KnownNoteIDs:            []string{"note-1"},
		KnownTagIDs:             []string{"tag-1", "tag-2"},
		JunctionFullScanCounter: 12,
		KnownNoteTagPairs: []IDPair{
			{NoteUUID: "note-1", TagUUID: "tag-1"},
		},
	}
	require.NoError(t, saveState(statePath, state))

	// Tag-2 was added to note-1 without modifying the note itself (no delta rows).
	db := &mockBearDB{
		noteTags: []beardb.NoteTagPair{
			{NoteUUID: "note-1", TagUUID: "tag-1"},
			{NoteUUID: "note-1", TagUUID: "tag-2"},
		},
		noteUUIDs: []string{"note-1"},
		tagUUIDs:  []string{"tag-1", "tag-2"},
	}
	hub := &mockHubClient{}
	bridge := newTestBridge(db, hub, statePath)

	err := bridge.Run(context.Background())
	require.NoError(t, err)

	require.Len(t, hub.pushes, 1)
	// Full scan detected the new tag-2 association.
	assert.Len(t, hub.pushes[0].NoteTags, 2) // Full snapshot for note-1.

	updatedState, err := loadState(statePath)
	require.NoError(t, err)
	assert.Equal(t, 13, updatedState.JunctionFullScanCounter)
	assert.Len(t, updatedState.KnownNoteTagPairs, 2)
}

func TestDeltaSync_NoFullScanOnNonInterval(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	state := &BridgeState{
		LastSyncAt:              100,
		KnownNoteIDs:            []string{"note-1"},
		KnownTagIDs:             []string{"tag-1", "tag-2"},
		JunctionFullScanCounter: 5, // Not a multiple of 12.
		KnownNoteTagPairs: []IDPair{
			{NoteUUID: "note-1", TagUUID: "tag-1"},
		},
	}
	require.NoError(t, saveState(statePath, state))

	db := &mockBearDB{
		noteTags: []beardb.NoteTagPair{
			{NoteUUID: "note-1", TagUUID: "tag-1"},
			{NoteUUID: "note-1", TagUUID: "tag-2"}, // Changed but won't be detected.
		},
		noteUUIDs: []string{"note-1"},
		tagUUIDs:  []string{"tag-1", "tag-2"},
	}
	hub := &mockHubClient{}
	bridge := newTestBridge(db, hub, statePath)

	err := bridge.Run(context.Background())
	require.NoError(t, err)

	// No entity changes, no full scan -> no push.
	assert.Empty(t, hub.pushes)
}

func TestFindChangedJunctionNotes(t *testing.T) {
	tests := []struct {
		name     string
		old      []IDPair
		new      []IDPair
		expected int
	}{
		{
			name:     "no changes",
			old:      []IDPair{{NoteUUID: "n1", TagUUID: "t1"}},
			new:      []IDPair{{NoteUUID: "n1", TagUUID: "t1"}},
			expected: 0,
		},
		{
			name:     "tag added",
			old:      []IDPair{{NoteUUID: "n1", TagUUID: "t1"}},
			new:      []IDPair{{NoteUUID: "n1", TagUUID: "t1"}, {NoteUUID: "n1", TagUUID: "t2"}},
			expected: 1,
		},
		{
			name:     "tag removed",
			old:      []IDPair{{NoteUUID: "n1", TagUUID: "t1"}, {NoteUUID: "n1", TagUUID: "t2"}},
			new:      []IDPair{{NoteUUID: "n1", TagUUID: "t1"}},
			expected: 1,
		},
		{
			name:     "new note",
			old:      []IDPair{{NoteUUID: "n1", TagUUID: "t1"}},
			new:      []IDPair{{NoteUUID: "n1", TagUUID: "t1"}, {NoteUUID: "n2", TagUUID: "t1"}},
			expected: 1,
		},
		{
			name:     "note removed",
			old:      []IDPair{{NoteUUID: "n1", TagUUID: "t1"}, {NoteUUID: "n2", TagUUID: "t1"}},
			new:      []IDPair{{NoteUUID: "n1", TagUUID: "t1"}},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findChangedJunctionNotes(tt.old, tt.new)
			assert.Len(t, result, tt.expected)
		})
	}
}

func TestStateRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	state := &BridgeState{
		LastSyncAt:              123.456,
		KnownNoteIDs:            []string{"note-1", "note-2"},
		KnownTagIDs:             []string{"tag-1"},
		KnownAttachmentIDs:      []string{"att-1"},
		KnownBacklinkIDs:        []string{"bl-1"},
		KnownNoteTagPairs:       []IDPair{{NoteUUID: "note-1", TagUUID: "tag-1"}},
		KnownPinnedNoteTagPairs: []IDPair{{NoteUUID: "note-2", TagUUID: "tag-1"}},
		JunctionFullScanCounter: 7,
	}

	require.NoError(t, saveState(statePath, state))

	loaded, err := loadState(statePath)
	require.NoError(t, err)
	assert.Equal(t, state, loaded)
}

func TestLoadState_NotExists(t *testing.T) {
	state, err := loadState("/nonexistent/path/state.json")
	require.NoError(t, err)
	assert.Nil(t, state)
}

func TestFlock(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	// First lock should succeed.
	f1, err := acquireLock(lockPath)
	require.NoError(t, err)
	require.NotNil(t, f1)

	// Second lock should fail.
	_, err = acquireLock(lockPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "another bridge instance is running")

	// After release, should be able to lock again.
	releaseLock(f1, testLogger())

	f2, err := acquireLock(lockPath)
	require.NoError(t, err)
	require.NotNil(t, f2)
	releaseLock(f2, testLogger())
}

func TestIsEmptyPush(t *testing.T) {
	assert.True(t, isEmptyPush(&models.SyncPushRequest{}))
	assert.False(t, isEmptyPush(&models.SyncPushRequest{
		Notes: []models.Note{{ID: "1"}},
	}))
	assert.False(t, isEmptyPush(&models.SyncPushRequest{
		DeletedNoteIDs: []string{"1"},
	}))
}

func TestMergeNoteTags(t *testing.T) {
	existing := []models.NoteTagPair{
		{NoteID: "n1", TagID: "t1"},
	}
	additional := []models.NoteTagPair{
		{NoteID: "n1", TagID: "t1"}, // Duplicate.
		{NoteID: "n2", TagID: "t1"}, // New.
	}

	result := mergeNoteTags(existing, additional)
	assert.Len(t, result, 2)
}

func TestLoadConfig_MissingRequired(t *testing.T) {
	// Clear all env vars.
	t.Setenv("BRIDGE_HUB_URL", "")
	t.Setenv("BRIDGE_HUB_TOKEN", "")
	t.Setenv("BEAR_TOKEN", "")

	_, err := loadConfig()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "BRIDGE_HUB_URL")
}

func TestLoadConfig_AllSet(t *testing.T) {
	t.Setenv("BRIDGE_HUB_URL", "http://localhost:8080")
	t.Setenv("BRIDGE_HUB_TOKEN", "test-token")
	t.Setenv("BEAR_TOKEN", "bear-token")
	t.Setenv("BRIDGE_STATE_PATH", "/tmp/test-state.json")
	t.Setenv("BEAR_DB_DIR", "/tmp/test-bear-db")

	cfg, err := loadConfig()
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:8080", cfg.hubURL)
	assert.Equal(t, "test-token", cfg.hubToken)
	assert.Equal(t, "bear-token", cfg.bearToken)
	assert.Equal(t, "/tmp/test-state.json", cfg.statePath)
	assert.Equal(t, "/tmp/test-bear-db", cfg.bearDBDir)
}

func TestInitialSync_UploadsAttachmentFiles(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	// Create a fake Bear data dir with attachment files.
	bearDataDir := filepath.Join(tmpDir, "bear-data")
	imgDir := filepath.Join(bearDataDir, "Local Files", "Note Images", "att-uuid-1")
	require.NoError(t, os.MkdirAll(imgDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(imgDir, "photo.jpg"), []byte("image-data"), 0o600))

	fileDir := filepath.Join(bearDataDir, "Local Files", "Note Files", "att-uuid-2")
	require.NoError(t, os.MkdirAll(fileDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(fileDir, "doc.pdf"), []byte("pdf-data"), 0o600))

	db := &mockBearDB{
		attachments: []mapper.BearAttachmentRow{
			{
				ZPK: 1, ZENT: 9, ZUNIQUEIDENTIFIER: strPtr("att-uuid-1"),
				ZFILENAME: strPtr("photo.jpg"), ZNOTE: strPtr("note-1"),
			},
			{
				ZPK: 2, ZENT: 8, ZUNIQUEIDENTIFIER: strPtr("att-uuid-2"),
				ZFILENAME: strPtr("doc.pdf"), ZNOTE: strPtr("note-1"),
			},
		},
		attUUIDs: []string{"att-uuid-1", "att-uuid-2"},
	}

	hub := &mockHubClient{}
	b := NewBridge(db, hub, nil, "", statePath, bearDataDir, testLogger())
	b.sleepFn = func(_ time.Duration) {}

	err := b.Run(context.Background())
	require.NoError(t, err)

	// Both attachment files should have been uploaded.
	assert.Len(t, hub.uploadedIDs, 2)
}

func TestInitialSync_SkipsEncryptedAndDeletedAttachments(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")
	bearDataDir := filepath.Join(tmpDir, "bear-data")

	encInt := int64(1)
	delInt := int64(1)

	db := &mockBearDB{
		attachments: []mapper.BearAttachmentRow{
			{
				ZPK: 1, ZENT: 9, ZUNIQUEIDENTIFIER: strPtr("att-encrypted"),
				ZFILENAME: strPtr("secret.jpg"), ZNOTE: strPtr("note-1"),
				ZENCRYPTED: &encInt,
			},
			{
				ZPK: 2, ZENT: 8, ZUNIQUEIDENTIFIER: strPtr("att-deleted"),
				ZFILENAME: strPtr("gone.pdf"), ZNOTE: strPtr("note-1"),
				ZPERMANENTLYDELETED: &delInt,
			},
		},
		attUUIDs: []string{"att-encrypted", "att-deleted"},
	}

	hub := &mockHubClient{}
	b := NewBridge(db, hub, nil, "", statePath, bearDataDir, testLogger())
	b.sleepFn = func(_ time.Duration) {}

	err := b.Run(context.Background())
	require.NoError(t, err)

	// Neither encrypted nor deleted attachments should be uploaded.
	assert.Empty(t, hub.uploadedIDs)
}

func TestResolveAttachmentFilePath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create image directory with file.
	imgDir := filepath.Join(tmpDir, "Local Files", "Note Images", "img-1")
	require.NoError(t, os.MkdirAll(imgDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(imgDir, "photo.jpg"), []byte("data"), 0o600))

	// Create file directory with file.
	fileDir := filepath.Join(tmpDir, "Local Files", "Note Files", "file-1")
	require.NoError(t, os.MkdirAll(fileDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(fileDir, "doc.pdf"), []byte("data"), 0o600))

	b := &Bridge{bearDataDir: tmpDir, logger: testLogger()}

	// Image type resolves to Note Images.
	path := b.resolveAttachmentFilePath("image", "img-1", "photo.jpg")
	assert.Equal(t, filepath.Join(imgDir, "photo.jpg"), path)

	// File type resolves to Note Files.
	path = b.resolveAttachmentFilePath("file", "file-1", "doc.pdf")
	assert.Equal(t, filepath.Join(fileDir, "doc.pdf"), path)

	// Video type also resolves to Note Files.
	path = b.resolveAttachmentFilePath("video", "file-1", "doc.pdf")
	assert.Equal(t, filepath.Join(fileDir, "doc.pdf"), path)

	// Missing file returns empty.
	path = b.resolveAttachmentFilePath("image", "nonexistent", "nope.jpg")
	assert.Empty(t, path)

	// Fallback to first file in directory when filename doesn't match.
	path = b.resolveAttachmentFilePath("image", "img-1", "wrong-name.jpg")
	assert.Equal(t, filepath.Join(imgDir, "photo.jpg"), path)
}

func TestDeltaSync_UploadsChangedAttachments(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")

	bearDataDir := filepath.Join(tmpDir, "bear-data")
	imgDir := filepath.Join(bearDataDir, "Local Files", "Note Images", "att-new")
	require.NoError(t, os.MkdirAll(imgDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(imgDir, "new.jpg"), []byte("new-data"), 0o600))

	state := &BridgeState{
		LastSyncAt:   100,
		KnownNoteIDs: []string{"note-1"},
	}
	require.NoError(t, saveState(statePath, state))

	db := &mockBearDB{
		attachments: []mapper.BearAttachmentRow{
			{
				ZPK: 1, ZENT: 9, ZUNIQUEIDENTIFIER: strPtr("att-new"),
				ZFILENAME: strPtr("new.jpg"), ZNOTE: strPtr("note-1"),
				ZMODIFICATIONDATE: floatPtr(150),
			},
		},
		noteUUIDs: []string{"note-1"},
		attUUIDs:  []string{"att-new"},
	}

	hub := &mockHubClient{}
	b := NewBridge(db, hub, nil, "", statePath, bearDataDir, testLogger())
	b.sleepFn = func(_ time.Duration) {}

	err := b.Run(context.Background())
	require.NoError(t, err)

	// Attachment file should have been uploaded.
	assert.Len(t, hub.uploadedIDs, 1)
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}
