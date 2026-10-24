package continuity

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

type fixture struct {
	s    *store.Store
	nr   *node.Repo
	repo *Repo
	p    project.Project
	node string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	pr := project.NewRepo(s)
	p, err := pr.Create(ctx, 1000, project.NewInput{
		Title: "T", Genres: []string{}, LengthTarget: "series", DefaultPOV: "first",
		OutlinePreset: project.OutlinePresetWebNovel,
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	nr := node.NewRepo(s)
	return &fixture{s: s, nr: nr, repo: NewRepo(s), p: p, node: *p.LastOpenedNodeID}
}

// reportInput builds a ReportInput at the scene's CURRENT content_version —
// what a real caller gets back from linetta_read_scene right before
// reporting — so a test that edits the scene between two Records sees the
// same staleness behavior the tool layer does.
func (f *fixture) reportInput(obs ...ObservationInput) ReportInput {
	ctx := context.Background()
	n, err := f.nr.Get(ctx, f.node)
	if err != nil {
		panic(err)
	}
	return ReportInput{ProjectID: f.p.ID, NodeID: f.node, SceneVersion: n.ContentVersion, Observations: obs}
}

func cleanObservation() ObservationInput {
	return ObservationInput{
		Category: CategoryOther, Kind: KindQuestion, Severity: SeverityLow,
		Summary: "확인이 필요하다", ParagraphFrom: 1, ParagraphTo: 1, Quote: "본문",
	}
}

func TestRepo_recordThenList(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)

	check, err := f.repo.Record(ctx, 2000, f.reportInput(cleanObservation()), SourceAgent)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if check.ID == "" {
		t.Fatal("Record returned an empty check id")
	}

	checks, err := f.repo.List(ctx, ListFilter{ProjectID: f.p.ID})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(checks) != 1 {
		t.Fatalf("List returned %d checks, want 1", len(checks))
	}
	if len(checks[0].Observations) != 1 {
		t.Fatalf("List returned %d observations, want 1", len(checks[0].Observations))
	}
	if checks[0].Observations[0].Status != StatusOpen {
		t.Errorf("new observation status = %q, want %q", checks[0].Observations[0].Status, StatusOpen)
	}
}

func TestRepo_emptyObservationsIsACleanCheck(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)

	if _, err := f.repo.Record(ctx, 2000, f.reportInput(), SourceAgent); err != nil {
		t.Fatalf("Record with no observations: %v", err)
	}
	checks, err := f.repo.List(ctx, ListFilter{ProjectID: f.p.ID})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(checks) != 1 || len(checks[0].Observations) != 0 {
		t.Fatalf("List = %+v, want one check with zero observations", checks)
	}
}

// A later check of the same scene replaces the earlier one's still-OPEN
// observations, but leaves one the writer already dismissed untouched — the
// writer's own judgement outlives a re-check.
func TestRepo_replacesOpenButNotDismissed(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)

	first, err := f.repo.Record(ctx, 2000, f.reportInput(cleanObservation(), cleanObservation()), SourceAgent)
	if err != nil {
		t.Fatalf("first Record: %v", err)
	}
	firstChecks, err := f.repo.List(ctx, ListFilter{ProjectID: f.p.ID})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	obsToKeep := firstChecks[0].Observations[0].ID
	obsToReplace := firstChecks[0].Observations[1].ID

	if _, err := f.repo.SetStatus(ctx, 2500, obsToKeep, StatusDismissed); err != nil {
		t.Fatalf("SetStatus dismiss: %v", err)
	}

	if _, err := f.repo.Record(ctx, 3000, f.reportInput(cleanObservation()), SourceAgent); err != nil {
		t.Fatalf("second Record: %v", err)
	}

	all, err := f.repo.List(ctx, ListFilter{ProjectID: f.p.ID})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	statuses := map[string]string{}
	for _, c := range all {
		if c.ID == first.ID {
			continue
		}
		for _, o := range c.Observations {
			statuses[o.ID] = o.Status
		}
	}
	// Walk every check's observations by id, regardless of which check.
	byID := map[string]Observation{}
	for _, c := range all {
		for _, o := range c.Observations {
			byID[o.ID] = o
		}
	}
	if got := byID[obsToKeep].Status; got != StatusDismissed {
		t.Errorf("dismissed observation status = %q, want %q (must survive a later check)", got, StatusDismissed)
	}
	if got := byID[obsToReplace].Status; got != StatusReplaced {
		t.Errorf("stale open observation status = %q, want %q", got, StatusReplaced)
	}
}

