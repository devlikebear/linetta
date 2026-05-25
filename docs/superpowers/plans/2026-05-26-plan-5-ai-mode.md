# Plan 5 — AI mode

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire `tars/pkg/llm` into the engine and turn the `편집 / AI` toggle into a real AI assistant. The writer flips to AI mode, types a prompt (or clicks a preset chip — 재작성 / 확장 / 요약), sees the response stream into the workspace, then accepts via `커서에 삽입` / `선택 영역 교체` / `다시 생성` / `버리기`. The engine automatically attaches the current scene body, the previous scene (300-char trim), the mentioned entities, and the project's `style_notes` to every call. Provider is locked to `claude-code-cli` in Plan 5 (the Settings UI to pick between Codex / Claude lands in Plan 6).

**Architecture:** A new `engine/internal/ai` package owns prompt assembly + the active runs registry. The existing `rpc.Server` gains a `Notifier` so a long-running handler can stream JSONRPC notifications (`ai.delta` / `ai.done` / `ai.error`) while a response was already returned with the run id. The Rust shell forwards those notifications as Tauri events (`ai-delta` / `ai-done` / `ai-error`). The Workspace renders the AI mode pane (replaces the Tiptap editor when toggled) and listens for the events to grow the streaming output. After the user picks an action, the Workspace either splices the output into the Tiptap editor (with a snapshot first for `replace`) or discards it; either way the run is recorded in `ai_runs` (already in the schema).

**Tech Stack additions:** none new — `tars/pkg/llm` was pinned in Plan 0 and is finally used here.

**Spec reference:** `docs/superpowers/specs/2026-05-25-linetta-immersive-rebuild-design.md` — §3 (`ai_runs`), §4.4 (AI mode UI), §5.4 (pipeline), §5.5 (presets / options), §5.6 (cancel), §11.1 item 6.

---

## Pre-flight

- [ ] `git describe --tags --exact-match plan-4-mentions-done` returns ok.
- [ ] `git status --short` is empty.
- [ ] `claude` CLI on `PATH` (used by tars `claude-code-cli` provider). Run `claude --version` and confirm a version prints.

---

## File Structure (created or modified)

```
engine/internal/rpc/
  server.go             (modified — Notifier + write mutex)
  server_test.go        (modified — notification round-trip test)

engine/internal/ai/
  ai.go                 (new — domain: Run, Status, Options)
  context.go            (new — buildContext: scene + prev + entities + style_notes)
  context_test.go       (new)
  prompts.go            (new — preset templates + system prompt builder)
  prompts_test.go       (new)
  runner.go             (new — manages active runs, cancellation, streaming)
  runner_test.go        (new — fake provider, verifies notifications + persistence)
  client.go             (new — thin wrapper around llm.NewProvider for claude-code-cli)

engine/internal/store/
  ai_runs_repo.go       (new — Insert/UpdateStatus + ListRecent)
  ai_runs_repo_test.go  (new)

engine/internal/rpc/handlers/
  ai.go                 (new — ai.run / ai.cancel handlers)
  ai_test.go            (new)

engine/cmd/linetta-engine/main.go  (modified — wires ai runner + handlers + notifier)

apps/desktop/src-tauri/src/
  jsonrpc.rs            (modified — forward notifications as Tauri events)
  lib.rs                (modified — emit on engine_notification → ai-delta etc.)

apps/desktop/src/
  lib/types.ts          (extended — AIRunOptions, AIDelta, AIDone, AIError)
  lib/rpc.ts            (extended — ai.run, ai.cancel + engine event hook)
  hooks/useEngineEvent.ts (new — typed Tauri event listener)
  routes/Workspace.tsx  (modified — mode state, AI panel switch, event wiring)
  components/ai/
    AIMode.tsx          (new — prompt + presets + streaming + 4 actions)
    AIMode.css          (new)
    AIContextPanel.tsx  (new — right panel checklist when AI mode is active)
  App.css               (APPEND minor)
```

---

## Task 1: `rpc.Server` gains a Notifier (TDD)

Long-running handlers need to push JSONRPC notifications mid-flight. Currently writes happen only in the read loop after a handler returns. We add a write mutex shared by responses + notifications, plus a `Notifier` that handlers can capture at registration time.

**Files:**
- Modify: `engine/internal/rpc/server.go`
- Modify: `engine/internal/rpc/server_test.go` (append one test)

- [ ] **Step 1: Append a failing test**

Add to `engine/internal/rpc/server_test.go`:

```go
func TestServer_notifierEmitsDuringServe(t *testing.T) {
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"slow"}` + "\n")
	out := &lineCapture{}

	s := NewServer()
	notifier := s.Notifier()
	s.Handle("slow", func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		// Simulate a long-running op that emits two notifications mid-handler.
		_ = notifier.Notify("progress", map[string]any{"step": 1})
		_ = notifier.Notify("progress", map[string]any{"step": 2})
		return json.RawMessage(`"ok"`), nil
	})
	if err := s.Serve(context.Background(), in, out); err != nil && err != io.EOF {
		t.Fatalf("Serve: %v", err)
	}

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3:\n%s", len(lines), out.String())
	}
	if !strings.Contains(lines[0], `"method":"progress"`) || !strings.Contains(lines[0], `"step":1`) {
		t.Errorf("line 0 = %q", lines[0])
	}
	if !strings.Contains(lines[1], `"method":"progress"`) || !strings.Contains(lines[1], `"step":2`) {
		t.Errorf("line 1 = %q", lines[1])
	}
	if !strings.Contains(lines[2], `"id":1`) || !strings.Contains(lines[2], `"result":"ok"`) {
		t.Errorf("line 2 = %q", lines[2])
	}
}

func TestServer_notifyBeforeServe_isError(t *testing.T) {
	s := NewServer()
	if err := s.Notifier().Notify("foo", nil); err == nil {
		t.Error("expected error when Notify is called before Serve")
	}
}
```

- [ ] **Step 2: Run — failures**

```bash
cd engine && go test ./internal/rpc/...
```

- [ ] **Step 3: Implement the Notifier**

Replace the `Server` struct and add the notifier (preserving existing methods):

```go
// Notifier sends JSONRPC notifications on the active stdio connection.
// Returns an error if the server is not currently serving.
type Notifier interface {
	Notify(method string, params any) error
}

// Server is a tiny JSONRPC 2.0 server dispatching to in-process handlers.
// One Server instance is bound to one stdio pair via Serve.
type Server struct {
	mu       sync.RWMutex
	handlers map[string]Handler
	writeMu  sync.Mutex
	codec    *Codec // set during Serve; nil otherwise
}

// NewServer returns a Server with no handlers registered.
func NewServer() *Server { return &Server{handlers: map[string]Handler{}} }

// Notifier returns a handle for sending notifications on the active connection.
// The same handle is valid across multiple Serve calls.
func (s *Server) Notifier() Notifier { return &serverNotifier{s: s} }

type serverNotifier struct{ s *Server }

func (n *serverNotifier) Notify(method string, params any) error {
	var raw json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}
		raw = b
	}
	n.s.writeMu.Lock()
	defer n.s.writeMu.Unlock()
	if n.s.codec == nil {
		return errors.New("rpc: server is not serving")
	}
	return n.s.codec.Write(Message{Method: method, Params: raw})
}
```

In `Serve`, set `s.codec` while running and wrap each response write with `writeMu`:

```go
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	codec := NewCodec(r, w)
	s.writeMu.Lock()
	s.codec = codec
	s.writeMu.Unlock()
	defer func() {
		s.writeMu.Lock()
		s.codec = nil
		s.writeMu.Unlock()
	}()

	write := func(m Message) {
		s.writeMu.Lock()
		defer s.writeMu.Unlock()
		_ = codec.Write(m)
	}

	for {
		msg, err := codec.Read()
		if errors.Is(err, io.EOF) {
			return io.EOF
		}
		if err != nil {
			write(Message{ID: json.RawMessage(`null`), Error: &Error{Code: CodeParseError, Message: err.Error()}})
			continue
		}

		isNotification := len(msg.ID) == 0
		s.mu.RLock()
		h, ok := s.handlers[msg.Method]
		s.mu.RUnlock()

		if !ok {
			if !isNotification {
				write(Message{ID: msg.ID, Error: &Error{Code: CodeMethodNotFound, Message: "method not found: " + msg.Method}})
			}
			continue
		}

		result, herr := h(ctx, msg.Params)
		if isNotification {
			continue
		}
		if herr != nil {
			var me *MethodError
			if errors.As(herr, &me) {
				write(Message{ID: msg.ID, Error: &Error{Code: me.Code, Message: me.Message, Data: me.Data}})
			} else {
				write(Message{ID: msg.ID, Error: &Error{Code: CodeInternalError, Message: herr.Error()}})
			}
			continue
		}
		write(Message{ID: msg.ID, Result: result})
	}
}
```

- [ ] **Step 4: PASS**

```bash
cd engine && go test ./internal/rpc/...
```

- [ ] **Step 5: Commit**

```bash
git add engine/internal/rpc/server.go engine/internal/rpc/server_test.go
git commit -m "feat(rpc): Server.Notifier — JSONRPC notifications during Serve"
```

---

## Task 2: `ai_runs` repo (TDD)

The `ai_runs` table from Plan 1 records every call so the writer can audit what context was sent. Plan 5 implements only Insert + UpdateStatus + ListRecent. Plan 6 may add UI; for now the table is for forensics.

**Files:**
- Create: `engine/internal/store/ai_runs_repo.go`
- Create: `engine/internal/store/ai_runs_repo_test.go`

- [ ] **Step 1: Domain + Repo + tests**

Write `engine/internal/store/ai_runs_repo.go`:

```go
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// AIRunStatus mirrors ai_runs.status values.
type AIRunStatus string

