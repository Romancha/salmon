# Sync Conflict Prevention: Queue Coalescing + Echo Detection

## Overview
- Eliminate false sync conflicts caused by (1) rapid sequential consumer writes and (2) Bear's `modified_at` changes after applying our own queue items
- Two complementary mechanisms: **queue coalescing** merges pending writes before they reach the bridge; **echo detection** recognizes Bear delta pushes that reflect our own writes
- No consumer API changes required; backward compatible

## Context (from discovery)
- Files/components involved:
  - `internal/store/sqlite.go` — `EnqueueWrite`, `updateExistingNote`, `detectContentConflict`, `AckQueueItems`, schema/migrations
  - `internal/api/notes_handler.go` — `createNote`, `updateNote`, `snapshotPendingBear`
  - `cmd/bridge/queue.go` — `applyCreate`, `applyUpdate`, ack construction
  - `internal/models/note.go` — Note struct (`ExpectedBearModifiedAt` field)
  - `internal/models/sync.go` — `SyncAckItem` (`BearModifiedAt` field)
  - `internal/store/store.go` — Store interface (no changes expected)
  - `internal/store/scan.go` — scan helpers for new column
- Related patterns: field-level conflict detection via `pending_bear_title`/`pending_bear_body` columns; idempotency-key deduplication in `EnqueueWrite`
- Dependencies: `modernc.org/sqlite` (pure Go), `moq` for mock generation

## Problem Analysis

### Problem 1: Rapid Sequential Writes
```
Consumer: POST update (title='B') → queue item #1 created, sync_status=pending_to_bear
Consumer: POST update (title='C') → queue item #2 created (separate item!)
Bridge:   lease items #1 and #2
Bridge:   apply #1 → Bear modified_at changes to T2
Bridge:   apply #2 → xcall update succeeds
Hub:      delta push arrives with modified_at=T2 (from item #1 apply)
Hub:      modified_at changed + pending_to_bear → detectContentConflict → FALSE CONFLICT
```
**Root cause**: Multiple queue items for the same note create a race where applying item N changes Bear's `modified_at`, triggering false conflict detection for item N+1.

### Problem 2: Bear Touch After Apply
```
Bridge:   applies consumer update → Bear modified_at changes to T2
Bridge:   next sync cycle reads Bear delta (modified_at=T2)
Consumer: POST update before delta push arrives at hub → sync_status=pending_to_bear
Hub:      delta push arrives with modified_at=T2 (echo of our own write!)
Hub:      modified_at changed + pending_to_bear → detectContentConflict → FALSE CONFLICT
```
**Root cause**: Hub cannot distinguish Bear's `modified_at` change caused by our own x-callback-url write from a genuine user edit in Bear.

## Development Approach
- **Testing approach**: TDD — write tests first for each behavior change
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task**
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change
- Maintain backward compatibility

## Testing Strategy
- **Unit tests**: required for every task
- Focus on `internal/store/sqlite_test.go` for store-level logic
- Focus on `internal/api/notes_handler_test.go` for API-level coalescing behavior
- Focus on `cmd/bridge/queue_test.go` for bridge ack changes

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix
- Update plan if implementation deviates from original scope
- Keep plan in sync with actual work done

## Implementation Steps

### Task 1: Add `expected_bear_modified_at` column and model field
- [x] write test in `internal/store/sqlite_test.go`: verify migration adds `expected_bear_modified_at` column to existing DB without the column
- [x] write test: verify new notes have `expected_bear_modified_at` as NULL by default
- [x] write test: verify `expected_bear_modified_at` round-trips through `CreateNote`/`GetNote`/`UpdateNote`
- [x] add `ExpectedBearModifiedAt *string` field to `models.Note` in `internal/models/note.go` (json:"-", same as pending_bear fields)
- [x] add `expected_bear_modified_at TEXT` column to CREATE TABLE in `internal/store/sqlite.go` schema (line ~121, after `pending_bear_body`)
- [x] add migration in `migratePendingBearColumns` (or new migration func) to ALTER TABLE ADD COLUMN for existing DBs
- [x] add column to `noteColumns()`, `noteValues()`, scan helpers in `internal/store/scan.go`
- [x] add column to all UPDATE/INSERT queries that touch notes: `CreateNote`, `UpdateNote`, `updateExistingNote` (both branches), `ackUpdateNoteStatus` (clear on synced transition)
- [x] run tests — must pass before next task

