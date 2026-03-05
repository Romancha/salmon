import Foundation

/// Response to a "status" IPC command.
/// Matches Go `ipc.StatusResponse`.
struct IPCStatusResponse: Codable, Equatable {
    let state: String
    let lastSync: String
    let lastError: String
    let stats: IPCSyncStats
    let error: String?

    enum CodingKeys: String, CodingKey {
        case state
        case lastSync = "last_sync"
        case lastError = "last_error"
        case stats
        case error
    }
}

/// Sync statistics from IPC status response.
/// Matches Go `ipc.SyncStats`.
struct IPCSyncStats: Codable, Equatable {
    let notesSynced: Int
    let tagsSynced: Int
    let queueProcessed: Int
    let lastDurationMs: Int64

    enum CodingKeys: String, CodingKey {
        case notesSynced = "notes_synced"
        case tagsSynced = "tags_synced"
        case queueProcessed = "queue_processed"
        case lastDurationMs = "last_duration_ms"
    }
}

/// Response to "sync_now" and "quit" IPC commands.
/// Matches Go `ipc.OkResponse`.
struct IPCOkResponse: Codable, Equatable {
    let ok: Bool
    let error: String?
}

/// A single log entry from IPC logs response.
/// Matches Go `ipc.LogEntry`.
struct IPCLogEntry: Codable, Equatable {
    let time: String
    let level: String
    let msg: String
}

/// Response to "logs" IPC command.
/// Matches Go `ipc.LogsResponse`.
struct IPCLogsResponse: Codable, Equatable {
    let entries: [IPCLogEntry]
    let error: String?
}

/// A single write queue item from IPC queue_status response.
/// Matches Go `ipc.QueueStatusItem`.
struct IPCQueueStatusItem: Codable, Equatable, Identifiable {
    let id: Int64
    let action: String
    let noteTitle: String
    let status: String
    let createdAt: String?

    enum CodingKeys: String, CodingKey {
        case id
        case action
        case noteTitle = "note_title"
        case status
        case createdAt = "created_at"
    }
}

/// Response to "queue_status" IPC command.
/// Matches Go `ipc.QueueStatusResponse`.
struct IPCQueueStatusResponse: Codable, Equatable {
    let items: [IPCQueueStatusItem]
    let error: String?
}
