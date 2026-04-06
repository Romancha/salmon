package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/romancha/salmon/internal/api"
	"github.com/romancha/salmon/internal/models"
	"github.com/romancha/salmon/internal/store"
)

const (
	consumerToken = "test-consumer-token"
	bridgeToken   = "test-bridge-token"
)

var testConsumerTokens = map[string]string{"testapp": consumerToken}

func setupServer(t *testing.T) (*httptest.Server, *store.SQLiteStore) {
	t.Helper()

	s, err := store.NewSQLiteStore(":memory:")
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, s.Close())
	})

	tmpDir := t.TempDir()

	srv := api.NewServer(s, testConsumerTokens, bridgeToken, tmpDir)
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

// --- HealthCheck tests ---

func TestHealthCheck(t *testing.T) {
	ts, _ := setupServer(t)

	resp := doRequest(t, ts, http.MethodGet, "/healthz", nil, "", nil)
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "ok", body["status"])
}

// --- Swagger UI tests ---

func TestSwaggerUI_WithConsumerAuth(t *testing.T) {
	ts, _ := setupServer(t)

	resp := doRequest(t, ts, http.MethodGet, "/api/docs/index.html", nil, consumerToken, nil)
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/html")
}

func TestSwaggerUI_DocJSON(t *testing.T) {
	ts, _ := setupServer(t)

	resp := doRequest(t, ts, http.MethodGet, "/api/docs/doc.json", nil, consumerToken, nil)
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")
}

func TestSwaggerUI_WithoutAuth(t *testing.T) {
	ts, _ := setupServer(t)

	resp := doRequest(t, ts, http.MethodGet, "/api/docs/index.html", nil, "", nil)
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
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

func TestAuth_WrongScope_BridgeOnConsumerRoute(t *testing.T) {
	ts, _ := setupServer(t)

	resp := doRequest(t, ts, http.MethodGet, "/api/notes", nil, bridgeToken, nil)
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestAuth_WrongScope_ConsumerOnBridgeRoute(t *testing.T) {
	ts, _ := setupServer(t)

	resp := doRequest(t, ts, http.MethodGet, "/api/sync/queue", nil, consumerToken, nil)
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestSyncStatus_AccessibleByBothTokens(t *testing.T) {
	ts, _ := setupServer(t)

	// consumer token should access sync/status.
	resp := doRequest(t, ts, http.MethodGet, "/api/sync/status", nil, consumerToken, nil)
	defer resp.Body.Close() //nolint:errcheck // test
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// bridge token should also access sync/status.
	resp2 := doRequest(t, ts, http.MethodGet, "/api/sync/status", nil, bridgeToken, nil)
	defer resp2.Body.Close() //nolint:errcheck // test
	assert.Equal(t, http.StatusOK, resp2.StatusCode)
}

func TestAuth_ValidToken(t *testing.T) {
	ts, _ := setupServer(t)

	resp := doRequest(t, ts, http.MethodGet, "/api/notes", nil, consumerToken, nil)
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestAuth_MultipleConsumers(t *testing.T) {
	s, err := store.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })

	tokens := map[string]string{
		"app1": "token-app1",
		"app2": "token-myapp",
	}
	srv := api.NewServer(s, tokens, bridgeToken, t.TempDir())
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	t.Run("both consumer tokens authenticate on consumer routes", func(t *testing.T) {
		resp1 := doRequest(t, ts, http.MethodGet, "/api/notes", nil, "token-app1", nil)
		defer resp1.Body.Close() //nolint:errcheck // test
		assert.Equal(t, http.StatusOK, resp1.StatusCode)

		resp2 := doRequest(t, ts, http.MethodGet, "/api/notes", nil, "token-myapp", nil)
		defer resp2.Body.Close() //nolint:errcheck // test
		assert.Equal(t, http.StatusOK, resp2.StatusCode)
	})

	t.Run("invalid token rejected", func(t *testing.T) {
		resp := doRequest(t, ts, http.MethodGet, "/api/notes", nil, "wrong-token", nil)
		defer resp.Body.Close() //nolint:errcheck // test
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("bridge token rejected on consumer routes", func(t *testing.T) {
		resp := doRequest(t, ts, http.MethodGet, "/api/notes", nil, bridgeToken, nil)
		defer resp.Body.Close() //nolint:errcheck // test
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("consumer tokens accepted on any-scope routes", func(t *testing.T) {
		resp1 := doRequest(t, ts, http.MethodGet, "/api/sync/status", nil, "token-app1", nil)
		defer resp1.Body.Close() //nolint:errcheck // test
		assert.Equal(t, http.StatusOK, resp1.StatusCode)

		resp2 := doRequest(t, ts, http.MethodGet, "/api/sync/status", nil, "token-myapp", nil)
		defer resp2.Body.Close() //nolint:errcheck // test
		assert.Equal(t, http.StatusOK, resp2.StatusCode)
	})

	t.Run("bridge token accepted on any-scope routes", func(t *testing.T) {
		resp := doRequest(t, ts, http.MethodGet, "/api/sync/status", nil, bridgeToken, nil)
		defer resp.Body.Close() //nolint:errcheck // test
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

func TestAuth_ConsumerIDFromContext_Helper(t *testing.T) {
	// ConsumerIDFromContext returns empty string when no value is set.
	assert.Equal(t, "", api.ConsumerIDFromContext(t.Context()))
}

// --- Notes tests ---

func TestListNotes_Empty(t *testing.T) {
	ts, _ := setupServer(t)

	resp := doRequest(t, ts, http.MethodGet, "/api/notes", nil, consumerToken, nil)

	result := readBodySlice(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Empty(t, result)
}

func TestListNotes_WithData(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-1", Title: "Test Note", Body: "body content",
	}))

	resp := doRequest(t, ts, http.MethodGet, "/api/notes", nil, consumerToken, nil)

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

	resp := doRequest(t, ts, http.MethodGet, "/api/notes?trashed=false", nil, consumerToken, nil)

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

	resp := doRequest(t, ts, http.MethodGet, "/api/notes?limit=2&offset=0", nil, consumerToken, nil)

	result := readBodySlice(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Len(t, result, 2)
}

func TestGetNote(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-1", Title: "Test Note", Body: "full body",
	}))

	resp := doRequest(t, ts, http.MethodGet, "/api/notes/note-1", nil, consumerToken, nil)

	result := readBody(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "Test Note", result["title"])
	assert.Equal(t, "full body", result["body"])
}

func TestGetNote_StripsInternalFields(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-1", Title: "Test Note", Body: "body", SyncStatus: "synced",
		BearRaw: `{"raw":"data"}`, EncryptedData: []byte("secret"), HubModifiedAt: "2025-01-01T00:00:00Z",
	}))

	// Add tag with BearRaw via sync push.
	require.NoError(t, s.CreateTag(t.Context(), &models.Tag{ID: "t1", Title: "work", BearRaw: "tag-raw"}))

	_, err := s.DB().ExecContext(t.Context(), "INSERT INTO note_tags (note_id, tag_id) VALUES (?, ?)", "note-1", "t1")
	require.NoError(t, err)

	resp := doRequest(t, ts, http.MethodGet, "/api/notes/note-1", nil, consumerToken, nil)

	result := readBody(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "Test Note", result["title"])

	// Internal fields must be absent.
	assert.Nil(t, result["bear_raw"], "bear_raw should not be exposed")
	assert.Nil(t, result["encrypted_data"], "encrypted_data should not be exposed")
	assert.Nil(t, result["hub_modified_at"], "hub_modified_at should not be exposed")

	// Tags should also have bear_raw stripped.
	tags, ok := result["tags"].([]any)
	require.True(t, ok)
	require.Len(t, tags, 1)
	tag := tags[0].(map[string]any)
	assert.Nil(t, tag["bear_raw"], "tag bear_raw should not be exposed")

	// sync_status and bear_id should still be present.
	assert.Equal(t, "synced", result["sync_status"])
}

func TestListNotes_StripsInternalFields(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-1", Title: "Note", Body: "body",
		BearRaw: `{"raw":"data"}`, HubModifiedAt: "2025-01-01T00:00:00Z",
	}))

	resp := doRequest(t, ts, http.MethodGet, "/api/notes", nil, consumerToken, nil)

	result := readBodySlice(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.Len(t, result, 1)
	assert.Nil(t, result[0]["bear_raw"], "bear_raw should not be exposed in list")
	assert.Nil(t, result[0]["hub_modified_at"], "hub_modified_at should not be exposed in list")
}

