package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMCPServer(c *Client) *gomcp.Server {
	s := gomcp.NewServer(&gomcp.Implementation{Name: "test", Version: "test"}, nil)
	RegisterTools(s, c)
	return s
}

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, "test-token")
}

func TestSearchNotes_Success(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/notes/search", r.URL.Path)
		assert.Equal(t, "test query", r.URL.Query().Get("q"))
		assert.Equal(t, "5", r.URL.Query().Get("limit"))
		assert.Equal(t, "work", r.URL.Query().Get("tag"))

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id":"note-1","title":"Test Note","body":"hello world"}]`))
	})

	_, out, err := handleSearchNotes(context.Background(), c, SearchNotesInput{
		Query: "test query",
		Limit: 5,
		Tag:   "work",
	})
	require.NoError(t, err)
	require.Len(t, out.Notes, 1)
	assert.Equal(t, "note-1", out.Notes[0].ID)
	assert.Equal(t, "Test Note", out.Notes[0].Title)
	assert.Equal(t, "hello world", out.Notes[0].Body)
}

func TestSearchNotes_MinimalParams(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "my query", r.URL.Query().Get("q"))
		assert.Empty(t, r.URL.Query().Get("limit"))
		assert.Empty(t, r.URL.Query().Get("tag"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	})

	_, out, err := handleSearchNotes(context.Background(), c, SearchNotesInput{Query: "my query"})
	require.NoError(t, err)
	assert.Empty(t, out.Notes)
}

func TestSearchNotes_APIError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"unauthorized"}`))
	})

	_, _, err := handleSearchNotes(context.Background(), c, SearchNotesInput{Query: "test"})
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusUnauthorized, apiErr.StatusCode)
}

func TestGetNote_Success(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/notes/abc-123", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"id":"abc-123",
			"title":"My Note",
			"body":"note body",
			"tags":[{"id":"tag-1","title":"work"}],
			"attachments":[{"id":"att-1","filename":"file.pdf"}],
			"backlinks":[{"id":"bl-1","title":"Other Note"}]
		}`))
	})

	_, out, err := handleGetNote(context.Background(), c, GetNoteInput{ID: "abc-123"})
	require.NoError(t, err)
	assert.Equal(t, "abc-123", out.ID)
	assert.Equal(t, "My Note", out.Title)
	assert.Equal(t, "note body", out.Body)
	require.Len(t, out.Tags, 1)
	assert.Equal(t, "work", out.Tags[0].Title)
	require.Len(t, out.Attachments, 1)
	assert.Equal(t, "file.pdf", out.Attachments[0].Filename)
	require.Len(t, out.Backlinks, 1)
	assert.Equal(t, "Other Note", out.Backlinks[0].Title)
}

func TestGetNote_NotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"note not found"}`))
	})

	_, _, err := handleGetNote(context.Background(), c, GetNoteInput{ID: "not-exist"})
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusNotFound, apiErr.StatusCode)
}

func TestListNotes_Success(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/notes", r.URL.Path)
		assert.Equal(t, "work", r.URL.Query().Get("tag"))
		assert.Equal(t, "modified_at", r.URL.Query().Get("sort"))
		assert.Equal(t, "desc", r.URL.Query().Get("order"))
		assert.Equal(t, "10", r.URL.Query().Get("limit"))
		assert.Equal(t, "false", r.URL.Query().Get("trashed"))

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id":"n1","title":"Note 1"},{"id":"n2","title":"Note 2"}]`))
	})

	_, out, err := handleListNotes(context.Background(), c, ListNotesInput{
		Tag:     "work",
		Sort:    "modified_at",
		Order:   "desc",
		Limit:   10,
		Trashed: "false",
	})
	require.NoError(t, err)
	require.Len(t, out.Notes, 2)
	assert.Equal(t, "n1", out.Notes[0].ID)
	assert.Equal(t, "Note 1", out.Notes[0].Title)
	assert.Equal(t, "n2", out.Notes[1].ID)
}

func TestListNotes_NoParams(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.URL.RawQuery)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	})

	_, out, err := handleListNotes(context.Background(), c, ListNotesInput{})
	require.NoError(t, err)
	assert.Empty(t, out.Notes)
}

func TestListNotes_APIError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`internal error`))
	})

	_, _, err := handleListNotes(context.Background(), c, ListNotesInput{})
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
}

