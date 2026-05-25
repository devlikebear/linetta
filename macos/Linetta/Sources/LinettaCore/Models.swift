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

public struct WorkStats: Codable, Equatable, Sendable {
    public var workID: String
    public var episodeCount: Int
    public var readyCount: Int
    public var wordCount: Int
    public var openContinuityIssueCount: Int
    public var pendingCanonProposalCount: Int

    enum CodingKeys: String, CodingKey {
        case workID = "work_id"
        case episodeCount = "episode_count"
        case readyCount = "ready_count"
        case wordCount = "word_count"
        case openContinuityIssueCount = "open_continuity_issue_count"
        case pendingCanonProposalCount = "pending_canon_proposal_count"
    }
}

public enum EpisodeStatus: String, Codable, CaseIterable, Identifiable, Sendable {
    case idea
    case outlined
    case drafting
    case reviewing
    case ready
    case published

    public var id: String { rawValue }

    public var label: String {
        switch self {
        case .idea: "Idea"
        case .outlined: "Outlined"
        case .drafting: "Drafting"
        case .reviewing: "Reviewing"
        case .ready: "Ready"
        case .published: "Published"
        }
    }
}

public struct Episode: Codable, Equatable, Identifiable, Hashable, Sendable {
    public var id: String
    public var workID: String
    public var title: String
    public var status: EpisodeStatus
    public var position: Int
    public var createdAt: String
    public var updatedAt: String

    enum CodingKeys: String, CodingKey {
        case id
        case workID = "work_id"
        case title
        case status
        case position
        case createdAt = "created_at"
        case updatedAt = "updated_at"
    }
}

public struct CreateEpisodeRequest: Codable, Equatable, Sendable {
    public var title: String

    public init(title: String) {
        self.title = title
    }
}

public struct UpdateEpisodeStatusRequest: Codable, Equatable, Sendable {
    public var status: EpisodeStatus

    public init(status: EpisodeStatus) {
        self.status = status
    }
}

public struct EpisodeBlueprint: Codable, Equatable, Identifiable, Sendable {
    public var id: String
    public var workID: String
    public var episodeID: String
    public var premise: String
    public var theme: String
    public var situation: String
    public var mustInclude: String
    public var mustAvoid: String
    public var structureNotes: String
    public var createdAt: String
    public var updatedAt: String

    enum CodingKeys: String, CodingKey {
        case id
        case workID = "work_id"
        case episodeID = "episode_id"
        case premise
        case theme
        case situation
        case mustInclude = "must_include"
        case mustAvoid = "must_avoid"
        case structureNotes = "structure_notes"
        case createdAt = "created_at"
        case updatedAt = "updated_at"
    }
}

public struct EpisodeVersion: Codable, Equatable, Identifiable, Hashable, Sendable {
    public var id: String
    public var workID: String
    public var episodeID: String
    public var sourceArtifactID: String
    public var body: String
    public var note: String
    public var createdAt: String

    enum CodingKeys: String, CodingKey {
        case id
        case workID = "work_id"
        case episodeID = "episode_id"
        case sourceArtifactID = "source_artifact_id"
        case body
        case note
        case createdAt = "created_at"
    }
}

public struct CreateEpisodeVersionRequest: Codable, Equatable, Sendable {
    public var sourceArtifactID: String
    public var body: String
    public var note: String

    public init(sourceArtifactID: String = "", body: String, note: String = "") {
        self.sourceArtifactID = sourceArtifactID
        self.body = body
        self.note = note
    }

    enum CodingKeys: String, CodingKey {
        case sourceArtifactID = "source_artifact_id"
        case body
        case note
    }
}

public struct SaveBlueprintRequest: Codable, Equatable, Sendable {
    public var premise: String
    public var theme: String
    public var situation: String
    public var mustInclude: String
    public var mustAvoid: String
    public var structureNotes: String

    public init(
        premise: String = "",
        theme: String = "",
        situation: String = "",
        mustInclude: String = "",
        mustAvoid: String = "",
        structureNotes: String = ""
    ) {
        self.premise = premise
        self.theme = theme
        self.situation = situation
        self.mustInclude = mustInclude
        self.mustAvoid = mustAvoid
        self.structureNotes = structureNotes
    }

    enum CodingKeys: String, CodingKey {
        case premise
        case theme
        case situation
        case mustInclude = "must_include"
        case mustAvoid = "must_avoid"
        case structureNotes = "structure_notes"
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

public enum ProposalChangeType: String, Codable, CaseIterable, Identifiable, Sendable {
    case create
    case update
    case archive
    case link

    public var id: String { rawValue }
}

public enum ProposalStatus: String, Codable, CaseIterable, Identifiable, Sendable {
    case pending
    case approved
    case rejected
    case deferred

    public var id: String { rawValue }
}

public struct CanonProposal: Codable, Equatable, Identifiable, Hashable, Sendable {
    public var id: String
    public var workID: String
    public var episodeID: String
    public var runID: String
    public var targetItemID: String
    public var changeType: ProposalChangeType
    public var kind: MemoryKind
    public var title: String
    public var beforeBody: String
    public var afterBody: String
    public var reason: String
    public var confidence: Double
    public var status: ProposalStatus
    public var createdAt: String
    public var decidedAt: String?

    enum CodingKeys: String, CodingKey {
        case id
        case workID = "work_id"
        case episodeID = "episode_id"
        case runID = "run_id"
        case targetItemID = "target_item_id"
        case changeType = "change_type"
        case kind
        case title
        case beforeBody = "before_body"
        case afterBody = "after_body"
        case reason
        case confidence
        case status
        case createdAt = "created_at"
        case decidedAt = "decided_at"
    }
}

public struct ProposalDecisionRequest: Codable, Equatable, Sendable {
    public var actor: String

