import XCTest

@testable import BearBridge

// MARK: - Mock process infrastructure

final class MockProcessHandle: ProcessHandle {
    var isRunning: Bool = true
    var terminateCalled = false

    func terminate() {
        terminateCalled = true
        isRunning = false
    }
}

final class MockProcessLauncher: ProcessLauncher {
    var launchCalled = false
    var launchCount = 0
    var lastExecutableURL: URL?
    var lastArguments: [String]?
    var lastEnvironment: [String: String]?
    var shouldThrow: Error?

    /// Stored callbacks so tests can simulate process behavior.
    var stdoutCallback: ((String) -> Void)?
    var stderrCallback: ((String) -> Void)?
    var terminationCallback: ((Int32) -> Void)?

    /// The handle returned by launch().
    var mockHandle = MockProcessHandle()

    func launch(
        executableURL: URL,
        arguments: [String],
        environment: [String: String],
        onStdoutLine: @escaping (String) -> Void,
        onStderrLine: @escaping (String) -> Void,
        onTermination: @escaping (Int32) -> Void
    ) throws -> ProcessHandle {
        if let error = shouldThrow {
            throw error
        }
        launchCalled = true
        launchCount += 1
        lastExecutableURL = executableURL
        lastArguments = arguments
        lastEnvironment = environment
        stdoutCallback = onStdoutLine
        stderrCallback = onStderrLine
        terminationCallback = onTermination
        mockHandle = MockProcessHandle()
        return mockHandle
    }
}

// MARK: - BridgeProcessManager Tests

final class BridgeProcessManagerTests: XCTestCase {

    // MARK: - Start / Stop

    func testStartLaunchesProcessWithDaemonFlag() throws {
        let launcher = MockProcessLauncher()
        let binaryPath = createTempExecutable()
        defer { removeTempFile(binaryPath) }

        let manager = BridgeProcessManager(binaryPath: binaryPath, launcher: launcher)
        try manager.start()

        XCTAssertTrue(launcher.launchCalled)
        XCTAssertEqual(launcher.lastArguments, ["--daemon"])
        XCTAssertEqual(manager.state, .running)
    }

    func testStartPassesEnvironment() throws {
        let launcher = MockProcessLauncher()
        let binaryPath = createTempExecutable()
        defer { removeTempFile(binaryPath) }

        let env = ["BRIDGE_HUB_URL": "https://hub.example.com", "BRIDGE_HUB_TOKEN": "secret"]
        let manager = BridgeProcessManager(binaryPath: binaryPath, environment: env, launcher: launcher)
        try manager.start()

        XCTAssertEqual(launcher.lastEnvironment?["BRIDGE_HUB_URL"], "https://hub.example.com")
        XCTAssertEqual(launcher.lastEnvironment?["BRIDGE_HUB_TOKEN"], "secret")
    }

    func testStartThrowsBinaryNotFound() {
        let launcher = MockProcessLauncher()
        let manager = BridgeProcessManager(binaryPath: "/nonexistent/bear-bridge", launcher: launcher)

        XCTAssertThrowsError(try manager.start()) { error in
            XCTAssertEqual(error as? BridgeProcessError, .binaryNotFound)
        }
        XCTAssertFalse(launcher.launchCalled)
    }

    func testStartThrowsAlreadyRunning() throws {
        let launcher = MockProcessLauncher()
        let binaryPath = createTempExecutable()
        defer { removeTempFile(binaryPath) }

        let manager = BridgeProcessManager(binaryPath: binaryPath, launcher: launcher)
        try manager.start()

        XCTAssertThrowsError(try manager.start()) { error in
            XCTAssertEqual(error as? BridgeProcessError, .alreadyRunning)
        }
    }

    func testStopTerminatesProcess() throws {
        let launcher = MockProcessLauncher()
        let binaryPath = createTempExecutable()
        defer { removeTempFile(binaryPath) }

        let manager = BridgeProcessManager(binaryPath: binaryPath, launcher: launcher)
        try manager.start()

        manager.stop()

        XCTAssertTrue(launcher.mockHandle.terminateCalled)
        XCTAssertEqual(manager.state, .stopped)
    }

    func testStopWhenAlreadyStopped() {
        let launcher = MockProcessLauncher()
        let manager = BridgeProcessManager(binaryPath: "/tmp/test", launcher: launcher)

        // Should not crash
        manager.stop()
        XCTAssertEqual(manager.state, .stopped)
    }

    // MARK: - State change callbacks

    func testOnStateChangeCalledOnStart() throws {
        let launcher = MockProcessLauncher()
        let binaryPath = createTempExecutable()
        defer { removeTempFile(binaryPath) }

        let manager = BridgeProcessManager(binaryPath: binaryPath, launcher: launcher)
        var states: [BridgeProcessManager.State] = []
        manager.onStateChange = { states.append($0) }

        try manager.start()

        XCTAssertEqual(states, [.running])
    }

