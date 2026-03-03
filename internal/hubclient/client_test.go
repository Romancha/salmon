package hubclient_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/romancha/bear-sync/internal/hubclient"
	"github.com/romancha/bear-sync/internal/models"
)

func newTestClient(t *testing.T, handler http.Handler) *hubclient.HTTPClient {
	t.Helper()

	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	return hubclient.NewHTTPClient(ts.URL, "test-token", nil)
}

func TestSyncPush_Success(t *testing.T) {
	var receivedBody []byte
	var receivedAuth string

	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))

	req := models.SyncPushRequest{
		Notes: []models.Note{{ID: "note-1", Title: "Test Note"}},
		Tags:  []models.Tag{{ID: "tag-1", Title: "test"}},
	}

	err := client.SyncPush(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, "Bearer test-token", receivedAuth)

	var decoded models.SyncPushRequest
	require.NoError(t, json.Unmarshal(receivedBody, &decoded))
	assert.Equal(t, "note-1", decoded.Notes[0].ID)
	assert.Equal(t, "tag-1", decoded.Tags[0].ID)
}

func TestSyncPush_ServerError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid request"}`))
	}))

	err := client.SyncPush(context.Background(), models.SyncPushRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid request")
}

func TestLeaseQueue_Success(t *testing.T) {
	items := []models.WriteQueueItem{
		{ID: 1, Action: "create", NoteID: "note-1", Payload: `{"title":"Test"}`, Status: "processing"},
		{ID: 2, Action: "update", NoteID: "note-2", Payload: `{"body":"Updated"}`, Status: "processing"},
	}

	var receivedPath string

	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.RequestURI()
		w.WriteHeader(http.StatusOK)
		resp, _ := json.Marshal(items)
		_, _ = w.Write(resp)
	}))

	result, err := client.LeaseQueue(context.Background(), "bridge-1")
	require.NoError(t, err)

	assert.Equal(t, "/api/sync/queue?processing_by=bridge-1", receivedPath)
	assert.Len(t, result, 2)
	assert.Equal(t, int64(1), result[0].ID)
	assert.Equal(t, "create", result[0].Action)
	assert.Equal(t, int64(2), result[1].ID)
}

func TestLeaseQueue_ConsumerIDDeserialized(t *testing.T) {
	items := []models.WriteQueueItem{
		{ID: 1, Action: "create", NoteID: "note-1", Payload: `{"title":"Test"}`, Status: "processing", ConsumerID: "openclaw"},
		{ID: 2, Action: "update", NoteID: "note-2", Payload: `{"body":"Updated"}`, Status: "processing", ConsumerID: "myapp"},
	}

	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		resp, _ := json.Marshal(items)
		_, _ = w.Write(resp)
	}))

	result, err := client.LeaseQueue(context.Background(), "bridge")
	require.NoError(t, err)

	require.Len(t, result, 2)
	assert.Equal(t, "openclaw", result[0].ConsumerID)
	assert.Equal(t, "myapp", result[1].ConsumerID)
}

func TestLeaseQueue_EmptyResponse(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))

	result, err := client.LeaseQueue(context.Background(), "bridge")
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestAckQueue_Success(t *testing.T) {
	var receivedBody []byte

	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))

	items := []models.SyncAckItem{
		{QueueID: 1, IdempotencyKey: "key-1", Status: "applied", BearID: "bear-uuid-1"},
		{QueueID: 2, IdempotencyKey: "key-2", Status: "failed", Error: "xcall timeout"},
	}

	err := client.AckQueue(context.Background(), items)
	require.NoError(t, err)

	var decoded models.SyncAckRequest
	require.NoError(t, json.Unmarshal(receivedBody, &decoded))
	assert.Len(t, decoded.Items, 2)
	assert.Equal(t, "applied", decoded.Items[0].Status)
	assert.Equal(t, "bear-uuid-1", decoded.Items[0].BearID)
	assert.Equal(t, "failed", decoded.Items[1].Status)
	assert.Equal(t, "xcall timeout", decoded.Items[1].Error)
}

func TestUploadAttachment_Success(t *testing.T) {
	var receivedBody []byte
	var receivedPath string

	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","path":"/data/attachments/att-1/photo.jpg"}`))
	}))

	fileData := []byte("fake-image-data")
	err := client.UploadAttachment(context.Background(), "att-1", bytes.NewReader(fileData))
	require.NoError(t, err)

	assert.Equal(t, "/api/sync/attachments/att-1", receivedPath)
	assert.Equal(t, fileData, receivedBody)
}

func TestGetSyncStatus_Success(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"last_sync_at": "2026-03-01T10:00:00Z",
			"last_push_at": "2026-03-01T10:05:00Z",
			"queue_size": 3,
			"initial_sync_complete": "true"
		}`))
	}))

	status, err := client.GetSyncStatus(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "2026-03-01T10:00:00Z", status.LastSyncAt)
	assert.Equal(t, "2026-03-01T10:05:00Z", status.LastPushAt)
	assert.Equal(t, 3, status.QueueSize)
	assert.Equal(t, "true", status.InitialSyncComplete)
}

func TestRetry_TransientError(t *testing.T) {
	var attempts atomic.Int32

	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"service unavailable"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))

	err := client.SyncPush(context.Background(), models.SyncPushRequest{})
	require.NoError(t, err)
	assert.Equal(t, int32(3), attempts.Load())
}

func TestRetry_AllAttemptsFail(t *testing.T) {
	var attempts atomic.Int32

	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"down"}`))
	}))

	err := client.SyncPush(context.Background(), models.SyncPushRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed after 3 attempts")
	assert.Equal(t, int32(3), attempts.Load())
}

func TestRetry_NonTransientErrorNoRetry(t *testing.T) {
	var attempts atomic.Int32

	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad request"}`))
	}))

	err := client.SyncPush(context.Background(), models.SyncPushRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad request")
	assert.Equal(t, int32(1), attempts.Load())
}

func TestRetry_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	err := client.SyncPush(ctx, models.SyncPushRequest{})
	require.Error(t, err)
}

func TestAuthHeader(t *testing.T) {
	var receivedAuth string

	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))

	_, err := client.LeaseQueue(context.Background(), "bridge")
	require.NoError(t, err)
	assert.Equal(t, "Bearer test-token", receivedAuth)
}

func TestUploadAttachment_NotFound(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"attachment not found"}`))
	}))

	err := client.UploadAttachment(context.Background(), "missing-id", strings.NewReader("data"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "attachment not found")
}

func TestContentType_SetForPOST(t *testing.T) {
	var receivedContentType string

	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))

	err := client.SyncPush(context.Background(), models.SyncPushRequest{})
	require.NoError(t, err)
	assert.Equal(t, "application/json", receivedContentType)
}
