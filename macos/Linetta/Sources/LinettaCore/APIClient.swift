import Foundation

public struct APIClient: Sendable {
    public var baseURL: URL
    public var session: URLSession

    public init(baseURL: URL = URL(string: "http://127.0.0.1:43190")!, session: URLSession = .shared) {
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

    public func listEpisodes(workID: String) async throws -> [Episode] {
        try await get(path: "/api/works/\(workID)/episodes")
    }

    public func createEpisode(workID: String, request: CreateEpisodeRequest) async throws -> Episode {
        try await send(path: "/api/works/\(workID)/episodes", method: "POST", body: request)
    }

    public func getBlueprint(workID: String, episodeID: String) async throws -> EpisodeBlueprint {
        try await get(path: "/api/works/\(workID)/episodes/\(episodeID)/blueprint")
    }

    public func saveBlueprint(workID: String, episodeID: String, request: SaveBlueprintRequest) async throws -> EpisodeBlueprint {
        try await send(path: "/api/works/\(workID)/episodes/\(episodeID)/blueprint", method: "PUT", body: request)
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
