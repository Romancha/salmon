package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/romancha/bear-sync/internal/models"
)

// listTags godoc
// @Summary List tags
// @Description Returns all tags synced from Bear.
// @Tags Tags
// @Produce json
// @Success 200 {array} models.Tag
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/tags [get]
func (s *Server) listTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.store.ListTags(r.Context())
	if err != nil {
		writeInternalError(w, "failed to list tags", err)
		return
	}

	if tags == nil {
		tags = []models.Tag{}
	}

	writeJSON(w, http.StatusOK, tags)
}

type addTagRequest struct {
	Tag string `json:"tag" example:"work/projects"`
}

// addTag godoc
// @Summary Add a tag to a note
// @Description Adds a tag to an existing note and enqueues the action for sync to Bear. Requires Idempotency-Key header.
// @Tags Tags
// @Accept json
// @Produce json
// @Param noteID path string true "Note ID"
// @Param Idempotency-Key header string true "Idempotency key for deduplication"
// @Param request body addTagRequest true "Tag to add"
// @Success 201 {object} models.WriteQueueItem
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/notes/{noteID}/tags [post]
func (s *Server) addTag(w http.ResponseWriter, r *http.Request) {
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

	var req addTagRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Tag == "" {
		writeError(w, http.StatusBadRequest, "tag is required")
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")
	consumerID := ConsumerIDFromContext(r.Context())

	// Check idempotency before enqueuing to prevent duplicate queue items on retries.
	if idempotencyKey != "" {
		existing, err := s.store.GetQueueItemByIdempotencyKey(r.Context(), idempotencyKey, consumerID)
		if isRetryableQueueItem(existing, err) {
			writeJSON(w, http.StatusCreated, existing)
			return
		}
	}

	tagPayload := map[string]string{"tag": req.Tag}
	if note.BearID != nil && *note.BearID != "" {
		tagPayload["bear_id"] = *note.BearID
	}

	payload, _ := json.Marshal(tagPayload) //nolint:errcheck // marshaling a simple map cannot fail

	item, err := s.store.EnqueueWrite(
		r.Context(), idempotencyKey, "add_tag", note.ID, string(payload), consumerID,
	)
	if err != nil {
		writeInternalError(w, "failed to enqueue write", err)
		return
	}

	writeJSON(w, http.StatusCreated, item)
}

type renameTagRequest struct {
	NewName string `json:"new_name" example:"work/archived-projects"`
}

// renameTag godoc
// @Summary Rename a tag
// @Description Renames a tag across all notes and enqueues the action for sync to Bear. Requires Idempotency-Key header.
// @Tags Tags
// @Accept json
// @Produce json
// @Param id path string true "Tag ID"
// @Param Idempotency-Key header string true "Idempotency key for deduplication"
// @Param request body renameTagRequest true "New tag name"
// @Success 202 {object} models.WriteQueueItem
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/tags/{id} [put]
func (s *Server) renameTag(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	tag, err := s.store.GetTag(r.Context(), id)
	if err != nil {
		writeInternalError(w, "failed to get tag", err)
		return
	}

	if tag == nil {
		writeError(w, http.StatusNotFound, "tag not found")
		return
	}

	var req renameTagRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.NewName == "" {
		writeError(w, http.StatusBadRequest, "new_name is required")
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")
	consumerID := ConsumerIDFromContext(r.Context())

	if idempotencyKey != "" {
		existing, err := s.store.GetQueueItemByIdempotencyKey(r.Context(), idempotencyKey, consumerID)
		if isRetryableQueueItem(existing, err) {
			writeJSON(w, http.StatusOK, existing)
			return
		}
	}

	payload, _ := json.Marshal(map[string]string{ //nolint:errcheck // cannot fail
		"name":     tag.Title,
		"new_name": req.NewName,
	})

	item, err := s.store.EnqueueWrite(r.Context(), idempotencyKey, "rename_tag", "", string(payload), consumerID)
	if err != nil {
		writeInternalError(w, "failed to enqueue write", err)
		return
	}

	writeJSON(w, http.StatusAccepted, item)
}

// deleteTag godoc
// @Summary Delete a tag
// @Description Deletes a tag from all notes and enqueues the action for sync to Bear. Requires Idempotency-Key header.
// @Tags Tags
// @Produce json
// @Param id path string true "Tag ID"
// @Param Idempotency-Key header string true "Idempotency key for deduplication"
// @Success 202 {object} models.WriteQueueItem
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/tags/{id} [delete]
func (s *Server) deleteTag(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	tag, err := s.store.GetTag(r.Context(), id)
	if err != nil {
		writeInternalError(w, "failed to get tag", err)
		return
	}

	if tag == nil {
		writeError(w, http.StatusNotFound, "tag not found")
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")
	consumerID := ConsumerIDFromContext(r.Context())

	if idempotencyKey != "" {
		existing, err := s.store.GetQueueItemByIdempotencyKey(r.Context(), idempotencyKey, consumerID)
		if isRetryableQueueItem(existing, err) {
			writeJSON(w, http.StatusOK, existing)
			return
		}
	}

	payload, _ := json.Marshal(map[string]string{ //nolint:errcheck // cannot fail
		"name": tag.Title,
	})

	item, err := s.store.EnqueueWrite(r.Context(), idempotencyKey, "delete_tag", "", string(payload), consumerID)
	if err != nil {
		writeInternalError(w, "failed to enqueue write", err)
		return
	}

	writeJSON(w, http.StatusAccepted, item)
}
