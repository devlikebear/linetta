# In-process MCP client + agent loop + `agent.*` RPC Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give Linetta a built-in writing agent that runs as an in-process client of Linetta's own MCP server, so the tools the writer already trusts are the only tools it has.

**Architecture:** A second `mcp.Server` instance is built from the same `mcphost.ToolDeps` and connected to an `mcp.Client` over `mcp.NewInMemoryTransports()` — no port, no token, no Origin check. `internal/agent` drives a hand-written loop (`llm.Client.Chat` → tool calls → `mcp.ClientSession.CallTool` → back into the message list) and reports progress as `agent.*` JSON-RPC notifications. Writes reach the UI through the existing `mcp.changed` path; nothing new is built for workspace refresh.

**Tech Stack:** Go 1.x engine, `github.com/modelcontextprotocol/go-sdk` v1.7.0, `github.com/devlikebear/tars` v0.34.3 (`pkg/llm` only), SQLite via `internal/store`, Tauri 2 / Rust shell, React 18 + TypeScript renderer.

---

## Verified protocol facts

Three facts were established by spike before this plan was written (`engine/internal/spiketmp`, since deleted). They contradict the design spec in one place, so they are recorded here rather than left to be rediscovered.

| Question | Answer | Consequence |
| --- | --- | --- |
| Does a Go `context.Context` **value** set by the client reach the server tool handler through `NewInMemoryTransports()`? | **No.** `ctx.Value(k)` is `nil` server-side. | The spec's "`ToolDeps.record` reads it from a context value" **does not work**. Task 1 carries `run_id` in the MCP `_meta` field instead. |
| Does `_meta` set with `CallToolParams.SetMeta` reach the handler? | **Yes.** `req.Params.GetMeta()["linetta/run_id"]` returns the value, alongside the SDK's own `io.modelcontextprotocol/*` keys. | This is the transport for `run_id`. |
| Does cancelling the client's `CallTool` context stop the server handler? | **Yes.** The caller returns immediately with `context.Canceled`, and the handler's own `ctx.Done()` fires — the SDK maps it to `notifications/cancelled`. | `agent.cancel` really does interrupt an in-flight tool call; the loop does not have to wait it out. |

A fourth, smaller fact: `ListTools` hands the client `Tool.InputSchema` as a `map[string]any`, not raw JSON. Converting to `llm.ToolSchema` therefore means `json.Marshal` on that map (Task 2).

## Two deviations from issue #93

**Error codes.** The issue names `-32011 agent_busy` and `-32012 provider_consent_required`. Linetta does not use per-failure JSON-RPC codes: #91 established that a failure the reader must understand is `CodeInvalidParams` (-32602) carrying a `{"reason": "..."}` data payload, and the renderer translates the reason. `provider_consent_required` already shipped that way. This plan follows the shipped convention; the numbers in the issue predate it.

**`useEngineEvent` listeners.** The issue asks for the `ffi.rs` mapping and the renderer listeners in the same change. There is no panel until #95, and a listener with nothing to render cannot be written honestly. Task 8 adds the `ffi.rs` mapping plus a Rust test that pins all five event names, so #95 cannot silently miss one — which is what "at the same time" was protecting against.

## Global Constraints

Every task's requirements implicitly include this section.

- **`pkg/agentloop` and `pkg/session` are banned engine-wide.** The loop is written here. `scripts/validate-story-core-deps.sh` enforces it and already permits `internal/agent`; do not edit that script.
- **`tars/pkg/llm` may be imported only by `internal/provider` and `internal/agent`.** `internal/storycontext`, `internal/storyops`, `internal/mcphost` and `internal/rpc/handlers` must not link it — handlers see interfaces with plain types only.
- **`internal/agent` and every file added to `internal/mcphost` carry `//go:build !mobile`.** Mobile gets a disabled twin (the `mcp_disabled.go` pattern).
- **The built-in agent always runs in full mode.** `settings.MCPMode` (`off`/`read_only`) governs *external* clients only. The agent works when external MCP is off.
- **`mcp_project_id` does not restrict the built-in agent.** The panel's open work is the scope, delivered as the scope line (Task 4).
- **The agent gets its own `mcphost.limiter` instance and its own `storyops.Service`.** Sharing the limiter with external clients lets them starve each other; sharing storyops lets the panel's undo button revert an external agent's batch.
- **Notifications are named `agent.*`. The `ai.*` names are never reused** — stale listeners from the removed companion must not match.
- **A new reason code must be added to `engine/internal/rpc/reason.go`, `apps/desktop/src/lib/rpcMessage.ts` and all three catalogues in `apps/desktop/src/lib/i18n.tsx` in the same commit.** An unmapped code falls through to `String(error)`, which prints the provider's raw response body.
- **Every `agent.*` method must be added to `RENDERER_ENGINE_METHODS` in `apps/desktop/src-tauri/src/lib.rs`,** which is sorted and binary-searched. Every `agent.*` notification must be added to `notification_event` in `apps/desktop/src-tauri/src/ffi.rs`. Missing either fails silently.
- **Consent: not a byte of manuscript text leaves without it.** The loop obtains its client through `provider.Source.Client`, which refuses before constructing anything able to send. Do not add a second path.
- **CI runs the full Go test suite on `windows-latest`, not just a build.** Any test touching host-scoped state (`$HOME`, the keychain, real ports) must isolate it through a shared helper — `os.UserHomeDir` reads `%USERPROFILE%` on Windows and `$HOME` on Unix.
- **Verification for every task:** `go build ./...` and `go test ./...` from `engine/`. Tasks touching build-tagged files also run `make test-mobile-engine`, `go build -tags mas ./...`, and `GOOS=windows go build ./...`. Task 8 additionally runs `make test-desktop` and `make test-tauri`.

---

### Task 1: The activity log learns who called

The audit trail currently cannot tell an external Claude Desktop session from the built-in panel. Two columns fix that, and the run id lets the panel group one turn's changes behind a single undo button.

**Files:**
- Create: `engine/internal/store/migrations/0017_mcp_activity_source.sql`
- Modify: `engine/internal/mcphost/activity.go`
- Modify: `engine/internal/mcphost/tools.go` (`ToolDeps`, `record`, `requireProject`, `allowedProjectID`)
- Test: `engine/internal/mcphost/activity_test.go` (create), `engine/internal/mcphost/tools_source_test.go` (create)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `mcphost.SourceExternal = "external"`, `mcphost.SourceAgent = "agent"` (string constants)
  - `mcphost.MetaRunID = "linetta/run_id"` (string constant — the `_meta` key)
  - `mcphost.ToolDeps.Source string` (empty means external)
  - `mcphost.ActivityEntry` gains `Source string \`json:"source"\`` and `RunID string \`json:"run_id,omitempty"\``

- [ ] **Step 1: Write the failing migration round-trip test**

Create `engine/internal/mcphost/activity_test.go`:

```go
//go:build !mobile

package mcphost

import (
	"context"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/store"
)

func newActivityRepo(t *testing.T) *ActivityRepo {
	t.Helper()
	st, err := store.Open(context.Background(), t.TempDir()+"/test.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return NewActivityRepo(st.DB())
}

// A row written by an external client carries no run id and reads back as
// "external" — which is also what every row written before 0017 becomes.
func TestRecord_defaultsToExternal(t *testing.T) {
	r := newActivityRepo(t)
	if err := r.Record(context.Background(), ActivityEntry{Tool: "linetta_read_scene", OK: true}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	got, err := r.List(context.Background(), 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1", len(got))
	}
	if got[0].Source != SourceExternal {
		t.Errorf("Source = %q, want %q", got[0].Source, SourceExternal)
	}
	if got[0].RunID != "" {
		t.Errorf("RunID = %q, want empty", got[0].RunID)
	}
}

// The built-in agent's rows are distinguishable and carry the turn they
// belong to, which is what the panel groups its undo button by.
func TestRecord_keepsSourceAndRunID(t *testing.T) {
	r := newActivityRepo(t)
	if err := r.Record(context.Background(), ActivityEntry{
		Tool: "linetta_write_scene", OK: true, Source: SourceAgent, RunID: "run-7",
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	got, err := r.List(context.Background(), 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got[0].Source != SourceAgent || got[0].RunID != "run-7" {
		t.Errorf("entry = %+v, want source=agent run_id=run-7", got[0])
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd engine && go test ./internal/mcphost/ -run TestRecord -v`
Expected: FAIL — `got[0].Source undefined (type ActivityEntry has no field or method Source)`.

- [ ] **Step 3: Write the migration**

Create `engine/internal/store/migrations/0017_mcp_activity_source.sql`:

```sql
-- Who called the tool. Before the built-in agent (#93) every row came from an
-- external MCP client, so that is the default and existing rows keep their
-- meaning without a backfill.
--
-- run_id groups one agent turn's calls together: the panel puts a single undo
-- button on a turn rather than one per tool call. It is NOT NULL DEFAULT ''
-- rather than nullable so the scan needs no COALESCE — an external row's
-- empty string and a nullable column's NULL say the same thing here.
ALTER TABLE mcp_activity ADD COLUMN source TEXT NOT NULL DEFAULT 'external';
ALTER TABLE mcp_activity ADD COLUMN run_id TEXT NOT NULL DEFAULT '';
```

- [ ] **Step 4: Extend `ActivityEntry`, `Record` and `List`**

In `engine/internal/mcphost/activity.go`, add the constants above the type:

```go
// Who called a tool. External is every MCP client that reaches the HTTP host;
// agent is Linetta's own panel, which speaks to a second server instance over
// an in-memory transport.
const (
	SourceExternal = "external"
	SourceAgent    = "agent"
)
```

Add the two fields to `ActivityEntry`:

```go
	OK        bool   `json:"ok"`
	Detail    string `json:"detail,omitempty"`
	Source    string `json:"source"`
	RunID     string `json:"run_id,omitempty"`
```

In `Record`, default the source and widen the insert:

```go
	if e.At == 0 {
		e.At = r.now()
	}
	if e.Source == "" {
		e.Source = SourceExternal
	}
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO mcp_activity (id, at, tool, project_id, target_id, ok, detail, source, run_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.At, e.Tool, e.ProjectID, e.TargetID, boolToInt(e.OK), truncate(e.Detail, 500),
		e.Source, e.RunID,
	); err != nil {
		return err
	}
```

In `List`, widen the select and the scan:

```go
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, at, tool, project_id, target_id, ok, detail, source, run_id
		 FROM mcp_activity ORDER BY at DESC, id DESC LIMIT ?`, limit)
```

```go
		if err := rows.Scan(&e.ID, &e.At, &e.Tool, &e.ProjectID, &e.TargetID, &ok, &e.Detail,
			&e.Source, &e.RunID); err != nil {
			return nil, err
		}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd engine && go test ./internal/mcphost/ -run TestRecord -v`
Expected: PASS (both).

- [ ] **Step 6: Write the failing test for the `_meta` run id and the project-restriction exemption**

Create `engine/internal/mcphost/tools_source_test.go`:

```go
//go:build !mobile

package mcphost

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type probeIn struct {
	ProjectID string `json:"project_id"`
}

func (p probeIn) scope() (string, string) { return p.ProjectID, "" }

