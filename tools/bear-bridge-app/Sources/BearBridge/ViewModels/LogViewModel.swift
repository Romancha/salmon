import Foundation

/// View model managing log entries for the log viewer window.
///
/// Supports text search, log level filtering, entry limit, and auto-scroll.
/// Entries are populated from IPC `logs` command and live stdout stream.
@MainActor
final class LogViewModel: ObservableObject {

    nonisolated static let defaultMaxEntries = 500

    @Published var entries: [LogEntry] = []
    @Published var searchText: String = ""
    @Published var activeLevels: Set<LogLevel> = Set(LogLevel.allCases)
    @Published var autoScroll: Bool = true
    @Published var isLoading: Bool = false

    let maxEntries: Int
    private let ipcClient: IPCClientProtocol?

    var filteredEntries: [LogEntry] {
        entries.filter { entry in
            guard activeLevels.contains(entry.level) else { return false }
            if searchText.isEmpty { return true }
            let query = searchText.lowercased()
            return entry.message.lowercased().contains(query)
                || entry.level.rawValue.lowercased().contains(query)
                || entry.fields.values.contains { $0.lowercased().contains(query) }
        }
    }

    init(ipcClient: IPCClientProtocol? = nil, maxEntries: Int = LogViewModel.defaultMaxEntries) {
        self.ipcClient = ipcClient
        self.maxEntries = maxEntries
    }

    /// Add a single log entry (from live stdout stream).
    func addEntry(_ entry: LogEntry) {
        entries.append(entry)
        trimEntries()
    }

    /// Add multiple log entries at once (from IPC response).
    func addEntries(_ newEntries: [LogEntry]) {
        entries.append(contentsOf: newEntries)
        trimEntries()
    }

    /// Load log entries from IPC `logs` command.
    func loadFromIPC() async {
        guard let client = ipcClient else { return }
        isLoading = true
        defer { isLoading = false }

        do {
            let response = try await client.getLogs(lines: maxEntries)
            let parsed = response.entries.compactMap { ipcEntry -> LogEntry? in
                let time = OutputParser.parseISO8601(ipcEntry.time) ?? Date()
                let level = LogLevel(rawValue: ipcEntry.level) ?? .info
                return LogEntry(time: time, level: level, message: ipcEntry.msg)
            }
            entries = parsed
        } catch {
            // Silently fail — entries stay as-is
        }
    }

    /// Clear all entries.
    func clearEntries() {
        entries.removeAll()
    }

    /// Toggle a log level filter on/off.
    func toggleLevel(_ level: LogLevel) {
        if activeLevels.contains(level) {
            activeLevels.remove(level)
        } else {
            activeLevels.insert(level)
        }
    }

    /// Check if a specific level is active.
    func isLevelActive(_ level: LogLevel) -> Bool {
        activeLevels.contains(level)
    }

    private func trimEntries() {
        if entries.count > maxEntries {
            entries.removeFirst(entries.count - maxEntries)
        }
    }
}
