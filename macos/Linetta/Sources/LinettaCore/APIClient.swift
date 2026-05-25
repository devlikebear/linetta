import Foundation

public struct APIClient: Sendable {
    public static let engineAddressDefaultsKey = "linetta.engineAddress"

    public static var defaultBaseURL: URL {
        if let value = UserDefaults.standard.string(forKey: engineAddressDefaultsKey),
           let url = URL(string: value) {
            return url
        }
        return URL(string: "http://127.0.0.1:43190")!
    }

    public var baseURL: URL
    public var session: URLSession

    public init(baseURL: URL = APIClient.defaultBaseURL, session: URLSession = .shared) {
        self.baseURL = baseURL
        self.session = session
    }

    public func url(path: String) -> URL {
        let normalized = path.hasPrefix("/") ? String(path.dropFirst()) : path
        return baseURL.appending(path: normalized)
    }

    public func health() async throws -> HealthStatus {
        try await get(path: "/health")
    }

    public func listWorks() async throws -> [Work] {
        try await get(path: "/api/works")
    }

    public func createWork(_ request: CreateWorkRequest) async throws -> Work {
        try await send(path: "/api/works", method: "POST", body: request)
    }

    func workStatsURL(workID: String) -> URL {
        url(path: "/api/works/\(workID)/stats")
    }

    public func workStats(workID: String) async throws -> WorkStats {
        var request = URLRequest(url: workStatsURL(workID: workID))
        request.httpMethod = "GET"
        return try await perform(request)
    }

    public func listEpisodes(workID: String) async throws -> [Episode] {
        try await get(path: "/api/works/\(workID)/episodes")
    }

    public func createEpisode(workID: String, request: CreateEpisodeRequest) async throws -> Episode {
        try await send(path: "/api/works/\(workID)/episodes", method: "POST", body: request)
    }

    public func updateEpisodeStatus(workID: String, episodeID: String, status: EpisodeStatus) async throws -> Episode {
        try await send(
            path: "/api/works/\(workID)/episodes/\(episodeID)/status",
            method: "PATCH",
            body: UpdateEpisodeStatusRequest(status: status)
        )
    }

    public func getBlueprint(workID: String, episodeID: String) async throws -> EpisodeBlueprint {
        try await get(path: "/api/works/\(workID)/episodes/\(episodeID)/blueprint")
    }

    public func saveBlueprint(workID: String, episodeID: String, request: SaveBlueprintRequest) async throws -> EpisodeBlueprint {
        try await send(path: "/api/works/\(workID)/episodes/\(episodeID)/blueprint", method: "PUT", body: request)
    }

    public func listEpisodeVersions(workID: String, episodeID: String) async throws -> [EpisodeVersion] {
        try await get(path: "/api/works/\(workID)/episodes/\(episodeID)/versions")
    }

    public func createEpisodeVersion(workID: String, episodeID: String, request: CreateEpisodeVersionRequest) async throws -> EpisodeVersion {
        try await send(path: "/api/works/\(workID)/episodes/\(episodeID)/versions", method: "POST", body: request)
    }

    func exportWorkMarkdownURL(workID: String) -> URL {
        url(path: "/api/works/\(workID)/export/markdown")
    }

    func exportEpisodeTextURL(workID: String, episodeID: String) -> URL {
        url(path: "/api/works/\(workID)/episodes/\(episodeID)/export/txt")
    }

    public func exportWorkMarkdown(workID: String) async throws -> String {
        var request = URLRequest(url: exportWorkMarkdownURL(workID: workID))
        request.httpMethod = "GET"
        return try await performText(request)
    }

    public func exportEpisodeText(workID: String, episodeID: String) async throws -> String {
        var request = URLRequest(url: exportEpisodeTextURL(workID: workID, episodeID: episodeID))
        request.httpMethod = "GET"
        return try await performText(request)
    }

    public func runEpisode(workID: String, episodeID: String, request: RunEpisodeRequest = RunEpisodeRequest()) async throws -> EpisodeRunResult {
        try await send(path: "/api/works/\(workID)/episodes/\(episodeID)/runs", method: "POST", body: request)
    }

    public func listRunArtifacts(runID: String) async throws -> [Artifact] {
        try await get(path: "/api/runs/\(runID)/artifacts")
    }

    public func listRunEvents(runID: String) async throws -> [RunEvent] {
        try await get(path: "/api/runs/\(runID)/events")
    }

