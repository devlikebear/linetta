package ledger

import (
	"testing"
)

func TestKnowsAtFollowsTheStory(t *testing.T) {
	f := setup(t, 4)
	yes, no := KnowsYes, KnowsNo
	q, err := f.repo.Add(f.ctx, 1, Entry{ProjectID: f.pid, Kind: KindQuestion, Body: "하람은 누구인가", Answer: "왕자다", PlantedNodeID: f.scenes[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	unanswered, err := f.repo.Add(f.ctx, 1, Entry{ProjectID: f.pid, Kind: KindQuestion, Body: "누가 불을 냈나", PlantedNodeID: f.scenes[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	knows := func(subject string, scene int, v *float64) Entry {
		e, err := f.repo.Add(f.ctx, int64(10+scene), Entry{ProjectID: f.pid, Kind: KindKnowledge, Subject: subject, Attribute: AttributeKnows,
			Body: "하람의 정체", About: q.ID, ValueNum: v, PlantedNodeID: f.scenes[scene].ID})
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	notYet := knows("서진", 0, &no)
	learnt := knows("서진", 2, &yes)
	other := knows("도윤", 1, &yes)
	// A knows fact with no question stays a note.
	if _, err := f.repo.Add(f.ctx, 5, Entry{ProjectID: f.pid, Kind: KindKnowledge, Subject: "서진", Attribute: AttributeKnows, Body: "길을 앎", PlantedNodeID: f.scenes[0].ID}); err != nil {
		t.Fatal(err)
	}
	// The reader learns it in the third scene.
	status, paid := StatusPaid, f.scenes[2].ID
	if _, err := f.repo.Update(f.ctx, 20, q.ID, Patch{Status: &status, PaidNodeID: &paid}); err != nil {
		t.Fatal(err)
	}

	at := func(scene int, subjects ...string) Knowledge {
		k, err := f.repo.KnowsAt(f.ctx, f.pid, f.scenes[scene].ID, subjects)
		if err != nil {
			t.Fatal(err)
		}
		return k
	}

	k := at(1, "서진")
	if len(k.Reader) != 0 || len(k.Secrets) != 1 || k.Secrets[0].ID != q.ID {
		t.Fatalf("before it is paid: reader %+v, secrets %+v", k.Reader, k.Secrets)
	}
	if got := k.People["서진"][q.ID]; got.ID != notYet.ID {
		t.Fatalf("서진 at 2화 = %+v, want the 1화 fact: the 3화 one is still ahead", got)
	}
	if _, mixed := k.People["도윤"]; mixed {
		t.Fatal("asked about 서진 alone, got 도윤 too")
	}
	for _, s := range k.Secrets {
		if s.ID == unanswered.ID {
			t.Fatal("a question with no answer is no secret to keep")
		}
	}

	k = at(2)
	if len(k.Reader) != 1 || k.Reader[0].ID != q.ID || len(k.Secrets) != 0 {
		t.Fatalf("in the scene that pays it: reader %+v, secrets %+v", k.Reader, k.Secrets)
	}
	if got := k.People["서진"][q.ID]; got.ID != learnt.ID {
		t.Fatalf("서진 at 3화 = %+v", got)
	}
	if got := k.People["도윤"][q.ID]; got.ID != other.ID {
		t.Fatalf("도윤 at 3화 = %+v", got)
	}
	if len(k.People["서진"]) != 1 {
		t.Fatalf("서진 knows %+v; a note without a question is not one", k.People["서진"])
	}

	k = at(0)
	if _, ok := k.People["도윤"]; ok {
		t.Fatal("도윤 learns it in 2화, not 1화")
	}
}