func TestGetNote_NotFound(t *testing.T) {
	ts, _ := setupServer(t)

	resp := doRequest(t, ts, http.MethodGet, "/api/notes/nonexistent", nil, consumerToken, nil)
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

	resp := doRequest(t, ts, http.MethodGet, "/api/notes/search?q=golang", nil, consumerToken, nil)

	result := readBodySlice(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Len(t, result, 1)
	assert.Equal(t, "Golang Tutorial", result[0]["title"])
}

func TestGetNote_WithAttachments(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-1", Title: "Note with file", Body: "body", SyncStatus: "synced",
	}))

	bearID := "bear-att-api"
	require.NoError(t, s.ProcessSyncPush(t.Context(), models.SyncPushRequest{
		Attachments: []models.Attachment{
			{ID: "att-1", BearID: &bearID, NoteID: "note-1", Type: "image", Filename: "photo.jpg", FileSize: 1024, Width: 640, Height: 480},
		},
	}))

	resp := doRequest(t, ts, http.MethodGet, "/api/notes/note-1", nil, consumerToken, nil)

	result := readBody(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	attachments, ok := result["attachments"].([]any)
	require.True(t, ok, "attachments should be an array")
	require.Len(t, attachments, 1)

	att := attachments[0].(map[string]any)
	assert.Equal(t, "att-1", att["id"])
	assert.Equal(t, "image", att["type"])
	assert.Equal(t, "photo.jpg", att["filename"])
	assert.Equal(t, float64(1024), att["file_size"])
	assert.Equal(t, float64(640), att["width"])
	assert.Equal(t, float64(480), att["height"])
	assert.Nil(t, att["bear_raw"], "internal fields should not be exposed")
	assert.Nil(t, att["file_path"], "internal fields should not be exposed")
	assert.Nil(t, att["note_id"], "note_id should not be in attachment info")
}

func TestListNotes_WithAttachments(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-1", Title: "Note", Body: "body", SyncStatus: "synced",
	}))

	bearID := "bear-att-list-api"
	require.NoError(t, s.ProcessSyncPush(t.Context(), models.SyncPushRequest{
		Attachments: []models.Attachment{
			{ID: "att-1", BearID: &bearID, NoteID: "note-1", Type: "file", Filename: "doc.pdf", FileSize: 2048},
		},
	}))

	resp := doRequest(t, ts, http.MethodGet, "/api/notes", nil, consumerToken, nil)

	result := readBodySlice(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.Len(t, result, 1)

	attachments, ok := result[0]["attachments"].([]any)
	require.True(t, ok, "attachments should be present in list response")
	require.Len(t, attachments, 1)

	att := attachments[0].(map[string]any)
	assert.Equal(t, "att-1", att["id"])
	assert.Equal(t, "file", att["type"])
}

func TestSearchNotes_MissingQuery(t *testing.T) {
	ts, _ := setupServer(t)

	resp := doRequest(t, ts, http.MethodGet, "/api/notes/search", nil, consumerToken, nil)
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCreateNote(t *testing.T) {
	ts, _ := setupServer(t)

	body := map[string]string{"title": "New Note", "body": "Content"}

	resp := doRequest(t, ts, http.MethodPost, "/api/notes", body, consumerToken,
		map[string]string{"Idempotency-Key": "key-1"})

	result := readBody(t, resp)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.Equal(t, "New Note", result["title"])
	assert.Equal(t, "pending_to_bear", result["sync_status"])
}

func TestCreateNote_MissingTitle(t *testing.T) {
	ts, _ := setupServer(t)

	body := map[string]string{"body": "Content"}

	resp := doRequest(t, ts, http.MethodPost, "/api/notes", body, consumerToken,
		map[string]string{"Idempotency-Key": "key-1"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCreateNote_MissingIdempotencyKey(t *testing.T) {
	ts, _ := setupServer(t)

	body := map[string]string{"title": "Test", "body": "Content"}

	resp := doRequest(t, ts, http.MethodPost, "/api/notes", body, consumerToken, nil)
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUpdateNote(t *testing.T) {
	ts, s := setupServer(t)

	bearID := "bear-note-abc"
	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-1", Title: "Old Title", Body: "Old body", BearID: &bearID,
	}))

	body := map[string]string{"body": "# New Title\nNew body"}

	resp := doRequest(t, ts, http.MethodPut, "/api/notes/note-1", body, consumerToken,
		map[string]string{"Idempotency-Key": "key-2"})

	result := readBody(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "New Title", result["title"])
	assert.Equal(t, "pending_to_bear", result["sync_status"])
}

func TestUpdateNote_NoBearID_409(t *testing.T) {
	ts, s := setupServer(t)

	// Note without bear_id simulates a note created by a consumer but not yet synced to Bear.
	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-pending", Title: "Pending Note", Body: "body", SyncStatus: "pending_to_bear",
	}))

	body := map[string]string{"body": "Updated body"}
	resp := doRequest(t, ts, http.MethodPut, "/api/notes/note-pending", body, consumerToken,
		map[string]string{"Idempotency-Key": "key-pending-upd"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestUpdateNote_NotFound(t *testing.T) {
	ts, _ := setupServer(t)

	body := map[string]string{"body": "# New Title\nSome body"}

	resp := doRequest(t, ts, http.MethodPut, "/api/notes/nonexistent", body, consumerToken,
		map[string]string{"Idempotency-Key": "key-2"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestUpdateNote_Encrypted403(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "enc-1", Title: "Encrypted", Body: "", Encrypted: 1,
	}))

	body := map[string]string{"body": "# Updated\nSome body"}

	resp := doRequest(t, ts, http.MethodPut, "/api/notes/enc-1", body, consumerToken,
		map[string]string{"Idempotency-Key": "key-3"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestTrashNote(t *testing.T) {
	ts, s := setupServer(t)

	bearID := "bear-trash-abc"
	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-1", Title: "To Trash", BearID: &bearID,
	}))

	resp := doRequest(t, ts, http.MethodDelete, "/api/notes/note-1", nil, consumerToken,
		map[string]string{"Idempotency-Key": "key-4"})

	result := readBody(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, float64(1), result["trashed"])
	assert.Equal(t, "pending_to_bear", result["sync_status"])
}

func TestTrashNote_NoBearID_409(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-pending", Title: "Pending Note", SyncStatus: "pending_to_bear",
	}))

	resp := doRequest(t, ts, http.MethodDelete, "/api/notes/note-pending", nil, consumerToken,
		map[string]string{"Idempotency-Key": "key-trash-no-bear"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestTrashNote_Encrypted403(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "enc-1", Title: "Encrypted", Encrypted: 1,
	}))

	resp := doRequest(t, ts, http.MethodDelete, "/api/notes/enc-1", nil, consumerToken,
		map[string]string{"Idempotency-Key": "key-5"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// --- Archive tests ---

func TestArchiveNote_Success(t *testing.T) {
	ts, s := setupServer(t)

	bearID := "bear-archive-abc"
	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-arch", Title: "To Archive", BearID: &bearID,
	}))

	resp := doRequest(t, ts, http.MethodPost, "/api/notes/note-arch/archive", nil, consumerToken,
		map[string]string{"Idempotency-Key": "key-arch-1"})

	result := readBody(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "pending_to_bear", result["sync_status"])
	assert.Equal(t, float64(1), result["archived"])

	// Verify database was actually updated.
	updated, err := s.GetNote(t.Context(), "note-arch")
	require.NoError(t, err)
	assert.Equal(t, "pending_to_bear", updated.SyncStatus)
	assert.Equal(t, 1, updated.Archived)
	assert.NotEmpty(t, updated.ArchivedAt)
}

