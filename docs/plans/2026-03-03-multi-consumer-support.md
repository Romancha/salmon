# Multi-Consumer Support

## Overview
Refactor bear-sync hub from being hardcoded to a single external consumer (openclaw) to supporting multiple external consumers, each with its own token and identity. All consumers share the same API surface (`/api/notes`, `/api/tags`, `/api/attachments`) and a single write queue with a `consumer_id` column for attribution.

**Problem:** Currently the hub is tightly coupled to "openclaw" — env var names, auth scopes, comments, and docs all reference it by name. Only one external token is supported.

**Key benefits:**
- Any number of external consumers can read notes and enqueue writes
- Each consumer is identified by name and authenticated with its own token
- Write queue items are attributed to the originating consumer
- Bridge processes all queue items regardless of consumer
- openclaw becomes just one configured consumer, not a hardcoded concept

## Context (from discovery)

**Files requiring changes:**
- `cmd/hub/main.go` — env var parsing (`HUB_OPENCLAW_TOKEN` → `HUB_CONSUMER_TOKENS`)
- `internal/api/server.go` — Server struct, auth middleware, route setup
- `internal/api/notes_handler.go` — write queue enqueue calls (pass consumer_id)
- `internal/api/tags_handler.go` — write queue enqueue call (pass consumer_id)
- `internal/store/sqlite.go` — schema migration, EnqueueWrite signature, queries
- `internal/store/store.go` — Store interface (EnqueueWrite signature)
- `internal/models/models.go` — WriteQueueItem struct (add ConsumerID field)
- `cmd/bridge/queue.go` — ack handling (consumer_id passthrough if needed)
- `CLAUDE.md`, `README.md` — documentation updates

**Patterns to preserve:**
- Interface-first design (Store interface)
- Env-var-only configuration (no config files)
- `context.Context` in all operations
- Error wrapping with `fmt.Errorf("...: %w", err)`
- Mocks via moq

## Development Approach
- **Testing approach**: TDD (tests first)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task**
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change
- Maintain backward compatibility during migration

## Testing Strategy
- **Unit tests**: required for every task
- Focus on auth middleware tests (multi-token parsing, consumer identification)
- Update all existing tests that reference openclaw token/scope
- Test write queue with consumer_id attribution

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix
- Update plan if implementation deviates from original scope

## Implementation Steps

### Task 1: Add ConsumerID to models and Store interface
- [x] write tests for `WriteQueueItem` with `ConsumerID` field serialization
- [x] add `ConsumerID string` field to `models.WriteQueueItem` in `internal/models/models.go`
- [x] update `Store.EnqueueWrite` signature in `internal/store/store.go` to accept `consumerID string` parameter
- [x] update moq-generated mocks: `make generate`
- [x] run tests — must pass before next task

### Task 2: Migrate database schema — add consumer_id column to write_queue
- [x] write tests for schema migration: new `write_queue` table has `consumer_id TEXT NOT NULL DEFAULT ''` column
- [x] add `consumer_id TEXT NOT NULL DEFAULT ''` column to `write_queue` CREATE TABLE in `internal/store/sqlite.go`
- [x] update `EnqueueWrite` implementation to INSERT consumer_id value
- [x] update all write_queue scan/read queries to include consumer_id in SELECT and scan into `ConsumerID`
- [x] write tests for EnqueueWrite with consumer_id (verify stored and returned)
- [x] run tests — must pass before next task

### Task 3: Implement multi-consumer token parsing
- [x] write tests for token parsing: `"openclaw:token1,myapp:token2"` → `map[string]string{"openclaw":"token1","myapp":"token2"}`
- [x] write tests for edge cases: empty string, single consumer, whitespace trimming, missing colon (error)
- [x] implement `parseConsumerTokens(raw string) (map[string]string, error)` in `internal/api/server.go`
- [x] run tests — must pass before next task

### Task 4: Refactor auth middleware for multi-consumer support
- [x] write tests for new auth middleware: valid consumer token → request proceeds with consumer_id in context; invalid token → 403; missing header → 401
- [x] write tests: consumer_id is extractable from request context after auth
- [x] replace `openclawToken string` field in Server struct with `consumerTokens map[string]string` (name→token mapping)
- [x] add context key type and `ConsumerIDFromContext(ctx) string` helper
- [x] refactor `authMiddleware` to support scopes: `"consumer"` (matches any consumer token, sets consumer_id in ctx), `"bridge"`, `"any"`
- [x] update route setup: replace `authMiddleware("openclaw")` with `authMiddleware("consumer")`
- [x] run tests — must pass before next task

