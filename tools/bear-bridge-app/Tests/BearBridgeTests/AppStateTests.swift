import XCTest

@testable import BearBridge

@MainActor
final class AppStateTests: XCTestCase {

    func testInitialState() {
        let state = AppState()

        XCTAssertEqual(state.syncStatus, .idle)
        XCTAssertNil(state.lastSyncTime)
        XCTAssertNil(state.lastError)
        XCTAssertEqual(state.stats.notesCount, 0)
        XCTAssertEqual(state.stats.tagsCount, 0)
        XCTAssertEqual(state.stats.queueCount, 0)
        XCTAssertEqual(state.stats.lastDurationMs, 0)
        XCTAssertFalse(state.bridgeRunning)
    }

    func testLastSyncDescriptionNever() {
        let state = AppState()
        XCTAssertEqual(state.lastSyncDescription, "Never")
    }

    func testLastSyncDescriptionWithDate() {
        let state = AppState()
        state.lastSyncTime = Date().addingTimeInterval(-120) // 2 minutes ago
        let desc = state.lastSyncDescription
        XCTAssertFalse(desc.isEmpty)
        XCTAssertNotEqual(desc, "Never")
    }

    func testUpdateFromStatusIdle() {
        let state = AppState()
        let stats = SyncStats(notesCount: 100, tagsCount: 10, queueCount: 0, lastDurationMs: 500)

        state.updateFromStatus(
            state: "idle",
            lastSync: "2026-03-04T12:00:00Z",
            lastError: nil,
            stats: stats
        )

        XCTAssertEqual(state.syncStatus, .idle)
        XCTAssertNotNil(state.lastSyncTime)
        XCTAssertNil(state.lastError)
        XCTAssertEqual(state.stats.notesCount, 100)
        XCTAssertEqual(state.stats.tagsCount, 10)
    }

    func testUpdateFromStatusSyncing() {
        let state = AppState()

        state.updateFromStatus(
            state: "syncing",
            lastSync: nil,
            lastError: nil,
            stats: nil
        )

        XCTAssertEqual(state.syncStatus, .syncing)
    }

    func testUpdateFromStatusError() {
        let state = AppState()

        state.updateFromStatus(
            state: "error",
            lastSync: nil,
            lastError: "connection refused",
            stats: nil
        )

        XCTAssertEqual(state.syncStatus, .error)
        XCTAssertEqual(state.lastError, "connection refused")
    }

    func testUpdateFromStatusUnknownFallsToIdle() {
        let state = AppState()

        state.updateFromStatus(
            state: "unknown_state",
            lastSync: nil,
            lastError: nil,
            stats: nil
        )

        XCTAssertEqual(state.syncStatus, .idle)
    }

    func testReset() {
        let state = AppState()
        state.syncStatus = .error
        state.lastSyncTime = Date()
        state.lastError = "some error"
        state.stats = SyncStats(notesCount: 50, tagsCount: 5, queueCount: 1, lastDurationMs: 100)
        state.bridgeRunning = true

        state.reset()

        XCTAssertEqual(state.syncStatus, .idle)
        XCTAssertNil(state.lastSyncTime)
        XCTAssertNil(state.lastError)
        XCTAssertEqual(state.stats.notesCount, 0)
        XCTAssertFalse(state.bridgeRunning)
    }

    func testSyncStatusDisplayText() {
        XCTAssertEqual(SyncStatus.idle.displayText, "Synced")
        XCTAssertEqual(SyncStatus.syncing.displayText, "Syncing...")
        XCTAssertEqual(SyncStatus.error.displayText, "Error")
    }

    func testSyncStatusIconColor() {
        XCTAssertEqual(SyncStatus.idle.iconColor, "green")
        XCTAssertEqual(SyncStatus.syncing.iconColor, "yellow")
        XCTAssertEqual(SyncStatus.error.iconColor, "red")
    }

    func testSyncStatsDefaultValues() {
        let stats = SyncStats()
        XCTAssertEqual(stats.notesCount, 0)
        XCTAssertEqual(stats.tagsCount, 0)
        XCTAssertEqual(stats.queueCount, 0)
        XCTAssertEqual(stats.lastDurationMs, 0)
    }

    func testUpdateFromStatusWithInvalidDateKeepsNil() {
        let state = AppState()

        state.updateFromStatus(
            state: "idle",
            lastSync: "not-a-date",
            lastError: nil,
            stats: nil
        )

        XCTAssertNil(state.lastSyncTime)
    }
}
