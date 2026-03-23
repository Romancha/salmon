package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/romancha/salmon/internal/beardb"
	"github.com/romancha/salmon/internal/ipc"
	"github.com/romancha/salmon/internal/mapper"
	"github.com/romancha/salmon/internal/models"
	"github.com/romancha/salmon/internal/xcallback"
)

// verifyDelay is how long to wait before verifying bear-xcall results in Bear SQLite.
const verifyDelay = 2 * time.Second

// maxXCallbackBodySize is the practical limit for x-callback-url body size (~50 KB).
// Notes with body exceeding this limit after URL-encoding will fail the bear-xcall invocation.
const maxXCallbackBodySize = 50 * 1024

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

// addFilePayload is the JSON structure for "add_file" action payloads.
type addFilePayload struct {
	AttachmentID string `json:"attachment_id"`
	Filename     string `json:"filename"`
}

// renameTagPayload is the JSON structure for "rename_tag" action payloads.
type renameTagPayload struct {
	Name    string `json:"name"`
	NewName string `json:"new_name"`
}

// deleteTagPayload is the JSON structure for "delete_tag" action payloads.
type deleteTagPayload struct {
	Name string `json:"name"`
}

// processQueue leases write queue items from the hub and applies them to Bear via bear-xcall.
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

	b.events.Emit(&SyncEvent{Event: "sync_progress", Phase: "processing_queue", Items: len(items)})
	b.logger.Info("processing write queue", "items", len(items))

	// Snapshot leased items into stats tracker for IPC visibility.
	if b.stats != nil {
		b.stats.SetQueueItems(buildQueueStatusItems(items))
	}

	ackItems := make([]models.SyncAckItem, 0, len(items))

	for i := range items {
		ack := b.applyQueueItem(ctx, &items[i])
		ackItems = append(ackItems, ack)
		// Update individual item status in tracker after processing.
		if b.stats != nil {
			b.stats.UpdateQueueItemStatus(ack.QueueID, ack.Status)
		}
	}

	if err := b.hub.AckQueue(ctx, ackItems); err != nil {
		return fmt.Errorf("ack queue: %w", err)
	}

	b.cycleQueue = len(ackItems)

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
	// Only check for note-targeted actions (rename_tag/delete_tag have empty NoteID).
	if item.NoteID != "" && item.NoteSyncStatus == "conflict" {
		b.handleConflictItem(ctx, item, &ack)
		return ack
	}

	var err error

	switch item.Action {
	case "create": //nolint:goconst // action string literal used in switch
		ack.BearID, ack.BearModifiedAt, err = b.applyCreate(ctx, item)
	case "update":
		ack.BearModifiedAt, err = b.applyUpdate(ctx, item)
	case "add_tag":
		err = b.applyAddTag(ctx, item)
	case "trash":
		err = b.applyTrash(ctx, item)
	case "add_file":
		err = b.applyAddFile(ctx, item)
	case "archive":
		err = b.applyArchive(ctx, item)
	case "rename_tag":
		err = b.applyRenameTag(ctx, item)
	case "delete_tag":
		err = b.applyDeleteTag(ctx, item)
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

// handleConflictItem creates a conflict note in Bear with the consumer version content.
// The original note in Bear keeps the user's version; a new note "[Conflict] Title" is created
// with the consumer content so the user can manually reconcile.
func (b *Bridge) handleConflictItem(ctx context.Context, item *models.WriteQueueItem, ack *models.SyncAckItem) {
	b.logger.Warn("conflict detected for queue item, creating conflict note",
		"queue_id", item.ID, "action", item.Action, "note_id", item.NoteID)

	// Extract the consumer content and original note tags from the payload.
	title, body, tags := b.extractConflictContent(ctx, item)
	if title == "" {
		title = "Untitled"
	}

	conflictTitle := "[Conflict] " + title

	// Create a conflict note in Bear via bear-xcall, preserving the original note's tags.
	bearID, err := b.xcall.Create(ctx, b.bearToken, conflictTitle, body, tags)
	if err != nil {
		ack.Status = "failed"
		ack.Error = fmt.Sprintf("create conflict note: %v", err)
		b.logger.Warn("failed to create conflict note", "queue_id", item.ID, "error", err)

		return
	}

	ack.Status = "applied"
	ack.ConflictResolved = true
	// Do not set ack.BearID here: bearID is for the new conflict copy note, not the original.
	// Setting it would overwrite the original note's bear_id in the hub, breaking the dual-ID mapping.
	b.logger.Info("conflict note created",
		"queue_id", item.ID, "conflict_bear_id", bearID, "conflict_title", conflictTitle)
}

// extractConflictContent extracts title, body, and tags from a queue item's payload for conflict resolution.
// Tags are fetched from Bear SQLite using the original note's bear_id.
func (b *Bridge) extractConflictContent(
	ctx context.Context, item *models.WriteQueueItem,
) (title, body string, tags []string) {
	var payloadMap map[string]any
	if err := json.Unmarshal([]byte(item.Payload), &payloadMap); err != nil {
		b.logger.Warn("failed to parse conflict item payload", "queue_id", item.ID, "error", err)
		return "", "", nil
	}

	if t, ok := payloadMap["title"].(string); ok {
		title = t
	}

	if bd, ok := payloadMap["body"].(string); ok {
		body = bd
	}

	bearID, _ := payloadMap["bear_id"].(string)

	// If no title in payload, try to get it from the original Bear note using bear_id.
	if title == "" && bearID != "" {
		note, err := b.db.NoteByUUID(ctx, bearID)
		if err == nil && note != nil {
			title = note.Title
		}
	}

	// Fetch tags from the original Bear note so the conflict copy preserves them.
	if bearID != "" {
		var err error
		tags, err = b.db.NoteTagTitles(ctx, bearID)
		if err != nil {
			b.logger.Warn("failed to read tags for conflict note", "bear_id", bearID, "error", err)
		}
	}

	return title, body, tags
}

// applyCreate creates a new note in Bear via bear-xcall and returns the bear_id and bear's modified_at.
func (b *Bridge) applyCreate(
	ctx context.Context, item *models.WriteQueueItem,
) (retBearID, retBearModifiedAt string, retErr error) {
	var payload createPayload
	if err := json.Unmarshal([]byte(item.Payload), &payload); err != nil {
		return "", "", fmt.Errorf("parse create payload: %w", err)
	}

	if len(payload.Body) > maxXCallbackBodySize {
		return "", "", fmt.Errorf("note body too large for x-callback-url (%d bytes, limit %d)", len(payload.Body), maxXCallbackBodySize)
	}

	bearID, err := b.xcall.Create(ctx, b.bearToken, payload.Title, payload.Body, payload.Tags)
	if err != nil {
		return "", "", fmt.Errorf("bear-xcall create: %w", err)
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

		return bearID, bearModifiedAtFromNote(note), nil
	}

	// Fallback verification: bear-xcall didn't return a UUID.
	b.logger.Warn("bear-xcall create returned empty identifier, attempting fallback verification")

	// Capture the baseline epoch BEFORE sleeping so the search window covers the time
	// when the note was actually created, not after the sleep has elapsed.
	createdAfter := currentCoreDataEpoch() - createFallbackWindow

	b.sleepFn(verifyDelay)
	matches, err := b.db.FindRecentNotesByTitle(ctx, payload.Title, createdAfter)
	if err != nil {
		return "", "", fmt.Errorf("fallback search: %w", err)
	}

	switch len(matches) {
	case 0:
		return "", "", fmt.Errorf("create fallback: note not found in Bear after creation")
	case 1:
		return matches[0].UUID, bearModifiedAtFromNote(&matches[0]), nil
	default:
		return "", "", fmt.Errorf("create fallback: ambiguous, found %d notes with title %q", len(matches), payload.Title)
	}
}

// applyUpdate updates a note body in Bear via bear-xcall. Returns Bear's modified_at after apply.
func (b *Bridge) applyUpdate(ctx context.Context, item *models.WriteQueueItem) (string, error) {
	var payload updatePayload
	if err := json.Unmarshal([]byte(item.Payload), &payload); err != nil {
		return "", fmt.Errorf("parse update payload: %w", err)
	}

	// Look up bear_id for this note. The item.NoteID is the hub UUID,
	// but we need the Bear UUID for bear-xcall. Check if note already has the desired content.
	note, err := b.findBearNoteForItem(ctx, item)
	if err != nil {
		return "", fmt.Errorf("find bear note: %w", err)
	}

	// Duplicate-safe: check if note already has the desired body.
	// Return the current modified_at so the hub can seed expected_bear_modified_at for echo detection,
	// even when this is a reprocessed item after lease expiry.
	if payload.Body != "" && note.Body == payload.Body {
		b.logger.Info("update already applied (body matches)", "bear_id", note.UUID)
		return bearModifiedAtFromNote(note), nil
	}

	body := payload.Body
	if body == "" {
		return "", fmt.Errorf("update payload has no body")
	}

	if len(body) > maxXCallbackBodySize {
		return "", fmt.Errorf("note body too large for x-callback-url (%d bytes, limit %d)", len(body), maxXCallbackBodySize)
	}

	if err := b.xcall.Update(ctx, b.bearToken, note.UUID, body); err != nil {
		return "", fmt.Errorf("bear-xcall update: %w", err)
	}

	// Verify update.
	b.sleepFn(verifyDelay)

	updated, err := b.db.NoteByUUID(ctx, note.UUID)
	if err != nil {
		b.logger.Warn("update verification query failed", "bear_id", note.UUID, "error", err)
		return "", nil // bear-xcall succeeded, verification is best-effort
	}

	if updated != nil && updated.Body != body {
		b.logger.Warn("update verification: body mismatch after update", "bear_id", note.UUID)
	}

	return bearModifiedAtFromNote(updated), nil
}

// applyAddTag adds a tag to a note in Bear via bear-xcall.
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
		return fmt.Errorf("bear-xcall add-tag: %w", err)
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

// applyTrash moves a note to trash in Bear via bear-xcall.
func (b *Bridge) applyTrash(ctx context.Context, item *models.WriteQueueItem) error {
	return b.applyNoteStateChange(ctx, item, noteStateChange{
		name:       "trash",
		isApplied:  func(n *beardb.NoteBasicInfo) bool { return n.Trashed == 1 },
		execute:    func(ctx context.Context, bearID string) error { return b.xcall.Trash(ctx, b.bearToken, bearID) },
		isVerified: func(n *beardb.NoteBasicInfo) bool { return n.Trashed == 1 },
	})
}

// findBearNoteForItem resolves the Bear UUID for a write queue item.
// The item.NoteID contains a "bear_id" field that maps to the Bear UUID.
// For items that target existing notes, we need to look up the note in Bear's SQLite.
func (b *Bridge) findBearNoteForItem(ctx context.Context, item *models.WriteQueueItem) (*beardb.NoteBasicInfo, error) {
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
				return note, nil
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
			return note, nil
		}
	}

	return nil, fmt.Errorf("cannot resolve bear UUID for queue item %d (note_id=%s)", item.ID, item.NoteID)
}

