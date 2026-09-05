package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/agentmemory"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

// memStub stands in for the repo so this test needs no database. The handler
// takes an interface for the same reason MCPController does: handlers must
// stay linkable on every build tag.
type memStub struct {
	bodies map[string]string
	saveFn func(scope agentmemory.Scope, projectID, body string) error
}

func (m *memStub) Load(_ context.Context, scope agentmemory.Scope, projectID string) (agentmemory.Document, error) {
	body := m.bodies[string(scope)+"|"+projectID]
	return agentmemory.Document{
		Scope: scope, ProjectID: projectID, Body: body,
		CharsUsed: len([]rune(body)), CharsBudget: scope.Budget(),
	}, nil
}

func (m *memStub) Save(_ context.Context, scope agentmemory.Scope, projectID, body string, now int64) (agentmemory.Document, error) {
	if m.saveFn != nil {
		if err := m.saveFn(scope, projectID, body); err != nil {
			return agentmemory.Document{}, err
		}
	}
	if m.bodies == nil {
		m.bodies = map[string]string{}
	}
	m.bodies[string(scope)+"|"+projectID] = body
	return agentmemory.Document{
		Scope: scope, ProjectID: projectID, Body: body,
		CharsUsed: len([]rune(body)), CharsBudget: scope.Budget(), UpdatedAt: now,
	}, nil
}

