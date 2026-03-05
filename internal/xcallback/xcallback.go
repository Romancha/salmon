package xcallback

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// maxAddFileSize is the maximum raw file size for AddFile (5 MB).
// Bear accepts base64-encoded data in the URL, which expands ~33%.
const maxAddFileSize = 5 * 1024 * 1024

// XCallback defines the interface for executing Bear x-callback-url actions via bear-xcall CLI.
type XCallback interface {
	// Create creates a new note in Bear and returns the bear_id (UUID) from x-success response.
	Create(ctx context.Context, token, title, body string, tags []string) (string, error)

	// Update replaces the body of an existing note in Bear.
	Update(ctx context.Context, token, bearID, body string) error

	// AddTag adds a tag to an existing note in Bear.
	AddTag(ctx context.Context, token, bearID, tag string) error

	// Trash moves a note to trash in Bear.
	Trash(ctx context.Context, token, bearID string) error

	// AddFile attaches a file to an existing note in Bear.
	// fileData must not exceed 5 MB (maxAddFileSize).
	AddFile(ctx context.Context, token, bearID, filename string, fileData []byte) error

	// Archive moves a note to the archive in Bear.
	Archive(ctx context.Context, token, bearID string) error

	// RenameTag renames a tag in Bear. All notes with the old tag are updated.
	RenameTag(ctx context.Context, token, oldName, newName string) error

	// DeleteTag deletes a tag from all notes in Bear.
	DeleteTag(ctx context.Context, token, tagName string) error
}

//go:generate moq -out xcallback_mock.go . XCallback

// xcallResult represents the JSON response from bear-xcall CLI.
type xcallResult struct {
	Identifier string `json:"identifier,omitempty"`
	Title      string `json:"title,omitempty"`
	Note       string `json:"note,omitempty"`
	ErrorCode  int    `json:"errorCode,omitempty"`
	ErrorMsg   string `json:"errorMessage,omitempty"`
}

// BearError represents an error returned by Bear via x-callback-url.
// The Code field contains Bear's numeric error code.
type BearError struct {
	Code int
	Msg  string
}

func (e *BearError) Error() string {
	return fmt.Sprintf("bear error: code=%d msg=%s", e.Code, e.Msg)
}

// CommandExecutor abstracts os/exec for testing.
type CommandExecutor interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
	// RunWithStdin runs a command with the given stdin data piped to the process.
	// Used by AddFile to bypass macOS ARG_MAX (1 MB) when passing large base64-encoded URLs.
	RunWithStdin(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error)
}

type defaultExecutor struct{}

func (e *defaultExecutor) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return e.runCmd(ctx, nil, name, args...)
}

func (e *defaultExecutor) RunWithStdin(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
	return e.runCmd(ctx, stdin, name, args...)
}

func (e *defaultExecutor) runCmd(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // name is bear-xcall path validated at init
	if stdin != nil {
		cmd.Stdin = stdin
	}
	out, err := cmd.Output()
	if err != nil {
		// bear-xcall writes structured error JSON to stdout even on non-zero exit.
		// Return stdout if available so the caller can parse Bear error details.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if len(out) > 0 {
				return out, nil
			}
			// stdout empty — include stderr detail (e.g. "Failed to open URL") in the error.
			// Mask any token values in stderr to prevent secret leakage in logs/hub storage.
			if len(exitErr.Stderr) > 0 {
				stderrMsg := maskTokenInText(strings.TrimSpace(string(exitErr.Stderr)))
				return nil, fmt.Errorf("exec %s: %w: %s", name, err, stderrMsg)
			}
		}
		return nil, fmt.Errorf("exec %s: %w", name, err)
	}
	return out, nil
}

// Xcall implements XCallback by invoking the bear-xcall CLI tool.
type Xcall struct {
	xcallPath string
	executor  CommandExecutor
	logger    *slog.Logger
}

// Option configures an Xcall instance.
type Option func(*Xcall)

// WithExecutor sets a custom command executor (for testing).
func WithExecutor(e CommandExecutor) Option {
	return func(x *Xcall) {
		x.executor = e
	}
}