### Task 2: Bridge reports `BearModifiedAt` in ack
- [x] write test in `cmd/bridge/queue_test.go`: `applyUpdate` ack includes `BearModifiedAt` read from Bear SQLite after apply
- [x] write test: `applyCreate` ack includes `BearModifiedAt` read from Bear SQLite after create
- [x] write test: if Bear SQLite read fails after apply, `BearModifiedAt` is empty (graceful degradation)
- [x] add `BearModifiedAt string` field to `models.SyncAckItem` in `internal/models/sync.go` (`json:"bear_modified_at,omitempty"`)
- [x] in `cmd/bridge/queue.go` `applyUpdate` (line ~314-327): after verify sleep, read `updated.ModifiedAt` and return it via a new return value or set on ack
- [x] in `cmd/bridge/queue.go` `applyCreate` (line ~244-256): after verify, read note's `ModifiedAt` from Bear SQLite and include in ack
- [x] update `applyQueueItem` to pass `BearModifiedAt` from apply result to the `SyncAckItem`
- [x] run tests — must pass before next task

### Task 3: Hub stores `expected_bear_modified_at` on ack
- [x] write test in `internal/store/sqlite_test.go`: when `AckQueueItems` receives ack with `BearModifiedAt`, the note's `expected_bear_modified_at` is set to that value
- [x] write test: when ack transitions note to `synced` (otherPending=0), `expected_bear_modified_at` is cleared (set to NULL)
- [x] write test: when ack keeps note as `pending_to_bear` (otherPending>0), `expected_bear_modified_at` is set (not cleared)
- [x] modify `ackUpdateNoteStatus` in `internal/store/sqlite.go`: add `BearModifiedAt` to UPDATE queries — set it when `item.BearModifiedAt != ""`, clear it (NULL) when transitioning to `synced`
- [x] run tests — must pass before next task

### Task 4: Echo detection in `updateExistingNote`
- [x] write test: Bear delta push with `modified_at` matching `expected_bear_modified_at` while note is `pending_to_bear` → skip conflict detection, stay `pending_to_bear`
- [x] write test: Bear delta push with `modified_at` NOT matching `expected_bear_modified_at` while `pending_to_bear` → proceed to `detectContentConflict` as before
- [x] write test: Bear delta push with `expected_bear_modified_at` NULL → proceed to `detectContentConflict` as before (backward compat)
- [x] write test: after echo is recognized, `expected_bear_modified_at` is cleared (consumed)
- [x] modify `updateExistingNote` signature to accept `expectedBearModifiedAt *string`
- [x] add echo detection logic before `detectContentConflict` call (line ~923): if `expectedBearModifiedAt != nil && note.ModifiedAt == *expectedBearModifiedAt`, set `newSyncStatus = syncStatusPendingToBear` and clear `expected_bear_modified_at` without calling `detectContentConflict`
- [x] update `upsertNote` to pass `expectedBearModifiedAt` from the existing note row to `updateExistingNote`
- [x] add `expected_bear_modified_at` to the SELECT in `upsertNote` (line ~874-909)
- [x] run tests — must pass before next task

### Task 5: Queue coalescing for update→update
- [ ] write test in `internal/store/sqlite_test.go`: `EnqueueWrite` with action `update` for a note that has an existing `pending` update item → updates existing item's payload, returns existing item
- [ ] write test: `EnqueueWrite` with action `update` for a note that has existing `processing` update item → creates NEW item (don't touch in-flight items)
- [ ] write test: `EnqueueWrite` with action `update` for a note that has existing `applied`/`failed` update item → creates NEW item
- [ ] write test: coalesced item preserves original `idempotency_key`, new key also resolves to same item
- [ ] write test: `EnqueueWrite` with action `update` for a note with NO existing pending item → creates new item (unchanged behavior)
- [ ] in `EnqueueWrite` (line ~1531): after idempotency check, before INSERT, add: `SELECT id, payload FROM write_queue WHERE note_id = ? AND action = 'update' AND status = 'pending' LIMIT 1`
- [ ] if found: UPDATE existing item's `payload` with new payload; also insert a mapping row or update existing to allow both idempotency keys to resolve (simplest: update the existing item's payload and return it; store the new idempotency_key→existing_item_id mapping)
- [ ] run tests — must pass before next task

