package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/romancha/bear-sync/internal/models"
)

// verifyDelay is how long to wait before verifying xcall results in Bear SQLite.
const verifyDelay = 2 * time.Second

// createFallbackWindow is the time window for finding recently created notes (5 seconds).
const createFallbackWindow = 5.0 // seconds in Core Data epoch

// createPayload is the JSON structure for "create" action payloads.
type createPayload struct {
	Title string   `json:"title"`
	Body  string   `json:"body"`
	Tags  []string `json:"tags,omitempty"`
}

// updatePayload is the JSON structure for "update" action payloads.
type updatePayload struct {
	Title string `json:"title,omitempty"`
	Body  string `json:"body,omitempty"`
}

// addTagPayload is the JSON structure for "add_tag" action payloads.
type addTagPayload struct {
	Tag string `json:"tag"`
}

// processQueue leases write queue items from the hub and applies them to Bear via xcall.
func (b *Bridge) processQueue(ctx context.Context) error {
	if b.xcall == nil {
		b.logger.Debug("xcallback not configured, skipping queue processing")
		return nil
	}

	items, err := b.hub.LeaseQueue(ctx, "bridge")
	if err != nil {
		return fmt.Errorf("lease queue: %w", err)
	}

	if len(items) == 0 {
		b.logger.Debug("no queue items to process")
		return nil
	}

	b.logger.Info("processing write queue", "items", len(items))

	var ackItems []models.SyncAckItem

	for i := range items {
		ack := b.applyQueueItem(ctx, &items[i])
		ackItems = append(ackItems, ack)
	}

	if err := b.hub.AckQueue(ctx, ackItems); err != nil {
		return fmt.Errorf("ack queue: %w", err)
	}

	b.logger.Info("queue processing complete",
		"total", len(ackItems),
		"applied", countByStatus(ackItems, "applied"),
		"failed", countByStatus(ackItems, "failed"))

	return nil
}

// applyQueueItem processes a single write queue item. Failed items are logged but don't block others.
func (b *Bridge) applyQueueItem(ctx context.Context, item *models.WriteQueueItem) models.SyncAckItem {
	ack := models.SyncAckItem{
		QueueID:        item.ID,
		IdempotencyKey: item.IdempotencyKey,
	}

	// Skip items for notes with sync_status=conflict — create a conflict copy instead.
	if item.NoteSyncStatus == "conflict" {
		b.handleConflictItem(ctx, item, &ack)
		return ack
	}

	var err error

	switch item.Action {
	case "create":
		ack.BearID, err = b.applyCreate(ctx, item)
	case "update":
		err = b.applyUpdate(ctx, item)
	case "add_tag":
		err = b.applyAddTag(ctx, item)
	case "trash":
		err = b.applyTrash(ctx, item)
	default:
		err = fmt.Errorf("unknown action: %s", item.Action)
	}

	if err != nil {
		ack.Status = "failed"
		ack.Error = err.Error()
		b.logger.Warn("queue item failed",
			"queue_id", item.ID,
			"action", item.Action,
			"error", err)
	} else {
		ack.Status = "applied"
		b.logger.Info("queue item applied",
			"queue_id", item.ID,
			"action", item.Action,
			"bear_id", ack.BearID)
	}

	return ack
}

// handleConflictItem creates a conflict note in Bear with the openclaw version content.
// The original note in Bear keeps the user's version; a new note "[Conflict] Title" is created
// with the openclaw content so the user can manually reconcile.
func (b *Bridge) handleConflictItem(ctx context.Context, item *models.WriteQueueItem, ack *models.SyncAckItem) {
	b.logger.Warn("conflict detected for queue item, creating conflict note",
		"queue_id", item.ID, "action", item.Action, "note_id", item.NoteID)

	// Extract the openclaw content from the payload.
	title, body := b.extractConflictContent(item)
	if title == "" {
		title = "Untitled"
	}

	conflictTitle := "[Conflict] " + title

	// Create a conflict note in Bear via xcall.
	bearID, err := b.xcall.Create(ctx, b.bearToken, conflictTitle, body, nil)
	if err != nil {
		ack.Status = "failed"
		ack.Error = fmt.Sprintf("create conflict note: %v", err)
		b.logger.Warn("failed to create conflict note", "queue_id", item.ID, "error", err)

		return
	}

	ack.Status = "applied"
	ack.BearID = bearID
	b.logger.Info("conflict note created",
		"queue_id", item.ID, "conflict_bear_id", bearID, "conflict_title", conflictTitle)
}