const (
	AIRunStreaming AIRunStatus = "streaming"
	AIRunDone      AIRunStatus = "done"
	AIRunError     AIRunStatus = "error"
	AIRunCancelled AIRunStatus = "cancelled"
)

// AIRun mirrors one row of ai_runs.
type AIRun struct {
	ID          string          `json:"id"`
	ProjectID   string          `json:"project_id"`
	NodeID      *string         `json:"node_id,omitempty"`
	Provider    string          `json:"provider"`
	Prompt      string          `json:"prompt"`
	ContextJSON json.RawMessage `json:"context_json"`
	Output      string          `json:"output"`
	Status      AIRunStatus     `json:"status"`
	ErrorMsg    *string         `json:"error,omitempty"`
	StartedAt   int64           `json:"started_at"`
	EndedAt     *int64          `json:"ended_at,omitempty"`
}

// AIRunsRepo persists ai_runs rows.
type AIRunsRepo struct{ s *Store }

func NewAIRunsRepo(s *Store) *AIRunsRepo { return &AIRunsRepo{s: s} }

// Insert creates the row with status=streaming. The caller (runner) calls
// UpdateStatus once the run terminates.
func (r *AIRunsRepo) Insert(ctx context.Context, run AIRun) error {
	if run.ID == "" || run.ProjectID == "" {
		return errors.New("ai_run: id and project_id required")
	}
	if run.ContextJSON == nil {
		run.ContextJSON = []byte("{}")
	}
	var nodeID any
	if run.NodeID != nil {
		nodeID = *run.NodeID
	}
	_, err := r.s.DB().ExecContext(ctx, `
INSERT INTO ai_runs (id, project_id, node_id, provider, prompt, context_json,
                     output, status, started_at)
VALUES (?, ?, ?, ?, ?, ?, '', ?, ?)`,
		run.ID, run.ProjectID, nodeID, run.Provider, run.Prompt,
		string(run.ContextJSON), string(run.Status), run.StartedAt)
	return err
}

// UpdateStatus finalizes a run. Pass status / output / error / endedAt.
func (r *AIRunsRepo) UpdateStatus(ctx context.Context, id string, status AIRunStatus, output string, errMsg string, endedAt int64) error {
	var errArg any
	if errMsg != "" {
		errArg = errMsg
	}
	_, err := r.s.DB().ExecContext(ctx, `
UPDATE ai_runs SET status = ?, output = ?, error = ?, ended_at = ?
 WHERE id = ?`, string(status), output, errArg, endedAt, id)
	return err
}

// ListRecent returns at most limit recent runs for a project.
func (r *AIRunsRepo) ListRecent(ctx context.Context, projectID string, limit int) ([]AIRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 30
	}
	rows, err := r.s.DB().QueryContext(ctx, `
SELECT id, project_id, node_id, provider, prompt, context_json, output, status,
       error, started_at, ended_at
  FROM ai_runs
 WHERE project_id = ?
 ORDER BY started_at DESC
 LIMIT ?`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AIRun
	for rows.Next() {
		var (
			run         AIRun
			nodeID      sql.NullString
			contextJSON string
			errMsg      sql.NullString
			endedAt     sql.NullInt64
		)
		if err := rows.Scan(&run.ID, &run.ProjectID, &nodeID, &run.Provider,
			&run.Prompt, &contextJSON, &run.Output, &run.Status,
			&errMsg, &run.StartedAt, &endedAt); err != nil {
			return nil, err
		}
		if nodeID.Valid {
			v := nodeID.String
			run.NodeID = &v
		}
		run.ContextJSON = json.RawMessage(contextJSON)
		if errMsg.Valid {
			v := errMsg.String
			run.ErrorMsg = &v
		}
		if endedAt.Valid {
			v := endedAt.Int64
			run.EndedAt = &v
		}
		out = append(out, run)
	}
	return out, rows.Err()
}
```

Write `engine/internal/store/ai_runs_repo_test.go`:

```go
package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestAIRunsRepo_InsertUpdateList(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	r := NewAIRunsRepo(s)
	ctx := context.Background()

	// Need a project row to satisfy the FK.
	if _, err := s.DB().ExecContext(ctx, `
INSERT INTO projects (id, title, genres, length_target, default_pov, created_at, updated_at)
VALUES ('p1', 'T', '["SF"]', 'novel', 'first', 0, 0)`); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	nid := "n1"
	if err := r.Insert(ctx, AIRun{
		ID: "r1", ProjectID: "p1", NodeID: &nid,
		Provider: "claude-code-cli", Prompt: "다시 써줘",
		ContextJSON: json.RawMessage(`{"entities":["해진"]}`),
		Status:      AIRunStreaming,
		StartedAt:   1000,
	}); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if err := r.UpdateStatus(ctx, "r1", AIRunDone, "결과 본문", "", 1500); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	got, err := r.ListRecent(ctx, "p1", 10)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Status != AIRunDone || got[0].Output != "결과 본문" || got[0].EndedAt == nil || *got[0].EndedAt != 1500 {
		t.Errorf("row = %+v", got[0])
	}
}
```

- [ ] **Step 2: PASS + commit**

```bash
cd engine && go test ./internal/store/...
git add engine/internal/store/ai_runs_repo.go engine/internal/store/ai_runs_repo_test.go
git commit -m "feat(store): ai_runs repo (Insert/UpdateStatus/ListRecent)"
```

---

## Task 3: AI context assembly (TDD)

The `ai.context` builder gathers everything the LLM should see for one call.

**Files:**
- Create: `engine/internal/ai/ai.go` (domain types)
- Create: `engine/internal/ai/context.go`
- Create: `engine/internal/ai/context_test.go`

- [ ] **Step 1: `ai.go`**

```go
// Package ai owns prompt assembly and run management for AI mode.
package ai

import "encoding/json"

// Options is the per-call user-selected options.
type Options struct {
	TonePreset bool `json:"tone_preset"` // include style_notes prominently
	ShortForm  bool `json:"short_form"`  // ask for one-paragraph length
}

// Context is the structured payload that prompts.go renders into the
// final prompt. Stored as ai_runs.context_json so the user can later see
// exactly what was sent.
type Context struct {
	ProjectID    string         `json:"project_id"`
	NodeID       string         `json:"node_id"`
	SceneLabel   string         `json:"scene_label"`
	SceneText    string         `json:"scene_text"`
	PrevSummary  string         `json:"prev_summary"`
	Entities     []EntityBrief  `json:"entities"`
	StyleNotes   string         `json:"style_notes"`
	UserPrompt   string         `json:"user_prompt"`
	Options      Options        `json:"options"`
}

type EntityBrief struct {
	Name       string            `json:"name"`
	Kind       string            `json:"kind"`
	Role       string            `json:"role"`
	Summary    string            `json:"summary"`
	Attributes map[string]string `json:"attributes"`
}

