import Foundation

// MARK: - Process abstraction for testability

/// Handle to a running bridge process.
protocol ProcessHandle: AnyObject {
    var isRunning: Bool { get }
    func terminate()
}

/// Launches a bridge process and returns a handle to control it.
/// Abstracted as a protocol so tests can inject a mock.
protocol ProcessLauncher {
    func launch(
        executableURL: URL,
        arguments: [String],
        environment: [String: String],
        onStdoutLine: @escaping (String) -> Void,
        onStderrLine: @escaping (String) -> Void,
        onTermination: @escaping (Int32) -> Void
    ) throws -> ProcessHandle
}

/// Errors from BridgeProcessManager.
enum BridgeProcessError: Error, Equatable {
    case binaryNotFound
    case alreadyRunning
}

// MARK: - BridgeProcessManager

/// Manages the bear-bridge daemon as a child process.
///
/// Responsibilities:
/// - Locates the bear-bridge binary (in .app bundle or ~/bin/)
/// - Launches it with `--daemon` flag and configured environment
/// - Parses stdout JSON lines into LogEntry and StatusEvent
/// - Restarts on unexpected exit (up to 3 retries)
/// - Terminates cleanly on stop()
final class BridgeProcessManager {

    enum State: Equatable {
        case stopped
        case running
        case restarting(attempt: Int)
    }

    static let maxRetries = 3

    private(set) var state: State = .stopped
    private var processHandle: ProcessHandle?
    private let parser = OutputParser()
    private var retryCount = 0
    private let environmentProvider: () -> [String: String]
    private let launcher: ProcessLauncher
    private let binaryPath: String?

    /// Called for each parsed log line from bridge stdout/stderr.
    var onLogEntry: ((LogEntry) -> Void)?
    /// Called for each parsed status event from bridge stdout.
    var onStatusEvent: ((StatusEvent) -> Void)?
    /// Called when the manager state changes.
    var onStateChange: ((State) -> Void)?

    /// - Parameters:
    ///   - binaryPath: Explicit path to bear-bridge binary. If nil, searches default locations.
    ///   - environmentProvider: Closure returning environment variables. Evaluated at each process launch.
    ///   - launcher: Process launcher (injectable for testing).
    init(binaryPath: String? = nil, environmentProvider: @escaping () -> [String: String] = { [:] }, launcher: ProcessLauncher? = nil) {
        self.binaryPath = binaryPath
        self.environmentProvider = environmentProvider
        self.launcher = launcher ?? SystemProcessLauncher()
    }

    /// Convenience initializer with a static environment dictionary.
    convenience init(binaryPath: String? = nil, environment: [String: String], launcher: ProcessLauncher? = nil) {
        self.init(binaryPath: binaryPath, environmentProvider: { environment }, launcher: launcher)
    }

    /// Start the bridge process.
    func start() throws {
        guard state == .stopped else {
            throw BridgeProcessError.alreadyRunning
        }
        guard let url = resolveBinaryURL() else {
            throw BridgeProcessError.binaryNotFound
        }
        retryCount = 0
        try launchProcess(at: url)
    }

    /// Stop the bridge process and reset retry state.
    func stop() {
        let wasRunning = processHandle?.isRunning ?? false
        state = .stopped
        retryCount = 0
        if wasRunning {
            processHandle?.terminate()
        }
        processHandle = nil
        onStateChange?(.stopped)
    }

    /// Restart the bridge process with fresh environment from the provider.
    func restart() throws {
        stop()
        guard let url = resolveBinaryURL() else {
            throw BridgeProcessError.binaryNotFound
        }
        retryCount = 0
        try launchProcess(at: url)
    }

    /// Resolve the bear-bridge binary URL.
    /// Checks: explicit path > .app bundle > ~/bin/
    func resolveBinaryURL() -> URL? {
        if let explicit = binaryPath {
            let url = URL(fileURLWithPath: explicit)
            if FileManager.default.isExecutableFile(atPath: url.path) {
                return url
            }
            return nil
        }

        // Check inside .app bundle (Contents/MacOS/bear-bridge)
        if let bundleExec = Bundle.main.executableURL {
            let bundled = bundleExec.deletingLastPathComponent().appendingPathComponent("bear-bridge")
            if FileManager.default.isExecutableFile(atPath: bundled.path) {
                return bundled
            }
        }

        // Check ~/bin/bear-bridge
        let homeBin = FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent("bin")
            .appendingPathComponent("bear-bridge")
        if FileManager.default.isExecutableFile(atPath: homeBin.path) {
            return homeBin
        }

        return nil
    }

