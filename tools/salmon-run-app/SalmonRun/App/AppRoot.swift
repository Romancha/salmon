import SwiftUI

/// Wrapper view that injects all services into the environment.
///
/// Every scene in BearBridgeApp wraps its content in AppRoot to ensure
/// consistent access to AppModel, ViewModels, and services via @EnvironmentObject.
struct AppRoot<Content: View>: View {
    @ObservedObject var app: AppModel
    @ViewBuilder var content: () -> Content

    var body: some View {
        content()
            .environmentObject(app)
            .environmentObject(app.statusViewModel)
            .environmentObject(app.logViewModel)
            .environmentObject(app.settingsManager)
    }
}
