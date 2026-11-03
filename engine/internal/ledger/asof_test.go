package ledger

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/node"
)

type fakeNames map[string]string

func (f fakeNames) Names(_ context.Context, _, name string) []string { return []string{name} }

func (f fakeNames) AllNames(context.Context, string) [][]string { return nil }

func (f fakeNames) CanonicalName(_ context.Context, _, name string) string {
	if canonical, ok := f[name]; ok {
		return canonical
	}
	return name
}

func (f fixture) fact(t *testing.T, at int, subject, attribute, body string) Entry {
	t.Helper()
	e, err := f.repo.Add(f.ctx, int64(100+at), Entry{ProjectID: f.pid, Kind: KindNumber, Subject: subject, Attribute: attribute, Body: body, PlantedNodeID: f.scenes[at].ID})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func values(entries []Entry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Subject+"/"+e.Attribute+"="+e.Body)
	}
	return out
}

func TestAsOfAnswersForTheSceneItIsAskedAbout(t *testing.T) {
	f := setup(t, 6)
	f.fact(t, 1, "주인공", "위치", "객잔")
	f.fact(t, 4, "주인공", "위치", "산문")
	for _, tc := range []struct {
		at   int
		want string
	}{{0, ""}, {1, "주인공/위치=객잔"}, {3, "주인공/위치=객잔"}, {4, "주인공/위치=산문"}, {5, "주인공/위치=산문"}} {
		got, err := f.repo.AsOf(f.ctx, f.pid, f.scenes[tc.at].ID)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(values(got), ",") != tc.want {
			t.Errorf("scene %d: got %v, want %q", tc.at, values(got), tc.want)
		}
	}
}

func TestAFactEndsAtItsValidTo(t *testing.T) {
	f := setup(t, 4)
	end := f.scenes[2].ID
	if _, err := f.repo.Add(f.ctx, 1, Entry{ProjectID: f.pid, Kind: KindNumber, Subject: "주인공", Attribute: "부상", Body: "왼팔", PlantedNodeID: f.scenes[0].ID, ValidToNodeID: end}); err != nil {
		t.Fatal(err)
	}
	before, _ := f.repo.AsOf(f.ctx, f.pid, f.scenes[1].ID)
	after, _ := f.repo.AsOf(f.ctx, f.pid, f.scenes[2].ID)
	if len(before) != 1 || len(after) != 0 {
		t.Fatalf("before %v after %v", values(before), values(after))
	}
}

func TestWhatAPersonKnowsPilesUp(t *testing.T) {
	f := setup(t, 3)
	f.fact(t, 0, "주인공", AttributeKnows, "호위의 실력")
	f.fact(t, 1, "주인공", AttributeKnows, "약의 출처")
	got, _ := f.repo.AsOf(f.ctx, f.pid, f.scenes[2].ID)
	if len(got) != 2 {
		t.Fatalf("got %v", values(got))
	}
	conflicts, _, err := f.repo.Conflicts(f.ctx, got[1])
	if err != nil || len(conflicts) != 0 {
		t.Fatalf("conflicts %+v, err %v", conflicts, err)
	}
}

func TestSupersededFactLeavesTheStateButNotTheLedger(t *testing.T) {
	f := setup(t, 5)
	old := f.fact(t, 1, "주인공", "소속", "없음")
	next := f.fact(t, 3, "주인공", "소속", "표국")
	if err := f.repo.Supersede(f.ctx, 9, old.ID, next); err != nil {
		t.Fatal(err)
	}
	got, _ := f.repo.Get(f.ctx, old.ID)
	if got.SupersededBy != next.ID || got.ValidToNodeID != f.scenes[3].ID {
		t.Fatalf("old = %+v", got)
	}
	now, _ := f.repo.AsOf(f.ctx, f.pid, f.scenes[4].ID)
	if len(now) != 1 || now[0].ID != next.ID {
		t.Fatalf("got %v", values(now))
	}
}

func TestAPinnedFactCannotBeSuperseded(t *testing.T) {
	f := setup(t, 3)
	old := f.fact(t, 0, "작품", "규칙", "칼을 쥔 적 없다")
	if _, err := f.repo.SetPinned(f.ctx, 2, old.ID, true); err != nil {
		t.Fatal(err)
	}
	next := f.fact(t, 1, "작품", "규칙", "다른 규칙")
	if err := f.repo.Supersede(f.ctx, 3, old.ID, next); !errors.Is(err, ErrSupersedePinned) {
		t.Fatalf("err = %v", err)
	}
}

