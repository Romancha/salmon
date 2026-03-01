package store_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/romancha/bear-sync/internal/models"
	"github.com/romancha/bear-sync/internal/store"
)

func newTestStore(t *testing.T) *store.SQLiteStore {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.NewSQLiteStore(dbPath)
	require.NoError(t, err)

	t.Cleanup(func() { _ = s.Close() })

	return s
}

func strPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}

// --- Note CRUD ---

func TestCreateAndGetNote(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	note := &models.Note{
		ID:         "note-1",
		BearID:     strPtr("bear-note-1"),
		Title:      "Test Note",
		Subtitle:   "A subtitle",
		Body:       "Hello world",
		Pinned:     1,
		CreatedAt:  "2025-01-01T00:00:00Z",
		ModifiedAt: "2025-01-02T00:00:00Z",
		SyncStatus: "synced",
	}

	err := s.CreateNote(ctx, note)
	require.NoError(t, err)

	got, err := s.GetNote(ctx, "note-1")
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, "note-1", got.ID)
	assert.Equal(t, "bear-note-1", *got.BearID)
	assert.Equal(t, "Test Note", got.Title)
	assert.Equal(t, "A subtitle", got.Subtitle)
	assert.Equal(t, "Hello world", got.Body)
	assert.Equal(t, 1, got.Pinned)
	assert.Equal(t, "synced", got.SyncStatus)
}

func TestGetNote_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	got, err := s.GetNote(ctx, "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestUpdateNote(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	note := &models.Note{
		ID:         "note-1",
		Title:      "Original",
		Body:       "Original body",
		SyncStatus: "synced",
	}
	require.NoError(t, s.CreateNote(ctx, note))

	note.Title = "Updated"
	note.Body = "Updated body"
	require.NoError(t, s.UpdateNote(ctx, note))

	got, err := s.GetNote(ctx, "note-1")
	require.NoError(t, err)
	assert.Equal(t, "Updated", got.Title)
	assert.Equal(t, "Updated body", got.Body)
}

func TestListNotes_Pagination(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		note := &models.Note{
			ID:         fmt.Sprintf("note-%d", i),
			Title:      fmt.Sprintf("Note %d", i),
			Body:       "body",
			ModifiedAt: fmt.Sprintf("2025-01-0%dT00:00:00Z", i+1),
			SyncStatus: "synced",
		}
		require.NoError(t, s.CreateNote(ctx, note))
	}

	notes, err := s.ListNotes(ctx, store.NoteFilter{Limit: 2, Offset: 0})
	require.NoError(t, err)
	assert.Len(t, notes, 2)

	notes, err = s.ListNotes(ctx, store.NoteFilter{Limit: 2, Offset: 3})
	require.NoError(t, err)
	assert.Len(t, notes, 2)
}

func TestListNotes_FilterByTag(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.CreateNote(ctx, &models.Note{ID: "n1", Title: "Note 1", Body: "b", SyncStatus: "synced"}))
	require.NoError(t, s.CreateNote(ctx, &models.Note{ID: "n2", Title: "Note 2", Body: "b", SyncStatus: "synced"}))
	require.NoError(t, s.CreateTag(ctx, &models.Tag{ID: "t1", Title: "work"}))

	_, err := s.DB().ExecContext(ctx, "INSERT INTO note_tags (note_id, tag_id) VALUES (?, ?)", "n1", "t1")
	require.NoError(t, err)

	notes, err := s.ListNotes(ctx, store.NoteFilter{Tag: "work"})
	require.NoError(t, err)
	assert.Len(t, notes, 1)
	assert.Equal(t, "n1", notes[0].ID)
}

func TestListNotes_FilterTrashed(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.CreateNote(ctx, &models.Note{ID: "n1", Title: "Active", Body: "b", SyncStatus: "synced"}))
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID: "n2", Title: "Trashed", Body: "b", Trashed: 1, SyncStatus: "synced",
	}))

	notes, err := s.ListNotes(ctx, store.NoteFilter{Trashed: boolPtr(false)})
	require.NoError(t, err)
	assert.Len(t, notes, 1)
	assert.Equal(t, "n1", notes[0].ID)

	notes, err = s.ListNotes(ctx, store.NoteFilter{Trashed: boolPtr(true)})
	require.NoError(t, err)
	assert.Len(t, notes, 1)
	assert.Equal(t, "n2", notes[0].ID)
}

