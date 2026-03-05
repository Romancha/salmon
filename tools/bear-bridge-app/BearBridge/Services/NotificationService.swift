import Foundation
import UserNotifications

/// Protocol abstracting notification operations for testability.
protocol NotificationServiceProtocol {
    /// Show a notification for a sync error if rate limiting allows.
    func showSyncError(_ message: String)

    /// Whether notifications are currently allowed (user preference + system permission).
    var isEnabled: Bool { get set }
}

/// Thread-safe rate limiter that tracks the last notification time per error message.
final class NotificationRateLimiter {
    private let interval: TimeInterval
    private var lastNotificationTimes: [String: Date] = [:]
    private let lock = NSLock()
    private let dateProvider: () -> Date

    /// - Parameters:
    ///   - interval: Minimum seconds between notifications for the same error.
    ///   - dateProvider: Closure returning current date (injectable for testing).
    init(interval: TimeInterval = 300, dateProvider: @escaping () -> Date = { Date() }) {
        self.interval = interval
        self.dateProvider = dateProvider
    }

    /// Returns true if a notification for this error should be allowed.
    func shouldAllow(error: String) -> Bool {
        lock.lock()
        defer { lock.unlock() }

        guard let lastTime = lastNotificationTimes[error] else {
            return true
        }
        return dateProvider().timeIntervalSince(lastTime) >= interval
    }

    /// Records that a notification was sent for this error.
    func record(error: String) {
        lock.lock()
        defer { lock.unlock() }
        lastNotificationTimes[error] = dateProvider()
    }

    /// Clears all rate limit tracking.
    func reset() {
        lock.lock()
        defer { lock.unlock() }
        lastNotificationTimes.removeAll()
    }
}

/// Manages macOS notifications for sync errors with rate limiting.
///
/// Rate limits notifications to at most one per `rateLimitInterval` for the same error message.
/// Respects the `isEnabled` flag (tied to user preference in Settings).
final class NotificationService: NSObject, NotificationServiceProtocol, UNUserNotificationCenterDelegate {

    static let categoryIdentifier = "SYNC_ERROR"
    static let actionOpenLogs = "OPEN_LOGS"

    var isEnabled: Bool = true
    var onOpenLogViewer: (() -> Void)?

    private let center: UNUserNotificationCenter
    let rateLimiter: NotificationRateLimiter

    /// - Parameters:
    ///   - center: Notification center (defaults to .current()).
    ///   - rateLimitInterval: Minimum seconds between notifications for the same error (default 300s = 5 min).
    init(center: UNUserNotificationCenter = .current(), rateLimitInterval: TimeInterval = 300) {
        self.center = center
        self.rateLimiter = NotificationRateLimiter(interval: rateLimitInterval)
        super.init()
        center.delegate = self
        registerCategory()
        requestAuthorization()
    }

    func showSyncError(_ message: String) {
        guard isEnabled else { return }
        guard rateLimiter.shouldAllow(error: message) else { return }

        let content = UNMutableNotificationContent()
        content.title = "Bear Bridge Sync Error"
        content.body = message
        content.categoryIdentifier = Self.categoryIdentifier
        content.sound = .default

        let request = UNNotificationRequest(
            identifier: "sync-error-\(UUID().uuidString)",
            content: content,
            trigger: nil
        )

        center.add(request)
        rateLimiter.record(error: message)
    }

    // MARK: - UNUserNotificationCenterDelegate

    func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        didReceive response: UNNotificationResponse,
        withCompletionHandler completionHandler: @escaping () -> Void
    ) {
        if response.actionIdentifier == Self.actionOpenLogs
            || response.actionIdentifier == UNNotificationDefaultActionIdentifier {
            DispatchQueue.main.async { [weak self] in
                self?.onOpenLogViewer?()
            }
        }
        completionHandler()
    }

    func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        willPresent notification: UNNotification,
        withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void
    ) {
        completionHandler([.banner, .sound])
    }

    // MARK: - Private

    private func registerCategory() {
        let openLogsAction = UNNotificationAction(
            identifier: Self.actionOpenLogs,
            title: "View Logs",
            options: .foreground
        )
        let category = UNNotificationCategory(
            identifier: Self.categoryIdentifier,
            actions: [openLogsAction],
            intentIdentifiers: []
        )
        center.setNotificationCategories([category])
    }

    private func requestAuthorization() {
        center.requestAuthorization(options: [.alert, .sound]) { _, _ in }
    }
}
