# Plan 4 — Entities + `@` mention + Entity Sheet

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make characters and places live in the writing. The writer types `@해진` → a small picker offers matching entities or `새 인물로 추가: "해진"`. Selecting one inserts a Tiptap atom that renders as a blue underlined `@해진`. On save, the engine recomputes the node's mention rows so the right context panel's `인물·장소` always reflects who currently appears in the active scene. Double-clicking a mention slides in an Entity Sheet on the right where the writer can edit the entity's kind, role, summary, and free-form attributes.

**Architecture:** Two new Go packages (`engine/internal/entity`, `engine/internal/mention`) wrap their respective tables with a Repo each. `node.UpdateContent` is extended to walk the Tiptap doc after each save and call `mention.Repo.ResyncForNode(nodeID, list)`, which delete-all-then-inserts the latest set. Five new RPC handlers expose entity CRUD + `mentions.list_for_node`. On the frontend, `@tiptap/extension-mention` (official Tiptap extension) is wrapped in a `MentionExtension` factory that delegates the suggestion popup to a React-controlled `MentionPicker`. The Workspace owns the picker state, the EntitySheet state, and the per-node mention list; the ContextPanel renders `인물·장소` from that list.

**Tech Stack additions:**
- `@tiptap/extension-mention` v2 — Tiptap's standard mention extension

**Spec reference:** `docs/superpowers/specs/2026-05-25-linetta-immersive-rebuild-design.md` — §3 (`entities`, `mentions`), §4.3 (right context panel), §4.8 (Entity Sheet), §5.2 (@ pipeline), §11.1 items 4–5.

---

## Pre-flight

- [ ] `git describe --tags --exact-match plan-3-tree-done` returns ok.
- [ ] `git status --short` is empty.
- [ ] Engine tests, frontend tsc, cargo check all green.

---

## File Structure (created or modified)

```
engine/internal/entity/
  entity.go            (new — domain)
  repo.go              (new — Search/Get/Create/Update)
  repo_test.go         (new)
engine/internal/mention/
  mention.go           (new — domain)
  walker.go            (new — Tiptap doc walker)
  walker_test.go       (new)
  repo.go              (new — ResyncForNode/ListForNode)
  repo_test.go         (new)
engine/internal/node/
  repo.go              (modified — UpdateContent calls mention resync)
  repo_test.go         (modified — mention sync test)
engine/internal/rpc/handlers/
  entities.go          (new)
  entities_test.go     (new)
  mentions.go          (new)
  mentions_test.go     (new)
engine/cmd/linetta-engine/main.go  (modified — registers new handlers)

apps/desktop/src/
  lib/types.ts         (extended — Entity, Mention)
  lib/rpc.ts           (extended — entities, mentions namespaces)
  components/editor/
    Tiptap.tsx         (modified — extra extensions prop + double-click forward)
    Tiptap.css         (extended — .mention styling)
    MentionExtension.ts (new — Tiptap mention wrapper)
    MentionPicker.tsx  (new — popup UI)
    MentionPicker.css  (new)
  components/
    EntitySheet.tsx    (new)
    EntitySheet.css    (new)
    ContextPanel.tsx   (modified — shows mentions list)
  routes/Workspace.tsx (modified — wires mentions + entity sheet + picker)
  App.css              (APPEND if needed)
```

---

## Task 1: entity package (TDD)

Domain + SQLite Repo. `Search` does case-insensitive substring on `name`, ordered by length (shortest first) so `해` matches `해진` before `해진의 친구`. `Create` auto-fills sane defaults (`role=""`, `summary=""`, `attributes="{}"`, `aliases="[]"`). `Update` accepts a partial input — only non-empty fields overwrite.

**Files:**
- Create: `engine/internal/entity/entity.go`
- Create: `engine/internal/entity/repo.go`
- Create: `engine/internal/entity/repo_test.go`

- [ ] **Step 1: `entity.go`** — domain types

```go
// Package entity owns Entity (character/place/item/concept) domain logic.
package entity

// Kinds.
const (
	KindCharacter = "character"
	KindPlace     = "place"
	KindItem      = "item"
	KindConcept   = "concept"
)

// Entity mirrors the SQLite row. Attributes is free-form key→string JSON.
type Entity struct {
	ID         string            `json:"id"`
	ProjectID  string            `json:"project_id"`
	Kind       string            `json:"kind"`
	Name       string            `json:"name"`
	Aliases    []string          `json:"aliases"`
	Role       string            `json:"role"`
	Summary    string            `json:"summary"`
	Attributes map[string]string `json:"attributes"`
	CreatedAt  int64             `json:"created_at"`
	UpdatedAt  int64             `json:"updated_at"`
}

// NewInput is what `entities.create` accepts.
type NewInput struct {
	ProjectID string `json:"project_id"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Role      string `json:"role"`
}

