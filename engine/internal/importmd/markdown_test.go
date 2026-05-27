package importmd

import (
	"encoding/json"
	"testing"
)

func TestParseInlines_plainText(t *testing.T) {
	got := ParseInlines("hello world")
	if len(got) != 1 || got[0].Type != "text" || got[0].Text != "hello world" || len(got[0].Marks) != 0 {
		t.Fatalf("plain: %+v", got)
	}
}

func TestParseInlines_bold(t *testing.T) {
	got := ParseInlines("a **bold** c")
	if len(got) != 3 {
		t.Fatalf("len=%d %+v", len(got), got)
	}
	if got[0].Text != "a " || len(got[0].Marks) != 0 {
		t.Errorf("0: %+v", got[0])
	}
	if got[1].Text != "bold" || len(got[1].Marks) != 1 || got[1].Marks[0].Type != "bold" {
		t.Errorf("1: %+v", got[1])
	}
	if got[2].Text != " c" || len(got[2].Marks) != 0 {
		t.Errorf("2: %+v", got[2])
	}
}

func TestParseInlines_italic(t *testing.T) {
	got := ParseInlines("_em_")
	if len(got) != 1 || got[0].Text != "em" || len(got[0].Marks) != 1 || got[0].Marks[0].Type != "italic" {
		t.Fatalf("italic: %+v", got)
	}
}

func TestParseInlines_nestedBoldItalic(t *testing.T) {
	got := ParseInlines("**a _b_ c**")
	if len(got) != 3 {
		t.Fatalf("len=%d %+v", len(got), got)
	}
	// "a " bold
	if got[0].Text != "a " || len(got[0].Marks) != 1 || got[0].Marks[0].Type != "bold" {
		t.Errorf("0: %+v", got[0])
	}
	// "b" bold + italic
	if got[1].Text != "b" || len(got[1].Marks) != 2 {
		t.Errorf("1: %+v", got[1])
	}
	have := map[string]bool{}
	for _, m := range got[1].Marks {
		have[m.Type] = true
	}
	if !have["bold"] || !have["italic"] {
		t.Errorf("1 marks: %+v", got[1].Marks)
	}
	if got[2].Text != " c" || len(got[2].Marks) != 1 || got[2].Marks[0].Type != "bold" {
		t.Errorf("2: %+v", got[2])
	}
}

func TestParseInlines_hardBreak(t *testing.T) {
	// trailing two spaces → hardBreak; subsequent content NOT in this line (single-line input)
	got := ParseInlines("foo  ")
	if len(got) != 2 {
		t.Fatalf("len=%d %+v", len(got), got)
	}
	if got[0].Type != "text" || got[0].Text != "foo" {
		t.Errorf("0: %+v", got[0])
	}
	if got[1].Type != "hardBreak" {
		t.Errorf("1: %+v", got[1])
	}
}

func TestParseInlines_unmatchedMark(t *testing.T) {
	got := ParseInlines("a **b c")
	// unmatched ** should pass through as raw text
	// "a **b c" → single text
	if len(got) != 1 || got[0].Text != "a **b c" || len(got[0].Marks) != 0 {
		t.Fatalf("unmatched: %+v", got)
	}
}

func TestParseBlocks_paragraphsSplitOnBlankLines(t *testing.T) {
	got := ParseBlocks("first line\nsecond line\n\nsecond para")
	if len(got) != 2 {
		t.Fatalf("len=%d %+v", len(got), got)
	}
	if got[0].Type != "paragraph" {
		t.Errorf("0 type: %s", got[0].Type)
	}
	// Multi-line paragraph: lines joined with space (no hardBreak)
	// Content should contain "first line second line"
	if len(got[0].Content) == 0 {
		t.Errorf("0 content empty")
	}
	combined := ""
	for _, c := range got[0].Content {
		in, ok := c.(TiptapInline)
		if !ok {
			t.Fatalf("content not inline: %T", c)
		}
		if in.Type == "text" {
			combined += in.Text
		} else if in.Type == "hardBreak" {
			combined += "<HB>"
		}
	}
	if combined != "first line second line" {
		t.Errorf("0 combined = %q", combined)
	}
	// 2nd paragraph
	if got[1].Type != "paragraph" {
		t.Errorf("1 type: %s", got[1].Type)
	}
}

func TestParseBlocks_blockquote(t *testing.T) {
	got := ParseBlocks("> quoted line\n> still quoted")
	if len(got) != 1 {
		t.Fatalf("len=%d %+v", len(got), got)
	}
	if got[0].Type != "blockquote" {
		t.Fatalf("type: %s", got[0].Type)
	}
	// Inner should be one paragraph
	if len(got[0].Content) != 1 {
		t.Fatalf("inner content len=%d", len(got[0].Content))
	}
	innerBlock, ok := got[0].Content[0].(TiptapBlock)
	if !ok {
		t.Fatalf("inner not block: %T", got[0].Content[0])
	}
	if innerBlock.Type != "paragraph" {
		t.Errorf("inner type: %s", innerBlock.Type)
	}
}

func TestParseBlocks_empty(t *testing.T) {
	got := ParseBlocks("")
	if len(got) != 0 {
		t.Fatalf("expected empty: %+v", got)
	}
}

func TestParseBlocks_jsonMarshalSanity(t *testing.T) {
	got := ParseBlocks("a **b**")
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Should be valid JSON containing "paragraph" and "bold"
	s := string(raw)
	if len(s) == 0 {
		t.Fatalf("empty marshal")
	}
	if !contains(s, "paragraph") || !contains(s, "bold") {
		t.Errorf("missing keys: %s", s)
	}
}

func TestParseBlocks_CRLFNormalized(t *testing.T) {
	got := ParseBlocks("a\r\n\r\nb")
	if len(got) != 2 {
		t.Fatalf("crlf: %+v", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
