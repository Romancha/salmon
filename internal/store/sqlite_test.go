package store_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/romancha/salmon/internal/models"
	"github.com/romancha/salmon/internal/store"

	_ "modernc.org/sqlite"
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

	// Create a note with pending_to_bear status (a consumer changed it).
	bearID := "bear-uuid-1"
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID:         "n1",
		BearID:     &bearID,
		Title:      "Consumer Title",
		Body:       "Consumer Body",
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
	assert.Equal(t, "Consumer Title", got.Title, "title should be preserved for pending_to_bear")
	assert.Equal(t, "Consumer Body", got.Body, "body should be preserved for pending_to_bear")
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

	item1, err := s.EnqueueWrite(ctx, "key-1", "create", "n1", `{"title":"New"}`, "")
	require.NoError(t, err)
	assert.Equal(t, "pending", item1.Status)
	assert.Equal(t, "create", item1.Action)

	// Same idempotency key → return existing, no duplicate.
	item2, err := s.EnqueueWrite(ctx, "key-1", "create", "n1", `{"title":"New"}`, "")
	require.NoError(t, err)
	assert.Equal(t, item1.ID, item2.ID)
}

func TestWriteQueue_LeaseAndExpiry(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.EnqueueWrite(ctx, "key-1", "create", "", `{"title":"New"}`, "")
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

	_, err := s.EnqueueWrite(ctx, "key-exp", "update", "n1", `{"body":"new"}`, "")
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

	item, err := s.EnqueueWrite(ctx, "key-ack", "create", "n1", `{"title":"Created"}`, "")
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

	// Verify: queue item was removed after ack.
	leased, err := s.LeaseQueueItems(ctx, "bridge-check", 5*time.Minute)
	require.NoError(t, err)
	assert.Nil(t, leased, "queue should be empty after ack")
}

func TestWriteQueue_AckApplied_FillsBearID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create a note with pending_to_bear (created by a consumer, no bear_id yet).
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID:         "n-new",
		Title:      "New Note",
		Body:       "body",
		SyncStatus: "pending_to_bear",
	}))

	item, err := s.EnqueueWrite(ctx, "key-fill", "create", "n-new", `{"title":"New Note"}`, "")
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

	item, err := s.EnqueueWrite(ctx, "key-fail", "trash", "n1", `{}`, "")
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
	item, err := s.EnqueueWrite(ctx, "lifecycle-key", "update", "n1", `{"body":"new"}`, "")
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

func TestWriteQueue_SchemaHasConsumerIDColumn(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	var colCount int
	err := s.DB().QueryRowContext(ctx,
		"SELECT count(*) FROM pragma_table_info('write_queue') WHERE name = 'consumer_id'",
	).Scan(&colCount)
	require.NoError(t, err)
	assert.Equal(t, 1, colCount, "write_queue should have consumer_id column")

	// Verify the default value by inserting without consumer_id and reading back.
	_, err = s.DB().ExecContext(ctx,
		"INSERT INTO write_queue (idempotency_key, action, payload) VALUES ('raw-key', 'create', '{}')",
	)
	require.NoError(t, err)

	var consumerID string
	err = s.DB().QueryRowContext(ctx,
		"SELECT consumer_id FROM write_queue WHERE idempotency_key = 'raw-key'",
	).Scan(&consumerID)
	require.NoError(t, err)
	assert.Equal(t, "", consumerID, "consumer_id default should be empty string")
}

func TestWriteQueue_EnqueueWithConsumerID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	item, err := s.EnqueueWrite(ctx, "key-consumer", "create", "", `{"title":"Test"}`, "testapp")
	require.NoError(t, err)
	assert.Equal(t, "testapp", item.ConsumerID)
	assert.Equal(t, "pending", item.Status)

	// Verify via GetQueueItemByIdempotencyKey.
	got, err := s.GetQueueItemByIdempotencyKey(ctx, "key-consumer", "testapp")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "testapp", got.ConsumerID)
}

func TestWriteQueue_EnqueueWithEmptyConsumerID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	item, err := s.EnqueueWrite(ctx, "key-empty", "create", "", `{"title":"Test"}`, "")
	require.NoError(t, err)
	assert.Equal(t, "", item.ConsumerID)
}

func TestWriteQueue_LeaseReturnsConsumerID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.EnqueueWrite(ctx, "key-lease-cid", "create", "", `{"title":"Test"}`, "myapp")
	require.NoError(t, err)

	items, err := s.LeaseQueueItems(ctx, "bridge-1", 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "myapp", items[0].ConsumerID)
}

func TestWriteQueue_IdempotencyReturnsConsumerID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.EnqueueWrite(ctx, "key-idem-cid", "create", "", `{"title":"Test"}`, "testapp")
	require.NoError(t, err)

	// Second call with same key returns existing item with consumer_id.
	item, err := s.EnqueueWrite(ctx, "key-idem-cid", "create", "", `{"title":"Test"}`, "testapp")
	require.NoError(t, err)
	assert.Equal(t, "testapp", item.ConsumerID)
}

func TestWriteQueue_IdempotencyIsScopedToConsumer(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Two consumers using the same idempotency key should not collide.
	item1, err := s.EnqueueWrite(ctx, "shared-key", "create", "n1", `{"title":"From A"}`, "consumer-a")
	require.NoError(t, err)
	assert.Equal(t, "consumer-a", item1.ConsumerID)

	item2, err := s.EnqueueWrite(ctx, "shared-key", "create", "n2", `{"title":"From B"}`, "consumer-b")
	require.NoError(t, err)
	assert.Equal(t, "consumer-b", item2.ConsumerID)

	// They should be different queue items.
	assert.NotEqual(t, item1.ID, item2.ID)

	// Lookup by key+consumer returns the correct item.
	got, err := s.GetQueueItemByIdempotencyKey(ctx, "shared-key", "consumer-a")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, item1.ID, got.ID)

	got2, err := s.GetQueueItemByIdempotencyKey(ctx, "shared-key", "consumer-b")
	require.NoError(t, err)
	require.NotNil(t, got2)
	assert.Equal(t, item2.ID, got2.ID)
}

func TestWriteQueue_FailedItemRetry(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Enqueue, lease, then fail the item.
	item, err := s.EnqueueWrite(ctx, "retry-key", "create", "n1", `{"title":"Original"}`, "testapp")
	require.NoError(t, err)
	assert.Equal(t, "pending", item.Status)

	items, err := s.LeaseQueueItems(ctx, "bridge-1", 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, items, 1)

	err = s.AckQueueItems(ctx, []models.SyncAckItem{
		{QueueID: item.ID, IdempotencyKey: "retry-key", Status: "failed", Error: "Bear not running"},
	})
	require.NoError(t, err)

	// Retry with the same idempotency key should reset the item to pending.
	retried, err := s.EnqueueWrite(ctx, "retry-key", "create", "n1", `{"title":"Retried"}`, "testapp")
	require.NoError(t, err)
	assert.Equal(t, item.ID, retried.ID, "should reuse same queue item ID")
	assert.Equal(t, "pending", retried.Status, "status should be reset to pending")
	assert.Equal(t, `{"title":"Retried"}`, retried.Payload, "payload should be updated")

	// The item should be leasable again.
	items, err = s.LeaseQueueItems(ctx, "bridge-1", 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, retried.ID, items[0].ID)
}

func TestWriteQueue_StaleAckAfterRetryReset(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Enqueue, lease, and fail the item.
	item, err := s.EnqueueWrite(ctx, "stale-key", "create", "n1", `{"title":"V1"}`, "testapp")
	require.NoError(t, err)

	_, err = s.LeaseQueueItems(ctx, "bridge-1", 5*time.Minute)
	require.NoError(t, err)

	err = s.AckQueueItems(ctx, []models.SyncAckItem{
		{QueueID: item.ID, IdempotencyKey: "stale-key", Status: "failed", Error: "Bear not running"},
	})
	require.NoError(t, err)

	// Consumer retries — resets item to pending.
	retried, err := s.EnqueueWrite(ctx, "stale-key", "create", "n1", `{"title":"V2"}`, "testapp")
	require.NoError(t, err)
	assert.Equal(t, "pending", retried.Status)

	// A stale "failed" ack arrives (duplicate from the first attempt).
	// It must be a no-op because the item was reset to pending.
	err = s.AckQueueItems(ctx, []models.SyncAckItem{
		{QueueID: item.ID, IdempotencyKey: "stale-key", Status: "failed", Error: "stale error"},
	})
	require.NoError(t, err)

	// Verify item is still pending (not corrupted back to failed).
	items, err := s.LeaseQueueItems(ctx, "bridge-2", 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, items, 1, "item should still be leasable after stale ack")
	assert.Equal(t, retried.ID, items[0].ID)
}

