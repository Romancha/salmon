package xcallback

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockExecutor records calls and returns preconfigured responses.
type mockExecutor struct {
	calls   []executorCall
	output  []byte
	err     error
	outputs [][]byte // for sequential calls
	errs    []error
	callIdx int
}

type executorCall struct {
	Name string
	Args []string
}

func (m *mockExecutor) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	m.calls = append(m.calls, executorCall{Name: name, Args: args})
	if len(m.outputs) > 0 {
		idx := m.callIdx
		m.callIdx++
		if idx < len(m.outputs) {
			var err error
			if idx < len(m.errs) {
				err = m.errs[idx]
			}
			return m.outputs[idx], err
		}
	}
	return m.output, m.err
}

func newTestXcall(executor *mockExecutor) *Xcall {
	return NewWithPath("/usr/local/bin/xcall", WithExecutor(executor))
}

func TestCreate(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		resp := xcallResult{Identifier: "ABC-123-DEF"}
		respJSON, _ := json.Marshal(resp)
		executor := &mockExecutor{output: respJSON}
		x := newTestXcall(executor)

		bearID, err := x.Create(context.Background(), "test-token", "My Note", "Hello world", []string{"tag1", "tag2"})

		require.NoError(t, err)
		assert.Equal(t, "ABC-123-DEF", bearID)
		require.Len(t, executor.calls, 1)

		call := executor.calls[0]
		assert.Equal(t, "/usr/local/bin/xcall", call.Name)
		assert.Equal(t, "-url", call.Args[0])

		callURL := call.Args[1]
		assert.True(t, strings.HasPrefix(callURL, "bear://x-callback-url/create?"))

		parsed, err := url.Parse(callURL)
		require.NoError(t, err)
		q := parsed.Query()
		assert.Equal(t, "test-token", q.Get("token"))
		assert.Equal(t, "My Note", q.Get("title"))
		assert.Equal(t, "Hello world", q.Get("text"))
		assert.Equal(t, "tag1,tag2", q.Get("tags"))
		assert.Equal(t, "no", q.Get("show_window"))
		assert.Equal(t, "no", q.Get("open_note"))
	})

	t.Run("success without tags", func(t *testing.T) {
		resp := xcallResult{Identifier: "ABC-123"}
		respJSON, _ := json.Marshal(resp)
		executor := &mockExecutor{output: respJSON}
		x := newTestXcall(executor)

		bearID, err := x.Create(context.Background(), "tok", "Title", "Body", nil)

		require.NoError(t, err)
		assert.Equal(t, "ABC-123", bearID)

		callURL := executor.calls[0].Args[1]
		parsed, _ := url.Parse(callURL)
		assert.Empty(t, parsed.Query().Get("tags"))
	})

	t.Run("empty identifier", func(t *testing.T) {
		resp := xcallResult{}
		respJSON, _ := json.Marshal(resp)
		executor := &mockExecutor{output: respJSON}
		x := newTestXcall(executor)

		_, err := x.Create(context.Background(), "tok", "Title", "Body", nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty identifier")
	})

	t.Run("bear error", func(t *testing.T) {
		resp := xcallResult{ErrorCode: 100, ErrorMsg: "something went wrong"}
		respJSON, _ := json.Marshal(resp)
		executor := &mockExecutor{output: respJSON}
		x := newTestXcall(executor)

		_, err := x.Create(context.Background(), "tok", "Title", "Body", nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "bear error")
		assert.Contains(t, err.Error(), "code=100")
	})

	t.Run("xcall execution error", func(t *testing.T) {
		executor := &mockExecutor{err: fmt.Errorf("exit status 1")}
		x := newTestXcall(executor)

		_, err := x.Create(context.Background(), "tok", "Title", "Body", nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "xcall create")
	})

	t.Run("invalid JSON response", func(t *testing.T) {
		executor := &mockExecutor{output: []byte("not json")}
		x := newTestXcall(executor)

		_, err := x.Create(context.Background(), "tok", "Title", "Body", nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid xcall JSON")
	})
}

func TestUpdate(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		resp := xcallResult{}
		respJSON, _ := json.Marshal(resp)
		executor := &mockExecutor{output: respJSON}
		x := newTestXcall(executor)

		err := x.Update(context.Background(), "test-token", "BEAR-UUID", "New body content")

		require.NoError(t, err)
		require.Len(t, executor.calls, 1)

		callURL := executor.calls[0].Args[1]
		assert.True(t, strings.HasPrefix(callURL, "bear://x-callback-url/add-text?"))

		parsed, err := url.Parse(callURL)
		require.NoError(t, err)
		q := parsed.Query()
		assert.Equal(t, "test-token", q.Get("token"))
		assert.Equal(t, "BEAR-UUID", q.Get("id"))
		assert.Equal(t, "New body content", q.Get("text"))
		assert.Equal(t, "replace_all", q.Get("mode"))
		assert.Equal(t, "no", q.Get("show_window"))
		assert.Equal(t, "no", q.Get("open_note"))
	})

	t.Run("bear error", func(t *testing.T) {
		resp := xcallResult{ErrorCode: 1, ErrorMsg: "not found"}
		respJSON, _ := json.Marshal(resp)
		executor := &mockExecutor{output: respJSON}
		x := newTestXcall(executor)

		err := x.Update(context.Background(), "tok", "ID", "body")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "bear error")
	})
}

