package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/romancha/salmon/internal/beardb"
	"github.com/romancha/salmon/internal/hubclient"
	"github.com/romancha/salmon/internal/ipc"
	"github.com/romancha/salmon/internal/mapper"
	"github.com/romancha/salmon/internal/models"
	"github.com/romancha/salmon/internal/xcallback"
)

// junctionFullScanInterval determines how often a full junction table scan is performed.
const junctionFullScanInterval = 12

// initialSyncBatchSize is the number of notes per batch during initial sync.
const initialSyncBatchSize = 50

// Bridge orchestrates the sync cycle between Bear SQLite and the hub.
type Bridge struct {
	db          beardb.BearDB
	hub         hubclient.HubClient
	xcall       xcallback.XCallback
	bearToken   string
	statePath   string
	bearDataDir string // Bear Application Data directory for reading attachment files
	logger      *slog.Logger
	sleepFn     func(time.Duration) // injectable sleep for testing
	events      *EventEmitter       // structured status events (nil = disabled)
	stats       *ipc.StatsTracker   // IPC stats tracker (nil = not in daemon mode)
	cycleNotes  int                 // notes synced in current cycle (reset per Run)
	cycleTags   int                 // tags synced in current cycle (reset per Run)
	cycleQueue  int                 // queue items processed in current cycle (reset per Run)
}

// NewBridge creates a new Bridge instance.
func NewBridge(
	db beardb.BearDB,
	hub hubclient.HubClient,
	xcall xcallback.XCallback,
	bearToken string,
	statePath string,
	bearDataDir string,
	logger *slog.Logger,
) *Bridge {
	return &Bridge{
		db:          db,
		hub:         hub,
		xcall:       xcall,
		bearToken:   bearToken,
		statePath:   statePath,
		bearDataDir: bearDataDir,
		logger:      logger,
		sleepFn:     time.Sleep,
	}
}

// Run executes a single sync cycle: delta export + push + queue processing.
// Emits structured status events (sync_start, sync_progress, sync_complete/sync_error)
// when an EventEmitter is configured.
func (b *Bridge) Run(ctx context.Context) (retErr error) {
	b.cycleNotes = 0
	b.cycleTags = 0
	b.cycleQueue = 0
	b.events.Emit(&SyncEvent{Event: "sync_start"})
	start := time.Now()

	defer func() {
		durationMs := time.Since(start).Milliseconds()
		if retErr != nil {
			b.events.Emit(&SyncEvent{Event: "sync_error", Error: retErr.Error(), DurationMs: durationMs})
		} else {
			b.events.Emit(&SyncEvent{
				Event:       "sync_complete",
				DurationMs:  durationMs,
				NotesSynced: b.cycleNotes,
				TagsSynced:  b.cycleTags,
				QueueItems:  b.cycleQueue,
			})
		}
	}()

	state, err := loadState(b.statePath)
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	if state == nil {
		if err := b.initialSync(ctx); err != nil {
			return err
		}
	} else {
		modTime := b.bearDBModTime()
		if modTime != 0 && modTime == state.LastDBModTimeNano {
			b.logger.Info("bear db unchanged, skipping delta sync")
		} else {
			state.LastDBModTimeNano = modTime
			if err := b.deltaSync(ctx, state); err != nil {
				return err
			}
		}
	}

	// Process write queue (apply pending operations from hub to Bear).
	if err := b.processQueue(ctx); err != nil {
		return fmt.Errorf("process queue: %w", err)
	}

	return nil
}

// bearDBModTime returns the latest modification time (UnixNano) of the Bear SQLite
// database file and its WAL file. Returns 0 if stat fails.
func (b *Bridge) bearDBModTime() int64 {
	dbPath := filepath.Join(b.bearDataDir, "database.sqlite")
	walPath := dbPath + "-wal"

	var maxNano int64

	if info, err := os.Stat(dbPath); err == nil {
		if t := info.ModTime().UnixNano(); t > maxNano {
			maxNano = t
		}
	}

	if info, err := os.Stat(walPath); err == nil {
		if t := info.ModTime().UnixNano(); t > maxNano {
			maxNano = t
		}
	}

	return maxNano
}

