import Foundation
#if canImport(Combine)
import Combine
#endif

public enum EngineStatus: Sendable, Equatable {
    case stopped
    case starting
    case healthy
    case failed(String)
    case external

    public var isHealthy: Bool {
        switch self {
        case .healthy, .external: return true
        default: return false
        }
    }
}

public enum EngineLaunchError: Error, LocalizedError {
    case binaryNotFound
    case alreadyRunning
    case readyTimeout(String)
    case readyParseFailed(String)
    case spawnFailed(String)

    public var errorDescription: String? {
        switch self {
        case .binaryNotFound:
            return "Linetta engine binary not found. Set LINETTA_BIN or place bin/linetta in the project."
        case .alreadyRunning:
            return "Engine is already running."
        case .readyTimeout(let msg):
            return "Timed out waiting for engine ready line: \(msg)"
        case .readyParseFailed(let line):
            return "Engine emitted an unexpected ready line: \(line)"
        case .spawnFailed(let msg):
            return "Failed to start engine: \(msg)"
        }
    }
}

@MainActor
public final class EngineController: ObservableObject {
    @Published public private(set) var status: EngineStatus = .stopped
    @Published public private(set) var address: URL?
    @Published public private(set) var pid: Int32?
    @Published public private(set) var recentLog: [String] = []

    public var binaryOverride: String?
    public var readyTimeout: TimeInterval = 10.0

    private var process: Process?
    private var stdoutPipe: Pipe?
    private var stderrPipe: Pipe?
    private var lastBinaryPath: URL?
    private var lastDBPath: URL?
    private let logCapacity = 200

    public init() {}

    public func startEmbedded(binaryPath: URL, dbPath: URL) async throws {
        if process != nil {
            throw EngineLaunchError.alreadyRunning
        }
        guard FileManager.default.isExecutableFile(atPath: binaryPath.path) else {
            throw EngineLaunchError.binaryNotFound
        }

        self.lastBinaryPath = binaryPath
        self.lastDBPath = dbPath
        status = .starting
        recentLog.removeAll(keepingCapacity: true)

        let proc = Process()
        proc.executableURL = binaryPath
        proc.arguments = ["serve", "--addr", "127.0.0.1:0", "--db", dbPath.path]
        proc.environment = ProcessInfo.processInfo.environment
        if let home = ProcessInfo.processInfo.environment["HOME"] {
            proc.currentDirectoryURL = URL(fileURLWithPath: home)
        }

        let outPipe = Pipe()
        let errPipe = Pipe()
        proc.standardOutput = outPipe
        proc.standardError = errPipe

        let readyStream: AsyncStream<String>
        var readyContinuation: AsyncStream<String>.Continuation!
        readyStream = AsyncStream { cont in readyContinuation = cont }

        attachReader(outPipe.fileHandleForReading, label: "stdout", readyContinuation: readyContinuation)
        attachReader(errPipe.fileHandleForReading, label: "stderr", readyContinuation: nil)

        proc.terminationHandler = { [weak self] terminated in
            Task { @MainActor in
                guard let self else { return }
                let reason = "exited code=\(terminated.terminationStatus) reason=\(terminated.terminationReason.rawValue)"
                self.appendLog("[engine] " + reason)
                if case .external = self.status { return }
                self.process = nil
                self.address = nil
                self.pid = nil
                if terminated.terminationStatus != 0 {
                    self.status = .failed(reason)
                } else if case .starting = self.status {
                    self.status = .failed(reason)
                } else {
                    self.status = .stopped
                }
            }
        }

        do {
            try proc.run()
        } catch {
            status = .failed(error.localizedDescription)
            throw EngineLaunchError.spawnFailed(error.localizedDescription)
        }
        self.process = proc
        self.stdoutPipe = outPipe
        self.stderrPipe = errPipe
        self.pid = proc.processIdentifier

        do {
            let readyLine = try await waitForReady(stream: readyStream, timeout: readyTimeout)
            guard let parsed = Self.parseReadyLine(readyLine) else {
                throw EngineLaunchError.readyParseFailed(readyLine)
            }
            self.address = parsed.address
            self.pid = parsed.pid
            self.status = .healthy
        } catch {
            await stop()
            throw error
        }
    }

    /// Stops the running embedded engine and starts a new one with the most
    /// recent binary/db path. Throws if the engine is not currently embedded
    /// or if it has never been started.
    public func restart() async throws {
        guard let bin = lastBinaryPath, let db = lastDBPath else {
            throw EngineLaunchError.binaryNotFound
        }
        if case .external = status {
            throw EngineLaunchError.alreadyRunning
        }
        await stop()
        try await startEmbedded(binaryPath: bin, dbPath: db)
    }

    public func attachExternal(address: URL) {
        process = nil
        stdoutPipe = nil
        stderrPipe = nil
        pid = nil
        self.address = address
        self.status = .external
    }

    public func stop() async {
        guard let proc = process else {
            if case .external = status {
                status = .stopped
                address = nil
            }
            return
        }
        process = nil
        if proc.isRunning {
            proc.terminate()
            let deadline = Date().addingTimeInterval(1.5)
            while proc.isRunning && Date() < deadline {
                try? await Task.sleep(nanoseconds: 50_000_000)
            }
            if proc.isRunning {
                kill(proc.processIdentifier, SIGKILL)
            }
        }
        address = nil
        pid = nil
        status = .stopped
    }

