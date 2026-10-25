package forecast

import (
	"context"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
)

func twoBranches() []BranchInput {
	return []BranchInput{
		{Title: "갈래 1", Premise: "주인공이 맞선다.", Beats: []BranchBeat{{EpisodeOffset: 1, Summary: "첫 비트"}}},
		{Title: "갈래 2", Premise: "주인공이 물러선다.", Beats: []BranchBeat{{EpisodeOffset: 1, Summary: "첫 비트"}}},
	}
}

func TestSave_storesAndAssignsBranchIDs(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	svc := New(env.st, env.deps)
	env.writeScene(t, env.sceneID, "쓰여진 문장.", 1)

	f, problems, err := svc.Save(ctx, 10_000, SaveInput{
		ProjectID: env.project.ID, Divergence: "다음 회차 갈래?", Horizon: 3, Branches: twoBranches(),
	}, SourceAgent)
	if err != nil {
		t.Fatalf("Save error: %v", err)
	}
	if len(problems) != 0 {
		t.Fatalf("problems = %+v, want none", problems)
	}
	if f.ID == "" {
		t.Fatal("forecast id not assigned")
	}
	if len(f.Branches) != 2 || f.Branches[0].ID != "b1" || f.Branches[1].ID != "b2" {
		t.Fatalf("branch ids = %+v, want b1,b2", f.Branches)
	}
	if f.BaseNodeID != env.sceneID {
		t.Errorf("base_node_id = %q, want the opening scene %q", f.BaseNodeID, env.sceneID)
	}
	if f.Fingerprint == "" {
		t.Error("fingerprint not set")
	}
}

func TestSave_rejectsTooFewOrTooManyBranches(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	svc := New(env.st, env.deps)

	_, problems, err := svc.Save(ctx, 10_000, SaveInput{
		ProjectID: env.project.ID, Divergence: "질문", Horizon: 3,
		Branches: []BranchInput{twoBranches()[0]}, // only 1
	}, SourceAgent)
	if err != nil {
		t.Fatalf("Save error: %v", err)
	}
	if len(problems) == 0 {
		t.Fatal("want a problem for 1 branch")
	}

	six := make([]BranchInput, 0, 6)
	for i := 0; i < 6; i++ {
		six = append(six, twoBranches()[0])
	}
	_, problems, err = svc.Save(ctx, 10_000, SaveInput{
		ProjectID: env.project.ID, Divergence: "질문", Horizon: 3, Branches: six,
	}, SourceAgent)
	if err != nil {
		t.Fatalf("Save error: %v", err)
	}
	if len(problems) == 0 {
		t.Fatal("want a problem for 6 branches")
	}

	rows, err := svc.List(ctx, env.project.ID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("a rejected save must store nothing, got %d rows", len(rows))
	}
}

func TestSave_rejectsOutOfRangeHorizonAndEpisodeOffset(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	svc := New(env.st, env.deps)

	_, problems, err := svc.Save(ctx, 10_000, SaveInput{
		ProjectID: env.project.ID, Divergence: "질문", Horizon: 11, Branches: twoBranches(),
	}, SourceAgent)
	if err != nil {
		t.Fatalf("Save error: %v", err)
	}
	if len(problems) == 0 {
		t.Fatal("want a problem for horizon=11")
	}

	branches := twoBranches()
	branches[0].Beats = []BranchBeat{{EpisodeOffset: 5, Summary: "범위 밖"}}
	_, problems, err = svc.Save(ctx, 10_000, SaveInput{
		ProjectID: env.project.ID, Divergence: "질문", Horizon: 3, Branches: branches,
	}, SourceAgent)
	if err != nil {
		t.Fatalf("Save error: %v", err)
	}
	if len(problems) == 0 {
		t.Fatal("want a problem for episode_offset=5 > horizon=3")
	}
}

