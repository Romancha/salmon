# Salmon Rebranding

## Overview
Full rebranding of the project from bear-sync/bear-bridge/BearBridge to Salmon-themed naming.

**Metaphor:** Bear (Bear.app notes) catches Salmon (data/notes) — salmon is the data that flows upstream from Bear to consumers and back. The bidirectional data flow mirrors the salmon run migration.

**Naming scheme:**
- Repository: `bear-sync` -> `salmon`
- Go module: `github.com/romancha/bear-sync` -> `github.com/romancha/salmon`
- Hub binary: `bear-sync-hub` -> `salmon-hub`
- Bridge binary: `bear-bridge` -> `salmon-run`
- macOS app: `BearBridge.app` -> `SalmonRun.app`
- bear-xcall: stays as-is (it calls Bear.app, name is logical)
- Env vars: `BRIDGE_*` -> `SALMON_*`, `HUB_*` -> `SALMON_HUB_*`
- State files: `~/.bear-bridge-state.json` -> `~/.salmon-state.json`
- IPC socket: `~/.bear-bridge.sock` -> `~/.salmon.sock`
- Lock file: `~/.bear-bridge.lock` -> `~/.salmon.lock`
- Docker image: `ghcr.io/romancha/bear-sync-hub` -> `ghcr.io/romancha/salmon-hub`
- Systemd service: `bear-sync-hub.service` -> `salmon-hub.service`

## Context
- ~100+ files affected across Go, Swift, YAML, Makefile, Dockerfile, docs
- Go module rename requires updating all import paths in ~50+ Go files
- Swift/Xcode project rename is the most complex part (pbxproj, directory structure, bundle IDs)
- CI/CD workflows reference binary names and app names extensively
- This is a breaking change for deployed instances (env vars, paths, service names)

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task**
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with + prefix
- Document issues/blockers with ! prefix

## Implementation Steps

### Task 1: Go module rename and import paths
- [x] Update `go.mod` module path: `github.com/romancha/bear-sync` -> `github.com/romancha/salmon`
- [x] Run `find . -name '*.go' -exec sed` to replace all import paths `github.com/romancha/bear-sync/` -> `github.com/romancha/salmon/`
- [x] Update any Go string literals referencing "bear-sync" or "bear-bridge" (log messages, help text, version strings)
- [x] Run `go mod tidy` to verify module consistency
- [x] Run `make test` - must pass before next task

### Task 2: Rename environment variables
- [x] In `cmd/bridge/main.go`: rename all `BRIDGE_*` env vars to `SALMON_*`
- [x] In `cmd/hub/main.go`: rename `HUB_*` env vars to `SALMON_HUB_*` where appropriate
- [x] Update default file paths: `.bear-bridge-state.json` -> `.salmon-state.json`, `.bear-bridge.sock` -> `.salmon.sock`, `.bear-bridge.lock` -> `.salmon.lock`
- [x] Update `.gitignore` entries for new file names
- [x] Update any tests referencing old env var names or file paths
- [x] Run `make test` - must pass before next task

### Task 3: Rename binary names in Makefile and build system
- [x] In `Makefile`: `BINARY_HUB=bear-sync-hub` -> `BINARY_HUB=salmon-hub`
- [x] In `Makefile`: `BINARY_BRIDGE=bear-bridge` -> `BINARY_BRIDGE=salmon-run`
- [x] Update all help text and echo statements in Makefile
- [x] Update `Dockerfile`: binary name references `bear-sync-hub` -> `salmon-hub`
- [x] Update `docker-compose.yml`: image name `bear-sync-hub` -> `salmon-hub`
- [x] Run `make build` to verify binaries build correctly
- [x] Run `make test` - must pass before next task

### Task 4: Rename deployment configs
- [x] Rename file `deploy/bear-sync-hub.service` -> `deploy/salmon-hub.service`
- [x] Update service file contents: description, user, group, paths (`/opt/bear-sync/` -> `/opt/salmon/`)
- [x] Update `deploy/Caddyfile`: domain and log path references
- [x] Update `deploy/Caddyfile.docker`: domain reference

### Task 5: Rename CI/CD workflows
- [x] Update `.github/workflows/docker-publish.yml`: image name `bear-sync-hub` -> `salmon-hub`
- [x] Update `.github/workflows/release-bridge.yml`: all binary names, app names, DMG names
  - `bear-bridge` -> `salmon-run`
  - `BearBridge.app` -> `SalmonRun.app`
  - `BearBridge-*.dmg` -> `SalmonRun-*.dmg`
- [x] Update `.github/workflows/ci.yml` if it references binary/app names
- [x] Verify workflow YAML syntax is valid

