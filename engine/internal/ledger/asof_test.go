package ledger

import (
	"context"
	"errors"
	"strings"
	"testing"

)

type fakeNames map[string]string

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

