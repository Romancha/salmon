package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/romancha/bear-sync/internal/models"
)

func (s *Server) listBacklinks(w http.ResponseWriter, r *http.Request) {
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

	backlinks, err := s.store.ListBacklinksByNote(r.Context(), noteID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list backlinks")
		return
	}

	if backlinks == nil {
		backlinks = []models.Backlink{}
	}

	writeJSON(w, http.StatusOK, backlinks)
}
