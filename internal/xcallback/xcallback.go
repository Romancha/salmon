package xcallback

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// XCallback defines the interface for executing Bear x-callback-url actions via xcall CLI.
type XCallback interface {
	// Create creates a new note in Bear and returns the bear_id (UUID) from x-success response.
	Create(ctx context.Context, token, title, body string, tags []string) (string, error)

	// Update replaces the body of an existing note in Bear.
	Update(ctx context.Context, token, bearID, body string) error

	// AddTag adds a tag to an existing note in Bear.
	AddTag(ctx context.Context, token, bearID, tag string) error

	// Trash moves a note to trash in Bear.
	Trash(ctx context.Context, token, bearID string) error
}

//go:generate moq -out xcallback_mock.go . XCallback

// xcallResult represents the JSON response from xcall CLI.
type xcallResult struct {
	Identifier string `json:"identifier,omitempty"`
	Title      string `json:"title,omitempty"`
	Note       string `json:"note,omitempty"`
	ErrorCode  int    `json:"errorCode,omitempty"`
	ErrorMsg   string `json:"errorMessage,omitempty"`
}

// CommandExecutor abstracts os/exec for testing.
type CommandExecutor interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type defaultExecutor struct{}

func (e *defaultExecutor) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // name is xcall path validated at init
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("exec %s: %w", name, err)
	}
	return out, nil
}

// Xcall implements XCallback by invoking the xcall CLI tool.
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
// by looking next to the running executable, then falling back to PATH.
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

	x.logger.Info("xcallback initialized", "xcall_path", x.xcallPath)

	return x, nil
}

// resolveBearXcallPath finds the bear-xcall binary inside the .app bundle.
// It first checks next to the running executable, then falls back to PATH.
func resolveBearXcallPath() (string, error) {
	// Try next to the running executable (e.g., bin/bear-xcall.app/Contents/MacOS/bear-xcall).
	exe, err := os.Executable()
	if err == nil {
		binDir := filepath.Dir(exe)
		appBinary := filepath.Join(binDir, "bear-xcall.app", "Contents", "MacOS", "bear-xcall")
		if _, err := os.Stat(appBinary); err == nil {
			return appBinary, nil
		}
	}

	// Fallback: look for bear-xcall in PATH.
	path, err := exec.LookPath("bear-xcall")
	if err != nil {
		return "", fmt.Errorf("bear-xcall.app not found next to executable and not in PATH: %w", err)
	}
	return path, nil
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

	callURL := "bear://x-callback-url/create?" + params.Encode()

	x.logger.Debug("executing xcall create", "url", MaskToken(callURL))

	output, err := x.executor.Run(ctx, x.xcallPath, "-url", callURL)
	if err != nil {
		return "", fmt.Errorf("xcall create: %w", err)
	}

	result, err := parseXcallResult(output)
	if err != nil {
		return "", fmt.Errorf("xcall create parse response: %w", err)
	}

	if result.ErrorCode != 0 {
		return "", fmt.Errorf("xcall create bear error: code=%d msg=%s", result.ErrorCode, result.ErrorMsg)
	}

	if result.Identifier == "" {
		// Return empty ID without error so the caller can attempt fallback verification.
		x.logger.Warn("xcall create: empty identifier in response")
		return "", nil
	}

	x.logger.Info("xcall create succeeded", "bear_id", result.Identifier)

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

	callURL := "bear://x-callback-url/add-text?" + params.Encode()

	x.logger.Debug("executing xcall update", "url", MaskToken(callURL), "bear_id", bearID)

	output, err := x.executor.Run(ctx, x.xcallPath, "-url", callURL)
	if err != nil {
		return fmt.Errorf("xcall update: %w", err)
	}

	result, err := parseXcallResult(output)
	if err != nil {
		return fmt.Errorf("xcall update parse response: %w", err)
	}

	if result.ErrorCode != 0 {
		return fmt.Errorf("xcall update bear error: code=%d msg=%s", result.ErrorCode, result.ErrorMsg)
	}

	x.logger.Info("xcall update succeeded", "bear_id", bearID)

	return nil
}

func (x *Xcall) AddTag(ctx context.Context, token, bearID, tag string) error {
	params := url.Values{}
	params.Set("token", token)
	params.Set("id", bearID)
	params.Set("tags", tag)
	params.Set("show_window", "no")
	params.Set("open_note", "no")

	callURL := "bear://x-callback-url/add-tag?" + params.Encode()

	x.logger.Debug("executing xcall add-tag", "url", MaskToken(callURL), "bear_id", bearID, "tag", tag)

	output, err := x.executor.Run(ctx, x.xcallPath, "-url", callURL)
	if err != nil {
		return fmt.Errorf("xcall add-tag: %w", err)
	}

	result, err := parseXcallResult(output)
	if err != nil {
		return fmt.Errorf("xcall add-tag parse response: %w", err)
	}

	if result.ErrorCode != 0 {
		return fmt.Errorf("xcall add-tag bear error: code=%d msg=%s", result.ErrorCode, result.ErrorMsg)
	}

	x.logger.Info("xcall add-tag succeeded", "bear_id", bearID, "tag", tag)

	return nil
}

func (x *Xcall) Trash(ctx context.Context, token, bearID string) error {
	params := url.Values{}
	params.Set("token", token)
	params.Set("id", bearID)
	params.Set("show_window", "no")

	callURL := "bear://x-callback-url/trash?" + params.Encode()

	x.logger.Debug("executing xcall trash", "url", MaskToken(callURL), "bear_id", bearID)

	output, err := x.executor.Run(ctx, x.xcallPath, "-url", callURL)
	if err != nil {
		return fmt.Errorf("xcall trash: %w", err)
	}

	result, err := parseXcallResult(output)
	if err != nil {
		return fmt.Errorf("xcall trash parse response: %w", err)
	}

	if result.ErrorCode != 0 {
		return fmt.Errorf("xcall trash bear error: code=%d msg=%s", result.ErrorCode, result.ErrorMsg)
	}

	x.logger.Info("xcall trash succeeded", "bear_id", bearID)

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

func parseXcallResult(output []byte) (*xcallResult, error) {
	if len(output) == 0 {
		return &xcallResult{}, nil
	}

	var result xcallResult
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("invalid xcall JSON response: %w", err)
	}

	return &result, nil
}