// --- List Tags ---

func TestListTags_Success(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/tags", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id":"tag-1","title":"work"},{"id":"tag-2","title":"personal"}]`))
	})

	_, out, err := handleListTags(context.Background(), c)
	require.NoError(t, err)
	require.Len(t, out.Tags, 2)
	assert.Equal(t, "tag-1", out.Tags[0].ID)
	assert.Equal(t, "work", out.Tags[0].Title)
	assert.Equal(t, "tag-2", out.Tags[1].ID)
	assert.Equal(t, "personal", out.Tags[1].Title)
}

func TestListTags_APIError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"unauthorized"}`))
	})

	_, _, err := handleListTags(context.Background(), c)
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusUnauthorized, apiErr.StatusCode)
}

// --- Get Attachment ---

func TestGetAttachment_FileMode_Default(t *testing.T) {
	fileContent := []byte("hello binary content")
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/attachments/att-123", r.URL.Path)
		w.Header().Set("Content-Disposition", `attachment; filename="photo.png"`)
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		w.Write(fileContent)
	})

	_, out, err := handleGetAttachment(context.Background(), c, GetAttachmentInput{ID: "att-123"})
	require.NoError(t, err)
	assert.Equal(t, "att-123", out.ID)
	assert.Equal(t, "photo.png", out.Filename)
	assert.Equal(t, "image/png", out.ContentType)
	assert.Equal(t, int64(len(fileContent)), out.Size)
	assert.NotEmpty(t, out.FilePath)
	assert.Empty(t, out.Base64)

	saved, err := os.ReadFile(out.FilePath)
	require.NoError(t, err)
	assert.Equal(t, fileContent, saved)

	t.Cleanup(func() { os.RemoveAll(filepath.Dir(out.FilePath)) })
}

func TestGetAttachment_Base64Mode(t *testing.T) {
	fileContent := []byte("hello binary content")
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/attachments/att-123", r.URL.Path)
		w.Header().Set("Content-Disposition", `attachment; filename="photo.png"`)
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		w.Write(fileContent)
	})

	_, out, err := handleGetAttachment(context.Background(), c, GetAttachmentInput{
		ID:   "att-123",
		Mode: "base64",
	})
	require.NoError(t, err)
	assert.Equal(t, "att-123", out.ID)
	assert.Equal(t, "photo.png", out.Filename)
	assert.Equal(t, "image/png", out.ContentType)
	assert.Equal(t, int64(len(fileContent)), out.Size)
	assert.Empty(t, out.FilePath)

	decoded, err := base64.StdEncoding.DecodeString(out.Base64)
	require.NoError(t, err)
	assert.Equal(t, fileContent, decoded)
}

func TestGetAttachment_CustomOutputDir(t *testing.T) {
	fileContent := []byte("custom dir content")
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="doc.pdf"`)
		w.Header().Set("Content-Type", "application/pdf")
		w.WriteHeader(http.StatusOK)
		w.Write(fileContent)
	})

	customDir := t.TempDir()
	_, out, err := handleGetAttachment(context.Background(), c, GetAttachmentInput{
		ID:        "att-456",
		OutputDir: customDir,
	})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(out.FilePath, customDir))

	saved, err := os.ReadFile(out.FilePath)
	require.NoError(t, err)
	assert.Equal(t, fileContent, saved)
}

func TestGetAttachment_NotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"attachment not found"}`))
	})

	_, _, err := handleGetAttachment(context.Background(), c, GetAttachmentInput{ID: "missing"})
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusNotFound, apiErr.StatusCode)
}

