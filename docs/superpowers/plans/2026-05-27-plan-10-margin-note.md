# Plan 10 — Post-MVP P2: Margin Note (여백 주석)

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the "여백 주석" (margin note) feature from spec §11.2 P2. Writers attach a short note to any cursor position inside a scene, see a small ☘︎ icon at that position, hover/click to read or edit, and have those notes injected into the AI context as `## 작가 주석`. The `notes` table already exists in `0001_init.sql` — no schema migration needed.

**Architecture:** A new engine package `note` mirrors `thread` (small CRUD `Repo`). Five RPC handlers (`notes.create/list_for_node/get/update/delete`). `ContextBuilder` gains `*note.Repo`, populates `Notes []NoteBrief`, and `prompts.go::buildUser` appends `## 작가 주석` between active threads and style notes. On the frontend, a Tiptap inline atom node `note-marker` (modeled on MentionExtension) renders ☘︎ at the anchor; the NodeView dispatches `linetta:note-{hover,hover-end,click}` events that Workspace listens for. A `NotePopover` shows the body (read mode on hover, edit mode on click). A Cmd+K command `여백 주석 추가` prompts, calls `notes.create`, then inserts the atom node.

**Tech stack additions:** None. Reuses Tiptap's `Node.create()` API.

**Spec reference:** §11.2 P2.

**Design decisions locked:**
1. Single-integer anchor (ProseMirror absolute position); no mark-tracking.
2. Inline atom node `note-marker` rendered as ☘︎ (U+2618). Hover → read popover; click → edit popover (저장 / 삭제).
3. AI context includes all notes for the current node as `## 작가 주석` bullets (body only; anchor omitted from prompt).

---

## Pre-flight

- [ ] Plan 9 tagged; `git status --short` empty.
- [ ] `cd engine && go test ./... -race` green.
- [ ] `cd apps/desktop && pnpm tsc -b && pnpm build` green.
- [ ] `notes` table exists: `sqlite3 "$LINETTA_HOME/library.db" ".schema notes"` shows the original 5 columns.

---

## File Structure

```
engine/internal/note/                            (new package — note.go, repo.go, repo_test.go)
engine/internal/rpc/handlers/notes.go            (new + notes_test.go)
engine/internal/ai/{ai.go, context.go, prompts.go} + tests (modified)
engine/cmd/linetta-engine/main.go                (modified — wire notes repo + handlers + ContextBuilder arg)
apps/desktop/src/lib/{types.ts, rpc.ts}          (modified)
apps/desktop/src/components/editor/NoteMarkerExtension.ts (new)
apps/desktop/src/components/{NotePopover.tsx, NotePopover.css} (new)
apps/desktop/src/routes/Workspace.tsx            (modified)
apps/desktop/src/components/editor/Tiptap.tsx    (modified — expose addNoteMarker/removeNoteMarker on handle)
```

---

## Phase A: Engine (3 tasks)

### Task 1: `engine/internal/note/` package

Files:
- Create `engine/internal/note/note.go`, `repo.go`, `repo_test.go`

Steps:

1. **`note.go`** — types only:

```go
package note

type Note struct {
	ID        string `json:"id"`
	NodeID    string `json:"node_id"`
	Anchor    int    `json:"anchor"`
	Body      string `json:"body"`
	CreatedAt int64  `json:"created_at"`
}

type NewInput struct {
	NodeID string `json:"node_id"`
	Anchor int    `json:"anchor"`
	Body   string `json:"body"`
}

type UpdateInput struct {
	ID   string `json:"id"`
	Body string `json:"body"`
}
```

2. **Failing tests in `repo_test.go`** (6 cases):