func TestWriteQueue_MigrationAddsConsumerIDColumn(t *testing.T) {
	// Create a store with the old schema (no consumer_id column).
	dbPath := filepath.Join(t.TempDir(), "test-migration.db")

	ctx := context.Background()

	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)

	// Create old-schema write_queue without consumer_id.
	_, err = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS write_queue (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		idempotency_key TEXT UNIQUE NOT NULL,
		action          TEXT NOT NULL,
		note_id         TEXT,
		payload         TEXT NOT NULL,
		created_at      TEXT DEFAULT (datetime('now')),
		status          TEXT DEFAULT 'pending',
		processing_by   TEXT,
		lease_until     TEXT,
		applied_at      TEXT,
		error           TEXT
	)`)
	require.NoError(t, err)

	// Insert a row with old schema.
	_, err = db.ExecContext(ctx,
		"INSERT INTO write_queue (idempotency_key, action, note_id, payload) VALUES (?, ?, ?, ?)",
		"old-key", "create", "n1", `{"title":"Old"}`,
	)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	// Reopen with NewSQLiteStore — migration should add consumer_id.
	s, err := store.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })

	// Verify the old row is readable and has default consumer_id.
	item, err := s.GetQueueItemByIdempotencyKey(context.Background(), "old-key", "")
	require.NoError(t, err)
	require.NotNil(t, item)
	assert.Equal(t, "", item.ConsumerID)
	assert.Equal(t, "create", item.Action)

	// Verify new rows can be inserted with consumer_id.
	newItem, err := s.EnqueueWrite(context.Background(), "new-key", "update", "n2", `{"body":"new"}`, "myapp")
	require.NoError(t, err)
	assert.Equal(t, "myapp", newItem.ConsumerID)

	// Verify same idempotency key can be used by different consumers after migration
	// (old schema had global UNIQUE on idempotency_key; migrated schema has UNIQUE(idempotency_key, consumer_id)).
	itemA, err := s.EnqueueWrite(context.Background(), "shared-key", "create", "n3", `{"title":"A"}`, "consumer-a")
	require.NoError(t, err)
	assert.Equal(t, "consumer-a", itemA.ConsumerID)

	itemB, err := s.EnqueueWrite(context.Background(), "shared-key", "create", "n4", `{"title":"B"}`, "consumer-b")
	require.NoError(t, err)
	assert.Equal(t, "consumer-b", itemB.ConsumerID)
	assert.NotEqual(t, itemA.ID, itemB.ID, "different consumers should get separate queue items")
}

func TestWriteQueue_MigrationFixesIntermediateSchema(t *testing.T) {
	// Simulate a DB created by an intermediate schema version: consumer_id column exists,
	// but UNIQUE constraint is still on idempotency_key alone (not compound).
	dbPath := filepath.Join(t.TempDir(), "test-intermediate-migration.db")

	ctx := context.Background()

	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)

	// Create intermediate schema: has consumer_id but old UNIQUE(idempotency_key).
	_, err = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS write_queue (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		idempotency_key TEXT UNIQUE NOT NULL,
		action          TEXT NOT NULL,
		note_id         TEXT,
		payload         TEXT NOT NULL,
		created_at      TEXT DEFAULT (datetime('now')),
		status          TEXT DEFAULT 'pending',
		processing_by   TEXT,
		lease_until     TEXT,
		applied_at      TEXT,
		error           TEXT,
		consumer_id     TEXT NOT NULL DEFAULT ''
	)`)
	require.NoError(t, err)

	// Insert a row.
	_, err = db.ExecContext(ctx,
		"INSERT INTO write_queue (idempotency_key, action, note_id, payload, consumer_id) VALUES (?, ?, ?, ?, ?)",
		"key1", "create", "n1", `{"title":"Test"}`, "consumer-a",
	)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	// Reopen with NewSQLiteStore — migration should detect missing compound unique and rebuild table.
	s, err := store.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })

	// Verify the old row survived migration with consumer_id preserved.
	item, err := s.GetQueueItemByIdempotencyKey(context.Background(), "key1", "consumer-a")
	require.NoError(t, err)
	require.NotNil(t, item)
	assert.Equal(t, "consumer-a", item.ConsumerID, "migration should preserve existing consumer_id")

	// Verify same idempotency key can now be used by different consumers
	// (this would have failed with the old global UNIQUE constraint).
	itemA, err := s.EnqueueWrite(context.Background(), "dup-key", "create", "n2", `{"title":"A"}`, "consumer-a")
	require.NoError(t, err)

	itemB, err := s.EnqueueWrite(context.Background(), "dup-key", "create", "n3", `{"title":"B"}`, "consumer-b")
	require.NoError(t, err)
	assert.NotEqual(t, itemA.ID, itemB.ID, "different consumers should get separate queue items")
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
	ctx := context.Background()

	s, err := store.NewSQLiteStore(dbPath)
	require.NoError(t, err)

	// Verify tables exist.
	var count int
	err = s.DB().QueryRowContext(ctx,
		"SELECT count(*) FROM sqlite_master WHERE type='table' AND "+
			"name IN ('notes','tags','note_tags','pinned_note_tags',"+
			"'attachments','backlinks','write_queue','sync_meta')",
	).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 8, count)

	// Verify FTS5 table.
	err = s.DB().QueryRowContext(ctx,
		"SELECT count(*) FROM sqlite_master WHERE type='table' AND name='notes_fts'",
	).Scan(&count)
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

func TestGetNote_WithTagsBacklinksAndAttachments(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.CreateNote(ctx, &models.Note{ID: "n1", Title: "Note 1", Body: "b", SyncStatus: "synced"}))
	require.NoError(t, s.CreateNote(ctx, &models.Note{ID: "n2", Title: "Note 2", Body: "b", SyncStatus: "synced"}))
	require.NoError(t, s.CreateTag(ctx, &models.Tag{ID: "t1", Title: "work"}))

	_, err := s.DB().ExecContext(ctx, "INSERT INTO note_tags (note_id, tag_id) VALUES (?, ?)", "n1", "t1")
	require.NoError(t, err)

	bearID := "bear-att-1"
	req := models.SyncPushRequest{
		Backlinks: []models.Backlink{
			{ID: "bl1", LinkedByID: "n2", LinkingToID: "n1", Title: "ref"},
		},
		Attachments: []models.Attachment{
			{ID: "att1", BearID: &bearID, NoteID: "n1", Type: "image", Filename: "photo.jpg", FileSize: 2048, Width: 800, Height: 600},
		},
	}
	require.NoError(t, s.ProcessSyncPush(ctx, req))

	got, err := s.GetNote(ctx, "n1")
	require.NoError(t, err)
	require.Len(t, got.Tags, 1)
	assert.Equal(t, "work", got.Tags[0].Title)
	require.Len(t, got.Backlinks, 1)
	assert.Equal(t, "n2", got.Backlinks[0].LinkedByID)
	require.Len(t, got.Attachments, 1)
	assert.Equal(t, "att1", got.Attachments[0].ID)
	assert.Equal(t, "image", got.Attachments[0].Type)
	assert.Equal(t, "photo.jpg", got.Attachments[0].Filename)
	assert.Equal(t, int64(2048), got.Attachments[0].FileSize)
	assert.Equal(t, 800, got.Attachments[0].Width)
	assert.Equal(t, 600, got.Attachments[0].Height)
}

func TestListNotes_WithAttachments(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.CreateNote(ctx, &models.Note{ID: "n1", Title: "Note with image", SyncStatus: "synced"}))
	require.NoError(t, s.CreateNote(ctx, &models.Note{ID: "n2", Title: "Note without attachments", SyncStatus: "synced"}))

	bearID := "bear-att-list"
	req := models.SyncPushRequest{
		Attachments: []models.Attachment{
			{ID: "att1", BearID: &bearID, NoteID: "n1", Type: "image", Filename: "pic.png", FileSize: 512},
		},
	}
	require.NoError(t, s.ProcessSyncPush(ctx, req))

	notes, err := s.ListNotes(ctx, store.NoteFilter{Limit: 50})
	require.NoError(t, err)
	require.Len(t, notes, 2)

	var withAtt, withoutAtt models.Note
	for _, n := range notes {
		if n.ID == "n1" {
			withAtt = n
		} else {
			withoutAtt = n
		}
	}

	require.Len(t, withAtt.Attachments, 1)
	assert.Equal(t, "att1", withAtt.Attachments[0].ID)
	assert.Equal(t, "image", withAtt.Attachments[0].Type)
	assert.Empty(t, withoutAtt.Attachments)
}