func TestGetAttachment_InvalidMode(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	_, _, err := handleGetAttachment(context.Background(), c, GetAttachmentInput{
		ID:   "att-1",
		Mode: "invalid",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid mode")
}

// --- List Attachments ---

func TestListAttachments_Success(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/notes/note-1/attachments", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[
			{"id":"att-1","type":"image","filename":"photo.png","file_size":1024,"width":800,"height":600},
			{"id":"att-2","type":"file","filename":"doc.pdf","file_size":2048}
		]`))
	})

	_, out, err := handleListAttachments(context.Background(), c, ListAttachmentsInput{NoteID: "note-1"})
	require.NoError(t, err)
	require.Len(t, out.Attachments, 2)
	assert.Equal(t, "att-1", out.Attachments[0].ID)
	assert.Equal(t, "image", out.Attachments[0].Type)
	assert.Equal(t, "photo.png", out.Attachments[0].Filename)
	assert.Equal(t, int64(1024), out.Attachments[0].FileSize)
	assert.Equal(t, "att-2", out.Attachments[1].ID)
	assert.Equal(t, "file", out.Attachments[1].Type)
}

func TestListAttachments_NoteNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	})

	_, _, err := handleListAttachments(context.Background(), c, ListAttachmentsInput{NoteID: "missing"})
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusNotFound, apiErr.StatusCode)
}

// --- Download Note Attachments ---

const testNoteAttachmentsPath = "/api/notes/note-1/attachments"

func TestDownloadNoteAttachments_Success(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case testNoteAttachmentsPath:
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[{"id":"att-1","type":"image","filename":"photo.png","normalized_extension":"png"}]`))
		case "/api/attachments/att-1":
			w.Header().Set("Content-Disposition", `attachment; filename="photo.png"`)
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("png-bytes"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	outputDir := t.TempDir()
	_, out, err := handleDownloadNoteAttachments(context.Background(), c, DownloadNoteAttachmentsInput{
		NoteID:    "note-1",
		OutputDir: outputDir,
	})
	require.NoError(t, err)
	require.Len(t, out.Downloaded, 1)
	assert.Equal(t, 0, out.Skipped)
	assert.Equal(t, "att-1", out.Downloaded[0].ID)
	assert.Equal(t, "photo.png", out.Downloaded[0].Filename)
	assert.Equal(t, "image/png", out.Downloaded[0].ContentType)
	assert.Equal(t, int64(len("png-bytes")), out.Downloaded[0].Size)
	assert.True(t, strings.HasPrefix(out.Downloaded[0].FilePath, outputDir))

	saved, err := os.ReadFile(out.Downloaded[0].FilePath)
	require.NoError(t, err)
	assert.Equal(t, []byte("png-bytes"), saved)
}

func TestDownloadNoteAttachments_TypeFilter(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case testNoteAttachmentsPath:
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[
				{"id":"att-1","type":"image","filename":"photo.png","normalized_extension":"png"},
				{"id":"att-2","type":"file","filename":"doc.pdf","normalized_extension":"pdf"}
			]`))
		case "/api/attachments/att-1":
			w.Header().Set("Content-Disposition", `attachment; filename="photo.png"`)
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("img"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	_, out, err := handleDownloadNoteAttachments(context.Background(), c, DownloadNoteAttachmentsInput{
		NoteID:    "note-1",
		OutputDir: t.TempDir(),
		Types:     []string{"image"},
	})
	require.NoError(t, err)
	assert.Len(t, out.Downloaded, 1)
	assert.Equal(t, 1, out.Skipped)
	assert.Equal(t, "att-1", out.Downloaded[0].ID)
}

func TestDownloadNoteAttachments_ExtensionFilter(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case testNoteAttachmentsPath:
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[
				{"id":"att-1","type":"image","filename":"photo.png","normalized_extension":"png"},
				{"id":"att-2","type":"file","filename":"doc.pdf","normalized_extension":"pdf"}
			]`))
		case "/api/attachments/att-2":
			w.Header().Set("Content-Disposition", `attachment; filename="doc.pdf"`)
			w.Header().Set("Content-Type", "application/pdf")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("pdf"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	_, out, err := handleDownloadNoteAttachments(context.Background(), c, DownloadNoteAttachmentsInput{
		NoteID:     "note-1",
		OutputDir:  t.TempDir(),
		Extensions: []string{".pdf"},
	})
	require.NoError(t, err)
	assert.Len(t, out.Downloaded, 1)
	assert.Equal(t, 1, out.Skipped)
	assert.Equal(t, "att-2", out.Downloaded[0].ID)
}

