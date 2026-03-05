import XCTest

@testable import BearBridge

// MARK: - Mock transport

final class MockIPCTransport: IPCTransport {
    var sendCallCount = 0
    var lastRequestData: Data?
    var lastSocketPath: String?
    var responseData: Data?
    var shouldThrow: Error?

    func send(request: Data, to socketPath: String) async throws -> Data {
        sendCallCount += 1
        lastRequestData = request
        lastSocketPath = socketPath
        if let error = shouldThrow {
            throw error
        }
        guard let data = responseData else {
            throw IPCClientError.invalidResponse
        }
        return data
    }

    /// Set a JSON-encodable response.
    func setResponse<T: Encodable>(_ value: T) {
        responseData = try? JSONEncoder().encode(value)
    }

    /// Parse the last sent request JSON.
    func lastRequest() -> [String: Any]? {
        guard let data = lastRequestData else { return nil }
        // Strip trailing newline
        var cleaned = data
        if cleaned.last == 0x0A {
            cleaned.removeLast()
        }
        return try? JSONSerialization.jsonObject(with: cleaned) as? [String: Any]
    }
}

// MARK: - BridgeIPCClient Tests

final class BridgeIPCClientTests: XCTestCase {

    // MARK: - Initialization

    func testDefaultSocketPath() {
        let client = BridgeIPCClient()
        let expectedPath = FileManager.default.homeDirectoryForCurrentUser.path + "/.bear-bridge.sock"
        XCTAssertEqual(client.socketPath, expectedPath)
    }

    func testCustomSocketPath() {
        let client = BridgeIPCClient(socketPath: "/tmp/test.sock")
        XCTAssertEqual(client.socketPath, "/tmp/test.sock")
    }

    // MARK: - getStatus

    func testGetStatusSendsCorrectCommand() async throws {
        let transport = MockIPCTransport()
        transport.setResponse(IPCStatusResponse(
            state: "idle",
            lastSync: "2026-03-04T12:00:00Z",
            lastError: "",
            stats: IPCSyncStats(notesSynced: 100, tagsSynced: 10, queueProcessed: 5, lastDurationMs: 1200),
            error: nil
        ))
        let client = BridgeIPCClient(socketPath: "/tmp/test.sock", transport: transport)

        _ = try await client.getStatus()

        let req = transport.lastRequest()
        XCTAssertEqual(req?["cmd"] as? String, "status")
        XCTAssertNil(req?["lines"])
    }

    func testGetStatusParsesResponse() async throws {
        let transport = MockIPCTransport()
        transport.setResponse(IPCStatusResponse(
            state: "syncing",
            lastSync: "2026-03-04T12:00:00Z",
            lastError: "previous error",
            stats: IPCSyncStats(notesSynced: 50, tagsSynced: 5, queueProcessed: 2, lastDurationMs: 800),
            error: nil
        ))
        let client = BridgeIPCClient(socketPath: "/tmp/test.sock", transport: transport)

        let status = try await client.getStatus()

        XCTAssertEqual(status.state, "syncing")
        XCTAssertEqual(status.lastSync, "2026-03-04T12:00:00Z")
        XCTAssertEqual(status.lastError, "previous error")
        XCTAssertEqual(status.stats.notesSynced, 50)
        XCTAssertEqual(status.stats.tagsSynced, 5)
        XCTAssertEqual(status.stats.queueProcessed, 2)
        XCTAssertEqual(status.stats.lastDurationMs, 800)
    }

    func testGetStatusThrowsOnTransportError() async {
        let transport = MockIPCTransport()
        transport.shouldThrow = IPCClientError.socketNotAvailable
        let client = BridgeIPCClient(socketPath: "/tmp/test.sock", transport: transport)

        do {
            _ = try await client.getStatus()
            XCTFail("Expected error")
        } catch {
            XCTAssertEqual(error as? IPCClientError, .socketNotAvailable)
        }
    }

    // MARK: - syncNow

    func testSyncNowSendsCorrectCommand() async throws {
        let transport = MockIPCTransport()
        transport.setResponse(IPCOkResponse(ok: true, error: nil))
        let client = BridgeIPCClient(socketPath: "/tmp/test.sock", transport: transport)

        _ = try await client.syncNow()

        let req = transport.lastRequest()
        XCTAssertEqual(req?["cmd"] as? String, "sync_now")
    }