### Task 6: Rename Swift/Xcode project — directory structure
- [x] Rename directory `tools/bear-bridge-app/` -> `tools/salmon-run-app/`
- [x] Rename directory `tools/salmon-run-app/BearBridge/` -> `tools/salmon-run-app/SalmonRun/`
- [x] Rename directory `tools/salmon-run-app/BearBridgeTests/` -> `tools/salmon-run-app/SalmonRunTests/`
- [x] Rename `BearBridge.xcodeproj` -> `SalmonRun.xcodeproj`
- [x] Rename Swift files that have "BearBridge" in their name (e.g., `BearBridgeApp.swift` -> `SalmonRunApp.swift`)

### Task 7: Update Swift source code
- [x] Update all Swift files: class/struct/enum names containing "BearBridge" -> "SalmonRun"
- [x] Update `@main` app struct name
- [x] Update UI strings: window titles, menu text, about text
- [x] Update `BridgeProcessManager` references to binary name `bear-bridge` -> `salmon-run`
- [x] Update `SettingsManager` references to env var names
- [x] Update `BridgeIPCClient` socket path references
- [x] Update `KeychainService` fallback bundle ID
- [x] Update all test files: `@testable import BearBridge` -> `@testable import SalmonRun`, env var references
+ Bundle identifier in project.pbxproj moved to Task 8 (requires pbxproj access)

### Task 8: Update Xcode project.pbxproj
- [x] Update all file references for renamed files/directories
- [x] Update product name: "BearBridge" -> "SalmonRun"
- [x] Update target names
- [x] Update scheme references
- [x] Update Info.plist paths if changed
- [x] Run `make build-app` to verify Xcode project builds

### Task 9: Update Swift tests
- [ ] Rename test files containing "BearBridge" -> "SalmonRun"
- [ ] Update test class names and import statements
- [ ] Update test assertions that reference old names/paths
- [ ] Run `make test-app` - must pass before next task

### Task 10: Update Makefile app-related targets
- [ ] Update `build-app` target: project path, scheme name
- [ ] Update `test-app` target: project path, scheme name
- [ ] Update `dmg` target: app name references
- [ ] Update `tools/create-dmg.sh`: volume name, app name references
- [ ] Run `make build-app` and `make dmg` to verify

### Task 11: Update bear-xcall bundle ID (keep name, update org)
- [ ] Update `tools/bear-xcall/Info.plist`: bundle ID `com.bear-sync.bear-xcall` -> `com.salmon.bear-xcall`
- [ ] Update any Makefile references if the bundle ID is used

### Task 12: Update README.md with Salmon branding and metaphor
- [ ] Rewrite README header and description with Salmon branding
- [ ] Add "Why Salmon?" section explaining the metaphor:
  - Bear catches salmon = Bear.app is the source of notes
  - Salmon run (upstream migration) = bridge pulling data from Bear to hub
  - Salmon stream = hub serving data to consumers
  - The bidirectional flow of data mirrors salmon lifecycle
- [ ] Update all binary names, paths, commands throughout README
- [ ] Update installation instructions (DMG names, binary names)
- [ ] Update deployment section (service name, paths, Docker image)
- [ ] Update environment variable documentation with new names

### Task 13: Update CLAUDE.md
- [ ] Update project name and description
- [ ] Update project structure section with new paths
- [ ] Update commands section with new binary/target names
- [ ] Update all references to env vars, file paths, binary names
- [ ] Update CI/CD section with new workflow references

### Task 14: Update remaining documentation
- [ ] Update `docs/consumer-api.md` with new naming
- [ ] Update completed plans in `docs/plans/completed/` — leave as historical record (no changes needed)

### Task 15: Verify acceptance criteria
- [ ] `make fmt` passes
- [ ] `make lint` passes with no warnings
- [ ] `make test` passes
- [ ] `make test-race` passes
- [ ] `make build` produces `salmon-hub` and `salmon-run` binaries
- [ ] `make build-app` produces `SalmonRun.app`
- [ ] `make build-xcall` still produces `bear-xcall.app`
- [ ] No remaining references to "bear-sync", "bear-bridge", "BearBridge" except in:
  - bear-xcall (intentionally kept)
  - `internal/beardb/` package name (reads Bear.app SQLite, name is correct)
  - Historical docs in `docs/plans/completed/`
  - Git history

## Post-Completion

**Manual verification:**
- Test SalmonRun.app on macOS — verify menu bar, settings, IPC all work
- Test salmon-run binary in daemon mode with new env vars
- Test salmon-hub Docker image builds and runs

**External system updates:**
- Rename GitHub repository from `bear-sync` to `salmon`
- Update Docker registry tags
- Update deployment on VPS: new binary paths, service name, env vars
- Update any API consumers with new endpoint domain if changed
- GitHub will auto-redirect old repo URL, but update bookmarks/docs

**Breaking changes to communicate:**
- All environment variables renamed (BRIDGE_* -> SALMON_*, HUB_* -> SALMON_HUB_*)
- State file path changed (~/.bear-bridge-state.json -> ~/.salmon-state.json)
- IPC socket path changed (~/.bear-bridge.sock -> ~/.salmon.sock)
- Binary names changed
- Docker image name changed
- Systemd service name changed
