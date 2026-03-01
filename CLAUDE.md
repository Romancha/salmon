# bear-sync

Monorepo with two Go binaries for syncing Bear notes with openclaw.

## Project Structure

- cmd/hub/ — API server (runs on VPS)
- cmd/bridge/ — Mac agent that reads Bear SQLite and syncs with hub
- internal/models/ — shared data models (Note, Tag, Attachment, Backlink, WriteQueueItem)
- internal/mapper/ — Bear SQLite → Hub model mapping
- internal/beardb/ — Bear SQLite reader (bridge only)
- internal/hubclient/ — HTTP client for hub API (bridge only)
- internal/store/ — SQLite store for hub (hub only)
- internal/api/ — HTTP handlers with chi router (hub only)
- internal/xcallback/ — Bear x-callback-url executor via xcall CLI (bridge only)
- testdata/ — test fixtures (test Bear SQLite)

## Commands

- make build — build both binaries to bin/
- make test — run all tests
- make test-coverage — run tests with coverage report
- make test-race — run tests with race detector
- make lint — run golangci-lint
- make fmt — format code (gofumpt + goimports)
- make tidy — go mod tidy
- make generate — run go generate (moq)

## Code Patterns

- Interface-first design for testability (Store, BearDB, HubClient, XCallback)
- Mocks via moq with //go:generate directives
- Structured logging with log/slog
- context.Context in all external operations
- Error wrapping: fmt.Errorf("message: %w", err)
- Configuration via environment variables (no config files)
- Tests with github.com/stretchr/testify (assert/require)
- Line length limit: 140 characters
- SQLite via modernc.org/sqlite (pure Go, no CGO)

## Database

- Hub uses SQLite with WAL mode, busy_timeout=5000, foreign_keys=ON
- Dual-ID pattern: hub UUID (PK) + bear_id (Bear's UUID, nullable, UNIQUE)
- FTS5 for full-text search on notes (title + body)

## Sync Architecture

- Bear is source-of-truth for user content
- Hub is read replica + write queue for openclaw
- Write flow: openclaw → hub write_queue → bridge lease → xcall to Bear → ack
- Read flow: Bear → bridge delta export → hub sync/push → openclaw API
- Delivery: effectively-once (openclaw→hub), at-least-once (hub→bridge), duplicate-safe (bridge apply)

## Auth

- Two Bearer tokens: one for openclaw (api/* scope), one for bridge (sync/* scope)
- Encrypted notes are read-only (403 for write operations)