func TestAddTag(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		resp := xcallResult{}
		respJSON, _ := json.Marshal(resp)
		executor := &mockExecutor{output: respJSON}
		x := newTestXcall(executor)

		err := x.AddTag(context.Background(), "test-token", "BEAR-UUID", "work/project")

		require.NoError(t, err)
		require.Len(t, executor.calls, 1)

		callURL := executor.calls[0].Args[1]
		assert.True(t, strings.HasPrefix(callURL, "bear://x-callback-url/add-tag?"))

		parsed, err := url.Parse(callURL)
		require.NoError(t, err)
		q := parsed.Query()
		assert.Equal(t, "test-token", q.Get("token"))
		assert.Equal(t, "BEAR-UUID", q.Get("id"))
		assert.Equal(t, "work/project", q.Get("tags"))
		assert.Equal(t, "no", q.Get("show_window"))
	})

	t.Run("special characters in tag", func(t *testing.T) {
		resp := xcallResult{}
		respJSON, _ := json.Marshal(resp)
		executor := &mockExecutor{output: respJSON}
		x := newTestXcall(executor)

		err := x.AddTag(context.Background(), "tok", "ID", "тег/с пробелами & спецсимволами")

		require.NoError(t, err)

		callURL := executor.calls[0].Args[1]
		parsed, _ := url.Parse(callURL)
		assert.Equal(t, "тег/с пробелами & спецсимволами", parsed.Query().Get("tags"))
	})
}

func TestTrash(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		resp := xcallResult{}
		respJSON, _ := json.Marshal(resp)
		executor := &mockExecutor{output: respJSON}
		x := newTestXcall(executor)

		err := x.Trash(context.Background(), "test-token", "BEAR-UUID")

		require.NoError(t, err)
		require.Len(t, executor.calls, 1)

		callURL := executor.calls[0].Args[1]
		assert.True(t, strings.HasPrefix(callURL, "bear://x-callback-url/trash?"))

		parsed, err := url.Parse(callURL)
		require.NoError(t, err)
		q := parsed.Query()
		assert.Equal(t, "test-token", q.Get("token"))
		assert.Equal(t, "BEAR-UUID", q.Get("id"))
		assert.Equal(t, "no", q.Get("show_window"))
	})

	t.Run("bear error", func(t *testing.T) {
		resp := xcallResult{ErrorCode: 2, ErrorMsg: "note not found"}
		respJSON, _ := json.Marshal(resp)
		executor := &mockExecutor{output: respJSON}
		x := newTestXcall(executor)

		err := x.Trash(context.Background(), "tok", "ID")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "bear error")
	})
}

