# Bear Bridge Menu Bar App

## Overview
- Native macOS SwiftUI menu bar application that wraps bear-bridge Go binary
- Replaces headless launchd agent with a user-friendly GUI showing sync status, logs, statistics
- Bridge runs as child process managed by the app; communication via stdout (JSON slog logs) + Unix socket for commands
- App is part of the signed distribution package from the bridge-distribution plan

## Context
- **Depends on:** `2026-03-04-bridge-distribution.md` (signing, notarization, GitHub Releases)
- **Current bridge:** headless CLI, one-shot execution every 5 min via launchd, JSON slog logging
- **bear-xcall.app:** existing Swift .app bundle in `tools/bear-xcall/` — pattern reference for Swift build in project
- **Bridge architecture:** reads Bear SQLite → pushes to hub → processes write queue → applies via xcallback
- **Stack:** SwiftUI (macOS 13+), bridge as child process via `Process()`

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task**
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change
- Maintain backward compatibility (bridge CLI must still work standalone)

## Testing Strategy
- **Unit tests (Go)**: required for daemon mode, Unix socket IPC, status reporting
- **Unit tests (Swift)**: XCTest for view models, IPC client, log parsing
- **Integration**: manual testing of menu bar app + bridge interaction

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix
- Update plan if implementation deviates from original scope

## Implementation Steps

### Task 1: Add daemon mode to bear-bridge
- [x] Add `--daemon` flag to `cmd/bridge/main.go` — runs sync loop continuously instead of one-shot
- [x] Add configurable sync interval via `BRIDGE_SYNC_INTERVAL` env (default 300s, matching current launchd)
- [x] Implement graceful shutdown in daemon mode (SIGTERM/SIGINT stops loop)
- [x] Ensure one-shot mode (no `--daemon`) still works identically for backward compatibility
- [x] Write tests for daemon loop start/stop, interval timing, signal handling
- [x] Run tests — must pass before next task

### Task 2: Add Unix socket IPC to bridge daemon
- [x] Create `internal/ipc/` package with Unix socket server
- [x] Socket path: `~/.bear-bridge.sock` (configurable via `BRIDGE_IPC_SOCKET`)
- [x] Implement JSON-based request/response protocol:
  - `{"cmd":"status"}` → `{"state":"idle|syncing|error","last_sync":"...","last_error":"...","stats":{...}}`
  - `{"cmd":"sync_now"}` → triggers immediate sync, returns `{"ok":true}`
  - `{"cmd":"logs","lines":50}` → returns last N log entries
  - `{"cmd":"quit"}` → graceful shutdown
- [x] Add stats tracking to bridge: notes synced, tags synced, write queue items processed, last sync duration
- [x] Server only starts in `--daemon` mode
- [x] Write tests for IPC server (all commands, malformed input, concurrent connections)
- [x] Write tests for stats tracking
- [x] Run tests — must pass before next task

### Task 3: Add structured status output to bridge sync
- [x] Emit structured JSON status events to stdout during sync:
  - `{"event":"sync_start","time":"..."}`
  - `{"event":"sync_progress","phase":"reading_bear|pushing_hub|processing_queue","notes":150}`
  - `{"event":"sync_complete","duration_ms":1200,"notes_synced":5,"tags_synced":2,"queue_items":1}`
  - `{"event":"sync_error","error":"..."}`
- [x] Maintain backward compatibility — status events are additional to existing slog output
- [x] Write tests for status event emission
- [x] Run tests — must pass before next task

### Task 4: Create SwiftUI menu bar app project
- [x] Create `tools/bear-bridge-app/` directory structure:
  - `BearBridge/` — Xcode project or Swift Package with executable target
  - `BearBridge/App/BearBridgeApp.swift` — @main entry, MenuBarExtra
  - `BearBridge/Views/` — SwiftUI views
  - `BearBridge/Services/` — bridge process manager, IPC client
  - `BearBridge/Models/` — data models
- [x] Create `BearBridgeApp.swift` with `MenuBarExtra` (macOS 13+)
- [x] Menu bar icon: SF Symbol `arrow.triangle.2.circlepath` (sync icon), changes color based on status
- [x] Basic menu popup with placeholder items
- [x] Add build target to Makefile: `make build-app` (compiles via `xcodebuild` or `swift build`)
- [x] Verify app launches and shows icon in menu bar
- [x] Write XCTests for app model initialization