func TestSave_validatesEntityFactNodeRefs(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	svc := New(env.st, env.deps)

	branches := twoBranches()
	branches[0].Decisions = []CharDecision{{EntityID: "no-such-entity", Decision: "결심한다"}}
	_, problems, err := svc.Save(ctx, 10_000, SaveInput{
		ProjectID: env.project.ID, Divergence: "질문", Horizon: 3, Branches: branches,
	}, SourceAgent)
	if err != nil {
		t.Fatalf("Save error: %v", err)
	}
	if len(problems) == 0 {
		t.Fatal("want a problem for an unknown entity_id")
	}

	e, err := env.entities.Create(ctx, 1_000, entity.NewInput{ProjectID: env.project.ID, Kind: "character", Name: "하린"})
	if err != nil {
		t.Fatalf("create entity: %v", err)
	}
	branches[0].Decisions = []CharDecision{{EntityID: e.ID, Decision: "결심한다"}}
	_, problems, err = svc.Save(ctx, 10_000, SaveInput{
		ProjectID: env.project.ID, Divergence: "질문", Horizon: 3, Branches: branches,
	}, SourceAgent)
	if err != nil {
		t.Fatalf("Save error: %v", err)
	}
	if len(problems) != 0 {
		t.Fatalf("problems = %+v, want none once entity_id is real", problems)
	}
}

// A risk's node_id is the scene the writer clicks through to open in the
// editor (ForecastSheet's risk-ref button → Workspace.navigateToNode), which
// treats whatever id it is given as a leaf and opens it with no further
// check. A container id (an episode or part) would open as an empty
// document and risk autosave writing prose into it — the same leaf-only
// rule Phase 2 holds continuity's own scene refs to. This covers both
// directions: the existing leaf scene passes, a container is rejected.
func TestSave_validatesRiskNodeRefIsALeafScene(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	svc := New(env.st, env.deps)

	container, err := env.nodes.CreateChild(ctx, env.sceneID, "container", "1부", "1부", 1_000)
	if err != nil {
		t.Fatalf("create container: %v", err)
	}

	branches := twoBranches()
	branches[0].Risks = []Risk{{Kind: "continuity", Description: "컨테이너를 가리킴", NodeID: container.ID}}
	_, problems, err := svc.Save(ctx, 10_000, SaveInput{
		ProjectID: env.project.ID, Divergence: "질문", Horizon: 3, Branches: branches,
	}, SourceAgent)
	if err != nil {
		t.Fatalf("Save error: %v", err)
	}
	if len(problems) == 0 {
		t.Fatal("want a problem for a risk node_id that names a container, not a leaf scene")
	}
	found := false
	for _, p := range problems {
		if strings.Contains(p.Message, "is not a leaf scene in this work") {
			found = true
		}
	}
	if !found {
		t.Fatalf("problems = %+v, want one naming node_id as not a leaf scene", problems)
	}

	branches[0].Risks = []Risk{{Kind: "continuity", Description: "씬을 가리킴", NodeID: env.sceneID}}
	_, problems, err = svc.Save(ctx, 10_000, SaveInput{
		ProjectID: env.project.ID, Divergence: "질문", Horizon: 3, Branches: branches,
	}, SourceAgent)
	if err != nil {
		t.Fatalf("Save error: %v", err)
	}
	if len(problems) != 0 {
		t.Fatalf("problems = %+v, want none once node_id names the leaf scene", problems)
	}
}