// UpdateInput is what `entities.update` accepts. Fields with their zero value
// are left unchanged. Use a nil map to leave attributes unchanged; use an empty
// map to clear them.
type UpdateInput struct {
	ID         string             `json:"id"`
	Kind       string             `json:"kind"`
	Name       string             `json:"name"`
	Role       string             `json:"role"`
	Summary    string             `json:"summary"`
	Attributes *map[string]string `json:"attributes,omitempty"`
}
```

- [ ] **Step 2: failing `repo_test.go`**

```go
package entity

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func openStoreAndProject(t *testing.T) (*store.Store, project.Project) {
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

func TestRepo_Create_thenGet(t *testing.T) {
	s, p := openStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()
	e, err := r.Create(ctx, 100, NewInput{ProjectID: p.ID, Kind: KindCharacter, Name: "해진", Role: "POV"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if e.ID == "" || e.Name != "해진" || e.Kind != "character" {
		t.Errorf("unexpected entity: %+v", e)
	}
	got, err := r.Get(ctx, e.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "해진" {
		t.Errorf("Get name = %q", got.Name)
	}
	if got.Attributes == nil {
		t.Error("Attributes should be a non-nil empty map on a fresh entity")
	}
}

func TestRepo_Create_rejectsDuplicateName(t *testing.T) {
	s, p := openStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()
	if _, err := r.Create(ctx, 100, NewInput{ProjectID: p.ID, Kind: KindCharacter, Name: "해진"}); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if _, err := r.Create(ctx, 200, NewInput{ProjectID: p.ID, Kind: KindCharacter, Name: "해진"}); err == nil {
		t.Error("duplicate name should have failed")
	}
}

func TestRepo_Search_caseInsensitiveSubstring_shortFirst(t *testing.T) {
	s, p := openStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()
	_, _ = r.Create(ctx, 100, NewInput{ProjectID: p.ID, Kind: KindCharacter, Name: "해진"})
	_, _ = r.Create(ctx, 110, NewInput{ProjectID: p.ID, Kind: KindCharacter, Name: "해진의 친구"})
	_, _ = r.Create(ctx, 120, NewInput{ProjectID: p.ID, Kind: KindPlace, Name: "동해 해변"})

	got, err := r.Search(ctx, p.ID, "해", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].Name != "해진" || got[1].Name != "해진의 친구" || got[2].Name != "동해 해변" {
		t.Errorf("ordering = %q,%q,%q", got[0].Name, got[1].Name, got[2].Name)
	}
}

func TestRepo_Update_partial(t *testing.T) {
	s, p := openStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()
	e, _ := r.Create(ctx, 100, NewInput{ProjectID: p.ID, Kind: KindCharacter, Name: "해진"})
	attrs := map[string]string{"나이": "32", "직업": "사진작가"}
	if err := r.Update(ctx, 200, UpdateInput{ID: e.ID, Role: "POV", Summary: "사진을 찍는 사람", Attributes: &attrs}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := r.Get(ctx, e.ID)
	if got.Role != "POV" || got.Summary != "사진을 찍는 사람" {
		t.Errorf("update missed: %+v", got)
	}
	if got.Attributes["나이"] != "32" {
		t.Errorf("attributes not stored: %+v", got.Attributes)
	}
}

func TestRepo_Search_emptyQuery_returnsRecent(t *testing.T) {
	s, p := openStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()
	_, _ = r.Create(ctx, 100, NewInput{ProjectID: p.ID, Kind: KindCharacter, Name: "A"})
	_, _ = r.Create(ctx, 200, NewInput{ProjectID: p.ID, Kind: KindCharacter, Name: "B"})

	got, err := r.Search(ctx, p.ID, "", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("empty query: got %d, want 2", len(got))
	}
}
```

- [ ] **Step 3: run — expect compile failure (NewRepo undefined, etc.)**

```bash
cd engine && go test ./internal/entity/...
```

- [ ] **Step 4: `repo.go`**

```go
package entity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/google/uuid"
)

// ErrNotFound is returned when an entity id does not exist.
var ErrNotFound = errors.New("entity not found")

// Repo persists Entities in SQLite.
type Repo struct {
	s *store.Store
}

func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

// Create inserts a new entity. Returns ErrNotFound only for missing project (FK).
func (r *Repo) Create(ctx context.Context, now int64, in NewInput) (Entity, error) {
	if in.ProjectID == "" || in.Name == "" {
		return Entity{}, fmt.Errorf("create entity: project_id and name required")
	}
	kind := in.Kind
	if kind == "" {
		kind = KindCharacter
	}
	id := uuid.NewString()
	_, err := r.s.DB().ExecContext(ctx, `
INSERT INTO entities (id, project_id, kind, name, aliases, role, summary, attributes,
                      created_at, updated_at)
VALUES (?, ?, ?, ?, '[]', ?, '', '{}', ?, ?)`,
		id, in.ProjectID, kind, in.Name, in.Role, now, now)
	if err != nil {
		return Entity{}, err
	}
	return r.Get(ctx, id)
}

// Get returns one entity by id.
func (r *Repo) Get(ctx context.Context, id string) (Entity, error) {
	row := r.s.DB().QueryRowContext(ctx, baseSelect+` WHERE id = ?`, id)
	e, err := scan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Entity{}, ErrNotFound
	}
	return e, err
}

// Update applies a partial input. Empty strings leave fields alone; an Attributes
// pointer to a (possibly empty) map overwrites the JSON column.
func (r *Repo) Update(ctx context.Context, now int64, in UpdateInput) error {
	if in.ID == "" {
		return fmt.Errorf("update entity: id required")
	}

	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Fetch existing so we can merge.
	row := tx.QueryRowContext(ctx, baseSelect+` WHERE id = ?`, in.ID)
	cur, err := scan(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}

	if in.Kind != "" {
		cur.Kind = in.Kind
	}
	if in.Name != "" {
		cur.Name = in.Name
	}
	cur.Role = in.Role
	cur.Summary = in.Summary
	if in.Attributes != nil {
		cur.Attributes = *in.Attributes
	}

	attrsJSON, err := json.Marshal(cur.Attributes)
	if err != nil {
		return err
	}
	aliasesJSON, err := json.Marshal(cur.Aliases)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE entities
   SET kind = ?, name = ?, aliases = ?, role = ?, summary = ?, attributes = ?, updated_at = ?
 WHERE id = ?`, cur.Kind, cur.Name, string(aliasesJSON), cur.Role, cur.Summary,
		string(attrsJSON), now, in.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// Search returns entities whose name contains `query` (case-insensitive), shortest
// name first; with an empty query it returns the project's most-recently-updated
// entities. Limit is capped at 50.
func (r *Repo) Search(ctx context.Context, projectID, query string, limit int) ([]Entity, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	q := strings.ToLower(strings.TrimSpace(query))

	var (
		rows *sql.Rows
		err  error
	)
	if q == "" {
		rows, err = r.s.DB().QueryContext(ctx, baseSelect+`
WHERE project_id = ?
ORDER BY updated_at DESC
LIMIT ?`, projectID, limit)
	} else {
		rows, err = r.s.DB().QueryContext(ctx, baseSelect+`
WHERE project_id = ? AND LOWER(name) LIKE ?
ORDER BY LENGTH(name) ASC, updated_at DESC
LIMIT ?`, projectID, "%"+q+"%", limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entity
	for rows.Next() {
		e, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

const baseSelect = `
SELECT id, project_id, kind, name, aliases, role, summary, attributes,
       created_at, updated_at
FROM entities`

type scanner interface{ Scan(...any) error }

func scan(row scanner) (Entity, error) {
	var (
		e        Entity
		aliases  string
		attrsRaw string
	)
	if err := row.Scan(&e.ID, &e.ProjectID, &e.Kind, &e.Name, &aliases, &e.Role,
		&e.Summary, &attrsRaw, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return Entity{}, err
	}
	if aliases == "" {
		e.Aliases = []string{}
	} else if err := json.Unmarshal([]byte(aliases), &e.Aliases); err != nil {
		return Entity{}, fmt.Errorf("decode aliases: %w", err)
	}
	if attrsRaw == "" {
		e.Attributes = map[string]string{}
	} else if err := json.Unmarshal([]byte(attrsRaw), &e.Attributes); err != nil {
		return Entity{}, fmt.Errorf("decode attributes: %w", err)
	}
	if e.Aliases == nil {
		e.Aliases = []string{}
	}
	if e.Attributes == nil {
		e.Attributes = map[string]string{}
	}
	return e, nil
}
```

- [ ] **Step 5: run — PASS** (5 tests)

```bash
cd engine && go test ./internal/entity/... -v
```

- [ ] **Step 6: commit**

```bash
git add engine/internal/entity
git commit -m "feat(entity): repo with Create/Get/Update/Search"
```

---

## Task 2: mention walker + repo (TDD)

**Files:**
- Create: `engine/internal/mention/mention.go` (domain)
- Create: `engine/internal/mention/walker.go` (Tiptap walker)
- Create: `engine/internal/mention/walker_test.go`
- Create: `engine/internal/mention/repo.go` (ResyncForNode + ListForNode)
- Create: `engine/internal/mention/repo_test.go`

The walker recognizes Tiptap atom nodes of `type: "mention"` with `attrs.id` (entity id) and `attrs.label` (surface text). It returns a flat list with stable ordering (DFS).

- [ ] **Step 1: `mention.go`**

```go
// Package mention persists per-node Entity references derived from the doc.
package mention

// Mention mirrors the mentions row, plus the surface text shown in the body.
type Mention struct {
	ID       string `json:"id"`
	NodeID   string `json:"node_id"`
	EntityID string `json:"entity_id"`
	Position int    `json:"position"`
	Surface  string `json:"surface"`
}

// Found is what the walker emits — a position+entity tuple before persistence.
type Found struct {
	EntityID string
	Position int
	Surface  string
}
```

- [ ] **Step 2: failing `walker_test.go`**

```go
package mention

import "testing"

func TestCollect_emptyDoc(t *testing.T) {
	if got := Collect([]byte(`{"type":"doc","content":[{"type":"paragraph"}]}`)); len(got) != 0 {
		t.Errorf("empty doc → %d mentions, want 0", len(got))
	}
}

func TestCollect_findsMentionAtoms(t *testing.T) {
	doc := []byte(`{"type":"doc","content":[
		{"type":"paragraph","content":[
			{"type":"text","text":"파도 소리. "},
			{"type":"mention","attrs":{"id":"e1","label":"해진"}},
			{"type":"text","text":"가 모래를 밟았다. "},
			{"type":"mention","attrs":{"id":"e2","label":"윤서"}}
		]}
	]}`)
	got := Collect(doc)
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	if got[0].EntityID != "e1" || got[0].Surface != "해진" {
		t.Errorf("first = %+v", got[0])
	}
	if got[1].EntityID != "e2" || got[1].Surface != "윤서" {
		t.Errorf("second = %+v", got[1])
	}
	if !(got[0].Position < got[1].Position) {
		t.Errorf("positions not monotonically increasing: %d, %d", got[0].Position, got[1].Position)
	}
}

func TestCollect_ignoresMalformedAtoms(t *testing.T) {
	// Mention with no attrs.id should be skipped (rather than panic).
	doc := []byte(`{"type":"doc","content":[
		{"type":"paragraph","content":[
			{"type":"mention","attrs":{"label":"X"}},
			{"type":"mention","attrs":{"id":"","label":""}}
		]}
	]}`)
	if got := Collect(doc); len(got) != 0 {
		t.Errorf("malformed → got %d, want 0", len(got))
	}
}

func TestCollect_malformedJSON_returnsEmpty(t *testing.T) {
	if got := Collect([]byte(`not json`)); len(got) != 0 {
		t.Errorf("bad json → %d mentions", len(got))
	}
}
```

- [ ] **Step 3: `walker.go`**

```go
package mention

import "encoding/json"

// Collect walks a Tiptap doc (raw JSON) and returns every mention atom in DFS
// order, skipping malformed entries. Returns nil-equivalent empty slice on bad
// input.
func Collect(rawDoc []byte) []Found {
	if len(rawDoc) == 0 {
		return nil
	}
	var any interface{}
	if err := json.Unmarshal(rawDoc, &any); err != nil {
		return nil
	}
	var out []Found
	pos := 0
	walk(any, &pos, &out)
	return out
}

func walk(v interface{}, pos *int, out *[]Found) {
	switch t := v.(type) {
	case map[string]interface{}:
		kind, _ := t["type"].(string)
		if kind == "mention" {
			attrs, _ := t["attrs"].(map[string]interface{})
			id, _ := attrs["id"].(string)
			label, _ := attrs["label"].(string)
			if id != "" && label != "" {
				*out = append(*out, Found{EntityID: id, Position: *pos, Surface: label})
			}
			*pos++ // mention atoms count as one position
			return
		}
		if kind == "text" {
			if s, ok := t["text"].(string); ok {
				*pos += len([]rune(s))
			}
			return
		}
		if content, ok := t["content"].([]interface{}); ok {
			for _, c := range content {
				walk(c, pos, out)
			}
		}
	case []interface{}:
		for _, c := range t {
			walk(c, pos, out)
		}
	}
}
```

- [ ] **Step 4: walker PASS**

```bash
cd engine && go test ./internal/mention/... -v
```

- [ ] **Step 5: failing `repo_test.go`**

```go
package mention

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

type fixture struct {
	store *store.Store
	mr    *Repo
	er    *entity.Repo
	pID   string
	nID   string
}

func newFixture(t *testing.T) fixture {
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
	return fixture{store: s, mr: NewRepo(s), er: entity.NewRepo(s), pID: p.ID, nID: *p.LastOpenedNodeID}
}

func TestResyncForNode_insertsValidMentions_dropsUnknown(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	e, _ := f.er.Create(ctx, 100, entity.NewInput{ProjectID: f.pID, Kind: "character", Name: "해진"})

	found := []Found{
		{EntityID: e.ID, Position: 5, Surface: "해진"},
		{EntityID: "missing-uuid", Position: 10, Surface: "윤서"}, // dropped (FK invalid)
	}
	if err := f.mr.ResyncForNode(ctx, f.nID, found); err != nil {
		t.Fatalf("ResyncForNode: %v", err)
	}
	got, err := f.mr.ListForNode(ctx, f.nID)
	if err != nil {
		t.Fatalf("ListForNode: %v", err)
	}
	if len(got) != 1 || got[0].EntityID != e.ID {
		t.Errorf("after resync: %+v", got)
	}
}

func TestResyncForNode_replacesPreviousSet(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a, _ := f.er.Create(ctx, 100, entity.NewInput{ProjectID: f.pID, Kind: "character", Name: "A"})
	b, _ := f.er.Create(ctx, 110, entity.NewInput{ProjectID: f.pID, Kind: "character", Name: "B"})

	_ = f.mr.ResyncForNode(ctx, f.nID, []Found{{EntityID: a.ID, Position: 1, Surface: "A"}})
	_ = f.mr.ResyncForNode(ctx, f.nID, []Found{{EntityID: b.ID, Position: 1, Surface: "B"}})

	got, _ := f.mr.ListForNode(ctx, f.nID)
	if len(got) != 1 || got[0].EntityID != b.ID {
		t.Errorf("after re-resync: %+v", got)
	}
}

func TestListForNode_includesEntityFields(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	e, _ := f.er.Create(ctx, 100, entity.NewInput{ProjectID: f.pID, Kind: "character", Name: "해진", Role: "POV"})
	_ = f.mr.ResyncForNode(ctx, f.nID, []Found{{EntityID: e.ID, Position: 1, Surface: "해진"}})

	got, err := f.mr.ListEntitiesForNode(ctx, f.nID)
	if err != nil {
		t.Fatalf("ListEntitiesForNode: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len %d", len(got))
	}
	if got[0].Name != "해진" || got[0].Role != "POV" {
		t.Errorf("entity hydration missed: %+v", got[0])
	}
}
```

- [ ] **Step 6: `repo.go`**

```go
package mention

import (
	"context"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/google/uuid"
)

// Repo persists mentions rows.
type Repo struct {
	s *store.Store
}

func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

// ResyncForNode replaces the node's mention set with `found`. Any Found whose
// EntityID does not exist in the entities table is silently dropped (the body
// might reference a stale entity id after a deletion — that's allowed).
func (r *Repo) ResyncForNode(ctx context.Context, nodeID string, found []Found) error {
	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM mentions WHERE node_id = ?`, nodeID); err != nil {
		return err
	}
	for _, f := range found {
		// Filter out entries with no matching entity row.
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM entities WHERE id = ?`, f.EntityID).Scan(&exists); err != nil {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO mentions (id, node_id, entity_id, position, surface)
VALUES (?, ?, ?, ?, ?)`, uuid.NewString(), nodeID, f.EntityID, f.Position, f.Surface); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListForNode returns raw mention rows ordered by position.
func (r *Repo) ListForNode(ctx context.Context, nodeID string) ([]Mention, error) {
	rows, err := r.s.DB().QueryContext(ctx, `
SELECT id, node_id, entity_id, position, surface
  FROM mentions
 WHERE node_id = ?
 ORDER BY position ASC`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Mention
	for rows.Next() {
		var m Mention
		if err := rows.Scan(&m.ID, &m.NodeID, &m.EntityID, &m.Position, &m.Surface); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListEntitiesForNode returns the distinct entities mentioned in the node,
// hydrated with their full fields, in first-appearance order.
func (r *Repo) ListEntitiesForNode(ctx context.Context, nodeID string) ([]entity.Entity, error) {
	rows, err := r.s.DB().QueryContext(ctx, `
SELECT e.id, e.project_id, e.kind, e.name, e.aliases, e.role, e.summary, e.attributes,
       e.created_at, e.updated_at
  FROM entities e
  JOIN (
    SELECT entity_id, MIN(position) AS pos
      FROM mentions
     WHERE node_id = ?
     GROUP BY entity_id
  ) m ON m.entity_id = e.id
 ORDER BY m.pos ASC`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return entity.ScanAll(rows)
}
```

Note: this references a helper `entity.ScanAll`. Add it to `engine/internal/entity/repo.go`:

```go
// ScanAll consumes a *sql.Rows whose columns match baseSelect and returns the
// full slice. Exposed for cross-package callers (e.g. mention.Repo).
func ScanAll(rows interface{ Next() bool; Scan(...any) error; Close() error; Err() error }) ([]Entity, error) {
	defer rows.Close()
	var out []Entity
	for rows.Next() {
		e, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
```

- [ ] **Step 7: PASS** (3 repo tests + 4 walker tests)

```bash
cd engine && go test ./internal/mention/... ./internal/entity/... -v
```

- [ ] **Step 8: commit**

```bash
git add engine/internal/mention engine/internal/entity/repo.go
git commit -m "feat(mention): walker + ResyncForNode + ListEntitiesForNode"
```

---

## Task 3: node.UpdateContent runs mention resync (modify existing + test)

**Files:**
- Modify: `engine/internal/node/repo.go` (extend UpdateContent)
- Modify: `engine/internal/node/repo_test.go` (add test)

We change `UpdateContent` to accept (or look up itself) a `*mention.Repo` so it can call `ResyncForNode` after writing the doc. Two ways to do this; the simplest avoiding import cycles is to make `UpdateContent` take an optional dependency: instead, we add a new method `UpdateContentSyncing(ctx, id, doc, now, mentionRepo)` and keep `UpdateContent` as a thin wrapper that NO LONGER does the sync. Then we call the new method from main.go.

Actually that's ugly. Let me do it the other way: extend the existing `UpdateContent` to take an optional resyncer callback so the package doesn't import mention.

**Decision:** make `UpdateContent` invoke a *resyncer* function passed in via a Repo option. The Repo gets a small `SetMentionResyncer(func(ctx, nodeID, doc) error)` setter; main.go wires it. This keeps the package boundary clean.

- [ ] **Step 1: Modify `engine/internal/node/repo.go`**

Add a field + setter on Repo (insert near `NewRepo`):

```go
// MentionResyncer is called after a successful UpdateContent. Returns an error
// only if persistence fails; typically the implementation is mention.Repo.ResyncFromDoc.
type MentionResyncer func(ctx context.Context, nodeID, doc string) error

// SetMentionResyncer wires the optional callback. If unset, UpdateContent
// skips resync (used in tests that don't care about mentions).
func (r *Repo) SetMentionResyncer(fn MentionResyncer) {
	r.resync = fn
}
```

And add the field to the struct:

```go
type Repo struct {
	s      *store.Store
	resync MentionResyncer
}
```

Then in `UpdateContent` after `tx.Commit()` succeeds, call:

```go
	if r.resync != nil {
		if err := r.resync(ctx, id, doc); err != nil {
			return err
		}
	}
	return nil
}
```

(Adjust the existing `return tx.Commit()` so the resync happens after commit but before returning.)

- [ ] **Step 2: Add a test in `repo_test.go`**

```go
func TestRepo_UpdateContent_callsResyncer(t *testing.T) {
	s, p := newStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()

	called := 0
	var gotDoc, gotID string
	r.SetMentionResyncer(func(_ context.Context, nodeID, doc string) error {
		called++
		gotID = nodeID
		gotDoc = doc
		return nil
	})
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"hi"}]}]}`
	if err := r.UpdateContent(ctx, *p.LastOpenedNodeID, doc, 9999); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}
	if called != 1 {
		t.Errorf("resyncer called %d times, want 1", called)
	}
	if gotID != *p.LastOpenedNodeID || gotDoc != doc {
		t.Errorf("resyncer args wrong: id=%q docLen=%d", gotID, len(gotDoc))
	}
}
```

- [ ] **Step 3: full node-pkg suite PASS**

```bash
cd engine && go test ./internal/node/... -v
```

- [ ] **Step 4: commit**

```bash
git add engine/internal/node/repo.go engine/internal/node/repo_test.go
git commit -m "feat(node): UpdateContent calls optional mention resyncer"
```

---

## Task 4: entity + mention RPC handlers (TDD)

**Files:**
- Create: `engine/internal/rpc/handlers/entities.go`
- Create: `engine/internal/rpc/handlers/entities_test.go`
- Create: `engine/internal/rpc/handlers/mentions.go`
- Create: `engine/internal/rpc/handlers/mentions_test.go`

Handlers: `entities.search`, `entities.get`, `entities.create`, `entities.update`, `mentions.list_for_node`.

- [ ] **Step 1: entities_test.go**

```go
package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func newEntityFixture(t *testing.T) (*entity.Repo, project.Project) {
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
	return entity.NewRepo(s), p
}

func TestCreateEntityHandler(t *testing.T) {
	r, p := newEntityFixture(t)
	h := CreateEntity(r, func() int64 { return 1234 })
	params := json.RawMessage(`{"project_id":"` + p.ID + `","kind":"character","name":"해진","role":"POV"}`)
	res, err := h(context.Background(), params)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var e entity.Entity
	_ = json.Unmarshal(res, &e)
	if e.Name != "해진" || e.Role != "POV" || e.CreatedAt != 1234 {
		t.Errorf("entity = %+v", e)
	}
}

func TestSearchEntityHandler(t *testing.T) {
	r, p := newEntityFixture(t)
	_, _ = r.Create(context.Background(), 100, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "해진"})
	_, _ = r.Create(context.Background(), 110, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "해진의 친구"})

	h := SearchEntities(r)
	res, err := h(context.Background(), json.RawMessage(`{"project_id":"`+p.ID+`","query":"해","limit":10}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got []entity.Entity
	_ = json.Unmarshal(res, &got)
	if len(got) != 2 || got[0].Name != "해진" {
		t.Errorf("results = %+v", got)
	}
}

func TestGetEntityHandler(t *testing.T) {
	r, p := newEntityFixture(t)
	created, _ := r.Create(context.Background(), 100, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "해진"})
	h := GetEntity(r)
	res, err := h(context.Background(), json.RawMessage(`{"id":"`+created.ID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var e entity.Entity
	_ = json.Unmarshal(res, &e)
	if e.Name != "해진" {
		t.Errorf("name = %q", e.Name)
	}
}

func TestUpdateEntityHandler(t *testing.T) {
	r, p := newEntityFixture(t)
	created, _ := r.Create(context.Background(), 100, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "해진"})
	h := UpdateEntity(r, func() int64 { return 5000 })
	params := json.RawMessage(`{"id":"` + created.ID + `","name":"해진","role":"POV","summary":"사진작가","attributes":{"나이":"32"}}`)
	if _, err := h(context.Background(), params); err != nil {
		t.Fatalf("handler: %v", err)
	}
	got, _ := r.Get(context.Background(), created.ID)
	if got.Role != "POV" || got.Attributes["나이"] != "32" {
		t.Errorf("update missed: %+v", got)
	}
}
```

- [ ] **Step 2: entities.go**

```go
package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

func CreateEntity(repo *entity.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in entity.NewInput
		if err := json.Unmarshal(params, &in); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		if in.ProjectID == "" || in.Name == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id and name required"}
		}
		e, err := repo.Create(ctx, now(), in)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(e)
	}
}

type searchEntitiesParams struct {
	ProjectID string `json:"project_id"`
	Query     string `json:"query"`
	Limit     int    `json:"limit"`
}

func SearchEntities(repo *entity.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p searchEntitiesParams
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id required"}
		}
		list, err := repo.Search(ctx, p.ProjectID, p.Query, p.Limit)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if list == nil {
			list = []entity.Entity{}
		}
		return json.Marshal(list)
	}
}