```go
package note

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func newRepo(t *testing.T) (*Repo, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil { t.Fatalf("store.Open: %v", err) }
	t.Cleanup(func() { _ = s.Close() })
	pr := project.NewRepo(s)
	p, err := pr.Create(context.Background(), 1000, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first",
	})
	if err != nil { t.Fatalf("create project: %v", err) }
	return NewRepo(s), *p.LastOpenedNodeID
}

func TestRepo_Create_thenGet(t *testing.T) {
	r, nodeID := newRepo(t)
	ctx := context.Background()
	n, err := r.Create(ctx, NewInput{NodeID: nodeID, Anchor: 7, Body: "여기 분위기 바꾸기"}, 5000)
	if err != nil { t.Fatalf("Create: %v", err) }
	if n.ID == "" || n.Anchor != 7 || n.Body != "여기 분위기 바꾸기" || n.CreatedAt != 5000 {
		t.Errorf("unexpected note: %+v", n)
	}
	got, err := r.Get(ctx, n.ID)
	if err != nil { t.Fatalf("Get: %v", err) }
	if got.Body != "여기 분위기 바꾸기" { t.Errorf("Get = %+v", got) }
}

func TestRepo_Create_rejectsEmpty(t *testing.T) {
	r, nodeID := newRepo(t)
	if _, err := r.Create(context.Background(), NewInput{NodeID: "", Anchor: 0, Body: "x"}, 1); err == nil {
		t.Error("expected error for empty node_id")
	}
	if _, err := r.Create(context.Background(), NewInput{NodeID: nodeID, Anchor: 0, Body: ""}, 1); err == nil {
		t.Error("expected error for empty body")
	}
}

func TestRepo_ListForNode_orderedByAnchor(t *testing.T) {
	r, nodeID := newRepo(t)
	ctx := context.Background()
	_, _ = r.Create(ctx, NewInput{NodeID: nodeID, Anchor: 30, Body: "C"}, 1)
	_, _ = r.Create(ctx, NewInput{NodeID: nodeID, Anchor: 10, Body: "A"}, 2)
	_, _ = r.Create(ctx, NewInput{NodeID: nodeID, Anchor: 20, Body: "B"}, 3)
	got, err := r.ListForNode(ctx, nodeID)
	if err != nil { t.Fatalf("ListForNode: %v", err) }
	if len(got) != 3 || got[0].Body != "A" || got[1].Body != "B" || got[2].Body != "C" {
		t.Errorf("order = %+v", got)
	}
}

func TestRepo_Update_bodyOnly(t *testing.T) {
	r, nodeID := newRepo(t)
	ctx := context.Background()
	n, _ := r.Create(ctx, NewInput{NodeID: nodeID, Anchor: 5, Body: "원본"}, 1)
	if err := r.Update(ctx, UpdateInput{ID: n.ID, Body: "수정됨"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := r.Get(ctx, n.ID)
	if got.Body != "수정됨" || got.Anchor != 5 {
		t.Errorf("Update changed too much: %+v", got)
	}
}

func TestRepo_Delete(t *testing.T) {
	r, nodeID := newRepo(t)
	ctx := context.Background()
	n, _ := r.Create(ctx, NewInput{NodeID: nodeID, Anchor: 1, Body: "x"}, 1)
	if err := r.Delete(ctx, n.ID); err != nil { t.Fatalf("Delete: %v", err) }
	if _, err := r.Get(ctx, n.ID); err == nil { t.Error("expected ErrNotFound after delete") }
}

func TestRepo_Get_notFound(t *testing.T) {
	r, _ := newRepo(t)
	if _, err := r.Get(context.Background(), "no-such-id"); err == nil {
		t.Error("expected ErrNotFound")
	}
}
```

3. **`repo.go`**:

```go
package note

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/google/uuid"
)

var ErrNotFound = errors.New("note not found")

type Repo struct { s *store.Store }

func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

func (r *Repo) Create(ctx context.Context, in NewInput, now int64) (Note, error) {
	if in.NodeID == "" { return Note{}, fmt.Errorf("create note: node_id required") }
	if in.Body == "" { return Note{}, fmt.Errorf("create note: body required") }
	id := uuid.NewString()
	if _, err := r.s.DB().ExecContext(ctx, `
INSERT INTO notes (id, node_id, anchor, body, created_at)
VALUES (?, ?, ?, ?, ?)`, id, in.NodeID, in.Anchor, in.Body, now); err != nil {
		return Note{}, err
	}
	return r.Get(ctx, id)
}

func (r *Repo) Get(ctx context.Context, id string) (Note, error) {
	row := r.s.DB().QueryRowContext(ctx, baseSelect+` WHERE id = ?`, id)
	n, err := scan(row)
	if errors.Is(err, sql.ErrNoRows) { return Note{}, ErrNotFound }
	return n, err
}

func (r *Repo) ListForNode(ctx context.Context, nodeID string) ([]Note, error) {
	rows, err := r.s.DB().QueryContext(ctx, baseSelect+` WHERE node_id = ? ORDER BY anchor ASC`, nodeID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []Note
	for rows.Next() {
		n, err := scan(rows)
		if err != nil { return nil, err }
		out = append(out, n)
	}
	return out, rows.Err()
}

func (r *Repo) Update(ctx context.Context, in UpdateInput) error {
	if in.ID == "" { return fmt.Errorf("update note: id required") }
	if in.Body == "" { return fmt.Errorf("update note: body required (use Delete to remove)") }
	res, err := r.s.DB().ExecContext(ctx, `UPDATE notes SET body = ? WHERE id = ?`, in.Body, in.ID)
	if err != nil { return err }
	if n, _ := res.RowsAffected(); n == 0 { return ErrNotFound }
	return nil
}

func (r *Repo) Delete(ctx context.Context, id string) error {
	res, err := r.s.DB().ExecContext(ctx, `DELETE FROM notes WHERE id = ?`, id)
	if err != nil { return err }
	if n, _ := res.RowsAffected(); n == 0 { return ErrNotFound }
	return nil
}

const baseSelect = `SELECT id, node_id, anchor, body, created_at FROM notes`

type scanner interface{ Scan(...any) error }

func scan(row scanner) (Note, error) {
	var n Note
	if err := row.Scan(&n.ID, &n.NodeID, &n.Anchor, &n.Body, &n.CreatedAt); err != nil {
		return Note{}, err
	}
	return n, nil
}
```

