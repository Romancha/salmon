package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_Get_Success(t *testing.T) {
	expected := map[string]string{"status": "ok"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/test", r.URL.Path)
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		assert.Equal(t, "bar", r.URL.Query().Get("foo"))
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(expected)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-token")
	data, err := c.get(context.Background(), "/api/test", url.Values{"foo": {"bar"}})
	require.NoError(t, err)

	var got map[string]string
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, "ok", got["status"])
}

func TestClient_Get_NoQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.URL.RawQuery)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-token")
	_, err := c.get(context.Background(), "/api/test", nil)
	require.NoError(t, err)
}

func TestClient_PostJSON_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		assert.NotEmpty(t, r.Header.Get("Idempotency-Key"))
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"123"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-token")
	data, err := c.postJSON(context.Background(), "/api/notes", map[string]string{"title": "Test"})
	require.NoError(t, err)

	var got map[string]string
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, "123", got["id"])
}

func TestClient_PutJSON_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.NotEmpty(t, r.Header.Get("Idempotency-Key"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"123"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-token")
	data, err := c.putJSON(context.Background(), "/api/notes/123", map[string]string{"body": "Updated"})
	require.NoError(t, err)
	assert.Contains(t, string(data), "123")
}

func TestClient_Delete_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/api/notes/123", r.URL.Path)
		assert.NotEmpty(t, r.Header.Get("Idempotency-Key"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"123"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-token")
	data, err := c.delete(context.Background(), "/api/notes/123")
	require.NoError(t, err)
	assert.Contains(t, string(data), "123")
}

func TestClient_ErrorCodes(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantMsg    string
	}{
		{
			name:       "401 unauthorized",
			statusCode: http.StatusUnauthorized,
			body:       `{"error":"unauthorized"}`,
			wantMsg:    "authentication failed: invalid or missing token",
		},
		{
			name:       "403 forbidden",
			statusCode: http.StatusForbidden,
			body:       `{"error":"encrypted notes are read-only"}`,
			wantMsg:    "forbidden: encrypted notes are read-only",
		},
		{
			name:       "404 not found",
			statusCode: http.StatusNotFound,
			body:       `{"error":"note not found"}`,
			wantMsg:    "not found",
		},
		{
			name:       "409 conflict with message",
			statusCode: http.StatusConflict,
			body:       `{"error":"note has unresolved conflicts; resolve conflicts before updating"}`,
			wantMsg:    "note has unresolved conflicts; resolve conflicts before updating",
		},
		{
			name:       "409 conflict without message",
			statusCode: http.StatusConflict,
			body:       `{}`,
			wantMsg:    "conflict: note not synced to Bear or has unresolved conflicts",
		},
		{
			name:       "400 bad request with message",
			statusCode: http.StatusBadRequest,
			body:       `{"error":"title is required"}`,
			wantMsg:    "hub API error 400: title is required",
		},
		{
			name:       "500 internal error without json",
			statusCode: http.StatusInternalServerError,
			body:       `not json`,
			wantMsg:    "hub API error: 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			c := NewClient(srv.URL, "test-token")
			_, err := c.get(context.Background(), "/api/test", nil)
			require.Error(t, err)

			var apiErr *APIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, tt.statusCode, apiErr.StatusCode)
			assert.Equal(t, tt.wantMsg, apiErr.Message)
		})
	}
}

func TestClient_AuthHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer my-secret-token", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "my-secret-token")
	_, err := c.get(context.Background(), "/api/test", nil)
	require.NoError(t, err)
}

func TestClient_IdempotencyKey_Unique(t *testing.T) {
	var keys []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-token")

	_, _ = c.postJSON(context.Background(), "/api/notes", map[string]string{"title": "A"})
	_, _ = c.postJSON(context.Background(), "/api/notes", map[string]string{"title": "B"})

	require.Len(t, keys, 2)
	assert.NotEqual(t, keys[0], keys[1], "idempotency keys must be unique per request")
}

func TestClient_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-token")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.get(ctx, "/api/test", nil)
	require.Error(t, err)
}

func TestClient_GetRaw_Success(t *testing.T) {
	content := []byte("binary file content")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/attachments/a1", r.URL.Path)
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Disposition", `attachment; filename="doc.pdf"`)
		w.Header().Set("Content-Type", "application/pdf")
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-token")
	resp, err := c.getRaw(context.Background(), "/api/attachments/a1")
	require.NoError(t, err)
	assert.Equal(t, content, resp.Body)
	assert.Equal(t, "doc.pdf", resp.Filename)
	assert.Equal(t, "application/pdf", resp.ContentType)
}

func TestClient_GetRaw_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-token")
	_, err := c.getRaw(context.Background(), "/api/attachments/missing")
	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusNotFound, apiErr.StatusCode)
}

func TestParseFilename(t *testing.T) {
	tests := []struct {
		name        string
		disposition string
		want        string
	}{
		{"standard", `attachment; filename="photo.png"`, "photo.png"},
		{"empty", "", ""},
		{"no filename", "attachment", ""},
		{"no quotes end", `attachment; filename="photo.png`, "photo.png"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseFilename(tt.disposition))
		})
	}
}