func TestSearchNotes_WithAttachments(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID: "n1", Title: "Searchable Note", Body: "unique keyword here", SyncStatus: "synced",
	}))

	bearID := "bear-att-search"
	req := models.SyncPushRequest{
		Attachments: []models.Attachment{
			{ID: "att1", BearID: &bearID, NoteID: "n1", Type: "file", Filename: "doc.pdf", FileSize: 4096},
		},
	}
	require.NoError(t, s.ProcessSyncPush(ctx, req))

	results, err := s.SearchNotes(ctx, "unique keyword", "", 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Len(t, results[0].Attachments, 1)
	assert.Equal(t, "att1", results[0].Attachments[0].ID)
	assert.Equal(t, "file", results[0].Attachments[0].Type)
	assert.Equal(t, "doc.pdf", results[0].Attachments[0].Filename)
}

// --- Conflict Detection ---

func TestProcessSyncPush_ConflictDetection(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create a note with pending_to_bear status, pending_bear fields capture
	// Bear's original content before consumer overwrote title/body.
	bearID := "bear-conflict-1"
	origTitle := "Original Bear Title"
	origBody := "Original Bear Body"
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID:               "n1",
		BearID:           &bearID,
		Title:            "Consumer Title",
		Body:             "Consumer Body",
		SyncStatus:       "pending_to_bear",
		HubModifiedAt:    "2025-01-01T12:00:00Z",
		ModifiedAt:       "2025-01-01T10:00:00Z",
		PendingBearTitle: &origTitle,
		PendingBearBody:  &origBody,
	}))

	// Bridge pushes the same note with a DIFFERENT modified_at AND changed body
	// (Bear user edited body). Consumer also changed body → field intersection → conflict.
	req := models.SyncPushRequest{
		Notes: []models.Note{
			{
				BearID:     &bearID,
				Title:      "Original Bear Title",
				Body:       "Bear Edited Body",
				ModifiedAt: "2025-01-01T11:00:00Z",
				SyncStatus: "synced",
			},
		},
	}
	require.NoError(t, s.ProcessSyncPush(ctx, req))

	got, err := s.GetNote(ctx, "n1")
	require.NoError(t, err)
	assert.Equal(t, "conflict", got.SyncStatus, "sync_status should be conflict")
	assert.Equal(t, "Consumer Title", got.Title, "consumer title should be preserved")
	assert.Equal(t, "Consumer Body", got.Body, "consumer body should be preserved")
}

func TestProcessSyncPush_NoConflictOnSameModifiedAt(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create a note with pending_to_bear status.
	bearID := "bear-noconflict-1"
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID:         "n1",
		BearID:     &bearID,
		Title:      "Consumer Title",
		Body:       "Consumer Body",
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
	assert.Equal(t, "Consumer Title", got.Title, "title should be preserved")
}

func TestProcessSyncPush_MetadataOnlyChangeNoConflict(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create a note with pending_to_bear status and pending_bear fields.
	// Consumer changed the body, Bear's original content is captured in pending_bear fields.
	bearID := "bear-metadata-1"
	origTitle := "Same Title"
	origBody := "Original Body"
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID:               "n1",
		BearID:           &bearID,
		Title:            "Same Title",
		Body:             "Consumer Body",
		SyncStatus:       "pending_to_bear",
		ModifiedAt:       "2025-01-01T10:00:00Z",
		PendingBearTitle: &origTitle,
		PendingBearBody:  &origBody,
	}))

	// Bridge pushes with a DIFFERENT modified_at but same content as pending_bear.
	// This is a metadata-only change (e.g. user opened the note in Bear).
	req := models.SyncPushRequest{
		Notes: []models.Note{
			{
				BearID:     &bearID,
				Title:      "Same Title",
				Body:       "Original Body",
				ModifiedAt: "2025-01-01T11:00:00Z", // Changed
				SyncStatus: "synced",
			},
		},
	}
	require.NoError(t, s.ProcessSyncPush(ctx, req))

	got, err := s.GetNote(ctx, "n1")
	require.NoError(t, err)
	assert.Equal(t, "pending_to_bear", got.SyncStatus, "metadata-only change should NOT trigger conflict")
	assert.Equal(t, "Same Title", got.Title, "title should be preserved")
	assert.Equal(t, "Consumer Body", got.Body, "consumer body should be preserved")
	require.NotNil(t, got.PendingBearTitle, "PendingBearTitle should survive Bear delta push")
	assert.Equal(t, "Same Title", *got.PendingBearTitle)
	require.NotNil(t, got.PendingBearBody, "PendingBearBody should survive Bear delta push")
	assert.Equal(t, "Original Body", *got.PendingBearBody)
}

func TestProcessSyncPush_FieldLevelConflict(t *testing.T) {
	tests := []struct {
		name             string
		hubTitle         string
		hubBody          string
		pendingBearTitle *string
		pendingBearBody  *string
		bearTitle        string
		bearBody         string
		wantSyncStatus   string
	}{
		{
			name:             "metadata drift only - no conflict",
			hubTitle:         "Consumer Title",
			hubBody:          "Consumer Body",
			pendingBearTitle: strPtr("Bear Title"),
			pendingBearBody:  strPtr("Bear Body"),
			bearTitle:        "Bear Title",
			bearBody:         "Bear Body",
			wantSyncStatus:   "pending_to_bear",
		},
		{
			name:             "both Bear and consumer changed body - conflict",
			hubTitle:         "Same Title",
			hubBody:          "Consumer Body",
			pendingBearTitle: strPtr("Same Title"),
			pendingBearBody:  strPtr("Original Body"),
			bearTitle:        "Same Title",
			bearBody:         "Bear Changed Body",
			wantSyncStatus:   "conflict",
		},
		{
			name:             "Bear changed title, consumer changed body - no field intersection",
			hubTitle:         "Same Title",
			hubBody:          "Consumer Body",
			pendingBearTitle: strPtr("Same Title"),
			pendingBearBody:  strPtr("Original Body"),
			bearTitle:        "Bear Changed Title",
			bearBody:         "Original Body",
			wantSyncStatus:   "pending_to_bear",
		},
		{
			name:             "Bear changed both, consumer changed body - body intersects",
			hubTitle:         "Same Title",
			hubBody:          "Consumer Body",
			pendingBearTitle: strPtr("Same Title"),
			pendingBearBody:  strPtr("Original Body"),
			bearTitle:        "Bear Changed Title",
			bearBody:         "Bear Changed Body",
			wantSyncStatus:   "conflict",
		},
		{
			name:             "both Bear and consumer changed title only - conflict",
			hubTitle:         "Consumer Title",
			hubBody:          "Same Body",
			pendingBearTitle: strPtr("Original Title"),
			pendingBearBody:  strPtr("Same Body"),
			bearTitle:        "Bear Changed Title",
			bearBody:         "Same Body",
			wantSyncStatus:   "conflict",
		},
		{
			name:             "consumer changed both, Bear changed title - title intersects",
			hubTitle:         "Consumer Title",
			hubBody:          "Consumer Body",
			pendingBearTitle: strPtr("Original Title"),
			pendingBearBody:  strPtr("Original Body"),
			bearTitle:        "Bear Changed Title",
			bearBody:         "Original Body",
			wantSyncStatus:   "conflict",
		},
		{
			name:             "NULL pending_bear fields - fallback to timestamp conflict",
			hubTitle:         "Consumer Title",
			hubBody:          "Consumer Body",
			pendingBearTitle: nil,
			pendingBearBody:  nil,
			bearTitle:        "Bear Title",
			bearBody:         "Bear Body",
			wantSyncStatus:   "conflict",
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			ctx := context.Background()

			bearID := fmt.Sprintf("bear-field-%d", i)
			require.NoError(t, s.CreateNote(ctx, &models.Note{
				ID:               "n1",
				BearID:           &bearID,
				Title:            tt.hubTitle,
				Body:             tt.hubBody,
				SyncStatus:       "pending_to_bear",
				ModifiedAt:       "2025-01-01T10:00:00Z",
				PendingBearTitle: tt.pendingBearTitle,
				PendingBearBody:  tt.pendingBearBody,
			}))

			req := models.SyncPushRequest{
				Notes: []models.Note{
					{
						BearID:     &bearID,
						Title:      tt.bearTitle,
						Body:       tt.bearBody,
						ModifiedAt: "2025-01-01T11:00:00Z",
						SyncStatus: "synced",
					},
				},
			}
			require.NoError(t, s.ProcessSyncPush(ctx, req))

			got, err := s.GetNote(ctx, "n1")
			require.NoError(t, err)
			assert.Equal(t, tt.wantSyncStatus, got.SyncStatus)
			assert.Equal(t, tt.hubTitle, got.Title, "consumer title preserved")
			assert.Equal(t, tt.hubBody, got.Body, "consumer body preserved")
		})
	}
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
	_, err := s.EnqueueWrite(ctx, "idem-1", "update", "n1", `{"body":"new body"}`, "")
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

