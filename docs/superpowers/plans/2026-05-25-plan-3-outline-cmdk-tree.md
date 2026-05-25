# Plan 3 — Outline panel + Cmd+K palette + tree operations

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn single-leaf workspaces into navigable multi-node trees. The user can: (a) reveal an outline by hovering the left edge, click any node to navigate to it; (b) press `⌘K` to open a command palette and run "새 씬 추가", "새 장 추가", "이름 바꾸기", "삭제", "이전/다음 씬", "이 씬 위로/아래로", "아웃라인 토글".

**Architecture:** The Go engine grows tree CRUD on `node.Repo` — `ListByProject`, `CreateSibling`, `CreateChild`, `Rename`, `Delete`, `MoveUp`/`MoveDown` — paired with matching JSONRPC handlers. The React side adds two new top-level components: `OutlinePanel` (left edge slide-in) and `CommandPalette` (centered modal with fuzzy filter and keyboard nav). The Workspace owns both panels' open/close state, listens for `⌘K` globally, and dispatches palette selections into navigation or RPC calls. Tree navigation uses a small `findFirstLeaf` helper that recurses into containers until it hits a leaf.

**Tech Stack additions:** none. Pure Go + React using already-installed deps.

**Spec reference:** `docs/superpowers/specs/2026-05-25-linetta-immersive-rebuild-design.md` — §4.6 (Command Palette), §4.7 (Outline Panel), §11.1 items 8–9.

---

## Pre-flight

- [ ] `git describe --tags --exact-match plan-2-edit-done` returns ok.
- [ ] `git status --short` is empty.
- [ ] `go test ./...` in `engine/` green; `pnpm tsc -b` and `cargo check` in `apps/desktop` green.

---

## File Structure (created or modified)

```
engine/internal/node/
  repo.go              (extended)
  repo_test.go         (extended)
engine/internal/rpc/handlers/
  nodes.go             (extended)
  nodes_test.go        (extended)
engine/cmd/linetta-engine/main.go  (extended — registers new handlers)

apps/desktop/src/
  lib/rpc.ts           (extended — nodes.* tree ops)
  lib/types.ts         (NodeRow already there)
  routes/Workspace.tsx (extended — wires outline + palette + first-leaf helper)
  components/
    OutlinePanel.tsx   (new)
    OutlinePanel.css   (new)
    CommandPalette.tsx (new)
    CommandPalette.css (new)
  hooks/
    useFirstLeaf.ts    (new — tree helpers)
  App.css              (APPEND — workspace tree styles)
```

---

## Task 1: node.Repo tree operations (TDD)

We extend `engine/internal/node/repo.go` with the six tree methods. Each test uses an in-memory store + a fresh project.

**Files:**
- Modify: `engine/internal/node/repo.go` (append methods)
- Modify: `engine/internal/node/repo_test.go` (append tests)

- [ ] **Step 1: Append tests**

Add the following test functions to `engine/internal/node/repo_test.go` (do not replace existing tests):

```go
func TestRepo_ListByProject_returnsTreeOrdered(t *testing.T) {
	s, p := newStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()

	// Project starts with one leaf ("씬 1"). Add a container sibling and a leaf inside it.
	chapter, err := r.CreateSibling(ctx, *p.LastOpenedNodeID, "container", "1장", "", 2000)
	if err != nil {
		t.Fatalf("CreateSibling chapter: %v", err)
	}
	if _, err := r.CreateChild(ctx, chapter.ID, "leaf", "씬 A", "", 3000); err != nil {
		t.Fatalf("CreateChild leaf: %v", err)
	}

	list, err := r.ListByProject(ctx, p.ID)
	if err != nil {
		t.Fatalf("ListByProject: %v", err)
	}
	if got, want := len(list), 3; got != want {
		t.Fatalf("len = %d, want %d", got, want)
	}
	// First two rows: project-root siblings ordered by ordinal.
	if list[0].Label != "씬 1" || list[1].Label != "1장" {
		t.Errorf("root order = %q, %q; want 씬 1, 1장", list[0].Label, list[1].Label)
	}
	// Third row: leaf inside chapter.
	if list[2].ParentID == nil || *list[2].ParentID != chapter.ID {
		t.Errorf("third row not under chapter")
	}
}

func TestRepo_CreateSibling_placesAfterReference(t *testing.T) {
	s, p := newStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()

	created, err := r.CreateSibling(ctx, *p.LastOpenedNodeID, "leaf", "씬 2", "", 2000)
	if err != nil {
		t.Fatalf("CreateSibling: %v", err)
	}
	if created.Ordinal != 1 {
		t.Errorf("ordinal = %d, want 1 (after 씬 1)", created.Ordinal)
	}
	if created.ParentID != nil {
		t.Errorf("expected top-level node, got parent_id = %v", created.ParentID)
	}
}

func TestRepo_CreateChild_lastOrdinal(t *testing.T) {
	s, p := newStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()

	chapter, _ := r.CreateSibling(ctx, *p.LastOpenedNodeID, "container", "1장", "", 2000)
	first, err := r.CreateChild(ctx, chapter.ID, "leaf", "씬 A", "", 3000)
	if err != nil {
		t.Fatalf("CreateChild: %v", err)
	}
	if first.Ordinal != 0 {
		t.Errorf("first child ordinal = %d, want 0", first.Ordinal)
	}
	second, _ := r.CreateChild(ctx, chapter.ID, "leaf", "씬 B", "", 4000)
	if second.Ordinal != 1 {
		t.Errorf("second child ordinal = %d, want 1", second.Ordinal)
	}
}

func TestRepo_Rename_updatesLabelAndTitle(t *testing.T) {
	s, p := newStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()

	if err := r.Rename(ctx, *p.LastOpenedNodeID, "프롤로그", "별이 떨어지는 밤", 5000); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	got, _ := r.Get(ctx, *p.LastOpenedNodeID)
	if got.Label != "프롤로그" {
		t.Errorf("label = %q", got.Label)
	}
	if got.Title != "별이 떨어지는 밤" {
		t.Errorf("title = %q", got.Title)
	}
	if got.UpdatedAt != 5000 {
		t.Errorf("updated_at = %d", got.UpdatedAt)
	}
}

func TestRepo_Delete_removesNode_andUpdatesProjectWordCount(t *testing.T) {
	s, p := newStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()

	// Write some content so word_count is non-zero, then delete.
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"세 글자"}]}]}`
	if err := r.UpdateContent(ctx, *p.LastOpenedNodeID, doc, 2000); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}

	if err := r.Delete(ctx, *p.LastOpenedNodeID, 3000); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := r.Get(ctx, *p.LastOpenedNodeID); err != ErrNotFound {
		t.Errorf("Get after Delete: err = %v, want ErrNotFound", err)
	}

	// Project word_count should reflow to 0.
	var pcount int
	if err := s.DB().QueryRowContext(ctx, `SELECT word_count FROM projects WHERE id = ?`, p.ID).Scan(&pcount); err != nil {
		t.Fatalf("project count: %v", err)
	}
	if pcount != 0 {
		t.Errorf("project.word_count = %d, want 0", pcount)
	}
}

