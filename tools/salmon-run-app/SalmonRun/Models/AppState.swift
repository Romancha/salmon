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

}

/// Sync statistics from the bridge.
struct SyncStats {
    var notesCount: Int = 0
    var tagsCount: Int = 0
    var queueCount: Int = 0
    var lastDurationMs: Int = 0
}
