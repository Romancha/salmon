# BearBridge App Improvements

## Overview
Three improvements to the BearBridge menu bar app:
1. Clean up README — remove all references to the pre-app era (launchd, headless agent, tar.gz archive)
2. Switch distribution to .dmg — proper macOS app distribution instead of tar.gz extraction
3. Redesign UI — follow MeetAlert (TimelyMeet) patterns for a polished native macOS experience

## Context
- **Reference project:** `/Users/romancha/dev/git/meet-alert/MeetAlert` (TimelyMeet)
- **Current app:** `tools/bear-bridge-app/` — SwiftUI menu bar app (macOS 13+, Swift Package Manager)
- **Current distribution:** signed tar.gz archives via GitHub Releases, installed via `make install-app`
- **Key files:** README.md, .github/workflows/release-bridge.yml, Makefile, all Swift sources in tools/bear-bridge-app/

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
- Update plan if implementation deviates from original scope

## Implementation Steps

### Task 1: Clean up README — remove pre-app era references
- [x] Remove "Launchd (headless)" section entirely (lines 214-255 area)
- [x] Remove "Switching from launchd Agent to Menu Bar App" section entirely (lines 336-362 area)
- [x] Remove "Install from GitHub Release" section that describes tar.gz extraction flow (lines 408-438 area)
- [x] Update Bridge description to not mention "one-shot" or launchd scheduling — bridge is managed by BearBridge.app
- [x] Update "Running" section under Bridge Setup — remove launchd references, focus on BearBridge.app as the primary way to run
- [x] Update "Install" section under Menu Bar App — describe .dmg installation (will be updated again in Task 3)
- [x] Remove `make install-bridge` / `make uninstall-bridge` references from README (keep in Makefile for developer use)
- [x] Remove references to `.env.bridge` config file — settings are managed by the app
- [x] Update "CI/CD" and "Publishing a release" sections to reflect .dmg distribution
- [x] Review entire README for any remaining references to the old headless/launchd flow and remove them
- [x] Ensure README still documents bridge CLI flags (`--daemon`, `--version`) for developer/advanced use

### Task 2: Create .dmg distribution workflow
- [ ] Create `tools/create-dmg.sh` script that:
  - Takes BearBridge.app path and output .dmg path as arguments
  - Creates a temporary DMG with BearBridge.app and an alias to /Applications
  - Sets window size, icon positions, background (optional)
  - Converts to compressed read-only DMG
  - Uses `hdiutil` (built into macOS, no external dependencies)
- [ ] Update `.github/workflows/release-bridge.yml`:
  - Replace tar.gz archive creation with .dmg creation
  - Create `BearBridge-{VERSION}-{arch}.dmg` per architecture
  - Sign the .dmg with Developer ID (`codesign --sign`)
  - Notarize the .dmg (submit .dmg directly to notarytool, not zip)
  - Staple notarization ticket to .dmg (`xcrun stapler staple`)
  - Generate SHA256 checksums for .dmg files
  - Upload .dmg + .sha256 as GitHub Release assets
  - Remove tar.gz archive creation steps
  - Remove inclusion of launchd plist, wrapper script, .env.bridge.example, entitlements.plist, Makefile in release
- [ ] Update Makefile:
  - Add `make dmg` target that calls `tools/create-dmg.sh`
  - Update `install-app` to work from .dmg context or local build
  - Remove release-archive-specific install logic (the `go.mod` detection for repo vs release)
- [ ] Update README install instructions to describe .dmg flow:
  - Download .dmg from GitHub Releases
  - Open .dmg, drag BearBridge.app to /Applications
  - Launch from /Applications
- [ ] Test full .dmg creation locally with `make dmg`

### Task 3: Migrate to Xcode project (prerequisite for UI redesign)
- [ ] Create `tools/bear-bridge-app/BearBridge.xcodeproj` using Xcode or `xcodebuild`
  - Target: macOS 13.0+
  - Bundle ID: com.romancha.bear-bridge
  - Product name: BearBridge
  - LSUIElement: true (menu bar only, no dock icon)
- [ ] Move Swift sources from `Sources/BearBridge/` into Xcode-compatible structure
- [ ] Migrate Info.plist into Xcode project settings
- [ ] Add Assets.xcassets with app icon (use SF Symbol or simple sync icon)
- [ ] Update Package.swift or remove if fully migrated to .xcodeproj
- [ ] Update Makefile `build-app` target to use `xcodebuild` instead of `swift build`
- [ ] Update Makefile `test-app` target to use `xcodebuild test`
- [ ] Update `.github/workflows/release-bridge.yml` build step for BearBridge.app
- [ ] Verify `make build-app` and `make test-app` still work
- [ ] Verify all existing tests pass

