# Plan 7 — Thread + Beat (스토리라인 마디 추적)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the first post-MVP feature — Thread (스토리라인) and its ordered Beats — so a writer can mark which storylines run through each scene, see the active threads on the right of the workspace, jump between scenes that share a thread from a dedicated Thread View, and have those storylines auto-injected into the AI context as `## 활성 스토리라인`.

**Architecture:** Two new engine packages (`thread`, `beat`) modeled on the existing `entity` package shape. The schema already exists in `0001_init.sql` so no DDL migration is required; **a 0002 migration adds two read-pattern indexes** because every UI query traverses `beats.thread_id`/`beats.node_id`. The `ai.ContextBuilder` learns to load all open threads that have at least one beat bound to the current node and emits them as a new prompt section. The frontend grows a `ThreadSheet` (mirroring `EntitySheet`), an `ActiveThreadsPanel` collapsible section in the workspace right rail, and a new `/workspace/:projectId/threads` route rendering vertical thread lanes. Two new Cmd+K commands wire it all up; the disabled `흐름(Thread)` palette entry is replaced with a real route.

**Tech Stack additions:** None. Pure Go + React work on the existing stack (Tauri 2, modernc.org/sqlite, React 18 + react-router-dom, Tiptap unchanged).

**Spec reference:** `docs/superpowers/specs/2026-05-25-linetta-immersive-rebuild-design.md` — §3 (Data Model: `threads` + `beats`), §4.9 (Thread View 04D), §5.4 step 3 ("Active threads" in AI context), §10 (privacy — context rides in `ai_runs.context_json`), §11.2 P1.

**Design decisions locked by the user:**
1. Beat ↔ Node = 1:N — one node may carry beats from multiple different threads.
2. AI "active threads" = ALL open threads that have ≥1 beat bound to the current node (auto, no picker). Cap ~1500 chars; up to 5 most-recent beats per thread.
3. Thread View = vertical lanes (one per thread). Beats are discs sized by `intensity` (1–3), colored by `threads.color`. Click → jump to bound node. Beats with NULL `node_id` (scene was deleted) render greyed and are non-clickable.

---

## Pre-flight

- [ ] Plan 6 MVP completion is tagged (`plan-6-mvp-completion-done`) and `git status --short` is empty.
- [ ] `cd engine && go test ./... -race` green.
- [ ] `cd apps/desktop && pnpm tsc -b && pnpm build` green.
- [ ] `cd apps/desktop/src-tauri && cargo check` green.
- [ ] Confirm `threads` and `beats` tables exist by running `sqlite3 "$LINETTA_HOME/library.db" ".schema threads" ".schema beats"`.

---

## File Structure (created or modified)

```
engine/internal/thread/
  thread.go                          (new — Thread struct + inputs)
  repo.go                            (new — CRUD + Close/Reopen)
  repo_test.go                       (new)

engine/internal/beat/
  beat.go                            (new — Beat struct + inputs)
  repo.go                            (new — CRUD + ordinal allocation)
  repo_test.go                       (new)

engine/internal/store/migrations/
  0002_thread_beat_indexes.sql       (new — two indexes)

engine/internal/rpc/handlers/
  threads.go                         (new — 6 handlers)
  threads_test.go                    (new)
  beats.go                           (new — 6 handlers)
  beats_test.go                      (new)

engine/internal/ai/
  ai.go                              (modified — Context.ActiveThreads + ActiveThread + BeatBrief)
  context.go                         (modified — load + cap active threads)
  context_test.go                    (modified — covers active-thread loading)
  prompts.go                         (modified — `## 활성 스토리라인` section)
  prompts_test.go                    (modified — assert new section)

engine/cmd/linetta-engine/main.go    (modified — repos + handler registration + ContextBuilder wiring)

apps/desktop/src/
  lib/types.ts                       (modified — Thread, Beat, BeatBrief, ActiveThread)
  lib/rpc.ts                         (modified — threads + beats namespaces)
  components/ThreadSheet.tsx         (new)
  components/ThreadSheet.css         (new)
  components/ActiveThreadsPanel.tsx  (new — replaces "활성 Thread" placeholder section)
  components/ContextPanel.tsx        (modified — render ActiveThreadsPanel inline)
  routes/ThreadView.tsx              (new)
  routes/ThreadView.css              (new)
  routes/Workspace.tsx               (modified — sheet state, Cmd+K entries, panel wiring)
  App.tsx                            (modified — `/workspace/:projectId/threads` route)
```

---

## Phase A: Engine — thread package

### Task 1: `engine/internal/thread` package (TDD)

A direct port of the entity package shape. `Thread` mirrors the SQLite row. `Repo` exposes `Create`, `Get`, `ListByProject` (excludes closed by default, with an `IncludeClosed` flag), `Update` (partial: name/color/summary), `Close`, `Reopen`. `Close` sets `closed_at = now`; `Reopen` sets it to NULL.

**Files:**
- Create: `engine/internal/thread/thread.go`
- Create: `engine/internal/thread/repo.go`
- Create: `engine/internal/thread/repo_test.go`

- [ ] **Step 1: Failing test**

`engine/internal/thread/repo_test.go`:

```go
package thread

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
	th, err := r.Create(ctx, NewInput{ProjectID: p.ID, Name: "잃어버린 시간", Color: "#c08a3e"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if th.ID == "" || th.Name != "잃어버린 시간" || th.Color != "#c08a3e" {
		t.Errorf("unexpected thread: %+v", th)
	}
	got, err := r.Get(ctx, th.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "잃어버린 시간" || got.ClosedAt != nil {
		t.Errorf("Get = %+v", got)
	}
}

func TestRepo_Create_defaultsColorWhenEmpty(t *testing.T) {
	s, p := openStoreAndProject(t)
	r := NewRepo(s)
	th, _ := r.Create(context.Background(), NewInput{ProjectID: p.ID, Name: "T"})
	if th.Color != "#666" {
		t.Errorf("default color = %q, want #666", th.Color)
	}
}

func TestRepo_ListByProject_excludesClosedByDefault(t *testing.T) {
	s, p := openStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()
	open, _ := r.Create(ctx, NewInput{ProjectID: p.ID, Name: "열린"})
	closed, _ := r.Create(ctx, NewInput{ProjectID: p.ID, Name: "닫힌"})
	if err := r.Close(ctx, closed.ID, 5000); err != nil {
		t.Fatalf("Close: %v", err)
	}

	got, err := r.ListByProject(ctx, p.ID, false)
	if err != nil {
		t.Fatalf("ListByProject: %v", err)
	}
	if len(got) != 1 || got[0].ID != open.ID {
		t.Errorf("open-only = %+v", got)
	}

	all, _ := r.ListByProject(ctx, p.ID, true)
	if len(all) != 2 {
		t.Errorf("include-closed got %d, want 2", len(all))
	}
}

func TestRepo_Update_partial(t *testing.T) {
	s, p := openStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()
	th, _ := r.Create(ctx, NewInput{ProjectID: p.ID, Name: "원본"})
	if err := r.Update(ctx, UpdateInput{ID: th.ID, Name: "수정됨", Summary: "요약 한 줄"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := r.Get(ctx, th.ID)
	if got.Name != "수정됨" || got.Summary != "요약 한 줄" || got.Color != "#666" {
		t.Errorf("update missed: %+v", got)
	}
}

func TestRepo_CloseAndReopen(t *testing.T) {
	s, p := openStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()
	th, _ := r.Create(ctx, NewInput{ProjectID: p.ID, Name: "T"})
	if err := r.Close(ctx, th.ID, 5000); err != nil {
		t.Fatalf("Close: %v", err)
	}
	closed, _ := r.Get(ctx, th.ID)
	if closed.ClosedAt == nil || *closed.ClosedAt != 5000 {
		t.Errorf("ClosedAt = %v", closed.ClosedAt)
	}
	if err := r.Reopen(ctx, th.ID); err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	reopened, _ := r.Get(ctx, th.ID)
	if reopened.ClosedAt != nil {
		t.Errorf("ClosedAt after reopen = %v", reopened.ClosedAt)
	}
}

func TestRepo_Get_notFound(t *testing.T) {
	s, _ := openStoreAndProject(t)
	r := NewRepo(s)
	if _, err := r.Get(context.Background(), "no-such-id"); err == nil {
		t.Error("expected ErrNotFound")
	}
}
```

- [ ] **Step 2: Run — failures**

```bash
cd engine && go test ./internal/thread/...
```

- [ ] **Step 3: Implement**

`engine/internal/thread/thread.go`:

```go
// Package thread owns Thread (스토리라인) domain logic. The schema already
// exists in 0001_init.sql; this package adds the Go layer.
package thread

// Thread mirrors the SQLite row. ClosedAt is nil while the thread is open.
type Thread struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	Summary   string `json:"summary"`
	ClosedAt  *int64 `json:"closed_at,omitempty"`
}

// NewInput is what `threads.create` accepts.
type NewInput struct {
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
}

// UpdateInput holds a partial patch. Empty strings leave fields alone.
type UpdateInput struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Color   string `json:"color"`
	Summary string `json:"summary"`
}
```

`engine/internal/thread/repo.go`:

```go
package thread

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/google/uuid"
)

// ErrNotFound is returned when a thread id does not exist.
var ErrNotFound = errors.New("thread not found")

// Repo persists Threads in SQLite.
type Repo struct {
	s *store.Store
}

func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

// Create inserts a new thread. Empty color defaults to #666.
func (r *Repo) Create(ctx context.Context, in NewInput) (Thread, error) {
	if in.ProjectID == "" || in.Name == "" {
		return Thread{}, fmt.Errorf("create thread: project_id and name required")
	}
	color := in.Color
	if color == "" {
		color = "#666"
	}
	id := uuid.NewString()
	if _, err := r.s.DB().ExecContext(ctx, `
INSERT INTO threads (id, project_id, name, color, summary, closed_at)
VALUES (?, ?, ?, ?, '', NULL)`, id, in.ProjectID, in.Name, color); err != nil {
		return Thread{}, err
	}
	return r.Get(ctx, id)
}

// Get returns one thread by id.
func (r *Repo) Get(ctx context.Context, id string) (Thread, error) {
	row := r.s.DB().QueryRowContext(ctx, baseSelect+` WHERE id = ?`, id)
	th, err := scan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Thread{}, ErrNotFound
	}
	return th, err
}

// ListByProject returns threads ordered by name. When includeClosed is false,
// rows with a non-null closed_at are filtered out.
func (r *Repo) ListByProject(ctx context.Context, projectID string, includeClosed bool) ([]Thread, error) {
	q := baseSelect + ` WHERE project_id = ?`
	if !includeClosed {
		q += ` AND closed_at IS NULL`
	}
	q += ` ORDER BY name COLLATE NOCASE`
	rows, err := r.s.DB().QueryContext(ctx, q, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Thread
	for rows.Next() {
		th, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, th)
	}
	return out, rows.Err()
}