    /// Synchronous variant for use during `applicationWillTerminate`, where
    /// the main thread is blocking and async Tasks on @MainActor would deadlock.
    /// Safe to call only from the main thread.
    public func stopSync() {
        guard let proc = process else {
            if case .external = status {
                status = .stopped
                address = nil
            }
            return
        }
        process = nil
        if proc.isRunning {
            proc.terminate()
            let deadline = Date().addingTimeInterval(1.5)
            while proc.isRunning && Date() < deadline {
                Thread.sleep(forTimeInterval: 0.05)
            }
            if proc.isRunning {
                kill(proc.processIdentifier, SIGKILL)
                // Brief wait so the kernel reaps before we return.
                let killDeadline = Date().addingTimeInterval(0.5)
                while proc.isRunning && Date() < killDeadline {
                    Thread.sleep(forTimeInterval: 0.02)
                }
            }
        }
        address = nil
        pid = nil
        status = .stopped
    }

    public nonisolated static func parseReadyLine(_ line: String) -> (address: URL, pid: Int32)? {
        let trimmed = line.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.hasPrefix("LINETTA_READY ") else { return nil }
        var hostPort: String?
        var pidValue: Int32?
        for token in trimmed.split(separator: " ").dropFirst() {
            let parts = token.split(separator: "=", maxSplits: 1)
            guard parts.count == 2 else { continue }
            switch parts[0] {
            case "addr": hostPort = String(parts[1])
            case "pid": pidValue = Int32(parts[1])
            default: continue
            }
        }
        guard let hostPort, let pidValue, let url = URL(string: "http://" + hostPort) else { return nil }
        return (url, pidValue)
    }

    public nonisolated static func resolveBinaryPath(override: String?) -> URL? {
        let fm = FileManager.default
        var candidates: [String] = []
        if let override, !override.isEmpty { candidates.append(override) }
        if let env = ProcessInfo.processInfo.environment["LINETTA_BIN"], !env.isEmpty {
            candidates.append(env)
        }
        let bundleDir = Bundle.main.bundleURL.deletingLastPathComponent()
        candidates.append(bundleDir.appendingPathComponent("bin/linetta").path)
        let cwd = fm.currentDirectoryPath
        candidates.append(cwd + "/bin/linetta")
        candidates.append(cwd + "/../../bin/linetta")

        for path in candidates {
            let resolved = (path as NSString).standardizingPath
            if fm.isExecutableFile(atPath: resolved) {
                return URL(fileURLWithPath: resolved)
            }
        }
        if let found = which("linetta") {
            return found
        }
        return nil
    }

    private nonisolated static func which(_ name: String) -> URL? {
        guard let pathEnv = ProcessInfo.processInfo.environment["PATH"] else { return nil }
        let fm = FileManager.default
        for dir in pathEnv.split(separator: ":") {
            let candidate = String(dir) + "/" + name
            if fm.isExecutableFile(atPath: candidate) {
                return URL(fileURLWithPath: candidate)
            }
        }
        return nil
    }

    private func attachReader(_ handle: FileHandle, label: String, readyContinuation: AsyncStream<String>.Continuation?) {
        let buffer = LineBuffer()
        handle.readabilityHandler = { [weak self] h in
            let chunk = h.availableData
            if chunk.isEmpty {
                h.readabilityHandler = nil
                if let tail = buffer.drainTail() {
                    Task { @MainActor [weak self] in self?.appendLog("[\(label)] " + tail) }
                }
                readyContinuation?.finish()
                return
            }
            for line in buffer.append(chunk) {
                Task { @MainActor [weak self] in self?.appendLog("[\(label)] " + line) }
                if let readyContinuation, line.hasPrefix("LINETTA_READY ") {
                    readyContinuation.yield(line)
                }
            }
        }
    }

    private func waitForReady(stream: AsyncStream<String>, timeout: TimeInterval) async throws -> String {
        try await withThrowingTaskGroup(of: String?.self) { group in
            group.addTask {
                for await line in stream {
                    if line.hasPrefix("LINETTA_READY ") {
                        return line
                    }
                }
                return nil
            }
            group.addTask {
                try await Task.sleep(nanoseconds: UInt64(timeout * 1_000_000_000))
                return nil
            }
            defer { group.cancelAll() }
            if let first = try await group.next(), let line = first {
                return line
            }
            throw EngineLaunchError.readyTimeout("no LINETTA_READY line within \(timeout)s")
        }
    }

    private final class LineBuffer: @unchecked Sendable {
        private var data = Data()
        private let lock = NSLock()

        func append(_ chunk: Data) -> [String] {
            lock.lock()
            defer { lock.unlock() }
            data.append(chunk)
            var lines: [String] = []
            while let newlineIdx = data.firstIndex(of: 0x0A) {
                let lineData = data.subdata(in: data.startIndex..<newlineIdx)
                data.removeSubrange(data.startIndex...newlineIdx)
                guard let text = String(data: lineData, encoding: .utf8) else { continue }
                lines.append(text.trimmingCharacters(in: CharacterSet(charactersIn: "\r")))
            }
            return lines
        }

        func drainTail() -> String? {
            lock.lock()
            defer { lock.unlock() }
            guard !data.isEmpty, let text = String(data: data, encoding: .utf8) else {
                data.removeAll()
                return nil
            }
            data.removeAll()
            return text
        }
    }

    private func appendLog(_ entry: String) {
        recentLog.append(entry)
        if recentLog.count > logCapacity {
            recentLog.removeFirst(recentLog.count - logCapacity)
        }
    }
}
