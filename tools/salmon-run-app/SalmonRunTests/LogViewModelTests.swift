import XCTest

@testable import SalmonRun

@MainActor
final class LogViewModelTests: XCTestCase {

    // MARK: - Helpers

    private func makeEntry(
        level: LogLevel = .info,
        message: String = "test message",
        fields: [String: String] = [:]
    ) -> LogEntry {
        LogEntry(time: Date(), level: level, message: message, fields: fields)
    }

    private func makeEntries(count: Int, level: LogLevel = .info) -> [LogEntry] {
        (0..<count).map { i in
            LogEntry(time: Date(), level: level, message: "message \(i)")
        }
    }

    // MARK: - Initial state

    func testInitialState() {
        let vm = LogViewModel()

        XCTAssertTrue(vm.entries.isEmpty)
        XCTAssertTrue(vm.searchText.isEmpty)
        XCTAssertEqual(vm.activeLevels, Set(LogLevel.allCases))
        XCTAssertTrue(vm.autoScroll)
        XCTAssertFalse(vm.isLoading)
        XCTAssertEqual(vm.maxEntries, LogViewModel.defaultMaxEntries)
    }

    func testCustomMaxEntries() {
        let vm = LogViewModel(maxEntries: 100)
        XCTAssertEqual(vm.maxEntries, 100)
    }

    // MARK: - Adding entries

    func testAddSingleEntry() {
        let vm = LogViewModel()
        let entry = makeEntry()

        vm.addEntry(entry)

        XCTAssertEqual(vm.entries.count, 1)
        XCTAssertEqual(vm.entries.first?.message, "test message")
    }

    func testAddMultipleEntries() {
        let vm = LogViewModel()
        let entries = makeEntries(count: 5)

        vm.addEntries(entries)

        XCTAssertEqual(vm.entries.count, 5)
    }

    func testAddEntryTrimsWhenOverLimit() {
        let vm = LogViewModel(maxEntries: 3)
        let entries = makeEntries(count: 3)
        vm.addEntries(entries)

        vm.addEntry(makeEntry(message: "new entry"))

        XCTAssertEqual(vm.entries.count, 3)
        XCTAssertEqual(vm.entries.last?.message, "new entry")
        XCTAssertEqual(vm.entries.first?.message, "message 1")
    }

    func testAddEntriesTrimsWhenOverLimit() {
        let vm = LogViewModel(maxEntries: 5)

        vm.addEntries(makeEntries(count: 8))

        XCTAssertEqual(vm.entries.count, 5)
        XCTAssertEqual(vm.entries.first?.message, "message 3")
        XCTAssertEqual(vm.entries.last?.message, "message 7")
    }

    func testClearEntries() {
        let vm = LogViewModel()
        vm.addEntries(makeEntries(count: 10))

        vm.clearEntries()

        XCTAssertTrue(vm.entries.isEmpty)
    }

    // MARK: - Text search filtering

    func testFilteredEntriesNoFilter() {
        let vm = LogViewModel()
        vm.addEntries(makeEntries(count: 5))

        XCTAssertEqual(vm.filteredEntries.count, 5)
    }

    func testFilteredEntriesByText() {
        let vm = LogViewModel()
        vm.addEntry(makeEntry(message: "sync started"))
        vm.addEntry(makeEntry(message: "reading bear db"))
        vm.addEntry(makeEntry(message: "sync complete"))

        vm.searchText = "sync"

        XCTAssertEqual(vm.filteredEntries.count, 2)
    }

    func testFilteredEntriesCaseInsensitive() {
        let vm = LogViewModel()
        vm.addEntry(makeEntry(message: "SYNC STARTED"))
        vm.addEntry(makeEntry(message: "sync complete"))
        vm.addEntry(makeEntry(message: "other"))

        vm.searchText = "Sync"

        XCTAssertEqual(vm.filteredEntries.count, 2)
    }

