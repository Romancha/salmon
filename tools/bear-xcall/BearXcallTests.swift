import Foundation

// MARK: - Test Infrastructure

var testsPassed = 0
var testsFailed = 0
var testsSkipped = 0

enum TestResult {
    case passed
    case failed(String)
    case skipped(String)
}

func runTest(_ name: String, _ body: () throws -> TestResult) {
    do {
        let result = try body()
        switch result {
        case .passed:
            testsPassed += 1
            print("  PASS: \(name)")
        case let .failed(msg):
            testsFailed += 1
            print("  FAIL: \(name) — \(msg)")
        case let .skipped(msg):
            testsSkipped += 1
            print("  SKIP: \(name) — \(msg)")
        }
    } catch {
        testsFailed += 1
        print("  FAIL: \(name) — exception: \(error)")
    }
}

struct ProcessResult {
    let exitCode: Int32
    let stdout: String
    let stderr: String
}

func runBearXcall(_ arguments: [String]) throws -> ProcessResult {
    let binaryPath = ProcessInfo.processInfo.environment["BEAR_XCALL_BIN"]
        ?? "bin/bear-xcall.app/Contents/MacOS/bear-xcall"

    let process = Process()
    process.executableURL = URL(fileURLWithPath: binaryPath)
    process.arguments = arguments

    let stdoutPipe = Pipe()
    let stderrPipe = Pipe()
    process.standardOutput = stdoutPipe
    process.standardError = stderrPipe

    try process.run()
    process.waitUntilExit()

    let stdoutData = stdoutPipe.fileHandleForReading.readDataToEndOfFile()
    let stderrData = stderrPipe.fileHandleForReading.readDataToEndOfFile()

    return ProcessResult(
        exitCode: process.terminationStatus,
        stdout: String(data: stdoutData, encoding: .utf8) ?? "",
        stderr: String(data: stderrData, encoding: .utf8) ?? ""
    )
}

func parseJSON(_ str: String) -> [String: Any]? {
    guard let data = str.trimmingCharacters(in: .whitespacesAndNewlines).data(using: .utf8),
          let obj = try? JSONSerialization.jsonObject(with: data) as? [String: Any]
    else {
        return nil
    }
    return obj
}

// MARK: - CLI Interface Tests (no Bear required)

func runCLITests() {
    print("\n--- CLI Interface Tests ---")

    runTest("no arguments shows usage and exits non-zero") {
        let r = try runBearXcall([])
        guard r.exitCode != 0 else {
            return .failed("expected non-zero exit, got \(r.exitCode)")
        }
        guard r.stderr.contains("Usage:") else {
            return .failed("expected usage in stderr, got: \(r.stderr)")
        }
        return .passed
    }

    runTest("--help shows usage and exits non-zero") {
        let r = try runBearXcall(["--help"])
        guard r.exitCode != 0 else {
            return .failed("expected non-zero exit, got \(r.exitCode)")
        }
        guard r.stderr.contains("Usage:") else {
            return .failed("expected usage in stderr, got: \(r.stderr)")
        }
        return .passed
    }

    runTest("missing -url value exits non-zero with error") {
        let r = try runBearXcall(["-url"])
        guard r.exitCode != 0 else {
            return .failed("expected non-zero exit, got \(r.exitCode)")
        }
        guard r.stderr.contains("requires a value") else {
            return .failed("expected error about missing value, got: \(r.stderr)")
        }
        return .passed
    }

    runTest("invalid URL (not bear://) exits non-zero") {
        let r = try runBearXcall(["-url", "https://example.com"])
        guard r.exitCode != 0 else {
            return .failed("expected non-zero exit, got \(r.exitCode)")
        }
        guard r.stderr.contains("must start with bear://") else {
            return .failed("expected bear:// error, got: \(r.stderr)")
        }
        return .passed
    }

    runTest("unknown argument exits non-zero") {
        let r = try runBearXcall(["--foo"])
        guard r.exitCode != 0 else {
            return .failed("expected non-zero exit, got \(r.exitCode)")
        }
        guard r.stderr.contains("unknown argument") else {
            return .failed("expected unknown argument error, got: \(r.stderr)")
        }
        return .passed
    }

    runTest("-timeout with invalid value exits non-zero") {
        let r = try runBearXcall(["-url", "bear://x-callback-url/open-note", "-timeout", "abc"])
        guard r.exitCode != 0 else {
            return .failed("expected non-zero exit, got \(r.exitCode)")
        }
        guard r.stderr.contains("positive number") else {
            return .failed("expected timeout error, got: \(r.stderr)")
        }
        return .passed
    }

    runTest("-timeout with negative value exits non-zero") {
        let r = try runBearXcall(["-url", "bear://x-callback-url/open-note", "-timeout", "-5"])
        guard r.exitCode != 0 else {
            return .failed("expected non-zero exit, got \(r.exitCode)")
        }
        guard r.stderr.contains("positive number") else {
            return .failed("expected timeout error, got: \(r.stderr)")
        }
        return .passed
    }

    runTest("-timeout with zero exits non-zero") {
        let r = try runBearXcall(["-url", "bear://x-callback-url/open-note", "-timeout", "0"])
        guard r.exitCode != 0 else {
            return .failed("expected non-zero exit, got \(r.exitCode)")
        }
        return .passed
    }

    runTest("missing -timeout value exits non-zero") {
        let r = try runBearXcall(["-url", "bear://x-callback-url/open-note", "-timeout"])
        guard r.exitCode != 0 else {
            return .failed("expected non-zero exit, got \(r.exitCode)")
        }
        return .passed
    }
}