// MarshalJSON kept as default; this comment satisfies the "ai.go is just types".
var _ = json.Marshal
```

- [ ] **Step 2: failing test**

Write `engine/internal/ai/context_test.go`:

```go
package ai

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func TestBuildContext_includesSceneEntitiesAndStyleNotes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	pr := project.NewRepo(s)
	p, _ := pr.Create(context.Background(), 1000, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "novel", DefaultPOV: "first",
	})

	// Set style_notes directly.
	_, _ = s.DB().ExecContext(context.Background(), `UPDATE projects SET style_notes = ? WHERE id = ?`, "단문 위주", p.ID)

	er := entity.NewRepo(s)
	mr := mention.NewRepo(s)
	nodes := node.NewRepo(s)
	nodes.SetMentionResyncer(func(ctx context.Context, nodeID, doc string) error {
		return mr.ResyncForNode(ctx, nodeID, mention.Collect([]byte(doc)))
	})

	e, _ := er.Create(context.Background(), 1100, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "해진", Role: "POV"})

	// Write scene with the mention.
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[
		{"type":"text","text":"파도 소리. "},
		{"type":"mention","attrs":{"id":"` + e.ID + `","label":"해진"}},
		{"type":"text","text":"이 모래를 밟았다."}
	]}]}`
	if err := nodes.UpdateContent(context.Background(), *p.LastOpenedNodeID, doc, 2000); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}

	builder := NewContextBuilder(pr, nodes, mr)
	got, err := builder.Build(context.Background(), *p.LastOpenedNodeID, "재작성", Options{TonePreset: true})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got.SceneLabel != "씬 1" {
		t.Errorf("scene_label = %q", got.SceneLabel)
	}
	if !contains(got.SceneText, "파도 소리") {
		t.Errorf("scene_text missing prose: %q", got.SceneText)
	}
	if got.StyleNotes != "단문 위주" {
		t.Errorf("style_notes = %q", got.StyleNotes)
	}
	if len(got.Entities) != 1 || got.Entities[0].Name != "해진" {
		t.Errorf("entities = %+v", got.Entities)
	}
	if got.UserPrompt != "재작성" {
		t.Errorf("prompt = %q", got.UserPrompt)
	}
	if !got.Options.TonePreset {
		t.Errorf("options not propagated")
	}
}

func TestBuildContext_prevSummary_trims300chars(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	pr := project.NewRepo(s)
	p, _ := pr.Create(context.Background(), 1000, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "novel", DefaultPOV: "first",
	})
	er := entity.NewRepo(s)
	mr := mention.NewRepo(s)
	nodes := node.NewRepo(s)
	nodes.SetMentionResyncer(func(ctx context.Context, nodeID, doc string) error {
		return mr.ResyncForNode(ctx, nodeID, mention.Collect([]byte(doc)))
	})

	// First leaf "씬 1" gets long content; add a second leaf "씬 2" and build
	// context for it — should pull a 300-char trim of 씬 1 as prev_summary.
	long := ""
	for i := 0; i < 400; i++ {
		long += "가"
	}
	docFirst := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + long + `"}]}]}`
	_ = nodes.UpdateContent(context.Background(), *p.LastOpenedNodeID, docFirst, 1100)

	second, _ := nodes.CreateSibling(context.Background(), *p.LastOpenedNodeID, "leaf", "씬 2", "", 1200)
	_ = er // unused

	builder := NewContextBuilder(pr, nodes, mr)
	got, err := builder.Build(context.Background(), second.ID, "확장", Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got.PrevSummary == "" {
		t.Fatal("prev_summary should be populated")
	}
	if r := []rune(got.PrevSummary); len(r) > 310 { // 300 + ellipsis slack
		t.Errorf("prev_summary too long: %d runes", len(r))
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
```

- [ ] **Step 3: `context.go` implementation**

```go
package ai

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
)

const prevSummaryMaxRunes = 300

// ContextBuilder gathers the Context payload from the repos.
type ContextBuilder struct {
	projects *project.Repo
	nodes    *node.Repo
	mentions *mention.Repo
}

func NewContextBuilder(projects *project.Repo, nodes *node.Repo, mentions *mention.Repo) *ContextBuilder {
	return &ContextBuilder{projects: projects, nodes: nodes, mentions: mentions}
}

// Build assembles the context for the given leaf node + user prompt + options.
func (b *ContextBuilder) Build(ctx context.Context, nodeID, prompt string, opts Options) (Context, error) {
	n, err := b.nodes.Get(ctx, nodeID)
	if err != nil {
		return Context{}, err
	}
	proj, err := b.projects.Get(ctx, n.ProjectID)
	if err != nil {
		return Context{}, err
	}

	sceneText := docToPlainText(n.ContentDoc)

	prev, err := b.findPreviousLeaf(ctx, n)
	if err != nil {
		return Context{}, err
	}
	prevSummary := ""
	if prev != nil {
		prevSummary = trimRunes(docToPlainText(prev.ContentDoc), prevSummaryMaxRunes)
	}

	ents, err := b.mentions.ListEntitiesForNode(ctx, nodeID)
	if err != nil {
		return Context{}, err
	}
	briefs := make([]EntityBrief, 0, len(ents))
	for _, e := range ents {
		briefs = append(briefs, EntityBrief{
			Name: e.Name, Kind: e.Kind, Role: e.Role, Summary: e.Summary, Attributes: e.Attributes,
		})
	}

	return Context{
		ProjectID:   proj.ID,
		NodeID:      n.ID,
		SceneLabel:  n.Label,
		SceneText:   sceneText,
		PrevSummary: prevSummary,
		Entities:    briefs,
		StyleNotes:  proj.StyleNotes,
		UserPrompt:  prompt,
		Options:     opts,
	}, nil
}

// findPreviousLeaf returns the previous leaf in DFS order (within the project),
// or nil if `cur` is the first leaf.
func (b *ContextBuilder) findPreviousLeaf(ctx context.Context, cur node.Node) (*node.Node, error) {
	all, err := b.nodes.ListByProject(ctx, cur.ProjectID)
	if err != nil {
		return nil, err
	}
	// Build a parent_id → []children index, then DFS roots in ordinal order.
	type pair struct {
		n  node.Node
		ok bool
	}
	byID := map[string]node.Node{}
	children := map[string][]node.Node{}
	for _, n := range all {
		byID[n.ID] = n
		key := ""
		if n.ParentID != nil {
			key = *n.ParentID
		}
		children[key] = append(children[key], n)
	}
	var leaves []node.Node
	var walk func(parent string)
	walk = func(parent string) {
		for _, c := range children[parent] {
			if c.Kind == "leaf" {
				leaves = append(leaves, c)
			}
			walk(c.ID)
		}
	}
	walk("")
	for i, l := range leaves {
		if l.ID == cur.ID && i > 0 {
			return &leaves[i-1], nil
		}
	}
	return nil, nil
}

// docToPlainText walks a Tiptap doc and concatenates text content. Mentions are
// rendered as `@label`. Block boundaries become newlines.
func docToPlainText(rawDoc *string) string {
	if rawDoc == nil || *rawDoc == "" {
		return ""
	}
	var any interface{}
	if err := json.Unmarshal([]byte(*rawDoc), &any); err != nil {
		return ""
	}
	var sb stringBuilder
	walkDoc(any, &sb, 0)
	return sb.String()
}

type stringBuilder struct {
	buf []byte
}

func (b *stringBuilder) WriteString(s string) { b.buf = append(b.buf, s...) }
func (b *stringBuilder) String() string       { return string(b.buf) }

func walkDoc(v interface{}, sb *stringBuilder, depth int) {
	switch t := v.(type) {
	case map[string]interface{}:
		kind, _ := t["type"].(string)
		if kind == "mention" {
			attrs, _ := t["attrs"].(map[string]interface{})
			label, _ := attrs["label"].(string)
			if label != "" {
				sb.WriteString("@")
				sb.WriteString(label)
			}
			return
		}
		if kind == "text" {
			if s, ok := t["text"].(string); ok {
				sb.WriteString(s)
			}
			return
		}
		// Block-level node: recurse children, then add a newline.
		if content, ok := t["content"].([]interface{}); ok {
			for _, c := range content {
				walkDoc(c, sb, depth+1)
			}
		}
		// Add a single newline between top-level blocks.
		if kind == "paragraph" || kind == "heading" || kind == "blockquote" {
			sb.WriteString("\n")
		}
	case []interface{}:
		for _, c := range t {
			walkDoc(c, sb, depth)
		}
	}
}

func trimRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
```

- [ ] **Step 4: PASS + commit**

```bash
cd engine && go test ./internal/ai/...
git add engine/internal/ai/ai.go engine/internal/ai/context.go engine/internal/ai/context_test.go
git commit -m "feat(ai): context builder (scene + prev summary + entities + style_notes)"
```

---

## Task 4: Prompt templates (TDD)

**Files:**
- Create: `engine/internal/ai/prompts.go`
- Create: `engine/internal/ai/prompts_test.go`

- [ ] **Step 1: `prompts.go`**

```go
package ai

import (
	"fmt"
	"strings"

	"github.com/devlikebear/tars/pkg/llm"
)

// PresetID identifies a built-in prompt template.
type PresetID string

const (
	PresetRewrite PresetID = "rewrite"
	PresetExpand  PresetID = "expand"
	PresetCompact PresetID = "compact"
	PresetFree    PresetID = "free" // verbatim user prompt, no template
)

// Templates returns the user-prompt seed for each preset. The UI calls this
// when the writer clicks a chip so the PROMPT textarea gets prefilled.
func PresetSeed(p PresetID) string {
	switch p {
	case PresetRewrite:
		return "이 문단을 다른 톤으로 다시 써줘."
	case PresetExpand:
		return "이 장면을 더 감각적으로 확장해줘."
	case PresetCompact:
		return "이 씬을 한 문단으로 요약해줘."
	}
	return ""
}

// BuildMessages converts a Context into the two-message system+user pair the
// engine sends to tars. The system message governs tone and length; the user
// message contains the structured context.
func BuildMessages(c Context) []llm.ChatMessage {
	system := buildSystem(c)
	user := buildUser(c)
	return []llm.ChatMessage{
		{Role: "system", Content: []llm.ContentBlock{{Type: "text", Text: system}}},
		{Role: "user", Content: []llm.ContentBlock{{Type: "text", Text: user}}},
	}
}

func buildSystem(c Context) string {
	var b strings.Builder
	b.WriteString("당신은 한국어 소설 작가의 인라인 편집기입니다. ")
	b.WriteString("작가가 요청한 작업을 본문 흐름에 맞게 수행하세요. ")
	b.WriteString("출력은 마크다운 헤더 없이 순수 본문만 작성합니다.\n\n")
	if c.Options.TonePreset && strings.TrimSpace(c.StyleNotes) != "" {
		b.WriteString("작가의 스타일 노트(반드시 따를 것):\n")
		b.WriteString(c.StyleNotes)
		b.WriteString("\n\n")
	}
	if c.Options.ShortForm {
		b.WriteString("출력은 한 문단 이내로 짧게 작성하세요.\n")
	}
	return b.String()
}

func buildUser(c Context) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## 현재 씬: %s\n", c.SceneLabel))
	b.WriteString(c.SceneText)
	b.WriteString("\n\n")
	if strings.TrimSpace(c.PrevSummary) != "" {
		b.WriteString("## 직전 씬 발췌\n")
		b.WriteString(c.PrevSummary)
		b.WriteString("\n\n")
	}
	if len(c.Entities) > 0 {
		b.WriteString("## 등장 인물·장소\n")
		for _, e := range c.Entities {
			b.WriteString(fmt.Sprintf("- @%s — %s", e.Name, kindLabel(e.Kind)))
			if e.Role != "" {
				b.WriteString(" / " + e.Role)
			}
			if e.Summary != "" {
				b.WriteString(": " + e.Summary)
			}
			if len(e.Attributes) > 0 {
				b.WriteString(" (")
				first := true
				for k, v := range e.Attributes {
					if !first {
						b.WriteString(", ")
					}
					first = false
					b.WriteString(k + ":" + v)
				}
				b.WriteString(")")
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if !c.Options.TonePreset && strings.TrimSpace(c.StyleNotes) != "" {
		b.WriteString("## 작가 메모\n")
		b.WriteString(c.StyleNotes)
		b.WriteString("\n\n")
	}
	b.WriteString("## 작가의 지시\n")
	b.WriteString(strings.TrimSpace(c.UserPrompt))
	return b.String()
}

func kindLabel(k string) string {
	switch k {
	case "character":
		return "인물"
	case "place":
		return "장소"
	case "item":
		return "물건"
	case "concept":
		return "개념"
	}
	return k
}
```

- [ ] **Step 2: `prompts_test.go`**

```go
package ai

import (
	"strings"
	"testing"
)

func TestPresetSeed(t *testing.T) {
	if PresetSeed(PresetRewrite) == "" {
		t.Error("rewrite seed should be non-empty")
	}
	if PresetSeed(PresetFree) != "" {
		t.Error("free preset should have no seed")
	}
}

func TestBuildMessages_shapesSystemAndUser(t *testing.T) {
	c := Context{
		ProjectID:   "p1",
		NodeID:      "n1",
		SceneLabel:  "씬 1",
		SceneText:   "파도 소리.",
		PrevSummary: "어제는 비가 왔다.",
		Entities: []EntityBrief{
			{Name: "해진", Kind: "character", Role: "POV", Summary: "사진작가"},
		},
		StyleNotes: "단문 위주",
		UserPrompt: "재작성",
		Options:    Options{TonePreset: true, ShortForm: true},
	}
	msgs := BuildMessages(c)
	if len(msgs) != 2 {
		t.Fatalf("len = %d", len(msgs))
	}
	if msgs[0].Role != "system" || msgs[1].Role != "user" {
		t.Errorf("roles = %q, %q", msgs[0].Role, msgs[1].Role)
	}
	sys := msgs[0].Content[0].Text
	if !strings.Contains(sys, "단문 위주") {
		t.Errorf("style notes not in system: %q", sys)
	}
	if !strings.Contains(sys, "한 문단 이내") {
		t.Errorf("short_form not in system: %q", sys)
	}
	usr := msgs[1].Content[0].Text
	if !strings.Contains(usr, "씬 1") || !strings.Contains(usr, "파도 소리") {
		t.Errorf("scene missing from user: %q", usr)
	}
	if !strings.Contains(usr, "@해진") || !strings.Contains(usr, "POV") {
		t.Errorf("entities missing from user: %q", usr)
	}
}
```

- [ ] **Step 3: PASS + commit**

```bash
cd engine && go test ./internal/ai/...
git add engine/internal/ai/prompts.go engine/internal/ai/prompts_test.go
git commit -m "feat(ai): preset templates + BuildMessages (system + user)"
```

---

## Task 5: Provider client wrapper

**Files:**
- Create: `engine/internal/ai/client.go`

Thin wrapper around `llm.NewProvider` so the runner can ask for `claude-code-cli`. Plan 6 will extend to switch on settings. No tests yet — this is wiring; integration is tested via the runner in Task 6.

```go
package ai

import (
	"github.com/devlikebear/tars/pkg/llm"
)

// ClientFactory creates an llm.Client for a given provider name. Wraps
// llm.NewProvider so tests can inject a fake without monkey-patching tars.
type ClientFactory func(provider, workDir string) (llm.Client, error)

// DefaultClientFactory delegates to tars.
func DefaultClientFactory(provider, workDir string) (llm.Client, error) {
	return llm.NewProvider(llm.ProviderOptions{
		Provider: provider,
		WorkDir:  workDir,
	})
}
```

- [ ] **Step 1: commit**

```bash
git add engine/internal/ai/client.go
git commit -m "feat(ai): ClientFactory wraps llm.NewProvider"
```

---

## Task 6: Runner — manages active runs (TDD)

The runner is the heart of AI mode: starts a streaming chat in a goroutine, pushes `ai.delta` notifications per chunk, fires `ai.done` or `ai.error` at the end, persists to `ai_runs`, and supports cancellation by run id.

**Files:**
- Create: `engine/internal/ai/runner.go`
- Create: `engine/internal/ai/runner_test.go`

- [ ] **Step 1: domain notification types** — append to `engine/internal/ai/ai.go`

Append after the existing types:

```go
// DeltaPayload is the body of an "ai.delta" notification.
type DeltaPayload struct {
	RunID string `json:"run_id"`
	Text  string `json:"text"`
}

// DonePayload is the body of an "ai.done" notification.
type DonePayload struct {
	RunID    string `json:"run_id"`
	FullText string `json:"full_text"`
}

// ErrorPayload is the body of an "ai.error" notification.
type ErrorPayload struct {
	RunID   string `json:"run_id"`
	Message string `json:"message"`
}

// CancelledPayload is the body of an "ai.cancelled" notification.
type CancelledPayload struct {
	RunID string `json:"run_id"`
}
```

- [ ] **Step 2: failing test**

Write `engine/internal/ai/runner_test.go`:

```go
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/tars/pkg/llm"
)

// fakeClient streams a fixed set of chunks. Implements llm.Client.
type fakeClient struct {
	chunks []string
	failAt int // index at which to return an error; -1 = never
	hold   chan struct{}
}

func (f *fakeClient) Ask(ctx context.Context, prompt string) (string, error) {
	return "", errors.New("not used")
}

func (f *fakeClient) Chat(ctx context.Context, messages []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	var full strings.Builder
	for i, c := range f.chunks {
		if ctx.Err() != nil {
			return llm.ChatResponse{}, ctx.Err()
		}
		if f.hold != nil {
			<-f.hold
		}
		if f.failAt == i {
			return llm.ChatResponse{}, errors.New("simulated provider failure")
		}
		full.WriteString(c)
		if opts.OnDelta != nil {
			opts.OnDelta(c)
		}
	}
	return llm.ChatResponse{Text: full.String()}, nil
}

type fakeNotifier struct {
	mu     sync.Mutex
	events []string
}

func (n *fakeNotifier) Notify(method string, params any) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	b, _ := json.Marshal(params)
	n.events = append(n.events, method+":"+string(b))
	return nil
}

func newFixture(t *testing.T) (*store.Store, project.Project) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	pr := project.NewRepo(s)
	p, _ := pr.Create(context.Background(), 1000, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first",
	})
	return s, p
}