func TestListNotes_Sorting(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID: "n1", Title: "B Note", Body: "b", ModifiedAt: "2025-01-01T00:00:00Z", SyncStatus: "synced",
	}))
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID: "n2", Title: "A Note", Body: "b", ModifiedAt: "2025-01-02T00:00:00Z", SyncStatus: "synced",
	}))

	notes, err := s.ListNotes(ctx, store.NoteFilter{Sort: "title", Order: "asc"})
	require.NoError(t, err)
	require.Len(t, notes, 2)
	assert.Equal(t, "A Note", notes[0].Title)
	assert.Equal(t, "B Note", notes[1].Title)
}

// --- FTS5 Search ---

func TestSearchNotes(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID: "n1", Title: "Go Programming", Body: "Learn Go language", SyncStatus: "synced",
	}))
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID: "n2", Title: "Python Guide", Body: "Learn Python basics", SyncStatus: "synced",
	}))

	results, err := s.SearchNotes(ctx, "Go", "", 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "n1", results[0].ID)
}

func TestSearchNotes_WithTagFilter(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID: "n1", Title: "Go Programming", Body: "Learn Go", SyncStatus: "synced",
	}))
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID: "n2", Title: "Go Tutorial", Body: "Advanced Go", SyncStatus: "synced",
	}))
	require.NoError(t, s.CreateTag(ctx, &models.Tag{ID: "t1", Title: "beginner"}))

	_, err := s.DB().ExecContext(ctx, "INSERT INTO note_tags (note_id, tag_id) VALUES (?, ?)", "n1", "t1")
	require.NoError(t, err)

	results, err := s.SearchNotes(ctx, "Go", "beginner", 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "n1", results[0].ID)
}

// --- Tag CRUD ---

func TestCreateAndListTags(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.CreateTag(ctx, &models.Tag{
		ID: "t1", BearID: strPtr("bear-t1"), Title: "work",
	}))
	require.NoError(t, s.CreateTag(ctx, &models.Tag{
		ID: "t2", Title: "personal",
	}))

	tags, err := s.ListTags(ctx)
	require.NoError(t, err)
	assert.Len(t, tags, 2)
	assert.Equal(t, "personal", tags[0].Title)
	assert.Equal(t, "work", tags[1].Title)
}

func TestGetTag(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.CreateTag(ctx, &models.Tag{ID: "t1", Title: "work"}))

	tag, err := s.GetTag(ctx, "t1")
	require.NoError(t, err)
	require.NotNil(t, tag)
	assert.Equal(t, "work", tag.Title)

	tag, err = s.GetTag(ctx, "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, tag)
}

// --- Attachment CRUD ---

func TestAttachments(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.CreateNote(ctx, &models.Note{ID: "n1", Title: "Note", Body: "b", SyncStatus: "synced"}))

	req := models.SyncPushRequest{
		Attachments: []models.Attachment{
			{
				ID: "a1", BearID: strPtr("bear-a1"), NoteID: "n1", Type: "image",
				Filename: "photo.jpg", FileSize: 1024,
			},
		},
	}
	require.NoError(t, s.ProcessSyncPush(ctx, req))

	att, err := s.GetAttachment(ctx, "a1")
	require.NoError(t, err)
	require.NotNil(t, att)
	assert.Equal(t, "photo.jpg", att.Filename)
	assert.Equal(t, int64(1024), att.FileSize)

	atts, err := s.ListAttachmentsByNote(ctx, "n1")
	require.NoError(t, err)
	assert.Len(t, atts, 1)

	// Test GetAttachmentByBearID.
	attByBear, err := s.GetAttachmentByBearID(ctx, "bear-a1")
	require.NoError(t, err)
	require.NotNil(t, attByBear)
	assert.Equal(t, "a1", attByBear.ID)
	assert.Equal(t, "photo.jpg", attByBear.Filename)

	// Non-existent bear_id returns nil.
	attByBear, err = s.GetAttachmentByBearID(ctx, "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, attByBear)
}

// --- Backlinks ---

