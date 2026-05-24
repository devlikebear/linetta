import XCTest
@testable import LinettaCore

final class APIClientTests: XCTestCase {
    func testBuildsURLsRelativeToEngineBaseURL() throws {
        let client = APIClient(baseURL: try XCTUnwrap(URL(string: "http://127.0.0.1:43190")))

        XCTAssertEqual(client.url(path: "/api/works").absoluteString, "http://127.0.0.1:43190/api/works")
        XCTAssertEqual(client.url(path: "health").absoluteString, "http://127.0.0.1:43190/health")
    }

    func testWorkDecodesFromEngineJSON() throws {
        let json = Data("""
        {
          "id": "work_1",
          "title": "Green Harbor",
          "genre": "climate fiction",
          "premise": "A caretaker hears a forgotten machine singing.",
          "status": "active",
          "created_at": "2026-05-24T09:00:00Z",
          "updated_at": "2026-05-24T09:01:00Z"
        }
        """.utf8)

        let work = try JSONDecoder.linetta.decode(Work.self, from: json)

        XCTAssertEqual(work.id, "work_1")
        XCTAssertEqual(work.title, "Green Harbor")
        XCTAssertEqual(work.genre, "climate fiction")
        XCTAssertEqual(work.status, "active")
    }

    func testCreateWorkRequestEncodesExpectedPayload() throws {
        let request = CreateWorkRequest(
            title: "Signal Rain",
            genre: "mystery",
            premise: "A lighthouse keeps answering storms."
        )

        let data = try JSONEncoder.linetta.encode(request)
        let json = try XCTUnwrap(String(data: data, encoding: .utf8))

        XCTAssertTrue(json.contains(#""title":"Signal Rain""#))
        XCTAssertTrue(json.contains(#""genre":"mystery""#))
        XCTAssertTrue(json.contains(#""premise":"A lighthouse keeps answering storms.""#))
    }
}