// A Go context value does not survive the MCP transport — verified by spike —
// so the run id rides in _meta. This is the test that pins that decision.
func TestRecord_readsTheRunIDFromMeta(t *testing.T) {
	repo := newActivityRepo(t)
	d := ToolDeps{Activity: repo, Source: SourceAgent}
	h := record(d, "probe", func(context.Context, *mcp.CallToolRequest, probeIn) (*mcp.CallToolResult, any, error) {
		return nil, nil, nil
	})

	req := &mcp.CallToolRequest{Params: &mcp.CallToolParams{Name: "probe"}}
	req.Params.SetMeta(map[string]any{MetaRunID: "run-42"})
	if _, _, err := h(context.Background(), req, probeIn{ProjectID: "p1"}); err != nil {
		t.Fatalf("handler: %v", err)
	}

	got, err := repo.List(context.Background(), 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got[0].Source != SourceAgent || got[0].RunID != "run-42" {
		t.Errorf("entry = %+v, want source=agent run_id=run-42", got[0])
	}
}

// The ordinary external call: no _meta of its own, and the log says so.
func TestRecord_externalCallWithNoMetaIsLoggedAsExternal(t *testing.T) {
	repo := newActivityRepo(t)
	d := ToolDeps{Activity: repo}
	h := record(d, "probe", func(context.Context, *mcp.CallToolRequest, probeIn) (*mcp.CallToolResult, any, error) {
		return nil, nil, nil
	})

	req := &mcp.CallToolRequest{Params: &mcp.CallToolParams{Name: "probe"}}
	if _, _, err := h(context.Background(), req, probeIn{ProjectID: "p1"}); err != nil {
		t.Fatalf("handler: %v", err)
	}

	got, err := repo.List(context.Background(), 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got[0].Source != SourceExternal || got[0].RunID != "" {
		t.Errorf("entry = %+v, want an external row with no run id", got[0])
	}
}

// An external client can put anything in _meta. It must not be able to claim
// it is the built-in agent, or the activity log stops being evidence.
func TestRecord_externalCallerCannotClaimARunID(t *testing.T) {
	repo := newActivityRepo(t)
	d := ToolDeps{Activity: repo} // Source unset: external
	h := record(d, "probe", func(context.Context, *mcp.CallToolRequest, probeIn) (*mcp.CallToolResult, any, error) {
		return nil, nil, nil
	})

	req := &mcp.CallToolRequest{Params: &mcp.CallToolParams{Name: "probe"}}
	req.Params.SetMeta(map[string]any{MetaRunID: "forged"})
	if _, _, err := h(context.Background(), req, probeIn{ProjectID: "p1"}); err != nil {
		t.Fatalf("handler: %v", err)
	}

	got, err := repo.List(context.Background(), 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got[0].Source != SourceExternal || got[0].RunID != "" {
		t.Errorf("entry = %+v, want an unforgeable external row", got[0])
	}
}

// mcp_project_id restricts external clients to one work. The built-in agent's
// scope is the work whose panel is open, so the restriction must not apply —
// otherwise a writer who locked MCP to one work loses the panel everywhere else.
func TestAllowedProjectID_doesNotRestrictTheAgent(t *testing.T) {
	st := newRestrictedSettings(t, "p-locked")
	if got := (ToolDeps{Settings: st}).allowedProjectID(); got != "p-locked" {
		t.Errorf("external allowedProjectID = %q, want p-locked", got)
	}
	if got := (ToolDeps{Settings: st, Source: SourceAgent}).allowedProjectID(); got != "" {
		t.Errorf("agent allowedProjectID = %q, want empty", got)
	}
}
```

Add the settings helper to the same file (`host_test.go` may already have one; if it does, reuse it and delete this):

```go
func newRestrictedSettings(t *testing.T, projectID string) *settings.Store {
	t.Helper()
	t.Setenv("LINETTA_HOME", t.TempDir())
	st, err := settings.NewWithSecretStore(settings.NewMemorySecretStore())
	if err != nil {
		t.Fatalf("settings: %v", err)
	}
	if _, err := st.Set(context.Background(), settings.Patch{MCPProjectID: &projectID}); err != nil {
		t.Fatalf("restrict: %v", err)
	}
	return st
}
```

- [ ] **Step 7: Run it to verify it fails**

Run: `cd engine && go test ./internal/mcphost/ -run 'TestRecord_reads|TestRecord_external|TestAllowedProjectID' -v`
Expected: FAIL — `unknown field Source in struct literal of type ToolDeps`.

- [ ] **Step 8: Add `Source` to `ToolDeps` and wire it through `record` and `allowedProjectID`**

In `engine/internal/mcphost/tools.go`, add to `ToolDeps` next to `Activity`:

```go
	// Source names who is calling: SourceExternal (the HTTP host) or
	// SourceAgent (the built-in panel's in-memory server). It is a field on
	// the deps rather than something read off the wire, so an external client
	// cannot claim to be the agent. Empty means external.
	Source string
```

Add the `_meta` key constant near the top of the file:

```go
// MetaRunID is the MCP _meta key the built-in agent stamps each tool call
// with. A Go context value does not cross the in-memory transport — the
// server handler sees a fresh context — so this is how one turn's calls are
// tied together in the activity log.
const MetaRunID = "linetta/run_id"
```

Replace `record`'s activity call:

```go
		ok := err == nil && (res == nil || !res.IsError)
		detail := ""
		if err != nil {
			detail = err.Error()
		} else if res != nil && res.IsError {
			detail = firstText(res)
		}
		d.recordActivity(ctx, tool, projectID, targetID, ok, detail, d.runIDOf(req))
		return res, out, err
```

Add the reader and widen `recordActivity`:

```go
// runIDOf reads the built-in agent's run id off the request. Only an agent
// server reads it: external callers control their own _meta, and a forged
// run id would put their edits under the panel's undo button.
func (d ToolDeps) runIDOf(req *mcp.CallToolRequest) string {
	if d.Source != SourceAgent || req == nil || req.Params == nil {
		return ""
	}
	id, _ := req.Params.GetMeta()[MetaRunID].(string)
	return id
}

func (d ToolDeps) recordActivity(ctx context.Context, tool, projectID, targetID string, ok bool, detail, runID string) {
	if d.Activity == nil {
		return
	}
	if err := d.Activity.Record(ctx, ActivityEntry{
		Tool:      tool,
		ProjectID: projectID,
		TargetID:  targetID,
		OK:        ok,
		Detail:    detail,
		Source:    d.sourceOrExternal(),
		RunID:     runID,
	}); err != nil {
		logf("activity log: %v", err)
	}
}

func (d ToolDeps) sourceOrExternal() string {
	if d.Source == "" {
		return SourceExternal
	}
	return d.Source
}
```

Exempt the agent in `allowedProjectID`:

```go
// allowedProjectID returns the restriction, or "" when every work is reachable.
//
// The built-in agent is never restricted: mcp_project_id exists so a writer
// can hand an external client one work only, and the panel's scope is
// whichever work it is open on. Tying the exemption to Source keeps it in the
// same place as the log stamp, so the two cannot drift apart.
func (d ToolDeps) allowedProjectID() string {
	if d.Settings == nil || d.Source == SourceAgent {
		return ""
	}
	return strings.TrimSpace(d.Settings.MCPProjectID())
}
```

Make `requireProject` read through it — today it queries the store directly, which would bypass the exemption:

```go
func (d ToolDeps) requireProject(ctx context.Context, projectID string) (project.Project, *mcp.CallToolResult) {
	projectID = strings.TrimSpace(projectID)
	restricted := d.allowedProjectID()
	if restricted != "" {
```

- [ ] **Step 9: Run the whole mcphost suite**

Run: `cd engine && go test ./internal/mcphost/ ./internal/store/ -v`
Expected: PASS. The pre-existing restriction tests still pass — external deps have `Source == ""`.

- [ ] **Step 10: Run the full suite and the tagged builds**

Run: `cd engine && go build ./... && go test ./...`
Run: `make test-mobile-engine`
Run: `cd engine && go build -tags mas ./... && GOOS=windows go build ./...`
Expected: all pass.

- [ ] **Step 11: Commit**

```bash
git add engine/internal/store/migrations/0017_mcp_activity_source.sql engine/internal/mcphost/
git commit -m "feat(mcphost): record who called a tool and which agent turn it belonged to (#93)"
```

---

### Task 2: The in-process MCP client

The agent's whole tool surface. A second `mcp.Server` built from the same `ToolDeps`, an `mcp.Client` on the other end of an in-memory pipe, and the two conversions the loop needs: MCP tool list → `llm.ToolSchema`, and `llm` tool call → MCP `CallTool`.

**Files:**
- Create: `engine/internal/agent/tools.go`
- Test: `engine/internal/agent/tools_test.go`

**Interfaces:**
- Consumes: `mcphost.MetaRunID` (Task 1).
- Produces:
  - `agent.RegisterTools = func(*mcp.Server)` — the caller installs the full tool set
  - `agent.connectTools(ctx context.Context, register RegisterTools) (*toolSession, error)`
  - `(*toolSession).schemas(ctx) ([]llm.ToolSchema, error)`
  - `(*toolSession).call(ctx, runID, name, arguments string) toolResult`
  - `(*toolSession).Close() error`
  - `type toolResult struct { Text string; IsError bool; BatchID string; NodeIDs []string; Truncated bool }`
  - `agent.maxToolResultChars = 24000`

- [ ] **Step 1: Write the failing test**

Create `engine/internal/agent/tools_test.go`:

```go
//go:build !mobile

package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/mcphost"
)

type echoIn struct {
	Text string `json:"text"`
}

type echoOut struct {
	UndoBatchID  string   `json:"undo_batch_id,omitempty"`
	ChangedNodes []string `json:"changed_nodes,omitempty"`
}

// stubTools installs one tool that echoes its input back, reports the run id
// it saw, and can be told to fail or to return an oversized body.
func stubTools(seenRunID *string) RegisterTools {
	return func(s *mcp.Server) {
		mcp.AddTool(s, &mcp.Tool{Name: "echo", Description: "echo the text back"},
			func(_ context.Context, req *mcp.CallToolRequest, in echoIn) (*mcp.CallToolResult, echoOut, error) {
				if seenRunID != nil && req != nil && req.Params != nil {
					id, _ := req.Params.GetMeta()[mcphost.MetaRunID].(string)
					*seenRunID = id
				}
				switch in.Text {
				case "boom":
					return &mcp.CallToolResult{
						IsError: true,
						Content: []mcp.Content{&mcp.TextContent{Text: "version conflict; re-read and retry"}},
					}, echoOut{}, nil
				case "huge":
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: strings.Repeat("x", maxToolResultChars+500)}},
					}, echoOut{}, nil
				}
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: "echo: " + in.Text}},
				}, echoOut{UndoBatchID: "batch-1", ChangedNodes: []string{"n1", "n2"}}, nil
			})
	}
}