func TestRepo_MoveUp_andMoveDown(t *testing.T) {
	s, p := newStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()

	first := *p.LastOpenedNodeID                                                          // ordinal 0
	second, _ := r.CreateSibling(ctx, first, "leaf", "씬 2", "", 2000)                    // ordinal 1
	third, _ := r.CreateSibling(ctx, second.ID, "leaf", "씬 3", "", 3000)                 // ordinal 2

	// Move third up → should swap with second.
	if err := r.MoveUp(ctx, third.ID, 4000); err != nil {
		t.Fatalf("MoveUp: %v", err)
	}
	list, _ := r.ListByProject(ctx, p.ID)
	if list[0].Label != "씬 1" || list[1].Label != "씬 3" || list[2].Label != "씬 2" {
		t.Errorf("after MoveUp(third) = %q,%q,%q; want 씬 1, 씬 3, 씬 2",
			list[0].Label, list[1].Label, list[2].Label)
	}

	// Move first down → swap with what's now in slot 1 (씬 3).
	if err := r.MoveDown(ctx, first, 5000); err != nil {
		t.Fatalf("MoveDown: %v", err)
	}
	list, _ = r.ListByProject(ctx, p.ID)
	if list[0].Label != "씬 3" || list[1].Label != "씬 1" {
		t.Errorf("after MoveDown(first) = %q,%q,...", list[0].Label, list[1].Label)
	}

	// MoveUp on the first-position node is a no-op (no error).
	if err := r.MoveUp(ctx, list[0].ID, 6000); err != nil {
		t.Errorf("MoveUp on first: %v", err)
	}
}
```

- [ ] **Step 2: Run — expect compile failures (undefined methods)**

```bash
cd engine && go test ./internal/node/...
```

- [ ] **Step 3: Append implementation to `repo.go`**

Append the following to `engine/internal/node/repo.go`:

```go
import "github.com/google/uuid"  // add this to the existing import block if not already there
```

(If the existing imports already list `github.com/google/uuid`, skip.) Then add at the end of the file:

```go
// ListByProject returns every node belonging to the project, sorted by
// (parent_id NULLS FIRST, ordinal). Callers build the tree in memory.
func (r *Repo) ListByProject(ctx context.Context, projectID string) ([]Node, error) {
	rows, err := r.s.DB().QueryContext(ctx, baseSelect+`
WHERE project_id = ?
ORDER BY (parent_id IS NULL) DESC, parent_id, ordinal`, projectID)
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

// CreateSibling inserts a new node after referenceID at the same parent + level.
// The new ordinal is referenceOrdinal + 1; existing siblings at >= that ordinal
// are shifted forward by 1.
func (r *Repo) CreateSibling(ctx context.Context, referenceID, kind, label, title string, now int64) (Node, error) {
	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return Node{}, err
	}
	defer tx.Rollback()

	var (
		ref      Node
		parentID *string
		refOrd   int
	)
	row := tx.QueryRowContext(ctx, baseSelect+` WHERE id = ?`, referenceID)
	ref, err = scan(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return Node{}, ErrNotFound
		}
		return Node{}, err
	}
	parentID = ref.ParentID
	refOrd = ref.Ordinal

	// Shift downstream siblings.
	if parentID == nil {
		if _, err := tx.ExecContext(ctx, `
UPDATE nodes SET ordinal = ordinal + 1
 WHERE project_id = ? AND parent_id IS NULL AND ordinal > ?`,
			ref.ProjectID, refOrd); err != nil {
			return Node{}, err
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
UPDATE nodes SET ordinal = ordinal + 1
 WHERE parent_id = ? AND ordinal > ?`, *parentID, refOrd); err != nil {
			return Node{}, err
		}
	}

	newID := uuid.NewString()
	var contentDoc any
	if kind == "leaf" {
		contentDoc = `{"type":"doc","content":[{"type":"paragraph"}]}`
	} else {
		contentDoc = nil
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO nodes (id, project_id, parent_id, ordinal, kind, label, title,
                   content_doc, status, word_count, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'draft', 0, ?, ?)`,
		newID, ref.ProjectID, parentID, refOrd+1, kind, label, title, contentDoc, now, now); err != nil {
		return Node{}, err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE projects SET updated_at = ? WHERE id = ?`, now, ref.ProjectID); err != nil {
		return Node{}, err
	}
	if err := tx.Commit(); err != nil {
		return Node{}, err
	}
	return r.Get(ctx, newID)
}

// CreateChild inserts a new node as the last child of parentID.
func (r *Repo) CreateChild(ctx context.Context, parentID, kind, label, title string, now int64) (Node, error) {
	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return Node{}, err
	}
	defer tx.Rollback()

	row := tx.QueryRowContext(ctx, baseSelect+` WHERE id = ?`, parentID)
	parent, err := scan(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return Node{}, ErrNotFound
		}
		return Node{}, err
	}

	var maxOrd sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MAX(ordinal) FROM nodes WHERE parent_id = ?`, parentID).Scan(&maxOrd); err != nil {
		return Node{}, err
	}
	nextOrd := 0
	if maxOrd.Valid {
		nextOrd = int(maxOrd.Int64) + 1
	}

	newID := uuid.NewString()
	var contentDoc any
	if kind == "leaf" {
		contentDoc = `{"type":"doc","content":[{"type":"paragraph"}]}`
	} else {
		contentDoc = nil
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO nodes (id, project_id, parent_id, ordinal, kind, label, title,
                   content_doc, status, word_count, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'draft', 0, ?, ?)`,
		newID, parent.ProjectID, parentID, nextOrd, kind, label, title, contentDoc, now, now); err != nil {
		return Node{}, err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE projects SET updated_at = ? WHERE id = ?`, now, parent.ProjectID); err != nil {
		return Node{}, err
	}
	if err := tx.Commit(); err != nil {
		return Node{}, err
	}
	return r.Get(ctx, newID)
}