func TestPendingBearColumnsExistAndNullable(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create a note without setting pending_bear fields — they should default to NULL.
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID:    "pending-bear-col-test",
		Title: "Test",
		Body:  "Body",
	}))

	// Verify columns exist and are NULL by default.
	var pendingTitle, pendingBody sql.NullString
	err := s.DB().QueryRowContext(ctx,
		"SELECT pending_bear_title, pending_bear_body FROM notes WHERE id = ?",
		"pending-bear-col-test",
	).Scan(&pendingTitle, &pendingBody)
	require.NoError(t, err)
	assert.False(t, pendingTitle.Valid, "pending_bear_title should be NULL by default")
	assert.False(t, pendingBody.Valid, "pending_bear_body should be NULL by default")

	// Verify fields round-trip through CreateNote → GetNote.
	note, err := s.GetNote(ctx, "pending-bear-col-test")
	require.NoError(t, err)
	assert.Nil(t, note.PendingBearTitle, "PendingBearTitle should be nil from NULL")
	assert.Nil(t, note.PendingBearBody, "PendingBearBody should be nil from NULL")

	// Verify we can write non-NULL values and read them back.
	_, err = s.DB().ExecContext(ctx,
		"UPDATE notes SET pending_bear_title = ?, pending_bear_body = ? WHERE id = ?",
		"Bear Title", "Bear Body", "pending-bear-col-test",
	)
	require.NoError(t, err)

	note, err = s.GetNote(ctx, "pending-bear-col-test")
	require.NoError(t, err)
	require.NotNil(t, note.PendingBearTitle)
	assert.Equal(t, "Bear Title", *note.PendingBearTitle)
	require.NotNil(t, note.PendingBearBody)
	assert.Equal(t, "Bear Body", *note.PendingBearBody)
}

func TestAckApplied_ClearsPendingBearFields(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create a note with pending_bear fields populated (simulating consumer update).
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID:               "n-clear-pending",
		Title:            "Consumer Title",
		Body:             "Consumer Body",
		SyncStatus:       "pending_to_bear",
		PendingBearTitle: strPtr("Original Bear Title"),
		PendingBearBody:  strPtr("Original Bear Body"),
	}))

	// Enqueue a write and process it.
	item, err := s.EnqueueWrite(ctx, "key-clear-pending", "update", "n-clear-pending", `{"title":"Consumer Title"}`, "")
	require.NoError(t, err)

	_, err = s.LeaseQueueItems(ctx, "bridge-1", 5*time.Minute)
	require.NoError(t, err)

	// Ack as applied — no other pending items.
	err = s.AckQueueItems(ctx, []models.SyncAckItem{
		{QueueID: item.ID, IdempotencyKey: "key-clear-pending", Status: "applied"},
	})
	require.NoError(t, err)

	// Verify: sync_status is synced AND pending_bear fields are cleared.
	note, err := s.GetNote(ctx, "n-clear-pending")
	require.NoError(t, err)
	assert.Equal(t, "synced", note.SyncStatus)
	assert.Nil(t, note.PendingBearTitle, "PendingBearTitle should be NULL after ack applied")
	assert.Nil(t, note.PendingBearBody, "PendingBearBody should be NULL after ack applied")
}

func TestAckApplied_PreservesPendingBearFieldsWhenOtherPending(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create a note with pending_bear fields populated.
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID:               "n-keep-pending",
		Title:            "Consumer Title",
		Body:             "Consumer Body",
		SyncStatus:       "pending_to_bear",
		PendingBearTitle: strPtr("Bear Title"),
		PendingBearBody:  strPtr("Bear Body"),
	}))

	// Enqueue TWO writes for the same note (different actions to avoid coalescing).
	item1, err := s.EnqueueWrite(ctx, "key-keep-1", "update", "n-keep-pending", `{"title":"T1"}`, "")
	require.NoError(t, err)
	_, err = s.EnqueueWrite(ctx, "key-keep-2", "add_tag", "n-keep-pending", `{"tag":"test"}`, "")
	require.NoError(t, err)

	// Lease and ack only the first one.
	_, err = s.LeaseQueueItems(ctx, "bridge-1", 5*time.Minute)
	require.NoError(t, err)

	err = s.AckQueueItems(ctx, []models.SyncAckItem{
		{QueueID: item1.ID, IdempotencyKey: "key-keep-1", Status: "applied"},
	})
	require.NoError(t, err)

	// Verify: sync_status stays pending_to_bear AND pending_bear fields are preserved.
	note, err := s.GetNote(ctx, "n-keep-pending")
	require.NoError(t, err)
	assert.Equal(t, "pending_to_bear", note.SyncStatus)
	require.NotNil(t, note.PendingBearTitle, "PendingBearTitle should be preserved when other pending items exist")
	assert.Equal(t, "Bear Title", *note.PendingBearTitle)
	require.NotNil(t, note.PendingBearBody, "PendingBearBody should be preserved when other pending items exist")
	assert.Equal(t, "Bear Body", *note.PendingBearBody)
}

func TestAckConflictResolved_ClearsPendingBearFields(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	bearID := "bear-conflict-resolved"
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID:               "n-conflict-resolved",
		BearID:           &bearID,
		Title:            "Title",
		Body:             "Body",
		SyncStatus:       "conflict",
		PendingBearTitle: strPtr("Old Bear Title"),
		PendingBearBody:  strPtr("Old Bear Body"),
	}))

	item, err := s.EnqueueWrite(ctx, "key-conflict-resolved", "update", "n-conflict-resolved", `{"title":"T"}`, "")
	require.NoError(t, err)

	_, err = s.LeaseQueueItems(ctx, "bridge-1", 5*time.Minute)
	require.NoError(t, err)

	err = s.AckQueueItems(ctx, []models.SyncAckItem{
		{QueueID: item.ID, IdempotencyKey: "key-conflict-resolved", Status: "applied", ConflictResolved: true},
	})
	require.NoError(t, err)

	note, err := s.GetNote(ctx, "n-conflict-resolved")
	require.NoError(t, err)
	assert.Equal(t, "synced", note.SyncStatus, "conflict should be resolved to synced")
	assert.Nil(t, note.PendingBearTitle, "PendingBearTitle should be NULL after conflict resolved")
	assert.Nil(t, note.PendingBearBody, "PendingBearBody should be NULL after conflict resolved")
}

// --- ExpectedBearModifiedAt column tests ---

