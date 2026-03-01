package models

// SyncPushRequest is sent by the bridge to push Bear data to the hub.
type SyncPushRequest struct {
	Notes          []Note       `json:"notes,omitempty"`
	Tags           []Tag        `json:"tags,omitempty"`
	NoteTags       []NoteTagPair `json:"note_tags,omitempty"`
	PinnedNoteTags []NoteTagPair `json:"pinned_note_tags,omitempty"`
	Attachments    []Attachment  `json:"attachments,omitempty"`
	Backlinks      []Backlink   `json:"backlinks,omitempty"`

	DeletedNoteIDs       []string `json:"deleted_note_ids,omitempty"`
	DeletedTagIDs        []string `json:"deleted_tag_ids,omitempty"`
	DeletedAttachmentIDs []string `json:"deleted_attachment_ids,omitempty"`
	DeletedBacklinkIDs   []string `json:"deleted_backlink_ids,omitempty"`

	Meta map[string]string `json:"meta,omitempty"` // key-value pairs to store in sync_meta
}

// NoteTagPair represents a note-tag association (used in junction tables).
type NoteTagPair struct {
	NoteID string `json:"note_id"`
	TagID  string `json:"tag_id"`
}

// SyncAckRequest is sent by the bridge to acknowledge processed write queue items.
type SyncAckRequest struct {
	Items []SyncAckItem `json:"items"`
}

// SyncAckItem represents the result of applying a single write queue item in Bear.
type SyncAckItem struct {
	QueueID        int64  `json:"queue_id"`
	IdempotencyKey string `json:"idempotency_key"`
	Status         string `json:"status"` // applied | failed
	BearID         string `json:"bear_id,omitempty"`
	Error          string `json:"error,omitempty"`
}
