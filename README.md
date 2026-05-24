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

## Architecture

Linetta uses Tessera's novel-team planner flow:

1. `leader.NewNovelTeamPlanner()` creates the four-step story plan.
2. `council.DefaultCouncil()` reviews the plan.
3. `mandate.New(...).Approve(...)` creates the approval gate.
4. `run.NewTaskGraphFromPlan(...)` builds the task graph.
5. `run.ExecuteTaskGraph(...)` dispatches the tasks through in-memory queue workers.

The current authoring backend is deterministic so the app is testable without
provider credentials. The executor boundary is isolated in `internal/novel`, so
an LLM-backed writer can replace the deterministic agent handlers later.

## Verification

```sh
go test ./...
```