// WithLogger sets a custom logger.
func WithLogger(l *slog.Logger) Option {
	return func(x *Xcall) {
		x.logger = l
	}
}

// New creates a new Xcall instance. It resolves the bear-xcall.app bundle path
// by looking next to the running executable.
func New(opts ...Option) (*Xcall, error) {
	x := &Xcall{
		executor: &defaultExecutor{},
		logger:   slog.Default(),
	}

	for _, opt := range opts {
		opt(x)
	}

	path, err := resolveBearXcallPath()
	if err != nil {
		return nil, fmt.Errorf("bear-xcall not found: %w", err)
	}
	x.xcallPath = path

	x.logger.Info("xcallback initialized", "bear_xcall_path", x.xcallPath)

	return x, nil
}

// resolveBearXcallPath finds the bear-xcall binary inside the .app bundle
// next to the running executable. The .app bundle structure is required for
// macOS LaunchServices to route bear-xcall:// callback URLs.
func resolveBearXcallPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot determine executable path: %w", err)
	}

	binDir := filepath.Dir(exe)
	appBinary := filepath.Join(binDir, "bear-xcall.app", "Contents", "MacOS", "bear-xcall")
	if _, err := os.Stat(appBinary); err != nil {
		return "", fmt.Errorf("bear-xcall.app not found at %s (run 'make build-xcall'): %w", filepath.Join(binDir, "bear-xcall.app"), err)
	}

	return appBinary, nil
}

// NewWithPath creates a new Xcall instance with an explicit path (skips LookPath).
// If the path ends with ".app", it resolves to the binary inside Contents/MacOS/.
func NewWithPath(xcallPath string, opts ...Option) *Xcall {
	resolved := xcallPath
	if strings.HasSuffix(xcallPath, ".app") {
		resolved = filepath.Join(xcallPath, "Contents", "MacOS", "bear-xcall")
	}

	x := &Xcall{
		xcallPath: resolved,
		executor:  &defaultExecutor{},
		logger:    slog.Default(),
	}

	for _, opt := range opts {
		opt(x)
	}

	return x
}

func (x *Xcall) Create(ctx context.Context, token, title, body string, tags []string) (string, error) {
	params := url.Values{}
	params.Set("token", token)
	params.Set("title", title)
	params.Set("text", body)
	if len(tags) > 0 {
		params.Set("tags", strings.Join(tags, ","))
	}
	params.Set("show_window", "no")
	params.Set("open_note", "no")

	callURL := "bear://x-callback-url/create?" + encodeParams(params)

	x.logger.Debug("executing bear-xcall create", "url", MaskToken(callURL))

	output, err := x.executor.Run(ctx, x.xcallPath, "-url", callURL)
	if err != nil {
		return "", fmt.Errorf("bear-xcall create: %w", err)
	}

	result, err := parseXcallResult(output)
	if err != nil {
		return "", fmt.Errorf("bear-xcall create parse response: %w", err)
	}

	if result.ErrorCode != 0 {
		return "", fmt.Errorf("bear-xcall create: %w", &BearError{Code: result.ErrorCode, Msg: result.ErrorMsg})
	}

	if result.Identifier == "" {
		// Return empty ID without error so the caller can attempt fallback verification.
		x.logger.Warn("bear-xcall create: empty identifier in response")
		return "", nil
	}

	x.logger.Info("bear-xcall create succeeded", "bear_id", result.Identifier)

	return result.Identifier, nil
}

func (x *Xcall) Update(ctx context.Context, token, bearID, body string) error {
	params := url.Values{}
	params.Set("token", token)
	params.Set("id", bearID)
	params.Set("text", body)
	params.Set("mode", "replace")
	params.Set("show_window", "no")
	params.Set("open_note", "no")

	callURL := "bear://x-callback-url/add-text?" + encodeParams(params)

	x.logger.Debug("executing bear-xcall update", "url", MaskToken(callURL), "bear_id", bearID)

	output, err := x.executor.Run(ctx, x.xcallPath, "-url", callURL)
	if err != nil {
		return fmt.Errorf("bear-xcall update: %w", err)
	}

	result, err := parseXcallResult(output)
	if err != nil {
		return fmt.Errorf("bear-xcall update parse response: %w", err)
	}

	if result.ErrorCode != 0 {
		return fmt.Errorf("bear-xcall update: %w", &BearError{Code: result.ErrorCode, Msg: result.ErrorMsg})
	}

	x.logger.Info("bear-xcall update succeeded", "bear_id", bearID)

	return nil
}