func TestArchiveNote_Encrypted403(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "enc-arch", Title: "Encrypted", Encrypted: 1,
	}))

	resp := doRequest(t, ts, http.MethodPost, "/api/notes/enc-arch/archive", nil, consumerToken,
		map[string]string{"Idempotency-Key": "key-arch-enc"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestArchiveNote_NoBearID_409(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-arch-nobear", Title: "Pending Note", SyncStatus: "pending_to_bear",
	}))

	resp := doRequest(t, ts, http.MethodPost, "/api/notes/note-arch-nobear/archive", nil, consumerToken,
		map[string]string{"Idempotency-Key": "key-arch-nobear"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestArchiveNote_ConflictState_409(t *testing.T) {
	ts, s := setupServer(t)

	bearID := "bear-arch-conflict"
	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-arch-conflict", Title: "Conflict Note", BearID: &bearID, SyncStatus: "conflict",
	}))

	resp := doRequest(t, ts, http.MethodPost, "/api/notes/note-arch-conflict/archive", nil, consumerToken,
		map[string]string{"Idempotency-Key": "key-arch-conflict"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestArchiveNote_MissingIdempotencyKey_400(t *testing.T) {
	ts, s := setupServer(t)

	bearID := "bear-arch-nokey"
	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-arch-nokey", Title: "Note", BearID: &bearID,
	}))

	resp := doRequest(t, ts, http.MethodPost, "/api/notes/note-arch-nokey/archive", nil, consumerToken, nil)
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// --- Tags tests ---

