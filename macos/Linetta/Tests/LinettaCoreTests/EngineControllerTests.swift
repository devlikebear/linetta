import XCTest
@testable import LinettaCore

final class EngineControllerTests: XCTestCase {
    func testParseReadyLineExtractsAddressAndPID() {
        let parsed = EngineController.parseReadyLine("LINETTA_READY addr=127.0.0.1:54321 pid=99887")
        XCTAssertNotNil(parsed)
        XCTAssertEqual(parsed?.address.absoluteString, "http://127.0.0.1:54321")
        XCTAssertEqual(parsed?.pid, 99887)
    }

    func testParseReadyLineRejectsWrongPrefix() {
        XCTAssertNil(EngineController.parseReadyLine("hello world"))
        XCTAssertNil(EngineController.parseReadyLine("LINETTA_READY"))
    }

    func testParseReadyLineTrimsWhitespace() {
        let parsed = EngineController.parseReadyLine("  LINETTA_READY addr=127.0.0.1:1 pid=2\n")
        XCTAssertEqual(parsed?.address.absoluteString, "http://127.0.0.1:1")
        XCTAssertEqual(parsed?.pid, 2)
    }

    func testResolveBinaryPathRespectsOverride() throws {
        let dir = NSTemporaryDirectory() + "linetta-resolve-\(UUID().uuidString)"
        try FileManager.default.createDirectory(atPath: dir, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(atPath: dir) }
        let binPath = dir + "/linetta"
        FileManager.default.createFile(atPath: binPath, contents: Data("#!/bin/sh\n".utf8), attributes: [.posixPermissions: 0o755])
        let resolved = EngineController.resolveBinaryPath(override: binPath)
        XCTAssertEqual(resolved?.path, binPath)
    }

    func testResolveBinaryPathHonorsLinettaBinEnv() throws {
        let dir = NSTemporaryDirectory() + "linetta-resolve-\(UUID().uuidString)"
        try FileManager.default.createDirectory(atPath: dir, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(atPath: dir) }
        let binPath = dir + "/linetta"
        FileManager.default.createFile(atPath: binPath, contents: Data("#!/bin/sh\n".utf8), attributes: [.posixPermissions: 0o755])
        setenv("LINETTA_BIN", binPath, 1)
        defer { unsetenv("LINETTA_BIN") }
        let resolved = EngineController.resolveBinaryPath(override: nil)
        XCTAssertEqual(resolved?.path, binPath)
    }

    @MainActor
    func testStartEmbeddedCapturesReadyAddress() async throws {
        let scriptDir = NSTemporaryDirectory() + "linetta-engine-\(UUID().uuidString)"
        try FileManager.default.createDirectory(atPath: scriptDir, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(atPath: scriptDir) }
        let scriptPath = scriptDir + "/linetta"
        let script = """
        #!/bin/sh
        echo "LINETTA_READY addr=127.0.0.1:65432 pid=$$"
        # keep alive so the controller treats it as healthy
        while true; do sleep 1; done
        """
        FileManager.default.createFile(atPath: scriptPath, contents: Data(script.utf8), attributes: [.posixPermissions: 0o755])

        let controller = EngineController()
        controller.readyTimeout = 3.0
        try await controller.startEmbedded(
            binaryPath: URL(fileURLWithPath: scriptPath),
            dbPath: URL(fileURLWithPath: scriptDir + "/dummy.db")
        )
        XCTAssertEqual(controller.status, .healthy)
        XCTAssertEqual(controller.address?.host, "127.0.0.1")
        XCTAssertEqual(controller.address?.port, 65432)
        await controller.stop()
        XCTAssertEqual(controller.status, .stopped)
        XCTAssertNil(controller.address)
    }

    @MainActor
    func testAttachExternalSetsExternalStatus() {
        let controller = EngineController()
        let addr = URL(string: "http://127.0.0.1:43190")!
        controller.attachExternal(address: addr)
        XCTAssertEqual(controller.status, .external)
        XCTAssertEqual(controller.address, addr)
        XCTAssertTrue(controller.status.isHealthy)
    }
}
