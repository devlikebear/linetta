# Plan 8 — Entity↔Entity Relationship with Auto Bidirectional Sync

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the final P1 post-MVP gap by giving writers a real "관계" surface on the `EntitySheet`: pick a target entity, pick or type a label (freeform with chip suggestions like `연인 · 부모 · 자녀 · 친구 · 적수 · 동료 · 형제`), optionally provide an inverse label, and have the engine atomically write either one singleton row (no inverse) or **two paired rows** that share a `pair_id` so deleting either side cascades to its partner. The list on each entity's sheet shows only rows where `from_id = X.id`, ordered by insertion (`id` UUID order — stable enough for this scope).

**Architecture:** One new engine package `relationship` modeled after Plan 7's `thread` package shape. The `relationships` table already exists in `0001_init.sql`; a **0003 migration** adds a nullable `pair_id TEXT` column plus two read-pattern indexes (`from_id`, `pair_id`). `Repo` exposes `CreateOne` (pair_id NULL), `CreatePair` (two rows in one tx sharing a fresh UUID pair_id, with swapped from/to and the two labels each writer typed), `ListByEntity`, `Update` (per-side only — paired side keeps its own label/notes), `Delete` (atomic both-rows if paired; single row otherwise), and `Get`. RPC handlers mirror `threads.*` verbatim. Frontend adds typed RPC bindings, a `RelationshipPicker` popover, and replaces the existing `<section className="entity-section relations">` placeholder in `EntitySheet.tsx` with a real list + "+ 관계 추가" button.

**Tech Stack additions:** None. Pure Go + React on the existing stack (Tauri 2, modernc.org/sqlite, React 18, no vitest in the desktop app).

**Spec reference:** `docs/superpowers/specs/2026-05-25-linetta-immersive-rebuild-design.md` — §3 (Data Model: `relationships`), §11.2 P1 (post-MVP relationships).

**Design decisions locked by the user:**
1. **Paired by default with optional inverse label.** Empty `inverse_label` → ONE row, `pair_id = NULL`. Non-empty `inverse_label` → TWO rows in one transaction, sharing one `pair_id = uuid()`.
2. **Label = freeform text with preset chips.** Chips: `연인 · 부모 · 자녀 · 친구 · 적수 · 동료 · 형제`. Clicking a chip fills the input; the user can still edit it.
3. **Delete sync.** `pair_id IS NOT NULL` → delete BOTH rows in a transaction. Singleton → delete the one row.
4. **Listing on EntitySheet.** When opening entity X's sheet, list rows where `from_id = X.id` ordered by `id`. The inverse half of a pair appears automatically when the partner entity's sheet is opened (its own `from_id = partner.id` rows).

---

## Pre-flight

- [ ] Plan 7 is tagged (`plan-7-thread-beat-done`) and `git status --short` is empty.
- [ ] `cd engine && go test ./... -race` green.
- [ ] `cd apps/desktop && pnpm tsc -b && pnpm build` green.
- [ ] `cd apps/desktop/src-tauri && cargo check` green.
- [ ] Confirm the `relationships` table exists by running `sqlite3 "$LINETTA_HOME/library.db" ".schema relationships"` — should show the 0001 definition WITHOUT `pair_id` (the 0003 migration is what adds it).

---

## File Structure (created or modified)

```
engine/internal/store/migrations/
  0003_relationship_pair_id.sql       (new — ALTER TABLE + 2 indexes)

engine/internal/relationship/
  relationship.go                     (new — Relationship struct + inputs)
  repo.go                             (new — CreateOne/CreatePair/Get/ListByEntity/Update/Delete)
  repo_test.go                        (new)

engine/internal/rpc/handlers/
  relationships.go                    (new — 6 handlers)
  relationships_test.go               (new)

engine/cmd/linetta-engine/main.go     (modified — repo + handler registration)

apps/desktop/src/
  lib/types.ts                        (modified — Relationship + inputs)
  lib/rpc.ts                          (modified — relationships namespace)
  lib/relationshipPresets.ts          (new — LABEL_PRESETS constant)
  components/RelationshipPicker.tsx   (new — target search + label chips + inverse)
  components/RelationshipPicker.css   (new)
  components/EntitySheet.tsx          (modified — replace lines 156–159 placeholder)
  components/EntitySheet.css          (modified — small .relation-row styles)
```

---

## Phase A: Engine (3 tasks)

### Task 1: 0003 migration — `pair_id` column + 2 indexes

The 0001 `relationships` table is missing the `pair_id` grouping column and any read-pattern indexes. SQLite only safely supports `ALTER TABLE ... ADD COLUMN` for nullable columns without a default, which is exactly what we need (`pair_id` is null for singletons). Indexes are additive and idempotent.

**Files:**
- Create: `engine/internal/store/migrations/0003_relationship_pair_id.sql`
- Touches: existing `engine/internal/store/migrations_test.go` (already validates that every embedded migration applies cleanly; no edits required)

- [ ] **Step 1: Write the migration**

`engine/internal/store/migrations/0003_relationship_pair_id.sql`:

```sql
-- Plan 8: bidirectional relationship pairing.
-- pair_id groups two rows that were created together (A→B and B→A).
-- NULL = singleton (no inverse). SQLite-safe: nullable, no default.
ALTER TABLE relationships ADD COLUMN pair_id TEXT;

CREATE INDEX IF NOT EXISTS idx_relationships_from ON relationships(from_id);
CREATE INDEX IF NOT EXISTS idx_relationships_pair ON relationships(pair_id);
```

- [ ] **Step 2: Run the existing migration test**

```bash
cd engine && go test ./internal/store/... -run TestApplyMigrations -race
```

Both `TestApplyMigrations_appliesOnce` and `TestApplyMigrations_createsProjectsTable` MUST still pass. They exercise the embed.FS reader through `ApplyMigrations`, so a malformed 0003 file would surface here.

- [ ] **Step 3: Add a dedicated assertion** (optional but cheap)

Append a new test in `engine/internal/store/migrations_test.go`:

```go
func TestApplyMigrations_addsRelationshipPairID(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply: %v", err)
	}
	// pair_id must accept NULL (singleton) and a TEXT value.
	if _, err := db.ExecContext(ctx, `