func TestBacklinks(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.CreateNote(ctx, &models.Note{ID: "n1", Title: "Note 1", Body: "b", SyncStatus: "synced"}))
	require.NoError(t, s.CreateNote(ctx, &models.Note{ID: "n2", Title: "Note 2", Body: "b", SyncStatus: "synced"}))

	req := models.SyncPushRequest{
		Backlinks: []models.Backlink{
			{ID: "bl1", BearID: strPtr("bear-bl1"), LinkedByID: "n1", LinkingToID: "n2", Title: "link"},
		},
	}
	require.NoError(t, s.ProcessSyncPush(ctx, req))

	bls, err := s.ListBacklinksByNote(ctx, "n2")
	require.NoError(t, err)
	assert.Len(t, bls, 1)
	assert.Equal(t, "n1", bls[0].LinkedByID)
}

// --- Sync Push ---

func TestProcessSyncPush_UpsertNotes(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	bearID := "bear-uuid-1"

	// First push: insert.
	req := models.SyncPushRequest{
		Notes: []models.Note{
			{BearID: &bearID, Title: "Original", Body: "Original body", SyncStatus: "synced"},
		},
	}
	require.NoError(t, s.ProcessSyncPush(ctx, req))

	notes, err := s.ListNotes(ctx, store.NoteFilter{})
	require.NoError(t, err)
	require.Len(t, notes, 1)
	assert.Equal(t, "Original", notes[0].Title)

	hubID := notes[0].ID

	// Second push: upsert (same bear_id → update).
	req = models.SyncPushRequest{
		Notes: []models.Note{
			{BearID: &bearID, Title: "Updated", Body: "Updated body", SyncStatus: "synced"},
		},
	}
	require.NoError(t, s.ProcessSyncPush(ctx, req))

	notes, err = s.ListNotes(ctx, store.NoteFilter{})
	require.NoError(t, err)
	require.Len(t, notes, 1)
	assert.Equal(t, hubID, notes[0].ID, "hub ID should be preserved")
	assert.Equal(t, "Updated", notes[0].Title)
}

func TestProcessSyncPush_PendingToBear_PreservesBodyTitle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create a note with pending_to_bear status (openclaw changed it).
	bearID := "bear-uuid-1"
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID:         "n1",
		BearID:     &bearID,
		Title:      "OpenClaw Title",
		Body:       "OpenClaw Body",
		SyncStatus: "pending_to_bear",
		Pinned:     0,
	}))

	// Bridge push with Bear data (different title/body, but should not overwrite).
	req := models.SyncPushRequest{
		Notes: []models.Note{
			{BearID: &bearID, Title: "Bear Title", Body: "Bear Body", Pinned: 1, SyncStatus: "synced"},
		},
	}
	require.NoError(t, s.ProcessSyncPush(ctx, req))

	got, err := s.GetNote(ctx, "n1")
	require.NoError(t, err)
	assert.Equal(t, "OpenClaw Title", got.Title, "title should be preserved for pending_to_bear")
	assert.Equal(t, "OpenClaw Body", got.Body, "body should be preserved for pending_to_bear")
	assert.Equal(t, 1, got.Pinned, "flags should be updated from Bear")
}

func TestProcessSyncPush_UpsertTags(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	bearID := "bear-tag-1"
	req := models.SyncPushRequest{
		Tags: []models.Tag{
			{BearID: &bearID, Title: "work", Version: 1},
		},
	}
	require.NoError(t, s.ProcessSyncPush(ctx, req))

	tags, err := s.ListTags(ctx)
	require.NoError(t, err)
	require.Len(t, tags, 1)
	assert.Equal(t, "work", tags[0].Title)

	// Upsert same tag.
	req = models.SyncPushRequest{
		Tags: []models.Tag{
			{BearID: &bearID, Title: "work", Version: 2},
		},
	}
	require.NoError(t, s.ProcessSyncPush(ctx, req))

	tags, err = s.ListTags(ctx)
	require.NoError(t, err)
	require.Len(t, tags, 1)
	assert.Equal(t, 2, tags[0].Version)
}