func TestRunner_streams_thenEmitsDone(t *testing.T) {
	s, p := newFixture(t)
	fake := &fakeClient{chunks: []string{"안녕 ", "세계", "!"}, failAt: -1}
	notif := &fakeNotifier{}

	runs := store.NewAIRunsRepo(s)
	r := NewRunner(notif, runs, func(_, _ string) (llm.Client, error) { return fake, nil }, "claude-code-cli")
	now := func() int64 { return 1234 }

	c := Context{ProjectID: p.ID, NodeID: *p.LastOpenedNodeID, SceneLabel: "씬 1", UserPrompt: "안녕"}
	runID, err := r.Start(context.Background(), c, now)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if runID == "" {
		t.Fatal("missing runID")
	}

	deadline := time.Now().Add(1 * time.Second)
	for {
		notif.mu.Lock()
		count := len(notif.events)
		notif.mu.Unlock()
		if count >= 4 || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	notif.mu.Lock()
	defer notif.mu.Unlock()
	if len(notif.events) != 4 {
		t.Fatalf("events = %v", notif.events)
	}
	for i, expected := range []string{"ai.delta", "ai.delta", "ai.delta", "ai.done"} {
		if !strings.HasPrefix(notif.events[i], expected) {
			t.Errorf("event[%d] = %q, want prefix %q", i, notif.events[i], expected)
		}
	}
	if !strings.Contains(notif.events[3], "안녕 세계!") {
		t.Errorf("full_text missing: %q", notif.events[3])
	}

	// Persisted as done.
	rows, _ := runs.ListRecent(context.Background(), p.ID, 5)
	if len(rows) != 1 || rows[0].Status != store.AIRunDone || rows[0].Output != "안녕 세계!" {
		t.Errorf("persisted = %+v", rows)
	}
}

func TestRunner_cancel_emitsCancelled_andPersistsCancelled(t *testing.T) {
	s, p := newFixture(t)
	fake := &fakeClient{chunks: []string{"한", "두"}, failAt: -1, hold: make(chan struct{})}
	notif := &fakeNotifier{}
	runs := store.NewAIRunsRepo(s)
	r := NewRunner(notif, runs, func(_, _ string) (llm.Client, error) { return fake, nil }, "claude-code-cli")
	now := func() int64 { return 1234 }

	c := Context{ProjectID: p.ID, NodeID: *p.LastOpenedNodeID, SceneLabel: "씬 1", UserPrompt: "안녕"}
	runID, err := r.Start(context.Background(), c, now)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Let it not advance (fake.hold blocks Chat). Cancel.
	if err := r.Cancel(runID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	close(fake.hold) // unblock Chat so it can observe ctx.Done

	deadline := time.Now().Add(1 * time.Second)
	for {
		notif.mu.Lock()
		cancelled := false
		for _, e := range notif.events {
			if strings.HasPrefix(e, "ai.cancelled") {
				cancelled = true
			}
		}
		notif.mu.Unlock()
		if cancelled || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	rows, _ := runs.ListRecent(context.Background(), p.ID, 5)
	if len(rows) != 1 || rows[0].Status != store.AIRunCancelled {
		t.Errorf("persisted = %+v", rows)
	}
}

func TestRunner_providerError_emitsError(t *testing.T) {
	s, p := newFixture(t)
	fake := &fakeClient{chunks: []string{"한", "두"}, failAt: 1} // error after first chunk
	notif := &fakeNotifier{}
	runs := store.NewAIRunsRepo(s)
	r := NewRunner(notif, runs, func(_, _ string) (llm.Client, error) { return fake, nil }, "claude-code-cli")
	now := func() int64 { return 1234 }

	c := Context{ProjectID: p.ID, NodeID: *p.LastOpenedNodeID, SceneLabel: "씬 1", UserPrompt: "안녕"}
	if _, err := r.Start(context.Background(), c, now); err != nil {
		t.Fatalf("Start: %v", err)
	}

	deadline := time.Now().Add(1 * time.Second)
	for {
		notif.mu.Lock()
		errored := false
		for _, e := range notif.events {
			if strings.HasPrefix(e, "ai.error") {
				errored = true
			}
		}
		notif.mu.Unlock()
		if errored || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
}
```

- [ ] **Step 3: `runner.go`**

```go
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/tars/pkg/llm"
	"github.com/google/uuid"
)

// Clock matches handlers.Clock.
type Clock func() int64

// Runner manages active AI runs.
type Runner struct {
	notify   rpc.Notifier
	runs     *store.AIRunsRepo
	factory  ClientFactory
	provider string
	workDir  string

	mu     sync.Mutex
	active map[string]context.CancelFunc
}

// NewRunner constructs a Runner. workDir is passed to claude-code-cli; can be
// the empty string (current working dir).
func NewRunner(notify rpc.Notifier, runs *store.AIRunsRepo, factory ClientFactory, provider string) *Runner {
	return &Runner{
		notify:   notify,
		runs:     runs,
		factory:  factory,
		provider: provider,
		active:   map[string]context.CancelFunc{},
	}
}

// Start enqueues a run and returns its id immediately. The work happens on a
// goroutine that emits notifications via the Notifier.
func (r *Runner) Start(ctx context.Context, c Context, now Clock) (string, error) {
	runID := uuid.NewString()
	startedAt := now()
	ctxJSON, _ := json.Marshal(c)

	var nodeID *string
	if c.NodeID != "" {
		v := c.NodeID
		nodeID = &v
	}
	if err := r.runs.Insert(ctx, store.AIRun{
		ID: runID, ProjectID: c.ProjectID, NodeID: nodeID,
		Provider:    r.provider,
		Prompt:      c.UserPrompt,
		ContextJSON: ctxJSON,
		Status:      store.AIRunStreaming,
		StartedAt:   startedAt,
	}); err != nil {
		return "", err
	}

	client, err := r.factory(r.provider, r.workDir)
	if err != nil {
		_ = r.runs.UpdateStatus(ctx, runID, store.AIRunError, "", err.Error(), now())
		return "", err
	}

	runCtx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	r.active[runID] = cancel
	r.mu.Unlock()

	go r.run(runCtx, runID, c, client, now)
	return runID, nil
}

func (r *Runner) run(ctx context.Context, runID string, c Context, client llm.Client, now Clock) {
	defer func() {
		r.mu.Lock()
		delete(r.active, runID)
		r.mu.Unlock()
	}()

	var full struct {
		mu  sync.Mutex
		buf string
	}
	msgs := BuildMessages(c)

	resp, err := client.Chat(ctx, msgs, llm.ChatOptions{
		OnDelta: func(text string) {
			full.mu.Lock()
			full.buf += text
			full.mu.Unlock()
			_ = r.notify.Notify("ai.delta", DeltaPayload{RunID: runID, Text: text})
		},
	})

	endedAt := now()
	if errors.Is(err, context.Canceled) {
		_ = r.notify.Notify("ai.cancelled", CancelledPayload{RunID: runID})
		full.mu.Lock()
		out := full.buf
		full.mu.Unlock()
		_ = r.runs.UpdateStatus(context.Background(), runID, store.AIRunCancelled, out, "", endedAt)
		return
	}
	if err != nil {
		_ = r.notify.Notify("ai.error", ErrorPayload{RunID: runID, Message: err.Error()})
		full.mu.Lock()
		out := full.buf
		full.mu.Unlock()
		_ = r.runs.UpdateStatus(context.Background(), runID, store.AIRunError, out, err.Error(), endedAt)
		return
	}

	finalText := resp.Text
	if finalText == "" {
		full.mu.Lock()
		finalText = full.buf
		full.mu.Unlock()
	}
	_ = r.notify.Notify("ai.done", DonePayload{RunID: runID, FullText: finalText})
	_ = r.runs.UpdateStatus(context.Background(), runID, store.AIRunDone, finalText, "", endedAt)
}

// Cancel cancels the run by id. Returns an error if no such run is active.
func (r *Runner) Cancel(runID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cancel, ok := r.active[runID]
	if !ok {
		return errors.New("ai: run not found or already finished")
	}
	cancel()
	return nil
}
```

- [ ] **Step 4: PASS + commit**

```bash
cd engine && go test ./internal/ai/... -race
git add engine/internal/ai/ai.go engine/internal/ai/runner.go engine/internal/ai/runner_test.go
git commit -m "feat(ai): runner with streaming + cancel + ai_runs persistence"
```

---

## Task 7: AI RPC handlers (TDD)

**Files:**
- Create: `engine/internal/rpc/handlers/ai.go`
- Create: `engine/internal/rpc/handlers/ai_test.go`

`ai.run` accepts `{ node_id, prompt, options }`, builds the Context via the injected ContextBuilder, and calls Runner.Start. `ai.cancel` accepts `{ run_id }`.

- [ ] **Step 1: tests**

Write `engine/internal/rpc/handlers/ai_test.go`:

```go
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/ai"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/tars/pkg/llm"
)

type capNotif struct {
	mu sync.Mutex
	es []string
}

func (n *capNotif) Notify(method string, _ any) error {
	n.mu.Lock()
	n.es = append(n.es, method)
	n.mu.Unlock()
	return nil
}

type streamingFake struct{}

func (streamingFake) Ask(_ context.Context, _ string) (string, error) {
	return "", errors.New("unused")
}
func (streamingFake) Chat(ctx context.Context, _ []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	if opts.OnDelta != nil {
		opts.OnDelta("결과")
	}
	return llm.ChatResponse{Text: "결과"}, nil
}

func newAIFixture(t *testing.T) (*ai.Runner, *ai.ContextBuilder, string, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	pr := project.NewRepo(s)
	p, _ := pr.Create(context.Background(), 1000, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first",
	})
	nodes := node.NewRepo(s)
	mr := mention.NewRepo(s)
	runs := store.NewAIRunsRepo(s)
	notif := &capNotif{}
	runner := ai.NewRunner(notif, runs, func(_, _ string) (llm.Client, error) { return streamingFake{}, nil }, "claude-code-cli")
	builder := ai.NewContextBuilder(pr, nodes, mr)
	return runner, builder, p.ID, *p.LastOpenedNodeID
}

func TestRunAIHandler_returnsRunID(t *testing.T) {
	runner, builder, _, nID := newAIFixture(t)
	h := RunAI(builder, runner, func() int64 { return 1 })

	params := json.RawMessage(`{"node_id":"` + nID + `","prompt":"안녕","options":{}}`)
	res, err := h(context.Background(), params)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got map[string]any
	_ = json.Unmarshal(res, &got)
	if got["run_id"] == "" || got["run_id"] == nil {
		t.Errorf("missing run_id: %+v", got)
	}
	time.Sleep(50 * time.Millisecond) // let the goroutine finish so t.Cleanup doesn't race
}

func TestCancelAIHandler_unknownRunID(t *testing.T) {
	runner, _, _, _ := newAIFixture(t)
	h := CancelAI(runner)
	if _, err := h(context.Background(), json.RawMessage(`{"run_id":"nope"}`)); err == nil {
		t.Error("expected error for unknown run_id")
	}
}
```

- [ ] **Step 2: `ai.go`**

```go
package handlers

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/ai"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

type runAIParams struct {
	NodeID  string     `json:"node_id"`
	Prompt  string     `json:"prompt"`
	Options ai.Options `json:"options"`
}

func RunAI(builder *ai.ContextBuilder, runner *ai.Runner, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p runAIParams
		if err := json.Unmarshal(params, &p); err != nil || p.NodeID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node_id required"}
		}
		c, err := builder.Build(ctx, p.NodeID, p.Prompt, p.Options)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		runID, err := runner.Start(ctx, c, func() int64 { return now() })
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(map[string]string{"run_id": runID})
	}
}

