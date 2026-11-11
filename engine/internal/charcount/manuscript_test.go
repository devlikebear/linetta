package charcount

import (
	"encoding/json"
	"strings"
	"testing"
)

func para(texts ...string) map[string]any {
	content := []any{}
	for i, s := range texts {
		if i > 0 {
			content = append(content, map[string]any{"type": "hardBreak"})
		}
		if s != "" {
			content = append(content, map[string]any{"type": "text", "text": s})
		}
	}
	return map[string]any{"type": "paragraph", "content": content}
}

func doc(t *testing.T, blocks ...any) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"type": "doc", "content": blocks})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestManuscriptLines(t *testing.T) {
	cases := []struct {
		name string
		raw  []byte
		want int
	}{
		{"19 characters and the indent fill one line", doc(t, para(strings.Repeat("가", 19))), 1},
		{"20 characters spill onto a second line", doc(t, para(strings.Repeat("가", 20))), 2},
		{"a short line of dialogue takes a whole line", doc(t, para("\"어디 가?\""), para("\"집.\"")), 2},
		{"spaces take a cell", doc(t, para("가 나 다 라 마 바 사 아 자 차")), 1},
		{"spaces take a cell, spilling over", doc(t, para("가 나 다 라 마 바 사 아 자 차 ")), 2},
		{"an empty paragraph is not counted", doc(t, para(), para("가")), 1},
		{"a line break starts a line without the indent", doc(t, para(strings.Repeat("가", 19), strings.Repeat("나", 20))), 2},
		{"a line break after nothing adds no line", doc(t, para("", "가")), 1},
		{"a heading is a paragraph", doc(t, map[string]any{"type": "heading", "content": []any{map[string]any{"type": "text", "text": "1장"}}}), 1},
		{"paragraphs inside a quote count", doc(t, map[string]any{"type": "blockquote", "content": []any{para("가"), para("나")}}), 2},
		{"marked text runs on in one paragraph", doc(t, map[string]any{"type": "paragraph", "content": []any{
			map[string]any{"type": "text", "text": strings.Repeat("가", 10)},
			map[string]any{"type": "text", "text": strings.Repeat("나", 10), "marks": []any{map[string]any{"type": "bold"}}},
		}}), 2},
		{"a mention is not text, the same as Doc", doc(t, map[string]any{"type": "paragraph", "content": []any{
			map[string]any{"type": "mention", "attrs": map[string]any{"label": "리네타"}},
		}}), 0},
		{"a horizontal rule is not counted", doc(t, para("가"), map[string]any{"type": "horizontalRule"}, para("나")), 2},
		{"empty input", nil, 0},
		{"malformed input", []byte("{"), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ManuscriptLines(tc.raw); got != tc.want {
				t.Errorf("ManuscriptLines = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestManuscriptPages(t *testing.T) {
	for lines, want := range map[int]int{0: 0, -1: 0, 1: 1, 10: 1, 11: 2, 800: 80, 801: 81} {
		if got := ManuscriptPages(lines); got != want {
			t.Errorf("ManuscriptPages(%d) = %d, want %d", lines, got, want)
		}
	}
}
