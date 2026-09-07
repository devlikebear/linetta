package companion

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/agentmemory"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

// curatedFixture is the adapter's whole world: a store, a work to hang the
// notes off, and the memory repo the Service reads through.
type curatedFixture struct {
	svc       *Service
	memories  *agentmemory.Repo
	store     *store.Store
	projectID string
}

func newCuratedFixture(t *testing.T) (context.Context, *curatedFixture) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	p, err := project.NewRepo(st).Create(ctx, 1, project.NewInput{
		Title: "작품", Genres: []string{"fantasy"}, LengthTarget: "novel", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	repo := agentmemory.NewRepo(st.DB())
	return ctx, &curatedFixture{
		svc:       NewService(t.TempDir()).WithCuratedMemory(repo),
		memories:  repo,
		store:     st,
		projectID: p.ID,
	}
}

// breakRow makes one scope's row unreadable without touching the other. The
// column is INTEGER NOT NULL, but SQLite stores what it cannot coerce, so the
// row survives the write and fails in Load's Scan into int64 — the narrowest
// way to fail exactly one of the two reads.
func breakRow(t *testing.T, f *curatedFixture, scope agentmemory.Scope, projectArg any) {
	t.Helper()
	db := f.store.DB()
	if _, err := db.Exec(
		`DELETE FROM agent_memory WHERE scope = ? AND project_id IS ?`, string(scope), projectArg); err != nil {
		t.Fatalf("clear row: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO agent_memory (scope, project_id, body, updated_at) VALUES (?, ?, ?, ?)`,
		string(scope), projectArg, "무엇이든", "not-a-number"); err != nil {
		t.Fatalf("corrupt row: %v", err)
	}
	if _, err := f.memories.Load(context.Background(), scope, stringOrEmpty(projectArg)); err == nil {
		t.Fatalf("scope %s still loads cleanly; this test can no longer fail", scope)
	}
}

func stringOrEmpty(v any) string {
	s, _ := v.(string)
	return s
}

// A Service built without WithCuratedMemory is the mobile/no-database shape.
// It must answer empty rather than panic: the brief still has to build.
func TestCuratedMemoryWithoutRepoIsEmpty(t *testing.T) {
	profile, notes := NewService(t.TempDir()).CuratedMemory(context.Background(), "p1")
	if profile != "" || notes != "" {
		t.Errorf("got %q / %q, want both empty", profile, notes)
	}
}

func TestCuratedMemoryReturnsBothDocuments(t *testing.T) {
	ctx, f := newCuratedFixture(t)
	if _, err := f.memories.Save(ctx, agentmemory.ScopeWriterProfile, "", "줄표 쓰지 않기", 1); err != nil {
		t.Fatalf("save profile: %v", err)
	}
	if _, err := f.memories.Save(ctx, agentmemory.ScopeWorkNotes, f.projectID, "민준은 3화부터 존댓말", 2); err != nil {
		t.Fatalf("save notes: %v", err)
	}
	profile, notes := f.svc.CuratedMemory(ctx, f.projectID)
	if profile != "줄표 쓰지 않기" || notes != "민준은 3화부터 존댓말" {
		t.Errorf("got %q / %q", profile, notes)
	}
}

// Nothing recorded yet is the normal case, not an error: agentmemory.Load
// returns an empty document for a missing row.
func TestCuratedMemoryWithNothingRecordedIsEmpty(t *testing.T) {
	ctx, f := newCuratedFixture(t)
	profile, notes := f.svc.CuratedMemory(ctx, f.projectID)
	if profile != "" || notes != "" {
		t.Errorf("got %q / %q, want both empty", profile, notes)
	}
}

// The two reads are independent. A failing profile read must not take the work
// notes down with it — the reads were once sequenced so it did.
func TestCuratedMemoryProfileFailureStillReturnsTheNotes(t *testing.T) {
	ctx, f := newCuratedFixture(t)
	if _, err := f.memories.Save(ctx, agentmemory.ScopeWorkNotes, f.projectID, "민준은 3화부터 존댓말", 2); err != nil {
		t.Fatalf("save notes: %v", err)
	}
	breakRow(t, f, agentmemory.ScopeWriterProfile, nil)

	profile, notes := f.svc.CuratedMemory(ctx, f.projectID)
	if profile != "" {
		t.Errorf("profile = %q, want empty — the unreadable document must not reach the brief", profile)
	}
	if notes != "민준은 3화부터 존댓말" {
		t.Errorf("notes = %q, want the saved notes — a profile failure must not cost them", notes)
	}
}

// The mirror image. A work-notes read fails on its own whenever the project id
// is empty, which needs no corruption at all.
func TestCuratedMemoryNotesFailureStillReturnsTheProfile(t *testing.T) {
	ctx, f := newCuratedFixture(t)
	if _, err := f.memories.Save(ctx, agentmemory.ScopeWriterProfile, "", "줄표 쓰지 않기", 1); err != nil {
		t.Fatalf("save profile: %v", err)
	}
	profile, notes := f.svc.CuratedMemory(ctx, "")
	if profile != "줄표 쓰지 않기" {
		t.Errorf("profile = %q, want the saved profile — a notes failure must not cost it", profile)
	}
	if notes != "" {
		t.Errorf("notes = %q, want empty", notes)
	}
}
