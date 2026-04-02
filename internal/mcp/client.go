package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
)

// Client is an HTTP client for the Salmon Hub consumer API.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a new Salmon Hub API client.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// get performs a GET request and returns the response body.
func (c *Client) get(ctx context.Context, path string, query url.Values) ([]byte, error) {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	return c.do(req)
}

// postJSON performs a POST request with a JSON body.
func (c *Client) postJSON(ctx context.Context, path string, body any) ([]byte, error) {
	return c.doJSON(ctx, http.MethodPost, path, body)
}

// putJSON performs a PUT request with a JSON body.
func (c *Client) putJSON(ctx context.Context, path string, body any) ([]byte, error) {
	return c.doJSON(ctx, http.MethodPut, path, body)
}

// delete performs a DELETE request.
func (c *Client) delete(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Idempotency-Key", uuid.NewString())

	return c.do(req)
}

// doJSON performs a request with a JSON body and an auto-generated Idempotency-Key.
func (c *Client) doJSON(ctx context.Context, method, path string, body any) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.NewString())

	return c.do(req)
}

// do executes an HTTP request with auth header and handles error responses.
func (c *Client) do(req *http.Request) ([]byte, error) {
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    parseErrorMessage(resp.StatusCode, data),
		}
	}

	return data, nil
}

// APIError represents an error response from the Salmon Hub API.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return e.Message
}

// parseErrorMessage extracts a human-readable error message from an API error response.
func parseErrorMessage(statusCode int, body []byte) string {
	switch statusCode {
	case http.StatusUnauthorized:
		return "authentication failed: invalid or missing token"
	case http.StatusForbidden:
		return "forbidden: encrypted notes are read-only"
	case http.StatusNotFound:
		return "not found"
	case http.StatusConflict:
		msg := extractJSONError(body)
		if msg != "" {
			return msg
		}
		return "conflict: note not synced to Bear or has unresolved conflicts"
	default:
		msg := extractJSONError(body)
		if msg != "" {
			return fmt.Sprintf("hub API error %d: %s", statusCode, msg)
		}
		return fmt.Sprintf("hub API error: %d", statusCode)
	}
}

// extractJSONError tries to extract the "error" field from a JSON error response.
func extractJSONError(body []byte) string {
	var errResp struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
		return errResp.Error
	}
	return ""
}