### Task 4: Redesign app architecture — adopt MeetAlert patterns
- [ ] Create `AppModel.swift` — central @StateObject managing all services (like MeetAlert's AppModel)
  - Owns: StatusViewModel, LogViewModel, SettingsManager, BridgeProcessManager, NotificationService, BridgeIPCClient
  - Single dependency injection point via @EnvironmentObject
- [ ] Create `AppRoot.swift` wrapper view for environment injection (like MeetAlert)
- [ ] Refactor `BearBridgeApp.swift`:
  - Use `@StateObject AppModel` instead of individual service instances
  - Use `MenuBarExtra(.window)` with `AppRoot` wrapping
  - Use `Settings { }` scene for settings (native macOS settings window)
  - Remove manual window management (openWindow, Notification-based window opening)
- [ ] Migrate ViewModels to use `@MainActor` consistently
- [ ] Update all tests for new architecture
- [ ] Verify all tests pass

### Task 5: Redesign Menu Bar popup UI
Based on MeetAlert patterns — clean sections, proper styling, native macOS feel.

- [ ] Redesign `MenuBarView.swift` with sections:
  ```
  ┌─────────────────────────────────────┐
  │  Sync Status                        │
  │  ● Connected — Last sync 2 min ago  │
  │                                     │
  │  ─────────────────────────────────  │
  │  Statistics                         │
  │  Notes    1,234                     │
  │  Tags     56                        │
  │  Queue    0 pending                 │
  │                                     │
  │  ─────────────────────────────────  │
  │  Write Queue (if items present)     │
  │  ┌ update "My Note"    pending ┐   │
  │  └ create "New Note"   leased  ┘   │
  │                                     │
  │  ─────────────────────────────────  │
  │  ⟳ Sync Now                        │
  │  📋 View Logs...                    │
  │  ⚙ Settings...                     │
  │  ─────────────────────────────────  │
  │  Quit Bear Bridge                   │
  │                                     │
  └─────────────────────────────────────┘
  ```
- [ ] Extract reusable components (like MeetAlert's Components/):
  - `StatusIndicator` — colored dot with label
  - `StatRow` — label + value row
  - `QueueItemRow` — write queue item display
  - `ActionButton` — styled menu action button
- [ ] Use proper SwiftUI styling:
  - `.groupBoxStyle` or section headers for visual grouping
  - Consistent padding and spacing
  - Native macOS control styling
  - Monospaced font for stats values
- [ ] Menu bar label view:
  - SF Symbol icon that changes based on status
  - Optional: show brief status text next to icon (like MeetAlert's SmartStatusText)
- [ ] Set appropriate `.defaultSize()` for the popup window
- [ ] Update tests for new view structure

### Task 6: Redesign Log Viewer window
- [ ] Redesign `LogViewerWindow.swift`:
  - Use `NavigationSplitView` or clean toolbar layout (like MeetAlert's settings)
  - Search field in toolbar
  - Level filter buttons with colored badges
  - Monospaced log entries with proper formatting
  - Color-coded log levels (debug: gray, info: blue, warn: orange, error: red)
  - Auto-scroll toggle in toolbar
  - Timestamp + level + message in each row
- [ ] Improve log entry rendering:
  - Use `Grid` or `LazyVStack` for better performance
  - Truncate long messages with disclosure
  - Copy-to-clipboard on click/right-click
- [ ] Update tests

### Task 7: Redesign Settings window
Based on MeetAlert's `EnhancedSettingsView` — NavigationSplitView with sidebar.

- [ ] Redesign `SettingsWindow.swift` using native `Settings` scene:
  - Use `TabView` with `.tabViewStyle(.automatic)` (macOS native tab style)
  - **Connection tab**: Hub URL, Hub Token (secure field), Bear Token (secure field)
  - **Sync tab**: Interval slider with live value display, Sync on launch toggle
  - **General tab**: Launch at Login, Notifications toggle
  - **About tab**: App version, bridge version, links
- [ ] Use `Form` with `Section` for clean macOS native layout
- [ ] Add validation indicators (checkmark when configured, warning when missing)
- [ ] Remove manual "Restart Bridge" button — auto-restart when settings change
- [ ] Update tests

### Task 8: Polish and verify
- [ ] Verify menu bar icon changes color correctly (green/yellow/red)
- [ ] Verify Sync Now triggers sync and shows progress
- [ ] Verify log viewer displays real-time logs with filtering
- [ ] Verify settings persist across app restarts
- [ ] Verify Launch at Login works
- [ ] Verify bridge auto-restarts on crash
- [ ] Run full test suite: `make test && make test-app`
- [ ] Run linter: `make lint`
- [ ] Update README with final installation instructions

## Technical Details

### .dmg Structure
```
BearBridge.dmg
├── BearBridge.app         # Signed + notarized app bundle
└── Applications (alias)   # Symlink to /Applications for drag-install
```

### App Bundle Structure (unchanged)
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

### MeetAlert Patterns to Adopt
1. **AppModel as central state** — single @StateObject owning all services
2. **AppRoot wrapper** — consistent environment injection
3. **Section-based popup** — clean visual grouping with headers
4. **Native Settings scene** — use SwiftUI `Settings { }` instead of manual windows
5. **@MainActor** — consistent thread safety annotations
6. **Component extraction** — reusable UI building blocks

### Distribution Changes Summary
| Aspect | Before | After |
|--------|--------|-------|
| Format | tar.gz archive | .dmg disk image |
| Installation | Extract + `make install-app` | Open .dmg, drag to /Applications |
| Location | ~/Applications/ | /Applications/ |
| Release assets | tar.gz + sha256 | .dmg + sha256 |
| Launchd support | Documented | Removed from docs |