func TestListTags_Empty(t *testing.T) {
	ts, _ := setupServer(t)

	resp := doRequest(t, ts, http.MethodGet, "/api/tags", nil, consumerToken, nil)

	result := readBodySlice(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Empty(t, result)
}

func TestListTags_WithData(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateTag(t.Context(), &models.Tag{
		ID: "tag-1", Title: "go/programming",
	}))

	resp := doRequest(t, ts, http.MethodGet, "/api/tags", nil, consumerToken, nil)

	result := readBodySlice(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Len(t, result, 1)
	assert.Equal(t, "go/programming", result[0]["title"])
}

func TestListTags_StripsInternalFields(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateTag(t.Context(), &models.Tag{
		ID: "tag-1", Title: "work", BearRaw: `{"raw":"tag-data"}`,
	}))

	resp := doRequest(t, ts, http.MethodGet, "/api/tags", nil, consumerToken, nil)

	result := readBodySlice(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.Len(t, result, 1)
	assert.Equal(t, "work", result[0]["title"])
	assert.Nil(t, result[0]["bear_raw"], "bear_raw should not be exposed")
}

func TestAddTag(t *testing.T) {
	ts, s := setupServer(t)

	bearID := "bear-tag-abc"
	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-1", Title: "Note", BearID: &bearID,
	}))

	body := map[string]string{"tag": "new-tag"}

	resp := doRequest(t, ts, http.MethodPost, "/api/notes/note-1/tags", body, consumerToken,
		map[string]string{"Idempotency-Key": "key-6"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
}

func TestAddTag_NoteNotFound(t *testing.T) {
	ts, _ := setupServer(t)

	body := map[string]string{"tag": "new-tag"}

	resp := doRequest(t, ts, http.MethodPost, "/api/notes/nonexistent/tags", body, consumerToken,
		map[string]string{"Idempotency-Key": "key-7"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestAddTag_NoBearID_409(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-pending", Title: "Pending Note", SyncStatus: "pending_to_bear",
	}))

	body := map[string]string{"tag": "new-tag"}

	resp := doRequest(t, ts, http.MethodPost, "/api/notes/note-pending/tags", body, consumerToken,
		map[string]string{"Idempotency-Key": "key-tag-no-bear"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestAddTag_Encrypted403(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "enc-1", Title: "Encrypted", Encrypted: 1,
	}))

	body := map[string]string{"tag": "new-tag"}

	resp := doRequest(t, ts, http.MethodPost, "/api/notes/enc-1/tags", body, consumerToken,
		map[string]string{"Idempotency-Key": "key-8"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestAddTag_MissingTag(t *testing.T) {
	ts, s := setupServer(t)

	bearID := "bear-tag-missing-abc"
	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-1", Title: "Note", BearID: &bearID,
	}))

	body := map[string]string{}

	resp := doRequest(t, ts, http.MethodPost, "/api/notes/note-1/tags", body, consumerToken,
		map[string]string{"Idempotency-Key": "key-9"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// --- Rename/Delete Tag tests ---

func TestRenameTag_Success(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateTag(t.Context(), &models.Tag{
		ID: "tag-rename-1", Title: "old/tag",
	}))

	body := map[string]string{"new_name": "new/tag"}

	resp := doRequest(t, ts, http.MethodPut, "/api/tags/tag-rename-1", body, consumerToken,
		map[string]string{"Idempotency-Key": "key-rename-1"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	result := readBody(t, resp)
	assert.Equal(t, "rename_tag", result["action"])
}

func TestRenameTag_NotFound(t *testing.T) {
	ts, _ := setupServer(t)

	body := map[string]string{"new_name": "new/tag"}

	resp := doRequest(t, ts, http.MethodPut, "/api/tags/nonexistent", body, consumerToken,
		map[string]string{"Idempotency-Key": "key-rename-2"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestRenameTag_MissingNewName(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateTag(t.Context(), &models.Tag{
		ID: "tag-rename-3", Title: "some/tag",
	}))

	body := map[string]string{}

	resp := doRequest(t, ts, http.MethodPut, "/api/tags/tag-rename-3", body, consumerToken,
		map[string]string{"Idempotency-Key": "key-rename-3"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRenameTag_MissingIdempotencyKey(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateTag(t.Context(), &models.Tag{
		ID: "tag-rename-4", Title: "another/tag",
	}))

	body := map[string]string{"new_name": "new/tag"}

	resp := doRequest(t, ts, http.MethodPut, "/api/tags/tag-rename-4", body, consumerToken, nil)
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestDeleteTag_Success(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateTag(t.Context(), &models.Tag{
		ID: "tag-del-1", Title: "delete/me",
	}))

	resp := doRequest(t, ts, http.MethodDelete, "/api/tags/tag-del-1", nil, consumerToken,
		map[string]string{"Idempotency-Key": "key-del-1"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	result := readBody(t, resp)
	assert.Equal(t, "delete_tag", result["action"])
}

func TestDeleteTag_NotFound(t *testing.T) {
	ts, _ := setupServer(t)

	resp := doRequest(t, ts, http.MethodDelete, "/api/tags/nonexistent", nil, consumerToken,
		map[string]string{"Idempotency-Key": "key-del-2"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestDeleteTag_MissingIdempotencyKey(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateTag(t.Context(), &models.Tag{
		ID: "tag-del-3", Title: "another/delete",
	}))

	resp := doRequest(t, ts, http.MethodDelete, "/api/tags/tag-del-3", nil, consumerToken, nil)
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRenameTag_DuplicateIdempotencyKey_202(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateTag(t.Context(), &models.Tag{
		ID: "tag-rename-dup", Title: "dup/rename",
	}))

	headers := map[string]string{"Idempotency-Key": "idem-rename-dup"}
	body := map[string]string{"new_name": "dup/renamed"}

	resp1 := doRequest(t, ts, http.MethodPut, "/api/tags/tag-rename-dup", body, consumerToken, headers)
	result1 := readBody(t, resp1)
	assert.Equal(t, http.StatusAccepted, resp1.StatusCode)

	resp2 := doRequest(t, ts, http.MethodPut, "/api/tags/tag-rename-dup", body, consumerToken, headers)
	result2 := readBody(t, resp2)
	assert.Equal(t, http.StatusAccepted, resp2.StatusCode)

	assert.Equal(t, result1["id"], result2["id"])

	items, err := s.LeaseQueueItems(t.Context(), "test", 5*time.Minute)
	require.NoError(t, err)
	assert.Len(t, items, 1)
}

func TestDeleteTag_DuplicateIdempotencyKey_202(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateTag(t.Context(), &models.Tag{
		ID: "tag-del-dup", Title: "dup/delete",
	}))

	headers := map[string]string{"Idempotency-Key": "idem-del-dup"}

	resp1 := doRequest(t, ts, http.MethodDelete, "/api/tags/tag-del-dup", nil, consumerToken, headers)
	result1 := readBody(t, resp1)
	assert.Equal(t, http.StatusAccepted, resp1.StatusCode)

	resp2 := doRequest(t, ts, http.MethodDelete, "/api/tags/tag-del-dup", nil, consumerToken, headers)
	result2 := readBody(t, resp2)
	assert.Equal(t, http.StatusAccepted, resp2.StatusCode)

	assert.Equal(t, result1["id"], result2["id"])

	items, err := s.LeaseQueueItems(t.Context(), "test", 5*time.Minute)
	require.NoError(t, err)
	assert.Len(t, items, 1)
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

	resp := doRequest(t, ts, http.MethodGet, "/api/notes/note-1/backlinks", nil, consumerToken, nil)

	result := readBodySlice(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Len(t, result, 1)
}

func TestListBacklinks_NoteNotFound(t *testing.T) {
	ts, _ := setupServer(t)

	resp := doRequest(t, ts, http.MethodGet, "/api/notes/nonexistent/backlinks", nil, consumerToken, nil)
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// --- Attachments tests ---

func TestListNoteAttachments(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{ID: "note-1", Title: "Note 1"}))

	bearID := "bear-att-1" //nolint:goconst // test data
	err := s.ProcessSyncPush(t.Context(), models.SyncPushRequest{
		Attachments: []models.Attachment{
			{
				ID: "att-1", BearID: &bearID, NoteID: "note-1",
				Type: "image", Filename: "photo.png", NormalizedExtension: "png",
				FileSize: 1024, Width: 800, Height: 600,
			},
		},
	})
	require.NoError(t, err)

	resp := doRequest(t, ts, http.MethodGet, "/api/notes/note-1/attachments", nil, consumerToken, nil)

	result := readBodySlice(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.Len(t, result, 1)
	assert.Equal(t, "image", result[0]["type"])
	assert.Equal(t, "photo.png", result[0]["filename"])
	assert.Equal(t, "png", result[0]["normalized_extension"])
}

func TestListNoteAttachments_NoteNotFound(t *testing.T) {
	ts, _ := setupServer(t)

	resp := doRequest(t, ts, http.MethodGet, "/api/notes/nonexistent/attachments", nil, consumerToken, nil)
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestListNoteAttachments_NoAttachments(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{ID: "note-1", Title: "Note 1"}))

	resp := doRequest(t, ts, http.MethodGet, "/api/notes/note-1/attachments", nil, consumerToken, nil)

	result := readBodySlice(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Empty(t, result)
}

func TestGetAttachment_NotFound(t *testing.T) {
	ts, _ := setupServer(t)

	resp := doRequest(t, ts, http.MethodGet, "/api/attachments/nonexistent", nil, consumerToken, nil)
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

	srv := api.NewServer(s, testConsumerTokens, bridgeToken, tmpDir)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp := doRequest(t, ts, http.MethodGet, "/api/attachments/att-1", nil, consumerToken, nil)
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

	_, err := s.EnqueueWrite(t.Context(), "idem-1", "update", "note-1", `{"body":"new"}`, "")
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

	_, err := s.EnqueueWrite(t.Context(), "idem-1", "create", "note-1", `{"title":"Note"}`, "")
	require.NoError(t, err)

	// Lease the item so it transitions to "processing" (acks only apply to processing items).
	_, err = s.LeaseQueueItems(t.Context(), "bridge-1", 5*time.Minute)
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

// uploadTestAttachment creates a note, pushes an attachment record, and uploads file content via bridge.
// Returns the attachment hub ID and the file content string.
func uploadTestAttachment(
	t *testing.T, ts *httptest.Server, s *store.SQLiteStore, bearAttID, filename, content string,
) string {
	t.Helper()

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{ID: "note-att-" + bearAttID, Title: "Note"}))

	err := s.ProcessSyncPush(t.Context(), models.SyncPushRequest{
		Attachments: []models.Attachment{
			{BearID: &bearAttID, NoteID: "note-att-" + bearAttID, Type: "file", Filename: filename},
		},
	})
	require.NoError(t, err)

	att, err := s.GetAttachmentByBearID(t.Context(), bearAttID)
	require.NoError(t, err)
	require.NotNil(t, att)

	uploadReq, err := http.NewRequest( //nolint:noctx // test
		http.MethodPost,
		ts.URL+"/api/sync/attachments/"+att.ID,
		bytes.NewReader([]byte(content)),
	)
	require.NoError(t, err)
	uploadReq.Header.Set("Authorization", "Bearer "+bridgeToken)

	uploadResp, err := http.DefaultClient.Do(uploadReq) //nolint:noctx,gosec // test
	require.NoError(t, err)
	defer uploadResp.Body.Close() //nolint:errcheck // test
	require.Equal(t, http.StatusOK, uploadResp.StatusCode)

	return att.ID
}

func TestSyncDownloadAttachment_Success(t *testing.T) {
	ts, s := setupServer(t)

	fileContent := "downloaded file content"
	attID := uploadTestAttachment(t, ts, s, "bear-att-dl", "download.txt", fileContent)

	dlResp := doRequest(t, ts, http.MethodGet, "/api/sync/attachments/"+attID, nil, bridgeToken, nil)
	defer dlResp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusOK, dlResp.StatusCode)

	body, err := io.ReadAll(dlResp.Body)
	require.NoError(t, err)
	assert.Equal(t, fileContent, string(body))
}

func TestSyncDownloadAttachment_NotFound(t *testing.T) {
	ts, _ := setupServer(t)

	resp := doRequest(t, ts, http.MethodGet, "/api/sync/attachments/nonexistent", nil, bridgeToken, nil)
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestSyncDownloadAttachment_MissingAuth(t *testing.T) {
	ts, _ := setupServer(t)

	resp := doRequest(t, ts, http.MethodGet, "/api/sync/attachments/some-id", nil, "", nil)
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
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
	assert.Equal(t, float64(0), result["queue_size"])
}

// --- Idempotency tests ---

func TestIdempotency_CreateNoteDuplicate(t *testing.T) {
	ts, s := setupServer(t)

	body := map[string]string{"title": "Note", "body": "Content"}
	headers := map[string]string{"Idempotency-Key": "idem-create-1"}

	resp1 := doRequest(t, ts, http.MethodPost, "/api/notes", body, consumerToken, headers)
	result1 := readBody(t, resp1)
	assert.Equal(t, http.StatusCreated, resp1.StatusCode)

	noteID := result1["id"].(string)

	// Second request with same idempotency key must return the same note (idempotent).
	resp2 := doRequest(t, ts, http.MethodPost, "/api/notes", body, consumerToken, headers)
	result2 := readBody(t, resp2)
	assert.Equal(t, http.StatusCreated, resp2.StatusCode)

	// Must return the same note ID, not create a second note.
	assert.Equal(t, noteID, result2["id"].(string), "idempotent request must return the original note ID")

	// Verify exactly one note exists with that ID.
	note, err := s.GetNote(t.Context(), noteID)
	require.NoError(t, err)
	assert.NotNil(t, note)
}

func TestSyncPush_CleansUpDeletedAttachmentFiles(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{ID: "note-1", Title: "Note"}))

	bearAttID := "bear-att-del"

	// Push an attachment first.
	err := s.ProcessSyncPush(t.Context(), models.SyncPushRequest{
		Attachments: []models.Attachment{
			{BearID: &bearAttID, NoteID: "note-1", Type: "file", Filename: "to-delete.txt"},
		},
	})
	require.NoError(t, err)

	// Upload a file for this attachment.
	att, err := s.GetAttachmentByBearID(t.Context(), bearAttID)
	require.NoError(t, err)
	require.NotNil(t, att)

	fileContent := "file to be deleted"
	req, err := http.NewRequest( //nolint:noctx // test
		http.MethodPost,
		ts.URL+"/api/sync/attachments/"+att.ID,
		bytes.NewReader([]byte(fileContent)),
	)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+bridgeToken)

	resp, err := http.DefaultClient.Do(req) //nolint:noctx,gosec // test
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify file exists on disk.
	att, err = s.GetAttachmentByBearID(t.Context(), bearAttID)
	require.NoError(t, err)
	require.NotEmpty(t, att.FilePath)
	_, statErr := os.Stat(att.FilePath)
	require.NoError(t, statErr, "file should exist before deletion")

	// Push with deleted_attachment_ids.
	deleteReq := models.SyncPushRequest{
		DeletedAttachmentIDs: []string{bearAttID},
	}
	deleteResp := doRequest(t, ts, http.MethodPost, "/api/sync/push", deleteReq, bridgeToken, nil)
	result := readBody(t, deleteResp)
	assert.Equal(t, http.StatusOK, deleteResp.StatusCode)
	assert.Equal(t, "ok", result["status"])

	// Verify file was cleaned up from disk.
	_, statErr = os.Stat(att.FilePath)
	assert.True(t, os.IsNotExist(statErr), "attachment file should be removed from disk")
}

func TestSyncPush_CleansUpPermanentlyDeletedAttachmentFiles(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{ID: "note-1", Title: "Note"}))

	bearAttID := "bear-att-perm"

	// Push attachment.
	err := s.ProcessSyncPush(t.Context(), models.SyncPushRequest{
		Attachments: []models.Attachment{
			{BearID: &bearAttID, NoteID: "note-1", Type: "image", Filename: "photo.jpg"},
		},
	})
	require.NoError(t, err)

	att, err := s.GetAttachmentByBearID(t.Context(), bearAttID)
	require.NoError(t, err)
	require.NotNil(t, att)

	// Upload file.
	fileReq, err := http.NewRequest( //nolint:noctx // test
		http.MethodPost,
		ts.URL+"/api/sync/attachments/"+att.ID,
		bytes.NewReader([]byte("photo data")),
	)
	require.NoError(t, err)
	fileReq.Header.Set("Authorization", "Bearer "+bridgeToken)

	fileResp, err := http.DefaultClient.Do(fileReq) //nolint:noctx,gosec // test
	require.NoError(t, err)
	defer fileResp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusOK, fileResp.StatusCode)

	att, err = s.GetAttachmentByBearID(t.Context(), bearAttID)
	require.NoError(t, err)
	require.NotEmpty(t, att.FilePath)

	// Push with permanently_deleted=1.
	pushReq := models.SyncPushRequest{
		Attachments: []models.Attachment{
			{BearID: &bearAttID, NoteID: "note-1", Type: "image", Filename: "photo.jpg", PermanentlyDeleted: 1},
		},
	}
	pushResp := doRequest(t, ts, http.MethodPost, "/api/sync/push", pushReq, bridgeToken, nil)
	result := readBody(t, pushResp)
	assert.Equal(t, http.StatusOK, pushResp.StatusCode)
	assert.Equal(t, "ok", result["status"])

	// Verify file was cleaned up from disk.
	_, statErr := os.Stat(att.FilePath)
	assert.True(t, os.IsNotExist(statErr), "attachment file should be removed from disk")
}

func TestGetAttachment_ServesFile(t *testing.T) {
	ts, s := setupServer(t)

	fileContent := "readme content"
	attID := uploadTestAttachment(t, ts, s, "bear-att-serve", "readme.txt", fileContent)

	getResp := doRequest(t, ts, http.MethodGet, "/api/attachments/"+attID, nil, consumerToken, nil)
	defer getResp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusOK, getResp.StatusCode)

	body, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)
	assert.Equal(t, fileContent, string(body))
}

// --- AddFile tests ---

// doMultipartUpload creates a multipart form request with a file field and sends it using the consumer token.
func doMultipartUpload(
	t *testing.T, ts *httptest.Server, path, filename string, fileData []byte, headers map[string]string,
) *http.Response {
	t.Helper()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	if filename != "" || fileData != nil {
		fw, err := w.CreateFormFile("file", filename)
		require.NoError(t, err)
		_, err = fw.Write(fileData)
		require.NoError(t, err)
	}

	require.NoError(t, w.Close())

	req, err := http.NewRequest(http.MethodPost, ts.URL+path, &buf) //nolint:noctx // test helper
	require.NoError(t, err)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+consumerToken)

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req) //nolint:noctx,gosec // test helper
	require.NoError(t, err)

	return resp
}

func TestAddFile_Success(t *testing.T) {
	ts, s := setupServer(t)

	bearID := "bear-addfile-1"
	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-af-1", Title: "Note", BearID: &bearID,
	}))

	resp := doMultipartUpload(t, ts, "/api/notes/note-af-1/attachments",
		"photo.jpg", []byte("fake image data"),
		map[string]string{"Idempotency-Key": "idem-af-1"})

	result := readBody(t, resp)
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
	assert.Equal(t, "add_file", result["action"])
	assert.Equal(t, "note-af-1", result["note_id"])

	// Verify queue item was created.
	items, err := s.LeaseQueueItems(t.Context(), "test", 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "add_file", items[0].Action)

	var payload map[string]string
	require.NoError(t, json.Unmarshal([]byte(items[0].Payload), &payload))
	assert.NotEmpty(t, payload["attachment_id"])
	assert.Equal(t, "photo.jpg", payload["filename"])
	assert.Equal(t, "bear-addfile-1", payload["bear_id"])
}