func TestProcessSyncPush_NoteTags_ScopedReplace(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	bearN1 := "bear-n1"
	bearN2 := "bear-n2"
	bearT1 := "bear-t1"
	bearT2 := "bear-t2"

	// Setup: two notes, two tags, initial associations.
	req := models.SyncPushRequest{
		Notes: []models.Note{
			{BearID: &bearN1, Title: "Note 1", Body: "b", SyncStatus: "synced"},
			{BearID: &bearN2, Title: "Note 2", Body: "b", SyncStatus: "synced"},
		},
		Tags: []models.Tag{
			{BearID: &bearT1, Title: "tag1"},
			{BearID: &bearT2, Title: "tag2"},
		},
		NoteTags: []models.NoteTagPair{
			{NoteID: bearN1, TagID: bearT1},
			{NoteID: bearN2, TagID: bearT2},
		},
	}
	require.NoError(t, s.ProcessSyncPush(ctx, req))

	// Second push: only note1 with tag2. Note2 associations should be untouched.
	req2 := models.SyncPushRequest{
		Notes: []models.Note{
			{BearID: &bearN1, Title: "Note 1 Updated", Body: "b", SyncStatus: "synced"},
		},
		NoteTags: []models.NoteTagPair{
			{NoteID: bearN1, TagID: bearT2},
		},
	}
	require.NoError(t, s.ProcessSyncPush(ctx, req2))

	// Verify note1 now has tag2 only.
	notes, err := s.ListNotes(ctx, store.NoteFilter{Tag: "tag1"})
	require.NoError(t, err)
	assert.Len(t, notes, 0, "note1 should no longer have tag1")

	notes, err = s.ListNotes(ctx, store.NoteFilter{Tag: "tag2"})
	require.NoError(t, err)
	assert.Len(t, notes, 2, "both notes should have tag2")
}

func TestProcessSyncPush_Deletions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	bearID := "bear-del-1"
	req := models.SyncPushRequest{
		Notes: []models.Note{
			{BearID: &bearID, Title: "To Delete", Body: "b", SyncStatus: "synced"},
		},
	}
	require.NoError(t, s.ProcessSyncPush(ctx, req))

	notes, err := s.ListNotes(ctx, store.NoteFilter{})
	require.NoError(t, err)
	require.Len(t, notes, 1)

	// Delete by bear_id.
	req2 := models.SyncPushRequest{
		DeletedNoteIDs: []string{bearID},
	}
	require.NoError(t, s.ProcessSyncPush(ctx, req2))

	notes, err = s.ListNotes(ctx, store.NoteFilter{})
	require.NoError(t, err)
	assert.Len(t, notes, 0)
}

func TestProcessSyncPush_FKCascadeDelete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	bearNoteID := "bear-note-cascade"
	bearTagID := "bear-tag-cascade"

	// First push: create notes and tags.
	req1 := models.SyncPushRequest{
		Notes: []models.Note{
			{BearID: &bearNoteID, Title: "Note", Body: "b", SyncStatus: "synced"},
		},
		Tags: []models.Tag{
			{BearID: &bearTagID, Title: "cascade-tag"},
		},
		NoteTags: []models.NoteTagPair{
			{NoteID: bearNoteID, TagID: bearTagID},
		},
	}
	require.NoError(t, s.ProcessSyncPush(ctx, req1))

	// Get the hub ID of the note for the attachment.
	notes, err := s.ListNotes(ctx, store.NoteFilter{})
	require.NoError(t, err)
	require.Len(t, notes, 1)
	hubNoteID := notes[0].ID

	// Second push: add attachment with hub note_id.
	req2 := models.SyncPushRequest{
		Attachments: []models.Attachment{
			{BearID: strPtr("bear-att-cascade"), NoteID: hubNoteID, Type: "file"},
		},
	}
	require.NoError(t, s.ProcessSyncPush(ctx, req2))

	atts, err := s.ListAttachmentsByNote(ctx, hubNoteID)
	require.NoError(t, err)
	require.Len(t, atts, 1)

	// Delete the note → attachments and note_tags should cascade.
	req3 := models.SyncPushRequest{
		DeletedNoteIDs: []string{bearNoteID},
	}
	require.NoError(t, s.ProcessSyncPush(ctx, req3))

	notes, err = s.ListNotes(ctx, store.NoteFilter{})
	require.NoError(t, err)
	assert.Len(t, notes, 0)

	// Attachment should have been cascade deleted.
	atts, err = s.ListAttachmentsByNote(ctx, hubNoteID)
	require.NoError(t, err)
	assert.Len(t, atts, 0)
}

// --- Write Queue ---

