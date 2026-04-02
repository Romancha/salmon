# Salmon MCP Server

## Overview

MCP (Model Context Protocol) server for the Salmon Hub API. Allows AI assistants (OpenClaw, Claude Code, Cursor, etc.) to interact with Bear notes natively — without shell commands or approval prompts.

- Replaces the current OpenClaw skill (curl-based) with a proper tool integration
- Works with any MCP-compatible client over stdio transport
- Covers all 13 consumer API operations (read + write)

## Context

- **SDK**: `github.com/modelcontextprotocol/go-sdk` (official, Anthropic + Google)
- **Location**: `cmd/mcp/main.go` (entry point) + `internal/mcp/` (logic)
- **Binary**: `salmon-mcp` (consistent with salmon-hub, salmon-run)
- **Auth**: env vars `SALMON_HUB_URL` + `SALMON_CONSUMER_TOKEN`
- **Transport**: stdio only (standard for local MCP servers)
- **Existing patterns**: follows `cmd/hub/`, `cmd/bridge/` structure; tests with testify + httptest

## Development Approach

- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task**
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change
- Maintain backward compatibility

## Testing Strategy

- **Unit tests**: httptest server to mock Salmon Hub API responses
- Test each tool handler: success, error codes (401, 404, 409), malformed responses
- Test HTTP client: timeouts, auth header, URL construction
- Test main.go: env var validation (missing URL, missing token)
- Pattern: table-driven tests with testify (assert/require), consistent with project

## Progress Tracking

- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with + prefix
- Document issues/blockers with ! prefix
- Update plan if implementation deviates from original scope

## Implementation Steps

### Task 1: Project scaffolding and HTTP client

- [x] add `github.com/modelcontextprotocol/go-sdk` dependency (`go get`)
- [x] create `internal/mcp/client.go` — HTTP client struct with `baseURL`, `token`, `http.Client` (30s timeout)
- [x] implement helper methods: `get(ctx, path, query) ([]byte, error)`, `postJSON(ctx, path, body) ([]byte, error)`, `putJSON(ctx, path, body) ([]byte, error)`, `delete(ctx, path) ([]byte, error)`
- [x] handle HTTP errors: map status codes to descriptive error messages (401 → "invalid token", 404 → "not found", 409 → "conflict/not synced", etc.)
- [x] auto-generate `Idempotency-Key` (UUID) for POST/PUT/DELETE requests
- [x] create `internal/mcp/types.go` — Input/Output structs for all 13 tools
- [x] write tests for HTTP client (`internal/mcp/client_test.go`): success, auth header, error codes, timeout
- [x] run `make test` — must pass

### Task 2: Read-only tools (notes)

- [x] create `internal/mcp/tools.go` with `RegisterTools(server, client)` function
- [x] implement `search_notes` tool — `GET /api/notes/search?q=&limit=&tag=`
- [x] implement `get_note` tool — `GET /api/notes/{id}`
- [x] implement `list_notes` tool — `GET /api/notes?tag=&sort=&order=&limit=&trashed=`
- [x] write tests for each tool handler: success response parsing, error propagation
- [x] run `make test` — must pass

### Task 3: Read-only tools (tags, attachments, sync)

- [x] implement `list_tags` tool — `GET /api/tags`
- [x] implement `get_attachment` tool — `GET /api/attachments/{id}` (return base64-encoded content + filename)
- [x] implement `sync_status` tool — `GET /api/sync/status`
- [x] implement `list_backlinks` tool — `GET /api/notes/{id}/backlinks`
- [x] write tests for each tool handler
- [x] run `make test` — must pass

### Task 4: Write tools

- [x] implement `create_note` tool — `POST /api/notes` (title, body, tags)
- [x] implement `update_note` tool — `PUT /api/notes/{id}` (title, body)
- [x] implement `trash_note` tool — `DELETE /api/notes/{id}`
- [x] implement `archive_note` tool — `POST /api/notes/{id}/archive`
- [x] implement `add_tag` tool — `POST /api/notes/{id}/tags` (tag name)
- [x] implement `rename_tag` tool — `PUT /api/tags/{id}` (new_name)
- [x] implement `delete_tag` tool — `DELETE /api/tags/{id}`
- [x] write tests for each write tool: success, 403 (encrypted), 409 (conflict)
- [x] run `make test` — must pass

### Task 5: Entry point and build

- [x] create `cmd/mcp/main.go` — read env vars, validate, create client, create MCP server, register tools, `server.Run()` over stdio
- [x] fail fast with clear error if `SALMON_HUB_URL` or `SALMON_CONSUMER_TOKEN` is missing
- [x] add `build-mcp` target to Makefile: `go build -o bin/salmon-mcp ./cmd/mcp`
- [x] add `salmon-mcp` to the main `build` target
- [x] write test for env var validation (missing vars → error)
- [x] run `make build` — must compile
- [x] run `make test` — must pass

### Task 6: CI/CD

- [x] update `.github/workflows/ci.yml` — ensure `make test` already covers `internal/mcp/` (no changes needed if `go test ./...`)
- [x] update `.github/workflows/docker-publish.yml` — add `salmon-mcp` binary to Docker image (or separate image if preferred)
- [x] verify `make lint` passes on new code
- [x] run `make test-race` — no data races
- [x] run `make test-coverage` — coverage does not decrease

