# Plan 9 — Post-MVP P2: LLM-Cached Previous-Scene Summary

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close MVP gap from spec §5.4 step 3 — "Previous-scene summary — MVP: first 300 chars trim. **Post-MVP: LLM-summarized + cached.**" When a leaf's content changes, fire-and-forget a background goroutine that asks the active LLM provider for a 3–5 sentence Korean summary and persists it on the `nodes` row. The AI `ContextBuilder` prefers the cached summary when fresh (`summary_for_version == content_version`) and otherwise falls back to the existing 300-rune trim. AI runs never block on summarization.

**Architecture:** One new engine package `summarizer` with a single-worker channel-fed goroutine. A 0004 migration adds three `NOT NULL DEFAULT`-backed columns to `nodes` (`summary`, `content_version`, `summary_for_version`). `node.Repo.UpdateContent` bumps `content_version` atomically with the doc write. New `node.Repo.SetSummary(id, summary, forVersion)` writes the cache result. The `nodes.update_content` RPC handler takes an optional `postUpdate func(id string)` callback that calls `summarizer.Enqueue(id)` after a successful save. The summarizer worker reads from a buffered channel, calls `client.Chat` non-streaming (no `OnDelta`), and writes back via `SetSummary` using the `content_version` captured before the LLM call — so a faster in-flight save that bumps the version will correctly re-enqueue and overwrite the stale result.

**Tech Stack additions:** None. Reuses the existing tars `llm.Client`, `settings.Store` `ProviderSource`, and the same `ClientFactory` the `Runner` uses.

**Spec reference:** `docs/superpowers/specs/2026-05-25-linetta-immersive-rebuild-design.md` — §5.4 step 3 (Previous-scene summary, post-MVP cache); §11.2 P2.

**Design decisions locked by the user:**
1. **Background, non-blocking.** `nodes.update_content` returns immediately; summarization fires-and-forgets after the RPC succeeds.
2. **Stale detection via versions.** `summary` is fresh iff `summary != "" AND summary_for_version == content_version`.
3. **Fallback chain in `ContextBuilder`.** Fresh cache wins; otherwise the existing 300-rune trim runs.
4. **Same provider as runs.** No tier routing in MVP. Reuse `settings.Store` as `ProviderSource` and the same `ClientFactory`.
5. **Drop on backpressure.** `Enqueue` uses `select { case ch <- id: default: }` — no blocking on the RPC path. Duplicate enqueues within the buffer naturally batch (the worker re-checks freshness before calling the LLM).

---

## Pre-flight

- [ ] Plan 8 is tagged (`plan-8-relationships-done`) and `git status --short` is empty.
- [ ] `cd engine && go test ./... -race` green.
- [ ] `cd apps/desktop && pnpm tsc -b && pnpm build` green.
- [ ] Confirm the `nodes` table is missing the new columns: `sqlite3 "$LINETTA_HOME/library.db" ".schema nodes"` shows no `summary`/`content_version`/`summary_for_version`.

---

## File Structure

```
engine/internal/store/migrations/
  0004_node_summary_cache.sql            (new — 3 ADD COLUMN NOT NULL DEFAULT)

engine/internal/node/
  node.go                                (modified — 3 new fields on Node)
  repo.go                                (modified — UpdateContent bumps version; new SetSummary; scan picks up new cols)
  repo_test.go                           (modified — new test cases)

engine/internal/summarizer/
  summarizer.go                          (new — Summarizer struct + Start/Enqueue/worker)
  summarizer_test.go                     (new — fake llm.Client + queue + freshness + version-race tests)

engine/internal/rpc/handlers/
  nodes.go                               (modified — UpdateNodeContent gains postUpdate arg)
  nodes_test.go                          (modified — pass nil for new arg)

engine/internal/ai/
  context.go                             (modified — prefer prev.Summary when fresh)
  context_test.go                        (modified — 3 new sub-cases)

engine/cmd/linetta-engine/main.go        (modified — instantiate summarizer, wire postUpdate)
```

