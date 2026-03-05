import XCTest

@testable import SalmonRun

// MARK: - Mock Keychain

final class MockKeychainService: KeychainServiceProtocol {
    var storage: [String: String] = [:]
    var saveCallCount = 0
    var deleteCallCount = 0
    var shouldThrow = false

    func save(key: String, value: String) throws {
        saveCallCount += 1
        if shouldThrow {
            throw KeychainError.osStatus(-1)
        }
        storage[key] = value
    }

    func load(key: String) -> String? {
        storage[key]
    }

    func delete(key: String) {
        deleteCallCount += 1
        storage.removeValue(forKey: key)
    }
}

// MARK: - Mock Settings Store (UserDefaults replacement)

final class MockSettingsStore: SettingsStore {
    var storage: [String: Any] = [:]
    var defaults: [String: Any] = [:]
    var setCallCount = 0

    func string(forKey key: String) -> String? {
        storage[key] as? String ?? defaults[key] as? String
    }

    func set(_ value: Any?, forKey key: String) {
        setCallCount += 1
        storage[key] = value
    }

    func integer(forKey key: String) -> Int {
        storage[key] as? Int ?? defaults[key] as? Int ?? 0
    }

    func bool(forKey key: String) -> Bool {
        storage[key] as? Bool ?? defaults[key] as? Bool ?? false
    }

    func register(defaults registrationDomain: [String: Any]) {
        for (key, value) in registrationDomain {
            defaults[key] = value
        }
    }
}

// MARK: - Mock Login Item Manager

final class MockLoginItemManager: LoginItemManager {
    var isEnabled: Bool = false
    var setEnabledCallCount = 0
    var shouldThrow = false

    func setEnabled(_ enabled: Bool) throws {
        setEnabledCallCount += 1
        if shouldThrow {
            throw NSError(domain: "test", code: 1)
        }
        isEnabled = enabled
    }
}

// MARK: - SettingsManager Tests

@MainActor
final class SettingsManagerTests: XCTestCase {

    private func makeManager(
        store: MockSettingsStore? = nil,
        keychain: MockKeychainService? = nil,
        loginItem: MockLoginItemManager? = nil
    ) -> (SettingsManager, MockSettingsStore, MockKeychainService, MockLoginItemManager) {
        let s = store ?? MockSettingsStore()
        let k = keychain ?? MockKeychainService()
        let l = loginItem ?? MockLoginItemManager()
        let m = SettingsManager(store: s, keychain: k, loginItemManager: l)
        return (m, s, k, l)
    }

    // MARK: - Initial state / defaults

    func testDefaultValues() {
        let (manager, _, _, _) = makeManager()

        XCTAssertEqual(manager.hubURL, "")
        XCTAssertEqual(manager.syncIntervalMinutes, SettingsManager.defaultSyncIntervalMinutes)
        XCTAssertFalse(manager.launchAtLogin)
        XCTAssertTrue(manager.notificationsEnabled)
    }

    func testDefaultSyncIntervalIsFive() {
        XCTAssertEqual(SettingsManager.defaultSyncIntervalMinutes, 5)
    }

    func testSyncIntervalRange() {
        XCTAssertEqual(SettingsManager.syncIntervalRange.lowerBound, 1)
        XCTAssertEqual(SettingsManager.syncIntervalRange.upperBound, 30)
    }

    // MARK: - Loading saved values

    func testLoadsSavedHubURL() {
        let store = MockSettingsStore()
        store.storage[SettingsKey.hubURL] = "https://hub.example.com"
        let (manager, _, _, _) = makeManager(store: store)

        XCTAssertEqual(manager.hubURL, "https://hub.example.com")
    }

    func testLoadsSavedSyncInterval() {
        let store = MockSettingsStore()
        store.storage[SettingsKey.syncIntervalMinutes] = 15
        let (manager, _, _, _) = makeManager(store: store)

        XCTAssertEqual(manager.syncIntervalMinutes, 15)
    }

    func testLoadsSavedNotificationsEnabled() {
        let store = MockSettingsStore()
        store.storage[SettingsKey.notificationsEnabled] = false
        let (manager, _, _, _) = makeManager(store: store)

        XCTAssertFalse(manager.notificationsEnabled)
    }