func TestExpectedBearModifiedAt_MigrationAddsColumn(t *testing.T) {
	// Create a DB with old schema (no expected_bear_modified_at column).
	dbPath := filepath.Join(t.TempDir(), "test-ebma-migration.db")
	ctx := context.Background()

	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)

	// Create notes table without expected_bear_modified_at.
	_, err = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS notes (
		rowid               INTEGER PRIMARY KEY AUTOINCREMENT,
		id                  TEXT NOT NULL UNIQUE,
		bear_id             TEXT UNIQUE,
		title               TEXT NOT NULL DEFAULT '',
		subtitle            TEXT DEFAULT '',
		body                TEXT NOT NULL DEFAULT '',
		archived            INTEGER DEFAULT 0,
		encrypted           INTEGER DEFAULT 0,
		has_files           INTEGER DEFAULT 0,
		has_images          INTEGER DEFAULT 0,
		has_source_code     INTEGER DEFAULT 0,
		locked              INTEGER DEFAULT 0,
		pinned              INTEGER DEFAULT 0,
		shown_in_today      INTEGER DEFAULT 0,
		trashed             INTEGER DEFAULT 0,
		permanently_deleted INTEGER DEFAULT 0,
		skip_sync           INTEGER DEFAULT 0,
		todo_completed      INTEGER DEFAULT 0,
		todo_incompleted    INTEGER DEFAULT 0,
		version             INTEGER DEFAULT 0,
		created_at          TEXT,
		modified_at         TEXT,
		archived_at         TEXT,
		encrypted_at        TEXT,
		locked_at           TEXT,
		pinned_at           TEXT,
		trashed_at          TEXT,
		order_date          TEXT,
		conflict_id_date    TEXT,
		last_editing_device TEXT,
		conflict_id         TEXT,
		encryption_id       TEXT,
		encrypted_data      BLOB,
		sync_status         TEXT DEFAULT 'synced',
		hub_modified_at     TEXT,
		bear_raw            TEXT,
		pending_bear_title  TEXT,
		pending_bear_body   TEXT
	)`)
	require.NoError(t, err)

	// Insert a row with old schema.
	_, err = db.ExecContext(ctx,
		"INSERT INTO notes (id, title, body) VALUES (?, ?, ?)",
		"n-old", "Old Note", "Old Body",
	)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	// Reopen with NewSQLiteStore — migration should add expected_bear_modified_at.
	s, err := store.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })

	// Verify column exists and old row has NULL.
	var ebma sql.NullString
	err = s.DB().QueryRowContext(ctx,
		"SELECT expected_bear_modified_at FROM notes WHERE id = ?", "n-old",
	).Scan(&ebma)
	require.NoError(t, err)
	assert.False(t, ebma.Valid, "expected_bear_modified_at should be NULL for migrated row")
}

func TestExpectedBearModifiedAt_NullByDefault(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID:    "n-ebma-null",
		Title: "Test",
		Body:  "Body",
	}))

	var ebma sql.NullString
	err := s.DB().QueryRowContext(ctx,
		"SELECT expected_bear_modified_at FROM notes WHERE id = ?", "n-ebma-null",
	).Scan(&ebma)
	require.NoError(t, err)
	assert.False(t, ebma.Valid, "expected_bear_modified_at should be NULL by default")

	note, err := s.GetNote(ctx, "n-ebma-null")
	require.NoError(t, err)
	assert.Nil(t, note.ExpectedBearModifiedAt, "ExpectedBearModifiedAt should be nil from NULL")
}

func TestExpectedBearModifiedAt_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	ebmaVal := "789654321.123456"

	// Create with ExpectedBearModifiedAt set.
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID:                     "n-ebma-rt",
		Title:                  "Test",
		Body:                   "Body",
		SyncStatus:             "pending_to_bear",
		ExpectedBearModifiedAt: &ebmaVal,
	}))

	// Verify round-trip through GetNote.
	note, err := s.GetNote(ctx, "n-ebma-rt")
	require.NoError(t, err)
	require.NotNil(t, note.ExpectedBearModifiedAt)
	assert.Equal(t, ebmaVal, *note.ExpectedBearModifiedAt)

	// Update and verify round-trip through UpdateNote.
	newVal := "789654999.654321"
	note.ExpectedBearModifiedAt = &newVal
	require.NoError(t, s.UpdateNote(ctx, note))

	note2, err := s.GetNote(ctx, "n-ebma-rt")
	require.NoError(t, err)
	require.NotNil(t, note2.ExpectedBearModifiedAt)
	assert.Equal(t, newVal, *note2.ExpectedBearModifiedAt)

	// Set to nil and verify cleared.
	note2.ExpectedBearModifiedAt = nil
	require.NoError(t, s.UpdateNote(ctx, note2))

	note3, err := s.GetNote(ctx, "n-ebma-rt")
	require.NoError(t, err)
	assert.Nil(t, note3.ExpectedBearModifiedAt, "ExpectedBearModifiedAt should be nil after clearing")
}

// --- Task 3: Hub stores expected_bear_modified_at on ack ---

func TestAckApplied_ExpectedBearModifiedAtBehavior(t *testing.T) {
	tests := []struct {
		name           string
		noteID         string
		numQueueItems  int // how many queue items to enqueue (ack first one)
		ackBearMod     string
		presetEBMA     *string // initial expected_bear_modified_at on note
		wantSyncStatus string
		wantEBMA       *string // expected expected_bear_modified_at after ack
	}{
		{
			name:           "sets expected_bear_modified_at from ack when other pending",
			noteID:         "n-ack-ebma-set",
			numQueueItems:  2,
			ackBearMod:     "726842700.5",
			wantSyncStatus: "pending_to_bear",
			wantEBMA:       strPtr("726842700.5"),
		},
		{
			name:           "clears expected_bear_modified_at when transitioning to synced",
			noteID:         "n-ack-ebma-clear",
			numQueueItems:  1,
			ackBearMod:     "726842999.0",
			presetEBMA:     strPtr("726842700.5"),
			wantSyncStatus: "synced",
			wantEBMA:       nil,
		},
		{
			name:           "keeps expected_bear_modified_at when other items pending",
			noteID:         "n-ack-ebma-keep",
			numQueueItems:  2,
			ackBearMod:     "726842700.5",
			wantSyncStatus: "pending_to_bear",
			wantEBMA:       strPtr("726842700.5"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			ctx := context.Background()

			require.NoError(t, s.CreateNote(ctx, &models.Note{
				ID:                     tt.noteID,
				Title:                  "Title",
				Body:                   "Body",
				SyncStatus:             "pending_to_bear",
				ExpectedBearModifiedAt: tt.presetEBMA,
			}))

			// Enqueue N writes for the same note (alternate actions to avoid coalescing).
			actions := []string{"update", "add_tag", "trash"}
			var firstItemID int64
			for i := range tt.numQueueItems {
				action := actions[i%len(actions)]
				item, err := s.EnqueueWrite(ctx,
					fmt.Sprintf("key-%s-%d", tt.noteID, i), action, tt.noteID,
					fmt.Sprintf(`{"title":"T%d"}`, i), "")
				require.NoError(t, err)
				if i == 0 {
					firstItemID = item.ID
				}
			}

			_, err := s.LeaseQueueItems(ctx, "bridge-1", 5*time.Minute)
			require.NoError(t, err)

			err = s.AckQueueItems(ctx, []models.SyncAckItem{
				{
					QueueID:        firstItemID,
					IdempotencyKey: fmt.Sprintf("key-%s-0", tt.noteID),
					Status:         "applied",
					BearModifiedAt: tt.ackBearMod,
				},
			})
			require.NoError(t, err)

			note, err := s.GetNote(ctx, tt.noteID)
			require.NoError(t, err)
			assert.Equal(t, tt.wantSyncStatus, note.SyncStatus)

			if tt.wantEBMA == nil {
				assert.Nil(t, note.ExpectedBearModifiedAt,
					"expected_bear_modified_at should be NULL")
			} else {
				require.NotNil(t, note.ExpectedBearModifiedAt,
					"expected_bear_modified_at should not be NULL")
				assert.Equal(t, *tt.wantEBMA, *note.ExpectedBearModifiedAt)
			}
		})
	}
}

// --- Task 4: Echo detection in updateExistingNote ---

func TestProcessSyncPush_EchoDetection(t *testing.T) {
	tests := []struct {
		name                   string
		expectedBearModifiedAt *string
		incomingModifiedAt     string
		existingModifiedAt     string
		pendingBearTitle       *string
		pendingBearBody        *string
		hubTitle               string
		hubBody                string
		bearTitle              string
		bearBody               string
		wantSyncStatus         string
		wantEBMANil            bool
	}{
		{
			name:                   "echo detected: modified_at matches expected_bear_modified_at",
			expectedBearModifiedAt: strPtr("2025-01-01T11:00:00Z"),
			incomingModifiedAt:     "2025-01-01T11:00:00Z",
			existingModifiedAt:     "2025-01-01T10:00:00Z",
			pendingBearTitle:       strPtr("Original Title"),
			pendingBearBody:        strPtr("Original Body"),
			hubTitle:               "Consumer Title",
			hubBody:                "Consumer Body",
			bearTitle:              "Consumer Title",
			bearBody:               "Consumer Body",
			wantSyncStatus:         "pending_to_bear",
			wantEBMANil:            true,
		},
		{
			name:                   "no echo: modified_at does not match expected_bear_modified_at",
			expectedBearModifiedAt: strPtr("2025-01-01T11:00:00Z"),
			incomingModifiedAt:     "2025-01-01T12:00:00Z",
			existingModifiedAt:     "2025-01-01T10:00:00Z",
			pendingBearTitle:       strPtr("Original Title"),
			pendingBearBody:        strPtr("Original Body"),
			hubTitle:               "Consumer Title",
			hubBody:                "Consumer Body",
			bearTitle:              "Original Title",
			bearBody:               "Bear Edited Body",
			wantSyncStatus:         "conflict",
			wantEBMANil:            false,
		},
		{
			name:                   "backward compat: expected_bear_modified_at NULL proceeds to conflict detection",
			expectedBearModifiedAt: nil,
			incomingModifiedAt:     "2025-01-01T11:00:00Z",
			existingModifiedAt:     "2025-01-01T10:00:00Z",
			pendingBearTitle:       strPtr("Original Title"),
			pendingBearBody:        strPtr("Original Body"),
			hubTitle:               "Consumer Title",
			hubBody:                "Consumer Body",
			bearTitle:              "Original Title",
			bearBody:               "Bear Edited Body",
			wantSyncStatus:         "conflict",
			wantEBMANil:            true,
		},
		{
			name:                   "echo clears expected_bear_modified_at",
			expectedBearModifiedAt: strPtr("2025-01-01T11:00:00Z"),
			incomingModifiedAt:     "2025-01-01T11:00:00Z",
			existingModifiedAt:     "2025-01-01T10:00:00Z",
			pendingBearTitle:       strPtr("Title"),
			pendingBearBody:        strPtr("Body"),
			hubTitle:               "Title",
			hubBody:                "Body",
			bearTitle:              "Title",
			bearBody:               "Body",
			wantSyncStatus:         "pending_to_bear",
			wantEBMANil:            true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			ctx := context.Background()
			bearID := "bear-echo-" + tt.name

			require.NoError(t, s.CreateNote(ctx, &models.Note{
				ID:                     "n-echo",
				BearID:                 &bearID,
				Title:                  tt.hubTitle,
				Body:                   tt.hubBody,
				SyncStatus:             "pending_to_bear",
				ModifiedAt:             tt.existingModifiedAt,
				PendingBearTitle:       tt.pendingBearTitle,
				PendingBearBody:        tt.pendingBearBody,
				ExpectedBearModifiedAt: tt.expectedBearModifiedAt,
			}))

			req := models.SyncPushRequest{
				Notes: []models.Note{
					{
						BearID:     &bearID,
						Title:      tt.bearTitle,
						Body:       tt.bearBody,
						ModifiedAt: tt.incomingModifiedAt,
						SyncStatus: "synced",
					},
				},
			}
			require.NoError(t, s.ProcessSyncPush(ctx, req))

			got, err := s.GetNote(ctx, "n-echo")
			require.NoError(t, err)
			assert.Equal(t, tt.wantSyncStatus, got.SyncStatus)
			if tt.wantEBMANil {
				assert.Nil(t, got.ExpectedBearModifiedAt,
					"expected_bear_modified_at should be cleared")
			} else {
				assert.NotNil(t, got.ExpectedBearModifiedAt,
					"expected_bear_modified_at should be preserved")
			}
		})
	}
}

func TestWriteQueue_CoalesceUpdatePending(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// First update: creates a new pending item.
	item1, err := s.EnqueueWrite(ctx, "key-1", "update", "n1", `{"body":"v1"}`, "consumer-a")
	require.NoError(t, err)
	assert.Equal(t, "pending", item1.Status)
	assert.Equal(t, `{"body":"v1"}`, item1.Payload)

	// Second update for same note: should coalesce into item1.
	item2, err := s.EnqueueWrite(ctx, "key-2", "update", "n1", `{"body":"v2"}`, "consumer-a")
	require.NoError(t, err)
	assert.Equal(t, item1.ID, item2.ID, "should return same queue item")
	assert.Equal(t, `{"body":"v2"}`, item2.Payload, "payload should be updated")
	assert.Equal(t, "key-1", item2.IdempotencyKey, "original idempotency key preserved")
	assert.Equal(t, "key-2", item2.SecondaryIdempotencyKey, "secondary key stored")
}

func TestWriteQueue_CoalesceUpdateProcessing(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// First update: create and lease it (processing).
	item1, err := s.EnqueueWrite(ctx, "key-1", "update", "n1", `{"body":"v1"}`, "consumer-a")
	require.NoError(t, err)
	_, err = s.LeaseQueueItems(ctx, "bridge-1", 5*time.Minute)
	require.NoError(t, err)

	// Second update for same note while first is processing: must create NEW item.
	item2, err := s.EnqueueWrite(ctx, "key-2", "update", "n1", `{"body":"v2"}`, "consumer-a")
	require.NoError(t, err)
	assert.NotEqual(t, item1.ID, item2.ID, "must create new item for in-flight")
	assert.Equal(t, `{"body":"v2"}`, item2.Payload)
	assert.Equal(t, "key-2", item2.IdempotencyKey)
}

func TestWriteQueue_CoalesceUpdateAppliedFailed(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create an update item and set its status to applied directly.
	item1, err := s.EnqueueWrite(ctx, "key-1", "update", "n1", `{"body":"v1"}`, "consumer-a")
	require.NoError(t, err)
	_, err = s.DB().ExecContext(ctx, "UPDATE write_queue SET status = 'applied' WHERE id = ?", item1.ID)
	require.NoError(t, err)

	// New update for same note: must create NEW item (applied items are not coalesced).
	item2, err := s.EnqueueWrite(ctx, "key-2", "update", "n1", `{"body":"v2"}`, "consumer-a")
	require.NoError(t, err)
	assert.NotEqual(t, item1.ID, item2.ID, "must create new item when existing is applied")
}

func TestWriteQueue_CoalescePreservesOriginalKey(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// First update.
	item1, err := s.EnqueueWrite(ctx, "key-orig", "update", "n1", `{"body":"v1"}`, "consumer-a")
	require.NoError(t, err)

	// Second update (coalesces).
	_, err = s.EnqueueWrite(ctx, "key-new", "update", "n1", `{"body":"v2"}`, "consumer-a")
	require.NoError(t, err)

	// Original key still resolves to the item.
	got1, err := s.GetQueueItemByIdempotencyKey(ctx, "key-orig", "consumer-a")
	require.NoError(t, err)
	require.NotNil(t, got1)
	assert.Equal(t, item1.ID, got1.ID)
	assert.Equal(t, `{"body":"v2"}`, got1.Payload)

	// New (secondary) key also resolves to the same item.
	got2, err := s.GetQueueItemByIdempotencyKey(ctx, "key-new", "consumer-a")
	require.NoError(t, err)
	require.NotNil(t, got2)
	assert.Equal(t, item1.ID, got2.ID)
}

func TestWriteQueue_CoalesceNoPendingItem(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// No existing pending item: creates new item (unchanged behavior).
	item, err := s.EnqueueWrite(ctx, "key-1", "update", "n1", `{"body":"v1"}`, "consumer-a")
	require.NoError(t, err)
	assert.Equal(t, "pending", item.Status)
	assert.Equal(t, "key-1", item.IdempotencyKey)
	assert.Equal(t, "", item.SecondaryIdempotencyKey)
}

func TestCoalesceCreateUpdate_ProcessingNotCoalesced(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create a note and enqueue a create action.
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID:         "n-proc",
		Title:      "Title",
		Body:       "Body",
		SyncStatus: "pending_to_bear",
	}))
	_, err := s.EnqueueWrite(ctx, "key-create", "create", "n-proc",
		`{"title":"Title","body":"Body"}`, "consumer-a")
	require.NoError(t, err)

	// Lease the item (transitions to processing).
	_, err = s.LeaseQueueItems(ctx, "bridge-1", 5*time.Minute)
	require.NoError(t, err)

	// Attempt create→update coalesce while item is processing: must return nil.
	coalesced, err := s.CoalesceCreateUpdate(ctx, "key-update", "n-proc",
		`{"title":"New Title"}`, "consumer-a")
	require.NoError(t, err)
	assert.Nil(t, coalesced, "must not coalesce into an in-flight (processing) create item")
}

func TestCoalesceCreateUpdate_CrossConsumerIsolation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Consumer A has a pending create.
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID:         "n-cross",
		Title:      "Title",
		Body:       "Body",
		SyncStatus: "pending_to_bear",
	}))
	_, err := s.EnqueueWrite(ctx, "key-create-a", "create", "n-cross",
		`{"title":"Title","body":"Body"}`, "consumer-a")
	require.NoError(t, err)

	// Consumer B attempts create→update coalesce: must return nil (not their item).
	coalesced, err := s.CoalesceCreateUpdate(ctx, "key-update-b", "n-cross",
		`{"title":"B Title"}`, "consumer-b")
	require.NoError(t, err)
	assert.Nil(t, coalesced, "consumer-b must not coalesce into consumer-a's create item")
}

// --- Acceptance Tests ---

// TestAcceptance_RapidUpdateCoalescing verifies Problem 1: rapid sequential consumer
// updates for the same note produce a single coalesced queue item and no false conflict
// when the Bear delta push arrives after the bridge applies the coalesced item.
func TestAcceptance_RapidUpdateCoalescing(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Setup: a synced note exists in the hub.
	bearID := "bear-rapid-1"
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID:         "n-rapid",
		BearID:     &bearID,
		Title:      "Original Title",
		Body:       "Original Body",
		SyncStatus: "synced",
		ModifiedAt: "2025-01-01T10:00:00Z",
	}))

	// Consumer sends two rapid updates (title='B' then title='C').
	item1, err := s.EnqueueWrite(ctx, "key-rapid-1", "update", "n-rapid",
		`{"title":"B","bear_id":"bear-rapid-1"}`, "consumer-a")
	require.NoError(t, err)

	item2, err := s.EnqueueWrite(ctx, "key-rapid-2", "update", "n-rapid",
		`{"title":"C","bear_id":"bear-rapid-1"}`, "consumer-a")
	require.NoError(t, err)

	// Coalescing: only one queue item should exist with the final payload.
	assert.Equal(t, item1.ID, item2.ID, "rapid updates should coalesce into one item")
	assert.Equal(t, `{"title":"C","bear_id":"bear-rapid-1"}`, item2.Payload)

	// Bridge leases and applies the single item.
	items, err := s.LeaseQueueItems(ctx, "bridge-1", 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, items, 1, "only one coalesced item should be leased")

	// Bridge ack with BearModifiedAt from Bear after apply.
	require.NoError(t, s.AckQueueItems(ctx, []models.SyncAckItem{
		{QueueID: items[0].ID, Status: "applied", BearModifiedAt: "2025-01-01T12:00:00Z"},
	}))

	// Note should transition to synced (no other pending items).
	got, err := s.GetNote(ctx, "n-rapid")
	require.NoError(t, err)
	assert.Equal(t, "synced", got.SyncStatus, "note should be synced after single ack")

	// Bear delta push arrives — should NOT trigger conflict.
	req := models.SyncPushRequest{
		Notes: []models.Note{
			{
				BearID:     &bearID,
				Title:      "C",
				Body:       "Original Body",
				ModifiedAt: "2025-01-01T12:00:00Z",
				SyncStatus: "synced",
			},
		},
	}
	require.NoError(t, s.ProcessSyncPush(ctx, req))

	got, err = s.GetNote(ctx, "n-rapid")
	require.NoError(t, err)
	assert.Equal(t, "synced", got.SyncStatus, "no false conflict from rapid updates")
}

// TestAcceptance_EchoDetection verifies Problem 2: Bear's modified_at change caused
// by our own x-callback-url write is recognized as an echo and does not trigger conflict.
// This is a store-level test, so sync_status transitions are set up directly (the API handler
// is what normally transitions notes to pending_to_bear via snapshotPendingBear).
func TestAcceptance_EchoDetection(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Setup: note is pending_to_bear with two queue items (update + add_tag).
	// This simulates a consumer that enqueued writes, with the API handler having
	// already transitioned the note to pending_to_bear.
	bearID := "bear-echo-true"
	origTitle := "Title"
	origBody := "Body"
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID:               "n-echo-true",
		BearID:           &bearID,
		Title:            "Title",
		Body:             "Body",
		SyncStatus:       "pending_to_bear",
		ModifiedAt:       "2025-01-01T10:00:00Z",
		PendingBearTitle: &origTitle,
		PendingBearBody:  &origBody,
	}))

	// Enqueue two items (different actions to avoid coalescing).
	upd, err := s.EnqueueWrite(ctx, "key-true-1", "update", "n-echo-true",
		`{"body":"New Body"}`, "consumer-a")
	require.NoError(t, err)
	addTag, err := s.EnqueueWrite(ctx, "key-true-2", "add_tag", "n-echo-true",
		`{"tag":"#test"}`, "consumer-a")
	require.NoError(t, err)

	// Bridge leases both items.
	leased, err := s.LeaseQueueItems(ctx, "bridge-1", 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, leased, 2)

	// Ack only the update item with BearModifiedAt=T2.
	// The add_tag item remains processing, so note stays pending_to_bear.
	require.NoError(t, s.AckQueueItems(ctx, []models.SyncAckItem{
		{QueueID: upd.ID, Status: "applied", BearModifiedAt: "2025-01-01T11:30:00Z"},
	}))

	// Note should still be pending_to_bear (add_tag is still processing).
	got, err := s.GetNote(ctx, "n-echo-true")
	require.NoError(t, err)
	assert.Equal(t, "pending_to_bear", got.SyncStatus)
	require.NotNil(t, got.ExpectedBearModifiedAt)
	assert.Equal(t, "2025-01-01T11:30:00Z", *got.ExpectedBearModifiedAt)

	// Bear delta push arrives with modified_at=T2 (echo of the update apply).
	// This should be recognized as an echo and NOT trigger conflict.
	req := models.SyncPushRequest{
		Notes: []models.Note{
			{
				BearID:     &bearID,
				Title:      "Title",
				Body:       "New Body",
				ModifiedAt: "2025-01-01T11:30:00Z",
				SyncStatus: "synced",
			},
		},
	}
	require.NoError(t, s.ProcessSyncPush(ctx, req))

	got, err = s.GetNote(ctx, "n-echo-true")
	require.NoError(t, err)
	assert.Equal(t, "pending_to_bear", got.SyncStatus, "echo detected, stays pending_to_bear")
	assert.Nil(t, got.ExpectedBearModifiedAt, "expected_bear_modified_at consumed after echo")

	// Ack the remaining add_tag item to clean up.
	require.NoError(t, s.AckQueueItems(ctx, []models.SyncAckItem{
		{QueueID: addTag.ID, Status: "applied"},
	}))

	got, err = s.GetNote(ctx, "n-echo-true")
	require.NoError(t, err)
	assert.Equal(t, "synced", got.SyncStatus, "all items acked, note becomes synced")
}

// TestAcceptance_CreateUpdateCoalescing verifies create->update coalescing:
// a single create item with the final content reaches Bear.
func TestAcceptance_CreateUpdateCoalescing(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create a note without a BearID (hub-only, pending create).
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID:         "n-create",
		Title:      "Initial Title",
		Body:       "Initial Body",
		SyncStatus: "pending_to_bear",
	}))

	// Enqueue the create action.
	createItem, err := s.EnqueueWrite(ctx, "key-create-1", "create", "n-create",
		`{"title":"Initial Title","body":"Initial Body","tags":["#tag1"]}`, "consumer-a")
	require.NoError(t, err)

	// Consumer updates the note before bridge processes it.
	coalesced, err := s.CoalesceCreateUpdate(ctx, "key-update-1", "n-create",
		`{"title":"Updated Title","body":"Updated Body"}`, "consumer-a")
	require.NoError(t, err)
	require.NotNil(t, coalesced, "should find and coalesce with pending create")
	assert.Equal(t, createItem.ID, coalesced.ID, "same queue item")

	// Verify the payload has the updated title/body but preserves tags.
	assert.Contains(t, coalesced.Payload, `"title":"Updated Title"`)
	assert.Contains(t, coalesced.Payload, `"body":"Updated Body"`)
	assert.Contains(t, coalesced.Payload, `"tags"`)
	assert.Contains(t, coalesced.Payload, `#tag1`)

	// Bridge leases — only one item.
	items, err := s.LeaseQueueItems(ctx, "bridge-1", 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "create", items[0].Action)
	assert.Contains(t, items[0].Payload, `"Updated Title"`)
}