func GetEntity(repo *entity.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		e, err := repo.Get(ctx, p.ID)
		if errors.Is(err, entity.ErrNotFound) {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "entity not found"}
		}
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(e)
	}
}

func UpdateEntity(repo *entity.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in entity.UpdateInput
		if err := json.Unmarshal(params, &in); err != nil || in.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := repo.Update(ctx, now(), in); err != nil {
			if errors.Is(err, entity.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "entity not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		got, err := repo.Get(ctx, in.ID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(got)
	}
}
```

- [ ] **Step 3: mentions_test.go + mentions.go**

```go
// mentions_test.go
package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func TestListMentionsForNodeHandler(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()
	pr := project.NewRepo(s)
	p, _ := pr.Create(context.Background(), 1000, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first",
	})
	er := entity.NewRepo(s)
	mr := mention.NewRepo(s)

	e, _ := er.Create(context.Background(), 2000, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "해진"})
	_ = mr.ResyncForNode(context.Background(), *p.LastOpenedNodeID, []mention.Found{
		{EntityID: e.ID, Position: 1, Surface: "해진"},
	})

	h := ListMentionsForNode(mr)
	res, err := h(context.Background(), json.RawMessage(`{"node_id":"`+*p.LastOpenedNodeID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got []entity.Entity
	_ = json.Unmarshal(res, &got)
	if len(got) != 1 || got[0].Name != "해진" {
		t.Errorf("got %+v", got)
	}
}
```

```go
// mentions.go
package handlers

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

type listMentionsParams struct {
	NodeID string `json:"node_id"`
}

func ListMentionsForNode(repo *mention.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p listMentionsParams
		if err := json.Unmarshal(params, &p); err != nil || p.NodeID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node_id required"}
		}
		list, err := repo.ListEntitiesForNode(ctx, p.NodeID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if list == nil {
			list = []entity.Entity{}
		}
		return json.Marshal(list)
	}
}
```

- [ ] **Step 4: PASS + commit**

```bash
cd engine && go test ./... -v 2>&1 | tail -30
git add engine/internal/rpc/handlers
git commit -m "feat(rpc): entities.* + mentions.list_for_node handlers"
```

---

## Task 5: wire main.go + smoke

**Files:**
- Modify: `engine/cmd/linetta-engine/main.go`

- [ ] **Step 1: Add repos + handler registrations**

Add to imports:
```go
"github.com/devlikebear/linetta/engine/internal/entity"
"github.com/devlikebear/linetta/engine/internal/mention"
```

After `snaps := snapshot.NewRepo(st)`:

```go
	entities := entity.NewRepo(st)
	mentions := mention.NewRepo(st)
	// Wire the mention resync into node.UpdateContent.
	nodes.SetMentionResyncer(func(ctx context.Context, nodeID, doc string) error {
		return mentions.ResyncForNode(ctx, nodeID, mention.Collect([]byte(doc)))
	})
