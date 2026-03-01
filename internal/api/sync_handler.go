package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/romancha/bear-sync/internal/models"
)

func (s *Server) syncPush(w http.ResponseWriter, r *http.Request) {
	var req models.SyncPushRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := s.store.ProcessSyncPush(r.Context(), req); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to process sync push")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	_ = s.store.SetSyncMeta(r.Context(), "last_push_at", now)

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) syncQueue(w http.ResponseWriter, r *http.Request) {
	processingBy := r.URL.Query().Get("processing_by")
	if processingBy == "" {
		processingBy = "bridge"
	}

	items, err := s.store.LeaseQueueItems(r.Context(), processingBy, 5*time.Minute)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to lease queue items")
		return
	}

	if items == nil {
		items = []models.WriteQueueItem{}
	}

	writeJSON(w, http.StatusOK, items)
}

func (s *Server) syncAck(w http.ResponseWriter, r *http.Request) {
	var req models.SyncAckRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := s.store.AckQueueItems(r.Context(), req.Items); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to ack queue items")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) syncUploadAttachment(w http.ResponseWriter, r *http.Request) {
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

	dir := filepath.Join(s.attachmentsDir, attachment.ID)

	if err := os.MkdirAll(dir, 0o750); err != nil { //nolint:gosec // path from DB, not user input
		writeError(w, http.StatusInternalServerError, "failed to create attachment directory")
		return
	}

	filename := attachment.Filename
	if filename == "" {
		filename = "file"
	}

	filePath := filepath.Join(dir, filename)

	f, err := os.Create(filePath) //nolint:gosec // path is constructed from internal data, not user input
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create file")
		return
	}
	defer f.Close() //nolint:errcheck // best-effort close on write path

	if _, err := io.Copy(f, r.Body); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to write file")
		return
	}

	attachment.FilePath = filePath
	if updateErr := s.store.UpdateAttachment(r.Context(), attachment); updateErr != nil {
		writeError(w, http.StatusInternalServerError, "failed to update attachment path")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "path": filePath})
}

type syncStatusResponse struct {
	LastSyncAt          string `json:"last_sync_at"`
	LastPushAt          string `json:"last_push_at"`
	QueueSize           string `json:"queue_size"`
	InitialSyncComplete string `json:"initial_sync_complete"`
}

func (s *Server) syncStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var resp syncStatusResponse

	resp.LastSyncAt, _ = s.store.GetSyncMeta(ctx, "last_sync_at")
	resp.LastPushAt, _ = s.store.GetSyncMeta(ctx, "last_push_at")
	resp.InitialSyncComplete, _ = s.store.GetSyncMeta(ctx, "initial_sync_complete")

	count, err := s.store.PendingQueueCount(ctx)
	if err == nil {
		resp.QueueSize = fmt.Sprintf("%d", count)
	}

	writeJSON(w, http.StatusOK, resp)
}
