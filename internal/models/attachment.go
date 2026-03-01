package models

// Attachment represents a Bear note attachment (file, image, or video) synced to the hub.
// Dual-id: ID is the hub UUID, BearID is Bear's ZUNIQUEIDENTIFIER.
type Attachment struct {
	ID     string  `json:"id"`
	BearID *string `json:"bear_id,omitempty"`
	NoteID string  `json:"note_id"`
	Type   string  `json:"type"` // file | image | video (from Z_ENT: 8=file, 9=image, 10=video)

	Filename            string `json:"filename,omitempty"`
	NormalizedExtension string `json:"normalized_extension,omitempty"`
	FileSize            int64  `json:"file_size,omitempty"`
	FileIndex           int    `json:"file_index,omitempty"`

	// Media metadata
	Width    int `json:"width,omitempty"`
	Height   int `json:"height,omitempty"`
	Animated int `json:"animated"`
	Duration int `json:"duration,omitempty"`
	Width1   int `json:"width1,omitempty"`
	Height1  int `json:"height1,omitempty"`

	// Flags
	Downloaded         int `json:"downloaded"`
	Encrypted          int `json:"encrypted"`
	PermanentlyDeleted int `json:"permanently_deleted"`
	SkipSync           int `json:"skip_sync"`
	Unused             int `json:"unused"`
	Uploaded           int `json:"uploaded"`
	Version            int `json:"version"`

	// Dates (ISO 8601)
	CreatedAt    string `json:"created_at,omitempty"`
	ModifiedAt   string `json:"modified_at,omitempty"`
	InsertedAt   string `json:"inserted_at,omitempty"`
	EncryptedAt  string `json:"encrypted_at,omitempty"`
	UnusedAt     string `json:"unused_at,omitempty"`
	UploadedAt   string `json:"uploaded_at,omitempty"`
	SearchTextAt string `json:"search_text_at,omitempty"`

	// Metadata
	LastEditingDevice string `json:"last_editing_device,omitempty"`
	EncryptionID      string `json:"encryption_id,omitempty"`
	SearchText        string `json:"search_text,omitempty"`
	EncryptedData     []byte `json:"encrypted_data,omitempty"`

	// File on disk (VPS)
	FilePath string `json:"file_path,omitempty"`

	BearRaw string `json:"bear_raw,omitempty"`
}
