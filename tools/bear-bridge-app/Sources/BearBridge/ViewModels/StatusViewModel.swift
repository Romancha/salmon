import Foundation

/// Protocol abstracting IPC operations for testability.
protocol IPCClientProtocol {
    func getStatus() async throws -> IPCStatusResponse
    func syncNow() async throws -> IPCOkResponse
    func getLogs(lines: Int) async throws -> IPCLogsResponse
    func getQueueStatus() async throws -> IPCQueueStatusResponse
    func quit() async throws -> IPCOkResponse
}

extension BridgeIPCClient: IPCClientProtocol {}

/// View model bridging BridgeIPCClient to SwiftUI state.
///
/// Polls the daemon for status at a configurable interval and exposes
/// @Published properties for the menu bar UI.
@MainActor
final class StatusViewModel: ObservableObject {

    @Published var syncStatus: SyncStatus = .idle
    @Published var lastSyncTime: Date?
    @Published var lastError: String?
    @Published var stats: SyncStats = SyncStats()
    @Published var isSyncing: Bool = false
    @Published var bridgeConnected: Bool = false
    @Published var queueItems: [IPCQueueStatusItem] = []

    private let ipcClient: IPCClientProtocol
    private let pollInterval: TimeInterval
    private var pollTask: Task<Void, Never>?
    private var notificationService: NotificationServiceProtocol?

    var lastSyncDescription: String {
        guard let lastSync = lastSyncTime else {
            return "Never"
        }
        let formatter = RelativeDateTimeFormatter()
        formatter.unitsStyle = .full
        return formatter.localizedString(for: lastSync, relativeTo: Date())
    }

    init(
        ipcClient: IPCClientProtocol,
        pollInterval: TimeInterval = 5,
        notificationService: NotificationServiceProtocol? = nil
    ) {
        self.ipcClient = ipcClient
        self.pollInterval = pollInterval
        self.notificationService = notificationService
    }

    /// Start polling the daemon for status updates.
    func startPolling() {
        stopPolling()
        pollTask = Task { [weak self] in
            guard let self else { return }
            while !Task.isCancelled {
                await self.refreshStatus()
                try? await Task.sleep(nanoseconds: UInt64(self.pollInterval * 1_000_000_000))
            }
        }
    }

    /// Stop polling.
    func stopPolling() {
        pollTask?.cancel()
        pollTask = nil
    }

    /// Fetch status once from the daemon.
    func refreshStatus() async {
        do {
            let response = try await ipcClient.getStatus()
            applyStatus(response)
            bridgeConnected = true
            // Fetch queue status (non-critical — don't fail the whole refresh).
            if let queueResponse = try? await ipcClient.getQueueStatus() {
                queueItems = queueResponse.items
            }
        } catch {
            bridgeConnected = false
        }
    }

    /// Trigger an immediate sync via IPC.
    func syncNow() async {
        guard !isSyncing else { return }
        isSyncing = true
        syncStatus = .syncing
        do {
            _ = try await ipcClient.syncNow()
            // After triggering, poll immediately to get updated state
            try? await Task.sleep(nanoseconds: 500_000_000)
            await refreshStatus()
        } catch {
            let errorMsg = error.localizedDescription
            lastError = errorMsg
            syncStatus = .error
            notificationService?.showSyncError(errorMsg)
        }
        isSyncing = false
    }

    /// Handle a status event from the bridge stdout stream (real-time updates).
    func handleStatusEvent(_ event: StatusEvent) {
        switch event.event {
        case .syncStart:
            syncStatus = .syncing
        case .syncComplete:
            syncStatus = .idle
            lastSyncTime = event.time
            if let notes = event.notesSynced, let tags = event.tagsSynced {
                stats = SyncStats(
                    notesCount: notes,
                    tagsCount: tags,
                    queueCount: event.queueItems ?? 0,
                    lastDurationMs: event.durationMs ?? 0
                )
            }
            lastError = nil
        case .syncError:
            syncStatus = .error
            if let error = event.error {
                lastError = error
                notificationService?.showSyncError(error)
            }
        case .syncProgress:
            break
        }
    }

    // MARK: - Private

    private func applyStatus(_ response: IPCStatusResponse) {
        syncStatus = SyncStatus(rawValue: response.state) ?? .idle
        if !response.lastSync.isEmpty, let date = ISO8601DateFormatter().date(from: response.lastSync) {
            lastSyncTime = date
        }
        let newError = response.lastError.isEmpty ? nil : response.lastError
        if let errorMsg = newError, errorMsg != lastError {
            notificationService?.showSyncError(errorMsg)
        }
        lastError = newError
        stats = SyncStats(
            notesCount: response.stats.notesSynced,
            tagsCount: response.stats.tagsSynced,
            queueCount: response.stats.queueProcessed,
            lastDurationMs: Int(response.stats.lastDurationMs)
        )
    }
}
