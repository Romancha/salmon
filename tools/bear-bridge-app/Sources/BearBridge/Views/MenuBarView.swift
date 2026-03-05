import SwiftUI

struct MenuBarView: View {
    @ObservedObject var appState: AppState

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
                Text(appState.syncStatus.displayText)
                    .font(.headline)
            }
            Text("Last sync: \(appState.lastSyncDescription)")
                .font(.caption)
                .foregroundColor(.secondary)
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 4)
    }

    private var syncButton: some View {
        Button {
            // Will be wired to IPC in Task 7
        } label: {
            Label("Sync Now", systemImage: "arrow.clockwise")
        }
    }

    private var statsSection: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text("Notes: \(appState.stats.notesCount.formatted())")
            Text("Tags: \(appState.stats.tagsCount.formatted())")
            Text("Queue: \(appState.stats.queueCount) pending")
        }
        .font(.caption)
        .padding(.horizontal, 12)
        .padding(.vertical, 4)
    }

    private var menuActions: some View {
        Group {
            Button {
                // Will open log window in Task 8
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
        switch appState.syncStatus {
        case .idle: return .green
        case .syncing: return .yellow
        case .error: return .red
        }
    }
}