    func testOnStateChangeCalledOnStop() throws {
        let launcher = MockProcessLauncher()
        let binaryPath = createTempExecutable()
        defer { removeTempFile(binaryPath) }

        let manager = BridgeProcessManager(binaryPath: binaryPath, launcher: launcher)
        try manager.start()

        var states: [BridgeProcessManager.State] = []
        manager.onStateChange = { states.append($0) }

        manager.stop()

        XCTAssertEqual(states, [.stopped])
    }

    // MARK: - Stdout parsing

    func testStdoutLogLineDispatchesToOnLogEntry() throws {
        let launcher = MockProcessLauncher()
        let binaryPath = createTempExecutable()
        defer { removeTempFile(binaryPath) }

        let manager = BridgeProcessManager(binaryPath: binaryPath, launcher: launcher)
        var entries: [LogEntry] = []
        manager.onLogEntry = { entries.append($0) }
        try manager.start()

        launcher.stdoutCallback?(
            "{\"time\":\"2026-03-04T12:00:00Z\",\"level\":\"INFO\",\"msg\":\"bridge starting\"}"
        )

        XCTAssertEqual(entries.count, 1)
        XCTAssertEqual(entries[0].level, .info)
        XCTAssertEqual(entries[0].message, "bridge starting")
    }

    func testStdoutEventLineDispatchesToOnStatusEvent() throws {
        let launcher = MockProcessLauncher()
        let binaryPath = createTempExecutable()
        defer { removeTempFile(binaryPath) }

        let manager = BridgeProcessManager(binaryPath: binaryPath, launcher: launcher)
        var events: [StatusEvent] = []
        manager.onStatusEvent = { events.append($0) }
        try manager.start()

        launcher.stdoutCallback?(
            "{\"event\":\"sync_start\",\"time\":\"2026-03-04T12:00:00Z\"}"
        )

        XCTAssertEqual(events.count, 1)
        XCTAssertEqual(events[0].event, .syncStart)
    }

    func testStdoutNonJSONIgnored() throws {
        let launcher = MockProcessLauncher()
        let binaryPath = createTempExecutable()
        defer { removeTempFile(binaryPath) }

        let manager = BridgeProcessManager(binaryPath: binaryPath, launcher: launcher)
        var entries: [LogEntry] = []
        var events: [StatusEvent] = []
        manager.onLogEntry = { entries.append($0) }
        manager.onStatusEvent = { events.append($0) }
        try manager.start()

        launcher.stdoutCallback?("not json")

        XCTAssertTrue(entries.isEmpty)
        XCTAssertTrue(events.isEmpty)
    }

    // MARK: - Stderr parsing

    func testStderrCreatesErrorLogEntry() throws {
        let launcher = MockProcessLauncher()
        let binaryPath = createTempExecutable()
        defer { removeTempFile(binaryPath) }

        let manager = BridgeProcessManager(binaryPath: binaryPath, launcher: launcher)
        var entries: [LogEntry] = []
        manager.onLogEntry = { entries.append($0) }
        try manager.start()

        launcher.stderrCallback?("something went wrong")

        XCTAssertEqual(entries.count, 1)
        XCTAssertEqual(entries[0].level, .error)
        XCTAssertEqual(entries[0].message, "something went wrong")
    }

    func testStderrEmptyLineIgnored() throws {
        let launcher = MockProcessLauncher()
        let binaryPath = createTempExecutable()
        defer { removeTempFile(binaryPath) }

        let manager = BridgeProcessManager(binaryPath: binaryPath, launcher: launcher)
        var entries: [LogEntry] = []
        manager.onLogEntry = { entries.append($0) }
        try manager.start()

        launcher.stderrCallback?("")

        XCTAssertTrue(entries.isEmpty)
    }

    // MARK: - Process termination and restart

    func testCleanExitStopsWithoutRestart() throws {
        let launcher = MockProcessLauncher()
        let binaryPath = createTempExecutable()
        defer { removeTempFile(binaryPath) }

        let manager = BridgeProcessManager(binaryPath: binaryPath, launcher: launcher)
        try manager.start()

        var states: [BridgeProcessManager.State] = []
        manager.onStateChange = { states.append($0) }

        launcher.terminationCallback?(0)

        XCTAssertEqual(manager.state, .stopped)
        XCTAssertEqual(states, [.stopped])
        XCTAssertEqual(launcher.launchCount, 1) // No restart
    }

    func testCrashTriggersRestart() throws {
        let launcher = MockProcessLauncher()
        let binaryPath = createTempExecutable()
        defer { removeTempFile(binaryPath) }

        let manager = BridgeProcessManager(binaryPath: binaryPath, launcher: launcher)
        try manager.start()
        XCTAssertEqual(launcher.launchCount, 1)

        var states: [BridgeProcessManager.State] = []
        manager.onStateChange = { states.append($0) }

        // Simulate crash (non-zero exit)
        launcher.terminationCallback?(1)

        XCTAssertEqual(launcher.launchCount, 2) // Restarted
        XCTAssertEqual(manager.state, .running)
        XCTAssertTrue(states.contains(.restarting(attempt: 1)))
        XCTAssertTrue(states.contains(.running))
    }