func (x *Xcall) AddTag(ctx context.Context, token, bearID, tag string) error {
	params := url.Values{}
	params.Set("token", token)
	params.Set("id", bearID)
	params.Set("tags", tag)
	params.Set("show_window", "no")
	params.Set("open_note", "no")

	callURL := "bear://x-callback-url/add-text?" + encodeParams(params)

	x.logger.Debug("executing bear-xcall add-tag", "url", MaskToken(callURL), "bear_id", bearID, "tag", tag)

	output, err := x.executor.Run(ctx, x.xcallPath, "-url", callURL)
	if err != nil {
		return fmt.Errorf("bear-xcall add-tag: %w", err)
	}

	result, err := parseXcallResult(output)
	if err != nil {
		return fmt.Errorf("bear-xcall add-tag parse response: %w", err)
	}

	if result.ErrorCode != 0 {
		return fmt.Errorf("bear-xcall add-tag: %w", &BearError{Code: result.ErrorCode, Msg: result.ErrorMsg})
	}

	x.logger.Info("bear-xcall add-tag succeeded", "bear_id", bearID, "tag", tag)

	return nil
}

func (x *Xcall) Trash(ctx context.Context, token, bearID string) error {
	params := url.Values{}
	params.Set("token", token)
	params.Set("id", bearID)
	params.Set("show_window", "no")

	callURL := "bear://x-callback-url/trash?" + encodeParams(params)

	x.logger.Debug("executing bear-xcall trash", "url", MaskToken(callURL), "bear_id", bearID)

	if err := x.executeAction(ctx, "trash", callURL); err != nil {
		return err
	}

	x.logger.Info("bear-xcall trash succeeded", "bear_id", bearID)

	return nil
}

func (x *Xcall) AddFile(ctx context.Context, token, bearID, filename string, fileData []byte) error {
	if len(fileData) > maxAddFileSize {
		return fmt.Errorf("bear-xcall add-file: file size %d exceeds limit %d bytes", len(fileData), maxAddFileSize)
	}

	encoded := base64.StdEncoding.EncodeToString(fileData)

	params := url.Values{}
	params.Set("token", token)
	params.Set("id", bearID)
	params.Set("filename", filename)
	params.Set("file", encoded)
	params.Set("show_window", "no")
	params.Set("open_note", "no")

	callURL := "bear://x-callback-url/add-file?" + encodeParams(params)

	x.logger.Debug("executing bear-xcall add-file", "bear_id", bearID, "filename", filename, "url_len", len(callURL))

	// Pipe URL via stdin ("-url -") to bypass macOS ARG_MAX (1 MB) limit.
	// A 5 MB file produces ~6.7 MB of base64 in the URL, far exceeding ARG_MAX.
	output, err := x.executor.RunWithStdin(ctx, bytes.NewReader([]byte(callURL)), x.xcallPath, "-url", "-")
	if err != nil {
		return fmt.Errorf("bear-xcall add-file: %w", err)
	}

	result, err := parseXcallResult(output)
	if err != nil {
		return fmt.Errorf("bear-xcall add-file parse response: %w", err)
	}

	if result.ErrorCode != 0 {
		return fmt.Errorf("bear-xcall add-file: %w", &BearError{Code: result.ErrorCode, Msg: result.ErrorMsg})
	}

	x.logger.Info("bear-xcall add-file succeeded", "bear_id", bearID, "filename", filename)

	return nil
}