4. Verify: `cd engine && go test ./internal/note/... -race`.

5. Commit:
```bash
git add engine/internal/note/
git commit -m "feat(note): add Repo for margin notes (CRUD + ListForNode)"
```

---

### Task 2: `notes.*` RPC handlers + main.go wire

Files:
- Create: `engine/internal/rpc/handlers/notes.go`
- Create: `engine/internal/rpc/handlers/notes_test.go`
- Modify: `engine/cmd/linetta-engine/main.go`

1. **`notes_test.go`** — 5 handler tests mirroring `threads_test.go` shape:

```go
package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/note"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

type noteFix struct {
	store  *store.Store
	nr     *note.Repo
	nodeID string
}

func newNoteFixture(t *testing.T) noteFix {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil { t.Fatalf("store.Open: %v", err) }
	t.Cleanup(func() { _ = s.Close() })
	pr := project.NewRepo(s)
	p, _ := pr.Create(context.Background(), 1, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first",
	})
	return noteFix{store: s, nr: note.NewRepo(s), nodeID: *p.LastOpenedNodeID}
}

func TestCreateNoteHandler(t *testing.T) {
	f := newNoteFixture(t)
	res, err := CreateNote(f.nr, func() int64 { return 9000 })(context.Background(),
		json.RawMessage(`{"node_id":"`+f.nodeID+`","anchor":12,"body":"여기 톤 바꾸기"}`))
	if err != nil { t.Fatalf("handler: %v", err) }
	var n note.Note
	_ = json.Unmarshal(res, &n)
	if n.Anchor != 12 || n.Body != "여기 톤 바꾸기" || n.CreatedAt != 9000 {
		t.Errorf("note = %+v", n)
	}
}

func TestListNotesForNodeHandler(t *testing.T) {
	f := newNoteFixture(t)
	_, _ = f.nr.Create(context.Background(), note.NewInput{NodeID: f.nodeID, Anchor: 5, Body: "A"}, 1)
	_, _ = f.nr.Create(context.Background(), note.NewInput{NodeID: f.nodeID, Anchor: 1, Body: "B"}, 2)
	res, err := ListNotesForNode(f.nr)(context.Background(),
		json.RawMessage(`{"node_id":"`+f.nodeID+`"}`))
	if err != nil { t.Fatalf("handler: %v", err) }
	var list []note.Note
	_ = json.Unmarshal(res, &list)
	if len(list) != 2 || list[0].Body != "B" || list[1].Body != "A" {
		t.Errorf("order = %+v", list)
	}
}

func TestGetNoteHandler(t *testing.T) {
	f := newNoteFixture(t)
	n, _ := f.nr.Create(context.Background(), note.NewInput{NodeID: f.nodeID, Anchor: 0, Body: "메모"}, 1)
	res, err := GetNote(f.nr)(context.Background(),
		json.RawMessage(`{"id":"`+n.ID+`"}`))
	if err != nil { t.Fatalf("handler: %v", err) }
	var got note.Note
	_ = json.Unmarshal(res, &got)
	if got.Body != "메모" { t.Errorf("got = %+v", got) }
}

func TestUpdateNoteHandler(t *testing.T) {
	f := newNoteFixture(t)
	n, _ := f.nr.Create(context.Background(), note.NewInput{NodeID: f.nodeID, Anchor: 3, Body: "원본"}, 1)
	res, err := UpdateNote(f.nr)(context.Background(),
		json.RawMessage(`{"id":"`+n.ID+`","body":"수정됨"}`))
	if err != nil { t.Fatalf("handler: %v", err) }
	var got note.Note
	_ = json.Unmarshal(res, &got)
	if got.Body != "수정됨" || got.Anchor != 3 { t.Errorf("got = %+v", got) }
}

func TestDeleteNoteHandler(t *testing.T) {
	f := newNoteFixture(t)
	n, _ := f.nr.Create(context.Background(), note.NewInput{NodeID: f.nodeID, Anchor: 0, Body: "x"}, 1)
	if _, err := DeleteNote(f.nr)(context.Background(),
		json.RawMessage(`{"id":"`+n.ID+`"}`)); err != nil { t.Fatalf("Delete: %v", err) }
	if _, err := f.nr.Get(context.Background(), n.ID); err == nil {
		t.Error("expected ErrNotFound after delete")
	}
}
```

