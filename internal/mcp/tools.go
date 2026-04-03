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
	registerCreateNote(s, c)
	registerUpdateNote(s, c)
	registerTrashNote(s, c)
	registerArchiveNote(s, c)
	registerAddTag(s, c)
	registerRenameTag(s, c)
	registerDeleteTag(s, c)
}

func registerSearchNotes(s *mcp.Server, c *Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "search_notes",
		Description: "Full-text search across Bear notes. " +
			"Returns matching notes with full body content.",
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
		Description: "List Bear notes with filtering, sorting, and pagination. " +
			"Does NOT include body — use get_note or search_notes to read content.",
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
	data, err := c.get(ctx, "/api/notes/"+url.PathEscape(input.ID), nil)
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
	resp, err := c.getRaw(ctx, "/api/attachments/"+url.PathEscape(input.ID))
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
	data, err := c.get(ctx, "/api/notes/"+url.PathEscape(input.NoteID)+"/backlinks", nil)
	if err != nil {
		return nil, ListBacklinksOutput{}, err
	}

	var out ListBacklinksOutput
	if err := json.Unmarshal(data, &out.Backlinks); err != nil {
		return nil, ListBacklinksOutput{}, fmt.Errorf("parsing backlinks: %w", err)
	}

	return nil, out, nil
}

// --- Write tools ---

func registerCreateNote(s *mcp.Server, c *Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "create_note",
		Description: "Create a new Bear note. Body is Markdown. " +
			"Do NOT put #tags in body if also passing them in tags array. " +
			"Returns 403 for encrypted notes.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input CreateNoteInput) (*mcp.CallToolResult, CreateNoteOutput, error) {
		return handleCreateNote(ctx, c, input)
	})
}

func registerUpdateNote(s *mcp.Server, c *Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "update_note",
		Description: "Update a Bear note's title and/or body (Markdown). " +
			"Returns 403 for encrypted notes, 409 if conflicts or not synced.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input UpdateNoteInput) (*mcp.CallToolResult, UpdateNoteOutput, error) {
		return handleUpdateNote(ctx, c, input)
	})
}

func registerTrashNote(s *mcp.Server, c *Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "trash_note",
		Description: "Move a Bear note to trash. Returns 403 for encrypted notes, 409 if note has unresolved conflicts.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input TrashNoteInput) (*mcp.CallToolResult, TrashNoteOutput, error) {
		return handleTrashNote(ctx, c, input)
	})
}

func registerArchiveNote(s *mcp.Server, c *Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "archive_note",
		Description: "Archive a Bear note. Returns 403 for encrypted notes, 409 if note has unresolved conflicts.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input ArchiveNoteInput) (*mcp.CallToolResult, ArchiveNoteOutput, error) {
		return handleArchiveNote(ctx, c, input)
	})
}

func registerAddTag(s *mcp.Server, c *Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "add_tag",
		Description: "Add a tag to a Bear note. Returns 403 for encrypted notes, 409 if note has unresolved conflicts.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input AddTagInput) (*mcp.CallToolResult, AddTagOutput, error) {
		return handleAddTag(ctx, c, input)
	})
}

func registerRenameTag(s *mcp.Server, c *Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "rename_tag",
		Description: "Rename an existing tag across all Bear notes",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input RenameTagInput) (*mcp.CallToolResult, RenameTagOutput, error) {
		return handleRenameTag(ctx, c, input)
	})
}

func registerDeleteTag(s *mcp.Server, c *Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "delete_tag",
		Description: "Remove a tag from all Bear notes (does not delete the notes themselves)",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input DeleteTagInput) (*mcp.CallToolResult, DeleteTagOutput, error) {
		return handleDeleteTag(ctx, c, input)
	})
}

func handleCreateNote(
	ctx context.Context, c *Client, input CreateNoteInput,
) (*mcp.CallToolResult, CreateNoteOutput, error) {
	body := map[string]any{"title": input.Title}
	if input.Body != "" {
		body["body"] = input.Body
	}
	if len(input.Tags) > 0 {
		body["tags"] = input.Tags
	}

	data, err := c.postJSON(ctx, "/api/notes", body)
	if err != nil {
		return nil, CreateNoteOutput{}, err
	}

	var out CreateNoteOutput
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, CreateNoteOutput{}, fmt.Errorf("parsing created note: %w", err)
	}

	return nil, out, nil
}

func handleUpdateNote(
	ctx context.Context, c *Client, input UpdateNoteInput,
) (*mcp.CallToolResult, UpdateNoteOutput, error) {
	body := map[string]any{"body": input.Body}
	if input.Title != "" {
		body["title"] = input.Title
	}

	data, err := c.putJSON(ctx, "/api/notes/"+url.PathEscape(input.ID), body)
	if err != nil {
		return nil, UpdateNoteOutput{}, err
	}

	var out UpdateNoteOutput
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, UpdateNoteOutput{}, fmt.Errorf("parsing updated note: %w", err)
	}

	return nil, out, nil
}

func handleTrashNote(
	ctx context.Context, c *Client, input TrashNoteInput,
) (*mcp.CallToolResult, TrashNoteOutput, error) {
	data, err := c.delete(ctx, "/api/notes/"+url.PathEscape(input.ID))
	if err != nil {
		return nil, TrashNoteOutput{}, err
	}

	var out TrashNoteOutput
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, TrashNoteOutput{}, fmt.Errorf("parsing trashed note: %w", err)
	}

	return nil, out, nil
}

func handleArchiveNote(
	ctx context.Context, c *Client, input ArchiveNoteInput,
) (*mcp.CallToolResult, ArchiveNoteOutput, error) {
	data, err := c.postJSON(ctx, "/api/notes/"+url.PathEscape(input.ID)+"/archive", struct{}{})
	if err != nil {
		return nil, ArchiveNoteOutput{}, err
	}

	var out ArchiveNoteOutput
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, ArchiveNoteOutput{}, fmt.Errorf("parsing archived note: %w", err)
	}

	return nil, out, nil
}

func handleAddTag(
	ctx context.Context, c *Client, input AddTagInput,
) (*mcp.CallToolResult, AddTagOutput, error) {
	body := map[string]string{"tag": input.Tag}

	data, err := c.postJSON(ctx, "/api/notes/"+url.PathEscape(input.NoteID)+"/tags", body)
	if err != nil {
		return nil, AddTagOutput{}, err
	}

	var out AddTagOutput
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, AddTagOutput{}, fmt.Errorf("parsing add tag result: %w", err)
	}

	return nil, out, nil
}

func handleRenameTag(
	ctx context.Context, c *Client, input RenameTagInput,
) (*mcp.CallToolResult, RenameTagOutput, error) {
	body := map[string]string{"new_name": input.NewName}

	data, err := c.putJSON(ctx, "/api/tags/"+url.PathEscape(input.ID), body)
	if err != nil {
		return nil, RenameTagOutput{}, err
	}

	var out RenameTagOutput
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, RenameTagOutput{}, fmt.Errorf("parsing rename tag result: %w", err)
	}

	return nil, out, nil
}

func handleDeleteTag(
	ctx context.Context, c *Client, input DeleteTagInput,
) (*mcp.CallToolResult, DeleteTagOutput, error) {
	data, err := c.delete(ctx, "/api/tags/"+url.PathEscape(input.ID))
	if err != nil {
		return nil, DeleteTagOutput{}, err
	}

	var out DeleteTagOutput
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, DeleteTagOutput{}, fmt.Errorf("parsing delete tag result: %w", err)
	}

	return nil, out, nil
}