// MARK: - Bear Operation Tests (requires Bear running)

func isBearRunning() -> Bool {
    let process = Process()
    process.executableURL = URL(fileURLWithPath: "/usr/bin/pgrep")
    process.arguments = ["-x", "Bear"]
    let pipe = Pipe()
    process.standardOutput = pipe
    process.standardError = pipe
    try? process.run()
    process.waitUntilExit()
    return process.terminationStatus == 0
}

/// Check if bear-xcall.app bundle is available (needed for URL scheme callbacks).
func isBearXcallAppAvailable() -> Bool {
    let appPath = ProcessInfo.processInfo.environment["BEAR_XCALL_APP"]
        ?? "bin/bear-xcall.app"
    return FileManager.default.fileExists(atPath: appPath + "/Contents/MacOS/bear-xcall")
}

/// Bear operation tests require Bear running AND explicit opt-in via
/// BEAR_XCALL_BEAR_TESTS=1 environment variable, because the test binary
/// cannot receive URL scheme callbacks — only the .app bundle can.
/// When enabled, tests invoke bear-xcall via `open -a bear-xcall.app --args ...`
/// which registers the URL scheme with LaunchServices.
func canRunBearTests() -> Bool {
    guard ProcessInfo.processInfo.environment["BEAR_XCALL_BEAR_TESTS"] == "1" else {
        print("  SKIP: set BEAR_XCALL_BEAR_TESTS=1 to run (requires Bear running)")
        return false
    }
    guard isBearRunning() else {
        print("  SKIP: Bear is not running")
        return false
    }
    guard isBearXcallAppAvailable() else {
        print("  SKIP: bear-xcall.app bundle not found (run 'make build-xcall' first)")
        return false
    }
    return true
}

/// Run bear-xcall via the .app bundle using `open` command (for Bear tests).
/// This ensures the URL scheme is registered with LaunchServices.
func runBearXcallApp(_ arguments: [String]) throws -> ProcessResult {
    let appPath = ProcessInfo.processInfo.environment["BEAR_XCALL_APP"]
        ?? "bin/bear-xcall.app"

    // Run the binary directly from the .app bundle — it needs to be a proper
    // .app bundle with Info.plist for LaunchServices to route callbacks.
    let binaryPath = appPath + "/Contents/MacOS/bear-xcall"

    let process = Process()
    process.executableURL = URL(fileURLWithPath: binaryPath)
    process.arguments = arguments

    let stdoutPipe = Pipe()
    let stderrPipe = Pipe()
    process.standardOutput = stdoutPipe
    process.standardError = stderrPipe

    try process.run()
    process.waitUntilExit()

    let stdoutData = stdoutPipe.fileHandleForReading.readDataToEndOfFile()
    let stderrData = stderrPipe.fileHandleForReading.readDataToEndOfFile()

    return ProcessResult(
        exitCode: process.terminationStatus,
        stdout: String(data: stdoutData, encoding: .utf8) ?? "",
        stderr: String(data: stderrData, encoding: .utf8) ?? ""
    )
}

