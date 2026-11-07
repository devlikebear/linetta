package ledger

import (
	"strings"
	"testing"
)

type knowsFixture struct {
	fixture
	question Entry
}

func newKnowsFixture(t *testing.T) knowsFixture {
	t.Helper()
	f := setup(t, 3)
	q, err := f.repo.Add(f.ctx, 1, Entry{ProjectID: f.pid, Kind: KindQuestion, Body: "하람은 누구인가", Answer: "왕자다"})
	if err != nil {
		t.Fatal(err)
	}
	return knowsFixture{f, q}
}

func (f knowsFixture) knows(t *testing.T, now int64, subject string, scene int, v float64, about string) Entry {
	t.Helper()
	e, err := f.repo.Add(f.ctx, now, Entry{ProjectID: f.pid, Kind: KindKnowledge, Subject: subject, Attribute: AttributeKnows,
		Body: "하람의 정체", About: about, ValueNum: &v, PlantedNodeID: f.scenes[scene].ID})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func (f knowsFixture) conflicts(t *testing.T, e Entry) []Conflict {
	t.Helper()
	got, previous, err := f.repo.Conflicts(f.ctx, e)
	if err != nil {
		t.Fatal(err)
	}
	if previous != nil {
		t.Fatalf("a knows fact is not a running number: %+v", previous)
	}
	return got
}

func TestKnowingThenNotKnowingConflicts(t *testing.T) {
	f := newKnowsFixture(t)
	first := f.knows(t, 1, "서진", 0, KnowsYes, f.question.ID)
	got := f.conflicts(t, f.knows(t, 2, "서진", 2, KnowsNo, f.question.ID))
	if len(got) != 1 || got[0].EntryID != first.ID || !strings.Contains(got[0].Reason, "1화") {
		t.Fatalf("conflicts = %+v", got)
	}
}

func TestLearningAgainWhatWasKnownConflicts(t *testing.T) {
	f := newKnowsFixture(t)
	first := f.knows(t, 1, "서진", 0, KnowsYes, f.question.ID)
	got := f.conflicts(t, f.knows(t, 2, "서진", 1, KnowsYes, f.question.ID))
	if len(got) != 1 || got[0].EntryID != first.ID || !strings.Contains(got[0].Reason, "이미 1화에서") {
		t.Fatalf("conflicts = %+v", got)
	}
}

func TestLearningIsNoConflict(t *testing.T) {
	f := newKnowsFixture(t)
	f.knows(t, 1, "서진", 0, KnowsNo, f.question.ID)
	if got := f.conflicts(t, f.knows(t, 2, "서진", 1, KnowsYes, f.question.ID)); len(got) != 0 {
		t.Fatalf("not knowing, then learning: %+v", got)
	}
}

func TestAReplacedKnowsFactIsNoConflict(t *testing.T) {
	f := newKnowsFixture(t)
	first := f.knows(t, 1, "서진", 0, KnowsYes, f.question.ID)
	forgot := f.knows(t, 2, "서진", 2, KnowsNo, f.question.ID)
	if err := f.repo.Supersede(f.ctx, 3, first.ID, forgot); err != nil {
		t.Fatal(err)
	}
	if got := f.conflicts(t, forgot); len(got) != 0 {
		t.Fatalf("an amnesia the writer meant: %+v", got)
	}
}

func TestKnowsFactsAreComparedOnlyWithinOnePersonAndQuestion(t *testing.T) {
	f := newKnowsFixture(t)
	other, err := f.repo.Add(f.ctx, 1, Entry{ProjectID: f.pid, Kind: KindQuestion, Body: "누가 불을 냈나"})
	if err != nil {
		t.Fatal(err)
	}
	f.knows(t, 1, "서진", 0, KnowsYes, f.question.ID)
	f.knows(t, 1, "도윤", 0, KnowsYes, other.ID)
	for name, e := range map[string]Entry{
		"another person":   f.knows(t, 2, "도윤", 2, KnowsNo, f.question.ID),
		"another question": f.knows(t, 2, "서진", 2, KnowsNo, other.ID),
	} {
		if got := f.conflicts(t, e); len(got) != 0 {
			t.Errorf("%s: %+v", name, got)
		}
	}
	// Knowing it in a later scene says nothing about not knowing it before.
	f.knows(t, 3, "하늘", 2, KnowsYes, f.question.ID)
	if got := f.conflicts(t, f.knows(t, 4, "하늘", 0, KnowsNo, f.question.ID)); len(got) != 0 {
		t.Fatalf("a later fact compared as an earlier one: %+v", got)
	}
}

// Without a question, what a person knows piles up and is not compared.
func TestKnowsFactsWithoutAQuestionAreNotCompared(t *testing.T) {
	f := newKnowsFixture(t)
	add := func(body string, scene int) Entry {
		e, err := f.repo.Add(f.ctx, int64(scene), Entry{ProjectID: f.pid, Kind: KindKnowledge, Subject: "서진", Attribute: AttributeKnows,
			Body: body, PlantedNodeID: f.scenes[scene].ID})
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	add("하람의 정체를 앎", 0)
	if got := f.conflicts(t, add("하람의 정체를 모름", 1)); len(got) != 0 {
		t.Fatalf("conflicts = %+v", got)
	}
}