---

## Phase A: Schema + Node repo (2 tasks)

### Task 1: 0004 migration — `summary`, `content_version`, `summary_for_version`

The existing `nodes` table has none of these columns. SQLite supports `ALTER TABLE ... ADD COLUMN <name> <type> NOT NULL DEFAULT <literal>` cleanly — the `DEFAULT` is what backfills every existing row. Bare `NOT NULL` (no default) would refuse on a non-empty table. The `migrations_test.go` harness re-applies all embedded migrations on a fresh DB; no edits required.

**Files:**
- Create: `engine/internal/store/migrations/0004_node_summary_cache.sql`

- [ ] **Step 1: Write the migration**

```sql
-- Plan 9: LLM-cached previous-scene summary.
ALTER TABLE nodes ADD COLUMN summary TEXT NOT NULL DEFAULT '';
ALTER TABLE nodes ADD COLUMN content_version INTEGER NOT NULL DEFAULT 0;
ALTER TABLE nodes ADD COLUMN summary_for_version INTEGER NOT NULL DEFAULT 0;
```

- [ ] **Step 2: Verify**

```bash
cd engine && go test ./internal/store/... -race
```

- [ ] **Step 3: Commit**

```bash
git add engine/internal/store/migrations/0004_node_summary_cache.sql
git commit -m "feat(store): 0004 add summary cache columns to nodes"
```

---

### Task 2: `node.Repo` — expose new fields, bump `content_version`, add `SetSummary`

**Files:**
- Modify: `engine/internal/node/node.go`
- Modify: `engine/internal/node/repo.go`
- Modify: `engine/internal/node/repo_test.go`

- [ ] **Step 1: Failing tests (append to `repo_test.go`)**

```go
func TestRepo_UpdateContent_bumpsContentVersion(t *testing.T) {
	s, p := newStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()

	before, _ := r.Get(ctx, *p.LastOpenedNodeID)
	if before.ContentVersion != 0 {
		t.Fatalf("seeded content_version = %d, want 0", before.ContentVersion)
	}

	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"한"}]}]}`
	_ = r.UpdateContent(ctx, *p.LastOpenedNodeID, doc, 9999)
	_ = r.UpdateContent(ctx, *p.LastOpenedNodeID, doc, 10000)

	after, _ := r.Get(ctx, *p.LastOpenedNodeID)
	if after.ContentVersion != 2 {
		t.Errorf("content_version = %d, want 2", after.ContentVersion)
	}
}

func TestRepo_SetSummary_writesBothFields(t *testing.T) {
	s, p := newStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()

	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"a"}]}]}`
	for i := 0; i < 3; i++ {
		_ = r.UpdateContent(ctx, *p.LastOpenedNodeID, doc, int64(1000+i))
	}
	if err := r.SetSummary(ctx, *p.LastOpenedNodeID, "요약된 본문.", 3); err != nil {
		t.Fatalf("SetSummary: %v", err)
	}
	got, _ := r.Get(ctx, *p.LastOpenedNodeID)
	if got.Summary != "요약된 본문." || got.SummaryForVersion != 3 || got.ContentVersion != 3 {
		t.Errorf("got = %+v", got)
	}
}

func TestRepo_SetSummary_unknownID_returnsErrNotFound(t *testing.T) {
	s, _ := newStoreAndProject(t)
	r := NewRepo(s)
	if err := r.SetSummary(context.Background(), "no-such", "x", 1); err != ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
```

If `newStoreAndProject` helper doesn't exist in the test file, mirror the pattern from `TestRepo_Create_returnsProjectWithGeneratedID_andFirstLeafNode` in `engine/internal/project/repo_test.go`.

- [ ] **Step 2: Add fields to `Node` struct (`node.go`)**

