# Field-Level Conflict Detection

## Overview

Replace timestamp-only conflict detection with field-level content comparison. Currently, any change to `modified_at` in Bear while a note is `pending_to_bear` triggers a conflict — even if the user only opened the note (metadata drift) without editing content. This causes false positives.

**New behavior:** conflict only fires when Bear changes a content field (title/body) that the consumer also changed. Metadata-only changes from Bear (modified_at, pinned, archived, etc.) are silently accepted without conflict.

**Problem it solves:** false conflict when user opens/touches a note in Bear between consumer create and update, without actually editing content.

## Context

- **Conflict detection logic:** `internal/store/sqlite.go` — `updateExistingNote()` (lines 855-897)
- **Lookup query:** `internal/store/sqlite.go` — `upsertNote()` (lines 834-853) fetches `id, sync_status, modified_at`
- **Consumer update handler:** `internal/api/notes_handler.go` — `updateNote()` (lines 317-425)
- **Existing tests:** `internal/store/sqlite_test.go` — `TestProcessSyncPush_ConflictDetection` (line 1125), `TestProcessSyncPush_NoConflictOnSameModifiedAt` (line 1162)
- **Note model:** `internal/models/note.go`
- **Ack logic:** `internal/store/sqlite.go` — `ackApplied()` (lines 1625-1687)

**Key architectural fact:** the pending_to_bear UPDATE query (lines 867-878) deliberately does NOT overwrite `title` and `body` — consumer's content is preserved in the hub record. The incoming Bear `note` parameter has Bear's current title/body. Both versions are available at conflict detection time.

**What's missing:** we need to know Bear's title/body at the moment `sync_status` transitioned to `pending_to_bear` (the "base" version). Currently not stored — hub record gets overwritten with consumer's version.

## Development Approach
- **Testing approach**: TDD — tests first, then implementation
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task**
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change
- Maintain backward compatibility

## Testing Strategy
- **Unit tests**: required for every task
- Focus on conflict detection scenarios in `sqlite_test.go`
- Table-driven tests for different field change combinations

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with + prefix
- Document issues/blockers with ! prefix
- Update plan if implementation deviates from original scope

## Implementation Steps

### Task 1: Add pending_bear_title / pending_bear_body columns to notes table

- [x] Write test: verify new columns exist after migration and are nullable
- [x] Add `pending_bear_title` and `pending_bear_body` TEXT columns (nullable) to `createNotesTable` in `internal/store/sqlite.go`
- [x] Add `PendingBearTitle` and `PendingBearBody` fields to `models.Note` struct (json:"-", not exposed in API responses, *string for nullable)
- [x] Update `noteValues()` / `scanNote()` in `internal/store/scan.go` to include new fields
- [x] Run tests — must pass before next task

### Task 2: Populate pending_bear columns on sync_status transition to pending_to_bear

- [x] Write test: after consumer `updateNote()`, verify `pending_bear_title` and `pending_bear_body` contain the pre-update Bear values
- [x] Write test: after consumer `createNote()`, verify `pending_bear_title` and `pending_bear_body` are empty (new note has no Bear base)
- [x] In `updateNote()` handler (`notes_handler.go`): before overwriting title/body, save current values to `note.PendingBearTitle` / `note.PendingBearBody`
- [x] Ensure rollback logic also restores pending_bear fields on enqueue failure
- [x] Run tests — must pass before next task

### Task 3: Clear pending_bear columns on sync_status transition to synced

- [x] Write test: after ack applied with no other pending items, verify `pending_bear_title` and `pending_bear_body` are cleared (empty/NULL)
- [x] Write test: after ack applied with other pending items, verify pending_bear fields are NOT cleared (sync_status stays pending_to_bear)
- [x] In `ackApplied()` (`sqlite.go`): when setting `sync_status = 'synced'`, also clear `pending_bear_title` and `pending_bear_body`
- [x] Run tests — must pass before next task

### Task 4: Field-level conflict detection in updateExistingNote()

