# Fill xcallback test gaps + Add new write actions (add_file, archive, rename_tag, delete_tag)

## Overview
- Fill xcallback test coverage gaps: Update, AddTag, Trash missing error path tests
- Add 4 new Bear x-callback-url write actions: add_file, archive, rename_tag, delete_tag
- Attachments currently flow one way (Bear → hub); consumers need to attach files to notes
- Archive gives consumers an alternative to trash (non-destructive)
- Tag rename/delete lets consumers manage tags through the write queue
- Bear `/add-file` accepts base64-encoded file data in URL (5 MB limit)
- Bear `/rename-tag` and `/delete-tag` use tag name strings, not UUIDs

## Context (from discovery)
- Files/components involved:
  - `internal/xcallback/xcallback.go` — XCallback interface + Xcall implementation (add new methods)
  - `internal/xcallback/xcallback_test.go` — unit tests (fill gaps + add new method tests)
  - `internal/hubclient/client.go` — HubClient interface (add DownloadAttachment)
  - `internal/api/server.go` — route registration (add new endpoints)
  - `internal/api/notes_handler.go` — note handlers (add addFile, archiveNote)
  - `internal/api/tags_handler.go` — tag handlers (add renameTag, deleteTag)
  - `internal/api/sync_handler.go` — sync handlers (add syncDownloadAttachment)
  - `internal/api/server_test.go` — API tests
  - `cmd/bridge/queue.go` — bridge queue processing (add 4 new apply functions)
  - `cmd/bridge/queue_test.go` — bridge queue tests
  - `cmd/bridge/sync_test.go` — mock structs for bridge tests
  - `tools/bear-xcall/BearXcallTests.swift` — Swift integration tests
- Related patterns found:
  - All xcallback methods follow same pattern: build URL → execute → parse JSON → check errorCode
  - Write queue actions: create, update, add_tag, trash — each has payload struct + apply function
  - Consumer API handlers validate: note exists, has bear_id, not encrypted, not in conflict
  - Idempotency enforced via (idempotency_key, consumer_id) unique constraint
  - Bridge verifies operations by reading Bear SQLite after xcall execution
  - `CommandExecutor` interface + `mockExecutor` for xcallback unit testing
- Dependencies identified:
  - Bear `/add-file` requires base64-encoded file data in URL parameter
  - Bridge needs to download files from hub before passing to bear-xcall
  - Existing `GET /api/attachments/{id}` is consumer-scoped; bridge needs its own endpoint
  - `encoding/base64` stdlib for add-file implementation

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task** — no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change
- Maintain backward compatibility

## Testing Strategy
- **Unit tests**: required for every task
- **Swift manual tests**: `tools/bear-xcall/BearXcallTests.swift` — manual test suite for CLI + Bear operations, run via `make test-xcall` (requires macOS + Bear running)
- Go code tested via existing mock infrastructure (`CommandExecutor`, `mockExecutor`, `XCallbackMock`)
- API handler tests follow existing patterns in `internal/api/server_test.go`
- Bridge queue tests use mock hub client and mock xcallback

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix
- Update plan if implementation deviates from original scope
- Keep plan in sync with actual work done

## Implementation Steps

### Task 1: Fill xcallback test gaps
- [x] add `TestUpdate/exec error` sub-test: executor returns error, assert contains "bear-xcall update"
- [x] add `TestUpdate/invalid JSON response` sub-test: executor returns `[]byte("not json")`, assert contains "invalid bear-xcall JSON"
- [x] add `TestAddTag/bear error` sub-test: executor returns `xcallResult{ErrorCode: 1, ErrorMsg: "not found"}`, assert contains "bear error"
- [x] add `TestAddTag/exec error` sub-test: assert contains "bear-xcall add-tag"
- [x] add `TestAddTag/invalid JSON response` sub-test: assert contains "invalid bear-xcall JSON"
- [x] add `TestTrash/exec error` sub-test: assert contains "bear-xcall trash"
- [x] add `TestTrash/invalid JSON response` sub-test: assert contains "invalid bear-xcall JSON"
- [x] run tests: `make test` — must pass before next task

