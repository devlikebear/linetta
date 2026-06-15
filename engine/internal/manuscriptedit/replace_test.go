package manuscriptedit

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/snapshot"
	"github.com/devlikebear/linetta/engine/internal/store"
)

type replaceFixture struct {
	ctx   context.Context
	nodes *node.Repo
	snaps *snapshot.Repo
	svc   *Service
	pid   string
	first string
}

func newReplaceFixture(t *testing.T) replaceFixture {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	p, err := project.NewRepo(st).Create(ctx, 1_000, project.NewInput{
		Title: "T", Genres: []string{"판타지"}, LengthTarget: "series", DefaultPOV: "third_limited",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	nodes := node.NewRepo(st)
	snaps := snapshot.NewRepo(st)
	return replaceFixture{
		ctx:   ctx,
		nodes: nodes,
		snaps: snaps,
		svc:   NewService(nodes, snaps),
		pid:   p.ID,
		first: *p.LastOpenedNodeID,
	}
}

func TestPlanReplaceGroupsOccurrencesByScene(t *testing.T) {
	f := newReplaceFixture(t)
	if err := f.nodes.UpdateContent(f.ctx, f.first, replaceDoc("민호는 웃었다. 민호는 고개를 들었다."), 1_100); err != nil {
		t.Fatalf("UpdateContent first: %v", err)
	}
	second, err := f.nodes.CreateRoot(f.ctx, f.pid, node.KindLeaf, "씬 2", "복도", 1_200)
	if err != nil {
		t.Fatalf("CreateRoot: %v", err)
	}
	if err := f.nodes.UpdateContent(f.ctx, second.ID, replaceDoc("민호 형은 문 앞에 섰다."), 1_300); err != nil {
		t.Fatalf("UpdateContent second: %v", err)
	}

	plan, err := f.svc.PlanReplace(f.ctx, ReplacePlanRequest{
		ProjectID: f.pid, Query: "민호", Replacement: "민준",
	})
	if err != nil {
		t.Fatalf("PlanReplace: %v", err)
	}
	if len(plan.Candidates) != 2 {
		t.Fatalf("candidates = %d, want 2: %+v", len(plan.Candidates), plan.Candidates)
	}
	if plan.Candidates[0].Occurrences != 2 {
		t.Fatalf("first occurrences = %d, want 2", plan.Candidates[0].Occurrences)
	}
	if !strings.Contains(plan.Candidates[0].Before, "민호") || !strings.Contains(plan.Candidates[0].After, "민준") {
		t.Fatalf("candidate should include before/after preview: %+v", plan.Candidates[0])
	}
	if !plan.Candidates[0].Selected {
		t.Fatal("candidates should default to selected")
	}
}

func TestApplyReplaceAppliesSelectedScenesAndCreatesSnapshots(t *testing.T) {
	f := newReplaceFixture(t)
	if err := f.nodes.UpdateContent(f.ctx, f.first, replaceDoc("민호는 웃었다. 민호는 고개를 들었다."), 1_100); err != nil {
		t.Fatalf("UpdateContent first: %v", err)
	}
	second, err := f.nodes.CreateRoot(f.ctx, f.pid, node.KindLeaf, "씬 2", "복도", 1_200)
	if err != nil {
		t.Fatalf("CreateRoot: %v", err)
	}
	if err := f.nodes.UpdateContent(f.ctx, second.ID, replaceDoc("민호는 문 앞에 섰다."), 1_300); err != nil {
		t.Fatalf("UpdateContent second: %v", err)
	}
	plan, err := f.svc.PlanReplace(f.ctx, ReplacePlanRequest{
		ProjectID: f.pid, Query: "민호", Replacement: "민준",
	})
	if err != nil {
		t.Fatalf("PlanReplace: %v", err)
	}

	result, err := f.svc.ApplyReplace(f.ctx, plan, []string{plan.Candidates[0].ID}, 2_000)
	if err != nil {
		t.Fatalf("ApplyReplace: %v", err)
	}
	if result.Applied != 1 || result.Skipped != 1 || len(result.Failures) != 0 {
		t.Fatalf("result = %+v, want applied=1 skipped=1 no failures", result)
	}
	first, _ := f.nodes.Get(f.ctx, f.first)
	secondGot, _ := f.nodes.Get(f.ctx, second.ID)
	if first.ContentDoc == nil || !strings.Contains(*first.ContentDoc, "민준") || strings.Contains(*first.ContentDoc, "민호") {
		t.Fatalf("first content not replaced: %v", first.ContentDoc)
	}
	if secondGot.ContentDoc == nil || !strings.Contains(*secondGot.ContentDoc, "민호") {
		t.Fatalf("second content should be unchanged: %v", secondGot.ContentDoc)
	}
	snap, err := f.snaps.LatestForNode(f.ctx, f.first)
	if err != nil {
		t.Fatalf("LatestForNode: %v", err)
	}
	if snap.Reason != snapshot.ReasonManual || !strings.Contains(snap.ContentDoc, "민호") {
		t.Fatalf("snapshot = %+v, want manual snapshot of old content", snap)
	}
}

func TestApplyReplaceKeepsMentionAtoms(t *testing.T) {
	f := newReplaceFixture(t)
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"민호는 "},{"type":"mention","attrs":{"id":"e1","label":"민호"}},{"type":"text","text":"를 보았다."}]}]}`
	if err := f.nodes.UpdateContent(f.ctx, f.first, doc, 1_100); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}
	plan, err := f.svc.PlanReplace(f.ctx, ReplacePlanRequest{
		ProjectID: f.pid, Query: "민호", Replacement: "민준",
	})
	if err != nil {
		t.Fatalf("PlanReplace: %v", err)
	}
	if len(plan.Candidates) != 1 || plan.Candidates[0].Occurrences != 1 {
		t.Fatalf("mention label should not count as a text-node occurrence: %+v", plan.Candidates)
	}

	result, err := f.svc.ApplyReplace(f.ctx, plan, []string{plan.Candidates[0].ID}, 2_000)
	if err != nil {
		t.Fatalf("ApplyReplace: %v", err)
	}
	if result.Applied != 1 {
		t.Fatalf("applied = %d, want 1", result.Applied)
	}
	got, _ := f.nodes.Get(f.ctx, f.first)
	if got.ContentDoc == nil || strings.Contains(*got.ContentDoc, `"text":"민호`) || !strings.Contains(*got.ContentDoc, `"text":"민준`) {
		t.Fatalf("text node was not replaced: %s", valueOrEmpty(got.ContentDoc))
	}
	if strings.Contains(*got.ContentDoc, `"label":"민준"`) || !strings.Contains(*got.ContentDoc, `"label":"민호"`) {
		t.Fatalf("mention atom label should be preserved: %s", *got.ContentDoc)
	}
}

func TestApplyReplaceFailsOnVersionMismatch(t *testing.T) {
	f := newReplaceFixture(t)
	if err := f.nodes.UpdateContent(f.ctx, f.first, replaceDoc("민호는 문장을 보았다."), 1_100); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}
	plan, err := f.svc.PlanReplace(f.ctx, ReplacePlanRequest{
		ProjectID: f.pid, Query: "민호", Replacement: "민준",
	})
	if err != nil {
		t.Fatalf("PlanReplace: %v", err)
	}

	if err := f.nodes.UpdateContent(f.ctx, f.first, replaceDoc("민호는 이미 바뀐 문장을 보았다."), 1_500); err != nil {
		t.Fatalf("UpdateContent mismatch: %v", err)
	}
	result, err := f.svc.ApplyReplace(f.ctx, plan, []string{plan.Candidates[0].ID}, 2_000)
	if err != nil {
		t.Fatalf("ApplyReplace: %v", err)
	}
	if result.Applied != 0 || len(result.Failures) != 1 || result.Failures[0].Reason != FailureVersionMismatch {
		t.Fatalf("result = %+v, want version mismatch failure", result)
	}
}

func replaceDoc(text string) string {
	return `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + text + `"}]}]}`
}

func valueOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