```

After the existing `nodes.move_down` handler registration:

```go
	s.Handle("entities.search", handlers.SearchEntities(entities))
	s.Handle("entities.get", handlers.GetEntity(entities))
	s.Handle("entities.create", handlers.CreateEntity(entities, clock))
	s.Handle("entities.update", handlers.UpdateEntity(entities, clock))
	s.Handle("mentions.list_for_node", handlers.ListMentionsForNode(mentions))
```

- [ ] **Step 2: build + stdio smoke**

```bash
cd engine && go build -o /tmp/linetta-engine-build ./cmd/linetta-engine
rm -rf /tmp/linetta-plan4-smoke

# Phase A: create project + entity
LINETTA_HOME=/tmp/linetta-plan4-smoke /tmp/linetta-engine-build --stdio <<'EOF' | tee /tmp/plan4-resp.txt
{"jsonrpc":"2.0","id":1,"method":"projects.create","params":{"title":"M","genres":["SF"],"length_target":"short","default_pov":"first"}}
EOF
PID=$(python3 -c 'import json,sys; print(json.loads(open("/tmp/plan4-resp.txt").read().splitlines()[0])["result"]["id"])')
NID=$(python3 -c 'import json,sys; print(json.loads(open("/tmp/plan4-resp.txt").read().splitlines()[0])["result"]["last_opened_node_id"])')

