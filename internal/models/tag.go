package models

// Tag represents a Bear tag synced to the hub.
// Dual-id: ID is the hub UUID, BearID is Bear's ZUNIQUEIDENTIFIER.
type Tag struct {
	ID     string  `json:"id"`
	BearID *string `json:"bear_id,omitempty"`

	Title             string `json:"title"`
	Pinned            int    `json:"pinned"`
	IsRoot            int    `json:"is_root"`
	HideSubtagsNotes  int    `json:"hide_subtags_notes"`
	Sorting           int    `json:"sorting"`
	SortingDirection  int    `json:"sorting_direction"`
	Encrypted         int    `json:"encrypted"`
	Version           int    `json:"version"`

	// Dates (ISO 8601)
	ModifiedAt          string `json:"modified_at,omitempty"`
	PinnedAt            string `json:"pinned_at,omitempty"`
	PinnedNotesAt       string `json:"pinned_notes_at,omitempty"`
	EncryptedAt         string `json:"encrypted_at,omitempty"`
	HideSubtagsNotesAt  string `json:"hide_subtags_notes_at,omitempty"`
	SortingAt           string `json:"sorting_at,omitempty"`
	SortingDirectionAt  string `json:"sorting_direction_at,omitempty"`
	TagConDate          string `json:"tag_con_date,omitempty"`
	TagCon              string `json:"tag_con,omitempty"`

	BearRaw string `json:"bear_raw,omitempty"`
}
