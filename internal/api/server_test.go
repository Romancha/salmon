package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/romancha/bear-sync/internal/api"
	"github.com/romancha/bear-sync/internal/models"
	"github.com/romancha/bear-sync/internal/store"
)

const (
	openclawToken = "test-openclaw-token"
	bridgeToken   = "test-bridge-token"
)

func setupServer(t *testing.T) (*httptest.Server, *store.SQLiteStore) {
	t.Helper()

	s, err := store.NewSQLiteStore(":memory:")
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, s.Close())
	})

	tmpDir := t.TempDir()

	srv := api.NewServer(s, openclawToken, bridgeToken, tmpDir)
	ts := httptest.NewServer(srv)

	t.Cleanup(ts.Close)

	return ts, s
}

func doRequest(
	t *testing.T, ts *httptest.Server,
	method, path string, body any, token string, headers map[string]string,
) *http.Response {
	t.Helper()

	var bodyReader io.Reader

	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)

		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, ts.URL+path, bodyReader) //nolint:noctx // test helper
	require.NoError(t, err)

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req) //nolint:noctx,gosec // test helper
	require.NoError(t, err)

	return resp
}

func readBody(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close() //nolint:errcheck // test helper

	var result map[string]any

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))

	return result
}

func readBodySlice(t *testing.T, resp *http.Response) []map[string]any {
	t.Helper()
	defer resp.Body.Close() //nolint:errcheck // test helper

	var result []map[string]any

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))

	return result
}

// --- Auth tests ---

func TestAuth_MissingHeader(t *testing.T) {
	ts, _ := setupServer(t)

	resp := doRequest(t, ts, http.MethodGet, "/api/notes", nil, "", nil)
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestAuth_InvalidToken(t *testing.T) {
	ts, _ := setupServer(t)

	resp := doRequest(t, ts, http.MethodGet, "/api/notes", nil, "wrong-token", nil)
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestAuth_WrongScope_BridgeOnOpenclawRoute(t *testing.T) {
	ts, _ := setupServer(t)

	resp := doRequest(t, ts, http.MethodGet, "/api/notes", nil, bridgeToken, nil)
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestAuth_WrongScope_OpenclawOnSyncRoute(t *testing.T) {
	ts, _ := setupServer(t)

	resp := doRequest(t, ts, http.MethodGet, "/api/sync/status", nil, openclawToken, nil)
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestAuth_ValidToken(t *testing.T) {
	ts, _ := setupServer(t)

	resp := doRequest(t, ts, http.MethodGet, "/api/notes", nil, openclawToken, nil)
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// --- Notes tests ---

func TestListNotes_Empty(t *testing.T) {
	ts, _ := setupServer(t)

	resp := doRequest(t, ts, http.MethodGet, "/api/notes", nil, openclawToken, nil)

	result := readBodySlice(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Empty(t, result)
}

func TestListNotes_WithData(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-1", Title: "Test Note", Body: "body content",
	}))

	resp := doRequest(t, ts, http.MethodGet, "/api/notes", nil, openclawToken, nil)

	result := readBodySlice(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Len(t, result, 1)
	assert.Equal(t, "Test Note", result[0]["title"])
	assert.Nil(t, result[0]["body"]) // body stripped in list (omitempty -> nil in JSON)
}

func TestListNotes_Filters(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-1", Title: "Normal Note", Trashed: 0,
	}))
	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-2", Title: "Trashed Note", Trashed: 1,
	}))

	resp := doRequest(t, ts, http.MethodGet, "/api/notes?trashed=false", nil, openclawToken, nil)

	result := readBodySlice(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Len(t, result, 1)
	assert.Equal(t, "Normal Note", result[0]["title"])
}

func TestListNotes_Pagination(t *testing.T) {
	ts, s := setupServer(t)

	for i := range 5 {
		require.NoError(t, s.CreateNote(t.Context(), &models.Note{
			ID:    fmt.Sprintf("note-%d", i),
			Title: fmt.Sprintf("Note %d", i),
		}))
	}

	resp := doRequest(t, ts, http.MethodGet, "/api/notes?limit=2&offset=0", nil, openclawToken, nil)

	result := readBodySlice(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Len(t, result, 2)
}

func TestGetNote(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-1", Title: "Test Note", Body: "full body",
	}))

	resp := doRequest(t, ts, http.MethodGet, "/api/notes/note-1", nil, openclawToken, nil)

	result := readBody(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "Test Note", result["title"])
	assert.Equal(t, "full body", result["body"])
}