LINETTA_HOME=/tmp/linetta-plan4-smoke /tmp/linetta-engine-build --stdio <<EOF | tee /tmp/plan4-ent.txt
{"jsonrpc":"2.0","id":2,"method":"entities.create","params":{"project_id":"$PID","kind":"character","name":"해진","role":"POV"}}
EOF
EID=$(python3 -c 'import json,sys; print(json.loads(open("/tmp/plan4-ent.txt").read().splitlines()[0])["result"]["id"])')

# Phase B: write doc with the mention, then verify mention resync happened.
LINETTA_HOME=/tmp/linetta-plan4-smoke /tmp/linetta-engine-build --stdio <<EOF
{"jsonrpc":"2.0","id":3,"method":"nodes.update_content","params":{"id":"$NID","doc":"{\"type\":\"doc\",\"content\":[{\"type\":\"paragraph\",\"content\":[{\"type\":\"text\",\"text\":\"파도 소리. \"},{\"type\":\"mention\",\"attrs\":{\"id\":\"$EID\",\"label\":\"해진\"}},{\"type\":\"text\",\"text\":\"이 모래를 밟았다.\"}]}]}"}}
{"jsonrpc":"2.0","id":4,"method":"mentions.list_for_node","params":{"node_id":"$NID"}}
EOF

rm -f /tmp/linetta-engine-build /tmp/plan4-resp.txt /tmp/plan4-ent.txt
rm -rf /tmp/linetta-plan4-smoke
```

Expected: id=4 result is an array containing one entity with `"name":"해진"`. Verify visually.

- [ ] **Step 3: commit + rebuild dev binary**

```bash
git add engine/cmd/linetta-engine/main.go
git commit -m "feat(engine): wire entity + mention repos and handlers"
./scripts/build-engine.sh
```

---

## Task 6: TS types + RPC additions

**Files:**
- Modify: `apps/desktop/src/lib/types.ts`
- Modify: `apps/desktop/src/lib/rpc.ts`

- [ ] **Step 1: Append types**

In `types.ts`, after the existing Snapshot block:

```ts
// Mirrors engine/internal/entity Entity struct.
export type EntityKind = "character" | "place" | "item" | "concept";

export interface Entity {
  id: string;
  project_id: string;
  kind: EntityKind;
  name: string;
  aliases: string[];
  role: string;
  summary: string;
  attributes: Record<string, string>;
  created_at: number;
  updated_at: number;
}

export interface NewEntityInput {
  project_id: string;
  kind: EntityKind;
  name: string;
  role?: string;
}

export interface UpdateEntityInput {
  id: string;
  kind?: EntityKind;
  name?: string;
  role?: string;
  summary?: string;
  attributes?: Record<string, string>;
}
```

- [ ] **Step 2: Extend `rpc.ts`** — add at the end:

```ts
import type { Entity, NewEntityInput, UpdateEntityInput } from "./types";

export const entities = {
  search: (projectId: string, query: string, limit = 20) =>
    rpcCall<Entity[]>("entities.search", { project_id: projectId, query, limit }),
  get: (id: string) => rpcCall<Entity>("entities.get", { id }),
  create: (input: NewEntityInput) => rpcCall<Entity>("entities.create", input),
  update: (input: UpdateEntityInput) => rpcCall<Entity>("entities.update", input),
};

export const mentions = {
  listForNode: (nodeId: string) =>
    rpcCall<Entity[]>("mentions.list_for_node", { node_id: nodeId }),
};
```

Note: the existing file already imports several types at the top; merge or add the new import accordingly.

- [ ] **Step 3: tsc + commit**

```bash
cd apps/desktop && pnpm tsc -b && cd ../..
git add apps/desktop/src/lib
git commit -m "feat(rpc): entities + mentions API clients"
```

---

## Task 7: Tiptap Mention extension

**Files:**
- Modify: `apps/desktop/package.json` (`@tiptap/extension-mention`)
- Create: `apps/desktop/src/components/editor/MentionExtension.ts`
- Modify: `apps/desktop/src/components/editor/Tiptap.tsx` (accept `extensions` prop)

- [ ] **Step 1: Install the extension**

```bash
cd apps/desktop && pnpm add @tiptap/extension-mention
```

- [ ] **Step 2: Write `MentionExtension.ts`**

```ts
import Mention from "@tiptap/extension-mention";
import type { Editor, Range } from "@tiptap/core";

export interface MentionItem {
  /** Existing entity id, or undefined for the "new entity" sentinel. */
  id?: string;
  /** Display name. For the sentinel: the typed-but-unmatched query. */
  name: string;
  /** Optional role for hint display. */
  role?: string;
  /** True for the "new entity" sentinel; the consumer creates it before insert. */
  isNew?: boolean;
}

export interface MentionPickerState {
  open: boolean;
  query: string;
  position: { left: number; top: number };
  items: MentionItem[];
  selectedIndex: number;
  /** Run the picker's currently-selected item. */
  pick: () => void;
  /** Run a specific item (by index). */
  pickAt: (index: number) => void;
  /** Move the selection up/down. */
  move: (delta: number) => void;
}

interface BuildOpts {
  search: (query: string) => Promise<MentionItem[]>;
  onStateChange: (state: MentionPickerState | null) => void;
}