func TestRepo_staleWhenSceneContentVersionMoves(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)

	if _, err := f.repo.Record(ctx, 2000, f.reportInput(cleanObservation()), SourceAgent); err != nil {
		t.Fatalf("Record: %v", err)
	}
	checks, err := f.repo.List(ctx, ListFilter{ProjectID: f.p.ID})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if checks[0].Stale {
		t.Fatal("check is stale before the scene changed")
	}

	if err := f.nr.UpdateContent(ctx, f.node,
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"바뀐 본문"}]}]}`, 2500); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}

	checks, err = f.repo.List(ctx, ListFilter{ProjectID: f.p.ID})
	if err != nil {
		t.Fatalf("List after edit: %v", err)
	}
	if !checks[0].Stale {
		t.Error("check is not stale after the scene's content_version changed")
	}
}

func TestRepo_setStatus_cannotTouchReplaced(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)

	if _, err := f.repo.Record(ctx, 2000, f.reportInput(cleanObservation()), SourceAgent); err != nil {
		t.Fatalf("first Record: %v", err)
	}
	checks, err := f.repo.List(ctx, ListFilter{ProjectID: f.p.ID})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	obsID := checks[0].Observations[0].ID

	if _, err := f.repo.Record(ctx, 3000, f.reportInput(), SourceAgent); err != nil {
		t.Fatalf("second Record: %v", err)
	}

	if _, err := f.repo.SetStatus(ctx, 3500, obsID, StatusResolved); err == nil {
		t.Error("SetStatus succeeded on a replaced observation, want an error")
	}
	if _, err := f.repo.SetStatus(ctx, 3500, "whatever", StatusReplaced); err == nil {
		t.Error("SetStatus to replaced directly should always be refused")
	}
}

func TestRepo_openCounts(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)

	if _, err := f.repo.Record(ctx, 2000, f.reportInput(cleanObservation(), cleanObservation()), SourceAgent); err != nil {
		t.Fatalf("Record: %v", err)
	}
	counts, err := f.repo.OpenCounts(ctx, f.p.ID)
	if err != nil {
		t.Fatalf("OpenCounts: %v", err)
	}
	if counts[f.node] != 2 {
		t.Errorf("OpenCounts[node] = %d, want 2", counts[f.node])
	}

	checks, err := f.repo.List(ctx, ListFilter{ProjectID: f.p.ID})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if _, err := f.repo.SetStatus(ctx, 2500, checks[0].Observations[0].ID, StatusResolved); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	counts, err = f.repo.OpenCounts(ctx, f.p.ID)
	if err != nil {
		t.Fatalf("OpenCounts after resolve: %v", err)
	}
	if counts[f.node] != 1 {
		t.Errorf("OpenCounts[node] after resolving one = %d, want 1", counts[f.node])
	}
}

// Deleting a node cascades to its continuity data (FK ON DELETE CASCADE),
// the same contract the writer-direction tables hold.
func TestRepo_deletedWithTheNode(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)

	if _, err := f.repo.Record(ctx, 2000, f.reportInput(cleanObservation()), SourceAgent); err != nil {
		t.Fatalf("Record: %v", err)
	}

	// A project needs at least one leaf, so add a sibling before deleting
	// the one holding the check.
	if _, err := f.nr.CreateSibling(ctx, f.node, node.KindLeaf, "다른 씬", "", 2200); err != nil {
		t.Fatalf("CreateSibling: %v", err)
	}
	if err := f.nr.Delete(ctx, f.node, 2500); err != nil {
		t.Fatalf("Delete node: %v", err)
	}

	var checkCount, obsCount int
	if err := f.s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM continuity_checks WHERE node_id = ?`, f.node).Scan(&checkCount); err != nil {
		t.Fatalf("count checks: %v", err)
	}
	if err := f.s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM continuity_observations WHERE node_id = ?`, f.node).Scan(&obsCount); err != nil {
		t.Fatalf("count observations: %v", err)
	}
	if checkCount != 0 || obsCount != 0 {
		t.Errorf("continuity rows survived node deletion: checks=%d observations=%d", checkCount, obsCount)
	}
}