type cancelAIParams struct {
	RunID string `json:"run_id"`
}

func CancelAI(runner *ai.Runner) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p cancelAIParams
		if err := json.Unmarshal(params, &p); err != nil || p.RunID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "run_id required"}
		}
		if err := runner.Cancel(p.RunID); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}
```

- [ ] **Step 3: PASS + commit**

```bash
cd engine && go test ./... 2>&1 | tail -20
git add engine/internal/rpc/handlers/ai.go engine/internal/rpc/handlers/ai_test.go
git commit -m "feat(rpc): ai.run + ai.cancel handlers"
```

---

## Task 8: Wire main.go + stdio smoke

**Files:**
- Modify: `engine/cmd/linetta-engine/main.go`

- [ ] **Step 1: Wire**

Add imports:
```go
"github.com/devlikebear/linetta/engine/internal/ai"
```

After `mentions := mention.NewRepo(st)`:
```go
	aiRuns := store.NewAIRunsRepo(st)
	notifier := s.Notifier() // declare BEFORE registering handlers (s already exists in this file: `s := rpc.NewServer()` happens below — move that up)
```

Reorder: build `s := rpc.NewServer()` *before* the repos, then construct ai.Runner with `s.Notifier()`. Continue:

```go
	contextBuilder := ai.NewContextBuilder(projects, nodes, mentions)
	runner := ai.NewRunner(s.Notifier(), aiRuns, ai.DefaultClientFactory, "claude-code-cli")

	s.Handle("ai.run", handlers.RunAI(contextBuilder, runner, clock))
	s.Handle("ai.cancel", handlers.CancelAI(runner))
