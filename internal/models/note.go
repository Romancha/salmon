package models

// Note represents a Bear note synced to the hub.
// Dual-id: ID is the hub-generated UUID (stable PK), BearID is Bear's ZUNIQUEIDENTIFIER (nullable, filled after sync).
type Note struct {
	RowID  int64   `json:"-"`
	ID     string  `json:"id"`
	BearID *string `json:"bear_id,omitempty"`

	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
	Body     string `json:"body,omitempty"`

	// Flags
	Archived           int `json:"archived"`
	Encrypted          int `json:"encrypted"`
	HasFiles           int `json:"has_files"`
	HasImages          int `json:"has_images"`
	HasSourceCode      int `json:"has_source_code"`
	Locked             int `json:"locked"`
	Pinned             int `json:"pinned"`
	ShownInToday       int `json:"shown_in_today"`
	Trashed            int `json:"trashed"`
	PermanentlyDeleted int `json:"permanently_deleted"`
	SkipSync           int `json:"skip_sync"`
	TodoCompleted      int `json:"todo_completed"`
	TodoIncompleted    int `json:"todo_incompleted"`
	Version            int `json:"version"`

	// Dates (ISO 8601)
	CreatedAt      string `json:"created_at,omitempty"`
	ModifiedAt     string `json:"modified_at,omitempty"`
	ArchivedAt     string `json:"archived_at,omitempty"`
	EncryptedAt    string `json:"encrypted_at,omitempty"`
	LockedAt       string `json:"locked_at,omitempty"`
	PinnedAt       string `json:"pinned_at,omitempty"`
	TrashedAt      string `json:"trashed_at,omitempty"`
	OrderDate      string `json:"order_date,omitempty"`
	ConflictIDDate string `json:"conflict_id_date,omitempty"`

	// Metadata
	LastEditingDevice string `json:"last_editing_device,omitempty"`
	ConflictID        string `json:"conflict_id,omitempty"`
	EncryptionID      string `json:"encryption_id,omitempty"`
	EncryptedData     []byte `json:"encrypted_data,omitempty"`

	// Hub-only fields
	SyncStatus    string `json:"sync_status"`
	HubModifiedAt string `json:"hub_modified_at,omitempty"`
	BearRaw       string `json:"bear_raw,omitempty"`

	// Field-level conflict detection: Bear's title/body at the moment sync_status transitioned to pending_to_bear.
	PendingBearTitle *string `json:"-"`
	PendingBearBody  *string `json:"-"`

	// Echo detection: Bear's modified_at timestamp after applying a queue item via x-callback-url.
	// Used to recognize "echo" delta pushes that reflect our own writes, not genuine user edits.
	ExpectedBearModifiedAt *string `json:"-"`

	// Joined data (populated by queries, not stored directly)
	Tags        []Tag            `json:"tags,omitempty"`
	Attachments []AttachmentInfo `json:"attachments,omitempty"`
	Backlinks   []Backlink       `json:"backlinks,omitempty"`
}

// StripInternal zeroes out fields that should not appear in consumer API responses.
// Because these fields carry omitempty, zeroing makes them vanish from JSON output.
func (n *Note) StripInternal() {
	n.BearRaw = ""
	n.EncryptedData = nil
	n.HubModifiedAt = ""
	for i := range n.Tags {
		n.Tags[i].StripInternal()
	}
	for i := range n.Backlinks {
		n.Backlinks[i].StripInternal()
	}
}
