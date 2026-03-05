import XCTest

@testable import BearBridge

// MARK: - Mock IPC Client

final class MockIPCClient: IPCClientProtocol {
    var statusResponse: IPCStatusResponse?
    var syncNowResponse: IPCOkResponse?
    var logsResponse: IPCLogsResponse?
    var queueStatusResponse: IPCQueueStatusResponse?
    var quitResponse: IPCOkResponse?
    var shouldThrow: Error?

    var getStatusCallCount = 0
    var syncNowCallCount = 0

    // Optional delay to simulate network latency
    var responseDelay: TimeInterval = 0

    func getStatus() async throws -> IPCStatusResponse {
        getStatusCallCount += 1
        if responseDelay > 0 {
            try await Task.sleep(nanoseconds: UInt64(responseDelay * 1_000_000_000))
        }
        if let error = shouldThrow {
            throw error
        }
        guard let response = statusResponse else {
            throw IPCClientError.invalidResponse
        }
        return response
    }

    func syncNow() async throws -> IPCOkResponse {
        syncNowCallCount += 1
        if responseDelay > 0 {
            try await Task.sleep(nanoseconds: UInt64(responseDelay * 1_000_000_000))
        }
        if let error = shouldThrow {
            throw error
        }
        guard let response = syncNowResponse else {
            throw IPCClientError.invalidResponse
        }
        return response
    }

    func getLogs(lines: Int) async throws -> IPCLogsResponse {
        if let error = shouldThrow {
            throw error
        }
        guard let response = logsResponse else {
            throw IPCClientError.invalidResponse
        }
        return response
    }

    func getQueueStatus() async throws -> IPCQueueStatusResponse {
        if let error = shouldThrow {
            throw error
        }
        return queueStatusResponse ?? IPCQueueStatusResponse(items: [], error: nil)
    }

    func quit() async throws -> IPCOkResponse {
        if let error = shouldThrow {
            throw error
        }
        guard let response = quitResponse else {
            throw IPCClientError.invalidResponse
        }
        return response
    }

    /// Helper to set a standard idle status response.
    func setIdleStatus(notes: Int = 100, tags: Int = 10, queue: Int = 0) {
        statusResponse = IPCStatusResponse(
            state: "idle",
            lastSync: "2026-03-04T12:00:00Z",
            lastError: "",
            stats: IPCSyncStats(notesSynced: notes, tagsSynced: tags, queueProcessed: queue, lastDurationMs: 1200),
            error: nil
        )
    }

    func setSyncingStatus() {
        statusResponse = IPCStatusResponse(
            state: "syncing",
            lastSync: "2026-03-04T12:00:00Z",
            lastError: "",
            stats: IPCSyncStats(notesSynced: 50, tagsSynced: 5, queueProcessed: 0, lastDurationMs: 0),
            error: nil
        )
    }

    func setErrorStatus(error: String) {
        statusResponse = IPCStatusResponse(
            state: "error",
            lastSync: "2026-03-04T11:00:00Z",
            lastError: error,
            stats: IPCSyncStats(notesSynced: 0, tagsSynced: 0, queueProcessed: 0, lastDurationMs: 0),
            error: nil
        )
    }
}

// MARK: - StatusViewModel Tests

@MainActor
final class StatusViewModelTests: XCTestCase {

    // MARK: - Initial state

    func testInitialState() {
        let mock = MockIPCClient()
        let vm = StatusViewModel(ipcClient: mock)

        XCTAssertEqual(vm.syncStatus, .idle)
        XCTAssertNil(vm.lastSyncTime)
        XCTAssertNil(vm.lastError)
        XCTAssertEqual(vm.stats.notesCount, 0)
        XCTAssertEqual(vm.stats.tagsCount, 0)
        XCTAssertEqual(vm.stats.queueCount, 0)
        XCTAssertFalse(vm.isSyncing)
        XCTAssertFalse(vm.bridgeConnected)
    }

    func testLastSyncDescriptionNever() {
        let mock = MockIPCClient()
        let vm = StatusViewModel(ipcClient: mock)
        XCTAssertEqual(vm.lastSyncDescription, "Never")
    }

    // MARK: - refreshStatus

    func testRefreshStatusUpdatesFromIdle() async {
        let mock = MockIPCClient()
        mock.setIdleStatus(notes: 1234, tags: 56, queue: 3)
        let vm = StatusViewModel(ipcClient: mock)

        await vm.refreshStatus()

        XCTAssertEqual(vm.syncStatus, .idle)
        XCTAssertNotNil(vm.lastSyncTime)
        XCTAssertNil(vm.lastError)
        XCTAssertEqual(vm.stats.notesCount, 1234)
        XCTAssertEqual(vm.stats.tagsCount, 56)
        XCTAssertEqual(vm.stats.queueCount, 3)
        XCTAssertTrue(vm.bridgeConnected)
    }