func TestConflictsNameTheFactADifferentValueMeets(t *testing.T) {
	f := setup(t, 4)
	first := f.fact(t, 1, "주인공", "위치", "객잔")
	second := f.fact(t, 2, "주인공", "위치", "객잔")
	if got, _, err := f.repo.Conflicts(f.ctx, second); err != nil || len(got) != 0 {
		t.Fatalf("the same value is no conflict: %+v, %v", got, err)
	}
	third := f.fact(t, 3, "주인공", "위치", "산문")
	got, _, err := f.repo.Conflicts(f.ctx, third)
	if err != nil || len(got) != 2 {
		t.Fatalf("got %+v, %v", got, err)
	}
	if got[0].EntryID != first.ID {
		t.Fatalf("got %+v", got[0])
	}
}

func TestAStoryDateThatGoesBackIsAConflict(t *testing.T) {
	f := setup(t, 3)
	day := func(at int, v float64) Entry {
		t.Helper()
		e, err := f.repo.Add(f.ctx, int64(at), Entry{ProjectID: f.pid, Kind: KindTimeline, Subject: "작품", Attribute: AttributeDate, Body: "날", ValueNum: &v, PlantedNodeID: f.scenes[at].ID})
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	day(0, 10)
	back := day(1, 7)
	conflicts, previous, err := f.repo.Conflicts(f.ctx, back)
	if err != nil {
		t.Fatal(err)
	}
	if previous == nil || previous.Delta != -3 {
		t.Fatalf("previous = %+v", previous)
	}
	found := false
	for _, c := range conflicts {
		found = found || strings.Contains(c.Reason, "뒤로")
	}
	if !found {
		t.Fatalf("conflicts = %+v", conflicts)
	}
}

func TestAnAliasFoldsOntoItsNameForAFactOnly(t *testing.T) {
	f := setup(t, 2)
	f.repo.WithSubjects(fakeNames{"아설": "백설화"})
	fact := f.fact(t, 0, "아설", "위치", "객잔")
	if fact.Subject != "백설화" {
		t.Fatalf("subject = %q", fact.Subject)
	}
	promise, err := f.repo.Add(f.ctx, 1, Entry{ProjectID: f.pid, Kind: KindPromise, Subject: "아설", Body: "약속"})
	if err != nil || promise.Subject != "아설" {
		t.Fatalf("promise %+v, err %v", promise, err)
	}
}

func TestFactFieldsBelongToFacts(t *testing.T) {
	f := setup(t, 2)
	_, err := f.repo.Add(f.ctx, 1, Entry{ProjectID: f.pid, Kind: KindPromise, Subject: "주인공", Attribute: "위치", Body: "객잔"})
	if !errors.Is(err, ErrFactOnly) {
		t.Fatalf("err = %v", err)
	}
	_, err = f.repo.Add(f.ctx, 1, Entry{ProjectID: f.pid, Kind: KindNumber, Attribute: "위치", Body: "객잔"})
	if !errors.Is(err, ErrFactNoSubject) {
		t.Fatalf("err = %v", err)
	}
	_, err = f.repo.Add(f.ctx, 1, Entry{ProjectID: f.pid, Kind: KindNumber, Body: "값", Strength: "maybe"})
	if !errors.Is(err, ErrInvalidStrength) {
		t.Fatalf("err = %v", err)
	}
}

func TestBriefShowsTheStateOfItsSceneAndMarksWhatIsUnconfirmed(t *testing.T) {
	f := setup(t, 4)
	if err := f.nodes.SetPlan(f.ctx, f.scenes[3].ID, "주인공이 객잔을 떠난다"); err != nil {
		t.Fatal(err)
	}
	f.fact(t, 0, "조연", "위치", "시장")
	f.fact(t, 1, "주인공", "위치", "객잔")
	if _, err := f.repo.Add(f.ctx, 1, Entry{ProjectID: f.pid, Kind: KindKnowledge, Subject: "주인공", Attribute: "소문", Body: "호위가 고수다", Strength: StrengthSoft, PlantedNodeID: f.scenes[1].ID}); err != nil {
		t.Fatal(err)
	}
	f.fact(t, 3, "주인공", "경지", "뒤에 얻을 것")
	got, err := f.repo.ContextLedger(f.ctx, f.pid, f.scenes[2].ID)
	if err != nil {
		t.Fatal(err)
	}
	all := strings.Join(got.State, "\n")
	if !strings.Contains(all, "위치=객잔") || !strings.Contains(all, "(미확정)") {
		t.Fatalf("state = %q", got.State)
	}
	if strings.Contains(all, "뒤에 얻을 것") {
		t.Fatalf("read ahead: %q", got.State)
	}
	if strings.Contains(strings.Join(got.Settled, "\n"), "객잔") {
		t.Fatalf("a fact appears as state, not settled: %q", got.Settled)
	}
	planned, _ := f.repo.ContextLedger(f.ctx, f.pid, f.scenes[3].ID)
	if len(planned.State) < 2 || !strings.HasPrefix(planned.State[0], "주인공") {
		t.Fatalf("the plan's subject comes first: %q", planned.State)
	}
}

func nested(t *testing.T, f fixture, arcs ...int) ([]node.Node, []node.Node) {
	t.Helper()
	var containers, scenes []node.Node
	for i, count := range arcs {
		arc, err := f.nodes.CreateChild(f.ctx, f.part.ID, node.KindContainer, fmt.Sprintf("아크%d", i+1), "", int64(500+i))
		if err != nil {
			t.Fatal(err)
		}
		containers = append(containers, arc)
		for j := range count {
			n, err := f.nodes.CreateChild(f.ctx, arc.ID, node.KindLeaf, fmt.Sprintf("%d-%d화", i+1, j+1), "", int64(600+i*10+j))
			if err != nil {
				t.Fatal(err)
			}
			scenes = append(scenes, n)
		}
	}
	return containers, scenes
}

func TestAPromiseLeavesTheBriefWhenItsArcEnds(t *testing.T) {
	f := setup(t, 0)
	_, scenes := nested(t, f, 2, 2)
	add := func(reach, body string, due string) {
		t.Helper()
		if _, err := f.repo.Add(f.ctx, 1, Entry{ProjectID: f.pid, Kind: KindPromise, Body: body, Reach: reach, PlantedNodeID: scenes[0].ID, DueNodeID: due}); err != nil {
			t.Fatal(err)
		}
	}
	add(ReachArc, "아크 약속", "")
	add(ReachVolume, "권 약속", "")
	add(ReachWork, "작품 약속", "")
	add(ReachArc, "기한 된 약속", scenes[3].ID)

	inArc, err := f.repo.ContextLedger(f.ctx, f.pid, scenes[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(append(inArc.Due, inArc.Open...), "|"); !strings.Contains(got, "아크 약속") {
		t.Fatalf("got %q", got)
	}
	next, err := f.repo.ContextLedger(f.ctx, f.pid, scenes[3].ID)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(append(next.Due, next.Open...), "|")
	for _, want := range []string{"권 약속", "작품 약속", "기한 된 약속"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "|아크 약속") || strings.HasPrefix(got, "아크 약속") {
		t.Errorf("a closed arc's promise is still in the brief: %q", got)
	}
	if next.Omitted == 0 {
		t.Error("what fell out of reach must still be counted")
	}
}

func TestABriefNamesTheArcItIsIn(t *testing.T) {
	f := setup(t, 0)
	arcs, scenes := nested(t, f, 3)
	if _, err := f.repo.SetArc(f.ctx, 1, Arc{NodeID: arcs[0].ID, Goal: "소문을 잡는다"}); err != nil {
		t.Fatal(err)
	}
	got, err := f.repo.ContextLedger(f.ctx, f.pid, scenes[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Arc == nil || got.Arc.Goal != "소문을 잡는다" || got.Arc.ScenesLeft != 2 {
		t.Fatalf("arc = %+v", got.Arc)
	}
}

func writeText(t *testing.T, f fixture, n node.Node, text string) {
	t.Helper()
	doc := fmt.Sprintf(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":%q}]}]}`, text)
	if err := f.nodes.UpdateContent(f.ctx, n.ID, doc, 9000); err != nil {
		t.Fatal(err)
	}
}

// The first scene of a real work had no mention links at all, and ranking by
// links buried it. The prose is what is compared.
func TestImpactFindsAnUnlinkedSceneFromItsWords(t *testing.T) {
	f := setup(t, 5)
	writeText(t, f, f.scenes[0], "검은 옻칠 위에 금박을 입힌 나무패였다.")
	writeText(t, f, f.scenes[1], "주인공은 국수를 먹었다.")
	writeText(t, f, f.scenes[3], "주인공은 나무패의 금박이 벗겨지는 것을 보았다.")
	e, err := f.repo.Add(f.ctx, 1, Entry{ProjectID: f.pid, Kind: KindDecision, Subject: "나무패", Body: "나무패 전체에 금박을 입혔다", PlantedNodeID: f.scenes[3].ID})
	if err != nil {
		t.Fatal(err)
	}
	got, omitted, err := f.repo.Impact(f.ctx, e.ID)
	if err != nil || omitted != 0 {
		t.Fatalf("omitted %d, err %v", omitted, err)
	}
	ids := make([]string, 0, len(got))
	for _, c := range got {
		ids = append(ids, c.NodeID)
	}
	if !slices.Contains(ids, f.scenes[0].ID) || !slices.Contains(ids, f.scenes[3].ID) {
		t.Fatalf("got %+v", got)
	}
	if slices.Contains(ids, f.scenes[1].ID) {
		t.Fatalf("a scene with nothing to do with it: %+v", got)
	}
	if got[0].NodeID != f.scenes[0].ID {
		t.Fatalf("candidates come in story order: %+v", got)
	}
}

func TestImpactStopsAtItsLimitAndSaysSo(t *testing.T) {
	f := setup(t, 20)
	for _, n := range f.scenes {
		writeText(t, f, n, "주인공은 나무패를 보았다.")
	}
	e, err := f.repo.Add(f.ctx, 1, Entry{ProjectID: f.pid, Kind: KindDecision, Subject: "주인공", Body: "나무패를 잃는다"})
	if err != nil {
		t.Fatal(err)
	}
	got, omitted, err := f.repo.Impact(f.ctx, e.ID)
	if err != nil || len(got) != impactMaxScenes || omitted != 8 {
		t.Fatalf("got %d, omitted %d, err %v", len(got), omitted, err)
	}
}

func TestSampleScenesMarkWhatOpensEachContainer(t *testing.T) {
	f := setup(t, 0)
	_, scenes := nested(t, f, 2, 2)
	for _, n := range scenes {
		writeText(t, f, n, "글")
	}
	got, err := f.repo.SampleScenes(f.ctx, f.pid)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || !got[0].Anchor || got[1].Anchor || !got[2].Anchor || got[3].Anchor {
		t.Fatalf("got %+v", got)
	}
}

func TestARumourOrAMovingDateIsNoConflict(t *testing.T) {
	f := setup(t, 3)
	f.fact(t, 0, "주인공", "직업", "포졸")
	lie, err := f.repo.Add(f.ctx, 1, Entry{ProjectID: f.pid, Kind: KindNumber, Subject: "주인공", Attribute: "직업", Body: "약장수", Strength: StrengthSoft, PlantedNodeID: f.scenes[1].ID})
	if err != nil {
		t.Fatal(err)
	}
	if got, _, err := f.repo.Conflicts(f.ctx, lie); err != nil || len(got) != 0 {
		t.Fatalf("a lie is the character's, not the story's: %+v, %v", got, err)
	}
	one, two := 1.0, 2.0
	if _, err := f.repo.Add(f.ctx, 1, Entry{ProjectID: f.pid, Kind: KindTimeline, Subject: "작품", Attribute: AttributeDate, Body: "1", ValueNum: &one, PlantedNodeID: f.scenes[0].ID}); err != nil {
		t.Fatal(err)
	}
	later, err := f.repo.Add(f.ctx, 1, Entry{ProjectID: f.pid, Kind: KindTimeline, Subject: "작품", Attribute: AttributeDate, Body: "2", ValueNum: &two, PlantedNodeID: f.scenes[1].ID})
	if err != nil {
		t.Fatal(err)
	}
	if got, _, err := f.repo.Conflicts(f.ctx, later); err != nil || len(got) != 0 {
		t.Fatalf("a day passing is no conflict: %+v, %v", got, err)
	}
}
