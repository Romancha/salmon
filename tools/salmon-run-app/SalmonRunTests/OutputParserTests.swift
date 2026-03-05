import XCTest

@testable import SalmonRun

final class OutputParserTests: XCTestCase {

    let parser = OutputParser()

    // MARK: - Log line parsing

    func testParseSlogInfoLine() {
        let line = """
        {"time":"2026-03-04T12:00:00Z","level":"INFO","msg":"bridge starting","hub_url":"https://example.com"}
        """
        guard case .log(let entry) = parser.parse(line: line) else {
            XCTFail("Expected log entry")
            return
        }
        XCTAssertEqual(entry.level, .info)
        XCTAssertEqual(entry.message, "bridge starting")
        XCTAssertEqual(entry.fields["hub_url"], "https://example.com")
    }

    func testParseSlogErrorLine() {
        let line = """
        {"time":"2026-03-04T12:00:00Z","level":"ERROR","msg":"sync failed","error":"connection refused"}
        """
        guard case .log(let entry) = parser.parse(line: line) else {
            XCTFail("Expected log entry")
            return
        }
        XCTAssertEqual(entry.level, .error)
        XCTAssertEqual(entry.message, "sync failed")
        XCTAssertEqual(entry.fields["error"], "connection refused")
    }

    func testParseSlogWarnLine() {
        let line = """
        {"time":"2026-03-04T12:00:00Z","level":"WARN","msg":"bear-xcall not available"}
        """
        guard case .log(let entry) = parser.parse(line: line) else {
            XCTFail("Expected log entry")
            return
        }
        XCTAssertEqual(entry.level, .warn)
        XCTAssertEqual(entry.message, "bear-xcall not available")
    }

    func testParseSlogDebugLine() {
        let line = """
        {"time":"2026-03-04T12:00:00Z","level":"DEBUG","msg":"reading bear db"}
        """
        guard case .log(let entry) = parser.parse(line: line) else {
            XCTFail("Expected log entry")
            return
        }
        XCTAssertEqual(entry.level, .debug)
    }

    func testParseSlogWithFractionalSeconds() {
        let line = """
        {"time":"2026-03-04T12:00:00.123456789Z","level":"INFO","msg":"test"}
        """
        guard case .log(let entry) = parser.parse(line: line) else {
            XCTFail("Expected log entry")
            return
        }
        XCTAssertNotNil(entry.time)
        XCTAssertEqual(entry.message, "test")
    }

    func testParseLogExtraFieldsExtracted() {
        let line = """
        {"time":"2026-03-04T12:00:00Z","level":"INFO","msg":"daemon: sync cycle completed","duration_ms":1200,"notes":50}
        """
        guard case .log(let entry) = parser.parse(line: line) else {
            XCTFail("Expected log entry")
            return
        }
        XCTAssertEqual(entry.fields["duration_ms"], "1200")
        XCTAssertEqual(entry.fields["notes"], "50")
        // Reserved keys should NOT be in fields
        XCTAssertNil(entry.fields["time"])
        XCTAssertNil(entry.fields["level"])
        XCTAssertNil(entry.fields["msg"])
    }

    func testParseLogMissingLevelDefaultsToInfo() {
        let line = """
        {"time":"2026-03-04T12:00:00Z","msg":"no level"}
        """
        guard case .log(let entry) = parser.parse(line: line) else {
            XCTFail("Expected log entry")
            return
        }
        XCTAssertEqual(entry.level, .info)
    }

    // MARK: - Event parsing

    func testParseSyncStartEvent() {
        let line = """
        {"event":"sync_start","time":"2026-03-04T12:00:00Z"}
        """
        guard case .event(let event) = parser.parse(line: line) else {
            XCTFail("Expected status event")
            return
        }
        XCTAssertEqual(event.event, .syncStart)
    }

    func testParseSyncProgressEvent() {
        let line = """
        {"event":"sync_progress","time":"2026-03-04T12:00:00Z","phase":"reading_bear","notes":1234}
        """
        guard case .event(let event) = parser.parse(line: line) else {
            XCTFail("Expected status event")
            return
        }
        XCTAssertEqual(event.event, .syncProgress)
        XCTAssertEqual(event.phase, "reading_bear")
        XCTAssertEqual(event.notes, 1234)
    }

    func testParseSyncCompleteEvent() {
        let line = """
        {"event":"sync_complete","time":"2026-03-04T12:00:00Z","duration_ms":1200,"notes_synced":5,"tags_synced":2,"queue_items":1}
        """
        guard case .event(let event) = parser.parse(line: line) else {
            XCTFail("Expected status event")
            return
        }
        XCTAssertEqual(event.event, .syncComplete)
        XCTAssertEqual(event.durationMs, 1200)
        XCTAssertEqual(event.notesSynced, 5)
        XCTAssertEqual(event.tagsSynced, 2)
        XCTAssertEqual(event.queueItems, 1)
    }

    func testParseSyncErrorEvent() {
        let line = """
        {"event":"sync_error","time":"2026-03-04T12:00:00Z","error":"connection refused"}
        """
        guard case .event(let event) = parser.parse(line: line) else {
            XCTFail("Expected status event")
            return
        }
        XCTAssertEqual(event.event, .syncError)
        XCTAssertEqual(event.error, "connection refused")
    }

    func testParseSyncProgressWithItems() {
        let line = """
        {"event":"sync_progress","time":"2026-03-04T12:00:00Z","phase":"processing_queue","items":3}
        """
        guard case .event(let event) = parser.parse(line: line) else {
            XCTFail("Expected status event")
            return
        }
        XCTAssertEqual(event.phase, "processing_queue")
        XCTAssertEqual(event.items, 3)
    }

    // MARK: - Error cases

    func testParseEmptyLine() {
        XCTAssertNil(parser.parse(line: ""))
    }

    func testParseNonJSON() {
        XCTAssertNil(parser.parse(line: "not json at all"))
    }

    func testParseInvalidJSON() {
        XCTAssertNil(parser.parse(line: "{invalid json}"))
    }

    func testParseJSONWithoutMsgOrEvent() {
        XCTAssertNil(parser.parse(line: "{\"foo\":\"bar\"}"))
    }

    func testParseUnknownEventType() {
        let line = """
        {"event":"unknown_event","time":"2026-03-04T12:00:00Z"}
        """
        XCTAssertNil(parser.parse(line: line))
    }

    // MARK: - ISO 8601 helper

    func testParseISO8601WithoutFractional() {
        let date = OutputParser.parseISO8601("2026-03-04T12:00:00Z")
        XCTAssertNotNil(date)
    }

    func testParseISO8601WithFractional() {
        let date = OutputParser.parseISO8601("2026-03-04T12:00:00.123Z")
        XCTAssertNotNil(date)
    }

    func testParseISO8601Invalid() {
        let date = OutputParser.parseISO8601("not-a-date")
        XCTAssertNil(date)
    }
}
