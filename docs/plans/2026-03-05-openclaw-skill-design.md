# OpenClaw Skill for Salmon Hub API

## Summary

Create an OpenClaw skill (`bear-salmon-notes`) that allows the OpenClaw AI assistant to read and write Bear notes via the Salmon Hub consumer API using curl + jq.

## Location

`tools/consumer/bear-salmon-notes/`

## Files

- `SKILL.md` — OpenClaw skill definition (YAML frontmatter + agent instructions)
- `README.md` — Human-readable setup and usage documentation

## Environment Variables

| Variable | Description |
|----------|-------------|
| `SALMON_HUB_URL` | Base URL of the Salmon Hub (e.g. `https://salmon.example.com`) |
| `SALMON_CONSUMER_TOKEN` | Consumer Bearer token provisioned by the hub operator |

## Dependencies

- `curl` — HTTP requests
- `jq` — JSON parsing and formatting

## Supported Operations

| # | Operation | Method | Endpoint |
|---|-----------|--------|----------|
| 1 | List notes | GET | `/api/notes` |
| 2 | Get note | GET | `/api/notes/{id}` |
| 3 | Search notes | GET | `/api/notes/search` |
| 4 | Create note | POST | `/api/notes` |
| 5 | Update note | PUT | `/api/notes/{id}` |
| 6 | Trash note | DELETE | `/api/notes/{id}` |
| 7 | Archive note | POST | `/api/notes/{id}/archive` |
| 8 | List tags | GET | `/api/tags` |
| 9 | Add tag to note | POST | `/api/notes/{id}/tags` |
| 10 | Rename tag | PUT | `/api/tags/{id}` |
| 11 | Delete tag | DELETE | `/api/tags/{id}` |
| 12 | Get attachment | GET | `/api/attachments/{id}` |
| 13 | List backlinks | GET | `/api/notes/{id}/backlinks` |
| 14 | Sync status | GET | `/api/sync/status` |

## Key Conventions

- All mutating requests (POST/PUT/DELETE) include `Idempotency-Key` header (generated via `uuidgen`)
- All requests include `Authorization: Bearer $SALMON_CONSUMER_TOKEN`
- Responses piped through `jq` for readability
- Error handling: skill documents 403 (encrypted), 409 (conflict/not synced), 400 (missing idempotency key)

## OpenClaw Configuration

```json
{
  "skills": {
    "entries": {
      "salmon-notes": {
        "enabled": true,
        "env": {
          "SALMON_HUB_URL": "https://salmon.example.com",
          "SALMON_CONSUMER_TOKEN": "your-token-here"
        }
      }
    }
  }
}
```

## Security

- API tokens stored in `openclaw.json` env config, never in SKILL.md
- Tokens injected as environment variables at agent runtime
- Encrypted notes are read-only (API returns 403)