// Rename updates label and title (both can be empty strings to clear).
func (r *Repo) Rename(ctx context.Context, id, label, title string, now int64) error {
	res, err := r.s.DB().ExecContext(ctx, `
UPDATE nodes SET label = ?, title = ?, updated_at = ?
 WHERE id = ?`, label, title, now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes the node (children cascade via FK). Recomputes the project's
// word_count.
func (r *Repo) Delete(ctx context.Context, id string, now int64) error {
	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var projectID string
	if err := tx.QueryRowContext(ctx, `SELECT project_id FROM nodes WHERE id = ?`, id).Scan(&projectID); err != nil {
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM nodes WHERE id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE projects
   SET word_count = COALESCE((SELECT SUM(word_count) FROM nodes WHERE project_id = ? AND kind = 'leaf'), 0),
       updated_at  = ?
 WHERE id = ?`, projectID, now, projectID); err != nil {
		return err
	}
	return tx.Commit()
}

// MoveUp swaps the node with its previous sibling (same parent_id). No-op if
// the node is already first.
func (r *Repo) MoveUp(ctx context.Context, id string, now int64) error {
	return r.swap(ctx, id, "up", now)
}

// MoveDown swaps the node with its next sibling. No-op if last.
func (r *Repo) MoveDown(ctx context.Context, id string, now int64) error {
	return r.swap(ctx, id, "down", now)
}

func (r *Repo) swap(ctx context.Context, id, direction string, now int64) error {
	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	row := tx.QueryRowContext(ctx, baseSelect+` WHERE id = ?`, id)
	cur, err := scan(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		return err
	}

	var (
		neighborID  string
		neighborOrd int
	)
	var query string
	if direction == "up" {
		if cur.ParentID == nil {
			query = `SELECT id, ordinal FROM nodes
 WHERE project_id = ? AND parent_id IS NULL AND ordinal < ?
 ORDER BY ordinal DESC LIMIT 1`
		} else {
			query = `SELECT id, ordinal FROM nodes
 WHERE parent_id = ? AND ordinal < ?
 ORDER BY ordinal DESC LIMIT 1`
		}
	} else {
		if cur.ParentID == nil {
			query = `SELECT id, ordinal FROM nodes
 WHERE project_id = ? AND parent_id IS NULL AND ordinal > ?
 ORDER BY ordinal ASC LIMIT 1`
		} else {
			query = `SELECT id, ordinal FROM nodes
 WHERE parent_id = ? AND ordinal > ?
 ORDER BY ordinal ASC LIMIT 1`
		}
	}
	scope := any(cur.ProjectID)
	if cur.ParentID != nil {
		scope = *cur.ParentID
	}
	err = tx.QueryRowContext(ctx, query, scope, cur.Ordinal).Scan(&neighborID, &neighborOrd)
	if err == sql.ErrNoRows {
		// No neighbor — no-op.
		return tx.Commit()
	}
	if err != nil {
		return err
	}

	// Two-step swap (avoid unique constraint conflicts if any — schema has none here
	// but this is the safe pattern).
	tmp := -1 - cur.Ordinal
	if _, err := tx.ExecContext(ctx, `UPDATE nodes SET ordinal = ?, updated_at = ? WHERE id = ?`, tmp, now, cur.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE nodes SET ordinal = ?, updated_at = ? WHERE id = ?`, cur.Ordinal, now, neighborID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE nodes SET ordinal = ?, updated_at = ? WHERE id = ?`, neighborOrd, now, cur.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE projects SET updated_at = ? WHERE id = ?`, now, cur.ProjectID); err != nil {
		return err
	}
	return tx.Commit()
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
cd engine && go test ./internal/node/... -v
```

Expected: all 6 new tests + existing tests PASS.

- [ ] **Step 5: Commit**

```bash
git add engine/internal/node/repo.go engine/internal/node/repo_test.go
git commit -m "feat(node): tree ops (List, CreateSibling, CreateChild, Rename, Delete, Move)"
```

---

## Task 2: Tree RPC handlers (TDD)

Add seven handlers: `nodes.list_tree`, `nodes.create_sibling`, `nodes.create_child`, `nodes.rename`, `nodes.delete`, `nodes.move_up`, `nodes.move_down`.

**Files:**
- Modify: `engine/internal/rpc/handlers/nodes.go` (append)
- Modify: `engine/internal/rpc/handlers/nodes_test.go` (append)

- [ ] **Step 1: Append tests**

Add to `nodes_test.go`:

```go
func TestListTreeHandler(t *testing.T) {
	f := newNodeFixture(t)
	chapter, _ := f.nodes.CreateSibling(context.Background(), f.nID, "container", "1장", "", 2000)
	_, _ = f.nodes.CreateChild(context.Background(), chapter.ID, "leaf", "씬 A", "", 3000)

	h := ListTree(f.nodes)
	res, err := h(context.Background(), json.RawMessage(`{"project_id":"`+f.pID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got []node.Node
	if err := json.Unmarshal(res, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
}

func TestCreateSiblingHandler(t *testing.T) {
	f := newNodeFixture(t)
	h := CreateSibling(f.nodes, func() int64 { return 1234 })
	params := json.RawMessage(`{"reference_id":"` + f.nID + `","kind":"leaf","label":"씬 2","title":""}`)
	res, err := h(context.Background(), params)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var n node.Node
	_ = json.Unmarshal(res, &n)
	if n.Label != "씬 2" || n.Kind != "leaf" {
		t.Errorf("got %+v", n)
	}
	if n.CreatedAt != 1234 {
		t.Errorf("clock not injected: %d", n.CreatedAt)
	}
}

func TestCreateChildHandler(t *testing.T) {
	f := newNodeFixture(t)
	chapter, _ := f.nodes.CreateSibling(context.Background(), f.nID, "container", "1장", "", 2000)
	h := CreateChild(f.nodes, func() int64 { return 5000 })
	params := json.RawMessage(`{"parent_id":"` + chapter.ID + `","kind":"leaf","label":"씬 A","title":""}`)
	res, err := h(context.Background(), params)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var n node.Node
	_ = json.Unmarshal(res, &n)
	if n.ParentID == nil || *n.ParentID != chapter.ID {
		t.Errorf("parent mismatch: %v", n.ParentID)
	}
}

func TestRenameHandler(t *testing.T) {
	f := newNodeFixture(t)
	h := RenameNode(f.nodes, func() int64 { return 9999 })
	if _, err := h(context.Background(), json.RawMessage(`{"id":"`+f.nID+`","label":"프롤로그","title":"별이 떨어지는 밤"}`)); err != nil {
		t.Fatalf("handler: %v", err)
	}
	got, _ := f.nodes.Get(context.Background(), f.nID)
	if got.Label != "프롤로그" || got.Title != "별이 떨어지는 밤" {
		t.Errorf("rename failed: %+v", got)
	}
}

func TestDeleteHandler(t *testing.T) {
	f := newNodeFixture(t)
	// Create a second leaf so the project still has a node after the delete.
	other, _ := f.nodes.CreateSibling(context.Background(), f.nID, "leaf", "씬 2", "", 2000)
	_ = other

	h := DeleteNode(f.nodes, func() int64 { return 1 })
	if _, err := h(context.Background(), json.RawMessage(`{"id":"`+f.nID+`"}`)); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if _, err := f.nodes.Get(context.Background(), f.nID); err != node.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestMoveHandlers(t *testing.T) {
	f := newNodeFixture(t)
	second, _ := f.nodes.CreateSibling(context.Background(), f.nID, "leaf", "씬 2", "", 2000)

	up := MoveUp(f.nodes, func() int64 { return 3000 })
	if _, err := up(context.Background(), json.RawMessage(`{"id":"`+second.ID+`"}`)); err != nil {
		t.Fatalf("MoveUp handler: %v", err)
	}
	tree, _ := f.nodes.ListByProject(context.Background(), f.pID)
	if tree[0].Label != "씬 2" || tree[1].Label != "씬 1" {
		t.Errorf("order after MoveUp: %q,%q", tree[0].Label, tree[1].Label)
	}

	down := MoveDown(f.nodes, func() int64 { return 4000 })
	if _, err := down(context.Background(), json.RawMessage(`{"id":"`+second.ID+`"}`)); err != nil {
		t.Fatalf("MoveDown handler: %v", err)
	}
	tree, _ = f.nodes.ListByProject(context.Background(), f.pID)
	if tree[0].Label != "씬 1" || tree[1].Label != "씬 2" {
		t.Errorf("order after MoveDown: %q,%q", tree[0].Label, tree[1].Label)
	}
}
```

- [ ] **Step 2: Run — expect failures**

```bash
cd engine && go test ./internal/rpc/handlers/...
```

- [ ] **Step 3: Append handlers to `nodes.go`**

Add at the end of `engine/internal/rpc/handlers/nodes.go`:

```go
// --- Tree ops ---

type projectIDParam struct {
	ProjectID string `json:"project_id"`
}

func ListTree(nodes *node.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p projectIDParam
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id required"}
		}
		list, err := nodes.ListByProject(ctx, p.ProjectID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if list == nil {
			list = []node.Node{}
		}
		return json.Marshal(list)
	}
}

type createSiblingParams struct {
	ReferenceID string `json:"reference_id"`
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	Title       string `json:"title"`
}

func CreateSibling(nodes *node.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p createSiblingParams
		if err := json.Unmarshal(params, &p); err != nil || p.ReferenceID == "" || p.Kind == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "reference_id and kind required"}
		}
		n, err := nodes.CreateSibling(ctx, p.ReferenceID, p.Kind, p.Label, p.Title, now())
		if err != nil {
			if errors.Is(err, node.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "reference not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(n)
	}
}

type createChildParams struct {
	ParentID string `json:"parent_id"`
	Kind     string `json:"kind"`
	Label    string `json:"label"`
	Title    string `json:"title"`
}

func CreateChild(nodes *node.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p createChildParams
		if err := json.Unmarshal(params, &p); err != nil || p.ParentID == "" || p.Kind == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "parent_id and kind required"}
		}
		n, err := nodes.CreateChild(ctx, p.ParentID, p.Kind, p.Label, p.Title, now())
		if err != nil {
			if errors.Is(err, node.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "parent not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(n)
	}
}

type renameParams struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Title string `json:"title"`
}

func RenameNode(nodes *node.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p renameParams
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := nodes.Rename(ctx, p.ID, p.Label, p.Title, now()); err != nil {
			if errors.Is(err, node.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}

func DeleteNode(nodes *node.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := nodes.Delete(ctx, p.ID, now()); err != nil {
			if errors.Is(err, node.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}

func MoveUp(nodes *node.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := nodes.MoveUp(ctx, p.ID, now()); err != nil {
			if errors.Is(err, node.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}

func MoveDown(nodes *node.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := nodes.MoveDown(ctx, p.ID, now()); err != nil {
			if errors.Is(err, node.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
cd engine && go test ./...
```

- [ ] **Step 5: Commit**

```bash
git add engine/internal/rpc/handlers/nodes.go engine/internal/rpc/handlers/nodes_test.go
git commit -m "feat(rpc): tree handlers (list/create/rename/delete/move)"
```

---

## Task 3: Wire main.go + smoke

**Files:**
- Modify: `engine/cmd/linetta-engine/main.go` (append handler registrations)

- [ ] **Step 1: Add the 7 new registrations**

In `main.go`, find the block of `s.Handle(...)` calls. Add after the existing `snapshots.create_manual` line:

```go
	s.Handle("nodes.list_tree", handlers.ListTree(nodes))
	s.Handle("nodes.create_sibling", handlers.CreateSibling(nodes, clock))
	s.Handle("nodes.create_child", handlers.CreateChild(nodes, clock))
	s.Handle("nodes.rename", handlers.RenameNode(nodes, clock))
	s.Handle("nodes.delete", handlers.DeleteNode(nodes, clock))
	s.Handle("nodes.move_up", handlers.MoveUp(nodes, clock))
	s.Handle("nodes.move_down", handlers.MoveDown(nodes, clock))
```

- [ ] **Step 2: Build + stdio smoke**

```bash
cd engine && go build -o /tmp/linetta-engine-build ./cmd/linetta-engine
rm -rf /tmp/linetta-plan3-smoke
LINETTA_HOME=/tmp/linetta-plan3-smoke /tmp/linetta-engine-build --stdio <<'EOF'
{"jsonrpc":"2.0","id":1,"method":"projects.create","params":{"title":"Tree","genres":["SF"],"length_target":"short","default_pov":"first"}}
EOF
```

Note the `last_opened_node_id` UUID. Then exercise the tree:

```bash
NID=<paste the UUID>
PID=<paste the project id>
LINETTA_HOME=/tmp/linetta-plan3-smoke /tmp/linetta-engine-build --stdio <<EOF
{"jsonrpc":"2.0","id":2,"method":"nodes.create_sibling","params":{"reference_id":"$NID","kind":"leaf","label":"씬 2","title":""}}
{"jsonrpc":"2.0","id":3,"method":"nodes.list_tree","params":{"project_id":"$PID"}}
EOF
rm -f /tmp/linetta-engine-build
rm -rf /tmp/linetta-plan3-smoke
```

Expected: `list_tree` returns an array with two leaves.

- [ ] **Step 3: Commit + rebuild dev binary**

```bash
git add engine/cmd/linetta-engine/main.go
git commit -m "feat(engine): register tree handlers"
./scripts/build-engine.sh
```

---

## Task 4: TS API additions

**Files:**
- Modify: `apps/desktop/src/lib/rpc.ts` (extend `nodes` namespace)

- [ ] **Step 1: Replace the `nodes` block**

Replace the existing `nodes = { ... }` export in `apps/desktop/src/lib/rpc.ts` with:

```ts
export const nodes = {
  get: (id: string) => rpcCall<NodeRow>("nodes.get", { id }),
  updateContent: (id: string, doc: string) =>
    rpcCall<NodeRow>("nodes.update_content", { id, doc }),
  setLastOpened: (projectId: string, nodeId: string) =>
    rpcCall<{ ok: true }>("nodes.set_last_opened", { project_id: projectId, node_id: nodeId }),
  listTree: (projectId: string) =>
    rpcCall<NodeRow[]>("nodes.list_tree", { project_id: projectId }),
  createSibling: (referenceId: string, kind: "leaf" | "container", label: string, title: string) =>
    rpcCall<NodeRow>("nodes.create_sibling", { reference_id: referenceId, kind, label, title }),
  createChild: (parentId: string, kind: "leaf" | "container", label: string, title: string) =>
    rpcCall<NodeRow>("nodes.create_child", { parent_id: parentId, kind, label, title }),
  rename: (id: string, label: string, title: string) =>
    rpcCall<{ ok: true }>("nodes.rename", { id, label, title }),
  delete: (id: string) => rpcCall<{ ok: true }>("nodes.delete", { id }),
  moveUp: (id: string) => rpcCall<{ ok: true }>("nodes.move_up", { id }),
  moveDown: (id: string) => rpcCall<{ ok: true }>("nodes.move_down", { id }),
};
```

- [ ] **Step 2: Type-check + commit**

```bash
cd apps/desktop && pnpm tsc -b && cd ../..
git add apps/desktop/src/lib/rpc.ts
git commit -m "feat(rpc): tree ops in nodes client"
```

---

## Task 5: Tree helpers + first-leaf hook

**Files:**
- Create: `apps/desktop/src/hooks/useFirstLeaf.ts`

- [ ] **Step 1: Write the helper**

```ts
import type { NodeRow } from "../lib/types";

export interface TreeNode extends NodeRow {
  children: TreeNode[];
}

/** Build a tree (parent_id NULL roots + recursive children) from a flat list.
 *  Caller must have sorted by (parent_id, ordinal) — `nodes.list_tree` already does. */
export function buildTree(rows: NodeRow[]): TreeNode[] {
  const byId = new Map<string, TreeNode>();
  for (const r of rows) byId.set(r.id, { ...r, children: [] });
  const roots: TreeNode[] = [];
  for (const r of rows) {
    const node = byId.get(r.id)!;
    if (!r.parent_id) {
      roots.push(node);
    } else {
      byId.get(r.parent_id)?.children.push(node);
    }
  }
  return roots;
}

/** Recurse into a node and return the first leaf descendant (DFS, ordinal order).
 *  Returns the node itself if it is already a leaf. Returns null if no leaf exists. */
export function findFirstLeaf(root: TreeNode): TreeNode | null {
  if (root.kind === "leaf") return root;
  for (const c of root.children) {
    const found = findFirstLeaf(c);
    if (found) return found;
  }
  return null;
}

/** Flatten a tree to a list in DFS order (used by Cmd+K's "search node"). */
export function flatten(roots: TreeNode[]): TreeNode[] {
  const out: TreeNode[] = [];
  const walk = (n: TreeNode) => {
    out.push(n);
    n.children.forEach(walk);
  };
  roots.forEach(walk);
  return out;
}

/** Return [prevLeaf, nextLeaf] relative to currentId in DFS leaf order. */
export function leafNeighbors(roots: TreeNode[], currentId: string): { prev: TreeNode | null; next: TreeNode | null } {
  const leaves: TreeNode[] = [];
  const walk = (n: TreeNode) => {
    if (n.kind === "leaf") leaves.push(n);
    n.children.forEach(walk);
  };
  roots.forEach(walk);
  const idx = leaves.findIndex((l) => l.id === currentId);
  if (idx === -1) return { prev: null, next: null };
  return { prev: idx > 0 ? leaves[idx - 1] : null, next: idx < leaves.length - 1 ? leaves[idx + 1] : null };
}
```

- [ ] **Step 2: Commit (type-check covered by Task 6)**

```bash
git add apps/desktop/src/hooks/useFirstLeaf.ts
git commit -m "feat(workspace): tree helpers — buildTree, findFirstLeaf, leafNeighbors"
```

---

## Task 6: OutlinePanel component

**Files:**
- Create: `apps/desktop/src/components/OutlinePanel.tsx`
- Create: `apps/desktop/src/components/OutlinePanel.css`

- [ ] **Step 1: Write `OutlinePanel.tsx`**

```tsx
import { useEffect, useRef, useState } from "react";
import type { TreeNode } from "../hooks/useFirstLeaf";
import "./OutlinePanel.css";

interface Props {
  tree: TreeNode[];
  currentId: string;
  onSelect: (node: TreeNode) => void;
}

const HOT_ZONE_PX = 16;
const RETRACT_AFTER_MS = 3000;

export function OutlinePanel({ tree, currentId, onSelect }: Props) {
  const [open, setOpen] = useState(false);
  const retractTimer = useRef<number | undefined>(undefined);

  useEffect(() => {
    const onMove = (e: MouseEvent) => {
      if (e.clientX <= HOT_ZONE_PX) {
        setOpen(true);
        if (retractTimer.current) {
          window.clearTimeout(retractTimer.current);
          retractTimer.current = undefined;
        }
      }
    };
    window.addEventListener("mousemove", onMove);
    return () => window.removeEventListener("mousemove", onMove);
  }, []);

  const handleMouseLeave = () => {
    if (retractTimer.current) window.clearTimeout(retractTimer.current);
    retractTimer.current = window.setTimeout(() => setOpen(false), RETRACT_AFTER_MS);
  };

  const handleMouseEnter = () => {
    if (retractTimer.current) {
      window.clearTimeout(retractTimer.current);
      retractTimer.current = undefined;
    }
  };

  return (
    <>
      <div className="outline-hot-zone" aria-hidden />
      <aside
        className={`outline-panel${open ? " open" : ""}`}
        onMouseEnter={handleMouseEnter}
        onMouseLeave={handleMouseLeave}
      >
        <header className="outline-head">아웃라인</header>
        <ul className="outline-tree">
          {tree.map((root) => (
            <OutlineRow key={root.id} node={root} depth={0} currentId={currentId} onSelect={onSelect} />
          ))}
        </ul>
      </aside>
    </>
  );
}

function OutlineRow({
  node,
  depth,
  currentId,
  onSelect,
}: {
  node: TreeNode;
  depth: number;
  currentId: string;
  onSelect: (n: TreeNode) => void;
}) {
  const active = node.id === currentId;
  return (
    <>
      <li
        className={`outline-row${active ? " active" : ""}${node.kind === "container" ? " container" : ""}`}
        style={{ paddingLeft: 0.75 + depth * 1.1 + "rem" }}
        onClick={() => onSelect(node)}
        role="button"
      >
        <span className="outline-label">{node.label}</span>
        {node.title && <span className="outline-title">. {node.title}</span>}
      </li>
      {node.children.map((c) => (
        <OutlineRow key={c.id} node={c} depth={depth + 1} currentId={currentId} onSelect={onSelect} />
      ))}
    </>
  );
}
```

- [ ] **Step 2: Write `OutlinePanel.css`**

```css
.outline-hot-zone {
  position: fixed;
  top: 50px;            /* below workspace top bar */
  left: 0;
  width: 16px;
  bottom: 0;
  z-index: 4;           /* below panel */
}

.outline-panel {
  position: fixed;
  top: 50px;
  left: 0;
  bottom: 0;
  width: 260px;
  background: #fbf9f3;
  border-right: 1px solid #ece9e0;
  transform: translateX(-100%);
  transition: transform 200ms ease;
  z-index: 6;
  overflow-y: auto;
}

.outline-panel.open {
  transform: translateX(0);
  box-shadow: 4px 0 18px rgba(0, 0, 0, 0.08);
}

.outline-head {
  padding: 0.75rem 1rem 0.5rem;
  font-size: 0.75rem;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: #6b6b6b;
}

.outline-tree {
  list-style: none;
  margin: 0;
  padding: 0;
}

.outline-row {
  padding: 0.35rem 1rem 0.35rem 0.75rem;
  cursor: pointer;
  font-size: 0.92rem;
  line-height: 1.4;
}

.outline-row:hover {
  background: #f1eee5;
}

.outline-row.active {
  background: #e7e2d3;
  font-weight: 500;
}

.outline-row.container {
  color: #4a4a4a;
}

.outline-title {
  color: #777;
  margin-left: 0.3rem;
}
```

- [ ] **Step 3: Commit (full type-check happens in Task 8)**

```bash
git add apps/desktop/src/components/OutlinePanel.tsx apps/desktop/src/components/OutlinePanel.css
git commit -m "feat(outline): slide-in panel with tree, hover-open + 3s auto-retract"
```

---

## Task 7: CommandPalette component

**Files:**
- Create: `apps/desktop/src/components/CommandPalette.tsx`
- Create: `apps/desktop/src/components/CommandPalette.css`

The palette is a controlled modal: the parent owns `open` and the command list. Filter is local. Keyboard nav (↑/↓/Enter/Esc) lives inside.

- [ ] **Step 1: Write `CommandPalette.tsx`**

```tsx
import { useEffect, useMemo, useRef, useState } from "react";
import "./CommandPalette.css";

export interface Command {
  id: string;
  section: string;
  label: string;
  hint?: string;          // right-side text (shortcut, "(곧 추가됨)", etc.)
  disabled?: boolean;
  run: () => void | Promise<void>;
}

interface Props {
  open: boolean;
  onClose: () => void;
  commands: Command[];
}

export function CommandPalette({ open, onClose, commands }: Props) {
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (open) {
      setQuery("");
      setActive(0);
      window.setTimeout(() => inputRef.current?.focus(), 0);
    }
  }, [open]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return commands;
    return commands.filter((c) => c.label.toLowerCase().includes(q) || c.section.toLowerCase().includes(q));
  }, [commands, query]);

  useEffect(() => {
    if (active >= filtered.length) setActive(0);
  }, [filtered, active]);

  if (!open) return null;

  const runIndex = (i: number) => {
    const c = filtered[i];
    if (!c || c.disabled) return;
    onClose();
    Promise.resolve(c.run()).catch((e) => console.error("palette command failed:", e));
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Escape") {
      e.preventDefault();
      onClose();
    } else if (e.key === "ArrowDown") {
      e.preventDefault();
      setActive((i) => Math.min(filtered.length - 1, i + 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setActive((i) => Math.max(0, i - 1));
    } else if (e.key === "Enter") {
      e.preventDefault();
      runIndex(active);
    }
  };

  // Group filtered commands by section for display.
  const groups: { section: string; items: Command[] }[] = [];
  for (const c of filtered) {
    const last = groups[groups.length - 1];
    if (last && last.section === c.section) {
      last.items.push(c);
    } else {
      groups.push({ section: c.section, items: [c] });
    }
  }

  let globalIdx = -1;

  return (
    <div className="palette-backdrop" onClick={onClose}>
      <div className="palette" onClick={(e) => e.stopPropagation()} onKeyDown={onKeyDown}>
        <input
          ref={inputRef}
          className="palette-input"
          placeholder="검색…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <div ref={listRef} className="palette-list">
          {groups.length === 0 && <p className="palette-empty">결과 없음</p>}
          {groups.map((g) => (
            <div key={g.section} className="palette-group">
              <p className="palette-section">{g.section}</p>
              {g.items.map((c) => {
                globalIdx++;
                const isActive = globalIdx === active;
                const idx = globalIdx;
                return (
                  <button
                    key={c.id}
                    className={`palette-row${isActive ? " active" : ""}${c.disabled ? " disabled" : ""}`}
                    onMouseMove={() => setActive(idx)}
                    onClick={() => runIndex(idx)}
                    disabled={c.disabled}
                  >
                    <span className="palette-label">{c.label}</span>
                    {c.hint && <span className="palette-hint">{c.hint}</span>}
                  </button>
                );
              })}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Write `CommandPalette.css`**

```css
.palette-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(20, 20, 20, 0.32);
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding-top: 18vh;
  z-index: 20;
}

.palette {
  background: #faf9f6;
  border-radius: 8px;
  width: min(560px, 92vw);
  box-shadow: 0 18px 50px rgba(0, 0, 0, 0.22);
  display: flex;
  flex-direction: column;
  max-height: 60vh;
  overflow: hidden;
}

.palette-input {
  font: inherit;
  font-size: 1rem;
  padding: 0.85rem 1rem;
  border: none;
  border-bottom: 1px solid #ece9e0;
  background: transparent;
  outline: none;
}

.palette-list {
  overflow-y: auto;
  padding: 0.25rem 0;
}

.palette-empty {
  padding: 1rem;
  margin: 0;
  color: #6b6b6b;
  font-size: 0.9rem;
}

.palette-group {
  padding: 0.25rem 0 0.5rem;
}

.palette-section {
  margin: 0;
  padding: 0.35rem 1rem 0.2rem;
  font-size: 0.7rem;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: #9a9a9a;
}

.palette-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  text-align: left;
  border: none;
  background: transparent;
  font: inherit;
  padding: 0.5rem 1rem;
  cursor: pointer;
}

.palette-row.active {
  background: #efece2;
}

.palette-row.disabled {
  cursor: not-allowed;
  color: #b8b6ad;
}

.palette-label {
  font-size: 0.92rem;
}

.palette-hint {
  font-size: 0.78rem;
  color: #9a9a9a;
}
```

- [ ] **Step 3: Commit**

```bash
git add apps/desktop/src/components/CommandPalette.tsx apps/desktop/src/components/CommandPalette.css
git commit -m "feat(palette): Cmd+K command palette component"
```

---

## Task 8: Wire Outline + Palette into Workspace + add styles

**Files:**
- Modify: `apps/desktop/src/routes/Workspace.tsx`
- Modify: `apps/desktop/src/App.css` (APPEND palette/outline tweaks if needed)

The Workspace now: loads the full tree, mounts the OutlinePanel, listens for `⌘K` globally, builds a `Command[]` array from the current tree + active node, handles palette selection (navigation or RPC + reload). Renaming uses a `window.prompt` (low fidelity but unblocks the MVP).

- [ ] **Step 1: Replace `apps/desktop/src/routes/Workspace.tsx`**

```tsx
import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { nodes, projects, snapshots } from "../lib/rpc";
import type { NodeRow, Project } from "../lib/types";
import { TiptapEditor } from "../components/editor/Tiptap";
import { ContextPanel, type SaveStatus } from "../components/ContextPanel";
import { OutlinePanel } from "../components/OutlinePanel";
import { CommandPalette, type Command } from "../components/CommandPalette";
import { useDebouncedCallback } from "../hooks/useDebouncedCallback";
import { useThrottledCallback } from "../hooks/useThrottledCallback";
import {
  buildTree,
  findFirstLeaf,
  flatten,
  leafNeighbors,
  type TreeNode,
} from "../hooks/useFirstLeaf";

const SAVE_DEBOUNCE_MS = 800;
const LAST_OPENED_THROTTLE_MS = 5000;

interface LoadState {
  project: Project;
  node: NodeRow;
  initialDoc: object;
  tree: TreeNode[];
}

export function Workspace() {
  const { projectId } = useParams();
  const navigate = useNavigate();
  const [load, setLoad] = useState<LoadState | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [toast, setToast] = useState<string | null>(null);
  const [charCount, setCharCount] = useState(0);
  const [typewriter, setTypewriter] = useState(false);
  const [saveStatus, setSaveStatus] = useState<SaveStatus>({ kind: "idle" });
  const [paletteOpen, setPaletteOpen] = useState(false);

  const showToast = (msg: string) => {
    setToast(msg);
    window.setTimeout(() => setToast(null), 1800);
  };

  const fetchTree = useCallback(
    async (pId: string, nId: string): Promise<LoadState> => {
      const p = await projects.get(pId);
      const flat = await nodes.listTree(pId);
      const tree = buildTree(flat);
      const node = flat.find((x) => x.id === nId);
      if (!node) {
        // Current node no longer exists — fall back to first leaf.
        const firstLeaf = tree.length > 0 ? findFirstLeaf(tree[0]) : null;
        if (!firstLeaf) throw new Error("project has no leaf");
        const n = await nodes.get(firstLeaf.id);
        const initialDoc = JSON.parse(n.content_doc ?? `{"type":"doc","content":[{"type":"paragraph"}]}`);
        return { project: p, node: n, initialDoc, tree };
      }
      const initialDoc = JSON.parse(node.content_doc ?? `{"type":"doc","content":[{"type":"paragraph"}]}`);
      return { project: p, node, initialDoc, tree };
    },
    [],
  );

  const refreshTree = useCallback(async () => {
    if (!load) return;
    const next = await fetchTree(load.project.id, load.node.id);
    setLoad(next);
  }, [load, fetchTree]);

  const navigateToNode = useCallback(
    async (target: TreeNode | NodeRow) => {
      if (!load) return;
      const leaf = "children" in target ? findFirstLeaf(target as TreeNode) : (target as NodeRow);
      if (!leaf) {
        showToast("이동할 씬이 없습니다");
        return;
      }
      const n = await nodes.get(leaf.id);
      const initialDoc = JSON.parse(n.content_doc ?? `{"type":"doc","content":[{"type":"paragraph"}]}`);
      setLoad({ ...load, node: n, initialDoc });
      setCharCount(n.word_count);
      nodes.setLastOpened(load.project.id, n.id).catch(() => { /* benign */ });
    },
    [load],
  );

  // Initial load.
  useEffect(() => {
    if (!projectId) return;
    let cancelled = false;
    (async () => {
      try {
        const p = await projects.get(projectId);
        if (!p.last_opened_node_id) throw new Error("project has no opened node");
        const next = await fetchTree(projectId, p.last_opened_node_id);
        if (!cancelled) {
          setLoad(next);
          setCharCount(next.node.word_count);
        }
      } catch (e) {
        if (!cancelled) setError(String(e));
      }
    })();
    return () => { cancelled = true; };
  }, [projectId, fetchTree]);

  // Global Cmd+R reload + Cmd+K palette toggle.
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const isMac = navigator.platform.toLowerCase().includes("mac");
      const mod = isMac ? e.metaKey : e.ctrlKey;
      if (!mod) return;
      if (e.key.toLowerCase() === "r") {
        e.preventDefault();
        window.location.reload();
      } else if (e.key.toLowerCase() === "k") {
        e.preventDefault();
        setPaletteOpen((v) => !v);
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, []);

  const saveNow = useCallback(
    async (doc: object) => {
      if (!load) return;
      setSaveStatus({ kind: "saving" });
      try {
        await nodes.updateContent(load.node.id, JSON.stringify(doc));
        setSaveStatus({ kind: "saved", at: Date.now() });
      } catch (e) {
        setSaveStatus({ kind: "error", message: String(e) });
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

  useEffect(() => {
    if (!load) return;
    throttledLastOpened();
  }, [load, throttledLastOpened]);

  const handleManualSave = useCallback(
    async (doc: object) => {
      if (!load) return;
      setSaveStatus({ kind: "saving" });
      try {
        await nodes.updateContent(load.node.id, JSON.stringify(doc));
        await snapshots.createManual(load.node.id, JSON.stringify(doc));
        setSaveStatus({ kind: "saved", at: Date.now() });
        showToast("스냅샷 저장됨");
      } catch (e) {
        setSaveStatus({ kind: "error", message: String(e) });
        setError(String(e));
      }
    },
    [load],
  );

  // --- Commands ---

  const commands: Command[] = useMemo(() => {
    if (!load) return [];
    const { prev, next } = leafNeighbors(load.tree, load.node.id);
    const allNodes = flatten(load.tree);
    const siblingsOfCurrent = allNodes.filter(
      (n) => (n.parent_id ?? null) === (load.node.parent_id ?? null),
    );
    const leafSiblings = siblingsOfCurrent.filter((n) => n.kind === "leaf");
    const containerSiblings = siblingsOfCurrent.filter((n) => n.kind === "container");
    const nextSceneLabel = `씬 ${leafSiblings.length + 1}`;
    const nextChapterLabel = `${containerSiblings.length + 1}장`;

    const cmds: Command[] = [];
    cmds.push({
      id: "go-prev",
      section: "이동",
      label: "이전 씬",
      hint: prev ? prev.label : "(없음)",
      disabled: !prev,
      run: async () => prev && navigateToNode(prev),
    });
    cmds.push({
      id: "go-next",
      section: "이동",
      label: "다음 씬",
      hint: next ? next.label : "(없음)",
      disabled: !next,
      run: async () => next && navigateToNode(next),
    });
    for (const leaf of allNodes.filter((n) => n.kind === "leaf").slice(0, 20)) {
      cmds.push({
        id: `go-${leaf.id}`,
        section: "이동",
        label: `씬으로 이동: ${leaf.label}`,
        hint: leaf.title || undefined,
        disabled: leaf.id === load.node.id,
        run: async () => navigateToNode(leaf),
      });
    }

    cmds.push({
      id: "new-scene",
      section: "노드",
      label: `여기 옆에 새 씬 (${nextSceneLabel})`,
      run: async () => {
        const created = await nodes.createSibling(load.node.id, "leaf", nextSceneLabel, "");
        await refreshTree();
        navigateToNode(created);
      },
    });
    cmds.push({
      id: "new-chapter",
      section: "노드",
      label: `여기 옆에 새 장 (${nextChapterLabel})`,
      run: async () => {
        const chapter = await nodes.createSibling(load.node.id, "container", nextChapterLabel, "");
        // Seed it with one leaf so it's navigable.
        const seeded = await nodes.createChild(chapter.id, "leaf", "씬 1", "");
        await refreshTree();
        navigateToNode(seeded);
      },
    });
    cmds.push({
      id: "rename",
      section: "노드",
      label: "이름 바꾸기",
      run: async () => {
        const nextLabel = window.prompt("새 이름 (label)", load.node.label) ?? "";
        if (!nextLabel.trim()) return;
        const nextTitle = window.prompt("부제 (title, 비워두려면 취소)", load.node.title) ?? "";
        await nodes.rename(load.node.id, nextLabel.trim(), nextTitle);
        await refreshTree();
        showToast("이름이 변경되었습니다");
      },
    });
    cmds.push({
      id: "delete",
      section: "노드",
      label: "삭제",
      hint: load.node.label,
      run: async () => {
        if (!window.confirm(`"${load.node.label}"을(를) 삭제하시겠습니까?`)) return;
        // Find a fallback target before deleting.
        const fallback = prev ?? next ?? null;
        await nodes.delete(load.node.id);
        if (fallback) {
          navigateToNode(fallback);
        } else {
          // No other leaf — bounce to Library.
          navigate("/");
        }
      },
    });
    cmds.push({
      id: "move-up",
      section: "노드",
      label: "이 씬 위로",
      run: async () => {
        await nodes.moveUp(load.node.id);
        await refreshTree();
      },
    });
    cmds.push({
      id: "move-down",
      section: "노드",
      label: "이 씬 아래로",
      run: async () => {
        await nodes.moveDown(load.node.id);
        await refreshTree();
      },
    });
    cmds.push({
      id: "view-outline",
      section: "보기",
      label: "아웃라인 (왼쪽 가장자리 호버)",
      disabled: true,
      hint: "↤",
      run: () => {},
    });
    cmds.push({
      id: "view-character",
      section: "보기",
      label: "캐릭터 시트",
      hint: "(곧 추가됨 — Plan 4)",
      disabled: true,
      run: () => {},
    });
    cmds.push({
      id: "view-threads",
      section: "보기",
      label: "흐름(Thread)",
      hint: "(곧 추가됨 — post-MVP)",
      disabled: true,
      run: () => {},
    });
    return cmds;
  }, [load, navigateToNode, refreshTree, navigate]);

  const breadcrumb = useMemo(() => {
    if (!load) return "";
    // Build an ancestor chain by walking parent_id pointers.
    const byId = new Map(load.tree.flatMap(function flatten(n: TreeNode): [string, TreeNode][] {
      return [[n.id, n], ...n.children.flatMap(flatten)];
    }));
    const chain: string[] = [];
    let cur: TreeNode | undefined = byId.get(load.node.id);
    while (cur) {
      chain.unshift(cur.label);
      cur = cur.parent_id ? byId.get(cur.parent_id) : undefined;
    }
    return `← 작품 · ${chain.join(" › ")}${load.node.title ? ` — ${load.node.title}` : ""}`;
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
          saveStatus={saveStatus}
        />
      </div>

      <OutlinePanel
        tree={load.tree}
        currentId={load.node.id}
        onSelect={(n) => navigateToNode(n)}
      />

      <CommandPalette
        open={paletteOpen}
        onClose={() => setPaletteOpen(false)}
        commands={commands}
      />

      {toast && <div className="ws-toast">{toast}</div>}
    </main>
  );
}
```

- [ ] **Step 2: Type-check + build**

```bash
cd apps/desktop && pnpm tsc -b && pnpm build
```

If TypeScript complains about the breadcrumb's nested `flatten` (it's a stateful helper), refactor to a small helper above the component or use the imported `flatten` from `hooks/useFirstLeaf.ts` (already exported). Easiest fix: replace the inline flatten with `flatten(load.tree)`:

```ts
const byId = new Map(flatten(load.tree).map((n) => [n.id, n] as const));
```

- [ ] **Step 3: Commit**

```bash
git add apps/desktop/src/routes/Workspace.tsx
git commit -m "feat(workspace): Outline + Cmd+K wired with tree navigation"
```

---

## Task 9: Pre-warm Rust + E2E manual smoke + milestone tag

- [ ] **Step 1: Rebuild dev binary + warm Rust**

```bash
./scripts/build-engine.sh
(cd apps/desktop/src-tauri && cargo build) >/dev/null 2>&1 || true
```

- [ ] **Step 2: Launch dev**

```bash
rm -rf /tmp/linetta-plan3
LINETTA_HOME=/tmp/linetta-plan3 ./scripts/dev.sh
```

- [ ] **Step 3: Manual walk-through**

In the Tauri window:
1. Create a new project. The Workspace opens on the auto-created `씬 1`.
2. Press `⌘K` → palette opens. Type `새 씬` → first row is "여기 옆에 새 씬 (씬 2)". Press Enter.
3. Editor now shows the empty `씬 2`. Type a line of prose. Wait ~1s — right panel shows "저장됨".
4. `⌘K` again → "여기 옆에 새 장 (1장)" → Enter. Editor jumps into a freshly seeded `씬 1` inside `1장`. Breadcrumb shows `← 작품 · 1장 › 씬 1`.
5. Hover the **left edge** of the window. Outline slides in. Confirm: `씬 1` (top), `씬 2` (top), `1장 ▸ 씬 1` (current is highlighted). Click `씬 2` — editor switches.
6. `⌘K` → "이 씬 위로". The selected scene moves up in the outline.
7. `⌘K` → "이름 바꾸기". Type a new label in the prompt, then Cancel the title prompt. The outline reflects the rename.
8. `⌘K` → "삭제" → Confirm. Editor navigates to a sibling. Re-open outline; the deleted node is gone.
9. Move mouse away from outline; after 3s it auto-retracts.

If any step fails, report the exact step + observed behavior.

- [ ] **Step 4: Tag**

```bash
git tag plan-3-tree-done
```

---

## Definition of Done

- `cd engine && go test ./...` green.
- `cd apps/desktop && pnpm tsc -b && pnpm build` green.
- `cd apps/desktop/src-tauri && cargo check` green.
- Manual walk-through in Task 9 completes.
- Tag `plan-3-tree-done` exists.

Next plan: **Plan 4 — Entities + @mention + Entity Sheet**.
