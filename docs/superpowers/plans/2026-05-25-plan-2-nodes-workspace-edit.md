# Plan 2 — Nodes + Workspace Edit mode (no @mention)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Workspace placeholder with a real edit experience. The user opens a project, sees a serif 65ch Tiptap editor on the auto-created `씬 1`, types prose, and the engine autosaves it (800ms debounce) — including word-count propagation to the leaf node and parent project, plus periodic and manual snapshots. The right context panel shows the current scene's status and live word count; `인물·장소` and `활성 Thread` sections are visible but say "(곧 추가됨)".

**Architecture:** A new `engine/internal/node` package wraps Node domain logic — `Get`, `UpdateContent` (recomputes word count and propagates to projects), `SetLastOpened`. A new `engine/internal/snapshot` package wraps `node_snapshots`. Four new RPC handlers (`nodes.get`, `nodes.update_content`, `nodes.set_last_opened`, `snapshots.create_manual`) hang off the engine. On the React side, a `Tiptap.tsx` component wraps `@tiptap/react` with `StarterKit`; the Workspace route loads the node body, mounts the editor, debounces saves through Tiptap's `onUpdate`, and handles `Cmd+S` for manual snapshots. The right panel shows live word count from the editor's local state for instant feedback while keeping the engine as the source of truth between saves.

**Tech Stack additions:**
- `@tiptap/react` — React wrapper around ProseMirror
- `@tiptap/pm` — ProseMirror primitives bundle
- `@tiptap/starter-kit` — paragraph + heading + bold + italic + blockquote + hardBreak + history + input rules
- Standard library `unicode/utf8` for character counting

**Spec reference:** `docs/superpowers/specs/2026-05-25-linetta-immersive-rebuild-design.md` — §3 (`nodes`, `node_snapshots` rows), §4.3 (Workspace Edit mode), §5.1 (editor model), §5.3 (save flow), §11.1 item 3.

---

## Pre-flight

- [ ] **Step P1: Plan 1 is done**

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git describe --tags --exact-match plan-1-library-done >/dev/null && echo ok
git status --short  # must be empty
```

- [ ] **Step P2: Toolchain unchanged**

`go version`, `pnpm --version`, `cargo --version`, `cargo tauri --version` — all still resolvable.

---

## File Structure (created or modified by this plan)

```
linetta/
├── engine/
│   ├── go.mod                              (unchanged)
│   ├── internal/
│   │   ├── node/                           (new package)
│   │   │   ├── node.go                     (domain types)
│   │   │   ├── word_count.go               (Tiptap doc walker)
│   │   │   ├── word_count_test.go
│   │   │   ├── repo.go                     (Get / UpdateContent / SetLastOpened)
│   │   │   └── repo_test.go
│   │   ├── snapshot/                       (new package)
│   │   │   ├── snapshot.go                 (domain)
│   │   │   ├── repo.go
│   │   │   └── repo_test.go
│   │   └── rpc/handlers/
│   │       ├── nodes.go                    (new)
│   │       ├── nodes_test.go               (new)
│   │       ├── snapshots.go                (new)
│   │       └── snapshots_test.go           (new)
│   └── cmd/linetta-engine/main.go          (modified — adds repos + handlers)
└── apps/desktop/
    ├── package.json                        (modified — adds 3 tiptap packages)
    ├── src/
    │   ├── lib/
    │   │   ├── types.ts                    (extended — Node + Snapshot types)
    │   │   └── rpc.ts                      (extended — nodes + snapshots APIs)
    │   ├── components/
    │   │   ├── editor/
    │   │   │   ├── Tiptap.tsx              (new — editor wrapper)
    │   │   │   └── Tiptap.css              (new)
    │   │   └── ContextPanel.tsx            (new — right column)
    │   ├── hooks/
    │   │   ├── useDebouncedCallback.ts     (new)
    │   │   └── useThrottledCallback.ts     (new)
    │   ├── routes/Workspace.tsx            (replaced — real Edit mode)
    │   └── App.css                         (APPEND — workspace + editor styles)
```

The `apps/desktop/src/hooks/` directory is created in this plan and will host more hooks (typewriter scroll, mention suggestion) in later plans.

---

## Task 1: Word-count walker (TDD)

We count Korean-style 자(character count, whitespace included). This is the unit shown on cards ("412자"). The function takes a Tiptap doc as raw JSON (decoded into `any` / `map[string]any`) and returns the total visible character count.

**Files:**
- Create: `engine/internal/node/word_count.go`
- Create: `engine/internal/node/word_count_test.go`

- [ ] **Step 1: Write the failing test**

```go
package node

import (
	"encoding/json"
	"testing"
)

func TestCountChars_emptyDoc(t *testing.T) {
	doc := []byte(`{"type":"doc","content":[{"type":"paragraph"}]}`)
	if got := CountChars(doc); got != 0 {
		t.Errorf("CountChars(empty) = %d, want 0", got)
	}
}

func TestCountChars_singleParagraph(t *testing.T) {
	// "안녕 세계" is 5 chars including the space.
	doc := []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"안녕 세계"}]}]}`)
	if got := CountChars(doc); got != 5 {
		t.Errorf("CountChars = %d, want 5", got)
	}
}

func TestCountChars_multipleNodes(t *testing.T) {
	doc := []byte(`{"type":"doc","content":[
		{"type":"paragraph","content":[{"type":"text","text":"파도 소리"}]},
		{"type":"paragraph","content":[{"type":"text","text":"가 멀어졌다."}]}
	]}`)
	// "파도 소리" = 5 chars, "가 멀어졌다." = 7 chars; total 12.
	if got := CountChars(doc); got != 12 {
		t.Errorf("CountChars = %d, want 12", got)
	}
}

func TestCountChars_marksAreIgnored(t *testing.T) {
	// Bold mark doesn't add chars.
	doc := []byte(`{"type":"doc","content":[{"type":"paragraph","content":[
		{"type":"text","text":"굵게","marks":[{"type":"bold"}]},
		{"type":"text","text":" 일반"}
	]}]}`)
	if got := CountChars(doc); got != 5 {
		t.Errorf("CountChars = %d, want 5", got)
	}
}

func TestCountChars_malformed_returnsZero(t *testing.T) {
	// Garbage input must not panic.
	cases := [][]byte{
		[]byte(`not json`),
		[]byte(`null`),
		[]byte(`{}`),
		nil,
	}
	for _, c := range cases {
		_ = json.RawMessage(c) // just to keep the import used
		if got := CountChars(c); got != 0 {
			t.Errorf("CountChars(%q) = %d, want 0", string(c), got)
		}
	}
}
```

- [ ] **Step 2: Run — expect compile failure (`CountChars` undefined)**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine
go test ./internal/node/...
```

- [ ] **Step 3: Implement `word_count.go`**

```go
// Package node owns Node domain logic for the Tiptap-content recursive tree.
package node

import (
	"encoding/json"
	"unicode/utf8"
)

// CountChars walks a Tiptap doc (raw JSON) and returns the total visible
// character count including spaces — what Korean writing UIs label as "자".
// Returns 0 for any malformed or empty input.
func CountChars(rawDoc []byte) int {
	if len(rawDoc) == 0 {
		return 0
	}
	var any interface{}
	if err := json.Unmarshal(rawDoc, &any); err != nil {
		return 0
	}
	return walk(any)
}

func walk(v interface{}) int {
	switch t := v.(type) {
	case map[string]interface{}:
		// A text node has {"type":"text","text":"..."} — count utf8 chars in text.
		if kind, _ := t["type"].(string); kind == "text" {
			if s, ok := t["text"].(string); ok {
				return utf8.RuneCountInString(s)
			}
			return 0
		}
		// Otherwise, recurse into "content" if present.
		if content, ok := t["content"].([]interface{}); ok {
			n := 0
			for _, c := range content {
				n += walk(c)
			}
			return n
		}
		return 0
	case []interface{}:
		n := 0
		for _, c := range t {
			n += walk(c)
		}
		return n
	}
	return 0
}
```

- [ ] **Step 4: Run — expect PASS (5 tests)**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine
go test ./internal/node/... -v
```

