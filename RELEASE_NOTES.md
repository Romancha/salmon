# Release Notes

## Version 0.6.0 — 02 Apr 2026

### 🎉 Major Features

**MCP Server**
- New `salmon-mcp` binary providing Model Context Protocol (MCP) server for AI assistant integration.
- All 13 consumer API operations exposed as MCP tools over stdio transport.
- Works with Claude Code, Cursor, OpenClaw, and any MCP-compatible client.
- Pre-built binaries for macOS and Linux (arm64 + amd64) published to GitHub Releases.

### 🐞 Fixes

- Fix missing URL path escaping for user-supplied IDs in MCP client requests.

---

## Version 0.5.0 — 30 Mar 2026

### 🆕 New

- Attachments are now included in note API responses (list, search, get) with `id`, `type`, `filename`, `file_size`, `width`, and `height` fields. Use the attachment `id` with `GET /api/attachments/{id}` to download files.

### 🐞 Fixes

- Internal fields (`bear_raw`, `encrypted_data`, `hub_modified_at`) are no longer exposed in consumer API responses for notes, tags, and backlinks.

---

## Version 0.4.1 — 23 Mar 2026

### 🐞 Fixes

- Fix false conflicts on create→update flow: preserve `expected_bear_modified_at` on create ack so echo detection works when a consumer updates a note before Bear's first delta push arrives.
- Fix missing `pending_bear_title`/`pending_bear_body` snapshot when create is acked with other pending queue items (e.g. `add_tag`), preventing unconditional conflict fallback.
- Copy original note's tags to `[Conflict]` copy notes — previously conflict copies were created without tags.
- Fix SalmonRun.app showing "1.0" instead of actual version: inject version via xcodebuild build settings instead of PlistBuddy (which was ignored by `GENERATE_INFOPLIST_FILE`).

---

## Version 0.4.0 — 18 Mar 2026

### 🎉 Major Features

**Sync Conflict Prevention**
- Echo detection prevents false conflicts when Bear's `modified_at` changes solely because bridge applied a consumer write.
- Queue coalescing merges rapid sequential consumer writes into a single pending item, avoiding unnecessary conflict triggers.
- Create-update coalescing merges updates into pending create items instead of creating separate queue entries.

### 🐞 Fixes

- Check `RowsAffected` in queue coalescing to prevent silent data loss when no rows match.

---

## Version 0.3.0 — 06 Mar 2026

### 🎉 Major Features

**SalmonRun Menu Bar App**
- Native macOS menu bar app (macOS 14+) wrapping bridge daemon with status display, sync control, and log viewer.
- Settings window with Keychain token storage and environment generation.
- Write queue status display and error notifications with rate limiting.
- Bridge daemon mode with continuous sync loop, Unix socket IPC, and structured status events.
- Distribution via signed and notarized `.dmg` disk images (arm64 + amd64).

**Field-Level Conflict Detection**
- Conflict detection now compares individual fields (title, body) instead of relying solely on `modified_at` timestamps.
- Metadata-only changes (pinned, etc.) no longer trigger false conflicts.

**Salmon Rebranding**
- Project rebranded from BearBridge to Salmon (module, binaries, environment variables, Swift app).

### 🆕 New

- Consumer API quick start guide with Swagger UI at `/api/docs/`.

### ✨ Improvements

- Skip delta sync when Bear DB mtime is unchanged.

### 🐞 Fixes

- Sign `bear-xcall.app` with Developer ID to fix recurring TCC prompts.
- Use percent-encoding (`%20`) instead of `+` for spaces in x-callback URLs.

---

## Version 0.2.3 — 04 Mar 2026

### 🐞 Fixes

- Skip backlinks referencing notes not present on hub.
- Remove quarantine attribute from `bear-xcall.app` on install.

---

## Version 0.2.2 — 04 Mar 2026

### ✨ Improvements

- Improve bridge macOS installation flow and add error logging.

---

## Version 0.2.1 — 04 Mar 2026

### 🐞 Fixes

- Change default hub port from 8080 to 7433.

---

## Version 0.2.0 — 04 Mar 2026

### 🎉 Major Features

**New Write Actions**
- Add consumer API endpoints for file upload, note archiving, tag rename, and tag delete.
- Bridge queue processing for `add_file`, `archive`, `rename_tag`, and `delete_tag` actions.

**bear-xcall Swift CLI**
- Replace external `xcall` dependency with custom `bear-xcall` Swift CLI tool bundled as a `.app`.

---

## Version 0.1.0 — 04 Mar 2026

### 🎉 Major Features

**Initial Release**
- Hub API server with SQLite store, FTS5 search, CRUD endpoints, and sync push.
- Bridge Mac agent reading Bear SQLite with delta export, batched initial sync, and write queue processing.
- Multi-consumer Bearer token authentication with per-consumer write attribution.
- Conflict detection and resolution for simultaneous Bear and consumer edits.
- Attachment file sync with upload from Bear and cleanup on deletion.
- Deployment configs (systemd, Caddy) and CI/CD pipeline.