```go
type Node struct {
	ID                string  `json:"id"`
	ProjectID         string  `json:"project_id"`
	ParentID          *string `json:"parent_id,omitempty"`
	Ordinal           int     `json:"ordinal"`
	Kind              string  `json:"kind"`
	Label             string  `json:"label"`
	Title             string  `json:"title"`
	ContentDoc        *string `json:"content_doc,omitempty"`
	Status            string  `json:"status"`
	WordCount         int     `json:"word_count"`
	Summary           string  `json:"summary"`
	ContentVersion    int     `json:"content_version"`
	SummaryForVersion int     `json:"summary_for_version"`
	CreatedAt         int64   `json:"created_at"`
	UpdatedAt         int64   `json:"updated_at"`
}
```

- [ ] **Step 3: Update `repo.go` baseSelect + scan + UpdateContent + SetSummary**

Replace `baseSelect`:

```go
const baseSelect = `
SELECT id, project_id, parent_id, ordinal, kind, label, title,
       content_doc, status, word_count, summary, content_version,
       summary_for_version, created_at, updated_at
FROM nodes`
```

Update `scan`:

```go
func scan(row scanner) (Node, error) {
	var (
		n          Node
		parentID   sql.NullString
		contentDoc sql.NullString
	)
	if err := row.Scan(&n.ID, &n.ProjectID, &parentID, &n.Ordinal, &n.Kind, &n.Label, &n.Title,
		&contentDoc, &n.Status, &n.WordCount, &n.Summary, &n.ContentVersion,
		&n.SummaryForVersion, &n.CreatedAt, &n.UpdatedAt); err != nil {
		return Node{}, err
	}
	if parentID.Valid {
		v := parentID.String
		n.ParentID = &v
	}
	if contentDoc.Valid {
		v := contentDoc.String
		n.ContentDoc = &v
	}
	return n, nil
}
```

Inside `UpdateContent`, change the inner UPDATE to include `content_version = content_version + 1`:

```go
	res, err := tx.ExecContext(ctx, `
UPDATE nodes
   SET content_doc = ?, word_count = ?, updated_at = ?,
       content_version = content_version + 1
 WHERE id = ?`, doc, count, now, id)
```

Append new method:

```go
// SetSummary writes the LLM-generated summary and the version it was generated
// for. Does NOT touch updated_at — derived field, not user content.
func (r *Repo) SetSummary(ctx context.Context, id string, summary string, forVersion int) error {
	res, err := r.s.DB().ExecContext(ctx, `
UPDATE nodes SET summary = ?, summary_for_version = ? WHERE id = ?`,
		summary, forVersion, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

```bash
cd engine && go test ./internal/node/... -race
```

- [ ] **Step 5: Commit**

```bash
git add engine/internal/node/
git commit -m "feat(node): bump content_version on update + add SetSummary"
```

---

## Phase B: Summarizer package (2 tasks)

### Task 3: `engine/internal/summarizer` package

**Files:**
- Create: `engine/internal/summarizer/summarizer.go`
- Create: `engine/internal/summarizer/summarizer_test.go`

- [ ] **Step 1: Failing tests**

`engine/internal/summarizer/summarizer_test.go`:

```go
package summarizer

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/ai"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/tars/pkg/llm"
)

type fixedProvider string

func (p fixedProvider) Provider() string { return string(p) }

type fakeClient struct {
	mu       sync.Mutex
	calls    [][]llm.ChatMessage
	response string
	block    chan struct{}
}

func (f *fakeClient) Ask(ctx context.Context, prompt string) (string, error) {
	return "", errors.New("not used")
}

func (f *fakeClient) Chat(ctx context.Context, messages []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return llm.ChatResponse{}, ctx.Err()
		}
	}
	f.mu.Lock()
	f.calls = append(f.calls, messages)
	f.mu.Unlock()
	return llm.ChatResponse{Message: llm.ChatMessage{Content: f.response}}, nil
}

func (f *fakeClient) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func newFixture(t *testing.T) (*store.Store, *node.Repo, project.Project) {
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
	return s, node.NewRepo(s), p
}

