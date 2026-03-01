package models

// WriteQueueItem represents a pending write operation from openclaw to Bear.
// Delivery semantics: effectively-once (openclaw->hub via Idempotency-Key),
// at-least-once (hub->bridge via lease), duplicate-safe (bridge pre-checks Bear state).
type WriteQueueItem struct {
	ID             int64  `json:"id"`
	IdempotencyKey string `json:"idempotency_key"`
	Action         string `json:"action"`  // create | update | add_tag | trash
	NoteID         string `json:"note_id,omitempty"`
	Payload        string `json:"payload"` // JSON with operation data
	CreatedAt      string `json:"created_at,omitempty"`
	Status         string `json:"status"`  // pending | processing | applied | failed
	ProcessingBy   string `json:"processing_by,omitempty"`
	LeaseUntil     string `json:"lease_until,omitempty"`
	AppliedAt      string `json:"applied_at,omitempty"`
	Error          string `json:"error,omitempty"`

	// NoteSyncStatus is populated during lease to indicate the associated note's sync_status.
	// Not stored in DB — filled by JOIN during LeaseQueueItems.
	NoteSyncStatus string `json:"note_sync_status,omitempty"`
}
