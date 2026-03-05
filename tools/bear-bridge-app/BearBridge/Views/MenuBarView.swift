import SwiftUI

struct MenuBarView: View {
    @EnvironmentObject var appModel: AppModel
    @EnvironmentObject var viewModel: StatusViewModel
    @EnvironmentObject var logViewModel: LogViewModel
    @EnvironmentObject var settingsManager: SettingsManager
    @Environment(\.openWindow) private var openWindow

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            statusSection
            Divider()
            syncButton
            Divider()
            statsSection
            queueSection
            Divider()
            menuActions
            Divider()
            quitButton
        }
        .padding(.vertical, 4)
    }

    private var statusSection: some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack(spacing: 6) {
                Circle()
                    .fill(statusColor)
                    .frame(width: 8, height: 8)
                Text(viewModel.syncStatus.displayText)
                    .font(.headline)
            }
            Text("Last sync: \(viewModel.lastSyncDescription)")
                .font(.caption)
                .foregroundColor(.secondary)
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 4)
    }

    private var syncButton: some View {
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
            HStack(spacing: 6) {
                if viewModel.isSyncing {
                    ProgressView()
                        .controlSize(.small)
                        .scaleEffect(0.7)
                }
                Label("Sync Now", systemImage: "arrow.clockwise")
            }
        }
        .disabled(viewModel.isSyncing || viewModel.syncStatus == .syncing)
    }

    private var statsSection: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text("Notes synced: \(viewModel.stats.notesCount.formatted())")
            Text("Tags synced: \(viewModel.stats.tagsCount.formatted())")
            Text("Queue: \(viewModel.queueItems.count) pending")
        }
        .font(.caption)
        .padding(.horizontal, 12)
        .padding(.vertical, 4)
    }

    @ViewBuilder
    private var queueSection: some View {
        if !viewModel.queueItems.isEmpty {
            Divider()
            DisclosureGroup("Write Queue (\(viewModel.queueItems.count))") {
                ForEach(viewModel.queueItems) { item in
                    HStack(spacing: 6) {
                        Text(item.action)
                            .font(.caption)
                            .foregroundColor(.secondary)
                        if !item.noteTitle.isEmpty {
                            Text(item.noteTitle)
                                .font(.caption)
                                .lineLimit(1)
                        }
                        Spacer()
                        Text(item.status)
                            .font(.caption2)
                            .foregroundColor(queueItemStatusColor(item.status))
                    }
                }
            }
            .font(.caption)
            .padding(.horizontal, 12)
            .padding(.vertical, 2)
        }
    }

    private var menuActions: some View {
        Group {
            Button {
                openWindow(id: "log-viewer")
            } label: {
                Label("View Logs...", systemImage: "doc.text")
            }
            Button {
                NSApp.sendAction(Selector(("showSettingsWindow:")), to: nil, from: nil)
            } label: {
                Label("Settings...", systemImage: "gear")
            }
        }
    }

    private var quitButton: some View {
        Button {
            appModel.shutdown()
            NSApplication.shared.terminate(nil)
        } label: {
            Text("Quit Bear Bridge")
        }
        .keyboardShortcut("q")
    }

    private var statusColor: Color {
        switch viewModel.syncStatus {
        case .idle: return .green
        case .syncing: return .yellow
        case .error: return .red
        }
    }

    private func queueItemStatusColor(_ status: String) -> Color {
        switch status {
        case "applied": return .green
        case "failed": return .red
        case "conflict": return .orange
        default: return .secondary
        }
    }
}