// maxBridgeAddFileSize is the maximum raw file size the bridge will pass to bear-xcall AddFile (5 MB).
// Matches xcallback.maxAddFileSize — validated here before the xcall invocation.
const maxBridgeAddFileSize = 5 * 1024 * 1024

// applyAddFile downloads an attachment from the hub and attaches it to a note in Bear via bear-xcall.
func (b *Bridge) applyAddFile(ctx context.Context, item *models.WriteQueueItem) error {
	var payload addFilePayload
	if err := json.Unmarshal([]byte(item.Payload), &payload); err != nil {
		return fmt.Errorf("parse add_file payload: %w", err)
	}

	if payload.AttachmentID == "" || payload.Filename == "" {
		return fmt.Errorf("add_file payload missing attachment_id or filename")
	}

	note, err := b.findBearNoteForItem(ctx, item)
	if err != nil {
		return fmt.Errorf("find bear note: %w", err)
	}

	// Duplicate-safe: check if note already has an attachment with the same filename.
	// Note: this is filename-only; distinct uploads with the same filename to the same note
	// will be treated as already applied. This is an acceptable trade-off for at-least-once
	// delivery idempotency — Bear's DB doesn't store hub attachment IDs for a more precise check.
	existingFiles, err := b.db.NoteAttachmentFilenames(ctx, note.UUID)
	if err != nil {
		b.logger.Warn("add_file duplicate check failed, proceeding", "bear_id", note.UUID, "error", err)
	} else {
		for _, f := range existingFiles {
			if f == payload.Filename {
				b.logger.Info("add_file already applied (filename exists)", "bear_id", note.UUID, "filename", payload.Filename)
				return nil
			}
		}
	}

	fileData, err := b.hub.DownloadAttachment(ctx, payload.AttachmentID)
	if err != nil {
		return fmt.Errorf("download attachment %s: %w", payload.AttachmentID, err)
	}

	if len(fileData) > maxBridgeAddFileSize {
		return fmt.Errorf("attachment %s too large (%d bytes, limit %d)", payload.AttachmentID, len(fileData), maxBridgeAddFileSize)
	}

	if err := b.xcall.AddFile(ctx, b.bearToken, note.UUID, payload.Filename, fileData); err != nil {
		return fmt.Errorf("bear-xcall add-file: %w", err)
	}

	b.logger.Info("bear-xcall add-file succeeded", "bear_id", note.UUID, "filename", payload.Filename)

	return nil
}

