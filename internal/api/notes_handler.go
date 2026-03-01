package api

import (
	"encoding/json"
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
		n, _ := strconv.Atoi(v)
		filter.Limit = n
	}

	if v := r.URL.Query().Get("offset"); v != "" {
		n, _ := strconv.Atoi(v)
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
		r.Context(), idempotencyKey, "create", note.ID, string(payload),
	); err != nil {
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

	var req updateNoteRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")
	now := time.Now().UTC().Format(time.RFC3339)

	if req.Title != "" {
		note.Title = req.Title
	}

	if req.Body != "" {
		note.Body = req.Body
	}

	note.SyncStatus = "pending_to_bear"
	note.HubModifiedAt = now
	note.ModifiedAt = now

	if err := s.store.UpdateNote(r.Context(), note); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update note")
		return
	}

	payload, _ := json.Marshal(req) //nolint:errcheck // marshaling a simple struct cannot fail

	if _, err := s.store.EnqueueWrite(
		r.Context(), idempotencyKey, "update", note.ID, string(payload),
	); err != nil {
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

	idempotencyKey := r.Header.Get("Idempotency-Key")
	now := time.Now().UTC().Format(time.RFC3339)

	note.Trashed = 1
	note.TrashedAt = now
	note.SyncStatus = "pending_to_bear"
	note.HubModifiedAt = now

	if err := s.store.UpdateNote(r.Context(), note); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update note")
		return
	}

	payload, _ := json.Marshal(map[string]string{"action": "trash"}) //nolint:errcheck // cannot fail

	if _, err := s.store.EnqueueWrite(
		r.Context(), idempotencyKey, "trash", note.ID, string(payload),
	); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to enqueue write")
		return
	}

	writeJSON(w, http.StatusOK, note)
}
