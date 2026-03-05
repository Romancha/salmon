import SwiftUI

struct SettingsWindow: View {
    @EnvironmentObject var appModel: AppModel
    @EnvironmentObject var settings: SettingsManager

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

            aboutTab
                .tabItem {
                    Label("About", systemImage: "info.circle")
                }
        }
        .tabViewStyle(.automatic)
        .frame(width: 450, height: 300)
        .onAppear {
            settings.refreshLoginItemStatus()
        }
        .alert("Settings Error", isPresented: hasSettingsError) {
            Button("OK") {
                settings.lastSettingsError = nil
            }
        } message: {
            Text(settings.lastSettingsError ?? "")
        }
    }

    private var hasSettingsError: Binding<Bool> {
        Binding(
            get: { settings.lastSettingsError != nil },
            set: { if !$0 { settings.lastSettingsError = nil } }
        )
    }

    // MARK: - Connection tab

    private var connectionTab: some View {
        Form {
            Section {
                HStack {
                    TextField("Hub URL", text: $settings.hubURL)
                        .textFieldStyle(.roundedBorder)
                        .help("The URL of your bear-sync hub server (e.g. https://hub.example.com)")
                        .onChange(of: settings.hubURL) { _ in
                            scheduleRestart()
                        }
                    validationIcon(isValid: !settings.hubURL.isEmpty)
                }
            } header: {
                Text("Hub Server")
            }

            Section {
                HStack {
                    SecureField("Hub Token", text: hubTokenBinding)
                        .textFieldStyle(.roundedBorder)
                        .help("Authentication token for the hub API (bridge scope)")
                    validationIcon(isValid: !settings.hubToken.isEmpty)
                }

                HStack {
                    SecureField("Bear Token", text: bearTokenBinding)
                        .textFieldStyle(.roundedBorder)
                        .help("Authentication token for Bear note access")
                    validationIcon(isValid: !settings.bearToken.isEmpty)
                }
            } header: {
                Text("Tokens (stored in Keychain)")
            }
        }
        .padding()
    }

    // MARK: - Sync tab

    private var syncTab: some View {
        Form {
            Section {
                VStack(alignment: .leading, spacing: 8) {
                    Text("Sync every \(settings.syncIntervalMinutes) min")
                        .font(.body)
                    Slider(
                        value: syncIntervalBinding,
                        in: syncIntervalSliderRange,
                        step: 1
                    )
                }
            } header: {
                Text("Sync Schedule")
            }

            Section {
                Toggle("Sync on launch", isOn: .constant(true))
                    .disabled(true)
                    .help("Bridge always syncs when it starts")
            } header: {
                Text("Behavior")
            }
        }
        .padding()
        .onChange(of: settings.syncIntervalMinutes) { _ in
            scheduleRestart()
        }
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

    // MARK: - About tab

    private var aboutTab: some View {
        Form {
            Section {
                LabeledContent("App Version") {
                    Text(appVersion)
                        .textSelection(.enabled)
                }
                LabeledContent("Bridge Version") {
                    Text(bridgeVersion)
                        .textSelection(.enabled)
                }
            } header: {
                Text("Version")
            }

            Section {
                LabeledContent("GitHub") {
                    Link("bear-sync", destination: URL(string: "https://github.com/romancha/bear-sync")!)
                }
            } header: {
                Text("Links")
            }
        }
        .padding()
    }

    // MARK: - Helpers

    @ViewBuilder
    private func validationIcon(isValid: Bool) -> some View {
        if isValid {
            Image(systemName: "checkmark.circle.fill")
                .foregroundColor(.green)
                .help("Configured")
        } else {
            Image(systemName: "exclamationmark.triangle.fill")
                .foregroundColor(.orange)
                .help("Not configured")
        }
    }

    private var appVersion: String {
        Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "Unknown"
    }

    private var bridgeVersion: String {
        appModel.statusViewModel.bridgeVersion ?? "Unknown"
    }

    private func scheduleRestart() {
        appModel.scheduleRestart()
    }

    private var hubTokenBinding: Binding<String> {
        Binding(
            get: { settings.hubToken },
            set: {
                settings.hubToken = $0
                scheduleRestart()
            }
        )
    }

    private var bearTokenBinding: Binding<String> {
        Binding(
            get: { settings.bearToken },
            set: {
                settings.bearToken = $0
                scheduleRestart()
            }
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