// applyArchive archives a note in Bear via bear-xcall.
func (b *Bridge) applyArchive(ctx context.Context, item *models.WriteQueueItem) error {
	return b.applyNoteStateChange(ctx, item, noteStateChange{
		name:       "archive",
		isApplied:  func(n *beardb.NoteBasicInfo) bool { return n.Archived == 1 },
		execute:    func(ctx context.Context, bearID string) error { return b.xcall.Archive(ctx, b.bearToken, bearID) },
		isVerified: func(n *beardb.NoteBasicInfo) bool { return n.Archived == 1 },
	})
}

// noteStateChange describes a note state mutation (trash, archive) for the shared apply helper.
type noteStateChange struct {
	name       string
	isApplied  func(*beardb.NoteBasicInfo) bool
	execute    func(ctx context.Context, bearID string) error
	isVerified func(*beardb.NoteBasicInfo) bool
}

// applyNoteStateChange is a shared helper for trash/archive — both follow the same pattern:
// find note → duplicate check → execute xcall → verify.
func (b *Bridge) applyNoteStateChange(ctx context.Context, item *models.WriteQueueItem, sc noteStateChange) error {
	note, err := b.findBearNoteForItem(ctx, item)
	if err != nil {
		return fmt.Errorf("find bear note: %w", err)
	}

	if sc.isApplied(note) {
		b.logger.Info(sc.name+" already applied", "bear_id", note.UUID)
		return nil
	}

	if err := sc.execute(ctx, note.UUID); err != nil {
		return fmt.Errorf("bear-xcall %s: %w", sc.name, err)
	}

	b.sleepFn(verifyDelay)

	updated, err := b.db.NoteByUUID(ctx, note.UUID)
	if err != nil {
		b.logger.Warn(sc.name+" verification query failed", "bear_id", note.UUID, "error", err)
		return nil
	}

	if updated != nil && !sc.isVerified(updated) {
		b.logger.Warn(sc.name+" verification: state not changed after bear-xcall", "bear_id", note.UUID)
	}

	return nil
}