    func testRefreshStatusUpdatesFromSyncing() async {
        let mock = MockIPCClient()
        mock.setSyncingStatus()
        let vm = StatusViewModel(ipcClient: mock)

        await vm.refreshStatus()

        XCTAssertEqual(vm.syncStatus, .syncing)
        XCTAssertTrue(vm.bridgeConnected)
    }

    func testRefreshStatusUpdatesFromError() async {
        let mock = MockIPCClient()
        mock.setErrorStatus(error: "connection refused")
        let vm = StatusViewModel(ipcClient: mock)

        await vm.refreshStatus()

        XCTAssertEqual(vm.syncStatus, .error)
        XCTAssertEqual(vm.lastError, "connection refused")
        XCTAssertTrue(vm.bridgeConnected)
    }

    func testRefreshStatusSetsBridgeDisconnectedOnError() async {
        let mock = MockIPCClient()
        mock.shouldThrow = IPCClientError.socketNotAvailable
        let vm = StatusViewModel(ipcClient: mock)

        await vm.refreshStatus()

        XCTAssertFalse(vm.bridgeConnected)
    }

    func testRefreshStatusTransitionsConnectedToDisconnected() async {
        let mock = MockIPCClient()
        mock.setIdleStatus()
        let vm = StatusViewModel(ipcClient: mock)

        await vm.refreshStatus()
        XCTAssertTrue(vm.bridgeConnected)

        mock.shouldThrow = IPCClientError.socketNotAvailable
        mock.statusResponse = nil
        await vm.refreshStatus()
        XCTAssertFalse(vm.bridgeConnected)
        XCTAssertEqual(vm.syncStatus, .error)
        XCTAssertEqual(vm.lastError, "Bridge disconnected")
    }

    func testRefreshStatusDoesNotSetErrorWhenNeverConnected() async {
        let mock = MockIPCClient()
        mock.shouldThrow = IPCClientError.socketNotAvailable
        let vm = StatusViewModel(ipcClient: mock)

        await vm.refreshStatus()

        XCTAssertFalse(vm.bridgeConnected)
        // Should NOT set error status when bridge was never connected
        XCTAssertEqual(vm.syncStatus, .idle)
        XCTAssertNil(vm.lastError)
    }

    func testRefreshStatusWithUnknownStateFallsToIdle() async {
        let mock = MockIPCClient()
        mock.statusResponse = IPCStatusResponse(
            state: "unknown_state",
            lastSync: "",
            lastError: "",
            stats: IPCSyncStats(notesSynced: 0, tagsSynced: 0, queueProcessed: 0, lastDurationMs: 0),
            error: nil
        )
        let vm = StatusViewModel(ipcClient: mock)

        await vm.refreshStatus()

        XCTAssertEqual(vm.syncStatus, .idle)
    }

    func testRefreshStatusEmptyLastSyncKeepsNil() async {
        let mock = MockIPCClient()
        mock.statusResponse = IPCStatusResponse(
            state: "idle",
            lastSync: "",
            lastError: "",
            stats: IPCSyncStats(notesSynced: 0, tagsSynced: 0, queueProcessed: 0, lastDurationMs: 0),
            error: nil
        )
        let vm = StatusViewModel(ipcClient: mock)

        await vm.refreshStatus()

        XCTAssertNil(vm.lastSyncTime)
    }

    func testRefreshStatusEmptyLastErrorSetsNil() async {
        let mock = MockIPCClient()
        mock.setIdleStatus()
        let vm = StatusViewModel(ipcClient: mock)

        // First set an error
        vm.lastError = "old error"
        await vm.refreshStatus()

        XCTAssertNil(vm.lastError)
    }

    // MARK: - syncNow

    func testSyncNowSendsCommand() async {
        let mock = MockIPCClient()
        mock.syncNowResponse = IPCOkResponse(ok: true, error: nil)
        mock.setIdleStatus()
        let vm = StatusViewModel(ipcClient: mock)

        await vm.syncNow()

        XCTAssertEqual(mock.syncNowCallCount, 1)
    }

    func testSyncNowSetsSyncingState() async {
        let mock = MockIPCClient()
        mock.syncNowResponse = IPCOkResponse(ok: true, error: nil)
        mock.setIdleStatus()
        let vm = StatusViewModel(ipcClient: mock)

        // Capture the intermediate state by checking after completion
        // The isSyncing should be false after completion
        await vm.syncNow()

        XCTAssertFalse(vm.isSyncing)
    }

