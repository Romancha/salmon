package store

import (
	"context"
	"time"

	"github.com/romancha/bear-sync/internal/models"
)

// NoteFilter defines filtering options for listing notes.
type NoteFilter struct {
	Tag       string
	Trashed   *bool
	Encrypted *bool
	Limit     int
	Offset    int
	Sort      string // column name: "modified_at", "created_at", "title"
	Order     string // "asc" or "desc"
}

// Store defines the persistence interface for the hub.
type Store interface {
	// Notes
	ListNotes(ctx context.Context, filter NoteFilter) ([]models.Note, error)
	GetNote(ctx context.Context, id string) (*models.Note, error)
	CreateNote(ctx context.Context, note *models.Note) error
	UpdateNote(ctx context.Context, note *models.Note) error
	DeleteNote(ctx context.Context, id string) error

	// FTS5 search
	SearchNotes(ctx context.Context, query string, tag string, limit int) ([]models.Note, error)

	// Tags
	ListTags(ctx context.Context) ([]models.Tag, error)
	GetTag(ctx context.Context, id string) (*models.Tag, error)
	CreateTag(ctx context.Context, tag *models.Tag) error

	// Attachments
	GetAttachment(ctx context.Context, id string) (*models.Attachment, error)
	GetAttachmentByBearID(ctx context.Context, bearID string) (*models.Attachment, error)
	ListAttachmentsByNote(ctx context.Context, noteID string) ([]models.Attachment, error)
	UpdateAttachment(ctx context.Context, attachment *models.Attachment) error

	// Backlinks
	ListBacklinksByNote(ctx context.Context, noteID string) ([]models.Backlink, error)

	// Sync Push
	ProcessSyncPush(ctx context.Context, req models.SyncPushRequest) error

	// Write Queue
	GetQueueItemByIdempotencyKey(ctx context.Context, key, consumerID string) (*models.WriteQueueItem, error)
	EnqueueWrite(ctx context.Context, idempotencyKey, action, noteID, payload, consumerID string) (*models.WriteQueueItem, error)
	LeaseQueueItems(ctx context.Context, processingBy string, leaseDuration time.Duration) ([]models.WriteQueueItem, error)
	AckQueueItems(ctx context.Context, items []models.SyncAckItem) error
	PendingQueueCount(ctx context.Context) (int, error)

	// Conflicts
	CountConflicts(ctx context.Context) (int, error)
	ListConflictNoteIDs(ctx context.Context) ([]string, error)

	// Sync Meta
	GetSyncMeta(ctx context.Context, key string) (string, error)
	SetSyncMeta(ctx context.Context, key, value string) error

	// Close closes the database connection.
	Close() error
}
