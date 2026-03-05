import Foundation

/// Log level matching slog JSON handler output.
enum LogLevel: String, CaseIterable, Comparable {
    case debug = "DEBUG"
    case info = "INFO"
    case warn = "WARN"
    case error = "ERROR"

    private var sortOrder: Int {
        switch self {
        case .debug: return 0
        case .info: return 1
        case .warn: return 2
        case .error: return 3
        }
    }

    static func < (lhs: LogLevel, rhs: LogLevel) -> Bool {
        lhs.sortOrder < rhs.sortOrder
    }
}

/// A single parsed log entry from bridge stdout (slog JSON format).
struct LogEntry: Identifiable {
    let id: UUID
    let time: Date
    let level: LogLevel
    let message: String
    let fields: [String: String]

    init(id: UUID = UUID(), time: Date, level: LogLevel, message: String, fields: [String: String] = [:]) {
        self.id = id
        self.time = time
        self.level = level
        self.message = message
        self.fields = fields
    }
}
