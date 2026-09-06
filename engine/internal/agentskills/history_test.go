package agentskills

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

// seedHistory opens a real on-disk store under t.TempDir() and creates one
// work, because skill_snapshots.project_id has a foreign key and store.Open
// turns PRAGMA foreign_keys on. Mirrors agentmemory_test.go's seedRepo.
func seedHistory(t *testing.T) (context.Context, *History, *store.Store, string) {
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
	return ctx, NewHistory(st.DB()), st, p.ID
}

func writerSkill(name, body string) Skill {
	return Skill{
		Name:        name,
		Scope:       ScopeWriter,
		Description: "d",
		Author:      AuthorWriter,
		Enabled:     true,
		Body:        body,
	}
}

func workSkill(projectID, name, body string) Skill {
	return Skill{
		Name:        name,
		Scope:       ScopeWork,
		ProjectID:   projectID,
		Description: "d",
		Author:      AuthorWriter,
		Enabled:     true,
		Body:        body,
	}
}

func TestRecordThenListReturnsTheVersion(t *testing.T) {
	ctx, h, _, _ := seedHistory(t)
	s := writerSkill("outline-helper", "v1 body")
	if err := h.Record(ctx, s, ReasonCreated, 1000); err != nil {
		t.Fatalf("Record: %v", err)
	}
	versions, err := h.List(ctx, ScopeWriter, "", "outline-helper", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("len(versions) = %d, want 1", len(versions))
	}
	v := versions[0]
	if v.Reason != ReasonCreated {
		t.Errorf("Reason = %q, want %q", v.Reason, ReasonCreated)
	}
	if v.CreatedAt != 1000 {
		t.Errorf("CreatedAt = %d, want 1000", v.CreatedAt)
	}
	if v.Skill.Body != "v1 body" {
		t.Errorf("Body = %q", v.Skill.Body)
	}
	if v.Skill.Name != "outline-helper" || v.Skill.Scope != ScopeWriter {
		t.Errorf("Skill identity not preserved: %+v", v.Skill)
	}
	if v.ID == "" {
		t.Errorf("ID must not be empty")
	}
}

// TestVersionCarriesTheStateAfterTheWriteNotBefore pins the brief's decision:
// a version records the NEW state a write produced, not the state that
// preceded it. Restoring version N must give back what the skill looked like
// at version N, not what it looked like one write earlier.
func TestVersionCarriesTheStateAfterTheWriteNotBefore(t *testing.T) {
	ctx, h, _, _ := seedHistory(t)
	if err := h.Record(ctx, writerSkill("x", "before"), ReasonCreated, 1000); err != nil {
		t.Fatalf("Record #1: %v", err)
	}
	if err := h.Record(ctx, writerSkill("x", "after"), ReasonEdited, 2000); err != nil {
		t.Fatalf("Record #2: %v", err)
	}
	versions, err := h.List(ctx, ScopeWriter, "", "x", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("len(versions) = %d, want 2", len(versions))
	}
	// Newest first: the edit's version carries "after", the post-edit state,
	// not "before".
	if versions[0].Reason != ReasonEdited || versions[0].Skill.Body != "after" {
		t.Errorf("newest version = %+v, want reason=%q body=%q", versions[0], ReasonEdited, "after")
	}
	if versions[1].Reason != ReasonCreated || versions[1].Skill.Body != "before" {
		t.Errorf("oldest version = %+v, want reason=%q body=%q", versions[1], ReasonCreated, "before")
	}
}

func TestListIsNewestFirstAndHonoursTheLimit(t *testing.T) {
	ctx, h, _, _ := seedHistory(t)
	bodies := []string{"v1", "v2", "v3"}
	for i, body := range bodies {
		now := int64(1000 * (i + 1))
		if err := h.Record(ctx, writerSkill("x", body), ReasonEdited, now); err != nil {
			t.Fatalf("Record %d: %v", i, err)
		}
	}
	versions, err := h.List(ctx, ScopeWriter, "", "x", 2)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("len(versions) = %d, want 2 (the limit)", len(versions))
	}
	if versions[0].Skill.Body != "v3" || versions[1].Skill.Body != "v2" {
		t.Errorf("got bodies %q, %q; want newest-first v3, v2", versions[0].Skill.Body, versions[1].Skill.Body)
	}
}