export function buildMentionExtension(opts: BuildOpts) {
  return Mention.configure({
    HTMLAttributes: { class: "mention" },
    renderText({ node }) {
      return `@${node.attrs.label ?? ""}`;
    },
    suggestion: {
      char: "@",
      items: async ({ query }) => {
        const matched = await opts.search(query);
        if (query.trim().length > 0 && !matched.some((m) => !m.isNew && m.name === query)) {
          matched.push({ name: query, isNew: true });
        }
        return matched;
      },
      command: ({ editor, range, props }: { editor: Editor; range: Range; props: any }) => {
        // The picker resolves `props` to a real (id,label) tuple via opts.search +
        // its own consumer. Here we just splice it into the doc.
        editor
          .chain()
          .focus()
          .insertContentAt(range, [
            { type: "mention", attrs: { id: props.id, label: props.label } },
            { type: "text", text: " " },
          ])
          .run();
      },
      render: () => {
        let currentItems: MentionItem[] = [];
        let currentRange: Range = { from: 0, to: 0 };
        let currentEditor: Editor | null = null;
        let currentQuery = "";
        let currentClientRect: (() => DOMRect | null) | null = null;
        let selectedIndex = 0;

        const recompute = () => {
          const rect = currentClientRect?.();
          opts.onStateChange({
            open: true,
            query: currentQuery,
            position: rect
              ? { left: rect.left, top: rect.bottom + 4 }
              : { left: 0, top: 0 },
            items: currentItems,
            selectedIndex,
            pick: () => pickAt(selectedIndex),
            pickAt,
            move: (delta) => {
              if (currentItems.length === 0) return;
              selectedIndex = (selectedIndex + delta + currentItems.length) % currentItems.length;
              recompute();
            },
          });
        };

        const pickAt = (index: number) => {
          const item = currentItems[index];
          if (!item || !currentEditor) return;
          // The Workspace's onStateChange consumer will turn an `isNew` item
          // into a real entity before calling `pick()`. Here we *only* commit
          // when the item has an id; otherwise we hand off to the consumer.
          if (item.id && !item.isNew) {
            currentEditor
              .chain()
              .focus()
              .deleteRange(currentRange)
              .insertContent([
                { type: "mention", attrs: { id: item.id, label: item.name } },
                { type: "text", text: " " },
              ])
              .run();
            opts.onStateChange(null);
          } else {
            // Re-emit state so the Workspace can resolve the new entity flow
            // (it sees the state, awaits entity creation, then calls pick()
            //  on a refreshed state). The simplest contract: dispatch a custom
            //  event with the item; Workspace listens and finishes the work.
            window.dispatchEvent(
              new CustomEvent("linetta:mention-pick-new", { detail: { query: item.name, range: currentRange, editor: currentEditor } }),
            );
            opts.onStateChange(null);
          }
        };

        return {
          onStart: (props) => {
            currentItems = props.items as MentionItem[];
            currentRange = props.range;
            currentEditor = props.editor;
            currentQuery = props.query;
            currentClientRect = props.clientRect ?? null;
            selectedIndex = 0;
            recompute();
          },
          onUpdate: (props) => {
            currentItems = props.items as MentionItem[];
            currentRange = props.range;
            currentEditor = props.editor;
            currentQuery = props.query;
            currentClientRect = props.clientRect ?? null;
            if (selectedIndex >= currentItems.length) selectedIndex = 0;
            recompute();
          },
          onKeyDown: (props) => {
            if (props.event.key === "ArrowDown") {
              if (currentItems.length === 0) return false;
              selectedIndex = (selectedIndex + 1) % currentItems.length;
              recompute();
              return true;
            }
            if (props.event.key === "ArrowUp") {
              if (currentItems.length === 0) return false;
              selectedIndex = (selectedIndex - 1 + currentItems.length) % currentItems.length;
              recompute();
              return true;
            }
            if (props.event.key === "Enter") {
              pickAt(selectedIndex);
              return true;
            }
            if (props.event.key === "Escape") {
              opts.onStateChange(null);
              return true;
            }
            return false;
          },
          onExit: () => {
            opts.onStateChange(null);
          },
        };
      },
    },
  });
}
```

- [ ] **Step 3: Extend `Tiptap.tsx`** to accept `extensions` prop

Find the existing `Props` interface; add:

```ts
  /** Extra extensions to merge with StarterKit (e.g., MentionExtension). */
  extensions?: any[];
```

In the `useEditor` call, change `extensions: [StarterKit.configure({})]` to:

```ts
extensions: [StarterKit.configure({}), ...(extensions ?? [])],
```

And destructure `extensions` in the component signature.

Also: forward a double-click on `.mention` to a callback prop (`onMentionDoubleClick?: (entityId: string) => void`):

In the props interface add:
```ts
onMentionDoubleClick?: (entityId: string) => void;
```

In the return JSX, attach `onDoubleClick` on `.tiptap-wrap`:
```tsx
onDoubleClick={(e) => {
  const t = (e.target as HTMLElement).closest('.mention');
  if (t && onMentionDoubleClick) {
    const id = t.getAttribute('data-entity-id') || t.getAttribute('data-id');
    if (id) onMentionDoubleClick(id);
  }
}}
```

Note: `@tiptap/extension-mention` by default renders `<span class="mention" data-type="mention" data-id="...">@해진</span>`. The `data-id` attribute carries the entity id.

- [ ] **Step 4: Append `.mention` styling**

In `Tiptap.css`:

```css
.tiptap-editor .ProseMirror .mention {
  color: #1f3b8c;
  text-decoration: underline;
  text-decoration-color: rgba(31, 59, 140, 0.4);
  text-underline-offset: 2px;
  cursor: pointer;
  white-space: nowrap;
}

.tiptap-editor .ProseMirror .mention:hover {
  background: rgba(31, 59, 140, 0.06);
  border-radius: 3px;
}
```

- [ ] **Step 5: tsc + commit**

```bash
cd apps/desktop && pnpm tsc -b && pnpm build
cd ../..
git add apps/desktop/package.json apps/desktop/pnpm-lock.yaml apps/desktop/src/components/editor
git commit -m "feat(editor): mention extension + double-click forwarding"
```

---

## Task 8: MentionPicker component

**Files:**
- Create: `apps/desktop/src/components/editor/MentionPicker.tsx`
- Create: `apps/desktop/src/components/editor/MentionPicker.css`

Renders the popup at `state.position`. Pure presentation: the parent gives it state + callbacks.

- [ ] **Step 1: `MentionPicker.tsx`**

```tsx
import "./MentionPicker.css";
import type { MentionPickerState } from "./MentionExtension";

const KIND_LABEL: Record<string, string> = {
  character: "인물",
  place: "장소",
  item: "물건",
  concept: "개념",
};

interface Props {
  state: MentionPickerState | null;
}

export function MentionPicker({ state }: Props) {
  if (!state || !state.open) return null;
  return (
    <div
      className="mention-picker"
      style={{ left: state.position.left, top: state.position.top }}
      onMouseDown={(e) => e.preventDefault()} // keep focus in the editor
    >
      {state.items.length === 0 && (
        <p className="mention-picker-empty">결과 없음</p>
      )}
      {state.items.map((item, i) => {
        const active = i === state.selectedIndex;
        return (
          <button
            type="button"
            key={item.id ?? `new-${item.name}`}
            className={`mention-row${active ? " active" : ""}${item.isNew ? " new" : ""}`}
            onMouseMove={() => state.pickAt /* preview only */}
            onClick={() => state.pickAt(i)}
          >
            <span className="mention-name">
              {item.isNew ? `새 인물로 추가: "${item.name}"` : item.name}
            </span>
            {!item.isNew && item.role && <span className="mention-role">{item.role}</span>}
          </button>
        );
      })}
    </div>
  );
}
```

- [ ] **Step 2: `MentionPicker.css`**

```css
.mention-picker {
  position: fixed;
  background: #faf9f6;
  border: 1px solid #d8d6cf;
  border-radius: 6px;
  box-shadow: 0 10px 30px rgba(0, 0, 0, 0.18);
  min-width: 220px;
  max-width: 320px;
  max-height: 260px;
  overflow-y: auto;
  padding: 0.25rem 0;
  z-index: 50;
}

.mention-picker-empty {
  margin: 0;
  padding: 0.6rem 1rem;
  color: #9a9a9a;
  font-size: 0.85rem;
}

.mention-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  text-align: left;
  border: none;
  background: transparent;
  font: inherit;
  padding: 0.4rem 0.9rem;
  cursor: pointer;
}

.mention-row.active {
  background: #efece2;
}

.mention-row.new {
  font-style: italic;
  color: #555;
}

.mention-name {
  font-size: 0.92rem;
}

.mention-role {
  font-size: 0.78rem;
  color: #9a9a9a;
}
```

- [ ] **Step 3: tsc + commit**

```bash
cd apps/desktop && pnpm tsc -b && cd ../..
git add apps/desktop/src/components/editor
git commit -m "feat(picker): mention autocomplete popup"
```

---

## Task 9: EntitySheet component

**Files:**
- Create: `apps/desktop/src/components/EntitySheet.tsx`
- Create: `apps/desktop/src/components/EntitySheet.css`

Right-edge slide-in. Loads the entity by id, shows: avatar(글자), name input, kind select, role input, summary textarea, attribute key/value rows with add/remove. Save calls `entities.update` then refreshes.

- [ ] **Step 1: `EntitySheet.tsx`**

```tsx
import { useEffect, useState } from "react";
import type { Entity, EntityKind, UpdateEntityInput } from "../lib/types";
import { entities } from "../lib/rpc";
import "./EntitySheet.css";