    public init(actor: String = "human") {
        self.actor = actor
    }
}

public enum ContinuityIssueSeverity: String, Codable, CaseIterable, Identifiable, Sendable {
    case info
    case warning
    case blocker

    public var id: String { rawValue }
}

public enum ContinuityIssueStatus: String, Codable, CaseIterable, Identifiable, Sendable {
    case open
    case accepted
    case resolved
    case ignored

    public var id: String { rawValue }
}

public struct ContinuityIssue: Codable, Equatable, Identifiable, Hashable, Sendable {
    public var id: String
    public var workID: String
    public var episodeID: String
    public var runID: String
    public var severity: ContinuityIssueSeverity
    public var title: String
    public var body: String
    public var relatedItemIDs: String
    public var status: ContinuityIssueStatus
    public var createdAt: String
    public var updatedAt: String

    enum CodingKeys: String, CodingKey {
        case id
        case workID = "work_id"
        case episodeID = "episode_id"
        case runID = "run_id"
        case severity
        case title
        case body
        case relatedItemIDs = "related_item_ids"
        case status
        case createdAt = "created_at"
        case updatedAt = "updated_at"
    }
}

public struct UpdateContinuityIssueRequest: Codable, Equatable, Sendable {
    public var status: ContinuityIssueStatus

    public init(status: ContinuityIssueStatus) {
        self.status = status
    }
}

public enum ArtifactKind: String, Codable, CaseIterable, Identifiable, Sendable {
    case museNotes = "muse-notes"
    case plotOutline = "plot-outline"
    case canonReview = "canon-review"
    case researchNotes = "research-notes"
    case draft
    case critique
    case editedDraft = "edited-draft"

    public var id: String { rawValue }
}

public struct Artifact: Codable, Equatable, Identifiable, Hashable, Sendable {
    public var id: String
    public var workID: String
    public var episodeID: String
    public var runID: String
    public var kind: ArtifactKind
    public var title: String
    public var body: String
    public var createdAt: String

    enum CodingKeys: String, CodingKey {
        case id
        case workID = "work_id"
        case episodeID = "episode_id"
        case runID = "run_id"
        case kind
        case title
        case body
        case createdAt = "created_at"
    }
}

public struct RunEvent: Codable, Equatable, Identifiable, Sendable {
    public var schemaVersion: Int
    public var seq: Int
    public var at: String
    public var type: String
    public var runID: String
    public var taskID: String?
    public var role: String?
    public var stage: String?
    public var message: String?

    public var id: Int { seq }

    enum CodingKeys: String, CodingKey {
        case schemaVersion = "schema_version"
        case seq
        case at
        case type
        case runID = "run_id"
        case taskID = "task_id"
        case role
        case stage
        case message
    }
}

public struct EpisodeRunResult: Codable, Equatable, Sendable {
    public var runID: String
    public var tesseraRunID: String
    public var status: String
    public var closure: String
    public var artifacts: [Artifact]
    public var events: [RunEvent]

    public init(runID: String, tesseraRunID: String, status: String, closure: String, artifacts: [Artifact], events: [RunEvent]) {
        self.runID = runID
        self.tesseraRunID = tesseraRunID
        self.status = status
        self.closure = closure
        self.artifacts = artifacts
        self.events = events
    }

    enum CodingKeys: String, CodingKey {
        case runID = "RunID"
        case tesseraRunID = "TesseraRunID"
        case status = "Status"
        case closure = "Closure"
        case artifacts = "Artifacts"
        case events = "Events"
    }
}

public struct RunEpisodeRequest: Codable, Equatable, Sendable {
    public var approvedBy: String

    public init(approvedBy: String = "human") {
        self.approvedBy = approvedBy
    }

    enum CodingKeys: String, CodingKey {
        case approvedBy = "approved_by"
    }
}

public struct AsyncRunStart: Codable, Equatable, Sendable {
    public var runID: String
    enum CodingKeys: String, CodingKey { case runID = "run_id" }
}

public struct VersionInfo: Codable, Equatable, Sendable {
    public var linetta: String
    public var tessera: String
    public var go: String
}

public struct LibraryInfo: Codable, Equatable, Sendable {
    public var dbPath: String
    public var configPath: String

    enum CodingKeys: String, CodingKey {
        case dbPath = "db_path"
        case configPath = "config_path"
    }
}

public struct LibraryBackupRequest: Codable, Equatable, Sendable {
    public var outPath: String
    enum CodingKeys: String, CodingKey { case outPath = "out_path" }
}

public struct LibraryBackupResult: Codable, Equatable, Sendable {
    public var outPath: String
    public var sizeBytes: Int64
    enum CodingKeys: String, CodingKey {
        case outPath = "out_path"
        case sizeBytes = "size_bytes"
    }
}

public struct LibraryRestoreRequest: Codable, Equatable, Sendable {
    public var inPath: String
    public var dbOut: String
    public var configOut: String
    public var force: Bool
    enum CodingKeys: String, CodingKey {
        case inPath = "in_path"
        case dbOut = "db_out"
        case configOut = "config_out"
        case force
    }
}

public struct LibraryRestoreResult: Codable, Equatable, Sendable {
    public var dbPath: String
    public var configPath: String
    enum CodingKeys: String, CodingKey {
        case dbPath = "db_path"
        case configPath = "config_path"
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