func newSession(t *testing.T, register RegisterTools) *toolSession {
	t.Helper()
	s, err := connectTools(context.Background(), register)
	if err != nil {
		t.Fatalf("connectTools: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// The model must see the tool descriptions the MCP layer already writes —
// keeping a second copy of them is the thing this design exists to avoid.
func TestSchemas_carryNameDescriptionAndParameters(t *testing.T) {
	s := newSession(t, stubTools(nil))
	got, err := s.schemas(context.Background())
	if err != nil {
		t.Fatalf("schemas: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d schemas, want 1", len(got))
	}
	if got[0].Type != "function" {
		t.Errorf("Type = %q, want function", got[0].Type)
	}
	if got[0].Function.Name != "echo" {
		t.Errorf("Name = %q", got[0].Function.Name)
	}
	if got[0].Function.Description != "echo the text back" {
		t.Errorf("Description = %q", got[0].Function.Description)
	}
	// The SDK hands the client a map[string]any; the model wants JSON.
	if !strings.Contains(string(got[0].Function.Parameters), `"text"`) {
		t.Errorf("Parameters = %s, want the input schema", got[0].Function.Parameters)
	}
}

func TestCall_returnsTextAndTheWriteMetadata(t *testing.T) {
	s := newSession(t, stubTools(nil))
	got := s.call(context.Background(), "run-1", "echo", `{"text":"hi"}`)
	if got.IsError {
		t.Fatalf("unexpected error result: %+v", got)
	}
	if !strings.Contains(got.Text, "echo: hi") {
		t.Errorf("Text = %q", got.Text)
	}
	if got.BatchID != "batch-1" {
		t.Errorf("BatchID = %q, want batch-1", got.BatchID)
	}
	if len(got.NodeIDs) != 2 || got.NodeIDs[0] != "n1" {
		t.Errorf("NodeIDs = %v", got.NodeIDs)
	}
}

// The run id is what ties a turn's writes together in the activity log.
func TestCall_stampsTheRunIDOnTheRequest(t *testing.T) {
	var seen string
	s := newSession(t, stubTools(&seen))
	s.call(context.Background(), "run-9", "echo", `{"text":"hi"}`)
	if seen != "run-9" {
		t.Errorf("server saw run id %q, want run-9", seen)
	}
}

// A tool error is the model's to recover from, not a transport failure: it
// comes back as a result the loop can hand straight to the model.
func TestCall_toolErrorIsAResultNotAFailure(t *testing.T) {
	s := newSession(t, stubTools(nil))
	got := s.call(context.Background(), "run-1", "echo", `{"text":"boom"}`)
	if !got.IsError {
		t.Fatal("want IsError")
	}
	if !strings.Contains(got.Text, "version conflict") {
		t.Errorf("Text = %q, want the tool's own message", got.Text)
	}
}

// linetta_read_scene legitimately returns long scenes, so the cap is generous
// — but a loop that keeps pasting a novel into the context has to hit a wall.
func TestCall_truncatesAnOversizedResult(t *testing.T) {
	s := newSession(t, stubTools(nil))
	got := s.call(context.Background(), "run-1", "echo", `{"text":"huge"}`)
	if !got.Truncated {
		t.Error("want Truncated")
	}
	if len([]rune(got.Text)) > maxToolResultChars+200 {
		t.Errorf("text is %d runes, want it capped near %d", len([]rune(got.Text)), maxToolResultChars)
	}
	if !strings.Contains(got.Text, "truncated") {
		t.Error("the model must be told the result was cut")
	}
}

// A name the server does not serve is the model's mistake. Reporting it as a
// result keeps the turn alive; a Go error would end it.
func TestCall_unknownToolComesBackAsAnErrorResult(t *testing.T) {
	s := newSession(t, stubTools(nil))
	got := s.call(context.Background(), "run-1", "linetta_nonexistent", `{}`)
	if !got.IsError {
		t.Fatal("want IsError")
	}
	if got.Text == "" {
		t.Error("want a message the model can act on")
	}
}

// Malformed arguments come from the model, not from Linetta.
func TestCall_malformedArgumentsComeBackAsAnErrorResult(t *testing.T) {
	s := newSession(t, stubTools(nil))
	got := s.call(context.Background(), "run-1", "echo", `{"text":`)
	if !got.IsError {
		t.Fatal("want IsError")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd engine && go test ./internal/agent/ -v`
Expected: FAIL — the package does not exist yet.

- [ ] **Step 3: Write the implementation**

Create `engine/internal/agent/tools.go`:

```go
//go:build !mobile

// Package agent is Linetta's built-in writing agent. It is a client of
// Linetta's own MCP server rather than a second tool layer: the same
// ToolDeps that serve Claude Desktop over HTTP are registered on a second
// mcp.Server here and reached over an in-memory pipe — no port, no token, no
// Origin check, and no second set of tool descriptions to keep in step.
//
// Part of the built-in BYOK agent (#90, issue #93).
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/mcphost"
	"github.com/devlikebear/tars/pkg/llm"
)

// maxToolResultChars caps one tool result before it enters the model's
// context. linetta_read_scene returning a long scene is normal, so the cap is
// generous; it exists so a loop cannot paste an entire manuscript into every
// subsequent request.
const maxToolResultChars = 24000

// RegisterTools installs the tool set on a fresh server. The caller supplies
// mcphost.ToolDeps.Register bound to full mode — the built-in agent is always
// full: an agent that cannot write the manuscript has no reason to exist.
type RegisterTools func(*mcp.Server)

// toolSession is one connected client/server pair. One per engine: the tools
// are stateless, and a session per run would re-handshake on every turn.
type toolSession struct {
	client *mcp.ClientSession
	server *mcp.ServerSession
}

// connectTools builds the server, registers the tools and dials it in memory.
func connectTools(ctx context.Context, register RegisterTools) (*toolSession, error) {
	if register == nil {
		return nil, fmt.Errorf("agent: no tool registration")
	}
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    mcphost.ServerName,
		Title:   "Linetta",
		Version: mcphost.ServerVersion,
	}, nil)
	register(srv)

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, serverTransport, nil)
	if err != nil {
		return nil, fmt.Errorf("agent: connect tool server: %w", err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{
		Name: "linetta-builtin-agent", Version: mcphost.ServerVersion,
	}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		_ = ss.Close()
		return nil, fmt.Errorf("agent: connect tool client: %w", err)
	}
	return &toolSession{client: cs, server: ss}, nil
}

func (s *toolSession) Close() error {
	if s == nil {
		return nil
	}
	err := s.client.Close()
	if serr := s.server.Close(); err == nil {
		err = serr
	}
	return err
}

// schemas converts the server's tool list into what the model wants. The
// descriptions come straight from the MCP layer, so the workflow they already
// spell out ("read the context before drafting", "refresh the summary after
// writing") reaches the model without being written twice.
func (s *toolSession) schemas(ctx context.Context) ([]llm.ToolSchema, error) {
	list, err := s.client.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("agent: list tools: %w", err)
	}
	out := make([]llm.ToolSchema, 0, len(list.Tools))
	for _, t := range list.Tools {
		// The SDK hands the client the input schema as a map[string]any; the
		// model's schema field is raw JSON.
		params, err := json.Marshal(t.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("agent: schema for %s: %w", t.Name, err)
		}
		out = append(out, llm.ToolSchema{
			Type: "function",
			Function: llm.ToolFunctionSchema{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			},
		})
	}
	return out, nil
}

// toolResult is one tool call reduced to what both the model and the panel
// need: the text to feed back, and the write metadata the undo button uses.
type toolResult struct {
	Text      string
	IsError   bool
	BatchID   string
	NodeIDs   []string
	Truncated bool
}

// call runs one tool. Every failure the model could have caused — a bad name,
// malformed arguments, a version conflict — comes back as an error *result*,
// not a Go error: the model reads it, corrects itself and tries again. Only
// the loop's own cancellation ends a turn.
func (s *toolSession) call(ctx context.Context, runID, name, arguments string) toolResult {
	args := map[string]any{}
	if trimmed := strings.TrimSpace(arguments); trimmed != "" && trimmed != "null" {
		if err := json.Unmarshal([]byte(trimmed), &args); err != nil {
			return toolResult{
				IsError: true,
				Text:    fmt.Sprintf("arguments for %s are not a JSON object: %v", name, err),
			}
		}
	}

	params := &mcp.CallToolParams{Name: name, Arguments: args}
	if runID != "" {
		params.SetMeta(map[string]any{mcphost.MetaRunID: runID})
	}
	res, err := s.client.CallTool(ctx, params)
	if err != nil {
		// A cancelled turn is the caller's business, not the model's.
		if ctx.Err() != nil {
			return toolResult{IsError: true, Text: "the writer stopped this turn"}
		}
		return toolResult{IsError: true, Text: fmt.Sprintf("tool %s could not be called: %v", name, err)}
	}

	out := toolResult{IsError: res.IsError}
	out.Text, out.Truncated = capText(textOf(res))
	out.BatchID, out.NodeIDs = writeMetadata(res)
	return out
}

func textOf(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func capText(s string) (string, bool) {
	r := []rune(s)
	if len(r) <= maxToolResultChars {
		return s, false
	}
	return string(r[:maxToolResultChars]) +
		"\n\n[truncated: the result was longer than this tool's limit. " +
		"Narrow the request if you need the rest.]", true
}

// writeMetadata pulls the undo batch and the touched scenes out of a write
// tool's structured output, so the panel can offer "undo this" without the
// loop knowing which tools are writes.
func writeMetadata(res *mcp.CallToolResult) (string, []string) {
	if res.StructuredContent == nil {
		return "", nil
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		return "", nil
	}
	var meta struct {
		UndoBatchID  string   `json:"undo_batch_id"`
		ChangedNodes []string `json:"changed_nodes"`
		NodeID       string   `json:"node_id"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return "", nil
	}
	nodes := meta.ChangedNodes
	if len(nodes) == 0 && meta.NodeID != "" {
		nodes = []string{meta.NodeID}
	}
	return meta.UndoBatchID, nodes
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd engine && go test ./internal/agent/ -v`
Expected: PASS (all seven).

- [ ] **Step 5: Verify the dependency gate still holds**

Run: `./scripts/validate-story-core-deps.sh`
Expected: `engine deps OK: pkg/llm only in provider/agent; no agentloop/session; story core clean`.

- [ ] **Step 6: Run the full suite and the tagged builds**

Run: `cd engine && go build ./... && go test ./...`
Run: `make test-mobile-engine`
Run: `cd engine && go build -tags mas ./... && GOOS=windows go build ./...`
Expected: all pass. `internal/agent` is `!mobile`, so the mobile build simply does not compile it.

- [ ] **Step 7: Commit**

```bash
git add engine/internal/agent/
git commit -m "feat(agent): connect the built-in agent to Linetta's own MCP server in memory (#93)"
```

---

### Task 3: The run registry

One run per work at a time, cancellable by id. Small, sharp, and the thing that keeps two panels from writing the same scene at once.

**Files:**
- Create: `engine/internal/agent/runs.go`
- Test: `engine/internal/agent/runs_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `agent.ErrBusy error` — sentinel for "this work already has a run"
  - `newRunRegistry() *runRegistry`
  - `(*runRegistry).start(projectID, runID string, cancel context.CancelFunc) error`
  - `(*runRegistry).cancel(runID string) bool`
  - `(*runRegistry).finish(projectID, runID string)`

- [ ] **Step 1: Write the failing test**

Create `engine/internal/agent/runs_test.go`:

```go
//go:build !mobile

package agent

import (
	"context"
	"errors"
	"testing"
)

func TestRuns_oneRunPerWork(t *testing.T) {
	r := newRunRegistry()
	if err := r.start("p1", "run-1", func() {}); err != nil {
		t.Fatalf("first start: %v", err)
	}
	if err := r.start("p1", "run-2", func() {}); !errors.Is(err, ErrBusy) {
		t.Fatalf("second start on the same work = %v, want ErrBusy", err)
	}
}

// Two works are two conversations. Blocking one on the other would make the
// panel unusable for a writer who keeps two books open.
func TestRuns_differentWorksRunTogether(t *testing.T) {
	r := newRunRegistry()
	if err := r.start("p1", "run-1", func() {}); err != nil {
		t.Fatalf("p1: %v", err)
	}
	if err := r.start("p2", "run-2", func() {}); err != nil {
		t.Fatalf("p2: %v", err)
	}
}

func TestRuns_finishReleasesTheWork(t *testing.T) {
	r := newRunRegistry()
	_ = r.start("p1", "run-1", func() {})
	r.finish("p1", "run-1")
	if err := r.start("p1", "run-2", func() {}); err != nil {
		t.Fatalf("start after finish: %v", err)
	}
}

// A late finish from a run that was already replaced must not evict the run
// that replaced it — otherwise a slow teardown silently permits a third run.
func TestRuns_finishIgnoresAStaleRunID(t *testing.T) {
	r := newRunRegistry()
	_ = r.start("p1", "run-1", func() {})
	r.finish("p1", "run-old")
	if err := r.start("p1", "run-2", func() {}); !errors.Is(err, ErrBusy) {
		t.Fatalf("a stale finish released the work: %v", err)
	}
}

func TestRuns_cancelInvokesTheRunsCancelFunc(t *testing.T) {
	r := newRunRegistry()
	ctx, cancel := context.WithCancel(context.Background())
	_ = r.start("p1", "run-1", cancel)
	if !r.cancel("run-1") {
		t.Fatal("cancel reported the run as unknown")
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("the run's context was not cancelled")
	}
}

// The panel can ask to cancel a run that already finished — a click landing
// just after the last token. That is not an error.
func TestRuns_cancelUnknownRunIsFalseNotAPanic(t *testing.T) {
	r := newRunRegistry()
	if r.cancel("never-existed") {
		t.Fatal("cancel claimed to have stopped a run that never ran")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd engine && go test ./internal/agent/ -run TestRuns -v`
Expected: FAIL — `undefined: newRunRegistry`.

- [ ] **Step 3: Write the implementation**

Create `engine/internal/agent/runs.go`:

```go
//go:build !mobile

package agent

import (
	"context"
	"errors"
	"sync"
)

// ErrBusy means the work already has a turn in flight. One run per work is
// the rule: two loops writing the same manuscript would interleave scene
// updates the writer never asked for, and the second would spend its budget
// fighting version conflicts with the first.
var ErrBusy = errors.New("agent: this work already has a turn running")

type runRegistry struct {
	mu        sync.Mutex
	byProject map[string]string             // projectID -> runID
	cancels   map[string]context.CancelFunc // runID -> cancel
}

func newRunRegistry() *runRegistry {
	return &runRegistry{
		byProject: map[string]string{},
		cancels:   map[string]context.CancelFunc{},
	}
}

// start claims the work for runID. It returns ErrBusy when another run holds it.
func (r *runRegistry) start(projectID, runID string, cancel context.CancelFunc) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, taken := r.byProject[projectID]; taken {
		return ErrBusy
	}
	r.byProject[projectID] = runID
	r.cancels[runID] = cancel
	return nil
}

// cancel stops the named run and reports whether it was still running. A
// false is not a failure: the writer's stop click can land after the last
// token, and the panel should not show an error for that.
func (r *runRegistry) cancel(runID string) bool {
	r.mu.Lock()
	cancel, ok := r.cancels[runID]
	r.mu.Unlock()
	if !ok {
		return false
	}
	cancel()
	return true
}

// finish releases the work. The runID is checked against the current holder
// so a late teardown from a run that has already been replaced cannot evict
// its successor and let a third run start.
func (r *runRegistry) finish(projectID, runID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byProject[projectID] == runID {
		delete(r.byProject, projectID)
	}
	delete(r.cancels, runID)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd engine && go test ./internal/agent/ -run TestRuns -v`
Expected: PASS (six).

- [ ] **Step 5: Run with the race detector**

Run: `cd engine && go test ./internal/agent/ -race`
Expected: PASS. The registry is reached from the RPC goroutine and the run goroutine at once.

- [ ] **Step 6: Commit**

```bash
git add engine/internal/agent/runs.go engine/internal/agent/runs_test.go
git commit -m "feat(agent): one turn per work, cancellable by run id (#93)"
```

---

### Task 4: Transcript and prompt assembly

What the model is told, and what is remembered. The system prompt is deliberately short: the brief is not pasted in — the agent fetches it with `linetta_get_story_context` exactly as an external agent does, which is how the tool descriptions stay honest.

**Files:**
- Create: `engine/internal/agent/prompt.go`
- Create: `engine/internal/agent/transcript.go`
- Modify: `engine/internal/settings/settings.go` (add a `Language()` accessor)
- Test: `engine/internal/agent/prompt_test.go`, `engine/internal/agent/transcript_test.go`

**Interfaces:**
- Consumes: `companion.HistoryRepo`, `companion.HistoryMessage` (existing).
- Produces:
  - `settings.(*Store).Language() string`
  - `type ScopeLookup interface { ProjectTitle(ctx, projectID string) string; NodeLabel(ctx, nodeID string) string }`
  - `systemPrompt(lang string) string`
  - `scopeLine(ctx, ScopeLookup, projectID, nodeID string) string`
  - `historyBudget = 40000`
  - `priorMessages(msgs []companion.HistoryMessage) []llm.ChatMessage`
  - `type transcript struct { repo *companion.HistoryRepo; clock func() int64 }`
  - `(*transcript).appendUser / appendAssistant / appendToolEvent / load / clear`
  - `type toolEvent struct { Name, Summary string; OK bool; BatchID string; NodeIDs []string }`

- [ ] **Step 1: Write the failing prompt test**

Create `engine/internal/agent/prompt_test.go`:

```go
//go:build !mobile

package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/companion"
)

type fakeScope struct {
	titles map[string]string
	labels map[string]string
}

func (f fakeScope) ProjectTitle(_ context.Context, id string) string { return f.titles[id] }
func (f fakeScope) NodeLabel(_ context.Context, id string) string    { return f.labels[id] }

func TestSystemPrompt_namesTheReplyLanguage(t *testing.T) {
	for _, lang := range []string{"ko", "en", "ja"} {
		got := systemPrompt(lang)
		if !strings.Contains(got, lang) {
			t.Errorf("systemPrompt(%q) does not name the language: %s", lang, got)
		}
	}
}

// The brief is fetched with a tool, never pasted in. If it ever appears here,
// the tool descriptions stop being exercised and start rotting.
func TestSystemPrompt_tellsTheAgentToReadContextFirst(t *testing.T) {
	got := systemPrompt("en")
	if !strings.Contains(got, "linetta_get_story_context") {
		t.Error("the prompt must point at the context tool")
	}
	if !strings.Contains(got, "linetta_create_checkpoint") {
		t.Error("the prompt must ask for a checkpoint before a large rewrite")
	}
}

func TestScopeLine_namesTheWorkAndTheOpenScene(t *testing.T) {
	s := fakeScope{
		titles: map[string]string{"p1": "은하수를 여행하는"},
		labels: map[string]string{"n1": "1장 · 출발"},
	}
	got := scopeLine(context.Background(), s, "p1", "n1")
	for _, want := range []string{"p1", "은하수를 여행하는", "n1", "1장 · 출발"} {
		if !strings.Contains(got, want) {
			t.Errorf("scope line %q is missing %q", got, want)
		}
	}
}

// With no scene open the line must not invent one; a bracket containing an
// empty id reads to the model as a scene it can address.
func TestScopeLine_omitsTheSceneWhenNoneIsOpen(t *testing.T) {
	s := fakeScope{titles: map[string]string{"p1": "제목"}}
	got := scopeLine(context.Background(), s, "p1", "")
	if strings.Contains(strings.ToLower(got), "scene") {
		t.Errorf("scope line %q mentions a scene that is not open", got)
	}
}

// Tool rows are not replayed: they are the bulkiest thing in the transcript
// and the model already saw their outcome in its own reply.
func TestPriorMessages_dropsToolRowsAndKeepsOrder(t *testing.T) {
	got := priorMessages([]companion.HistoryMessage{
		{Role: "user", Content: "first"},
		{Role: "tool", Content: `{"name":"linetta_write_scene"}`},
		{Role: "assistant", Content: "second"},
	})
	if len(got) != 2 {
		t.Fatalf("got %d messages, want 2: %+v", len(got), got)
	}
	if got[0].Content != "first" || got[1].Content != "second" {
		t.Errorf("order or content wrong: %+v", got)
	}
}

// The budget is filled from the most recent turn backwards, so what survives
// is the end of the conversation, not its beginning.
func TestPriorMessages_keepsTheNewestWithinBudget(t *testing.T) {
	big := strings.Repeat("a", historyBudget-10)
	got := priorMessages([]companion.HistoryMessage{
		{Role: "user", Content: "oldest"},
		{Role: "assistant", Content: big},
		{Role: "user", Content: "newest"},
	})
	joined := ""
	for _, m := range got {
		joined += m.Content
	}
	if !strings.Contains(joined, "newest") {
		t.Error("the newest turn was dropped")
	}
	if strings.Contains(joined, "oldest") {
		t.Error("the budget did not bite")
	}
}

func TestPriorMessages_mapsRolesForTheModel(t *testing.T) {
	got := priorMessages([]companion.HistoryMessage{
		{Role: "user", Content: "q"},
		{Role: "assistant", Content: "a"},
	})
	if got[0].Role != "user" || got[1].Role != "assistant" {
		t.Errorf("roles = %q,%q", got[0].Role, got[1].Role)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd engine && go test ./internal/agent/ -run 'TestSystemPrompt|TestScopeLine|TestPriorMessages' -v`
Expected: FAIL — `undefined: systemPrompt`.

- [ ] **Step 3: Write `prompt.go`**

Create `engine/internal/agent/prompt.go`:

```go
//go:build !mobile

package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/tars/pkg/llm"
)

// historyBudget caps how much of the earlier conversation is replayed, in
// characters, filled newest-first. No summarisation: a compaction pass that
// quietly rewrites what the writer said is worse than an honest cut, and
// session search (sub-project 4) is where this gets revisited.
const historyBudget = 40000

// ScopeLookup resolves the names in the scope line. An interface rather than
// the repos themselves so this package stays out of project/node — and so the
// prompt can be tested without a database.
type ScopeLookup interface {
	ProjectTitle(ctx context.Context, projectID string) string
	NodeLabel(ctx context.Context, nodeID string) string
}

// systemPrompt is deliberately short. The story brief is NOT pasted in: the
// agent fetches it with linetta_get_story_context exactly as an external
// agent does, which is the only way the tool descriptions stay honest — if
// the workflow they describe stops working, this agent notices first.
//
// There is no per-work language field in Linetta, so the manuscript rule is
// stated as "match what is already written" rather than naming a language.
func systemPrompt(lang string) string {
	return fmt.Sprintf(`You are Linetta's writing agent. You work inside the writer's own app, on their manuscript, with the writer holding the final say on every word.

Reply to the writer in %q (their app language). Write manuscript prose in the language the existing manuscript is written in — read a scene first if you are unsure.

How you work:
- Call linetta_get_story_context before drafting anything, so you write from the work's actual state rather than a guess.
- After writing or revising a scene, refresh its summary so the rest of the work stays accurate.
- Before a large rewrite you are not certain about, call linetta_create_checkpoint first so the writer can get their version back.`, lang)
}

// scopeLine prefixes the writer's message with the work and the scene the
// panel is open on. Its whole job is to stop the agent asking "which scene?"
// — anything more than that it finds out with tools.
func scopeLine(ctx context.Context, look ScopeLookup, projectID, nodeID string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[work: %s %q]", projectID, titleOr(look, ctx, projectID))
	if strings.TrimSpace(nodeID) != "" {
		fmt.Fprintf(&b, " [open scene: %s %q]", nodeID, labelOr(look, ctx, nodeID))
	}
	return b.String()
}

func titleOr(look ScopeLookup, ctx context.Context, id string) string {
	if look == nil {
		return ""
	}
	return look.ProjectTitle(ctx, id)
}

func labelOr(look ScopeLookup, ctx context.Context, id string) string {
	if look == nil {
		return ""
	}
	return look.NodeLabel(ctx, id)
}

// priorMessages replays earlier turns within the budget, newest first, then
// restores chronological order. Tool rows are skipped: they are the bulkiest
// thing in the transcript, and the model already stated their outcome in the
// reply that followed them.
func priorMessages(msgs []companion.HistoryMessage) []llm.ChatMessage {
	kept := make([]llm.ChatMessage, 0, len(msgs))
	used := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		if strings.TrimSpace(m.Content) == "" {
			continue
		}
		if used+len(m.Content) > historyBudget {
			break
		}
		used += len(m.Content)
		kept = append(kept, llm.ChatMessage{Role: m.Role, Content: m.Content})
	}
	for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
		kept[i], kept[j] = kept[j], kept[i]
	}
	return kept
}
```

- [ ] **Step 4: Write the failing transcript test**

Create `engine/internal/agent/transcript_test.go`:

```go
//go:build !mobile

package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func newTranscript(t *testing.T) *transcript {
	t.Helper()
	st, err := store.Open(context.Background(), t.TempDir()+"/test.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return &transcript{
		repo:  companion.NewHistoryRepo(st.DB()),
		clock: func() int64 { return 1700000000000 },
	}
}

// The transcript reuses companion_messages so the 1.0 archive export picks up
// the new conversations without being touched.
func TestTranscript_roundTripsATurn(t *testing.T) {
	tr := newTranscript(t)
	ctx := context.Background()
	if err := tr.appendUser(ctx, "p1", "n1", "run-1", "write the opening"); err != nil {
		t.Fatalf("appendUser: %v", err)
	}
	if err := tr.appendAssistant(ctx, "p1", "n1", "run-1", "here it is", companion.HistoryStatusDone); err != nil {
		t.Fatalf("appendAssistant: %v", err)
	}
	got, err := tr.load(ctx, "p1", 50)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2", len(got))
	}
	if got[0].Role != "user" || got[1].Role != "assistant" {
		t.Errorf("roles = %q,%q", got[0].Role, got[1].Role)
	}
	if got[1].RunID != "run-1" {
		t.Errorf("RunID = %q", got[1].RunID)
	}
}

// A tool event is a row the panel renders as a chip. Its content is JSON so
// the panel can show the name, whether it worked, and an undo button.
func TestTranscript_toolEventIsStructuredJSON(t *testing.T) {
	tr := newTranscript(t)
	ctx := context.Background()
	if err := tr.appendToolEvent(ctx, "p1", "n1", "run-1", toolEvent{
		Name: "linetta_write_scene", Summary: "wrote 1장", OK: true,
		BatchID: "batch-1", NodeIDs: []string{"n1"},
	}); err != nil {
		t.Fatalf("appendToolEvent: %v", err)
	}
	got, err := tr.load(ctx, "p1", 50)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got[0].Role != "tool" {
		t.Fatalf("Role = %q, want tool", got[0].Role)
	}
	var ev toolEvent
	if err := json.Unmarshal([]byte(got[0].Content), &ev); err != nil {
		t.Fatalf("tool content is not JSON: %v (%s)", err, got[0].Content)
	}
	if ev.Name != "linetta_write_scene" || !ev.OK || ev.BatchID != "batch-1" {
		t.Errorf("event = %+v", ev)
	}
}

// agent.clear wipes the conversation. The activity log is a separate table and
// deliberately survives — it is the writer's record of what was done to the
// manuscript, not of what was said.
func TestTranscript_clearRemovesTheConversation(t *testing.T) {
	tr := newTranscript(t)
	ctx := context.Background()
	_ = tr.appendUser(ctx, "p1", "", "run-1", "hello")
	if err := tr.clear(ctx, "p1"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	got, err := tr.load(ctx, "p1", 50)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d rows after clear, want 0", len(got))
	}
}
```

- [ ] **Step 5: Run it to verify it fails**

Run: `cd engine && go test ./internal/agent/ -run TestTranscript -v`
Expected: FAIL — `undefined: transcript`.

- [ ] **Step 6: Write `transcript.go`**

Create `engine/internal/agent/transcript.go`:

```go
//go:build !mobile

package agent

import (
	"context"
	"encoding/json"
	"time"

	"github.com/devlikebear/linetta/engine/internal/companion"
)

// transcript stores the panel's conversation. It reuses companion_messages
// rather than adding a table: the columns already match, and the 1.0 archive
// export (export.companion_history) then carries the new conversations with
// no change of its own.
type transcript struct {
	repo  *companion.HistoryRepo
	clock func() int64
}

func (t *transcript) now() int64 {
	if t.clock != nil {
		return t.clock()
	}
	return time.Now().UnixMilli()
}

func (t *transcript) append(ctx context.Context, m companion.HistoryMessage) error {
	m.CreatedAt = t.now()
	return t.repo.Append(ctx, m)
}

func (t *transcript) appendUser(ctx context.Context, projectID, nodeID, runID, content string) error {
	return t.append(ctx, companion.HistoryMessage{
		ProjectID: projectID, NodeID: nodeID, RunID: runID,
		Role: "user", Content: content, Status: companion.HistoryStatusDone,
	})
}

func (t *transcript) appendAssistant(ctx context.Context, projectID, nodeID, runID, content, status string) error {
	return t.append(ctx, companion.HistoryMessage{
		ProjectID: projectID, NodeID: nodeID, RunID: runID,
		Role: "assistant", Content: content, Status: status,
	})
}

// toolEvent is what the panel renders as a chip under the reply: which tool
// ran, whether it worked, and — for a write — what to undo.
type toolEvent struct {
	Name    string   `json:"name"`
	Summary string   `json:"summary,omitempty"`
	OK      bool     `json:"ok"`
	BatchID string   `json:"batch_id,omitempty"`
	NodeIDs []string `json:"node_ids,omitempty"`
}

func (t *transcript) appendToolEvent(ctx context.Context, projectID, nodeID, runID string, ev toolEvent) error {
	body, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return t.append(ctx, companion.HistoryMessage{
		ProjectID: projectID, NodeID: nodeID, RunID: runID,
		Role: "tool", Content: string(body), Status: companion.HistoryStatusDone,
	})
}

func (t *transcript) load(ctx context.Context, projectID string, limit int) ([]companion.HistoryMessage, error) {
	return t.repo.List(ctx, companion.HistoryQuery{
		ProjectID: projectID,
		Scope:     companion.HistoryViewProject,
		Limit:     limit,
	})
}

// clear drops the conversation. The activity log is a different table and
// stays: it records what was done to the manuscript, which the writer needs
// whether or not they wanted to keep the chat.
func (t *transcript) clear(ctx context.Context, projectID string) error {
	return t.repo.Clear(ctx, companion.HistoryQuery{
		ProjectID: projectID,
		Scope:     companion.HistoryViewProject,
	})
}

// markRun stamps every row of a run with how the turn ended, so the panel can
// show a cancelled turn as cancelled and offer a retry on a failed one.
//
// The context is stripped of cancellation on purpose: this is called on the
// way out of a turn that was very often cancelled, and a cancelled context
// would refuse the very write that records the cancellation.
func (t *transcript) markRun(ctx context.Context, runID, status string) error {
	return t.repo.MarkRunStatus(context.WithoutCancel(ctx), runID, status)
}
```

- [ ] **Step 7: Add the `Language` accessor**

In `engine/internal/settings/settings.go`, next to the other read accessors:

```go
// Language is the app's UI language (ko/en/ja). The built-in agent replies in
// it, so it is read per turn rather than captured at start-up.
func (s *Store) Language() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Language
}
```

- [ ] **Step 8: Run the tests to verify they pass**

Run: `cd engine && go test ./internal/agent/ ./internal/settings/ -v`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add engine/internal/agent/prompt.go engine/internal/agent/prompt_test.go engine/internal/agent/transcript.go engine/internal/agent/transcript_test.go engine/internal/settings/settings.go
git commit -m "feat(agent): system prompt, scope line, history budget and the reused transcript (#93)"
```

---

### Task 5: The loop

The 200-line centre of the feature. Chat, run the tool calls it asks for, feed the results back, stop when it stops asking — or when it hits one of the three walls.

**Files:**
- Create: `engine/internal/agent/agent.go`
- Create: `engine/internal/agent/loop.go`
- Test: `engine/internal/agent/loop_test.go`

**Interfaces:**
- Consumes: `toolSession`/`connectTools`/`toolResult`/`RegisterTools` (Task 2), `runRegistry`/`ErrBusy` (Task 3), `systemPrompt`/`scopeLine`/`priorMessages`/`ScopeLookup`/`transcript`/`toolEvent` (Task 4).
- Produces:
  - `type ProviderSource interface { Client(id string) (llm.Client, provider.Resolved, error) }`
  - `type Deps struct { Providers ProviderSource; History *companion.HistoryRepo; Scope ScopeLookup; Register RegisterTools; Notify func(method string, params any); Language func() string; Undo func(ctx context.Context, batchID string) error; Clock func() int64 }`
  - `func New(d Deps) *Service`
  - `func (s *Service) Run(ctx context.Context, req RunRequest) (string, error)`
  - `func (s *Service) Cancel(runID string) error`
  - `func (s *Service) History(ctx context.Context, projectID string, limit int) ([]companion.HistoryMessage, error)`
  - `func (s *Service) Clear(ctx context.Context, projectID string) error`
  - `func (s *Service) Undo(ctx context.Context, batchID string) error`
  - `func (s *Service) Close() error`
  - `type RunRequest struct { ProjectID, NodeID, Prompt string }`
  - `maxIterations = 24`, `maxRepeatedToolErrors = 3`

- [ ] **Step 1: Write the failing test**

Create `engine/internal/agent/loop_test.go`:

```go
//go:build !mobile

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/provider"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/tars/pkg/llm"
)

// scriptedClient replays a fixed list of responses, one per Chat call, and
// records what it was asked. The last response repeats if the loop asks for
// more, which is how the iteration cap is exercised.
type scriptedClient struct {
	mu        sync.Mutex
	responses []llm.ChatResponse
	calls     int
	lastMsgs  []llm.ChatMessage
	lastOpts  llm.ChatOptions
}

func (c *scriptedClient) Ask(context.Context, string) (string, error) { return "", nil }

func (c *scriptedClient) Chat(ctx context.Context, msgs []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return llm.ChatResponse{}, err
	}
	c.calls++
	c.lastMsgs = msgs
	c.lastOpts = opts
	i := c.calls - 1
	if i >= len(c.responses) {
		i = len(c.responses) - 1
	}
	resp := c.responses[i]
	if opts.OnDelta != nil && resp.Message.Content != "" {
		opts.OnDelta(resp.Message.Content)
	}
	return resp, nil
}

func (c *scriptedClient) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func (c *scriptedClient) messages() []llm.ChatMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]llm.ChatMessage(nil), c.lastMsgs...)
}

func (c *scriptedClient) options() llm.ChatOptions {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastOpts
}

type fakeProviders struct {
	client llm.Client
	err    error
}

func (f fakeProviders) Client(string) (llm.Client, provider.Resolved, error) {
	return f.client, provider.Resolved{ID: "anthropic", Model: "test-model"}, f.err
}

// recorder collects notifications so a test can assert on the stream the
// panel will see.
type recorder struct {
	mu   sync.Mutex
	seen []struct {
		Method string
		Params any
	}
}

func (r *recorder) notify(method string, params any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = append(r.seen, struct {
		Method string
		Params any
	}{method, params})
}

func (r *recorder) methods() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.seen))
	for _, e := range r.seen {
		out = append(out, e.Method)
	}
	return out
}

func (r *recorder) has(method string) bool {
	for _, m := range r.methods() {
		if m == method {
			return true
		}
	}
	return false
}

func textReply(text string) llm.ChatResponse {
	return llm.ChatResponse{
		Message: llm.ChatMessage{Role: "assistant", Content: text},
		Usage:   llm.Usage{InputTokens: 10, OutputTokens: 5},
	}
}

func toolReply(name, args string) llm.ChatResponse {
	return llm.ChatResponse{
		Message: llm.ChatMessage{
			Role:      "assistant",
			ToolCalls: []llm.ToolCall{{ID: "call-1", Name: name, Arguments: args}},
		},
	}
}

func newService(t *testing.T, client llm.Client, rec *recorder) *Service {
	t.Helper()
	st, err := store.Open(context.Background(), t.TempDir()+"/test.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := New(Deps{
		Providers: fakeProviders{client: client},
		History:   companion.NewHistoryRepo(st.DB()),
		Scope:     fakeScope{titles: map[string]string{"p1": "제목"}},
		Register:  stubTools(nil),
		Notify:    rec.notify,
		Language:  func() string { return "ko" },
		Clock:     func() int64 { return 1700000000000 },
	})
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

// waitFor polls until cond holds or the deadline passes. Run is asynchronous
// by contract — it returns a run id and the work happens in a goroutine — so
// every assertion about the outcome has to wait for it.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestRun_returnsARunIDImmediatelyAndStreams(t *testing.T) {
	rec := &recorder{}
	svc := newService(t, &scriptedClient{responses: []llm.ChatResponse{textReply("좋아요")}}, rec)

	runID, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "안녕"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if runID == "" {
		t.Fatal("Run must return a run id the panel can cancel by")
	}
	waitFor(t, "agent.done", func() bool { return rec.has("agent.done") })
	if !rec.has("agent.delta") {
		t.Error("no agent.delta was emitted; the panel would show nothing until the end")
	}
}

// The scope line is why the agent does not have to ask "which scene?".
func TestRun_prefixesThePromptWithTheScopeLine(t *testing.T) {
	rec := &recorder{}
	c := &scriptedClient{responses: []llm.ChatResponse{textReply("ok")}}
	svc := newService(t, c, rec)

	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", NodeID: "n1", Prompt: "이 씬 고쳐줘"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "agent.done", func() bool { return rec.has("agent.done") })

	msgs := c.messages()
	if len(msgs) == 0 {
		t.Fatal("no messages reached the model")
	}
	last := msgs[len(msgs)-1]
	if !strings.Contains(last.Content, "[work: p1") || !strings.Contains(last.Content, "이 씬 고쳐줘") {
		t.Errorf("last message = %q, want the scope line then the prompt", last.Content)
	}
	if msgs[0].Role != "system" {
		t.Errorf("first message role = %q, want system", msgs[0].Role)
	}
}

// The whole design in one assertion: the model is offered the MCP tools,
// with the MCP layer's own descriptions.
func TestRun_offersTheMCPToolsToTheModel(t *testing.T) {
	rec := &recorder{}
	c := &scriptedClient{responses: []llm.ChatResponse{textReply("ok")}}
	svc := newService(t, c, rec)
	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "hi"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "agent.done", func() bool { return rec.has("agent.done") })

	tools := c.options().Tools
	if len(tools) == 0 {
		t.Fatal("the model was offered no tools")
	}
	if tools[0].Function.Description == "" {
		t.Error("tool descriptions must reach the model; they carry the workflow")
	}
}

func TestRun_executesAToolCallAndFeedsTheResultBack(t *testing.T) {
	rec := &recorder{}
	c := &scriptedClient{responses: []llm.ChatResponse{
		toolReply("echo", `{"text":"hi"}`),
		textReply("다 했어요"),
	}}
	svc := newService(t, c, rec)
	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "go"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "agent.done", func() bool { return rec.has("agent.done") })

	if c.count() != 2 {
		t.Fatalf("model was called %d times, want 2", c.count())
	}
	msgs := c.messages()
	var sawToolRole bool
	for _, m := range msgs {
		if m.Role == "tool" && strings.Contains(m.Content, "echo: hi") {
			sawToolRole = true
		}
	}
	if !sawToolRole {
		t.Errorf("the tool result never reached the model: %+v", msgs)
	}
	if !rec.has("agent.tool") {
		t.Error("no agent.tool notification; the panel would show no activity")
	}
}

// A runaway agent must hit a wall before it rewrites forty scenes.
func TestRun_stopsAtTheIterationCap(t *testing.T) {
	rec := &recorder{}
	c := &scriptedClient{responses: []llm.ChatResponse{toolReply("echo", `{"text":"hi"}`)}}
	svc := newService(t, c, rec)
	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "loop"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "the run to end", func() bool { return rec.has("agent.done") || rec.has("agent.error") })

	if c.count() > maxIterations {
		t.Errorf("model called %d times, want at most %d", c.count(), maxIterations)
	}
	if !rec.has("agent.error") {
		t.Error("hitting the cap must be reported, not silently swallowed")
	}
}

// Three identical failures in a row means the model is not learning from the
// error. Handing it a fourth wastes the writer's tokens.
func TestRun_stopsAfterTheSameToolFailsThreeTimes(t *testing.T) {
	rec := &recorder{}
	c := &scriptedClient{responses: []llm.ChatResponse{toolReply("echo", `{"text":"boom"}`)}}
	svc := newService(t, c, rec)
	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "fail"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "the run to end", func() bool { return rec.has("agent.done") || rec.has("agent.error") })

	if c.count() > maxRepeatedToolErrors+1 {
		t.Errorf("model called %d times, want the loop cut at %d", c.count(), maxRepeatedToolErrors+1)
	}
}

func TestRun_secondRunOnTheSameWorkIsBusy(t *testing.T) {
	rec := &recorder{}
	release := make(chan struct{})
	blocking := &blockingClient{release: release}
	svc := newService(t, blocking, rec)

	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "first"}); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	waitFor(t, "the first run to reach the model", func() bool { return blocking.entered() })

	_, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "second"})
	if err == nil {
		t.Fatal("a second run on the same work must be refused")
	}
	var re *rpc.ReasonError
	if !errors.As(err, &re) || re.Reason != rpc.ReasonAgentBusy {
		t.Fatalf("err = %v, want a %s reason", err, rpc.ReasonAgentBusy)
	}
	close(release)
}

func TestRun_cancelEndsTheTurnAndReportsIt(t *testing.T) {
	rec := &recorder{}
	release := make(chan struct{})
	blocking := &blockingClient{release: release}
	svc := newService(t, blocking, rec)

	runID, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "long"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "the run to reach the model", func() bool { return blocking.entered() })

	if err := svc.Cancel(runID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	waitFor(t, "agent.cancelled", func() bool { return rec.has("agent.cancelled") })
	close(release)

	// The work is free again once the cancelled run tears down.
	waitFor(t, "the work to be released", func() bool {
		_, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "next"})
		return err == nil
	})
}

// blockingClient parks inside Chat until released or cancelled.
type blockingClient struct {
	mu      sync.Mutex
	arrived bool
	release chan struct{}
}

func (b *blockingClient) entered() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.arrived
}

func (b *blockingClient) Ask(context.Context, string) (string, error) { return "", nil }

func (b *blockingClient) Chat(ctx context.Context, _ []llm.ChatMessage, _ llm.ChatOptions) (llm.ChatResponse, error) {
	b.mu.Lock()
	b.arrived = true
	b.mu.Unlock()
	select {
	case <-ctx.Done():
		return llm.ChatResponse{}, ctx.Err()
	case <-b.release:
		return textReply("released"), nil
	}
}

// A provider failure is a reason code, never the provider's raw body.
func TestRun_providerFailureBecomesAReasonCode(t *testing.T) {
	rec := &recorder{}
	st, err := store.Open(context.Background(), t.TempDir()+"/test.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := New(Deps{
		Providers: fakeProviders{err: &rpc.ReasonError{Reason: rpc.ReasonProviderConsentRequired}},
		History:   companion.NewHistoryRepo(st.DB()),
		Scope:     fakeScope{},
		Register:  stubTools(nil),
		Notify:    rec.notify,
		Language:  func() string { return "ko" },
	})
	t.Cleanup(func() { _ = svc.Close() })

	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "hi"}); err == nil {
		t.Fatal("a run without consent must be refused")
	}
}

func TestRun_writesTheTurnToTheTranscript(t *testing.T) {
	rec := &recorder{}
	c := &scriptedClient{responses: []llm.ChatResponse{
		toolReply("echo", `{"text":"hi"}`),
		textReply("끝"),
	}}
	svc := newService(t, c, rec)
	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "go"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "agent.done", func() bool { return rec.has("agent.done") })

	msgs, err := svc.History(context.Background(), "p1", 50)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	roles := map[string]int{}
	for _, m := range msgs {
		roles[m.Role]++
	}
	if roles["user"] != 1 || roles["assistant"] < 1 || roles["tool"] != 1 {
		t.Errorf("transcript roles = %v, want one user, one tool and a reply", roles)
	}
	for _, m := range msgs {
		if m.Role != "tool" {
			continue
		}
		var ev toolEvent
		if err := json.Unmarshal([]byte(m.Content), &ev); err != nil {
			t.Errorf("tool row is not JSON: %s", m.Content)
		}
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd engine && go test ./internal/agent/ -run TestRun -v`
Expected: FAIL — `undefined: New`.

- [ ] **Step 3: Add the two reason codes**

In `engine/internal/rpc/reason.go`, after the Codex block:

```go
	// The built-in agent's loop (#93). Busy is a state the writer resolves by
	// waiting or pressing stop; the iteration limit means the agent was cut
	// off mid-task and the reply says how far it got.
	ReasonAgentBusy           = "agent_busy"
	ReasonAgentIterationLimit = "agent_iteration_limit"
```

- [ ] **Step 4: Write `agent.go`**

Create `engine/internal/agent/agent.go`:

```go
//go:build !mobile

package agent

import (
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"

	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/provider"
	"github.com/devlikebear/tars/pkg/llm"
)

// ProviderSource resolves the writer's settings into a client. Read per turn,
// never cached: changing the model in settings applies to the next message
// without restarting the engine. *provider.Source satisfies this.
type ProviderSource interface {
	Client(id string) (llm.Client, provider.Resolved, error)
}

// Deps are the collaborators the agent needs.
type Deps struct {
	Providers ProviderSource
	History   *companion.HistoryRepo
	Scope     ScopeLookup
	// Register installs the full tool set on the agent's own server. The
	// caller binds it to an mcphost.ToolDeps carrying Source: SourceAgent,
	// its own limiter and its own storyops service.
	Register RegisterTools
	Notify   func(method string, params any)
	Language func() string
	// Undo reverts a structural batch. It must be bound to the SAME storyops
	// service the agent's tools use — undo batches live in memory on the
	// service, so any other instance simply does not have the batch.
	Undo  func(ctx context.Context, batchID string) error
	Clock func() int64
}

// Service is the built-in agent. One per engine.
type Service struct {
	deps Deps
	tr   *transcript
	runs *runRegistry

	// The tool session is built on the first run rather than at start-up:
	// a writer who never opens the panel should not pay for a second MCP
	// server, and Open must not fail because of one.
	toolsMu sync.Mutex
	tools   *toolSession
}

// New wires the service. Nothing connects until the first run.
func New(d Deps) *Service {
	return &Service{
		deps: d,
		tr:   &transcript{repo: d.History, clock: d.Clock},
		runs: newRunRegistry(),
	}
}

// Close tears down the tool session.
func (s *Service) Close() error {
	s.toolsMu.Lock()
	defer s.toolsMu.Unlock()
	if s.tools == nil {
		return nil
	}
	err := s.tools.Close()
	s.tools = nil
	return err
}

// session returns the connected tool session, building it once. A failed
// attempt leaves nothing cached, so the next run retries instead of
// inheriting a broken session forever.
func (s *Service) session(ctx context.Context) (*toolSession, error) {
	s.toolsMu.Lock()
	defer s.toolsMu.Unlock()
	if s.tools != nil {
		return s.tools, nil
	}
	ts, err := connectTools(ctx, s.deps.Register)
	if err != nil {
		return nil, err
	}
	s.tools = ts
	return ts, nil
}

func (s *Service) notify(method string, params any) {
	if s.deps.Notify != nil {
		s.deps.Notify(method, params)
	}
}

func (s *Service) language() string {
	if s.deps.Language == nil {
		return "en"
	}
	if lang := s.deps.Language(); lang != "" {
		return lang
	}
	return "en"
}

// History returns the panel's conversation for a work.
func (s *Service) History(ctx context.Context, projectID string, limit int) ([]companion.HistoryMessage, error) {
	return s.tr.load(ctx, projectID, limit)
}

// Clear drops the conversation. The activity log survives on purpose.
func (s *Service) Clear(ctx context.Context, projectID string) error {
	return s.tr.clear(ctx, projectID)
}

// Undo reverts a structural batch the agent applied.
func (s *Service) Undo(ctx context.Context, batchID string) error {
	if s.deps.Undo == nil {
		return errors.New("agent: undo is not wired")
	}
	return s.deps.Undo(ctx, batchID)
}

// Cancel stops a run. An unknown run id is not an error: the writer's stop
// click can land after the last token.
func (s *Service) Cancel(runID string) error {
	s.runs.cancel(runID)
	return nil
}

func newRunID() string { return uuid.NewString() }
```

- [ ] **Step 5: Write `loop.go`**

Create `engine/internal/agent/loop.go`:

```go
//go:build !mobile

package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/tars/pkg/llm"
)

// maxIterations caps tool calls per turn. A runaway agent should hit a wall
// the writer can see in the activity log, not rewrite forty scenes.
const maxIterations = 24

// maxRepeatedToolErrors ends a turn when the same tool returns the same error
// this many times running. A tool error is normally the model's to recover
// from — it re-reads and retries — but a model that has failed identically
// three times is not learning, and a fourth attempt spends the writer's money
// to prove it.
const maxRepeatedToolErrors = 3

// RunRequest is one message from the panel.
type RunRequest struct {
	ProjectID string
	NodeID    string
	Prompt    string
}

// Run starts a turn and returns its id immediately; progress arrives as
// agent.* notifications. Everything that can be refused synchronously — no
// consent, no credential, another turn already running — is refused here, so
// the panel gets an error it can render instead of a notification it has to
// correlate.
func (s *Service) Run(ctx context.Context, req RunRequest) (string, error) {
	projectID := strings.TrimSpace(req.ProjectID)
	if projectID == "" {
		return "", &rpc.ReasonError{
			Reason: rpc.ReasonProjectNotFound,
			Err:    errors.New("agent: project_id is required"),
		}
	}
	client, resolved, err := s.deps.Providers.Client("")
	if err != nil {
		return "", err
	}
	tools, err := s.session(ctx)
	if err != nil {
		return "", err
	}
	schemas, err := tools.schemas(ctx)
	if err != nil {
		return "", err
	}

	runID := newRunID()
	// Deliberately NOT derived from the caller's ctx: that context belongs to
	// one JSON-RPC call and is cancelled the moment Run returns, which would
	// kill the turn before its first token.
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	if err := s.runs.start(projectID, runID, cancel); err != nil {
		cancel()
		if errors.Is(err, ErrBusy) {
			return "", &rpc.ReasonError{Reason: rpc.ReasonAgentBusy, Err: err}
		}
		return "", err
	}

	if err := s.tr.appendUser(runCtx, projectID, req.NodeID, runID, req.Prompt); err != nil {
		s.runs.finish(projectID, runID)
		cancel()
		return "", err
	}

	go func() {
		defer cancel()
		defer s.runs.finish(projectID, runID)
		s.loop(runCtx, loopState{
			runID:    runID,
			req:      req,
			client:   client,
			model:    resolved.Model,
			session:  tools,
			schemas:  schemas,
			language: s.language(),
		})
	}()
	return runID, nil
}

type loopState struct {
	runID    string
	req      RunRequest
	client   llm.Client
	model    string
	session  *toolSession
	schemas  []llm.ToolSchema
	language string
}

type deltaPayload struct {
	RunID string `json:"run_id"`
	Text  string `json:"text"`
}

type toolPayload struct {
	RunID   string   `json:"run_id"`
	Name    string   `json:"name"`
	State   string   `json:"state"` // started | done | error
	Summary string   `json:"summary,omitempty"`
	BatchID string   `json:"batch_id,omitempty"`
	NodeIDs []string `json:"node_ids,omitempty"`
}

type usagePayload struct {
	Input  int `json:"input"`
	Output int `json:"output"`
}

type donePayload struct {
	RunID string       `json:"run_id"`
	Model string       `json:"model,omitempty"`
	Usage usagePayload `json:"usage"`
}

type errorPayload struct {
	RunID   string `json:"run_id"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type cancelledPayload struct {
	RunID string `json:"run_id"`
}

// loop is the turn. It ends when the model stops asking for tools, when it
// hits the iteration cap, when the same tool fails repeatedly, or when the
// writer cancels.
func (s *Service) loop(ctx context.Context, st loopState) {
	msgs := s.openingMessages(ctx, st)

	var usage usagePayload
	lastToolError := ""
	repeats := 0

	for iter := 0; iter < maxIterations; iter++ {
		resp, err := st.client.Chat(ctx, msgs, llm.ChatOptions{
			Tools:   st.schemas,
			OnDelta: func(text string) { s.notify("agent.delta", deltaPayload{st.runID, text}) },
		})
		if err != nil {
			s.endWithError(ctx, st, err)
			return
		}
		usage.Input += resp.Usage.InputTokens
		usage.Output += resp.Usage.OutputTokens

		if text := strings.TrimSpace(resp.Message.Content); text != "" {
			if err := s.tr.appendAssistant(ctx, st.req.ProjectID, st.req.NodeID, st.runID,
				resp.Message.Content, companion.HistoryStatusDone); err != nil {
				logf("transcript: %v", err)
			}
		}

		if len(resp.Message.ToolCalls) == 0 {
			s.notify("agent.done", donePayload{RunID: st.runID, Model: st.model, Usage: usage})
			return
		}

		msgs = append(msgs, resp.Message)
		for _, call := range resp.Message.ToolCalls {
			if ctx.Err() != nil {
				s.endCancelled(ctx, st)
				return
			}
			result := s.runTool(ctx, st, call)
			msgs = append(msgs, llm.ChatMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    result.Text,
			})

			if !result.IsError {
				lastToolError, repeats = "", 0
				continue
			}
			signature := call.Name + "\x00" + result.Text
			if signature == lastToolError {
				repeats++
			} else {
				lastToolError, repeats = signature, 1
			}
			if repeats >= maxRepeatedToolErrors {
				s.endWithReason(ctx, st, rpc.ReasonAgentIterationLimit,
					fmt.Sprintf("%s failed the same way %d times running", call.Name, repeats))
				return
			}
		}
	}

	s.endWithReason(ctx, st, rpc.ReasonAgentIterationLimit,
		fmt.Sprintf("stopped after %d tool calls in one turn", maxIterations))
}

// openingMessages is system prompt + budgeted history + the scope-prefixed
// message. History is loaded BEFORE the user row is counted, because Run has
// already written it — replaying it here would double it.
func (s *Service) openingMessages(ctx context.Context, st loopState) []llm.ChatMessage {
	msgs := []llm.ChatMessage{{Role: "system", Content: systemPrompt(st.language)}}

	prior, err := s.tr.load(ctx, st.req.ProjectID, 200)
	if err != nil {
		logf("history: %v", err)
	}
	// Drop the row Run just appended: it is delivered below, with its scope line.
	if n := len(prior); n > 0 && prior[n-1].RunID == st.runID && prior[n-1].Role == "user" {
		prior = prior[:n-1]
	}
	msgs = append(msgs, priorMessages(prior)...)

	scope := scopeLine(ctx, s.deps.Scope, st.req.ProjectID, st.req.NodeID)
	return append(msgs, llm.ChatMessage{
		Role:    "user",
		Content: scope + "\n\n" + st.req.Prompt,
	})
}

// runTool executes one call, telling the panel before and after. The activity
// log entry is written server-side by mcphost.record, which reads the run id
// off the request's _meta.
func (s *Service) runTool(ctx context.Context, st loopState, call llm.ToolCall) toolResult {
	s.notify("agent.tool", toolPayload{
		RunID: st.runID, Name: call.Name, State: "started",
		Summary: summarizeArgs(call.Arguments),
	})

	result := st.session.call(ctx, st.runID, call.Name, call.Arguments)

	state := "done"
	if result.IsError {
		state = "error"
	}
	s.notify("agent.tool", toolPayload{
		RunID: st.runID, Name: call.Name, State: state,
		Summary: summarize(result.Text), BatchID: result.BatchID, NodeIDs: result.NodeIDs,
	})
	if err := s.tr.appendToolEvent(ctx, st.req.ProjectID, st.req.NodeID, st.runID, toolEvent{
		Name: call.Name, Summary: summarize(result.Text), OK: !result.IsError,
		BatchID: result.BatchID, NodeIDs: result.NodeIDs,
	}); err != nil {
		logf("transcript: %v", err)
	}
	return result
}

func (s *Service) endCancelled(ctx context.Context, st loopState) {
	if err := s.tr.markRun(ctx, st.runID, companion.HistoryStatusCancelled); err != nil {
		logf("transcript: %v", err)
	}
	s.notify("agent.cancelled", cancelledPayload{st.runID})
}

// endWithError maps a provider failure to a reason code. The provider's own
// body never becomes UI text — v0.8.5's lesson.
func (s *Service) endWithError(ctx context.Context, st loopState, err error) {
	if errors.Is(err, context.Canceled) || ctx.Err() != nil {
		s.endCancelled(ctx, st)
		return
	}
	reason := rpc.ReasonProviderUnreachable
	var re *rpc.ReasonError
	if errors.As(err, &re) {
		reason = re.Reason
	}
	s.endWithReason(ctx, st, reason, err.Error())
}

func (s *Service) endWithReason(ctx context.Context, st loopState, reason, message string) {
	if err := s.tr.markRun(ctx, st.runID, companion.HistoryStatusFailed); err != nil {
		logf("transcript: %v", err)
	}
	s.notify("agent.error", errorPayload{RunID: st.runID, Reason: reason, Message: message})
}

func summarize(s string) string {
	const max = 160
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

func summarizeArgs(args string) string { return summarize(args) }
```

Add `engine/internal/agent/log.go`:

```go
//go:build !mobile

package agent

import "fmt"

// logf writes agent diagnostics to stdout, the way mcphost does. Failures
// here are never fatal to a turn: a transcript row that did not save must not
// cost the writer the reply it belonged to.
func logf(format string, args ...any) {
	fmt.Printf("agent: "+format+"\n", args...)
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `cd engine && go test ./internal/agent/ -v`
Expected: PASS.

- [ ] **Step 7: Run with the race detector**

Run: `cd engine && go test ./internal/agent/ -race -count=2`
Expected: PASS. The loop goroutine, the notifier and the registry all touch shared state.

- [ ] **Step 8: Verify the dependency gate and the tagged builds**

Run: `./scripts/validate-story-core-deps.sh`
Run: `cd engine && go build ./... && go test ./...`
Run: `make test-mobile-engine`
Run: `cd engine && go build -tags mas ./... && GOOS=windows go build ./...`
Expected: all pass.

- [ ] **Step 9: Commit**

```bash
git add engine/internal/agent/ engine/internal/rpc/reason.go
git commit -m "feat(agent): the turn loop — tools, limits, cancellation and streaming (#93)"
```

---

### Task 6: RPC surface and engine wiring

Five methods, a mobile twin, and the second `ToolDeps` that makes the agent a first-class client of its own server.

**Files:**
- Create: `engine/internal/rpc/handlers/agent.go`
- Create: `engine/internal/rpc/handlers/agent_test.go`
- Create: `engine/internal/engineapp/agent_enabled.go`
- Create: `engine/internal/engineapp/agent_disabled.go`
- Modify: `engine/internal/engineapp/mcp_enabled.go` (return an agent-side registrar)
- Modify: `engine/internal/engineapp/mcp_disabled.go` (mirror the shape)
- Modify: `engine/internal/engineapp/engineapp.go` (wire and register)

**Interfaces:**
- Consumes: `agent.Service` and its methods (Task 5), `rpc.ReasonAgentBusy` (Task 5).
- Produces:
  - `handlers.AgentController` interface: `Run(ctx, projectID, nodeID, prompt string) (string, error)`, `Cancel(ctx, runID string) error`, `History(ctx, projectID string, limit int) (json.RawMessage, error)`, `Clear(ctx, projectID string) error`, `Undo(ctx, batchID string) error`
  - `handlers.AgentRun/AgentCancel/AgentHistory/AgentClear/AgentUndo(ctrl AgentController) rpc.Handler`
  - `engineapp.agentController` (both build tags)
  - `setupMCP` returns `(*mcpController, mcphost.ToolDeps, func() error)` — the deps template the agent clones

- [ ] **Step 1: Write the failing handler test**

Create `engine/internal/rpc/handlers/agent_test.go`:

```go
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/rpc"
)

type fakeAgent struct {
	runID     string
	runErr    error
	sawPrompt string
	sawNode   string
	cancelled string
	cleared   string
	undone    string
}

func (f *fakeAgent) Run(_ context.Context, projectID, nodeID, prompt string) (string, error) {
	f.sawPrompt, f.sawNode = prompt, nodeID
	return f.runID, f.runErr
}
func (f *fakeAgent) Cancel(_ context.Context, runID string) error { f.cancelled = runID; return nil }
func (f *fakeAgent) History(context.Context, string, int) (json.RawMessage, error) {
	return json.RawMessage(`[]`), nil
}
func (f *fakeAgent) Clear(_ context.Context, projectID string) error { f.cleared = projectID; return nil }
func (f *fakeAgent) Undo(_ context.Context, batchID string) error    { f.undone = batchID; return nil }

func TestAgentRun_returnsTheRunID(t *testing.T) {
	f := &fakeAgent{runID: "run-1"}
	out, err := AgentRun(f)(context.Background(),
		json.RawMessage(`{"project_id":"p1","node_id":"n1","prompt":"안녕"}`))
	if err != nil {
		t.Fatalf("AgentRun: %v", err)
	}
	var got struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.RunID != "run-1" {
		t.Errorf("run_id = %q", got.RunID)
	}
	if f.sawPrompt != "안녕" || f.sawNode != "n1" {
		t.Errorf("controller saw prompt=%q node=%q", f.sawPrompt, f.sawNode)
	}
}

// The busy reason has to survive the handler, or the panel prints the
// engine's English sentence instead of a translated one.
func TestAgentRun_carriesTheBusyReasonThrough(t *testing.T) {
	f := &fakeAgent{runErr: &rpc.ReasonError{Reason: rpc.ReasonAgentBusy, Err: errors.New("busy")}}
	_, err := AgentRun(f)(context.Background(), json.RawMessage(`{"project_id":"p1","prompt":"x"}`))
	if err == nil {
		t.Fatal("want an error")
	}
	var me *rpc.MethodError
	if !errors.As(err, &me) {
		t.Fatalf("err = %T, want *rpc.MethodError", err)
	}
	if string(me.Data) != `{"reason":"agent_busy"}` {
		t.Errorf("data = %s", me.Data)
	}
}

func TestAgentRun_requiresAPrompt(t *testing.T) {
	f := &fakeAgent{runID: "run-1"}
	if _, err := AgentRun(f)(context.Background(), json.RawMessage(`{"project_id":"p1","prompt":"  "}`)); err == nil {
		t.Fatal("an empty prompt must be refused before a provider is dialled")
	}
}

func TestAgentCancelAndClearAndUndoReachTheController(t *testing.T) {
	f := &fakeAgent{}
	if _, err := AgentCancel(f)(context.Background(), json.RawMessage(`{"run_id":"run-3"}`)); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := AgentClear(f)(context.Background(), json.RawMessage(`{"project_id":"p1"}`)); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, err := AgentUndo(f)(context.Background(), json.RawMessage(`{"batch_id":"b1"}`)); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if f.cancelled != "run-3" || f.cleared != "p1" || f.undone != "b1" {
		t.Errorf("controller saw cancel=%q clear=%q undo=%q", f.cancelled, f.cleared, f.undone)
	}
}

func TestAgentHistory_returnsAJSONArray(t *testing.T) {
	out, err := AgentHistory(&fakeAgent{})(context.Background(), json.RawMessage(`{"project_id":"p1"}`))
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if string(out) != `[]` {
		t.Errorf("out = %s", out)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd engine && go test ./internal/rpc/handlers/ -run TestAgent -v`
Expected: FAIL — `undefined: AgentRun`.

- [ ] **Step 3: Write the handlers**

Create `engine/internal/rpc/handlers/agent.go`:

```go
package handlers

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// AgentController is the slice of the built-in agent the RPC layer needs.
// An interface with plain types for two reasons: this file compiles on every
// build tag (the agent itself is //go:build !mobile), and handlers must never
// link tars/pkg/llm.
type AgentController interface {
	Run(ctx context.Context, projectID, nodeID, prompt string) (string, error)
	Cancel(ctx context.Context, runID string) error
	History(ctx context.Context, projectID string, limit int) (json.RawMessage, error)
	Clear(ctx context.Context, projectID string) error
	Undo(ctx context.Context, batchID string) error
}

type agentRunParams struct {
	ProjectID string `json:"project_id"`
	NodeID    string `json:"node_id,omitempty"`
	Prompt    string `json:"prompt"`
}

type agentRunResult struct {
	RunID string `json:"run_id"`
}

// AgentRun returns a handler for agent.run. It hands back a run id at once;
// the turn itself arrives as agent.* notifications.
func AgentRun(ctrl AgentController) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p agentRunParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "agent.run: " + err.Error()}
		}
		if strings.TrimSpace(p.Prompt) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "prompt is required"}
		}
		runID, err := ctrl.Run(ctx, p.ProjectID, p.NodeID, p.Prompt)
		if err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		return json.Marshal(agentRunResult{RunID: runID})
	}
}

type agentCancelParams struct {
	RunID string `json:"run_id"`
}

// AgentCancel returns a handler for agent.cancel. Cancelling a run that has
// already finished is not an error — the stop click can land late.
func AgentCancel(ctrl AgentController) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p agentCancelParams
		if len(params) > 0 {
			_ = json.Unmarshal(params, &p)
		}
		if err := ctrl.Cancel(ctx, p.RunID); err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}

type agentHistoryParams struct {
	ProjectID string `json:"project_id"`
	Limit     int    `json:"limit,omitempty"`
}

// AgentHistory returns a handler for agent.history.
func AgentHistory(ctrl AgentController) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p agentHistoryParams
		if len(params) > 0 {
			_ = json.Unmarshal(params, &p)
		}
		out, err := ctrl.History(ctx, p.ProjectID, p.Limit)
		if err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		return out, nil
	}
}

type agentClearParams struct {
	ProjectID string `json:"project_id"`
}

// AgentClear returns a handler for agent.clear. It drops the conversation;
// the activity log is a separate record and stays.
func AgentClear(ctrl AgentController) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p agentClearParams
		if len(params) > 0 {
			_ = json.Unmarshal(params, &p)
		}
		if err := ctrl.Clear(ctx, p.ProjectID); err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}

type agentUndoParams struct {
	BatchID string `json:"batch_id"`
}

// AgentUndo returns a handler for agent.undo: the panel's revert button.
func AgentUndo(ctrl AgentController) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p agentUndoParams
		if len(params) > 0 {
			_ = json.Unmarshal(params, &p)
		}
		if strings.TrimSpace(p.BatchID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "batch_id is required"}
		}
		if err := ctrl.Undo(ctx, p.BatchID); err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}
```

- [ ] **Step 4: Run the handler tests to verify they pass**

Run: `cd engine && go test ./internal/rpc/handlers/ -run TestAgent -v`
Expected: PASS.

- [ ] **Step 5: Have `setupMCP` hand back the deps template**

`register()` in `engineapp.go` is compiled on every build tag, so it can only name a type that exists on every build tag. `mcphost.ToolDeps` does not — mobile never links `mcphost`. Both twins therefore declare the same alias name over a different underlying type, and `register()` only ever passes the value along.

In `engine/internal/engineapp/mcp_enabled.go`, add the alias above `setupMCP` and widen the signature:

```go
// agentToolDeps is what setupMCP hands the built-in agent to clone for its
// own in-memory server (#93). An alias rather than the type itself because
// register() is compiled on every build tag and mobile has no mcphost.
type agentToolDeps = mcphost.ToolDeps

// setupMCP builds the external HTTP host and returns the tool deps it was
// built from, so the agent's server is wired from the same place rather than
// from a second copy of this list.
func setupMCP(deps mcpDeps) (*mcpController, agentToolDeps, func() error) {
```

and change the final return:

```go
	return ctrl, tools, host.Stop
}
```

In `engine/internal/engineapp/mcp_disabled.go`, declare the mobile side of the alias and mirror the signature:

```go
// agentToolDeps is empty on mobile: the MCP tool layer is not compiled, so
// there is nothing to hand the agent — which is also not compiled.
type agentToolDeps = struct{}

func setupMCP(mcpDeps) (*mcpController, agentToolDeps, func() error) {
	return &mcpController{}, agentToolDeps{}, func() error { return nil }
}
```

- [ ] **Step 6: Write the agent controller twins**

Create `engine/internal/engineapp/agent_enabled.go`:

```go
//go:build !mobile

package engineapp

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/agent"
	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/mcphost"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/provider"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/storyops"
)

const agentAvailable = true

// agentDeps is what setupAgent needs from register().
type agentDeps struct {
	tools    mcphost.ToolDeps // the external host's deps, cloned below
	story    *storyops.Service
	history  *companion.HistoryRepo
	projects *project.Repo
	nodes    *node.Repo
	settings *settings.Store
	src      *provider.Source
	notify   func(method string, params any)
	clock    func() int64
}

// scopeLookup answers the two names the scope line carries. Failures are
// silent on purpose: a missing title must not cost the writer their turn.
type scopeLookup struct {
	projects *project.Repo
	nodes    *node.Repo
}

func (s scopeLookup) ProjectTitle(ctx context.Context, id string) string {
	p, err := s.projects.Get(ctx, id)
	if err != nil {
		return ""
	}
	return p.Title
}

func (s scopeLookup) NodeLabel(ctx context.Context, id string) string {
	if id == "" {
		return ""
	}
	n, err := s.nodes.Get(ctx, id)
	if err != nil {
		return ""
	}
	if n.Title != "" {
		return n.Title
	}
	return n.Label
}

// agentController adapts *agent.Service to handlers.AgentController.
type agentController struct {
	svc *agent.Service
}

func setupAgent(deps agentDeps) (*agentController, func() error) {
	// The agent's own tool deps: same repos, three deliberate differences.
	//
	//  - Source marks every row it writes in the activity log, and exempts it
	//    from the mcp_project_id restriction meant for external clients.
	//  - Its own limiter: sharing the external one lets a busy Claude Desktop
	//    session starve the panel, and vice versa.
	//  - Its own storyops service: undo batches live in memory on the service,
	//    so this is what stops the panel's undo button reverting an external
	//    agent's batch.
	tools := deps.tools
	tools.Source = mcphost.SourceAgent
	tools.Limiter = mcphost.NewLimiter()
	tools.Story = deps.story

	svc := agent.New(agent.Deps{
		Providers: deps.src,
		History:   deps.history,
		Scope:     scopeLookup{projects: deps.projects, nodes: deps.nodes},
		Register: func(s *mcp.Server) {
			// Always full: an agent that cannot write the manuscript has no
			// reason to exist. settings.MCPMode governs external clients only.
			tools.Register(s, settings.MCPModeFull)
		},
		Notify:   deps.notify,
		Language: deps.settings.Language,
		Undo: func(ctx context.Context, batchID string) error {
			return deps.story.UndoApply(ctx, batchID, deps.clock)
		},
		Clock: deps.clock,
	})
	return &agentController{svc: svc}, svc.Close
}

func (c *agentController) Run(ctx context.Context, projectID, nodeID, prompt string) (string, error) {
	return c.svc.Run(ctx, agent.RunRequest{ProjectID: projectID, NodeID: nodeID, Prompt: prompt})
}

func (c *agentController) Cancel(_ context.Context, runID string) error {
	return c.svc.Cancel(runID)
}

func (c *agentController) History(ctx context.Context, projectID string, limit int) (json.RawMessage, error) {
	msgs, err := c.svc.History(ctx, projectID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]agentHistoryRow, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, agentHistoryRow{
			ID: m.ID, ProjectID: m.ProjectID, NodeID: m.NodeID, NodeLabel: m.NodeLabel,
			RunID: m.RunID, Role: m.Role, Status: m.Status, Content: m.Content, CreatedAt: m.CreatedAt,
		})
	}
	return json.Marshal(out)
}

// agentHistoryRow is the panel's shape. companion.HistoryMessage has no JSON
// tags — it is an internal row — so it is projected rather than marshalled.
type agentHistoryRow struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	NodeID    string `json:"node_id,omitempty"`
	NodeLabel string `json:"node_label,omitempty"`
	RunID     string `json:"run_id,omitempty"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	Content   string `json:"content"`
	CreatedAt int64  `json:"created_at"`
}

func (c *agentController) Clear(ctx context.Context, projectID string) error {
	return c.svc.Clear(ctx, projectID)
}

func (c *agentController) Undo(ctx context.Context, batchID string) error {
	return c.svc.Undo(ctx, batchID)
}
```

Create `engine/internal/engineapp/agent_disabled.go`:

```go
//go:build mobile

package engineapp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/provider"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/storyops"
)

// The built-in agent needs the MCP tool layer, which mobile does not compile.
// The controller stays so register() is identical on every build and the
// frontend gets a clear refusal rather than a missing method.
const agentAvailable = false

type agentDeps struct {
	tools    agentToolDeps
	story    *storyops.Service
	history  *companion.HistoryRepo
	projects *project.Repo
	nodes    *node.Repo
	settings *settings.Store
	src      *provider.Source
	notify   func(method string, params any)
	clock    func() int64
}

type agentController struct{}

var errAgentUnavailable = errors.New("the built-in agent is not available in this build")

func setupAgent(agentDeps) (*agentController, func() error) {
	return &agentController{}, func() error { return nil }
}

func (c *agentController) Run(context.Context, string, string, string) (string, error) {
	return "", errAgentUnavailable
}
func (c *agentController) Cancel(context.Context, string) error { return nil }
func (c *agentController) History(context.Context, string, int) (json.RawMessage, error) {
	return json.RawMessage(`[]`), nil
}
func (c *agentController) Clear(context.Context, string) error { return nil }
func (c *agentController) Undo(context.Context, string) error  { return errAgentUnavailable }
```

- [ ] **Step 7: Wire it in `engineapp.go`**

Change the `setupMCP` call site to take the third return:

```go
	mcpCtrl, mcpTools, stopMCP := setupMCP(mcpDeps{
```

After the provider block (`providers := providerService{src: providerSrc}`), add:

```go
	// The built-in agent (#93). A second storyops service of its own, for the
	// same reason the MCP host has one: undo batches live in memory on the
	// service, so the panel's undo button can only revert what the panel did.
	agentStory := storyops.New(projects, nodes, threads, beats, entities, relationships).
		WithFacts(facts).
		WithSnapshots(snaps).
		WithMemory(companionSvc)

	agentCtrl, stopAgent := setupAgent(agentDeps{
		tools:    mcpTools,
		story:    agentStory,
		history:  companionHistory,
		projects: projects,
		nodes:    nodes,
		settings: settingsStore,
		src:      providerSrc,
		notify:   func(method string, params any) { _ = s.Notifier().Notify(method, params) },
		clock:    clock,
	})
	a.closers = append(a.closers, stopAgent)
```

Keep the provider source on `App` so Task 7 can reach it. In the `App` struct in `engineapp.go`:

```go
type App struct {
	server  *rpc.Server
	cancel  context.CancelFunc
	closers []func() error
	once    sync.Once
	// providerSrc is kept for the test seam in agent_enabled.go: the agent's
	// loop is only testable if its client can be replaced without a network.
	providerSrc *provider.Source
}
```

and set it where the source is built: `a.providerSrc = providerSrc`.

Add the capability flag to `engine/internal/rpc/handlers/diagnostics.go`:

```go
type Capabilities struct {
	GitSyncAvailable bool
	MCPAvailable     bool
	AgentAvailable   bool
}
```

surface it in `diagnosticsPayload` the way `MCPAvailable` is surfaced, pass `AgentAvailable: agentAvailable` in the `caps` literal, then register the methods after the codex block:

```go
	s.Handle("agent.run", handlers.AgentRun(agentCtrl))
	s.Handle("agent.cancel", handlers.AgentCancel(agentCtrl))
	s.Handle("agent.history", handlers.AgentHistory(agentCtrl))
	s.Handle("agent.clear", handlers.AgentClear(agentCtrl))
	s.Handle("agent.undo", handlers.AgentUndo(agentCtrl))
```

- [ ] **Step 8: Build and run the whole suite**

Run: `cd engine && go build ./... && go test ./...`
Run: `make test-mobile-engine`
Run: `cd engine && go build -tags mas ./... && GOOS=windows go build ./...`
Run: `./scripts/validate-story-core-deps.sh`
Expected: all pass.

- [ ] **Step 9: Commit**

```bash
git add engine/internal/rpc/handlers/agent.go engine/internal/rpc/handlers/agent_test.go engine/internal/engineapp/
git commit -m "feat(engine): agent.* RPC surface, wired to a second in-memory tool server (#93)"
```

---

### Task 7: The integration test

The one test that proves the design. A scripted model, the **real** in-memory MCP server, and **real** repositories: read the context, write a scene, refresh the summary — then check the manuscript actually changed, the activity log says `source=agent` with the run id, `mcp.changed` fired, and the transcript holds the turn.

**Files:**
- Create: `engine/internal/engineapp/agent_wiring_test.go`
- Modify: `engine/internal/engineapp/agent_enabled.go` (a test seam for the provider)

**Interfaces:**
- Consumes: everything from Tasks 1–6.
- Produces:
  - `(*App).SetProviderFactoryForTest(f provider.ClientFactory)`
  - `(*App).CaptureNotificationsForTest() *notificationLog`
  - `notificationLog` with `add(method string)` / `saw(method string) bool`

- [ ] **Step 1: Write the failing test**

Create `engine/internal/engineapp/agent_wiring_test.go`:

```go
//go:build !mobile

package engineapp

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/tars/pkg/llm"
)

// scriptedModel plays a fixed sequence of turns: read the context, write a
// scene, then say it is done. It is the only fake in this test — the MCP
// server, the tools and the database are real.
type scriptedModel struct {
	mu    sync.Mutex
	turn  int
	steps []llm.ChatResponse
}

func (m *scriptedModel) Ask(context.Context, string) (string, error) { return "", nil }

func (m *scriptedModel) Chat(ctx context.Context, _ []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return llm.ChatResponse{}, err
	}
	i := m.turn
	if i >= len(m.steps) {
		i = len(m.steps) - 1
	}
	m.turn++
	resp := m.steps[i]
	if opts.OnDelta != nil && resp.Message.Content != "" {
		opts.OnDelta(resp.Message.Content)
	}
	return resp, nil
}

func toolTurn(name, args string) llm.ChatResponse {
	return llm.ChatResponse{Message: llm.ChatMessage{
		Role:      "assistant",
		ToolCalls: []llm.ToolCall{{ID: "c" + name, Name: name, Arguments: args}},
	}}
}

// The full loop against the real tool layer: the agent reads the work, writes
// a scene, and everything the writer relies on afterwards is in place.
func TestAgentRun_writesThroughTheRealMCPToolsAndIsAudited(t *testing.T) {
	app := openApp(t)

	// A real work with a real scene.
	projectID, nodeID := seedProjectWithScene(t, app)

	consent := `{"provider":"anthropic","providers":{"anthropic":{"api_key":"sk-test","consented_at":1700000000000}}}`
	if _, rpcErr := call(t, app, "settings.set", consent); rpcErr != nil {
		t.Fatalf("settings.set: %+v", rpcErr)
	}

	// The scenario from the issue: read the context, write the scene, refresh
	// the summary. A scripted model cannot read a tool's reply, so the
	// versions are hard-coded — a fresh scene is at content_version 0, and
	// the write above bumps it to 1. If write_scene ever stops bumping the
	// version, this test is what notices.
	model := &scriptedModel{steps: []llm.ChatResponse{
		toolTurn("linetta_get_story_context", `{"project_id":"`+projectID+`","node_id":"`+nodeID+`"}`),
		toolTurn("linetta_write_scene",
			`{"node_id":"`+nodeID+`","text":"비가 내렸다. 그는 우산을 펴지 않았다.","expected_content_version":0}`),
		toolTurn("linetta_write_summary",
			`{"node_id":"`+nodeID+`","summary":"비 오는 날 우산을 펴지 않는 남자가 등장한다.","expected_content_version":1}`),
		{Message: llm.ChatMessage{Role: "assistant", Content: "1장을 썼습니다."}},
	}}
	app.SetProviderFactoryForTest(func(llm.ProviderOptions) (llm.Client, error) { return model, nil })

	changed := app.CaptureNotificationsForTest()

	out, rpcErr := call(t, app, "agent.run",
		`{"project_id":"`+projectID+`","node_id":"`+nodeID+`","prompt":"1장 초고를 써줘"}`)
	if rpcErr != nil {
		t.Fatalf("agent.run: %+v", rpcErr)
	}
	var run struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(out, &run); err != nil {
		t.Fatalf("decode run: %v", err)
	}
	if run.RunID == "" {
		t.Fatal("agent.run returned no run id")
	}

	waitForNotification(t, changed, "agent.done")

	// 1. The manuscript really changed.
	body := nodeContent(t, app, nodeID)
	if !strings.Contains(body, "비가 내렸다") {
		t.Errorf("the scene was not written: %q", body)
	}

	// 2. The activity log knows who did it and which turn it belonged to.
	actRaw, rpcErr := call(t, app, "mcp.activity", `{"limit":50}`)
	if rpcErr != nil {
		t.Fatalf("mcp.activity: %+v", rpcErr)
	}
	var entries []struct {
		Tool   string `json:"tool"`
		Source string `json:"source"`
		RunID  string `json:"run_id"`
	}
	if err := json.Unmarshal(actRaw, &entries); err != nil {
		t.Fatalf("decode activity: %v", err)
	}
	var wrote bool
	for _, e := range entries {
		if e.Tool != "linetta_write_scene" {
			continue
		}
		wrote = true
		if e.Source != "agent" {
			t.Errorf("activity source = %q, want agent", e.Source)
		}
		if e.RunID != run.RunID {
			t.Errorf("activity run_id = %q, want %q", e.RunID, run.RunID)
		}
	}
	if !wrote {
		t.Error("the write never reached the activity log")
	}

	// 3. The UI refresh path fired — the agent's writes reuse mcp.changed
	//    rather than adding a second refresh channel.
	if !changed.saw("mcp.changed") {
		t.Error("no mcp.changed; the workspace would show stale text")
	}

	// 4. The panel can restore the turn.
	histRaw, rpcErr := call(t, app, "agent.history", `{"project_id":"`+projectID+`","limit":50}`)
	if rpcErr != nil {
		t.Fatalf("agent.history: %+v", rpcErr)
	}
	var hist []struct {
		Role  string `json:"role"`
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(histRaw, &hist); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	roles := map[string]int{}
	for _, m := range hist {
		roles[m.Role]++
	}
	if roles["user"] != 1 || roles["tool"] != 3 || roles["assistant"] < 1 {
		t.Errorf("transcript roles = %v, want one prompt, three tool events and a reply", roles)
	}
}

// Without consent nothing is constructed that could send a byte.
func TestAgentRun_refusesWithoutProviderConsent(t *testing.T) {
	app := openApp(t)
	projectID, _ := seedProjectWithScene(t, app)
	noConsent := `{"provider":"anthropic","providers":{"anthropic":{"api_key":"sk-test"}}}`
	if _, rpcErr := call(t, app, "settings.set", noConsent); rpcErr != nil {
		t.Fatalf("settings.set: %+v", rpcErr)
	}
	app.SetProviderFactoryForTest(func(llm.ProviderOptions) (llm.Client, error) {
		t.Fatal("a client must not be built without consent")
		return nil, nil
	})
	_, rpcErr := call(t, app, "agent.run", `{"project_id":"`+projectID+`","prompt":"써줘"}`)
	if rpcErr == nil {
		t.Fatal("agent.run without consent must fail")
	}
	if !strings.Contains(string(rpcErr.Data), "provider_consent_required") {
		t.Errorf("error data = %s, want a provider_consent_required reason", rpcErr.Data)
	}
}

// External MCP being off must not disable the panel: the built-in agent is a
// client of a second, in-memory server that never binds a port.
func TestAgentRun_worksWhileExternalMCPIsOff(t *testing.T) {
	app := openApp(t)
	projectID, nodeID := seedProjectWithScene(t, app)

	st, rpcErr := call(t, app, "mcp.status", "")
	if rpcErr != nil {
		t.Fatalf("mcp.status: %+v", rpcErr)
	}
	var status mcpStatus
	if err := json.Unmarshal(st, &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status.Running {
		t.Fatal("this test needs external MCP off")
	}

	consent := `{"provider":"anthropic","providers":{"anthropic":{"api_key":"sk-test","consented_at":1700000000000}}}`
	if _, rpcErr := call(t, app, "settings.set", consent); rpcErr != nil {
		t.Fatalf("settings.set: %+v", rpcErr)
	}
	model := &scriptedModel{steps: []llm.ChatResponse{
		toolTurn("linetta_read_scene", `{"node_id":"`+nodeID+`"}`),
		{Message: llm.ChatMessage{Role: "assistant", Content: "읽었습니다."}},
	}}
	app.SetProviderFactoryForTest(func(llm.ProviderOptions) (llm.Client, error) { return model, nil })
	changed := app.CaptureNotificationsForTest()

	if _, rpcErr := call(t, app, "agent.run", `{"project_id":"`+projectID+`","prompt":"읽어줘"}`); rpcErr != nil {
		t.Fatalf("agent.run: %+v", rpcErr)
	}
	waitForNotification(t, changed, "agent.done")
}
```

Add the helpers to the same file:

```go
// seedProjectWithScene creates a work and returns it with its first scene.
// projects.create seeds an outline and reports the opened scene directly, so
// there is no tree to walk — mcp_write_test.go's fixture does the same.
func seedProjectWithScene(t *testing.T, app *App) (string, string) {
	t.Helper()
	out, rpcErr := call(t, app, "projects.create",
		`{"title":"테스트 작품","genres":["fantasy"],"length_target":"short","default_pov":"first"}`)
	if rpcErr != nil {
		t.Fatalf("projects.create: %+v", rpcErr)
	}
	var proj struct {
		ID               string  `json:"id"`
		LastOpenedNodeID *string `json:"last_opened_node_id"`
	}
	if err := json.Unmarshal(out, &proj); err != nil {
		t.Fatalf("decode project: %v", err)
	}
	if proj.LastOpenedNodeID == nil {
		t.Fatal("projects.create seeded no scene to write into")
	}
	return proj.ID, *proj.LastOpenedNodeID
}

// nodeContent returns a scene's stored body. content_doc is the editor's own
// JSON document, so the prose is asserted with Contains rather than equality.
func nodeContent(t *testing.T, app *App, nodeID string) string {
	t.Helper()
	out, rpcErr := call(t, app, "nodes.get", `{"id":"`+nodeID+`"}`)
	if rpcErr != nil {
		t.Fatalf("nodes.get: %+v", rpcErr)
	}
	var n struct {
		ContentDoc *string `json:"content_doc"`
	}
	if err := json.Unmarshal(out, &n); err != nil {
		t.Fatalf("decode node: %v", err)
	}
	if n.ContentDoc == nil {
		return ""
	}
	return *n.ContentDoc
}

func waitForNotification(t *testing.T, log *notificationLog, method string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !log.saw(method) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", method)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
```

`notificationLog` itself is declared in `agent_enabled.go` (Step 3), not here — a non-test file returns it, so it cannot live in a `_test.go`.

- [ ] **Step 2: Run it to verify it fails**

Run: `cd engine && go test ./internal/engineapp/ -run TestAgentRun -v`
Expected: FAIL — `app.SetProviderFactoryForTest undefined`.

- [ ] **Step 3: Add the two test seams**

Append to `engine/internal/engineapp/agent_enabled.go` (a `!mobile` file — the agent does not exist on mobile, and neither should its seams):

```go
// SetProviderFactoryForTest replaces the client factory so a test can drive
// the loop without a network. Test-only: production wires llm.NewProvider in
// provider.NewSource.
func (a *App) SetProviderFactoryForTest(f provider.ClientFactory) {
	a.providerSrc.WithFactory(f)
}

// notificationLog records what the engine emitted. The agent's contract is
// largely a notification contract — a run id comes back immediately and
// everything after it arrives as agent.* — so a test that cannot see the
// notifications can only assert on side effects, and would pass on an engine
// that did the work but told the panel nothing.
type notificationLog struct {
	mu     sync.Mutex
	events []string
}

func (n *notificationLog) add(method string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.events = append(n.events, method)
}

func (n *notificationLog) saw(method string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, e := range n.events {
		if e == method {
			return true
		}
	}
	return false
}

// CaptureNotificationsForTest routes notifications into a log. It REPLACES
// the current notifier rather than chaining: in a test the notifier is the
// stdio default, which nothing is reading, and a getter on rpc.Server exists
// only to serve a chain nobody needs.
func (a *App) CaptureNotificationsForTest() *notificationLog {
	log := &notificationLog{}
	a.SetNotifier(func(method string, _ json.RawMessage) { log.add(method) })
	return log
}
```

`sync` is already imported by `engineapp.go` but not by `agent_enabled.go` — add it there.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd engine && go test ./internal/engineapp/ -run TestAgentRun -v`
Expected: PASS (three).

- [ ] **Step 5: Run the whole suite with the race detector**

Run: `cd engine && go test ./... -race`
Expected: PASS.

- [ ] **Step 6: Run the tagged builds**

Run: `make test-mobile-engine`
Run: `cd engine && go build -tags mas ./... && GOOS=windows go build ./...`
Run: `cd engine && GOOS=windows go test -c ./internal/engineapp/ -o /dev/null`
Expected: all pass. The last one is cheap insurance: CI runs this package's tests on Windows.

- [ ] **Step 7: Commit**

```bash
git add engine/internal/engineapp/
git commit -m "test(agent): the full turn against the real MCP tools and real repos (#93)"
```

---

### Task 8: Desktop plumbing

The engine can now do everything; the shell cannot yet let it. Three lists and one map, all of which fail silently when missed.

**Files:**
- Modify: `apps/desktop/src-tauri/src/lib.rs` (`RENDERER_ENGINE_METHODS`)
- Modify: `apps/desktop/src-tauri/src/ffi.rs` (`notification_event` + its test)
- Modify: `apps/desktop/src/lib/rpcMessage.ts`
- Modify: `apps/desktop/src/lib/i18n.tsx` (all three catalogues)
- Test: `apps/desktop/src/lib/rpcMessage.test.ts` (existing file — add cases)

**Interfaces:**
- Consumes: `rpc.ReasonAgentBusy`, `rpc.ReasonAgentIterationLimit` (Task 5); the `agent.*` methods (Task 6) and notifications (Task 5).
- Produces: nothing further tasks depend on. This is the last task.

- [ ] **Step 1: Write the failing Rust test**

In `apps/desktop/src-tauri/src/ffi.rs`, extend the existing test module:

```rust
    #[test]
    fn agent_notifications_are_forwarded_to_the_renderer() {
        // The panel is driven entirely by these. A method missing here fails
        // silently: the engine emits, nothing listens, and the panel hangs on
        // a reply that already arrived.
        assert_eq!(notification_event("agent.delta"), Some("agent-delta"));
        assert_eq!(notification_event("agent.tool"), Some("agent-tool"));
        assert_eq!(notification_event("agent.done"), Some("agent-done"));
        assert_eq!(notification_event("agent.error"), Some("agent-error"));
        assert_eq!(notification_event("agent.cancelled"), Some("agent-cancelled"));
        // The removed companion's names must never come back.
        assert_eq!(notification_event("ai.delta"), None);
    }
```

In `apps/desktop/src-tauri/src/lib.rs`, find the test covering `RENDERER_ENGINE_METHODS` (there is one asserting the list is sorted) and add:

```rust
    #[test]
    fn agent_methods_are_reachable_from_the_renderer() {
        for method in [
            "agent.run",
            "agent.cancel",
            "agent.history",
            "agent.clear",
            "agent.undo",
        ] {
            assert!(
                is_renderer_engine_method(method),
                "{method} is not in RENDERER_ENGINE_METHODS; the panel would get a silent refusal"
            );
        }
    }
```

- [ ] **Step 2: Run them to verify they fail**

Run: `cd apps/desktop/src-tauri && cargo test`
Expected: FAIL on both new tests.

- [ ] **Step 3: Add the methods and the event map**

In `apps/desktop/src-tauri/src/lib.rs`, insert into `RENDERER_ENGINE_METHODS` **in sorted position** — the list is binary-searched, so an out-of-order entry is unreachable. `agent.*` sorts before `backup.*`, so these five go at the very top:

```rust
const RENDERER_ENGINE_METHODS: &[&str] = &[
    "agent.cancel",
    "agent.clear",
    "agent.history",
    "agent.run",
    "agent.undo",
    "backup.create_recovery",
```

In `apps/desktop/src-tauri/src/ffi.rs`:

```rust
fn notification_event(method: &str) -> Option<&'static str> {
    match method {
        // An external MCP client changed the manuscript. Without this the
        // writer would keep looking at text the agent already replaced.
        "mcp.changed" => Some("mcp-changed"),
        // The built-in agent's turn (#93). The panel is driven entirely by
        // these: text as it streams, tool activity, and the three ways a turn
        // ends. The removed companion's ai.* names are deliberately not
        // reused — a stale listener must never match a new event.
        "agent.delta" => Some("agent-delta"),
        "agent.tool" => Some("agent-tool"),
        "agent.done" => Some("agent-done"),
        "agent.error" => Some("agent-error"),
        "agent.cancelled" => Some("agent-cancelled"),
        _ => None,
    }
}
```

- [ ] **Step 4: Run the Rust tests to verify they pass**

Run: `cd apps/desktop/src-tauri && cargo test`
Expected: PASS, including the pre-existing sortedness test.

- [ ] **Step 5: Write the failing frontend test**

In `apps/desktop/src/lib/rpcMessage.test.ts`, add:

The file already builds `ko` / `en` / `ja` translators at the top and constructs errors with `new RpcError(method, message, code, data)`. Add inside the existing `describe("rpcErrorMessage", ...)`:

```ts
  it("translates the built-in agent's reason codes in every language", () => {
    // An unmapped code falls through to the engine's English sentence. For
    // the agent that sentence names internal limits; for its provider
    // neighbours it carries the provider's own response body.
    for (const reason of ["agent_busy", "agent_iteration_limit"]) {
      const error = new RpcError("agent.run", "raw engine sentence", -32602, {
        reason,
      });
      for (const t of [ko, en, ja]) {
        const shown = rpcErrorMessage(error, t);
        expect(shown).not.toContain("raw engine sentence");
        expect(shown).not.toContain(reason);
        expect(shown.length).toBeGreaterThan(0);
      }
    }
  });
```

- [ ] **Step 6: Run it to verify it fails**

Run: `cd apps/desktop && pnpm test rpcMessage`
Expected: FAIL — the message still contains the raw code.

- [ ] **Step 7: Map and translate**

In `apps/desktop/src/lib/rpcMessage.ts`, after the Codex entry:

```ts
  // The built-in agent's loop (#93). Busy is a state the writer resolves by
  // waiting or pressing stop; the iteration limit means the turn was cut off
  // partway and the reply says how far it got.
  agent_busy: "errors.agentBusy",
  agent_iteration_limit: "errors.agentIterationLimit",
```

In `apps/desktop/src/lib/i18n.tsx`, add to **all three** catalogues next to `errors.codexPortInUse`:

```ts
// ko
    "errors.agentBusy": "이 작품에서 이미 대화가 진행 중입니다. 끝나기를 기다리거나 중지한 뒤 다시 보내주세요.",
    "errors.agentIterationLimit": "한 번의 대화에서 도구를 너무 많이 사용해 중간에 멈췄습니다. 답변에 어디까지 했는지 적혀 있습니다. 더 작게 나눠서 요청해 보세요.",
```

```ts
// en
    "errors.agentBusy": "This work already has a turn in progress. Wait for it to finish, or stop it and send again.",
    "errors.agentIterationLimit": "The turn used too many tools and was stopped partway. The reply says how far it got — try asking for a smaller piece of work.",
```

```ts
// ja
    "errors.agentBusy": "この作品ではすでにやり取りが進行中です。終わるまで待つか、停止してからもう一度送信してください。",
    "errors.agentIterationLimit": "1回のやり取りでツールを使いすぎたため、途中で停止しました。どこまで進んだかは返信に書かれています。もう少し小さく分けて依頼してみてください。",
```

All three keys must exist in all three catalogues: `MessageKey` is derived from one of them, so a key added to only two fails the type check rather than shipping an untranslated string.

- [ ] **Step 8: Run the frontend tests and the type check**

Run: `cd apps/desktop && pnpm test rpcMessage`
Run: `make test-desktop`
Run: `make test-tauri`
Expected: all pass. `MessageKey` is derived from the catalogue, so a key added to one catalogue and not the others fails the type check.

- [ ] **Step 9: Full verification**

Run: `cd engine && go build ./... && go test ./...`
Run: `make test-mobile-engine`
Run: `cd engine && go build -tags mas ./... && GOOS=windows go build ./...`
Run: `./scripts/validate-story-core-deps.sh`
Run: `make test`
Expected: all pass.

- [ ] **Step 10: Commit**

```bash
git add apps/desktop/src-tauri/src/lib.rs apps/desktop/src-tauri/src/ffi.rs apps/desktop/src/lib/rpcMessage.ts apps/desktop/src/lib/i18n.tsx apps/desktop/src/lib/rpcMessage.test.ts
git commit -m "feat(desktop): reach the agent methods and forward its notifications (#93)"
```

---

## What this plan deliberately leaves out

- **The panel itself is #95.** Nothing in `apps/desktop/src/components` is added here beyond the two reason-code translations. After Task 8 the engine is complete and reachable, and a manual JSON-RPC call is the only way to drive it.
- **Memory (#97) and skills (#98).** `systemPrompt` has no memory block and no skill list. Those tasks extend this function; they do not rewrite it.
- **History compaction.** The budget cuts; it does not summarise. Revisited with session search in sub-project 4.
- **A model catalogue.** `Resolved.Model` empty means tars' own default, exactly as in #91.

## Open questions for #95

Record answers in the panel's plan rather than deciding them here:

- `agent.history` returns the whole project transcript. Should the panel filter by scene the way the companion's `HistoryViewScene` did, or is the work the only useful scope now that the scope line carries the scene?
- A turn that hits `agent_iteration_limit` ends with `agent.error`, but its partial reply is already in the transcript. Does the panel render both, or fold the error into the reply bubble?
- `agent.undo` takes one `batch_id`. A turn with three writes has three batches. Does the panel offer three undo buttons, or should the engine grow an "undo this run" that walks them in reverse?