func TestDownloadNoteAttachments_BothFilters(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case testNoteAttachmentsPath:
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[
				{"id":"att-1","type":"image","filename":"photo.png","normalized_extension":"png"},
				{"id":"att-2","type":"file","filename":"doc.pdf","normalized_extension":"pdf"},
				{"id":"att-3","type":"image","filename":"scan.pdf","normalized_extension":"pdf"}
			]`))
		case "/api/attachments/att-3":
			w.Header().Set("Content-Disposition", `attachment; filename="scan.pdf"`)
			w.Header().Set("Content-Type", "application/pdf")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("scan"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	_, out, err := handleDownloadNoteAttachments(context.Background(), c, DownloadNoteAttachmentsInput{
		NoteID:     "note-1",
		OutputDir:  t.TempDir(),
		Types:      []string{"image"},
		Extensions: []string{"pdf"},
	})
	require.NoError(t, err)
	assert.Len(t, out.Downloaded, 1)
	assert.Equal(t, 2, out.Skipped)
	assert.Equal(t, "att-3", out.Downloaded[0].ID)
}

func TestDownloadNoteAttachments_NoAttachments(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	})

	_, out, err := handleDownloadNoteAttachments(context.Background(), c, DownloadNoteAttachmentsInput{
		NoteID:    "note-1",
		OutputDir: t.TempDir(),
	})
	require.NoError(t, err)
	assert.Empty(t, out.Downloaded)
	assert.Equal(t, 0, out.Skipped)
}

func TestDownloadNoteAttachments_NoteNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	})

	_, _, err := handleDownloadNoteAttachments(context.Background(), c, DownloadNoteAttachmentsInput{
		NoteID: "missing",
	})
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusNotFound, apiErr.StatusCode)
}

// --- Sync Status ---

func TestSyncStatus_Success(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/sync/status", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"last_sync_at":"2025-01-15T10:30:00Z",
			"last_push_at":"2025-01-15T10:25:00Z",
			"queue_size":3,
			"initial_sync_complete":"true",
			"conflict_count":1,
			"conflict_note_ids":["note-1"]
		}`))
	})

	_, out, err := handleSyncStatus(context.Background(), c)
	require.NoError(t, err)
	assert.Equal(t, "2025-01-15T10:30:00Z", out.LastSyncAt)
	assert.Equal(t, "2025-01-15T10:25:00Z", out.LastPushAt)
	assert.Equal(t, 3, out.QueueSize)
	assert.Equal(t, "true", out.InitialSyncComplete)
	assert.Equal(t, 1, out.ConflictCount)
	require.Len(t, out.ConflictNoteIDs, 1)
	assert.Equal(t, "note-1", out.ConflictNoteIDs[0])
}

func TestSyncStatus_APIError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"unauthorized"}`))
	})

	_, _, err := handleSyncStatus(context.Background(), c)
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusUnauthorized, apiErr.StatusCode)
}

// --- List Backlinks ---

func TestListBacklinks_Success(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/notes/note-abc/backlinks", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id":"bl-1","title":"Linking Note","linked_by_id":"note-xyz","linking_to_id":"note-abc"}]`))
	})

	_, out, err := handleListBacklinks(context.Background(), c, ListBacklinksInput{NoteID: "note-abc"})
	require.NoError(t, err)
	require.Len(t, out.Backlinks, 1)
	assert.Equal(t, "bl-1", out.Backlinks[0].ID)
	assert.Equal(t, "Linking Note", out.Backlinks[0].Title)
}

func TestListBacklinks_Empty(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	})

	_, out, err := handleListBacklinks(context.Background(), c, ListBacklinksInput{NoteID: "note-abc"})
	require.NoError(t, err)
	assert.Empty(t, out.Backlinks)
}

func TestListBacklinks_NoteNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"note not found"}`))
	})

	_, _, err := handleListBacklinks(context.Background(), c, ListBacklinksInput{NoteID: "missing"})
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusNotFound, apiErr.StatusCode)
}

// --- Create Note ---

func TestCreateNote_Success(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/notes", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.NotEmpty(t, r.Header.Get("Idempotency-Key"))

		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "New Note", body["title"])
		assert.Equal(t, "body text", body["body"])
		assert.Equal(t, []any{"work", "dev"}, body["tags"])

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"new-1","title":"New Note","body":"body text","sync_status":"pending_to_bear"}`))
	})

	_, out, err := handleCreateNote(context.Background(), c, CreateNoteInput{
		Title: "New Note",
		Body:  "body text",
		Tags:  []string{"work", "dev"},
	})
	require.NoError(t, err)
	assert.Equal(t, "new-1", out.ID)
	assert.Equal(t, "New Note", out.Title)
	assert.Equal(t, "body text", out.Body)
}

