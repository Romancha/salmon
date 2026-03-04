# Bridge Distribution via GitHub Releases

## Overview
- Automate building, signing, notarizing, and publishing bear-bridge + bear-xcall as a signed macOS distribution via GitHub Actions
- Users download a signed `.tar.gz` or `.zip` from GitHub Releases, extract, and run `make install-bridge`
- Triggered by pushing a `v*` tag (same pattern as existing docker-publish workflow for hub)
- Solves: manual build/sign process, Gatekeeper warnings for unsigned binaries

## Context
- **Existing CI:** `.github/workflows/ci.yml` (lint, test, test-race on ubuntu), `docker-publish.yml` (hub Docker image on tag push)
- **Code signing:** Makefile already supports `CODESIGN_IDENTITY` with entitlements and hardened runtime for bear-xcall.app
- **Build targets:** `bear-bridge` (Go binary), `bear-xcall.app` (Swift .app bundle), both output to `bin/`
- **macOS-only:** bridge requires macOS (Bear SQLite, launchd, xcallback)
- **No goreleaser:** project uses plain Makefile builds

## Development Approach
- **Testing approach**: Regular (code first, then tests)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task**
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change
- Maintain backward compatibility

## Testing Strategy
- **Unit tests**: required for every task with code changes
- **CI validation**: test the workflow with a dry-run or pre-release tag before real release

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with ➕ prefix
- Document issues/blockers with ⚠️ prefix
- Update plan if implementation deviates from original scope

## Implementation Steps

### Task 1: Add version injection via ldflags
- [x] Add `version` variable in `cmd/bridge/main.go` (set via `-ldflags -X`)
- [x] Add `--version` flag to bridge that prints version and exits
- [x] Update Makefile `build` target to accept `VERSION` variable and inject via ldflags
- [x] Write tests for version flag handling
- [x] Run tests — must pass before next task

### Task 2: Create GitHub Actions release workflow for bridge
- [x] Create `.github/workflows/release-bridge.yml` triggered on `v*` tags
- [x] Job runs on `macos-latest` (required for Swift compilation and code signing)
- [x] Steps: checkout, setup Go 1.26, build bridge binary with version from tag
- [x] Steps: compile bear-xcall.app via `swiftc` (same as Makefile build-xcall)
- [x] Steps: create release archive — `bear-bridge-darwin-arm64.tar.gz` containing:
  - `bin/bear-bridge`
  - `bin/bear-xcall.app/` (full bundle)
  - `deploy/com.romancha.bear-bridge.plist`
  - `deploy/bear-bridge-wrapper.sh`
  - `deploy/.env.bridge.example`
  - `Makefile` (for `make install-bridge`)
- [x] Steps: upload archive as artifact (for use in later signing step)
- [x] Verify workflow syntax with `actionlint` or similar

### Task 3: Add code signing and notarization to release workflow
- [x] Add GitHub secrets documentation/checklist:
  - `APPLE_CERTIFICATE` — base64-encoded Developer ID Application .p12
  - `APPLE_CERTIFICATE_PASSWORD` — .p12 password
  - `APPLE_TEAM_ID` — Apple Team ID
  - `APPLE_ID` — Apple ID email for notarization
  - `APPLE_ID_PASSWORD` — app-specific password for notarytool
- [x] Add workflow step: import certificate into temporary keychain
- [x] Add workflow step: sign `bear-bridge` binary with Developer ID (`codesign --sign "Developer ID Application: ..." --options runtime`)
- [x] Add workflow step: sign `bear-xcall.app` with Developer ID + entitlements (same as Makefile install-bridge)
- [x] Add workflow step: create zip for notarization, submit via `xcrun notarytool submit --wait`
- [x] Add workflow step: staple notarization ticket via `xcrun stapler staple`
- [x] Add workflow step: verify signatures with `codesign --verify --deep --strict`
- [x] Run tests — must pass before next task

### Task 4: Add GitHub Release publishing step
- [ ] Add workflow step: create final archive after signing/notarization
- [ ] Add workflow step: generate SHA256 checksum for archive
- [ ] Add workflow step: create GitHub Release using `softprops/action-gh-release` action
  - Title: tag name (e.g., `v0.1.0`)
  - Body: auto-generated from commits or changelog
  - Assets: `.tar.gz` archive + `.sha256` checksum file
- [ ] Add amd64 build support (Intel Mac) — build matrix or universal binary via `lipo`
- [ ] Test with a pre-release tag (e.g., `v0.0.1-rc.1`) to validate full pipeline

### Task 5: Update Makefile install-bridge for downloaded releases
- [ ] Ensure `make install-bridge` works from extracted archive (not just from repo root)
- [ ] Update `CODESIGN_IDENTITY` default — skip re-signing for already-signed releases (detect existing signature)
- [ ] Add `make verify-bridge` target to check signatures are valid
- [ ] Update documentation in deploy/.env.bridge.example if needed
- [ ] Write tests for any new Makefile logic
- [ ] Run tests — must pass before next task

### Task 6: Verify acceptance criteria
- [ ] Verify: `git tag v0.x.x && git push --tags` triggers full pipeline
- [ ] Verify: signed binary passes `spctl --assess --type execute`
- [ ] Verify: notarized app passes Gatekeeper on fresh macOS install
- [ ] Verify: `make install-bridge` works from downloaded archive
- [ ] Run full test suite (unit tests)
- [ ] Run linter — all issues must be fixed

### Task 7: [Final] Update documentation
- [ ] Add release process section to README.md
- [ ] Document required GitHub secrets setup
- [ ] Document how users install from GitHub Releases

## Technical Details

### Archive structure
```
bear-bridge-v0.1.0-darwin-arm64.tar.gz
├── bear-bridge              # signed Go binary
├── bear-xcall.app/          # signed + notarized Swift app bundle
│   └── Contents/
│       ├── MacOS/bear-xcall
│       └── Info.plist
├── com.romancha.bear-bridge.plist
├── bear-bridge-wrapper.sh
├── .env.bridge.example
└── install.sh               # simplified installer (alternative to Makefile)
```

### Code signing flow in CI
```
1. Import .p12 cert → temporary keychain
2. codesign bear-bridge (Go binary) — Developer ID + hardened runtime
3. codesign bear-xcall.app — Developer ID + entitlements + hardened runtime
4. Zip both → notarytool submit --wait
5. stapler staple bear-xcall.app (stapling .app only, not standalone binary)
6. Verify all signatures
7. Package into release archive
```

### GitHub Secrets needed
| Secret | Description |
|--------|-------------|
| `APPLE_CERTIFICATE` | Base64-encoded .p12 (Developer ID Application) |
| `APPLE_CERTIFICATE_PASSWORD` | Password for .p12 |
| `APPLE_TEAM_ID` | Apple Developer Team ID |
| `APPLE_ID` | Apple ID email for notarytool |
| `APPLE_ID_PASSWORD` | App-specific password for notarytool |

## Post-Completion

**Manual verification:**
- Download release on a clean macOS machine and verify Gatekeeper passes
- Test `make install-bridge` from extracted archive on fresh user account
- Verify launchd agent starts correctly after install

**Future improvements:**
- Homebrew tap formula for `brew install bear-bridge`
- Auto-update mechanism (Sparkle or custom)
- Universal binary (arm64 + amd64) via `lipo`