- [ ] **Step 5: Commit**

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add engine/internal/node/word_count.go engine/internal/node/word_count_test.go
git commit -m "feat(node): Tiptap doc character counter"
```

---

## Task 2: Node domain types + repo (TDD)

**Files:**
- Create: `engine/internal/node/node.go`
- Create: `engine/internal/node/repo.go`
- Create: `engine/internal/node/repo_test.go`

- [ ] **Step 1: Write domain types**

```go
package node

// Node mirrors the SQLite row. content_doc is the raw Tiptap JSON; the engine
// stores and serves it verbatim, never re-shaping it.
type Node struct {
	ID         string  `json:"id"`
	ProjectID  string  `json:"project_id"`
	ParentID   *string `json:"parent_id,omitempty"`
	Ordinal    int     `json:"ordinal"`
	Kind       string  `json:"kind"`     // 'container' | 'leaf'
	Label      string  `json:"label"`
	Title      string  `json:"title"`
	ContentDoc *string `json:"content_doc,omitempty"` // null for containers
	Status     string  `json:"status"`
	WordCount  int     `json:"word_count"`
	CreatedAt  int64   `json:"created_at"`
	UpdatedAt  int64   `json:"updated_at"`
}
```

Use Write to put this content into `engine/internal/node/node.go`.

- [ ] **Step 2: Write the failing repo test**

Use Write to create `engine/internal/node/repo_test.go`:

```go
package node

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func newStoreAndProject(t *testing.T) (*store.Store, project.Project) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	pr := project.NewRepo(s)
	p, err := pr.Create(context.Background(), 1000, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	return s, p
}

func TestRepo_Get_firstLeaf(t *testing.T) {
	s, p := newStoreAndProject(t)
	if p.LastOpenedNodeID == nil {
		t.Fatal("project has no first leaf")
	}
	r := NewRepo(s)
	n, err := r.Get(context.Background(), *p.LastOpenedNodeID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if n.Kind != "leaf" {
		t.Errorf("kind = %q, want leaf", n.Kind)
	}
	if n.Label != "씬 1" {
		t.Errorf("label = %q, want 씬 1", n.Label)
	}
	if n.ContentDoc == nil {
		t.Fatal("first leaf has no content_doc")
	}
}

func TestRepo_UpdateContent_updatesWordCount_andProjectCount(t *testing.T) {
	s, p := newStoreAndProject(t)
	r := NewRepo(s)
	pr := project.NewRepo(s)
	ctx := context.Background()

	// Insert content with 5 visible characters.
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"안녕 세계"}]}]}`
	if err := r.UpdateContent(ctx, *p.LastOpenedNodeID, doc, 9999); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}

	got, err := r.Get(ctx, *p.LastOpenedNodeID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.WordCount != 5 {
		t.Errorf("node.word_count = %d, want 5", got.WordCount)
	}
	if got.UpdatedAt != 9999 {
		t.Errorf("node.updated_at = %d, want 9999", got.UpdatedAt)
	}

	pp, err := pr.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("project Get: %v", err)
	}
	if pp.WordCount != 5 {
		t.Errorf("project.word_count = %d, want 5", pp.WordCount)
	}
	if pp.UpdatedAt != 9999 {
		t.Errorf("project.updated_at = %d, want 9999", pp.UpdatedAt)
	}
}

func TestRepo_SetLastOpened(t *testing.T) {
	s, p := newStoreAndProject(t)
	r := NewRepo(s)
	pr := project.NewRepo(s)
	ctx := context.Background()

	original := *p.LastOpenedNodeID
	if err := r.SetLastOpened(ctx, p.ID, original, 1234); err != nil {
		t.Fatalf("SetLastOpened: %v", err)
	}

	pp, err := pr.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("project Get: %v", err)
	}
	if pp.LastOpenedNodeID == nil || *pp.LastOpenedNodeID != original {
		t.Errorf("last_opened_node_id = %v, want %q", pp.LastOpenedNodeID, original)
	}
	if pp.UpdatedAt != 1234 {
		t.Errorf("project.updated_at = %d, want 1234", pp.UpdatedAt)
	}
}

func TestRepo_UpdateContent_rejectsMissingNode(t *testing.T) {
	s, _ := newStoreAndProject(t)
	r := NewRepo(s)
	err := r.UpdateContent(context.Background(), "no-such-id", `{"type":"doc"}`, 1)
	if err != ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
```

- [ ] **Step 3: Run — expect compile failure**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine
go test ./internal/node/...
```

- [ ] **Step 4: Implement `repo.go`**

```go
package node

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/devlikebear/linetta/engine/internal/store"
)

// ErrNotFound is returned when a node id does not exist.
var ErrNotFound = errors.New("node not found")

// Repo persists Nodes in SQLite and keeps derived counts on projects in sync.
type Repo struct {
	s *store.Store
}

// NewRepo returns a Repo backed by the given Store.
func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

// Get returns a single node by id.
func (r *Repo) Get(ctx context.Context, id string) (Node, error) {
	row := r.s.DB().QueryRowContext(ctx, baseSelect+` WHERE id = ?`, id)
	n, err := scan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Node{}, ErrNotFound
	}
	return n, err
}