2. **`notes.go`** (handlers):

```go
package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/devlikebear/linetta/engine/internal/note"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

func CreateNote(repo *note.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in note.NewInput
		if err := json.Unmarshal(params, &in); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		if in.NodeID == "" || in.Body == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node_id and body required"}
		}
		n, err := repo.Create(ctx, in, now())
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(n)
	}
}

type listNotesParams struct {
	NodeID string `json:"node_id"`
}

func ListNotesForNode(repo *note.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p listNotesParams
		if err := json.Unmarshal(params, &p); err != nil || p.NodeID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node_id required"}
		}
		list, err := repo.ListForNode(ctx, p.NodeID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if list == nil { list = []note.Note{} }
		return json.Marshal(list)
	}
}

func GetNote(repo *note.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		n, err := repo.Get(ctx, p.ID)
		if errors.Is(err, note.ErrNotFound) {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "note not found"}
		}
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(n)
	}
}

func UpdateNote(repo *note.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in note.UpdateInput
		if err := json.Unmarshal(params, &in); err != nil || in.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := repo.Update(ctx, in); err != nil {
			if errors.Is(err, note.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "note not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		got, err := repo.Get(ctx, in.ID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(got)
	}
}

func DeleteNote(repo *note.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := repo.Delete(ctx, p.ID); err != nil {
			if errors.Is(err, note.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "note not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(map[string]bool{"ok": true})
	}
}
```

3. **Wire main.go**:
- Add import `"github.com/devlikebear/linetta/engine/internal/note"`
- After `beats := beat.NewRepo(st)` add `notes := note.NewRepo(st)`
- Register 5 handlers near the threads.* block:

```go
s.Handle("notes.create", handlers.CreateNote(notes, clock))
s.Handle("notes.list_for_node", handlers.ListNotesForNode(notes))
s.Handle("notes.get", handlers.GetNote(notes))
s.Handle("notes.update", handlers.UpdateNote(notes))
s.Handle("notes.delete", handlers.DeleteNote(notes))
```

(Task 3 will update the `ai.NewContextBuilder(...)` line to pass `notes` as 6th arg.)

4. Verify + commit:
```bash
cd engine && go test ./... -race && go build ./...
git add engine/internal/rpc/handlers/notes.go engine/internal/rpc/handlers/notes_test.go engine/cmd/linetta-engine/main.go
git commit -m "feat(rpc): notes.* handlers wired into engine main"
```

---

### Task 3: AI context — `## 작가 주석` section

Files: `ai.go`, `context.go`, `context_test.go`, `prompts.go`, `prompts_test.go`, `main.go`.

1. **`ai.go`** — add type:

```go
// NoteBrief is a margin-note line sent to the LLM. Anchor stays in the JSON
// payload (for ai_runs inspection) but is omitted from the prompt text.
type NoteBrief struct {
	Anchor int    `json:"anchor"`
	Body   string `json:"body"`
}
```

Add field to `Context` struct after `ActiveThreads`:

```go
Notes []NoteBrief `json:"notes"`
```

2. **`prompts_test.go`** — append failing test:

```go
func TestBuildUser_includesNotesSection(t *testing.T) {
	c := Context{
		SceneLabel: "씬 1",
		SceneText:  "본문",
		Notes: []NoteBrief{
			{Anchor: 5, Body: "여기 톤을 더 차갑게"},
			{Anchor: 22, Body: "@해진의 대사로 받기"},
		},
		UserPrompt: "확장",
	}
	msgs := BuildMessages(c)
	user := msgs[1].Content
	if !strings.Contains(user, "## 작가 주석") {
		t.Fatalf("missing 작가 주석 header: %q", user)
	}
	if !strings.Contains(user, "- 여기 톤을 더 차갑게") || !strings.Contains(user, "- @해진의 대사로 받기") {
		t.Errorf("missing note bodies: %q", user)
	}
	if strings.Contains(user, "anchor") {
		t.Errorf("anchor key leaked: %q", user)
	}
}
```

3. **`prompts.go::buildUser`** — between the `활성 스토리라인` block and the `if !c.Options.TonePreset && ... StyleNotes` block, insert:

```go
if len(c.Notes) > 0 {
	b.WriteString("## 작가 주석\n")
	for _, n := range c.Notes {
		b.WriteString("- ")
		b.WriteString(n.Body)
		b.WriteString("\n")
	}
	b.WriteString("\n")
}
```

