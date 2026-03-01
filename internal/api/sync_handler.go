package api

import (
	"context"
	"fmt"
	"io"
	"log/slog"
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

	// Collect attachment IDs to clean up before DB deletion removes the records.
	cleanupIDs := s.collectAttachmentCleanupIDs(r.Context(), req)

	if err := s.store.ProcessSyncPush(r.Context(), req); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to process sync push")
		return
	}

	// Clean up attachment files from disk after successful DB processing.
	s.cleanupAttachmentFiles(cleanupIDs)

	now := time.Now().UTC().Format(time.RFC3339)

	_ = s.store.SetSyncMeta(r.Context(), "last_push_at", now)

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// collectAttachmentCleanupIDs gathers attachment IDs whose files should be removed from disk.
// This includes explicitly deleted attachments and those marked as permanently_deleted.
//nolint:gocritic // req is read-only, no need for pointer
func (s *Server) collectAttachmentCleanupIDs(ctx context.Context, req models.SyncPushRequest) []string {
	var ids []string

	// Attachments in deleted_attachment_ids will be removed from DB — clean their files.
	for _, bearID := range req.DeletedAttachmentIDs {
		att, err := s.store.GetAttachmentByBearID(ctx, bearID)
		if err != nil || att == nil {
			continue
		}
		ids = append(ids, att.ID)
	}

	// Attachments pushed with permanently_deleted=1.
	for i := range req.Attachments {
		if req.Attachments[i].PermanentlyDeleted == 1 {
			if req.Attachments[i].BearID != nil && *req.Attachments[i].BearID != "" {
				att, err := s.store.GetAttachmentByBearID(ctx, *req.Attachments[i].BearID)
				if err != nil || att == nil {
					continue
				}
				ids = append(ids, att.ID)
			}
		}
	}

	return ids
}

// cleanupAttachmentFiles removes attachment directories from disk.
func (s *Server) cleanupAttachmentFiles(attachmentIDs []string) {
	for _, id := range attachmentIDs {
		dir := filepath.Join(s.attachmentsDir, id)
		if err := os.RemoveAll(dir); err != nil { //nolint:gosec // path from internal data
			slog.Warn("failed to remove attachment files", "attachment_id", id, "error", err)
		} else {
			slog.Debug("removed attachment files", "attachment_id", id)
		}
	}
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
	LastSyncAt          string   `json:"last_sync_at"`
	LastPushAt          string   `json:"last_push_at"`
	QueueSize           string   `json:"queue_size"`
	InitialSyncComplete string   `json:"initial_sync_complete"`
	ConflictCount       int      `json:"conflict_count"`
	ConflictNoteIDs     []string `json:"conflict_note_ids,omitempty"`
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

	conflictCount, err := s.store.CountConflicts(ctx)
	if err == nil {
		resp.ConflictCount = conflictCount
	}

	if conflictCount > 0 {
		conflictIDs, err := s.store.ListConflictNoteIDs(ctx)
		if err == nil {
			resp.ConflictNoteIDs = conflictIDs
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
