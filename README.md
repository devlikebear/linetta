# Linetta

Linetta is a Tessera-based novel-writing agent app. It turns a writing goal into
a mandate-gated agent run, then produces a small novel packet:

- world notes
- plot outline
- first chapter draft
- editorial review

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

The Phase 1 API exposes:

- `GET /health`
- `GET /api/works`
- `POST /api/works`
- `GET /api/works/{workID}`
- `GET /api/works/{workID}/episodes`
- `POST /api/works/{workID}/episodes`

## Mac App

The SwiftUI scaffold lives in `macos/Linetta`:

```sh
cd macos/Linetta
swift test
swift run Linetta
```

The app opens to a work gallery, can create a new work through the local engine,
and shows a first workspace shell for the selected work.

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