### Task 6: Queue coalescing for create→update (merge into create)
- [ ] write test: consumer creates note (queue item: create, pending), then calls `updateNote` → existing create item's payload is updated with new title/body, no second item created
- [ ] write test: create→update coalescing preserves tags from original create payload
- [ ] write test: `updateNote` handler returns 409 when `BearID` is nil — verify this guard is relaxed for notes with pending create items (or update coalescing path bypasses this check)
- [ ] modify `updateNote` handler in `internal/api/notes_handler.go`: when `note.BearID == nil`, check if a pending `create` queue item exists for this note; if yes, update its payload with new title/body instead of returning 409
- [ ] update note's title/body in DB (same as current updateNote logic)
- [ ] run tests — must pass before next task

### Task 7: Verify acceptance criteria
- [ ] verify Problem 1 scenario: rapid update→update produces single coalesced queue item, no false conflict
- [ ] verify Problem 2 scenario: Bear echo after apply is detected, no false conflict
- [ ] verify create→update coalescing: single create item with final content reaches Bear
- [ ] verify backward compat: notes without `expected_bear_modified_at` (NULL) behave as before
- [ ] verify real conflicts still detected: Bear user edits body + consumer edits body → conflict fires
- [ ] run full test suite: `make test`
- [ ] run race detector: `make test-race`
- [ ] run linter: `make lint`
- [ ] run formatter: `make fmt`
- [ ] verify test coverage does not decrease: `make test-coverage`

### Task 8: [Final] Update documentation
- [ ] update `CLAUDE.md` sync_status lifecycle section with echo detection and coalescing behavior
- [ ] update field-level conflict detection section to mention `expected_bear_modified_at`
- [ ] regenerate mocks if Store interface changed: `make generate`

## Technical Details

### New DB column
```sql
ALTER TABLE notes ADD COLUMN expected_bear_modified_at TEXT;
```
Core Data epoch float64 string (same format as `modified_at`). Set by bridge ack, consumed by echo detection, cleared on `synced` transition.

### Echo detection flow
```
Bridge applies update → reads Bear modified_at (T2) → ack {BearModifiedAt: "T2"}
Hub ackUpdateNoteStatus → UPDATE notes SET expected_bear_modified_at = 'T2'
Bear delta push arrives → modified_at = T2
updateExistingNote: T2 == expected_bear_modified_at → ECHO, skip conflict detection
                    clear expected_bear_modified_at (consumed)
```

### Queue coalescing flow
```
Consumer: PUT /api/notes/X {body: "v1"} → queue item #1 (action=update, payload={body:"v1"})
Consumer: PUT /api/notes/X {body: "v2"} → EnqueueWrite finds item #1 pending
           → UPDATE write_queue SET payload = '{"body":"v2"}' WHERE id = #1
           → return item #1 (coalesced)
Bridge: lease → only item #1 with payload {body:"v2"} (final version)
```

### Idempotency key handling for coalesced items
When coalescing, the new idempotency key needs to resolve to the coalesced item. Options:
1. **Secondary key column** — `write_queue` gets `secondary_idempotency_key` column
2. **Separate mapping table** — `idempotency_keys(key, consumer_id, queue_item_id)`
3. **Accept that new key doesn't resolve** — consumer retries with same key get idempotent response; new key on coalesced item is lost

Recommended: Option 1 (secondary key column) — simplest, handles the common case of exactly one coalescing event per queue item.

### SyncAckItem change
```go
type SyncAckItem struct {
    // ... existing fields ...
    BearModifiedAt string `json:"bear_modified_at,omitempty"`
}
```

## Post-Completion
**Manual verification:**
- Test with real Bear + OpenClaw: create note, rapid edits, verify no false conflicts
- Test echo scenario: update note, wait for bridge sync, update again before next delta push
- Monitor logs for "conflict" entries during normal OpenClaw usage

**Monitoring:**
- Track conflict rate before/after deployment to verify improvement