interface Props {
  entityId: string | null;
  onClose: () => void;
  onSaved?: (entity: Entity) => void;
}

const KIND_LABEL: Record<EntityKind, string> = {
  character: "인물",
  place: "장소",
  item: "물건",
  concept: "개념",
};

export function EntitySheet({ entityId, onClose, onSaved }: Props) {
  const [entity, setEntity] = useState<Entity | null>(null);
  const [draft, setDraft] = useState<UpdateEntityInput | null>(null);
  const [attrRows, setAttrRows] = useState<{ key: string; value: string }[]>([]);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!entityId) return;
    setEntity(null);
    setError(null);
    entities.get(entityId).then((e) => {
      setEntity(e);
      setDraft({
        id: e.id,
        kind: e.kind,
        name: e.name,
        role: e.role,
        summary: e.summary,
        attributes: e.attributes,
      });
      setAttrRows(Object.entries(e.attributes).map(([key, value]) => ({ key, value })));
    }).catch((e) => setError(String(e)));
  }, [entityId]);

  if (!entityId) return null;

  const onSave = async () => {
    if (!draft) return;
    setSaving(true);
    setError(null);
    try {
      const attributes: Record<string, string> = {};
      for (const row of attrRows) {
        if (row.key.trim() !== "") attributes[row.key.trim()] = row.value;
      }
      const saved = await entities.update({ ...draft, attributes });
      setEntity(saved);
      if (onSaved) onSaved(saved);
      onClose();
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  };

  return (
    <aside className="entity-sheet" onMouseDown={(e) => e.stopPropagation()}>
      <header className="entity-head">
        <span>엔티티 편집</span>
        <button type="button" className="entity-close" onClick={onClose} aria-label="닫기">×</button>
      </header>

      {error && <p className="entity-error">{error}</p>}
      {!entity && !error && <p className="entity-loading">불러오는 중…</p>}

      {entity && draft && (
        <div className="entity-body">
          <div className="entity-id-row">
            <div className="entity-avatar">{(draft.name ?? entity.name).slice(0, 1)}</div>
            <div className="entity-id-text">
              <input
                className="entity-name"
                value={draft.name ?? ""}
                onChange={(e) => setDraft({ ...draft, name: e.target.value })}
                placeholder="이름"
              />
              <div className="entity-kind-row">
                <select
                  value={draft.kind ?? entity.kind}
                  onChange={(e) => setDraft({ ...draft, kind: e.target.value as EntityKind })}
                >
                  {(Object.keys(KIND_LABEL) as EntityKind[]).map((k) => (
                    <option key={k} value={k}>{KIND_LABEL[k]}</option>
                  ))}
                </select>
                <input
                  className="entity-role"
                  value={draft.role ?? ""}
                  onChange={(e) => setDraft({ ...draft, role: e.target.value })}
                  placeholder="역할 (예: POV)"
                />
              </div>
            </div>
          </div>

          <section className="entity-section">
            <h5>요약</h5>
            <textarea
              value={draft.summary ?? ""}
              onChange={(e) => setDraft({ ...draft, summary: e.target.value })}
              rows={3}
            />
          </section>

          <section className="entity-section">
            <h5>속성</h5>
            <div className="attr-table">
              {attrRows.map((row, i) => (
                <div className="attr-row" key={i}>
                  <input
                    className="attr-key"
                    value={row.key}
                    placeholder="키 (예: 나이)"
                    onChange={(e) => {
                      const next = [...attrRows];
                      next[i] = { ...row, key: e.target.value };
                      setAttrRows(next);
                    }}
                  />
                  <input
                    className="attr-value"
                    value={row.value}
                    placeholder="값 (예: 32)"
                    onChange={(e) => {
                      const next = [...attrRows];
                      next[i] = { ...row, value: e.target.value };
                      setAttrRows(next);
                    }}
                  />
                  <button
                    type="button"
                    className="attr-del"
                    onClick={() => setAttrRows(attrRows.filter((_, j) => j !== i))}
                    aria-label="삭제"
                  >×</button>
                </div>
              ))}
              <button
                type="button"
                className="attr-add"
                onClick={() => setAttrRows([...attrRows, { key: "", value: "" }])}
              >+ 속성 추가</button>
            </div>
          </section>

          <section className="entity-section relations">
            <h5>관계</h5>
            <p className="entity-empty">(post-MVP)</p>
          </section>

          <div className="entity-actions">
            <button type="button" onClick={onClose} disabled={saving}>취소</button>
            <button type="button" className="primary" onClick={onSave} disabled={saving}>
              {saving ? "저장 중…" : "저장"}
            </button>
          </div>
        </div>
      )}
    </aside>
  );
}
```

- [ ] **Step 2: `EntitySheet.css`**

```css
.entity-sheet {
  position: fixed;
  top: 50px;
  right: 0;
  bottom: 0;
  width: 340px;
  background: #faf9f6;
  border-left: 1px solid #ece9e0;
  box-shadow: -6px 0 24px rgba(0, 0, 0, 0.08);
  display: flex;
  flex-direction: column;
  z-index: 15;
  overflow: hidden;
}

.entity-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.85rem 1rem;
  border-bottom: 1px solid #ece9e0;
  font-size: 0.75rem;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: #6b6b6b;
}

.entity-close {
  background: none;
  border: none;
  font-size: 1.2rem;
  line-height: 1;
  color: #9a9a9a;
  cursor: pointer;
}

