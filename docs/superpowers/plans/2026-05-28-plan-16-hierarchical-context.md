# Plan 16 — Hierarchical AI Context (장편 연재 대응)

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Long-form drafts (multi-부, multi-장 novels) overflow the single-leaf+prev-summary AI context shipped in Plan 9. The project's existing node tree + mentions + threads + relationships already form an implicit "graph index" — instead of stuffing the full doc into every AI call, this plan ships **layers 1–3 of a five-layer hierarchically compressed context**: (1) container summary rollup, (2) entity dossier, (3) topology-RAG via co-mention. Layers 4 (pinned spans) and 5 (embedding retrieval) stay deferred.

**Architecture:** No schema migration. We reuse the Plan 9 `summary` / `content_version` / `summary_for_version` columns for *every* node, including containers. `node.Repo.UpdateContent` bumps `content_version` on every ancestor via a recursive CTE so a stale container is trivially detectable as `summary_for_version < content_version`. `summarizer.summarizeOne` extends to handle containers by concatenating its children's `Label + summary` and re-running the existing LLM call with a container-flavored system prompt. `ai.ContextBuilder.Build` gains three helpers (`loadHierarchicalContext`, `dossierFor`, `loadRelatedScenes`) that read these summaries lazily — on cache miss it synchronously triggers `summarizer.summarizeOne` so the answer is never stale. `prompts.go::buildUser` learns five new section headers and re-orders the output. `ai.Context` gains three new fields, all snapshotted into `ai_runs.context_json` for audit.

**Tech Stack additions:** None. Reuses `node.Repo`, `summarizer.Summarizer`, `mention.Repo`, the existing `llm.Client` plumbing.

**Spec reference:** internal design memo "Hierarchical AI Context for Linetta" (2026-05-28). Pin layer (4) and embedding layer (5) are deferred to Plan 17 / Plan 18.

**Design decisions locked by the user:**

1. **Container summary rollup (layer 1)** — lazy. `nodes.UpdateContent` propagates `content_version + 1` up the ancestor chain via recursive CTE. ContextBuilder, when it needs an ancestor's summary, checks `summary != "" AND summary_for_version == content_version`; on miss, synchronously invokes `summarizer.summarizeOne(containerID)`. Distance-based selection: current leaf body verbatim → other leaves in same 장 → other 장 in same 부 (장 rollups) → other 부 (부 rollups) → root synopsis.
2. **Entity dossier (layer 2)** — no new schema. For each mentioned entity, query `mentions JOIN nodes` for the latest 5 leaves where it appears (excluding current), take each `summary`'s first line; attach as `Recent []string` on `EntityBrief`.
3. **Topology RAG (layer 3)** — co-mention top-3. Given mentions of the current node `E = {e1, …}`, find leaves where ≥ 2 entities from `E` co-appear; rank by `co_mention_count DESC, updated_at DESC`; take top 3. Exclude leaves already in the nearby/same-chapter sets (set-based filter on the Go side).
4. **Lazy synchronous rollup.** On a stale container hit, `ContextBuilder` calls `summarizer.summarizeOne` inline. We accept the latency cost on the AI-Start path; nightly autosaves keep the cache warm so a cold call only happens on a freshly-edited container.
5. **Budgets.** `hierarchicalMaxChars = 2500` total budget for layer 1's rendered text; `containerSummaryMaxRunes = 4000` for the LLM input that produces a container rollup; entity dossier `Recent` limited to first-line of last 5 leaves; topology RAG hard-capped at 3 results.
6. **Section order in the prompt = same order in `Context.JSON`**, so the audit log in `ai_runs.context_json` matches what the LLM saw.

---

## Pre-flight

- [ ] Plan 15 is tagged (`plan-15-ux-polish-done`) and `git status --short` is empty.
- [ ] `cd engine && go test ./... -race` green.
- [ ] `cd apps/desktop && pnpm tsc -b && pnpm build` green.
- [ ] Confirm the schema already has the summary cache columns from Plan 9: `sqlite3 "$LINETTA_HOME/library.db" ".schema nodes"` shows `summary`, `content_version`, `summary_for_version`. **No new migration in Plan 16.**
- [ ] Confirm Plan 14 markdown import is wired so the smoke walk has a multi-chapter project available.

---

## File Structure

```
engine/internal/node/
  repo.go                                (modified — UpdateContent propagates content_version + 1 to ancestors; new ListChildren helper)
  repo_test.go                           (modified — new ancestor-bump test)

engine/internal/summarizer/
  summarizer.go                          (modified — summarizeOne handles kind=='container'; container system prompt; depth guard)
  summarizer_test.go                     (modified — depth-2 tree rollup test)

engine/internal/ai/
  ai.go                                  (modified — Context.Hierarchical, RelatedScenes; EntityBrief.Recent; SceneSummary type)
  context.go                             (modified — loadHierarchicalContext, dossierFor, loadRelatedScenes wiring)
  context_test.go                        (modified — 3 new tests: layer 1, layer 2, layer 3)
  prompts.go                             (modified — buildUser re-ordered, 5 new sections, dossier-under-entity rendering)
  prompts_test.go                        (modified if exists — golden text additions)
```

No new files. No new migrations. No new dependencies.

---

## Phase A: Schema-aware ancestor versioning (1 task)

### Task 1 — `node.Repo.UpdateContent` propagates `content_version + 1` to ancestors

**Why:** Without this, a 장 / 부 container's `summary_for_version` is forever `0` and its `content_version` is forever `0`, so the freshness predicate `summary_for_version == content_version` is always true even after a descendant leaf has changed. We want "any descendant write makes every ancestor stale."

**Files:**
- Modify: `engine/internal/node/repo.go`
- Modify: `engine/internal/node/repo_test.go`

- [ ] **Step 1: Failing test (append to `repo_test.go`)**

```go
func TestRepo_UpdateContent_bumpsAncestorContentVersion(t *testing.T) {
	s, p := newStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()

	// 부 → 장 → 씬 tree. The seeded project has one root leaf; we'll instead
	// build a fresh container chain via CreateSibling/CreateChild.
	part, err := r.CreateSibling(ctx, *p.LastOpenedNodeID, "container", "1부", "", 1100)
	if err != nil {
		t.Fatalf("part: %v", err)
	}
	chapter, err := r.CreateChild(ctx, part.ID, "container", "1장", "", 1110)
	if err != nil {
		t.Fatalf("chapter: %v", err)
	}
	scene, err := r.CreateChild(ctx, chapter.ID, "leaf", "씬 1", "", 1120)
	if err != nil {
		t.Fatalf("scene: %v", err)
	}

	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"한"}]}]}`
	if err := r.UpdateContent(ctx, scene.ID, doc, 1200); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}

	gotScene, _ := r.Get(ctx, scene.ID)
	gotChap, _ := r.Get(ctx, chapter.ID)
	gotPart, _ := r.Get(ctx, part.ID)
	if gotScene.ContentVersion != 1 {
		t.Errorf("scene.content_version = %d, want 1", gotScene.ContentVersion)
	}
	if gotChap.ContentVersion != 1 {
		t.Errorf("chapter.content_version = %d, want 1 (ancestor bumped)", gotChap.ContentVersion)
	}
	if gotPart.ContentVersion != 1 {
		t.Errorf("part.content_version = %d, want 1 (ancestor bumped)", gotPart.ContentVersion)
	}
	if gotChap.SummaryForVersion != 0 || gotPart.SummaryForVersion != 0 {
		t.Errorf("ancestor summary_for_version should still be 0 (stale): chap=%d part=%d",
			gotChap.SummaryForVersion, gotPart.SummaryForVersion)
	}

	// Second write bumps all three again.
	if err := r.UpdateContent(ctx, scene.ID, doc, 1300); err != nil {
		t.Fatalf("UpdateContent#2: %v", err)
	}
	gotChap2, _ := r.Get(ctx, chapter.ID)
	if gotChap2.ContentVersion != 2 {
		t.Errorf("after second write, chapter.content_version = %d, want 2", gotChap2.ContentVersion)
	}
}
```

If `newStoreAndProject` isn't already a helper in the file, mirror the existing pattern in `repo_test.go` (it was added in Plan 9).

- [ ] **Step 2: Modify `UpdateContent` in `engine/internal/node/repo.go`**

After the existing leaf `UPDATE nodes ... content_version = content_version + 1 WHERE id = ?` but **before** the `projects` total recompute, insert the ancestor bump. modernc.org/sqlite (the driver used per `engine/internal/store/store.go`) fully supports `WITH RECURSIVE`. The CTE walks up via `parent_id` and the outer `UPDATE` matches `nodes.id` against the CTE row set.

```go
	res, err := tx.ExecContext(ctx, `
UPDATE nodes
   SET content_doc = ?, word_count = ?, updated_at = ?,
       content_version = content_version + 1
 WHERE id = ?`, doc, count, now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}

	// Plan 16: every ancestor's content_version is bumped so the descendant's
	// edit invalidates ancestor container summaries (summary_for_version <
	// content_version → stale).
	if _, err := tx.ExecContext(ctx, `
WITH RECURSIVE ancestors(id) AS (
  SELECT parent_id FROM nodes WHERE id = ? AND parent_id IS NOT NULL
  UNION ALL
  SELECT n.parent_id FROM nodes n
    JOIN ancestors a ON n.id = a.id
   WHERE n.parent_id IS NOT NULL
)
UPDATE nodes
   SET content_version = content_version + 1,
       updated_at = ?
 WHERE id IN (SELECT id FROM ancestors)`, id, now); err != nil {
		return fmt.Errorf("bump ancestor content_version: %w", err)
	}