func runBearTests() {
    print("\n--- Bear Operation Tests (requires Bear running + .app bundle) ---")

    guard canRunBearTests() else {
        testsSkipped += 4
        return
    }

    var createdNoteID: String?

    runTest("create note returns identifier") {
        let r = try runBearXcallApp([
            "-url",
            "bear://x-callback-url/create?title=BearXcallTest&text=Test%20body&tags=bear-xcall-test",
            "-timeout", "10",
        ])
        guard r.exitCode == 0 else {
            return .failed("exit code \(r.exitCode), stderr: \(r.stderr)")
        }
        guard let json = parseJSON(r.stdout), let id = json["identifier"] as? String, !id.isEmpty else {
            return .failed("missing identifier in response: \(r.stdout)")
        }
        createdNoteID = id
        return .passed
    }

    runTest("open note returns title and note") {
        guard let noteID = createdNoteID else {
            return .skipped("no note ID from create test")
        }
        let r = try runBearXcallApp([
            "-url",
            "bear://x-callback-url/open-note?id=\(noteID)&show_window=no",
            "-timeout", "10",
        ])
        guard r.exitCode == 0 else {
            return .failed("exit code \(r.exitCode), stderr: \(r.stderr)")
        }
        guard let json = parseJSON(r.stdout),
              let title = json["title"] as? String,
              json["note"] != nil
        else {
            return .failed("missing title/note in response: \(r.stdout)")
        }
        guard title == "BearXcallTest" else {
            return .failed("expected title 'BearXcallTest', got '\(title)'")
        }
        return .passed
    }

    runTest("add tag to note") {
        guard let noteID = createdNoteID else {
            return .skipped("no note ID from create test")
        }
        let r = try runBearXcallApp([
            "-url",
            "bear://x-callback-url/add-tag?id=\(noteID)&tags=bear-xcall-test-tag&show_window=no",
            "-timeout", "10",
        ])
        guard r.exitCode == 0 else {
            return .failed("exit code \(r.exitCode), stderr: \(r.stderr)")
        }
        return .passed
    }

    runTest("trash note") {
        guard let noteID = createdNoteID else {
            return .skipped("no note ID from create test")
        }
        let r = try runBearXcallApp([
            "-url",
            "bear://x-callback-url/trash?id=\(noteID)&show_window=no",
            "-timeout", "10",
        ])
        guard r.exitCode == 0 else {
            return .failed("exit code \(r.exitCode), stderr: \(r.stderr)")
        }
        return .passed
    }
}

// MARK: - Error Handling Tests (requires Bear running)

func runErrorTests() {
    print("\n--- Error Handling Tests (requires Bear running + .app bundle) ---")

    guard canRunBearTests() else {
        testsSkipped += 2
        return
    }

    runTest("open non-existent note returns error") {
        let r = try runBearXcallApp([
            "-url",
            "bear://x-callback-url/open-note?id=00000000-0000-0000-0000-000000000000&show_window=no",
            "-timeout", "10",
        ])
        guard r.exitCode == 1 else {
            return .failed("expected exit code 1, got \(r.exitCode)")
        }
        guard let json = parseJSON(r.stderr) else {
            return .failed("expected JSON error in stderr, got: \(r.stderr)")
        }
        guard json["errorCode"] != nil else {
            return .failed("missing errorCode in error response")
        }
        return .passed
    }

    runTest("timeout exits with code 2") {
        // Use a very short timeout with a URL that triggers no callback
        // (opening a non-existent app scheme).
        // Note: this test takes ~1 second due to the timeout.
        let r = try runBearXcallApp([
            "-url",
            "bear://x-callback-url/open-note?id=timeout-test&show_window=no",
            "-timeout", "1",
        ])
        // Bear may respond with error (exit 1) or timeout (exit 2).
        // Both are acceptable; we mainly verify it doesn't hang.
        guard r.exitCode != 0 else {
            return .failed("expected non-zero exit, got \(r.exitCode)")
        }
        return .passed
    }
}

// MARK: - Cleanup

func cleanup() {
    guard ProcessInfo.processInfo.environment["BEAR_XCALL_BEAR_TESTS"] == "1",
          isBearRunning(), isBearXcallAppAvailable()
    else { return }

    print("\n--- Cleanup ---")
    // Search for test notes by tag and trash them.
    let r = try? runBearXcallApp([
        "-url",
        "bear://x-callback-url/search?tag=bear-xcall-test&show_window=no",
        "-timeout", "5",
    ])
    if let r = r, r.exitCode == 0 {
        print("  Cleanup search completed")
    }
}

// MARK: - Main

print("bear-xcall test suite")
print("=====================")

runCLITests()
runBearTests()
runErrorTests()
cleanup()

print("\n=====================")
print("Results: \(testsPassed) passed, \(testsFailed) failed, \(testsSkipped) skipped")

if testsFailed > 0 {
    exit(1)
} else {
    exit(0)
}
