package api

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
)

func (s *Server) getAttachment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	attachment, err := s.store.GetAttachment(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get attachment")
		return
	}

	if attachment == nil {
		writeError(w, http.StatusNotFound, "attachment not found")
		return
	}

	filename := filepath.Base(attachment.Filename)
	if filename == "" || filename == "." || filename == "/" {
		filename = "file"
	}

	filePath := filepath.Join(s.attachmentsDir, attachment.ID, filename)

	if _, err := os.Stat(filePath); os.IsNotExist(err) { //nolint:gosec // path from DB, not user input
		writeError(w, http.StatusNotFound, "attachment file not found on disk")
		return
	}

	http.ServeFile(w, r, filePath)
}