### Task 5: Update hub main.go — env var migration
- [x] write tests for config validation: `HUB_CONSUMER_TOKENS` required, parsed correctly
- [x] replace `HUB_OPENCLAW_TOKEN` env var with `HUB_CONSUMER_TOKENS` in `cmd/hub/main.go`
- [x] update Server constructor call to pass parsed `map[string]string`
- [x] update startup log messages to list registered consumer names (not tokens)
- [x] run tests — must pass before next task

### Task 6: Pass consumer_id through API handlers to write queue
- [x] write tests: POST /api/notes creates queue item with correct consumer_id from auth context
- [x] write tests: PUT /api/notes/{id} creates queue item with correct consumer_id
- [x] write tests: DELETE /api/notes/{id} creates queue item with correct consumer_id
- [x] write tests: POST /api/notes/{noteID}/tags creates queue item with correct consumer_id
- [x] update `handleCreateNote` to extract consumer_id from context and pass to `EnqueueWrite`
- [x] update `handleUpdateNote` to extract consumer_id from context and pass to `EnqueueWrite`
- [x] update `handleTrashNote` to extract consumer_id from context and pass to `EnqueueWrite`
- [x] update `handleAddTag` to extract consumer_id from context and pass to `EnqueueWrite`
- [x] run tests — must pass before next task

### Task 7: Update bridge queue processing (passthrough)
- [x] verify bridge queue processing handles `ConsumerID` field (scan from API response)
- [x] write test: bridge ack works regardless of consumer_id value
- [x] update bridge queue tests if any mock data needs ConsumerID
- [x] run tests — must pass before next task

### Task 8: Rename all openclaw references in code
- [x] grep for remaining "openclaw" references in `.go` files (comments, variable names, log messages)
- [x] rename to generic terms: "consumer", "client", "external"
- [x] update test names and test data that reference "openclaw"
- [x] run tests — must pass before next task

### Task 9: Verify acceptance criteria
- [ ] verify multiple consumers can authenticate with different tokens
- [ ] verify write queue items are attributed to correct consumer
- [ ] verify bridge processes queue items from any consumer
- [ ] run full test suite: `make test`
- [ ] run linter: `make lint`
- [ ] run tests with race detector: `make test-race`

### Task 10: Update documentation
- [ ] update `CLAUDE.md` — replace openclaw references with generic consumer terminology
- [ ] update `README.md` — document `HUB_CONSUMER_TOKENS` env var format and multi-consumer setup
- [ ] update deploy configs if they reference `HUB_OPENCLAW_TOKEN`

## Technical Details

**Token format:**
```
HUB_CONSUMER_TOKENS="openclaw:secret1,myapp:secret2,another:secret3"
```
Parsed into `map[string]string`:
```go
map[string]string{
    "openclaw": "secret1",
    "myapp":    "secret2",
    "another":  "secret3",
}
```

**Context propagation:**
```go
type contextKey string
const consumerIDKey contextKey = "consumer_id"

func ConsumerIDFromContext(ctx context.Context) string {
    v, _ := ctx.Value(consumerIDKey).(string)
    return v
}
```

**Auth middleware change:**
- Current: `authMiddleware("openclaw")` checks `r.Header.Get("Authorization")` against single `openclawToken`
- New: `authMiddleware("consumer")` iterates `consumerTokens` map, finds matching token, sets consumer name in context

**Write queue schema change:**
```sql
-- Added column:
consumer_id TEXT NOT NULL DEFAULT ''
```

**EnqueueWrite signature change:**
```go
// Before:
EnqueueWrite(ctx context.Context, idempotencyKey, action, noteID, payload string) (*models.WriteQueueItem, error)
// After:
EnqueueWrite(ctx context.Context, idempotencyKey, action, noteID, payload, consumerID string) (*models.WriteQueueItem, error)
```

## Post-Completion

**Manual verification:**
- Test with multiple consumer tokens configured
- Verify existing openclaw integration still works (just needs token format change in deployment)
- Check bridge sync cycle works end-to-end with new schema

**Deployment updates:**
- Update systemd unit / environment file on VPS: `HUB_OPENCLAW_TOKEN` → `HUB_CONSUMER_TOKENS="openclaw:<existing-token>"`
- Notify openclaw team of env var change (non-breaking if they only change deployment config)