func TestMaskToken(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "masks token in create URL",
			input:    "bear://x-callback-url/create?token=secret123&title=Hello",
			expected: "bear://x-callback-url/create?title=Hello&token=%2A%2A%2A",
		},
		{
			name:     "no token present",
			input:    "bear://x-callback-url/create?title=Hello",
			expected: "bear://x-callback-url/create?title=Hello",
		},
		{
			name:     "invalid URL returns as-is",
			input:    "://invalid",
			expected: "://invalid",
		},
		{
			name:     "empty token unchanged",
			input:    "bear://x-callback-url/create?token=&title=Hello",
			expected: "bear://x-callback-url/create?title=Hello&token=",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaskToken(tt.input)
			// Parse both to compare query params regardless of order
			if tt.name == "invalid URL returns as-is" {
				assert.Equal(t, tt.input, result)
				return
			}
			parsedResult, err := url.Parse(result)
			require.NoError(t, err)
			parsedExpected, err := url.Parse(tt.expected)
			require.NoError(t, err)
			assert.Equal(t, parsedExpected.Query(), parsedResult.Query())
		})
	}
}

func TestBuildURL(t *testing.T) {
	params := url.Values{}
	params.Set("token", "abc")
	params.Set("title", "Hello World")

	result := BuildURL("create", params)

	assert.True(t, strings.HasPrefix(result, "bear://x-callback-url/create?"))
	parsed, err := url.Parse(result)
	require.NoError(t, err)
	assert.Equal(t, "abc", parsed.Query().Get("token"))
	assert.Equal(t, "Hello World", parsed.Query().Get("title"))
}

func TestURLEncoding(t *testing.T) {
	t.Run("special characters in title and body", func(t *testing.T) {
		resp := xcallResult{Identifier: "ID-1"}
		respJSON, _ := json.Marshal(resp)
		executor := &mockExecutor{output: respJSON}
		x := newTestXcall(executor)

		title := "Note with <special> chars & \"quotes\""
		body := "Line1\nLine2\n# Header\n- list item"

		_, err := x.Create(context.Background(), "tok", title, body, nil)
		require.NoError(t, err)

		callURL := executor.calls[0].Args[1]
		parsed, err := url.Parse(callURL)
		require.NoError(t, err)
		assert.Equal(t, title, parsed.Query().Get("title"))
		assert.Equal(t, body, parsed.Query().Get("text"))
	})

	t.Run("unicode characters", func(t *testing.T) {
		resp := xcallResult{Identifier: "ID-2"}
		respJSON, _ := json.Marshal(resp)
		executor := &mockExecutor{output: respJSON}
		x := newTestXcall(executor)

		title := "Заметка на русском 日本語"
		body := "Контент с эмодзи 🚀"

		_, err := x.Create(context.Background(), "tok", title, body, nil)
		require.NoError(t, err)

		callURL := executor.calls[0].Args[1]
		parsed, err := url.Parse(callURL)
		require.NoError(t, err)
		assert.Equal(t, title, parsed.Query().Get("title"))
		assert.Equal(t, body, parsed.Query().Get("text"))
	})
}

func TestParseXcallResult(t *testing.T) {
	t.Run("empty output", func(t *testing.T) {
		result, err := parseXcallResult([]byte{})
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Empty(t, result.Identifier)
	})

	t.Run("valid JSON with identifier", func(t *testing.T) {
		input := `{"identifier":"ABC-123","title":"My Note"}`
		result, err := parseXcallResult([]byte(input))
		require.NoError(t, err)
		assert.Equal(t, "ABC-123", result.Identifier)
		assert.Equal(t, "My Note", result.Title)
	})

	t.Run("error response", func(t *testing.T) {
		input := `{"errorCode":100,"errorMessage":"Token not valid"}`
		result, err := parseXcallResult([]byte(input))
		require.NoError(t, err)
		assert.Equal(t, 100, result.ErrorCode)
		assert.Equal(t, "Token not valid", result.ErrorMsg)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := parseXcallResult([]byte("not json"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid xcall JSON")
	})
}

func TestMaskTokenNotInLogs(t *testing.T) {
	rawURL := "bear://x-callback-url/create?token=my-secret-token-123&title=Test"
	masked := MaskToken(rawURL)

	assert.NotContains(t, masked, "my-secret-token-123")
	assert.Contains(t, masked, "token=")
}
