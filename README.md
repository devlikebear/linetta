# Linetta

Linetta is a Tessera-powered novel-writing agent app. It turns a writing goal
into a mandate-gated agent run, then produces a small novel packet:

- world notes
- plot outline
- first chapter draft
- editorial review

## What is Tessera?

Tessera is a separate Go library and CLI maintained at
[devlikebear/tessera](https://github.com/devlikebear/tessera). Linetta depends
on that repository as the `github.com/devlikebear/tessera` Go module and embeds
its packages as the orchestration layer for agent work.

In this app, Tessera turns a writing goal into an auditable task graph: it
creates role-based tasks, reviews the plan before execution, wraps the approved
plan in a mandate, runs the tasks through queue workers, and emits run events
that can be saved or visualized.

Linetta owns the product surface around works, episodes, canon memory,
manuscript versions, backups, and the Mac UI. Tessera owns the reusable workflow
primitives behind that surface: config loading, planner/council review,
mandates, queues, task execution, retries, role limits, and observability.

The current authoring backend uses deterministic task handlers so local runs and
tests do not require provider credentials. The Tessera config still keeps `llm`
and `roles` fields validated so Linetta can later swap in provider-backed agents
without changing the app-level workflow.

## Usage

```sh
go run ./cmd/linetta \
  --goal "Draft a hopeful climate fiction opening" \
  --title "Harbor of Glass"
```

Markdown is the default output. JSON is also available:

```sh
go run ./cmd/linetta \
  --goal "Draft a mystery opening" \
  --title "Signal Rain" \
  --format json
```

Write output to a file:

```sh
go run ./cmd/linetta \
  --goal "Draft a solarpunk opening" \
  --title "Green Harbor" \
  --output draft.md
```

## Local Engine

Start the local library engine before opening the Mac app:

```sh
go run ./cmd/linetta serve \
  --db .linetta/dev.db \
  --addr 127.0.0.1:43190
```

The local engine API exposes:

- `GET /health`
- `GET /api/works`
- `POST /api/works`
- `GET /api/works/{workID}`
- `GET /api/works/{workID}/stats`
- `GET /api/works/{workID}/export/markdown`
- `GET /api/works/{workID}/episodes`
- `POST /api/works/{workID}/episodes`
- `PATCH /api/works/{workID}/episodes/{episodeID}/status`
- `GET /api/works/{workID}/episodes/{episodeID}/blueprint`
- `PUT /api/works/{workID}/episodes/{episodeID}/blueprint`
- `GET /api/works/{workID}/episodes/{episodeID}/versions`
- `POST /api/works/{workID}/episodes/{episodeID}/versions`
- `GET /api/works/{workID}/episodes/{episodeID}/export/txt`
- `POST /api/works/{workID}/episodes/{episodeID}/runs`
- `GET /api/works/{workID}/memory`
- `POST /api/works/{workID}/memory`
- `GET /api/works/{workID}/memory/search?q=...`
- `GET /api/works/{workID}/proposals?status=pending`
- `GET /api/works/{workID}/episodes/{episodeID}/continuity`
- `POST /api/proposals/{proposalID}/approve`
- `POST /api/proposals/{proposalID}/reject`
- `POST /api/proposals/{proposalID}/defer`
- `PATCH /api/continuity/{issueID}`
- `GET /api/runs/{runID}/artifacts`
- `GET /api/runs/{runID}/events`
- `GET /api/runs/{runID}/events/stream`

## Mac App

The SwiftUI scaffold lives in `macos/Linetta`:

```sh
cd macos/Linetta
swift test
swift run Linetta
```

The app opens to a searchable work gallery, creates works through the local
engine, manages canon memory, runs Tessera episode agents, reviews canon diffs,
stores manuscript versions, and exports Markdown/TXT files.

## Backup

Create a portable backup with the SQLite library and an optional Tessera config
snapshot:

```sh
go run ./cmd/linetta export-library \
  --db .linetta/dev.db \
  --config .linetta/tessera.yaml \
  --out backup.zip
```

Restore into a new database path:

```sh
go run ./cmd/linetta import-library \
  --in backup.zip \
  --db .linetta/restored.db
```

Use `--force` only when intentionally overwriting an existing restore path.

## Tessera Config

Linetta can load Tessera YAML or JSON config files:

```sh
go run ./cmd/linetta \
  --config examples/tessera.yaml \
  --goal "Draft a hopeful climate fiction opening" \
  --title "Harbor of Glass"
```

The app currently applies these config fields:

- `run.id`
- `run.workers`
- `run.max_attempts`
- `run.role_limits`
- `queue.lease_timeout`
- `observe.events_jsonl`
- `observe.report_json`
- `observe.html_report`

`llm` and `roles` are validated by Tessera's config loader and kept in the file
so the app can later swap the deterministic authoring handlers for real
provider-backed agents without changing the config shape.

## Run Visualization

When `observe.events_jsonl` or `observe.html_report` is set, Linetta records
Tessera run events and writes a static HTML run report:

```sh
go run ./cmd/linetta \
  --config examples/tessera.yaml \
  --goal "Draft a lighthouse mystery" \
  --title "Signal Rain"
```

You can also visualize an existing events file:

```sh
go run ./cmd/linetta visualize \
  .tessera/runs/linetta/events.jsonl \
  --out .tessera/runs/linetta/report.html
```

## Architecture

Linetta uses Tessera's novel-team planner flow:

1. `leader.NewNovelTeamPlanner()` creates the four-step story plan.
2. `council.DefaultCouncil()` reviews the plan.
3. `mandate.New(...).Approve(...)` creates the approval gate.
4. `run.NewTaskGraphFromPlan(...)` builds the task graph.
5. `run.ExecuteTaskGraph(...)` dispatches the tasks through in-memory queue workers.
6. `observe.JSONLinesSink` and `visualize.WriteHTMLReport` turn run events into
   inspectable execution artifacts.

The current authoring backend is deterministic so the app is testable without
provider credentials. The executor boundary is isolated in `internal/novel`, so
an LLM-backed writer can replace the deterministic agent handlers later.

## Verification

```sh
go test ./...
```
