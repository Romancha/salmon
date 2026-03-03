package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/romancha/bear-sync/internal/models"
	"github.com/romancha/bear-sync/internal/store"
)

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
		writeError(w, http.StatusInternalServerError, "failed to list notes")
		return
	}

	if notes == nil {
		notes = []models.Note{}
	}

	// Strip body from list response.
	for i := range notes {
		notes[i].Body = ""
	}

	writeJSON(w, http.StatusOK, notes)
}

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
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}

	if notes == nil {
		notes = []models.Note{}
	}

	writeJSON(w, http.StatusOK, notes)
}

func (s *Server) getNote(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	note, err := s.store.GetNote(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get note")
		return
	}

	if note == nil {
		writeError(w, http.StatusNotFound, "note not found")
		return
	}

	writeJSON(w, http.StatusOK, note)
}

type createNoteRequest struct {
	Title string   `json:"title"`
	Body  string   `json:"body"`
	Tags  []string `json:"tags,omitempty"`
}

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

	// Check idempotency before creating the note to prevent duplicate notes on retries.
	// Skip early return for failed items so the caller can retry a previously failed enqueue.
	if idempotencyKey != "" {
		existing, err := s.store.GetQueueItemByIdempotencyKey(r.Context(), idempotencyKey)
		if isRetryableQueueItem(existing, err) {
			note, _ := s.store.GetNote(r.Context(), existing.NoteID) //nolint:errcheck // best-effort lookup
			if note != nil {
				writeJSON(w, http.StatusCreated, note)
			} else {
				// Queue item exists but note lookup failed — still return success to avoid duplicates.
				writeJSON(w, http.StatusCreated, map[string]string{"id": existing.NoteID, "status": "accepted"})
			}
			return
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)

	payload, _ := json.Marshal(req) //nolint:errcheck // marshaling a simple struct cannot fail

	note := &models.Note{
		ID:            generateID(),
		Title:         req.Title,
		Body:          req.Body,
		SyncStatus:    "pending_to_bear",
		HubModifiedAt: now,
		CreatedAt:     now,
		ModifiedAt:    now,
	}

	if err := s.store.CreateNote(r.Context(), note); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create note")
		return
	}

	if _, err := s.store.EnqueueWrite(
		r.Context(), idempotencyKey, "create", note.ID, string(payload), ConsumerIDFromContext(r.Context()),
	); err != nil {
		// Compensate: delete the orphaned note to avoid a stuck pending_to_bear record.
		if delErr := s.store.DeleteNote(r.Context(), note.ID); delErr != nil {
			slog.Error("failed to delete orphaned note after enqueue failure", //nolint:gosec // G706: note_id is generated UUID, not user input
				"note_id", note.ID, "enqueue_error", err.Error(), "delete_error", delErr.Error())
		}
		writeError(w, http.StatusInternalServerError, "failed to enqueue write")
		return
	}

	writeJSON(w, http.StatusCreated, note)
}

type updateNoteRequest struct {
	Title string `json:"title,omitempty"`
	Body  string `json:"body,omitempty"`
}