func longDoc(text string) string {
	return `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + text + `"}]}]}`
}

func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out: %s", msg)
}

func TestSummarizer_writesSummaryAndMatchesVersion(t *testing.T) {
	_, nodes, p := newFixture(t)
	ctx := context.Background()
	body := ""
	for i := 0; i < 200; i++ { body += "가나다라마" }
	_ = nodes.UpdateContent(ctx, *p.LastOpenedNodeID, longDoc(body), 1100)

	fake := &fakeClient{response: "이것은 요약문이다."}
	sum := New(nodes, fixedProvider("fake"),
		func(_, _ string) (llm.Client, error) { return fake, nil })
	stop := sum.Start(ctx)
	defer stop()

	sum.Enqueue(*p.LastOpenedNodeID)
	waitFor(t, func() bool {
		n, _ := nodes.Get(ctx, *p.LastOpenedNodeID)
		return n.Summary == "이것은 요약문이다."
	}, "summary lands")

	n, _ := nodes.Get(ctx, *p.LastOpenedNodeID)
	if n.SummaryForVersion != n.ContentVersion {
		t.Errorf("versions: summary_for=%d content=%d", n.SummaryForVersion, n.ContentVersion)
	}
}

func TestSummarizer_skipsWhenAlreadyFresh(t *testing.T) {
	_, nodes, p := newFixture(t)
	ctx := context.Background()
	body := ""
	for i := 0; i < 200; i++ { body += "가" }
	_ = nodes.UpdateContent(ctx, *p.LastOpenedNodeID, longDoc(body), 1100)
	n, _ := nodes.Get(ctx, *p.LastOpenedNodeID)
	_ = nodes.SetSummary(ctx, n.ID, "이미 요약됨.", n.ContentVersion)

	fake := &fakeClient{response: "새로 만든 요약."}
	sum := New(nodes, fixedProvider("fake"),
		func(_, _ string) (llm.Client, error) { return fake, nil })
	stop := sum.Start(ctx)
	defer stop()

	sum.Enqueue(*p.LastOpenedNodeID)
	time.Sleep(100 * time.Millisecond)

	if fake.callCount() != 0 {
		t.Errorf("LLM called %d times despite fresh cache", fake.callCount())
	}
}

func TestSummarizer_shortContent_writesPlaintextWithoutLLM(t *testing.T) {
	_, nodes, p := newFixture(t)
	ctx := context.Background()
	_ = nodes.UpdateContent(ctx, *p.LastOpenedNodeID, longDoc("짧다."), 1100)

	fake := &fakeClient{response: "should not be used"}
	sum := New(nodes, fixedProvider("fake"),
		func(_, _ string) (llm.Client, error) { return fake, nil })
	stop := sum.Start(ctx)
	defer stop()

	sum.Enqueue(*p.LastOpenedNodeID)
	waitFor(t, func() bool {
		n, _ := nodes.Get(ctx, *p.LastOpenedNodeID)
		return n.Summary != ""
	}, "short summary lands")

	if fake.callCount() != 0 {
		t.Errorf("LLM called %d times for short content", fake.callCount())
	}
}

func TestSummarizer_reRunsAfterContentChange(t *testing.T) {
	_, nodes, p := newFixture(t)
	ctx := context.Background()
	body := ""
	for i := 0; i < 200; i++ { body += "가" }
	_ = nodes.UpdateContent(ctx, *p.LastOpenedNodeID, longDoc(body), 1100)

	fake := &fakeClient{response: "v1 요약"}
	sum := New(nodes, fixedProvider("fake"),
		func(_, _ string) (llm.Client, error) { return fake, nil })
	stop := sum.Start(ctx)
	defer stop()

	sum.Enqueue(*p.LastOpenedNodeID)
	waitFor(t, func() bool {
		n, _ := nodes.Get(ctx, *p.LastOpenedNodeID)
		return n.Summary == "v1 요약"
	}, "v1")

	body2 := ""
	for i := 0; i < 200; i++ { body2 += "나" }
	_ = nodes.UpdateContent(ctx, *p.LastOpenedNodeID, longDoc(body2), 1200)
	fake.mu.Lock()
	fake.response = "v2 요약"
	fake.mu.Unlock()
	sum.Enqueue(*p.LastOpenedNodeID)
	waitFor(t, func() bool {
		n, _ := nodes.Get(ctx, *p.LastOpenedNodeID)
		return n.Summary == "v2 요약"
	}, "v2")
}

