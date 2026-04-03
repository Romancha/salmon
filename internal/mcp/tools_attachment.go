package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/romancha/salmon/internal/models"
)

const attachmentsTempDir = "salmon-attachments"

func handleGetAttachment(
	ctx context.Context, c *Client, input GetAttachmentInput,
) (*gomcp.CallToolResult, GetAttachmentOutput, error) {
	if input.Mode != "" && input.Mode != "file" && input.Mode != "base64" {
		return nil, GetAttachmentOutput{}, fmt.Errorf("invalid mode %q: must be file or base64", input.Mode)
	}

	resp, err := c.getRaw(ctx, "/api/attachments/"+url.PathEscape(input.ID))
	if err != nil {
		return nil, GetAttachmentOutput{}, err
	}

	if input.Mode == "base64" {
		return nil, GetAttachmentOutput{
			ID:          input.ID,
			Filename:    resp.Filename,
			ContentType: resp.ContentType,
			Size:        int64(len(resp.Body)),
			Base64:      base64.StdEncoding.EncodeToString(resp.Body),
		}, nil
	}

	filePath, err := saveToFile(defaultOutputDir(), input.ID, resp.Filename, resp.Body)
	if err != nil {
		return nil, GetAttachmentOutput{}, fmt.Errorf("saving attachment file: %w", err)
	}

	return nil, GetAttachmentOutput{
		ID:          input.ID,
		Filename:    resp.Filename,
		ContentType: resp.ContentType,
		Size:        int64(len(resp.Body)),
		FilePath:    filePath,
	}, nil
}

func handleListAttachments(
	ctx context.Context, c *Client, input ListAttachmentsInput,
) (*gomcp.CallToolResult, ListAttachmentsOutput, error) {
	data, err := c.get(ctx, "/api/notes/"+url.PathEscape(input.NoteID)+"/attachments", nil)
	if err != nil {
		return nil, ListAttachmentsOutput{}, err
	}

	var out ListAttachmentsOutput
	if err := json.Unmarshal(data, &out.Attachments); err != nil {
		return nil, ListAttachmentsOutput{}, fmt.Errorf("parsing attachments: %w", err)
	}

	return nil, out, nil
}

func handleDownloadNoteAttachments(
	ctx context.Context, c *Client, input DownloadNoteAttachmentsInput, //nolint:gocritic // hugeParam: value required by SDK
) (*gomcp.CallToolResult, DownloadNoteAttachmentsOutput, error) {
	data, err := c.get(ctx, "/api/notes/"+url.PathEscape(input.NoteID)+"/attachments", nil)
	if err != nil {
		return nil, DownloadNoteAttachmentsOutput{}, err
	}

	var attachments []models.AttachmentMeta
	if err := json.Unmarshal(data, &attachments); err != nil {
		return nil, DownloadNoteAttachmentsOutput{}, fmt.Errorf("parsing attachments: %w", err)
	}

	typeSet := toStringSet(input.Types)
	extSet := toStringSet(normalizeExtensions(input.Extensions))

	outputDir := input.OutputDir
	if outputDir == "" {
		outputDir = defaultOutputDir()
	}

	downloaded := make([]DownloadedAttachment, 0, len(attachments))
	skipped := 0

	for i := range attachments {
		att := &attachments[i]
		if !matchesFilter(att, typeSet, extSet) {
			skipped++
			continue
		}

		resp, err := c.getRaw(ctx, "/api/attachments/"+url.PathEscape(att.ID))
		if err != nil {
			return nil, DownloadNoteAttachmentsOutput{}, fmt.Errorf("downloading attachment %s: %w", att.ID, err)
		}

		filename := resp.Filename
		if filename == "" {
			filename = att.Filename
		}

		filePath, err := saveToFile(outputDir, att.ID, filename, resp.Body)
		if err != nil {
			return nil, DownloadNoteAttachmentsOutput{}, fmt.Errorf("saving attachment %s: %w", att.ID, err)
		}

		downloaded = append(downloaded, DownloadedAttachment{
			ID:          att.ID,
			Filename:    filename,
			ContentType: resp.ContentType,
			Size:        int64(len(resp.Body)),
			FilePath:    filePath,
		})
	}

	return nil, DownloadNoteAttachmentsOutput{Downloaded: downloaded, Skipped: skipped}, nil
}

// defaultOutputDir returns the default directory for saving attachment files.
func defaultOutputDir() string {
	return filepath.Join(os.TempDir(), attachmentsTempDir)
}

// saveToFile writes data to dir/{id}/{filename}, creating directories as needed.
func saveToFile(dir, id, filename string, data []byte) (string, error) {
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, `/\`) {
		return "", fmt.Errorf("invalid attachment id: %q", id)
	}

	if filename == "" {
		filename = "file"
	}

	dest := filepath.Join(dir, id, filepath.Base(filename))

	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return "", fmt.Errorf("creating directory: %w", err)
	}

	if err := os.WriteFile(dest, data, 0o600); err != nil {
		return "", fmt.Errorf("writing file: %w", err)
	}

	return dest, nil
}

// matchesFilter returns true if the attachment passes both type and extension filters.
// Both filters must match when both are non-empty. Empty filter means accept all.
func matchesFilter(att *models.AttachmentMeta, typeSet, extSet map[string]struct{}) bool {
	if len(typeSet) > 0 {
		if _, ok := typeSet[strings.ToLower(att.Type)]; !ok {
			return false
		}
	}

	if len(extSet) > 0 {
		ext := strings.ToLower(att.NormalizedExtension)
		if _, ok := extSet[ext]; !ok {
			return false
		}
	}

	return true
}

// toStringSet converts a string slice to a lowercase lookup map.
func toStringSet(ss []string) map[string]struct{} {
	m := make(map[string]struct{}, len(ss))
	for _, s := range ss {
		m[strings.ToLower(s)] = struct{}{}
	}
	return m
}

// normalizeExtensions strips leading dots from extensions.
func normalizeExtensions(exts []string) []string {
	out := make([]string, len(exts))
	for i, e := range exts {
		out[i] = strings.TrimPrefix(strings.TrimSpace(e), ".")
	}
	return out
}
