import XCTest

@testable import BearBridge

@MainActor
final class LogViewerWindowTests: XCTestCase {

    // MARK: - LogEntryRow

    func testLogEntryRowCreation() {
        let entry = LogEntry(time: Date(), level: .info, message: "test message")
        let row = LogEntryRow(entry: entry)
        XCTAssertNotNil(row.body)
    }

    func testLogEntryRowWithLongMessage() {
        let longMessage = String(repeating: "a", count: 300)
        let entry = LogEntry(time: Date(), level: .warn, message: longMessage)
        let row = LogEntryRow(entry: entry)
        XCTAssertNotNil(row.body)
    }

    func testLogEntryRowWithShortMessage() {
        let entry = LogEntry(time: Date(), level: .debug, message: "short")
        let row = LogEntryRow(entry: entry)
        XCTAssertNotNil(row.body)
    }

    func testLogEntryRowWithExactTruncationLimit() {
        let message = String(repeating: "x", count: 200)
        let entry = LogEntry(time: Date(), level: .error, message: message)
        let row = LogEntryRow(entry: entry)
        XCTAssertNotNil(row.body)
    }

    func testLogEntryRowWithAllLevels() {
        for level in LogLevel.allCases {
            let entry = LogEntry(time: Date(), level: level, message: "msg for \(level.rawValue)")
            let row = LogEntryRow(entry: entry)
            XCTAssertNotNil(row.body)
        }
    }

    // MARK: - LogLevel color

    func testLogLevelDebugColor() {
        // Debug should use gray (not secondary) for colored badge styling
        XCTAssertEqual(LogLevel.debug.color, .gray)
    }

    func testLogLevelInfoColor() {
        XCTAssertEqual(LogLevel.info.color, .blue)
    }

    func testLogLevelWarnColor() {
        XCTAssertEqual(LogLevel.warn.color, .orange)
    }

    func testLogLevelErrorColor() {
        XCTAssertEqual(LogLevel.error.color, .red)
    }

    // MARK: - LogViewerWindow structure

    func testLogViewerWindowCreation() {
        let vm = LogViewModel()
        let window = LogViewerWindow().environmentObject(vm)
        XCTAssertNotNil(window)
    }

    func testLogViewerWindowWithEntries() {
        let vm = LogViewModel()
        vm.addEntry(LogEntry(time: Date(), level: .info, message: "test"))
        vm.addEntry(LogEntry(time: Date(), level: .error, message: "error"))
        let window = LogViewerWindow().environmentObject(vm)
        XCTAssertNotNil(window)
    }

    func testLogViewerWindowWithFiltering() {
        let vm = LogViewModel()
        vm.addEntry(LogEntry(time: Date(), level: .info, message: "sync started"))
        vm.addEntry(LogEntry(time: Date(), level: .error, message: "sync failed"))
        vm.searchText = "sync"
        vm.activeLevels = [.error]
        let window = LogViewerWindow().environmentObject(vm)
        XCTAssertNotNil(window)
        XCTAssertEqual(vm.filteredEntries.count, 1)
    }
}
