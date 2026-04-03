package mcp

import "github.com/romancha/salmon/internal/models"

// --- Search Notes ---

// SearchNotesInput is the input for the search_notes tool.
type SearchNotesInput struct {
	Query string `json:"query" jsonschema:"Full-text search query across note titles and bodies"`
	Limit int    `json:"limit,omitempty" jsonschema:"Max results (default 20, max 200)"`
	Tag   string `json:"tag,omitempty" jsonschema:"Filter by tag name"`
}

// SearchNotesOutput is the output for the search_notes tool.
type SearchNotesOutput struct {
	Notes []models.Note `json:"notes"`
}

// --- Get Note ---

// GetNoteInput is the input for the get_note tool.
type GetNoteInput struct {
	ID string `json:"id" jsonschema:"Note ID (hub UUID)"`
}

// GetNoteOutput is the output for the get_note tool.
type GetNoteOutput struct {
	models.Note
}

// --- List Notes ---

// ListNotesInput is the input for the list_notes tool.
type ListNotesInput struct {
	Tag     string `json:"tag,omitempty" jsonschema:"Filter by tag name"`
	Sort    string `json:"sort,omitempty" jsonschema:"Sort by: modified_at, created_at, or title"`
	Order   string `json:"order,omitempty" jsonschema:"Sort order: asc or desc"`
	Limit   int    `json:"limit,omitempty" jsonschema:"Max results (max 200)"`
	Trashed string `json:"trashed,omitempty" jsonschema:"Filter trashed notes: true or false"`
}

// ListNotesOutput is the output for the list_notes tool.
type ListNotesOutput struct {
	Notes []models.Note `json:"notes"`
}

// --- List Tags ---

// ListTagsOutput is the output for the list_tags tool.
type ListTagsOutput struct {
	Tags []models.Tag `json:"tags"`
}

// --- Get Attachment ---

// GetAttachmentInput is the input for the get_attachment tool.
type GetAttachmentInput struct {
	ID        string `json:"id" jsonschema:"Attachment ID (hub UUID, from note attachments array)"`
	Mode      string `json:"mode,omitempty" jsonschema:"Download mode: file (default) saves to disk; base64 returns encoded content"`
	OutputDir string `json:"output_dir,omitempty" jsonschema:"Directory to save file (default: system temp dir). Only used in file mode."`
}

// GetAttachmentOutput is the output for the get_attachment tool.
// In file mode: FilePath is set, Base64 is empty.
// In base64 mode: Base64 is set, FilePath is empty.
type GetAttachmentOutput struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size,omitempty"`
	FilePath    string `json:"file_path,omitempty"`
	Base64      string `json:"base64,omitempty"`
}

// --- List Attachments ---

// ListAttachmentsInput is the input for the list_attachments tool.
type ListAttachmentsInput struct {
	NoteID string `json:"note_id" jsonschema:"Note ID to list attachments for (hub UUID)"`
}

// ListAttachmentsOutput is the output for the list_attachments tool.
type ListAttachmentsOutput struct {
	Attachments []models.AttachmentMeta `json:"attachments"`
}

// --- Download Note Attachments ---

// DownloadNoteAttachmentsInput is the input for the download_note_attachments tool.
type DownloadNoteAttachmentsInput struct {
	NoteID     string   `json:"note_id" jsonschema:"Note ID (hub UUID)"`
	OutputDir  string   `json:"output_dir,omitempty" jsonschema:"Directory to save files (default: system temp dir)"`
	Types      []string `json:"types,omitempty" jsonschema:"Bear attachment types to include: image, file, video. Omit for all."`
	Extensions []string `json:"extensions,omitempty" jsonschema:"File extensions to include (e.g. pdf, png). Omit for all."`
}

// DownloadedAttachment is a single downloaded attachment result.
type DownloadedAttachment struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	FilePath    string `json:"file_path"`
}