func TestSummarizer_enqueueIsNonBlocking(t *testing.T) {
	_, nodes, _ := newFixture(t)
	ctx := context.Background()
	fake := &fakeClient{response: "ok", block: make(chan struct{})}
	sum := New(nodes, fixedProvider("fake"),
		func(_, _ string) (llm.Client, error) { return fake, nil })
	stop := sum.Start(ctx)
	defer func() {
		close(fake.block)
		stop()
	}()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 10_000; i++ {
			sum.Enqueue("any-id")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Enqueue blocked under flood")
	}
}

// Compile-time guard that ai.ProviderSource is satisfied.
var _ ai.ProviderSource = fixedProvider("")
```

- [ ] **Step 2: Implementation**

`engine/internal/summarizer/summarizer.go`:

```go
// Package summarizer keeps node.summary in sync with node.content_doc by
// running a background LLM summarization job after every content change.
package summarizer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/devlikebear/linetta/engine/internal/ai"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/tars/pkg/llm"
)

const queueSize = 256
const minRunesForLLM = 60
const systemPrompt = "다음 한국어 본문을 3~5문장으로 요약하라. 등장인물·장소·핵심 사건은 반드시 보존하라. 새 정보 추가 금지."

type Summarizer struct {
	nodes   *node.Repo
	src     ai.ProviderSource
	factory ai.ClientFactory
	ch      chan string
}

func New(nodes *node.Repo, src ai.ProviderSource, factory ai.ClientFactory) *Summarizer {
	return &Summarizer{
		nodes: nodes, src: src, factory: factory,
		ch: make(chan string, queueSize),
	}
}

func (s *Summarizer) Start(parent context.Context) (stop func()) {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case id := <-s.ch:
				s.summarizeOne(ctx, id)
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			<-done
		})
	}
}

func (s *Summarizer) Enqueue(nodeID string) {
	if nodeID == "" {
		return
	}
	select {
	case s.ch <- nodeID:
	default:
		// queue full — drop; a later save will re-enqueue.
	}
}

func (s *Summarizer) summarizeOne(ctx context.Context, nodeID string) {
	n, err := s.nodes.Get(ctx, nodeID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "summarizer: get %s: %v\n", nodeID, err)
		return
	}
	if n.Kind != "leaf" || n.ContentDoc == nil {
		return
	}
	if n.Summary != "" && n.SummaryForVersion == n.ContentVersion {
		return
	}

	capturedVersion := n.ContentVersion
	plain := strings.TrimSpace(docToPlainText(*n.ContentDoc))
	if plain == "" {
		return
	}

	if runeLen(plain) < minRunesForLLM {
		if err := s.nodes.SetSummary(ctx, nodeID, plain, capturedVersion); err != nil {
			fmt.Fprintf(os.Stderr, "summarizer: SetSummary (short) %s: %v\n", nodeID, err)
		}
		return
	}

	provider := s.src.Provider()
	client, err := s.factory(provider, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "summarizer: factory(%s): %v\n", provider, err)
		return
	}

	msgs := []llm.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: plain},
	}
	resp, err := client.Chat(ctx, msgs, llm.ChatOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "summarizer: Chat %s: %v\n", nodeID, err)
		return
	}
	summary := strings.TrimSpace(resp.Message.Content)
	if summary == "" {
		return
	}
	if err := s.nodes.SetSummary(ctx, nodeID, summary, capturedVersion); err != nil {
		fmt.Fprintf(os.Stderr, "summarizer: SetSummary %s: %v\n", nodeID, err)
	}
}