func (s *Server) updateNote(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	note, err := s.store.GetNote(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get note")
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

	var req updateNoteRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Body == "" {
		writeError(w, http.StatusBadRequest, "body is required (title-only updates are not supported)")
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")

	// Check idempotency before mutating to prevent timestamp re-bumping on retries.
	// Skip early return for failed items so the caller can retry a previously failed enqueue.
	if idempotencyKey != "" {
		existing, err := s.store.GetQueueItemByIdempotencyKey(r.Context(), idempotencyKey)
		if isRetryableQueueItem(existing, err) {
			writeJSON(w, http.StatusOK, note)
			return
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Save original state for rollback if enqueue fails.
	oldTitle := note.Title
	oldBody := note.Body
	oldSyncStatus := note.SyncStatus
	oldHubModifiedAt := note.HubModifiedAt

	if req.Title != "" {
		note.Title = req.Title
	}

	if req.Body != "" {
		note.Body = req.Body
	}

	note.SyncStatus = "pending_to_bear"
	note.HubModifiedAt = now
	// Do NOT overwrite ModifiedAt here — it must retain the Bear-sourced value
	// so that conflict detection in updateExistingNote can compare correctly.

	if err := s.store.UpdateNote(r.Context(), note); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update note")
		return
	}

	payloadMap := map[string]string{}
	if req.Title != "" {
		payloadMap["title"] = req.Title
	}
	if req.Body != "" {
		payloadMap["body"] = req.Body
	}
	if note.BearID != nil && *note.BearID != "" {
		payloadMap["bear_id"] = *note.BearID
	}

	payload, _ := json.Marshal(payloadMap) //nolint:errcheck // marshaling a simple map cannot fail

	if _, err := s.store.EnqueueWrite(
		r.Context(), idempotencyKey, "update", note.ID, string(payload), ConsumerIDFromContext(r.Context()),
	); err != nil {
		// Restore original note state to avoid permanently stuck pending_to_bear.
		note.Title = oldTitle
		note.Body = oldBody
		note.SyncStatus = oldSyncStatus
		note.HubModifiedAt = oldHubModifiedAt
		if restoreErr := s.store.UpdateNote(r.Context(), note); restoreErr != nil {
			logNoteRestoreError(note.ID, err, restoreErr)
		}
		writeError(w, http.StatusInternalServerError, "failed to enqueue write")
		return
	}

	writeJSON(w, http.StatusOK, note)
}

func (s *Server) trashNote(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	note, err := s.store.GetNote(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get note")
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

	// Check idempotency before mutating to prevent timestamp re-bumping on retries.
	// Skip early return for failed items so the caller can retry a previously failed enqueue.
	if idempotencyKey != "" {
		existing, err := s.store.GetQueueItemByIdempotencyKey(r.Context(), idempotencyKey)
		if isRetryableQueueItem(existing, err) {
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

	note.Trashed = 1
	note.TrashedAt = now
	note.SyncStatus = "pending_to_bear"
	note.HubModifiedAt = now

	if err := s.store.UpdateNote(r.Context(), note); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update note")
		return
	}

	trashPayload := map[string]string{"action": "trash"}
	if note.BearID != nil && *note.BearID != "" {
		trashPayload["bear_id"] = *note.BearID
	}

	payload, _ := json.Marshal(trashPayload) //nolint:errcheck // cannot fail

	if _, err := s.store.EnqueueWrite(
		r.Context(), idempotencyKey, "trash", note.ID, string(payload), ConsumerIDFromContext(r.Context()),
	); err != nil {
		// Restore original note state to avoid permanently stuck pending_to_bear.
		note.Trashed = oldTrashed
		note.TrashedAt = oldTrashedAt
		note.SyncStatus = oldSyncStatus
		note.HubModifiedAt = oldHubModifiedAt
		if restoreErr := s.store.UpdateNote(r.Context(), note); restoreErr != nil {
			logNoteRestoreError(note.ID, err, restoreErr)
		}
		writeError(w, http.StatusInternalServerError, "failed to enqueue write")
		return
	}

	writeJSON(w, http.StatusOK, note)
}

// isRetryableQueueItem returns true if an existing queue item for this idempotency key
// should short-circuit the handler. Failed items return false so callers can retry.
func isRetryableQueueItem(item *models.WriteQueueItem, err error) bool {
	return err == nil && item != nil && item.Status != "failed"
}

// logNoteRestoreError logs when a note state rollback fails after an enqueue error.
func logNoteRestoreError(noteID string, enqueueErr, restoreErr error) {
	slog.Error("failed to restore note state after enqueue failure", //nolint:gosec // G706: error strings are internal
		"note_id", noteID, "enqueue_error", enqueueErr.Error(), "restore_error", restoreErr.Error())
}