// initialSyncData holds all data read from Bear during initial sync.
type initialSyncData struct {
	noteRows       []mapper.BearNoteRow
	tagRows        []mapper.BearTagRow
	attachmentRows []mapper.BearAttachmentRow
	backlinkRows   []mapper.BearBacklinkRow
	tags           []models.Tag
	attachments    []models.Attachment
	backlinks      []models.Backlink
	noteTags       []beardb.NoteTagPair
	pinnedNoteTags []beardb.NoteTagPair
}

// readAllBearData reads all entities from Bear SQLite for initial sync.
func (b *Bridge) readAllBearData(ctx context.Context) (*initialSyncData, error) {
	d := &initialSyncData{}
	var err error

	if d.noteRows, err = b.db.Notes(ctx, 0); err != nil {
		return nil, fmt.Errorf("read all notes: %w", err)
	}
	if d.tagRows, err = b.db.Tags(ctx, 0); err != nil {
		return nil, fmt.Errorf("read all tags: %w", err)
	}
	if d.attachmentRows, err = b.db.Attachments(ctx, 0); err != nil {
		return nil, fmt.Errorf("read all attachments: %w", err)
	}
	if d.backlinkRows, err = b.db.Backlinks(ctx, 0); err != nil {
		return nil, fmt.Errorf("read all backlinks: %w", err)
	}
	if d.noteTags, err = b.db.NoteTags(ctx); err != nil {
		return nil, fmt.Errorf("read all note tags: %w", err)
	}
	if d.pinnedNoteTags, err = b.db.PinnedNoteTags(ctx); err != nil {
		return nil, fmt.Errorf("read all pinned note tags: %w", err)
	}
	if d.tags, err = mapTags(d.tagRows); err != nil {
		return nil, fmt.Errorf("map tags: %w", err)
	}
	if d.attachments, err = mapAttachments(d.attachmentRows); err != nil {
		return nil, fmt.Errorf("map attachments: %w", err)
	}
	if d.backlinks, err = mapBacklinks(d.backlinkRows); err != nil {
		return nil, fmt.Errorf("map backlinks: %w", err)
	}

	return d, nil
}

// initialSync performs the first-time full export from Bear to the hub.
func (b *Bridge) initialSync(ctx context.Context) error {
	b.logger.Info("starting initial sync")

	d, err := b.readAllBearData(ctx)
	if err != nil {
		return err
	}
	b.events.Emit(&SyncEvent{Event: "sync_progress", Phase: "reading_bear", Notes: len(d.noteRows)})

	b.events.Emit(&SyncEvent{Event: "sync_progress", Phase: "pushing_hub", Notes: len(d.noteRows)})
	if err := b.pushNotesBatched(ctx, d.noteRows); err != nil {
		return err
	}

	// Push tags, attachments, backlinks, junction tables, and mark initial sync complete on hub.
	metaReq := models.SyncPushRequest{
		Tags:           d.tags,
		Attachments:    d.attachments,
		Backlinks:      d.backlinks,
		NoteTags:       convertNoteTags(d.noteTags),
		PinnedNoteTags: convertNoteTags(d.pinnedNoteTags),
		Meta:           map[string]string{"initial_sync_complete": "true"},
	}

	if err := b.hub.SyncPush(ctx, metaReq); err != nil {
		return fmt.Errorf("push tags/attachments/backlinks: %w", err)
	}

	// Upload attachment files after metadata push.
	if len(d.attachments) > 0 {
		b.uploadAttachmentModels(ctx, d.attachments)
	}

	// Build and save initial state.
	state := &BridgeState{
		LastSyncAt:              currentCoreDataEpoch(),
		LastDBModTimeNano:       b.bearDBModTime(),
		KnownNoteIDs:            extractNoteUUIDs(d.noteRows),
		KnownTagIDs:             extractTagUUIDs(d.tagRows),
		KnownAttachmentIDs:      extractAttachmentUUIDs(d.attachmentRows),
		KnownBacklinkIDs:        extractBacklinkUUIDs(d.backlinkRows),
		KnownNoteTagPairs:       convertToIDPairs(d.noteTags),
		KnownPinnedNoteTagPairs: convertToIDPairs(d.pinnedNoteTags),
	}

	if err := saveState(b.statePath, state); err != nil {
		return fmt.Errorf("save initial state: %w", err)
	}

	b.cycleNotes = len(d.noteRows)
	b.cycleTags = len(d.tagRows)

	b.logger.Info("initial sync complete",
		"notes", len(d.noteRows),
		"tags", len(d.tagRows),
		"attachments", len(d.attachmentRows),
		"backlinks", len(d.backlinkRows))

	return nil
}