### Task 5: Implement bridge process manager in Swift
- [x] Create `BridgeProcessManager` class:
  - Locates `bear-bridge` binary (bundled in .app or in `~/bin/`)
  - Launches as `Process()` with `--daemon` flag
  - Captures stdout/stderr via `Pipe()`
  - Monitors process lifecycle (restart on unexpected exit, max 3 retries)
  - Passes environment variables from app settings
- [x] Parse JSON log lines from stdout into structured `LogEntry` model
- [x] Parse status events from stdout for real-time UI updates
- [x] Handle process termination on app quit
- [x] Write XCTests for process manager (mock Process)
- [x] Run tests — must pass before next task

### Task 6: Implement IPC client in Swift
- [x] Create `BridgeIPCClient` class:
  - Connects to Unix socket at `~/.bear-bridge.sock`
  - Sends JSON commands, parses JSON responses
  - Auto-reconnect on connection loss
  - Timeout handling (5s per request)
- [x] Implement methods: `getStatus()`, `syncNow()`, `getLogs(lines:)`, `quit()`
- [x] Publish status updates via Combine/async streams for SwiftUI binding
- [x] Write XCTests for IPC client (mock socket)
- [x] Run tests — must pass before next task

### Task 7: Build menu bar UI — status and sync control
- [x] Menu bar popup layout:
  ```
  ┌─────────────────────────┐
  │ ● Synced                │  ← status indicator (green/yellow/red)
  │ Last sync: 2 min ago    │
  │ ─────────────────────── │
  │ ▸ Sync Now              │  ← triggers immediate sync
  │ ─────────────────────── │
  │ Notes: 1,234            │
  │ Tags: 56                │
  │ Queue: 0 pending        │
  │ ─────────────────────── │
  │ ▸ View Logs...          │  ← opens log window
  │ ▸ Settings...           │  ← opens settings window
  │ ─────────────────────── │
  │ ▸ Quit Bear Bridge      │
  └─────────────────────────┘
  ```
- [x] Status indicator: green (synced), yellow (syncing), red (error) — reflected in menu bar icon
- [x] "Sync Now" button sends IPC command, shows spinner during sync
- [x] Statistics section shows live data from IPC status response
- [x] Wire up `BridgeIPCClient` to SwiftUI `@Observable` view model
- [x] Write XCTests for view model state transitions
- [x] Run tests — must pass before next task

### Task 8: Build log viewer window
- [x] Create `LogViewerWindow` — separate SwiftUI window opened from menu
- [x] Display log entries in scrollable list with:
  - Timestamp, level (color-coded), message
  - Auto-scroll to bottom for new entries
  - Search/filter by text
  - Level filter (error, warning, info, debug)
- [x] Log entries populated from IPC `logs` command + live stdout stream
- [x] Limit displayed entries (last 500, configurable)
- [x] Write XCTests for log filtering and parsing
- [x] Run tests — must pass before next task

### Task 9: Build settings window
- [x] Create `SettingsWindow` with tabs/sections:
  - **Connection**: Hub URL, Hub Token, Bear Token (secure storage in Keychain)
  - **Sync**: interval slider (1-30 min), sync on app launch toggle
  - **General**: Launch at Login toggle (via `SMAppService` or `LoginItem`), notification preferences
- [x] Settings persist to `UserDefaults` (non-sensitive) and `Keychain` (tokens)
- [x] Settings generate environment for bridge process (replaces `.env.bridge`)
- [x] Launch at Login uses `SMAppService.mainApp` (macOS 13+)
- [x] Write XCTests for settings persistence and environment generation
- [x] Run tests — must pass before next task

### Task 10: Add error notifications
- [x] Show macOS notification (`UNUserNotificationCenter`) on sync errors
- [x] Notification shows error summary, click opens log viewer
- [x] Rate-limit notifications (max 1 per 5 minutes for same error)
- [x] Notification preference in settings (on/off)
- [x] Write XCTests for notification rate limiting
- [x] Run tests — must pass before next task