    // MARK: - Private

    private func launchProcess(at url: URL) throws {
        let handle = try launcher.launch(
            executableURL: url,
            arguments: ["--daemon"],
            environment: environmentProvider(),
            onStdoutLine: { [weak self] line in self?.handleStdoutLine(line) },
            onStderrLine: { [weak self] line in self?.handleStderrLine(line) },
            onTermination: { [weak self] status in self?.handleTermination(status: status) }
        )
        processHandle = handle
        state = .running
        onStateChange?(.running)
    }

    private func handleStdoutLine(_ line: String) {
        guard let parsed = parser.parse(line: line) else { return }
        switch parsed {
        case .log(let entry):
            onLogEntry?(entry)
        case .event(let event):
            onStatusEvent?(event)
        }
    }

    private func handleStderrLine(_ line: String) {
        guard !line.isEmpty else { return }
        let entry = LogEntry(time: Date(), level: .error, message: line)
        onLogEntry?(entry)
    }

    private func handleTermination(status: Int32) {
        processHandle = nil

        // If stopped intentionally, don't restart.
        if state == .stopped {
            return
        }

        // Clean exit — just stop.
        if status == 0 {
            state = .stopped
            onStateChange?(.stopped)
            return
        }

        // Unexpected termination — try to restart.
        retryCount += 1
        if retryCount <= Self.maxRetries {
            state = .restarting(attempt: retryCount)
            onStateChange?(state)

            guard let url = resolveBinaryURL() else {
                state = .stopped
                onStateChange?(.stopped)
                return
            }
            do {
                try launchProcess(at: url)
            } catch {
                state = .stopped
                onStateChange?(.stopped)
            }
        } else {
            state = .stopped
            onStateChange?(.stopped)
        }
    }
}

// MARK: - System process launcher (real implementation)

/// Launches a real Foundation.Process with pipe-based I/O.
final class SystemProcessLauncher: ProcessLauncher {

    func launch(
        executableURL: URL,
        arguments: [String],
        environment: [String: String],
        onStdoutLine: @escaping (String) -> Void,
        onStderrLine: @escaping (String) -> Void,
        onTermination: @escaping (Int32) -> Void
    ) throws -> ProcessHandle {
        let process = Process()
        process.executableURL = executableURL
        process.arguments = arguments
        if !environment.isEmpty {
            var env = ProcessInfo.processInfo.environment
            for (key, value) in environment {
                env[key] = value
            }
            process.environment = env
        }

        let stdoutPipe = Pipe()
        let stderrPipe = Pipe()
        process.standardOutput = stdoutPipe
        process.standardError = stderrPipe

        setupLineReader(pipe: stdoutPipe, onLine: onStdoutLine)
        setupLineReader(pipe: stderrPipe, onLine: onStderrLine)

        process.terminationHandler = { proc in
            stdoutPipe.fileHandleForReading.readabilityHandler = nil
            stderrPipe.fileHandleForReading.readabilityHandler = nil
            let status = proc.terminationStatus
            DispatchQueue.main.async {
                onTermination(status)
            }
        }

        try process.run()
        return SystemProcessHandle(process: process)
    }

    private func setupLineReader(pipe: Pipe, onLine: @escaping (String) -> Void) {
        var buffer = Data()
        let newline = Data([0x0A])
        pipe.fileHandleForReading.readabilityHandler = { handle in
            let data = handle.availableData
            guard !data.isEmpty else { return }
            buffer.append(data)
            while let range = buffer.range(of: newline) {
                let lineData = buffer.subdata(in: buffer.startIndex..<range.lowerBound)
                buffer.removeSubrange(buffer.startIndex...range.lowerBound)
                if let line = String(data: lineData, encoding: .utf8), !line.isEmpty {
                    DispatchQueue.main.async {
                        onLine(line)
                    }
                }
            }
        }
    }
}

/// Handle wrapping a real Foundation.Process.
final class SystemProcessHandle: ProcessHandle {
    private let process: Process

    init(process: Process) {
        self.process = process
    }

    var isRunning: Bool { process.isRunning }

    func terminate() {
        if process.isRunning {
            process.terminate()
        }
    }
}
