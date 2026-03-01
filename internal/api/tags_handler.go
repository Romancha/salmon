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

	payload, _ := json.Marshal(req) //nolint:errcheck // marshaling a simple struct cannot fail

	item, err := s.store.EnqueueWrite(
		r.Context(), idempotencyKey, "add_tag", note.ID, string(payload),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to enqueue write")
		return
	}

	writeJSON(w, http.StatusCreated, item)
}
