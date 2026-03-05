package xcallback

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type executorCall struct {
	Name string
	Args []string
}

// mockExecutor records calls and returns preconfigured responses.
type mockExecutor struct {
	calls     []executorCall
	output    []byte
	err       error
	outputs   [][]byte // for sequential calls
	errs      []error
	callIdx   int
	lastStdin []byte // captured stdin from RunWithStdin
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

func (m *mockExecutor) RunWithStdin(_ context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
	if stdin != nil {
		m.lastStdin, _ = io.ReadAll(stdin)
	}
	return m.Run(context.Background(), name, args...)
}

func newTestXcall(executor *mockExecutor) *Xcall {
	return NewWithPath("/usr/local/bin/bear-xcall", WithExecutor(executor))
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
		assert.Equal(t, "/usr/local/bin/bear-xcall", call.Name)
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

	t.Run("empty identifier returns empty string without error", func(t *testing.T) {
		resp := xcallResult{}
		respJSON, _ := json.Marshal(resp)
		executor := &mockExecutor{output: respJSON}
		x := newTestXcall(executor)

		bearID, err := x.Create(context.Background(), "tok", "Title", "Body", nil)

		require.NoError(t, err)
		assert.Empty(t, bearID)
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

	t.Run("bear-xcall execution error", func(t *testing.T) {
		executor := &mockExecutor{err: fmt.Errorf("exit status 1")}
		x := newTestXcall(executor)

		_, err := x.Create(context.Background(), "tok", "Title", "Body", nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "bear-xcall create")
	})

	t.Run("invalid JSON response", func(t *testing.T) {
		executor := &mockExecutor{output: []byte("not json")}
		x := newTestXcall(executor)

		_, err := x.Create(context.Background(), "tok", "Title", "Body", nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid bear-xcall JSON")
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
		assert.Equal(t, "replace", q.Get("mode"))
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

	t.Run("exec error", func(t *testing.T) {
		executor := &mockExecutor{err: fmt.Errorf("exit status 1")}
		x := newTestXcall(executor)

		err := x.Update(context.Background(), "tok", "ID", "body")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "bear-xcall update")
	})

	t.Run("invalid JSON response", func(t *testing.T) {
		executor := &mockExecutor{output: []byte("not json")}
		x := newTestXcall(executor)

		err := x.Update(context.Background(), "tok", "ID", "body")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid bear-xcall JSON")
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
		assert.True(t, strings.HasPrefix(callURL, "bear://x-callback-url/add-text?"))

		parsed, err := url.Parse(callURL)
		require.NoError(t, err)
		q := parsed.Query()
		assert.Equal(t, "test-token", q.Get("token"))
		assert.Equal(t, "BEAR-UUID", q.Get("id"))
		assert.Equal(t, "work/project", q.Get("tags"))
		assert.Equal(t, "no", q.Get("show_window"))
	})

	t.Run("bear error", func(t *testing.T) {
		resp := xcallResult{ErrorCode: 1, ErrorMsg: "not found"}
		respJSON, _ := json.Marshal(resp)
		executor := &mockExecutor{output: respJSON}
		x := newTestXcall(executor)

		err := x.AddTag(context.Background(), "tok", "ID", "tag")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "bear error")
	})

	t.Run("exec error", func(t *testing.T) {
		executor := &mockExecutor{err: fmt.Errorf("exit status 1")}
		x := newTestXcall(executor)

		err := x.AddTag(context.Background(), "tok", "ID", "tag")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "bear-xcall add-tag")
	})

	t.Run("invalid JSON response", func(t *testing.T) {
		executor := &mockExecutor{output: []byte("not json")}
		x := newTestXcall(executor)

		err := x.AddTag(context.Background(), "tok", "ID", "tag")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid bear-xcall JSON")
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

// testNoteAction holds config for testing Trash/Archive (identical signatures).
type testNoteAction struct {
	name      string
	urlPath   string
	execLabel string
	callFn    func(x *Xcall, ctx context.Context, token, bearID string) error
}

var noteActions = []testNoteAction{
	{"Trash", "bear://x-callback-url/trash?", "bear-xcall trash", func(x *Xcall, ctx context.Context, token, bearID string) error {
		return x.Trash(ctx, token, bearID)
	}},
	{"Archive", "bear://x-callback-url/archive?", "bear-xcall archive", func(x *Xcall, ctx context.Context, token, bearID string) error {
		return x.Archive(ctx, token, bearID)
	}},
}

func TestNoteActions(t *testing.T) {
	for _, action := range noteActions {
		t.Run(action.name+"/success", func(t *testing.T) {
			resp := xcallResult{}
			respJSON, _ := json.Marshal(resp)
			executor := &mockExecutor{output: respJSON}
			x := newTestXcall(executor)

			err := action.callFn(x, context.Background(), "test-token", "BEAR-UUID")

			require.NoError(t, err)
			require.Len(t, executor.calls, 1)

			callURL := executor.calls[0].Args[1]
			assert.True(t, strings.HasPrefix(callURL, action.urlPath))

			parsed, err := url.Parse(callURL)
			require.NoError(t, err)
			q := parsed.Query()
			assert.Equal(t, "test-token", q.Get("token"))
			assert.Equal(t, "BEAR-UUID", q.Get("id"))
			assert.Equal(t, "no", q.Get("show_window"))
		})

		t.Run(action.name+"/bear error", func(t *testing.T) {
			resp := xcallResult{ErrorCode: 2, ErrorMsg: "note not found"}
			respJSON, _ := json.Marshal(resp)
			executor := &mockExecutor{output: respJSON}
			x := newTestXcall(executor)

			err := action.callFn(x, context.Background(), "tok", "ID")

			require.Error(t, err)
			assert.Contains(t, err.Error(), "bear error")
		})

		t.Run(action.name+"/exec error", func(t *testing.T) {
			executor := &mockExecutor{err: fmt.Errorf("exit status 1")}
			x := newTestXcall(executor)

			err := action.callFn(x, context.Background(), "tok", "ID")

			require.Error(t, err)
			assert.Contains(t, err.Error(), action.execLabel)
		})

		t.Run(action.name+"/invalid JSON response", func(t *testing.T) {
			executor := &mockExecutor{output: []byte("not json")}
			x := newTestXcall(executor)

			err := action.callFn(x, context.Background(), "tok", "ID")

			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid bear-xcall JSON")
		})
	}
}

func TestAddFile(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		resp := xcallResult{}
		respJSON, _ := json.Marshal(resp)
		executor := &mockExecutor{output: respJSON}
		x := newTestXcall(executor)

		fileData := []byte("hello world file content")
		err := x.AddFile(context.Background(), "test-token", "BEAR-UUID", "photo.jpg", fileData)

		require.NoError(t, err)
		require.Len(t, executor.calls, 1)

		// AddFile uses RunWithStdin with "-url -" to bypass ARG_MAX for large URLs.
		assert.Equal(t, []string{"-url", "-"}, executor.calls[0].Args)

		// The actual URL is piped via stdin.
		callURL := string(executor.lastStdin)
		assert.True(t, strings.HasPrefix(callURL, "bear://x-callback-url/add-file?"))

		parsed, err := url.Parse(callURL)
		require.NoError(t, err)
		q := parsed.Query()
		assert.Equal(t, "test-token", q.Get("token"))
		assert.Equal(t, "BEAR-UUID", q.Get("id"))
		assert.Equal(t, "photo.jpg", q.Get("filename"))
		assert.Equal(t, base64.StdEncoding.EncodeToString(fileData), q.Get("file"))
		assert.Equal(t, "no", q.Get("show_window"))
		assert.Equal(t, "no", q.Get("open_note"))
	})

	t.Run("bear error", func(t *testing.T) {
		resp := xcallResult{ErrorCode: 1, ErrorMsg: "not found"}
		respJSON, _ := json.Marshal(resp)
		executor := &mockExecutor{output: respJSON}
		x := newTestXcall(executor)

		err := x.AddFile(context.Background(), "tok", "ID", "file.txt", []byte("data"))

		require.Error(t, err)
		assert.Contains(t, err.Error(), "bear error")
	})

	t.Run("exec error", func(t *testing.T) {
		executor := &mockExecutor{err: fmt.Errorf("exit status 1")}
		x := newTestXcall(executor)

		err := x.AddFile(context.Background(), "tok", "ID", "file.txt", []byte("data"))

		require.Error(t, err)
		assert.Contains(t, err.Error(), "bear-xcall add-file")
	})

	t.Run("invalid JSON response", func(t *testing.T) {
		executor := &mockExecutor{output: []byte("not json")}
		x := newTestXcall(executor)

		err := x.AddFile(context.Background(), "tok", "ID", "file.txt", []byte("data"))

		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid bear-xcall JSON")
	})

	t.Run("file too large", func(t *testing.T) {
		executor := &mockExecutor{}
		x := newTestXcall(executor)

		largeData := make([]byte, 6*1024*1024) // 6 MB
		err := x.AddFile(context.Background(), "tok", "ID", "big.bin", largeData)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds limit")
		assert.Empty(t, executor.calls, "executor should not be called for oversized files")
	})
}

func TestRenameTag(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		resp := xcallResult{}
		respJSON, _ := json.Marshal(resp)
		executor := &mockExecutor{output: respJSON}
		x := newTestXcall(executor)

		err := x.RenameTag(context.Background(), "test-token", "old/tag", "new/tag")

		require.NoError(t, err)
		require.Len(t, executor.calls, 1)

		callURL := executor.calls[0].Args[1]
		assert.True(t, strings.HasPrefix(callURL, "bear://x-callback-url/rename-tag?"))

		parsed, err := url.Parse(callURL)
		require.NoError(t, err)
		q := parsed.Query()
		assert.Equal(t, "test-token", q.Get("token"))
		assert.Equal(t, "old/tag", q.Get("name"))
		assert.Equal(t, "new/tag", q.Get("new_name"))
		assert.Equal(t, "no", q.Get("show_window"))
	})

	t.Run("bear error", func(t *testing.T) {
		resp := xcallResult{ErrorCode: 1, ErrorMsg: "tag is locked"}
		respJSON, _ := json.Marshal(resp)
		executor := &mockExecutor{output: respJSON}
		x := newTestXcall(executor)

		err := x.RenameTag(context.Background(), "tok", "old", "new")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "bear error")
	})

	t.Run("exec error", func(t *testing.T) {
		executor := &mockExecutor{err: fmt.Errorf("exit status 1")}
		x := newTestXcall(executor)

		err := x.RenameTag(context.Background(), "tok", "old", "new")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "bear-xcall rename-tag")
	})

	t.Run("invalid JSON response", func(t *testing.T) {
		executor := &mockExecutor{output: []byte("not json")}
		x := newTestXcall(executor)

		err := x.RenameTag(context.Background(), "tok", "old", "new")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid bear-xcall JSON")
	})

	t.Run("special characters in names", func(t *testing.T) {
		resp := xcallResult{}
		respJSON, _ := json.Marshal(resp)
		executor := &mockExecutor{output: respJSON}
		x := newTestXcall(executor)

		err := x.RenameTag(context.Background(), "tok", "тег/подтег", "новый/тег & символы")

		require.NoError(t, err)

		callURL := executor.calls[0].Args[1]
		parsed, _ := url.Parse(callURL)
		assert.Equal(t, "тег/подтег", parsed.Query().Get("name"))
		assert.Equal(t, "новый/тег & символы", parsed.Query().Get("new_name"))
	})
}