// UpdateContent replaces the leaf node's content_doc, recomputes its word_count,
// updates `projects.word_count` (= sum of leaf word_counts in that project),
// and touches `updated_at` on both rows.
func (r *Repo) UpdateContent(ctx context.Context, id string, doc string, now int64) error {
	count := CountChars([]byte(doc))

	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
UPDATE nodes
   SET content_doc = ?, word_count = ?, updated_at = ?
 WHERE id = ?`, doc, count, now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}

	// Recompute project total + touch its updated_at.
	if _, err := tx.ExecContext(ctx, `
UPDATE projects
   SET word_count = COALESCE((
        SELECT SUM(word_count) FROM nodes
         WHERE project_id = projects.id AND kind = 'leaf'), 0),
       updated_at = ?
 WHERE id = (SELECT project_id FROM nodes WHERE id = ?)`, now, id); err != nil {
		return fmt.Errorf("update project totals: %w", err)
	}

	return tx.Commit()
}

// SetLastOpened updates projects.last_opened_node_id and projects.updated_at.
func (r *Repo) SetLastOpened(ctx context.Context, projectID, nodeID string, now int64) error {
	res, err := r.s.DB().ExecContext(ctx, `
UPDATE projects SET last_opened_node_id = ?, updated_at = ?
 WHERE id = ?`, nodeID, now, projectID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

const baseSelect = `
SELECT id, project_id, parent_id, ordinal, kind, label, title,
       content_doc, status, word_count, created_at, updated_at
FROM nodes`

type scanner interface{ Scan(...any) error }

func scan(row scanner) (Node, error) {
	var (
		n          Node
		parentID   sql.NullString
		contentDoc sql.NullString
	)
	if err := row.Scan(&n.ID, &n.ProjectID, &parentID, &n.Ordinal, &n.Kind, &n.Label, &n.Title,
		&contentDoc, &n.Status, &n.WordCount, &n.CreatedAt, &n.UpdatedAt); err != nil {
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

- [ ] **Step 5: Run — expect PASS (4 tests)**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine
go test ./internal/node/... -v
```

- [ ] **Step 6: Run full suite**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine
go test ./...
```

- [ ] **Step 7: Commit**

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add engine/internal/node/node.go engine/internal/node/repo.go engine/internal/node/repo_test.go
git commit -m "feat(node): repo with Get / UpdateContent / SetLastOpened"
```

---

## Task 3: Snapshot package (TDD)

Snapshots persist a copy of `nodes.content_doc` at a moment in time with a `reason`. Plan 2 uses `manual` (Cmd+S) and `autosave` (engine inserts when last autosave > 60s ago). `ai-replace` is wired in Plan 5.

**Files:**
- Create: `engine/internal/snapshot/snapshot.go`
- Create: `engine/internal/snapshot/repo.go`
- Create: `engine/internal/snapshot/repo_test.go`

- [ ] **Step 1: Domain type**

```go
// Package snapshot persists node_snapshots — point-in-time copies of a leaf
// node's content_doc, tagged with a reason (manual, autosave, ai-replace).
package snapshot

// Snapshot mirrors the node_snapshots row.
type Snapshot struct {
	ID         string `json:"id"`
	NodeID     string `json:"node_id"`
	ContentDoc string `json:"content_doc"`
	Reason     string `json:"reason"`
	CreatedAt  int64  `json:"created_at"`
}

// Reasons.
const (
	ReasonManual    = "manual"
	ReasonAutosave  = "autosave"
	ReasonAIReplace = "ai-replace"
)
```

Use Write to create `engine/internal/snapshot/snapshot.go`.

- [ ] **Step 2: Failing test**

Write `engine/internal/snapshot/repo_test.go`:

```go
package snapshot

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func newRepoWithNode(t *testing.T) (*Repo, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	pr := project.NewRepo(s)
	p, err := pr.Create(context.Background(), 1000, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	return NewRepo(s), *p.LastOpenedNodeID
}

func TestCreate_returnsRowWithGeneratedID(t *testing.T) {
	r, nodeID := newRepoWithNode(t)
	got, err := r.Create(context.Background(), nodeID, `{"type":"doc"}`, ReasonManual, 5000)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.ID == "" {
		t.Error("missing id")
	}
	if got.NodeID != nodeID {
		t.Errorf("node_id = %q", got.NodeID)
	}
	if got.Reason != ReasonManual {
		t.Errorf("reason = %q", got.Reason)
	}
	if got.CreatedAt != 5000 {
		t.Errorf("created_at = %d", got.CreatedAt)
	}
}

func TestLatestForNode_returnsMostRecent(t *testing.T) {
	r, nodeID := newRepoWithNode(t)
	ctx := context.Background()
	_, _ = r.Create(ctx, nodeID, `{"v":1}`, ReasonAutosave, 1000)
	_, _ = r.Create(ctx, nodeID, `{"v":2}`, ReasonAutosave, 2000)
	_, _ = r.Create(ctx, nodeID, `{"v":3}`, ReasonManual, 3000)

	latest, err := r.LatestForNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("LatestForNode: %v", err)
	}
	if latest.CreatedAt != 3000 || latest.ContentDoc != `{"v":3}` {
		t.Errorf("latest = %+v, want v=3", latest)
	}
}

func TestLatestForNode_emptyReturnsNotFound(t *testing.T) {
	r, _ := newRepoWithNode(t)
	_, err := r.LatestForNode(context.Background(), "no-such-node")
	if err != ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestLatestAutosaveTime(t *testing.T) {
	r, nodeID := newRepoWithNode(t)
	ctx := context.Background()
	// Mix of reasons; only the most recent autosave matters.
	_, _ = r.Create(ctx, nodeID, "{}", ReasonManual, 1000)
	_, _ = r.Create(ctx, nodeID, "{}", ReasonAutosave, 2000)
	_, _ = r.Create(ctx, nodeID, "{}", ReasonManual, 3000)
	_, _ = r.Create(ctx, nodeID, "{}", ReasonAutosave, 4000)

	got, ok, err := r.LatestAutosaveTime(ctx, nodeID)
	if err != nil {
		t.Fatalf("LatestAutosaveTime: %v", err)
	}
	if !ok || got != 4000 {
		t.Errorf("got %d ok=%v, want 4000 true", got, ok)
	}

	r2, otherNode := newRepoWithNode(t)
	_, _, err = r2.LatestAutosaveTime(context.Background(), otherNode)
	if err != nil {
		t.Fatalf("LatestAutosaveTime empty: %v", err)
	}
}
```

- [ ] **Step 3: Run — expect compile failure**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine
go test ./internal/snapshot/...
```

- [ ] **Step 4: Implement `repo.go`**

```go
package snapshot

import (
	"context"
	"database/sql"
	"errors"

	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/google/uuid"
)

// ErrNotFound is returned when no snapshot exists for a query.
var ErrNotFound = errors.New("snapshot not found")

// Repo persists node_snapshots rows.
type Repo struct {
	s *store.Store
}

// NewRepo returns a Repo backed by the given Store.
func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

// Create inserts a snapshot and returns it with a generated id.
func (r *Repo) Create(ctx context.Context, nodeID, doc, reason string, now int64) (Snapshot, error) {
	id := uuid.NewString()
	if _, err := r.s.DB().ExecContext(ctx, `
INSERT INTO node_snapshots (id, node_id, content_doc, reason, created_at)
VALUES (?, ?, ?, ?, ?)`, id, nodeID, doc, reason, now); err != nil {
		return Snapshot{}, err
	}
	return Snapshot{ID: id, NodeID: nodeID, ContentDoc: doc, Reason: reason, CreatedAt: now}, nil
}

// LatestForNode returns the most recent snapshot for the node (any reason).
func (r *Repo) LatestForNode(ctx context.Context, nodeID string) (Snapshot, error) {
	row := r.s.DB().QueryRowContext(ctx, `
SELECT id, node_id, content_doc, reason, created_at
  FROM node_snapshots
 WHERE node_id = ?
 ORDER BY created_at DESC
 LIMIT 1`, nodeID)
	var s Snapshot
	if err := row.Scan(&s.ID, &s.NodeID, &s.ContentDoc, &s.Reason, &s.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Snapshot{}, ErrNotFound
		}
		return Snapshot{}, err
	}
	return s, nil
}

// LatestAutosaveTime returns (created_at, true) of the most recent autosave
// for the node, or (0, false) if none.
func (r *Repo) LatestAutosaveTime(ctx context.Context, nodeID string) (int64, bool, error) {
	var t sql.NullInt64
	err := r.s.DB().QueryRowContext(ctx, `
SELECT MAX(created_at) FROM node_snapshots
 WHERE node_id = ? AND reason = 'autosave'`, nodeID).Scan(&t)
	if err != nil {
		return 0, false, err
	}
	if !t.Valid {
		return 0, false, nil
	}
	return t.Int64, true, nil
}
```

- [ ] **Step 5: Run — PASS**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine
go test ./internal/snapshot/... -v
```

- [ ] **Step 6: Commit**

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add engine/internal/snapshot
git commit -m "feat(snapshot): Create + LatestForNode + LatestAutosaveTime"
```

---

## Task 4: Node + Snapshot RPC handlers (TDD)

We expose four handlers: `nodes.get`, `nodes.update_content` (which also creates an autosave snapshot if > 60s have passed since the last one), `nodes.set_last_opened`, and `snapshots.create_manual`.

**Files:**
- Create: `engine/internal/rpc/handlers/nodes.go`
- Create: `engine/internal/rpc/handlers/nodes_test.go`
- Create: `engine/internal/rpc/handlers/snapshots.go`
- Create: `engine/internal/rpc/handlers/snapshots_test.go`

- [ ] **Step 1: Write the failing handler test for nodes**

Write `engine/internal/rpc/handlers/nodes_test.go`:

```go
package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/snapshot"
	"github.com/devlikebear/linetta/engine/internal/store"
)