```

Note: `updated_at` is also bumped on ancestors so the existing topology-RAG query (Task 5) can use `MAX(n.updated_at)` for recency ranking. No behaviour change elsewhere — containers' `updated_at` is otherwise touched only by `Rename` / `swap` / `Delete`.

- [ ] **Step 3: Also add the `ListChildren` helper used by Task 2**

Append at the bottom of `repo.go`:

```go
// ListChildren returns the direct children of parentID in ordinal order.
func (r *Repo) ListChildren(ctx context.Context, parentID string) ([]Node, error) {
	rows, err := r.s.DB().QueryContext(ctx, baseSelect+`
WHERE parent_id = ?
ORDER BY ordinal`, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Node
	for rows.Next() {
		n, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Run tests**

```bash
cd engine && go test ./internal/node/... -race
```

- [ ] **Step 5: Commit**

```bash
git add engine/internal/node/
git commit -m "feat(node): propagate content_version to ancestors and add ListChildren"
```

---

## Phase B: Container-aware summarizer (1 task)

### Task 2 — `summarizer.summarizeOne` handles `kind == "container"`

**Why:** Until now `summarizeOne` early-returns on `n.Kind != "leaf"`. To fill in 장 / 부 / root rollups we need it to recurse into children. The "input" for a container is `Label\nsummary\n` for each child (truncated to `containerSummaryMaxRunes = 4000`). If a child container is itself stale we synchronously call `summarizeOne(child.ID)` first (base case: leaves, which use the existing branch).

**Order-of-operations example** for a tree `1부 → 1장 → {씬 1, 씬 2}`:

1. `summarizeOne(1부)` → loads children: `[1장]`. 1장's summary is stale → recurse.
2. `summarizeOne(1장)` → loads children: `[씬 1, 씬 2]`. Both leaves; if their summaries are stale, recurse into each.
3. `summarizeOne(씬 1)` → existing leaf path → LLM call → `SetSummary`. Same for 씬 2.
4. Back in `summarizeOne(1장)` — children's summaries are now fresh; concatenate `씬 1\n{summary1}\n\n씬 2\n{summary2}` and run a container LLM call; `SetSummary`.
5. Back in `summarizeOne(1부)` — 1장 is fresh; concatenate `1장\n{summary1장}` and run; `SetSummary`.

**Depth guard:** hard cap depth at 6 (well past realistic novel hierarchies). Cycles cannot occur (FK parent_id is acyclic by construction), but the guard saves us if a future schema relaxation breaks that invariant.

**Files:**
- Modify: `engine/internal/summarizer/summarizer.go`
- Modify: `engine/internal/summarizer/summarizer_test.go`

- [ ] **Step 1: Failing test (append to `summarizer_test.go`)**

```go
func TestSummarizer_containerRollupBuildsDepth2Tree(t *testing.T) {
	_, nodes, p := newFixture(t)
	ctx := context.Background()

	// Build 부 → 장 → 씬 tree. The seeded project has one root leaf; create a
	// part container as a sibling and put a chapter + two scenes under it.
	part, _ := nodes.CreateSibling(ctx, *p.LastOpenedNodeID, "container", "1부", "", 1100)
	chap, _ := nodes.CreateChild(ctx, part.ID, "container", "1장", "", 1110)
	scene1, _ := nodes.CreateChild(ctx, chap.ID, "leaf", "씬 1", "", 1120)
	scene2, _ := nodes.CreateChild(ctx, chap.ID, "leaf", "씬 2", "", 1130)

	body := ""
	for i := 0; i < 200; i++ {
		body += "가나다라마"
	}
	_ = nodes.UpdateContent(ctx, scene1.ID, longDoc(body), 1200)
	_ = nodes.UpdateContent(ctx, scene2.ID, longDoc(body), 1210)

	// Fake LLM returns a unique sentence per call so we can prove the chain.
	type capture struct {
		input string
		reply string
	}
	var (
		mu    sync.Mutex
		calls []capture
	)
	fake := &fakeClient{response: ""}
	// Replace Chat with a custom impl by wrapping in a wrapperClient. Cleanest
	// path: extend the existing fakeClient to support per-call response func.
	fake.responseFn = func(messages []llm.ChatMessage) string {
		mu.Lock()
		defer mu.Unlock()
		userMsg := messages[len(messages)-1].Content
		reply := fmt.Sprintf("요약#%d", len(calls)+1)
		calls = append(calls, capture{input: userMsg, reply: reply})
		return reply
	}

	sum := New(nodes, fixedProvider("fake"),
		func(_, _ string) (llm.Client, error) { return fake, nil })
	stop := sum.Start(ctx)
	defer stop()

	sum.Enqueue(part.ID)

	waitFor(t, func() bool {
		n, _ := nodes.Get(ctx, part.ID)
		return n.Summary != "" && n.SummaryForVersion == n.ContentVersion
	}, "part summary lands")

	mu.Lock()
	defer mu.Unlock()
	if len(calls) < 4 {
		// 4 = 씬 1, 씬 2, 1장, 1부. (Could be ≥4 if duplicate enqueues fire.)
		t.Fatalf("LLM call count = %d, want >= 4", len(calls))
	}
	// The 1장 call's input must include both 씬 1 and 씬 2 labels.
	var chapInput string
	for _, c := range calls {
		if strings.Contains(c.input, "씬 1") && strings.Contains(c.input, "씬 2") {
			chapInput = c.input
			break
		}
	}
	if chapInput == "" {
		t.Errorf("no 1장 rollup call found; calls = %+v", calls)
	}
}
```

Extend the existing `fakeClient` struct to support `responseFn func([]llm.ChatMessage) string` — when set, `Chat` returns that; otherwise falls back to the existing `response` string field. This change is local to `summarizer_test.go`. Add `responseFn` to the struct definition.

In `Chat`:

```go
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
	fn := f.responseFn
	resp := f.response
	f.mu.Unlock()
	if fn != nil {
		return llm.ChatResponse{Message: llm.ChatMessage{Content: fn(messages)}}, nil
	}
	return llm.ChatResponse{Message: llm.ChatMessage{Content: resp}}, nil
}
```

- [ ] **Step 2: Extend `summarizer.go`**

Add new constants and a depth-guarded container path. At the top:

```go
const containerSummaryMaxRunes = 4000
const maxSummarizeDepth = 6
const containerSystemPrompt = "다음은 한 컨테이너에 속한 자식 노드들의 요약입니다. 이 컨테이너의 전체 흐름을 3~5문장의 한국어로 다시 요약하라. 등장인물·장소·핵심 사건은 보존하라. 새 정보 추가 금지."
```

Replace `summarizeOne` to accept an explicit depth and dispatch on kind. Keep the existing public `Enqueue` path unchanged — it always calls `summarizeOne(ctx, id, 0)`.

```go
func (s *Summarizer) summarizeOne(ctx context.Context, nodeID string) {
	s.summarizeOneDepth(ctx, nodeID, 0)
}

func (s *Summarizer) summarizeOneDepth(ctx context.Context, nodeID string, depth int) {
	if depth > maxSummarizeDepth {
		fmt.Fprintf(os.Stderr, "summarizer: depth cap hit at %s (depth=%d)\n", nodeID, depth)
		return
	}
	n, err := s.nodes.Get(ctx, nodeID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "summarizer: get %s: %v\n", nodeID, err)
		return
	}
	if n.Summary != "" && n.SummaryForVersion == n.ContentVersion {
		return
	}
	switch n.Kind {
	case "leaf":
		s.summarizeLeaf(ctx, n)
	case "container":
		s.summarizeContainer(ctx, n, depth)
	}
}
```

Extract the existing leaf body into `summarizeLeaf`:

```go
func (s *Summarizer) summarizeLeaf(ctx context.Context, n node.Node) {
	if n.ContentDoc == nil {
		return
	}
	capturedVersion := n.ContentVersion
	plain := strings.TrimSpace(docToPlainText(*n.ContentDoc))
	if plain == "" {
		return
	}
	if runeLen(plain) < minRunesForLLM {
		if err := s.nodes.SetSummary(ctx, n.ID, plain, capturedVersion); err != nil {
			fmt.Fprintf(os.Stderr, "summarizer: SetSummary (short) %s: %v\n", n.ID, err)
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
		fmt.Fprintf(os.Stderr, "summarizer: Chat %s: %v\n", n.ID, err)
		return
	}
	summary := strings.TrimSpace(resp.Message.Content)
	if summary == "" {
		return
	}
	if err := s.nodes.SetSummary(ctx, n.ID, summary, capturedVersion); err != nil {
		fmt.Fprintf(os.Stderr, "summarizer: SetSummary %s: %v\n", n.ID, err)
	}
}
```

Add the new container path:

```go
func (s *Summarizer) summarizeContainer(ctx context.Context, n node.Node, depth int) {
	capturedVersion := n.ContentVersion
	children, err := s.nodes.ListChildren(ctx, n.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "summarizer: ListChildren %s: %v\n", n.ID, err)
		return
	}
	if len(children) == 0 {
		return
	}
	// Recurse into stale children first, depth-first.
	for _, c := range children {
		if c.Summary == "" || c.SummaryForVersion != c.ContentVersion {
			s.summarizeOneDepth(ctx, c.ID, depth+1)
		}
	}
	// Re-read children after the recursion so we see the fresh summaries.
	children, err = s.nodes.ListChildren(ctx, n.ID)
	if err != nil {
		return
	}
	var b strings.Builder
	for _, c := range children {
		if c.Summary == "" {
			continue
		}
		b.WriteString(c.Label)
		b.WriteString("\n")
		b.WriteString(c.Summary)
		b.WriteString("\n\n")
	}
	input := strings.TrimSpace(b.String())
	if input == "" {
		return
	}
	// Truncate from the end if the concatenation exceeds the budget.
	if r := []rune(input); len(r) > containerSummaryMaxRunes {
		input = string(r[:containerSummaryMaxRunes])
	}

	provider := s.src.Provider()
	client, err := s.factory(provider, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "summarizer: factory(%s): %v\n", provider, err)
		return
	}
	msgs := []llm.ChatMessage{
		{Role: "system", Content: containerSystemPrompt},
		{Role: "user", Content: input},
	}
	resp, err := client.Chat(ctx, msgs, llm.ChatOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "summarizer: Chat (container) %s: %v\n", n.ID, err)
		return
	}
	summary := strings.TrimSpace(resp.Message.Content)
	if summary == "" {
		return
	}
	if err := s.nodes.SetSummary(ctx, n.ID, summary, capturedVersion); err != nil {
		fmt.Fprintf(os.Stderr, "summarizer: SetSummary (container) %s: %v\n", n.ID, err)
	}
}
```

Also remove the now-redundant `if n.Kind != "leaf" || n.ContentDoc == nil { return }` short-circuit and the manual freshness check from the old monolithic `summarizeOne` (the dispatcher handles both).

- [ ] **Step 3: Run tests**

```bash
cd engine && go test ./internal/summarizer/... -race
```

- [ ] **Step 4: Commit**

```bash
git add engine/internal/summarizer/ engine/internal/node/
git commit -m "feat(summarizer): roll up container summaries from children with depth guard"
```

---

## Phase C: Hierarchical context retrieval (1 task)

### Task 3 — `ContextBuilder.loadHierarchicalContext` + new `Context.Hierarchical` field

**Files:**
- Modify: `engine/internal/ai/ai.go`
- Modify: `engine/internal/ai/context.go`
- Modify: `engine/internal/ai/context_test.go`

- [ ] **Step 1: Add types to `ai.go`**

Append:

```go
// SceneSummary is one rendered leaf/scene rollup (label + body). Used by both
// hierarchical layer 1 and topology RAG layer 3.
type SceneSummary struct {
	NodeID string `json:"node_id"`
	Label  string `json:"label"` // breadcrumb path, e.g. "1부 / 1장 / 씬 3"
	Body   string `json:"body"`
}

// ChapterSummary is the 장-level rollup body + its breadcrumb label.
type ChapterSummary struct {
	NodeID string `json:"node_id"`
	Label  string `json:"label"`
	Body   string `json:"body"`
}

// PartSummary is the 부-level rollup.
type PartSummary struct {
	NodeID string `json:"node_id"`
	Label  string `json:"label"`
	Body   string `json:"body"`
}

// HierarchicalContext is layer 1 of Plan 16's hierarchical context. Each slice
// may be empty; renderer skips empty sections.
type HierarchicalContext struct {
	NearbyLeafSummaries   []SceneSummary   `json:"nearby_leaf_summaries"`
	SameChapterSummaries  []SceneSummary   `json:"same_chapter_summaries"`
	OtherChapterSummaries []ChapterSummary `json:"other_chapter_summaries"`
	OtherPartSummaries    []PartSummary    `json:"other_part_summaries"`
	ProjectSynopsis       string           `json:"project_synopsis"`
}
```

Extend `Context`:

```go
type Context struct {
	ProjectID     string              `json:"project_id"`
	NodeID        string              `json:"node_id"`
	SceneLabel    string              `json:"scene_label"`
	SceneText     string              `json:"scene_text"`
	PrevSummary   string              `json:"prev_summary"`
	Hierarchical  HierarchicalContext `json:"hierarchical"`
	RelatedScenes []SceneSummary      `json:"related_scenes"`
	Entities      []EntityBrief       `json:"entities"`
	ActiveThreads []ActiveThread      `json:"active_threads"`
	Notes         []NoteBrief         `json:"notes"`
	StyleNotes    string              `json:"style_notes"`
	UserPrompt    string              `json:"user_prompt"`
	Options       Options             `json:"options"`
}
```

Extend `EntityBrief`:

```go
type EntityBrief struct {
	Name       string            `json:"name"`
	Kind       string            `json:"kind"`
	Role       string            `json:"role"`
	Summary    string            `json:"summary"`
	Attributes map[string]string `json:"attributes"`
	Recent     []string          `json:"recent"` // Plan 16 layer 2 dossier
}
```

- [ ] **Step 2: Add the builder dependency on the summarizer**

`ContextBuilder` needs to call `summarizer.summarizeOne` on cache miss. Avoid a cyclic import: `summarizer` already imports `ai` (for `ProviderSource` / `ClientFactory`). Instead of importing the summarizer package into `ai`, define a small interface in `ai` and inject. Add to `context.go`:

```go
// SummaryRefresher is what ContextBuilder calls when an ancestor container has
// a stale summary. summarizer.Summarizer satisfies this via a thin adapter in
// cmd/linetta-engine wiring.
type SummaryRefresher interface {
	RefreshNow(ctx context.Context, nodeID string)
}

type noopRefresher struct{}

func (noopRefresher) RefreshNow(context.Context, string) {}
```

Extend `NewContextBuilder`:

```go
type ContextBuilder struct {
	projects  *project.Repo
	nodes     *node.Repo
	mentions  *mention.Repo
	threads   *thread.Repo
	beats     *beat.Repo
	notes     *note.Repo
	refresher SummaryRefresher
}

func NewContextBuilder(projects *project.Repo, nodes *node.Repo, mentions *mention.Repo, threads *thread.Repo, beats *beat.Repo, notes *note.Repo) *ContextBuilder {
	return &ContextBuilder{
		projects: projects, nodes: nodes, mentions: mentions,
		threads: threads, beats: beats, notes: notes,
		refresher: noopRefresher{},
	}
}

// WithSummaryRefresher returns a copy of b that synchronously refreshes stale
// container summaries on hierarchical context loads.
func (b *ContextBuilder) WithSummaryRefresher(r SummaryRefresher) *ContextBuilder {
	cp := *b
	cp.refresher = r
	return &cp
}
```

Then in `cmd/linetta-engine/main.go`, after `summ := summarizer.New(...)` add a tiny adapter — but this hookup detail belongs to Task 6's wiring step. For now, the default `noopRefresher` means existing call sites compile.

Expose a public `RefreshNow` method on `Summarizer`:

```go
// In engine/internal/summarizer/summarizer.go:
func (s *Summarizer) RefreshNow(ctx context.Context, nodeID string) {
	s.summarizeOneDepth(ctx, nodeID, 0)
}
```

- [ ] **Step 3: Implement `loadHierarchicalContext` in `context.go`**

Add the budget constant and helper:

```go
const hierarchicalMaxChars = 2500

// loadHierarchicalContext gathers layer-1 (container rollup) data for cur.
// On stale container hits it calls b.refresher.RefreshNow synchronously so the
// returned summaries are fresh.
func (b *ContextBuilder) loadHierarchicalContext(ctx context.Context, cur node.Node) (HierarchicalContext, []string, error) {
	all, err := b.nodes.ListByProject(ctx, cur.ProjectID)
	if err != nil {
		return HierarchicalContext{}, nil, err
	}
	byID := make(map[string]node.Node, len(all))
	children := map[string][]node.Node{}
	for _, n := range all {
		byID[n.ID] = n
		key := ""
		if n.ParentID != nil {
			key = *n.ParentID
		}
		children[key] = append(children[key], n)
	}
	// DFS leaves in document order.
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

	curIdx := -1
	for i, l := range leaves {
		if l.ID == cur.ID {
			curIdx = i
			break
		}
	}

	out := HierarchicalContext{}
	nearbyIDs := make([]string, 0, 3)

	// Nearby: 2 prior + 1 next (in DFS-leaf order), all excluding cur.
	if curIdx >= 0 {
		for _, j := range []int{curIdx - 2, curIdx - 1, curIdx + 1} {
			if j < 0 || j >= len(leaves) || j == curIdx {
				continue
			}
			lf := leaves[j]
			label := breadcrumbLabel(byID, lf)
			body := freshLeafSummary(lf)
			if body == "" {
				continue
			}
			out.NearbyLeafSummaries = append(out.NearbyLeafSummaries, SceneSummary{
				NodeID: lf.ID, Label: label, Body: body,
			})
			nearbyIDs = append(nearbyIDs, lf.ID)
		}
	}

	// Same chapter: every other leaf whose parent_id == cur.parent_id, minus
	// the nearby IDs and cur itself.
	if cur.ParentID != nil {
		exclude := map[string]bool{cur.ID: true}
		for _, id := range nearbyIDs {
			exclude[id] = true
		}
		for _, sib := range children[*cur.ParentID] {
			if sib.Kind != "leaf" || exclude[sib.ID] {
				continue
			}
			body := freshLeafSummary(sib)
			if body == "" {
				continue
			}
			out.SameChapterSummaries = append(out.SameChapterSummaries, SceneSummary{
				NodeID: sib.ID, Label: breadcrumbLabel(byID, sib), Body: body,
			})
		}
	}

	// Other chapters within same 부: siblings of cur's parent (containers).
	if cur.ParentID != nil {
		curChap := byID[*cur.ParentID]
		if curChap.ParentID != nil {
			for _, sibChap := range children[*curChap.ParentID] {
				if sibChap.Kind != "container" || sibChap.ID == curChap.ID {
					continue
				}
				body := b.refreshAndRead(ctx, sibChap)
				if body == "" {
					continue
				}
				out.OtherChapterSummaries = append(out.OtherChapterSummaries, ChapterSummary{
					NodeID: sibChap.ID, Label: breadcrumbLabel(byID, sibChap), Body: body,
				})
			}
			// Other parts: siblings of curChap.parent_id at the project root.
			curPart := byID[*curChap.ParentID]
			for _, sibPart := range children[parentKeyOf(curPart)] {
				if sibPart.Kind != "container" || sibPart.ID == curPart.ID {
					continue
				}
				body := b.refreshAndRead(ctx, sibPart)
				if body == "" {
					continue
				}
				out.OtherPartSummaries = append(out.OtherPartSummaries, PartSummary{
					NodeID: sibPart.ID, Label: breadcrumbLabel(byID, sibPart), Body: body,
				})
			}
		}
	}

	// Project synopsis: the root if there's a single root container, or the
	// concatenation summary at virtual root. For flat projects (no containers
	// at all) this is empty.
	rootContainers := []node.Node{}
	for _, n := range children[""] {
		if n.Kind == "container" {
			rootContainers = append(rootContainers, n)
		}
	}
	if len(rootContainers) == 1 {
		out.ProjectSynopsis = b.refreshAndRead(ctx, rootContainers[0])
	} else if len(rootContainers) > 1 {
		// No virtual root in the schema — concatenate part summaries directly.
		var b2 strings.Builder
		for _, rc := range rootContainers {
			body := b.refreshAndRead(ctx, rc)
			if body == "" {
				continue
			}
			b2.WriteString(rc.Label)
			b2.WriteString(": ")
			b2.WriteString(body)
			b2.WriteString("\n")
		}
		out.ProjectSynopsis = strings.TrimSpace(b2.String())
	}

	// Apply the total-budget cap. We trim trailing items (cheapest signal
	// first to drop) until the rendered estimate is under hierarchicalMaxChars.
	trimToBudget(&out, hierarchicalMaxChars)

	return out, nearbyIDs, nil
}

func freshLeafSummary(n node.Node) string {
	if n.Summary != "" && n.SummaryForVersion == n.ContentVersion {
		return n.Summary
	}
	if n.ContentDoc != nil {
		return trimRunes(docToPlainText(n.ContentDoc), prevSummaryMaxRunes)
	}
	return ""
}

func (b *ContextBuilder) refreshAndRead(ctx context.Context, n node.Node) string {
	if n.Summary != "" && n.SummaryForVersion == n.ContentVersion {
		return n.Summary
	}
	b.refresher.RefreshNow(ctx, n.ID)
	got, err := b.nodes.Get(ctx, n.ID)
	if err != nil {
		return ""
	}
	if got.Summary != "" && got.SummaryForVersion == got.ContentVersion {
		return got.Summary
	}
	// Last-resort fallback: empty (better than a stale string).
	return ""
}

func parentKeyOf(n node.Node) string {
	if n.ParentID == nil {
		return ""
	}
	return *n.ParentID
}

func breadcrumbLabel(byID map[string]node.Node, n node.Node) string {
	parts := []string{n.Label}
	cur := n
	for cur.ParentID != nil {
		p, ok := byID[*cur.ParentID]
		if !ok {
			break
		}
		parts = append([]string{p.Label}, parts...)
		cur = p
	}
	return strings.Join(parts, " / ")
}

// trimToBudget drops trailing entries from the lowest-priority sections first
// (other_part → other_chapter → same_chapter → nearby) until the estimated
// rendered size is under maxChars.
func trimToBudget(h *HierarchicalContext, maxChars int) {
	estimate := func() int {
		total := len(h.ProjectSynopsis)
		for _, s := range h.NearbyLeafSummaries {
			total += len(s.Label) + len(s.Body) + 4
		}
		for _, s := range h.SameChapterSummaries {
			total += len(s.Label) + len(s.Body) + 4
		}
		for _, s := range h.OtherChapterSummaries {
			total += len(s.Label) + len(s.Body) + 4
		}
		for _, s := range h.OtherPartSummaries {
			total += len(s.Label) + len(s.Body) + 4
		}
		return total
	}
	for estimate() > maxChars && len(h.OtherPartSummaries) > 0 {
		h.OtherPartSummaries = h.OtherPartSummaries[:len(h.OtherPartSummaries)-1]
	}
	for estimate() > maxChars && len(h.OtherChapterSummaries) > 0 {
		h.OtherChapterSummaries = h.OtherChapterSummaries[:len(h.OtherChapterSummaries)-1]
	}
	for estimate() > maxChars && len(h.SameChapterSummaries) > 0 {
		h.SameChapterSummaries = h.SameChapterSummaries[:len(h.SameChapterSummaries)-1]
	}
	for estimate() > maxChars && len(h.NearbyLeafSummaries) > 0 {
		h.NearbyLeafSummaries = h.NearbyLeafSummaries[:len(h.NearbyLeafSummaries)-1]
	}
}
```

- [ ] **Step 4: Wire into `Build`**

In `ContextBuilder.Build`, after computing `prevSummary`, add:

```go
	hierarchical, nearbyIDs, err := b.loadHierarchicalContext(ctx, n)
	if err != nil {
		return Context{}, err
	}
```

Then return with `Hierarchical: hierarchical`. The `nearbyIDs` slice flows into Task 5's `loadRelatedScenes` for set-based exclusion.

- [ ] **Step 5: Failing test (append to `context_test.go`)**

```go
func TestBuildContext_hierarchical_populatesNearbySameChapterAndPart(t *testing.T) {
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
	mr := mention.NewRepo(s)
	nodes := node.NewRepo(s)
	nodes.SetMentionResyncer(func(ctx context.Context, nodeID, doc string) error {
		return mr.ResyncForNode(ctx, nodeID, mention.Collect([]byte(doc)))
	})

	// Build 1부 → 1장 → {씬 1, 씬 2, 씬 3, 씬 4}, plus 2부 → 2장 → 씬 5.
	part1, _ := nodes.CreateSibling(context.Background(), *p.LastOpenedNodeID, "container", "1부", "", 1100)
	chap1, _ := nodes.CreateChild(context.Background(), part1.ID, "container", "1장", "", 1110)
	s1, _ := nodes.CreateChild(context.Background(), chap1.ID, "leaf", "씬 1", "", 1120)
	s2, _ := nodes.CreateChild(context.Background(), chap1.ID, "leaf", "씬 2", "", 1130)
	s3, _ := nodes.CreateChild(context.Background(), chap1.ID, "leaf", "씬 3", "", 1140)
	s4, _ := nodes.CreateChild(context.Background(), chap1.ID, "leaf", "씬 4", "", 1150)
	part2, _ := nodes.CreateSibling(context.Background(), part1.ID, "container", "2부", "", 1160)
	chap2, _ := nodes.CreateChild(context.Background(), part2.ID, "container", "2장", "", 1170)
	_, _ = nodes.CreateChild(context.Background(), chap2.ID, "leaf", "씬 5", "", 1180)

	docOf := func(text string) string {
		return `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + text + `"}]}]}`
	}
	_ = nodes.UpdateContent(context.Background(), s1.ID, docOf("씬1 본문"), 1200)
	_ = nodes.UpdateContent(context.Background(), s2.ID, docOf("씬2 본문"), 1210)
	_ = nodes.UpdateContent(context.Background(), s3.ID, docOf("씬3 본문 — 현재 씬"), 1220)
	_ = nodes.UpdateContent(context.Background(), s4.ID, docOf("씬4 본문"), 1230)

	// Seed fresh summaries on every leaf and every container so the builder
	// has cached data without needing a real summarizer.
	seedFresh := func(id, body string) {
		got, _ := nodes.Get(context.Background(), id)
		_ = nodes.SetSummary(context.Background(), id, body, got.ContentVersion)
	}
	seedFresh(s1.ID, "씬1 요약")
	seedFresh(s2.ID, "씬2 요약")
	seedFresh(s4.ID, "씬4 요약")
	seedFresh(chap1.ID, "1장 요약")
	seedFresh(part1.ID, "1부 요약")
	seedFresh(chap2.ID, "2장 요약")
	seedFresh(part2.ID, "2부 요약")

	builder := NewContextBuilder(pr, nodes, mr, thread.NewRepo(s), beat.NewRepo(s), note.NewRepo(s))
	got, err := builder.Build(context.Background(), s3.ID, "확장", Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Nearby: should contain 씬 1, 씬 2 (the 2 priors) and 씬 4 (next).
	gotNearby := map[string]bool{}
	for _, ss := range got.Hierarchical.NearbyLeafSummaries {
		gotNearby[ss.NodeID] = true
	}
	for _, want := range []string{s1.ID, s2.ID, s4.ID} {
		if !gotNearby[want] {
			t.Errorf("nearby missing %s; got %+v", want, got.Hierarchical.NearbyLeafSummaries)
		}
	}
	// Same chapter must NOT include any nearby id nor cur.
	for _, ss := range got.Hierarchical.SameChapterSummaries {
		if gotNearby[ss.NodeID] || ss.NodeID == s3.ID {
			t.Errorf("same_chapter leaked nearby/self: %s", ss.NodeID)
		}
	}
	// Other parts: should contain 2부.
	foundPart := false
	for _, ps := range got.Hierarchical.OtherPartSummaries {
		if ps.NodeID == part2.ID && ps.Body == "2부 요약" {
			foundPart = true
		}
	}
	if !foundPart {
		t.Errorf("other_part_summaries missing 2부: %+v", got.Hierarchical.OtherPartSummaries)
	}
	// Project synopsis is empty (two root containers, no single root → falls
	// into the multi-root concatenation branch).
	if !strings.Contains(got.Hierarchical.ProjectSynopsis, "1부") {
		t.Errorf("project_synopsis = %q, want to mention 1부", got.Hierarchical.ProjectSynopsis)
	}
}
```

- [ ] **Step 6: Run + commit**

```bash
cd engine && go test ./internal/ai/... -race
git add engine/internal/ai/ engine/internal/summarizer/summarizer.go
git commit -m "feat(ai): layer-1 hierarchical context (nearby + same-chapter + part rollups)"
```

---

## Phase D: Entity dossier + topology RAG (2 tasks)

### Task 4 — Entity dossier (`EntityBrief.Recent`)

**Files:**
- Modify: `engine/internal/ai/context.go`
- Modify: `engine/internal/ai/context_test.go`

The exact SQL pulls the latest 5 leaves where the entity is mentioned (excluding the current node), with a non-empty `summary`:

```sql
SELECT n.summary
  FROM mentions m
  JOIN nodes n ON n.id = m.node_id
 WHERE m.entity_id = ?
   AND m.node_id != ?
   AND n.summary != ''
 ORDER BY n.updated_at DESC
 LIMIT 5;
```

- [ ] **Step 1: Add `dossierFor` helper in `context.go`**

```go
// dossierFor returns the first-line of the summary on up to 5 most-recent
// other leaves where entityID is mentioned. Empty slice if no hits.
func (b *ContextBuilder) dossierFor(ctx context.Context, entityID, currentNodeID string) ([]string, error) {
	rows, err := b.nodes.DB().QueryContext(ctx, `
SELECT n.summary
  FROM mentions m
  JOIN nodes n ON n.id = m.node_id
 WHERE m.entity_id = ?
   AND m.node_id != ?
   AND n.summary != ''
 ORDER BY n.updated_at DESC
 LIMIT 5`, entityID, currentNodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		first := strings.SplitN(s, "\n", 2)[0]
		first = strings.TrimSpace(first)
		if first != "" {
			out = append(out, first)
		}
	}
	return out, rows.Err()
}
```

This requires exposing the underlying DB via `node.Repo`. The cleanest path: add a `DB()` accessor on `node.Repo` if not already exposed (mirror `store.Store.DB()`). Check first:

```bash
grep -n "func (r \*Repo) DB\|func (r \*Repo) Store" engine/internal/node/repo.go
```

If absent, add to `repo.go`:

```go
// DB returns the underlying *sql.DB. Used by ContextBuilder for cross-table
// queries that don't fit cleanly into the node repo's surface.
func (r *Repo) DB() *sql.DB { return r.s.DB() }
```

Alternatively, place `dossierFor` in `mention.Repo` and call it from `ContextBuilder`. Preferred — keeps `node.Repo` clean:

```go
// In engine/internal/mention/repo.go:
func (r *Repo) RecentSummariesForEntity(ctx context.Context, entityID, excludeNodeID string, limit int) ([]string, error) {
	rows, err := r.s.DB().QueryContext(ctx, `
SELECT n.summary
  FROM mentions m
  JOIN nodes n ON n.id = m.node_id
 WHERE m.entity_id = ?
   AND m.node_id != ?
   AND n.summary != ''
 ORDER BY n.updated_at DESC
 LIMIT ?`, entityID, excludeNodeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		first := strings.TrimSpace(strings.SplitN(s, "\n", 2)[0])
		if first != "" {
			out = append(out, first)
		}
	}
	return out, rows.Err()
}
```

Then in `context.go`:

```go
	briefs := make([]EntityBrief, 0, len(ents))
	for _, e := range ents {
		recent, err := b.mentions.RecentSummariesForEntity(ctx, e.ID, nodeID, 5)
		if err != nil {
			return Context{}, err
		}
		briefs = append(briefs, EntityBrief{
			Name: e.Name, Kind: e.Kind, Role: e.Role, Summary: e.Summary,
			Attributes: e.Attributes, Recent: recent,
		})
	}
```

- [ ] **Step 2: Failing test**

```go
func TestBuildContext_entityDossier_populatesRecentFromOtherLeaves(t *testing.T) {
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

	e, _ := er.Create(context.Background(), 1050, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "해진"})

	first := *p.LastOpenedNodeID
	doc := func(text string) string {
		return `{"type":"doc","content":[{"type":"paragraph","content":[
			{"type":"text","text":"` + text + ` "},
			{"type":"mention","attrs":{"id":"` + e.ID + `","label":"해진"}}
		]}]}`
	}
	_ = nodes.UpdateContent(context.Background(), first, doc("씬 1에서"), 1100)
	gotFirst, _ := nodes.Get(context.Background(), first)
	_ = nodes.SetSummary(context.Background(), first, "해진은 모래에 처음 도착했다.\n계속 ...", gotFirst.ContentVersion)

	second, _ := nodes.CreateSibling(context.Background(), first, "leaf", "씬 2", "", 1200)
	_ = nodes.UpdateContent(context.Background(), second.ID, doc("씬 2의 현재"), 1300)

	builder := NewContextBuilder(pr, nodes, mr, thread.NewRepo(s), beat.NewRepo(s), note.NewRepo(s))
	got, err := builder.Build(context.Background(), second.ID, "확장", Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(got.Entities) != 1 {
		t.Fatalf("entities = %d", len(got.Entities))
	}
	if len(got.Entities[0].Recent) != 1 || got.Entities[0].Recent[0] != "해진은 모래에 처음 도착했다." {
		t.Errorf("dossier = %+v, want one line", got.Entities[0].Recent)
	}
}
```

- [ ] **Step 3: Commit**

```bash
cd engine && go test ./internal/ai/... ./internal/mention/... -race
git add engine/internal/ai/ engine/internal/mention/
git commit -m "feat(ai): layer-2 entity dossier from recent mentions"
```

---

### Task 5 — Topology RAG (`Context.RelatedScenes`)

**Files:**
- Modify: `engine/internal/mention/repo.go`
- Modify: `engine/internal/ai/context.go`
- Modify: `engine/internal/ai/context_test.go`

The co-mention SQL:

```sql
WITH cur_ents AS (
  SELECT entity_id FROM mentions WHERE node_id = ?
)
SELECT m.node_id, COUNT(DISTINCT m.entity_id) AS k, MAX(n.updated_at) AS last_seen
  FROM mentions m
  JOIN nodes n ON n.id = m.node_id
 WHERE m.entity_id IN (SELECT entity_id FROM cur_ents)
   AND m.node_id != ?
   AND n.summary != ''
 GROUP BY m.node_id
HAVING k >= 2
 ORDER BY k DESC, last_seen DESC
 LIMIT 3;
```

- [ ] **Step 1: Add `CoMentionLeaves` to `mention/repo.go`**

```go
// CoMentionResult is one row of the topology-RAG result set.
type CoMentionResult struct {
	NodeID   string
	K        int   // distinct entities from the current node that appear here
	LastSeen int64 // nodes.updated_at
}

// CoMentionLeaves returns up to `limit` leaves that share at least 2 entities
// with currentNodeID, ranked by (shared-count desc, updated_at desc). Excludes
// currentNodeID itself. Caller is expected to filter further if it wants to
// exclude additional ids (e.g. those already in the nearby set).
func (r *Repo) CoMentionLeaves(ctx context.Context, currentNodeID string, limit int) ([]CoMentionResult, error) {
	rows, err := r.s.DB().QueryContext(ctx, `
WITH cur_ents AS (
  SELECT entity_id FROM mentions WHERE node_id = ?
)
SELECT m.node_id, COUNT(DISTINCT m.entity_id) AS k, MAX(n.updated_at) AS last_seen
  FROM mentions m
  JOIN nodes n ON n.id = m.node_id
 WHERE m.entity_id IN (SELECT entity_id FROM cur_ents)
   AND m.node_id != ?
   AND n.summary != ''
 GROUP BY m.node_id
HAVING k >= 2
 ORDER BY k DESC, last_seen DESC
 LIMIT ?`, currentNodeID, currentNodeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CoMentionResult
	for rows.Next() {
		var rec CoMentionResult
		if err := rows.Scan(&rec.NodeID, &rec.K, &rec.LastSeen); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}
```

- [ ] **Step 2: `loadRelatedScenes` in `context.go`**

```go
func (b *ContextBuilder) loadRelatedScenes(ctx context.Context, cur node.Node, excludeIDs []string) ([]SceneSummary, error) {
	// Fetch top-K + small buffer so the post-filter still gives 3 if possible.
	results, err := b.mentions.CoMentionLeaves(ctx, cur.ID, 3+len(excludeIDs))
	if err != nil {
		return nil, err
	}
	excl := make(map[string]bool, len(excludeIDs)+1)
	excl[cur.ID] = true
	for _, id := range excludeIDs {
		excl[id] = true
	}
	// Need the byID map for breadcrumb labels — reuse ListByProject.
	all, err := b.nodes.ListByProject(ctx, cur.ProjectID)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]node.Node, len(all))
	for _, n := range all {
		byID[n.ID] = n
	}
	out := make([]SceneSummary, 0, 3)
	for _, r := range results {
		if excl[r.NodeID] {
			continue
		}
		n, ok := byID[r.NodeID]
		if !ok {
			continue
		}
		if n.Summary == "" {
			continue
		}
		out = append(out, SceneSummary{
			NodeID: n.ID, Label: breadcrumbLabel(byID, n), Body: n.Summary,
		})
		if len(out) >= 3 {
			break
		}
	}
	return out, nil
}
```

In `Build`, after `loadHierarchicalContext` returns `nearbyIDs`:

```go
	related, err := b.loadRelatedScenes(ctx, n, nearbyIDs)
	if err != nil {
		return Context{}, err
	}
```

Return `RelatedScenes: related`.

- [ ] **Step 3: Failing test**

```go
func TestBuildContext_relatedScenes_returnsCoMentionTop3(t *testing.T) {
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

	e1, _ := er.Create(context.Background(), 1050, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "해진"})
	e2, _ := er.Create(context.Background(), 1060, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "민호"})

	withBoth := func(text string) string {
		return `{"type":"doc","content":[{"type":"paragraph","content":[
			{"type":"text","text":"` + text + `"},
			{"type":"mention","attrs":{"id":"` + e1.ID + `","label":"해진"}},
			{"type":"mention","attrs":{"id":"` + e2.ID + `","label":"민호"}}
		]}]}`
	}
	withOne := func(text, eid, lbl string) string {
		return `{"type":"doc","content":[{"type":"paragraph","content":[
			{"type":"text","text":"` + text + `"},
			{"type":"mention","attrs":{"id":"` + eid + `","label":"` + lbl + `"}}
		]}]}`
	}

	cur := *p.LastOpenedNodeID
	_ = nodes.UpdateContent(context.Background(), cur, withBoth("현재 — "), 1100)

	// Co-mention leaf (both entities together).
	co, _ := nodes.CreateSibling(context.Background(), cur, "leaf", "씬 co", "", 1200)
	_ = nodes.UpdateContent(context.Background(), co.ID, withBoth("co — "), 1210)
	gotCo, _ := nodes.Get(context.Background(), co.ID)
	_ = nodes.SetSummary(context.Background(), co.ID, "co 요약", gotCo.ContentVersion)

	// Single-entity leaf (should NOT appear — only 1 shared entity).
	solo, _ := nodes.CreateSibling(context.Background(), co.ID, "leaf", "씬 solo", "", 1300)
	_ = nodes.UpdateContent(context.Background(), solo.ID, withOne("solo — ", e1.ID, "해진"), 1310)
	gotSolo, _ := nodes.Get(context.Background(), solo.ID)
	_ = nodes.SetSummary(context.Background(), solo.ID, "solo 요약", gotSolo.ContentVersion)

	builder := NewContextBuilder(pr, nodes, mr, thread.NewRepo(s), beat.NewRepo(s), note.NewRepo(s))
	got, err := builder.Build(context.Background(), cur, "확장", Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(got.RelatedScenes) != 1 || got.RelatedScenes[0].NodeID != co.ID {
		t.Errorf("related = %+v, want [co]", got.RelatedScenes)
	}
}
```

- [ ] **Step 4: Commit**

```bash
cd engine && go test ./internal/ai/... ./internal/mention/... -race
git add engine/internal/ai/ engine/internal/mention/
git commit -m "feat(ai): layer-3 topology RAG via co-mention co-occurrence"
```

---

## Phase E: Prompt integration (2 tasks)

### Task 6 — `prompts.go::buildUser` reorder + 5 new sections + dossier under entities + wire `SummaryRefresher`

**Files:**
- Modify: `engine/internal/ai/prompts.go`
- Modify: `engine/cmd/linetta-engine/main.go`

**Final section order in `buildUser`:**

1. `## 작품 전반` (project synopsis — if non-empty)
2. `## 인근 줄거리` (other-chapter `Body` lines, then other-part `Body` lines — if non-empty)
3. `## 같은 장 다른 씬` (same-chapter leaf summaries — if non-empty)
4. `## 직전·직후 씬 발췌` (nearby leaf summaries — replaces the old single `## 직전 씬 발췌`)
5. `## 관련 과거 씬` (topology RAG — if non-empty)
6. `## 현재 씬: {label}` + body
7. `## 등장 인물·장소` (with `Recent` indented as `· {line}`)
8. `## 활성 스토리라인`
9. `## 작가 주석`
10. `## 작가 메모` (when `tone != "my"` and `style_notes` is non-empty)
11. `## 작가의 지시`

The legacy `## 직전 씬 발췌` block (which uses `c.PrevSummary`) is removed: layer-1's `## 직전·직후 씬 발췌` supersedes it. We still keep `Context.PrevSummary` populated as the verbatim fallback for `Hierarchical.NearbyLeafSummaries` when curIdx-1 has no fresh summary — but the prompt renderer no longer emits a dedicated section for it. (If you want to preserve backward compatibility for ai_runs replay, leave `PrevSummary` in the JSON but skip it in `buildUser`.)

- [ ] **Step 1: Rewrite `buildUser`**

```go
func buildUser(c Context) string {
	var b strings.Builder

	if strings.TrimSpace(c.Hierarchical.ProjectSynopsis) != "" {
		b.WriteString("## 작품 전반\n")
		b.WriteString(c.Hierarchical.ProjectSynopsis)
		b.WriteString("\n\n")
	}

	if len(c.Hierarchical.OtherChapterSummaries) > 0 || len(c.Hierarchical.OtherPartSummaries) > 0 {
		b.WriteString("## 인근 줄거리\n")
		for _, ch := range c.Hierarchical.OtherChapterSummaries {
			b.WriteString("- ")
			b.WriteString(ch.Label)
			b.WriteString(": ")
			b.WriteString(ch.Body)
			b.WriteString("\n")
		}
		for _, pt := range c.Hierarchical.OtherPartSummaries {
			b.WriteString("- ")
			b.WriteString(pt.Label)
			b.WriteString(": ")
			b.WriteString(pt.Body)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if len(c.Hierarchical.SameChapterSummaries) > 0 {
		b.WriteString("## 같은 장 다른 씬\n")
		for _, ss := range c.Hierarchical.SameChapterSummaries {
			b.WriteString("- ")
			b.WriteString(ss.Label)
			b.WriteString(": ")
			b.WriteString(ss.Body)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if len(c.Hierarchical.NearbyLeafSummaries) > 0 {
		b.WriteString("## 직전·직후 씬 발췌\n")
		for _, ss := range c.Hierarchical.NearbyLeafSummaries {
			b.WriteString("- ")
			b.WriteString(ss.Label)
			b.WriteString(": ")
			b.WriteString(ss.Body)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if len(c.RelatedScenes) > 0 {
		b.WriteString("## 관련 과거 씬\n")
		for _, ss := range c.RelatedScenes {
			b.WriteString("- ")
			b.WriteString(ss.Label)
			b.WriteString(": ")
			b.WriteString(ss.Body)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(fmt.Sprintf("## 현재 씬: %s\n", c.SceneLabel))
	b.WriteString(c.SceneText)
	b.WriteString("\n\n")

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
			for _, line := range e.Recent {
				b.WriteString("  · ")
				b.WriteString(line)
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}

	if len(c.ActiveThreads) > 0 {
		b.WriteString("## 활성 스토리라인\n")
		for _, t := range c.ActiveThreads {
			line := fmt.Sprintf("- [%s] %s", t.Color, t.Name)
			if t.Summary != "" {
				line += " — " + t.Summary
			}
			b.WriteString(line)
			b.WriteString("\n")
			for _, bt := range t.RecentBeats {
				b.WriteString(fmt.Sprintf("  · #%d %s\n", bt.Ordinal, bt.Label))
			}
		}
		b.WriteString("\n")
	}

	if len(c.Notes) > 0 {
		b.WriteString("## 작가 주석\n")
		for _, n := range c.Notes {
			b.WriteString("- ")
			b.WriteString(n.Body)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if c.Options.Tone != TonePresetMy && strings.TrimSpace(c.StyleNotes) != "" {
		b.WriteString("## 작가 메모\n")
		b.WriteString(c.StyleNotes)
		b.WriteString("\n\n")
	}

	b.WriteString("## 작가의 지시\n")
	b.WriteString(strings.TrimSpace(c.UserPrompt))
	return b.String()
}
```

- [ ] **Step 2: Wire `SummaryRefresher` in `cmd/linetta-engine/main.go`**

Find the line where `ContextBuilder` is constructed. After:

```go
summ := summarizer.New(nodes, settingsStore, ai.DefaultClientFactory)
stopSummarizer := summ.Start(ctx)
defer stopSummarizer()
```

Change the builder construction so that the runner uses the refresher-aware variant:

```go
ctxBuilder := ai.NewContextBuilder(projects, nodes, mentions, threads, beats, notes).WithSummaryRefresher(summ)
```

`summ` (`*summarizer.Summarizer`) satisfies `ai.SummaryRefresher` via the `RefreshNow` method added in Task 3 Step 2.

- [ ] **Step 3: Update any existing `prompts_test.go` golden strings**

```bash
grep -rn "## 직전 씬 발췌" engine/internal/ai/
```

If a golden snapshot test exists, update the expected text to match the new section order. If `prompts_test.go` doesn't exist yet, skip this step.

- [ ] **Step 4: Run + commit**

```bash
cd engine && go test ./... -race && go build ./cmd/linetta-engine
git add engine/internal/ai/prompts.go engine/cmd/linetta-engine/main.go engine/internal/ai/prompts_test.go
git commit -m "feat(ai): render hierarchical context, dossier, and related-scenes in prompts"
```

---

### Task 7 — Consolidated context_test scenarios (cleanup pass)

The three new tests added in Tasks 3 / 4 / 5 already give per-layer coverage. This task does a final scrub:

- [ ] **Step 1:** Verify all old `prev_summary`-only tests still pass. Specifically `TestBuildContext_prevSummary_*` from Plan 9 — these tests assert `got.PrevSummary` directly, not the rendered prompt. They should still pass because we kept `Context.PrevSummary` populated (only the renderer dropped the section).

- [ ] **Step 2:** Add one cross-layer integration test that asserts the rendered `buildUser` output contains the 5 new headers in the documented order when all layers have data. Append:

```go
func TestBuildUser_sectionOrder_withAllLayers(t *testing.T) {
	c := Context{
		SceneLabel: "씬 3",
		SceneText:  "본문",
		Hierarchical: HierarchicalContext{
			ProjectSynopsis:       "작품 시놉시스",
			NearbyLeafSummaries:   []SceneSummary{{Label: "1부 / 1장 / 씬 2", Body: "씬 2 요약"}},
			SameChapterSummaries:  []SceneSummary{{Label: "1부 / 1장 / 씬 4", Body: "씬 4 요약"}},
			OtherChapterSummaries: []ChapterSummary{{Label: "1부 / 2장", Body: "2장 요약"}},
			OtherPartSummaries:    []PartSummary{{Label: "2부", Body: "2부 요약"}},
		},
		RelatedScenes: []SceneSummary{{Label: "1부 / 1장 / 씬 7", Body: "관련 씬 요약"}},
		Entities: []EntityBrief{{
			Name: "해진", Kind: "character",
			Recent: []string{"해진 dossier line 1", "해진 dossier line 2"},
		}},
		UserPrompt: "확장",
	}
	got := buildUser(c)
	want := []string{
		"## 작품 전반",
		"## 인근 줄거리",
		"## 같은 장 다른 씬",
		"## 직전·직후 씬 발췌",
		"## 관련 과거 씬",
		"## 현재 씬: 씬 3",
		"## 등장 인물·장소",
		"  · 해진 dossier line 1",
		"## 작가의 지시",
	}
	last := -1
	for _, s := range want {
		idx := strings.Index(got, s)
		if idx < 0 {
			t.Errorf("missing %q in prompt", s)
			continue
		}
		if idx < last {
			t.Errorf("out-of-order: %q at %d came before previous (last=%d)", s, idx, last)
		}
		last = idx
	}
}
```

- [ ] **Step 3: Run + commit**

```bash
cd engine && go test ./internal/ai/... -race
git add engine/internal/ai/context_test.go engine/internal/ai/prompts_test.go
git commit -m "test(ai): assert hierarchical prompt section order end-to-end"
```

---

## Phase F: Smoke + tag (1 task)

### Task 8 — Manual walkthrough + tag

- [ ] **Step 1: Rebuild engine + launch**

```bash
./scripts/build-engine.sh
LINETTA_HOME=/tmp/linetta-plan16 ./scripts/dev.sh
```

- [ ] **Step 2: Import a multi-chapter project**

Use the Plan 14 markdown import (`Library → 가져오기`) to load a `.md` containing at least two H1 부 sections, each with at least two H2 장 sections, each with at least three scenes. Wait ~30 seconds — every autosave fires the background summarizer; we want all leaves and containers to have populated `summary`.

```bash
sqlite3 /tmp/linetta-plan16/library.db \
  "SELECT kind, label, length(summary), content_version, summary_for_version FROM nodes ORDER BY kind, label"
```

Expect: every leaf row has `length(summary) > 0` and `content_version == summary_for_version`. Every container row likewise (after the lazy build on first AI-mode invocation, see Step 3).

- [ ] **Step 3: Open AI mode in a scene in chapter 3 of part 1; run any generation**

Then inspect the recorded context:

```bash
sqlite3 /tmp/linetta-plan16/library.db \
  "SELECT json_extract(context_json,'\$.hierarchical') FROM ai_runs ORDER BY started_at DESC LIMIT 1" | jq
```

Expected shape:
- `nearby_leaf_summaries` length ≤ 3 (typically 3 — 2 priors + 1 next)
- `same_chapter_summaries` length ≥ 0 (other leaves of chapter 3 minus nearby)
- `other_chapter_summaries` length ≥ 2 (chapters 1, 2, plus 4+ if present)
- `other_part_summaries` length ≥ 1 (part 2, …)
- `project_synopsis` non-empty

- [ ] **Step 4: Inspect the actual prompt**

```bash
sqlite3 /tmp/linetta-plan16/library.db \
  "SELECT prompt FROM ai_runs ORDER BY started_at DESC LIMIT 1"
```

Verify the 6 new section headers appear in order: `## 작품 전반` → `## 인근 줄거리` → `## 같은 장 다른 씬` → `## 직전·직후 씬 발췌` → (optional) `## 관련 과거 씬` → `## 현재 씬`. Verify `## 등장 인물·장소` shows indented `  · {line}` dossier entries under any entity that has appeared elsewhere.

- [ ] **Step 5: Topology RAG smoke**

Pick two entities that previously co-appeared only in part 1 chapter 1. In the current scene (somewhere far away), insert both `@`-mentions. Trigger an AI run. Inspect:

```bash
sqlite3 /tmp/linetta-plan16/library.db \
  "SELECT json_extract(context_json,'\$.related_scenes') FROM ai_runs ORDER BY started_at DESC LIMIT 1" | jq
```

Expect: the previously-co-mentioned chapter-1 scene's `node_id` shows up here, with a non-empty `body`.

- [ ] **Step 6: Edit a scene + verify ancestor staleness propagates**

```bash
# Before the edit:
sqlite3 /tmp/linetta-plan16/library.db \
  "SELECT label, content_version, summary_for_version FROM nodes WHERE kind='container'"
```

Edit any scene in the desktop app, wait 2 seconds for autosave. Re-query — every ancestor container's `content_version` has bumped, and its `summary_for_version` is now behind. Trigger an AI run from a scene in a *different* part; the renderer's `refreshAndRead` kicks the summarizer synchronously and the audit log's `hierarchical.other_part_summaries` reflects the new content.

- [ ] **Step 7: `go test ./... -race` and `pnpm tsc -b && pnpm build` both green**

```bash
cd engine && go test ./... -race
cd apps/desktop && pnpm tsc -b && pnpm build
```

- [ ] **Step 8: Tag**

```bash
git tag plan-16-hierarchical-context-done
```

---

## Done conditions

- [ ] `cd engine && go test ./... -race` green.
- [ ] `cd apps/desktop && pnpm tsc -b && pnpm build` green.
- [ ] Smoke walkthrough Steps 3, 4, 5, 6 all produce the expected SQL output.
- [ ] Tag `plan-16-hierarchical-context-done` exists.
- [ ] No new files outside the file structure list above.
- [ ] No new migrations (Plan 9 columns sufficient).

---

## Self-review checklist (executed during drafting)

1. **Recursive CTE compatibility:** modernc.org/sqlite implements full SQLite syntax including `WITH RECURSIVE`. The CTE in Task 1 is a standard ancestor walk; nothing exotic. Confirmed by grepping the driver import (`engine/internal/store/store.go`) and the SQLite docs — supported since 3.8.3, modernc.org/sqlite tracks current.
2. **Recursive summarizer termination:** `summarizeContainer` only recurses into stale children, and only via `summarizeOneDepth(ctx, child.ID, depth+1)`. Leaves are the base case (`summarizeLeaf`, no recursion). The hard cap `maxSummarizeDepth = 6` is a safety net; cycles cannot occur because `nodes.parent_id` is a tree (no FK cycles allowed). Confirmed.
3. **"Next leaf" edge case:** In `loadHierarchicalContext`, the iteration `for _, j := range []int{curIdx - 2, curIdx - 1, curIdx + 1}` guards `j < 0 || j >= len(leaves)`. If `cur` is the very last leaf, `curIdx + 1 == len(leaves)` and that slot is skipped — `NearbyLeafSummaries` ends up with at most 2 entries. Correct.
4. **Topology RAG exclusion is Go-side:** `loadRelatedScenes` builds `excl := map[string]bool` from `nearbyIDs + cur.ID` and post-filters the SQL result. The SQL itself only excludes `m.node_id != ?` (cur). Confirmed.
5. **Budget constants spelled out:** `hierarchicalMaxChars = 2500` (in `context.go`), `containerSummaryMaxRunes = 4000` (in `summarizer.go`), `prevSummaryMaxRunes = 300` (existing). All used at the documented call sites.
6. **JSON ↔ prompt order parity:** The `Context` struct field order matches the section emission order in `buildUser`: `Hierarchical.ProjectSynopsis` → `OtherChapter+OtherPart` → `SameChapter` → `Nearby` → `RelatedScenes` → scene → entities (with `Recent` dossier) → threads → notes → style notes → user prompt. Audit reproducibility holds.

### Critical Files for Implementation

- /Users/changheonshin/workspace/myworks/linetta/engine/internal/node/repo.go
- /Users/changheonshin/workspace/myworks/linetta/engine/internal/summarizer/summarizer.go
- /Users/changheonshin/workspace/myworks/linetta/engine/internal/ai/context.go
- /Users/changheonshin/workspace/myworks/linetta/engine/internal/ai/prompts.go
- /Users/changheonshin/workspace/myworks/linetta/engine/internal/mention/repo.go