func TestDeleteTag(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		resp := xcallResult{}
		respJSON, _ := json.Marshal(resp)
		executor := &mockExecutor{output: respJSON}
		x := newTestXcall(executor)

		err := x.DeleteTag(context.Background(), "test-token", "old/tag")

		require.NoError(t, err)
		require.Len(t, executor.calls, 1)

		callURL := executor.calls[0].Args[1]
		assert.True(t, strings.HasPrefix(callURL, "bear://x-callback-url/delete-tag?"))

		parsed, err := url.Parse(callURL)
		require.NoError(t, err)
		q := parsed.Query()
		assert.Equal(t, "test-token", q.Get("token"))
		assert.Equal(t, "old/tag", q.Get("name"))
		assert.Equal(t, "no", q.Get("show_window"))
	})

	t.Run("bear error", func(t *testing.T) {
		resp := xcallResult{ErrorCode: 1, ErrorMsg: "tag is locked"}
		respJSON, _ := json.Marshal(resp)
		executor := &mockExecutor{output: respJSON}
		x := newTestXcall(executor)

		err := x.DeleteTag(context.Background(), "tok", "sometag")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "bear error")
	})

	t.Run("exec error", func(t *testing.T) {
		executor := &mockExecutor{err: fmt.Errorf("exit status 1")}
		x := newTestXcall(executor)

		err := x.DeleteTag(context.Background(), "tok", "sometag")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "bear-xcall delete-tag")
	})

	t.Run("invalid JSON response", func(t *testing.T) {
		executor := &mockExecutor{output: []byte("not json")}
		x := newTestXcall(executor)

		err := x.DeleteTag(context.Background(), "tok", "sometag")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid bear-xcall JSON")
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

func TestEncodeParamsUsesPercentEncoding(t *testing.T) {
	t.Run("spaces encoded as %20 not +", func(t *testing.T) {
		resp := xcallResult{Identifier: "ID-1"}
		respJSON, _ := json.Marshal(resp)
		executor := &mockExecutor{output: respJSON}
		x := newTestXcall(executor)

		_, err := x.Create(context.Background(), "tok", "My Note Title", "Hello world body", nil)
		require.NoError(t, err)

		rawURL := executor.calls[0].Args[1]
		// Raw URL must use %20 for spaces, not + (Bear treats + literally)
		assert.NotContains(t, rawURL, "My+Note")
		assert.NotContains(t, rawURL, "Hello+world")
		assert.Contains(t, rawURL, "My%20Note%20Title")
		assert.Contains(t, rawURL, "Hello%20world%20body")

		// Verify values still decode correctly
		parsed, err := url.Parse(rawURL)
		require.NoError(t, err)
		assert.Equal(t, "My Note Title", parsed.Query().Get("title"))
		assert.Equal(t, "Hello world body", parsed.Query().Get("text"))
	})
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
		assert.Contains(t, err.Error(), "invalid bear-xcall JSON")
	})
}