type nodeFixture struct {
	store *store.Store
	proj  *project.Repo
	nodes *node.Repo
	snaps *snapshot.Repo
	pID   string
	nID   string
}

func newNodeFixture(t *testing.T) nodeFixture {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	pr := project.NewRepo(s)
	p, err := pr.Create(context.Background(), 1000, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return nodeFixture{
		store: s, proj: pr, nodes: node.NewRepo(s), snaps: snapshot.NewRepo(s),
		pID: p.ID, nID: *p.LastOpenedNodeID,
	}
}

func TestGetNodeHandler(t *testing.T) {
	f := newNodeFixture(t)
	h := GetNode(f.nodes)
	res, err := h(context.Background(), json.RawMessage(`{"id":"`+f.nID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var n node.Node
	if err := json.Unmarshal(res, &n); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if n.Label != "씬 1" {
		t.Errorf("label = %q", n.Label)
	}
}

func TestUpdateNodeContentHandler_createsAutosaveSnapshotOnFirstSave(t *testing.T) {
	f := newNodeFixture(t)
	clock := int64(10_000)
	h := UpdateNodeContent(f.nodes, f.snaps, func() int64 { return clock })

	res, err := h(context.Background(), json.RawMessage(`{"id":"`+f.nID+`","doc":"{\"type\":\"doc\",\"content\":[{\"type\":\"paragraph\",\"content\":[{\"type\":\"text\",\"text\":\"파도 소리\"}]}]}"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var out node.Node
	if err := json.Unmarshal(res, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.WordCount != 5 {
		t.Errorf("word_count = %d, want 5", out.WordCount)
	}

	// First save → snapshot must be created.
	at, ok, err := f.snaps.LatestAutosaveTime(context.Background(), f.nID)
	if err != nil || !ok {
		t.Fatalf("expected an autosave snapshot to exist; ok=%v err=%v", ok, err)
	}
	if at != 10_000 {
		t.Errorf("autosave at = %d, want 10000", at)
	}
}

func TestUpdateNodeContentHandler_noSnapshotWithin60s(t *testing.T) {
	f := newNodeFixture(t)
	clock := int64(10_000)
	h := UpdateNodeContent(f.nodes, f.snaps, func() int64 { return clock })

	if _, err := h(context.Background(), json.RawMessage(`{"id":"`+f.nID+`","doc":"{}"}`)); err != nil {
		t.Fatalf("save 1: %v", err)
	}
	clock = 30_000 // 20 seconds later
	if _, err := h(context.Background(), json.RawMessage(`{"id":"`+f.nID+`","doc":"{}"}`)); err != nil {
		t.Fatalf("save 2: %v", err)
	}
	// Should still only be the first snapshot (no new one within 60s).
	at, _, _ := f.snaps.LatestAutosaveTime(context.Background(), f.nID)
	if at != 10_000 {
		t.Errorf("autosave at = %d, want 10000 (snapshot should not have been refreshed)", at)
	}

	clock = 80_000 // > 60s after last snapshot
	if _, err := h(context.Background(), json.RawMessage(`{"id":"`+f.nID+`","doc":"{}"}`)); err != nil {
		t.Fatalf("save 3: %v", err)
	}
	at, _, _ = f.snaps.LatestAutosaveTime(context.Background(), f.nID)
	if at != 80_000 {
		t.Errorf("autosave at = %d, want 80000", at)
	}
}

func TestSetLastOpenedHandler(t *testing.T) {
	f := newNodeFixture(t)
	h := SetLastOpened(f.nodes, func() int64 { return 9999 })
	params := json.RawMessage(`{"project_id":"` + f.pID + `","node_id":"` + f.nID + `"}`)
	if _, err := h(context.Background(), params); err != nil {
		t.Fatalf("handler: %v", err)
	}
	got, err := f.proj.Get(context.Background(), f.pID)
	if err != nil {
		t.Fatalf("project Get: %v", err)
	}
	if got.LastOpenedNodeID == nil || *got.LastOpenedNodeID != f.nID {
		t.Errorf("last_opened = %v", got.LastOpenedNodeID)
	}
}
```

- [ ] **Step 2: Run — expect compile failure**

- [ ] **Step 3: Implement `nodes.go`**

```go
package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/snapshot"
)

// AutosaveIntervalMillis controls how often UpdateNodeContent inserts a fresh
// autosave snapshot. Exposed for tests; production passes time.Now().UnixMilli.
const AutosaveIntervalMillis int64 = 60_000

type updateContentParams struct {
	ID  string `json:"id"`
	Doc string `json:"doc"`
}

// GetNode returns a handler for nodes.get.
func GetNode(nodes *node.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		n, err := nodes.Get(ctx, p.ID)
		if errors.Is(err, node.ErrNotFound) {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node not found"}
		}
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(n)
	}
}

// UpdateNodeContent returns a handler for nodes.update_content. After saving,
// if more than AutosaveIntervalMillis have elapsed since the last autosave for
// this node, a fresh autosave snapshot is inserted.
func UpdateNodeContent(nodes *node.Repo, snaps *snapshot.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p updateContentParams
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id and doc required"}
		}
		t := now()
		if err := nodes.UpdateContent(ctx, p.ID, p.Doc, t); err != nil {
			if errors.Is(err, node.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}

		// Maybe-snapshot.
		last, ok, err := snaps.LatestAutosaveTime(ctx, p.ID)
		if err == nil && (!ok || t-last >= AutosaveIntervalMillis) {
			_, _ = snaps.Create(ctx, p.ID, p.Doc, snapshot.ReasonAutosave, t)
		}

		got, err := nodes.Get(ctx, p.ID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(got)
	}
}

type setLastOpenedParams struct {
	ProjectID string `json:"project_id"`
	NodeID    string `json:"node_id"`
}

// SetLastOpened returns a handler for nodes.set_last_opened.
func SetLastOpened(nodes *node.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p setLastOpenedParams
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" || p.NodeID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id and node_id required"}
		}
		if err := nodes.SetLastOpened(ctx, p.ProjectID, p.NodeID, now()); err != nil {
			if errors.Is(err, node.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}
```

- [ ] **Step 4: Run nodes tests — PASS**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine
go test ./internal/rpc/handlers/... -run 'Node|SetLastOpened' -v
```

- [ ] **Step 5: Failing snapshot handler test**

Write `engine/internal/rpc/handlers/snapshots_test.go`:

```go
package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/snapshot"
)

func TestCreateManualSnapshotHandler(t *testing.T) {
	f := newNodeFixture(t)
	h := CreateManualSnapshot(f.snaps, func() int64 { return 7777 })

	res, err := h(context.Background(), json.RawMessage(`{"node_id":"`+f.nID+`","doc":"{\"v\":1}"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got snapshot.Snapshot
	if err := json.Unmarshal(res, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Reason != snapshot.ReasonManual {
		t.Errorf("reason = %q", got.Reason)
	}
	if got.CreatedAt != 7777 {
		t.Errorf("created_at = %d", got.CreatedAt)
	}
}
```

- [ ] **Step 6: Run — expect compile failure**

- [ ] **Step 7: Implement `snapshots.go`**

```go
package handlers

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/snapshot"
)

type manualSnapshotParams struct {
	NodeID string `json:"node_id"`
	Doc    string `json:"doc"`
}

// CreateManualSnapshot returns a handler for snapshots.create_manual.
func CreateManualSnapshot(snaps *snapshot.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p manualSnapshotParams
		if err := json.Unmarshal(params, &p); err != nil || p.NodeID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node_id and doc required"}
		}
		got, err := snaps.Create(ctx, p.NodeID, p.Doc, snapshot.ReasonManual, now())
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(got)
	}
}
```

- [ ] **Step 8: Run full suite — all PASS**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine
go test ./...
```

- [ ] **Step 9: Commit**

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add engine/internal/rpc/handlers/nodes.go engine/internal/rpc/handlers/nodes_test.go engine/internal/rpc/handlers/snapshots.go engine/internal/rpc/handlers/snapshots_test.go
git commit -m "feat(rpc): nodes.* and snapshots.create_manual handlers"
```

---

## Task 5: Wire new repos + handlers into main.go + stdio smoke

**Files:**
- Modify: `engine/cmd/linetta-engine/main.go` (full replacement)

- [ ] **Step 1: Replace `main.go`**

```go
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/devlikebear/tars/pkg/llm" // pin

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/paths"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/rpc/handlers"
	"github.com/devlikebear/linetta/engine/internal/snapshot"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func main() {
	stdio := flag.Bool("stdio", false, "serve JSONRPC over stdin/stdout")
	flag.Parse()

	if !*stdio {
		fmt.Fprintln(os.Stderr, "linetta-engine: --stdio required (other modes land in later plans)")
		os.Exit(2)
	}

	if err := paths.EnsureHome(); err != nil {
		fail("ensure home: %v", err)
	}
	dbPath, err := paths.DBPath()
	if err != nil {
		fail("db path: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	st, err := store.Open(ctx, dbPath)
	if err != nil {
		fail("open store: %v", err)
	}
	defer st.Close()

	projects := project.NewRepo(st)
	nodes := node.NewRepo(st)
	snaps := snapshot.NewRepo(st)
	clock := func() int64 { return time.Now().UnixMilli() }

	s := rpc.NewServer()
	s.Handle("ping", handlers.Ping)
	s.Handle("projects.create", handlers.CreateProject(projects, clock))
	s.Handle("projects.list", handlers.ListProjects(projects))
	s.Handle("projects.get", handlers.GetProject(projects))
	s.Handle("projects.archive", handlers.ArchiveProject(projects, clock))
	s.Handle("nodes.get", handlers.GetNode(nodes))
	s.Handle("nodes.update_content", handlers.UpdateNodeContent(nodes, snaps, clock))
	s.Handle("nodes.set_last_opened", handlers.SetLastOpened(nodes, clock))
	s.Handle("snapshots.create_manual", handlers.CreateManualSnapshot(snaps, clock))

	if err := s.Serve(ctx, os.Stdin, os.Stdout); err != nil && !errors.Is(err, io.EOF) {
		fail("serve: %v", err)
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "linetta-engine: "+format+"\n", args...)
	os.Exit(1)
}
```

- [ ] **Step 2: Build to /tmp**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/engine
go build -o /tmp/linetta-engine-build ./cmd/linetta-engine
```

- [ ] **Step 3: Stdio smoke (clean temp dir)**

```bash
rm -rf /tmp/linetta-plan2-smoke
LINETTA_HOME=/tmp/linetta-plan2-smoke /tmp/linetta-engine-build --stdio <<'EOF'
{"jsonrpc":"2.0","id":1,"method":"projects.create","params":{"title":"Smoke","genres":["SF"],"length_target":"short","default_pov":"first"}}
EOF
```

Expected: one line, the new project with `last_opened_node_id`. Note the node id from the result (we'll call it `$NID`). The plan can't compute the UUID for you — just visually verify the response is well-formed.

Now exercise the node handlers (replace `<NODE_ID>` with the value you observed):

```bash
LINETTA_HOME=/tmp/linetta-plan2-smoke /tmp/linetta-engine-build --stdio <<EOF
{"jsonrpc":"2.0","id":2,"method":"nodes.get","params":{"id":"<NODE_ID>"}}
{"jsonrpc":"2.0","id":3,"method":"nodes.update_content","params":{"id":"<NODE_ID>","doc":"{\"type\":\"doc\",\"content\":[{\"type\":\"paragraph\",\"content\":[{\"type\":\"text\",\"text\":\"안녕 세계\"}]}]}"}}
{"jsonrpc":"2.0","id":4,"method":"nodes.get","params":{"id":"<NODE_ID>"}}
EOF
rm -f /tmp/linetta-engine-build
rm -rf /tmp/linetta-plan2-smoke
```

Expected:
- id=2 → node with `word_count` = 0
- id=3 → node with `word_count` = 5 (안녕 세계)
- id=4 → confirms persistence

- [ ] **Step 4: Confirm clean tree, then commit**

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git status --short  # only main.go modified
git add engine/cmd/linetta-engine/main.go
git commit -m "feat(engine): wire node + snapshot repos and handlers"
```

- [ ] **Step 5: Rebuild dev engine binary**

```bash
./scripts/build-engine.sh
```

---

## Task 6: Tiptap dependency + editor component

**Files:**
- Modify: `apps/desktop/package.json` (adds 3 Tiptap packages)
- Create: `apps/desktop/src/components/editor/Tiptap.tsx`
- Create: `apps/desktop/src/components/editor/Tiptap.css`

- [ ] **Step 1: Install Tiptap**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop
pnpm add @tiptap/react @tiptap/pm @tiptap/starter-kit
```

Expected: three packages added. Versions may differ from what the plan was written against — current Tiptap 2.x is fine.

- [ ] **Step 2: Write `apps/desktop/src/components/editor/Tiptap.tsx`**

```tsx
import { useEditor, EditorContent, type Editor } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import { useEffect, useMemo, useRef } from "react";
import "./Tiptap.css";

interface Props {
  /** Tiptap JSON doc — controls the editor's initial state. The component is
   *  uncontrolled afterwards; consumers respond to onUpdate. */
  initialDoc: object;
  /** Called whenever the document changes (every keystroke). Debounce upstream. */
  onChange: (doc: object) => void;
  /** Called with the character count after each change (whitespace-included). */
  onCharCount?: (count: number) => void;
  /** Typewriter scroll: keeps the active line near the viewport center. */
  typewriter?: boolean;
  /** Manual-save hotkey handler — receives the current doc; consumer issues the RPC. */
  onManualSave?: (doc: object) => void;
}

export function TiptapEditor({ initialDoc, onChange, onCharCount, typewriter, onManualSave }: Props) {
  // Stable reference for the initial doc to avoid resetting on every render.
  const initialKey = useMemo(() => JSON.stringify(initialDoc).length, [initialDoc]);

  const editor = useEditor(
    {
      extensions: [StarterKit.configure({})],
      content: initialDoc,
      autofocus: "end",
      onUpdate: ({ editor }) => {
        const doc = editor.getJSON();
        onChange(doc);
        if (onCharCount) onCharCount(countChars(doc));
      },
    },
    // Re-create the editor only when the initial doc actually changes id/length —
    // avoids cursor jumps from upstream re-renders.
    [initialKey],
  );

  useEffect(() => {
    if (!editor) return;
    if (onCharCount) onCharCount(countChars(editor.getJSON()));
  }, [editor, onCharCount]);

  // Cmd+S → manual save (intercept before browser save dialog).
  useEffect(() => {
    if (!editor) return;
    const handler = (e: KeyboardEvent) => {
      const isMac = navigator.platform.toLowerCase().includes("mac");
      const isSave = (isMac ? e.metaKey : e.ctrlKey) && e.key.toLowerCase() === "s";
      if (!isSave) return;
      e.preventDefault();
      if (onManualSave) onManualSave(editor.getJSON());
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [editor, onManualSave]);

  return (
    <div className={`tiptap-wrap${typewriter ? " typewriter" : ""}`}>
      <EditorContent editor={editor} className="tiptap-editor" />
      <TypewriterScroll editor={editor} enabled={!!typewriter} />
    </div>
  );
}

/** Scrolls the editor so the current cursor line stays near viewport center. */
function TypewriterScroll({ editor, enabled }: { editor: Editor | null; enabled: boolean }) {
  const lastTop = useRef<number>(-1);
  useEffect(() => {
    if (!editor || !enabled) return;
    const handler = () => {
      const view = editor.view;
      const pos = view.state.selection.head;
      const coords = view.coordsAtPos(pos);
      const target = window.innerHeight / 2;
      const delta = coords.top - target;
      if (Math.abs(delta) < 4) return;
      if (delta === lastTop.current) return;
      lastTop.current = delta;
      window.scrollBy({ top: delta, behavior: "smooth" });
    };
    editor.on("selectionUpdate", handler);
    editor.on("update", handler);
    return () => {
      editor.off("selectionUpdate", handler);
      editor.off("update", handler);
    };
  }, [editor, enabled]);
  return null;
}

/** Lightweight TS port of engine/internal/node.CountChars. */
function countChars(node: any): number {
  if (!node || typeof node !== "object") return 0;
  if (node.type === "text" && typeof node.text === "string") return [...node.text].length;
  if (Array.isArray(node.content)) {
    let n = 0;
    for (const c of node.content) n += countChars(c);
    return n;
  }
  return 0;
}
```

- [ ] **Step 3: Write `apps/desktop/src/components/editor/Tiptap.css`**

```css
.tiptap-wrap {
  display: flex;
  justify-content: center;
  padding: 4rem 2rem;
}

.tiptap-wrap.typewriter {
  padding-top: 40vh;
  padding-bottom: 40vh;
}

.tiptap-editor {
  width: min(65ch, 100%);
}

.tiptap-editor .ProseMirror {
  outline: none;
  font-family: ui-serif, Georgia, "Apple SD Gothic Neo", serif;
  font-size: 1.1rem;
  line-height: 1.85;
  color: #1a1a1a;
}

.tiptap-editor .ProseMirror p {
  margin: 0 0 1.25rem;
}

.tiptap-editor .ProseMirror blockquote {
  border-left: 3px solid #c8c5bd;
  padding-left: 1rem;
  margin: 0 0 1.25rem;
  color: #555;
}

.tiptap-editor .ProseMirror h1,
.tiptap-editor .ProseMirror h2,
.tiptap-editor .ProseMirror h3 {
  font-family: inherit;
  font-weight: 600;
}

.tiptap-editor .ProseMirror hr {
  border: none;
  border-top: 1px solid #d8d6cf;
  margin: 2rem 0;
}
```

- [ ] **Step 4: Type-check + build**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop
pnpm tsc -b
pnpm build
```

If TypeScript complains about `useEditor`'s 2nd argument (deps array), check the installed Tiptap version. In `@tiptap/react` v2.x, the signature is `useEditor(options, deps?)` — if your installed version differs, drop the deps array and just call `useEditor(options)` (the component will re-create when remounted, which is fine for our flow). Note the change in the report.

- [ ] **Step 5: Commit**

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/package.json apps/desktop/pnpm-lock.yaml apps/desktop/src/components/editor
git commit -m "feat(editor): Tiptap component wrapper with typewriter + manual save hook"
```

---

## Task 7: Debounce + throttle hooks

**Files:**
- Create: `apps/desktop/src/hooks/useDebouncedCallback.ts`
- Create: `apps/desktop/src/hooks/useThrottledCallback.ts`

- [ ] **Step 1: Make hooks dir**

```bash
mkdir -p /Users/changheonshin/workspace/myworks/linetta/apps/desktop/src/hooks
```

- [ ] **Step 2: Write `useDebouncedCallback.ts`**

```ts
import { useEffect, useMemo, useRef } from "react";

/** Returns a stable callback that delays running `fn` until `delayMs` have
 *  elapsed without a new call. The latest `fn` is always used. */
export function useDebouncedCallback<T extends (...args: any[]) => void>(
  fn: T,
  delayMs: number,
): T {
  const ref = useRef(fn);
  useEffect(() => { ref.current = fn; }, [fn]);
  const timer = useRef<number | undefined>(undefined);
  return useMemo(
    () =>
      ((...args: any[]) => {
        if (timer.current !== undefined) window.clearTimeout(timer.current);
        timer.current = window.setTimeout(() => ref.current(...args), delayMs);
      }) as T,
    [delayMs],
  );
}
```

- [ ] **Step 3: Write `useThrottledCallback.ts`**

```ts
import { useEffect, useMemo, useRef } from "react";

/** Calls `fn` at most once every `intervalMs`. The first call is immediate;
 *  subsequent calls within the window are coalesced and dispatched at window end. */
export function useThrottledCallback<T extends (...args: any[]) => void>(
  fn: T,
  intervalMs: number,
): T {
  const ref = useRef(fn);
  useEffect(() => { ref.current = fn; }, [fn]);
  const lastRun = useRef<number>(0);
  const queued = useRef<{ args: any[] } | null>(null);
  const timer = useRef<number | undefined>(undefined);

  return useMemo(
    () =>
      ((...args: any[]) => {
        const now = Date.now();
        const elapsed = now - lastRun.current;
        if (elapsed >= intervalMs) {
          lastRun.current = now;
          ref.current(...args);
          return;
        }
        queued.current = { args };
        if (timer.current !== undefined) return;
        timer.current = window.setTimeout(() => {
          timer.current = undefined;
          lastRun.current = Date.now();
          if (queued.current) {
            const q = queued.current;
            queued.current = null;
            ref.current(...q.args);
          }
        }, intervalMs - elapsed);
      }) as T,
    [intervalMs],
  );
}
```

- [ ] **Step 4: Type-check**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop
pnpm tsc -b
```

- [ ] **Step 5: Commit**

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/src/hooks
git commit -m "feat(hooks): useDebouncedCallback + useThrottledCallback"
```

---

## Task 8: TS types + RPC additions for nodes + snapshots

**Files:**
- Modify: `apps/desktop/src/lib/types.ts` (extend)
- Modify: `apps/desktop/src/lib/rpc.ts` (extend)

- [ ] **Step 1: Replace `apps/desktop/src/lib/types.ts` (preserve Project block, add Node + Snapshot)**

```ts
// Mirrors engine/internal/project Project struct (JSON tag names).
export type LengthTarget = "flash" | "short" | "novella" | "novel" | "series";
export type DefaultPOV = "first" | "third_limited" | "omniscient";

export interface Project {
  id: string;
  title: string;
  genres: string[];
  length_target: LengthTarget;
  default_pov: DefaultPOV;
  style_notes: string;
  word_count: number;
  last_opened_node_id?: string;
  created_at: number;
  updated_at: number;
  archived_at?: number;
}

export interface NewProjectInput {
  title: string;
  genres: string[];
  length_target: LengthTarget;
  default_pov: DefaultPOV;
}

export interface ListProjectsParams {
  include_archived?: boolean;
  limit?: number;
}

// Mirrors engine/internal/node Node struct.
export type NodeKind = "container" | "leaf";
export type NodeStatus = "draft" | "revision" | "final";

export interface NodeRow {
  id: string;
  project_id: string;
  parent_id?: string;
  ordinal: number;
  kind: NodeKind;
  label: string;
  title: string;
  content_doc?: string; // raw JSON string for leaves
  status: NodeStatus;
  word_count: number;
  created_at: number;
  updated_at: number;
}

// Mirrors engine/internal/snapshot Snapshot struct.
export type SnapshotReason = "manual" | "autosave" | "ai-replace";

export interface Snapshot {
  id: string;
  node_id: string;
  content_doc: string;
  reason: SnapshotReason;
  created_at: number;
}
```

- [ ] **Step 2: Replace `apps/desktop/src/lib/rpc.ts`**

```ts
import { invoke } from "@tauri-apps/api/core";
import type {
  ListProjectsParams,
  NewProjectInput,
  NodeRow,
  Project,
  Snapshot,
} from "./types";

// Tauri commands defined in src-tauri.

export async function enginePing(): Promise<string> {
  return invoke<string>("engine_ping");
}

export async function rpcCall<T>(method: string, params?: unknown): Promise<T> {
  return invoke<T>("engine_call", { method, params: params ?? null });
}

export const projects = {
  create: (input: NewProjectInput) => rpcCall<Project>("projects.create", input),
  list: (params: ListProjectsParams = {}) => rpcCall<Project[]>("projects.list", params),
  get: (id: string) => rpcCall<Project>("projects.get", { id }),
  archive: (id: string) => rpcCall<{ ok: true }>("projects.archive", { id }),
};

export const nodes = {
  get: (id: string) => rpcCall<NodeRow>("nodes.get", { id }),
  updateContent: (id: string, doc: string) =>
    rpcCall<NodeRow>("nodes.update_content", { id, doc }),
  setLastOpened: (projectId: string, nodeId: string) =>
    rpcCall<{ ok: true }>("nodes.set_last_opened", { project_id: projectId, node_id: nodeId }),
};

export const snapshots = {
  createManual: (nodeId: string, doc: string) =>
    rpcCall<Snapshot>("snapshots.create_manual", { node_id: nodeId, doc }),
};
```

- [ ] **Step 3: Type-check + build**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop
pnpm tsc -b
pnpm build
```

- [ ] **Step 4: Commit**

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/src/lib
git commit -m "feat(rpc): nodes and snapshots API clients"
```

---

## Task 9: Workspace Edit mode + ContextPanel

**Files:**
- Create: `apps/desktop/src/components/ContextPanel.tsx`
- Modify: `apps/desktop/src/routes/Workspace.tsx` (replace stub with full Edit mode)
- Modify: `apps/desktop/src/App.css` (APPEND workspace + context-panel styles)

- [ ] **Step 1: Write `ContextPanel.tsx`**

```tsx
import type { NodeRow, Project } from "../lib/types";

interface Props {
  project: Project;
  node: NodeRow;
  charCount: number;
  typewriter: boolean;
  onToggleTypewriter: () => void;
}

const STATUS_LABEL: Record<NodeRow["status"], string> = {
  draft: "초고",
  revision: "퇴고",
  final: "완성",
};

export function ContextPanel({ node, charCount, typewriter, onToggleTypewriter }: Props) {
  return (
    <aside className="ctx-panel">
      <section className="ctx-section">
        <h4>인물 · 장소</h4>
        <p className="ctx-empty">(곧 추가됨 — Plan 4)</p>
      </section>

      <section className="ctx-section">
        <h4>활성 Thread</h4>
        <p className="ctx-empty">(곧 추가됨 — post-MVP)</p>
      </section>

      <section className="ctx-section">
        <h4>씬 상태</h4>
        <p className="ctx-line">
          ● {STATUS_LABEL[node.status]} · {charCount.toLocaleString("ko-KR")}자
        </p>
      </section>

      <section className="ctx-section">
        <h4>옵션</h4>
        <label className="ctx-check">
          <input type="checkbox" checked={typewriter} onChange={onToggleTypewriter} />
          타자기 모드
        </label>
      </section>
    </aside>
  );
}
```

- [ ] **Step 2: Replace `apps/desktop/src/routes/Workspace.tsx`**

```tsx
import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { nodes, projects, snapshots } from "../lib/rpc";
import type { NodeRow, Project } from "../lib/types";
import { TiptapEditor } from "../components/editor/Tiptap";
import { ContextPanel } from "../components/ContextPanel";
import { useDebouncedCallback } from "../hooks/useDebouncedCallback";
import { useThrottledCallback } from "../hooks/useThrottledCallback";

const SAVE_DEBOUNCE_MS = 800;
const LAST_OPENED_THROTTLE_MS = 5000;

interface LoadState {
  project: Project;
  node: NodeRow;
  initialDoc: object;
}

export function Workspace() {
  const { projectId } = useParams();
  const [load, setLoad] = useState<LoadState | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [toast, setToast] = useState<string | null>(null);
  const [charCount, setCharCount] = useState(0);
  const [typewriter, setTypewriter] = useState(false);

  // Initial load: project → first leaf node → parse content_doc.
  useEffect(() => {
    if (!projectId) return;
    let cancelled = false;
    (async () => {
      try {
        const p = await projects.get(projectId);
        if (!p.last_opened_node_id) {
          throw new Error("project has no opened node");
        }
        const n = await nodes.get(p.last_opened_node_id);
        const docStr = n.content_doc ?? `{"type":"doc","content":[{"type":"paragraph"}]}`;
        const initialDoc = JSON.parse(docStr);
        if (!cancelled) {
          setLoad({ project: p, node: n, initialDoc });
          setCharCount(n.word_count);
        }
      } catch (e) {
        if (!cancelled) setError(String(e));
      }
    })();
    return () => { cancelled = true; };
  }, [projectId]);

  const saveNow = useCallback(
    async (doc: object) => {
      if (!load) return;
      try {
        await nodes.updateContent(load.node.id, JSON.stringify(doc));
      } catch (e) {
        setError(String(e));
      }
    },
    [load],
  );

  const debouncedSave = useDebouncedCallback(saveNow, SAVE_DEBOUNCE_MS);

  const throttledLastOpened = useThrottledCallback(
    useCallback(() => {
      if (!load) return;
      nodes.setLastOpened(load.project.id, load.node.id).catch(() => { /* benign */ });
    }, [load]),
    LAST_OPENED_THROTTLE_MS,
  );

  // Touch last_opened periodically while the editor is active.
  useEffect(() => {
    if (!load) return;
    throttledLastOpened();
  }, [load, throttledLastOpened]);

  const handleManualSave = useCallback(
    async (doc: object) => {
      if (!load) return;
      try {
        // Flush latest content first so the snapshot is in sync.
        await nodes.updateContent(load.node.id, JSON.stringify(doc));
        await snapshots.createManual(load.node.id, JSON.stringify(doc));
        showToast("스냅샷 저장됨");
      } catch (e) {
        setError(String(e));
      }
    },
    [load],
  );

  const showToast = (msg: string) => {
    setToast(msg);
    window.setTimeout(() => setToast(null), 1800);
  };

  const breadcrumb = useMemo(() => {
    if (!load) return "";
    return `← 작품 · ${load.node.label}${load.node.title ? ` — ${load.node.title}` : ""}`;
  }, [load]);

  if (error) {
    return (
      <main className="shell">
        <p><Link to="/">← Library</Link></p>
        <p className="error">{error}</p>
      </main>
    );
  }
  if (!load) {
    return (
      <main className="shell">
        <p className="hint">불러오는 중…</p>
      </main>
    );
  }

  return (
    <main className="workspace">
      <header className="ws-top">
        <Link to="/" className="ws-breadcrumb">{breadcrumb}</Link>
        <span className="ws-modes">
          <span className="mode-toggle on">편집</span>
          <span className="mode-toggle">AI</span>
        </span>
        <span className="ws-zen">ZEN</span>
      </header>

      <div className="ws-body">
        <div className="ws-editor">
          <TiptapEditor
            initialDoc={load.initialDoc}
            onChange={(doc) => {
              debouncedSave(doc);
              throttledLastOpened();
            }}
            onCharCount={setCharCount}
            typewriter={typewriter}
            onManualSave={handleManualSave}
          />
        </div>
        <ContextPanel
          project={load.project}
          node={load.node}
          charCount={charCount}
          typewriter={typewriter}
          onToggleTypewriter={() => setTypewriter((v) => !v)}
        />
      </div>

      {toast && <div className="ws-toast">{toast}</div>}
    </main>
  );
}
```

- [ ] **Step 3: APPEND workspace styles to `apps/desktop/src/App.css`**

```css
/* Workspace */
.workspace {
  min-height: 100vh;
  display: grid;
  grid-template-rows: auto 1fr;
}

.ws-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.75rem 1.25rem;
  border-bottom: 1px solid #ece9e0;
  background: #faf9f6;
  position: sticky;
  top: 0;
  z-index: 5;
}

.ws-breadcrumb {
  color: #4a4a4a;
  text-decoration: none;
  font-size: 0.9rem;
}

.ws-breadcrumb:hover {
  color: #1a1a1a;
}

.ws-modes {
  display: flex;
  border: 1px solid #1a1a1a;
  border-radius: 6px;
  overflow: hidden;
}

.mode-toggle {
  padding: 0.3rem 0.9rem;
  font-size: 0.85rem;
  cursor: pointer;
  user-select: none;
}

.mode-toggle.on {
  background: #1a1a1a;
  color: #faf9f6;
}

.ws-zen {
  font-size: 0.85rem;
  color: #6b6b6b;
}

.ws-body {
  display: grid;
  grid-template-columns: 1fr 220px;
  gap: 0;
}

.ws-editor {
  min-height: calc(100vh - 50px);
}

.ctx-panel {
  border-left: 1px solid #ece9e0;
  padding: 1.5rem 1rem;
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
  background: #f6f4ee;
}

.ctx-section h4 {
  margin: 0 0 0.5rem;
  font-size: 0.75rem;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: #6b6b6b;
}

.ctx-empty {
  margin: 0;
  color: #9a9a9a;
  font-size: 0.85rem;
}

.ctx-line {
  margin: 0;
  font-size: 0.9rem;
}

.ctx-check {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.9rem;
  cursor: pointer;
}

.ws-toast {
  position: fixed;
  bottom: 1.5rem;
  left: 50%;
  transform: translateX(-50%);
  background: #1a1a1a;
  color: #faf9f6;
  padding: 0.5rem 1rem;
  border-radius: 999px;
  font-size: 0.85rem;
}
```

- [ ] **Step 4: Type-check + build**

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop
pnpm tsc -b
pnpm build
```

- [ ] **Step 5: Verify status**

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git status --short
```
Expected:
- `M apps/desktop/src/App.css`
- `M apps/desktop/src/routes/Workspace.tsx`
- `?? apps/desktop/src/components/ContextPanel.tsx`

- [ ] **Step 6: Commit**

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/src
git commit -m "feat(workspace): Tiptap edit mode + autosave + context panel"
```

---

## Task 10: E2E smoke + milestone tag

Interactive. The controller pre-builds Rust + Go, then asks the human to run dev.sh and walk through the flow.

- [ ] **Step 1: Pre-warm builds**

```bash
cd /Users/changheonshin/workspace/myworks/linetta
./scripts/build-engine.sh
(cd apps/desktop/src-tauri && cargo build) >/dev/null 2>&1 || true
```

- [ ] **Step 2: Run dev**

```bash
rm -rf /tmp/linetta-plan2
LINETTA_HOME=/tmp/linetta-plan2 ./scripts/dev.sh
```

- [ ] **Step 3: Manual walk-through**

In the Tauri window:
1. Empty library → modal auto-opens. Create "은하의 노래" (단편, 1인칭).
2. After `시작`, the route is `/workspace/<id>`. **You should see a Tiptap editor**, the breadcrumb `← 작품 · 씬 1`, the mode toggle (편집 highlighted), and the right context panel with `인물·장소`, `활성 Thread`, `씬 상태` (`● 초고 · 0자`), and a `타자기 모드` checkbox.
3. **Type a sentence** in the editor — e.g., "파도 소리가 멀어졌다. 해진은 모래에 손을 묻은 채 사진을 바라보았다." The character count on the right updates as you type. Within ~1 second of stopping, autosave fires.
4. **Press Cmd+S** — a toast `스냅샷 저장됨` appears.
5. **Toggle 타자기 모드** — when enabled, the editor pads to ~40vh top/bottom and the active line tries to stay near the viewport center as you type.
6. **Reload the window** (Cmd+R) — the body comes back exactly as you left it.
7. **Click `← 작품 · 씬 1`** to return to Library — the card now shows the updated word count (e.g., "단편 · 42자" instead of "초안 시작 전").

- [ ] **Step 4: DB sanity check (after Ctrl-C of dev.sh)**

```bash
# Open the WAL'd DB read-only via sqlite (modernc.org/sqlite is the engine driver; system sqlite3 is fine for inspection).
sqlite3 /tmp/linetta-plan2/library.db "SELECT id, label, word_count FROM nodes;"
sqlite3 /tmp/linetta-plan2/library.db "SELECT reason, count(*) FROM node_snapshots GROUP BY reason;"
```
Expected:
- One node row, `word_count` matching what you typed.
- One or more `manual` snapshots (per each Cmd+S) plus an `autosave` snapshot if you typed for more than the autosave interval.

If `sqlite3` isn't installed: `brew install sqlite` or skip the DB check.

- [ ] **Step 5: Tag**

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git tag plan-2-edit-done
```

---

## Self-review checklist (run after writing the plan, not at execution time)

1. **Spec coverage**
   - §3 `nodes`, `node_snapshots`: Tasks 2, 3, 5. ✓
   - §4.3 Workspace Edit chrome (top breadcrumb / 편집·AI toggle / ZEN / right panel / typewriter toggle): Task 9. ✓ (AI toggle and ZEN are visual stubs; their real wiring lands in Plans 5–6.)
   - §5.1 editor model (Tiptap + serif + 65ch + input rules): Task 6. ✓
   - §5.3 save flow (debounce 800ms, propagate word_count, autosave snapshot rule, Cmd+S manual): Tasks 4, 9. ✓
   - §11.1 item 3 (Edit mode + autosave + Cmd+S + context panel): all of Plan 2. ✓

2. **Placeholder scan**
   - The placeholders in ContextPanel ("(곧 추가됨 — Plan 4)", "(post-MVP)") are intentional UI hints called out in the design doc.
   - The mode toggle's `편집` is hard-coded as `on` (no real toggle yet) — Plan 5 wires the AI mode behind it. Acceptable for Plan 2 because the AI route doesn't exist yet.
   - ZEN label is static text — Plan 6 makes it interactive.

3. **Type consistency**
   - Go `node.Node` JSON tags ↔ TS `NodeRow` (snake_case throughout). ✓
   - Go `snapshot.Snapshot` JSON tags ↔ TS `Snapshot`. ✓
   - RPC method names match in handlers, main.go registration, and rpc.ts (`nodes.get`, `nodes.update_content`, `nodes.set_last_opened`, `snapshots.create_manual`). ✓

4. **Cross-task dependencies**
   - Task 1 (CountChars) → Task 2 (UpdateContent uses it). ✓
   - Task 2 + 3 (repos) → Task 4 (handlers). ✓
   - Task 4 → Task 5 (main wiring). ✓
   - Tasks 6 + 7 + 8 → Task 9 (Workspace uses Tiptap + hooks + rpc). ✓
   - Task 5 + 9 → Task 10 (smoke). ✓

---

## Definition of Done

- `cd engine && go test ./...` is green.
- `cd apps/desktop && pnpm tsc -b && pnpm build` is green.
- `cd apps/desktop/src-tauri && cargo check` is green.
- Manual smoke in Task 10 walks: open project → type prose → see live word count → autosave fires → Cmd+S toasts → reload preserves body → Library card shows updated word count.
- Tag `plan-2-edit-done` exists.

When done, the next plan is **Plan 3 — Outline + Cmd+K + 트리 조작**.
