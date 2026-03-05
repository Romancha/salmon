import SwiftUI

/// Colored dot with label indicating sync status.
struct StatusIndicator: View {
    let status: SyncStatus
    let lastSyncDescription: String
    let bridgeConnected: Bool
    let lastError: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack(spacing: 6) {
                Circle()
                    .fill(statusColor)
                    .frame(width: 8, height: 8)
                Text(statusLabel)
                    .font(.headline)
            }
            Text(subtitleText)
                .font(.caption)
                .foregroundColor(.secondary)
                .lineLimit(2)
        }
    }

    private var statusColor: Color {
        if !bridgeConnected { return .gray }
        switch status {
        case .idle: return .green
        case .syncing: return .yellow
        case .error: return .red
        }
    }

    private var statusLabel: String {
        if !bridgeConnected { return "Disconnected" }
        return status.displayText
    }

    private var subtitleText: String {
        if !bridgeConnected {
            return "Bridge is not running"
        }
        if let error = lastError, status == .error {
            return error
        }
        return "Last sync: \(lastSyncDescription)"
    }
}
