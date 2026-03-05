import SwiftUI

/// Displays a single write queue item with action, title, and status badge.
struct QueueItemRow: View {
    let item: IPCQueueStatusItem

    var body: some View {
        HStack(spacing: 6) {
            Text(item.action)
                .font(.caption)
                .fontWeight(.medium)
                .foregroundColor(.secondary)
            if !item.noteTitle.isEmpty {
                Text("\"\(item.noteTitle)\"")
                    .font(.caption)
                    .lineLimit(1)
                    .truncationMode(.tail)
            }
            Spacer()
            Text(item.status)
                .font(.caption2)
                .fontWeight(.medium)
                .padding(.horizontal, 6)
                .padding(.vertical, 2)
                .background(statusColor.opacity(0.15))
                .foregroundColor(statusColor)
                .clipShape(Capsule())
        }
    }

    private var statusColor: Color {
        switch item.status {
        case "applied": return .green
        case "failed": return .red
        case "conflict": return .orange
        case "leased": return .blue
        default: return .secondary
        }
    }
}