func TestNewWithPathAppBundle(t *testing.T) {
	t.Run("resolves .app bundle to binary inside Contents/MacOS", func(t *testing.T) {
		executor := &mockExecutor{output: []byte(`{}`)}
		x := NewWithPath("/path/to/bear-xcall.app", WithExecutor(executor))

		_ = x.Trash(context.Background(), "tok", "ID")

		require.Len(t, executor.calls, 1)
		assert.Equal(t, "/path/to/bear-xcall.app/Contents/MacOS/bear-xcall", executor.calls[0].Name)
	})

	t.Run("plain binary path used as-is", func(t *testing.T) {
		executor := &mockExecutor{output: []byte(`{}`)}
		x := NewWithPath("/usr/local/bin/bear-xcall", WithExecutor(executor))

		_ = x.Trash(context.Background(), "tok", "ID")

		require.Len(t, executor.calls, 1)
		assert.Equal(t, "/usr/local/bin/bear-xcall", executor.calls[0].Name)
	})
}

func TestMaskTokenInText(t *testing.T) {
	t.Run("masks token in stderr message with bear URL", func(t *testing.T) {
		text := `Failed to open URL (is Bear installed?): bear://x-callback-url/create?token=secret123&title=Hello`
		result := maskTokenInText(text)
		assert.NotContains(t, result, "secret123")
		assert.Contains(t, result, "bear://x-callback-url/create")
	})

	t.Run("no bear URL unchanged", func(t *testing.T) {
		text := "some random error message"
		result := maskTokenInText(text)
		assert.Equal(t, text, result)
	})

	t.Run("bear URL without token unchanged", func(t *testing.T) {
		text := "Failed: bear://x-callback-url/create?title=Hello"
		result := maskTokenInText(text)
		assert.Equal(t, text, result)
	})
}

func TestMaskTokenNotInLogs(t *testing.T) {
	rawURL := "bear://x-callback-url/create?token=my-secret-token-123&title=Test"
	masked := MaskToken(rawURL)

	assert.NotContains(t, masked, "my-secret-token-123")
	assert.Contains(t, masked, "token=")
}
