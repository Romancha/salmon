package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterTools registers all Salmon MCP tools on the given server.
func RegisterTools(s *mcp.Server, c *Client) {
	registerSearchNotes(s, c)
	registerGetNote(s, c)
	registerListNotes(s, c)
	registerListTags(s, c)
	registerGetAttachment(s, c)
	registerSyncStatus(s, c)
	registerListBacklinks(s, c)
}

func registerSearchNotes(s *mcp.Server, c *Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "search_notes",
		Description: "Full-text search across Bear notes (titles and bodies)",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input SearchNotesInput) (*mcp.CallToolResult, SearchNotesOutput, error) {
		return handleSearchNotes(ctx, c, input)
	})
}

func registerGetNote(s *mcp.Server, c *Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_note",
		Description: "Get a single Bear note by ID with full body, tags, attachments, and backlinks",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input GetNoteInput) (*mcp.CallToolResult, GetNoteOutput, error) {
		return handleGetNote(ctx, c, input)
	})
}

func registerListNotes(s *mcp.Server, c *Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_notes",
		Description: "List Bear notes (without body). Supports filtering by tag, sorting, and pagination",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input ListNotesInput) (*mcp.CallToolResult, ListNotesOutput, error) {
		return handleListNotes(ctx, c, input)
	})
}

func handleSearchNotes(ctx context.Context, c *Client, input SearchNotesInput) (*mcp.CallToolResult, SearchNotesOutput, error) {
	q := url.Values{}
	q.Set("q", input.Query)
	if input.Limit > 0 {
		q.Set("limit", strconv.Itoa(input.Limit))
	}
	if input.Tag != "" {
		q.Set("tag", input.Tag)
	}

	data, err := c.get(ctx, "/api/notes/search", q)
	if err != nil {
		return nil, SearchNotesOutput{}, err
	}

	var out SearchNotesOutput
	if err := json.Unmarshal(data, &out.Notes); err != nil {
		return nil, SearchNotesOutput{}, fmt.Errorf("parsing search results: %w", err)
	}

	return nil, out, nil
}

func handleGetNote(ctx context.Context, c *Client, input GetNoteInput) (*mcp.CallToolResult, GetNoteOutput, error) {
	data, err := c.get(ctx, "/api/notes/"+input.ID, nil)
	if err != nil {
		return nil, GetNoteOutput{}, err
	}

	var out GetNoteOutput
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, GetNoteOutput{}, fmt.Errorf("parsing note: %w", err)
	}

	return nil, out, nil
}

func registerListTags(s *mcp.Server, c *Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_tags",
		Description: "List all tags from Bear notes",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input ListTagsInput) (*mcp.CallToolResult, ListTagsOutput, error) {
		return handleListTags(ctx, c, input)
	})
}

func registerGetAttachment(s *mcp.Server, c *Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_attachment",
		Description: "Download a Bear note attachment by ID. Returns base64-encoded content with filename and content type",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input GetAttachmentInput) (*mcp.CallToolResult, GetAttachmentOutput, error) {
		return handleGetAttachment(ctx, c, input)
	})
}

func registerSyncStatus(s *mcp.Server, c *Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "sync_status",
		Description: "Get the current sync status between Bear and Salmon Hub",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input SyncStatusInput) (*mcp.CallToolResult, SyncStatusOutput, error) {
		return handleSyncStatus(ctx, c, input)
	})
}

func registerListBacklinks(s *mcp.Server, c *Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_backlinks",
		Description: "List all notes that link to a given note (backlinks)",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input ListBacklinksInput) (*mcp.CallToolResult, ListBacklinksOutput, error) {
		return handleListBacklinks(ctx, c, input)
	})
}

func handleListNotes(ctx context.Context, c *Client, input ListNotesInput) (*mcp.CallToolResult, ListNotesOutput, error) {
	q := url.Values{}
	if input.Tag != "" {
		q.Set("tag", input.Tag)
	}
	if input.Sort != "" {
		q.Set("sort", input.Sort)
	}
	if input.Order != "" {
		q.Set("order", input.Order)
	}
	if input.Limit > 0 {
		q.Set("limit", strconv.Itoa(input.Limit))
	}
	if input.Trashed != "" {
		q.Set("trashed", input.Trashed)
	}

	data, err := c.get(ctx, "/api/notes", q)
	if err != nil {
		return nil, ListNotesOutput{}, err
	}

	var out ListNotesOutput
	if err := json.Unmarshal(data, &out.Notes); err != nil {
		return nil, ListNotesOutput{}, fmt.Errorf("parsing notes list: %w", err)
	}

	return nil, out, nil
}

func handleListTags(ctx context.Context, c *Client, _ ListTagsInput) (*mcp.CallToolResult, ListTagsOutput, error) {
	data, err := c.get(ctx, "/api/tags", nil)
	if err != nil {
		return nil, ListTagsOutput{}, err
	}

	var out ListTagsOutput
	if err := json.Unmarshal(data, &out.Tags); err != nil {
		return nil, ListTagsOutput{}, fmt.Errorf("parsing tags list: %w", err)
	}

	return nil, out, nil
}

func handleGetAttachment(ctx context.Context, c *Client, input GetAttachmentInput) (*mcp.CallToolResult, GetAttachmentOutput, error) {
	resp, err := c.getRaw(ctx, "/api/attachments/"+input.ID)
	if err != nil {
		return nil, GetAttachmentOutput{}, err
	}

	encoded := base64.StdEncoding.EncodeToString(resp.Body)

	return nil, GetAttachmentOutput{
		ID:          input.ID,
		Filename:    resp.Filename,
		ContentType: resp.ContentType,
		Base64:      encoded,
	}, nil
}

func handleSyncStatus(ctx context.Context, c *Client, _ SyncStatusInput) (*mcp.CallToolResult, SyncStatusOutput, error) {
	data, err := c.get(ctx, "/api/sync/status", nil)
	if err != nil {
		return nil, SyncStatusOutput{}, err
	}

	var out SyncStatusOutput
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, SyncStatusOutput{}, fmt.Errorf("parsing sync status: %w", err)
	}

	return nil, out, nil
}

func handleListBacklinks(
	ctx context.Context, c *Client, input ListBacklinksInput,
) (*mcp.CallToolResult, ListBacklinksOutput, error) {
	data, err := c.get(ctx, "/api/notes/"+input.NoteID+"/backlinks", nil)
	if err != nil {
		return nil, ListBacklinksOutput{}, err
	}

	var out ListBacklinksOutput
	if err := json.Unmarshal(data, &out.Backlinks); err != nil {
		return nil, ListBacklinksOutput{}, fmt.Errorf("parsing backlinks: %w", err)
	}

	return nil, out, nil
}