// pushNotesBatched pushes notes in batches during initial sync.
func (b *Bridge) pushNotesBatched(ctx context.Context, noteRows []mapper.BearNoteRow) error {
	for i := 0; i < len(noteRows); i += initialSyncBatchSize {
		end := i + initialSyncBatchSize
		if end > len(noteRows) {
			end = len(noteRows)
		}

		batch := noteRows[i:end]
		notes, err := mapNotes(batch)
		if err != nil {
			return fmt.Errorf("map notes batch %d: %w", i/initialSyncBatchSize, err)
		}

		req := models.SyncPushRequest{Notes: notes}
		if err := b.hub.SyncPush(ctx, req); err != nil {
			return fmt.Errorf("push notes batch %d: %w", i/initialSyncBatchSize, err)
		}

		b.logger.Info("pushed notes batch",
			"batch", i/initialSyncBatchSize+1,
			"count", len(notes),
			"total", len(noteRows))
	}

	return nil
}

// deltaSync performs an incremental export of changed entities.
func (b *Bridge) deltaSync(ctx context.Context, state *BridgeState) error {
	b.logger.Info("starting delta sync", "last_sync_at", state.LastSyncAt)

	req, err := b.buildDeltaPush(ctx, state)
	if err != nil {
		return fmt.Errorf("build delta push: %w", err)
	}
	b.events.Emit(&SyncEvent{Event: "sync_progress", Phase: "reading_bear", Notes: len(req.Notes)})

	if isEmptyPush(req) {
		b.logger.Info("no changes detected")
		// Still increment the junction counter even when no changes.
		state.JunctionFullScanCounter++
		if err := saveState(b.statePath, state); err != nil {
			return fmt.Errorf("save state: %w", err)
		}
		return nil
	}

	// Include the sync timestamp in meta so the hub's sync/status endpoint reflects it.
	now := currentCoreDataEpoch()
	if req.Meta == nil {
		req.Meta = map[string]string{}
	}
	req.Meta["last_sync_at"] = time.Now().UTC().Format(time.RFC3339)

	b.events.Emit(&SyncEvent{Event: "sync_progress", Phase: "pushing_hub", Notes: len(req.Notes)})
	if err := b.hub.SyncPush(ctx, *req); err != nil {
		return fmt.Errorf("push delta: %w", err)
	}

	// Upload attachment files for new/changed attachments.
	if len(req.Attachments) > 0 {
		b.uploadAttachmentModels(ctx, req.Attachments)
	}

	// Update state after successful push.
	state.LastSyncAt = now
	state.JunctionFullScanCounter++

	// Update known IDs with current UUIDs from Bear.
	if err := b.updateKnownIDs(ctx, state); err != nil {
		return fmt.Errorf("update known IDs: %w", err)
	}

	if err := saveState(b.statePath, state); err != nil {
		return fmt.Errorf("save state after delta: %w", err)
	}

	b.cycleNotes = len(req.Notes)
	b.cycleTags = len(req.Tags)

	b.logger.Info("delta sync complete",
		"notes", len(req.Notes),
		"tags", len(req.Tags),
		"attachments", len(req.Attachments),
		"backlinks", len(req.Backlinks),
		"deleted_notes", len(req.DeletedNoteIDs),
		"deleted_tags", len(req.DeletedTagIDs))

	return nil
}