4. **`context.go`** — extend constructor + Build:

```go
import (
	// existing
	"github.com/devlikebear/linetta/engine/internal/note"
)

type ContextBuilder struct {
	projects *project.Repo
	nodes    *node.Repo
	mentions *mention.Repo
	threads  *thread.Repo
	beats    *beat.Repo
	notes    *note.Repo
}

func NewContextBuilder(projects *project.Repo, nodes *node.Repo, mentions *mention.Repo, threads *thread.Repo, beats *beat.Repo, notes *note.Repo) *ContextBuilder {
	return &ContextBuilder{projects: projects, nodes: nodes, mentions: mentions, threads: threads, beats: beats, notes: notes}
}
```

In `Build`, after the `active, err := b.loadActiveThreads(...)` block and BEFORE the return:

```go
var noteBriefs []NoteBrief
if b.notes != nil {
	ns, err := b.notes.ListForNode(ctx, nodeID)
	if err != nil { return Context{}, err }
	noteBriefs = make([]NoteBrief, 0, len(ns))
	for _, n := range ns {
		noteBriefs = append(noteBriefs, NoteBrief{Anchor: n.Anchor, Body: n.Body})
	}
}
```

Add to returned struct: `Notes: noteBriefs,`.

5. **`context_test.go`** — update all `NewContextBuilder(...)` call sites to pass 6th arg `note.NewRepo(s)`. Add import. Add one new test:

```go
func TestBuildContext_includesNotesForNode(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil { t.Fatalf("store.Open: %v", err) }
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
	nr := note.NewRepo(s)
	_, _ = nr.Create(context.Background(), note.NewInput{NodeID: *p.LastOpenedNodeID, Anchor: 7, Body: "톤 바꾸기"}, 1000)

	builder := NewContextBuilder(pr, nodes, mr, thread.NewRepo(s), beat.NewRepo(s), nr)
	got, err := builder.Build(context.Background(), *p.LastOpenedNodeID, "확장", Options{})
	if err != nil { t.Fatalf("Build: %v", err) }
	if len(got.Notes) != 1 || got.Notes[0].Body != "톤 바꾸기" || got.Notes[0].Anchor != 7 {
		t.Errorf("notes = %+v", got.Notes)
	}
}
```

6. **`main.go`** — update `ai.NewContextBuilder(projects, nodes, mentions, threads, beats)` → `ai.NewContextBuilder(projects, nodes, mentions, threads, beats, notes)`.

7. Verify + commit:
```bash
cd engine && go test ./... -race && go build ./...
git add engine/internal/ai/ engine/cmd/linetta-engine/main.go
git commit -m "feat(ai): inject 작가 주석 section into AI context"
```

---

## Phase B: Frontend (3 tasks)

### Task 4: TS types + `notes` rpc namespace

Files: `types.ts`, `rpc.ts`.

1. Append to `types.ts`:

```ts
// Mirrors engine/internal/note Note struct.
export interface Note {
  id: string;
  node_id: string;
  anchor: number;
  body: string;
  created_at: number;
}

export interface NewNoteInput {
  node_id: string;
  anchor: number;
  body: string;
}

export interface UpdateNoteInput {
  id: string;
  body: string;
}

export interface NoteBrief {
  anchor: number;
  body: string;
}
```

2. Append to `rpc.ts` (add 3 imports + 1 namespace):

```ts
import type {
  // ... existing
  NewNoteInput,
  Note,
  UpdateNoteInput,
} from "./types";

export const notes = {
  create: (input: NewNoteInput) => rpcCall<Note>("notes.create", input),
  listForNode: (nodeId: string) =>
    rpcCall<Note[]>("notes.list_for_node", { node_id: nodeId }),
  get: (id: string) => rpcCall<Note>("notes.get", { id }),
  update: (input: UpdateNoteInput) => rpcCall<Note>("notes.update", input),
  delete: (id: string) => rpcCall<{ ok: true }>("notes.delete", { id }),
};
```

3. Verify + commit:
```bash
cd apps/desktop && pnpm tsc -b
git add apps/desktop/src/lib/types.ts apps/desktop/src/lib/rpc.ts
git commit -m "feat(rpc): add notes namespace + Note types to frontend"
```

---

### Task 5: `NoteMarkerExtension` + `NotePopover`

Files:
- Create: `apps/desktop/src/components/editor/NoteMarkerExtension.ts`
- Create: `apps/desktop/src/components/NotePopover.tsx`
- Create: `apps/desktop/src/components/NotePopover.css`

1. **`NoteMarkerExtension.ts`**:

