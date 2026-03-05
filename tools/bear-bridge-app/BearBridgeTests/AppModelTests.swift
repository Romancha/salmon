import XCTest

@testable import BearBridge

@MainActor
final class AppModelTests: XCTestCase {

    private func makeAppModel(
        configured: Bool = false,
        launcher: MockProcessLauncher? = nil
    ) -> (AppModel, MockProcessLauncher) {
        let store = MockSettingsStore()
        let keychain = MockKeychainService()
        let loginItem = MockLoginItemManager()

        if configured {
            store.storage[SettingsKey.hubURL] = "https://hub.example.com"
            keychain.storage[TokenKey.hubToken] = "hub-token"
            keychain.storage[TokenKey.bearToken] = "bear-token"
        }

        let settings = SettingsManager(store: store, keychain: keychain, loginItemManager: loginItem)
        let mockLauncher = launcher ?? MockProcessLauncher()
        let binaryPath = "/tmp/fake-bear-bridge"
        FileManager.default.createFile(atPath: binaryPath, contents: nil)
        // Make it executable
        try? FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: binaryPath)

        let pm = BridgeProcessManager(
            binaryPath: binaryPath,
            environmentProvider: { settings.bridgeEnvironment() },
            launcher: mockLauncher
        )

        let ipcClient = MockIPCClient()
        let model = AppModel(
            settingsManager: settings,
            ipcClient: ipcClient,
            processManager: pm
        )
        return (model, mockLauncher)
    }

    // MARK: - Initialization

    func testInitialStateNotInitialized() {
        let (model, _) = makeAppModel()
        XCTAssertFalse(model.isInitialized)
    }

    func testInitializeOnlyOnce() {
        let (model, launcher) = makeAppModel(configured: true)
        model.initialize()
        model.initialize()
        XCTAssertTrue(model.isInitialized)
        XCTAssertEqual(launcher.launchCount, 1)
    }

    func testInitializeStartsBridgeWhenConfigured() {
        let (model, launcher) = makeAppModel(configured: true)
        model.initialize()
        XCTAssertTrue(model.isInitialized)
        XCTAssertTrue(launcher.launchCalled)
    }

    func testInitializeDoesNotStartBridgeWhenNotConfigured() {
        let (model, launcher) = makeAppModel(configured: false)
        model.initialize()
        XCTAssertTrue(model.isInitialized)
        XCTAssertFalse(launcher.launchCalled)
    }

    // MARK: - Service ownership

    func testOwnsAllServices() {
        let (model, _) = makeAppModel()
        XCTAssertNotNil(model.statusViewModel)
        XCTAssertNotNil(model.logViewModel)
        XCTAssertNotNil(model.settingsManager)
        XCTAssertNotNil(model.notificationService)
        XCTAssertNotNil(model.processManager)
    }

    // MARK: - Restart bridge

    func testRestartBridge() {
        let (model, launcher) = makeAppModel(configured: true)
        model.initialize()
        XCTAssertEqual(launcher.launchCount, 1)

        model.restartBridge()
        XCTAssertEqual(launcher.launchCount, 2)
    }

    // MARK: - Shutdown

    func testShutdownStopsProcess() {
        let (model, launcher) = makeAppModel(configured: true)
        model.initialize()
        XCTAssertTrue(launcher.mockHandle.isRunning)

        model.shutdown()
        XCTAssertTrue(launcher.mockHandle.terminateCalled)
    }

    // MARK: - Log and status event wiring

    func testLogEntryWiring() {
        let (model, launcher) = makeAppModel(configured: true)
        model.initialize()

        let logJSON = """
        {"time":"2026-03-04T12:00:00Z","level":"info","msg":"test log message"}
        """
        launcher.stdoutCallback?(logJSON)

        let expectation = XCTestExpectation(description: "Log entry added")
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.1) {
            XCTAssertEqual(model.logViewModel.entries.count, 1)
            XCTAssertEqual(model.logViewModel.entries.first?.message, "test log message")
            expectation.fulfill()
        }
        wait(for: [expectation], timeout: 1.0)
    }

    func testStatusEventWiring() {
        let (model, launcher) = makeAppModel(configured: true)
        model.initialize()

        let eventJSON = """
        {"event":"sync_complete","time":"2026-03-04T12:00:00Z","notes_synced":42,"tags_synced":5,"queue_items":0,"duration_ms":1500}
        """
        launcher.stdoutCallback?(eventJSON)

        let expectation = XCTestExpectation(description: "Status event handled")
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.1) {
            XCTAssertEqual(model.statusViewModel.syncStatus, .idle)
            XCTAssertEqual(model.statusViewModel.stats.notesCount, 42)
            expectation.fulfill()
        }
        wait(for: [expectation], timeout: 1.0)
    }

    // MARK: - Error handling

    func testInitializeWithBinaryNotFoundSetsError() {
        let store = MockSettingsStore()
        let keychain = MockKeychainService()
        store.storage[SettingsKey.hubURL] = "https://hub.example.com"
        keychain.storage[TokenKey.hubToken] = "token"
        keychain.storage[TokenKey.bearToken] = "token"
        let settings = SettingsManager(store: store, keychain: keychain, loginItemManager: MockLoginItemManager())

        // Process manager with no valid binary path
        let pm = BridgeProcessManager(binaryPath: "/nonexistent/path", environmentProvider: { [:] })
        let model = AppModel(settingsManager: settings, ipcClient: MockIPCClient(), processManager: pm)

        model.initialize()

        XCTAssertEqual(model.statusViewModel.syncStatus, .error)
        XCTAssertNotNil(model.statusViewModel.lastError)
    }

    // MARK: - Cleanup

    override func tearDown() {
        try? FileManager.default.removeItem(atPath: "/tmp/fake-bear-bridge")
        super.tearDown()
    }
}