    func testFilteredEntriesSearchInFields() {
        let vm = LogViewModel()
        vm.addEntry(makeEntry(message: "note updated", fields: ["note_id": "ABC123"]))
        vm.addEntry(makeEntry(message: "tag added", fields: ["tag": "work"]))

        vm.searchText = "ABC123"

        XCTAssertEqual(vm.filteredEntries.count, 1)
        XCTAssertEqual(vm.filteredEntries.first?.message, "note updated")
    }

    func testFilteredEntriesEmptySearch() {
        let vm = LogViewModel()
        vm.addEntries(makeEntries(count: 3))

        vm.searchText = ""

        XCTAssertEqual(vm.filteredEntries.count, 3)
    }

    func testFilteredEntriesNoMatch() {
        let vm = LogViewModel()
        vm.addEntries(makeEntries(count: 3))

        vm.searchText = "nonexistent"

        XCTAssertTrue(vm.filteredEntries.isEmpty)
    }

    // MARK: - Level filtering

    func testFilterByLevel() {
        let vm = LogViewModel()
        vm.addEntry(makeEntry(level: .info, message: "info msg"))
        vm.addEntry(makeEntry(level: .error, message: "error msg"))
        vm.addEntry(makeEntry(level: .debug, message: "debug msg"))

        vm.activeLevels = [.error]

        XCTAssertEqual(vm.filteredEntries.count, 1)
        XCTAssertEqual(vm.filteredEntries.first?.message, "error msg")
    }

    func testFilterByMultipleLevels() {
        let vm = LogViewModel()
        vm.addEntry(makeEntry(level: .info, message: "info"))
        vm.addEntry(makeEntry(level: .error, message: "error"))
        vm.addEntry(makeEntry(level: .debug, message: "debug"))
        vm.addEntry(makeEntry(level: .warn, message: "warn"))

        vm.activeLevels = [.info, .error]

        XCTAssertEqual(vm.filteredEntries.count, 2)
    }

    func testFilterNoLevelsActive() {
        let vm = LogViewModel()
        vm.addEntries(makeEntries(count: 5))

        vm.activeLevels = []

        XCTAssertTrue(vm.filteredEntries.isEmpty)
    }

    func testToggleLevel() {
        let vm = LogViewModel()

        XCTAssertTrue(vm.isLevelActive(.debug))
        vm.toggleLevel(.debug)
        XCTAssertFalse(vm.isLevelActive(.debug))
        vm.toggleLevel(.debug)
        XCTAssertTrue(vm.isLevelActive(.debug))
    }

    func testIsLevelActive() {
        let vm = LogViewModel()

        for level in LogLevel.allCases {
            XCTAssertTrue(vm.isLevelActive(level))
        }
    }

    // MARK: - Combined text + level filtering

    func testTextAndLevelFilterCombined() {
        let vm = LogViewModel()
        vm.addEntry(makeEntry(level: .info, message: "sync started"))
        vm.addEntry(makeEntry(level: .error, message: "sync failed"))
        vm.addEntry(makeEntry(level: .info, message: "reading db"))

        vm.searchText = "sync"
        vm.activeLevels = [.error]

        XCTAssertEqual(vm.filteredEntries.count, 1)
        XCTAssertEqual(vm.filteredEntries.first?.message, "sync failed")
    }

    // MARK: - Load from IPC

    func testLoadFromIPC() async {
        let mock = MockIPCClient()
        mock.logsResponse = IPCLogsResponse(
            entries: [
                IPCLogEntry(time: "2026-03-04T12:00:00Z", level: "INFO", msg: "sync started"),
                IPCLogEntry(time: "2026-03-04T12:00:01Z", level: "ERROR", msg: "connection lost"),
            ],
            error: nil
        )
        let vm = LogViewModel(ipcClient: mock, maxEntries: 100)

        await vm.loadFromIPC()

        XCTAssertEqual(vm.entries.count, 2)
        XCTAssertEqual(vm.entries[0].message, "sync started")
        XCTAssertEqual(vm.entries[0].level, .info)
        XCTAssertEqual(vm.entries[1].message, "connection lost")
        XCTAssertEqual(vm.entries[1].level, .error)
        XCTAssertFalse(vm.isLoading)
    }