### Task 11: Add write queue status display
- [x] Show pending write queue items count in menu popup
- [x] Expandable section showing queue item details:
  - Action type (create, update, add_tag, etc.)
  - Target note title
  - Status (pending, leased, conflict)
- [x] Extend IPC protocol: `{"cmd":"queue_status"}` → queue items list
- [x] Implement Go-side handler for queue_status IPC command
- [x] Write tests (Go + Swift) for queue status
- [x] Run tests — must pass before next task

### Task 12: Integrate app into distribution workflow
- [x] Update `release-bridge.yml` to build menu bar app (`xcodebuild` on macOS runner)
- [x] Sign app with Developer ID Application certificate
- [x] Notarize the .app bundle
- [x] Include `BearBridge.app` in release archive alongside bear-bridge CLI
- [x] Update `install.sh` to handle .app installation (copy to `/Applications/` or `~/Applications/`)
- [x] Verify full pipeline with pre-release tag

### Task 13: Verify acceptance criteria
- [x] Verify: menu bar icon appears and shows correct status
- [x] Verify: Sync Now triggers immediate sync and shows progress
- [x] Verify: log viewer displays real-time logs with filtering
- [x] Verify: settings persist across app restarts
- [x] Verify: Launch at Login works
- [x] Verify: bridge auto-restarts if process dies
- [x] Verify: app + bridge work after download from GitHub Releases (signed + notarized)
- [x] Run full test suite (Go unit tests + Swift XCTests)
- [x] Run linter — all issues must be fixed

### Task 14: [Final] Update documentation
- [x] Update README.md with menu bar app section
- [x] Add screenshots of menu bar UI
- [x] Document settings and configuration
- [x] Document how to switch from launchd agent to menu bar app

## Technical Details

### IPC Protocol (Unix Socket)
```
Socket: ~/.bear-bridge.sock
Format: newline-delimited JSON

Request:  {"cmd":"status"}\n
Response: {"state":"idle","last_sync":"2026-03-04T12:00:00Z","last_error":"","stats":{"notes":1234,"tags":56,"queue":0,"last_duration_ms":1200}}\n

Request:  {"cmd":"sync_now"}\n
Response: {"ok":true}\n

Request:  {"cmd":"logs","lines":50}\n
Response: {"entries":[{"time":"...","level":"info","msg":"..."},...]}\n

Request:  {"cmd":"queue_status"}\n
Response: {"items":[{"action":"update","note_title":"...","status":"pending"},...]}\n

Request:  {"cmd":"quit"}\n
Response: {"ok":true}\n
```

### App bundle structure
```
BearBridge.app/
└── Contents/
    ├── MacOS/
    │   ├── BearBridge          # SwiftUI app binary
    │   ├── bear-bridge         # Go bridge binary (embedded)
    │   └── bear-xcall.app/     # xcallback helper (embedded)
    ├── Resources/
    │   └── Assets.xcassets
    └── Info.plist
```

### Status event stream (stdout)
```json
{"event":"sync_start","time":"2026-03-04T12:00:00Z"}
{"event":"sync_progress","phase":"reading_bear","notes":1234}
{"event":"sync_progress","phase":"pushing_hub","notes":5}
{"event":"sync_progress","phase":"processing_queue","items":2}
{"event":"sync_complete","duration_ms":1200,"notes_synced":5,"tags_synced":2,"queue_items":1}
```

### Data flow
```
BearBridge.app (SwiftUI)
    │
    ├── Process() → bear-bridge --daemon
    │       │
    │       ├── stdout → JSON log lines + status events → parsed by app
    │       └── ~/.bear-bridge.sock → IPC commands ← app sends status/sync_now/logs
    │
    └── UserDefaults + Keychain → settings → env vars for bridge process
```

## Post-Completion

**Manual verification:**
- Test on macOS 13 (Ventura) and macOS 14+ (Sonoma)
- Verify Gatekeeper passes for signed/notarized .app
- Test upgrade path: launchd agent → menu bar app (remove plist, install app)
- Test with large note collection (1000+ notes)
- Verify memory usage is acceptable for long-running daemon

**Future improvements:**
- Sparkle auto-update framework integration
- Quick note creation from menu bar
- Keyboard shortcut for Sync Now (global hotkey)
- Touch Bar support (if relevant)
- Widgets for macOS Sonoma desktop widgets