func runeLen(s string) int { return len([]rune(s)) }

func docToPlainText(raw string) string {
	if raw == "" {
		return ""
	}
	var v interface{}
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return ""
	}
	var sb strings.Builder
	walkDoc(v, &sb)
	return sb.String()
}

func walkDoc(v interface{}, sb *strings.Builder) {
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
		if content, ok := t["content"].([]interface{}); ok {
			for _, c := range content {
				walkDoc(c, sb)
			}
		}
		if kind == "paragraph" || kind == "heading" || kind == "blockquote" {
			sb.WriteString("\n")
		}
	case []interface{}:
		for _, c := range t {
			walkDoc(c, sb)
		}
	}
}
```

- [ ] **Step 3: Run + commit**

```bash
cd engine && go test ./internal/summarizer/... -race
git add engine/internal/summarizer/
git commit -m "feat(summarizer): background LLM cache for previous-scene summary"
```

---

### Task 4: Wire summarizer + `postUpdate` hook on `nodes.update_content`

**Files:**
- Modify: `engine/internal/rpc/handlers/nodes.go`
- Modify: `engine/internal/rpc/handlers/nodes_test.go`
- Modify: `engine/cmd/linetta-engine/main.go`

- [ ] **Step 1: Extend `UpdateNodeContent` signature**

In `engine/internal/rpc/handlers/nodes.go`, change signature to take a `postUpdate func(nodeID string)` arg. Call it AFTER successful save + autosave + re-Get, only if non-nil:

```go
func UpdateNodeContent(nodes *node.Repo, snaps *snapshot.Repo, now Clock, postUpdate func(nodeID string)) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		// ... existing logic unchanged ...
		got, err := nodes.Get(ctx, p.ID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if postUpdate != nil {
			postUpdate(p.ID)
		}
		return json.Marshal(got)
	}
}
```

- [ ] **Step 2: Update test fixture calls**

In `engine/internal/rpc/handlers/nodes_test.go`, every call to `UpdateNodeContent(...)` gets a trailing `nil`. Add two new tests:

```go
func TestUpdateNodeContentHandler_callsPostUpdateAfterSuccess(t *testing.T) {
	f := newNodeFixture(t)
	var got []string
	hook := func(id string) { got = append(got, id) }
	h := UpdateNodeContent(f.nodes, f.snaps, func() int64 { return 10_000 }, hook)
	if _, err := h(context.Background(), json.RawMessage(`{"id":"`+f.nID+`","doc":"{\"type\":\"doc\"}"}`)); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if len(got) != 1 || got[0] != f.nID {
		t.Errorf("postUpdate calls = %v, want [%q]", got, f.nID)
	}
}