    // MARK: - Persistence on change

    func testHubURLPersistsOnChange() {
        let (manager, store, _, _) = makeManager()
        manager.hubURL = "https://new-hub.example.com"
        XCTAssertEqual(store.storage[SettingsKey.hubURL] as? String, "https://new-hub.example.com")
    }

    func testSyncIntervalPersistsOnChange() {
        let (manager, store, _, _) = makeManager()
        manager.syncIntervalMinutes = 10
        XCTAssertEqual(store.storage[SettingsKey.syncIntervalMinutes] as? Int, 10)
    }

    func testLaunchAtLoginPersistsOnChange() {
        let (manager, store, _, _) = makeManager()
        manager.launchAtLogin = true
        XCTAssertEqual(store.storage[SettingsKey.launchAtLogin] as? Bool, true)
    }

    func testNotificationsEnabledPersistsOnChange() {
        let (manager, store, _, _) = makeManager()
        manager.notificationsEnabled = false
        XCTAssertEqual(store.storage[SettingsKey.notificationsEnabled] as? Bool, false)
    }

    // MARK: - Token management (Keychain)

    func testHubTokenSavesToKeychain() {
        let (manager, _, keychain, _) = makeManager()
        manager.hubToken = "secret-hub-token"

        XCTAssertEqual(keychain.storage[TokenKey.hubToken], "secret-hub-token")
        XCTAssertEqual(keychain.saveCallCount, 1)
    }

    func testBearTokenSavesToKeychain() {
        let (manager, _, keychain, _) = makeManager()
        manager.bearToken = "secret-bear-token"

        XCTAssertEqual(keychain.storage[TokenKey.bearToken], "secret-bear-token")
        XCTAssertEqual(keychain.saveCallCount, 1)
    }

    func testHubTokenLoadsFromKeychain() {
        let keychain = MockKeychainService()
        keychain.storage[TokenKey.hubToken] = "existing-token"
        let (manager, _, _, _) = makeManager(keychain: keychain)

        XCTAssertEqual(manager.hubToken, "existing-token")
    }

    func testBearTokenLoadsFromKeychain() {
        let keychain = MockKeychainService()
        keychain.storage[TokenKey.bearToken] = "existing-bear-token"
        let (manager, _, _, _) = makeManager(keychain: keychain)

        XCTAssertEqual(manager.bearToken, "existing-bear-token")
    }

    func testEmptyHubTokenDeletesFromKeychain() {
        let keychain = MockKeychainService()
        keychain.storage[TokenKey.hubToken] = "old-token"
        let (manager, _, _, _) = makeManager(keychain: keychain)

        manager.hubToken = ""

        XCTAssertNil(keychain.storage[TokenKey.hubToken])
        XCTAssertEqual(keychain.deleteCallCount, 1)
    }

    func testEmptyBearTokenDeletesFromKeychain() {
        let keychain = MockKeychainService()
        keychain.storage[TokenKey.bearToken] = "old-token"
        let (manager, _, _, _) = makeManager(keychain: keychain)

        manager.bearToken = ""

        XCTAssertNil(keychain.storage[TokenKey.bearToken])
        XCTAssertEqual(keychain.deleteCallCount, 1)
    }

    // MARK: - Environment generation

    func testBridgeEnvironmentWithAllSettings() {
        let keychain = MockKeychainService()
        keychain.storage[TokenKey.hubToken] = "hub-token-123"
        keychain.storage[TokenKey.bearToken] = "bear-token-456"
        let (manager, _, _, _) = makeManager(keychain: keychain)
        manager.hubURL = "https://hub.example.com"
        manager.syncIntervalMinutes = 10

        let env = manager.bridgeEnvironment()

        XCTAssertEqual(env["SALMON_HUB_URL"], "https://hub.example.com")
        XCTAssertEqual(env["SALMON_HUB_TOKEN"], "hub-token-123")
        XCTAssertEqual(env["SALMON_BEAR_TOKEN"], "bear-token-456")
        XCTAssertEqual(env["SALMON_SYNC_INTERVAL"], "600")
    }