    func testSyncNowParsesOkResponse() async throws {
        let transport = MockIPCTransport()
        transport.setResponse(IPCOkResponse(ok: true, error: nil))
        let client = BridgeIPCClient(socketPath: "/tmp/test.sock", transport: transport)

        let response = try await client.syncNow()

        XCTAssertTrue(response.ok)
        XCTAssertNil(response.error)
    }

    // MARK: - getLogs

    func testGetLogsSendsCorrectCommand() async throws {
        let transport = MockIPCTransport()
        transport.setResponse(IPCLogsResponse(entries: [], error: nil))
        let client = BridgeIPCClient(socketPath: "/tmp/test.sock", transport: transport)

        _ = try await client.getLogs(lines: 25)

        let req = transport.lastRequest()
        XCTAssertEqual(req?["cmd"] as? String, "logs")
        XCTAssertEqual(req?["lines"] as? Int, 25)
    }

    func testGetLogsDefaultLines() async throws {
        let transport = MockIPCTransport()
        transport.setResponse(IPCLogsResponse(entries: [], error: nil))
        let client = BridgeIPCClient(socketPath: "/tmp/test.sock", transport: transport)

        _ = try await client.getLogs()

        let req = transport.lastRequest()
        XCTAssertEqual(req?["lines"] as? Int, 50)
    }

    func testGetLogsParsesEntries() async throws {
        let transport = MockIPCTransport()
        transport.setResponse(IPCLogsResponse(
            entries: [
                IPCLogEntry(time: "2026-03-04T12:00:00Z", level: "INFO", msg: "sync started"),
                IPCLogEntry(time: "2026-03-04T12:00:01Z", level: "ERROR", msg: "connection failed"),
            ],
            error: nil
        ))
        let client = BridgeIPCClient(socketPath: "/tmp/test.sock", transport: transport)

        let response = try await client.getLogs()

        XCTAssertEqual(response.entries.count, 2)
        XCTAssertEqual(response.entries[0].level, "INFO")
        XCTAssertEqual(response.entries[0].msg, "sync started")
        XCTAssertEqual(response.entries[1].level, "ERROR")
        XCTAssertEqual(response.entries[1].msg, "connection failed")
    }

    // MARK: - getQueueStatus

    func testGetQueueStatusSendsCorrectCommand() async throws {
        let transport = MockIPCTransport()
        transport.setResponse(IPCQueueStatusResponse(items: [], error: nil))
        let client = BridgeIPCClient(socketPath: "/tmp/test.sock", transport: transport)

        _ = try await client.getQueueStatus()

        let req = transport.lastRequest()
        XCTAssertEqual(req?["cmd"] as? String, "queue_status")
    }

    func testGetQueueStatusParsesResponse() async throws {
        let transport = MockIPCTransport()
        transport.setResponse(IPCQueueStatusResponse(
            items: [
                IPCQueueStatusItem(id: 1, action: "create", noteTitle: "My Note", status: "processing", createdAt: "2026-03-04T12:00:00Z"),
                IPCQueueStatusItem(id: 2, action: "add_tag", noteTitle: "work", status: "applied", createdAt: nil),
            ],
            error: nil
        ))
        let client = BridgeIPCClient(socketPath: "/tmp/test.sock", transport: transport)

        let response = try await client.getQueueStatus()

        XCTAssertEqual(response.items.count, 2)
        XCTAssertEqual(response.items[0].id, 1)
        XCTAssertEqual(response.items[0].action, "create")
        XCTAssertEqual(response.items[0].noteTitle, "My Note")
        XCTAssertEqual(response.items[0].status, "processing")
        XCTAssertEqual(response.items[0].createdAt, "2026-03-04T12:00:00Z")
        XCTAssertEqual(response.items[1].id, 2)
        XCTAssertEqual(response.items[1].action, "add_tag")
        XCTAssertEqual(response.items[1].status, "applied")
        XCTAssertNil(response.items[1].createdAt)
    }

    func testGetQueueStatusEmptyItems() async throws {
        let transport = MockIPCTransport()
        transport.setResponse(IPCQueueStatusResponse(items: [], error: nil))
        let client = BridgeIPCClient(socketPath: "/tmp/test.sock", transport: transport)

        let response = try await client.getQueueStatus()

        XCTAssertTrue(response.items.isEmpty)
    }

    // MARK: - quit

