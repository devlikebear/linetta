import Foundation

public struct HealthStatus: Codable, Equatable, Sendable {
    public var ok: Bool
}

public struct Work: Codable, Equatable, Identifiable, Hashable, Sendable {
    public var id: String
    public var title: String
    public var genre: String
    public var premise: String
    public var status: String
    public var createdAt: String
    public var updatedAt: String

    enum CodingKeys: String, CodingKey {
        case id
        case title
        case genre
        case premise
        case status
        case createdAt = "created_at"
        case updatedAt = "updated_at"
    }
}

public struct CreateWorkRequest: Codable, Equatable, Sendable {
    public var title: String
    public var genre: String
    public var premise: String

    public init(title: String, genre: String, premise: String) {
        self.title = title
        self.genre = genre
        self.premise = premise
    }
}

public extension JSONDecoder {
    static var linetta: JSONDecoder {
        JSONDecoder()
    }
}

public extension JSONEncoder {
    static var linetta: JSONEncoder {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys]
        return encoder
    }
}
