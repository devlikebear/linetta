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

// raceNodeRepo wraps a real *node.Repo and, on the first UpdateContentIfVersion
// call for a watched node, injects a concurrent save (bumping content_version)
// before delegating to the real conditional write. This simulates a save
// landing in the window between ApplyReplace's version check and its write
// (#107), which a plain UpdateContent + prior-check would silently clobber.
type raceNodeRepo struct {
	*node.Repo
	watchNodeID   string
	concurrentDoc string
	concurrentNow int64
	injected      bool
}

func (r *raceNodeRepo) UpdateContentIfVersion(ctx context.Context, id string, doc string, expectedVersion int, now int64) error {
	if !r.injected && id == r.watchNodeID {
		r.injected = true
		if err := r.Repo.UpdateContent(ctx, id, r.concurrentDoc, r.concurrentNow); err != nil {
			return err
		}
	}
	return r.Repo.UpdateContentIfVersion(ctx, id, doc, expectedVersion, now)
}

func TestApplyReplaceFailsOnVersionMismatchFromConcurrentWriteRace(t *testing.T) {
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

	concurrentDoc := replaceDoc("민호는 동시에 저장된 문장을 보았다.")
	racy := &raceNodeRepo{
		Repo:          f.nodes,
		watchNodeID:   f.first,
		concurrentDoc: concurrentDoc,
		concurrentNow: 1_800,
	}
	svc := NewService(racy, f.snaps)

	result, err := svc.ApplyReplace(f.ctx, plan, []string{plan.Candidates[0].ID}, 2_000)
	if err != nil {
		t.Fatalf("ApplyReplace: %v", err)
	}
	if result.Applied != 0 || len(result.Failures) != 1 || result.Failures[0].Reason != FailureVersionMismatch {
		t.Fatalf("result = %+v, want version mismatch failure", result)
	}
	if result.Failures[0].NodeID != f.first {
		t.Fatalf("failure node = %s, want %s", result.Failures[0].NodeID, f.first)
	}

	got, err := f.nodes.Get(f.ctx, f.first)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ContentDoc == nil || !strings.Contains(*got.ContentDoc, "동시에 저장된") {
		t.Fatalf("concurrent save should survive the clobbered write: %v", valueOrEmpty(got.ContentDoc))
	}
	if strings.Contains(*got.ContentDoc, "민준") {
		t.Fatalf("replace must not have applied over the concurrent save: %v", *got.ContentDoc)
	}
}

