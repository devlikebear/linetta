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
	if c.Fact.Attribute == AttributeKnows {
		kind = KindKnowledge
	}
	first, err := f.repo.Add(f.ctx, 1, Entry{ProjectID: f.pid, Kind: kind, Subject: c.Fact.Subject, Attribute: c.Fact.Attribute,
		Body: c.Fact.Earlier, ValueNum: value(c.Fact.Earlier), PlantedNodeID: f.scenes[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	strength := StrengthHard
	if c.Fact.LaterSoft {
		strength = StrengthSoft
	}
	second, err := f.repo.Add(f.ctx, 2, Entry{ProjectID: f.pid, Kind: kind, Subject: c.Fact.Subject, Attribute: c.Fact.Attribute,
		Body: c.Fact.Later, ValueNum: value(c.Fact.Later), Strength: strength, PlantedNodeID: f.scenes[1].ID})
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
// What a person knows piles up, so it is not compared and that type scores zero
// here on purpose: a contradiction there takes reading, not values.
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
}

// Given a changed fact, the scene it clashes with has to be among the candidates:
// the agent reads nothing else.
func TestConsistencyCasesFindTheEarlierScene(t *testing.T) {
	found, all := 0, 0
	for _, c := range loadEvalCases(t) {
		if c.Split != "eval" || !c.Contradiction {
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
