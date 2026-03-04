package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/romancha/bear-sync/internal/models"
)

func (s *Server) listTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.store.ListTags(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tags")
		return
	}

	if tags == nil {
		tags = []models.Tag{}
	}

	writeJSON(w, http.StatusOK, tags)
}

type addTagRequest struct {
	Tag string `json:"tag"`
}

func (s *Server) addTag(w http.ResponseWriter, r *http.Request) {
	noteID := chi.URLParam(r, "noteID")

	note, err := s.store.GetNote(r.Context(), noteID)
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
		writeError(w, http.StatusInternalServerError, "failed to enqueue write")
		return
	}

	writeJSON(w, http.StatusCreated, item)
}

type renameTagRequest struct {
	NewName string `json:"new_name"`
}

func (s *Server) renameTag(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	tag, err := s.store.GetTag(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get tag")
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
		writeError(w, http.StatusInternalServerError, "failed to enqueue write")
		return
	}

	writeJSON(w, http.StatusAccepted, item)
}

func (s *Server) deleteTag(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	tag, err := s.store.GetTag(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get tag")
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
		writeError(w, http.StatusInternalServerError, "failed to enqueue write")
		return
	}

	writeJSON(w, http.StatusAccepted, item)
}
