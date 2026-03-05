package beardb

import (
	"context"

	"github.com/romancha/salmon/internal/mapper"
)

// NoteTagPair represents a resolved note-tag junction table entry with Bear UUIDs.
type NoteTagPair struct {
	NoteUUID string
	TagUUID  string
}

// NoteBasicInfo contains minimal note data for write queue verification.
type NoteBasicInfo struct {
	UUID     string
	Title    string
	Body     string
	Trashed  int64
	Archived int64
}

// BearDB defines the read-only interface for accessing Bear's SQLite database.
type BearDB interface {
	// Notes returns notes modified since lastSyncAt (Core Data epoch).
	// If lastSyncAt is 0, returns all notes.
	Notes(ctx context.Context, lastSyncAt float64) ([]mapper.BearNoteRow, error)

	// Tags returns tags modified since lastSyncAt (Core Data epoch).
	// If lastSyncAt is 0, returns all tags.
	Tags(ctx context.Context, lastSyncAt float64) ([]mapper.BearTagRow, error)

	// Attachments returns attachments modified since lastSyncAt (Core Data epoch).
	// If lastSyncAt is 0, returns all attachments.
	Attachments(ctx context.Context, lastSyncAt float64) ([]mapper.BearAttachmentRow, error)

	// Backlinks returns backlinks modified since lastSyncAt (Core Data epoch).
	// If lastSyncAt is 0, returns all backlinks.
	Backlinks(ctx context.Context, lastSyncAt float64) ([]mapper.BearBacklinkRow, error)

	// NoteTags returns all note-tag associations with resolved UUIDs.
	NoteTags(ctx context.Context) ([]NoteTagPair, error)

	// PinnedNoteTags returns all pinned note-tag associations with resolved UUIDs.
	PinnedNoteTags(ctx context.Context) ([]NoteTagPair, error)

	// NoteTagsForNotes returns note-tag associations for specific note UUIDs.
	NoteTagsForNotes(ctx context.Context, noteUUIDs []string) ([]NoteTagPair, error)

	// PinnedNoteTagsForNotes returns pinned note-tag associations for specific note UUIDs.
	PinnedNoteTagsForNotes(ctx context.Context, noteUUIDs []string) ([]NoteTagPair, error)

	// AllNoteUUIDs returns UUIDs of all notes (for deletion detection).
	AllNoteUUIDs(ctx context.Context) ([]string, error)

	// AllTagUUIDs returns UUIDs of all tags (for deletion detection).
	AllTagUUIDs(ctx context.Context) ([]string, error)

	// AllAttachmentUUIDs returns UUIDs of all attachments (for deletion detection).
	AllAttachmentUUIDs(ctx context.Context) ([]string, error)

	// AllBacklinkUUIDs returns UUIDs of all backlinks (for deletion detection).
	AllBacklinkUUIDs(ctx context.Context) ([]string, error)

	// NoteByUUID returns basic note info by Bear UUID for write queue verification.
	// Returns nil if note not found.
	NoteByUUID(ctx context.Context, bearUUID string) (*NoteBasicInfo, error)

	// NoteTagTitles returns tag titles for a note identified by Bear UUID.
	NoteTagTitles(ctx context.Context, bearUUID string) ([]string, error)

	// NoteAttachmentFilenames returns filenames of attachments for a note identified by Bear UUID.
	NoteAttachmentFilenames(ctx context.Context, bearUUID string) ([]string, error)

	// FindRecentNotesByTitle finds notes by title created after the given Core Data epoch timestamp.
	FindRecentNotesByTitle(ctx context.Context, title string, createdAfter float64) ([]NoteBasicInfo, error)

	// Close closes the database connection.
	Close() error
}
