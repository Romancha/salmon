import SwiftUI

@main
struct BearBridgeApp: App {
    @StateObject private var appState = AppState()

    var body: some Scene {
        MenuBarExtra {
            MenuBarView(appState: appState)
        } label: {
            Image(systemName: menuBarIcon)
                .symbolRenderingMode(.palette)
                .foregroundStyle(menuBarIconColor)
        }
        .menuBarExtraStyle(.window)
    }

    private var menuBarIcon: String {
        "arrow.triangle.2.circlepath"
    }

    private var menuBarIconColor: Color {
        switch appState.syncStatus {
        case .idle: return .primary
        case .syncing: return .yellow
        case .error: return .red
        }
    }
}