func TestWriteQueue_EnqueueAndIdempotency(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	item1, err := s.EnqueueWrite(ctx, "key-1", "create", "n1", `{"title":"New"}`)
	require.NoError(t, err)
	assert.Equal(t, "pending", item1.Status)
	assert.Equal(t, "create", item1.Action)

	// Same idempotency key → return existing, no duplicate.
	item2, err := s.EnqueueWrite(ctx, "key-1", "create", "n1", `{"title":"New"}`)
	require.NoError(t, err)
	assert.Equal(t, item1.ID, item2.ID)
}

func TestWriteQueue_LeaseAndExpiry(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.EnqueueWrite(ctx, "key-1", "create", "", `{"title":"New"}`)
	require.NoError(t, err)

	// Lease items.
	items, err := s.LeaseQueueItems(ctx, "bridge-1", 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "processing", items[0].Status)
	assert.Equal(t, "bridge-1", items[0].ProcessingBy)

	// No more items to lease.
	items2, err := s.LeaseQueueItems(ctx, "bridge-1", 5*time.Minute)
	require.NoError(t, err)
	assert.Len(t, items2, 0)
}

func TestWriteQueue_LeaseExpiry(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.EnqueueWrite(ctx, "key-exp", "update", "n1", `{"body":"new"}`)
	require.NoError(t, err)

	// Lease items.
	items, err := s.LeaseQueueItems(ctx, "bridge-1", 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, items, 1)

	// Simulate lease expiry by setting lease_until to the past.
	pastTime := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	_, err = s.DB().ExecContext(ctx,
		"UPDATE write_queue SET lease_until = ? WHERE idempotency_key = ?",
		pastTime, "key-exp",
	)
	require.NoError(t, err)

	// Lease again - expired item should be available.
	items, err = s.LeaseQueueItems(ctx, "bridge-2", 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "bridge-2", items[0].ProcessingBy)
}

func TestWriteQueue_AckApplied(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	item, err := s.EnqueueWrite(ctx, "key-ack", "create", "n1", `{"title":"Created"}`)
	require.NoError(t, err)

	// Lease.
	_, err = s.LeaseQueueItems(ctx, "bridge-1", 5*time.Minute)
	require.NoError(t, err)

	// Ack with bear_id.
	err = s.AckQueueItems(ctx, []models.SyncAckItem{
		{QueueID: item.ID, IdempotencyKey: "key-ack", Status: "applied", BearID: "bear-new-123"},
	})
	require.NoError(t, err)

	// Verify: repeated ack is no-op.
	err = s.AckQueueItems(ctx, []models.SyncAckItem{
		{QueueID: item.ID, IdempotencyKey: "key-ack", Status: "applied", BearID: "bear-new-123"},
	})
	require.NoError(t, err)
}

func TestWriteQueue_AckApplied_FillsBearID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create a note with pending_to_bear (created by openclaw, no bear_id yet).
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID:         "n-new",
		Title:      "New Note",
		Body:       "body",
		SyncStatus: "pending_to_bear",
	}))

	item, err := s.EnqueueWrite(ctx, "key-fill", "create", "n-new", `{"title":"New Note"}`)
	require.NoError(t, err)

	_, err = s.LeaseQueueItems(ctx, "bridge-1", 5*time.Minute)
	require.NoError(t, err)

	// Ack with bear_id.
	err = s.AckQueueItems(ctx, []models.SyncAckItem{
		{QueueID: item.ID, IdempotencyKey: "key-fill", Status: "applied", BearID: "bear-created-uuid"},
	})
	require.NoError(t, err)

	// Verify note now has bear_id and synced status.
	got, err := s.GetNote(ctx, "n-new")
	require.NoError(t, err)
	require.NotNil(t, got.BearID)
	assert.Equal(t, "bear-created-uuid", *got.BearID)
	assert.Equal(t, "synced", got.SyncStatus)
}

func TestWriteQueue_AckFailed(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	item, err := s.EnqueueWrite(ctx, "key-fail", "trash", "n1", `{}`)
	require.NoError(t, err)

	_, err = s.LeaseQueueItems(ctx, "bridge-1", 5*time.Minute)
	require.NoError(t, err)

	err = s.AckQueueItems(ctx, []models.SyncAckItem{
		{QueueID: item.ID, IdempotencyKey: "key-fail", Status: "failed", Error: "Bear not running"},
	})
	require.NoError(t, err)
}

