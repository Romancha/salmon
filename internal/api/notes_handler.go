package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/romancha/salmon/internal/models"
	"github.com/romancha/salmon/internal/store"
)

// maxAddFileSize is the maximum file size for the addFile endpoint (5 MB).
// Must match the bridge-side limit (maxBridgeAddFileSize) to prevent files from being
// accepted by the hub but permanently rejected by the bridge.
const maxAddFileSize = 5 * 1024 * 1024

// errFileTooLarge is returned by writeUploadedFile when actual bytes exceed maxAddFileSize.
var errFileTooLarge = errors.New("file exceeds 5 MB limit")

// listNotes godoc
// @Summary List notes
// @Description Returns a list of notes with optional filtering. Body field is stripped from list responses.
// @Tags Notes
// @Produce json
// @Param tag query string false "Filter by tag name"
// @Param sort query string false "Sort column" Enums(modified_at, created_at, title)
// @Param order query string false "Sort order" Enums(asc, desc)
// @Param trashed query boolean false "Filter by trashed status"
// @Param encrypted query boolean false "Filter by encrypted status"
// @Param limit query integer false "Max results (max 200)"
// @Param offset query integer false "Offset for pagination"
// @Success 200 {array} models.Note
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/notes [get]
func (s *Server) listNotes(w http.ResponseWriter, r *http.Request) {
	filter := store.NoteFilter{
		Tag:   r.URL.Query().Get("tag"),
		Sort:  r.URL.Query().Get("sort"),
		Order: r.URL.Query().Get("order"),
	}

	if v := r.URL.Query().Get("trashed"); v != "" {
		b := v == "true"
		filter.Trashed = &b
	}

	if v := r.URL.Query().Get("encrypted"); v != "" {
		b := v == "true"
		filter.Encrypted = &b
	}

	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "invalid limit parameter")
			return
		}
		if n > 200 {
			n = 200
		}
		filter.Limit = n
	}

	if v := r.URL.Query().Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "invalid offset parameter")
			return
		}
		filter.Offset = n
	}

	notes, err := s.store.ListNotes(r.Context(), filter)
	if err != nil {
		writeInternalError(w, "failed to list notes", err)
		return
	}

	if notes == nil {
		notes = []models.Note{}
	}

	// Strip body and internal fields from list response.
	for i := range notes {
		notes[i].Body = ""
		notes[i].StripInternal()
	}

	writeJSON(w, http.StatusOK, notes)
}

// searchNotes godoc
// @Summary Search notes
// @Description Full-text search across note titles and bodies using FTS5.
// @Tags Notes
// @Produce json
// @Param q query string true "Search query"
// @Param tag query string false "Filter by tag name"
// @Param limit query integer false "Max results (default 20, max 200)"
// @Success 200 {array} models.Note
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/notes/search [get]
func (s *Server) searchNotes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "q parameter is required")
		return
	}

	tag := r.URL.Query().Get("tag")

	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		n, _ := strconv.Atoi(v)
		if n > 0 {
			limit = n
		}
		if limit > 200 {
			limit = 200
		}
	}

	notes, err := s.store.SearchNotes(r.Context(), q, tag, limit)
	if err != nil {
		writeInternalError(w, "search failed", err)
		return
	}

	if notes == nil {
		notes = []models.Note{}
	}

	for i := range notes {
		notes[i].StripInternal()
	}

	writeJSON(w, http.StatusOK, notes)
}

// getNote godoc
// @Summary Get a note
// @Description Returns a single note by ID, including body, tags, attachments, and backlinks.
// @Tags Notes
// @Produce json
// @Param id path string true "Note ID"
// @Success 200 {object} models.Note
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/notes/{id} [get]
func (s *Server) getNote(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	note, err := s.store.GetNote(r.Context(), id)
	if err != nil {
		writeInternalError(w, "failed to get note", err)
		return
	}

	if note == nil {
		writeError(w, http.StatusNotFound, "note not found")
		return
	}

	note.StripInternal()

	writeJSON(w, http.StatusOK, note)
}

type createNoteRequest struct {
	Title string   `json:"title" example:"Meeting Notes"`
	Body  string   `json:"body" example:"# Meeting Notes\nDiscussed project roadmap."`
	Tags  []string `json:"tags,omitempty" example:"work,meetings"`
}