    func testBridgeEnvironmentOmitsEmptyValues() {
        let (manager, _, _, _) = makeManager()

        let env = manager.bridgeEnvironment()

        XCTAssertNil(env["SALMON_HUB_URL"])
        XCTAssertNil(env["SALMON_HUB_TOKEN"])
        XCTAssertNil(env["SALMON_BEAR_TOKEN"])
        // Sync interval is always present
        XCTAssertEqual(env["SALMON_SYNC_INTERVAL"], "300")
    }

    func testBridgeEnvironmentSyncIntervalConversion() {
        let (manager, _, _, _) = makeManager()
        manager.syncIntervalMinutes = 1

        let env = manager.bridgeEnvironment()
        XCTAssertEqual(env["SALMON_SYNC_INTERVAL"], "60")

        manager.syncIntervalMinutes = 30
        let env2 = manager.bridgeEnvironment()
        XCTAssertEqual(env2["SALMON_SYNC_INTERVAL"], "1800")
    }

    func testBridgeEnvironmentDefaultInterval() {
        let (manager, _, _, _) = makeManager()

        let env = manager.bridgeEnvironment()
        // Default is 5 min = 300 seconds
        XCTAssertEqual(env["SALMON_SYNC_INTERVAL"], "300")
    }

    // MARK: - isConfigured

    func testIsConfiguredAllSet() {
        let keychain = MockKeychainService()
        keychain.storage[TokenKey.hubToken] = "token"
        keychain.storage[TokenKey.bearToken] = "token"
        let (manager, _, _, _) = makeManager(keychain: keychain)
        manager.hubURL = "https://hub.example.com"

        XCTAssertTrue(manager.isConfigured)
    }

    func testIsConfiguredMissingHubURL() {
        let keychain = MockKeychainService()
        keychain.storage[TokenKey.hubToken] = "token"
        keychain.storage[TokenKey.bearToken] = "token"
        let (manager, _, _, _) = makeManager(keychain: keychain)

        XCTAssertFalse(manager.isConfigured)
    }

    func testIsConfiguredMissingHubToken() {
        let keychain = MockKeychainService()
        keychain.storage[TokenKey.bearToken] = "token"
        let (manager, _, _, _) = makeManager(keychain: keychain)
        manager.hubURL = "https://hub.example.com"

        XCTAssertFalse(manager.isConfigured)
    }

    func testIsConfiguredMissingBearToken() {
        let keychain = MockKeychainService()
        keychain.storage[TokenKey.hubToken] = "token"
        let (manager, _, _, _) = makeManager(keychain: keychain)
        manager.hubURL = "https://hub.example.com"

        XCTAssertFalse(manager.isConfigured)
    }

    // MARK: - Launch at Login

    func testLaunchAtLoginCallsLoginItemManager() {
        let (manager, _, _, loginItem) = makeManager()

        manager.launchAtLogin = true

        XCTAssertEqual(loginItem.setEnabledCallCount, 1)
        XCTAssertTrue(loginItem.isEnabled)
    }

    func testLaunchAtLoginDisable() {
        let loginItem = MockLoginItemManager()
        loginItem.isEnabled = true
        let (manager, _, _, _) = makeManager(loginItem: loginItem)

        manager.launchAtLogin = false

        XCTAssertFalse(loginItem.isEnabled)
    }

    func testRefreshLoginItemStatus() {
        let loginItem = MockLoginItemManager()
        loginItem.isEnabled = true
        let (manager, _, _, _) = makeManager(loginItem: loginItem)

        // Initially false from defaults
        XCTAssertFalse(manager.launchAtLogin)

        manager.refreshLoginItemStatus()

        XCTAssertTrue(manager.launchAtLogin)
    }

    func testRefreshLoginItemStatusDoesNotCallSetEnabled() {
        let loginItem = MockLoginItemManager()
        loginItem.isEnabled = true
        let (manager, _, _, _) = makeManager(loginItem: loginItem)

        let callsBefore = loginItem.setEnabledCallCount
        manager.refreshLoginItemStatus()

        // Refresh should NOT call setEnabled back to the system
        XCTAssertEqual(loginItem.setEnabledCallCount, callsBefore)
        XCTAssertTrue(manager.launchAtLogin)
    }