func (x *Xcall) Archive(ctx context.Context, token, bearID string) error {
	params := url.Values{}
	params.Set("token", token)
	params.Set("id", bearID)
	params.Set("show_window", "no")

	callURL := "bear://x-callback-url/archive?" + encodeParams(params)

	x.logger.Debug("executing bear-xcall archive", "url", MaskToken(callURL), "bear_id", bearID)

	if err := x.executeAction(ctx, "archive", callURL); err != nil {
		return err
	}

	x.logger.Info("bear-xcall archive succeeded", "bear_id", bearID)

	return nil
}

func (x *Xcall) RenameTag(ctx context.Context, token, oldName, newName string) error {
	params := url.Values{}
	params.Set("token", token)
	params.Set("name", oldName)
	params.Set("new_name", newName)
	params.Set("show_window", "no")

	callURL := "bear://x-callback-url/rename-tag?" + encodeParams(params)

	x.logger.Debug("executing bear-xcall rename-tag", "url", MaskToken(callURL), "old_name", oldName, "new_name", newName)

	if err := x.executeAction(ctx, "rename-tag", callURL); err != nil {
		return err
	}

	x.logger.Info("bear-xcall rename-tag succeeded", "old_name", oldName, "new_name", newName)

	return nil
}

func (x *Xcall) DeleteTag(ctx context.Context, token, tagName string) error {
	params := url.Values{}
	params.Set("token", token)
	params.Set("name", tagName)
	params.Set("show_window", "no")

	callURL := "bear://x-callback-url/delete-tag?" + encodeParams(params)

	x.logger.Debug("executing bear-xcall delete-tag", "url", MaskToken(callURL), "tag_name", tagName)

	if err := x.executeAction(ctx, "delete-tag", callURL); err != nil {
		return err
	}

	x.logger.Info("bear-xcall delete-tag succeeded", "tag_name", tagName)

	return nil
}

// MaskToken replaces the token value in a URL string with "***" for safe logging.
func MaskToken(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	q := parsed.Query()
	if q.Get("token") != "" {
		q.Set("token", "***")
	}
	parsed.RawQuery = q.Encode()

	return parsed.String()
}

// maskTokenInText finds a bear:// URL in text and masks the token parameter.
// This prevents secret leakage when stderr content is included in error messages
// that may be logged or sent to the hub.
func maskTokenInText(text string) string {
	const bearPrefix = "bear://"
	idx := strings.Index(text, bearPrefix)
	if idx < 0 {
		return text
	}
	// Find the end of the URL (space, newline, or end of string).
	end := len(text)
	for i := idx; i < len(text); i++ {
		if text[i] == ' ' || text[i] == '\n' || text[i] == '\t' {
			end = i
			break
		}
	}
	rawURL := text[idx:end]
	masked := MaskToken(rawURL)
	return text[:idx] + masked + text[end:]
}

// executeAction runs a bear-xcall action URL, parses the response, and checks for Bear errors.
// Used by simple actions (trash, archive, delete-tag) that take a single URL and return no data.
func (x *Xcall) executeAction(ctx context.Context, action, callURL string) error {
	output, err := x.executor.Run(ctx, x.xcallPath, "-url", callURL)
	if err != nil {
		return fmt.Errorf("bear-xcall %s: %w", action, err)
	}

	result, err := parseXcallResult(output)
	if err != nil {
		return fmt.Errorf("bear-xcall %s parse response: %w", action, err)
	}

	if result.ErrorCode != 0 {
		return fmt.Errorf("bear-xcall %s: %w", action, &BearError{Code: result.ErrorCode, Msg: result.ErrorMsg})
	}

	return nil
}

// encodeParams encodes url.Values for use in x-callback-url schemes.
// url.Values.Encode() produces application/x-www-form-urlencoded where spaces are "+".
// Bear's x-callback-url handler treats "+" literally, so we must use percent-encoding (%20).
func encodeParams(v url.Values) string {
	return strings.ReplaceAll(v.Encode(), "+", "%20")
}

func parseXcallResult(output []byte) (*xcallResult, error) {
	if len(output) == 0 {
		return &xcallResult{}, nil
	}

	var result xcallResult
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("invalid bear-xcall JSON response: %w", err)
	}

	return &result, nil
}
