import SwiftUI

/// Central application model owning all services and view models.
///
/// Single dependency injection point — created once as @StateObject in BearBridgeApp
/// and distributed to all views via @EnvironmentObject through AppRoot.
@MainActor
final class AppModel: ObservableObject {

    let statusViewModel: StatusViewModel
    let logViewModel: LogViewModel
    let settingsManager: SettingsManager
    let notificationService: NotificationService
    let processManager: BridgeProcessManager

    @Published var isInitialized = false

    /// Creates AppModel with all services wired together.
    /// - Parameters:
    ///   - settingsManager: Settings manager (injectable for testing).
    ///   - ipcClient: IPC client (injectable for testing).
    ///   - notificationService: Notification service (injectable for testing).
    ///   - processManager: Bridge process manager (injectable for testing).
    init(
        settingsManager: SettingsManager? = nil,
        ipcClient: IPCClientProtocol? = nil,
        notificationService: NotificationService? = nil,
        processManager: BridgeProcessManager? = nil
    ) {
        let settings = settingsManager ?? SettingsManager()
        let client = ipcClient ?? BridgeIPCClient()
        let notifications = notificationService ?? NotificationService()
        notifications.isEnabled = settings.notificationsEnabled

        let logVM = LogViewModel(ipcClient: client)
        let statusVM = StatusViewModel(ipcClient: client, notificationService: notifications)

        let pm = processManager ?? BridgeProcessManager(environmentProvider: { settings.bridgeEnvironment() })
        pm.onLogEntry = { entry in
            DispatchQueue.main.async {
                logVM.addEntry(entry)
            }
        }
        pm.onStatusEvent = { event in
            DispatchQueue.main.async {
                statusVM.handleStatusEvent(event)
            }
        }

        self.settingsManager = settings
        self.statusViewModel = statusVM
        self.logViewModel = logVM
        self.notificationService = notifications
        self.processManager = pm

        notifications.onOpenLogViewer = {
            NotificationCenter.default.post(name: .openLogViewer, object: nil)
        }
    }

    /// Initialize the app — start bridge if configured.
    func initialize() {
        guard !isInitialized else { return }
        isInitialized = true

        if settingsManager.isConfigured {
            do {
                try processManager.start()
            } catch {
                statusViewModel.syncStatus = .error
                statusViewModel.lastError = "Failed to start bridge: \(error.localizedDescription)"
            }
        }
    }

    /// Restart the bridge process (e.g. after settings change).
    func restartBridge() {
        do {
            try processManager.restart()
        } catch {
            statusViewModel.syncStatus = .error
            statusViewModel.lastError = "Failed to start bridge: \(error.localizedDescription)"
        }
    }

    /// Clean shutdown.
    func shutdown() {
        processManager.stop()
    }
}
