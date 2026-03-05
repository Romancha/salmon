import Foundation

// MARK: - Transport abstraction for testability

/// Abstraction for sending IPC commands over a Unix socket.
/// Each call connects, sends the request, reads the response, and disconnects.
protocol IPCTransport {
    func send(request: Data, to socketPath: String) async throws -> Data
}

// MARK: - Errors

enum IPCClientError: Error, Equatable {
    case socketNotAvailable
    case connectionFailed(String)
    case timeout
    case invalidResponse
    case serverError(String)
}

// MARK: - BridgeIPCClient

/// Client for communicating with the bear-bridge daemon over Unix socket IPC.
///
/// Each command creates a fresh socket connection (matching the Go server's
/// one-request-per-connection model). Auto-reconnect is implicit: if the
/// daemon restarts and re-creates the socket, subsequent calls will connect
/// to the new socket.
final class BridgeIPCClient {

    let socketPath: String
    private let transport: IPCTransport
    private let encoder = JSONEncoder()
    private let decoder = JSONDecoder()

    /// Creates an IPC client for the given socket path.
    /// - Parameters:
    ///   - socketPath: Path to the Unix socket (default: ~/.bear-bridge.sock).
    ///   - transport: Transport layer (injectable for testing).
    init(socketPath: String? = nil, transport: IPCTransport? = nil) {
        self.socketPath = socketPath ?? {
            let home = FileManager.default.homeDirectoryForCurrentUser.path
            return home + "/.bear-bridge.sock"
        }()
        self.transport = transport ?? UnixSocketTransport()
    }

    /// Queries the current bridge status.
    func getStatus() async throws -> IPCStatusResponse {
        let request = IPCRequest(cmd: "status")
        return try await sendCommand(request)
    }

    /// Triggers an immediate sync cycle.
    func syncNow() async throws -> IPCOkResponse {
        let request = IPCRequest(cmd: "sync_now")
        return try await sendCommand(request)
    }

    /// Retrieves the last N log entries from the bridge.
    func getLogs(lines: Int = 50) async throws -> IPCLogsResponse {
        let request = IPCRequest(cmd: "logs", lines: lines)
        return try await sendCommand(request)
    }

    /// Retrieves the current write queue status.
    func getQueueStatus() async throws -> IPCQueueStatusResponse {
        let request = IPCRequest(cmd: "queue_status")
        return try await sendCommand(request)
    }

    /// Requests graceful shutdown of the bridge daemon.
    func quit() async throws -> IPCOkResponse {
        let request = IPCRequest(cmd: "quit")
        return try await sendCommand(request)
    }

    // MARK: - Private

    private func sendCommand<T: Decodable>(_ request: IPCRequest) async throws -> T {
        let requestData = try encoder.encode(request)
        // Append newline delimiter (protocol requirement)
        var payload = requestData
        payload.append(0x0A) // '\n'

        let responseData = try await transport.send(request: payload, to: socketPath)
        return try decoder.decode(T.self, from: responseData)
    }
}

/// Internal request model matching Go `ipc.Request`.
private struct IPCRequest: Encodable {
    let cmd: String
    let lines: Int?

    init(cmd: String, lines: Int? = nil) {
        self.cmd = cmd
        self.lines = lines
    }
}

// MARK: - Unix socket transport (real implementation)

/// Sends IPC commands over a real Unix domain socket using POSIX APIs.
final class UnixSocketTransport: IPCTransport {

    static let defaultTimeout: TimeInterval = 5

    private let timeout: TimeInterval

    init(timeout: TimeInterval = UnixSocketTransport.defaultTimeout) {
        self.timeout = timeout
    }

    func send(request: Data, to socketPath: String) async throws -> Data {
        // Verify socket file exists
        guard FileManager.default.fileExists(atPath: socketPath) else {
            throw IPCClientError.socketNotAvailable
        }

        // Create Unix domain socket
        let fd = Darwin.socket(AF_UNIX, SOCK_STREAM, 0)
        guard fd >= 0 else {
            throw IPCClientError.connectionFailed("socket() failed: \(errno)")
        }

        defer { Darwin.close(fd) }

        // Connect
        var addr = sockaddr_un()
        addr.sun_family = sa_family_t(AF_UNIX)
        let pathBytes = socketPath.utf8CString
        guard pathBytes.count <= MemoryLayout.size(ofValue: addr.sun_path) else {
            throw IPCClientError.connectionFailed("socket path too long")
        }
        withUnsafeMutablePointer(to: &addr.sun_path) { ptr in
            ptr.withMemoryRebound(to: CChar.self, capacity: pathBytes.count) { dest in
                for (i, byte) in pathBytes.enumerated() {
                    dest[i] = byte
                }
            }
        }

        let connectResult = withUnsafePointer(to: &addr) { ptr in
            ptr.withMemoryRebound(to: sockaddr.self, capacity: 1) { sockaddrPtr in
                Darwin.connect(fd, sockaddrPtr, socklen_t(MemoryLayout<sockaddr_un>.size))
            }
        }
        guard connectResult == 0 else {
            throw IPCClientError.connectionFailed("connect() failed: \(errno)")
        }

        // Set timeouts
        var tv = timeval(tv_sec: Int(timeout), tv_usec: 0)
        setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, &tv, socklen_t(MemoryLayout<timeval>.size))
        setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &tv, socklen_t(MemoryLayout<timeval>.size))

        // Send request
        let sent = request.withUnsafeBytes { buf in
            Darwin.send(fd, buf.baseAddress!, buf.count, 0)
        }
        guard sent == request.count else {
            if errno == EAGAIN || errno == EWOULDBLOCK {
                throw IPCClientError.timeout
            }
            throw IPCClientError.connectionFailed("send() failed: \(errno)")
        }

        // Read response (read until newline or EOF)
        var responseData = Data()
        let bufferSize = 4096
        var buffer = [UInt8](repeating: 0, count: bufferSize)
        while true {
            let bytesRead = Darwin.recv(fd, &buffer, bufferSize, 0)
            if bytesRead < 0 {
                if errno == EAGAIN || errno == EWOULDBLOCK {
                    throw IPCClientError.timeout
                }
                throw IPCClientError.connectionFailed("recv() failed: \(errno)")
            }
            if bytesRead == 0 {
                break // EOF
            }
            responseData.append(contentsOf: buffer[0..<bytesRead])
            // Check for newline delimiter
            if buffer[0..<bytesRead].contains(0x0A) {
                break
            }
        }

        // Trim trailing newline
        if responseData.last == 0x0A {
            responseData.removeLast()
        }

        guard !responseData.isEmpty else {
            throw IPCClientError.invalidResponse
        }

        return responseData
    }
}