func TestWriteQueue_FullLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Enqueue.
	item, err := s.EnqueueWrite(ctx, "lifecycle-key", "update", "n1", `{"body":"new"}`)
	require.NoError(t, err)
	assert.Equal(t, "pending", item.Status)

	// Lease.
	items, err := s.LeaseQueueItems(ctx, "bridge-1", 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "processing", items[0].Status)

	// Ack.
	err = s.AckQueueItems(ctx, []models.SyncAckItem{
		{QueueID: items[0].ID, IdempotencyKey: "lifecycle-key", Status: "applied"},
	})
	require.NoError(t, err)

	// Nothing left to lease.
	items, err = s.LeaseQueueItems(ctx, "bridge-1", 5*time.Minute)
	require.NoError(t, err)
	assert.Len(t, items, 0)
}

// --- Sync Meta ---

func TestSyncMeta(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Get non-existent key.
	val, err := s.GetSyncMeta(ctx, "last_sync_at")
	require.NoError(t, err)
	assert.Equal(t, "", val)

	// Set and get.
	err = s.SetSyncMeta(ctx, "last_sync_at", "2025-01-01T00:00:00Z")
	require.NoError(t, err)

	val, err = s.GetSyncMeta(ctx, "last_sync_at")
	require.NoError(t, err)
	assert.Equal(t, "2025-01-01T00:00:00Z", val)

	// Upsert.
	err = s.SetSyncMeta(ctx, "last_sync_at", "2025-01-02T00:00:00Z")
	require.NoError(t, err)

	val, err = s.GetSyncMeta(ctx, "last_sync_at")
	require.NoError(t, err)
	assert.Equal(t, "2025-01-02T00:00:00Z", val)
}

// --- Migration ---

func TestNewSQLiteStore_Migration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	s, err := store.NewSQLiteStore(dbPath)
	require.NoError(t, err)

	// Verify tables exist.
	var count int
	err = s.DB().QueryRow(
		"SELECT count(*) FROM sqlite_master WHERE type='table' AND " +
			"name IN ('notes','tags','note_tags','pinned_note_tags'," +
			"'attachments','backlinks','write_queue','sync_meta')",
	).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 8, count)

	// Verify FTS5 table.
	err = s.DB().QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='notes_fts'").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	require.NoError(t, s.Close())

	// Reopen — migration should be idempotent.
	s2, err := store.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	require.NoError(t, s2.Close())
}

// --- Reopen and persistence ---

func TestStore_Persistence(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	ctx := context.Background()

	s, err := store.NewSQLiteStore(dbPath)
	require.NoError(t, err)

	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID: "n1", Title: "Persistent", Body: "data", SyncStatus: "synced",
	}))
	require.NoError(t, s.Close())

	s2, err := store.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	defer func() { require.NoError(t, s2.Close()) }()

	got, err := s2.GetNote(ctx, "n1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Persistent", got.Title)
}

// --- Note with tags loaded ---

func TestGetNote_WithTagsAndBacklinks(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.CreateNote(ctx, &models.Note{ID: "n1", Title: "Note 1", Body: "b", SyncStatus: "synced"}))
	require.NoError(t, s.CreateNote(ctx, &models.Note{ID: "n2", Title: "Note 2", Body: "b", SyncStatus: "synced"}))
	require.NoError(t, s.CreateTag(ctx, &models.Tag{ID: "t1", Title: "work"}))

	_, err := s.DB().ExecContext(ctx, "INSERT INTO note_tags (note_id, tag_id) VALUES (?, ?)", "n1", "t1")
	require.NoError(t, err)

	req := models.SyncPushRequest{
		Backlinks: []models.Backlink{
			{ID: "bl1", LinkedByID: "n2", LinkingToID: "n1", Title: "ref"},
		},
	}
	require.NoError(t, s.ProcessSyncPush(ctx, req))

	got, err := s.GetNote(ctx, "n1")
	require.NoError(t, err)
	require.Len(t, got.Tags, 1)
	assert.Equal(t, "work", got.Tags[0].Title)
	require.Len(t, got.Backlinks, 1)
	assert.Equal(t, "n2", got.Backlinks[0].LinkedByID)
}

// --- Conflict Detection ---

