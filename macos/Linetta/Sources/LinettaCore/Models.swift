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

public enum MemoryKind: String, Codable, CaseIterable, Identifiable, Sendable {
    case character
    case worldFact = "world_fact"
    case timelineEvent = "timeline_event"
    case plotThread = "plot_thread"
    case styleRule = "style_rule"
    case source

    public var id: String { rawValue }

    public var label: String {
        switch self {
        case .character: "Characters"
        case .worldFact: "World"
        case .timelineEvent: "Timeline"
        case .plotThread: "Threads"
        case .styleRule: "Style"
        case .source: "Sources"
        }
    }
}

public enum MemoryStatus: String, Codable, CaseIterable, Identifiable, Sendable {
    case draft
    case canon
    case archived

    public var id: String { rawValue }
}

public enum MemoryImportance: String, Codable, CaseIterable, Identifiable, Sendable {
    case low
    case medium
    case high

    public var id: String { rawValue }
}

public struct MemoryItem: Codable, Equatable, Identifiable, Hashable, Sendable {
    public var id: String
    public var workID: String
    public var kind: MemoryKind
    public var title: String
    public var body: String
    public var status: MemoryStatus
    public var importance: MemoryImportance
    public var createdAt: String
    public var updatedAt: String

    enum CodingKeys: String, CodingKey {
        case id
        case workID = "work_id"
        case kind
        case title
        case body
        case status
        case importance
        case createdAt = "created_at"
        case updatedAt = "updated_at"
    }
}

public struct CreateMemoryRequest: Codable, Equatable, Sendable {
    public var kind: MemoryKind
    public var title: String
    public var body: String
    public var status: MemoryStatus
    public var importance: MemoryImportance
    public var reason: String
    public var actor: String

    public init(
        kind: MemoryKind,
        title: String,
        body: String = "",
        status: MemoryStatus = .draft,
        importance: MemoryImportance = .medium,
        reason: String = "",
        actor: String = "human"
    ) {
        self.kind = kind
        self.title = title
        self.body = body
        self.status = status
        self.importance = importance
        self.reason = reason
        self.actor = actor
    }
}

public struct UpdateMemoryRequest: Codable, Equatable, Sendable {
    public var title: String
    public var body: String
    public var status: MemoryStatus
    public var importance: MemoryImportance
    public var reason: String
    public var actor: String

    public init(
        title: String,
        body: String,
        status: MemoryStatus,
        importance: MemoryImportance,
        reason: String = "",
        actor: String = "human"
    ) {
        self.title = title
        self.body = body
        self.status = status
        self.importance = importance
        self.reason = reason
        self.actor = actor
    }
}

public struct ArchiveMemoryRequest: Codable, Equatable, Sendable {
    public var reason: String
    public var actor: String

    public init(reason: String, actor: String = "human") {
        self.reason = reason
        self.actor = actor
    }
}

public struct MemoryDecision: Codable, Equatable, Identifiable, Sendable {
    public var id: String
    public var workID: String
    public var canonItemID: String
    public var decisionType: String
    public var reason: String
    public var actor: String
    public var createdAt: String

    enum CodingKeys: String, CodingKey {
        case id
        case workID = "work_id"
        case canonItemID = "canon_item_id"
        case decisionType = "decision_type"
        case reason
        case actor
        case createdAt = "created_at"
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
