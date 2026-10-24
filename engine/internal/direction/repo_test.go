package direction

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func textDoc(text string) string {
	return `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + text + `"}]}]}`
}

// testFixture wires a fresh store with a webnovel-preset project whose root
// holds four single-scene episodes, 1화..4화, none written yet.
type testFixture struct {
	s        *store.Store
	nr       *node.Repo
	pr       *project.Repo
	repo     *Repo
	project  project.Project
	episodes []string // node ids, ordinal order: 1화..4화
}

func newFixture(t *testing.T) *testFixture {
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
	first := *p.LastOpenedNodeID
	if err := nr.Rename(ctx, first, "1화", "", 1000); err != nil {
		t.Fatalf("rename first: %v", err)
	}
	ids := []string{first}
	ref := first
	for _, label := range []string{"2화", "3화", "4화"} {
		n, err := nr.CreateSibling(ctx, ref, node.KindLeaf, label, "", 1000)
		if err != nil {
			t.Fatalf("create sibling %s: %v", label, err)
		}
		ids = append(ids, n.ID)
		ref = n.ID
	}

	return &testFixture{
		s: s, nr: nr, pr: pr,
		repo:     NewRepo(s, nr, pr),
		project:  p,
		episodes: ids,
	}
}

// write gives episode i (0-based) nonzero word_count.
func (f *testFixture) write(t *testing.T, i int) {
	t.Helper()
	if err := f.nr.UpdateContent(context.Background(), f.episodes[i], textDoc("글이 있다"), 2000); err != nil {
		t.Fatalf("write episode %d: %v", i, err)
	}
}