// TestAcceptance_BackwardCompat_NullExpectedBearModifiedAt verifies that notes
// without expected_bear_modified_at (NULL) behave exactly as before — conflict
// detection proceeds based on field-level comparison.
func TestAcceptance_BackwardCompat_NullExpectedBearModifiedAt(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	bearID := "bear-compat-1"
	origTitle := "Base Title"
	origBody := "Base Body"
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID:                     "n-compat",
		BearID:                 &bearID,
		Title:                  "Consumer Title",
		Body:                   "Consumer Body",
		SyncStatus:             "pending_to_bear",
		ModifiedAt:             "2025-01-01T10:00:00Z",
		PendingBearTitle:       &origTitle,
		PendingBearBody:        &origBody,
		ExpectedBearModifiedAt: nil, // NULL — backward compat scenario.
	}))

	// Bear delta push with changed modified_at and Bear changed body (same field consumer changed).
	// With NULL expected_bear_modified_at, should fall through to field-level conflict detection.
	req := models.SyncPushRequest{
		Notes: []models.Note{
			{
				BearID:     &bearID,
				Title:      "Base Title",
				Body:       "Bear Changed Body",
				ModifiedAt: "2025-01-01T11:00:00Z",
				SyncStatus: "synced",
			},
		},
	}
	require.NoError(t, s.ProcessSyncPush(ctx, req))

	got, err := s.GetNote(ctx, "n-compat")
	require.NoError(t, err)
	assert.Equal(t, "conflict", got.SyncStatus,
		"with NULL expected_bear_modified_at, field-level conflict should fire as before")
}

