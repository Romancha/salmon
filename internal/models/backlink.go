package models

// Backlink represents a link between two Bear notes synced to the hub.
// Dual-id: ID is the hub UUID, BearID is Bear's ZUNIQUEIDENTIFIER.
type Backlink struct {
	ID     string  `json:"id"`
	BearID *string `json:"bear_id,omitempty"`

	LinkedByID  string `json:"linked_by_id"`  // note that contains the link
	LinkingToID string `json:"linking_to_id"` // note being linked to

	Title    string `json:"title,omitempty"`
	Location int    `json:"location,omitempty"`
	Version  int    `json:"version"`

	ModifiedAt string `json:"modified_at,omitempty"`

	BearRaw string `json:"bear_raw,omitempty"`
}

// StripInternal zeroes out fields that should not appear in consumer API responses.
func (b *Backlink) StripInternal() {
	b.BearRaw = ""
}
