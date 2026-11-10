package ledger

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

type timeFixture struct{ fixture }

func newTimeFixture(t *testing.T, episodes int) timeFixture {
	t.Helper()
	return timeFixture{setup(t, episodes)}
}

func (f timeFixture) add(t *testing.T, e Entry) Entry {
	t.Helper()
	e.ProjectID = f.pid
	got, err := f.repo.Add(f.ctx, e.CreatedAt, e)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// event is a timeline entry planted at scene, with its story date when known.
func (f timeFixture) event(t *testing.T, scene int, body string, date *float64) Entry {
	t.Helper()
	return f.add(t, Entry{Kind: KindTimeline, Body: body, ValueNum: date, PlantedNodeID: f.scenes[scene].ID, CreatedAt: 1})
}

func (f timeFixture) elapsed(t *testing.T, now int64, scene int, ev Entry, days float64, body string) Entry {
	t.Helper()
	return f.add(t, Entry{Kind: KindTimeline, Subject: ev.Body, Attribute: AttributeElapsed, Body: body, About: ev.ID,
		ValueNum: &days, PlantedNodeID: f.scenes[scene].ID, CreatedAt: now})
}

func (f timeFixture) date(t *testing.T, now int64, scene int, day float64) Entry {
	t.Helper()
	return f.add(t, Entry{Kind: KindTimeline, Subject: "작품", Attribute: AttributeDate, Body: fmt.Sprintf("%g일", day),
		ValueNum: &day, PlantedNodeID: f.scenes[scene].ID, CreatedAt: now})
}

func (f timeFixture) age(t *testing.T, now int64, scene int, subject string, years float64) Entry {
	t.Helper()
	return f.add(t, Entry{Kind: KindDecision, Subject: subject, Attribute: AttributeAge, Body: fmt.Sprintf("%g살", years),
		ValueNum: &years, PlantedNodeID: f.scenes[scene].ID, CreatedAt: now})
}

func (f timeFixture) compare(t *testing.T, e Entry) ([]Conflict, *Previous) {
	t.Helper()
	c, p, err := f.repo.Conflicts(f.ctx, e)
	if err != nil {
		t.Fatal(err)
	}
	return c, p
}

func ptr(v float64) *float64 { return &v }

func TestToleranceFollowsTheSizeOfTheCount(t *testing.T) {
	for _, c := range []struct{ days, want float64 }{
		{0, 1}, {13, 1}, {14, 7}, {59, 7}, {60, 30}, {729, 30}, {730, 183}, {-90, 30},
	} {
		if got := tolerance(c.days); got != c.want {
			t.Errorf("tolerance(%g) = %g, want %g", c.days, got, c.want)
		}
	}
}

func TestClockAtReadsTheScenesOwnFacts(t *testing.T) {
	f := newTimeFixture(t, 2)
	known := f.event(t, 0, "전쟁 발발", ptr(100))
	unknown := f.event(t, 0, "등대지기 실종", nil)
	events := map[string]Entry{known.ID: known, unknown.ID: unknown}
	date := f.date(t, 2, 0, 400)
	fromKnown := f.elapsed(t, 3, 0, known, 30, "한 달")
	fromUnknown := f.elapsed(t, 4, 0, unknown, 90, "석 달")
	for name, c := range map[string]struct {
		facts []Entry
		skip  string
		want  float64
		ok    bool
	}{
		"a date alone":                      {[]Entry{date}, "", 400, true},
		"elapsed from a dated event":        {[]Entry{fromKnown}, "", 130, true},
		"both: the date wins":               {[]Entry{fromKnown, date}, "", 400, true},
		"elapsed from an undated event":     {[]Entry{fromUnknown}, "", 0, false},
		"the event being compared":          {[]Entry{fromKnown}, known.ID, 0, false},
		"nothing":                           {nil, "", 0, false},
		"a soft date says nothing for sure": {[]Entry{{Attribute: AttributeDate, ValueNum: ptr(5), Strength: StrengthSoft, Status: StatusOpen}}, "", 0, false},
	} {
		got, ok := clockAt(c.facts, events, c.skip)
		if ok != c.ok || got != c.want {
			t.Errorf("%s: clock = %g, %v; want %g, %v", name, got, ok, c.want, c.ok)
		}
	}
}

// The case #346 found: "석 달" in 3화, "삼 년" in 7화.
func TestElapsedJumpWithNoStoryDateIsReturnedNotFlagged(t *testing.T) {
	f := newTimeFixture(t, 8)
	ev := f.event(t, 0, "등대지기 실종", nil)
	first := f.elapsed(t, 1, 2, ev, 90, "석 달")
	got, previous := f.compare(t, f.elapsed(t, 2, 6, ev, 1095, "삼 년"))
	if len(got) != 0 {
		t.Fatalf("with no story date the engine does not judge a jump: %+v", got)
	}
	if previous == nil || previous.EntryID != first.ID || previous.JumpDays != 1005 || previous.ScenesBetween != 4 {
		t.Fatalf("previous = %+v", previous)
	}
}

func TestElapsedJumpAgainstAnotherMeasureConflicts(t *testing.T) {
	f := newTimeFixture(t, 8)
	ev := f.event(t, 0, "등대지기 실종", nil)
	f.date(t, 1, 2, 100)
	f.date(t, 1, 6, 112)
	f.elapsed(t, 2, 2, ev, 90, "석 달")
	got, previous := f.compare(t, f.elapsed(t, 3, 6, ev, 1095, "삼 년"))
	if len(got) != 1 || !strings.Contains(got[0].Reason, "3화와 7화 사이에 다른 기준으로는 12일이 흘렀는데") ||
		!strings.Contains(got[0].Reason, "1,005일 늘었습니다") {
		t.Fatalf("conflicts = %+v", got)
	}
	if previous == nil || previous.JumpDays != 0 {
		t.Fatalf("a judged jump is no jump to report: %+v", previous)
	}
}

// Another event with a known date is a measure too.
func TestElapsedFlowCanBeMeasuredFromAnotherEvent(t *testing.T) {
	f := newTimeFixture(t, 8)
	ev := f.event(t, 0, "등대지기 실종", nil)
	war := f.event(t, 0, "전쟁 발발", ptr(0))
	f.elapsed(t, 1, 2, war, 10, "열흘")
	f.elapsed(t, 1, 6, war, 22, "보름 남짓")
	f.elapsed(t, 2, 2, ev, 90, "석 달")
	if got, _ := f.compare(t, f.elapsed(t, 3, 6, ev, 1095, "삼 년")); len(got) != 1 {
		t.Fatalf("conflicts = %+v", got)
	}
}

func TestElapsedThatGrowsWithTheStoryIsNoConflict(t *testing.T) {
	f := newTimeFixture(t, 8)
	ev := f.event(t, 0, "등대지기 실종", nil)
	f.date(t, 1, 2, 100)
	f.date(t, 1, 6, 190)
	f.elapsed(t, 2, 2, ev, 90, "석 달")
	if got, previous := f.compare(t, f.elapsed(t, 3, 6, ev, 180, "반년")); len(got) != 0 || previous == nil || previous.Delta != 90 {
		t.Fatalf("석 달, then 90 days on, 반년: %+v %+v", got, previous)
	}
}

func TestElapsedThatShrinksConflicts(t *testing.T) {
	f := newTimeFixture(t, 8)
	ev := f.event(t, 0, "등대지기 실종", nil)
	f.elapsed(t, 1, 2, ev, 90, "석 달")
	got, _ := f.compare(t, f.elapsed(t, 2, 6, ev, 30, "한 달"))
	if len(got) != 1 || got[0].Reason != "3화에서 이미 90일이 지났는데 7화에서는 30일입니다" {
		t.Fatalf("conflicts = %+v", got)
	}
}

func TestElapsedToleranceBoundary(t *testing.T) {
	for _, c := range []struct {
		later    float64
		conflict bool
	}{{60, false}, {59, true}} {
		f := newTimeFixture(t, 4)
		ev := f.event(t, 0, "실종", nil)
		f.elapsed(t, 1, 1, ev, 90, "석 달")
		if got, _ := f.compare(t, f.elapsed(t, 2, 3, ev, c.later, "두 달")); (len(got) > 0) != c.conflict {
			t.Errorf("90 then %g: %+v", c.later, got)
		}
	}
	// With story dates: 90 days pass, and the count grows by 120 or 121, against
	// a 30-day tolerance.
	for _, c := range []struct {
		later    float64
		conflict bool
	}{{210, false}, {211, true}} {
		f := newTimeFixture(t, 4)
		ev := f.event(t, 0, "실종", nil)
		f.date(t, 1, 1, 0)
		f.date(t, 1, 3, 90)
		f.elapsed(t, 2, 1, ev, 90, "석 달")
		if got, _ := f.compare(t, f.elapsed(t, 3, 3, ev, c.later, "일곱 달")); (len(got) > 0) != c.conflict {
			t.Errorf("90 then %g over 90 days: %+v", c.later, got)
		}
	}
}

func TestSoftAndReplacedElapsedAreNotCompared(t *testing.T) {
	f := newTimeFixture(t, 4)
	ev := f.event(t, 0, "실종", nil)
	f.add(t, Entry{Kind: KindTimeline, Subject: ev.Body, Attribute: AttributeElapsed, Body: "석 달이라더라", About: ev.ID,
		ValueNum: ptr(90), Strength: StrengthSoft, PlantedNodeID: f.scenes[1].ID, CreatedAt: 1})
	if got, previous := f.compare(t, f.elapsed(t, 2, 3, ev, 30, "한 달")); len(got) != 0 || previous != nil {
		t.Fatalf("a rumour is not a measure: %+v %+v", got, previous)
	}
	rumour := f.add(t, Entry{Kind: KindTimeline, Subject: ev.Body, Attribute: AttributeElapsed, Body: "삼 년이라더라", About: ev.ID,
		ValueNum: ptr(1095), Strength: StrengthSoft, PlantedNodeID: f.scenes[3].ID, CreatedAt: 3})
	if got, previous := f.compare(t, rumour); len(got) != 0 || previous != nil {
		t.Fatalf("a rumour is not compared either: %+v %+v", got, previous)
	}

	g := newTimeFixture(t, 4)
	ev = g.event(t, 0, "실종", nil)
	old := g.elapsed(t, 1, 1, ev, 90, "석 달")
	fixed := g.elapsed(t, 2, 3, ev, 30, "한 달")
	if err := g.repo.Supersede(g.ctx, 3, old.ID, fixed); err != nil {
		t.Fatal(err)
	}
	if got, _ := g.compare(t, fixed); len(got) != 0 {
		t.Fatalf("the writer replaced it: %+v", got)
	}
}

func TestElapsedFactsPointAtEvents(t *testing.T) {
	f := newTimeFixture(t, 2)
	q := f.add(t, Entry{Kind: KindQuestion, Body: "누가 등대지기를 데려갔나", CreatedAt: 1})
	ev := f.event(t, 0, "등대지기 실종", nil)
	measure := f.elapsed(t, 2, 1, ev, 90, "석 달")
	elapsed := func(about string) Entry {
		return Entry{ProjectID: f.pid, Kind: KindTimeline, Subject: "실종", Attribute: AttributeElapsed, Body: "석 달", About: about, ValueNum: ptr(90)}
	}
	for name, c := range map[string]struct {
		e    Entry
		want error
	}{
		"elapsed about a question":  {elapsed(q.ID), ErrAboutNotEvent},
		"elapsed about an elapsed":  {elapsed(measure.ID), ErrAboutNotEvent},
		"elapsed about nothing":     {elapsed("없는-사건"), ErrAboutNotEvent},
		"knows about an event":      {Entry{ProjectID: f.pid, Kind: KindKnowledge, Subject: "서진", Attribute: AttributeKnows, Body: "앎", About: ev.ID, ValueNum: ptr(1)}, ErrAboutNotQuestion},
		"elapsed of the wrong kind": {Entry{ProjectID: f.pid, Kind: KindDecision, Subject: "실종", Attribute: AttributeElapsed, Body: "석 달", About: ev.ID, ValueNum: ptr(90)}, ErrAboutWrongFact},
	} {
		if _, err := f.repo.Add(f.ctx, 1, c.e); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
		}
	}
	other := setup(t, 1)
	if _, err := other.repo.Add(other.ctx, 1, Entry{ProjectID: other.pid, Kind: KindTimeline, Subject: "실종", Attribute: AttributeElapsed,
		Body: "석 달", About: ev.ID, ValueNum: ptr(90)}); !errors.Is(err, ErrAboutNotEvent) {
		t.Fatalf("about an event of another work: %v", err)
	}

	got, err := f.repo.Get(f.ctx, measure.ID)
	if err != nil || got.About != ev.ID || *got.ValueNum != 90 || got.Attribute != AttributeElapsed {
		t.Fatalf("stored = %+v, %v", got, err)
	}
	// Kept without an event or a number, and not compared.
	loose := f.add(t, Entry{Kind: KindTimeline, Subject: "실종", Attribute: AttributeElapsed, Body: "한참", PlantedNodeID: f.scenes[1].ID, CreatedAt: 3})
	if c, p := f.compare(t, loose); len(c) != 0 || p != nil {
		t.Fatalf("an elapsed time with no event: %+v %+v", c, p)
	}
	noNumber := f.add(t, Entry{Kind: KindTimeline, Subject: "실종", Attribute: AttributeElapsed, Body: "한참", About: ev.ID, PlantedNodeID: f.scenes[1].ID, CreatedAt: 4})
	if c, p := f.compare(t, noNumber); len(c) != 0 || p != nil {
		t.Fatalf("an elapsed time with no number: %+v %+v", c, p)
	}
}