INSERT INTO projects (id, title, genres, length_target, default_pov, created_at, updated_at)
VALUES ('p1', 'T', '["SF"]', 'short', 'first', 0, 0)`); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO entities (id, project_id, kind, name, created_at, updated_at)
VALUES ('e1','p1','character','A',0,0),
       ('e2','p1','character','B',0,0)`); err != nil {
		t.Fatalf("seed entities: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO relationships (id, project_id, from_id, to_id, label, pair_id)
VALUES ('r1','p1','e1','e2','친구', NULL),
       ('r2','p1','e1','e2','친구','pair-xyz')`); err != nil {
		t.Fatalf("insert relationships: %v", err)
	}
}
```

Run:

```bash
cd engine && go test ./internal/store/... -race
```

- [ ] **Step 4: Commit**

```
git add engine/internal/store/migrations/0003_relationship_pair_id.sql engine/internal/store/migrations_test.go
git commit -m "feat(store): 0003 migration adds nullable relationships.pair_id + indexes"
```

---

### Task 2: `engine/internal/relationship` package (TDD)

Mirror the `engine/internal/thread` package shape: a struct file plus a Repo with one constructor and small focused methods. Every method gets a test before any implementation lands.

**Files:**
- Create: `engine/internal/relationship/relationship.go`
- Create: `engine/internal/relationship/repo.go`
- Create: `engine/internal/relationship/repo_test.go`

- [ ] **Step 1: Failing tests**

`engine/internal/relationship/repo_test.go`:

```go
package relationship

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

type fixture struct {
	s    *store.Store
	r    *Repo
	pID  string
	a, b string // entity IDs
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
	p, err := pr.Create(context.Background(), 1000, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	er := entity.NewRepo(s)
	a, err := er.Create(context.Background(), 2000, entity.NewInput{
		ProjectID: p.ID, Kind: entity.KindCharacter, Name: "해진",
	})
	if err != nil {
		t.Fatalf("create A: %v", err)
	}
	b, err := er.Create(context.Background(), 2001, entity.NewInput{
		ProjectID: p.ID, Kind: entity.KindCharacter, Name: "아지",
	})
	if err != nil {
		t.Fatalf("create B: %v", err)
	}
	return fixture{s: s, r: NewRepo(s), pID: p.ID, a: a.ID, b: b.ID}
}

func TestRepo_CreateOne_singleton(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	r, err := f.r.CreateOne(ctx, NewInput{
		ProjectID: f.pID, FromID: f.a, ToID: f.b, Label: "엄마",
	})
	if err != nil {
		t.Fatalf("CreateOne: %v", err)
	}
	if r.ID == "" || r.Label != "엄마" || r.PairID != nil {
		t.Errorf("singleton mismatch: %+v", r)
	}
	got, err := f.r.Get(ctx, r.ID)
	if err != nil || got.PairID != nil {
		t.Errorf("Get singleton: %+v err=%v", got, err)
	}
}

func TestRepo_CreatePair_twoRowsShareNonNilPairID(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	rows, err := f.r.CreatePair(ctx, NewPairInput{
		ProjectID: f.pID, FromID: f.a, ToID: f.b,
		Label: "친구", InverseLabel: "친구",
	})
	if err != nil {
		t.Fatalf("CreatePair: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	// (a) two rows exist
	// (b) both share the same non-nil pair_id
	if rows[0].PairID == nil || rows[1].PairID == nil {
		t.Fatalf("pair_id must be non-nil on both rows: %+v", rows)
	}
	if *rows[0].PairID != *rows[1].PairID {
		t.Errorf("pair_id mismatch: %q vs %q", *rows[0].PairID, *rows[1].PairID)
	}
	// (c) from_id/to_id swapped between rows
	forward := rows[0]
	inverse := rows[1]
	if !(forward.FromID == f.a && forward.ToID == f.b) {
		t.Errorf("forward row not A→B: %+v", forward)
	}
	if !(inverse.FromID == f.b && inverse.ToID == f.a) {
		t.Errorf("inverse row not B→A: %+v", inverse)
	}
	// (d) labels assigned correctly: forward=Label, inverse=InverseLabel
	if forward.Label != "친구" || inverse.Label != "친구" {
		t.Errorf("labels: forward=%q inverse=%q", forward.Label, inverse.Label)
	}
}

func TestRepo_CreatePair_distinctLabels(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	rows, err := f.r.CreatePair(ctx, NewPairInput{
		ProjectID: f.pID, FromID: f.a, ToID: f.b,
		Label: "부모", InverseLabel: "자녀",
	})
	if err != nil {
		t.Fatalf("CreatePair: %v", err)
	}
	if rows[0].Label != "부모" || rows[1].Label != "자녀" {
		t.Errorf("distinct labels not preserved: forward=%q inverse=%q",
			rows[0].Label, rows[1].Label)
	}
}

func TestRepo_ListByEntity_filtersByFromID_orderedByID(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	// A→B paired (creates A→B and B→A)
	if _, err := f.r.CreatePair(ctx, NewPairInput{
		ProjectID: f.pID, FromID: f.a, ToID: f.b, Label: "친구", InverseLabel: "친구",
	}); err != nil {
		t.Fatalf("pair: %v", err)
	}
	// A→B singleton (e.g. "엄마")
	if _, err := f.r.CreateOne(ctx, NewInput{
		ProjectID: f.pID, FromID: f.a, ToID: f.b, Label: "엄마",
	}); err != nil {
		t.Fatalf("single: %v", err)
	}
	listA, err := f.r.ListByEntity(ctx, f.a)
	if err != nil {
		t.Fatalf("ListByEntity A: %v", err)
	}
	if len(listA) != 2 {
		t.Errorf("A list = %d rows, want 2 (paired A→B + singleton A→B)", len(listA))
	}
	listB, err := f.r.ListByEntity(ctx, f.b)
	if err != nil {
		t.Fatalf("ListByEntity B: %v", err)
	}
	if len(listB) != 1 || listB[0].FromID != f.b {
		t.Errorf("B list = %+v, want only the inverse half of the pair", listB)
	}
}

func TestRepo_Update_paired_onlyTouchesOneSide(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	rows, _ := f.r.CreatePair(ctx, NewPairInput{
		ProjectID: f.pID, FromID: f.a, ToID: f.b,
		Label: "친구", InverseLabel: "친구",
	})
	if err := f.r.Update(ctx, UpdateInput{
		ID: rows[0].ID, Label: "절친", Notes: "유년기부터",
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	gotForward, _ := f.r.Get(ctx, rows[0].ID)
	gotInverse, _ := f.r.Get(ctx, rows[1].ID)
	if gotForward.Label != "절친" || gotForward.Notes != "유년기부터" {
		t.Errorf("forward not updated: %+v", gotForward)
	}
	if gotInverse.Label != "친구" || gotInverse.Notes != "" {
		t.Errorf("inverse should be untouched: %+v", gotInverse)
	}
}

func TestRepo_Delete_paired_removesBothRowsAtomically(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	rows, _ := f.r.CreatePair(ctx, NewPairInput{
		ProjectID: f.pID, FromID: f.a, ToID: f.b,
		Label: "친구", InverseLabel: "친구",
	})
	if err := f.r.Delete(ctx, rows[0].ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := f.r.Get(ctx, rows[0].ID); err == nil {
		t.Error("forward row still exists")
	}
	if _, err := f.r.Get(ctx, rows[1].ID); err == nil {
		t.Error("inverse row still exists — pair delete must be atomic")
	}
}

func TestRepo_Delete_singleton_doesNotAffectOthers(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	single, _ := f.r.CreateOne(ctx, NewInput{
		ProjectID: f.pID, FromID: f.a, ToID: f.b, Label: "엄마",
	})
	pair, _ := f.r.CreatePair(ctx, NewPairInput{
		ProjectID: f.pID, FromID: f.a, ToID: f.b,
		Label: "친구", InverseLabel: "친구",
	})
	if err := f.r.Delete(ctx, single.ID); err != nil {
		t.Fatalf("Delete singleton: %v", err)
	}
	if _, err := f.r.Get(ctx, pair[0].ID); err != nil {
		t.Errorf("pair forward should survive: %v", err)
	}
	if _, err := f.r.Get(ctx, pair[1].ID); err != nil {
		t.Errorf("pair inverse should survive: %v", err)
	}
}

func TestRepo_Get_notFound(t *testing.T) {
	f := newFixture(t)
	if _, err := f.r.Get(context.Background(), "no-such-id"); err == nil {
		t.Error("expected ErrNotFound")
	}
}
```

Run (should fail to compile because the package doesn't exist yet):

```bash
cd engine && go test ./internal/relationship/... -race
```

- [ ] **Step 2: Implement `relationship.go`**

`engine/internal/relationship/relationship.go`:

```go
// Package relationship owns Entity↔Entity edges (관계). The schema lives in
// 0001_init.sql; 0003 adds the nullable pair_id column used to group inverse
// rows so deletes can cascade to the partner.
package relationship

// Relationship mirrors the SQLite row. PairID is nil for singletons (no inverse).
type Relationship struct {
	ID        string  `json:"id"`
	ProjectID string  `json:"project_id"`
	FromID    string  `json:"from_id"`
	ToID      string  `json:"to_id"`
	Label     string  `json:"label"`
	Notes     string  `json:"notes"`
	PairID    *string `json:"pair_id,omitempty"`
}

// NewInput is the singleton form (no inverse).
type NewInput struct {
	ProjectID string `json:"project_id"`
	FromID    string `json:"from_id"`
	ToID      string `json:"to_id"`
	Label     string `json:"label"`
	Notes     string `json:"notes"`
}

// NewPairInput creates two rows in one transaction: (From→To label) and
// (To→From inverse_label), sharing one fresh pair_id.
type NewPairInput struct {
	ProjectID    string `json:"project_id"`
	FromID       string `json:"from_id"`
	ToID         string `json:"to_id"`
	Label        string `json:"label"`
	InverseLabel string `json:"inverse_label"`
	Notes        string `json:"notes"`
}

// UpdateInput patches a single row. The paired side keeps its own values.
type UpdateInput struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Notes string `json:"notes"`
}
```

- [ ] **Step 3: Implement `repo.go`**

`engine/internal/relationship/repo.go`:

```go
package relationship

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/google/uuid"
)

// ErrNotFound is returned when a relationship id does not exist.
var ErrNotFound = errors.New("relationship not found")

// Repo persists Relationships in SQLite.
type Repo struct {
	s *store.Store
}

func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

// CreateOne inserts a singleton row (pair_id = NULL).
func (r *Repo) CreateOne(ctx context.Context, in NewInput) (Relationship, error) {
	if in.ProjectID == "" || in.FromID == "" || in.ToID == "" || in.Label == "" {
		return Relationship{}, fmt.Errorf("create relationship: project_id, from_id, to_id, label required")
	}
	id := uuid.NewString()
	if _, err := r.s.DB().ExecContext(ctx, `
INSERT INTO relationships (id, project_id, from_id, to_id, label, notes, pair_id)
VALUES (?, ?, ?, ?, ?, ?, NULL)`,
		id, in.ProjectID, in.FromID, in.ToID, in.Label, in.Notes); err != nil {
		return Relationship{}, err
	}
	return r.Get(ctx, id)
}

// CreatePair inserts two rows in one transaction, sharing a fresh pair_id.
// Return order: [forward(A→B, Label), inverse(B→A, InverseLabel)].
func (r *Repo) CreatePair(ctx context.Context, in NewPairInput) ([]Relationship, error) {
	if in.ProjectID == "" || in.FromID == "" || in.ToID == "" ||
		in.Label == "" || in.InverseLabel == "" {
		return nil, fmt.Errorf("create pair: project_id, from_id, to_id, label, inverse_label required")
	}
	pairID := uuid.NewString()
	forwardID := uuid.NewString()
	inverseID := uuid.NewString()

	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
INSERT INTO relationships (id, project_id, from_id, to_id, label, notes, pair_id)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		forwardID, in.ProjectID, in.FromID, in.ToID, in.Label, in.Notes, pairID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO relationships (id, project_id, from_id, to_id, label, notes, pair_id)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		inverseID, in.ProjectID, in.ToID, in.FromID, in.InverseLabel, "", pairID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	forward, err := r.Get(ctx, forwardID)
	if err != nil {
		return nil, err
	}
	inverse, err := r.Get(ctx, inverseID)
	if err != nil {
		return nil, err
	}
	return []Relationship{forward, inverse}, nil
}

// Get returns one relationship by id.
func (r *Repo) Get(ctx context.Context, id string) (Relationship, error) {
	row := r.s.DB().QueryRowContext(ctx, baseSelect+` WHERE id = ?`, id)
	rel, err := scan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Relationship{}, ErrNotFound
	}
	return rel, err
}

// ListByEntity returns rows where from_id = entityID, ordered by id (UUID order
// is stable and close enough to insertion order for this scope).
func (r *Repo) ListByEntity(ctx context.Context, entityID string) ([]Relationship, error) {
	rows, err := r.s.DB().QueryContext(ctx,
		baseSelect+` WHERE from_id = ? ORDER BY id`, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Relationship
	for rows.Next() {
		rel, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rel)
	}
	return out, rows.Err()
}

// Update patches a single row. The paired side keeps its own label/notes.
func (r *Repo) Update(ctx context.Context, in UpdateInput) error {
	if in.ID == "" {
		return fmt.Errorf("update relationship: id required")
	}
	res, err := r.s.DB().ExecContext(ctx,
		`UPDATE relationships SET label = ?, notes = ? WHERE id = ?`,
		in.Label, in.Notes, in.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes the row. If pair_id is not NULL, both rows of the pair are
// removed in one transaction; otherwise only the one row is removed.
func (r *Repo) Delete(ctx context.Context, id string) error {
	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var pairID sql.NullString
	if err := tx.QueryRowContext(ctx,
		`SELECT pair_id FROM relationships WHERE id = ?`, id).Scan(&pairID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if pairID.Valid && pairID.String != "" {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM relationships WHERE pair_id = ?`, pairID.String); err != nil {
			return err
		}
	} else {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM relationships WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

const baseSelect = `
SELECT id, project_id, from_id, to_id, label, notes, pair_id
FROM relationships`

type scanner interface{ Scan(...any) error }

func scan(row scanner) (Relationship, error) {
	var (
		rel    Relationship
		pairID sql.NullString
	)
	if err := row.Scan(&rel.ID, &rel.ProjectID, &rel.FromID, &rel.ToID,
		&rel.Label, &rel.Notes, &pairID); err != nil {
		return Relationship{}, err
	}
	if pairID.Valid && pairID.String != "" {
		v := pairID.String
		rel.PairID = &v
	}
	return rel, nil
}
```

- [ ] **Step 4: Run tests**

```bash
cd engine && go test ./internal/relationship/... -race
```

All seven tests must pass.

- [ ] **Step 5: Commit**

```
git add engine/internal/relationship/
git commit -m "feat(relationship): Repo with CreateOne/CreatePair/Get/List/Update/Delete + atomic pair delete"
```

---

### Task 3: `relationships.*` RPC handlers (TDD)

Mirror the `threads.*` handler file shape verbatim — one exported function per RPC, each returning an `rpc.Handler`, decoding JSON into the repo's typed input, mapping `ErrNotFound` to `CodeInvalidParams`, everything else to `CodeInternalError`.

**Files:**
- Create: `engine/internal/rpc/handlers/relationships.go`
- Create: `engine/internal/rpc/handlers/relationships_test.go`
- Modify: `engine/cmd/linetta-engine/main.go` (import + 5 `s.Handle` lines)

- [ ] **Step 1: Failing tests**

`engine/internal/rpc/handlers/relationships_test.go`:

```go
package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/store"
)

type relFix struct {
	rr   *relationship.Repo
	pID  string
	a, b string
}

func newRelFixture(t *testing.T) relFix {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	pr := project.NewRepo(s)
	p, _ := pr.Create(context.Background(), 1, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first",
	})
	er := entity.NewRepo(s)
	a, _ := er.Create(context.Background(), 2, entity.NewInput{
		ProjectID: p.ID, Kind: entity.KindCharacter, Name: "해진",
	})
	b, _ := er.Create(context.Background(), 3, entity.NewInput{
		ProjectID: p.ID, Kind: entity.KindCharacter, Name: "아지",
	})
	return relFix{rr: relationship.NewRepo(s), pID: p.ID, a: a.ID, b: b.ID}
}