func TestAddFile_EncryptedNote_403(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "enc-af", Title: "Encrypted", Encrypted: 1,
	}))

	resp := doMultipartUpload(t, ts, "/api/notes/enc-af/attachments",
		"doc.pdf", []byte("data"),
		map[string]string{"Idempotency-Key": "idem-af-enc"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestAddFile_NoBearID_409(t *testing.T) {
	ts, s := setupServer(t)

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-af-nobear", Title: "Pending", SyncStatus: "pending_to_bear",
	}))

	resp := doMultipartUpload(t, ts, "/api/notes/note-af-nobear/attachments",
		"image.png", []byte("data"),
		map[string]string{"Idempotency-Key": "idem-af-nobear"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestAddFile_ConflictState_409(t *testing.T) {
	ts, s := setupServer(t)

	bearID := "bear-af-conflict"
	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-af-conflict", Title: "Conflicted", BearID: &bearID, SyncStatus: "conflict",
	}))

	resp := doMultipartUpload(t, ts, "/api/notes/note-af-conflict/attachments",
		"file.txt", []byte("data"),
		map[string]string{"Idempotency-Key": "idem-af-conflict"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestAddFile_MissingIdempotencyKey_400(t *testing.T) {
	ts, s := setupServer(t)

	bearID := "bear-af-noidem"
	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-af-noidem", Title: "Note", BearID: &bearID,
	}))

	resp := doMultipartUpload(t, ts, "/api/notes/note-af-noidem/attachments",
		"readme.md", []byte("data"), nil)
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestAddFile_MissingFile_400(t *testing.T) {
	ts, s := setupServer(t)

	bearID := "bear-af-nofile"
	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-af-nofile", Title: "Note", BearID: &bearID,
	}))

	// Send a request with no file field (empty JSON body instead of multipart).
	resp := doRequest(t, ts, http.MethodPost, "/api/notes/note-af-nofile/attachments",
		map[string]string{}, consumerToken,
		map[string]string{"Idempotency-Key": "idem-af-nofile"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestAddFile_DuplicateIdempotencyKey_202(t *testing.T) {
	ts, s := setupServer(t)

	bearID := "bear-af-dup"
	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-af-dup", Title: "Note", BearID: &bearID,
	}))

	headers := map[string]string{"Idempotency-Key": "idem-af-dup"}

	// First request.
	resp1 := doMultipartUpload(t, ts, "/api/notes/note-af-dup/attachments",
		"photo.jpg", []byte("data"), headers)
	result1 := readBody(t, resp1)
	assert.Equal(t, http.StatusAccepted, resp1.StatusCode)

	// Second request with same idempotency key.
	resp2 := doMultipartUpload(t, ts, "/api/notes/note-af-dup/attachments",
		"photo.jpg", []byte("data"), headers)
	result2 := readBody(t, resp2)
	assert.Equal(t, http.StatusAccepted, resp2.StatusCode)

	// Should return the same queue item.
	assert.Equal(t, result1["id"], result2["id"])

	// Only one queue item should exist.
	items, err := s.LeaseQueueItems(t.Context(), "test", 5*time.Minute)
	require.NoError(t, err)
	assert.Len(t, items, 1)
}