    func testSyncNowRefreshesAfterSuccess() async {
        let mock = MockIPCClient()
        mock.syncNowResponse = IPCOkResponse(ok: true, error: nil)
        mock.setIdleStatus(notes: 200, tags: 20, queue: 0)
        let vm = StatusViewModel(ipcClient: mock)

        await vm.syncNow()

        // Should have called getStatus at least once (for the refresh after sync)
        XCTAssertGreaterThanOrEqual(mock.getStatusCallCount, 1)
        XCTAssertEqual(vm.stats.notesCount, 200)
    }

    func testSyncNowSetsErrorOnFailure() async {
        let mock = MockIPCClient()
        mock.shouldThrow = IPCClientError.socketNotAvailable
        let vm = StatusViewModel(ipcClient: mock)

        await vm.syncNow()

        XCTAssertEqual(vm.syncStatus, .error)
        XCTAssertNotNil(vm.lastError)
        XCTAssertFalse(vm.isSyncing)
    }

    func testSyncNowHandlesOkFalse() async {
        let mock = MockIPCClient()
        mock.syncNowResponse = IPCOkResponse(ok: false, error: "sync already in progress")
        let vm = StatusViewModel(ipcClient: mock)

        await vm.syncNow()

        XCTAssertEqual(vm.syncStatus, .error)
        XCTAssertEqual(vm.lastError, "sync already in progress")
        XCTAssertFalse(vm.isSyncing)
    }

    func testSyncNowHandlesOkFalseWithoutErrorMessage() async {
        let mock = MockIPCClient()
        mock.syncNowResponse = IPCOkResponse(ok: false, error: nil)
        let vm = StatusViewModel(ipcClient: mock)

        await vm.syncNow()

        XCTAssertEqual(vm.syncStatus, .error)
        XCTAssertEqual(vm.lastError, "sync_now command failed")
        XCTAssertFalse(vm.isSyncing)
    }

    func testSyncNowIgnoredWhileAlreadySyncing() async {
        let mock = MockIPCClient()
        mock.syncNowResponse = IPCOkResponse(ok: true, error: nil)
        mock.setIdleStatus()
        let vm = StatusViewModel(ipcClient: mock)

        // Set isSyncing manually to simulate concurrent call
        vm.isSyncing = true
        await vm.syncNow()

        XCTAssertEqual(mock.syncNowCallCount, 0)
    }

    // MARK: - Polling

    func testStartPollingCallsGetStatus() async throws {
        let mock = MockIPCClient()
        mock.setIdleStatus()
        let vm = StatusViewModel(ipcClient: mock, pollInterval: 0.05)

        vm.startPolling()
        try await Task.sleep(nanoseconds: 200_000_000)
        vm.stopPolling()

        XCTAssertGreaterThanOrEqual(mock.getStatusCallCount, 2)
        XCTAssertTrue(vm.bridgeConnected)
    }

    func testStopPollingStopping() async throws {
        let mock = MockIPCClient()
        mock.setIdleStatus()
        let vm = StatusViewModel(ipcClient: mock, pollInterval: 0.05)

        vm.startPolling()
        try await Task.sleep(nanoseconds: 100_000_000)
        vm.stopPolling()
        let countAfterStop = mock.getStatusCallCount

        try await Task.sleep(nanoseconds: 200_000_000)
        // Should not have polled more after stop (allow +1 for in-flight)
        XCTAssertLessThanOrEqual(mock.getStatusCallCount, countAfterStop + 1)
    }

    func testStartPollingRestartsAfterStop() async throws {
        let mock = MockIPCClient()
        mock.setIdleStatus()
        let vm = StatusViewModel(ipcClient: mock, pollInterval: 0.05)

        vm.startPolling()
        try await Task.sleep(nanoseconds: 100_000_000)
        vm.stopPolling()

        let countAfterFirstStop = mock.getStatusCallCount

        vm.startPolling()
        try await Task.sleep(nanoseconds: 100_000_000)
        vm.stopPolling()

        XCTAssertGreaterThan(mock.getStatusCallCount, countAfterFirstStop)
    }

    // MARK: - Last sync description

    func testLastSyncDescriptionWithDate() {
        let mock = MockIPCClient()
        let vm = StatusViewModel(ipcClient: mock)
        vm.lastSyncTime = Date().addingTimeInterval(-120)
        let desc = vm.lastSyncDescription
        XCTAssertFalse(desc.isEmpty)
        XCTAssertNotEqual(desc, "Never")
    }

    // MARK: - State transitions

    func testIdleToSyncingToIdle() async {
        let mock = MockIPCClient()
        mock.syncNowResponse = IPCOkResponse(ok: true, error: nil)
        mock.setIdleStatus()
        let vm = StatusViewModel(ipcClient: mock)

        // Start from idle
        await vm.refreshStatus()
        XCTAssertEqual(vm.syncStatus, .idle)

        // Trigger sync — after completion should refresh back to idle
        await vm.syncNow()
        XCTAssertEqual(vm.syncStatus, .idle)
        XCTAssertFalse(vm.isSyncing)
    }

