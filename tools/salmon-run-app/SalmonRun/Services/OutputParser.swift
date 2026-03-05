import Foundation

/// Result of parsing a single stdout line from the bridge.
enum ParsedLine {
    case log(LogEntry)
    case event(StatusEvent)
}

/// Parses newline-delimited JSON from bridge stdout into LogEntry or StatusEvent.
///
/// Bridge stdout contains two types of JSON lines:
/// - slog JSON logs: `{"time":"...","level":"INFO","msg":"...","key":"value"}`
/// - Status events: `{"event":"sync_start","time":"...",...}`
///
/// Lines with an "event" key are parsed as StatusEvent; lines with "msg" key as LogEntry.
final class OutputParser {

    private static let reservedLogKeys: Set<String> = ["time", "level", "msg"]

    func parse(line: String) -> ParsedLine? {
        guard !line.isEmpty,
              let data = line.data(using: .utf8),
              let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any]
        else {
            return nil
        }

        if json["event"] != nil {
            return parseEvent(json)
        }
        if json["msg"] != nil {
            return parseLog(json)
        }
        return nil
    }

    // MARK: - Private

    private func parseLog(_ json: [String: Any]) -> ParsedLine? {
        guard let msg = json["msg"] as? String else { return nil }

        let time: Date
        if let timeStr = json["time"] as? String {
            time = Self.parseISO8601(timeStr) ?? Date()
        } else {
            time = Date()
        }

        let levelStr = (json["level"] as? String) ?? "INFO"
        let level = LogLevel(rawValue: levelStr) ?? .info

        var fields: [String: String] = [:]
        for (key, value) in json where !Self.reservedLogKeys.contains(key) {
            fields[key] = String(describing: value)
        }

        return .log(LogEntry(time: time, level: level, message: msg, fields: fields))
    }

    private func parseEvent(_ json: [String: Any]) -> ParsedLine? {
        guard let eventStr = json["event"] as? String,
              let eventType = StatusEventType(rawValue: eventStr)
        else {
            return nil
        }

        let time: Date
        if let timeStr = json["time"] as? String {
            time = Self.parseISO8601(timeStr) ?? Date()
        } else {
            time = Date()
        }

        return .event(StatusEvent(
            event: eventType,
            time: time,
            phase: json["phase"] as? String,
            notes: json["notes"] as? Int,
            items: json["items"] as? Int,
            durationMs: (json["duration_ms"] as? NSNumber)?.intValue,
            notesSynced: (json["notes_synced"] as? NSNumber)?.intValue,
            tagsSynced: (json["tags_synced"] as? NSNumber)?.intValue,
            queueItems: (json["queue_items"] as? NSNumber)?.intValue,
            error: json["error"] as? String
        ))
    }

    private static let iso8601WithFractional: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return f
    }()

    private static let iso8601WithoutFractional: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime]
        return f
    }()

    /// Parse ISO 8601 date string, handling both with and without fractional seconds.
    static func parseISO8601(_ string: String) -> Date? {
        iso8601WithFractional.date(from: string) ?? iso8601WithoutFractional.date(from: string)
    }
}