// applyRenameTag renames a tag in Bear via bear-xcall.
func (b *Bridge) applyRenameTag(ctx context.Context, item *models.WriteQueueItem) error {
	var payload renameTagPayload
	if err := json.Unmarshal([]byte(item.Payload), &payload); err != nil {
		return fmt.Errorf("parse rename_tag payload: %w", err)
	}

	if payload.Name == "" || payload.NewName == "" {
		return fmt.Errorf("rename_tag payload missing name or new_name")
	}

	if err := b.xcall.RenameTag(ctx, b.bearToken, payload.Name, payload.NewName); err != nil {
		return fmt.Errorf("bear-xcall rename-tag: %w", err)
	}

	b.logger.Info("bear-xcall rename-tag succeeded", "old_name", payload.Name, "new_name", payload.NewName)

	return nil
}

// applyDeleteTag deletes a tag from all notes in Bear via bear-xcall.
func (b *Bridge) applyDeleteTag(ctx context.Context, item *models.WriteQueueItem) error {
	var payload deleteTagPayload
	if err := json.Unmarshal([]byte(item.Payload), &payload); err != nil {
		return fmt.Errorf("parse delete_tag payload: %w", err)
	}

	if payload.Name == "" {
		return fmt.Errorf("delete_tag payload missing name")
	}

	if err := b.xcall.DeleteTag(ctx, b.bearToken, payload.Name); err != nil {
		var bearErr *xcallback.BearError
		if errors.As(err, &bearErr) && strings.Contains(bearErr.Msg, "not found") {
			b.logger.Info("delete_tag skipped: tag not found in Bear", "tag_name", payload.Name)
			return nil
		}
		return fmt.Errorf("bear-xcall delete-tag: %w", err)
	}

	b.logger.Info("bear-xcall delete-tag succeeded", "tag_name", payload.Name)

	return nil
}

// bearModifiedAtFromNote converts a NoteBasicInfo's ModifiedAt to the RFC3339 string format used by the hub.
// Returns empty string if note is nil or ModifiedAt is zero.
func bearModifiedAtFromNote(note *beardb.NoteBasicInfo) string {
	if note == nil || note.ModifiedAt == 0 {
		return ""
	}
	return mapper.ConvertCoreDataDate(&note.ModifiedAt)
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

// buildQueueStatusItems converts leased WriteQueueItems to IPC QueueStatusItems.
func buildQueueStatusItems(items []models.WriteQueueItem) []ipc.QueueStatusItem {
	result := make([]ipc.QueueStatusItem, len(items))
	for i := range items {
		result[i] = ipc.QueueStatusItem{
			ID:        items[i].ID,
			Action:    items[i].Action,
			NoteTitle: extractNoteTitle(items[i].Payload, items[i].Action),
			Status:    items[i].Status,
			CreatedAt: items[i].CreatedAt,
		}
	}
	return result
}

// extractNoteTitle extracts the note title from a queue item payload where available.
func extractNoteTitle(payload, action string) string {
	var m map[string]any
	if json.Unmarshal([]byte(payload), &m) != nil {
		return ""
	}

	if title, ok := m["title"].(string); ok && title != "" {
		return title
	}

	switch action {
	case "rename_tag":
		if name, ok := m["name"].(string); ok {
			return name
		}
	case "delete_tag":
		if name, ok := m["name"].(string); ok {
			return name
		}
	case "add_tag":
		if tag, ok := m["tag"].(string); ok {
			return tag
		}
	}

	return ""
}