// TestAcceptance_RealConflictStillDetected verifies that genuine conflicts
// (Bear user edits body + consumer edits body) are still correctly detected
// even with the new echo detection and coalescing in place.
func TestAcceptance_RealConflictStillDetected(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	bearID := "bear-real-conflict"
	origTitle := "Same Title"
	origBody := "Base Body Content"
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID:                     "n-conflict",
		BearID:                 &bearID,
		Title:                  "Same Title",
		Body:                   "Consumer Changed Body",
		SyncStatus:             "pending_to_bear",
		ModifiedAt:             "2025-01-01T10:00:00Z",
		PendingBearTitle:       &origTitle,
		PendingBearBody:        &origBody,
		ExpectedBearModifiedAt: strPtr("2025-01-01T09:00:00Z"), // Set but won't match incoming.
	}))

	// Bear user edits the body independently (modified_at differs from expected).
	req := models.SyncPushRequest{
		Notes: []models.Note{
			{
				BearID:     &bearID,
				Title:      "Same Title",
				Body:       "Bear User Changed Body",
				ModifiedAt: "2025-01-01T11:00:00Z", // Different from expected (09:00).
				SyncStatus: "synced",
			},
		},
	}
	require.NoError(t, s.ProcessSyncPush(ctx, req))

	got, err := s.GetNote(ctx, "n-conflict")
	require.NoError(t, err)
	assert.Equal(t, "conflict", got.SyncStatus,
		"real conflict: both Bear user and consumer changed body → conflict must fire")
	assert.Equal(t, "Same Title", got.Title, "hub title preserved")
	assert.Equal(t, "Consumer Changed Body", got.Body, "consumer body preserved")
}