    func testRefreshLoginItemStatusPersistsToStore() {
        let loginItem = MockLoginItemManager()
        loginItem.isEnabled = true
        let (manager, store, _, _) = makeManager(loginItem: loginItem)

        manager.refreshLoginItemStatus()

        // Should persist the system state to the store
        XCTAssertEqual(store.storage[SettingsKey.launchAtLogin] as? Bool, true)
    }

    func testRefreshLoginItemStatusNoChangeWhenMatching() {
        let loginItem = MockLoginItemManager()
        loginItem.isEnabled = false
        let (manager, store, _, _) = makeManager(loginItem: loginItem)
        let initialSetCount = store.setCallCount

        manager.refreshLoginItemStatus()

        // launchAtLogin was already false, so no change and no extra store write
        XCTAssertEqual(store.setCallCount, initialSetCount)
    }

    // MARK: - Defaults registration

    func testDefaultsAreRegistered() {
        let store = MockSettingsStore()
        _ = SettingsManager(store: store, keychain: MockKeychainService(), loginItemManager: MockLoginItemManager())

        XCTAssertEqual(store.defaults[SettingsKey.syncIntervalMinutes] as? Int, 5)
        XCTAssertEqual(store.defaults[SettingsKey.launchAtLogin] as? Bool, false)
        XCTAssertEqual(store.defaults[SettingsKey.notificationsEnabled] as? Bool, true)
    }

    // MARK: - Error surfacing

    func testKeychainSaveErrorSurfaced() {
        let keychain = MockKeychainService()
        keychain.shouldThrow = true
        let (manager, _, _, _) = makeManager(keychain: keychain)

        manager.hubToken = "new-token"

        XCTAssertNotNil(manager.lastSettingsError)
        XCTAssertTrue(manager.lastSettingsError?.contains("Keychain") ?? false)
    }

    func testBearTokenKeychainSaveErrorSurfaced() {
        let keychain = MockKeychainService()
        keychain.shouldThrow = true
        let (manager, _, _, _) = makeManager(keychain: keychain)

        manager.bearToken = "new-token"

        XCTAssertNotNil(manager.lastSettingsError)
        XCTAssertTrue(manager.lastSettingsError?.contains("Keychain") ?? false)
    }

    func testLaunchAtLoginErrorSurfaced() {
        let loginItem = MockLoginItemManager()
        loginItem.shouldThrow = true
        let (manager, _, _, _) = makeManager(loginItem: loginItem)

        manager.launchAtLogin = true

        XCTAssertNotNil(manager.lastSettingsError)
        XCTAssertTrue(manager.lastSettingsError?.contains("Launch at Login") ?? false)
    }

    func testLaunchAtLoginRevertsOnError() {
        let loginItem = MockLoginItemManager()
        loginItem.shouldThrow = true
        let (manager, store, _, _) = makeManager(loginItem: loginItem)

        // Default is false; try to enable (should fail and revert)
        manager.launchAtLogin = true

        // Value should be reverted in memory
        XCTAssertFalse(manager.launchAtLogin)
        // Store should NOT have been updated with the failed value
        XCTAssertNil(store.storage[SettingsKey.launchAtLogin])
    }

    func testNoErrorOnSuccessfulKeychainSave() {
        let (manager, _, _, _) = makeManager()

        manager.hubToken = "valid-token"

        XCTAssertNil(manager.lastSettingsError)
    }

    // MARK: - Multiple changes

    func testMultipleSettingsChanges() {
        let keychain = MockKeychainService()
        let (manager, _, _, _) = makeManager(keychain: keychain)

        manager.hubURL = "https://hub1.example.com"
        manager.hubToken = "token1"
        manager.bearToken = "bear1"
        manager.syncIntervalMinutes = 15

        var env = manager.bridgeEnvironment()
        XCTAssertEqual(env["SALMON_HUB_URL"], "https://hub1.example.com")
        XCTAssertEqual(env["SALMON_SYNC_INTERVAL"], "900")

        // Change settings
        manager.hubURL = "https://hub2.example.com"
        manager.syncIntervalMinutes = 20

        env = manager.bridgeEnvironment()
        XCTAssertEqual(env["SALMON_HUB_URL"], "https://hub2.example.com")
        XCTAssertEqual(env["SALMON_SYNC_INTERVAL"], "1200")
    }
}