    public func listMemory(
        workID: String,
        kind: MemoryKind? = nil,
        status: MemoryStatus? = nil,
        query: String? = nil
    ) async throws -> [MemoryItem] {
        let path: String
        var queryItems: [URLQueryItem] = []
        if let query, !query.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            path = "/api/works/\(workID)/memory/search"
            queryItems.append(URLQueryItem(name: "q", value: query))
        } else {
            path = "/api/works/\(workID)/memory"
            if let kind {
                queryItems.append(URLQueryItem(name: "kind", value: kind.rawValue))
            }
            if let status {
                queryItems.append(URLQueryItem(name: "status", value: status.rawValue))
            }
        }
        var request = URLRequest(url: url(path: path, queryItems: queryItems))
        request.httpMethod = "GET"
        return try await perform(request)
    }

    public func createMemory(workID: String, request: CreateMemoryRequest) async throws -> MemoryItem {
        try await send(path: "/api/works/\(workID)/memory", method: "POST", body: request)
    }

    public func updateMemory(workID: String, itemID: String, request: UpdateMemoryRequest) async throws -> MemoryItem {
        try await send(path: "/api/works/\(workID)/memory/\(itemID)", method: "PATCH", body: request)
    }

    public func archiveMemory(workID: String, itemID: String, request: ArchiveMemoryRequest) async throws -> MemoryItem {
        try await send(path: "/api/works/\(workID)/memory/\(itemID)/archive", method: "POST", body: request)
    }

    public func listMemoryDecisions(workID: String) async throws -> [MemoryDecision] {
        try await get(path: "/api/works/\(workID)/memory/decisions")
    }

    public func listProposals(workID: String, status: ProposalStatus? = nil) async throws -> [CanonProposal] {
        var queryItems: [URLQueryItem] = []
        if let status {
            queryItems.append(URLQueryItem(name: "status", value: status.rawValue))
        }
        var request = URLRequest(url: url(path: "/api/works/\(workID)/proposals", queryItems: queryItems))
        request.httpMethod = "GET"
        return try await perform(request)
    }

    public func approveProposal(proposalID: String, actor: String = "human") async throws -> CanonProposal {
        try await send(path: "/api/proposals/\(proposalID)/approve", method: "POST", body: ProposalDecisionRequest(actor: actor))
    }

    public func rejectProposal(proposalID: String, actor: String = "human") async throws -> CanonProposal {
        try await send(path: "/api/proposals/\(proposalID)/reject", method: "POST", body: ProposalDecisionRequest(actor: actor))
    }

    public func deferProposal(proposalID: String, actor: String = "human") async throws -> CanonProposal {
        try await send(path: "/api/proposals/\(proposalID)/defer", method: "POST", body: ProposalDecisionRequest(actor: actor))
    }

    public func listContinuityIssues(workID: String, episodeID: String) async throws -> [ContinuityIssue] {
        try await get(path: "/api/works/\(workID)/episodes/\(episodeID)/continuity")
    }

    public func updateContinuityIssue(issueID: String, status: ContinuityIssueStatus) async throws -> ContinuityIssue {
        try await send(path: "/api/continuity/\(issueID)", method: "PATCH", body: UpdateContinuityIssueRequest(status: status))
    }

    public func libraryInfo() async throws -> LibraryInfo {
        try await get(path: "/api/library/info")
    }

    public func version() async throws -> VersionInfo {
        try await get(path: "/api/version")
    }

    public func libraryBackup(outPath: String) async throws -> LibraryBackupResult {
        try await send(path: "/api/library/backup", method: "POST", body: LibraryBackupRequest(outPath: outPath))
    }

    public func libraryRestore(inPath: String, dbOut: String, configOut: String? = nil, force: Bool = false) async throws -> LibraryRestoreResult {
        try await send(
            path: "/api/library/restore",
            method: "POST",
            body: LibraryRestoreRequest(inPath: inPath, dbOut: dbOut, configOut: configOut ?? "", force: force)
        )
    }

    private func get<T: Decodable>(path: String) async throws -> T {
        var request = URLRequest(url: url(path: path))
        request.httpMethod = "GET"
        return try await perform(request)
    }

    private func url(path: String, queryItems: [URLQueryItem]) -> URL {
        guard !queryItems.isEmpty else {
            return url(path: path)
        }
        var components = URLComponents(url: url(path: path), resolvingAgainstBaseURL: false)!
        components.queryItems = queryItems
        return components.url!
    }

    private func send<Body: Encodable, Response: Decodable>(
        path: String,
        method: String,
        body: Body
    ) async throws -> Response {
        var request = URLRequest(url: url(path: path))
        request.httpMethod = method
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try JSONEncoder.linetta.encode(body)
        return try await perform(request)
    }

    private func perform<T: Decodable>(_ request: URLRequest) async throws -> T {
        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse else {
            throw APIError.invalidResponse
        }
        guard (200..<300).contains(http.statusCode) else {
            let message = (try? JSONDecoder.linetta.decode(ErrorResponse.self, from: data).error) ?? "HTTP \(http.statusCode)"
            throw APIError.server(message)
        }
        return try JSONDecoder.linetta.decode(T.self, from: data)
    }

    private func performText(_ request: URLRequest) async throws -> String {
        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse else {
            throw APIError.invalidResponse
        }
        guard (200..<300).contains(http.statusCode) else {
            let message = (try? JSONDecoder.linetta.decode(ErrorResponse.self, from: data).error) ?? "HTTP \(http.statusCode)"
            throw APIError.server(message)
        }
        return String(decoding: data, as: UTF8.self)
    }
}

public enum APIError: Error, LocalizedError, Equatable {
    case invalidResponse
    case server(String)

    public var errorDescription: String? {
        switch self {
        case .invalidResponse:
            return "Invalid response from Linetta engine."
        case .server(let message):
            return message
        }
    }
}

private struct ErrorResponse: Decodable {
    var error: String
}
