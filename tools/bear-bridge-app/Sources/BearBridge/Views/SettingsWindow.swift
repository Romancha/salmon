import SwiftUI

struct SettingsWindow: View {
    @ObservedObject var settings: SettingsManager

    var body: some View {
        TabView {
            connectionTab
                .tabItem {
                    Label("Connection", systemImage: "network")
                }

            syncTab
                .tabItem {
                    Label("Sync", systemImage: "arrow.triangle.2.circlepath")
                }

            generalTab
                .tabItem {
                    Label("General", systemImage: "gear")
                }
        }
        .frame(width: 450, height: 300)
        .onAppear {
            settings.refreshLoginItemStatus()
        }
    }

    // MARK: - Connection tab

    private var connectionTab: some View {
        Form {
            Section {
                TextField("Hub URL", text: $settings.hubURL)
                    .textFieldStyle(.roundedBorder)
                    .help("The URL of your bear-sync hub server (e.g. https://hub.example.com)")
            } header: {
                Text("Hub Server")
            }

            Section {
                SecureField("Hub Token", text: hubTokenBinding)
                    .textFieldStyle(.roundedBorder)
                    .help("Authentication token for the hub API (bridge scope)")

                SecureField("Bear Token", text: bearTokenBinding)
                    .textFieldStyle(.roundedBorder)
                    .help("Authentication token for Bear note access")
            } header: {
                Text("Tokens (stored in Keychain)")
            }

            if settings.isConfigured {
                HStack {
                    Image(systemName: "checkmark.circle.fill")
                        .foregroundColor(.green)
                    Text("All connection settings configured")
                        .font(.caption)
                        .foregroundColor(.secondary)
                }
            }

            restartBridgeButton
        }
        .padding()
    }

    // MARK: - Sync tab

    private var syncTab: some View {
        Form {
            Section {
                HStack {
                    Text("Sync every \(settings.syncIntervalMinutes) min")
                    Spacer()
                    Slider(
                        value: syncIntervalBinding,
                        in: syncIntervalSliderRange,
                        step: 1
                    )
                    .frame(width: 200)
                }

                Toggle("Sync on app launch", isOn: $settings.syncOnLaunch)
            } header: {
                Text("Sync Schedule")
            }

            restartBridgeButton
        }
        .padding()
    }

    // MARK: - General tab

    private var generalTab: some View {
        Form {
            Section {
                Toggle("Launch at Login", isOn: $settings.launchAtLogin)
                    .help("Automatically start Bear Bridge when you log in")
            } header: {
                Text("Startup")
            }

            Section {
                Toggle("Show error notifications", isOn: $settings.notificationsEnabled)
                    .help("Show a macOS notification when sync errors occur")
            } header: {
                Text("Notifications")
            }
        }
        .padding()
    }

    private var restartBridgeButton: some View {
        Section {
            Button("Restart Bridge to Apply Changes") {
                NotificationCenter.default.post(name: .restartBridge, object: nil)
            }
            .font(.caption)
        }
    }

    // MARK: - Bindings for Keychain-backed properties

    private var hubTokenBinding: Binding<String> {
        Binding(
            get: { settings.hubToken },
            set: { settings.hubToken = $0 }
        )
    }

    private var bearTokenBinding: Binding<String> {
        Binding(
            get: { settings.bearToken },
            set: { settings.bearToken = $0 }
        )
    }

    private var syncIntervalBinding: Binding<Double> {
        Binding(
            get: { Double(settings.syncIntervalMinutes) },
            set: { settings.syncIntervalMinutes = Int($0) }
        )
    }

    private var syncIntervalSliderRange: ClosedRange<Double> {
        let lower = Double(SettingsManager.syncIntervalRange.lowerBound)
        let upper = Double(SettingsManager.syncIntervalRange.upperBound)
        return lower...upper
    }
}