```

(The existing `s.Handle("ping", ...)` and friends stay as-is.)

- [ ] **Step 2: build + (no easy stdio smoke without live LLM — just confirm engine starts)**

```bash
cd engine && go build -o /tmp/linetta-engine-build ./cmd/linetta-engine
LINETTA_HOME=/tmp/lp5-smoke /tmp/linetta-engine-build --stdio <<'EOF'
{"jsonrpc":"2.0","id":1,"method":"ping"}
EOF
rm -f /tmp/linetta-engine-build
rm -rf /tmp/lp5-smoke
```
Expected: `{"jsonrpc":"2.0","id":1,"result":"pong"}` — proves no init panic.

- [ ] **Step 3: commit + rebuild dev binary**

```bash
git add engine/cmd/linetta-engine/main.go
git commit -m "feat(engine): wire ai.runner + ai.run/ai.cancel handlers"
./scripts/build-engine.sh
```

---

## Task 9: Rust shell — forward JSONRPC notifications as Tauri events

**Files:**
- Modify: `apps/desktop/src-tauri/src/jsonrpc.rs`
- Modify: `apps/desktop/src-tauri/src/lib.rs`

The Plan 0 client currently drops notifications. We change it to invoke a callback for each one. lib.rs converts them to Tauri events.

- [ ] **Step 1: `jsonrpc.rs` — add NotificationHandler**

Add a type and modify `Client::new`:

```rust
pub type NotificationHandler = std::sync::Arc<dyn Fn(String, serde_json::Value) + Send + Sync>;

pub struct Client {
    next_id: AtomicI64,
    stdin: Mutex<ChildStdin>,
    pending: Pending,
}

impl Client {
    pub fn new(stdin: ChildStdin, stdout: ChildStdout, on_notification: NotificationHandler) -> Arc<Self> {
        // ... existing setup ...
        // In the reader task, when resp.method.is_some() && resp.id.is_none():
        //   let params = resp.params.unwrap_or(Value::Null);
        //   (on_notification)(resp.method.unwrap(), params);
        //   continue;
    }
    // call() unchanged
}
```

Replace the spawned reader loop's notification branch:

```rust
if let Some(method) = resp.method.clone() {
    if resp.id.is_none() {
        let params = resp.params.clone().unwrap_or(Value::Null);
        (on_notification)(method, params);
        continue;
    }
}
```

- [ ] **Step 2: `engine.rs` — pass an emitter handler when constructing Client**

Modify `engine::spawn` to accept the AppHandle (it already does) and create the notification callback:

```rust
pub async fn spawn(app: &tauri::AppHandle) -> Result<EngineHandle> {
    // ... existing spawn / take stdin/stdout ...
    let handle_clone = app.clone();
    let on_notification: NotificationHandler = std::sync::Arc::new(move |method: String, params: Value| {
        // Route ai.* method names to Tauri events with the same name.
        let event = match method.as_str() {
            "ai.delta" => "ai-delta",
            "ai.done" => "ai-done",
            "ai.error" => "ai-error",
            "ai.cancelled" => "ai-cancelled",
            _ => return, // ignore unknown
        };
        let _ = handle_clone.emit(event, params);
    });
    let client = Client::new(stdin, stdout, on_notification);
    Ok(EngineHandle { client, _child: child })
}
```

You'll need `use tauri::Emitter;` in `engine.rs` and the `NotificationHandler` import.

- [ ] **Step 3: cargo check**

```bash
cd apps/desktop/src-tauri && cargo check 2>&1 | tail -10
```
Expected: silent.

- [ ] **Step 4: commit**

```bash
git add apps/desktop/src-tauri/src/jsonrpc.rs apps/desktop/src-tauri/src/engine.rs
git commit -m "feat(shell): forward engine JSONRPC notifications as Tauri events"
```

---

## Task 10: TS types + RPC + event hook

**Files:**
- Modify: `apps/desktop/src/lib/types.ts`
- Modify: `apps/desktop/src/lib/rpc.ts`
- Create: `apps/desktop/src/hooks/useEngineEvent.ts`

- [ ] **Step 1: Types**

Append to `types.ts`:

```ts
export interface AIOptions {
  tone_preset: boolean;
  short_form: boolean;
}

export interface AIDelta {
  run_id: string;
  text: string;
}

export interface AIDone {
  run_id: string;
  full_text: string;
}

export interface AIError {
  run_id: string;
  message: string;
}

export interface AICancelled {
  run_id: string;
}
```

- [ ] **Step 2: rpc.ts**

Append the `ai` namespace:

```ts
export const ai = {
  run: (nodeId: string, prompt: string, options: AIOptions) =>
    rpcCall<{ run_id: string }>("ai.run", { node_id: nodeId, prompt, options }),
  cancel: (runId: string) => rpcCall<{ ok: true }>("ai.cancel", { run_id: runId }),
};
```

Add `AIOptions` to the existing import block from `./types`.

- [ ] **Step 3: useEngineEvent.ts**

```ts
import { listen, type UnlistenFn } from "@tauri-apps/api/event";
import { useEffect } from "react";

/** Subscribe to a Tauri event. The handler is captured via ref so callers can
 *  pass inline arrows without churning the subscription. */
export function useEngineEvent<T>(event: string, handler: (payload: T) => void) {
  useEffect(() => {
    let unlisten: UnlistenFn | null = null;
    let cancelled = false;
    listen<T>(event, (e) => handler(e.payload)).then((fn) => {
      if (cancelled) {
        fn();
      } else {
        unlisten = fn;
      }
    });
    return () => {
      cancelled = true;
      if (unlisten) unlisten();
    };
  }, [event, handler]);
}
```

- [ ] **Step 4: tsc + commit**

```bash
cd apps/desktop && pnpm tsc -b && cd ../..
git add apps/desktop/src/lib apps/desktop/src/hooks
git commit -m "feat(rpc): ai client + useEngineEvent hook"
```

---

## Task 11: AIMode component

The AI mode pane replaces the Tiptap editor when the writer flips the toggle. Its layout (per spec §4.4):
- Top: PROMPT textarea with three preset chips above it (재작성/확장/요약)
- Below: streaming output region (`결과` preview)
- A meta line above the output: `생성됨 · 컨텍스트: @해진, @윤서 · style notes` — built from the last received `ai.delta`'s context (or just from props since we already know the mentions + style_notes)
- Action buttons at the bottom: `커서에 삽입` / `선택 영역 교체` / `다시 생성` / `버리기`. Plus a `생성` / `취소` toggle on top of the output region.

**Files:**
- Create: `apps/desktop/src/components/ai/AIMode.tsx`
- Create: `apps/desktop/src/components/ai/AIMode.css`
- Create: `apps/desktop/src/components/ai/AIContextPanel.tsx` (right panel when AI mode active)

Wire-up details belong to the Workspace (Task 12); here we author pure components controlled by props.

- [ ] **Step 1: AIMode.tsx**

```tsx
import { useState, type FormEvent } from "react";
import "./AIMode.css";

