import Foundation
import ServiceManagement

/// Keys used in UserDefaults for non-sensitive settings.
enum SettingsKey {
    static let hubURL = "hubURL"
    static let syncIntervalMinutes = "syncIntervalMinutes"
    static let launchAtLogin = "launchAtLogin"
    static let notificationsEnabled = "notificationsEnabled"
}

/// Keys used in Keychain for sensitive tokens.
enum TokenKey {
    static let hubToken = "hubToken"
    static let bearToken = "bearToken"
}

/// Protocol for settings storage, enabling test mocking of UserDefaults.
protocol SettingsStore {
    func string(forKey key: String) -> String?
    func set(_ value: Any?, forKey key: String)
    func integer(forKey key: String) -> Int
    func bool(forKey key: String) -> Bool
    func register(defaults: [String: Any])
}

extension UserDefaults: SettingsStore {}

/// Protocol for controlling Launch at Login, enabling test mocking.
protocol LoginItemManager {
    var isEnabled: Bool { get }
    func setEnabled(_ enabled: Bool) throws
}

/// Real implementation using SMAppService (macOS 13+).
@available(macOS 13.0, *)
struct SystemLoginItemManager: LoginItemManager {
    var isEnabled: Bool {
        SMAppService.mainApp.status == .enabled
    }

    func setEnabled(_ enabled: Bool) throws {
        if enabled {
            try SMAppService.mainApp.register()
        } else {
            try SMAppService.mainApp.unregister()
        }
    }
}

/// Manages all app settings: UserDefaults for non-sensitive values, Keychain for tokens.
///
/// Provides `bridgeEnvironment()` to generate environment variables for the bridge process.
@MainActor
final class SettingsManager: ObservableObject {

    static let defaultSyncIntervalMinutes = 5
    static let syncIntervalRange = 1...30

    @Published var hubURL: String {
        didSet { store.set(hubURL, forKey: SettingsKey.hubURL) }
    }

    @Published var syncIntervalMinutes: Int {
        didSet { store.set(syncIntervalMinutes, forKey: SettingsKey.syncIntervalMinutes) }
    }

    @Published var launchAtLogin: Bool {
        didSet {
            guard !suppressLaunchAtLoginDidSet else { return }
            do {
                try loginItemManager?.setEnabled(launchAtLogin)
            } catch {
                lastSettingsError = "Failed to update Launch at Login: \(error.localizedDescription)"
                suppressLaunchAtLoginDidSet = true
                launchAtLogin = oldValue
                suppressLaunchAtLoginDidSet = false
                return
            }
            store.set(launchAtLogin, forKey: SettingsKey.launchAtLogin)
        }
    }

    @Published var notificationsEnabled: Bool {
        didSet { store.set(notificationsEnabled, forKey: SettingsKey.notificationsEnabled) }
    }

    @Published var lastSettingsError: String?

    private let store: SettingsStore
    private let keychain: KeychainServiceProtocol
    private let loginItemManager: LoginItemManager?
    private var suppressLaunchAtLoginDidSet = false

    /// - Parameters:
    ///   - store: Settings store (defaults to UserDefaults.standard).
    ///   - keychain: Keychain service (defaults to real Keychain).
    ///   - loginItemManager: Login item manager (defaults to SMAppService on macOS 13+).
    init(
        store: SettingsStore? = nil,
        keychain: KeychainServiceProtocol? = nil,
        loginItemManager: LoginItemManager? = nil
    ) {
        let resolvedStore = store ?? UserDefaults.standard
        let resolvedKeychain = keychain ?? KeychainService()

        // Register defaults before reading
        resolvedStore.register(defaults: [
            SettingsKey.syncIntervalMinutes: Self.defaultSyncIntervalMinutes,
            SettingsKey.launchAtLogin: false,
            SettingsKey.notificationsEnabled: true,
        ])

        self.store = resolvedStore
        self.keychain = resolvedKeychain

        if let loginItemManager {
            self.loginItemManager = loginItemManager
        } else if #available(macOS 13.0, *) {
            self.loginItemManager = SystemLoginItemManager()
        } else {
            self.loginItemManager = nil
        }

        // Suppress didSet side effects during init
        self.suppressLaunchAtLoginDidSet = true

        // Load saved values
        self.hubURL = resolvedStore.string(forKey: SettingsKey.hubURL) ?? ""
        self.syncIntervalMinutes = resolvedStore.integer(forKey: SettingsKey.syncIntervalMinutes)
        self.launchAtLogin = resolvedStore.bool(forKey: SettingsKey.launchAtLogin)
        self.notificationsEnabled = resolvedStore.bool(forKey: SettingsKey.notificationsEnabled)

        self.suppressLaunchAtLoginDidSet = false
    }

    // MARK: - Token management (Keychain)

    var hubToken: String {
        get { keychain.load(key: TokenKey.hubToken) ?? "" }
        set {
            if newValue.isEmpty {
                keychain.delete(key: TokenKey.hubToken)
            } else {
                do {
                    try keychain.save(key: TokenKey.hubToken, value: newValue)
                } catch {
                    lastSettingsError = "Failed to save hub token to Keychain: \(error.localizedDescription)"
                }
            }
            objectWillChange.send()
        }
    }

    var bearToken: String {
        get { keychain.load(key: TokenKey.bearToken) ?? "" }
        set {
            if newValue.isEmpty {
                keychain.delete(key: TokenKey.bearToken)
            } else {
                do {
                    try keychain.save(key: TokenKey.bearToken, value: newValue)
                } catch {
                    lastSettingsError = "Failed to save Bear token to Keychain: \(error.localizedDescription)"
                }
            }
            objectWillChange.send()
        }
    }

    // MARK: - Environment generation

    /// Generates the environment variables needed by the bear-bridge process.
    /// Only includes variables that have non-empty values.
    func bridgeEnvironment() -> [String: String] {
        var env: [String: String] = [:]

        if !hubURL.isEmpty {
            env["BRIDGE_HUB_URL"] = hubURL
        }
        if !hubToken.isEmpty {
            env["BRIDGE_HUB_TOKEN"] = hubToken
        }
        if !bearToken.isEmpty {
            env["BEAR_TOKEN"] = bearToken
        }

        let intervalSeconds = syncIntervalMinutes * 60
        env["BRIDGE_SYNC_INTERVAL"] = String(intervalSeconds)

        return env
    }

    /// Whether all required connection settings are configured.
    var isConfigured: Bool {
        !hubURL.isEmpty && !hubToken.isEmpty && !bearToken.isEmpty
    }

    // MARK: - Launch at Login state sync

    /// Refreshes launchAtLogin from the system state (in case user changed it in System Settings).
    /// Bypasses didSet to avoid calling setEnabled back to the system.
    func refreshLoginItemStatus() {
        if let manager = loginItemManager {
            let systemState = manager.isEnabled
            if launchAtLogin != systemState {
                suppressLaunchAtLoginDidSet = true
                launchAtLogin = systemState
                suppressLaunchAtLoginDidSet = false
                store.set(launchAtLogin, forKey: SettingsKey.launchAtLogin)
            }
        }
    }
}
