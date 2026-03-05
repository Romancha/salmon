import XCTest

@testable import BearBridge

// MARK: - Mock Notification Service

final class MockNotificationService: NotificationServiceProtocol {
    var isEnabled: Bool = true
    var shownErrors: [String] = []
    var showSyncErrorCallCount = 0

    func showSyncError(_ message: String) {
        showSyncErrorCallCount += 1
        if isEnabled {
            shownErrors.append(message)
        }
    }
}

// MARK: - NotificationRateLimiter Tests

final class NotificationRateLimiterTests: XCTestCase {

    func testFirstErrorIsAlwaysAllowed() {
        let limiter = NotificationRateLimiter(interval: 300)
        XCTAssertTrue(limiter.shouldAllow(error: "connection refused"))
    }

    func testSameErrorBlockedWithinInterval() {
        let limiter = NotificationRateLimiter(interval: 300)
        limiter.record(error: "connection refused")
        XCTAssertFalse(limiter.shouldAllow(error: "connection refused"))
    }

    func testDifferentErrorAllowedAfterRecord() {
        let limiter = NotificationRateLimiter(interval: 300)
        limiter.record(error: "connection refused")
        XCTAssertTrue(limiter.shouldAllow(error: "timeout"))
    }

    func testSameErrorAllowedAfterIntervalExpires() {
        var currentTime = Date()
        let limiter = NotificationRateLimiter(interval: 300) { currentTime }

        limiter.record(error: "connection refused")
        XCTAssertFalse(limiter.shouldAllow(error: "connection refused"))

        // Advance time past interval
        currentTime = currentTime.addingTimeInterval(301)
        XCTAssertTrue(limiter.shouldAllow(error: "connection refused"))
    }

    func testSameErrorBlockedJustBeforeIntervalExpires() {
        var currentTime = Date()
        let limiter = NotificationRateLimiter(interval: 300) { currentTime }

        limiter.record(error: "connection refused")

        // Advance time to just under interval
        currentTime = currentTime.addingTimeInterval(299)
        XCTAssertFalse(limiter.shouldAllow(error: "connection refused"))
    }

    func testSameErrorAllowedAtExactIntervalBoundary() {
        var currentTime = Date()
        let limiter = NotificationRateLimiter(interval: 300) { currentTime }

        limiter.record(error: "connection refused")

        // Advance time to exactly interval
        currentTime = currentTime.addingTimeInterval(300)
        XCTAssertTrue(limiter.shouldAllow(error: "connection refused"))
    }

    func testMultipleErrorsTrackedIndependently() {
        let limiter = NotificationRateLimiter(interval: 300)

        limiter.record(error: "error A")
        limiter.record(error: "error B")

        XCTAssertFalse(limiter.shouldAllow(error: "error A"))
        XCTAssertFalse(limiter.shouldAllow(error: "error B"))
        XCTAssertTrue(limiter.shouldAllow(error: "error C"))
    }

    func testResetClearsAllTracking() {
        let limiter = NotificationRateLimiter(interval: 300)

        limiter.record(error: "error A")
        limiter.record(error: "error B")
        XCTAssertFalse(limiter.shouldAllow(error: "error A"))

        limiter.reset()

        XCTAssertTrue(limiter.shouldAllow(error: "error A"))
        XCTAssertTrue(limiter.shouldAllow(error: "error B"))
    }

    func testRecordUpdatesTimestamp() {
        var currentTime = Date()
        let limiter = NotificationRateLimiter(interval: 300) { currentTime }

        limiter.record(error: "error A")

        // Advance time 200s and re-record
        currentTime = currentTime.addingTimeInterval(200)
        limiter.record(error: "error A")

        // At 400s from start (200s from last record) — should still be blocked
        currentTime = currentTime.addingTimeInterval(200)
        XCTAssertFalse(limiter.shouldAllow(error: "error A"))

        // At 501s from start (301s from last record) — should be allowed
        currentTime = currentTime.addingTimeInterval(101)
        XCTAssertTrue(limiter.shouldAllow(error: "error A"))
    }

    func testDefaultIntervalIsFiveMinutes() {
        var currentTime = Date()
        let limiter = NotificationRateLimiter { currentTime }

        limiter.record(error: "test")

        // At 299s — blocked
        currentTime = currentTime.addingTimeInterval(299)
        XCTAssertFalse(limiter.shouldAllow(error: "test"))

        // At 300s — allowed
        currentTime = currentTime.addingTimeInterval(1)
        XCTAssertTrue(limiter.shouldAllow(error: "test"))
    }