### Task 7: Verify acceptance criteria

- [x] all 13 tools registered and tested (search_notes, get_note, list_notes, list_tags, get_attachment, sync_status, list_backlinks, create_note, update_note, trash_note, archive_note, add_tag, rename_tag, delete_tag)
- [x] `salmon-mcp` binary builds and starts (prints server info on stderr, listens on stdio)
- [x] works with Claude Code: add to `.claude/mcp.json`, verify tool discovery (skipped - requires manual verification with live Claude Code instance)
- [x] run full test suite (`make test`)
- [x] run linter (`make lint`) — all clean
- [x] run race detector (`make test-race`) — no races
- [x] verify test coverage (`make test-coverage`)

### Task 8: Update documentation

- [x] update `README.md` — add "MCP Server" section: what it is, build, configure, connect to clients (Claude Code, OpenClaw)
- [x] add MCP server config examples: `.claude/mcp.json` for Claude Code, `openclaw.json` for OpenClaw
- [x] update `CLAUDE.md` — add `cmd/mcp/` and `internal/mcp/` to project structure, add `make build-mcp` to commands
- [x] update `tools/consumer/openclaw/bear-salmon-notes/README.md` — mention MCP server as preferred alternative

## Technical Details

### Tool definitions (all 13)

| Tool | Method | Endpoint | Input fields | Output |
|------|--------|----------|-------------|--------|
| `search_notes` | GET | `/api/notes/search` | `query` (required), `limit?`, `tag?` | notes array with body |
| `get_note` | GET | `/api/notes/{id}` | `id` (required) | full note + tags + attachments + backlinks |
| `list_notes` | GET | `/api/notes` | `tag?`, `sort?`, `order?`, `limit?`, `trashed?` | notes array without body |
| `list_tags` | GET | `/api/tags` | — | tags array |
| `get_attachment` | GET | `/api/attachments/{id}` | `id` (required) | base64 content + filename + content_type |
| `sync_status` | GET | `/api/sync/status` | — | sync metadata |
| `list_backlinks` | GET | `/api/notes/{id}/backlinks` | `note_id` (required) | backlinks array |
| `create_note` | POST | `/api/notes` | `title` (required), `body?`, `tags?` | created note + queue item |
| `update_note` | PUT | `/api/notes/{id}` | `id` (required), `title?`, `body` (required) | updated note + queue item |
| `trash_note` | DELETE | `/api/notes/{id}` | `id` (required) | queue item |
| `archive_note` | POST | `/api/notes/{id}/archive` | `id` (required) | queue item |
| `add_tag` | POST | `/api/notes/{id}/tags` | `note_id` (required), `tag` (required) | queue item |
| `rename_tag` | PUT | `/api/tags/{id}` | `id` (required), `new_name` (required) | queue item |
| `delete_tag` | DELETE | `/api/tags/{id}` | `id` (required) | queue item |

### Input/Output struct pattern

```go
// Input — Go struct with jsonschema tags, auto-mapped to MCP tool parameters
type SearchNotesInput struct {
    Query string `json:"query" jsonschema:"required,description=Full-text search query"`
    Limit int    `json:"limit,omitempty" jsonschema:"description=Max results (default 20, max 200)"`
    Tag   string `json:"tag,omitempty" jsonschema:"description=Filter by tag name"`
}

// Output — Go struct, serialized as JSON in MCP response
type SearchNotesOutput struct {
    Notes []Note `json:"notes"`
}
```

### Error handling

```go
// HTTP errors → MCP tool errors (returned as mcp.NewToolResultError)
switch resp.StatusCode {
case 401: return "authentication failed: invalid or missing token"
case 403: return "forbidden: encrypted notes are read-only"
case 404: return "not found"
case 409: return "conflict: note not synced to Bear or has unresolved conflicts"
default:  return fmt.Sprintf("hub API error: %d", resp.StatusCode)
}
```

### Client config example (Claude Code)

```json
// .claude/mcp.json
{
  "mcpServers": {
    "salmon": {
      "command": "/path/to/salmon-mcp",
      "env": {
        "SALMON_HUB_URL": "https://salmon.example.com",
        "SALMON_CONSUMER_TOKEN": "your-token"
      }
    }
  }
}
```

### Client config example (OpenClaw)

```json
// openclaw.json — mcpServers section
{
  "mcpServers": {
    "salmon": {
      "command": "/path/to/salmon-mcp",
      "env": {
        "SALMON_HUB_URL": "https://salmon.example.com",
        "SALMON_CONSUMER_TOKEN": "your-token"
      }
    }
  }
}
```

## Post-Completion

**Manual verification:**
- Smoke test with Claude Code: search notes, get note, create note
- Smoke test with OpenClaw: verify tools appear without approval prompts
- Verify `salmon-mcp` doesn't leak token in stderr/logs

**Future considerations:**
- Publish to MCP registries (npm wrapper or standalone binary releases)
- Add `upload_attachment` tool (POST /api/notes/{id}/attachments — multipart, deferred due to complexity)
- SSE/HTTP transport for remote usage (currently stdio only)
- Release workflow for salmon-mcp binary (similar to release-bridge.yml)
