import SwiftUI

struct MenuBarView: View {
    @EnvironmentObject var appModel: AppModel
    @EnvironmentObject var viewModel: StatusViewModel
    @EnvironmentObject var logViewModel: LogViewModel
    @EnvironmentObject var settingsManager: SettingsManager
    @Environment(\.openWindow) private var openWindow

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            // Sync Status
            sectionHeader("Sync Status")
            StatusIndicator(
                status: viewModel.syncStatus,
                lastSyncDescription: viewModel.lastSyncDescription,
                bridgeConnected: viewModel.bridgeConnected,
                lastError: viewModel.lastError
            )
            .padding(.horizontal, 12)
            .padding(.bottom, 8)

            Divider()

            // Statistics
            sectionHeader("Statistics")
            VStack(spacing: 4) {
                StatRow(label: "Notes", value: viewModel.stats.notesCount.formatted())
                StatRow(label: "Tags", value: viewModel.stats.tagsCount.formatted())
                StatRow(label: "Queue", value: "\(viewModel.queueItems.count) pending")
            }
            .padding(.horizontal, 12)
            .padding(.bottom, 8)

            // Write Queue (conditional)
            if !viewModel.queueItems.isEmpty {
                Divider()
                sectionHeader("Write Queue")
                VStack(spacing: 4) {
                    ForEach(viewModel.queueItems) { item in
                        QueueItemRow(item: item)
                    }
                }
                .padding(.horizontal, 12)
                .padding(.bottom, 8)
            }

            Divider()

            // Actions
            VStack(spacing: 0) {
                syncNowButton
                ActionButton(title: "View Logs...", systemImage: "doc.text") {
                    activateApp()
                    openWindow(id: "log-viewer")
                    DispatchQueue.main.async {
                        for window in NSApp.windows where window.title.contains("Logs") {
                            window.makeKeyAndOrderFront(nil)
                            window.orderFrontRegardless()
                            break
                        }
                    }
                }
                SettingsLink {
                    HStack(spacing: 8) {
                        Image(systemName: "gear")
                            .frame(width: 16)
                            .foregroundColor(.secondary)
                        Text("Settings...")
                        Spacer()
                    }
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .padding(.horizontal, 12)
                .padding(.vertical, 4)
            }
            .padding(.vertical, 4)

            Divider()

            // Quit
            Button {
                appModel.shutdown()
                NSApplication.shared.terminate(nil)
            } label: {
                HStack(spacing: 8) {
                    Image(systemName: "power")
                        .frame(width: 16)
                        .foregroundColor(.secondary)
                    Text("Quit Salmon Run")
                    Spacer()
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .padding(.horizontal, 12)
            .padding(.vertical, 6)
            .keyboardShortcut("q")
        }
        .padding(.vertical, 4)
        .frame(width: 280)
    }

    private func activateApp() {
        NSApp.activate()
    }

    // MARK: - Subviews

    private func sectionHeader(_ title: String) -> some View {
        Text(title)
            .font(.subheadline)
            .fontWeight(.semibold)
            .foregroundColor(.secondary)
            .padding(.horizontal, 12)
            .padding(.top, 8)
            .padding(.bottom, 4)
    }

    private var syncNowButton: some View {
        Button {
            Task {
                if appModel.processManager.state == .stopped {
                    guard settingsManager.isConfigured else {
                        viewModel.lastError = "Configure connection settings before syncing"
                        viewModel.syncStatus = .error
                        return
                    }
                    do {
                        try appModel.processManager.start()
                        try? await Task.sleep(nanoseconds: 1_000_000_000)
                    } catch {
                        viewModel.lastError = "Failed to start bridge: \(error.localizedDescription)"
                        viewModel.syncStatus = .error
                        return
                    }
                }
                await viewModel.syncNow()
            }
        } label: {
            HStack(spacing: 8) {
                if viewModel.isSyncing {
                    ProgressView()
                        .controlSize(.small)
                        .scaleEffect(0.7)
                        .frame(width: 16)
                } else {
                    Image(systemName: "arrow.clockwise")
                        .frame(width: 16)
                        .foregroundColor(.secondary)
                }
                Text("Sync Now")
                Spacer()
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .padding(.horizontal, 12)
        .padding(.vertical, 4)
        .disabled(viewModel.isSyncing || viewModel.syncStatus == .syncing)
    }
}