// DownloadNoteAttachmentsOutput is the output for the download_note_attachments tool.
type DownloadNoteAttachmentsOutput struct {
	Downloaded []DownloadedAttachment `json:"downloaded"`
	Skipped    int                    `json:"skipped"`
}

// --- Sync Status ---

// SyncStatusOutput is the output for the sync_status tool.
type SyncStatusOutput struct {
	LastSyncAt          string   `json:"last_sync_at"`
	LastPushAt          string   `json:"last_push_at"`
	QueueSize           int      `json:"queue_size"`
	InitialSyncComplete string   `json:"initial_sync_complete"`
	ConflictCount       int      `json:"conflict_count"`
	ConflictNoteIDs     []string `json:"conflict_note_ids,omitempty"`
}

// --- List Backlinks ---

// ListBacklinksInput is the input for the list_backlinks tool.
type ListBacklinksInput struct {
	NoteID string `json:"note_id" jsonschema:"Note ID to get backlinks for (hub UUID)"`
}

// ListBacklinksOutput is the output for the list_backlinks tool.
type ListBacklinksOutput struct {
	Backlinks []models.Backlink `json:"backlinks"`
}

// --- Create Note ---

// CreateNoteInput is the input for the create_note tool.
type CreateNoteInput struct {
	Title string   `json:"title" jsonschema:"Note title (plain text, not Markdown)"`
	Body  string   `json:"body,omitempty" jsonschema:"Note body in Markdown format"`
	Tags  []string `json:"tags,omitempty" jsonschema:"Tags to assign (do NOT also put #tags in body)"`
}

// CreateNoteOutput is the output for the create_note tool.
type CreateNoteOutput struct {
	models.Note
}

// --- Update Note ---

// UpdateNoteInput is the input for the update_note tool.
type UpdateNoteInput struct {
	ID    string `json:"id" jsonschema:"Note ID (hub UUID)"`
	Title string `json:"title,omitempty" jsonschema:"New title (plain text)"`
	Body  string `json:"body" jsonschema:"New body in Markdown (replaces entire body)"`
}

// UpdateNoteOutput is the output for the update_note tool.
type UpdateNoteOutput struct {
	models.Note
}

// --- Trash Note ---

// TrashNoteInput is the input for the trash_note tool.
type TrashNoteInput struct {
	ID string `json:"id" jsonschema:"Note ID (hub UUID)"`
}

// TrashNoteOutput is the output for the trash_note tool.
type TrashNoteOutput struct {
	models.Note
}

// --- Archive Note ---

// ArchiveNoteInput is the input for the archive_note tool.
type ArchiveNoteInput struct {
	ID string `json:"id" jsonschema:"Note ID (hub UUID)"`
}

// ArchiveNoteOutput is the output for the archive_note tool.
type ArchiveNoteOutput struct {
	models.Note
}

// --- Add Tag ---

// AddTagInput is the input for the add_tag tool.
type AddTagInput struct {
	NoteID string `json:"note_id" jsonschema:"Note ID to add the tag to (hub UUID)"`
	Tag    string `json:"tag" jsonschema:"Tag name to add (e.g. work/projects)"`
}

// AddTagOutput is the output for the add_tag tool.
type AddTagOutput struct {
	models.WriteQueueItem
}

// --- Rename Tag ---

// RenameTagInput is the input for the rename_tag tool.
type RenameTagInput struct {
	ID      string `json:"id" jsonschema:"Tag ID (hub UUID, from list_tags)"`
	NewName string `json:"new_name" jsonschema:"New tag name"`
}

// RenameTagOutput is the output for the rename_tag tool.
type RenameTagOutput struct {
	models.WriteQueueItem
}

// --- Delete Tag ---

// DeleteTagInput is the input for the delete_tag tool.
type DeleteTagInput struct {
	ID string `json:"id" jsonschema:"Tag ID (hub UUID, from list_tags)"`
}

// DeleteTagOutput is the output for the delete_tag tool.
type DeleteTagOutput struct {
	models.WriteQueueItem
}