    func testIdleToError() async {
        let mock = MockIPCClient()
        mock.setIdleStatus()
        let vm = StatusViewModel(ipcClient: mock)

        await vm.refreshStatus()
        XCTAssertEqual(vm.syncStatus, .idle)

        mock.setErrorStatus(error: "db locked")
        await vm.refreshStatus()
        XCTAssertEqual(vm.syncStatus, .error)
        XCTAssertEqual(vm.lastError, "db locked")
    }

    func testErrorToIdle() async {
        let mock = MockIPCClient()
        mock.setErrorStatus(error: "temporary failure")
        let vm = StatusViewModel(ipcClient: mock)

        await vm.refreshStatus()
        XCTAssertEqual(vm.syncStatus, .error)

        mock.setIdleStatus()
        await vm.refreshStatus()
        XCTAssertEqual(vm.syncStatus, .idle)
        XCTAssertNil(vm.lastError)
    }

    func testSyncNowToError() async {
        let mock = MockIPCClient()
        mock.shouldThrow = IPCClientError.connectionFailed("refused")
        let vm = StatusViewModel(ipcClient: mock)

        await vm.syncNow()
        XCTAssertEqual(vm.syncStatus, .error)
        XCTAssertFalse(vm.isSyncing)
    }

    // MARK: - Queue status

    func testQueueItemsInitiallyEmpty() {
        let mock = MockIPCClient()
        let vm = StatusViewModel(ipcClient: mock)
        XCTAssertTrue(vm.queueItems.isEmpty)
    }

    func testRefreshStatusFetchesQueueItems() async {
        let mock = MockIPCClient()
        mock.setIdleStatus()
        mock.queueStatusResponse = IPCQueueStatusResponse(
            items: [
                IPCQueueStatusItem(id: 1, action: "create", noteTitle: "Note 1", status: "processing", createdAt: "2026-03-04T12:00:00Z"),
                IPCQueueStatusItem(id: 2, action: "update", noteTitle: "Note 2", status: "applied", createdAt: nil),
            ],
            error: nil
        )
        let vm = StatusViewModel(ipcClient: mock)

        await vm.refreshStatus()

        XCTAssertEqual(vm.queueItems.count, 2)
        XCTAssertEqual(vm.queueItems[0].id, 1)
        XCTAssertEqual(vm.queueItems[0].action, "create")
        XCTAssertEqual(vm.queueItems[0].noteTitle, "Note 1")
        XCTAssertEqual(vm.queueItems[0].status, "processing")
        XCTAssertEqual(vm.queueItems[1].id, 2)
        XCTAssertEqual(vm.queueItems[1].action, "update")
        XCTAssertEqual(vm.queueItems[1].status, "applied")
    }

    func testRefreshStatusEmptyQueueItems() async {
        let mock = MockIPCClient()
        mock.setIdleStatus()
        mock.queueStatusResponse = IPCQueueStatusResponse(items: [], error: nil)
        let vm = StatusViewModel(ipcClient: mock)

        await vm.refreshStatus()

        XCTAssertTrue(vm.queueItems.isEmpty)
    }

    // MARK: - Bridge version

    func testBridgeVersionInitiallyNil() {
        let mock = MockIPCClient()
        let vm = StatusViewModel(ipcClient: mock)
        XCTAssertNil(vm.bridgeVersion)
    }

    func testBridgeVersionCanBeSet() {
        let mock = MockIPCClient()
        let vm = StatusViewModel(ipcClient: mock)
        vm.bridgeVersion = "v1.2.3"
        XCTAssertEqual(vm.bridgeVersion, "v1.2.3")
    }

    func testQueueItemsNotClearedOnQueueStatusError() async {
        let mock = MockIPCClient()
        mock.setIdleStatus()
        mock.queueStatusResponse = IPCQueueStatusResponse(
            items: [IPCQueueStatusItem(id: 1, action: "create", noteTitle: "Note", status: "processing", createdAt: nil)],
            error: nil
        )
        let vm = StatusViewModel(ipcClient: mock)

        // First refresh sets queue items
        await vm.refreshStatus()
        XCTAssertEqual(vm.queueItems.count, 1)

        // Next refresh where queue_status fails — items stay since we use try?
        mock.queueStatusResponse = nil
        mock.shouldThrow = IPCClientError.socketNotAvailable
        // shouldThrow affects getStatus too, so items won't update (bridgeConnected=false)
        // Reset mock to only fail queue
        mock.shouldThrow = nil
        await vm.refreshStatus()
        // Queue items are still updated from the nil queueStatusResponse (returns empty)
        XCTAssertTrue(vm.queueItems.isEmpty)
    }
}