func TestGetMemoryReturnsBothDocuments(t *testing.T) {
	store := &memStub{bodies: map[string]string{
		"writer_profile|": "프로필",
		"work_notes|p1":   "노트",
	}}
	raw, err := GetMemory(store)(context.Background(), json.RawMessage(`{"project_id":"p1"}`))
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	var got struct {
		WriterProfile agentmemory.Document `json:"writer_profile"`
		WorkNotes     agentmemory.Document `json:"work_notes"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.WriterProfile.Body != "프로필" || got.WorkNotes.Body != "노트" {
		t.Errorf("got %+v", got)
	}
	if got.WorkNotes.CharsBudget != 2200 {
		t.Errorf("CharsBudget = %d", got.WorkNotes.CharsBudget)
	}
}

func TestGetMemoryWithNoWorkStillReturnsTheProfile(t *testing.T) {
	store := &memStub{bodies: map[string]string{"writer_profile|": "프로필"}}
	raw, err := GetMemory(store)(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if !strings.Contains(string(raw), "프로필") {
		t.Errorf("got %s", raw)
	}
}

func TestSetMemoryRejectsAnUnknownScope(t *testing.T) {
	_, err := SetMemory(&memStub{}, func() int64 { return 1 }, nil)(
		context.Background(), json.RawMessage(`{"scope":"nonsense","body":"x"}`))
	if err == nil {
		t.Fatal("want an error")
	}
}

// A writer pasting text with a zero-width space, or overrunning the budget,
// must get a usable message — not an opaque internal error.
func TestSetMemorySurfacesARefusalUsefully(t *testing.T) {
	store := &memStub{saveFn: func(agentmemory.Scope, string, string) error { return agentmemory.ErrOverBudget }}
	_, err := SetMemory(store, func() int64 { return 1 }, nil)(
		context.Background(), json.RawMessage(`{"scope":"writer_profile","body":"x"}`))
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "budget") {
		t.Errorf("the message must say what went wrong; got %v", err)
	}
}

func TestSetMemoryNotifies(t *testing.T) {
	var method string
	notify := func(m string, _ any) { method = m }
	if _, err := SetMemory(&memStub{}, func() int64 { return 1 }, notify)(
		context.Background(), json.RawMessage(`{"scope":"writer_profile","body":"x"}`)); err != nil {
		t.Fatalf("SetMemory: %v", err)
	}
	if method != "memory.changed" {
		t.Errorf("method = %q — another window would show a stale textarea", method)
	}
}

// realMemoryRepo opens a real on-disk store and returns the actual
// agentmemory.Repo plus one real work id. Mirrors
// agentmemory/agentmemory_test.go's seedRepo, and exists for one reason:
// memStub cannot fail the way the repo fails.
//
// memStub's Load never returns an error, so it cannot reproduce
// projectArg's refusal of work notes with no work id — which is exactly the
// refusal memory.get used to walk into, taking the writer profile down with
// it, while the stubbed suite stayed green. A fake that cannot reproduce the
// store's validation will hide the next bug of that shape too, so the
// no-work path is tested against the real thing. The stub keeps the paths
// where a fake is the right tool (forcing a Save failure).
//
// This does not put tars/pkg/llm into the handlers test binary: neither
// internal/store nor internal/project links it, and
// scripts/validate-story-core-deps.sh checks ./internal/rpc/handlers with
// `go list -test -deps` on every run.
func realMemoryRepo(t *testing.T) (context.Context, *agentmemory.Repo, string) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	p, err := project.NewRepo(st).Create(ctx, 1, project.NewInput{
		Title: "작품", Genres: []string{"fantasy"}, LengthTarget: "novel", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	return ctx, agentmemory.NewRepo(st.DB()), p.ID
}

// The Settings pane opens before the writer has picked a work, so this is the
// first memory.get the app ever makes. Against the real repo it used to fail
// outright — Load refuses work notes with no work id — and the profile the
// handler had already read successfully was thrown away with it.
func TestGetMemoryWithNoWorkStillReturnsTheProfileAgainstTheRealStore(t *testing.T) {
	ctx, repo, _ := realMemoryRepo(t)
	if _, err := repo.Save(ctx, agentmemory.ScopeWriterProfile, "", "짧은 문장을 쓴다", 1000); err != nil {
		t.Fatalf("seed the profile: %v", err)
	}

	raw, err := GetMemory(repo)(ctx, json.RawMessage(`{"project_id":""}`))
	if err != nil {
		t.Fatalf("GetMemory with no work must succeed: %v", err)
	}
	var got getMemoryResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.WriterProfile.Body != "짧은 문장을 쓴다" {
		t.Errorf("the profile must survive a call with no work; got %+v", got.WriterProfile)
	}
	if got.WriterProfile.CharsBudget != 1400 {
		t.Errorf("WriterProfile.CharsBudget = %d, want 1400", got.WriterProfile.CharsBudget)
	}
	if got.WorkNotes.Body != "" {
		t.Errorf("WorkNotes.Body = %q, want empty when no work is picked", got.WorkNotes.Body)
	}
	if got.WorkNotes.Scope != agentmemory.ScopeWorkNotes {
		t.Errorf("WorkNotes.Scope = %q, want %q", got.WorkNotes.Scope, agentmemory.ScopeWorkNotes)
	}
	if got.WorkNotes.CharsBudget != 2200 {
		t.Errorf("WorkNotes.CharsBudget = %d — the pane draws its capacity line from this "+
			"before anything is typed", got.WorkNotes.CharsBudget)
	}
}

// The same call once a work is open still reads the row, so the empty-work
// branch above cannot be "return empty forever".
func TestGetMemoryWithAWorkReadsTheRealRow(t *testing.T) {
	ctx, repo, projectID := realMemoryRepo(t)
	if _, err := repo.Save(ctx, agentmemory.ScopeWorkNotes, projectID, "민준은 3화부터 존댓말", 1000); err != nil {
		t.Fatalf("seed the notes: %v", err)
	}
	raw, err := GetMemory(repo)(ctx, json.RawMessage(`{"project_id":`+quote(projectID)+`}`))
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	var got getMemoryResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.WorkNotes.Body != "민준은 3화부터 존댓말" {
		t.Errorf("WorkNotes.Body = %q", got.WorkNotes.Body)
	}
	if got.WorkNotes.ProjectID != projectID {
		t.Errorf("WorkNotes.ProjectID = %q, want %q", got.WorkNotes.ProjectID, projectID)
	}
}

// A work id that is not in the library reaches SQLite as a foreign-key
// refusal. The writer must get a sentence, not the driver's own text.
func TestSetMemoryOnAnUnknownWorkSaysSomethingUsable(t *testing.T) {
	ctx, repo, _ := realMemoryRepo(t)
	_, err := SetMemory(repo, func() int64 { return 1 }, nil)(
		ctx, json.RawMessage(`{"scope":"work_notes","project_id":"no-such-work","body":"메모"}`))
	if err == nil {
		t.Fatal("saving notes against a work that is not there must be refused")
	}
	if strings.Contains(err.Error(), "FOREIGN KEY") || strings.Contains(err.Error(), "constraint failed") {
		t.Errorf("the SQLite driver's own text must not reach the writer; got %v", err)
	}
	if !strings.Contains(err.Error(), "no-such-work") {
		t.Errorf("the message must name the work it could not find; got %v", err)
	}
}

func quote(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}
