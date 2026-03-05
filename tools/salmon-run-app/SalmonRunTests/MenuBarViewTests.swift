import XCTest

@testable import BearBridge

@MainActor
final class MenuBarViewTests: XCTestCase {

    // MARK: - StatusIndicator

    func testStatusIndicatorIdleConnected() {
        let indicator = StatusIndicator(
            status: .idle,
            lastSyncDescription: "2 minutes ago",
            bridgeConnected: true,
            lastError: nil
        )
        // Verify the view can be created with idle state
        XCTAssertNotNil(indicator.body)
    }

    func testStatusIndicatorSyncingConnected() {
        let indicator = StatusIndicator(
            status: .syncing,
            lastSyncDescription: "1 minute ago",
            bridgeConnected: true,
            lastError: nil
        )
        XCTAssertNotNil(indicator.body)
    }

    func testStatusIndicatorErrorWithMessage() {
        let indicator = StatusIndicator(
            status: .error,
            lastSyncDescription: "5 minutes ago",
            bridgeConnected: true,
            lastError: "Connection refused"
        )
        XCTAssertNotNil(indicator.body)
    }

    func testStatusIndicatorDisconnected() {
        let indicator = StatusIndicator(
            status: .idle,
            lastSyncDescription: "Never",
            bridgeConnected: false,
            lastError: nil
        )
        XCTAssertNotNil(indicator.body)
    }

    // MARK: - StatRow

    func testStatRowCreation() {
        let row = StatRow(label: "Notes", value: "1,234")
        XCTAssertNotNil(row.body)
    }

    // MARK: - QueueItemRow

    func testQueueItemRowPending() {
        let item = IPCQueueStatusItem(id: 1, action: "create", noteTitle: "My Note", status: "pending", createdAt: nil)
        let row = QueueItemRow(item: item)
        XCTAssertNotNil(row.body)
    }

    func testQueueItemRowApplied() {
        let item = IPCQueueStatusItem(id: 2, action: "update", noteTitle: "Test", status: "applied", createdAt: nil)
        let row = QueueItemRow(item: item)
        XCTAssertNotNil(row.body)
    }

    func testQueueItemRowFailed() {
        let item = IPCQueueStatusItem(id: 3, action: "trash", noteTitle: "Old Note", status: "failed", createdAt: nil)
        let row = QueueItemRow(item: item)
        XCTAssertNotNil(row.body)
    }

    func testQueueItemRowConflict() {
        let item = IPCQueueStatusItem(id: 4, action: "update", noteTitle: "Conflicted", status: "conflict", createdAt: nil)
        let row = QueueItemRow(item: item)
        XCTAssertNotNil(row.body)
    }

    func testQueueItemRowLeased() {
        let item = IPCQueueStatusItem(id: 5, action: "create", noteTitle: "New Note", status: "leased", createdAt: nil)
        let row = QueueItemRow(item: item)
        XCTAssertNotNil(row.body)
    }

    func testQueueItemRowEmptyTitle() {
        let item = IPCQueueStatusItem(id: 6, action: "delete_tag", noteTitle: "", status: "pending", createdAt: nil)
        let row = QueueItemRow(item: item)
        XCTAssertNotNil(row.body)
    }

    // MARK: - ActionButton

    func testActionButtonCreation() {
        var tapped = false
        let button = ActionButton(title: "Sync Now", systemImage: "arrow.clockwise") {
            tapped = true
        }
        XCTAssertNotNil(button.body)
        // Action callback is stored correctly
        button.action()
        XCTAssertTrue(tapped)
    }
}
