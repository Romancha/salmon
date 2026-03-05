package hubclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/romancha/salmon/internal/models"
)

// SyncStatus represents the hub's current sync state.
type SyncStatus struct {
	LastSyncAt          string `json:"last_sync_at"`
	LastPushAt          string `json:"last_push_at"`
	QueueSize           int    `json:"queue_size"`
	InitialSyncComplete string `json:"initial_sync_complete"`
}

// HubClient defines the interface for communicating with the hub API.
type HubClient interface {
	SyncPush(ctx context.Context, req models.SyncPushRequest) error
	LeaseQueue(ctx context.Context, processingBy string) ([]models.WriteQueueItem, error)
	AckQueue(ctx context.Context, items []models.SyncAckItem) error
	UploadAttachment(ctx context.Context, attachmentID string, reader io.Reader) error
	DownloadAttachment(ctx context.Context, attachmentID string) ([]byte, error)
	GetSyncStatus(ctx context.Context) (*SyncStatus, error)
}

const (
	maxRetries    = 3
	baseDelay     = 500 * time.Millisecond
	maxDelay      = 10 * time.Second
	clientTimeout = 60 * time.Second
)

// HTTPClient implements HubClient using HTTP requests to the hub API.
type HTTPClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
	logger     *slog.Logger
}

// NewHTTPClient creates a new hub HTTP client.
func NewHTTPClient(baseURL, token string, logger *slog.Logger) *HTTPClient {
	if logger == nil {
		logger = slog.Default()
	}

	return &HTTPClient{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: clientTimeout,
		},
		logger: logger,
	}
}

func (c *HTTPClient) SyncPush(ctx context.Context, req models.SyncPushRequest) error { //nolint:gocritic // matches API contract
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal sync push request: %w", err)
	}

	resp, err := c.doWithRetry(ctx, http.MethodPost, "/api/sync/push", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("sync push: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close on response body

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sync push: %w", parseErrorResponse(resp))
	}

	return nil
}

func (c *HTTPClient) LeaseQueue(ctx context.Context, processingBy string) ([]models.WriteQueueItem, error) {
	path := "/api/sync/queue?processing_by=" + processingBy

	resp, err := c.doWithRetry(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("lease queue: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close on response body

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lease queue: %w", parseErrorResponse(resp))
	}

	var items []models.WriteQueueItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("decode lease queue response: %w", err)
	}

	return items, nil
}

func (c *HTTPClient) AckQueue(ctx context.Context, items []models.SyncAckItem) error {
	req := models.SyncAckRequest{Items: items}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal ack request: %w", err)
	}

	resp, err := c.doWithRetry(ctx, http.MethodPost, "/api/sync/ack", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("ack queue: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close on response body

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ack queue: %w", parseErrorResponse(resp))
	}

	return nil
}

func (c *HTTPClient) UploadAttachment(ctx context.Context, attachmentID string, reader io.Reader) error {
	path := "/api/sync/attachments/" + attachmentID

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("create upload request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.httpClient.Do(req) //nolint:bodyclose,gosec // caller-like close below; URL from trusted config
	if err != nil {
		return fmt.Errorf("upload attachment: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close on response body

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upload attachment: %w", parseErrorResponse(resp))
	}

	return nil
}

const maxDownloadSize = 10 * 1024 * 1024 // 10 MB

func (c *HTTPClient) DownloadAttachment(ctx context.Context, attachmentID string) ([]byte, error) {
	path := "/api/sync/attachments/" + attachmentID

	resp, err := c.doWithRetry(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("download attachment: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close on response body

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download attachment: %w", parseErrorResponse(resp))
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDownloadSize+1))
	if err != nil {
		return nil, fmt.Errorf("read attachment body: %w", err)
	}

	if len(data) > maxDownloadSize {
		return nil, fmt.Errorf("attachment %s exceeds maximum size of %d bytes", attachmentID, maxDownloadSize)
	}

	return data, nil
}

func (c *HTTPClient) GetSyncStatus(ctx context.Context) (*SyncStatus, error) {
	resp, err := c.doWithRetry(ctx, http.MethodGet, "/api/sync/status", nil)
	if err != nil {
		return nil, fmt.Errorf("get sync status: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close on response body

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get sync status: %w", parseErrorResponse(resp))
	}

	var status SyncStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("decode sync status response: %w", err)
	}

	return &status, nil
}

// doWithRetry executes an HTTP request with exponential backoff for transient errors.
func (c *HTTPClient) doWithRetry(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	var lastErr error

	// Buffer the body so we can retry.
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = io.ReadAll(body)
		if err != nil {
			return nil, fmt.Errorf("read request body: %w", err)
		}
	}

	for attempt := range maxRetries {
		var reqBody io.Reader
		if bodyBytes != nil {
			reqBody = bytes.NewReader(bodyBytes)
		}

		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+c.token)
		if bodyBytes != nil && method != http.MethodGet {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.httpClient.Do(req) //nolint:bodyclose,gosec // caller closes body; URL from trusted config
		if err != nil {
			lastErr = err

			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, fmt.Errorf("request %s %s: %w", method, path, err)
			}

			c.logger.Warn("request failed, retrying",
				"method", method, "path", path,
				"attempt", attempt+1, "error", err)

			if err := sleepWithContext(ctx, backoffDelay(attempt)); err != nil {
				return nil, fmt.Errorf("retry wait: %w", err)
			}

			continue
		}

		if isTransientStatus(resp.StatusCode) {
			lastErr = parseErrorResponse(resp)
			resp.Body.Close() //nolint:errcheck,gosec // closing before retry

			c.logger.Warn("transient error, retrying",
				"method", method, "path", path,
				"status", resp.StatusCode, "attempt", attempt+1)

			if err := sleepWithContext(ctx, backoffDelay(attempt)); err != nil {
				return nil, fmt.Errorf("retry wait: %w", err)
			}

			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("request %s %s failed after %d attempts: %w", method, path, maxRetries, lastErr)
}

func backoffDelay(attempt int) time.Duration {
	delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attempt)))
	if delay > maxDelay {
		delay = maxDelay
	}

	return delay
}

func isTransientStatus(code int) bool {
	return code == http.StatusTooManyRequests ||
		code == http.StatusBadGateway ||
		code == http.StatusServiceUnavailable ||
		code == http.StatusGatewayTimeout
}

type apiError struct {
	StatusCode int
	Message    string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Message)
}

func parseErrorResponse(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096)) //nolint:errcheck // best-effort read, capped at 4KB

	var errResp struct {
		Error string `json:"error"`
	}

	if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
		return &apiError{StatusCode: resp.StatusCode, Message: errResp.Error}
	}

	return &apiError{StatusCode: resp.StatusCode, Message: string(body)}
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return fmt.Errorf("context done: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}
