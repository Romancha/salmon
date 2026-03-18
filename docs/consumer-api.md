# Consumer API Quick Start Guide

The Salmon Hub API lets external consumers read and write Bear notes through a REST API. Bear remains the source of truth — writes go through a queue and are applied to Bear by the bridge agent.

## Base URL

All endpoints are served under the hub's address. Replace `$HUB_URL` with your hub's base URL (e.g., `https://salmon.example.com`).

## Authentication

Every request must include a Bearer token in the `Authorization` header:

```
Authorization: Bearer <your-consumer-token>
```

Tokens are provisioned by the hub operator via the `SALMON_HUB_CONSUMER_TOKENS` environment variable. Each consumer gets a unique name and token.

## Idempotency

All mutating requests (`POST`, `PUT`, `DELETE`) require an `Idempotency-Key` header:

```
Idempotency-Key: <unique-string>
```

Replaying the same key returns the original result without side effects. Use a UUID or another unique value per logical operation. Omitting this header returns HTTP 400.

## Endpoints

### List Notes

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "$HUB_URL/api/notes?sort=modified_at&order=desc&limit=50"
```

Query parameters:
- `tag` — filter by tag name
- `sort` — `modified_at`, `created_at`, or `title`
- `order` — `asc` or `desc`
- `trashed` — filter by trashed status (`true`/`false`)
- `encrypted` — filter by encrypted status (`true`/`false`)
- `limit` — max results (0–200)
- `offset` — pagination offset

Returns a JSON array of note objects. The `body` field is omitted in list responses.

### Get Note

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "$HUB_URL/api/notes/<note-id>"
```

Returns a single note with full `body`, `tags`, and `backlinks`.

### Search Notes

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "$HUB_URL/api/notes/search?q=project&limit=20"
```

Query parameters:
- `q` (required) — full-text search query
- `tag` — filter by tag name
- `limit` — max results (default 20, max 200)

### Create Note

```bash
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -d '{"title":"Meeting Notes","body":"# Meeting Notes\nDiscussed project roadmap.","tags":["work","meetings"]}' \
  "$HUB_URL/api/notes"
```

Request body:
- `title` (string, required) — note title
- `body` (string) — note body (Markdown)
- `tags` (string array, optional) — tags to assign

Returns HTTP 201 with the created note. The note's `sync_status` will be `pending_to_bear` until the bridge syncs it to Bear.

### Update Note

```bash
curl -X PUT -H "Authorization: Bearer $TOKEN" \
  -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -d '{"title":"Updated Title","body":"# Updated Content\nNew body text."}' \
  "$HUB_URL/api/notes/<note-id>"
```

Request body:
- `title` (string, optional) — new title
- `body` (string, required) — new body

Returns HTTP 200 with the updated note.

Constraints:
- Cannot update encrypted notes (403)
- Cannot update notes not yet synced to Bear (409), unless the note has a pending create queue item (update is merged into the pending create)
- Cannot update notes with unresolved conflicts (409)

### Trash Note

```bash
curl -X DELETE -H "Authorization: Bearer $TOKEN" \
  -H "Idempotency-Key: $(uuidgen)" \
  "$HUB_URL/api/notes/<note-id>"
```

Moves the note to trash in Bear. Returns HTTP 200 with the updated note. Same constraints as update.

### Archive Note

```bash
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Idempotency-Key: $(uuidgen)" \
  "$HUB_URL/api/notes/<note-id>/archive"
```

Archives the note in Bear. Returns HTTP 200 with the updated note. Same constraints as update.

### Add File to Note

```bash
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Idempotency-Key: $(uuidgen)" \
  -F "file=@document.pdf" \
  "$HUB_URL/api/notes/<note-id>/attachments"
```

Uploads a file attachment (max 5 MB). Returns HTTP 202 with the write queue item.

### List Tags

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "$HUB_URL/api/tags"
```

Returns a JSON array of all tags.

### Add Tag to Note

```bash
curl -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -d '{"tag":"work/projects"}' \
  "$HUB_URL/api/notes/<note-id>/tags"
```

Returns HTTP 201 with the write queue item.

### Rename Tag

```bash
curl -X PUT -H "Authorization: Bearer $TOKEN" \
  -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -d '{"new_name":"work/archived-projects"}' \
  "$HUB_URL/api/tags/<tag-id>"
```

Returns HTTP 202 with the write queue item.

### Delete Tag

```bash
curl -X DELETE -H "Authorization: Bearer $TOKEN" \
  -H "Idempotency-Key: $(uuidgen)" \
  "$HUB_URL/api/tags/<tag-id>"
```

Returns HTTP 202 with the write queue item.

### Get Attachment

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "$HUB_URL/api/attachments/<attachment-id>" \
  --output file.pdf
```

Downloads an attachment file as binary with `Content-Disposition: attachment`.

### List Backlinks

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "$HUB_URL/api/notes/<note-id>/backlinks"
```

Returns notes that link to the given note.

### Sync Status

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "$HUB_URL/api/sync/status"
```

Returns:

```json
{
  "last_sync_at": "2025-01-15T10:30:00Z",
  "last_push_at": "2025-01-15T10:25:00Z",
  "queue_size": 3,
  "initial_sync_complete": "true",
  "conflict_count": 0
}
```

## Write Flow

Consumer writes follow an asynchronous queue-based flow:

1. Consumer sends a write request (create, update, trash, etc.) to the hub
2. Hub validates the request and enqueues a write queue item
3. Bridge agent periodically leases pending queue items from the hub
4. Bridge applies each item to Bear via `bear-xcall` (x-callback-url)
5. Bridge acknowledges the item back to hub as applied or failed

Notes in `pending_to_bear` status have their `title` and `body` protected from being overwritten by Bear delta pushes, ensuring the consumer's changes are preserved until applied.

## Sync Status Lifecycle

Each note has a `sync_status` field:

- `synced` — normal state; Bear deltas freely update hub fields
- `pending_to_bear` — a write is queued; title/body are protected from Bear overwrites
- `conflict` — a Bear push arrived while the note was `pending_to_bear` and Bear changed a content field (title or body) that the consumer also changed; metadata-only changes do not trigger conflict. The bridge creates a `[Conflict] Title` note in Bear instead of applying the queued change

## Error Codes

| Status | Meaning |
|--------|---------|
| 400 | Invalid request — missing required fields, bad parameters, or missing `Idempotency-Key` |
| 401 | Missing or invalid `Authorization` header |
| 403 | Forbidden — encrypted notes are read-only |
| 404 | Resource not found |
| 409 | Conflict — note not yet synced to Bear, or has unresolved conflicts |
| 413 | File too large (max 5 MB for consumer uploads) |
| 500 | Internal server error |

All errors return JSON:

```json
{
  "error": "descriptive error message"
}
```

## Interactive API Docs

Swagger UI is available at `/api/docs/` (requires consumer authentication). It provides interactive documentation where you can explore all endpoints and try requests directly.
