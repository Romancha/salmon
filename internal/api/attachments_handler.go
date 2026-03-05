package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/romancha/salmon/internal/models"
)

const fallbackFilename = "file"

// getAttachment godoc
// @Summary Get an attachment
// @Description Downloads the attachment file by ID. Returns the file with Content-Disposition header.
// @Tags Attachments
// @Produce octet-stream
// @Param id path string true "Attachment ID"
// @Success 200 {file} binary
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/attachments/{id} [get]
func (s *Server) getAttachment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	attachment, err := s.store.GetAttachment(r.Context(), id)
	if err != nil {
		writeInternalError(w, "failed to get attachment", err)
		return
	}

	s.serveAttachmentFile(w, r, attachment)
}

// serveAttachmentFile resolves and serves the file for the given attachment.
func (s *Server) serveAttachmentFile(w http.ResponseWriter, r *http.Request, attachment *models.Attachment) {
	if attachment == nil {
		writeError(w, http.StatusNotFound, "attachment not found")
		return
	}

	filename := filepath.Base(attachment.Filename)
	if filename == "" || filename == "." || filename == "/" {
		filename = fallbackFilename
	}

	filePath := filepath.Join(s.attachmentsDir, attachment.ID, filename)

	if _, err := os.Stat(filePath); os.IsNotExist(err) { //nolint:gosec // path from DB, not user input
		writeError(w, http.StatusNotFound, "attachment file not found on disk")
		return
	}

	sanitized := strings.NewReplacer(`"`, `_`, "\r", "", "\n", "").Replace(filename)
	w.Header().Set("Content-Disposition", `attachment; filename="`+sanitized+`"`)
	http.ServeFile(w, r, filePath)
}

// serveConsumerUploadedFile serves a file uploaded by a consumer via the addFile endpoint.
// Consumer-uploaded files are stored on disk at attachmentsDir/{id}/{filename} without a DB record.
func (s *Server) serveConsumerUploadedFile(w http.ResponseWriter, r *http.Request, id string) {
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, `/\`) {
		writeError(w, http.StatusBadRequest, "invalid attachment id")
		return
	}

	dir := filepath.Join(s.attachmentsDir, id)

	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		writeError(w, http.StatusNotFound, "attachment not found")
		return
	}

	filename := entries[0].Name()
	filePath := filepath.Join(dir, filename)

	sanitized := strings.NewReplacer(`"`, `_`, "\r", "", "\n", "").Replace(filename)
	w.Header().Set("Content-Disposition", `attachment; filename="`+sanitized+`"`)
	http.ServeFile(w, r, filePath) //nolint:gosec // path from internal generated ID
}
