package continuity

import (
	"fmt"
	"strings"
	"testing"
)

func sampleWork(n int) []SampleScene {
	scenes := make([]SampleScene, 0, n)
	for i := range n {
		scenes = append(scenes, SampleScene{NodeID: fmt.Sprintf("s%d", i), Label: fmt.Sprintf("%d화", i+1), Version: 1, Anchor: i == 0})
	}
	return scenes
}

func earlier(pairs []SamplePair) map[string]bool {
	out := map[string]bool{}
	for _, p := range pairs {
		out[p.Earlier.NodeID] = true
	}
	return out
}

func TestSamplePairsAskAFixedFewOfEachScene(t *testing.T) {
	for _, n := range []int{20, 200, 2000} {
		got := SamplePairs(sampleWork(n), fmt.Sprintf("s%d", n-1), "", nil)
		if len(got) != 4 {
			t.Fatalf("%d scenes: %d pairs", n, len(got))
		}
	}
}

func TestSamplePairsTakeTheAnchorAndTheLastTwo(t *testing.T) {
	got := earlier(SamplePairs(sampleWork(10), "s9", "", nil))
	for _, want := range []string{"s0", "s8", "s7"} {
		if !got[want] {
			t.Fatalf("missing %s: %v", want, got)
		}
	}
}

func TestSamplePairsAreTheSameForTheSameScene(t *testing.T) {
	a := SamplePairs(sampleWork(50), "s40", "", nil)
	b := SamplePairs(sampleWork(50), "s40", "", nil)
	if fmt.Sprint(a) != fmt.Sprint(b) {
		t.Fatalf("%v\n%v", a, b)
	}
}

func TestSamplePairsUseTheCandidateInsteadOfChance(t *testing.T) {
	got := earlier(SamplePairs(sampleWork(50), "s40", "s12", nil))
	if !got["s12"] || len(got) != 4 {
		t.Fatalf("got %v", got)
	}
}

func TestSamplePairsSkipWhatWasCompared(t *testing.T) {
	got := SamplePairs(sampleWork(10), "s9", "", func(p SamplePair) bool { return p.Earlier.NodeID == "s8" })
	if earlier(got)["s8"] {
		t.Fatalf("got %v", got)
	}
}

func TestTheFirstSceneHasNothingToCompareWith(t *testing.T) {
	if got := SamplePairs(sampleWork(3), "s0", "", nil); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestSampleRequestCarriesRulesAndPairsAndNothingElse(t *testing.T) {
	pairs := SamplePairs(sampleWork(5), "s4", "", nil)
	got := SampleRequest("p", []string{"칼을 쥔 적 없다"}, pairs)
	if want := "칼을 쥔 적 없다"; !strings.Contains(got, want) {
		t.Fatalf("missing %q in %q", want, got)
	}
	if !strings.Contains(got, "5화 (s4)") {
		t.Fatalf("got %q", got)
	}
	if calls := SampleCalls(pairs); calls < 2*len(pairs) {
		t.Fatalf("calls = %d for %d pairs", calls, len(pairs))
	}
}
