import Foundation

/// Represents the current sync status of the bridge.
enum SyncStatus: String {
    case idle
    case syncing
    case error

    var displayText: String {
        switch self {
        case .idle: return "Synced"
        case .syncing: return "Syncing..."
        case .error: return "Error"
        }
    }

    var iconColor: String {
        switch self {
        case .idle: return "green"
        case .syncing: return "yellow"
        case .error: return "red"
        }
    }
}

/// Sync statistics from the bridge.
struct SyncStats {
    var notesCount: Int = 0
    var tagsCount: Int = 0
    var queueCount: Int = 0
    var lastDurationMs: Int = 0
}

/// Observable state for the menu bar app.
@MainActor
final class AppState: ObservableObject {
    @Published var syncStatus: SyncStatus = .idle
    @Published var lastSyncTime: Date?
    @Published var lastError: String?
    @Published var stats: SyncStats = SyncStats()
    @Published var bridgeRunning: Bool = false

    var lastSyncDescription: String {
        guard let lastSync = lastSyncTime else {
            return "Never"
        }
        let formatter = RelativeDateTimeFormatter()
        formatter.unitsStyle = .full
        return formatter.localizedString(for: lastSync, relativeTo: Date())
    }

    func updateFromStatus(state: String, lastSync: String?, lastError: String?, stats: SyncStats?) {
        syncStatus = SyncStatus(rawValue: state) ?? .idle
        if let lastSync, let date = ISO8601DateFormatter().date(from: lastSync) {
            lastSyncTime = date
        }
        self.lastError = lastError
        if let stats {
            self.stats = stats
        }
    }

    func reset() {
        syncStatus = .idle
        lastSyncTime = nil
        lastError = nil
        stats = SyncStats()
        bridgeRunning = false
    }
}
