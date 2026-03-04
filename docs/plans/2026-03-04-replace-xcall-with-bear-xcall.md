# Replace xcall with bear-xcall Swift CLI

## Overview
- Replace the abandoned [xcall](https://github.com/martinfinke/xcall) (2017, unmaintained) with a custom Swift CLI `bear-xcall`
- xcall has known issues on modern macOS: hangs on M1/M2, no x-success response, permission errors
- bear-xcall does the same thing: takes `bear://x-callback-url/...` URL, returns JSON response to stdout
- Drop-in replacement: same CLI interface, same JSON output format, no changes to Go XCallback interface

## Context (from discovery)
- Files/components involved:
  - `tools/bear-xcall/main.swift` — new Swift CLI (to create)
  - `tools/bear-xcall/Info.plist` — URL scheme registration (to create)
  - `internal/xcallback/xcallback.go` — path resolution (`LookPath("xcall")` → bear-xcall)
  - `cmd/bridge/main.go` — initializes xcallback via `xcallback.New()`
  - `Makefile` — add Swift build step
- Related patterns found:
  - `CommandExecutor` interface abstracts exec for testing
  - `NewWithPath()` used in tests to skip LookPath
  - `bin/` already in .gitignore
- Dependencies identified:
  - macOS-only (bridge runs on Mac)
  - Swift compiler (`swiftc`) available on macOS by default
  - AppKit framework for NSWorkspace and NSAppleEventManager

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
- **Swift manual tests**: `tools/bear-xcall/BearXcallTests.swift` — XCTest suite for CLI validation, run via `make test-xcall` (requires macOS + Bear running)
- Go code tested via existing mock infrastructure (`CommandExecutor`, `XCallbackMock`)

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix
- Update plan if implementation deviates from original scope
- Keep plan in sync with actual work done

## Implementation Steps

### Task 1: Create bear-xcall Swift CLI
- [x] create `tools/bear-xcall/` directory
- [x] create `tools/bear-xcall/main.swift` with:
  - argument parsing (`-url`, `-timeout` with default 10s)
  - NSApplication setup for event loop
  - NSAppleEventManager handler for `kInternetEventClass`/`kAEGetURL`
  - inject `x-success=bear-xcall://x-callback-url/success` and `x-error=bear-xcall://x-callback-url/error` into the URL
  - open URL via `NSWorkspace.shared.open()`
  - on callback: parse query params → JSON dict → stdout, exit(0 or 1)
  - on timeout: error JSON → stderr, exit(2)
- [x] create `tools/bear-xcall/Info.plist` with `bear-xcall://` URL scheme registration and `CFBundleIdentifier`
- [x] verify `swiftc` compiles successfully: `swiftc -o bin/bear-xcall.app/Contents/MacOS/bear-xcall tools/bear-xcall/main.swift`
- [x] verify Info.plist copy: `cp tools/bear-xcall/Info.plist bin/bear-xcall.app/Contents/`

### Task 2: Create Swift manual tests for bear-xcall CLI
- [x] create `tools/bear-xcall/BearXcallTests.swift` with XCTest suite:
  - **CLI interface tests** (no Bear required):
    - test missing `-url` argument → exit code != 0, error message to stderr
    - test invalid URL format (not `bear://`) → exit code != 0, error message
    - test `-timeout` argument parsing (valid integer, invalid value)
    - test `--help` or no arguments shows usage
  - **Bear operation tests** (requires Bear running):
    - test create note: `-url "bear://x-callback-url/create?title=TestNote&text=body&tags=bear-xcall-test"` → exit 0, stdout JSON with `identifier` field
    - test open/read note: use identifier from create → exit 0, stdout JSON with `title`, `note` fields
    - test add-tag: add `bear-xcall-test-tag` to created note → exit 0
    - test trash note: trash the created test note → exit 0
  - **Error handling tests** (requires Bear running):
    - test open non-existent note ID → exit 1, stderr JSON with `errorCode`/`errorMessage`
    - test timeout: use `-timeout 1` with a URL that won't get a callback → exit 2
  - cleanup: trash any test notes created during the run (tag `bear-xcall-test`)
- [x] add `test-xcall` target to Makefile:
  - compile test file: `swiftc -o bin/bear-xcall-tests tools/bear-xcall/BearXcallTests.swift -framework XCTest`
  - run: `bin/bear-xcall-tests`
  - add `uname` guard (macOS only)
- [x] verify `make test-xcall` runs and CLI interface tests pass (Bear not required for these)
- [x] document in Makefile `help` target: `test-xcall — run bear-xcall manual tests (macOS + Bear)`

### Task 3: Update Makefile build
- [x] add `build-xcall` target that creates `.app` bundle structure, compiles Swift, copies Info.plist
- [x] add `uname` guard to skip on Linux
- [x] add `build-xcall` as dependency of `build` target
- [x] add `tools` and `help` sections to reflect bear-xcall
- [x] verify `make build` works on macOS
- [x] verify `make clean` removes `bin/bear-xcall.app/`

### Task 4: Update Go xcallback to use bear-xcall
- [x] update `xcallback.New()` in `internal/xcallback/xcallback.go`: change `LookPath("xcall")` to resolve `bear-xcall.app` bundle path
- [x] update `NewWithPath()` to accept `.app` bundle path
- [x] update `defaultExecutor.Run()` to call binary inside `.app/Contents/MacOS/`
- [x] update tests in `internal/xcallback/xcallback_test.go` to reflect new binary name
- [x] run tests: `make test` — must pass before next task

### Task 5: Update bridge initialization
- [ ] update `cmd/bridge/main.go` log messages from "xcall" to "bear-xcall"
- [ ] update any hardcoded "xcall" references in bridge code
- [ ] run tests: `make test` — must pass before next task

### Task 6: Verify acceptance criteria
- [ ] verify all requirements from Overview are implemented
- [ ] verify edge cases are handled (timeout, missing Bear, invalid URL)
- [ ] run full test suite: `make test`
- [ ] run tests with race detector: `make test-race`
- [ ] run manual CLI tests: `make test-xcall` — all tests must pass
- [ ] run linter: `make lint` — all issues must be fixed
- [ ] run formatter: `make fmt`

### Task 7: [Final] Update documentation
- [ ] update CLAUDE.md to mention bear-xcall instead of xcall
- [ ] update README.md if xcall is mentioned
- [ ] update deploy/ docs if xcall installation instructions exist

## Technical Details

### Swift CLI architecture
```
bear-xcall -url "bear://x-callback-url/create?title=Test" [-timeout 10]

Flow:
1. Parse args: -url, -timeout (default 10s)
2. Create NSApplication (needed for macOS event loop)
3. Register URL scheme handler via NSAppleEventManager (kInternetEventClass/kAEGetURL)
4. Inject x-success/x-error callback URLs into the bear:// URL:
   &x-success=bear-xcall://x-callback-url/success
   &x-error=bear-xcall://x-callback-url/error
5. Open URL via NSWorkspace.shared.open()
6. Run event loop with timeout
7. On callback: parse query params → JSON → stdout, exit(0 or 1)
8. On timeout: error JSON → stderr, exit(2)
```

### File structure
```
tools/bear-xcall/
  main.swift              — single-file Swift CLI (~80-100 lines)
  Info.plist              — URL scheme registration (bear-xcall://)
  BearXcallTests.swift    — XCTest manual tests (CLI + Bear operations)

bin/bear-xcall.app/   — built .app bundle (gitignored)
  Contents/
    MacOS/bear-xcall  — compiled binary
    Info.plist        — copied from tools/
```

### Output format (backward compatible with xcall)

Success (stdout):
```json
{
  "identifier": "UUID",
  "title": "Note Title",
  "note": "Note body..."
}
```

Error (stderr):
```json
{
  "errorCode": 1,
  "errorMessage": "description"
}
```

## Post-Completion

**Manual verification:**
- Test bear-xcall with real Bear app on macOS: create, update, add-tag, trash
- Verify timeout behavior when Bear is not running
- Verify callback works on both Intel and Apple Silicon Macs

**Deployment:**
- Update launchd plist if xcall path was referenced
- Ensure bear-xcall.app is built during bridge deployment
