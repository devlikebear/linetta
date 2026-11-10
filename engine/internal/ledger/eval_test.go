package ledger

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"testing"
)

type evalCase struct {
	ID            string   `json:"id"`
	Type          string   `json:"type"`
	Split         string   `json:"split"`
	Contradiction bool     `json:"contradiction"`
	Scenes        []string `json:"scenes"`
	Fact          struct {
		Subject   string `json:"subject"`
		Attribute string `json:"attribute"`
		Earlier   string `json:"earlier"`
		Later     string `json:"later"`
		LaterSoft bool   `json:"later_soft"`
		Replaces  bool   `json:"replaces"`
		Number    bool   `json:"number"`
		// About is the question a knows case is about, and Knows which way each
		// of its two facts goes, 1 knows and 0 does not.
		About string    `json:"about"`
		Knows []float64 `json:"knows"`
		// Event is what an elapsed case counts from, and Clock each scene's
		// story date in days, recorded as the measure the comparison reads.
		Event string    `json:"event"`
		Clock []float64 `json:"clock"`
	} `json:"fact"`
}

func loadEvalCases(t *testing.T) []evalCase {
	t.Helper()
	raw, err := os.ReadFile("testdata/eval/consistency.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []evalCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	return cases
}

type score struct{ tp, fp, fn, tn int }

func (s score) precision() float64 {
	if s.tp+s.fp == 0 {
		return 1
	}
	return float64(s.tp) / float64(s.tp+s.fp)
}

func (s score) recall() float64 {
	if s.tp+s.fn == 0 {
		return 1
	}
	return float64(s.tp) / float64(s.tp+s.fn)
}

// A case's two facts as an agent records them, and whether the comparison spoke.
func evalFlags(t *testing.T, c evalCase) bool {
	t.Helper()
	f := setup(t, 2)
	writeText(t, f, f.scenes[0], c.Scenes[0])
	writeText(t, f, f.scenes[1], c.Scenes[1])
	value := func(text string) *float64 {
		if !c.Fact.Number {
			return nil
		}
		v, err := strconv.ParseFloat(text, 64)
		if err != nil {
			t.Fatal(err)
		}
		return &v
	}
	kind := KindNumber
	var about string
	earlier, later := value(c.Fact.Earlier), value(c.Fact.Later)
	if c.Fact.Attribute == AttributeKnows {
		kind = KindKnowledge
		q, err := f.repo.Add(f.ctx, 1, Entry{ProjectID: f.pid, Kind: KindQuestion, Body: c.Fact.About})
		if err != nil {
			t.Fatal(err)
		}
		about, earlier, later = q.ID, &c.Fact.Knows[0], &c.Fact.Knows[1]
	}
	if c.Fact.Attribute == AttributeElapsed {
		kind = KindTimeline
		ev, err := f.repo.Add(f.ctx, 1, Entry{ProjectID: f.pid, Kind: KindTimeline, Body: c.Fact.Event, PlantedNodeID: f.scenes[0].ID})
		if err != nil {
			t.Fatal(err)
		}
		about = ev.ID
		for i, day := range c.Fact.Clock {
			if _, err := f.repo.Add(f.ctx, 1, Entry{ProjectID: f.pid, Kind: KindTimeline, Subject: "작품", Attribute: AttributeDate,
				Body: strconv.FormatFloat(day, 'f', -1, 64) + "일", ValueNum: &day, PlantedNodeID: f.scenes[i].ID}); err != nil {
				t.Fatal(err)
			}
		}
	}
	first, err := f.repo.Add(f.ctx, 1, Entry{ProjectID: f.pid, Kind: kind, Subject: c.Fact.Subject, Attribute: c.Fact.Attribute,
		Body: c.Fact.Earlier, ValueNum: earlier, About: about, PlantedNodeID: f.scenes[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	strength := StrengthHard
	if c.Fact.LaterSoft {
		strength = StrengthSoft
	}
	second, err := f.repo.Add(f.ctx, 2, Entry{ProjectID: f.pid, Kind: kind, Subject: c.Fact.Subject, Attribute: c.Fact.Attribute,
		Body: c.Fact.Later, ValueNum: later, About: about, Strength: strength, PlantedNodeID: f.scenes[1].ID})
	if err != nil {
		t.Fatal(err)
	}
	if c.Fact.Replaces {
		if err := f.repo.Supersede(f.ctx, 3, first.ID, second); err != nil {
			t.Fatal(err)
		}
		second, _ = f.repo.Get(f.ctx, second.ID)
	}
	conflicts, _, err := f.repo.Conflicts(f.ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	return len(conflicts) > 0
}

// The share of the consistency set the engine can decide on its own. It reads
// values, not prose, so this measures the comparison given facts recorded under
// the right subject and attribute. Whether an agent records them that way is not
// measured here. The floors hold what was measured; lower one only on purpose.
// What a person knows piles up, so a knows case is recorded as two facts about
// one question, which way each goes given by the case: the score is what the
// comparison finds when an agent links them so, not whether it does.
func TestConsistencyCasesAgainstFactComparison(t *testing.T) {
	byType := map[string]*score{}
	var total score
	for _, c := range loadEvalCases(t) {
		if c.Split != "eval" {
			continue
		}
		if byType[c.Type] == nil {
			byType[c.Type] = &score{}
		}
		flagged := evalFlags(t, c)
		for _, s := range []*score{byType[c.Type], &total} {
			switch {
			case c.Contradiction && flagged:
				s.tp++
			case c.Contradiction:
				s.fn++
			case flagged:
				s.fp++
			default:
				s.tn++
			}
		}
	}
	types := make([]string, 0, len(byType))
	for k := range byType {
		types = append(types, k)
	}
	slices.Sort(types)
	for _, k := range types {
		s := byType[k]
		fmt.Printf("EVAL fact %-10s tp=%d fp=%d fn=%d tn=%d\n", k, s.tp, s.fp, s.fn, s.tn)
	}
	fmt.Printf("EVAL fact total precision=%.2f recall=%.2f\n", total.precision(), total.recall())
	if total.recall() < 0.85 || total.precision() < 0.95 {
		t.Fatalf("precision %.2f recall %.2f", total.precision(), total.recall())
	}
	// Phase 7 measured knows at tp=4 fp=0 fn=0 tn=4, from zero before it.
	if k := byType["knowledge"]; k == nil || k.recall() < 1 || k.precision() < 1 {
		t.Fatalf("knowledge: %+v", k)
	}
	// Phase 8: elapsed time from zero (no such type), with both scenes' story
	// dates recorded. An age as a number must not flag what the word did not.
	if k := byType["elapsed"]; k == nil || k.recall() < 0.8 || k.precision() < 0.95 {
		t.Fatalf("elapsed: %+v", k)
	}
	if k := byType["age"]; k == nil || k.recall() < 0.8 || k.precision() < 0.95 {
		t.Fatalf("age: %+v", k)
	}
}

// Given a changed fact, the scene it clashes with has to be among the candidates:
// the agent reads nothing else. The elapsed and age cases are for comparing
// values: their facts' bodies are bare numbers, which no search of the prose
// finds, and the age cases repeat the alias ones. They are left out so this
// stays the measure it was.
func TestConsistencyCasesFindTheEarlierScene(t *testing.T) {
	found, all := 0, 0
	for _, c := range loadEvalCases(t) {
		if c.Split != "eval" || !c.Contradiction || c.Type == "elapsed" || c.Type == "age" {
			continue
		}
		f := setup(t, 2)
		writeText(t, f, f.scenes[0], c.Scenes[0])
		writeText(t, f, f.scenes[1], c.Scenes[1])
		e, err := f.repo.Add(f.ctx, 1, Entry{ProjectID: f.pid, Kind: KindDecision, Subject: c.Fact.Subject, Body: c.Fact.Later, PlantedNodeID: f.scenes[1].ID})
		if err != nil {
			t.Fatal(err)
		}
		got, _, err := f.repo.Impact(f.ctx, e.ID)
		if err != nil {
			t.Fatal(err)
		}
		all++
		if slices.ContainsFunc(got, func(x Candidate) bool { return x.NodeID == f.scenes[0].ID }) {
			found++
		}
	}
	fmt.Printf("EVAL impact recall=%d/%d\n", found, all)
	if float64(found)/float64(all) < 0.8 {
		t.Fatalf("found %d of %d", found, all)
	}
}