func TestGetNote_NotFound(t *testing.T) {
	ts, _ := setupServer(t)

	resp := doRequest(t, ts, http.MethodGet, "/api/notes/nonexistent", nil, openclawToken, nil)
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestSearchNotes(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-1", Title: "Golang Tutorial", Body: "Learn Go programming",
	}))
	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-2", Title: "Python Guide", Body: "Learn Python programming",
	}))

	resp := doRequest(t, ts, http.MethodGet, "/api/notes/search?q=golang", nil, openclawToken, nil)

	result := readBodySlice(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Len(t, result, 1)
	assert.Equal(t, "Golang Tutorial", result[0]["title"])
}

func TestSearchNotes_MissingQuery(t *testing.T) {
	ts, _ := setupServer(t)

	resp := doRequest(t, ts, http.MethodGet, "/api/notes/search", nil, openclawToken, nil)
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCreateNote(t *testing.T) {
	ts, _ := setupServer(t)

	body := map[string]string{"title": "New Note", "body": "Content"}

	resp := doRequest(t, ts, http.MethodPost, "/api/notes", body, openclawToken,
		map[string]string{"Idempotency-Key": "key-1"})

	result := readBody(t, resp)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.Equal(t, "New Note", result["title"])
	assert.Equal(t, "pending_to_bear", result["sync_status"])
}

func TestCreateNote_MissingTitle(t *testing.T) {
	ts, _ := setupServer(t)

	body := map[string]string{"body": "Content"}

	resp := doRequest(t, ts, http.MethodPost, "/api/notes", body, openclawToken,
		map[string]string{"Idempotency-Key": "key-1"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCreateNote_MissingIdempotencyKey(t *testing.T) {
	ts, _ := setupServer(t)

	body := map[string]string{"title": "Test", "body": "Content"}

	resp := doRequest(t, ts, http.MethodPost, "/api/notes", body, openclawToken, nil)
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUpdateNote(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-1", Title: "Old Title", Body: "Old body",
	}))

	body := map[string]string{"title": "New Title", "body": "New body"}

	resp := doRequest(t, ts, http.MethodPut, "/api/notes/note-1", body, openclawToken,
		map[string]string{"Idempotency-Key": "key-2"})

	result := readBody(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "New Title", result["title"])
	assert.Equal(t, "pending_to_bear", result["sync_status"])
}

func TestUpdateNote_NotFound(t *testing.T) {
	ts, _ := setupServer(t)

	body := map[string]string{"title": "New Title"}

	resp := doRequest(t, ts, http.MethodPut, "/api/notes/nonexistent", body, openclawToken,
		map[string]string{"Idempotency-Key": "key-2"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestUpdateNote_Encrypted403(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "enc-1", Title: "Encrypted", Body: "", Encrypted: 1,
	}))

	body := map[string]string{"title": "Updated"}

	resp := doRequest(t, ts, http.MethodPut, "/api/notes/enc-1", body, openclawToken,
		map[string]string{"Idempotency-Key": "key-3"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestTrashNote(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-1", Title: "To Trash",
	}))

	resp := doRequest(t, ts, http.MethodDelete, "/api/notes/note-1", nil, openclawToken,
		map[string]string{"Idempotency-Key": "key-4"})

	result := readBody(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, float64(1), result["trashed"])
	assert.Equal(t, "pending_to_bear", result["sync_status"])
}

func TestTrashNote_Encrypted403(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "enc-1", Title: "Encrypted", Encrypted: 1,
	}))

	resp := doRequest(t, ts, http.MethodDelete, "/api/notes/enc-1", nil, openclawToken,
		map[string]string{"Idempotency-Key": "key-5"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// --- Tags tests ---

func TestListTags_Empty(t *testing.T) {
	ts, _ := setupServer(t)

	resp := doRequest(t, ts, http.MethodGet, "/api/tags", nil, openclawToken, nil)

	result := readBodySlice(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Empty(t, result)
}

func TestListTags_WithData(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateTag(t.Context(), &models.Tag{
		ID: "tag-1", Title: "go/programming",
	}))

	resp := doRequest(t, ts, http.MethodGet, "/api/tags", nil, openclawToken, nil)

	result := readBodySlice(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Len(t, result, 1)
	assert.Equal(t, "go/programming", result[0]["title"])
}

func TestAddTag(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-1", Title: "Note",
	}))

	body := map[string]string{"tag": "new-tag"}

	resp := doRequest(t, ts, http.MethodPost, "/api/notes/note-1/tags", body, openclawToken,
		map[string]string{"Idempotency-Key": "key-6"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
}

func TestAddTag_NoteNotFound(t *testing.T) {
	ts, _ := setupServer(t)

	body := map[string]string{"tag": "new-tag"}

	resp := doRequest(t, ts, http.MethodPost, "/api/notes/nonexistent/tags", body, openclawToken,
		map[string]string{"Idempotency-Key": "key-7"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestAddTag_Encrypted403(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "enc-1", Title: "Encrypted", Encrypted: 1,
	}))

	body := map[string]string{"tag": "new-tag"}

	resp := doRequest(t, ts, http.MethodPost, "/api/notes/enc-1/tags", body, openclawToken,
		map[string]string{"Idempotency-Key": "key-8"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestAddTag_MissingTag(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-1", Title: "Note",
	}))

	body := map[string]string{}

	resp := doRequest(t, ts, http.MethodPost, "/api/notes/note-1/tags", body, openclawToken,
		map[string]string{"Idempotency-Key": "key-9"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// --- Backlinks tests ---

func TestListBacklinks(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{ID: "note-1", Title: "Note 1"}))
	require.NoError(t, s.CreateNote(t.Context(), &models.Note{ID: "note-2", Title: "Note 2"}))

	err := s.ProcessSyncPush(t.Context(), models.SyncPushRequest{
		Backlinks: []models.Backlink{
			{ID: "bl-1", LinkedByID: "note-2", LinkingToID: "note-1", Title: "link"},
		},
	})
	require.NoError(t, err)

	resp := doRequest(t, ts, http.MethodGet, "/api/notes/note-1/backlinks", nil, openclawToken, nil)

	result := readBodySlice(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Len(t, result, 1)
}

func TestListBacklinks_NoteNotFound(t *testing.T) {
	ts, _ := setupServer(t)

	resp := doRequest(t, ts, http.MethodGet, "/api/notes/nonexistent/backlinks", nil, openclawToken, nil)
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// --- Attachments tests ---

func TestGetAttachment_NotFound(t *testing.T) {
	ts, _ := setupServer(t)

	resp := doRequest(t, ts, http.MethodGet, "/api/attachments/nonexistent", nil, openclawToken, nil)
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestGetAttachment_FileOnDisk(t *testing.T) {
	tmpDir := t.TempDir()

	s, err := store.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{ID: "note-1", Title: "Note"}))

	bearID := "bear-att-1"

	err = s.ProcessSyncPush(t.Context(), models.SyncPushRequest{
		Attachments: []models.Attachment{
			{
				ID: "att-1", BearID: &bearID, NoteID: "note-1",
				Type: "file", Filename: "test.txt",
			},
		},
	})
	require.NoError(t, err)

	att, err := s.GetAttachment(t.Context(), "att-1")
	require.NoError(t, err)

	attDir := filepath.Join(tmpDir, att.ID)
	require.NoError(t, os.MkdirAll(attDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(attDir, "test.txt"), []byte("file content"), 0o600))

	srv := api.NewServer(s, openclawToken, bridgeToken, tmpDir)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp := doRequest(t, ts, http.MethodGet, "/api/attachments/att-1", nil, openclawToken, nil)
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	b, _ := io.ReadAll(resp.Body) //nolint:errcheck // test
	assert.Equal(t, "file content", string(b))
}

// --- Sync tests ---

func TestSyncPush(t *testing.T) {
	ts, _ := setupServer(t)

	bearID := "bear-note-1"
	bearTagID := "bear-tag-1"
	req := models.SyncPushRequest{
		Notes: []models.Note{
			{BearID: &bearID, Title: "Pushed Note", Body: "content"},
		},
		Tags: []models.Tag{
			{BearID: &bearTagID, Title: "test-tag"},
		},
	}

	resp := doRequest(t, ts, http.MethodPost, "/api/sync/push", req, bridgeToken, nil)

	result := readBody(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "ok", result["status"])
}

func TestSyncQueue_Empty(t *testing.T) {
	ts, _ := setupServer(t)

	resp := doRequest(t, ts, http.MethodGet, "/api/sync/queue", nil, bridgeToken, nil)

	result := readBodySlice(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Empty(t, result)
}

func TestSyncQueue_WithItems(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{ID: "note-1", Title: "Note"}))

	_, err := s.EnqueueWrite(t.Context(), "idem-1", "update", "note-1", `{"body":"new"}`)
	require.NoError(t, err)

	resp := doRequest(t, ts, http.MethodGet, "/api/sync/queue", nil, bridgeToken, nil)

	result := readBodySlice(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Len(t, result, 1)
	assert.Equal(t, "processing", result[0]["status"])
}

func TestSyncAck(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-1", Title: "Note", SyncStatus: "pending_to_bear",
	}))

	_, err := s.EnqueueWrite(t.Context(), "idem-1", "create", "note-1", `{"title":"Note"}`)
	require.NoError(t, err)

	ackReq := models.SyncAckRequest{
		Items: []models.SyncAckItem{
			{QueueID: 1, IdempotencyKey: "idem-1", Status: "applied", BearID: "bear-uuid-123"},
		},
	}

	resp := doRequest(t, ts, http.MethodPost, "/api/sync/ack", ackReq, bridgeToken, nil)

	result := readBody(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "ok", result["status"])

	// Verify bear_id was set on the note.
	note, err := s.GetNote(t.Context(), "note-1")
	require.NoError(t, err)
	require.NotNil(t, note.BearID)
	assert.Equal(t, "bear-uuid-123", *note.BearID)
	assert.Equal(t, "synced", note.SyncStatus)
}

func TestSyncUploadAttachment(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{ID: "note-1", Title: "Note"}))

	bearID := "bear-att-1"

	err := s.ProcessSyncPush(t.Context(), models.SyncPushRequest{
		Attachments: []models.Attachment{
			{
				ID: "att-1", BearID: &bearID, NoteID: "note-1",
				Type: "file", Filename: "uploaded.txt",
			},
		},
	})
	require.NoError(t, err)

	fileContent := "uploaded file content"

	req, err := http.NewRequest( //nolint:noctx // test
		http.MethodPost,
		ts.URL+"/api/sync/attachments/att-1",
		bytes.NewReader([]byte(fileContent)),
	)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+bridgeToken)

	resp, err := http.DefaultClient.Do(req) //nolint:noctx,gosec // test
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestSyncStatus(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.SetSyncMeta(t.Context(), "last_push_at", "2025-01-01T00:00:00Z"))
	require.NoError(t, s.SetSyncMeta(t.Context(), "initial_sync_complete", "true"))

	resp := doRequest(t, ts, http.MethodGet, "/api/sync/status", nil, bridgeToken, nil)

	result := readBody(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "2025-01-01T00:00:00Z", result["last_push_at"])
	assert.Equal(t, "true", result["initial_sync_complete"])
	assert.Equal(t, "0", result["queue_size"])
}

// --- Idempotency tests ---

func TestIdempotency_CreateNoteDuplicate(t *testing.T) {
	ts, s := setupServer(t)

	body := map[string]string{"title": "Note", "body": "Content"}
	headers := map[string]string{"Idempotency-Key": "idem-create-1"}

	resp1 := doRequest(t, ts, http.MethodPost, "/api/notes", body, openclawToken, headers)
	result1 := readBody(t, resp1)
	assert.Equal(t, http.StatusCreated, resp1.StatusCode)

	noteID := result1["id"].(string)

	// Second request creates another note but the queue item is deduplicated by idempotency key.
	resp2 := doRequest(t, ts, http.MethodPost, "/api/notes", body, openclawToken, headers)
	defer resp2.Body.Close() //nolint:errcheck // test
	assert.Equal(t, http.StatusCreated, resp2.StatusCode)

	// Verify original note exists.
	note, err := s.GetNote(t.Context(), noteID)
	require.NoError(t, err)
	assert.NotNil(t, note)
}
