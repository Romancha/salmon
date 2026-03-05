import SwiftUI

extension Notification.Name {
    static let openLogViewer = Notification.Name("openLogViewer")
}

@main
struct BearBridgeApp: App {
    @StateObject private var appModel = AppModel()
    @Environment(\.openWindow) private var openWindow

    var body: some Scene {
        MenuBarExtra {
            AppRoot(app: appModel) {
                MenuBarView()
            }
            .onAppear {
                appModel.statusViewModel.startPolling()
            }
            .onDisappear {
                appModel.statusViewModel.stopPolling()
            }
            .onReceive(NotificationCenter.default.publisher(for: .openLogViewer)) { _ in
                openWindow(id: "log-viewer")
            }
            .onReceive(NotificationCenter.default.publisher(for: NSApplication.willTerminateNotification)) { _ in
                appModel.statusViewModel.stopPolling()
                appModel.shutdown()
            }
        } label: {
            Image(systemName: menuBarIcon)
                .symbolRenderingMode(.palette)
                .foregroundStyle(menuBarIconColor)
        }
        .menuBarExtraStyle(.window)

        Window("Bear Bridge Logs", id: "log-viewer") {
            AppRoot(app: appModel) {
                LogViewerWindow()
            }
        }
        .defaultSize(width: 700, height: 500)

        Settings {
            AppRoot(app: appModel) {
                SettingsWindow()
            }
            .onReceive(appModel.settingsManager.$notificationsEnabled) { enabled in
                appModel.notificationService.isEnabled = enabled
            }
        }
    }

    private var menuBarIcon: String {
        if !appModel.statusViewModel.bridgeConnected {
            return "arrow.triangle.2.circlepath"
        }
        switch appModel.statusViewModel.syncStatus {
        case .idle: return "arrow.triangle.2.circlepath"
        case .syncing: return "arrow.triangle.2.circlepath"
        case .error: return "exclamationmark.arrow.triangle.2.circlepath"
        }
    }

    private var menuBarIconColor: Color {
        if !appModel.statusViewModel.bridgeConnected {
            return .secondary
        }
        switch appModel.statusViewModel.syncStatus {
        case .idle: return .green
        case .syncing: return .yellow
        case .error: return .red
        }
    }
}