// buildDeltaPush reads changed entities from Bear and builds a SyncPushRequest.
func (b *Bridge) buildDeltaPush(ctx context.Context, state *BridgeState) (*models.SyncPushRequest, error) {
	// Read delta entities (>= lastSyncAt for overlap-window protection).
	noteRows, err := b.db.Notes(ctx, state.LastSyncAt)
	if err != nil {
		return nil, fmt.Errorf("read delta notes: %w", err)
	}

	tagRows, err := b.db.Tags(ctx, state.LastSyncAt)
	if err != nil {
		return nil, fmt.Errorf("read delta tags: %w", err)
	}

	attachmentRows, err := b.db.Attachments(ctx, state.LastSyncAt)
	if err != nil {
		return nil, fmt.Errorf("read delta attachments: %w", err)
	}

	backlinkRows, err := b.db.Backlinks(ctx, state.LastSyncAt)
	if err != nil {
		return nil, fmt.Errorf("read delta backlinks: %w", err)
	}

	// Map entities.
	notes, err := mapNotes(noteRows)
	if err != nil {
		return nil, fmt.Errorf("map delta notes: %w", err)
	}

	tags, err := mapTags(tagRows)
	if err != nil {
		return nil, fmt.Errorf("map delta tags: %w", err)
	}

	attachments, err := mapAttachments(attachmentRows)
	if err != nil {
		return nil, fmt.Errorf("map delta attachments: %w", err)
	}

	backlinks, err := mapBacklinks(backlinkRows)
	if err != nil {
		return nil, fmt.Errorf("map delta backlinks: %w", err)
	}

	req := &models.SyncPushRequest{
		Notes:       notes,
		Tags:        tags,
		Attachments: attachments,
		Backlinks:   backlinks,
	}

	// Junction table delta: for each changed note, get full tag snapshot.
	if len(noteRows) > 0 {
		noteUUIDs := extractNoteUUIDs(noteRows)
		junctionTags, jErr := b.db.NoteTagsForNotes(ctx, noteUUIDs)
		if jErr != nil {
			return nil, fmt.Errorf("read note tags for changed notes: %w", jErr)
		}
		req.NoteTags = convertNoteTags(junctionTags)

		pinnedJunctionTags, jErr := b.db.PinnedNoteTagsForNotes(ctx, noteUUIDs)
		if jErr != nil {
			return nil, fmt.Errorf("read pinned note tags for changed notes: %w", jErr)
		}
		req.PinnedNoteTags = convertNoteTags(pinnedJunctionTags)
	}

	// Junction table full-scan every junctionFullScanInterval cycles.
	if state.JunctionFullScanCounter > 0 && state.JunctionFullScanCounter%junctionFullScanInterval == 0 {
		fullScanTags, fullScanPinned, err := b.junctionFullScan(ctx, state)
		if err != nil {
			return nil, fmt.Errorf("junction full scan: %w", err)
		}
		// Merge full-scan results into request (append to any delta results).
		req.NoteTags = mergeNoteTags(req.NoteTags, fullScanTags)
		req.PinnedNoteTags = mergeNoteTags(req.PinnedNoteTags, fullScanPinned)
	}

	// Deletion detection.
	deletedNoteIDs, err := b.detectDeletions(ctx, state.KnownNoteIDs, b.db.AllNoteUUIDs)
	if err != nil {
		return nil, fmt.Errorf("detect note deletions: %w", err)
	}
	req.DeletedNoteIDs = deletedNoteIDs

	deletedTagIDs, err := b.detectDeletions(ctx, state.KnownTagIDs, b.db.AllTagUUIDs)
	if err != nil {
		return nil, fmt.Errorf("detect tag deletions: %w", err)
	}
	req.DeletedTagIDs = deletedTagIDs

	deletedAttIDs, err := b.detectDeletions(ctx, state.KnownAttachmentIDs, b.db.AllAttachmentUUIDs)
	if err != nil {
		return nil, fmt.Errorf("detect attachment deletions: %w", err)
	}
	req.DeletedAttachmentIDs = deletedAttIDs

	deletedBlIDs, err := b.detectDeletions(ctx, state.KnownBacklinkIDs, b.db.AllBacklinkUUIDs)
	if err != nil {
		return nil, fmt.Errorf("detect backlink deletions: %w", err)
	}
	req.DeletedBacklinkIDs = deletedBlIDs

	return req, nil
}