func TestList_reportsStaleAndClearsOnceCanonReverts(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	svc := New(env.st, env.deps)
	env.writeScene(t, env.sceneID, "원래 문장.", 1)

	f, _, err := svc.Save(ctx, 10_000, SaveInput{
		ProjectID: env.project.ID, Divergence: "질문", Horizon: 3, Branches: twoBranches(),
	}, SourceAgent)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	rows, err := svc.List(ctx, env.project.ID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 || rows[0].Stale {
		t.Fatalf("fresh forecast should not be stale: %+v", rows)
	}

	env.writeScene(t, env.sceneID, "바뀐 문장.", 2)
	rows, err = svc.List(ctx, env.project.ID)
	if err != nil {
		t.Fatalf("List after edit: %v", err)
	}
	if !rows[0].Stale {
		t.Fatal("forecast should be stale after the scene changed")
	}
	foundEpisodes := false
	for _, r := range rows[0].Reasons {
		if r.Group == "episodes" {
			foundEpisodes = true
		}
	}
	if !foundEpisodes {
		t.Errorf("reasons = %+v, want an episodes entry", rows[0].Reasons)
	}

	env.writeScene(t, env.sceneID, "원래 문장.", 3)
	rows, err = svc.List(ctx, env.project.ID)
	if err != nil {
		t.Fatalf("List after revert: %v", err)
	}
	if rows[0].Stale {
		t.Error("forecast should be fresh again once canon reverted")
	}
	_ = f
}

func TestSelect_oneSelectionPerProject(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	svc := New(env.st, env.deps)

	f1, _, err := svc.Save(ctx, 10_000, SaveInput{ProjectID: env.project.ID, Divergence: "q1", Horizon: 3, Branches: twoBranches()}, SourceAgent)
	if err != nil {
		t.Fatalf("Save f1: %v", err)
	}
	f2, _, err := svc.Save(ctx, 10_001, SaveInput{ProjectID: env.project.ID, Divergence: "q2", Horizon: 3, Branches: twoBranches()}, SourceAgent)
	if err != nil {
		t.Fatalf("Save f2: %v", err)
	}

	if _, err := svc.Select(ctx, 10_002, env.project.ID, f1.ID, "b1"); err != nil {
		t.Fatalf("Select f1: %v", err)
	}
	if _, found, err := svc.Selected(ctx, env.project.ID); err != nil || !found {
		t.Fatalf("Selected after f1 select: found=%v err=%v", found, err)
	}

	if _, err := svc.Select(ctx, 10_003, env.project.ID, f2.ID, "b2"); err != nil {
		t.Fatalf("Select f2: %v", err)
	}
	sel, found, err := svc.Selected(ctx, env.project.ID)
	if err != nil || !found {
		t.Fatalf("Selected after f2 select: found=%v err=%v", found, err)
	}
	if sel.ID != f2.ID || sel.SelectedBranch != "b2" {
		t.Errorf("selected = %+v, want f2/b2", sel)
	}

	rows, err := svc.List(ctx, env.project.ID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	selectedCount := 0
	for _, r := range rows {
		if r.SelectedBranch != "" {
			selectedCount++
		}
	}
	if selectedCount != 1 {
		t.Errorf("selectedCount = %d, want 1 (selecting f2 must clear f1's selection)", selectedCount)
	}
}

func TestSelect_rejectsUnknownBranchID(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	svc := New(env.st, env.deps)

	f, _, err := svc.Save(ctx, 10_000, SaveInput{ProjectID: env.project.ID, Divergence: "q", Horizon: 3, Branches: twoBranches()}, SourceAgent)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := svc.Select(ctx, 10_001, env.project.ID, f.ID, "b99"); err == nil {
		t.Fatal("want an error selecting a branch id that does not exist")
	}
}

func TestClearSelection(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	svc := New(env.st, env.deps)

	f, _, err := svc.Save(ctx, 10_000, SaveInput{ProjectID: env.project.ID, Divergence: "q", Horizon: 3, Branches: twoBranches()}, SourceAgent)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := svc.Select(ctx, 10_001, env.project.ID, f.ID, "b1"); err != nil {
		t.Fatalf("Select: %v", err)
	}
	if err := svc.ClearSelection(ctx, env.project.ID); err != nil {
		t.Fatalf("ClearSelection: %v", err)
	}
	if _, found, err := svc.Selected(ctx, env.project.ID); err != nil || found {
		t.Fatalf("Selected after clear: found=%v err=%v, want none", found, err)
	}

	// Clearing with nothing selected is not an error.
	if err := svc.ClearSelection(ctx, env.project.ID); err != nil {
		t.Fatalf("ClearSelection with nothing selected: %v", err)
	}
}

func TestDelete(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(t)
	svc := New(env.st, env.deps)

	f, _, err := svc.Save(ctx, 10_000, SaveInput{ProjectID: env.project.ID, Divergence: "q", Horizon: 3, Branches: twoBranches()}, SourceAgent)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := svc.Delete(ctx, env.project.ID, f.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	rows, err := svc.List(ctx, env.project.ID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %+v, want none after delete", rows)
	}

	// Deleting an id that no longer exists is not an error.
	if err := svc.Delete(ctx, env.project.ID, f.ID); err != nil {
		t.Fatalf("Delete again: %v", err)
	}
}
