import Foundation

public enum StoragePaths {
    public static var dataDir: URL {
        let home = FileManager.default.homeDirectoryForCurrentUser
        return home.appendingPathComponent(".linetta", isDirectory: true)
    }

    public static var defaultDB: URL {
        dataDir.appendingPathComponent("linetta.db", isDirectory: false)
    }

    @discardableResult
    public static func ensureDataDirectory() throws -> URL {
        let url = dataDir
        try FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
        return url
    }
}
