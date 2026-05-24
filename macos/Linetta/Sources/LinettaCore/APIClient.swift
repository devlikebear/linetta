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

    private func get<T: Decodable>(path: String) async throws -> T {
        var request = URLRequest(url: url(path: path))
        request.httpMethod = "GET"
        return try await perform(request)
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
