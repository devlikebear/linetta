package agentmemory

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// The one-connection pool guarantees no two statements run at the same
// instant, but it does not make "read the current row, decide, then
// delete-then-insert" one atomic unit — see Save's doc comment. A plain read
// taken before the transaction opens releases the connection the instant it
// returns, leaving a gap before BeginTx where a second, independent Save can
// run to completion and change the very row the first Save is about to
// decide against.
//
// This test forces that interleaving deterministically — no sleep-based
// timing decides pass or fail — using a hook Save invokes (via a context
// key it never sets itself outside tests) immediately after reading the
// stored row and before deciding anything from it:
//
//  1. Seed an over-budget row (2300 runes; the budget is 1400).
//  2. Call Save for A: writing 2000 runes (still over budget, but a shrink
//     relative to the seeded 2300, so the escape hatch should allow it).
//  3. A's hook starts Save for B on its own goroutine: writing 1500 runes
//     (also over budget, a shrink relative to 2300). The hook then waits —
//     with a bounded timeout used only as a deadlock guard, never as the
//     pass/fail signal — to see whether B finishes inside the wait.
//
// Pre-fix: A's read of "before" (via a plain query) has already released
// the connection by the time the hook runs, so B's entire read-decide-write
// completes inside the wait. The hook returns, and A resumes holding its
// stale before=2300 — 2000 still looks like a shrink against that stale
// value, so A's write is wrongly allowed even though the row actually there
// is now B's 1500 runes, and it silently overwrites B's shrink. The final
// body ends up A's 2000 runes.
//
// Post-fix: A's read happens inside its own open transaction, which holds
// the pool's one connection until A commits. B's BeginTx can only block on
// that connection — it is structurally unable to complete during the wait —
// so the hook's timeout branch fires (a blocked BeginTx never returns; this
// is not a close race) and A commits using the row genuinely still there
// (2300 vs 2000: correctly a shrink, allowed). B then runs, sees A's fresh
// 2000-rune row, and its own shrink to 1500 is still correctly allowed. The
// final body ends up B's 1500 runes: A's decision was made against the row
// that was actually there, and nothing was silently clobbered.
func TestSaveDecidesTheShrinkOnlyRuleInsideItsOwnTransaction(t *testing.T) {
	ctx, repo, _ := seedRepo(t)
	// Seed a row directly, bypassing Save's own budget check — same setup as
	// TestSaveAllowsAShrinkingSaveEvenWhileStillOverBudget.
	seed := strings.Repeat("가", 2300)
	if _, err := repo.db.ExecContext(ctx,
		`INSERT INTO agent_memory (scope, project_id, body, updated_at) VALUES (?, ?, ?, ?)`,
		string(ScopeWriterProfile), nil, seed, 1); err != nil {
		t.Fatalf("seed: %v", err)
	}

	bDone := make(chan struct{})
	bErr := make(chan error, 1)
	hook := func() {
		go func() {
			// Plain ctx, not hookCtx: B must not trip this same hook.
			_, err := repo.Save(ctx, ScopeWriterProfile, "", strings.Repeat("가", 1500), 2)
			bErr <- err
			close(bDone)
		}()
		select {
		case <-bDone:
			// Pre-fix path: B's whole Save fit in the gap.
		case <-time.After(500 * time.Millisecond):
			// Post-fix path: B is genuinely blocked on A's open transaction.
		}
	}
	hookCtx := context.WithValue(ctx, saveRaceHookKey{}, hook)

	if _, err := repo.Save(hookCtx, ScopeWriterProfile, "", strings.Repeat("가", 2000), 3); err != nil {
		t.Fatalf("A Save: %v", err)
	}
	select {
	case err := <-bErr:
		if err != nil {
			t.Fatalf("B Save: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("B Save never completed after A released the connection")
	}

	got, err := repo.Load(ctx, ScopeWriterProfile, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := strings.Repeat("가", 1500)
	if got.Body != want {
		t.Fatalf("Body has %d runes, want B's 1500 — A must decide the shrink-only rule "+
			"against the row inside its own transaction, not a value read before it opened",
			len([]rune(got.Body)))
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

// ---------- Edit ----------

func TestEditAppliesToWhatIsStoredAndPersists(t *testing.T) {
	ctx, repo, projectID := seedRepo(t)
	if _, err := repo.Edit(ctx, ScopeWorkNotes, projectID, ActionAdd, "", "민준은 3화부터 존댓말", 1); err != nil {
		t.Fatalf("first Edit: %v", err)
	}
	got, err := repo.Edit(ctx, ScopeWorkNotes, projectID, ActionAdd, "", "서연은 존댓말을 쓰지 않는다", 2)
	if err != nil {
		t.Fatalf("second Edit: %v", err)
	}
	want := "민준은 3화부터 존댓말\n서연은 존댓말을 쓰지 않는다"
	if got.Body != want {
		t.Fatalf("Body = %q, want %q — the second Edit must build on what the first wrote", got.Body, want)
	}
	if got.CharsUsed != len([]rune(want)) || got.CharsBudget != workNotesBudget || got.UpdatedAt != 2 {
		t.Errorf("Document = %+v", got)
	}
	stored, err := repo.Load(ctx, ScopeWorkNotes, projectID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if stored.Body != want {
		t.Errorf("stored Body = %q, want %q", stored.Body, want)
	}
}

// Edit's refusals are Apply's refusals, taken against the body Edit itself
// read, and a refused Edit must leave the document exactly as it was.
func TestARefusedEditWritesNothing(t *testing.T) {
	ctx, repo, projectID := seedRepo(t)
	if _, err := repo.Edit(ctx, ScopeWorkNotes, projectID, ActionAdd, "", "지켜야 할 내용", 1); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	cases := map[string]struct{ action, find, text string }{
		"no match":    {ActionRemove, "없음", ""},
		"bad action":  {"rewrite", "", "x"},
		"empty text":  {ActionAdd, "", "   "},
		"over budget": {ActionAdd, "", strings.Repeat("가", 2201)},
		"invisible":   {ActionAdd, "", "안녕​하세요"},
	}
	for name, c := range cases {
		if _, err := repo.Edit(ctx, ScopeWorkNotes, projectID, c.action, c.find, c.text, 2); err == nil {
			t.Errorf("%s: want a refusal", name)
		}
		got, err := repo.Load(ctx, ScopeWorkNotes, projectID)
		if err != nil {
			t.Fatalf("%s: Load: %v", name, err)
		}
		if got.Body != "지켜야 할 내용" {
			t.Fatalf("%s: Body = %q, want the earlier memory untouched", name, got.Body)
		}
	}
}

// The shrink-only escape hatch has to survive the move into one transaction:
// an agent must still be able to dig out of a document that is already over
// budget, or every remove it tries would itself be refused as over budget.
func TestEditKeepsTheShrinkOnlyEscapeHatch(t *testing.T) {
	ctx, repo, _ := seedRepo(t)
	// Seed directly, bypassing the budget check, the same way
	// TestSaveAllowsAShrinkingSaveEvenWhileStillOverBudget does.
	seed := strings.Repeat("가", 1500) + "\n" + strings.Repeat("나", 200)
	if _, err := repo.db.ExecContext(ctx,
		`INSERT INTO agent_memory (scope, project_id, body, updated_at) VALUES (?, ?, ?, ?)`,
		string(ScopeWriterProfile), nil, seed, 1); err != nil {
		t.Fatalf("seed: %v", err)
	}
	got, err := repo.Edit(ctx, ScopeWriterProfile, "", ActionRemove, "나", "", 2)
	if err != nil {
		t.Fatalf("Edit: %v — a shrinking edit must be allowed even while still over budget", err)
	}
	if got.Body != strings.Repeat("가", 1500) {
		t.Fatalf("Body has %d runes, want the 1500-rune line left behind", len([]rune(got.Body)))
	}
	// Growing an already-over-budget document is still refused.
	if _, err := repo.Edit(ctx, ScopeWriterProfile, "", ActionAdd, "", "한 줄 더", 3); !errors.Is(err, ErrOverBudget) {
		t.Fatalf("Edit = %v, want ErrOverBudget", err)
	}
}

// Two writers edit the same document at once — the built-in agent and a
// connected external client, which is this feature's normal shape, not a
// contrived one. Both edits must survive. This is the test that fails against
// a Load-then-Apply-then-Save caller: that caller reads outside any
// transaction, so B's whole read-modify-write lands in the gap between A's
// read and A's write, and A's save then discards B's line while having told
// B it succeeded.
func TestEditDoesNotLoseAConcurrentEdit(t *testing.T) {
	ctx, repo, projectID := seedRepo(t)
	if _, err := repo.Edit(ctx, ScopeWorkNotes, projectID, ActionAdd, "", "처음", 1); err != nil {
		t.Fatalf("seed Edit: %v", err)
	}

	bDone := make(chan struct{})
	bErr := make(chan error, 1)
	hook := func() {
		go func() {
			// Plain ctx, not hookCtx: B must not trip this same hook.
			_, err := repo.Edit(ctx, ScopeWorkNotes, projectID, ActionAdd, "", "B의 줄", 2)
			bErr <- err
			close(bDone)
		}()
		select {
		case <-bDone:
			// Pre-fix path: B's whole read-modify-write fit in the gap.
		case <-time.After(500 * time.Millisecond):
			// Post-fix path: B is genuinely blocked on A's open transaction.
		}
	}
	hookCtx := context.WithValue(ctx, editRaceHookKey{}, hook)

	if _, err := repo.Edit(hookCtx, ScopeWorkNotes, projectID, ActionAdd, "", "A의 줄", 3); err != nil {
		t.Fatalf("A Edit: %v", err)
	}
	select {
	case err := <-bErr:
		if err != nil {
			t.Fatalf("B Edit: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("B Edit never completed after A released the connection")
	}

	got, err := repo.Load(ctx, ScopeWorkNotes, projectID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := "처음\nA의 줄\nB의 줄"
	if got.Body != want {
		t.Fatalf("Body = %q, want %q — B must apply its edit to the body A committed, "+
			"and neither line may be lost", got.Body, want)
	}
}

func TestEditRefusesAMismatchedScopeAndWork(t *testing.T) {
	ctx, repo, projectID := seedRepo(t)
	if _, err := repo.Edit(ctx, ScopeWorkNotes, "", ActionAdd, "", "x", 1); err == nil {
		t.Error("work notes with no work must be refused")
	}
	if _, err := repo.Edit(ctx, ScopeWriterProfile, projectID, ActionAdd, "", "x", 1); err == nil {
		t.Error("the writer profile is global; a work id must be refused rather than silently ignored")
	}
}

// An unknown work id is a mistake a caller can act on — pick a real work —
// so it must arrive as a typed error carrying a sentence, not as the SQLite
// driver's own "constraint failed: FOREIGN KEY constraint failed". Every
// other refusal on this path (over budget, an invisible character, a
// markdown heading) already reads that way, and this one reaches both the
// Settings pane (memory.set) and an agent (linetta_edit_memory).
//
// These two also stand in for the driver check itself: isForeignKeyViolation
// matches SQLite's numeric result code rather than the driver's wording, and
// nothing else in the package exercises that. If modernc.org/sqlite stopped
// reporting SQLITE_CONSTRAINT_FOREIGNKEY through a `Code() int` — a driver
// upgrade, a swapped driver — these go red instead of the raw string
// quietly reaching a writer again.
func TestSaveOnAnUnknownWorkSaysSo(t *testing.T) {
	ctx, repo, _ := seedRepo(t)
	_, err := repo.Save(ctx, ScopeWorkNotes, "no-such-work", "민준은 3화부터 존댓말", 1000)
	if !errors.Is(err, ErrUnknownProject) {
		t.Fatalf("Save error = %v, want ErrUnknownProject", err)
	}
	if !strings.Contains(err.Error(), "no-such-work") {
		t.Errorf("the message must name the id the caller passed; got %v", err)
	}
	if strings.Contains(err.Error(), "FOREIGN KEY") || strings.Contains(err.Error(), "constraint failed") {
		t.Errorf("the driver's own text must not reach the caller; got %v", err)
	}
}

func TestEditOnAnUnknownWorkSaysSo(t *testing.T) {
	ctx, repo, _ := seedRepo(t)
	_, err := repo.Edit(ctx, ScopeWorkNotes, "no-such-work", ActionAdd, "", "민준은 3화부터 존댓말", 1000)
	if !errors.Is(err, ErrUnknownProject) {
		t.Fatalf("Edit error = %v, want ErrUnknownProject", err)
	}
	if strings.Contains(err.Error(), "FOREIGN KEY") || strings.Contains(err.Error(), "constraint failed") {
		t.Errorf("the driver's own text must not reach the caller; got %v", err)
	}
}

// codedError stands in for a driver error carrying a sqlite result code. A
// fake is the right tool here and the real database is not: only the
// foreign-key code is reachable through Save and Edit, so a genuine
// non-foreign-key write failure (a NOT NULL violation, a disk error) cannot
// be provoked from outside the package — and it is precisely those that must
// NOT be dressed up as a bad work id.
type codedError struct{ code int }

func (e codedError) Error() string { return "constraint failed (driver text)" }
func (e codedError) Code() int     { return e.code }

func TestOnlyTheForeignKeyCodeBecomesAnUnknownWork(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"foreign key", codedError{787}, true},
		{"a plain constraint violation", codedError{19}, false},
		{"NOT NULL", codedError{1299}, false},
		{"disk I/O", codedError{10}, false},
		{"an error with no sqlite code at all", errors.New("boom"), false},
		{"a wrapped foreign key", fmt.Errorf("insert: %w", codedError{787}), true},
	} {
		got := asWriteError(tc.err, "p1")
		if errors.Is(got, ErrUnknownProject) != tc.want {
			t.Errorf("%s: ErrUnknownProject = %v, want %v (got %v)",
				tc.name, !tc.want, tc.want, got)
		}
		if !tc.want && got != tc.err {
			t.Errorf("%s: a failure that is not the foreign key must pass through untouched; got %v",
				tc.name, got)
		}
	}
}