func TestGetFocus_notSetIsAValidState(t *testing.T) {
	f := newFixture(t)
	state, err := f.repo.GetFocus(context.Background(), f.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Present {
		t.Fatalf("got %+v, want Present=false", state)
	}
}

func TestGetFocus_unknownProject(t *testing.T) {
	f := newFixture(t)
	if _, err := f.repo.GetFocus(context.Background(), "missing"); err != ErrProjectNotFound {
		t.Fatalf("err = %v, want ErrProjectNotFound", err)
	}
}

func TestSetFocus_overLimitIsRejectedNotTruncated(t *testing.T) {
	f := newFixture(t)
	long := strings.Repeat("가", MaxFocusRunes+1)
	if _, err := f.repo.SetFocus(context.Background(), 1000, f.project.ID, long); err != ErrFocusTooLong {
		t.Fatalf("err = %v, want ErrFocusTooLong", err)
	}
	// Rejected, so nothing was stored.
	state, err := f.repo.GetFocus(context.Background(), f.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Present {
		t.Fatalf("a rejected focus must not be stored, got %+v", state)
	}
}

func TestSetFocus_emptyBodyClearsTheRow(t *testing.T) {
	f := newFixture(t)
	f.write(t, 0)
	if _, err := f.repo.SetFocus(context.Background(), 1000, f.project.ID, "집중할 것"); err != nil {
		t.Fatal(err)
	}
	state, err := f.repo.SetFocus(context.Background(), 2000, f.project.ID, "   ")
	if err != nil {
		t.Fatal(err)
	}
	if state.Present {
		t.Fatalf("empty body must clear the focus, got %+v", state)
	}
	state, err = f.repo.GetFocus(context.Background(), f.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Present {
		t.Fatalf("GetFocus after clearing must report Present=false, got %+v", state)
	}
}

func TestSetFocus_anchorsToTheLastWrittenEpisode(t *testing.T) {
	f := newFixture(t)
	f.write(t, 0) // 1화 written
	state, err := f.repo.SetFocus(context.Background(), 1000, f.project.ID, "둘의 오해를 풀지 않는다")
	if err != nil {
		t.Fatal(err)
	}
	if state.AnchorNodeID != f.episodes[0] {
		t.Fatalf("anchor = %q, want %q (the engine's choice, not the caller's)", state.AnchorNodeID, f.episodes[0])
	}
}

func TestSetFocus_anchorIsNilWhenNothingIsWrittenYet(t *testing.T) {
	f := newFixture(t)
	state, err := f.repo.SetFocus(context.Background(), 1000, f.project.ID, "초점")
	if err != nil {
		t.Fatal(err)
	}
	if state.AnchorNodeID != "" {
		t.Fatalf("anchor = %q, want empty", state.AnchorNodeID)
	}
	if state.Stale {
		t.Fatalf("no anchor from nothing written yet must not be reported stale: %+v", state)
	}
}

// Staleness: 0 episodes since, 2 episodes since, 3 episodes since, and the
// anchor episode deleted.
func TestGetFocus_staleness(t *testing.T) {
	f := newFixture(t)
	f.write(t, 0) // 1화 written; anchors here
	state, err := f.repo.SetFocus(context.Background(), 1000, f.project.ID, "초점")
	if err != nil {
		t.Fatal(err)
	}
	if state.EpisodesSince != 0 || state.Stale {
		t.Fatalf("0 episodes since: got %+v", state)
	}

	f.write(t, 1) // 2화 written too; last written is now ordinal 2
	state, err = f.repo.GetFocus(context.Background(), f.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.EpisodesSince != 1 || state.Stale {
		t.Fatalf("1 episode since: got %+v", state)
	}

	f.write(t, 2) // 3화 written; last written ordinal 3 → 2 episodes since
	state, err = f.repo.GetFocus(context.Background(), f.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.EpisodesSince != 2 || state.Stale {
		t.Fatalf("2 episodes since: got %+v", state)
	}

	f.write(t, 3) // 4화 written; last written ordinal 4 → 3 episodes since: stale
	state, err = f.repo.GetFocus(context.Background(), f.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.EpisodesSince != 3 || !state.Stale {
		t.Fatalf("3 episodes since must be stale: got %+v", state)
	}

	// The anchor episode (1화) is deleted: EpisodesSince=-1, Stale=true.
	if err := f.nr.Delete(context.Background(), f.episodes[0], 5000); err != nil {
		t.Fatal(err)
	}
	state, err = f.repo.GetFocus(context.Background(), f.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.EpisodesSince != -1 || !state.Stale {
		t.Fatalf("deleted anchor: got %+v, want EpisodesSince=-1, Stale=true", state)
	}
}

func TestSetFocus_keepAsIs_reAnchorsToTheSameBody(t *testing.T) {
	f := newFixture(t)
	f.write(t, 0)
	if _, err := f.repo.SetFocus(context.Background(), 1000, f.project.ID, "초점"); err != nil {
		t.Fatal(err)
	}
	f.write(t, 1)
	f.write(t, 2)
	f.write(t, 3) // now stale: 3 episodes since

	// "그대로 유지" is calling SetFocus again with the same body.
	state, err := f.repo.SetFocus(context.Background(), 9000, f.project.ID, "초점")
	if err != nil {
		t.Fatal(err)
	}
	if state.Stale || state.EpisodesSince != 0 {
		t.Fatalf("re-anchoring with the same body must clear staleness: got %+v", state)
	}
	if state.AnchorNodeID != f.episodes[3] {
		t.Fatalf("anchor = %q, want the now-newest episode %q", state.AnchorNodeID, f.episodes[3])
	}
}

func TestSetSceneIntent_overLimitsAreRejectedNotTruncated(t *testing.T) {
	f := newFixture(t)
	nodeID := f.episodes[0]

	if _, err := f.repo.SetSceneIntent(context.Background(), 1000, SceneIntent{
		NodeID: nodeID, Goal: strings.Repeat("가", MaxGoalRunes+1),
	}); err != ErrGoalTooLong {
		t.Fatalf("err = %v, want ErrGoalTooLong", err)
	}

	items := make([]string, MaxListItems+1)
	for i := range items {
		items[i] = "항목" + strings.Repeat("x", i)
	}
	if _, err := f.repo.SetSceneIntent(context.Background(), 1000, SceneIntent{
		NodeID: nodeID, Keep: items,
	}); err != ErrTooManyItems {
		t.Fatalf("err = %v, want ErrTooManyItems", err)
	}

	if _, err := f.repo.SetSceneIntent(context.Background(), 1000, SceneIntent{
		NodeID: nodeID, Avoid: []string{strings.Repeat("가", MaxItemRunes+1)},
	}); err != ErrItemTooLong {
		t.Fatalf("err = %v, want ErrItemTooLong", err)
	}

	// None of the rejected calls stored anything.
	_, present, err := f.repo.GetSceneIntent(context.Background(), nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if present {
		t.Fatal("rejected scene intents must not be stored")
	}
}

func TestSetSceneIntent_emptyClearsTheRow(t *testing.T) {
	f := newFixture(t)
	nodeID := f.episodes[0]
	if _, err := f.repo.SetSceneIntent(context.Background(), 1000, SceneIntent{
		NodeID: nodeID, Goal: "목표", Keep: []string{"유지"}, Avoid: []string{"고백"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.SetSceneIntent(context.Background(), 2000, SceneIntent{NodeID: nodeID}); err != nil {
		t.Fatal(err)
	}
	_, present, err := f.repo.GetSceneIntent(context.Background(), nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if present {
		t.Fatal("empty goal/keep/avoid must clear the row")
	}
}

func TestSetSceneIntent_dedupsAndTrimsKeepAvoid(t *testing.T) {
	f := newFixture(t)
	nodeID := f.episodes[0]
	out, err := f.repo.SetSceneIntent(context.Background(), 1000, SceneIntent{
		NodeID: nodeID, Goal: "  목표  ", Keep: []string{" 고백 ", "고백", "COFFEE", "coffee"}, Avoid: []string{""},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Goal != "목표" {
		t.Errorf("goal = %q, want trimmed", out.Goal)
	}
	if len(out.Keep) != 2 || out.Keep[0] != "고백" || out.Keep[1] != "COFFEE" {
		t.Errorf("keep = %+v, want [고백 COFFEE] (case-insensitive dedup, first form kept)", out.Keep)
	}
	if len(out.Avoid) != 0 {
		t.Errorf("avoid = %+v, want empty (blank items dropped)", out.Avoid)
	}
}

func TestSetSceneIntent_rejectsNonScene(t *testing.T) {
	f := newFixture(t)
	containerID := f.episodes[1]
	if err := f.nr.ConvertLeafToContainer(context.Background(), containerID, 1000); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.SetSceneIntent(context.Background(), 1000, SceneIntent{
		NodeID: containerID, Goal: "목표",
	}); err != ErrNotScene {
		t.Fatalf("err = %v, want ErrNotScene", err)
	}
}

func TestSetSceneIntent_unknownNode(t *testing.T) {
	f := newFixture(t)
	if _, err := f.repo.SetSceneIntent(context.Background(), 1000, SceneIntent{
		NodeID: "missing", Goal: "목표",
	}); err != ErrNodeNotFound {
		t.Fatalf("err = %v, want ErrNodeNotFound", err)
	}
}

func TestSceneIntent_cascadesOnNodeDelete(t *testing.T) {
	f := newFixture(t)
	nodeID := f.episodes[1]
	if _, err := f.repo.SetSceneIntent(context.Background(), 1000, SceneIntent{
		NodeID: nodeID, Goal: "목표",
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.nr.Delete(context.Background(), nodeID, 2000); err != nil {
		t.Fatal(err)
	}
	_, present, err := f.repo.GetSceneIntent(context.Background(), nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if present {
		t.Fatal("deleting the node must cascade-delete its scene intent")
	}
}