    func testQuitSendsCorrectCommand() async throws {
        let transport = MockIPCTransport()
        transport.setResponse(IPCOkResponse(ok: true, error: nil))
        let client = BridgeIPCClient(socketPath: "/tmp/test.sock", transport: transport)

        _ = try await client.quit()

        let req = transport.lastRequest()
        XCTAssertEqual(req?["cmd"] as? String, "quit")
    }

    func testQuitParsesResponse() async throws {
        let transport = MockIPCTransport()
        transport.setResponse(IPCOkResponse(ok: true, error: nil))
        let client = BridgeIPCClient(socketPath: "/tmp/test.sock", transport: transport)

        let response = try await client.quit()

        XCTAssertTrue(response.ok)
    }

    // MARK: - Error handling

    func testInvalidResponseThrowsDecodingError() async {
        let transport = MockIPCTransport()
        transport.responseData = "not json".data(using: .utf8)
        let client = BridgeIPCClient(socketPath: "/tmp/test.sock", transport: transport)

        do {
            let _: IPCStatusResponse = try await client.getStatus()
            XCTFail("Expected decoding error")
        } catch {
            XCTAssertTrue(error is DecodingError)
        }
    }

    func testTimeoutErrorPropagated() async {
        let transport = MockIPCTransport()
        transport.shouldThrow = IPCClientError.timeout
        let client = BridgeIPCClient(socketPath: "/tmp/test.sock", transport: transport)

        do {
            _ = try await client.getStatus()
            XCTFail("Expected timeout error")
        } catch {
            XCTAssertEqual(error as? IPCClientError, .timeout)
        }
    }

    func testConnectionFailedErrorPropagated() async {
        let transport = MockIPCTransport()
        transport.shouldThrow = IPCClientError.connectionFailed("connect() failed: 61")
        let client = BridgeIPCClient(socketPath: "/tmp/test.sock", transport: transport)

        do {
            _ = try await client.syncNow()
            XCTFail("Expected connection error")
        } catch {
            if case .connectionFailed(let msg) = error as? IPCClientError {
                XCTAssertTrue(msg.contains("61"))
            } else {
                XCTFail("Expected connectionFailed error")
            }
        }
    }

    // MARK: - Socket path passed to transport

    func testSocketPathPassedToTransport() async throws {
        let transport = MockIPCTransport()
        transport.setResponse(IPCOkResponse(ok: true, error: nil))
        let client = BridgeIPCClient(socketPath: "/custom/path.sock", transport: transport)

        _ = try await client.syncNow()

        XCTAssertEqual(transport.lastSocketPath, "/custom/path.sock")
    }

    // MARK: - Multiple sequential commands

    func testMultipleCommandsEachCreateSeparateConnection() async throws {
        let transport = MockIPCTransport()
        transport.setResponse(IPCOkResponse(ok: true, error: nil))
        let client = BridgeIPCClient(socketPath: "/tmp/test.sock", transport: transport)

        _ = try await client.syncNow()
        _ = try await client.syncNow()
        _ = try await client.syncNow()

        XCTAssertEqual(transport.sendCallCount, 3)
    }

}

// MARK: - IPCModels Tests

final class IPCModelsTests: XCTestCase {

    func testStatusResponseDecoding() throws {
        let json = """
        {"state":"idle","last_sync":"2026-03-04T12:00:00Z","last_error":"","stats":{"notes_synced":100,"tags_synced":10,"queue_processed":5,"last_duration_ms":1200}}
        """
        let data = json.data(using: .utf8)!
        let response = try JSONDecoder().decode(IPCStatusResponse.self, from: data)

        XCTAssertEqual(response.state, "idle")
        XCTAssertEqual(response.lastSync, "2026-03-04T12:00:00Z")
        XCTAssertEqual(response.lastError, "")
        XCTAssertEqual(response.stats.notesSynced, 100)
        XCTAssertEqual(response.stats.tagsSynced, 10)
        XCTAssertEqual(response.stats.queueProcessed, 5)
        XCTAssertEqual(response.stats.lastDurationMs, 1200)
        XCTAssertNil(response.error)
    }

    func testStatusResponseWithError() throws {
        let json = """
        {"state":"error","last_sync":"","last_error":"db locked","stats":{"notes_synced":0,"tags_synced":0,"queue_processed":0,"last_duration_ms":0},"error":"internal"}
        """
        let data = json.data(using: .utf8)!
        let response = try JSONDecoder().decode(IPCStatusResponse.self, from: data)

        XCTAssertEqual(response.state, "error")
        XCTAssertEqual(response.lastError, "db locked")
        XCTAssertEqual(response.error, "internal")
    }