### Task 2: Add `AddFile` to XCallback interface
- [x] add `AddFile(ctx context.Context, token, bearID, filename string, fileData []byte) error` to `XCallback` interface in `internal/xcallback/xcallback.go`
- [x] add `maxAddFileSize = 5 * 1024 * 1024` constant
- [x] implement `AddFile` on `Xcall`: validate size ≤ 5 MB, base64-encode file data, build `bear://x-callback-url/add-file?` URL with params (id, filename, file, show_window=no, open_note=no), execute and parse response
- [x] add `TestAddFile/success` sub-test: verify URL prefix, base64-encoded file param, all query params
- [x] add `TestAddFile/bear error` sub-test
- [x] add `TestAddFile/exec error` sub-test
- [x] add `TestAddFile/invalid JSON response` sub-test
- [x] add `TestAddFile/file too large` sub-test: 6 MB data, assert error contains size limit message
- [x] run tests: `make test` — must pass before next task

### Task 3: Add `Archive` to XCallback interface
- [x] add `Archive(ctx context.Context, token, bearID string) error` to `XCallback` interface
- [x] implement `Archive` on `Xcall`: build `bear://x-callback-url/archive?id=<bearID>&show_window=no`, same pattern as `Trash`
- [x] add `TestArchive/success` sub-test: verify URL prefix `bear://x-callback-url/archive?`, params (token, id, show_window)
- [x] add `TestArchive/bear error` sub-test
- [x] add `TestArchive/exec error` sub-test
- [x] add `TestArchive/invalid JSON response` sub-test
- [x] run tests: `make test` — must pass before next task

### Task 4: Add `RenameTag` to XCallback interface
- [x] add `RenameTag(ctx context.Context, token, oldName, newName string) error` to `XCallback` interface
- [x] implement `RenameTag` on `Xcall`: build `bear://x-callback-url/rename-tag?name=<old>&new_name=<new>&show_window=no`
- [x] add `TestRenameTag/success` sub-test: verify URL prefix, params (token, name, new_name, show_window)
- [x] add `TestRenameTag/bear error` sub-test (locked tag scenario)
- [x] add `TestRenameTag/exec error` sub-test
- [x] add `TestRenameTag/invalid JSON response` sub-test
- [x] add `TestRenameTag/special characters in names` sub-test: unicode and slash characters in tag names
- [x] run tests: `make test` — must pass before next task

### Task 5: Add `DeleteTag` to XCallback interface
- [x] add `DeleteTag(ctx context.Context, token, tagName string) error` to `XCallback` interface
- [x] implement `DeleteTag` on `Xcall`: build `bear://x-callback-url/delete-tag?name=<tagName>&show_window=no`
- [x] add `TestDeleteTag/success` sub-test: verify URL prefix, params (token, name, show_window)
- [x] add `TestDeleteTag/bear error` sub-test (locked tag scenario)
- [x] add `TestDeleteTag/exec error` sub-test
- [x] add `TestDeleteTag/invalid JSON response` sub-test
- [x] run tests: `make test` — must pass before next task

### Task 6: Regenerate mocks after interface changes
- [x] run `make generate` to regenerate `internal/xcallback/xcallback_mock.go` with new methods (AddFile, Archive, RenameTag, DeleteTag)
- [x] run tests: `make test` — must pass before next task

### Task 7: Add `DownloadAttachment` to HubClient
- [x] add `DownloadAttachment(ctx context.Context, attachmentID string) ([]byte, error)` to `HubClient` interface in `internal/hubclient/client.go`
- [x] implement on `HTTPClient`: `GET /api/sync/attachments/{id}`, read body with 10 MB limit reader, handle error responses
- [x] run `make generate` to regenerate hub client mock
- [x] run tests: `make test` — must pass before next task

