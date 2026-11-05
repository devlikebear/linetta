package main

import "testing"

func TestAsScenesSplitsLinesAndSkipsBlank(t *testing.T) {
	got := asScenes([]string{"첫 줄이다.\n\n  둘째 줄이다.  \n", "한 줄."})
	if len(got) != 2 {
		t.Fatalf("scenes = %d, want 2", len(got))
	}
	if p := got[0].Paragraphs; len(p) != 2 || p[1] != "둘째 줄이다." {
		t.Fatalf("paragraphs = %q", p)
	}
}

func TestMeasureCountsEachGroup(t *testing.T) {
	out := measure(input{Groups: map[string][]string{
		"mine": {"오늘은 비가 왔다. 우산을 챙겼다."},
		"ai":   {"그는 웃었고, 그녀는 울었다."},
	}})
	if out["mine"].Stats.Sentences != 2 {
		t.Fatalf("mine sentences = %d, want 2", out["mine"].Stats.Sentences)
	}
	if out["ai"].Check["parallel_contrast"] != 1 {
		t.Fatalf("ai check = %v, want one parallel_contrast", out["ai"].Check)
	}
}