func TestAgeIsTimeSinceBirth(t *testing.T) {
	f := newTimeFixture(t, 4)
	f.date(t, 1, 0, 0)
	f.date(t, 1, 1, 370)
	f.age(t, 2, 0, "도윤", 17)
	if got, _ := f.compare(t, f.age(t, 3, 1, "도윤", 18)); len(got) != 0 {
		t.Fatalf("a year on, a year older: %+v", got)
	}

	g := newTimeFixture(t, 4)
	g.date(t, 1, 0, 0)
	g.date(t, 1, 1, 12)
	g.age(t, 2, 0, "도윤", 17)
	got, _ := g.compare(t, g.age(t, 3, 1, "도윤", 30))
	if len(got) != 1 || !strings.Contains(got[0].Reason, "12일이 흘렀는데, 도윤의 나이는 13살 늘었습니다") {
		t.Fatalf("열일곱, twelve days on, 서른: %+v", got)
	}

	h := newTimeFixture(t, 4)
	h.age(t, 2, 0, "도윤", 17)
	got, previous := h.compare(t, h.age(t, 3, 1, "도윤", 30))
	if len(got) != 0 || previous == nil || previous.JumpDays != 13*daysPerYear || previous.ScenesBetween != 1 {
		t.Fatalf("no story date: %+v %+v", got, previous)
	}
	if got, _ := h.compare(t, h.age(t, 4, 2, "도윤", 16)); len(got) != 1 || !strings.Contains(got[0].Reason, "이미 30살이었는데") {
		t.Fatalf("younger again: %+v", got)
	}
}