```ts
import { Node, mergeAttributes, type RawCommands } from "@tiptap/core";

declare module "@tiptap/core" {
  interface Commands<ReturnType> {
    noteMarker: {
      addNoteMarker: (noteId: string) => ReturnType;
      removeNoteMarker: (noteId: string) => ReturnType;
    };
  }
}

export const NoteMarkerExtension = Node.create({
  name: "noteMarker",

  inline: true,
  group: "inline",
  atom: true,
  selectable: true,
  draggable: false,

  addAttributes() {
    return {
      noteId: {
        default: null,
        parseHTML: (el) => (el as HTMLElement).getAttribute("data-note-id"),
        renderHTML: (attrs) => ({ "data-note-id": attrs.noteId }),
      },
    };
  },

  parseHTML() {
    return [{ tag: "span.note-marker[data-note-id]" }];
  },

  renderHTML({ HTMLAttributes }) {
    return [
      "span",
      mergeAttributes(HTMLAttributes, { class: "note-marker" }),
      "☘︎",
    ];
  },

  addNodeView() {
    return ({ node }) => {
      const dom = document.createElement("span");
      dom.className = "note-marker";
      dom.setAttribute("data-note-id", node.attrs.noteId ?? "");
      dom.setAttribute("contenteditable", "false");
      dom.textContent = "☘︎";

      const onEnter = () => {
        window.dispatchEvent(new CustomEvent("linetta:note-hover", {
          detail: { noteId: node.attrs.noteId, target: dom },
        }));
      };
      const onLeave = () => {
        window.dispatchEvent(new CustomEvent("linetta:note-hover-end", {
          detail: { noteId: node.attrs.noteId },
        }));
      };
      const onClick = (e: MouseEvent) => {
        e.preventDefault();
        e.stopPropagation();
        window.dispatchEvent(new CustomEvent("linetta:note-click", {
          detail: { noteId: node.attrs.noteId, target: dom },
        }));
      };

      dom.addEventListener("mouseenter", onEnter);
      dom.addEventListener("mouseleave", onLeave);
      dom.addEventListener("mousedown", onClick);

      return {
        dom,
        destroy() {
          dom.removeEventListener("mouseenter", onEnter);
          dom.removeEventListener("mouseleave", onLeave);
          dom.removeEventListener("mousedown", onClick);
        },
      };
    };
  },

  addCommands(): Partial<RawCommands> {
    return {
      addNoteMarker:
        (noteId: string) =>
        ({ chain }) =>
          chain()
            .focus()
            .insertContent({ type: "noteMarker", attrs: { noteId } })
            .run(),
      removeNoteMarker:
        (noteId: string) =>
        ({ state, dispatch, tr }) => {
          let foundPos = -1;
          let foundSize = 0;
          state.doc.descendants((n, pos) => {
            if (foundPos !== -1) return false;
            if (n.type.name === "noteMarker" && n.attrs.noteId === noteId) {
              foundPos = pos;
              foundSize = n.nodeSize;
              return false;
            }
            return true;
          });
          if (foundPos === -1) return false;
          if (dispatch) {
            dispatch(tr.delete(foundPos, foundPos + foundSize));
          }
          return true;
        },
    };
  },
});
```

2. **`NotePopover.css`**:

```css
.note-marker {
  display: inline-block;
  font-size: 0.9em;
  color: #c08a3e;
  cursor: pointer;
  user-select: none;
  padding: 0 1px;
  border-radius: 2px;
  transition: background 0.12s ease;
}
.note-marker:hover { background: rgba(192, 138, 62, 0.18); }

.note-popover {
  position: absolute;
  width: 280px;
  background: #fff;
  border: 1px solid rgba(0, 0, 0, 0.08);
  border-radius: 6px;
  box-shadow: 0 6px 24px rgba(0, 0, 0, 0.12);
  padding: 12px;
  z-index: 80;
  font-size: 14px;
  color: #222;
}
.note-popover .note-body {
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.5;
}
.note-popover textarea {
  width: 100%;
  min-height: 80px;
  resize: vertical;
  font: inherit;
  border: 1px solid rgba(0, 0, 0, 0.12);
  border-radius: 4px;
  padding: 6px 8px;
  box-sizing: border-box;
}
.note-popover .note-actions {
  display: flex;
  gap: 8px;
  justify-content: flex-end;
  margin-top: 8px;
}
.note-popover button {
  padding: 4px 10px;
  border: 1px solid rgba(0, 0, 0, 0.15);
  border-radius: 4px;
  background: #f7f7f7;
  cursor: pointer;
  font-size: 13px;
}
.note-popover button.primary { background: #c08a3e; color: #fff; border-color: #a87530; }
.note-popover button.danger { color: #c0392b; border-color: rgba(192, 57, 43, 0.4); }
.note-popover button:hover { filter: brightness(0.96); }
```