// Update applies a partial input. Empty strings leave fields alone.
func (r *Repo) Update(ctx context.Context, in UpdateInput) error {
	if in.ID == "" {
		return fmt.Errorf("update thread: id required")
	}
	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	row := tx.QueryRowContext(ctx, baseSelect+` WHERE id = ?`, in.ID)
	cur, err := scan(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if in.Name != "" {
		cur.Name = in.Name
	}
	if in.Color != "" {
		cur.Color = in.Color
	}
	cur.Summary = in.Summary
	if _, err := tx.ExecContext(ctx, `
UPDATE threads SET name = ?, color = ?, summary = ? WHERE id = ?`,
		cur.Name, cur.Color, cur.Summary, in.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// Close stamps closed_at on the row.
func (r *Repo) Close(ctx context.Context, id string, now int64) error {
	res, err := r.s.DB().ExecContext(ctx, `UPDATE threads SET closed_at = ? WHERE id = ?`, now, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Reopen clears closed_at on the row.
func (r *Repo) Reopen(ctx context.Context, id string) error {
	res, err := r.s.DB().ExecContext(ctx, `UPDATE threads SET closed_at = NULL WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

const baseSelect = `
SELECT id, project_id, name, color, summary, closed_at
FROM threads`

type scanner interface{ Scan(...any) error }

func scan(row scanner) (Thread, error) {
	var (
		th     Thread
		closed sql.NullInt64
	)
	if err := row.Scan(&th.ID, &th.ProjectID, &th.Name, &th.Color, &th.Summary, &closed); err != nil {
		return Thread{}, err
	}
	if closed.Valid {
		v := closed.Int64
		th.ClosedAt = &v
	}
	return th, nil
}
```

- [ ] **Step 4: Run — green**

```bash
cd engine && go test ./internal/thread/... -race
```

- [ ] **Step 5: Commit**

```bash
git add engine/internal/thread/
git commit -m "feat(thread): Thread repo with Create/Get/List/Update/Close/Reopen"
```

---

### Task 2: `threads.*` RPC handlers + main.go wiring (TDD)

Six handlers mirroring the entity handler shape. Use the existing `Clock` and `idParam` types from `handlers/projects.go`.

**Files:**
- Create: `engine/internal/rpc/handlers/threads.go`
- Create: `engine/internal/rpc/handlers/threads_test.go`
- Modify: `engine/cmd/linetta-engine/main.go`

- [ ] **Step 1: Failing test**

`engine/internal/rpc/handlers/threads_test.go`:

```go
package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

type threadFix struct {
	store *store.Store
	tr    *thread.Repo
	pID   string
}

func newThreadFixture(t *testing.T) threadFix {
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
	return threadFix{store: s, tr: thread.NewRepo(s), pID: p.ID}
}

func TestCreateThreadHandler(t *testing.T) {
	f := newThreadFixture(t)
	res, err := CreateThread(f.tr)(context.Background(),
		json.RawMessage(`{"project_id":"`+f.pID+`","name":"잃어버린 시간","color":"#c08a3e"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var th thread.Thread
	_ = json.Unmarshal(res, &th)
	if th.Name != "잃어버린 시간" || th.Color != "#c08a3e" {
		t.Errorf("thread = %+v", th)
	}
}

func TestListThreadsHandler_filtersClosed(t *testing.T) {
	f := newThreadFixture(t)
	open, _ := f.tr.Create(context.Background(), thread.NewInput{ProjectID: f.pID, Name: "열린"})
	closed, _ := f.tr.Create(context.Background(), thread.NewInput{ProjectID: f.pID, Name: "닫힌"})
	_ = f.tr.Close(context.Background(), closed.ID, 1000)

	res, err := ListThreads(f.tr)(context.Background(),
		json.RawMessage(`{"project_id":"`+f.pID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var list []thread.Thread
	_ = json.Unmarshal(res, &list)
	if len(list) != 1 || list[0].ID != open.ID {
		t.Errorf("default list = %+v", list)
	}

	res2, _ := ListThreads(f.tr)(context.Background(),
		json.RawMessage(`{"project_id":"`+f.pID+`","include_closed":true}`))
	var all []thread.Thread
	_ = json.Unmarshal(res2, &all)
	if len(all) != 2 {
		t.Errorf("include_closed = %d", len(all))
	}
}

func TestUpdateThreadHandler(t *testing.T) {
	f := newThreadFixture(t)
	th, _ := f.tr.Create(context.Background(), thread.NewInput{ProjectID: f.pID, Name: "원본"})
	res, err := UpdateThread(f.tr)(context.Background(),
		json.RawMessage(`{"id":"`+th.ID+`","name":"새 이름","summary":"한 줄"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got thread.Thread
	_ = json.Unmarshal(res, &got)
	if got.Name != "새 이름" || got.Summary != "한 줄" {
		t.Errorf("update missed: %+v", got)
	}
}

func TestCloseAndReopenHandlers(t *testing.T) {
	f := newThreadFixture(t)
	th, _ := f.tr.Create(context.Background(), thread.NewInput{ProjectID: f.pID, Name: "T"})
	if _, err := CloseThread(f.tr, func() int64 { return 2000 })(context.Background(),
		json.RawMessage(`{"id":"`+th.ID+`"}`)); err != nil {
		t.Fatalf("close: %v", err)
	}
	got, _ := f.tr.Get(context.Background(), th.ID)
	if got.ClosedAt == nil || *got.ClosedAt != 2000 {
		t.Errorf("ClosedAt = %v", got.ClosedAt)
	}
	if _, err := ReopenThread(f.tr)(context.Background(),
		json.RawMessage(`{"id":"`+th.ID+`"}`)); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got2, _ := f.tr.Get(context.Background(), th.ID)
	if got2.ClosedAt != nil {
		t.Errorf("ClosedAt after reopen = %v", got2.ClosedAt)
	}
}

func TestGetThreadHandler_notFound(t *testing.T) {
	f := newThreadFixture(t)
	_, err := GetThread(f.tr)(context.Background(), json.RawMessage(`{"id":"missing"}`))
	if err == nil {
		t.Error("expected error for missing id")
	}
}
```

- [ ] **Step 2: Run — failures**

```bash
cd engine && go test ./internal/rpc/handlers/...
```

- [ ] **Step 3: Implement**

`engine/internal/rpc/handlers/threads.go`:

```go
package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

// CreateThread returns a handler for threads.create.
func CreateThread(repo *thread.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in thread.NewInput
		if err := json.Unmarshal(params, &in); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		if in.ProjectID == "" || in.Name == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id and name required"}
		}
		th, err := repo.Create(ctx, in)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(th)
	}
}

type listThreadsParams struct {
	ProjectID      string `json:"project_id"`
	IncludeClosed  bool   `json:"include_closed"`
}

// ListThreads returns a handler for threads.list.
func ListThreads(repo *thread.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p listThreadsParams
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id required"}
		}
		list, err := repo.ListByProject(ctx, p.ProjectID, p.IncludeClosed)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if list == nil {
			list = []thread.Thread{}
		}
		return json.Marshal(list)
	}
}

// GetThread returns a handler for threads.get.
func GetThread(repo *thread.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		th, err := repo.Get(ctx, p.ID)
		if errors.Is(err, thread.ErrNotFound) {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "thread not found"}
		}
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(th)
	}
}

// UpdateThread returns a handler for threads.update.
func UpdateThread(repo *thread.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in thread.UpdateInput
		if err := json.Unmarshal(params, &in); err != nil || in.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := repo.Update(ctx, in); err != nil {
			if errors.Is(err, thread.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "thread not found"}
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

// CloseThread returns a handler for threads.close.
func CloseThread(repo *thread.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := repo.Close(ctx, p.ID, now()); err != nil {
			if errors.Is(err, thread.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "thread not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		got, _ := repo.Get(ctx, p.ID)
		return json.Marshal(got)
	}
}

// ReopenThread returns a handler for threads.reopen.
func ReopenThread(repo *thread.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := repo.Reopen(ctx, p.ID); err != nil {
			if errors.Is(err, thread.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "thread not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		got, _ := repo.Get(ctx, p.ID)
		return json.Marshal(got)
	}
}
```

- [ ] **Step 4: Wire up in `main.go`**

In `engine/cmd/linetta-engine/main.go`:

1. Add the import `"github.com/devlikebear/linetta/engine/internal/thread"`.
2. After `mentions := mention.NewRepo(st)` add: `threads := thread.NewRepo(st)`.
3. Below the existing entity handlers (after `s.Handle("entities.update", ...)`), insert:

```go
s.Handle("threads.create", handlers.CreateThread(threads))
s.Handle("threads.list", handlers.ListThreads(threads))
s.Handle("threads.get", handlers.GetThread(threads))
s.Handle("threads.update", handlers.UpdateThread(threads))
s.Handle("threads.close", handlers.CloseThread(threads, clock))
s.Handle("threads.reopen", handlers.ReopenThread(threads))
```

- [ ] **Step 5: Run — green**

```bash
cd engine && go test ./internal/rpc/handlers/... ./internal/thread/... -race
cd engine && go build ./cmd/linetta-engine
```

- [ ] **Step 6: Commit**

```bash
git add engine/internal/rpc/handlers/threads.go engine/internal/rpc/handlers/threads_test.go engine/cmd/linetta-engine/main.go
git commit -m "feat(rpc): threads.create/list/get/update/close/reopen handlers"
```

---

## Phase B: Engine — beat package

### Task 3: `engine/internal/beat` package (TDD)

`Beat` mirrors the SQLite row. `Repo` exposes:

- `Create(ctx, NewInput{ThreadID, NodeID *string, Label, Intensity int}) (Beat, error)` — runs inside a transaction; ordinal = `COALESCE(MAX(ordinal),0)+1` within the thread. **Why a transaction:** two concurrent inserts on the same thread must not collide on ordinal; the transaction holds an implicit write lock so the SELECT MAX + INSERT pair is atomic.
- `Get(ctx, id)` — single beat.
- `ListByThread(ctx, threadID)` — ordered by ordinal ASC.
- `ListByNode(ctx, nodeID)` — every beat bound to the node, ordered by `(thread_id, ordinal)`.
- `Update(ctx, UpdateInput{ID, Label, Intensity})` — partial; intensity clamped 1..3.
- `Reorder(ctx, threadID, []string)` — accepts a permutation of beat ids belonging to the thread and rewrites their ordinals to `1..N`. Validates that the slice covers exactly the thread's beats.
- `Delete(ctx, id)` — hard delete; no cascade concerns (only the row goes).

**Files:**
- Create: `engine/internal/beat/beat.go`
- Create: `engine/internal/beat/repo.go`
- Create: `engine/internal/beat/repo_test.go`

- [ ] **Step 1: Failing test**

`engine/internal/beat/repo_test.go`:

```go
package beat

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

func openFixture(t *testing.T) (*store.Store, project.Project, thread.Thread) {
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
	tr := thread.NewRepo(s)
	th, _ := tr.Create(context.Background(), thread.NewInput{ProjectID: p.ID, Name: "T"})
	return s, p, th
}

func TestRepo_Create_assignsAscendingOrdinals(t *testing.T) {
	s, p, th := openFixture(t)
	r := NewRepo(s)
	ctx := context.Background()
	b1, err := r.Create(ctx, NewInput{ThreadID: th.ID, NodeID: p.LastOpenedNodeID, Label: "첫 마디", Intensity: 1})
	if err != nil {
		t.Fatalf("Create b1: %v", err)
	}
	b2, err := r.Create(ctx, NewInput{ThreadID: th.ID, Label: "둘째 마디", Intensity: 2})
	if err != nil {
		t.Fatalf("Create b2: %v", err)
	}
	if b1.Ordinal != 1 || b2.Ordinal != 2 {
		t.Errorf("ordinals = %d,%d want 1,2", b1.Ordinal, b2.Ordinal)
	}
	if b2.NodeID != nil {
		t.Errorf("NodeID = %v, want nil", b2.NodeID)
	}
}

func TestRepo_Create_intensityClampedAndDefaulted(t *testing.T) {
	s, _, th := openFixture(t)
	r := NewRepo(s)
	ctx := context.Background()
	b0, _ := r.Create(ctx, NewInput{ThreadID: th.ID, Label: "기본"})
	if b0.Intensity != 1 {
		t.Errorf("default intensity = %d, want 1", b0.Intensity)
	}
	bHi, _ := r.Create(ctx, NewInput{ThreadID: th.ID, Label: "초과", Intensity: 99})
	if bHi.Intensity != 3 {
		t.Errorf("clamp-high = %d, want 3", bHi.Intensity)
	}
}

func TestRepo_ListByThread_ordered(t *testing.T) {
	s, _, th := openFixture(t)
	r := NewRepo(s)
	ctx := context.Background()
	_, _ = r.Create(ctx, NewInput{ThreadID: th.ID, Label: "1"})
	_, _ = r.Create(ctx, NewInput{ThreadID: th.ID, Label: "2"})
	_, _ = r.Create(ctx, NewInput{ThreadID: th.ID, Label: "3"})
	got, err := r.ListByThread(ctx, th.ID)
	if err != nil {
		t.Fatalf("ListByThread: %v", err)
	}
	if len(got) != 3 || got[0].Label != "1" || got[2].Label != "3" {
		t.Errorf("order = %+v", got)
	}
}

func TestRepo_ListByNode_returnsBeatsFromManyThreads(t *testing.T) {
	s, p, th := openFixture(t)
	tr := thread.NewRepo(s)
	thOther, _ := tr.Create(context.Background(), thread.NewInput{ProjectID: p.ID, Name: "다른 스토리"})
	r := NewRepo(s)
	ctx := context.Background()
	nodeID := *p.LastOpenedNodeID
	_, _ = r.Create(ctx, NewInput{ThreadID: th.ID, NodeID: &nodeID, Label: "A"})
	_, _ = r.Create(ctx, NewInput{ThreadID: thOther.ID, NodeID: &nodeID, Label: "B"})
	_, _ = r.Create(ctx, NewInput{ThreadID: th.ID, Label: "Z"}) // unbound, must NOT appear
	got, err := r.ListByNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("ListByNode: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len = %d, want 2", len(got))
	}
}

func TestRepo_Update_intensityAndLabel(t *testing.T) {
	s, _, th := openFixture(t)
	r := NewRepo(s)
	ctx := context.Background()
	b, _ := r.Create(ctx, NewInput{ThreadID: th.ID, Label: "원본", Intensity: 1})
	if err := r.Update(ctx, UpdateInput{ID: b.ID, Label: "수정", Intensity: 3}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := r.Get(ctx, b.ID)
	if got.Label != "수정" || got.Intensity != 3 {
		t.Errorf("update missed: %+v", got)
	}
}

func TestRepo_Reorder_rewritesOrdinals(t *testing.T) {
	s, _, th := openFixture(t)
	r := NewRepo(s)
	ctx := context.Background()
	b1, _ := r.Create(ctx, NewInput{ThreadID: th.ID, Label: "1"})
	b2, _ := r.Create(ctx, NewInput{ThreadID: th.ID, Label: "2"})
	b3, _ := r.Create(ctx, NewInput{ThreadID: th.ID, Label: "3"})
	if err := r.Reorder(ctx, th.ID, []string{b3.ID, b1.ID, b2.ID}); err != nil {
		t.Fatalf("Reorder: %v", err)
	}
	got, _ := r.ListByThread(ctx, th.ID)
	if got[0].ID != b3.ID || got[1].ID != b1.ID || got[2].ID != b2.ID {
		t.Errorf("post-reorder = %+v", got)
	}
	if got[0].Ordinal != 1 || got[1].Ordinal != 2 || got[2].Ordinal != 3 {
		t.Errorf("ordinals = %d,%d,%d", got[0].Ordinal, got[1].Ordinal, got[2].Ordinal)
	}
}

func TestRepo_Reorder_rejectsIncompletePermutation(t *testing.T) {
	s, _, th := openFixture(t)
	r := NewRepo(s)
	ctx := context.Background()
	b1, _ := r.Create(context.Background(), NewInput{ThreadID: th.ID, Label: "1"})
	_, _ = r.Create(ctx, NewInput{ThreadID: th.ID, Label: "2"})
	if err := r.Reorder(ctx, th.ID, []string{b1.ID}); err == nil {
		t.Error("expected error on partial permutation")
	}
}

func TestRepo_Delete(t *testing.T) {
	s, _, th := openFixture(t)
	r := NewRepo(s)
	ctx := context.Background()
	b, _ := r.Create(ctx, NewInput{ThreadID: th.ID, Label: "X"})
	if err := r.Delete(ctx, b.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := r.Get(ctx, b.ID); err == nil {
		t.Error("expected ErrNotFound after delete")
	}
}

func TestRepo_BeatNodeIDNulledByCascade(t *testing.T) {
	// When the bound node is deleted, beats.node_id becomes NULL (ON DELETE SET NULL).
	// Verify by direct DELETE in SQL — the migration's FK already covers this.
	s, p, th := openFixture(t)
	r := NewRepo(s)
	ctx := context.Background()
	nodeID := *p.LastOpenedNodeID
	b, _ := r.Create(ctx, NewInput{ThreadID: th.ID, NodeID: &nodeID, Label: "B"})
	if _, err := s.DB().ExecContext(ctx, `DELETE FROM nodes WHERE id = ?`, nodeID); err != nil {
		t.Fatalf("delete node: %v", err)
	}
	got, err := r.Get(ctx, b.ID)
	if err != nil {
		t.Fatalf("Get after node delete: %v", err)
	}
	if got.NodeID != nil {
		t.Errorf("NodeID = %v, want nil after cascade", got.NodeID)
	}
}
```

- [ ] **Step 2: Run — failures**

```bash
cd engine && go test ./internal/beat/...
```

- [ ] **Step 3: Implement**

`engine/internal/beat/beat.go`:

```go
// Package beat owns Beat (Thread의 마디) domain logic. Beats belong to a
// Thread (cascade delete) and optionally bind to a Node (ON DELETE SET NULL).
package beat

// Beat mirrors the SQLite row. NodeID is nil when the beat is unbound or its
// bound node was deleted.
type Beat struct {
	ID        string  `json:"id"`
	ThreadID  string  `json:"thread_id"`
	NodeID    *string `json:"node_id,omitempty"`
	Ordinal   int     `json:"ordinal"`
	Label     string  `json:"label"`
	Intensity int     `json:"intensity"`
}

// NewInput is what `beats.create` accepts. Ordinal is assigned by the repo.
type NewInput struct {
	ThreadID  string  `json:"thread_id"`
	NodeID    *string `json:"node_id,omitempty"`
	Label     string  `json:"label"`
	Intensity int     `json:"intensity"`
}

// UpdateInput is what `beats.update` accepts. Empty Label leaves the field
// alone; Intensity == 0 leaves it alone (use 1..3 to set).
type UpdateInput struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Intensity int    `json:"intensity"`
}
```

`engine/internal/beat/repo.go`:

```go
package beat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/google/uuid"
)

// ErrNotFound is returned when a beat id does not exist.
var ErrNotFound = errors.New("beat not found")

// Repo persists Beats in SQLite.
type Repo struct {
	s *store.Store
}

func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

// Create assigns the next ordinal within the thread (atomic in a transaction
// to avoid collisions on concurrent inserts) and inserts the row. Intensity
// is clamped to 1..3; 0 defaults to 1.
func (r *Repo) Create(ctx context.Context, in NewInput) (Beat, error) {
	if in.ThreadID == "" {
		return Beat{}, fmt.Errorf("create beat: thread_id required")
	}
	intensity := clampIntensity(in.Intensity)

	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return Beat{}, err
	}
	defer tx.Rollback()

	var maxOrd sql.NullInt64
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(ordinal), 0) FROM beats WHERE thread_id = ?`,
		in.ThreadID).Scan(&maxOrd); err != nil {
		return Beat{}, err
	}
	ordinal := int(maxOrd.Int64) + 1

	id := uuid.NewString()
	if _, err := tx.ExecContext(ctx, `
INSERT INTO beats (id, thread_id, node_id, ordinal, label, intensity)
VALUES (?, ?, ?, ?, ?, ?)`, id, in.ThreadID, nullStr(in.NodeID), ordinal, in.Label, intensity); err != nil {
		return Beat{}, err
	}
	if err := tx.Commit(); err != nil {
		return Beat{}, err
	}
	return r.Get(ctx, id)
}

// Get returns one beat by id.
func (r *Repo) Get(ctx context.Context, id string) (Beat, error) {
	row := r.s.DB().QueryRowContext(ctx, baseSelect+` WHERE id = ?`, id)
	b, err := scan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Beat{}, ErrNotFound
	}
	return b, err
}

// ListByThread returns the thread's beats ordered by ordinal ASC.
func (r *Repo) ListByThread(ctx context.Context, threadID string) ([]Beat, error) {
	rows, err := r.s.DB().QueryContext(ctx,
		baseSelect+` WHERE thread_id = ? ORDER BY ordinal ASC`, threadID)
	if err != nil {
		return nil, err
	}
	return scanAll(rows)
}

// ListByNode returns every beat bound to the node, ordered by (thread_id, ordinal).
// Beats unbound (node_id IS NULL) are excluded.
func (r *Repo) ListByNode(ctx context.Context, nodeID string) ([]Beat, error) {
	rows, err := r.s.DB().QueryContext(ctx,
		baseSelect+` WHERE node_id = ? ORDER BY thread_id, ordinal`, nodeID)
	if err != nil {
		return nil, err
	}
	return scanAll(rows)
}

// Update applies a partial input.
func (r *Repo) Update(ctx context.Context, in UpdateInput) error {
	if in.ID == "" {
		return fmt.Errorf("update beat: id required")
	}
	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	row := tx.QueryRowContext(ctx, baseSelect+` WHERE id = ?`, in.ID)
	cur, err := scan(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if in.Label != "" {
		cur.Label = in.Label
	}
	if in.Intensity != 0 {
		cur.Intensity = clampIntensity(in.Intensity)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE beats SET label = ?, intensity = ? WHERE id = ?`,
		cur.Label, cur.Intensity, in.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// Reorder rewrites the thread's beat ordinals according to the supplied id slice.
// The slice MUST be a permutation of the thread's existing beat ids; otherwise
// the function returns an error and leaves the table untouched.
func (r *Repo) Reorder(ctx context.Context, threadID string, ids []string) error {
	existing, err := r.ListByThread(ctx, threadID)
	if err != nil {
		return err
	}
	if len(existing) != len(ids) {
		return fmt.Errorf("reorder: got %d ids, thread has %d beats", len(ids), len(existing))
	}
	have := map[string]bool{}
	for _, b := range existing {
		have[b.ID] = true
	}
	for _, id := range ids {
		if !have[id] {
			return fmt.Errorf("reorder: id %s not in thread %s", id, threadID)
		}
	}

	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Two-phase: bump every ordinal by a large offset first, then write the
	// final values. This sidesteps any future UNIQUE(thread_id, ordinal)
	// constraint without changing semantics today.
	if _, err := tx.ExecContext(ctx,
		`UPDATE beats SET ordinal = ordinal + 1000000 WHERE thread_id = ?`, threadID); err != nil {
		return err
	}
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx,
			`UPDATE beats SET ordinal = ? WHERE id = ?`, i+1, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Delete removes the beat.
func (r *Repo) Delete(ctx context.Context, id string) error {
	res, err := r.s.DB().ExecContext(ctx, `DELETE FROM beats WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

const baseSelect = `
SELECT id, thread_id, node_id, ordinal, label, intensity
FROM beats`

type scanner interface{ Scan(...any) error }

func scan(row scanner) (Beat, error) {
	var (
		b      Beat
		nodeID sql.NullString
	)
	if err := row.Scan(&b.ID, &b.ThreadID, &nodeID, &b.Ordinal, &b.Label, &b.Intensity); err != nil {
		return Beat{}, err
	}
	if nodeID.Valid {
		v := nodeID.String
		b.NodeID = &v
	}
	return b, nil
}

func scanAll(rows *sql.Rows) ([]Beat, error) {
	defer rows.Close()
	var out []Beat
	for rows.Next() {
		b, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func clampIntensity(v int) int {
	if v <= 0 {
		return 1
	}
	if v > 3 {
		return 3
	}
	return v
}

func nullStr(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}
```

- [ ] **Step 4: Run — green**

```bash
cd engine && go test ./internal/beat/... -race
```

- [ ] **Step 5: Commit**

```bash
git add engine/internal/beat/
git commit -m "feat(beat): Beat repo with atomic ordinal allocation, reorder, delete"
```

---

### Task 4: `beats.*` RPC handlers + main.go wiring (TDD)

Six handlers. `beats.create` (no Clock — beats have no timestamps), `beats.list_by_thread`, `beats.list_by_node`, `beats.update`, `beats.reorder`, `beats.delete`.

**Files:**
- Create: `engine/internal/rpc/handlers/beats.go`
- Create: `engine/internal/rpc/handlers/beats_test.go`
- Modify: `engine/cmd/linetta-engine/main.go`

- [ ] **Step 1: Failing test**

`engine/internal/rpc/handlers/beats_test.go`:

```go
package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

type beatFix struct {
	store *store.Store
	br    *beat.Repo
	tr    *thread.Repo
	pID   string
	thID  string
	nID   string
}

func newBeatFixture(t *testing.T) beatFix {
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
	tr := thread.NewRepo(s)
	th, _ := tr.Create(context.Background(), thread.NewInput{ProjectID: p.ID, Name: "T"})
	return beatFix{store: s, br: beat.NewRepo(s), tr: tr, pID: p.ID, thID: th.ID, nID: *p.LastOpenedNodeID}
}

func TestCreateBeatHandler(t *testing.T) {
	f := newBeatFixture(t)
	params := json.RawMessage(`{"thread_id":"` + f.thID + `","node_id":"` + f.nID + `","label":"첫 마디","intensity":2}`)
	res, err := CreateBeat(f.br)(context.Background(), params)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var b beat.Beat
	_ = json.Unmarshal(res, &b)
	if b.Label != "첫 마디" || b.Intensity != 2 || b.Ordinal != 1 {
		t.Errorf("beat = %+v", b)
	}
}

func TestListByThreadHandler(t *testing.T) {
	f := newBeatFixture(t)
	_, _ = f.br.Create(context.Background(), beat.NewInput{ThreadID: f.thID, Label: "A"})
	_, _ = f.br.Create(context.Background(), beat.NewInput{ThreadID: f.thID, Label: "B"})
	res, err := ListBeatsByThread(f.br)(context.Background(),
		json.RawMessage(`{"thread_id":"`+f.thID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var list []beat.Beat
	_ = json.Unmarshal(res, &list)
	if len(list) != 2 {
		t.Errorf("len = %d", len(list))
	}
}

func TestListByNodeHandler(t *testing.T) {
	f := newBeatFixture(t)
	_, _ = f.br.Create(context.Background(), beat.NewInput{ThreadID: f.thID, NodeID: &f.nID, Label: "bound"})
	_, _ = f.br.Create(context.Background(), beat.NewInput{ThreadID: f.thID, Label: "unbound"})
	res, _ := ListBeatsByNode(f.br)(context.Background(),
		json.RawMessage(`{"node_id":"`+f.nID+`"}`))
	var list []beat.Beat
	_ = json.Unmarshal(res, &list)
	if len(list) != 1 || list[0].Label != "bound" {
		t.Errorf("list = %+v", list)
	}
}

func TestUpdateBeatHandler(t *testing.T) {
	f := newBeatFixture(t)
	b, _ := f.br.Create(context.Background(), beat.NewInput{ThreadID: f.thID, Label: "원본"})
	if _, err := UpdateBeat(f.br)(context.Background(),
		json.RawMessage(`{"id":"`+b.ID+`","label":"수정","intensity":3}`)); err != nil {
		t.Fatalf("handler: %v", err)
	}
	got, _ := f.br.Get(context.Background(), b.ID)
	if got.Label != "수정" || got.Intensity != 3 {
		t.Errorf("update missed: %+v", got)
	}
}

func TestReorderBeatsHandler(t *testing.T) {
	f := newBeatFixture(t)
	b1, _ := f.br.Create(context.Background(), beat.NewInput{ThreadID: f.thID, Label: "1"})
	b2, _ := f.br.Create(context.Background(), beat.NewInput{ThreadID: f.thID, Label: "2"})
	params := json.RawMessage(`{"thread_id":"` + f.thID + `","ids":["` + b2.ID + `","` + b1.ID + `"]}`)
	if _, err := ReorderBeats(f.br)(context.Background(), params); err != nil {
		t.Fatalf("handler: %v", err)
	}
	got, _ := f.br.ListByThread(context.Background(), f.thID)
	if got[0].ID != b2.ID || got[1].ID != b1.ID {
		t.Errorf("post-reorder = %+v", got)
	}
}

func TestDeleteBeatHandler(t *testing.T) {
	f := newBeatFixture(t)
	b, _ := f.br.Create(context.Background(), beat.NewInput{ThreadID: f.thID, Label: "X"})
	if _, err := DeleteBeat(f.br)(context.Background(),
		json.RawMessage(`{"id":"`+b.ID+`"}`)); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if _, err := f.br.Get(context.Background(), b.ID); err == nil {
		t.Error("not deleted")
	}
}
```

- [ ] **Step 2: Run — failures**

```bash
cd engine && go test ./internal/rpc/handlers/...
```

- [ ] **Step 3: Implement**

`engine/internal/rpc/handlers/beats.go`:

```go
package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// CreateBeat returns a handler for beats.create.
func CreateBeat(repo *beat.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in beat.NewInput
		if err := json.Unmarshal(params, &in); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		if in.ThreadID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "thread_id required"}
		}
		b, err := repo.Create(ctx, in)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(b)
	}
}

type listBeatsByThreadParams struct {
	ThreadID string `json:"thread_id"`
}

// ListBeatsByThread returns a handler for beats.list_by_thread.
func ListBeatsByThread(repo *beat.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p listBeatsByThreadParams
		if err := json.Unmarshal(params, &p); err != nil || p.ThreadID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "thread_id required"}
		}
		list, err := repo.ListByThread(ctx, p.ThreadID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if list == nil {
			list = []beat.Beat{}
		}
		return json.Marshal(list)
	}
}

type listBeatsByNodeParams struct {
	NodeID string `json:"node_id"`
}

// ListBeatsByNode returns a handler for beats.list_by_node.
func ListBeatsByNode(repo *beat.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p listBeatsByNodeParams
		if err := json.Unmarshal(params, &p); err != nil || p.NodeID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node_id required"}
		}
		list, err := repo.ListByNode(ctx, p.NodeID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if list == nil {
			list = []beat.Beat{}
		}
		return json.Marshal(list)
	}
}

// UpdateBeat returns a handler for beats.update.
func UpdateBeat(repo *beat.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in beat.UpdateInput
		if err := json.Unmarshal(params, &in); err != nil || in.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := repo.Update(ctx, in); err != nil {
			if errors.Is(err, beat.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "beat not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		got, _ := repo.Get(ctx, in.ID)
		return json.Marshal(got)
	}
}

type reorderBeatsParams struct {
	ThreadID string   `json:"thread_id"`
	IDs      []string `json:"ids"`
}

// ReorderBeats returns a handler for beats.reorder.
func ReorderBeats(repo *beat.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p reorderBeatsParams
		if err := json.Unmarshal(params, &p); err != nil || p.ThreadID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "thread_id and ids required"}
		}
		if err := repo.Reorder(ctx, p.ThreadID, p.IDs); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}

// DeleteBeat returns a handler for beats.delete.
func DeleteBeat(repo *beat.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := repo.Delete(ctx, p.ID); err != nil {
			if errors.Is(err, beat.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "beat not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}
```

- [ ] **Step 4: Wire up in `main.go`**

1. Add import `"github.com/devlikebear/linetta/engine/internal/beat"`.
2. After `threads := thread.NewRepo(st)` add: `beats := beat.NewRepo(st)`.
3. Below the new `threads.*` handlers, insert:

```go
s.Handle("beats.create", handlers.CreateBeat(beats))
s.Handle("beats.list_by_thread", handlers.ListBeatsByThread(beats))
s.Handle("beats.list_by_node", handlers.ListBeatsByNode(beats))
s.Handle("beats.update", handlers.UpdateBeat(beats))
s.Handle("beats.reorder", handlers.ReorderBeats(beats))
s.Handle("beats.delete", handlers.DeleteBeat(beats))
```

- [ ] **Step 5: Run — green**

```bash
cd engine && go test ./internal/rpc/handlers/... ./internal/beat/... -race
cd engine && go build ./cmd/linetta-engine
```

- [ ] **Step 6: Commit**

```bash
git add engine/internal/rpc/handlers/beats.go engine/internal/rpc/handlers/beats_test.go engine/cmd/linetta-engine/main.go
git commit -m "feat(rpc): beats.create/list_by_thread/list_by_node/update/reorder/delete handlers"
```

---

### Task 4b: Add 0002 index migration

Two indexes accelerate the new read paths:
- `idx_beats_thread` (thread_id, ordinal) — `ListByThread` order.
- `idx_beats_node` (node_id) — `ListByNode` filter, used on every workspace navigation.

A `threads` table index isn't needed; `ListByProject` performs a full-table scan but the thread count per project is tiny.

**Files:**
- Create: `engine/internal/store/migrations/0002_thread_beat_indexes.sql`

- [ ] **Step 1: Implement**

`engine/internal/store/migrations/0002_thread_beat_indexes.sql`:

```sql
CREATE INDEX IF NOT EXISTS idx_beats_thread ON beats(thread_id, ordinal);
CREATE INDEX IF NOT EXISTS idx_beats_node   ON beats(node_id);
```

- [ ] **Step 2: Verify migration applies**

```bash
cd engine && go test ./internal/store/... -race
```

The existing `TestRun_appliesEmbeddedMigrations` test in `migrations_test.go` walks every file in `migrations/` and asserts the recorded version; it will pick this up automatically.

- [ ] **Step 3: Commit**

```bash
git add engine/internal/store/migrations/0002_thread_beat_indexes.sql
git commit -m "feat(store): 0002 indexes on beats(thread_id, ordinal) and beats(node_id)"
```

---

## Phase C: Engine — AI context

### Task 5: Active-thread loading in `ContextBuilder` + prompt section (TDD)

`Context` grows an `ActiveThreads []ActiveThread` field. `ActiveThread = { Name, Color, Summary, RecentBeats []BeatBrief }`, `BeatBrief = { Label, Ordinal }`. `ContextBuilder.Build` calls a new `loadActiveThreads(ctx, nodeID)` that:

1. `SELECT DISTINCT thread_id FROM beats WHERE node_id = ?`
2. `SELECT * FROM threads WHERE id IN (...) AND closed_at IS NULL` (use `thread.Repo.Get` per id to avoid building dynamic IN-clauses; thread counts are tiny).
3. For each open thread, `beat.Repo.ListByThread(threadID)` → take the **last 5** by ordinal → map to `BeatBrief{Label, Ordinal}`.
4. Cap the rendered section at ~1500 chars by dropping trailing threads whole until it fits.

`prompts.go buildUser` emits a new section **after** entities and **before** style notes:

```
## 활성 스토리라인
- [#c08a3e] 잃어버린 시간 — 요약 한 줄
  · #3 모래 위 사진
  · #4 모래에 박힌 자전거
```

`ContextBuilder` constructor signature changes from `NewContextBuilder(projects, nodes, mentions)` to `NewContextBuilder(projects, nodes, mentions, threads, beats)`. Update the construction site in `main.go`.

**Files:**
- Modify: `engine/internal/ai/ai.go`
- Modify: `engine/internal/ai/context.go`
- Modify: `engine/internal/ai/context_test.go`
- Modify: `engine/internal/ai/prompts.go`
- Modify: `engine/internal/ai/prompts_test.go`
- Modify: `engine/cmd/linetta-engine/main.go`

- [ ] **Step 1: Failing test — context loading**

Append to `engine/internal/ai/context_test.go`:

```go
func TestBuildContext_activeThreadsForCurrentNode(t *testing.T) {
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
	tr := thread.NewRepo(s)
	br := beat.NewRepo(s)

	// Open thread bound to the current node via two beats.
	th, _ := tr.Create(context.Background(), thread.NewInput{ProjectID: p.ID, Name: "잃어버린 시간", Color: "#c08a3e"})
	_ = tr.Update(context.Background(), thread.UpdateInput{ID: th.ID, Summary: "요약"})
	nID := *p.LastOpenedNodeID
	_, _ = br.Create(context.Background(), beat.NewInput{ThreadID: th.ID, NodeID: &nID, Label: "마디 1"})
	_, _ = br.Create(context.Background(), beat.NewInput{ThreadID: th.ID, NodeID: &nID, Label: "마디 2"})

	// Closed thread bound to the same node — must NOT appear.
	closed, _ := tr.Create(context.Background(), thread.NewInput{ProjectID: p.ID, Name: "닫힌"})
	_, _ = br.Create(context.Background(), beat.NewInput{ThreadID: closed.ID, NodeID: &nID, Label: "닫힌 마디"})
	_ = tr.Close(context.Background(), closed.ID, 2000)

	builder := NewContextBuilder(pr, nodes, mr, tr, br)
	got, err := builder.Build(context.Background(), nID, "재작성", Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(got.ActiveThreads) != 1 {
		t.Fatalf("active = %d, want 1 (open only)", len(got.ActiveThreads))
	}
	at := got.ActiveThreads[0]
	if at.Name != "잃어버린 시간" || at.Color != "#c08a3e" || at.Summary != "요약" {
		t.Errorf("active = %+v", at)
	}
	if len(at.RecentBeats) != 2 || at.RecentBeats[0].Label != "마디 1" {
		t.Errorf("beats = %+v", at.RecentBeats)
	}
}
```

You must also update `TestBuildContext_includesSceneEntitiesAndStyleNotes` and `TestBuildContext_prevSummary_trims300chars` to pass the new args: `NewContextBuilder(pr, nodes, mr, thread.NewRepo(s), beat.NewRepo(s))`.

- [ ] **Step 2: Failing test — prompt rendering**

Append to `engine/internal/ai/prompts_test.go`:

```go
func TestBuildUser_includesActiveThreads(t *testing.T) {
	c := Context{
		SceneLabel: "씬 1",
		SceneText:  "본문",
		ActiveThreads: []ActiveThread{
			{
				Name:    "잃어버린 시간",
				Color:   "#c08a3e",
				Summary: "여름 한 철의 기억",
				RecentBeats: []BeatBrief{
					{Label: "사진을 찍는 손", Ordinal: 3},
					{Label: "사라진 자전거", Ordinal: 4},
				},
			},
		},
		UserPrompt: "확장",
	}
	msgs := BuildMessages(c)
	usr := msgs[1].Content
	if !strings.Contains(usr, "## 활성 스토리라인") {
		t.Errorf("missing header: %q", usr)
	}
	if !strings.Contains(usr, "잃어버린 시간") || !strings.Contains(usr, "여름 한 철의 기억") {
		t.Errorf("thread metadata missing: %q", usr)
	}
	if !strings.Contains(usr, "#3 사진을 찍는 손") || !strings.Contains(usr, "#4 사라진 자전거") {
		t.Errorf("beats missing: %q", usr)
	}
}

func TestBuildUser_omitsActiveThreadsHeaderWhenEmpty(t *testing.T) {
	c := Context{SceneLabel: "씬 1", SceneText: "본문", UserPrompt: "재작성"}
	usr := BuildMessages(c)[1].Content
	if strings.Contains(usr, "활성 스토리라인") {
		t.Errorf("header should not appear when empty: %q", usr)
	}
}
```

- [ ] **Step 3: Run — failures**

```bash
cd engine && go test ./internal/ai/...
```

- [ ] **Step 4: Implement — ai.go**

In `engine/internal/ai/ai.go`, add fields:

```go
// Add to Context:
ActiveThreads []ActiveThread `json:"active_threads"`

// New types beneath EntityBrief:
type ActiveThread struct {
	Name        string      `json:"name"`
	Color       string      `json:"color"`
	Summary     string      `json:"summary"`
	RecentBeats []BeatBrief `json:"recent_beats"`
}

type BeatBrief struct {
	Label   string `json:"label"`
	Ordinal int    `json:"ordinal"`
}
```

- [ ] **Step 5: Implement — context.go**

In `engine/internal/ai/context.go`:

1. Add imports `"github.com/devlikebear/linetta/engine/internal/beat"` and `"github.com/devlikebear/linetta/engine/internal/thread"`.
2. Update the struct and constructor:

```go
const activeThreadsMaxChars = 1500
const recentBeatsPerThread = 5

type ContextBuilder struct {
	projects *project.Repo
	nodes    *node.Repo
	mentions *mention.Repo
	threads  *thread.Repo
	beats    *beat.Repo
}

func NewContextBuilder(projects *project.Repo, nodes *node.Repo, mentions *mention.Repo, threads *thread.Repo, beats *beat.Repo) *ContextBuilder {
	return &ContextBuilder{projects: projects, nodes: nodes, mentions: mentions, threads: threads, beats: beats}
}
```

3. Inside `Build`, after the entities block and before `return Context{...}`:

```go
active, err := b.loadActiveThreads(ctx, nodeID)
if err != nil {
	return Context{}, err
}
```

Add `ActiveThreads: active,` to the returned struct.

4. New helper at file end:

```go
func (b *ContextBuilder) loadActiveThreads(ctx context.Context, nodeID string) ([]ActiveThread, error) {
	bs, err := b.beats.ListByNode(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	// Collect unique thread ids preserving first-seen order.
	seen := map[string]bool{}
	var threadIDs []string
	for _, bt := range bs {
		if !seen[bt.ThreadID] {
			seen[bt.ThreadID] = true
			threadIDs = append(threadIDs, bt.ThreadID)
		}
	}
	out := make([]ActiveThread, 0, len(threadIDs))
	for _, tid := range threadIDs {
		th, err := b.threads.Get(ctx, tid)
		if err != nil {
			continue // benign: stale row
		}
		if th.ClosedAt != nil {
			continue
		}
		all, err := b.beats.ListByThread(ctx, tid)
		if err != nil {
			return nil, err
		}
		// Take last N by ordinal.
		start := 0
		if len(all) > recentBeatsPerThread {
			start = len(all) - recentBeatsPerThread
		}
		recents := make([]BeatBrief, 0, len(all)-start)
		for _, x := range all[start:] {
			recents = append(recents, BeatBrief{Label: x.Label, Ordinal: x.Ordinal})
		}
		out = append(out, ActiveThread{
			Name: th.Name, Color: th.Color, Summary: th.Summary, RecentBeats: recents,
		})
	}
	return capActiveThreads(out, activeThreadsMaxChars), nil
}

// capActiveThreads drops trailing entries (whole) until the rough rendered
// size is under maxChars. Cheap approximation: name + summary + each beat label.
func capActiveThreads(in []ActiveThread, maxChars int) []ActiveThread {
	total := 0
	for i, t := range in {
		size := len(t.Name) + len(t.Summary) + 8
		for _, b := range t.RecentBeats {
			size += len(b.Label) + 8
		}
		if total+size > maxChars && i > 0 {
			return in[:i]
		}
		total += size
	}
	return in
}
```

- [ ] **Step 6: Implement — prompts.go**

In `engine/internal/ai/prompts.go`, inside `buildUser`, insert the new section after the entities block and before the style-notes block:

```go
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
```

- [ ] **Step 7: Update `main.go` ContextBuilder construction**

Replace:

```go
contextBuilder := ai.NewContextBuilder(projects, nodes, mentions)
```

with:

```go
contextBuilder := ai.NewContextBuilder(projects, nodes, mentions, threads, beats)
```

(Both `threads` and `beats` were already created in Tasks 2 and 4 above.)

- [ ] **Step 8: Run — green**

```bash
cd engine && go test ./internal/ai/... -race
cd engine && go build ./cmd/linetta-engine
cd engine && go test ./... -race
```

- [ ] **Step 9: Commit**

```bash
git add engine/internal/ai/ engine/cmd/linetta-engine/main.go
git commit -m "feat(ai): inject active threads (open + bound to current node) into AI context"
```

---

## Phase D: Frontend — typed RPC

### Task 6: TypeScript types and RPC namespaces

No vitest configured in `apps/desktop` (verified — `package.json` has no test script and no `vitest.config.ts`). Verification is `pnpm tsc -b`.

**Files:**
- Modify: `apps/desktop/src/lib/types.ts`
- Modify: `apps/desktop/src/lib/rpc.ts`

- [ ] **Step 1: Add types**

Append to `apps/desktop/src/lib/types.ts`:

```ts
// Mirrors engine/internal/thread Thread struct.
export interface Thread {
  id: string;
  project_id: string;
  name: string;
  color: string;
  summary: string;
  closed_at?: number;
}

export interface NewThreadInput {
  project_id: string;
  name: string;
  color?: string;
}

export interface UpdateThreadInput {
  id: string;
  name?: string;
  color?: string;
  summary?: string;
}

// Mirrors engine/internal/beat Beat struct.
export interface Beat {
  id: string;
  thread_id: string;
  node_id?: string;
  ordinal: number;
  label: string;
  intensity: number;
}

export interface NewBeatInput {
  thread_id: string;
  node_id?: string;
  label?: string;
  intensity?: number;
}

export interface UpdateBeatInput {
  id: string;
  label?: string;
  intensity?: number;
}

// Mirrors engine/internal/ai ActiveThread / BeatBrief (used in ai_runs.context_json).
export interface BeatBrief {
  label: string;
  ordinal: number;
}

export interface ActiveThread {
  name: string;
  color: string;
  summary: string;
  recent_beats: BeatBrief[];
}
```

- [ ] **Step 2: Add RPC namespaces**

Append to `apps/desktop/src/lib/rpc.ts` after the `mentions` block and update the import list at the top to include the new types.

Update imports:

```ts
import type {
  AIOptions,
  Beat,
  Entity,
  ExportPayload,
  ListProjectsParams,
  NewBeatInput,
  NewEntityInput,
  NewProjectInput,
  NewThreadInput,
  NodeRow,
  Project,
  Settings,
  SettingsPatch,
  Snapshot,
  SnapshotEntry,
  Thread,
  UpdateBeatInput,
  UpdateEntityInput,
  UpdateThreadInput,
} from "./types";
```

Add namespaces:

```ts
export const threads = {
  create: (input: NewThreadInput) => rpcCall<Thread>("threads.create", input),
  list: (projectId: string, includeClosed = false) =>
    rpcCall<Thread[]>("threads.list", { project_id: projectId, include_closed: includeClosed }),
  get: (id: string) => rpcCall<Thread>("threads.get", { id }),
  update: (input: UpdateThreadInput) => rpcCall<Thread>("threads.update", input),
  close: (id: string) => rpcCall<Thread>("threads.close", { id }),
  reopen: (id: string) => rpcCall<Thread>("threads.reopen", { id }),
};

export const beats = {
  create: (input: NewBeatInput) => rpcCall<Beat>("beats.create", input),
  listByThread: (threadId: string) =>
    rpcCall<Beat[]>("beats.list_by_thread", { thread_id: threadId }),
  listByNode: (nodeId: string) =>
    rpcCall<Beat[]>("beats.list_by_node", { node_id: nodeId }),
  update: (input: UpdateBeatInput) => rpcCall<Beat>("beats.update", input),
  reorder: (threadId: string, ids: string[]) =>
    rpcCall<{ ok: true }>("beats.reorder", { thread_id: threadId, ids }),
  delete: (id: string) => rpcCall<{ ok: true }>("beats.delete", { id }),
};
```

- [ ] **Step 3: Verify**

```bash
cd apps/desktop && pnpm tsc -b
```

- [ ] **Step 4: Commit**

```bash
git add apps/desktop/src/lib/types.ts apps/desktop/src/lib/rpc.ts
git commit -m "feat(desktop): typed RPC bindings for threads + beats + active-thread context"
```

---

## Phase E: Frontend — ThreadSheet + ActiveThreadsPanel

### Task 7: `ThreadSheet` component

A right-slide-in editor mirroring `EntitySheet` structure and CSS variables. Fields:

- Name input (top, large).
- Color picker — fixed palette of 8 swatches: `#c0392b`, `#c08a3e`, `#b58a00`, `#3e8e41`, `#2980b9`, `#7e57c2`, `#d35d6e`, `#666` (default).
- Summary textarea (3 rows).
- Beats section: ordered list of the thread's beats. Each row = `#ordinal` + label input (inline) + intensity selector (1/2/3 buttons) + "삭제" button. Below the list: "+ 새 마디 추가" button. (Beats are unbound — node_id null — when created from this sheet; the right context panel `+` flow is what creates node-bound beats.)
- Action row: `이 스토리라인 닫기` button (calls `threads.close`; closes the sheet on success), `취소`, `저장`.

The sheet receives `threadId | null` plus an `onClose` and `onSaved(thread)` callbacks identical to `EntitySheet`. It also receives a `mode?: "new-with-node" | "edit"` and `seedNodeID?: string` — when `mode === "new-with-node"`, the sheet opens against a **just-created** thread and creates one beat bound to `seedNodeID` immediately after the first save. This is what Task 10's Cmd+K command needs.

**Files:**
- Create: `apps/desktop/src/components/ThreadSheet.tsx`
- Create: `apps/desktop/src/components/ThreadSheet.css`

- [ ] **Step 1: Implement CSS**

`apps/desktop/src/components/ThreadSheet.css` — exactly mirror `EntitySheet.css` selectors, but renamed to `.thread-*`. Add a small section:

```css
.thread-colors { display: flex; gap: 0.35rem; flex-wrap: wrap; }
.thread-color-swatch {
  width: 22px; height: 22px; border-radius: 50%;
  border: 2px solid transparent; cursor: pointer;
  padding: 0; background-clip: content-box;
}
.thread-color-swatch.sel { border-color: #1a1a1a; }

.beat-row {
  display: grid;
  grid-template-columns: auto 1fr auto auto;
  gap: 0.4rem;
  align-items: center;
}
.beat-ordinal { color: #9a9a9a; font-variant-numeric: tabular-nums; min-width: 1.6rem; }
.beat-intensity { display: flex; gap: 0.2rem; }
.beat-intensity button {
  font: inherit; font-size: 0.8rem;
  width: 24px; height: 24px; border-radius: 4px;
  border: 1px solid #d8d6cf; background: white; cursor: pointer;
  padding: 0;
}
.beat-intensity button.sel { background: #1a1a1a; color: #faf9f6; border-color: #1a1a1a; }

.thread-close-action {
  background: none; border: none; color: #a8312f;
  cursor: pointer; font: inherit; font-size: 0.85rem;
  margin-right: auto; padding: 0;
}
```

- [ ] **Step 2: Implement component**

`apps/desktop/src/components/ThreadSheet.tsx`:

```tsx
import { useCallback, useEffect, useState } from "react";
import type { Beat, Thread, UpdateThreadInput } from "../lib/types";
import { beats as beatsApi, threads as threadsApi } from "../lib/rpc";
import "./ThreadSheet.css";

const PALETTE = ["#c0392b", "#c08a3e", "#b58a00", "#3e8e41", "#2980b9", "#7e57c2", "#d35d6e", "#666"];

interface Props {
  threadId: string | null;
  seedNodeId?: string;            // when set, the first save creates a beat bound to this node
  onClose: () => void;
  onSaved?: (thread: Thread) => void;
}

export function ThreadSheet({ threadId, seedNodeId, onClose, onSaved }: Props) {
  const [thread, setThread] = useState<Thread | null>(null);
  const [draft, setDraft] = useState<UpdateThreadInput | null>(null);
  const [beatList, setBeatList] = useState<Beat[]>([]);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async (id: string) => {
    const [th, bs] = await Promise.all([threadsApi.get(id), beatsApi.listByThread(id)]);
    setThread(th);
    setDraft({ id: th.id, name: th.name, color: th.color, summary: th.summary });
    setBeatList(bs);
  }, []);

  useEffect(() => {
    if (!threadId) return;
    setError(null);
    reload(threadId).catch((e) => setError(String(e)));
  }, [threadId, reload]);

  if (!threadId) return null;

  const onSave = async () => {
    if (!draft) return;
    setSaving(true);
    setError(null);
    try {
      const saved = await threadsApi.update(draft);
      // Seed-node-on-first-save: if the thread has no beats and the caller asked
      // to bind one to seedNodeId, do that now.
      if (seedNodeId && beatList.length === 0) {
        await beatsApi.create({ thread_id: saved.id, node_id: seedNodeId, label: "" });
      }
      setThread(saved);
      onSaved?.(saved);
      onClose();
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  };

  const addBeat = async () => {
    try {
      const created = await beatsApi.create({ thread_id: threadId, label: "" });
      setBeatList((prev) => [...prev, created]);
    } catch (e) {
      setError(String(e));
    }
  };

  const updateBeat = async (b: Beat, patch: { label?: string; intensity?: number }) => {
    const next = { ...b, ...patch };
    setBeatList((prev) => prev.map((x) => (x.id === b.id ? next : x)));
    try {
      await beatsApi.update({ id: b.id, label: next.label, intensity: next.intensity });
    } catch (e) {
      setError(String(e));
    }
  };

  const deleteBeat = async (b: Beat) => {
    try {
      await beatsApi.delete(b.id);
      setBeatList((prev) => prev.filter((x) => x.id !== b.id));
    } catch (e) {
      setError(String(e));
    }
  };

  const closeThread = async () => {
    try {
      await threadsApi.close(threadId);
      onClose();
    } catch (e) {
      setError(String(e));
    }
  };

  return (
    <aside className="thread-sheet" onMouseDown={(e) => e.stopPropagation()}>
      <header className="thread-head">
        <span>스토리라인 편집</span>
        <button type="button" className="thread-close" onClick={onClose} aria-label="닫기">×</button>
      </header>

      {error && <p className="thread-error">{error}</p>}
      {!thread && !error && <p className="thread-loading">불러오는 중…</p>}

      {thread && draft && (
        <div className="thread-body">
          <section className="thread-section">
            <input
              className="thread-name"
              value={draft.name ?? ""}
              onChange={(e) => setDraft({ ...draft, name: e.target.value })}
              placeholder="스토리라인 이름"
            />
          </section>

          <section className="thread-section">
            <h5>색</h5>
            <div className="thread-colors">
              {PALETTE.map((c) => (
                <button
                  key={c}
                  type="button"
                  aria-label={c}
                  className={`thread-color-swatch${draft.color === c ? " sel" : ""}`}
                  style={{ backgroundColor: c }}
                  onClick={() => setDraft({ ...draft, color: c })}
                />
              ))}
            </div>
          </section>

          <section className="thread-section">
            <h5>요약</h5>
            <textarea
              value={draft.summary ?? ""}
              onChange={(e) => setDraft({ ...draft, summary: e.target.value })}
              rows={3}
            />
          </section>

          <section className="thread-section">
            <h5>마디</h5>
            {beatList.length === 0 && <p className="thread-empty">아직 마디 없음</p>}
            {beatList.map((b) => (
              <div className="beat-row" key={b.id}>
                <span className="beat-ordinal">#{b.ordinal}</span>
                <input
                  className="attr-value"
                  value={b.label}
                  onChange={(e) => updateBeat(b, { label: e.target.value })}
                  placeholder="마디 설명"
                />
                <div className="beat-intensity">
                  {[1, 2, 3].map((lvl) => (
                    <button
                      key={lvl}
                      type="button"
                      className={b.intensity === lvl ? "sel" : ""}
                      onClick={() => updateBeat(b, { intensity: lvl })}
                    >{lvl}</button>
                  ))}
                </div>
                <button type="button" className="attr-del" onClick={() => deleteBeat(b)} aria-label="삭제">×</button>
              </div>
            ))}
            <button type="button" className="attr-add" onClick={addBeat}>+ 새 마디 추가</button>
          </section>

          <div className="thread-actions">
            <button type="button" className="thread-close-action" onClick={closeThread}>
              이 스토리라인 닫기
            </button>
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

- [ ] **Step 3: Verify**

```bash
cd apps/desktop && pnpm tsc -b
```

- [ ] **Step 4: Commit**

```bash
git add apps/desktop/src/components/ThreadSheet.tsx apps/desktop/src/components/ThreadSheet.css
git commit -m "feat(desktop): ThreadSheet for editing name/color/summary/beats"
```

---

### Task 8: `ActiveThreadsPanel` in the right context column

Replace the `활성 Thread (곧 추가됨 — post-MVP)` placeholder in `ContextPanel.tsx` with a real list. Each row:

- Colored dot (uses `thread.color`)
- Thread name (button — clicking opens ThreadSheet for that thread)
- Small `+` icon-button on the right — opens a tiny inline label prompt; on Enter, creates a beat bound to the current node.

When there are no active threads, show the existing empty state copy "아직 마디 없음".

**Files:**
- Create: `apps/desktop/src/components/ActiveThreadsPanel.tsx`
- Modify: `apps/desktop/src/components/ContextPanel.tsx`
- Modify: `apps/desktop/src/routes/Workspace.tsx`

- [ ] **Step 1: Implement panel**

`apps/desktop/src/components/ActiveThreadsPanel.tsx`:

```tsx
import { useCallback, useEffect, useState } from "react";
import type { Thread } from "../lib/types";
import { beats as beatsApi, threads as threadsApi } from "../lib/rpc";

interface Props {
  projectId: string;
  nodeId: string;
  onOpenThread: (threadId: string) => void;
  onChanged?: () => void;
}

interface Row {
  thread: Thread;
}

export function ActiveThreadsPanel({ projectId, nodeId, onOpenThread, onChanged }: Props) {
  const [rows, setRows] = useState<Row[] | null>(null);
  const [adding, setAdding] = useState<string | null>(null); // threadId currently showing the label-prompt
  const [draftLabel, setDraftLabel] = useState("");

  const reload = useCallback(async () => {
    try {
      const nodeBeats = await beatsApi.listByNode(nodeId);
      const ids = Array.from(new Set(nodeBeats.map((b) => b.thread_id)));
      if (ids.length === 0) {
        setRows([]);
        return;
      }
      const all = await threadsApi.list(projectId, false);
      const map = new Map(all.map((t) => [t.id, t]));
      setRows(ids.map((id) => map.get(id)).filter((t): t is Thread => !!t).map((thread) => ({ thread })));
    } catch {
      setRows([]); // benign
    }
  }, [projectId, nodeId]);

  useEffect(() => { reload(); }, [reload]);

  const submitBeat = async (threadId: string) => {
    if (!draftLabel.trim()) { setAdding(null); return; }
    try {
      await beatsApi.create({ thread_id: threadId, node_id: nodeId, label: draftLabel.trim() });
      setAdding(null);
      setDraftLabel("");
      onChanged?.();
      reload();
    } catch { /* benign */ }
  };

  return (
    <section className="ctx-section">
      <h4>활성 Thread</h4>
      {rows && rows.length === 0 && <p className="ctx-empty">이 씬에 연결된 스토리라인 없음</p>}
      {rows && rows.map(({ thread }) => (
        <div key={thread.id} className="ctx-entity-row" style={{ display: "flex", alignItems: "center", gap: "0.4rem" }}>
          <button
            type="button"
            className="ctx-entity"
            onClick={() => onOpenThread(thread.id)}
            style={{ flex: 1 }}
          >
            <span
              aria-hidden
              style={{
                display: "inline-block", width: 10, height: 10, borderRadius: "50%",
                backgroundColor: thread.color, marginRight: "0.5rem",
              }}
            />
            <span className="ctx-entity-name">{thread.name}</span>
          </button>
          <button
            type="button"
            aria-label="이 씬에 마디 추가"
            onClick={() => { setAdding(thread.id); setDraftLabel(""); }}
            style={{ background: "none", border: "1px solid #d8d6cf", borderRadius: 4, cursor: "pointer", padding: "0 0.35rem" }}
          >+</button>
        </div>
      ))}
      {adding && (
        <input
          autoFocus
          className="attr-value"
          value={draftLabel}
          placeholder="이 씬에서 일어난 마디 (Enter)"
          onChange={(e) => setDraftLabel(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") { e.preventDefault(); submitBeat(adding); }
            else if (e.key === "Escape") { e.preventDefault(); setAdding(null); }
          }}
          onBlur={() => setAdding(null)}
        />
      )}
    </section>
  );
}
```

- [ ] **Step 2: Modify ContextPanel.tsx**

Replace the placeholder `활성 Thread` section. The component must now receive `projectId`, `nodeId`, an `onOpenThread` callback, and an `onThreadDataChanged` callback. Insert the panel where the placeholder was:

```tsx
<ActiveThreadsPanel
  projectId={project.id}
  nodeId={node.id}
  onOpenThread={onOpenThread}
  onChanged={onThreadDataChanged}
/>
```

Add the new props to the `Props` interface and the import.

- [ ] **Step 3: Wire up in Workspace.tsx**

Add `[threadSheetId, setThreadSheetId] = useState<string | null>(null)` to the component state. Pass `onOpenThread={setThreadSheetId}` and `onThreadDataChanged={() => { /* benign; ActiveThreadsPanel self-reloads */ }}` to `<ContextPanel ... />`.

Render `ThreadSheet` in the same conditional column as `EntitySheet` and `AIContextPanel` — extend the existing precedence chain: if `entitySheetId` → EntitySheet, else if `threadSheetId` → ThreadSheet, else if AI mode → AIContextPanel, else ContextPanel. The `with-sheet` body class should be applied when either sheet is open.

- [ ] **Step 4: Verify**

```bash
cd apps/desktop && pnpm tsc -b
```

- [ ] **Step 5: Commit**

```bash
git add apps/desktop/src/components/ActiveThreadsPanel.tsx apps/desktop/src/components/ContextPanel.tsx apps/desktop/src/routes/Workspace.tsx
git commit -m "feat(desktop): right-panel ActiveThreadsPanel + ThreadSheet integration"
```

---

## Phase F: Cmd+K + Thread View

### Task 9: Thread View route

A new route `/workspace/:projectId/threads` rendering vertical lanes. Each lane = one open thread (closed threads hidden). The lane is a horizontal track with beats positioned proportionally between min and max ordinal across **the union of all rendered threads** (so lanes align by relative position). Beat = `<button>` disc:

- Size 14/22/30px for intensity 1/2/3.
- Background = `thread.color`.
- `node_id == null` → render with greyed background (`#999` + dashed border) and `disabled` attribute. Spec §3 cascade: when a bound node is deleted, `node_id` becomes NULL automatically.
- Hover → native `title` showing the label.
- Click → `navigate("/workspace/:projectId")` with `state: { jumpToNodeId: beat.node_id }`. Workspace's initial-load `useEffect` should pick up the location state via `useLocation`. **Add to Workspace.tsx initial-load effect:**

```ts
const location = useLocation();
const jumpTo = (location.state as { jumpToNodeId?: string } | null)?.jumpToNodeId;
// Use jumpTo in preference to p.last_opened_node_id when set.
```

Loading state: spinner while the thread+beat queries run.

**Files:**
- Create: `apps/desktop/src/routes/ThreadView.tsx`
- Create: `apps/desktop/src/routes/ThreadView.css`
- Modify: `apps/desktop/src/App.tsx`
- Modify: `apps/desktop/src/routes/Workspace.tsx` (consume location state)

- [ ] **Step 1: Implement view**

`apps/desktop/src/routes/ThreadView.tsx`:

```tsx
import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { beats as beatsApi, threads as threadsApi } from "../lib/rpc";
import type { Beat, Thread } from "../lib/types";
import "./ThreadView.css";

const INTENSITY_PX: Record<number, number> = { 1: 14, 2: 22, 3: 30 };

interface Lane {
  thread: Thread;
  beats: Beat[];
}

export function ThreadView() {
  const { projectId } = useParams();
  const navigate = useNavigate();
  const [lanes, setLanes] = useState<Lane[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!projectId) return;
    let cancelled = false;
    (async () => {
      try {
        const ts = await threadsApi.list(projectId, false);
        const lanes = await Promise.all(
          ts.map(async (t) => ({ thread: t, beats: await beatsApi.listByThread(t.id) })),
        );
        if (!cancelled) setLanes(lanes);
      } catch (e) {
        if (!cancelled) setError(String(e));
      }
    })();
    return () => { cancelled = true; };
  }, [projectId]);

  const maxOrdinal = useMemo(
    () => Math.max(1, ...((lanes ?? []).flatMap((l) => l.beats.map((b) => b.ordinal)))),
    [lanes],
  );

  if (error) return <main className="shell"><p className="error">{error}</p></main>;
  if (!lanes) return <main className="shell"><p className="hint">불러오는 중…</p></main>;

  const jumpTo = (b: Beat) => {
    if (!b.node_id) return;
    navigate(`/workspace/${projectId}`, { state: { jumpToNodeId: b.node_id } });
  };

  return (
    <main className="thread-view">
      <header className="thread-view-top">
        <Link to={`/workspace/${projectId}`} className="thread-view-back">← 작업실</Link>
        <h1>흐름</h1>
      </header>

      {lanes.length === 0 && <p className="hint">아직 스토리라인이 없습니다. Cmd+K → "이 씬을 새 Thread로 표시"로 시작해보세요.</p>}

      <div className="thread-lanes">
        {lanes.map(({ thread, beats }) => (
          <div className="thread-lane" key={thread.id}>
            <div className="thread-lane-head">
              <span className="thread-dot" style={{ backgroundColor: thread.color }} />
              <span className="thread-lane-name">{thread.name}</span>
              {thread.summary && <span className="thread-lane-summary">{thread.summary}</span>}
            </div>
            <div className="thread-lane-track">
              {beats.map((b) => {
                const left = `${((b.ordinal - 1) / Math.max(1, maxOrdinal - 1)) * 100}%`;
                const size = INTENSITY_PX[b.intensity] ?? 14;
                const isOrphan = !b.node_id;
                return (
                  <button
                    key={b.id}
                    type="button"
                    className={`beat-disc${isOrphan ? " orphan" : ""}`}
                    title={`#${b.ordinal} ${b.label}${isOrphan ? " (씬 삭제됨)" : ""}`}
                    style={{
                      left,
                      width: size,
                      height: size,
                      backgroundColor: isOrphan ? "#999" : thread.color,
                    }}
                    disabled={isOrphan}
                    onClick={() => jumpTo(b)}
                  />
                );
              })}
            </div>
          </div>
        ))}
      </div>
    </main>
  );
}
```

`apps/desktop/src/routes/ThreadView.css`:

```css
.thread-view { padding: 2rem 3rem; max-width: 1100px; margin: 0 auto; }
.thread-view-top { display: flex; align-items: baseline; gap: 1rem; margin-bottom: 2rem; }
.thread-view-back { text-decoration: none; color: #6b6b6b; font-size: 0.9rem; }
.thread-view h1 { font-family: serif; font-weight: 400; font-size: 1.6rem; margin: 0; }

.thread-lanes { display: flex; flex-direction: column; gap: 1.6rem; }
.thread-lane { display: flex; flex-direction: column; gap: 0.5rem; }
.thread-lane-head { display: flex; align-items: center; gap: 0.5rem; }
.thread-dot { display: inline-block; width: 12px; height: 12px; border-radius: 50%; }
.thread-lane-name { font-size: 0.95rem; }
.thread-lane-summary { color: #9a9a9a; font-size: 0.85rem; margin-left: 0.5rem; }

.thread-lane-track {
  position: relative;
  height: 38px;
  background: linear-gradient(to bottom, transparent calc(50% - 1px), #ece9e0 calc(50% - 1px), #ece9e0 calc(50% + 1px), transparent calc(50% + 1px));
}
.beat-disc {
  position: absolute;
  top: 50%;
  transform: translate(-50%, -50%);
  border-radius: 50%;
  border: 2px solid white;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.15);
  cursor: pointer;
  padding: 0;
}
.beat-disc.orphan { border-style: dashed; cursor: not-allowed; opacity: 0.6; }
.beat-disc:hover:not(.orphan) { transform: translate(-50%, -50%) scale(1.15); }
```

- [ ] **Step 2: Register route in `App.tsx`**

Add `import { ThreadView } from "./routes/ThreadView";` and a new `<Route path="/workspace/:projectId/threads" element={<ThreadView />} />` between Workspace and Settings.

- [ ] **Step 3: Consume location state in Workspace**

In `apps/desktop/src/routes/Workspace.tsx`:

1. Import `useLocation`: `import { Link, useLocation, useNavigate, useParams } from "react-router-dom";`
2. Add inside the component: `const location = useLocation();`
3. Modify the initial-load `useEffect` so the target node is the `jumpToNodeId` from location state when present:

```ts
const jumpTo = (location.state as { jumpToNodeId?: string } | null)?.jumpToNodeId;
const target = jumpTo ?? p.last_opened_node_id;
if (!target) throw new Error("project has no opened node");
const next = await fetchTree(projectId, target);
```

Also clear the state after consuming it so a manual reload doesn't keep jumping: after `setLoad(next)`, call `navigate(location.pathname, { replace: true, state: null });` (only when `jumpTo` was set).

4. Enable the disabled `view-threads` command in the `commands` memo — replace the disabled stub with:

```ts
cmds.push({
  id: "view-threads",
  section: "보기",
  label: "흐름 (Thread View)",
  run: () => navigate(`/workspace/${load.project.id}/threads`),
});
```

- [ ] **Step 4: Verify**

```bash
cd apps/desktop && pnpm tsc -b && pnpm build
```

- [ ] **Step 5: Commit**

```bash
git add apps/desktop/src/routes/ThreadView.tsx apps/desktop/src/routes/ThreadView.css apps/desktop/src/App.tsx apps/desktop/src/routes/Workspace.tsx
git commit -m "feat(desktop): Thread View — vertical lanes with intensity-sized discs + jump-to-node"
```

---

### Task 10: "이 씬을 새 Thread로 표시" Cmd+K command

Add a new Cmd+K command in the `노드` section. Behavior:

1. Prompt the user (via the existing `promptDialog`) for the thread name. Default = `load.node.title || load.node.label`.
2. Call `threads.create({ project_id, name, color: "#666" })`.
3. Open `ThreadSheet` for the new thread id, passing `seedNodeId={load.node.id}` — the sheet's "save" handler creates the first beat bound to the current node automatically (per Task 7's seed logic). The user is then dropped into the sheet to refine the name/color/summary and add a label to the first beat.

**Files:**
- Modify: `apps/desktop/src/routes/Workspace.tsx`

- [ ] **Step 1: Add state**

Already added in Task 8: `threadSheetId` exists. Add a parallel `threadSheetSeedNodeId, setThreadSheetSeedNodeId = useState<string | null>(null)`. Pass it as the `seedNodeId` prop to ThreadSheet (cleared on close).

- [ ] **Step 2: Add command**

Inside the `commands` memo, add after `new-chapter`:

```ts
cmds.push({
  id: "mark-thread",
  section: "노드",
  label: "이 씬을 새 Thread로 표시",
  run: async () => {
    const name = await promptDialog("새 스토리라인 이름", load.node.title || load.node.label);
    if (name === null) return;
    const trimmed = name.trim();
    if (!trimmed) return;
    try {
      const t = await threadsApi.create({ project_id: load.project.id, name: trimmed, color: "#666" });
      setThreadSheetSeedNodeId(load.node.id);
      setThreadSheetId(t.id);
    } catch (e) {
      showToast("스토리라인 생성 실패: " + String(e));
    }
  },
});
```

Add `import { threads as threadsApi, ... } from "../lib/rpc";` to the imports if not already present from Task 8.

- [ ] **Step 3: Wire ThreadSheet onClose to clear seed**

```tsx
{threadSheetId && (
  <ThreadSheet
    threadId={threadSheetId}
    seedNodeId={threadSheetSeedNodeId ?? undefined}
    onClose={() => {
      setThreadSheetId(null);
      setThreadSheetSeedNodeId(null);
      focusEditor();
    }}
    onSaved={() => { /* ActiveThreadsPanel self-reloads via its own effect when the user navigates back */ }}
  />
)}
```

- [ ] **Step 4: Verify**

```bash
cd apps/desktop && pnpm tsc -b
```

- [ ] **Step 5: Commit**

```bash
git add apps/desktop/src/routes/Workspace.tsx
git commit -m "feat(workspace): Cmd+K — mark this scene as a new Thread (seeds first beat)"
```

---

## Phase G: E2E smoke + tag

### Task 11: Manual end-to-end smoke

- [ ] **Step 1: Build**

```bash
cd engine && go test ./... -race
cd engine && go build ./cmd/linetta-engine
cd apps/desktop && pnpm tsc -b && pnpm build
cd apps/desktop && pnpm tauri dev
```

- [ ] **Step 2: Walk through the feature**

1. Open an existing project (or create one). Confirm three or more leaf scenes exist (create with Cmd+K → 새 씬 if needed).
2. **Thread creation flow:** On scene 1, hit Cmd+K → "이 씬을 새 Thread로 표시". Name it "잃어버린 시간". The ThreadSheet opens with one auto-created unbound-label beat. Pick a color (e.g. amber #c08a3e), type a summary, label the first beat "사진을 찍는 손", click 저장.
3. **Bind to more scenes:** Navigate to scene 2 (Cmd+K → 다음 씬). In the right-panel "활성 Thread" section, click `+` next to "잃어버린 시간", type "모래 위 발자국", press Enter. Repeat on scene 3 with "사라진 자전거".
4. **Verify right-panel:** Each of the three scenes now shows the colored dot + "잃어버린 시간" in the right panel. Scene 4 (untouched) shows the empty state "이 씬에 연결된 스토리라인 없음".
5. **AI context verification:** On scene 2, switch to AI mode, type "확장" in the prompt, click 생성. After the run, inspect the DB:

```bash
sqlite3 "$LINETTA_HOME/library.db" "SELECT context_json FROM ai_runs ORDER BY started_at DESC LIMIT 1;" | jq '.active_threads'
```

The output must contain one entry with `name: "잃어버린 시간"`, color, summary, and a `recent_beats` array. Also verify the prompt text used the section header by checking the streamed assistant response was contextually aware (look at engine stderr).

6. **Thread View:** Hit Cmd+K → "흐름 (Thread View)". One lane labeled "잃어버린 시간" with three discs, ordered left-to-right. Hovering shows tooltips. Click the middle disc → workspace jumps to scene 2.
7. **Orphan rendering:** Back in the workspace, navigate to scene 3, Cmd+K → "삭제" (confirm). Cmd+K → "흐름". The third disc should now be dashed/grey and non-clickable.
8. **Close thread:** From the workspace, click the dot row to reopen the ThreadSheet, click "이 스토리라인 닫기". The sheet closes; the right panel and Thread View no longer show the thread; an AI run on those scenes no longer includes the section.

- [ ] **Step 3: If everything passes — tag**

```bash
git tag plan-7-thread-beat-done
git push --tags
```

- [ ] **Step 4: If something fails**

Apply superpowers:systematic-debugging. The most likely failure surfaces:
- **Mention picker/PromptDialog focus race** when the ThreadSheet opens immediately after `promptDialog` resolves. Mitigation: wrap the ThreadSheet open in `setTimeout(..., 0)` if the dialog teardown clobbers focus.
- **Beat ordinal collision** if a previous schema run left non-monotonic ordinals — `Reorder`'s two-phase update handles this but verify by re-running Task 3 tests.
- **Location-state jump** lingering across reloads — verify the `navigate(location.pathname, { replace: true, state: null })` cleanup actually runs once (use React DevTools).

---

## Open design questions

1. **Beat dedup on a single (thread, node) pair.** Nothing in the schema or the Phase E "+" flow prevents creating multiple beats from the same thread on the same node. The user might want this (two different moments inside one scene), or might want a UNIQUE constraint. Current design **allows duplicates** — flag for user.
2. **Thread color palette source.** Hardcoded into ThreadSheet (8 swatches). Could be moved to settings later for user customization. Not blocking.
3. **`ai_runs.context_json` schema drift.** Old rows lack `active_threads`. The frontend never reads `context_json` today, but if a future "AI run inspector" surfaces, it should handle the missing field. Not blocking for Plan 7.

---

### Critical Files for Implementation

- /Users/changheonshin/workspace/myworks/linetta/engine/internal/thread/repo.go
- /Users/changheonshin/workspace/myworks/linetta/engine/internal/beat/repo.go
- /Users/changheonshin/workspace/myworks/linetta/engine/internal/ai/context.go
- /Users/changheonshin/workspace/myworks/linetta/apps/desktop/src/components/ThreadSheet.tsx
- /Users/changheonshin/workspace/myworks/linetta/apps/desktop/src/routes/Workspace.tsx

---

