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

    func testEpisodeDecodesTypedStatusFromEngineJSON() throws {
        let json = Data("""
        {
          "id": "episode_1",
          "work_id": "work_1",
          "title": "Episode 1",
          "status": "ready",
          "position": 1,
          "created_at": "2026-05-24T09:00:00Z",
          "updated_at": "2026-05-24T09:01:00Z"
        }
        """.utf8)

        let episode = try JSONDecoder.linetta.decode(Episode.self, from: json)

        XCTAssertEqual(episode.status, .ready)
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

    func testEpisodeVersionDecodesFromEngineJSON() throws {
        let json = Data("""
        {
          "id": "version_1",
          "work_id": "work_1",
          "episode_id": "episode_1",
          "source_artifact_id": "artifact_1",
          "body": "Edited manuscript body.",
          "note": "adopt edited draft",
          "created_at": "2026-05-24T09:00:00Z"
        }
        """.utf8)

        let version = try JSONDecoder.linetta.decode(EpisodeVersion.self, from: json)

        XCTAssertEqual(version.id, "version_1")
        XCTAssertEqual(version.sourceArtifactID, "artifact_1")
        XCTAssertEqual(version.body, "Edited manuscript body.")
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

    func testCanonProposalDecodesFromEngineJSON() throws {
        let json = Data("""
        {
          "id": "proposal_1",
          "work_id": "work_1",
          "episode_id": "episode_1",
          "run_id": "run_1",
          "target_item_id": "",
          "change_type": "create",
          "kind": "plot_thread",
          "title": "Episode Thread: Episode 1",
          "before_body": "",
          "after_body": "A proposed canon note.",
          "reason": "Generated by the Canon Keeper.",
          "confidence": 0.72,
          "status": "pending",
          "created_at": "2026-05-24T09:00:00Z"
        }
        """.utf8)

        let proposal = try JSONDecoder.linetta.decode(CanonProposal.self, from: json)

        XCTAssertEqual(proposal.id, "proposal_1")
        XCTAssertEqual(proposal.episodeID, "episode_1")
        XCTAssertEqual(proposal.changeType, .create)
        XCTAssertEqual(proposal.kind, .plotThread)
        XCTAssertEqual(proposal.status, .pending)
        XCTAssertEqual(proposal.confidence, 0.72)
    }

    func testContinuityIssueDecodesFromEngineJSON() throws {
        let json = Data("""
        {
          "id": "issue_1",
          "work_id": "work_1",
          "episode_id": "episode_1",
          "run_id": "run_1",
          "severity": "warning",
          "title": "Review canon acceptance",
          "body": "Confirm before canonizing.",
          "related_item_ids": "canon_1,canon_2",
          "status": "open",
          "created_at": "2026-05-24T09:00:00Z",
          "updated_at": "2026-05-24T09:01:00Z"
        }
        """.utf8)

        let issue = try JSONDecoder.linetta.decode(ContinuityIssue.self, from: json)

        XCTAssertEqual(issue.id, "issue_1")
        XCTAssertEqual(issue.severity, .warning)
        XCTAssertEqual(issue.status, .open)
        XCTAssertEqual(issue.relatedItemIDs, "canon_1,canon_2")
    }

    func testReviewRequestPayloadsEncodeExpectedFields() throws {
        let decision = ProposalDecisionRequest(actor: "human")
        let decisionJSON = try XCTUnwrap(String(data: JSONEncoder.linetta.encode(decision), encoding: .utf8))
        XCTAssertTrue(decisionJSON.contains(#""actor":"human""#))

        let update = UpdateContinuityIssueRequest(status: .resolved)
        let updateJSON = try XCTUnwrap(String(data: JSONEncoder.linetta.encode(update), encoding: .utf8))
        XCTAssertTrue(updateJSON.contains(#""status":"resolved""#))
    }

    func testEpisodeStatusRequestEncodesExpectedPayload() throws {
        let request = UpdateEpisodeStatusRequest(status: .published)

        let json = try XCTUnwrap(String(data: JSONEncoder.linetta.encode(request), encoding: .utf8))

        XCTAssertTrue(json.contains(#""status":"published""#))
    }

    func testCreateEpisodeVersionRequestEncodesExpectedPayload() throws {
        let request = CreateEpisodeVersionRequest(
            sourceArtifactID: "artifact_1",
            body: "Manuscript body",
            note: "adopted artifact"
        )

        let json = try XCTUnwrap(String(data: JSONEncoder.linetta.encode(request), encoding: .utf8))

        XCTAssertTrue(json.contains(#""source_artifact_id":"artifact_1""#))
        XCTAssertTrue(json.contains(#""body":"Manuscript body""#))
        XCTAssertTrue(json.contains(#""note":"adopted artifact""#))
    }
}