// TestListTieBreaksOnInsertionOrderNotID is the regression test for the
// Critical review finding: List's ORDER BY used to tie-break on the uuid
// `id`, which is random per row, so two versions sharing a created_at came
// back in a random order instead of insertion order. The fix tie-breaks on
// SQLite's implicit rowid, which only ever increases with insertion — the
// same fix internal/companion/history.go shipped for issue #95.
//
// This records n=10 versions that all share one created_at. With a random
// tie-break, the odds of them coming back in exact insertion order by luck
// are about 1 in 10! (~3.6 million) — reverting the fix to `id DESC` makes
// this fail essentially every run, not just flakily. (Verified by mutation:
// reverting the ORDER BY to `id DESC` makes this test fail.)
func TestListTieBreaksOnInsertionOrderNotID(t *testing.T) {
	const n = 10
	const sameCreatedAt = int64(5000)
	ctx, h, _, _ := seedHistory(t)

	wantBodies := make([]string, n)
	for i := 0; i < n; i++ {
		body := fmt.Sprintf("v%d", i)
		wantBodies[i] = body
		if err := h.Record(ctx, writerSkill("x", body), ReasonEdited, sameCreatedAt); err != nil {
			t.Fatalf("Record %d: %v", i, err)
		}
	}

	versions, err := h.List(ctx, ScopeWriter, "", "x", n)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(versions) != n {
		t.Fatalf("len(versions) = %d, want %d", len(versions), n)
	}

	// All n rows share created_at, so ordering depends entirely on the
	// tie-break. Newest-first on insertion order means the LAST body
	// recorded (v9) must come back FIRST, v8 second, and so on.
	gotBodies := make([]string, n)
	for i, v := range versions {
		gotBodies[i] = v.Skill.Body
	}
	for i := 0; i < n; i++ {
		want := wantBodies[n-1-i]
		if gotBodies[i] != want {
			t.Fatalf("versions[%d].Body = %q, want %q (insertion order, newest first) — full order: %v",
				i, gotBodies[i], want, gotBodies)
		}
	}
}

// TestListDefaultsLimitWhenCallerSaysNothing pins the Minor review finding:
// limit <= 0 used to mean "unlimited" via SQLite's LIMIT -1. It now falls
// back to defaultHistoryLimit instead, for any non-positive value.
func TestListDefaultsLimitWhenCallerSaysNothing(t *testing.T) {
	ctx, h, _, _ := seedHistory(t)
	total := defaultHistoryLimit + 5
	for i := 0; i < total; i++ {
		body := fmt.Sprintf("v%d", i)
		if err := h.Record(ctx, writerSkill("x", body), ReasonEdited, int64(i+1)); err != nil {
			t.Fatalf("Record %d: %v", i, err)
		}
	}
	for _, limit := range []int{0, -1, -100} {
		versions, err := h.List(ctx, ScopeWriter, "", "x", limit)
		if err != nil {
			t.Fatalf("List(limit=%d): %v", limit, err)
		}
		if len(versions) != defaultHistoryLimit {
			t.Errorf("List(limit=%d): len(versions) = %d, want defaultHistoryLimit (%d)", limit, len(versions), defaultHistoryLimit)
		}
	}
}