func TestApplyReplaceHappyPathStillAppliesAndBumpsVersion(t *testing.T) {
	f := newReplaceFixture(t)
	if err := f.nodes.UpdateContent(f.ctx, f.first, replaceDoc("민호는 문장을 보았다."), 1_100); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}
	before, err := f.nodes.Get(f.ctx, f.first)
	if err != nil {
		t.Fatalf("Get before: %v", err)
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
	if result.Applied != 1 || len(result.Failures) != 0 {
		t.Fatalf("result = %+v, want applied=1 no failures", result)
	}

	after, err := f.nodes.Get(f.ctx, f.first)
	if err != nil {
		t.Fatalf("Get after: %v", err)
	}
	if after.ContentDoc == nil || !strings.Contains(*after.ContentDoc, "민준") || strings.Contains(*after.ContentDoc, "민호") {
		t.Fatalf("content not replaced: %v", valueOrEmpty(after.ContentDoc))
	}
	if after.ContentVersion != before.ContentVersion+1 {
		t.Fatalf("content_version = %d, want %d", after.ContentVersion, before.ContentVersion+1)
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

// apply runs one replace through the service and returns the resulting doc.
func (f replaceFixture) apply(t *testing.T, doc, query, replacement string) string {
	t.Helper()
	if err := f.nodes.UpdateContent(f.ctx, f.first, doc, 1_100); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}
	plan, err := f.svc.PlanReplace(f.ctx, ReplacePlanRequest{ProjectID: f.pid, Query: query, Replacement: replacement, MatchCase: true})
	if err != nil {
		t.Fatalf("PlanReplace: %v", err)
	}
	if len(plan.Candidates) != 1 {
		t.Fatalf("candidates = %d, want 1", len(plan.Candidates))
	}
	if _, err := f.svc.ApplyReplace(f.ctx, plan, []string{plan.Candidates[0].ID}, 2_000); err != nil {
		t.Fatalf("ApplyReplace: %v", err)
	}
	got, err := f.nodes.Get(f.ctx, f.first)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	return *got.ContentDoc
}

const seojinDoc = `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"그때 "},{"type":"mention","attrs":{"id":"e1","label":"서진"}},{"type":"text","text":"의 후드티가 보였다."}]}]}`

func TestReplaceCrossesALinkedNameAndKeepsTheLink(t *testing.T) {
	for _, query := range []string{"서진의 후드티", "@서진의 후드티"} {
		f := newReplaceFixture(t)
		got := f.apply(t, seojinDoc, query, "서진의 낡은 후드티")
		for _, want := range []string{`"text":"그때 "`, `"label":"서진"`, `"text":"의 낡은 후드티가 보였다."`} {
			if !strings.Contains(got, want) {
				t.Errorf("query %q: missing %s in %s", query, want, got)
			}
		}
	}
}

func TestReplaceLeavesAQueryThatCutsALinkedNameAlone(t *testing.T) {
	f := newReplaceFixture(t)
	if err := f.nodes.UpdateContent(f.ctx, f.first, seojinDoc, 1_100); err != nil {
		t.Fatal(err)
	}
	plan, err := f.svc.PlanReplace(f.ctx, ReplacePlanRequest{ProjectID: f.pid, Query: "진의", Replacement: "수의", MatchCase: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Candidates) != 0 {
		t.Fatalf("a match inside a linked name must not count: %+v", plan.Candidates)
	}
}

func TestReplaceAcrossParagraphsMergesThem(t *testing.T) {
	f := newReplaceFixture(t)
	doc := `{"type":"doc","content":[` +
		`{"type":"paragraph","content":[{"type":"text","text":"하나. 끊어 버릴 것만 같았다."}]},` +
		`{"type":"paragraph","content":[{"type":"text","text":"\"근데 왜요?\" 둘."}]}]}`
	got := f.apply(t, doc, "끊어 버릴 것만 같았다.\n\n\"근데", "끊어질 듯했다. \"근데")
	if !strings.Contains(got, `하나. 끊어질 듯했다. \"근데 왜요?\" 둘.`) {
		t.Fatalf("paragraphs not merged: %s", got)
	}
	if strings.Count(got, `"paragraph"`) != 1 {
		t.Fatalf("want one paragraph: %s", got)
	}
}

func TestReplaceNewlineStartsAParagraph(t *testing.T) {
	f := newReplaceFixture(t)
	got := f.apply(t, replaceDoc("앞. 가운데. 뒤."), "가운데.", "첫째.\n\n둘째.")
	if strings.Count(got, `"paragraph"`) != 2 {
		t.Fatalf("want two paragraphs: %s", got)
	}
	for _, want := range []string{`"text":"앞. 첫째."`, `"text":"둘째. 뒤."`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
}

func TestReplaceDoesNotReachAcrossAnIllustration(t *testing.T) {
	f := newReplaceFixture(t)
	doc := `{"type":"doc","content":[` +
		`{"type":"paragraph","content":[{"type":"text","text":"앞"}]},` +
		`{"type":"illustration","attrs":{"id":"i1"}},` +
		`{"type":"paragraph","content":[{"type":"text","text":"뒤"}]}]}`
	if err := f.nodes.UpdateContent(f.ctx, f.first, doc, 1_100); err != nil {
		t.Fatal(err)
	}
	plan, err := f.svc.PlanReplace(f.ctx, ReplacePlanRequest{ProjectID: f.pid, Query: "앞\n뒤", Replacement: "합침", MatchCase: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Candidates) != 0 {
		t.Fatalf("a match must not swallow a picture: %+v", plan.Candidates)
	}
}

func TestReplaceKeepsTrailingSpaceExact(t *testing.T) {
	f := newReplaceFixture(t)
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"그런데도 "},{"type":"mention","attrs":{"id":"e1","label":"리뷰봇"}}]}]}`
	got := f.apply(t, doc, "그런데도 ", "그럼에도 ")
	if !strings.Contains(got, `"text":"그럼에도 "`) {
		t.Fatalf("space doubled or lost: %s", got)
	}
}

func TestReplaceKeepsFormattingOfTheTextItTakesOver(t *testing.T) {
	f := newReplaceFixture(t)
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"평범 "},{"type":"text","text":"강조된 말","marks":[{"type":"bold"}]}]}]}`
	got := f.apply(t, doc, "강조된", "굵은")
	if !strings.Contains(got, `"marks":[{"type":"bold"}],"text":"굵은 말"`) && !strings.Contains(got, `"text":"굵은 말"`) {
		t.Fatalf("formatting split the text: %s", got)
	}
	if strings.Count(got, `"marks"`) != 1 {
		t.Fatalf("want one bold run: %s", got)
	}
}

func TestReplaceIgnoresCaseWithoutChangingOffsets(t *testing.T) {
	f := newReplaceFixture(t)
	if err := f.nodes.UpdateContent(f.ctx, f.first, replaceDoc("ẞ Kelvin K 끝"), 1_100); err != nil {
		t.Fatal(err)
	}
	plan, err := f.svc.PlanReplace(f.ctx, ReplacePlanRequest{ProjectID: f.pid, Query: "kelvin", Replacement: "켈빈"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Candidates) != 1 || !strings.Contains(plan.Candidates[0].After, "ẞ 켈빈 K 끝") {
		t.Fatalf("candidates = %+v", plan.Candidates)
	}
}
