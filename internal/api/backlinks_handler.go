package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/romancha/salmon/internal/models"
)

// listBacklinks godoc
// @Summary List backlinks for a note
// @Description Returns all notes that link to the specified note.
// @Tags Backlinks
// @Produce json
// @Param noteID path string true "Note ID"
// @Success 200 {array} models.Backlink
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/notes/{noteID}/backlinks [get]
func (s *Server) listBacklinks(w http.ResponseWriter, r *http.Request) {
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

	backlinks, err := s.store.ListBacklinksByNote(r.Context(), noteID)
	if err != nil {
		writeInternalError(w, "failed to list backlinks", err)
		return
	}

	if backlinks == nil {
		backlinks = []models.Backlink{}
	}

	for i := range backlinks {
		backlinks[i].StripInternal()
	}

	writeJSON(w, http.StatusOK, backlinks)
}
