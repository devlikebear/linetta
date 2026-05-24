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

    func testMemoryItemDecodesFromEngineJSON() throws {
        let json = Data("""
        {
          "id": "canon_1",
          "work_id": "work_1",
          "kind": "character",
          "title": "Mira",
          "body": "A tide-garden caretaker.",
          "status": "canon",
          "importance": "high",
          "created_at": "2026-05-24T09:00:00Z",
          "updated_at": "2026-05-24T09:01:00Z"
        }
        """.utf8)

        let item = try JSONDecoder.linetta.decode(MemoryItem.self, from: json)

        XCTAssertEqual(item.id, "canon_1")
        XCTAssertEqual(item.workID, "work_1")
        XCTAssertEqual(item.kind, .character)
        XCTAssertEqual(item.status, .canon)
        XCTAssertEqual(item.importance, .high)
    }

    func testCreateMemoryRequestEncodesExpectedPayload() throws {
        let request = CreateMemoryRequest(
            kind: .worldFact,
            title: "Tide Gardens",
            body: "Public infrastructure that protects the harbor.",
            status: .draft,
            importance: .medium,
            reason: "Initial worldbuilding",
            actor: "human"
        )

        let data = try JSONEncoder.linetta.encode(request)
        let json = try XCTUnwrap(String(data: data, encoding: .utf8))

        XCTAssertTrue(json.contains(#""kind":"world_fact""#))
        XCTAssertTrue(json.contains(#""status":"draft""#))
        XCTAssertTrue(json.contains(#""importance":"medium""#))
    }

    func testEpisodeBlueprintDecodesFromEngineJSON() throws {
        let json = Data("""
        {
          "id": "blueprint_1",
          "work_id": "work_1",
          "episode_id": "episode_1",
          "premise": "Mira hears the harbor singing.",
          "theme": "Memory as infrastructure",
          "situation": "A pump changes rhythm.",
          "must_include": "lullaby clue",
          "must_avoid": "exposition dump",
          "structure_notes": "Open with ritual.",
          "created_at": "2026-05-24T09:00:00Z",
          "updated_at": "2026-05-24T09:01:00Z"
        }
        """.utf8)

        let blueprint = try JSONDecoder.linetta.decode(EpisodeBlueprint.self, from: json)

        XCTAssertEqual(blueprint.id, "blueprint_1")
        XCTAssertEqual(blueprint.workID, "work_1")
        XCTAssertEqual(blueprint.episodeID, "episode_1")
        XCTAssertEqual(blueprint.mustAvoid, "exposition dump")
    }

    func testEpisodeRunResultDecodesArtifactsAndEvents() throws {
        let json = Data("""
        {
          "RunID": "run_1",
          "TesseraRunID": "run_1",
          "Status": "closed",
          "Closure": "normal",
          "Artifacts": [
            {
              "id": "artifact_1",
              "work_id": "work_1",
              "episode_id": "episode_1",
              "run_id": "run_1",
              "kind": "draft",
              "title": "Draft",
              "body": "Draft body",
              "created_at": "2026-05-24T09:00:00Z"
            }
          ],
          "Events": [
            {
              "schema_version": 1,
              "seq": 1,
              "at": "2026-05-24T09:00:00Z",
              "type": "task.succeeded",
              "run_id": "run_1",
              "task_id": "draft",
              "role": "writer"
            }
          ]
        }
        """.utf8)

        let result = try JSONDecoder.linetta.decode(EpisodeRunResult.self, from: json)

        XCTAssertEqual(result.runID, "run_1")
        XCTAssertEqual(result.closure, "normal")
        XCTAssertEqual(result.artifacts.first?.kind, .draft)
        XCTAssertEqual(result.events.first?.type, "task.succeeded")
    }
}