    func testMaxRetriesExhausted() throws {
        let launcher = MockProcessLauncher()
        let binaryPath = createTempExecutable()
        defer { removeTempFile(binaryPath) }

        let manager = BridgeProcessManager(binaryPath: binaryPath, launcher: launcher)
        try manager.start()
        XCTAssertEqual(launcher.launchCount, 1)

        // Crash maxRetries times — each triggers a restart
        for i in 1...BridgeProcessManager.maxRetries {
            launcher.terminationCallback?(1)
            XCTAssertEqual(manager.state, .running, "Should be running after restart \(i)")
        }
        XCTAssertEqual(launcher.launchCount, 1 + BridgeProcessManager.maxRetries)

        // One more crash exceeds maxRetries — should stop
        launcher.terminationCallback?(1)
        XCTAssertEqual(manager.state, .stopped)
        XCTAssertEqual(launcher.launchCount, 1 + BridgeProcessManager.maxRetries) // No additional launch
    }

    func testIntentionalStopPreventsRestart() throws {
        let launcher = MockProcessLauncher()
        let binaryPath = createTempExecutable()
        defer { removeTempFile(binaryPath) }

        let manager = BridgeProcessManager(binaryPath: binaryPath, launcher: launcher)
        try manager.start()

        manager.stop()

        // Simulate delayed termination callback after stop
        launcher.terminationCallback?(1)

        XCTAssertEqual(manager.state, .stopped)
        XCTAssertEqual(launcher.launchCount, 1) // No restart
    }

    func testManualStartResetsRetryCount() throws {
        let launcher = MockProcessLauncher()
        let binaryPath = createTempExecutable()
        defer { removeTempFile(binaryPath) }

        let manager = BridgeProcessManager(binaryPath: binaryPath, launcher: launcher)
        try manager.start()

        // Crash twice
        launcher.terminationCallback?(1)
        launcher.terminationCallback?(1)
        XCTAssertEqual(launcher.launchCount, 3)

        // Manually stop and restart — retryCount resets
        manager.stop()
        try manager.start()
        XCTAssertEqual(launcher.launchCount, 4)

        // Can crash and restart again (retryCount was reset by start())
        launcher.terminationCallback?(1)
        XCTAssertEqual(manager.state, .running)
        XCTAssertEqual(launcher.launchCount, 5)
    }

    func testRestartFailsWhenLaunchThrows() throws {
        let launcher = MockProcessLauncher()
        let binaryPath = createTempExecutable()
        defer { removeTempFile(binaryPath) }

        let manager = BridgeProcessManager(binaryPath: binaryPath, launcher: launcher)
        try manager.start()

        // Make subsequent launches fail
        launcher.shouldThrow = BridgeProcessError.binaryNotFound

        var states: [BridgeProcessManager.State] = []
        manager.onStateChange = { states.append($0) }

        launcher.terminationCallback?(1)

        XCTAssertEqual(manager.state, .stopped)
        XCTAssertTrue(states.contains(.restarting(attempt: 1)))
        XCTAssertTrue(states.contains(.stopped))
    }

    // MARK: - Binary resolution

    func testResolveBinaryWithExplicitPath() {
        let binaryPath = createTempExecutable()
        defer { removeTempFile(binaryPath) }

        let manager = BridgeProcessManager(binaryPath: binaryPath)
        let url = manager.resolveBinaryURL()

        XCTAssertNotNil(url)
        XCTAssertEqual(url?.path, binaryPath)
    }

    func testResolveBinaryWithInvalidExplicitPath() {
        let manager = BridgeProcessManager(binaryPath: "/nonexistent/bear-bridge")
        let url = manager.resolveBinaryURL()

        XCTAssertNil(url)
    }

    // MARK: - LogLevel

    func testLogLevelComparable() {
        XCTAssertTrue(LogLevel.debug < LogLevel.info)
        XCTAssertTrue(LogLevel.info < LogLevel.warn)
        XCTAssertTrue(LogLevel.warn < LogLevel.error)
        XCTAssertFalse(LogLevel.error < LogLevel.debug)
    }

    // MARK: - Helpers

    private func createTempExecutable() -> String {
        let path = NSTemporaryDirectory() + "bear-bridge-test-\(UUID().uuidString)"
        FileManager.default.createFile(atPath: path, contents: "#!/bin/sh\n".data(using: .utf8))
        try? FileManager.default.setAttributes(
            [.posixPermissions: 0o755],
            ofItemAtPath: path
        )
        return path
    }

    private func removeTempFile(_ path: String) {
        try? FileManager.default.removeItem(atPath: path)
    }
}
