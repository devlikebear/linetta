package style

import (
	"reflect"
	"testing"
)

// Paragraphs is the one split both linetta_analyze_style and
// linetta_report_continuity number from (story-assist Phase 2, 2.A.1). These
// tests pin the exact slice sceneOf already relied on implicitly, so a future
// change to either caller's own numbering can't drift from this one.
func TestParagraphs_matchesSceneOf(t *testing.T) {
	doc := `{"type":"doc","content":[
		{"type":"paragraph","content":[{"type":"text","text":"첫 문단."}]},
		{"type":"paragraph","content":[{"type":"text","text":"둘째 문단."}]}
	]}`
	got := Paragraphs(doc)
	want := []string{"첫 문단.", "둘째 문단."}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Paragraphs = %v, want %v", got, want)
	}
}

func TestParagraphs_dropsEmptyParagraphs(t *testing.T) {
	// An empty paragraph node renders as an empty block, which DocToPlainText
	// joins with "\n\n" like any other — Paragraphs must filter it out so
	// the remaining paragraphs keep the numbers a writer would expect, not
	// numbers with gaps for blank lines.
	doc := `{"type":"doc","content":[
		{"type":"paragraph","content":[{"type":"text","text":"하나."}]},
		{"type":"paragraph"},
		{"type":"paragraph","content":[{"type":"text","text":"   "}]},
		{"type":"paragraph","content":[{"type":"text","text":"둘."}]}
	]}`
	got := Paragraphs(doc)
	want := []string{"하나.", "둘."}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Paragraphs = %v, want %v", got, want)
	}
}

func TestParagraphs_mentionRendersAsLabel(t *testing.T) {
	doc := `{"type":"doc","content":[
		{"type":"paragraph","content":[
			{"type":"text","text":"해진은 "},
			{"type":"mention","attrs":{"id":"e1","label":"해진"}},
			{"type":"text","text":"을 봤다."}
		]}
	]}`
	got := Paragraphs(doc)
	want := []string{"해진은 해진을 봤다."}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Paragraphs = %v, want %v", got, want)
	}
}

func TestParagraphs_emptyDocReturnsNil(t *testing.T) {
	if got := Paragraphs(""); got != nil {
		t.Errorf("Paragraphs(\"\") = %v, want nil", got)
	}
	if got := Paragraphs(`{"type":"doc","content":[]}`); got != nil {
		t.Errorf("Paragraphs(empty doc) = %v, want nil", got)
	}
}
