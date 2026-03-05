import SwiftUI

extension Notification.Name {
    static let openLogViewer = Notification.Name("openLogViewer")
    static let restartBridge = Notification.Name("restartBridge")
}

@main
struct BearBridgeApp: App {
    @StateObject private var viewModel: StatusViewModel
    @StateObject private var logViewModel: LogViewModel
    @StateObject private var settingsManager: SettingsManager
    @Environment(\.openWindow) private var openWindow
    private let notificationService: NotificationService
    private let processManager: BridgeProcessManager

    init() {
        let ipcClient = BridgeIPCClient()
        let settings = SettingsManager()
        let notifications = NotificationService()
        notifications.isEnabled = settings.notificationsEnabled
        notifications.onOpenLogViewer = {
            NotificationCenter.default.post(name: .openLogViewer, object: nil)
        }

        let logVM = LogViewModel(ipcClient: ipcClient)

        let statusVM = StatusViewModel(
            ipcClient: ipcClient,
            notificationService: notifications
        )
        _viewModel = StateObject(wrappedValue: statusVM)
        _logViewModel = StateObject(wrappedValue: logVM)
        _settingsManager = StateObject(wrappedValue: settings)
        self.notificationService = notifications

        let pm = BridgeProcessManager(environmentProvider: { settings.bridgeEnvironment() })
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
        self.processManager = pm

        if settings.isConfigured {
            do {
                try pm.start()
            } catch {
                statusVM.syncStatus = .error
                statusVM.lastError = "Failed to start bridge: \(error.localizedDescription)"
            }
        }
    }

    var body: some Scene {
        MenuBarExtra {
            MenuBarView(viewModel: viewModel, logViewModel: logViewModel, processManager: processManager)
                .onAppear {
                    viewModel.startPolling()
                }
                .onDisappear {
                    viewModel.stopPolling()
                }
                .onReceive(NotificationCenter.default.publisher(for: .openLogViewer)) { _ in
                    openWindow(id: "log-viewer")
                }
        } label: {
            Image(systemName: menuBarIcon)
                .symbolRenderingMode(.palette)
                .foregroundStyle(menuBarIconColor)
        }
        .menuBarExtraStyle(.window)

        Window("Bear Bridge Logs", id: "log-viewer") {
            LogViewerWindow(viewModel: logViewModel)
        }
        .defaultSize(width: 700, height: 500)

        Window("Bear Bridge Settings", id: "settings") {
            SettingsWindow(settings: settingsManager)
                .onReceive(settingsManager.$notificationsEnabled) { enabled in
                    notificationService.isEnabled = enabled
                }
                .onReceive(NotificationCenter.default.publisher(for: .restartBridge)) { _ in
                    try? processManager.restart()
                }
        }
        .defaultSize(width: 450, height: 300)
    }

    private var menuBarIcon: String {
        "arrow.triangle.2.circlepath"
    }

    private var menuBarIconColor: Color {
        switch viewModel.syncStatus {
        case .idle: return .primary
        case .syncing: return .yellow
        case .error: return .red
        }
    }
}