// --- Consumer ID propagation tests ---

func TestCreateNote_QueueItemHasConsumerID(t *testing.T) {
	s, err := store.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })

	tokens := map[string]string{"myapp": "token-myapp"}
	srv := api.NewServer(s, tokens, bridgeToken, t.TempDir())
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	body := map[string]string{"title": "Note", "body": "Content"}
	resp := doRequest(t, ts, http.MethodPost, "/api/notes", body, "token-myapp",
		map[string]string{"Idempotency-Key": "cid-create-1"})
	defer resp.Body.Close() //nolint:errcheck // test
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	items, err := s.LeaseQueueItems(t.Context(), "test", 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "myapp", items[0].ConsumerID)
}

func TestUpdateNote_QueueItemHasConsumerID(t *testing.T) {
	s, err := store.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })

	tokens := map[string]string{"updater": "token-upd"}
	srv := api.NewServer(s, tokens, bridgeToken, t.TempDir())
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	bearID := "bear-upd-cid"
	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-cid-upd", Title: "Old", Body: "Old body", BearID: &bearID,
	}))

	body := map[string]string{"body": "# New\nNew body"}
	resp := doRequest(t, ts, http.MethodPut, "/api/notes/note-cid-upd", body, "token-upd",
		map[string]string{"Idempotency-Key": "cid-update-1"})
	defer resp.Body.Close() //nolint:errcheck // test
	require.Equal(t, http.StatusOK, resp.StatusCode)

	items, err := s.LeaseQueueItems(t.Context(), "test", 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "updater", items[0].ConsumerID)
}

func TestTrashNote_QueueItemHasConsumerID(t *testing.T) {
	s, err := store.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })

	tokens := map[string]string{"external": "token-ext"}
	srv := api.NewServer(s, tokens, bridgeToken, t.TempDir())
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	bearID := "bear-trash-cid"
	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-cid-trash", Title: "To Trash", BearID: &bearID,
	}))

	resp := doRequest(t, ts, http.MethodDelete, "/api/notes/note-cid-trash", nil, "token-ext",
		map[string]string{"Idempotency-Key": "cid-trash-1"})
	defer resp.Body.Close() //nolint:errcheck // test
	require.Equal(t, http.StatusOK, resp.StatusCode)

	items, err := s.LeaseQueueItems(t.Context(), "test", 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "external", items[0].ConsumerID)
}