// TestAcceptance_CreateThenUpdate_NoFalseConflict verifies the full create→ack→update→Bear delta
// push flow does NOT produce a false conflict. The key scenario: consumer creates a note via the API,
// bridge applies the create and acks with BearModifiedAt, consumer then updates the note, and Bear's
// first delta push arrives. Because expected_bear_modified_at is preserved on the create ack, echo
// detection fires and the delta push is treated as an echo of the create — no conflict.
func TestAcceptance_CreateThenUpdate_NoFalseConflict(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Step 1: Consumer creates a note via the API.
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID:         "n-create-update",
		Title:      "My Health",
		Body:       "Initial body content",
		SyncStatus: "pending_to_bear",
		ModifiedAt: "2025-06-15T10:00:00Z",
	}))

	createItem, err := s.EnqueueWrite(ctx, "key-create", "create", "n-create-update",
		`{"title":"My Health","body":"Initial body content","tags":["#health"]}`, "consumer-a")
	require.NoError(t, err)

	// Step 2: Bridge leases and processes the create.
	_, err = s.LeaseQueueItems(ctx, "bridge-1", 5*time.Minute)
	require.NoError(t, err)

	// Step 3: Bridge acks with BearID and BearModifiedAt (Bear created the note at this timestamp).
	bearID := "bear-created-uuid"
	bearModifiedAt := "2025-06-15T10:00:05Z"
	require.NoError(t, s.AckQueueItems(ctx, []models.SyncAckItem{
		{
			QueueID: createItem.ID, IdempotencyKey: "key-create", Status: "applied",
			BearID: bearID, BearModifiedAt: bearModifiedAt,
		},
	}))

	// Verify: note is synced with bear_id and expected_bear_modified_at preserved.
	got, err := s.GetNote(ctx, "n-create-update")
	require.NoError(t, err)
	assert.Equal(t, "synced", got.SyncStatus)
	require.NotNil(t, got.BearID)
	assert.Equal(t, bearID, *got.BearID)
	require.NotNil(t, got.ExpectedBearModifiedAt, "expected_bear_modified_at should be preserved on create ack")
	assert.Equal(t, bearModifiedAt, *got.ExpectedBearModifiedAt)

	// Step 4: Consumer updates the note (BearID is now set, takes normal update path).
	updatedBody := "Initial body content\n\n## TODO\n- Revaccination in 2026"
	pendingBearTitle := "My Health"
	pendingBearBody := "Initial body content"
	got.Title = "My Health"
	got.Body = updatedBody
	got.SyncStatus = "pending_to_bear"
	got.PendingBearTitle = &pendingBearTitle
	got.PendingBearBody = &pendingBearBody
	require.NoError(t, s.UpdateNote(ctx, got))

	_, err = s.EnqueueWrite(ctx, "key-update", "update", "n-create-update",
		`{"body":"`+updatedBody+`","bear_id":"`+bearID+`"}`, "consumer-a")
	require.NoError(t, err)

	// Step 5: Bear delta push arrives with the note from the create. Bear may have slightly
	// reformatted the body (e.g., added trailing newline). The key is that modified_at matches
	// expected_bear_modified_at, so echo detection fires.
	bearBody := "Initial body content\n" // Bear added a trailing newline.
	req := models.SyncPushRequest{
		Notes: []models.Note{
			{
				BearID:     &bearID,
				Title:      "My Health",
				Body:       bearBody,
				ModifiedAt: bearModifiedAt, // Matches expected_bear_modified_at.
				SyncStatus: "synced",
			},
		},
	}
	require.NoError(t, s.ProcessSyncPush(ctx, req))

	// Verify: NO conflict — echo detection recognized this as our own write.
	got, err = s.GetNote(ctx, "n-create-update")
	require.NoError(t, err)
	assert.Equal(t, "pending_to_bear", got.SyncStatus,
		"should stay pending_to_bear (echo of create, update still pending), NOT conflict")
	assert.Equal(t, "My Health", got.Title, "hub title preserved")
	assert.Equal(t, updatedBody, got.Body, "consumer's updated body preserved")
	assert.Nil(t, got.ExpectedBearModifiedAt, "expected_bear_modified_at consumed by echo detection")
}

// TestAcceptance_CreateAck_SetsSnapshotWhenOtherPending verifies that when a create is acked
// with other pending queue items, pending_bear_title/body are set from the current hub values.
func TestAcceptance_CreateAck_SetsSnapshotWhenOtherPending(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create a note via API (no BearID, pending_bear_* are NULL).
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID:         "n-create-addtag",
		Title:      "Tagged Note",
		Body:       "Some body",
		SyncStatus: "pending_to_bear",
	}))

	// Enqueue create + add_tag (separate items, not coalesced).
	createItem, err := s.EnqueueWrite(ctx, "key-c", "create", "n-create-addtag",
		`{"title":"Tagged Note","body":"Some body"}`, "consumer-a")
	require.NoError(t, err)
	_, err = s.EnqueueWrite(ctx, "key-t", "add_tag", "n-create-addtag",
		`{"tag":"#test"}`, "consumer-a")
	require.NoError(t, err)

	// Bridge leases both, processes create first.
	_, err = s.LeaseQueueItems(ctx, "bridge-1", 5*time.Minute)
	require.NoError(t, err)

	// Ack create with BearID. add_tag is still processing → otherPending > 0.
	bearID := "bear-tagged-note"
	require.NoError(t, s.AckQueueItems(ctx, []models.SyncAckItem{
		{
			QueueID: createItem.ID, IdempotencyKey: "key-c", Status: "applied",
			BearID: bearID, BearModifiedAt: "2025-06-15T10:00:05Z",
		},
	}))

	// Verify: pending_bear_* are set (not NULL) and reflect the hub's current title/body.
	got, err := s.GetNote(ctx, "n-create-addtag")
	require.NoError(t, err)
	assert.Equal(t, "pending_to_bear", got.SyncStatus)
	require.NotNil(t, got.PendingBearTitle, "pending_bear_title should be set when otherPending > 0")
	assert.Equal(t, "Tagged Note", *got.PendingBearTitle)
	require.NotNil(t, got.PendingBearBody, "pending_bear_body should be set when otherPending > 0")
	assert.Equal(t, "Some body", *got.PendingBearBody)
}