### Task 8: Add bridge-scoped attachment download endpoint
- [x] add `GET /sync/attachments/{id}` route under bridge auth group in `internal/api/server.go`
- [x] add `syncDownloadAttachment` handler in `internal/api/sync_handler.go` (reuse `getAttachment` logic: lookup attachment, resolve file path, serve file)
- [x] add test: success — upload attachment file, then download via bridge endpoint, verify content
- [x] add test: attachment not found → 404
- [x] add test: missing bridge auth → 401
- [x] run tests: `make test` — must pass before next task

### Task 9: Consumer API endpoint — file upload
- [x] add `addFile` handler in `internal/api/notes_handler.go`: validate note (exists, has bear_id, not encrypted, not in conflict), parse multipart form, store file to `attachmentsDir/{generatedID}/{filename}`, enqueue `action="add_file"` with payload `{attachment_id, filename}`, return 202
- [x] register `POST /notes/{noteID}/attachments` route with `idempotencyRequired` + `bodyLimitMiddleware(10<<20)` in `internal/api/server.go`
- [x] add test: success — 202 with queue item
- [x] add test: encrypted note → 403
- [x] add test: no bear_id → 409
- [x] add test: conflict state → 409
- [x] add test: missing idempotency key → 400
- [x] add test: missing file in form → 400
- [x] add test: duplicate idempotency key → 200 returns existing
- [x] run tests: `make test` — must pass before next task

### Task 10: Consumer API endpoint — archive note
- [x] add `archiveNote` handler in `internal/api/notes_handler.go` (same pattern as `trashNote`): validate note, set `sync_status="pending_to_bear"`, enqueue `action="archive"` with payload `{bear_id}`, return 202
- [x] register `POST /notes/{noteID}/archive` route with `idempotencyRequired` in `internal/api/server.go`
- [x] add test: success — 202
- [x] add test: encrypted note → 403
- [x] add test: no bear_id → 409
- [x] add test: conflict state → 409
- [x] add test: missing idempotency key → 400
- [x] run tests: `make test` — must pass before next task

### Task 11: Consumer API endpoints — rename & delete tag
- [x] add `renameTag` handler in `internal/api/tags_handler.go`: validate tag exists, parse `{new_name}` from body, enqueue `action="rename_tag"` with payload `{name, new_name}`, return 202
- [x] add `deleteTag` handler in `internal/api/tags_handler.go`: validate tag exists, enqueue `action="delete_tag"` with payload `{name}`, return 202
- [x] register `PUT /tags/{id}` and `DELETE /tags/{id}` routes with `idempotencyRequired` in `internal/api/server.go`
- [x] add test: rename success — 202
- [x] add test: rename tag not found → 404
- [x] add test: rename missing new_name → 400
- [x] add test: rename missing idempotency key → 400
- [x] add test: delete success — 202
- [x] add test: delete tag not found → 404
- [x] add test: delete missing idempotency key → 400
- [x] run tests: `make test` — must pass before next task

### Task 12: Bridge queue processing — add_file, archive, rename_tag, delete_tag
- [x] add `addFilePayload` struct: `{AttachmentID, Filename}`
- [x] add `case "add_file"` in `applyQueueItem` switch, implement `applyAddFile`: parse payload → resolve bear UUID → download from hub → validate size ≤ 5 MB → call `xcall.AddFile()`
- [x] add `case "archive"` in switch, implement `applyArchive`: parse payload → resolve bear UUID → duplicate check (already archived?) → call `xcall.Archive()`
- [x] add `renameTagPayload` struct: `{Name, NewName}`
- [x] add `case "rename_tag"` in switch, implement `applyRenameTag`: parse payload → call `xcall.RenameTag(oldName, newName)` → verify in Bear SQLite
- [x] add `deleteTagPayload` struct: `{Name}`
- [x] add `case "delete_tag"` in switch, implement `applyDeleteTag`: parse payload → call `xcall.DeleteTag(name)` → verify tag gone from Bear SQLite
- [x] update mock structs in `cmd/bridge/sync_test.go` to include `DownloadAttachment` and new xcallback methods (AddFile, Archive, RenameTag, DeleteTag)
- [x] add test: add_file success
- [x] add test: add_file download error → failed ack
- [x] add test: add_file xcall error → failed ack
- [x] add test: add_file invalid payload → failed ack
- [x] add test: add_file too large → failed ack
- [x] add test: archive success
- [x] add test: archive already archived → skip
- [x] add test: archive xcall error → failed ack
- [x] add test: archive invalid payload → failed ack
- [x] add test: rename_tag success
- [x] add test: rename_tag xcall error → failed ack
- [x] add test: rename_tag invalid payload → failed ack
- [x] add test: delete_tag success
- [x] add test: delete_tag not found → skip
- [x] add test: delete_tag xcall error → failed ack
- [x] add test: delete_tag invalid payload → failed ack
- [x] run tests: `make test` — must pass before next task

