---
name: bear-salmon-notes
version: 1.0.0
description: Read and write Bear (https://bear.app) notes via the Salmon Hub API. List, search, create, update, trash, archive notes. Manage tags. Download attachments. View backlinks and sync status.
homepage: https://github.com/romancha/salmon
metadata: {"openclaw":{"emoji":"🐻","requires":{"bins":["curl","jq"],"env":["SALMON_HUB_URL","SALMON_CONSUMER_TOKEN"]},"primaryEnv":"SALMON_CONSUMER_TOKEN"}}
---

# Bear Salmon Notes

Interact with [Bear](https://bear.app) notes through the Salmon Hub API. Bear is the source of truth — writes go through an async queue and are applied to Bear by the bridge agent.

Bear notes use Markdown format. When creating or updating notes, always write the `body` field in Markdown. Bear supports headings (`#`, `##`), lists (`-`, `1.`), checkboxes (`- [ ]`, `- [x]`), bold (`**bold**`), italic (`*italic*`), code blocks, links, and inline tags (`#tag`, `#tag/subtag`). The `title` field is plain text — do not use Markdown in it.

All responses are JSON. Pipe through `jq` for readability.

## Interactive API docs

Swagger UI is available at `$SALMON_HUB_URL/api/docs/` (requires authentication). Use it to explore all endpoints interactively and try requests in the browser.

## Data models

### Note

Key fields returned by note endpoints:

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Hub UUID (use this for all API calls) |
| `bear_id` | string? | Bear's internal UUID (null until synced) |
| `title` | string | Note title |
| `subtitle` | string | Auto-generated subtitle |
| `body` | string | Full Markdown body (omitted in list responses) |
| `sync_status` | string | `synced`, `pending_to_bear`, or `conflict` |
| `created_at` | string | ISO 8601 timestamp |
| `modified_at` | string | ISO 8601 timestamp |
| `trashed` | int | 1 if trashed, 0 otherwise |
| `archived` | int | 1 if archived, 0 otherwise |
| `encrypted` | int | 1 if encrypted (read-only), 0 otherwise |
| `pinned` | int | 1 if pinned, 0 otherwise |
| `has_files` | int | 1 if has file attachments |
| `has_images` | int | 1 if has image attachments |
| `todo_completed` | int | Count of completed todos |
| `todo_incompleted` | int | Count of incomplete todos |
| `tags` | Tag[] | Attached tags (in get/search responses) |
| `backlinks` | Backlink[] | Notes linking to this one (in get response) |

### Tag

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Hub UUID |
| `title` | string | Tag name (e.g. `work/projects`) |
| `pinned` | int | 1 if pinned |
| `modified_at` | string | ISO 8601 timestamp |

### Backlink

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Hub UUID |
| `linked_by_id` | string | Note ID that contains the link |
| `linking_to_id` | string | Note ID being linked to |
| `title` | string | Link anchor text |

### WriteQueueItem (returned by write operations)

| Field | Type | Description |
|-------|------|-------------|
| `id` | int | Queue item ID |
| `action` | string | `create`, `update`, `add_tag`, `trash`, `add_file`, `archive`, `rename_tag`, `delete_tag` |
| `note_id` | string | Associated note ID |
| `status` | string | `pending`, `processing`, `applied`, `failed` |
| `created_at` | string | ISO 8601 timestamp |
| `error` | string | Error message if failed |

### Example: list notes response

```json
[
  {
    "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "title": "Meeting Notes",
    "subtitle": "Discussed project roadmap",
    "sync_status": "synced",
    "created_at": "2025-01-15T10:30:00Z",
    "modified_at": "2025-01-15T14:20:00Z",
    "trashed": 0,
    "archived": 0,
    "encrypted": 0,
    "pinned": 0,
    "has_files": 0,
    "has_images": 1,
    "todo_completed": 2,
    "todo_incompleted": 3,
    "tags": [
      {"id": "f1e2d3c4-b5a6-7890-fedc-ba0987654321", "title": "work/meetings"}
    ]
  }
]
```

## Authentication

Every request must include:

```bash
-H "Authorization: Bearer $SALMON_CONSUMER_TOKEN"
```

## Idempotency

All mutating requests (POST, PUT, DELETE) MUST include a unique `Idempotency-Key` header. Generate one per request:

```bash
-H "Idempotency-Key: $(uuidgen)"
```

Replaying the same key returns the original result without side effects.

## Operations

### List notes

```bash
curl -s -H "Authorization: Bearer $SALMON_CONSUMER_TOKEN" \
  "$SALMON_HUB_URL/api/notes?sort=modified_at&order=desc&limit=50" | jq
```

Query parameters:
- `tag` — filter by tag name
- `sort` — `modified_at`, `created_at`, or `title`
- `order` — `asc` or `desc`
- `trashed` — `true` or `false`
- `encrypted` — `true` or `false`
- `limit` — max results (0–200)
- `offset` — pagination offset

The `body` field is omitted in list responses.

### Get note by ID

```bash
curl -s -H "Authorization: Bearer $SALMON_CONSUMER_TOKEN" \
  "$SALMON_HUB_URL/api/notes/NOTE_ID" | jq
```

Returns the full note with `body`, `tags`, and `backlinks`.

### Search notes

```bash
curl -s -H "Authorization: Bearer $SALMON_CONSUMER_TOKEN" \
  "$SALMON_HUB_URL/api/notes/search?q=QUERY&limit=20" | jq
```

Parameters:
- `q` (required) — full-text search query
- `tag` — filter by tag name
- `limit` — max results (default 20, max 200)

### Create note

```bash
curl -s -X POST -H "Authorization: Bearer $SALMON_CONSUMER_TOKEN" \
  -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -d '{"title":"TITLE","body":"BODY","tags":["tag1","tag2"]}' \
  "$SALMON_HUB_URL/api/notes" | jq
```

Fields:
- `title` (required) — note title
- `body` (optional) — Markdown content
- `tags` (optional) — array of tag names

IMPORTANT: Do NOT put `#tag` inline in the `body` if you also pass those tags in the `tags` array. Bear adds tags from the `tags` parameter automatically — duplicating them in the body will result in double tags in the note.

Returns HTTP 201. The note will have `sync_status: pending_to_bear` until the bridge syncs it to Bear.

### Update note

```bash
curl -s -X PUT -H "Authorization: Bearer $SALMON_CONSUMER_TOKEN" \
  -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -d '{"title":"NEW_TITLE","body":"NEW_BODY"}' \
  "$SALMON_HUB_URL/api/notes/NOTE_ID" | jq
```

Fields:
- `title` (optional) — new title
- `body` (required) — new body

Errors:
- 403 — encrypted notes are read-only
- 409 — note not yet synced to Bear, or has unresolved conflicts

### Trash note

```bash
curl -s -X DELETE -H "Authorization: Bearer $SALMON_CONSUMER_TOKEN" \
  -H "Idempotency-Key: $(uuidgen)" \
  "$SALMON_HUB_URL/api/notes/NOTE_ID" | jq
```

Moves the note to trash in Bear. Same constraints as update (403/409).

### Archive note

```bash
curl -s -X POST -H "Authorization: Bearer $SALMON_CONSUMER_TOKEN" \
  -H "Idempotency-Key: $(uuidgen)" \
  "$SALMON_HUB_URL/api/notes/NOTE_ID/archive" | jq
```

Archives the note in Bear. Same constraints as update (403/409).

### List tags

```bash
curl -s -H "Authorization: Bearer $SALMON_CONSUMER_TOKEN" \
  "$SALMON_HUB_URL/api/tags" | jq
```

Returns all tags.

### Add tag to note

```bash
curl -s -X POST -H "Authorization: Bearer $SALMON_CONSUMER_TOKEN" \
  -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -d '{"tag":"TAG_NAME"}' \
  "$SALMON_HUB_URL/api/notes/NOTE_ID/tags" | jq
```

Returns HTTP 201 with the write queue item.

### Rename tag

```bash
curl -s -X PUT -H "Authorization: Bearer $SALMON_CONSUMER_TOKEN" \
  -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -d '{"new_name":"NEW_TAG_NAME"}' \
  "$SALMON_HUB_URL/api/tags/TAG_ID" | jq
```

Returns HTTP 202 with the write queue item.

### Delete tag

```bash
curl -s -X DELETE -H "Authorization: Bearer $SALMON_CONSUMER_TOKEN" \
  -H "Idempotency-Key: $(uuidgen)" \
  "$SALMON_HUB_URL/api/tags/TAG_ID" | jq
```

Returns HTTP 202 with the write queue item.

### Get attachment

```bash
curl -s -H "Authorization: Bearer $SALMON_CONSUMER_TOKEN" \
  "$SALMON_HUB_URL/api/attachments/ATTACHMENT_ID" --output FILE_NAME
```

Downloads binary file with `Content-Disposition: attachment`.

### List backlinks

```bash
curl -s -H "Authorization: Bearer $SALMON_CONSUMER_TOKEN" \
  "$SALMON_HUB_URL/api/notes/NOTE_ID/backlinks" | jq
```

Returns notes that link to the given note.

### Sync status

```bash
curl -s -H "Authorization: Bearer $SALMON_CONSUMER_TOKEN" \
  "$SALMON_HUB_URL/api/sync/status" | jq
```

Returns `last_sync_at`, `last_push_at`, `queue_size`, `initial_sync_complete`, `conflict_count`.

## Write flow

Writes are asynchronous:
1. You send a write request — hub enqueues it
2. Bridge agent leases and applies it to Bear via x-callback-url
3. Note `sync_status` transitions: `synced` → `pending_to_bear` → `synced`

If Bear modifies the note while it's `pending_to_bear`, a conflict is created (`sync_status: conflict`).

## Error codes

- 400 — invalid request or missing `Idempotency-Key`
- 401 — missing or invalid token
- 403 — encrypted notes are read-only
- 404 — resource not found
- 409 — note not synced to Bear or has unresolved conflicts
- 413 — file too large (max 5 MB)
