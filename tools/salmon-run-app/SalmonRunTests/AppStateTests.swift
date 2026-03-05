import XCTest

@testable import SalmonRun

final class SyncStatusTests: XCTestCase {

    func testSyncStatusDisplayText() {
        XCTAssertEqual(SyncStatus.idle.displayText, "Synced")
        XCTAssertEqual(SyncStatus.syncing.displayText, "Syncing...")
        XCTAssertEqual(SyncStatus.error.displayText, "Error")
    }

    func testSyncStatsDefaultValues() {
        let stats = SyncStats()
        XCTAssertEqual(stats.notesCount, 0)
        XCTAssertEqual(stats.tagsCount, 0)
        XCTAssertEqual(stats.queueCount, 0)
        XCTAssertEqual(stats.lastDurationMs, 0)
    }
}
