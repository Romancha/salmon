import SwiftUI

@main
struct BearBridgeApp: App {
    @StateObject private var viewModel: StatusViewModel
    @StateObject private var logViewModel: LogViewModel

    init() {
        let ipcClient = BridgeIPCClient()
        _viewModel = StateObject(wrappedValue: StatusViewModel(ipcClient: ipcClient))
        _logViewModel = StateObject(wrappedValue: LogViewModel(ipcClient: ipcClient))
    }

    var body: some Scene {
        MenuBarExtra {
            MenuBarView(viewModel: viewModel, logViewModel: logViewModel)
                .onAppear {
                    viewModel.startPolling()
                }
                .onDisappear {
                    viewModel.stopPolling()
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