// TestListClampsLimitToTheMaximum pins the ceiling half of the same fix:
// a caller-supplied limit above maxHistoryLimit is clamped to it, so a
// client-controlled value (Task 8's RPC handler) can never pull the whole
// table.
func TestListClampsLimitToTheMaximum(t *testing.T) {
	ctx, h, _, _ := seedHistory(t)
	total := maxHistoryLimit + 5
	for i := 0; i < total; i++ {
		body := fmt.Sprintf("v%d", i)
		if err := h.Record(ctx, writerSkill("x", body), ReasonEdited, int64(i+1)); err != nil {
			t.Fatalf("Record %d: %v", i, err)
		}
	}
	versions, err := h.List(ctx, ScopeWriter, "", "x", total*10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(versions) != maxHistoryLimit {
		t.Fatalf("len(versions) = %d, want maxHistoryLimit (%d)", len(versions), maxHistoryLimit)
	}
	wantNewest := fmt.Sprintf("v%d", total-1)
	if versions[0].Skill.Body != wantNewest {
		t.Errorf("newest version body = %q, want %q", versions[0].Skill.Body, wantNewest)
	}
}

// TestListKeysOnScopeProjectAndName pins that two skills sharing a name in
// different scopes, or in different works, are different histories — the
// query must filter on all three, not just name.
func TestListKeysOnScopeProjectAndName(t *testing.T) {
	ctx, h, st, projectID := seedHistory(t)
	p2, err := project.NewRepo(st).Create(ctx, 1, project.NewInput{
		Title: "작품2", Genres: []string{"fantasy"}, LengthTarget: "novel", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create second project: %v", err)
	}

	if err := h.Record(ctx, writerSkill("x", "writer-body"), ReasonCreated, 1000); err != nil {
		t.Fatalf("Record writer: %v", err)
	}
	if err := h.Record(ctx, workSkill(projectID, "x", "work1-body"), ReasonCreated, 1000); err != nil {
		t.Fatalf("Record work1: %v", err)
	}
	if err := h.Record(ctx, workSkill(p2.ID, "x", "work2-body"), ReasonCreated, 1000); err != nil {
		t.Fatalf("Record work2: %v", err)
	}

	writerVersions, err := h.List(ctx, ScopeWriter, "", "x", 10)
	if err != nil {
		t.Fatalf("List writer: %v", err)
	}
	if len(writerVersions) != 1 || writerVersions[0].Skill.Body != "writer-body" {
		t.Errorf("writer scope leaked another scope's rows: %+v", writerVersions)
	}

	work1Versions, err := h.List(ctx, ScopeWork, projectID, "x", 10)
	if err != nil {
		t.Fatalf("List work1: %v", err)
	}
	if len(work1Versions) != 1 || work1Versions[0].Skill.Body != "work1-body" {
		t.Errorf("work1 leaked another project's rows: %+v", work1Versions)
	}

	work2Versions, err := h.List(ctx, ScopeWork, p2.ID, "x", 10)
	if err != nil {
		t.Fatalf("List work2: %v", err)
	}
	if len(work2Versions) != 1 || work2Versions[0].Skill.Body != "work2-body" {
		t.Errorf("work2 leaked another project's rows: %+v", work2Versions)
	}
}

func TestDeletingAProjectCascadesItsWorkScopeRows(t *testing.T) {
	ctx, h, st, projectID := seedHistory(t)
	if err := h.Record(ctx, workSkill(projectID, "x", "body"), ReasonCreated, 1000); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := project.NewRepo(st).Delete(ctx, projectID); err != nil {
		t.Fatalf("delete project: %v", err)
	}
	var count int
	if err := st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM skill_snapshots WHERE project_id = ?`, projectID).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 — deleting the project must cascade its work-scope history", count)
	}
}

func TestGetOnAMissingIDIsAnError(t *testing.T) {
	ctx, h, _, _ := seedHistory(t)
	_, err := h.Get(ctx, "does-not-exist")
	if err == nil {
		t.Fatal("Get on a missing id: want an error, got nil")
	}
	if !errors.Is(err, ErrVersionNotFound) {
		t.Errorf("err = %v, want ErrVersionNotFound", err)
	}
}

func TestGetReturnsTheRecordedVersion(t *testing.T) {
	ctx, h, _, _ := seedHistory(t)
	if err := h.Record(ctx, writerSkill("x", "body"), ReasonCreated, 1000); err != nil {
		t.Fatalf("Record: %v", err)
	}
	versions, err := h.List(ctx, ScopeWriter, "", "x", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	got, err := h.Get(ctx, versions[0].ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Skill.Body != "body" || got.Reason != ReasonCreated || got.CreatedAt != 1000 {
		t.Errorf("Get = %+v, want it to match the recorded version", got)
	}
}

// TestRecordOnDeleteKeepsTheSkillsLastBody pins the ruling for a deletion
// record: body holds the skill's LAST body (what it looked like right before
// it was deleted), not an empty string. A version marked deleted must be a
// complete, restorable snapshot on its own — a writer browsing history should
// be able to restore straight from the row marked "deleted" without having to
// know to reach for the row before it.
func TestRecordOnDeleteKeepsTheSkillsLastBody(t *testing.T) {
	ctx, h, _, _ := seedHistory(t)
	if err := h.Record(ctx, writerSkill("x", "content"), ReasonCreated, 1000); err != nil {
		t.Fatalf("Record created: %v", err)
	}
	if err := h.Record(ctx, writerSkill("x", "content"), ReasonDeleted, 2000); err != nil {
		t.Fatalf("Record deleted: %v", err)
	}
	versions, err := h.List(ctx, ScopeWriter, "", "x", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if versions[0].Reason != ReasonDeleted || versions[0].Skill.Body != "content" {
		t.Errorf("deletion version = %+v, want reason=%q body=%q", versions[0], ReasonDeleted, "content")
	}
}

func TestRecordRejectsAnUnknownReason(t *testing.T) {
	ctx, h, _, _ := seedHistory(t)
	err := h.Record(ctx, writerSkill("x", "body"), "sideways", 1000)
	if err == nil {
		t.Fatal("Record with an unknown reason: want an error, got nil")
	}
	if !errors.Is(err, ErrInvalidReason) {
		t.Errorf("err = %v, want errors.Is(err, ErrInvalidReason)", err)
	}
	// And the row must never have landed: a rejected Record must not leave
	// a partial write behind.
	versions, listErr := h.List(ctx, ScopeWriter, "", "x", 10)
	if listErr != nil {
		t.Fatalf("List: %v", listErr)
	}
	if len(versions) != 0 {
		t.Errorf("len(versions) = %d, want 0 — a rejected Record must not write a row", len(versions))
	}
}
