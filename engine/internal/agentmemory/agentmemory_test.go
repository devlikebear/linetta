package agentmemory

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

// seedRepo opens a real on-disk store under t.TempDir() and creates one work,
// because agent_memory.project_id has a foreign key and store.Open turns
// PRAGMA foreign_keys on. Mirrors companion/history_test.go's seedWork.
func seedRepo(t *testing.T) (context.Context, *Repo, string) {
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
	return ctx, NewRepo(st.DB()), p.ID
}

func TestLoadMissingReturnsAnEmptyDocumentNotAnError(t *testing.T) {
	ctx, repo, projectID := seedRepo(t)
	got, err := repo.Load(ctx, ScopeWorkNotes, projectID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Body != "" || got.CharsUsed != 0 {
		t.Errorf("want an empty document, got %+v", got)
	}
	if got.CharsBudget != 2200 {
		t.Errorf("CharsBudget = %d, want 2200 even when the row is absent", got.CharsBudget)
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	ctx, repo, projectID := seedRepo(t)
	if _, err := repo.Save(ctx, ScopeWorkNotes, projectID, "민준은 3화부터 존댓말", 1000); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.Load(ctx, ScopeWorkNotes, projectID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Body != "민준은 3화부터 존댓말" {
		t.Errorf("Body = %q", got.Body)
	}
	if got.UpdatedAt != 1000 {
		t.Errorf("UpdatedAt = %d, want 1000", got.UpdatedAt)
	}
	if got.CharsUsed != len([]rune("민준은 3화부터 존댓말")) {
		t.Errorf("CharsUsed = %d — it must be runes, not bytes", got.CharsUsed)
	}
}

func TestSaveReplacesRatherThanAppending(t *testing.T) {
	ctx, repo, projectID := seedRepo(t)
	if _, err := repo.Save(ctx, ScopeWorkNotes, projectID, "first", 1); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := repo.Save(ctx, ScopeWorkNotes, projectID, "second", 2); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, _ := repo.Load(ctx, ScopeWorkNotes, projectID)
	if got.Body != "second" {
		t.Fatalf("Body = %q, want the second save to have replaced the first", got.Body)
	}
}

// The global row and a work's row share the scope column; the two partial
// unique indexes are what keep them apart.
func TestWriterProfileIsGlobalAndWorkNotesAreNot(t *testing.T) {
	ctx, repo, projectID := seedRepo(t)
	if _, err := repo.Save(ctx, ScopeWriterProfile, "", "줄표 쓰지 않기", 1); err != nil {
		t.Fatalf("Save profile: %v", err)
	}
	if _, err := repo.Save(ctx, ScopeWorkNotes, projectID, "작품 노트", 1); err != nil {
		t.Fatalf("Save notes: %v", err)
	}
	profile, _ := repo.Load(ctx, ScopeWriterProfile, "")
	notes, _ := repo.Load(ctx, ScopeWorkNotes, projectID)
	if profile.Body != "줄표 쓰지 않기" || notes.Body != "작품 노트" {
		t.Fatalf("the two scopes collided: profile=%q notes=%q", profile.Body, notes.Body)
	}
}

func TestSaveRefusesOverBudget(t *testing.T) {
	ctx, repo, _ := seedRepo(t)
	// Hangul, so a rune budget and a byte budget give different answers: 1401
	// runes is 4203 bytes. A byte budget would have rejected at 467 characters.
	if _, err := repo.Save(ctx, ScopeWriterProfile, "", strings.Repeat("가", 1401), 1); !errors.Is(err, ErrOverBudget) {
		t.Fatalf("Save 1401 runes = %v, want ErrOverBudget", err)
	}
	if _, err := repo.Save(ctx, ScopeWriterProfile, "", strings.Repeat("가", 1400), 1); err != nil {
		t.Fatalf("Save exactly 1400 runes = %v, want nil", err)
	}
}

// Apply's budgeted() helper lets a result through when it is over budget but
// shorter than what it replaces, so an agent can dig its way out of a
// document that is already too big. Save must accept the same shape of save
// once it reaches here — otherwise Apply would say an edit succeeded and
// Save would then refuse to persist it, leaving the agent stuck.
func TestSaveAllowsAShrinkingSaveEvenWhileStillOverBudget(t *testing.T) {
	ctx, repo, _ := seedRepo(t)
	// Seed a row directly, bypassing Save's own budget check — this is what a
	// hand-edited row, or a document from before a budget shrank, looks like.
	over := strings.Repeat("가", 2300)
	if _, err := repo.db.ExecContext(ctx,
		`INSERT INTO agent_memory (scope, project_id, body, updated_at) VALUES (?, ?, ?, ?)`,
		string(ScopeWriterProfile), nil, over, 1); err != nil {
		t.Fatalf("seed: %v", err)
	}
	shorter := strings.Repeat("가", 1500) // still over the 1400 budget, but shorter than 2300
	if _, err := repo.Save(ctx, ScopeWriterProfile, "", shorter, 2); err != nil {
		t.Fatalf("Save: %v — a shrinking save must be allowed even while still over budget", err)
	}
	got, err := repo.Load(ctx, ScopeWriterProfile, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Body != shorter {
		t.Fatalf("Body was not saved: got %q", got.Body)
	}

	// But a save that GROWS from that same over-budget starting point must
	// still be refused — the escape hatch is for shrinking only.
	grown := strings.Repeat("가", 1600)
	if _, err := repo.Save(ctx, ScopeWriterProfile, "", grown, 3); !errors.Is(err, ErrOverBudget) {
		t.Fatalf("Save (growing, still over budget) = %v, want ErrOverBudget", err)
	}
	got, _ = repo.Load(ctx, ScopeWriterProfile, "")
	if got.Body != shorter {
		t.Fatalf("a refused save must leave the previous body intact; got %q", got.Body)
	}
}

func TestSaveScreens(t *testing.T) {
	ctx, repo, _ := seedRepo(t)
	if _, err := repo.Save(ctx, ScopeWriterProfile, "", "안녕​하세요", 1); !errors.Is(err, ErrInvisible) {
		t.Fatalf("Save = %v, want ErrInvisible — Screen must run before the write", err)
	}
	got, _ := repo.Load(ctx, ScopeWriterProfile, "")
	if got.Body != "" {
		t.Fatalf("a rejected save must not have written; Body = %q", got.Body)
	}
}

// A rejected save must leave the PREVIOUS memory intact, not just skip the
// write of the new one.
func TestARejectedSaveKeepsWhatWasThere(t *testing.T) {
	ctx, repo, _ := seedRepo(t)
	if _, err := repo.Save(ctx, ScopeWriterProfile, "", "지켜야 할 내용", 1); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := repo.Save(ctx, ScopeWriterProfile, "", strings.Repeat("가", 5000), 2); err == nil {
		t.Fatal("want a refusal")
	}
	got, _ := repo.Load(ctx, ScopeWriterProfile, "")
	if got.Body != "지켜야 할 내용" {
		t.Fatalf("Body = %q, want the earlier memory untouched", got.Body)
	}
}

func TestWorkNotesRequireAProjectAndTheProfileForbidsOne(t *testing.T) {
	ctx, repo, projectID := seedRepo(t)
	if _, err := repo.Save(ctx, ScopeWorkNotes, "", "x", 1); err == nil {
		t.Error("work notes with no work must be refused")
	}
	if _, err := repo.Save(ctx, ScopeWriterProfile, projectID, "x", 1); err == nil {
		t.Error("the writer profile is global; a work id must be refused rather than silently ignored")
	}
}

func TestParseScope(t *testing.T) {
	if s, err := ParseScope("work_notes"); err != nil || s != ScopeWorkNotes {
		t.Errorf("ParseScope(work_notes) = %v, %v", s, err)
	}
	if _, err := ParseScope("nonsense"); err == nil {
		t.Error("an unknown scope must be an error, not a zero Scope")
	}
}