func TestAddTag_QueueItemHasConsumerID(t *testing.T) {
	s, err := store.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })

	tokens := map[string]string{"tagger": "token-tag"}
	srv := api.NewServer(s, tokens, bridgeToken, t.TempDir())
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	bearID := "bear-tag-cid"
	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-cid-tag", Title: "Note", BearID: &bearID,
	}))

	body := map[string]string{"tag": "new-tag"}
	resp := doRequest(t, ts, http.MethodPost, "/api/notes/note-cid-tag/tags", body, "token-tag",
		map[string]string{"Idempotency-Key": "cid-tag-1"})
	defer resp.Body.Close() //nolint:errcheck // test
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	items, err := s.LeaseQueueItems(t.Context(), "test", 5*time.Minute)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "tagger", items[0].ConsumerID)
}

func TestUpdateNote_PopulatesPendingBearFields(t *testing.T) {
	_, s := setupServer(t)

	bearID := "bear-note-pending"
	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-pb", Title: "Bear Title", Body: "Bear Body", BearID: &bearID,
	}))

	// Simulate consumer update via the handler by doing what the handler does:
	// get note, save pending_bear fields, update title/body, set sync_status.
	note, err := s.GetNote(t.Context(), "note-pb")
	require.NoError(t, err)

	// Save Bear's current values before consumer overwrites.
	note.PendingBearTitle = strPtr(note.Title)
	note.PendingBearBody = strPtr(note.Body)
	note.Title = "Consumer Title"
	note.Body = "Consumer Body"
	note.SyncStatus = "pending_to_bear"

	require.NoError(t, s.UpdateNote(t.Context(), note))

	// Verify pending_bear fields contain the pre-update Bear values.
	updated, err := s.GetNote(t.Context(), "note-pb")
	require.NoError(t, err)
	assert.Equal(t, "Consumer Title", updated.Title)
	assert.Equal(t, "Consumer Body", updated.Body)
	assert.Equal(t, "pending_to_bear", updated.SyncStatus)
	require.NotNil(t, updated.PendingBearTitle)
	require.NotNil(t, updated.PendingBearBody)
	assert.Equal(t, "Bear Title", *updated.PendingBearTitle)
	assert.Equal(t, "Bear Body", *updated.PendingBearBody)
}

func TestUpdateNote_PendingBearFieldsViaHTTP(t *testing.T) {
	ts, s := setupServer(t)

	bearID := "bear-http-pb"
	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-http-pb", Title: "Original Title", Body: "Original Body", BearID: &bearID,
	}))

	body := map[string]string{"body": "# Updated Title\nUpdated Body"}
	resp := doRequest(t, ts, http.MethodPut, "/api/notes/note-http-pb", body, consumerToken,
		map[string]string{"Idempotency-Key": "key-pb-http"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify pending_bear fields were saved with pre-update values.
	note, err := s.GetNote(t.Context(), "note-http-pb")
	require.NoError(t, err)
	assert.Equal(t, "Updated Title", note.Title)
	assert.Equal(t, "# Updated Title\nUpdated Body", note.Body)
	require.NotNil(t, note.PendingBearTitle, "pending_bear_title should be set after consumer update")
	require.NotNil(t, note.PendingBearBody, "pending_bear_body should be set after consumer update")
	assert.Equal(t, "Original Title", *note.PendingBearTitle)
	assert.Equal(t, "Original Body", *note.PendingBearBody)
}

func TestCreateNote_PendingBearFieldsEmpty(t *testing.T) {
	_, s := setupServer(t)

	note := &models.Note{
		ID:         "note-create-pb",
		Title:      "New Note",
		Body:       "New Body",
		SyncStatus: "pending_to_bear",
	}

	require.NoError(t, s.CreateNote(t.Context(), note))

	created, err := s.GetNote(t.Context(), "note-create-pb")
	require.NoError(t, err)
	assert.Nil(t, created.PendingBearTitle, "pending_bear_title should be nil for new notes")
	assert.Nil(t, created.PendingBearBody, "pending_bear_body should be nil for new notes")
}

func TestSyncStatus_WithConflicts(t *testing.T) {
	ts, s := setupServer(t)

	bearID := "bear-conflict-1"
	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "n1", BearID: &bearID, Title: "Conflicted", SyncStatus: "conflict",
		ModifiedAt: "2025-01-01T12:00:00Z",
	}))

	resp := doRequest(t, ts, http.MethodGet, "/api/sync/status", nil, bridgeToken, nil)

	result := readBody(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, float64(1), result["conflict_count"])

	conflictIDs, ok := result["conflict_note_ids"].([]any)
	require.True(t, ok)
	require.Len(t, conflictIDs, 1)
	assert.Equal(t, "n1", conflictIDs[0])
}

func TestTrashNote_SetsPendingBearFields(t *testing.T) {
	ts, s := setupServer(t)

	bearID := "bear-trash-pb"
	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-trash-pb", Title: "My Title", Body: "My Body", BearID: &bearID,
	}))

	resp := doRequest(t, ts, http.MethodDelete, "/api/notes/note-trash-pb", nil, consumerToken,
		map[string]string{"Idempotency-Key": "key-trash-pb"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	note, err := s.GetNote(t.Context(), "note-trash-pb")
	require.NoError(t, err)
	assert.Equal(t, "pending_to_bear", note.SyncStatus)
	require.NotNil(t, note.PendingBearTitle, "pending_bear_title should be set after trash")
	require.NotNil(t, note.PendingBearBody, "pending_bear_body should be set after trash")
	assert.Equal(t, "My Title", *note.PendingBearTitle)
	assert.Equal(t, "My Body", *note.PendingBearBody)
	// Title/body should be unchanged (trash doesn't modify content).
	assert.Equal(t, "My Title", note.Title)
	assert.Equal(t, "My Body", note.Body)
}

func TestArchiveNote_SetsPendingBearFields(t *testing.T) {
	ts, s := setupServer(t)

	bearID := "bear-arch-pb"
	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-arch-pb", Title: "Arch Title", Body: "Arch Body", BearID: &bearID,
	}))

	resp := doRequest(t, ts, http.MethodPost, "/api/notes/note-arch-pb/archive", nil, consumerToken,
		map[string]string{"Idempotency-Key": "key-arch-pb"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	note, err := s.GetNote(t.Context(), "note-arch-pb")
	require.NoError(t, err)
	assert.Equal(t, "pending_to_bear", note.SyncStatus)
	require.NotNil(t, note.PendingBearTitle, "pending_bear_title should be set after archive")
	require.NotNil(t, note.PendingBearBody, "pending_bear_body should be set after archive")
	assert.Equal(t, "Arch Title", *note.PendingBearTitle)
	assert.Equal(t, "Arch Body", *note.PendingBearBody)
}

func TestUpdateNote_ConsecutiveUpdatePreservesSnapshot(t *testing.T) {
	// A second consumer update on an already pending_to_bear note must preserve
	// the original Bear snapshot, not overwrite it with the consumer's values.
	ts, s := setupServer(t)

	bearID := "bear-consec"
	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-consec", Title: "Bear Original", Body: "Bear Original Body", BearID: &bearID,
	}))

	// First consumer update: sets pending_bear fields to Bear's original values.
	resp := doRequest(t, ts, http.MethodPut, "/api/notes/note-consec",
		map[string]string{"body": "# Consumer V1\nConsumer Body V1"}, consumerToken,
		map[string]string{"Idempotency-Key": "key-consec-1"})
	defer resp.Body.Close() //nolint:errcheck // test
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	note, err := s.GetNote(t.Context(), "note-consec")
	require.NoError(t, err)
	assert.Equal(t, "Consumer V1", note.Title)
	require.NotNil(t, note.PendingBearTitle)
	assert.Equal(t, "Bear Original", *note.PendingBearTitle)
	assert.Equal(t, "Bear Original Body", *note.PendingBearBody)

	// Second consumer update: pending_bear fields must still point to Bear's original values.
	resp2 := doRequest(t, ts, http.MethodPut, "/api/notes/note-consec",
		map[string]string{"body": "# Consumer V2\nConsumer Body V2"}, consumerToken,
		map[string]string{"Idempotency-Key": "key-consec-2"})
	defer resp2.Body.Close() //nolint:errcheck // test
	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	note2, err := s.GetNote(t.Context(), "note-consec")
	require.NoError(t, err)
	assert.Equal(t, "Consumer V2", note2.Title)
	require.NotNil(t, note2.PendingBearTitle, "pending_bear_title must survive consecutive update")
	assert.Equal(t, "Bear Original", *note2.PendingBearTitle, "snapshot must be Bear's original, not consumer V1")
	assert.Equal(t, "Bear Original Body", *note2.PendingBearBody, "snapshot must be Bear's original body")
}