// junctionFullScan reads all junction table pairs and compares with saved snapshot.
// Returns note_tags and pinned_note_tags for notes with changed associations.
func (b *Bridge) junctionFullScan(
	ctx context.Context, state *BridgeState,
) (noteTags, pinnedTags []models.NoteTagPair, err error) {
	b.logger.Info("performing junction table full scan")

	currentNoteTags, err := b.db.NoteTags(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("read all note tags: %w", err)
	}

	currentPinnedNoteTags, err := b.db.PinnedNoteTags(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("read all pinned note tags: %w", err)
	}

	// Find notes with changed note_tags.
	changedNoteTagNotes := findChangedJunctionNotes(state.KnownNoteTagPairs, convertToIDPairs(currentNoteTags))
	changedPinnedNotes := findChangedJunctionNotes(
		state.KnownPinnedNoteTagPairs, convertToIDPairs(currentPinnedNoteTags),
	)

	// Update state with current snapshot.
	state.KnownNoteTagPairs = convertToIDPairs(currentNoteTags)
	state.KnownPinnedNoteTagPairs = convertToIDPairs(currentPinnedNoteTags)

	// Build full tag snapshots for affected notes.
	// For notes with zero remaining tags, include a sentinel pair (empty TagID)
	// so that replaceNoteTags will still resolve the note and delete its old tags.
	var noteTagResults []models.NoteTagPair
	for _, noteUUID := range changedNoteTagNotes {
		found := false
		for _, pair := range currentNoteTags {
			if pair.NoteUUID == noteUUID {
				noteTagResults = append(noteTagResults, models.NoteTagPair{
					NoteID: pair.NoteUUID,
					TagID:  pair.TagUUID,
				})
				found = true
			}
		}
		if !found {
			// Note lost all tags — sentinel pair triggers tag deletion in replaceNoteTags.
			noteTagResults = append(noteTagResults, models.NoteTagPair{
				NoteID: noteUUID,
				TagID:  "",
			})
		}
	}

	var pinnedTagResults []models.NoteTagPair
	for _, noteUUID := range changedPinnedNotes {
		found := false
		for _, pair := range currentPinnedNoteTags {
			if pair.NoteUUID == noteUUID {
				pinnedTagResults = append(pinnedTagResults, models.NoteTagPair{
					NoteID: pair.NoteUUID,
					TagID:  pair.TagUUID,
				})
				found = true
			}
		}
		if !found {
			pinnedTagResults = append(pinnedTagResults, models.NoteTagPair{
				NoteID: noteUUID,
				TagID:  "",
			})
		}
	}

	b.logger.Info("junction full scan complete",
		"changed_note_tag_notes", len(changedNoteTagNotes),
		"changed_pinned_notes", len(changedPinnedNotes))

	return noteTagResults, pinnedTagResults, nil
}

// findChangedJunctionNotes compares old and new junction pairs and returns note UUIDs with changes.
func findChangedJunctionNotes(oldPairs, newPairs []IDPair) []string {
	oldSet := make(map[string]map[string]bool)
	for _, p := range oldPairs {
		if oldSet[p.NoteUUID] == nil {
			oldSet[p.NoteUUID] = make(map[string]bool)
		}
		oldSet[p.NoteUUID][p.TagUUID] = true
	}

	newSet := make(map[string]map[string]bool)
	for _, p := range newPairs {
		if newSet[p.NoteUUID] == nil {
			newSet[p.NoteUUID] = make(map[string]bool)
		}
		newSet[p.NoteUUID][p.TagUUID] = true
	}

	changedNotes := make(map[string]bool)

	// Check for additions or changes.
	for noteUUID, tags := range newSet {
		oldTags, exists := oldSet[noteUUID]
		if !exists {
			changedNotes[noteUUID] = true
			continue
		}
		if len(tags) != len(oldTags) {
			changedNotes[noteUUID] = true
			continue
		}
		for tagUUID := range tags {
			if !oldTags[tagUUID] {
				changedNotes[noteUUID] = true
				break
			}
		}
	}

	// Check for removals (notes that existed in old but not in new).
	for noteUUID := range oldSet {
		if _, exists := newSet[noteUUID]; !exists {
			changedNotes[noteUUID] = true
		}
	}

	result := make([]string, 0, len(changedNotes))
	for noteUUID := range changedNotes {
		result = append(result, noteUUID)
	}

	return result
}

// detectDeletions compares known IDs with current IDs and returns deleted ones.
func (b *Bridge) detectDeletions(
	ctx context.Context,
	knownIDs []string,
	currentIDsFn func(context.Context) ([]string, error),
) ([]string, error) {
	currentIDs, err := currentIDsFn(ctx)
	if err != nil {
		return nil, fmt.Errorf("get current IDs: %w", err)
	}

	currentSet := make(map[string]bool, len(currentIDs))
	for _, id := range currentIDs {
		currentSet[id] = true
	}

	var deleted []string
	for _, id := range knownIDs {
		if !currentSet[id] {
			deleted = append(deleted, id)
		}
	}

	return deleted, nil
}