- [x] Write test: Bear delta with changed `modified_at` but same title/body as pending_bear → NO conflict (metadata drift)
- [x] Write test: Bear delta with changed `modified_at` AND changed body (body differs from pending_bear_body), consumer also changed body → CONFLICT
- [x] Write test: Bear delta with changed `modified_at` AND changed title, but consumer only changed body → NO conflict (no field intersection)
- [x] Write test: Bear delta with changed `modified_at` AND changed both title and body, consumer changed body → CONFLICT (body intersects)
- [x] Write test: pending_bear fields are NULL/empty (create flow) — fallback to current behavior (timestamp-based conflict)
- [x] Update `upsertNote()` SELECT to also fetch `pending_bear_title`, `pending_bear_body`, `title`, `body`
- [x] Update `updateExistingNote()` signature to accept the new fields
- [x] Implement field-level comparison logic: conflict only if Bear changed a content field AND consumer's write queue also targets that field
- [x] Run tests — must pass before next task

### Task 5: Handle other write actions (trash, archive, add_tag)

- [x] Write test: trash action on a note where Bear changed body → NO conflict (trash doesn't modify title/body)
- [x] In `trashNote()`, `archiveNote()`, `addTag()` handlers: set `pending_bear_title`/`pending_bear_body` when transitioning to pending_to_bear
- [x] These actions don't change title/body, so field-level check should never conflict on content (only on metadata, which we now ignore)
- [x] Run tests — must pass before next task

### Task 6: Update existing conflict tests

- [x] Update `TestProcessSyncPush_ConflictDetection` to set up pending_bear fields and verify conflict fires only on content intersection
- [x] Update `TestProcessSyncPush_NoConflictOnSameModifiedAt` to verify it still passes
- [x] Add test: metadata-only change (modified_at differs, content same) → no conflict
- [x] Run tests — must pass before next task

### Task 7: Verify acceptance criteria

- [ ] Verify: metadata-only Bear changes don't trigger conflict
- [ ] Verify: content changes on non-intersecting fields don't trigger conflict
- [ ] Verify: content changes on intersecting fields DO trigger conflict
- [ ] Verify: create flow (no pending_bear base) falls back to timestamp-based detection
- [ ] Run full test suite (`make test`)
- [ ] Run race detector (`make test-race`)
- [ ] Run linter (`make lint`)
- [ ] Verify test coverage does not decrease (`make test-coverage`)

### Task 8: [Final] Update documentation

- [ ] Update CLAUDE.md sync_status lifecycle section to document field-level conflict detection
- [ ] Add comment in code explaining the field-level comparison strategy

## Technical Details

### Data flow

```
Consumer PUT /api/notes/:id  (body changed)
  ├── Save Bear's current title/body → note.PendingBearTitle, PendingBearBody
  ├── Overwrite note.Title/Body with consumer's version
  ├── Set sync_status = pending_to_bear
  └── EnqueueWrite (payload has consumer's changes)

Bridge delta push (ProcessSyncPush)
  ├── upsertNote() fetches: id, sync_status, modified_at, title, body,
  │                          pending_bear_title, pending_bear_body
  └── updateExistingNote() if pending_to_bear:
        ├── modified_at same? → no conflict (as before)
        ├── modified_at changed?
        │     ├── pending_bear fields empty? → fallback to timestamp conflict (create flow)
        │     ├── Compare Bear delta title vs pending_bear_title
        │     ├── Compare Bear delta body vs pending_bear_body
        │     ├── Bear changed title AND consumer changed title? → CONFLICT
        │     ├── Bear changed body AND consumer changed body? → CONFLICT
        │     └── Otherwise → no conflict, just metadata drift
        └── Update metadata fields (as before, title/body NOT overwritten)

Ack applied
  └── sync_status → synced: clear pending_bear_title, pending_bear_body
```

### Consumer field detection

To know which fields consumer changed, compare hub's current title/body with pending_bear_title/body:
- `hub.Title != pending_bear_title` → consumer changed title
- `hub.Body != pending_bear_body` → consumer changed body

### Conflict condition (pseudocode)

```
bearTitleChanged := bearDelta.Title != pendingBearTitle
bearBodyChanged := bearDelta.Body != pendingBearBody
consumerChangedTitle := hubTitle != pendingBearTitle
consumerChangedBody := hubBody != pendingBearBody

conflict := (bearTitleChanged && consumerChangedTitle) ||
            (bearBodyChanged && consumerChangedBody)
```

## Post-Completion

**Manual verification:**
- Test with real Bear app: create note via API, open in Bear (trigger metadata change), update via API — verify no false conflict
- Test real conflict: create note via API, edit in Bear, update same field via API — verify conflict fires