    func testLoadFromIPCReplacesExistingEntries() async {
        let mock = MockIPCClient()
        mock.logsResponse = IPCLogsResponse(
            entries: [IPCLogEntry(time: "2026-03-04T12:00:00Z", level: "INFO", msg: "from ipc")],
            error: nil
        )
        let vm = LogViewModel(ipcClient: mock)
        vm.addEntry(makeEntry(message: "old entry"))

        await vm.loadFromIPC()

        XCTAssertEqual(vm.entries.count, 1)
        XCTAssertEqual(vm.entries.first?.message, "from ipc")
    }

    func testLoadFromIPCHandlesError() async {
        let mock = MockIPCClient()
        mock.shouldThrow = IPCClientError.socketNotAvailable
        let vm = LogViewModel(ipcClient: mock)
        vm.addEntry(makeEntry(message: "existing"))

        await vm.loadFromIPC()

        XCTAssertEqual(vm.entries.count, 1)
        XCTAssertEqual(vm.entries.first?.message, "existing")
        XCTAssertFalse(vm.isLoading)
    }

    func testLoadFromIPCWithNoClient() async {
        let vm = LogViewModel(ipcClient: nil)

        await vm.loadFromIPC()

        XCTAssertTrue(vm.entries.isEmpty)
        XCTAssertFalse(vm.isLoading)
    }

    func testLoadFromIPCParsesUnknownLevelAsInfo() async {
        let mock = MockIPCClient()
        mock.logsResponse = IPCLogsResponse(
            entries: [IPCLogEntry(time: "2026-03-04T12:00:00Z", level: "TRACE", msg: "trace msg")],
            error: nil
        )
        let vm = LogViewModel(ipcClient: mock)

        await vm.loadFromIPC()

        XCTAssertEqual(vm.entries.count, 1)
        XCTAssertEqual(vm.entries.first?.level, .info)
    }

    func testLoadFromIPCParsesInvalidDateAsCurrent() async {
        let mock = MockIPCClient()
        mock.logsResponse = IPCLogsResponse(
            entries: [IPCLogEntry(time: "bad-date", level: "INFO", msg: "msg")],
            error: nil
        )
        let vm = LogViewModel(ipcClient: mock)

        await vm.loadFromIPC()

        XCTAssertEqual(vm.entries.count, 1)
        XCTAssertNotNil(vm.entries.first?.time)
    }

    // MARK: - Auto-scroll

    func testAutoScrollDefaultEnabled() {
        let vm = LogViewModel()
        XCTAssertTrue(vm.autoScroll)
    }

    func testAutoScrollToggle() {
        let vm = LogViewModel()
        vm.autoScroll = false
        XCTAssertFalse(vm.autoScroll)
    }

    // MARK: - LogLevel extensions

    func testLogLevelShortLabel() {
        XCTAssertEqual(LogLevel.debug.shortLabel, "DBG")
        XCTAssertEqual(LogLevel.info.shortLabel, "INF")
        XCTAssertEqual(LogLevel.warn.shortLabel, "WRN")
        XCTAssertEqual(LogLevel.error.shortLabel, "ERR")
    }

    // MARK: - Entry limit edge cases

    func testExactlyAtLimit() {
        let vm = LogViewModel(maxEntries: 3)
        vm.addEntries(makeEntries(count: 3))

        XCTAssertEqual(vm.entries.count, 3)
    }

    func testAddSingleEntryAtLimit() {
        let vm = LogViewModel(maxEntries: 2)
        vm.addEntry(makeEntry(message: "first"))
        vm.addEntry(makeEntry(message: "second"))
        vm.addEntry(makeEntry(message: "third"))

        XCTAssertEqual(vm.entries.count, 2)
        XCTAssertEqual(vm.entries.first?.message, "second")
        XCTAssertEqual(vm.entries.last?.message, "third")
    }

    func testSearchMatchesLevelText() {
        let vm = LogViewModel()
        vm.addEntry(makeEntry(level: .error, message: "something happened"))

        vm.searchText = "error"

        XCTAssertEqual(vm.filteredEntries.count, 1)
    }
}