// createNote godoc
// @Summary Create a note
// @Description Creates a new note and enqueues it for sync to Bear. Requires Idempotency-Key header.
// @Tags Notes
// @Accept json
// @Produce json
// @Param Idempotency-Key header string true "Idempotency key for deduplication"
// @Param request body createNoteRequest true "Note to create"
// @Success 201 {object} models.Note
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/notes [post]
func (s *Server) createNote(w http.ResponseWriter, r *http.Request) {
	var req createNoteRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")

	consumerID := ConsumerIDFromContext(r.Context())

	// Check idempotency before creating the note to prevent duplicate notes on retries.
	// Skip early return for failed items so the caller can retry a previously failed enqueue.
	if idempotencyKey != "" {
		existing, err := s.store.GetQueueItemByIdempotencyKey(r.Context(), idempotencyKey, consumerID)
		if isRetryableQueueItem(existing, err) {
			note, _ := s.store.GetNote(r.Context(), existing.NoteID) //nolint:errcheck // best-effort lookup
			if note != nil {
				note.StripInternal()
				writeJSON(w, http.StatusCreated, note)
			} else {
				// Queue item exists but note lookup failed — still return success to avoid duplicates.
				writeJSON(w, http.StatusCreated, map[string]string{"id": existing.NoteID, "status": "accepted"})
			}
			return
		}

		// Clean up the orphaned note from a previously failed "create" to avoid a stuck pending_to_bear record.
		if existing != nil && existing.Status == "failed" && existing.Action == "create" && existing.NoteID != "" {
			if delErr := s.store.DeleteNote(r.Context(), existing.NoteID); delErr != nil {
				slog.Error("failed to delete orphaned note from failed create", //nolint:gosec // G706: note_id is generated UUID
					"note_id", existing.NoteID, "error", delErr.Error())
			}
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)

	payload, _ := json.Marshal(req) //nolint:errcheck // marshaling a simple struct cannot fail

	note := &models.Note{
		ID:            generateID(),
		Title:         req.Title,
		Body:          req.Body,
		SyncStatus:    syncStatusPendingToBear,
		HubModifiedAt: now,
		CreatedAt:     now,
		ModifiedAt:    now,
	}

	if err := s.store.CreateNote(r.Context(), note); err != nil {
		writeInternalError(w, "failed to create note", err)
		return
	}

	queueItem, err := s.store.EnqueueWrite(
		r.Context(), idempotencyKey, "create", note.ID, string(payload), consumerID,
	)
	if err != nil {
		// Compensate: delete the orphaned note to avoid a stuck pending_to_bear record.
		if delErr := s.store.DeleteNote(r.Context(), note.ID); delErr != nil {
			slog.Error("failed to delete orphaned note after enqueue failure", //nolint:gosec // G706: note_id is generated UUID, not user input
				"note_id", note.ID, "enqueue_error", err.Error(), "delete_error", delErr.Error())
		}
		writeInternalError(w, "failed to enqueue write", err)
		return
	}

	// Concurrent retry guard: if EnqueueWrite returned an existing queue item pointing to a different note
	// (e.g. another concurrent retry already reset the failed item), our note is the orphan — clean it up.
	if queueItem.NoteID != note.ID {
		if delErr := s.store.DeleteNote(r.Context(), note.ID); delErr != nil {
			slog.Error("failed to delete orphaned note from concurrent retry", //nolint:gosec // G706: note_id is generated UUID
				"note_id", note.ID, "error", delErr.Error())
		}
		existingNote, _ := s.store.GetNote(r.Context(), queueItem.NoteID) //nolint:errcheck // best-effort lookup
		if existingNote != nil {
			existingNote.StripInternal()
			writeJSON(w, http.StatusCreated, existingNote)
		} else {
			writeJSON(w, http.StatusCreated, map[string]string{"id": queueItem.NoteID, "status": "accepted"})
		}
		return
	}

	note.StripInternal()

	writeJSON(w, http.StatusCreated, note)
}

type updateNoteRequest struct {
	Body string `json:"body" example:"# Updated Content\nNew body text."`
}

// updateNote godoc
// @Summary Update a note
// @Description Updates an existing note's body. Title is auto-extracted from the first line. Requires Idempotency-Key header.
// @Tags Notes
// @Accept json
// @Produce json
// @Param id path string true "Note ID"
// @Param Idempotency-Key header string true "Idempotency key for deduplication"
// @Param request body updateNoteRequest true "Fields to update"
// @Success 200 {object} models.Note
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/notes/{id} [put]
func (s *Server) updateNote(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	note, err := s.store.GetNote(r.Context(), id)
	if err != nil {
		writeInternalError(w, "failed to get note", err)
		return
	}

	if note == nil {
		writeError(w, http.StatusNotFound, "note not found")
		return
	}

	if note.Encrypted == 1 {
		writeError(w, http.StatusForbidden, "encrypted notes are read-only")
		return
	}

	if note.SyncStatus == syncStatusConflict {
		writeError(w, http.StatusConflict, "note has unresolved conflicts; resolve conflicts before updating")
		return
	}

	var req updateNoteRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Body == "" {
		writeError(w, http.StatusBadRequest, "body is required")
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")
	consumerID := ConsumerIDFromContext(r.Context())

	// Check idempotency before mutating to prevent timestamp re-bumping on retries.
	// Skip early return for failed items so the caller can retry a previously failed enqueue.
	if idempotencyKey != "" {
		existing, err := s.store.GetQueueItemByIdempotencyKey(r.Context(), idempotencyKey, consumerID)
		if isRetryableQueueItem(existing, err) {
			note.StripInternal()
			writeJSON(w, http.StatusOK, note)
			return
		}
	}

	// If note has no BearID, it hasn't synced to Bear yet. Try create→update coalescing.
	if note.BearID == nil || *note.BearID == "" {
		s.handleCreateUpdateCoalesce(w, r, note, req, idempotencyKey, consumerID)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Save original state for rollback if enqueue fails.
	oldTitle := note.Title
	oldBody := note.Body
	oldSyncStatus := note.SyncStatus
	oldHubModifiedAt := note.HubModifiedAt
	oldPendingBearTitle := note.PendingBearTitle
	oldPendingBearBody := note.PendingBearBody

	snapshotPendingBear(note)

	note.Title = extractTitleFromBody(req.Body)
	note.Body = req.Body

	note.SyncStatus = syncStatusPendingToBear
	note.HubModifiedAt = now
	// Do NOT overwrite ModifiedAt here — it must retain the Bear-sourced value
	// so that conflict detection in updateExistingNote can compare correctly.

	if err := s.store.UpdateNote(r.Context(), note); err != nil {
		writeInternalError(w, "failed to update note", err)
		return
	}

	payloadMap := map[string]string{
		"body": req.Body,
	}
	if note.BearID != nil && *note.BearID != "" {
		payloadMap["bear_id"] = *note.BearID
	}

	payload, _ := json.Marshal(payloadMap) //nolint:errcheck // marshaling a simple map cannot fail

	if _, err := s.store.EnqueueWrite(
		r.Context(), idempotencyKey, "update", note.ID, string(payload), consumerID,
	); err != nil {
		// Restore original note state to avoid permanently stuck pending_to_bear.
		note.Title = oldTitle
		note.Body = oldBody
		note.SyncStatus = oldSyncStatus
		note.HubModifiedAt = oldHubModifiedAt
		note.PendingBearTitle = oldPendingBearTitle
		note.PendingBearBody = oldPendingBearBody
		if restoreErr := s.store.UpdateNote(r.Context(), note); restoreErr != nil {
			logNoteRestoreError(note.ID, err, restoreErr)
		}
		writeInternalError(w, "failed to enqueue write", err)
		return
	}

	note.StripInternal()

	writeJSON(w, http.StatusOK, note)
}

// trashNote godoc
// @Summary Trash a note
// @Description Moves a note to trash. Requires Idempotency-Key header.
// @Tags Notes
// @Produce json
// @Param id path string true "Note ID"
// @Param Idempotency-Key header string true "Idempotency key for deduplication"
// @Success 200 {object} models.Note
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/notes/{id} [delete]
func (s *Server) trashNote(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	note, err := s.store.GetNote(r.Context(), id)
	if err != nil {
		writeInternalError(w, "failed to get note", err)
		return
	}

	if note == nil {
		writeError(w, http.StatusNotFound, "note not found")
		return
	}

	if note.Encrypted == 1 {
		writeError(w, http.StatusForbidden, "encrypted notes are read-only")
		return
	}

	if note.BearID == nil || *note.BearID == "" {
		writeError(w, http.StatusConflict, "note not yet synced to Bear; retry after initial sync completes")
		return
	}

	if note.SyncStatus == syncStatusConflict {
		writeError(w, http.StatusConflict, "note has unresolved conflicts; resolve conflicts before trashing")
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")
	consumerID := ConsumerIDFromContext(r.Context())

	// Check idempotency before mutating to prevent timestamp re-bumping on retries.
	// Skip early return for failed items so the caller can retry a previously failed enqueue.
	if idempotencyKey != "" {
		existing, err := s.store.GetQueueItemByIdempotencyKey(r.Context(), idempotencyKey, consumerID)
		if isRetryableQueueItem(existing, err) {
			note.StripInternal()
			writeJSON(w, http.StatusOK, note)
			return
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Save original state for rollback if enqueue fails.
	oldTrashed := note.Trashed
	oldTrashedAt := note.TrashedAt
	oldSyncStatus := note.SyncStatus
	oldHubModifiedAt := note.HubModifiedAt
	oldPendingBearTitle := note.PendingBearTitle
	oldPendingBearBody := note.PendingBearBody

	snapshotPendingBear(note)

	note.Trashed = 1
	note.TrashedAt = now
	note.SyncStatus = syncStatusPendingToBear
	note.HubModifiedAt = now

	if err := s.store.UpdateNote(r.Context(), note); err != nil {
		writeInternalError(w, "failed to update note", err)
		return
	}

	trashPayload := map[string]string{"action": "trash"}
	if note.BearID != nil && *note.BearID != "" {
		trashPayload["bear_id"] = *note.BearID
	}

	payload, _ := json.Marshal(trashPayload) //nolint:errcheck // cannot fail

	if _, err := s.store.EnqueueWrite(
		r.Context(), idempotencyKey, "trash", note.ID, string(payload), consumerID,
	); err != nil {
		// Restore original note state to avoid permanently stuck pending_to_bear.
		note.Trashed = oldTrashed
		note.TrashedAt = oldTrashedAt
		note.SyncStatus = oldSyncStatus
		note.HubModifiedAt = oldHubModifiedAt
		note.PendingBearTitle = oldPendingBearTitle
		note.PendingBearBody = oldPendingBearBody
		if restoreErr := s.store.UpdateNote(r.Context(), note); restoreErr != nil {
			logNoteRestoreError(note.ID, err, restoreErr)
		}
		writeInternalError(w, "failed to enqueue write", err)
		return
	}

	note.StripInternal()

	writeJSON(w, http.StatusOK, note)
}

// archiveNote godoc
// @Summary Archive a note
// @Description Archives a note. Requires Idempotency-Key header.
// @Tags Notes
// @Produce json
// @Param id path string true "Note ID"
// @Param Idempotency-Key header string true "Idempotency key for deduplication"
// @Success 200 {object} models.Note
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/notes/{id}/archive [post]
func (s *Server) archiveNote(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	note, err := s.store.GetNote(r.Context(), id)
	if err != nil {
		writeInternalError(w, "failed to get note", err)
		return
	}

	if note == nil {
		writeError(w, http.StatusNotFound, "note not found")
		return
	}

	if note.Encrypted == 1 {
		writeError(w, http.StatusForbidden, "encrypted notes are read-only")
		return
	}

	if note.BearID == nil || *note.BearID == "" {
		writeError(w, http.StatusConflict, "note not yet synced to Bear; retry after initial sync completes")
		return
	}

	if note.SyncStatus == syncStatusConflict {
		writeError(w, http.StatusConflict, "note has unresolved conflicts; resolve conflicts before archiving")
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")
	consumerID := ConsumerIDFromContext(r.Context())

	if idempotencyKey != "" {
		existing, err := s.store.GetQueueItemByIdempotencyKey(r.Context(), idempotencyKey, consumerID)
		if isRetryableQueueItem(existing, err) {
			note.StripInternal()
			writeJSON(w, http.StatusOK, note)
			return
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Save original state for rollback if enqueue fails.
	oldArchived := note.Archived
	oldArchivedAt := note.ArchivedAt
	oldSyncStatus := note.SyncStatus
	oldHubModifiedAt := note.HubModifiedAt
	oldPendingBearTitle := note.PendingBearTitle
	oldPendingBearBody := note.PendingBearBody

	snapshotPendingBear(note)

	note.Archived = 1
	note.ArchivedAt = now
	note.SyncStatus = syncStatusPendingToBear
	note.HubModifiedAt = now

	if err := s.store.UpdateNote(r.Context(), note); err != nil {
		writeInternalError(w, "failed to update note", err)
		return
	}

	archivePayload := map[string]string{"bear_id": *note.BearID}
	payload, _ := json.Marshal(archivePayload) //nolint:errcheck // cannot fail

	if _, err := s.store.EnqueueWrite(
		r.Context(), idempotencyKey, "archive", note.ID, string(payload), consumerID,
	); err != nil {
		// Restore original note state to avoid permanently stuck pending_to_bear.
		note.Archived = oldArchived
		note.ArchivedAt = oldArchivedAt
		note.SyncStatus = oldSyncStatus
		note.HubModifiedAt = oldHubModifiedAt
		note.PendingBearTitle = oldPendingBearTitle
		note.PendingBearBody = oldPendingBearBody
		if restoreErr := s.store.UpdateNote(r.Context(), note); restoreErr != nil {
			logNoteRestoreError(note.ID, err, restoreErr)
		}
		writeInternalError(w, "failed to enqueue write", err)
		return
	}

	note.StripInternal()

	writeJSON(w, http.StatusOK, note)
}

// addFile godoc
// @Summary Attach a file to a note
// @Description Uploads a file and attaches it to a note. Max file size is 5 MB. Requires Idempotency-Key header.
// @Tags Notes
// @Accept multipart/form-data
// @Produce json
// @Param noteID path string true "Note ID"
// @Param Idempotency-Key header string true "Idempotency key for deduplication"
// @Param file formData file true "File to attach"
// @Success 202 {object} models.WriteQueueItem
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 413 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/notes/{noteID}/attachments [post]
func (s *Server) addFile(w http.ResponseWriter, r *http.Request) {
	noteID := chi.URLParam(r, "noteID")

	note, err := s.store.GetNote(r.Context(), noteID)
	if err != nil {
		writeInternalError(w, "failed to get note", err)
		return
	}

	if note == nil {
		writeError(w, http.StatusNotFound, "note not found")
		return
	}

	if note.Encrypted == 1 {
		writeError(w, http.StatusForbidden, "encrypted notes are read-only")
		return
	}

	if note.BearID == nil || *note.BearID == "" {
		writeError(w, http.StatusConflict, "note not yet synced to Bear; retry after initial sync completes")
		return
	}

	if note.SyncStatus == syncStatusConflict {
		writeError(w, http.StatusConflict, "note has unresolved conflicts; resolve conflicts before updating")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close() //nolint:errcheck // multipart file

	if header.Size > maxAddFileSize {
		writeError(w, http.StatusRequestEntityTooLarge, "file exceeds 5 MB limit")
		return
	}

	filename := filepath.Base(header.Filename)
	if filename == "" || filename == "." || filename == "/" {
		filename = fallbackFilename
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")
	consumerID := ConsumerIDFromContext(r.Context())

	if idempotencyKey != "" {
		existing, err := s.store.GetQueueItemByIdempotencyKey(r.Context(), idempotencyKey, consumerID)
		if isRetryableQueueItem(existing, err) {
			writeJSON(w, http.StatusAccepted, existing)
			return
		}
	}

	attachmentID := generateID()

	dir := filepath.Join(s.attachmentsDir, attachmentID)

	if err := writeUploadedFile(dir, filename, file); err != nil {
		if errors.Is(err, errFileTooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, err.Error())
			return
		}
		writeInternalError(w, "failed to write uploaded file", err)
		return
	}

	payload, _ := json.Marshal(map[string]string{ //nolint:errcheck // cannot fail
		"attachment_id": attachmentID,
		"filename":      filename,
		"bear_id":       *note.BearID,
	})

	item, err := s.store.EnqueueWrite(
		r.Context(), idempotencyKey, "add_file", note.ID, string(payload), consumerID,
	)
	if err != nil {
		os.RemoveAll(dir) //nolint:errcheck,gosec // cleanup on enqueue failure
		writeInternalError(w, "failed to enqueue write", err)
		return
	}

	// If EnqueueWrite returned an existing item (concurrent idempotent request won the race),
	// clean up our orphaned file directory since the winning request's file is already on disk.
	var existingPayload struct {
		AttachmentID string `json:"attachment_id"`
	}
	if json.Unmarshal([]byte(item.Payload), &existingPayload) == nil && existingPayload.AttachmentID != attachmentID {
		os.RemoveAll(dir) //nolint:errcheck,gosec // cleanup orphaned file from lost race
	}

	writeJSON(w, http.StatusAccepted, item)
}

// writeUploadedFile creates the directory and writes the uploaded file to disk.
// It enforces the maxAddFileSize limit on actual bytes written (not just the declared header size).
// On error, it cleans up and returns an error suitable for the HTTP response.
func writeUploadedFile(dir, filename string, file io.Reader) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("failed to create attachment directory")
	}

	filePath := filepath.Join(dir, filename)

	f, err := os.Create(filePath) //nolint:gosec // path from internal generated ID + sanitized filename
	if err != nil {
		os.RemoveAll(dir) //nolint:errcheck,gosec // cleanup
		return fmt.Errorf("failed to create file")
	}

	// Limit actual bytes written to maxAddFileSize+1 to detect oversized files
	// regardless of what the multipart header.Size declared.
	limited := io.LimitReader(file, maxAddFileSize+1)

	n, err := io.Copy(f, limited)
	if err != nil {
		f.Close()         //nolint:errcheck,gosec // closing before cleanup
		os.RemoveAll(dir) //nolint:errcheck,gosec // cleanup
		return fmt.Errorf("failed to write file")
	}

	if n > maxAddFileSize {
		f.Close()         //nolint:errcheck,gosec // closing before cleanup
		os.RemoveAll(dir) //nolint:errcheck,gosec // cleanup
		return errFileTooLarge
	}

	if err := f.Close(); err != nil {
		os.RemoveAll(dir) //nolint:errcheck,gosec // cleanup
		return fmt.Errorf("failed to finalize file")
	}

	return nil
}

// handleCreateUpdateCoalesce attempts to merge an update into a pending create queue item
// for a note that hasn't synced to Bear yet. If no pending create exists, returns 409.
func (s *Server) handleCreateUpdateCoalesce(
	w http.ResponseWriter, r *http.Request,
	note *models.Note, req updateNoteRequest,
	idempotencyKey, consumerID string,
) {
	payloadMap := map[string]string{
		"body": req.Body,
	}
	payload, _ := json.Marshal(payloadMap) //nolint:errcheck // marshaling a simple map cannot fail

	coalesced, err := s.store.CoalesceCreateUpdate(
		r.Context(), idempotencyKey, note.ID, string(payload), consumerID,
	)
	if err != nil {
		writeInternalError(w, "failed to coalesce create-update", err)
		return
	}
	if coalesced == nil {
		writeError(w, http.StatusConflict, "note not yet synced to Bear; retry after initial sync completes")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	note.Title = extractTitleFromBody(req.Body)
	note.Body = req.Body
	note.HubModifiedAt = now
	if err := s.store.UpdateNote(r.Context(), note); err != nil {
		writeInternalError(w, "failed to update note", err)
		return
	}

	note.StripInternal()

	writeJSON(w, http.StatusOK, note)
}

// isRetryableQueueItem returns true if an existing queue item for this idempotency key
// should short-circuit the handler. Failed items return false so callers can retry.
func isRetryableQueueItem(item *models.WriteQueueItem, err error) bool {
	return err == nil && item != nil && item.Status != "failed"
}

// snapshotPendingBear stores Bear's current title/body as the base for field-level conflict detection.
// Only sets fields on initial transition to pending_to_bear; preserves existing snapshot on consecutive updates.
func snapshotPendingBear(note *models.Note) {
	if note.PendingBearTitle != nil {
		return
	}
	bearTitle := note.Title
	bearBody := note.Body
	note.PendingBearTitle = &bearTitle
	note.PendingBearBody = &bearBody
}

// extractTitleFromBody extracts the title from the first line of the note body.
// Bear stores the title as the first line of ZTEXT, optionally prefixed with markdown heading (#).
func extractTitleFromBody(body string) string {
	if body == "" {
		return ""
	}

	firstLine := body
	if idx := strings.IndexByte(body, '\n'); idx != -1 {
		firstLine = body[:idx]
	}

	// Strip markdown heading prefix (e.g. "# Title" → "Title", "## Sub" → "Sub").
	// Use TrimLeft only after confirming "# " pattern to avoid stripping # from hashtags like "#tag".
	trimmed := strings.TrimSpace(firstLine)
	if strings.HasPrefix(trimmed, "# ") || strings.HasPrefix(trimmed, "## ") {
		trimmed = strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
	}

	return trimmed
}

// logNoteRestoreError logs when a note state rollback fails after an enqueue error.
func logNoteRestoreError(noteID string, enqueueErr, restoreErr error) {
	slog.Error("failed to restore note state after enqueue failure", //nolint:gosec // G706: error strings are internal
		"note_id", noteID, "enqueue_error", enqueueErr.Error(), "restore_error", restoreErr.Error())
}