.entity-close:hover { color: #1a1a1a; }

.entity-body {
  flex: 1;
  overflow-y: auto;
  padding: 1rem;
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.entity-loading, .entity-error, .entity-empty { padding: 1rem; margin: 0; color: #6b6b6b; }
.entity-error { color: #a8312f; }

.entity-id-row {
  display: flex;
  gap: 0.75rem;
}

.entity-avatar {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  background: #1a1a1a;
  color: #faf9f6;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 1.2rem;
  flex-shrink: 0;
}

.entity-id-text { flex: 1; display: flex; flex-direction: column; gap: 0.4rem; }

.entity-name {
  font: inherit;
  font-size: 1rem;
  padding: 0.35rem 0.5rem;
  border: 1px solid #d8d6cf;
  border-radius: 4px;
  background: white;
}

.entity-kind-row { display: flex; gap: 0.4rem; }
.entity-kind-row select,
.entity-kind-row .entity-role {
  font: inherit;
  font-size: 0.9rem;
  padding: 0.3rem 0.5rem;
  border: 1px solid #d8d6cf;
  border-radius: 4px;
  background: white;
}
.entity-kind-row .entity-role { flex: 1; }

.entity-section h5 {
  margin: 0 0 0.4rem;
  font-size: 0.75rem;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: #6b6b6b;
}

.entity-section textarea {
  width: 100%;
  font: inherit;
  font-size: 0.9rem;
  padding: 0.4rem 0.55rem;
  border: 1px solid #d8d6cf;
  border-radius: 4px;
  background: white;
  resize: vertical;
}

.attr-table { display: flex; flex-direction: column; gap: 0.35rem; }
.attr-row { display: grid; grid-template-columns: 1fr 1fr auto; gap: 0.35rem; }
.attr-key, .attr-value {
  font: inherit;
  font-size: 0.85rem;
  padding: 0.3rem 0.45rem;
  border: 1px solid #d8d6cf;
  border-radius: 4px;
  background: white;
}
.attr-del {
  background: none;
  border: 1px solid #d8d6cf;
  border-radius: 4px;
  color: #9a9a9a;
  cursor: pointer;
  padding: 0 0.4rem;
}
.attr-add {
  font: inherit;
  font-size: 0.85rem;
  align-self: flex-start;
  border: 1px dashed #c8c5bd;
  border-radius: 999px;
  padding: 0.25rem 0.7rem;
  background: transparent;
  cursor: pointer;
  color: #555;
}

.entity-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.4rem;
  margin-top: auto;
}
.entity-actions button {
  font: inherit;
  font-size: 0.9rem;
  padding: 0.4rem 1rem;
  background: white;
  border: 1px solid #1a1a1a;
  border-radius: 4px;
  cursor: pointer;
}
.entity-actions button.primary { background: #1a1a1a; color: #faf9f6; }
```

- [ ] **Step 3: tsc + commit**

```bash
cd apps/desktop && pnpm tsc -b && cd ../..
git add apps/desktop/src/components/EntitySheet.tsx apps/desktop/src/components/EntitySheet.css
git commit -m "feat(entity-sheet): right slide-in editor for entity attributes"
```

---

## Task 10: Workspace integration

Wire it all together. The Workspace:
- Holds `mentionState`, `pickerOpen`, `entitySheetId`
- Builds the `MentionExtension` once per project, passes it as `extensions` to TiptapEditor
- Renders `MentionPicker` overlay with the current state
- Listens for `linetta:mention-pick-new` events; pops the EntitySheet pre-filled with a freshly-created entity (or just creates it silently and inserts)
- On TiptapEditor's `onMentionDoubleClick(entityId)`, opens EntitySheet
- After every successful save, fetches `mentions.listForNode(currentNodeId)` and stores in load state; ContextPanel renders it
- After entity create/update via Sheet, refreshes the mention list

**Files:**
- Modify: `apps/desktop/src/routes/Workspace.tsx`
- Modify: `apps/desktop/src/components/ContextPanel.tsx`

- [ ] **Step 1: Extend `ContextPanel.tsx` Props**

Add to Props:
```ts
mentionedEntities: Entity[];
onMentionClick: (entityId: string) => void;
```

Replace the `(곧 추가됨 — Plan 4)` placeholder for "인물 · 장소" with:

```tsx
<section className="ctx-section">
  <h4>인물 · 장소</h4>
  {mentionedEntities.length === 0 && (
    <p className="ctx-empty">아직 @멘션 없음</p>
  )}
  {mentionedEntities.map((e) => (
    <button
      key={e.id}
      type="button"
      className="ctx-entity"
      onClick={() => onMentionClick(e.id)}
    >
      <span className="ctx-entity-avatar">{e.name.slice(0, 1)}</span>
      <span className="ctx-entity-name">{e.name}</span>
      {e.role && <span className="ctx-entity-role">{e.role}</span>}
    </button>
  ))}
</section>
```

Append to `App.css`:

```css
.ctx-entity {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  background: none;
  border: none;
  font: inherit;
  font-size: 0.85rem;
  padding: 0.25rem 0;
  cursor: pointer;
  width: 100%;
  text-align: left;
}
.ctx-entity:hover { color: #1a1a1a; }
.ctx-entity-avatar {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background: #d8d6cf;
  color: #1a1a1a;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 0.7rem;
}
.ctx-entity-role { color: #9a9a9a; margin-left: auto; font-size: 0.75rem; }
```

- [ ] **Step 2: Extend `Workspace.tsx`**

State additions:
```ts
const [mentionState, setMentionState] = useState<MentionPickerState | null>(null);
const [entitySheetId, setEntitySheetId] = useState<string | null>(null);
const [mentioned, setMentioned] = useState<Entity[]>([]);
```

Build the extension (must depend only on `projectId`, which is stable):
```ts
const mentionExtension = useMemo(() => {
  if (!projectId) return null;
  return buildMentionExtension({
    search: (query) => entities.search(projectId, query),
    onStateChange: setMentionState,
  });
}, [projectId]);
```

Refresh mentioned entities whenever `load.node.id` changes OR after a save settles:
```ts
const refreshMentioned = useCallback(async (nodeId: string) => {
  try { setMentioned(await mentionsApi.listForNode(nodeId)); }
  catch { /* benign */ }
}, []);

useEffect(() => {
  if (load) refreshMentioned(load.node.id);
}, [load?.node.id, refreshMentioned]);
```

Where `mentionsApi` is imported as `import { mentions as mentionsApi } from "../lib/rpc"` (alias to avoid collision with the local `mentioned` state name).

Hook into the existing `saveNow` so that after each successful save, mentions are refetched:
```ts
const saveNow = useCallback(async (doc: object) => {
  if (!load) return;
  setSaveStatus({ kind: "saving" });
  try {
    await nodes.updateContent(load.node.id, JSON.stringify(doc));
    setSaveStatus({ kind: "saved", at: Date.now() });
    refreshMentioned(load.node.id);
  } catch (e) { ... }
}, [load, refreshMentioned]);
```

Listen for the "new entity" event:
```ts
useEffect(() => {
  if (!projectId) return;
  const handler = async (e: Event) => {
    const detail = (e as CustomEvent).detail as { query: string; range: Range; editor: any };
    try {
      const created = await entities.create({ project_id: projectId, kind: "character", name: detail.query });
      detail.editor.chain().focus().deleteRange(detail.range).insertContent([
        { type: "mention", attrs: { id: created.id, label: created.name } },
        { type: "text", text: " " },
      ]).run();
      setEntitySheetId(created.id); // open the sheet so writer can refine
    } catch (e) { /* surface via toast */ }
  };
  window.addEventListener("linetta:mention-pick-new", handler);
  return () => window.removeEventListener("linetta:mention-pick-new", handler);
}, [projectId]);
```

Pass the extension to TiptapEditor:
```tsx
<TiptapEditor
  key={load.node.id}
  ref={editorRef}
  extensions={mentionExtension ? [mentionExtension] : []}
  onMentionDoubleClick={(id) => setEntitySheetId(id)}
  ...
/>
```

Render the picker and the sheet:
```tsx
<MentionPicker state={mentionState} />
<EntitySheet
  entityId={entitySheetId}
  onClose={() => { setEntitySheetId(null); focusEditor(); refreshMentioned(load.node.id); }}
  onSaved={() => refreshMentioned(load.node.id)}
/>
```

Pass new props to ContextPanel:
```tsx
<ContextPanel
  ...
  mentionedEntities={mentioned}
  onMentionClick={(id) => setEntitySheetId(id)}
/>
```

- [ ] **Step 3: tsc + build**

```bash
cd apps/desktop && pnpm tsc -b && pnpm build
```

The implementer may need to adjust the `Range` import inside the event handler (it's a Tiptap type). The cleanest source is `import type { Range } from "@tiptap/core"`. If TS complains, type the detail field as `any` for Plan 4 and tighten later.

- [ ] **Step 4: commit**

```bash
git add apps/desktop/src
git commit -m "feat(workspace): @mention picker + entity sheet + 인물·장소 panel"
```

---

## Task 11: E2E smoke + milestone tag

- [ ] **Step 1: Pre-warm**

```bash
./scripts/build-engine.sh
(cd apps/desktop/src-tauri && cargo build) >/dev/null 2>&1 || true
```

- [ ] **Step 2: Run dev**

```bash
rm -rf /tmp/linetta-plan4
LINETTA_HOME=/tmp/linetta-plan4 ./scripts/dev.sh
```

- [ ] **Step 3: Manual walk-through**

1. New project ("멘션 테스트"). Workspace opens on the empty 씬 1.
2. Type: `파도 소리. ` then `@해진`. The picker appears under the cursor. The only entry is `새 인물로 추가: "해진"`.
3. Press Enter. The mention atom inserts (`@해진` underlined). The Entity Sheet opens on the right with `이름=해진`, `kind=인물`. Fill `역할=POV`, add an attribute `직업=사진작가`, save.
4. Continue typing: `이 모래를 밟았다. ` then `@윤서` Enter — another entity created.
5. After a moment, the right context panel `인물 · 장소` shows both `해진 (POV)` and `윤서`.
6. **Double-click** the `@해진` text. Entity Sheet reopens with the saved values. Change summary, save. Sheet closes; the body's `@해진` mention is unchanged.
7. Click `해진` in the right panel — the sheet opens too.
8. Type a fresh `@해`. Both `해진` and… well, just `해진` matches; press Enter inserts it (no duplicate created).
9. Cmd+R reload. The body persists, the mentions panel re-populates from the engine.

- [ ] **Step 4: Tag**

```bash
git tag plan-4-mentions-done
```

---

## Definition of Done

- `cd engine && go test ./...` green.
- `cd apps/desktop && pnpm tsc -b && pnpm build` green.
- `cd apps/desktop/src-tauri && cargo check` green.
- Manual walk-through (Task 11) succeeds.
- Tag `plan-4-mentions-done` exists.

Next plan: **Plan 5 — AI mode** (PROMPT + 프리셋 + 스트리밍 + 컨텍스트 자동 첨부 + 톤 프리셋).
