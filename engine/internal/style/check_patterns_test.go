package style

import "testing"

func rulesOf(rep Report) map[string]int {
	out := map[string]int{}
	for _, v := range rep.Violations {
		out[v.Rule]++
	}
	return out
}

func TestPatternsFindParallelAndExplainerInNarration(t *testing.T) {
	rep := Check([]Scene{{NodeID: "a", Label: "1화", Paragraphs: []string{
		"하는 위로 쌓였고, 그 한 줄은 바닥에 깔렸다.",
		"도윤은 고개를 들었고, 서진은 화면을 닫았다.",
		"그건 도윤이 읽었기 때문이다.",
		"도윤은 키보드를 당겨 앉으며 천천히 다시 읽었다.",
	}}}, Rules{})
	got := rulesOf(rep)
	if got[RuleParallel] != 2 || got[RuleExplainer] != 1 {
		t.Fatalf("got %v in %+v", got, rep.Violations)
	}
}

func TestPatternsSpareDialogue(t *testing.T) {
	rep := Check([]Scene{{NodeID: "a", Label: "1화", Paragraphs: []string{
		`"도윤은 고개를 들었고, 서진은 화면을 닫았다."`,
	}}}, Rules{})
	if rep.Total != 0 {
		t.Fatalf("speech flagged: %+v", rep.Violations)
	}
}

func TestRepeatedLineAcrossScenes(t *testing.T) {
	line := "도윤은 세 줄을 천천히 다시 읽었다."
	rep := Check([]Scene{
		{NodeID: "a", Label: "4화", Paragraphs: []string{line}},
		{NodeID: "b", Label: "5화", Paragraphs: []string{"다른 문장이 하나 있었다.", line, "끝이 다르게 맺어졌다."}},
	}, Rules{})
	if rulesOf(rep)[RuleRepeatedLine] != 1 {
		t.Fatalf("got %+v", rep.Violations)
	}
}

func TestEndingFormulaAcrossConsecutiveScenes(t *testing.T) {
	rep := Check([]Scene{
		{NodeID: "a", Label: "1화", Paragraphs: []string{"둘 다 사람은 아니었다."}},
		{NodeID: "b", Label: "2화", Paragraphs: []string{"둘 다 사람이었다."}},
		{NodeID: "c", Label: "3화", Paragraphs: []string{"약관은 읽지 않았다."}},
	}, Rules{})
	if rulesOf(rep)[RuleEndingFormula] != 1 {
		t.Fatalf("got %+v", rep.Violations)
	}
}