// extractConflictContent extracts the title and body from a queue item's payload for conflict resolution.
func (b *Bridge) extractConflictContent(item *models.WriteQueueItem) (title, body string) {
	var payloadMap map[string]any
	if err := json.Unmarshal([]byte(item.Payload), &payloadMap); err != nil {
		return "", ""
	}

	if t, ok := payloadMap["title"].(string); ok {
		title = t
	}

	if bd, ok := payloadMap["body"].(string); ok {
		body = bd
	}

	// If no title in payload, try to get it from the original Bear note.
	if title == "" && item.NoteID != "" {
		note, err := b.db.NoteByUUID(context.Background(), item.NoteID)
		if err == nil && note != nil {
			title = note.Title
		}

		// Also try bear_id from payload.
		if title == "" {
			if bearID, ok := payloadMap["bear_id"].(string); ok && bearID != "" {
				note, err := b.db.NoteByUUID(context.Background(), bearID)
				if err == nil && note != nil {
					title = note.Title
				}
			}
		}
	}

	return title, body
}

// applyCreate creates a new note in Bear via xcall and returns the bear_id.
func (b *Bridge) applyCreate(ctx context.Context, item *models.WriteQueueItem) (string, error) {
	var payload createPayload
	if err := json.Unmarshal([]byte(item.Payload), &payload); err != nil {
		return "", fmt.Errorf("parse create payload: %w", err)
	}

	bearID, err := b.xcall.Create(ctx, b.bearToken, payload.Title, payload.Body, payload.Tags)
	if err != nil {
		return "", fmt.Errorf("xcall create: %w", err)
	}

	if bearID != "" {
		// Verify by reading back from Bear SQLite.
		b.sleepFn(verifyDelay)

		note, vErr := b.db.NoteByUUID(ctx, bearID)
		if vErr != nil {
			b.logger.Warn("create verification query failed", "bear_id", bearID, "error", vErr)
		} else if note == nil {
			b.logger.Warn("create verification: note not found in Bear SQLite yet", "bear_id", bearID)
		}

		return bearID, nil
	}

	// Fallback verification: xcall didn't return a UUID.
	b.logger.Warn("xcall create returned empty identifier, attempting fallback verification")

	b.sleepFn(verifyDelay)

	createdAfter := currentCoreDataEpoch() - createFallbackWindow
	matches, err := b.db.FindRecentNotesByTitle(ctx, payload.Title, createdAfter)
	if err != nil {
		return "", fmt.Errorf("fallback search: %w", err)
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("create fallback: note not found in Bear after creation")
	case 1:
		return matches[0].UUID, nil
	default:
		return "", fmt.Errorf("create fallback: ambiguous, found %d notes with title %q", len(matches), payload.Title)
	}
}

// applyUpdate updates a note body in Bear via xcall.
func (b *Bridge) applyUpdate(ctx context.Context, item *models.WriteQueueItem) error {
	var payload updatePayload
	if err := json.Unmarshal([]byte(item.Payload), &payload); err != nil {
		return fmt.Errorf("parse update payload: %w", err)
	}

	// Look up bear_id for this note. The item.NoteID is the hub UUID,
	// but we need the Bear UUID for xcall. Check if note already has the desired content.
	note, err := b.findBearNoteForItem(ctx, item)
	if err != nil {
		return fmt.Errorf("find bear note: %w", err)
	}

	// Duplicate-safe: check if note already has the desired body.
	if payload.Body != "" && note.Body == payload.Body {
		b.logger.Info("update already applied (body matches)", "bear_id", note.UUID)
		return nil
	}

	body := payload.Body
	if body == "" {
		return fmt.Errorf("update payload has no body")
	}

	if err := b.xcall.Update(ctx, b.bearToken, note.UUID, body); err != nil {
		return fmt.Errorf("xcall update: %w", err)
	}

	// Verify update.
	b.sleepFn(verifyDelay)

	updated, err := b.db.NoteByUUID(ctx, note.UUID)
	if err != nil {
		b.logger.Warn("update verification query failed", "bear_id", note.UUID, "error", err)
		return nil // xcall succeeded, verification is best-effort
	}

	if updated != nil && updated.Body != body {
		b.logger.Warn("update verification: body mismatch after update", "bear_id", note.UUID)
	}

	return nil
}