### Task 13: Swift integration tests
- [x] add `add-file` test in `runBearTests()`: base64 encode small text → call `/add-file` with created note ID → verify exit 0
- [x] add `archive` test in `runBearTests()`: create note → archive via `/archive` → verify exit 0
- [x] add `rename-tag` test: rename `bear-xcall-test` → `bear-xcall-test-renamed` → verify exit 0 → rename back
- [x] add `delete-tag` test: create tag → delete via `/delete-tag` → verify exit 0
- [x] update `testsSkipped` count for Bear operation tests section
- [x] run tests: `BEAR_XCALL_BEAR_TESTS=1 make test-xcall` — must pass before next task

### Task 14: Verify acceptance criteria
- [ ] verify all 4 new actions implemented end-to-end (consumer API → hub queue → bridge apply → Bear)
- [ ] verify xcallback test coverage gaps are filled
- [ ] run full test suite: `make test`
- [ ] run tests with race detector: `make test-race`
- [ ] run linter: `make lint` — all issues must be fixed
- [ ] run formatter: `make fmt`

### Task 15: [Final] Update documentation
- [ ] update CLAUDE.md to mention new write actions (add_file, archive, rename_tag, delete_tag)
- [ ] update README.md API section if consumer endpoints are documented

## Technical Details

### Bear x-callback-url actions used
| Action | URL | Key params |
|--------|-----|-----------|
| `/add-file` | `bear://x-callback-url/add-file?` | `id`, `filename`, `file` (base64), `show_window` |
| `/archive` | `bear://x-callback-url/archive?` | `id`, `show_window` |
| `/rename-tag` | `bear://x-callback-url/rename-tag?` | `name`, `new_name`, `show_window` |
| `/delete-tag` | `bear://x-callback-url/delete-tag?` | `name`, `show_window` |

### Write queue action payloads
```
add_file:    {"attachment_id": "<hub-uuid>", "filename": "photo.jpg"}
archive:     {"bear_id": "<bear-uuid>"}
rename_tag:  {"name": "old/tag", "new_name": "new/tag"}
delete_tag:  {"name": "tag/to/delete"}
```

### File size constraint
- `maxAddFileSize = 5 * 1024 * 1024` (5 MB raw)
- Base64 expands ~33% → ~6.7 MB in URL
- Validated before base64 encoding in xcallback.AddFile()
- Validated again in bridge applyAddFile() after downloading from hub

### Key design decisions
1. **Base64 in URL** — Bear requires it for `/add-file`, no alternative
2. **Bridge downloads from hub** — no temp files, file data stays in memory
3. **No sync_status change for add_file** — doesn't modify note title/body
4. **sync_status change for archive** — same as trash (pending_to_bear → synced)
5. **No sync_status for tag operations** — rename/delete-tag are global, not per-note
6. **Tag names in payload** — Bear uses name strings, not UUIDs; bridge doesn't need bear_id resolution for tags
7. **Separate bridge download endpoint** — `GET /api/sync/attachments/{id}` under bridge auth scope

## Post-Completion

**Manual verification:**
- Test add-file with real Bear app: attach image, verify it appears in note
- Test archive with real Bear app: verify note moves to archive
- Test rename-tag: verify all notes update to new tag
- Test delete-tag: verify tag removed from all notes
- Verify all operations work on both Intel and Apple Silicon Macs

**Deployment:**
- No deployment changes needed — new actions use existing write queue infrastructure
- Consumer apps need to be updated to use new API endpoints
