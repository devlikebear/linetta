import XCTest
@testable import LinettaCore

final class StoragePathsTests: XCTestCase {
    func testDataDirIsUnderUserHome() {
        let home = FileManager.default.homeDirectoryForCurrentUser.path
        XCTAssertTrue(StoragePaths.dataDir.path.hasPrefix(home))
        XCTAssertEqual(StoragePaths.dataDir.lastPathComponent, ".linetta")
    }

    func testDefaultDBSitsInsideDataDir() {
        XCTAssertEqual(StoragePaths.defaultDB.lastPathComponent, "linetta.db")
        XCTAssertEqual(StoragePaths.defaultDB.deletingLastPathComponent(), StoragePaths.dataDir)
    }
}