// updateKnownIDs refreshes the known ID sets in state from the current Bear database.
func (b *Bridge) updateKnownIDs(ctx context.Context, state *BridgeState) error {
	noteIDs, err := b.db.AllNoteUUIDs(ctx)
	if err != nil {
		return fmt.Errorf("get all note UUIDs: %w", err)
	}
	state.KnownNoteIDs = noteIDs

	tagIDs, err := b.db.AllTagUUIDs(ctx)
	if err != nil {
		return fmt.Errorf("get all tag UUIDs: %w", err)
	}
	state.KnownTagIDs = tagIDs

	attIDs, err := b.db.AllAttachmentUUIDs(ctx)
	if err != nil {
		return fmt.Errorf("get all attachment UUIDs: %w", err)
	}
	state.KnownAttachmentIDs = attIDs

	blIDs, err := b.db.AllBacklinkUUIDs(ctx)
	if err != nil {
		return fmt.Errorf("get all backlink UUIDs: %w", err)
	}
	state.KnownBacklinkIDs = blIDs

	noteTags, err := b.db.NoteTags(ctx)
	if err != nil {
		return fmt.Errorf("get all note tags: %w", err)
	}
	state.KnownNoteTagPairs = convertToIDPairs(noteTags)

	pinnedNoteTags, err := b.db.PinnedNoteTags(ctx)
	if err != nil {
		return fmt.Errorf("get all pinned note tags: %w", err)
	}
	state.KnownPinnedNoteTagPairs = convertToIDPairs(pinnedNoteTags)

	return nil
}

// uploadAttachmentModels uploads attachment files from Bear's local storage to the hub.
// Errors are logged but don't fail the sync — metadata is already pushed successfully.
func (b *Bridge) uploadAttachmentModels(ctx context.Context, attachments []models.Attachment) {
	for i := range attachments {
		att := &attachments[i]
		if att.BearID == nil || *att.BearID == "" {
			continue
		}
		if att.PermanentlyDeleted == 1 || att.Encrypted == 1 {
			continue
		}

		filePath := b.resolveAttachmentFilePath(att.Type, *att.BearID, att.Filename)
		if filePath == "" {
			b.logger.Debug("no file path resolved for attachment", "bear_id", *att.BearID, "type", att.Type)
			continue
		}

		// Use bear_id for uploads: the hub resolves by bear_id since mapper-generated IDs
		// are ephemeral and may not match the hub's assigned ID after upsert.
		if err := b.uploadSingleAttachment(ctx, *att.BearID, filePath); err != nil {
			b.logger.Warn("failed to upload attachment file",
				"attachment_id", att.ID, "bear_id", *att.BearID, "path", filePath, "error", err)
		}
	}
}

// resolveAttachmentFilePath determines the full file path for a Bear attachment.
// Bear stores files in Local Files/Note Images or Note Files subdirectories.
func (b *Bridge) resolveAttachmentFilePath(attType, bearID, filename string) string {
	var subdir string

	switch attType {
	case "image", "video":
		subdir = "Note Images"
	default:
		subdir = "Note Files"
	}

	dir := filepath.Join(b.bearDataDir, "Local Files", subdir, bearID)

	if filename != "" {
		candidate := filepath.Join(dir, filename)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// If filename doesn't match or is empty, try to find any file in the directory.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	for _, e := range entries {
		if !e.IsDir() {
			return filepath.Join(dir, e.Name())
		}
	}

	return ""
}

// uploadSingleAttachment reads a file from disk and uploads it to the hub.
func (b *Bridge) uploadSingleAttachment(ctx context.Context, attachmentID, filePath string) error {
	f, err := os.Open(filePath) //nolint:gosec // path constructed from internal data
	if err != nil {
		return fmt.Errorf("open attachment file: %w", err)
	}
	defer f.Close() //nolint:errcheck // best-effort close on read path

	if err := b.hub.UploadAttachment(ctx, attachmentID, f); err != nil {
		return fmt.Errorf("upload to hub: %w", err)
	}

	b.logger.Debug("uploaded attachment file", "attachment_id", attachmentID, "path", filePath)

	return nil
}

// Helper functions for mapping Bear rows to models.

func mapNotes(rows []mapper.BearNoteRow) ([]models.Note, error) {
	notes := make([]models.Note, 0, len(rows))
	for i := range rows {
		note, err := mapper.MapBearNote(&rows[i])
		if err != nil {
			return nil, fmt.Errorf("map note %d: %w", i, err)
		}
		notes = append(notes, note)
	}
	return notes, nil
}