// An age with no number is compared as before: any other value is a conflict.
func TestAgeWithoutANumberStaysAFreeFact(t *testing.T) {
	f := newTimeFixture(t, 3)
	f.add(t, Entry{Kind: KindDecision, Subject: "도윤", Attribute: AttributeAge, Body: "서른", PlantedNodeID: f.scenes[0].ID, CreatedAt: 1})
	later := f.add(t, Entry{Kind: KindDecision, Subject: "도윤", Attribute: AttributeAge, Body: "서른하나", PlantedNodeID: f.scenes[2].ID, CreatedAt: 2})
	if got, _ := f.compare(t, later); len(got) != 1 {
		t.Fatalf("conflicts = %+v", got)
	}
}

func TestAFlashbackLeavesStoryOrderAlone(t *testing.T) {
	f := newTimeFixture(t, 4)
	ev := f.event(t, 0, "실종", nil)
	f.date(t, 1, 0, 100)
	f.elapsed(t, 2, 0, ev, 90, "석 달")
	place := f.add(t, Entry{Kind: KindDecision, Subject: "서진", Attribute: "위치", Body: "등대", PlantedNodeID: f.scenes[0].ID, CreatedAt: 3})
	if err := f.nodes.SetFlashback(f.ctx, f.scenes[1].ID, true); err != nil {
		t.Fatal(err)
	}
	back := f.date(t, 4, 1, 10)
	if got, _ := f.compare(t, back); len(got) != 0 {
		t.Fatalf("a flashback's date goes back: %+v", got)
	}
	if got, _ := f.compare(t, f.elapsed(t, 5, 1, ev, 3, "사흘")); len(got) != 0 {
		t.Fatalf("a flashback's elapsed time is smaller: %+v", got)
	}
	f.add(t, Entry{Kind: KindDecision, Subject: "서진", Attribute: "위치", Body: "옛집", PlantedNodeID: f.scenes[1].ID, CreatedAt: 6})

	// After the flashback the story goes on from where it was.
	state, err := f.repo.AsOf(f.ctx, f.pid, f.scenes[2].ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range state {
		if e.Attribute == "위치" && e.ID != place.ID {
			t.Fatalf("the flashback changed the present: %+v", e)
		}
	}
	if got, _ := f.compare(t, f.date(t, 7, 2, 105)); len(got) != 0 {
		t.Fatalf("after the flashback, compared with it: %+v", got)
	}
	if got, previous := f.compare(t, f.elapsed(t, 8, 2, ev, 30, "한 달")); len(got) != 1 || previous.ValueNum != 90 {
		t.Fatalf("after the flashback, compared with 1화: %+v %+v", got, previous)
	}
	// Inside the flashback, its own facts are compared with each other.
	if got, _ := f.compare(t, f.date(t, 9, 1, 5)); len(got) != 1 {
		t.Fatalf("within the flashback: %+v", got)
	}
	if err := f.nodes.SetFlashback(f.ctx, f.part.ID, true); err == nil {
		t.Fatal("a part is not a flashback")
	}
}

func TestBriefTellsTheStoryTime(t *testing.T) {
	f := newTimeFixture(t, 8)
	ev := f.event(t, 0, "등대지기 실종", ptr(10))
	f.elapsed(t, 2, 2, ev, 90, "석 달")
	f.date(t, 3, 2, 100)
	other := f.event(t, 1, "전쟁", nil)
	f.elapsed(t, 4, 3, other, 3, "사흘")
	// Ahead of the scene being written: must not show.
	f.elapsed(t, 5, 6, ev, 1095, "삼 년")
	if err := f.nodes.SetPlan(f.ctx, f.scenes[4].ID, "등대지기가 사라진 뒤의 마을"); err != nil {
		t.Fatal(err)
	}

	b, err := f.repo.ContextLedger(f.ctx, f.pid, f.scenes[4].ID)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(b.Time, "\n")
	if !strings.Contains(text, "마지막으로 아는 작중 날짜: 100일 (3화)") {
		t.Fatalf("time = %q", b.Time)
	}
	if !strings.Contains(text, "등대지기 실종(1화): 3화에서 석 달(약 90일)") || !strings.Contains(text, "전쟁(2화): 4화에서 사흘(약 3일)") {
		t.Fatalf("time = %q", b.Time)
	}
	if strings.Contains(text, "삼 년") {
		t.Fatalf("read ahead: %q", b.Time)
	}
	if strings.Index(text, "등대지기") > strings.Index(text, "전쟁") {
		t.Fatalf("the event the scene talks about comes first: %q", b.Time)
	}
	for _, line := range b.State {
		if strings.Contains(line, "석 달") {
			t.Fatalf("an elapsed time is told once, under the story time: %q", b.State)
		}
	}

	f.date(t, 6, 4, 130)
	b, err = f.repo.ContextLedger(f.ctx, f.pid, f.scenes[4].ID)
	if err != nil {
		t.Fatal(err)
	}
	text = strings.Join(b.Time, "\n")
	if !strings.Contains(text, "이 회차의 작중 날짜: 130일") || !strings.Contains(text, "이 회차에서 약 120일") {
		t.Fatalf("with this scene's date: %q", b.Time)
	}
}

// A synthetic work of a thousand episodes, each measuring time from its own
// event: the section stays in its budget and says what it left out.
func TestBriefStoryTimeStaysInItsBudget(t *testing.T) {
	f := newTimeFixture(t, 1000)
	for i := range 1000 {
		ev := f.event(t, i, fmt.Sprintf("사건 %d: %s", i, strings.Repeat("긴 이름 ", 4)), ptr(float64(i)))
		f.elapsed(t, int64(i+2), i, ev, float64(i), strings.Repeat("오래 ", 10))
	}
	b, err := f.repo.ContextLedger(f.ctx, f.pid, f.scenes[999].ID)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, line := range b.Time {
		n += utf8.RuneCountInString(line)
	}
	if n > briefTimeRunes || len(b.Time) < 2 {
		t.Fatalf("%d lines ran to %d runes", len(b.Time), n)
	}
	if b.Omitted < 1000-len(b.Time) {
		t.Fatalf("omitted = %d with %d shown", b.Omitted, len(b.Time))
	}
}