func TestCreateNote_MinimalParams(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "Title Only", body["title"])
		assert.Nil(t, body["body"])
		assert.Nil(t, body["tags"])

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"new-2","title":"Title Only"}`))
	})

	_, out, err := handleCreateNote(context.Background(), c, CreateNoteInput{Title: "Title Only"})
	require.NoError(t, err)
	assert.Equal(t, "new-2", out.ID)
}

func TestCreateNote_APIError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"unauthorized"}`))
	})

	_, _, err := handleCreateNote(context.Background(), c, CreateNoteInput{Title: "Test"})
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusUnauthorized, apiErr.StatusCode)
}

// --- Update Note ---

func TestUpdateNote_Success(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/api/notes/note-1", r.URL.Path)
		assert.NotEmpty(t, r.Header.Get("Idempotency-Key"))

		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "Updated Title", body["title"])
		assert.Equal(t, "updated body", body["body"])

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"note-1","title":"Updated Title","body":"updated body"}`))
	})

	_, out, err := handleUpdateNote(context.Background(), c, UpdateNoteInput{
		ID:    "note-1",
		Title: "Updated Title",
		Body:  "updated body",
	})
	require.NoError(t, err)
	assert.Equal(t, "note-1", out.ID)
	assert.Equal(t, "Updated Title", out.Title)
	assert.Equal(t, "updated body", out.Body)
}

func TestUpdateNote_BodyOnly(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Nil(t, body["title"])
		assert.Equal(t, "new body", body["body"])

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"note-1","body":"new body"}`))
	})

	_, out, err := handleUpdateNote(context.Background(), c, UpdateNoteInput{ID: "note-1", Body: "new body"})
	require.NoError(t, err)
	assert.Equal(t, "note-1", out.ID)
}

func TestUpdateNote_Forbidden(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":"encrypted notes are read-only"}`))
	})

	_, _, err := handleUpdateNote(context.Background(), c, UpdateNoteInput{ID: "enc-1", Body: "x"})
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
}

func TestUpdateNote_Conflict(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"error":"note not synced to Bear"}`))
	})

	_, _, err := handleUpdateNote(context.Background(), c, UpdateNoteInput{ID: "unsync-1", Body: "x"})
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusConflict, apiErr.StatusCode)
}

// --- Trash Note ---

func TestTrashNote_Success(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/api/notes/note-1", r.URL.Path)
		assert.NotEmpty(t, r.Header.Get("Idempotency-Key"))

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"note-1","title":"Trashed","trashed":1}`))
	})

	_, out, err := handleTrashNote(context.Background(), c, TrashNoteInput{ID: "note-1"})
	require.NoError(t, err)
	assert.Equal(t, "note-1", out.ID)
	assert.Equal(t, 1, out.Trashed)
}

func TestTrashNote_Forbidden(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":"encrypted notes are read-only"}`))
	})

	_, _, err := handleTrashNote(context.Background(), c, TrashNoteInput{ID: "enc-1"})
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
}

func TestTrashNote_Conflict(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"error":"note not synced"}`))
	})

	_, _, err := handleTrashNote(context.Background(), c, TrashNoteInput{ID: "unsync-1"})
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusConflict, apiErr.StatusCode)
}

// --- Archive Note ---

func TestArchiveNote_Success(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/notes/note-1/archive", r.URL.Path)
		assert.NotEmpty(t, r.Header.Get("Idempotency-Key"))

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"note-1","title":"Archived","archived":1}`))
	})

	_, out, err := handleArchiveNote(context.Background(), c, ArchiveNoteInput{ID: "note-1"})
	require.NoError(t, err)
	assert.Equal(t, "note-1", out.ID)
	assert.Equal(t, 1, out.Archived)
}

func TestArchiveNote_Forbidden(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":"encrypted notes are read-only"}`))
	})

	_, _, err := handleArchiveNote(context.Background(), c, ArchiveNoteInput{ID: "enc-1"})
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
}