func TestProcessSyncPush_ConflictDetection(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create a note with pending_to_bear status and a known modified_at.
	bearID := "bear-conflict-1"
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID:            "n1",
		BearID:        &bearID,
		Title:         "OpenClaw Title",
		Body:          "OpenClaw Body",
		SyncStatus:    "pending_to_bear",
		HubModifiedAt: "2025-01-01T12:00:00Z",
		ModifiedAt:    "2025-01-01T10:00:00Z",
	}))

	// Bridge pushes the same note with a DIFFERENT modified_at (user changed it in Bear).
	req := models.SyncPushRequest{
		Notes: []models.Note{
			{
				BearID:     &bearID,
				Title:      "Bear Title",
				Body:       "Bear Body",
				ModifiedAt: "2025-01-01T11:00:00Z", // Changed since last push
				SyncStatus: "synced",
			},
		},
	}
	require.NoError(t, s.ProcessSyncPush(ctx, req))

	got, err := s.GetNote(ctx, "n1")
	require.NoError(t, err)
	assert.Equal(t, "conflict", got.SyncStatus, "sync_status should be conflict")
	assert.Equal(t, "OpenClaw Title", got.Title, "title should be preserved")
	assert.Equal(t, "OpenClaw Body", got.Body, "body should be preserved")
}

func TestProcessSyncPush_NoConflictOnSameModifiedAt(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create a note with pending_to_bear status.
	bearID := "bear-noconflict-1"
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID:         "n1",
		BearID:     &bearID,
		Title:      "OpenClaw Title",
		Body:       "OpenClaw Body",
		SyncStatus: "pending_to_bear",
		ModifiedAt: "2025-01-01T10:00:00Z",
	}))

	// Bridge pushes with the SAME modified_at (overlap-window duplicate, not a real change).
	req := models.SyncPushRequest{
		Notes: []models.Note{
			{
				BearID:     &bearID,
				Title:      "Bear Title",
				Body:       "Bear Body",
				ModifiedAt: "2025-01-01T10:00:00Z", // Same as stored
				SyncStatus: "synced",
			},
		},
	}
	require.NoError(t, s.ProcessSyncPush(ctx, req))

	got, err := s.GetNote(ctx, "n1")
	require.NoError(t, err)
	assert.Equal(t, "pending_to_bear", got.SyncStatus, "sync_status should stay pending_to_bear")
	assert.Equal(t, "OpenClaw Title", got.Title, "title should be preserved")
}

func TestCountConflicts(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	count, err := s.CountConflicts(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	bearID1 := "bear-1"
	bearID2 := "bear-2"
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID: "n1", BearID: &bearID1, Title: "Note 1", SyncStatus: "conflict",
	}))
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID: "n2", BearID: &bearID2, Title: "Note 2", SyncStatus: "synced",
	}))

	count, err = s.CountConflicts(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestListConflictNoteIDs(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	ids, err := s.ListConflictNoteIDs(ctx)
	require.NoError(t, err)
	assert.Empty(t, ids)

	bearID1 := "bear-1"
	bearID2 := "bear-2"
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID: "n1", BearID: &bearID1, Title: "Note 1", SyncStatus: "conflict", ModifiedAt: "2025-01-01T12:00:00Z",
	}))
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID: "n2", BearID: &bearID2, Title: "Note 2", SyncStatus: "conflict", ModifiedAt: "2025-01-02T12:00:00Z",
	}))

	ids, err = s.ListConflictNoteIDs(ctx)
	require.NoError(t, err)
	require.Len(t, ids, 2)
	assert.Equal(t, "n2", ids[0]) // Most recent first
	assert.Equal(t, "n1", ids[1])
}

func TestLeaseQueueItems_IncludesNoteSyncStatus(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create a note with conflict status.
	bearID := "bear-conflict"
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID: "n1", BearID: &bearID, Title: "Conflicted Note", SyncStatus: "conflict",
	}))

	// Enqueue a write for the conflicted note.
	_, err := s.EnqueueWrite(ctx, "idem-1", "update", "n1", `{"body":"new body"}`)
	require.NoError(t, err)

	// Lease and verify sync_status is included.
	items, err := s.LeaseQueueItems(ctx, "bridge", 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "conflict", items[0].NoteSyncStatus)
}

// --- Temp dir cleanup ---

func TestStore_CleanupOnClose(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cleanup.db")
	s, err := store.NewSQLiteStore(dbPath)
	require.NoError(t, err)

	err = s.Close()
	require.NoError(t, err)

	// DB file should still exist (we don't delete on close).
	_, err = os.Stat(dbPath)
	require.NoError(t, err)
}