    func testStaticConstants() {
        XCTAssertEqual(NotificationService.categoryIdentifier, "SYNC_ERROR")
        XCTAssertEqual(NotificationService.actionOpenLogs, "OPEN_LOGS")
    }
}

// MARK: - StatusViewModel + Notification Integration Tests

@MainActor
final class StatusViewModelNotificationTests: XCTestCase {

    func testErrorStatusTriggersNotification() async {
        let mockIPC = MockIPCClient()
        let mockNotifications = MockNotificationService()
        mockIPC.setErrorStatus(error: "db locked")

        let vm = StatusViewModel(
            ipcClient: mockIPC,
            notificationService: mockNotifications
        )

        await vm.refreshStatus()

        XCTAssertEqual(mockNotifications.shownErrors, ["db locked"])
    }

    func testIdleStatusDoesNotTriggerNotification() async {
        let mockIPC = MockIPCClient()
        let mockNotifications = MockNotificationService()
        mockIPC.setIdleStatus()

        let vm = StatusViewModel(
            ipcClient: mockIPC,
            notificationService: mockNotifications
        )

        await vm.refreshStatus()

        XCTAssertTrue(mockNotifications.shownErrors.isEmpty)
    }

    func testSameErrorDoesNotTriggerDuplicateNotification() async {
        let mockIPC = MockIPCClient()
        let mockNotifications = MockNotificationService()
        mockIPC.setErrorStatus(error: "db locked")

        let vm = StatusViewModel(
            ipcClient: mockIPC,
            notificationService: mockNotifications
        )

        await vm.refreshStatus()
        await vm.refreshStatus()

        // Only one notification because lastError hasn't changed
        XCTAssertEqual(mockNotifications.shownErrors.count, 1)
    }

    func testDifferentErrorTriggersNewNotification() async {
        let mockIPC = MockIPCClient()
        let mockNotifications = MockNotificationService()

        let vm = StatusViewModel(
            ipcClient: mockIPC,
            notificationService: mockNotifications
        )

        mockIPC.setErrorStatus(error: "db locked")
        await vm.refreshStatus()

        mockIPC.setErrorStatus(error: "connection refused")
        await vm.refreshStatus()

        XCTAssertEqual(mockNotifications.shownErrors, ["db locked", "connection refused"])
    }

    func testErrorClearedThenReoccursTriggersNotification() async {
        let mockIPC = MockIPCClient()
        let mockNotifications = MockNotificationService()

        let vm = StatusViewModel(
            ipcClient: mockIPC,
            notificationService: mockNotifications
        )

        mockIPC.setErrorStatus(error: "db locked")
        await vm.refreshStatus()
        XCTAssertEqual(mockNotifications.shownErrors.count, 1)

        // Error clears
        mockIPC.setIdleStatus()
        await vm.refreshStatus()

        // Same error re-occurs
        mockIPC.setErrorStatus(error: "db locked")
        await vm.refreshStatus()
        XCTAssertEqual(mockNotifications.shownErrors.count, 2)
    }

    func testSyncNowFailureTriggersNotification() async {
        let mockIPC = MockIPCClient()
        let mockNotifications = MockNotificationService()
        mockIPC.shouldThrow = IPCClientError.socketNotAvailable

        let vm = StatusViewModel(
            ipcClient: mockIPC,
            notificationService: mockNotifications
        )

        await vm.syncNow()

        XCTAssertEqual(mockNotifications.showSyncErrorCallCount, 1)
    }

    func testNoNotificationServiceDoesNotCrash() async {
        let mockIPC = MockIPCClient()
        mockIPC.setErrorStatus(error: "test error")

        let vm = StatusViewModel(ipcClient: mockIPC)

        await vm.refreshStatus()

        XCTAssertEqual(vm.lastError, "test error")
    }

    func testDisabledNotificationServiceDoesNotRecord() async {
        let mockIPC = MockIPCClient()
        let mockNotifications = MockNotificationService()
        mockNotifications.isEnabled = false
        mockIPC.setErrorStatus(error: "db locked")

        let vm = StatusViewModel(
            ipcClient: mockIPC,
            notificationService: mockNotifications
        )

        await vm.refreshStatus()

        // showSyncError was called but isEnabled=false so no shownErrors recorded
        XCTAssertEqual(mockNotifications.showSyncErrorCallCount, 1)
        XCTAssertTrue(mockNotifications.shownErrors.isEmpty)
    }
}
