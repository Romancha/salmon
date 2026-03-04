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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/romancha/bear-sync/internal/api"
	"github.com/romancha/bear-sync/internal/models"
	"github.com/romancha/bear-sync/internal/store"
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

	body := map[string]string{"title": "New Title", "body": "New body"}

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

	body := map[string]string{"title": "New Title", "body": "Some body"}

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

	body := map[string]string{"title": "Updated", "body": "Some body"}

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

	require.NoError(t, s.CreateNote(t.Context(), &models.Note{ID: "note-1", Title: "Note"}))

	bearAttID := "bear-att-serve"

	err := s.ProcessSyncPush(t.Context(), models.SyncPushRequest{
		Attachments: []models.Attachment{
			{BearID: &bearAttID, NoteID: "note-1", Type: "file", Filename: "readme.txt"},
		},
	})
	require.NoError(t, err)

	att, err := s.GetAttachmentByBearID(t.Context(), bearAttID)
	require.NoError(t, err)
	require.NotNil(t, att)

	// Upload file.
	fileContent := "readme content"
	uploadReq, err := http.NewRequest( //nolint:noctx // test
		http.MethodPost,
		ts.URL+"/api/sync/attachments/"+att.ID,
		bytes.NewReader([]byte(fileContent)),
	)
	require.NoError(t, err)
	uploadReq.Header.Set("Authorization", "Bearer "+bridgeToken)

	uploadResp, err := http.DefaultClient.Do(uploadReq) //nolint:noctx,gosec // test
	require.NoError(t, err)
	defer uploadResp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusOK, uploadResp.StatusCode)

	// Now GET the attachment via consumer API.
	getResp := doRequest(t, ts, http.MethodGet, "/api/attachments/"+att.ID, nil, consumerToken, nil)
	defer getResp.Body.Close() //nolint:errcheck // test

	assert.Equal(t, http.StatusOK, getResp.StatusCode)

	body, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)
	assert.Equal(t, fileContent, string(body))
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

	body := map[string]string{"title": "New", "body": "New body"}
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