export type AIRunStatus =
  | { kind: "idle" }
  | { kind: "running"; runId: string; text: string }
  | { kind: "done"; text: string }
  | { kind: "error"; message: string; text: string };

interface Props {
  status: AIRunStatus;
  /** Initial prompt (populated by preset chips). */
  prompt: string;
  onPromptChange: (v: string) => void;
  options: { tone_preset: boolean; short_form: boolean };
  onOptionsChange: (o: { tone_preset: boolean; short_form: boolean }) => void;
  onPresetClick: (preset: "rewrite" | "expand" | "compact") => void;
  onRun: () => void;
  onCancel: () => void;
  onInsert: (text: string) => void;
  onReplace: (text: string) => void;
  onRegenerate: () => void;
  onDiscard: () => void;
  /** One-line summary of what the engine attached (for the meta line). */
  contextSummary: string;
}

export function AIMode(props: Props) {
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (props.status.kind === "running") props.onCancel();
    else props.onRun();
  };

  const showActions = props.status.kind === "done";
  const streamingText =
    props.status.kind === "running" || props.status.kind === "error"
      ? props.status.text
      : props.status.kind === "done"
      ? props.status.text
      : "";

  return (
    <div className="aimode">
      <form className="aimode-prompt" onSubmit={submit}>
        <div className="aimode-presets">
          <button type="button" onClick={() => props.onPresetClick("rewrite")}>재작성</button>
          <button type="button" onClick={() => props.onPresetClick("expand")}>확장</button>
          <button type="button" onClick={() => props.onPresetClick("compact")}>요약</button>
        </div>
        <label className="aimode-prompt-label">PROMPT</label>
        <textarea
          className="aimode-textarea"
          value={props.prompt}
          onChange={(e) => props.onPromptChange(e.target.value)}
          placeholder="작가의 지시를 입력하세요…"
          rows={4}
        />
        <div className="aimode-toolbar">
          <label className="aimode-check">
            <input
              type="checkbox"
              checked={props.options.tone_preset}
              onChange={(e) => props.onOptionsChange({ ...props.options, tone_preset: e.target.checked })}
            /> 톤 프리셋 "내 톤"
          </label>
          <label className="aimode-check">
            <input
              type="checkbox"
              checked={props.options.short_form}
              onChange={(e) => props.onOptionsChange({ ...props.options, short_form: e.target.checked })}
            /> 길이: 한 문단
          </label>
          <span className="aimode-spacer" />
          <button type="submit" className="aimode-run">
            {props.status.kind === "running" ? "취소" : "생성"}
          </button>
        </div>
      </form>

      <section className="aimode-output">
        <p className="aimode-meta">{streamingText ? `생성됨 · ${props.contextSummary}` : "결과가 여기에 표시됩니다"}</p>
        <div className="aimode-stream">
          {streamingText.split(/\n/).map((line, i) => (
            <p key={i}>{line || " "}</p>
          ))}
          {props.status.kind === "running" && <span className="aimode-cursor">▌</span>}
          {props.status.kind === "error" && (
            <p className="aimode-error">오류: {props.status.message}</p>
          )}
        </div>
        {showActions && (
          <div className="aimode-actions">
            <button type="button" onClick={() => props.onInsert(streamingText)}>커서에 삽입</button>
            <button type="button" onClick={() => props.onReplace(streamingText)}>선택 영역 교체</button>
            <button type="button" onClick={props.onRegenerate}>다시 생성</button>
            <button type="button" onClick={props.onDiscard}>버리기</button>
          </div>
        )}
      </section>
    </div>
  );
}
```

- [ ] **Step 2: AIMode.css**

```css
.aimode {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  padding: 2rem;
  max-width: 720px;
  margin: 0 auto;
  height: 100%;
  overflow-y: auto;
  overflow-x: hidden;
}
.aimode-prompt {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
}
.aimode-presets {
  display: flex;
  gap: 0.4rem;
  flex-wrap: wrap;
}
.aimode-presets button {
  font: inherit;
  font-size: 0.85rem;
  padding: 0.25rem 0.7rem;
  border: 1px solid #c8c5bd;
  background: white;
  border-radius: 999px;
  cursor: pointer;
}
.aimode-presets button:hover { background: #f6f4ee; }
.aimode-prompt-label {
  font-size: 0.7rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: #6b6b6b;
}
.aimode-textarea {
  font: inherit;
  font-size: 1rem;
  padding: 0.6rem 0.75rem;
  border: 1px solid #d8d6cf;
  border-radius: 4px;
  background: white;
  resize: vertical;
  min-height: 5rem;
}
.aimode-toolbar {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  flex-wrap: wrap;
}
.aimode-check {
  font-size: 0.85rem;
  display: flex;
  align-items: center;
  gap: 0.3rem;
}
.aimode-spacer { flex: 1; }
.aimode-run {
  font: inherit;
  background: #1a1a1a;
  color: #faf9f6;
  border: none;
  border-radius: 4px;
  padding: 0.45rem 1.2rem;
  cursor: pointer;
}
.aimode-output {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  flex: 1;
}
.aimode-meta {
  font-size: 0.8rem;
  color: #6b6b6b;
  margin: 0;
}
.aimode-stream {
  font-family: ui-serif, Georgia, "Apple SD Gothic Neo", serif;
  font-size: 1.05rem;
  line-height: 1.8;
  padding: 0.85rem 1rem;
  background: #fffefb;
  border: 1px solid #ece9e0;
  border-radius: 6px;
  min-height: 6rem;
  white-space: pre-wrap;
}
.aimode-stream p { margin: 0 0 0.5rem; }
.aimode-cursor {
  animation: aimode-blink 1s steps(2, jump-none) infinite;
  color: #6b6b6b;
}
@keyframes aimode-blink {
  50% { opacity: 0; }
}
.aimode-error { color: #a8312f; font-size: 0.9rem; }
.aimode-actions {
  display: flex;
  gap: 0.5rem;
  flex-wrap: wrap;
}
.aimode-actions button {
  font: inherit;
  font-size: 0.9rem;
  padding: 0.4rem 0.95rem;
  background: white;
  border: 1px solid #1a1a1a;
  border-radius: 4px;
  cursor: pointer;
}
.aimode-actions button:first-child {
  background: #1a1a1a;
  color: #faf9f6;
}
```

- [ ] **Step 3: AIContextPanel.tsx**

Right panel content when AI mode is active. Shows the checklist of "AI에게 전달됨":

```tsx
import type { Entity, Project } from "../../lib/types";

interface Props {
  project: Project;
  mentioned: Entity[];
  options: { tone_preset: boolean; short_form: boolean };
  onOptionsChange: (o: { tone_preset: boolean; short_form: boolean }) => void;
}

export function AIContextPanel({ project, mentioned, options, onOptionsChange }: Props) {
  return (
    <aside className="ctx-panel">
      <section className="ctx-section">
        <h4>AI에게 전달됨</h4>
        <ul className="ai-context-checklist">
          <li>✓ 현재 씬 본문</li>
          <li>✓ 직전 씬 발췌 (300자)</li>
          {mentioned.length > 0 && (
            <li>
              ✓ @멘션: {mentioned.map((e) => e.name).join(", ")}
            </li>
          )}
          {project.style_notes && <li>✓ 작품 style notes</li>}
        </ul>
      </section>
      <section className="ctx-section">
        <h4>옵션</h4>
        <label className="ctx-check">
          <input
            type="checkbox"
            checked={options.tone_preset}
            onChange={(e) => onOptionsChange({ ...options, tone_preset: e.target.checked })}
          />
          톤 프리셋 "내 톤"
        </label>
        <label className="ctx-check">
          <input
            type="checkbox"
            checked={options.short_form}
            onChange={(e) => onOptionsChange({ ...options, short_form: e.target.checked })}
          />
          길이: 한 문단
        </label>
      </section>
    </aside>
  );
}
```

Append to `App.css`:

```css
.ai-context-checklist {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  font-size: 0.85rem;
}
```

- [ ] **Step 4: tsc + commit**

```bash
cd apps/desktop && pnpm tsc -b && cd ../..
git add apps/desktop/src/components/ai apps/desktop/src/App.css
git commit -m "feat(ai-mode): prompt + presets + streaming + actions UI"
```

---

## Task 12: Workspace integrates AI mode

Make the `편집 / AI` toggle real. State: `mode: "edit" | "ai"`. When AI: render `<AIMode>` in place of the editor and `<AIContextPanel>` in place of the context panel (when EntitySheet isn't claiming the slot). Listen for `ai-delta` / `ai-done` / `ai-error` / `ai-cancelled` Tauri events and update the AIMode status.

**Files:**
- Modify: `apps/desktop/src/routes/Workspace.tsx`

The diff is large. Here are the key additions; the implementer is expected to merge into the existing component.

- [ ] **Step 1: State + handlers**

Add imports:
```ts
import { ai as aiApi } from "../lib/rpc";
import { useEngineEvent } from "../hooks/useEngineEvent";
import type { AIDelta, AIDone, AIError, AICancelled, AIOptions } from "../lib/types";
import { AIMode, type AIRunStatus } from "../components/ai/AIMode";
import { AIContextPanel } from "../components/ai/AIContextPanel";
```

Add state:
```ts
const [mode, setMode] = useState<"edit" | "ai">("edit");
const [aiPrompt, setAiPrompt] = useState("");
const [aiOptions, setAiOptions] = useState<AIOptions>({ tone_preset: false, short_form: false });
const [aiStatus, setAiStatus] = useState<AIRunStatus>({ kind: "idle" });
const aiRunIdRef = useRef<string | null>(null);
```

Wire the event listeners:
```ts
useEngineEvent<AIDelta>("ai-delta", (p) => {
  if (p.run_id !== aiRunIdRef.current) return;
  setAiStatus((s) => {
    const prev = s.kind === "running" ? s.text : "";
    return { kind: "running", runId: p.run_id, text: prev + p.text };
  });
});
useEngineEvent<AIDone>("ai-done", (p) => {
  if (p.run_id !== aiRunIdRef.current) return;
  setAiStatus({ kind: "done", text: p.full_text });
  aiRunIdRef.current = null;
});
useEngineEvent<AIError>("ai-error", (p) => {
  if (p.run_id !== aiRunIdRef.current) return;
  setAiStatus((s) => ({ kind: "error", message: p.message, text: s.kind === "running" ? s.text : "" }));
  aiRunIdRef.current = null;
});
useEngineEvent<AICancelled>("ai-cancelled", (p) => {
  if (p.run_id !== aiRunIdRef.current) return;
  setAiStatus({ kind: "idle" });
  aiRunIdRef.current = null;
});
```

The AI run + cancel + presets + actions handlers:
```ts
const handlePreset = (preset: "rewrite" | "expand" | "compact") => {
  const seeds: Record<typeof preset, string> = {
    rewrite: "이 문단을 다른 톤으로 다시 써줘.",
    expand: "이 장면을 더 감각적으로 확장해줘.",
    compact: "이 씬을 한 문단으로 요약해줘.",
  };
  setAiPrompt(seeds[preset]);
};

const startAIRun = async () => {
  if (!load || !aiPrompt.trim()) return;
  setAiStatus({ kind: "running", runId: "pending", text: "" });
  try {
    const { run_id } = await aiApi.run(load.node.id, aiPrompt, aiOptions);
    aiRunIdRef.current = run_id;
    setAiStatus({ kind: "running", runId: run_id, text: "" });
  } catch (e) {
    setAiStatus({ kind: "error", message: String(e), text: "" });
  }
};

const cancelAIRun = async () => {
  if (!aiRunIdRef.current) return;
  try { await aiApi.cancel(aiRunIdRef.current); } catch { /* benign */ }
};

const insertResult = (text: string) => {
  if (!load || !text) return;
  const ed = (editorRef.current as any);
  // Use a small imperative escape hatch on the editor via its public focus().
  // For Plan 5 we re-fetch the node, splice in text, and save through normal autosave.
  // Simplest: append a paragraph with the result.
  const docStr = JSON.stringify({
    type: "doc",
    content: [
      ...(load.initialDoc as any).content ?? [],
      { type: "paragraph", content: [{ type: "text", text }] },
    ],
  });
  nodes.updateContent(load.node.id, docStr).then(() => {
    setMode("edit");
    setAiStatus({ kind: "idle" });
    refreshTreeKeepNode(load.node.id);
    setTimeout(() => focusEditor(), 0);
  });
};

const replaceWithResult = (text: string) => {
  if (!load || !text) return;
  const docStr = JSON.stringify({
    type: "doc",
    content: [{ type: "paragraph", content: [{ type: "text", text }] }],
  });
  // Take a manual snapshot first so the body is recoverable.
  snapshots.createManual(load.node.id, JSON.stringify(load.initialDoc)).catch(() => {});
  nodes.updateContent(load.node.id, docStr).then(() => {
    setMode("edit");
    setAiStatus({ kind: "idle" });
    refreshTreeKeepNode(load.node.id);
    setTimeout(() => focusEditor(), 0);
  });
};
```

(Note: the insert/replace logic here is intentionally simple — it splices at the doc level rather than via Tiptap commands. Plan 5 trades clean Tiptap integration for shipping; later polish can use editor commands and the actual cursor / selection.)

Build the context summary string for the meta line:
```ts
const aiContextSummary = useMemo(() => {
  const parts: string[] = ["현재 씬 + 직전 씬"];
  if (mentioned.length > 0) parts.push("@" + mentioned.map((e) => e.name).join(", @"));
  if (load?.project.style_notes) parts.push("style notes");
  return parts.join(" · ");
}, [mentioned, load]);
```

Switch the editor surface and the right panel based on `mode`:

```tsx
<header className="ws-top">
  <Link to="/" className="ws-breadcrumb">{breadcrumb}</Link>
  <span className="ws-modes">
    <button type="button" className={`mode-toggle${mode === "edit" ? " on" : ""}`} onClick={() => setMode("edit")}>편집</button>
    <button type="button" className={`mode-toggle${mode === "ai" ? " on" : ""}`} onClick={() => setMode("ai")}>AI</button>
  </span>
  <span className="ws-zen">ZEN</span>
</header>

<div className={`ws-body${entitySheetId ? " with-sheet" : ""}`}>
  {mode === "edit" ? (
    <div className="ws-editor">
      <TiptapEditor ... />
    </div>
  ) : (
    <AIMode
      status={aiStatus}
      prompt={aiPrompt}
      onPromptChange={setAiPrompt}
      options={aiOptions}
      onOptionsChange={setAiOptions}
      onPresetClick={handlePreset}
      onRun={startAIRun}
      onCancel={cancelAIRun}
      onInsert={insertResult}
      onReplace={replaceWithResult}
      onRegenerate={startAIRun}
      onDiscard={() => setAiStatus({ kind: "idle" })}
      contextSummary={aiContextSummary}
    />
  )}
  {entitySheetId ? (
    <EntitySheet ... />
  ) : mode === "ai" ? (
    <AIContextPanel
      project={load.project}
      mentioned={mentioned}
      options={aiOptions}
      onOptionsChange={setAiOptions}
    />
  ) : (
    <ContextPanel ... />
  )}
</div>
```

Also: the `mode-toggle` was previously rendered as `<span>`. Change to `<button type="button">` and update CSS if needed.

- [ ] **Step 2: tsc + build**

```bash
cd apps/desktop && pnpm tsc -b && pnpm build
```

If TypeScript complains about `mode-toggle` previously being a span but our change makes it a button, update `App.css` to add `background: none; border: none; cursor: pointer;` to `.mode-toggle` so it visually matches.

- [ ] **Step 3: commit**

```bash
git add apps/desktop/src/routes/Workspace.tsx apps/desktop/src/App.css
git commit -m "feat(workspace): AI mode toggle wires AIMode + AIContextPanel + event hooks"
```

---

## Task 13: E2E smoke + milestone tag

This is interactive. Requires `claude` CLI on PATH and your Claude OAuth session valid.

- [ ] **Step 1: Pre-warm**

```bash
./scripts/build-engine.sh
(cd apps/desktop/src-tauri && cargo build) >/dev/null 2>&1 || true
```

- [ ] **Step 2: Run dev**

```bash
rm -rf /tmp/linetta-plan5
LINETTA_HOME=/tmp/linetta-plan5 ./scripts/dev.sh
```

- [ ] **Step 3: Manual walk-through**

1. Create a new project ("AI 테스트"). Open `씬 1`.
2. In Edit mode, type a few lines of prose. Wait for autosave to settle.
3. Click `AI` in the mode toggle. The editor area is replaced by the AIMode panel; right panel shows `AI에게 전달됨` checklist with `현재 씬 본문 / 직전 씬 발췌 / 작품 style notes`.
4. Click the **확장** preset chip → the PROMPT textarea fills with the seed sentence. Tweak it if you want.
5. Click `생성`. Within ~1–2s the streaming pane should start filling with prose, blinking cursor visible.
6. When streaming finishes, the 4 action buttons appear. Click **커서에 삽입**. The mode flips back to 편집 and a new paragraph appears at the end of the body.
7. Flip to AI again, run another generation, then click **선택 영역 교체**. The whole scene is replaced; a manual snapshot from before is in `node_snapshots` (verify via `sqlite3 /tmp/linetta-plan5/library.db "SELECT reason, count(*) FROM node_snapshots GROUP BY reason"`).
8. Flip to AI, click **생성**, then click **취소** mid-stream. The streaming stops; the panel resets to idle. `sqlite3 ... "SELECT status FROM ai_runs ORDER BY started_at DESC LIMIT 1"` should show `cancelled`.
9. If `claude` is not on PATH or auth is broken, you'll get a friendly error in the panel; the `ai_runs` row marks `error`.

If any step fails, report the exact step + observed behavior.

- [ ] **Step 4: Tag**

```bash
git tag plan-5-ai-mode-done
```

---

## Definition of Done

- `cd engine && go test ./... -race` green.
- `cd apps/desktop && pnpm tsc -b && pnpm build` green.
- `cd apps/desktop/src-tauri && cargo check` green.
- Manual walk-through (Task 13) succeeds end-to-end with real `claude` CLI.
- Tag `plan-5-ai-mode-done` exists.

Next plan: **Plan 6 — ZEN + Version restore + Export + Settings + Backups** (the last MVP plan).
