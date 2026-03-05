import SwiftUI

struct MenuBarView: View {
    @ObservedObject var viewModel: StatusViewModel
    @ObservedObject var logViewModel: LogViewModel
    @Environment(\.openWindow) private var openWindow

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            statusSection
            Divider()
            syncButton
            Divider()
            statsSection
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
        .disabled(viewModel.isSyncing)
    }

    private var statsSection: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text("Notes: \(viewModel.stats.notesCount.formatted())")
            Text("Tags: \(viewModel.stats.tagsCount.formatted())")
            Text("Queue: \(viewModel.stats.queueCount) pending")
        }
        .font(.caption)
        .padding(.horizontal, 12)
        .padding(.vertical, 4)
    }

    private var menuActions: some View {
        Group {
            Button {
                openWindow(id: "log-viewer")
            } label: {
                Label("View Logs...", systemImage: "doc.text")
            }
            Button {
                // Will open settings window in Task 9
            } label: {
                Label("Settings...", systemImage: "gear")
            }
        }
    }

    private var quitButton: some View {
        Button {
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
}