// applyAddTag adds a tag to a note in Bear via xcall.
func (b *Bridge) applyAddTag(ctx context.Context, item *models.WriteQueueItem) error {
	var payload addTagPayload
	if err := json.Unmarshal([]byte(item.Payload), &payload); err != nil {
		return fmt.Errorf("parse add_tag payload: %w", err)
	}

	note, err := b.findBearNoteForItem(ctx, item)
	if err != nil {
		return fmt.Errorf("find bear note: %w", err)
	}

	// Duplicate-safe: check if note already has the tag.
	tags, err := b.db.NoteTagTitles(ctx, note.UUID)
	if err != nil {
		return fmt.Errorf("check existing tags: %w", err)
	}

	for _, t := range tags {
		if t == payload.Tag {
			b.logger.Info("add_tag already applied (tag exists)", "bear_id", note.UUID, "tag", payload.Tag)
			return nil
		}
	}

	if err := b.xcall.AddTag(ctx, b.bearToken, note.UUID, payload.Tag); err != nil {
		return fmt.Errorf("xcall add-tag: %w", err)
	}

	// Verify tag addition.
	b.sleepFn(verifyDelay)

	updatedTags, err := b.db.NoteTagTitles(ctx, note.UUID)
	if err != nil {
		b.logger.Warn("add_tag verification query failed", "bear_id", note.UUID, "error", err)
		return nil
	}

	found := false
	for _, t := range updatedTags {
		if t == payload.Tag {
			found = true
			break
		}
	}

	if !found {
		b.logger.Warn("add_tag verification: tag not found after add", "bear_id", note.UUID, "tag", payload.Tag)
	}

	return nil
}

// applyTrash moves a note to trash in Bear via xcall.
func (b *Bridge) applyTrash(ctx context.Context, item *models.WriteQueueItem) error {
	note, err := b.findBearNoteForItem(ctx, item)
	if err != nil {
		return fmt.Errorf("find bear note: %w", err)
	}

	// Duplicate-safe: check if note is already trashed.
	if note.Trashed == 1 {
		b.logger.Info("trash already applied (note is trashed)", "bear_id", note.UUID)
		return nil
	}

	if err := b.xcall.Trash(ctx, b.bearToken, note.UUID); err != nil {
		return fmt.Errorf("xcall trash: %w", err)
	}

	// Verify trash.
	b.sleepFn(verifyDelay)

	updated, err := b.db.NoteByUUID(ctx, note.UUID)
	if err != nil {
		b.logger.Warn("trash verification query failed", "bear_id", note.UUID, "error", err)
		return nil
	}

	if updated != nil && updated.Trashed != 1 {
		b.logger.Warn("trash verification: note not trashed after xcall", "bear_id", note.UUID)
	}

	return nil
}

// findBearNoteForItem resolves the Bear UUID for a write queue item.
// The item.NoteID contains a "bear_id" field that maps to the Bear UUID.
// For items that target existing notes, we need to look up the note in Bear's SQLite.
func (b *Bridge) findBearNoteForItem(ctx context.Context, item *models.WriteQueueItem) (*bearNoteRef, error) {
	// The NoteID on the queue item is the hub UUID. We need to find the corresponding Bear UUID.
	// The hub stores bear_id on the note. When the queue item is leased, the hub should include
	// enough info to find the note. We look for bear_id in the payload or use the note_id
	// which might already be the bear_id depending on the flow.

	// Try to parse bear_id from payload first.
	var payloadMap map[string]any
	if err := json.Unmarshal([]byte(item.Payload), &payloadMap); err == nil {
		if bearID, ok := payloadMap["bear_id"].(string); ok && bearID != "" {
			note, err := b.db.NoteByUUID(ctx, bearID)
			if err != nil {
				return nil, fmt.Errorf("query note by bear_id from payload: %w", err)
			}
			if note != nil {
				return &bearNoteRef{UUID: note.UUID, Title: note.Title, Body: note.Body, Trashed: note.Trashed}, nil
			}
		}
	}

	// Try using NoteID directly as a Bear UUID (the hub may store bear_id in note_id for queue items).
	if item.NoteID != "" {
		note, err := b.db.NoteByUUID(ctx, item.NoteID)
		if err != nil {
			return nil, fmt.Errorf("query note by note_id: %w", err)
		}
		if note != nil {
			return &bearNoteRef{UUID: note.UUID, Title: note.Title, Body: note.Body, Trashed: note.Trashed}, nil
		}
	}

	return nil, fmt.Errorf("cannot resolve bear UUID for queue item %d (note_id=%s)", item.ID, item.NoteID)
}

// bearNoteRef holds resolved Bear note info for queue processing.
type bearNoteRef struct {
	UUID    string
	Title   string
	Body    string
	Trashed int64
}

// countByStatus counts ack items with the given status.
func countByStatus(items []models.SyncAckItem, status string) int {
	count := 0
	for i := range items {
		if items[i].Status == status {
			count++
		}
	}
	return count
}