func TestUpdateNodeContentHandler_doesNotCallPostUpdateOnError(t *testing.T) {
	f := newNodeFixture(t)
	called := 0
	hook := func(id string) { called++ }
	h := UpdateNodeContent(f.nodes, f.snaps, func() int64 { return 10_000 }, hook)
	if _, err := h(context.Background(), json.RawMessage(`{"id":"no-such","doc":"{}"}`)); err == nil {
		t.Fatal("expected error")
	}
	if called != 0 {
		t.Errorf("postUpdate called %d times on error", called)
	}
}
```

- [ ] **Step 3: Wire in `main.go`**

Add import `"github.com/devlikebear/linetta/engine/internal/summarizer"`. After `runner :=`:

```go
summ := summarizer.New(nodes, settingsStore, ai.DefaultClientFactory)
stopSummarizer := summ.Start(ctx)
defer stopSummarizer()
```

Update the handler line:

```go
s.Handle("nodes.update_content", handlers.UpdateNodeContent(nodes, snaps, clock, summ.Enqueue))
```

- [ ] **Step 4: Run + commit**

```bash
cd engine && go test ./... -race && go build ./cmd/linetta-engine
git add engine/internal/rpc/handlers/nodes.go engine/internal/rpc/handlers/nodes_test.go engine/cmd/linetta-engine/main.go
git commit -m "feat(handlers): enqueue background summarizer after nodes.update_content"
```

---

## Phase C: AI context cache hit (1 task)

### Task 5: Prefer `prev.Summary` when fresh in `ContextBuilder.Build`

**Files:**
- Modify: `engine/internal/ai/context.go`
- Modify: `engine/internal/ai/context_test.go`

- [ ] **Step 1: Failing tests**

Append to `engine/internal/ai/context_test.go` 3 tests covering cache hit / stale / empty fallback. Pattern: same fixture builder as existing `TestBuildContext_*` tests; for cache hit, after `UpdateContent`, call `nodes.SetSummary(prevN.ID, "캐시된 요약", prevN.ContentVersion)` and assert `got.PrevSummary == "캐시된 요약"`. For stale, set summary for an older version (`prevN.ContentVersion-1` or `0`). For empty, don't call SetSummary.

- [ ] **Step 2: Implementation**

In `engine/internal/ai/context.go`, find where `prevSummary` is computed (currently a single `trimRunes(docToPlainText(...), prevSummaryMaxRunes)` call). Replace with:

```go
prevSummary := ""
if prev != nil {
	if prev.Summary != "" && prev.SummaryForVersion == prev.ContentVersion {
		prevSummary = prev.Summary
	} else if prev.ContentDoc != nil {
		prevSummary = trimRunes(docToPlainText(*prev.ContentDoc), prevSummaryMaxRunes)
	}
}
```

If `prev` is currently passed as a value (not pointer) or `prev.ContentDoc` is already dereferenced, adapt to the actual current shape. The key behavior is: cache hit short-circuits the trim.

- [ ] **Step 3: Run + commit**

```bash
cd engine && go test ./internal/ai/... -race
git add engine/internal/ai/context.go engine/internal/ai/context_test.go
git commit -m "feat(ai): prefer cached prev-scene summary when fresh, else 300-rune trim"
```

---

## Phase D: Smoke + tag (1 task)

### Task 6: End-to-end smoke + tag

- [ ] **Step 1: Rebuild engine + launch**

```bash
./scripts/build-engine.sh
LINETTA_HOME=/tmp/linetta-plan9 ./scripts/dev.sh
```

- [ ] **Step 2: Write a long first scene (~400+ chars Korean with 2-3 @mentions)**

Wait ~3 seconds after typing stops (autosave + summarizer).

- [ ] **Step 3: Inspect**

```bash
sqlite3 /tmp/linetta-plan9/library.db \
  "SELECT label, length(summary), content_version, summary_for_version FROM nodes WHERE kind='leaf' ORDER BY ordinal"
```

Expect: non-zero summary length, `content_version == summary_for_version` (both > 0).

- [ ] **Step 4: Create 씬 2, switch to AI mode, run any generation**

- [ ] **Step 5: Inspect AI context**

```bash
sqlite3 /tmp/linetta-plan9/library.db \
  "SELECT json_extract(context_json,'\$.prev_summary') FROM ai_runs ORDER BY started_at DESC LIMIT 1"
```

Expect: 3–5 sentence Korean summary (not a 300-char prefix; no `…` ellipsis).

- [ ] **Step 6: Edit 씬 1 significantly, wait, reverify**

`content_version` bumped, then `summary_for_version` catches up, `summary` text changed.

- [ ] **Step 7: Tag**

```bash
git tag plan-9-prev-summary-cache-done
```

---

## Done conditions

- [ ] `cd engine && go test ./... -race` green.
- [ ] `cd apps/desktop && pnpm tsc -b && pnpm build` green.
- [ ] Smoke walk-through Steps 3, 5, 6 all show expected output.
- [ ] `plan-9-prev-summary-cache-done` tag exists.