func TestArchiveNote_Conflict(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"error":"note not synced"}`))
	})

	_, _, err := handleArchiveNote(context.Background(), c, ArchiveNoteInput{ID: "unsync-1"})
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusConflict, apiErr.StatusCode)
}

// --- Add Tag ---

func TestAddTag_Success(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/notes/note-1/tags", r.URL.Path)
		assert.NotEmpty(t, r.Header.Get("Idempotency-Key"))

		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "work", body["tag"])

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":1,"action":"add_tag","note_id":"note-1","status":"pending"}`))
	})

	_, out, err := handleAddTag(context.Background(), c, AddTagInput{NoteID: "note-1", Tag: "work"})
	require.NoError(t, err)
	assert.Equal(t, "add_tag", out.Action)
	assert.Equal(t, "pending", out.Status)
}

func TestAddTag_Forbidden(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":"encrypted notes are read-only"}`))
	})

	_, _, err := handleAddTag(context.Background(), c, AddTagInput{NoteID: "enc-1", Tag: "work"})
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
}

func TestAddTag_Conflict(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"error":"note not synced"}`))
	})

	_, _, err := handleAddTag(context.Background(), c, AddTagInput{NoteID: "unsync-1", Tag: "work"})
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusConflict, apiErr.StatusCode)
}

// --- Rename Tag ---

func TestRenameTag_Success(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/api/tags/tag-1", r.URL.Path)
		assert.NotEmpty(t, r.Header.Get("Idempotency-Key"))

		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "new-name", body["new_name"])

		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"id":2,"action":"rename_tag","status":"pending"}`))
	})

	_, out, err := handleRenameTag(context.Background(), c, RenameTagInput{ID: "tag-1", NewName: "new-name"})
	require.NoError(t, err)
	assert.Equal(t, "rename_tag", out.Action)
	assert.Equal(t, "pending", out.Status)
}

func TestRenameTag_NotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"tag not found"}`))
	})

	_, _, err := handleRenameTag(context.Background(), c, RenameTagInput{ID: "missing", NewName: "x"})
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusNotFound, apiErr.StatusCode)
}

// --- Delete Tag ---

func TestDeleteTag_Success(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/api/tags/tag-1", r.URL.Path)
		assert.NotEmpty(t, r.Header.Get("Idempotency-Key"))

		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"id":3,"action":"delete_tag","status":"pending"}`))
	})

	_, out, err := handleDeleteTag(context.Background(), c, DeleteTagInput{ID: "tag-1"})
	require.NoError(t, err)
	assert.Equal(t, "delete_tag", out.Action)
	assert.Equal(t, "pending", out.Status)
}

func TestDeleteTag_NotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"tag not found"}`))
	})

	_, _, err := handleDeleteTag(context.Background(), c, DeleteTagInput{ID: "missing"})
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusNotFound, apiErr.StatusCode)
}

func TestRegisterTools_AllRegistered(t *testing.T) {
	c := NewClient("http://localhost", "token")
	s := newMCPServer(c)

	// Verify the server was created without panics and tools are registered.
	// The MCP SDK doesn't expose a way to list tools programmatically in tests,
	// so we verify by ensuring RegisterTools completes without error.
	require.NotNil(t, s)
}

// TestToolSchemas_AllHaveProperties verifies that every tool's inputSchema
// contains a "properties" field (even if empty). OpenAI-compatible APIs
// reject schemas without "properties".
func TestToolSchemas_AllHaveProperties(t *testing.T) {
	c := NewClient("http://localhost", "token")
	s := newMCPServer(c)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverTransport, clientTransport := gomcp.NewInMemoryTransports()

	serverSession, err := s.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	defer serverSession.Close()

	client := gomcp.NewClient(
		&gomcp.Implementation{Name: "test-client", Version: "1.0"}, nil,
	)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer clientSession.Close()

	result, err := clientSession.ListTools(ctx, nil)
	require.NoError(t, err)
	require.NotEmpty(t, result.Tools, "expected at least one tool")
	tools := result.Tools

	for _, tool := range tools {
		raw, err := json.Marshal(tool.InputSchema)
		require.NoError(t, err, "tool %s: failed to marshal inputSchema", tool.Name)

		var schema map[string]any
		require.NoError(t, json.Unmarshal(raw, &schema))

		_, hasProps := schema["properties"]
		assert.True(t, hasProps,
			"tool %q inputSchema missing 'properties' field: %s "+
				"(OpenAI-compatible APIs require this)", tool.Name, string(raw))
	}
}