func mapTags(rows []mapper.BearTagRow) ([]models.Tag, error) {
	tags := make([]models.Tag, 0, len(rows))
	for i := range rows {
		tag, err := mapper.MapBearTag(&rows[i])
		if err != nil {
			return nil, fmt.Errorf("map tag %d: %w", i, err)
		}
		tags = append(tags, tag)
	}
	return tags, nil
}

func mapAttachments(rows []mapper.BearAttachmentRow) ([]models.Attachment, error) {
	attachments := make([]models.Attachment, 0, len(rows))
	for i := range rows {
		att, err := mapper.MapBearAttachment(&rows[i])
		if err != nil {
			return nil, fmt.Errorf("map attachment %d: %w", i, err)
		}
		attachments = append(attachments, att)
	}
	return attachments, nil
}

func mapBacklinks(rows []mapper.BearBacklinkRow) ([]models.Backlink, error) {
	backlinks := make([]models.Backlink, 0, len(rows))
	for i := range rows {
		bl, err := mapper.MapBearBacklink(&rows[i])
		if err != nil {
			return nil, fmt.Errorf("map backlink %d: %w", i, err)
		}
		backlinks = append(backlinks, bl)
	}
	return backlinks, nil
}

func convertNoteTags(pairs []beardb.NoteTagPair) []models.NoteTagPair {
	result := make([]models.NoteTagPair, len(pairs))
	for i, p := range pairs {
		result[i] = models.NoteTagPair{
			NoteID: p.NoteUUID,
			TagID:  p.TagUUID,
		}
	}
	return result
}

func convertToIDPairs(pairs []beardb.NoteTagPair) []IDPair {
	result := make([]IDPair, len(pairs))
	for i, p := range pairs {
		result[i] = IDPair{
			NoteUUID: p.NoteUUID,
			TagUUID:  p.TagUUID,
		}
	}
	return result
}

func mergeNoteTags(existing, additional []models.NoteTagPair) []models.NoteTagPair {
	if len(additional) == 0 {
		return existing
	}

	seen := make(map[string]bool, len(existing))
	for _, p := range existing {
		seen[p.NoteID+"|"+p.TagID] = true
	}

	for _, p := range additional {
		key := p.NoteID + "|" + p.TagID
		if !seen[key] {
			existing = append(existing, p)
			seen[key] = true
		}
	}

	return existing
}

func extractNoteUUIDs(rows []mapper.BearNoteRow) []string {
	result := make([]string, 0, len(rows))
	for i := range rows {
		if rows[i].ZUNIQUEIDENTIFIER != nil {
			result = append(result, *rows[i].ZUNIQUEIDENTIFIER)
		}
	}
	return result
}

func extractTagUUIDs(rows []mapper.BearTagRow) []string {
	result := make([]string, 0, len(rows))
	for i := range rows {
		if rows[i].ZUNIQUEIDENTIFIER != nil {
			result = append(result, *rows[i].ZUNIQUEIDENTIFIER)
		}
	}
	return result
}

func extractAttachmentUUIDs(rows []mapper.BearAttachmentRow) []string {
	result := make([]string, 0, len(rows))
	for i := range rows {
		if rows[i].ZUNIQUEIDENTIFIER != nil {
			result = append(result, *rows[i].ZUNIQUEIDENTIFIER)
		}
	}
	return result
}

func extractBacklinkUUIDs(rows []mapper.BearBacklinkRow) []string {
	result := make([]string, 0, len(rows))
	for i := range rows {
		if rows[i].ZUNIQUEIDENTIFIER != nil {
			result = append(result, *rows[i].ZUNIQUEIDENTIFIER)
		}
	}
	return result
}

func isEmptyPush(req *models.SyncPushRequest) bool {
	return len(req.Notes) == 0 &&
		len(req.Tags) == 0 &&
		len(req.NoteTags) == 0 &&
		len(req.PinnedNoteTags) == 0 &&
		len(req.Attachments) == 0 &&
		len(req.Backlinks) == 0 &&
		len(req.DeletedNoteIDs) == 0 &&
		len(req.DeletedTagIDs) == 0 &&
		len(req.DeletedAttachmentIDs) == 0 &&
		len(req.DeletedBacklinkIDs) == 0
}

func currentCoreDataEpoch() float64 {
	return float64(time.Now().Unix() - mapper.CoreDataEpochOffset)
}