    func testOkResponseDecoding() throws {
        let json = """
        {"ok":true}
        """
        let data = json.data(using: .utf8)!
        let response = try JSONDecoder().decode(IPCOkResponse.self, from: data)

        XCTAssertTrue(response.ok)
        XCTAssertNil(response.error)
    }

    func testOkResponseWithError() throws {
        let json = """
        {"ok":false,"error":"unknown command: foo"}
        """
        let data = json.data(using: .utf8)!
        let response = try JSONDecoder().decode(IPCOkResponse.self, from: data)

        XCTAssertFalse(response.ok)
        XCTAssertEqual(response.error, "unknown command: foo")
    }

    func testLogsResponseDecoding() throws {
        let json = """
        {"entries":[{"time":"2026-03-04T12:00:00Z","level":"INFO","msg":"sync started"},{"time":"2026-03-04T12:00:01Z","level":"ERROR","msg":"failed"}]}
        """
        let data = json.data(using: .utf8)!
        let response = try JSONDecoder().decode(IPCLogsResponse.self, from: data)

        XCTAssertEqual(response.entries.count, 2)
        XCTAssertEqual(response.entries[0].time, "2026-03-04T12:00:00Z")
        XCTAssertEqual(response.entries[0].level, "INFO")
        XCTAssertEqual(response.entries[0].msg, "sync started")
        XCTAssertNil(response.error)
    }

    func testLogsResponseEmpty() throws {
        let json = """
        {"entries":[]}
        """
        let data = json.data(using: .utf8)!
        let response = try JSONDecoder().decode(IPCLogsResponse.self, from: data)

        XCTAssertTrue(response.entries.isEmpty)
    }

    func testSyncStatsEquatable() {
        let a = IPCSyncStats(notesSynced: 10, tagsSynced: 2, queueProcessed: 1, lastDurationMs: 500)
        let b = IPCSyncStats(notesSynced: 10, tagsSynced: 2, queueProcessed: 1, lastDurationMs: 500)
        let c = IPCSyncStats(notesSynced: 20, tagsSynced: 2, queueProcessed: 1, lastDurationMs: 500)

        XCTAssertEqual(a, b)
        XCTAssertNotEqual(a, c)
    }

    func testQueueStatusResponseDecoding() throws {
        let json = """
        {"items":[{"id":1,"action":"create","note_title":"My Note","status":"processing","created_at":"2026-03-04T12:00:00Z"},{"id":2,"action":"add_tag","note_title":"work","status":"applied"}]}
        """
        let data = json.data(using: .utf8)!
        let response = try JSONDecoder().decode(IPCQueueStatusResponse.self, from: data)

        XCTAssertEqual(response.items.count, 2)
        XCTAssertEqual(response.items[0].id, 1)
        XCTAssertEqual(response.items[0].action, "create")
        XCTAssertEqual(response.items[0].noteTitle, "My Note")
        XCTAssertEqual(response.items[0].status, "processing")
        XCTAssertEqual(response.items[0].createdAt, "2026-03-04T12:00:00Z")
        XCTAssertEqual(response.items[1].id, 2)
        XCTAssertEqual(response.items[1].action, "add_tag")
        XCTAssertEqual(response.items[1].noteTitle, "work")
        XCTAssertEqual(response.items[1].status, "applied")
        XCTAssertNil(response.items[1].createdAt)
        XCTAssertNil(response.error)
    }

    func testQueueStatusResponseEmpty() throws {
        let json = """
        {"items":[]}
        """
        let data = json.data(using: .utf8)!
        let response = try JSONDecoder().decode(IPCQueueStatusResponse.self, from: data)

        XCTAssertTrue(response.items.isEmpty)
    }

    func testQueueStatusItemEquatable() {
        let a = IPCQueueStatusItem(id: 1, action: "create", noteTitle: "Note", status: "processing", createdAt: nil)
        let b = IPCQueueStatusItem(id: 1, action: "create", noteTitle: "Note", status: "processing", createdAt: nil)
        let c = IPCQueueStatusItem(id: 2, action: "update", noteTitle: "Note", status: "applied", createdAt: nil)

        XCTAssertEqual(a, b)
        XCTAssertNotEqual(a, c)
    }

    func testQueueStatusItemIdentifiable() {
        let item = IPCQueueStatusItem(id: 42, action: "create", noteTitle: "Note", status: "processing", createdAt: nil)
        XCTAssertEqual(item.id, 42)
    }
}

