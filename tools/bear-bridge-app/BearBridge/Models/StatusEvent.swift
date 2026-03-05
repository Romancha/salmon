import Foundation

/// Type of sync status event emitted by the bridge daemon.
enum StatusEventType: String {
    case syncStart = "sync_start"
    case syncProgress = "sync_progress"
    case syncComplete = "sync_complete"
    case syncError = "sync_error"
}

/// A structured status event parsed from bridge stdout.
/// These events are emitted by the bridge EventEmitter alongside slog log lines.
struct StatusEvent: Equatable {
    let event: StatusEventType
    let time: Date
    let phase: String?
    let notes: Int?
    let items: Int?
    let durationMs: Int?
    let notesSynced: Int?
    let tagsSynced: Int?
    let queueItems: Int?
    let error: String?

    init(
        event: StatusEventType,
        time: Date = Date(),
        phase: String? = nil,
        notes: Int? = nil,
        items: Int? = nil,
        durationMs: Int? = nil,
        notesSynced: Int? = nil,
        tagsSynced: Int? = nil,
        queueItems: Int? = nil,
        error: String? = nil
    ) {
        self.event = event
        self.time = time
        self.phase = phase
        self.notes = notes
        self.items = items
        self.durationMs = durationMs
        self.notesSynced = notesSynced
        self.tagsSynced = tagsSynced
        self.queueItems = queueItems
        self.error = error
    }
}