func TestTrashNote_NoConflictOnBearBodyChange(t *testing.T) {
	// Trash doesn't modify title/body, so even if Bear changes body,
	// field-level conflict detection should NOT fire (no field intersection).
	_, s := setupServer(t)
	ctx := t.Context()

	bearID := "bear-trash-noconflict"
	require.NoError(t, s.CreateNote(ctx, &models.Note{
		ID:               "n1",
		BearID:           &bearID,
		Title:            "Original Title",
		Body:             "Original Body",
		SyncStatus:       "pending_to_bear",
		ModifiedAt:       "2025-01-01T10:00:00Z",
		PendingBearTitle: strPtr("Original Title"),
		PendingBearBody:  strPtr("Original Body"),
		Trashed:          1,
	}))

	// Bear pushes with changed body and different modified_at.
	req := models.SyncPushRequest{
		Notes: []models.Note{
			{
				BearID:     &bearID,
				Title:      "Original Title",
				Body:       "Bear Changed Body",
				ModifiedAt: "2025-01-01T11:00:00Z",
				SyncStatus: "synced",
			},
		},
	}
	require.NoError(t, s.ProcessSyncPush(ctx, req))

	got, err := s.GetNote(ctx, "n1")
	require.NoError(t, err)
	// No conflict because consumer (trash) didn't change title or body.
	// hub title/body == pending_bear title/body → no consumer content change.
	assert.Equal(t, "pending_to_bear", got.SyncStatus, "trash action should not conflict on Bear body change")
}

func TestUpdateNote_CreateUpdateCoalescing(t *testing.T) {
	ts, s := setupServer(t)
	ctx := t.Context()

	// Step 1: Create a note via the API (this creates a pending "create" queue item).
	createBody := map[string]any{"title": "Original Title", "body": "Original body", "tags": []string{"tag1", "tag2"}}
	createResp := doRequest(t, ts, http.MethodPost, "/api/notes", createBody, consumerToken,
		map[string]string{"Idempotency-Key": "create-key-1"})
	createResult := readBody(t, createResp)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)
	noteID := createResult["id"].(string)

	// Step 2: Update the note before it syncs to Bear (BearID is nil).
	updateBody := map[string]string{"body": "# Updated Title\nUpdated body"}
	updateResp := doRequest(t, ts, http.MethodPut, "/api/notes/"+noteID, updateBody, consumerToken,
		map[string]string{"Idempotency-Key": "update-key-1"})
	updateResult := readBody(t, updateResp)
	assert.Equal(t, http.StatusOK, updateResp.StatusCode)
	assert.Equal(t, "Updated Title", updateResult["title"])
	assert.Equal(t, "# Updated Title\nUpdated body", updateResult["body"])

	// Verify only one queue item exists (the create item was coalesced).
	pendingCount, err := s.PendingQueueCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, pendingCount, "should have exactly one queue item (coalesced)")

	// Verify the create queue item's payload was updated with new body.
	item, err := s.GetQueueItemByIdempotencyKey(ctx, "create-key-1", "testapp")
	require.NoError(t, err)
	require.NotNil(t, item)
	assert.Equal(t, "create", item.Action, "coalesced item should remain a create action")
	assert.Contains(t, item.Payload, `"body":"# Updated Title\nUpdated body"`)
}

func TestUpdateNote_CreateUpdateCoalescing_PreservesTags(t *testing.T) {
	ts, s := setupServer(t)
	ctx := t.Context()

	// Create note with tags.
	createBody := map[string]any{"title": "Tagged Note", "body": "body", "tags": []string{"work", "important"}}
	createResp := doRequest(t, ts, http.MethodPost, "/api/notes", createBody, consumerToken,
		map[string]string{"Idempotency-Key": "create-tagged"})
	createResult := readBody(t, createResp)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)
	noteID := createResult["id"].(string)

	// Update body only.
	updateBody := map[string]string{"body": "New body content"}
	updateResp := doRequest(t, ts, http.MethodPut, "/api/notes/"+noteID, updateBody, consumerToken,
		map[string]string{"Idempotency-Key": "update-tagged"})
	defer updateResp.Body.Close() //nolint:errcheck // test
	assert.Equal(t, http.StatusOK, updateResp.StatusCode)

	// Verify tags are preserved in the coalesced payload.
	item, err := s.GetQueueItemByIdempotencyKey(ctx, "create-tagged", "testapp")
	require.NoError(t, err)
	require.NotNil(t, item)
	assert.Contains(t, item.Payload, `"tags"`)
	assert.Contains(t, item.Payload, `"work"`)
	assert.Contains(t, item.Payload, `"important"`)
	assert.Contains(t, item.Payload, `"body":"New body content"`)
}

func TestUpdateNote_NoBearID_NoPendingCreate_409(t *testing.T) {
	ts, s := setupServer(t)

	// Create a note directly in DB without bear_id and without a pending create queue item.
	require.NoError(t, s.CreateNote(t.Context(), &models.Note{
		ID: "note-orphan", Title: "Orphan", Body: "body", SyncStatus: "pending_to_bear",
	}))

	body := map[string]string{"body": "Updated body"}
	resp := doRequest(t, ts, http.MethodPut, "/api/notes/note-orphan", body, consumerToken,
		map[string]string{"Idempotency-Key": "key-orphan-upd"})
	defer resp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func strPtr(s string) *string { return &s }