3. **`NotePopover.tsx`**:

```tsx
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { notes as notesApi } from "../lib/rpc";
import type { Note } from "../lib/types";
import "./NotePopover.css";

interface Props {
  noteId: string;
  targetEl: HTMLElement | null;
  mode: "read" | "edit";
  onClose: () => void;
  onSaved?: (n: Note) => void;
  onDeleted?: (noteId: string) => void;
}

export function NotePopover({ noteId, targetEl, mode, onClose, onSaved, onDeleted }: Props) {
  const popoverRef = useRef<HTMLDivElement | null>(null);
  const [note, setNote] = useState<Note | null>(null);
  const [draft, setDraft] = useState("");
  const [busy, setBusy] = useState(false);
  const [pos, setPos] = useState<{ left: number; top: number } | null>(null);

  useEffect(() => {
    let cancelled = false;
    notesApi.get(noteId).then(
      (n) => {
        if (cancelled) return;
        setNote(n);
        setDraft(n.body);
      },
      () => { if (!cancelled) onClose(); },
    );
    return () => { cancelled = true; };
  }, [noteId, onClose]);

  useLayoutEffect(() => {
    if (!targetEl) return;
    const rect = targetEl.getBoundingClientRect();
    const popW = 280;
    let left = rect.left + window.scrollX;
    if (left + popW > window.innerWidth - 12) {
      left = window.innerWidth - popW - 12;
    }
    const top = rect.bottom + window.scrollY + 6;
    setPos({ left, top });
  }, [targetEl, note]);

  useEffect(() => {
    if (mode !== "edit") return;
    const handler = (e: MouseEvent) => {
      if (!popoverRef.current) return;
      if (popoverRef.current.contains(e.target as Node)) return;
      if (targetEl && targetEl.contains(e.target as Node)) return;
      onClose();
    };
    window.addEventListener("mousedown", handler);
    return () => window.removeEventListener("mousedown", handler);
  }, [mode, onClose, targetEl]);

  if (!note || !pos) return null;

  const save = async () => {
    const body = draft.trim();
    if (!body) return;
    setBusy(true);
    try {
      const updated = await notesApi.update({ id: noteId, body });
      onSaved?.(updated);
      onClose();
    } finally { setBusy(false); }
  };

  const remove = async () => {
    setBusy(true);
    try {
      await notesApi.delete(noteId);
      onDeleted?.(noteId);
      onClose();
    } finally { setBusy(false); }
  };

  return (
    <div
      ref={popoverRef}
      className="note-popover"
      style={{ left: pos.left, top: pos.top }}
      onMouseDown={(e) => e.stopPropagation()}
    >
      {mode === "read" ? (
        <div className="note-body">{note.body}</div>
      ) : (
        <>
          <textarea
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            autoFocus
            disabled={busy}
          />
          <div className="note-actions">
            <button className="danger" onClick={remove} disabled={busy}>삭제</button>
            <button className="primary" onClick={save} disabled={busy || !draft.trim()}>저장</button>
          </div>
        </>
      )}
    </div>
  );
}
```

4. Verify + commit:
```bash
cd apps/desktop && pnpm tsc -b
git add apps/desktop/src/components/editor/NoteMarkerExtension.ts apps/desktop/src/components/NotePopover.tsx apps/desktop/src/components/NotePopover.css
git commit -m "feat(editor): NoteMarker Tiptap extension + NotePopover component"
```

---

### Task 6: Integrate into Workspace + Tiptap handle

Files: `Workspace.tsx`, `Tiptap.tsx`.

1. **`Workspace.tsx` imports**:

```ts
import { NoteMarkerExtension } from "../components/editor/NoteMarkerExtension";
import { NotePopover } from "../components/NotePopover";
import { notes as notesApi } from "../lib/rpc";
```

2. **Register extension** — find `<TiptapEditor ... extensions={mentionExtension ? [mentionExtension] : []}` and change to:

```tsx
extensions={[
  ...(mentionExtension ? [mentionExtension] : []),
  NoteMarkerExtension,
]}
```

3. **State + listeners**:

```tsx
const [notePopover, setNotePopover] = useState<{
  noteId: string;
  targetEl: HTMLElement;
  mode: "read" | "edit";
} | null>(null);

useEffect(() => {
  const onHover = (e: Event) => {
    const ce = e as CustomEvent<{ noteId: string; target: HTMLElement }>;
    setNotePopover((cur) => {
      if (cur && cur.mode === "edit" && cur.noteId === ce.detail.noteId) return cur;
      return { noteId: ce.detail.noteId, targetEl: ce.detail.target, mode: "read" };
    });
  };
  const onHoverEnd = (e: Event) => {
    const ce = e as CustomEvent<{ noteId: string }>;
    setNotePopover((cur) => {
      if (!cur) return null;
      if (cur.mode === "edit") return cur;
      if (cur.noteId !== ce.detail.noteId) return cur;
      return null;
    });
  };
  const onClick = (e: Event) => {
    const ce = e as CustomEvent<{ noteId: string; target: HTMLElement }>;
    setNotePopover({ noteId: ce.detail.noteId, targetEl: ce.detail.target, mode: "edit" });
  };
  window.addEventListener("linetta:note-hover", onHover);
  window.addEventListener("linetta:note-hover-end", onHoverEnd);
  window.addEventListener("linetta:note-click", onClick);
  return () => {
    window.removeEventListener("linetta:note-hover", onHover);
    window.removeEventListener("linetta:note-hover-end", onHoverEnd);
    window.removeEventListener("linetta:note-click", onClick);
  };
}, []);
```

4. **Cmd+K command** (push into `commands` useMemo, near `mark-thread`):

```tsx
cmds.push({
  id: "add-note",
  section: "노드",
  label: "여백 주석 추가",
  run: async () => {
    const body = await promptDialog("여백 주석 본문", "");
    if (body === null) return;
    const trimmed = body.trim();
    if (!trimmed) return;
    const sel = editorRef.current?.getSelection();
    const anchor = sel?.from ?? 0;
    try {
      const created = await notesApi.create({
        node_id: load.node.id,
        anchor,
        body: trimmed,
      });
      if (sel) editorRef.current?.setSelection(sel);
      editorRef.current?.addNoteMarker(created.id);
    } catch (e) {
      showToast("주석 추가 실패: " + String(e));
    }
  },
});
```

5. **Render popover** at end of JSX:

```tsx
{notePopover && (
  <NotePopover
    noteId={notePopover.noteId}
    targetEl={notePopover.targetEl}
    mode={notePopover.mode}
    onClose={() => setNotePopover(null)}
    onDeleted={(id) => { editorRef.current?.removeNoteMarker(id); }}
  />
)}
```

6. **`Tiptap.tsx`** — add to `TiptapHandle` interface:

```ts
addNoteMarker: (noteId: string) => void;
removeNoteMarker: (noteId: string) => void;
```

Inside `useImperativeHandle`:

```ts
addNoteMarker: (noteId: string) => {
  (editor?.commands as any)?.addNoteMarker?.(noteId);
},
removeNoteMarker: (noteId: string) => {
  (editor?.commands as any)?.removeNoteMarker?.(noteId);
},
```

7. Verify + commit:
```bash
cd apps/desktop && pnpm tsc -b && pnpm build
git add apps/desktop/src/routes/Workspace.tsx apps/desktop/src/components/editor/Tiptap.tsx
git commit -m "feat(workspace): wire 여백 주석 popover + Cmd+K add command"
```

---

## Phase C: Smoke + tag (1 task)

### Task 7: Manual smoke + tag

Run `./scripts/build-engine.sh && LINETTA_HOME=/tmp/linetta-plan10 ./scripts/dev.sh`.

1. 작품 생성 → 씬 1에 200+ 자 입력.
2. 커서 위치에서 Cmd+K → `여백 주석 추가` → "이 부분 더 차갑게" → 저장.
3. ☘︎ 아이콘 등장. 본문 텍스트 그대로.
4. Hover → 본문 popover.
5. 마우스 떠남 → popover 닫힘.
6. 클릭 → 편집 popover.
7. 본문 수정 → 저장 → 다시 hover → 갱신된 본문.
8. 클릭 → 삭제 → 마커 사라짐.
9. 새 주석 추가 후 AI 모드 → 생성. `sqlite3 /tmp/linetta-plan10/library.db "SELECT json_extract(context_json,'$.notes') FROM ai_runs ORDER BY started_at DESC LIMIT 1"` → 주석 본문 보임.
10. `sqlite3 /tmp/linetta-plan10/library.db "SELECT prompt FROM ai_runs ORDER BY started_at DESC LIMIT 1"` 또는 prompt 들어있는 컬럼에서 `## 작가 주석` 섹션 확인 (단, prompt 컬럼이 별도로 없으면 context_json 본문 충분).
11. 앱 재시작 → 마커 그대로 (atom 노드는 saved doc에 있음).

태그:
```bash
git tag plan-10-margin-note-done
```

---

## Done conditions

- [ ] `go test ./... -race` green.
- [ ] `pnpm tsc -b && pnpm build` green.
- [ ] Smoke checklist all passes.
- [ ] `plan-10-margin-note-done` tag exists.