func TestCreateOneRelationshipHandler(t *testing.T) {
	f := newRelFixture(t)
	res, err := CreateOneRelationship(f.rr)(context.Background(),
		json.RawMessage(`{"project_id":"`+f.pID+`","from_id":"`+f.a+`","to_id":"`+f.b+`","label":"엄마"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var rel relationship.Relationship
	_ = json.Unmarshal(res, &rel)
	if rel.Label != "엄마" || rel.PairID != nil {
		t.Errorf("singleton mismatch: %+v", rel)
	}
}

func TestCreatePairRelationshipHandler(t *testing.T) {
	f := newRelFixture(t)
	res, err := CreatePairRelationship(f.rr)(context.Background(),
		json.RawMessage(`{"project_id":"`+f.pID+`","from_id":"`+f.a+`","to_id":"`+f.b+
			`","label":"부모","inverse_label":"자녀"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var rows []relationship.Relationship
	_ = json.Unmarshal(res, &rows)
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	if rows[0].PairID == nil || rows[1].PairID == nil || *rows[0].PairID != *rows[1].PairID {
		t.Errorf("pair_id not shared: %+v", rows)
	}
	if rows[0].Label != "부모" || rows[1].Label != "자녀" {
		t.Errorf("labels swapped wrong: %+v", rows)
	}
}

func TestListRelationshipsByEntityHandler(t *testing.T) {
	f := newRelFixture(t)
	_, _ = f.rr.CreatePair(context.Background(), relationship.NewPairInput{
		ProjectID: f.pID, FromID: f.a, ToID: f.b,
		Label: "친구", InverseLabel: "친구",
	})
	res, err := ListRelationshipsByEntity(f.rr)(context.Background(),
		json.RawMessage(`{"entity_id":"`+f.a+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var list []relationship.Relationship
	_ = json.Unmarshal(res, &list)
	if len(list) != 1 || list[0].FromID != f.a {
		t.Errorf("list = %+v", list)
	}
}

func TestUpdateRelationshipHandler(t *testing.T) {
	f := newRelFixture(t)
	rel, _ := f.rr.CreateOne(context.Background(), relationship.NewInput{
		ProjectID: f.pID, FromID: f.a, ToID: f.b, Label: "친구",
	})
	res, err := UpdateRelationship(f.rr)(context.Background(),
		json.RawMessage(`{"id":"`+rel.ID+`","label":"절친","notes":"메모"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got relationship.Relationship
	_ = json.Unmarshal(res, &got)
	if got.Label != "절친" || got.Notes != "메모" {
		t.Errorf("update missed: %+v", got)
	}
}

func TestDeleteRelationshipHandler_pair(t *testing.T) {
	f := newRelFixture(t)
	rows, _ := f.rr.CreatePair(context.Background(), relationship.NewPairInput{
		ProjectID: f.pID, FromID: f.a, ToID: f.b,
		Label: "친구", InverseLabel: "친구",
	})
	if _, err := DeleteRelationship(f.rr)(context.Background(),
		json.RawMessage(`{"id":"`+rows[0].ID+`"}`)); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := f.rr.Get(context.Background(), rows[1].ID); err == nil {
		t.Error("inverse row should be gone (atomic pair delete)")
	}
}
```

Run (won't compile yet):

```bash
cd engine && go test ./internal/rpc/handlers/... -run Relationship -race
```

- [ ] **Step 2: Implement handlers**

`engine/internal/rpc/handlers/relationships.go`:

```go
package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// CreateOneRelationship handles relationships.create_one (singleton, no inverse).
func CreateOneRelationship(repo *relationship.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in relationship.NewInput
		if err := json.Unmarshal(params, &in); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		if in.ProjectID == "" || in.FromID == "" || in.ToID == "" || in.Label == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams,
				Message: "project_id, from_id, to_id, label required"}
		}
		rel, err := repo.CreateOne(ctx, in)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(rel)
	}
}

// CreatePairRelationship handles relationships.create_pair (two rows, shared pair_id).
func CreatePairRelationship(repo *relationship.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in relationship.NewPairInput
		if err := json.Unmarshal(params, &in); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		if in.ProjectID == "" || in.FromID == "" || in.ToID == "" ||
			in.Label == "" || in.InverseLabel == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams,
				Message: "project_id, from_id, to_id, label, inverse_label required"}
		}
		rows, err := repo.CreatePair(ctx, in)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(rows)
	}
}

type listByEntityParams struct {
	EntityID string `json:"entity_id"`
}

// ListRelationshipsByEntity handles relationships.list_by_entity.
func ListRelationshipsByEntity(repo *relationship.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p listByEntityParams
		if err := json.Unmarshal(params, &p); err != nil || p.EntityID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "entity_id required"}
		}
		list, err := repo.ListByEntity(ctx, p.EntityID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if list == nil {
			list = []relationship.Relationship{}
		}
		return json.Marshal(list)
	}
}

// UpdateRelationship handles relationships.update (single row only).
func UpdateRelationship(repo *relationship.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in relationship.UpdateInput
		if err := json.Unmarshal(params, &in); err != nil || in.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := repo.Update(ctx, in); err != nil {
			if errors.Is(err, relationship.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "relationship not found"}
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

// DeleteRelationship handles relationships.delete (atomic pair delete if pair_id non-NULL).
func DeleteRelationship(repo *relationship.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := repo.Delete(ctx, p.ID); err != nil {
			if errors.Is(err, relationship.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "relationship not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(map[string]bool{"ok": true})
	}
}
```

- [ ] **Step 3: Run handler tests**

```bash
cd engine && go test ./internal/rpc/handlers/... -race
```

- [ ] **Step 4: Wire into `main.go`**

In `engine/cmd/linetta-engine/main.go`:

1. Add import after the existing `"github.com/devlikebear/linetta/engine/internal/project"` line:
   ```go
   "github.com/devlikebear/linetta/engine/internal/relationship"
   ```
2. After `beats := beat.NewRepo(st)` (around line 66), add:
   ```go
   relationships := relationship.NewRepo(st)
   ```
3. After the existing `beats.delete` handler registration (around line 126), append:
   ```go
   s.Handle("relationships.create_one", handlers.CreateOneRelationship(relationships))
   s.Handle("relationships.create_pair", handlers.CreatePairRelationship(relationships))
   s.Handle("relationships.list_by_entity", handlers.ListRelationshipsByEntity(relationships))
   s.Handle("relationships.update", handlers.UpdateRelationship(relationships))
   s.Handle("relationships.delete", handlers.DeleteRelationship(relationships))
   ```

- [ ] **Step 5: Full engine check**

```bash
cd engine && go test ./... -race && go build ./...
```

- [ ] **Step 6: Commit**

```
git add engine/internal/rpc/handlers/relationships.go engine/internal/rpc/handlers/relationships_test.go engine/cmd/linetta-engine/main.go
git commit -m "feat(rpc): relationships.create_one/create_pair/list_by_entity/update/delete handlers"
```

---

## Phase B: Frontend (2 tasks)

### Task 4: TS types + rpc namespace + presets

**Files:**
- Modify: `apps/desktop/src/lib/types.ts`
- Modify: `apps/desktop/src/lib/rpc.ts`
- Create: `apps/desktop/src/lib/relationshipPresets.ts`

- [ ] **Step 1: Add types**

Append to `apps/desktop/src/lib/types.ts`:

```ts
// Mirrors engine/internal/relationship Relationship struct.
export interface Relationship {
  id: string;
  project_id: string;
  from_id: string;
  to_id: string;
  label: string;
  notes: string;
  pair_id?: string;
}

export interface NewRelationshipInput {
  project_id: string;
  from_id: string;
  to_id: string;
  label: string;
  notes?: string;
}

export interface NewRelationshipPairInput {
  project_id: string;
  from_id: string;
  to_id: string;
  label: string;
  inverse_label: string;
  notes?: string;
}

export interface UpdateRelationshipInput {
  id: string;
  label: string;
  notes: string;
}
```

- [ ] **Step 2: Add the `relationships` namespace**

In `apps/desktop/src/lib/rpc.ts`, add `Relationship`, `NewRelationshipInput`, `NewRelationshipPairInput`, `UpdateRelationshipInput` to the type-only import block, then append at the end of the file:

```ts
export const relationships = {
  createOne: (input: NewRelationshipInput) =>
    rpcCall<Relationship>("relationships.create_one", input),
  createPair: (input: NewRelationshipPairInput) =>
    rpcCall<Relationship[]>("relationships.create_pair", input),
  listByEntity: (entityId: string) =>
    rpcCall<Relationship[]>("relationships.list_by_entity", { entity_id: entityId }),
  update: (input: UpdateRelationshipInput) =>
    rpcCall<Relationship>("relationships.update", input),
  delete: (id: string) => rpcCall<{ ok: true }>("relationships.delete", { id }),
};
```

- [ ] **Step 3: Add the presets file**

Create `apps/desktop/src/lib/relationshipPresets.ts`:

```ts
// Chip suggestions shown above the label inputs in RelationshipPicker.
// Click fills the input; freeform editing remains allowed.
export const LABEL_PRESETS = [
  "연인",
  "부모",
  "자녀",
  "친구",
  "적수",
  "동료",
  "형제",
] as const;
```

- [ ] **Step 4: Verify build**

```bash
cd apps/desktop && pnpm tsc -b
```

- [ ] **Step 5: Commit**

```
git add apps/desktop/src/lib/types.ts apps/desktop/src/lib/rpc.ts apps/desktop/src/lib/relationshipPresets.ts
git commit -m "feat(desktop): typed RPC bindings for relationships + label preset chips"
```

---

### Task 5: `RelationshipPicker` component + `EntitySheet` "관계" section replacement

This is the biggest frontend task: a popover for picking a target entity + labels, and the in-sheet list that replaces the existing placeholder. Note: there is no vitest in the desktop app, so verification is purely `pnpm tsc -b`.

**Files:**
- Create: `apps/desktop/src/components/RelationshipPicker.tsx`
- Create: `apps/desktop/src/components/RelationshipPicker.css`
- Modify: `apps/desktop/src/components/EntitySheet.tsx`
- Modify: `apps/desktop/src/components/EntitySheet.css`

- [ ] **Step 1: Create the picker**

`apps/desktop/src/components/RelationshipPicker.tsx` — full file:

```tsx
import { useEffect, useRef, useState } from "react";
import { entities, relationships } from "../lib/rpc";
import { LABEL_PRESETS } from "../lib/relationshipPresets";
import type { Entity } from "../lib/types";
import "./RelationshipPicker.css";

interface Props {
  projectId: string;
  fromEntityId: string;
  // Entities to hide from the search results (typically [fromEntityId]).
  excludeIds?: string[];
  onClose: () => void;
  onCreated: () => void; // parent refreshes the list
}

export function RelationshipPicker({
  projectId,
  fromEntityId,
  excludeIds = [],
  onClose,
  onCreated,
}: Props) {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<Entity[]>([]);
  const [target, setTarget] = useState<Entity | null>(null);
  const [label, setLabel] = useState("");
  const [inverseLabel, setInverseLabel] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const debounceRef = useRef<number | null>(null);

  const hide = new Set([fromEntityId, ...excludeIds]);

  useEffect(() => {
    if (debounceRef.current) window.clearTimeout(debounceRef.current);
    debounceRef.current = window.setTimeout(async () => {
      try {
        const list = await entities.search(projectId, query, 20);
        setResults(list.filter((e) => !hide.has(e.id)));
      } catch (e) {
        setError(String(e));
      }
    }, 200);
    return () => {
      if (debounceRef.current) window.clearTimeout(debounceRef.current);
    };
  }, [query, projectId, fromEntityId]);

  const onSave = async () => {
    if (!target || label.trim() === "") return;
    setSaving(true);
    setError(null);
    try {
      if (inverseLabel.trim() === "") {
        await relationships.createOne({
          project_id: projectId,
          from_id: fromEntityId,
          to_id: target.id,
          label: label.trim(),
        });
      } else {
        await relationships.createPair({
          project_id: projectId,
          from_id: fromEntityId,
          to_id: target.id,
          label: label.trim(),
          inverse_label: inverseLabel.trim(),
        });
      }
      onCreated();
      onClose();
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="rel-picker-backdrop" onMouseDown={onClose}>
      <div
        className="rel-picker"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <header className="rel-picker-head">
          <span>관계 추가</span>
          <button
            type="button"
            className="rel-picker-close"
            onClick={onClose}
            aria-label="닫기"
          >×</button>
        </header>

        {error && <p className="rel-picker-error">{error}</p>}

        <section className="rel-picker-section">
          <h6>대상</h6>
          {!target ? (
            <>
              <input
                className="rel-picker-search"
                placeholder="엔티티 이름 검색"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                autoFocus
              />
              <div className="rel-picker-results">
                {results.length === 0 && (
                  <p className="rel-picker-empty">결과 없음</p>
                )}
                {results.map((e) => (
                  <button
                    type="button"
                    key={e.id}
                    className="rel-picker-result"
                    onClick={() => setTarget(e)}
                  >
                    <span className="rel-picker-result-name">{e.name}</span>
                    {e.role && (
                      <span className="rel-picker-result-role">{e.role}</span>
                    )}
                  </button>
                ))}
              </div>
            </>
          ) : (
            <div className="rel-picker-target">
              <span className="rel-picker-target-name">{target.name}</span>
              <button
                type="button"
                className="rel-picker-change"
                onClick={() => setTarget(null)}
              >변경</button>
            </div>
          )}
        </section>

        <section className="rel-picker-section">
          <h6>관계 (이쪽 → 상대)</h6>
          <div className="rel-picker-chips">
            {LABEL_PRESETS.map((p) => (
              <button
                key={p}
                type="button"
                className={`rel-picker-chip${label === p ? " active" : ""}`}
                onClick={() => setLabel(p)}
              >{p}</button>
            ))}
          </div>
          <input
            className="rel-picker-label"
            placeholder="예: 친구"
            value={label}
            onChange={(e) => setLabel(e.target.value)}
          />
        </section>

        <section className="rel-picker-section">
          <h6>역방향 라벨 (선택)</h6>
          <p className="rel-picker-hint">
            비워두면 단방향 한 줄만 저장됩니다. 입력하면 상대 쪽에도 자동으로 추가됩니다.
          </p>
          <div className="rel-picker-chips">
            {LABEL_PRESETS.map((p) => (
              <button
                key={p}
                type="button"
                className={`rel-picker-chip${inverseLabel === p ? " active" : ""}`}
                onClick={() => setInverseLabel(p)}
              >{p}</button>
            ))}
          </div>
          <input
            className="rel-picker-label"
            placeholder="예: 친구 (또는 비워두기)"
            value={inverseLabel}
            onChange={(e) => setInverseLabel(e.target.value)}
          />
        </section>

        <div className="rel-picker-actions">
          <button type="button" onClick={onClose} disabled={saving}>취소</button>
          <button
            type="button"
            className="primary"
            disabled={saving || !target || label.trim() === ""}
            onClick={onSave}
          >{saving ? "저장 중…" : "저장"}</button>
        </div>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Create the picker styles**

`apps/desktop/src/components/RelationshipPicker.css`:

```css
.rel-picker-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(20, 20, 20, 0.25);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 50;
}

.rel-picker {
  width: min(440px, 92vw);
  max-height: 86vh;
  background: #faf9f6;
  border: 1px solid #d8d6cf;
  border-radius: 8px;
  box-shadow: 0 12px 40px rgba(0, 0, 0, 0.15);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.rel-picker-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.85rem 1rem;
  border-bottom: 1px solid #ece9e0;
  font-size: 0.75rem;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: #6b6b6b;
}

.rel-picker-close {
  background: none;
  border: none;
  color: #9a9a9a;
  font-size: 1.2rem;
  line-height: 1;
  cursor: pointer;
}
.rel-picker-close:hover { color: #1a1a1a; }

.rel-picker-error {
  color: #a8312f;
  margin: 0;
  padding: 0.5rem 1rem;
  font-size: 0.85rem;
}

.rel-picker-section {
  padding: 0.75rem 1rem;
  border-bottom: 1px solid #ece9e0;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}
.rel-picker-section:last-of-type { border-bottom: none; }
.rel-picker-section h6 {
  margin: 0;
  font-size: 0.72rem;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: #6b6b6b;
}

.rel-picker-search,
.rel-picker-label {
  font: inherit;
  font-size: 0.9rem;
  padding: 0.4rem 0.55rem;
  border: 1px solid #d8d6cf;
  border-radius: 4px;
  background: white;
  width: 100%;
}

.rel-picker-results {
  max-height: 180px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
}
.rel-picker-result {
  display: flex;
  justify-content: space-between;
  text-align: left;
  font: inherit;
  font-size: 0.9rem;
  padding: 0.4rem 0.55rem;
  background: white;
  border: 1px solid #ece9e0;
  border-radius: 4px;
  margin-bottom: 0.25rem;
  cursor: pointer;
}
.rel-picker-result:hover { background: #f3f1ea; }
.rel-picker-result-role {
  font-size: 0.78rem;
  color: #9a9a9a;
}
.rel-picker-empty { color: #9a9a9a; margin: 0.25rem 0; font-size: 0.85rem; }

.rel-picker-target {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.4rem 0.55rem;
  background: white;
  border: 1px solid #d8d6cf;
  border-radius: 4px;
}
.rel-picker-target-name { font-size: 0.95rem; }
.rel-picker-change {
  font: inherit;
  font-size: 0.8rem;
  padding: 0.2rem 0.55rem;
  background: transparent;
  border: 1px solid #d8d6cf;
  border-radius: 999px;
  cursor: pointer;
  color: #555;
}

.rel-picker-chips { display: flex; flex-wrap: wrap; gap: 0.3rem; }
.rel-picker-chip {
  font: inherit;
  font-size: 0.82rem;
  padding: 0.2rem 0.7rem;
  border: 1px solid #d8d6cf;
  border-radius: 999px;
  background: white;
  color: #333;
  cursor: pointer;
}
.rel-picker-chip:hover { background: #f3f1ea; }
.rel-picker-chip.active {
  background: #1a1a1a;
  color: #faf9f6;
  border-color: #1a1a1a;
}

.rel-picker-hint {
  margin: 0;
  font-size: 0.78rem;
  color: #6b6b6b;
}

.rel-picker-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.4rem;
  padding: 0.75rem 1rem;
  border-top: 1px solid #ece9e0;
}
.rel-picker-actions button {
  font: inherit;
  font-size: 0.9rem;
  padding: 0.4rem 1rem;
  background: white;
  border: 1px solid #1a1a1a;
  border-radius: 4px;
  cursor: pointer;
}
.rel-picker-actions button.primary {
  background: #1a1a1a;
  color: #faf9f6;
}
.rel-picker-actions button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
```

- [ ] **Step 3: Replace the placeholder section in `EntitySheet.tsx`**

The current file `apps/desktop/src/components/EntitySheet.tsx` at lines **156–159** has:

```tsx
          <section className="entity-section relations">
            <h5>관계</h5>
            <p className="entity-empty">(post-MVP)</p>
          </section>
```

Replace those four lines with the real section below. Also (a) extend the import block to bring in `Relationship` + `relationships` + the new component, (b) add state for the row list, sheet refresh, and picker visibility.

**Imports — replace the top of `EntitySheet.tsx`:**

Before:

```tsx
import { useEffect, useState } from "react";
import type { Entity, EntityKind, UpdateEntityInput } from "../lib/types";
import { entities } from "../lib/rpc";
import "./EntitySheet.css";
```

After:

```tsx
import { useCallback, useEffect, useState } from "react";
import type { Entity, EntityKind, Relationship, UpdateEntityInput } from "../lib/types";
import { entities, relationships } from "../lib/rpc";
import { RelationshipPicker } from "./RelationshipPicker";
import "./EntitySheet.css";
```

**State additions — inside `EntitySheet`, alongside the existing `useState` hooks:**

Add right after `const [error, setError] = useState<string | null>(null);`:

```tsx
  const [rels, setRels] = useState<Relationship[]>([]);
  const [relTargets, setRelTargets] = useState<Record<string, Entity>>({});
  const [pickerOpen, setPickerOpen] = useState(false);

  const refreshRels = useCallback(async (eid: string) => {
    const list = await relationships.listByEntity(eid);
    setRels(list);
    // Hydrate target names (skip duplicates).
    const need = Array.from(new Set(list.map((r) => r.to_id))).filter(
      (id) => !relTargets[id],
    );
    if (need.length === 0) return;
    const fetched = await Promise.all(need.map((id) => entities.get(id)));
    setRelTargets((cur) => {
      const next = { ...cur };
      for (const e of fetched) next[e.id] = e;
      return next;
    });
  }, [relTargets]);
```

**Existing effect — extend to also load rels.** Replace the existing `useEffect` body (around lines 26–42) with:

```tsx
  useEffect(() => {
    if (!entityId) return;
    setEntity(null);
    setError(null);
    setRels([]);
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
    refreshRels(entityId).catch((e) => setError(String(e)));
  }, [entityId, refreshRels]);
```

**Section replacement — lines 156–159.** Replace the four-line placeholder with:

```tsx
          <section className="entity-section relations">
            <h5>관계</h5>
            {rels.length === 0 && (
              <p className="entity-empty">아직 관계가 없습니다.</p>
            )}
            {rels.length > 0 && (
              <ul className="relation-list">
                {rels.map((r) => {
                  const target = relTargets[r.to_id];
                  return (
                    <li className="relation-row" key={r.id}>
                      <span className="relation-target">
                        {target ? target.name : r.to_id.slice(0, 6)}
                      </span>
                      <span className="relation-dash"> — </span>
                      <span className="relation-label">{r.label}</span>
                      <button
                        type="button"
                        className="relation-del"
                        aria-label="삭제"
                        onClick={async () => {
                          await relationships.delete(r.id);
                          if (entity) await refreshRels(entity.id);
                        }}
                      >×</button>
                    </li>
                  );
                })}
              </ul>
            )}
            <button
              type="button"
              className="relation-add"
              onClick={() => setPickerOpen(true)}
            >+ 관계 추가</button>
            {pickerOpen && entity && (
              <RelationshipPicker
                projectId={entity.project_id}
                fromEntityId={entity.id}
                onClose={() => setPickerOpen(false)}
                onCreated={() => {
                  if (entity) refreshRels(entity.id);
                }}
              />
            )}
          </section>
```

- [ ] **Step 4: Add the small list styles**

Append to `apps/desktop/src/components/EntitySheet.css`:

```css
.relation-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}
.relation-row {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.3rem 0.5rem;
  background: white;
  border: 1px solid #ece9e0;
  border-radius: 4px;
  font-size: 0.9rem;
}
.relation-target { font-weight: 500; }
.relation-dash { color: #9a9a9a; }
.relation-label { color: #333; flex: 1; }
.relation-del {
  background: none;
  border: 1px solid #d8d6cf;
  border-radius: 4px;
  color: #9a9a9a;
  cursor: pointer;
  padding: 0 0.4rem;
}
.relation-del:hover { color: #1a1a1a; }
.relation-add {
  font: inherit;
  font-size: 0.85rem;
  align-self: flex-start;
  border: 1px dashed #c8c5bd;
  border-radius: 999px;
  padding: 0.25rem 0.7rem;
  background: transparent;
  cursor: pointer;
  color: #555;
  margin-top: 0.4rem;
}
```

- [ ] **Step 5: Verify typecheck + build**

```bash
cd apps/desktop && pnpm tsc -b && pnpm build
```

Then make sure the Tauri shell still compiles (no engine binding shape changed, but the embedded engine binary did — Tauri side just shells out via `engine_call`, no FFI change):

```bash
cd apps/desktop/src-tauri && cargo check
```

- [ ] **Step 6: Commit**

```
git add apps/desktop/src/components/RelationshipPicker.tsx apps/desktop/src/components/RelationshipPicker.css apps/desktop/src/components/EntitySheet.tsx apps/desktop/src/components/EntitySheet.css
git commit -m "feat(desktop): EntitySheet 관계 section with RelationshipPicker + auto bidirectional sync"
```

---

## Phase C: Smoke + tag (1 task)

### Task 6: Manual walk-through + tag

No automated end-to-end harness exists for the desktop app (verified — no vitest, no Playwright suite). The smoke check exercises the whole stack via the actual UI plus a single sqlite read at the end.

- [ ] **Step 1: Boot a clean app**

```bash
cd apps/desktop && pnpm tauri dev
```

- [ ] **Step 2: Set up the fixtures**

In the running app:
1. Open or create a project.
2. Create two character entities (any node, via the mention picker or directly via the EntitySheet):
   - A = `해진`
   - B = `아지`

- [ ] **Step 3: Paired relationship**

1. Open the EntitySheet for `해진`.
2. Click `+ 관계 추가`.
3. In the picker: search "아지" → click the result row.
4. For label, click the `친구` chip.
5. For inverse label, click the `친구` chip.
6. Click 저장.
7. **Assert:** the 관계 section on `해진`'s sheet now shows `아지 — 친구` with a `×` delete button.
8. Close the sheet. Open the EntitySheet for `아지`.
9. **Assert:** the 관계 section shows `해진 — 친구` (the auto-created inverse).

- [ ] **Step 4: Atomic pair delete**

1. From `해진`'s side, click the `×` next to `아지 — 친구`.
2. **Assert:** the row disappears from `해진`'s sheet.
3. Open `아지`'s sheet.
4. **Assert:** `해진 — 친구` is gone too — the partner row was cascaded.

- [ ] **Step 5: Asymmetric singleton**

1. On `해진`'s sheet, click `+ 관계 추가`.
2. Pick `아지` as target.
3. Type `엄마` into the label input. **Leave the inverse label empty.**
4. Save.
5. **Assert:** `해진`'s sheet now shows `아지 — 엄마`.
6. Open `아지`'s sheet.
7. **Assert:** the 엄마 row is **NOT** present (singletons are unidirectional).

- [ ] **Step 6: Inspect the DB**

```bash
sqlite3 "$LINETTA_HOME/library.db" "SELECT from_id, to_id, label, pair_id FROM relationships;"
```

**Assert:**
- Paired rows (created in Step 3 before deletion) — if you recreate one, two rows share a non-NULL `pair_id`.
- The asymmetric `엄마` singleton from Step 5 has `pair_id = NULL`.
- Schema includes the new column: `sqlite3 "$LINETTA_HOME/library.db" "PRAGMA table_info(relationships);" | grep pair_id` returns one line.
- Indexes present: `sqlite3 "$LINETTA_HOME/library.db" ".indexes relationships"` lists `idx_relationships_from` and `idx_relationships_pair`.

- [ ] **Step 7: Quit dev mode, commit nothing**

The smoke task has no code changes.

- [ ] **Step 8: Tag the plan**

```
git tag plan-8-relationships-done
git log -1 --decorate=short
```

---

## Done conditions

- [ ] `cd engine && go test ./... -race` green.
- [ ] `cd apps/desktop && pnpm tsc -b && pnpm build` green.
- [ ] `cd apps/desktop/src-tauri && cargo check` green.
- [ ] Manual walk-through (Phase C steps 3–6) all asserts hold.
- [ ] `plan-8-relationships-done` tag exists on the latest commit.

---

### Critical Files for Implementation

- /Users/changheonshin/workspace/myworks/linetta/engine/internal/store/migrations/0003_relationship_pair_id.sql
- /Users/changheonshin/workspace/myworks/linetta/engine/internal/relationship/repo.go
- /Users/changheonshin/workspace/myworks/linetta/engine/internal/rpc/handlers/relationships.go
- /Users/changheonshin/workspace/myworks/linetta/apps/desktop/src/components/RelationshipPicker.tsx
- /Users/changheonshin/workspace/myworks/linetta/apps/desktop/src/components/EntitySheet.tsx
