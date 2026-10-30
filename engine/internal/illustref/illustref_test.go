package illustref

import (
	"strings"
	"testing"
)

func TestPlaceholderRoundTrip(t *testing.T) {
	cases := []struct{ id, caption, line string }{
		{"a1b2-c3", "우산 아래", "[[illustration:a1b2-c3|우산 아래]]"},
		{"a1b2-c3", "", "[[illustration:a1b2-c3]]"},
		{"a1b2-c3", " two\nlines ]] here ", "[[illustration:a1b2-c3|two lines ] ] here]]"},
	}
	for _, tc := range cases {
		line := Placeholder(tc.id, tc.caption)
		if line != tc.line {
			t.Errorf("Placeholder(%q, %q) = %q, want %q", tc.id, tc.caption, line, tc.line)
		}
		id, caption, ok := Parse("  " + line + "  ")
		if !ok || id != tc.id {
			t.Errorf("Parse(%q) = %q %q %v", line, id, caption, ok)
		}
	}
	for _, line := range []string{
		"[[illustration:]]",
		"before [[illustration:abc]]",
		"[[illustration:abc]] after",
		"[[illustration:a b]]",
		"[[illustration:abc|x]",
		"[[image:abc]]",
	} {
		if _, _, ok := Parse(line); ok {
			t.Errorf("Parse(%q) accepted a line that is not exactly a placeholder", line)
		}
	}
}

const doc = `{"type":"doc","content":[` +
	`{"type":"paragraph","content":[{"type":"text","text":"비"}]},` +
	`{"type":"illustration","attrs":{"id":"old-1","caption":"우산"}},` +
	`{"type":"blockquote","content":[{"type":"illustration","attrs":{"id":"old-2","caption":""}}]},` +
	`{"type":"illustration","attrs":{"id":"old-1","caption":"again"}}]}`

func TestIDs(t *testing.T) {
	got, err := IDs(doc)
	if err != nil || strings.Join(got, ",") != "old-1,old-2" {
		t.Fatalf("IDs = %v, %v", got, err)
	}
	if got, err := IDs(`{"type":"doc","content":[{"type":"paragraph"}]}`); err != nil || got != nil {
		t.Fatalf("no illustrations: %v %v", got, err)
	}
	if _, err := IDs(`{"type":"illustration"`); err == nil {
		t.Fatal("broken JSON that names the node must be an error")
	}
}

func TestRewriteIDs(t *testing.T) {
	out, changed, err := RewriteIDs(doc, map[string]string{"old-1": "new-1"})
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	got, _ := IDs(out)
	if strings.Join(got, ",") != "new-1,old-2" {
		t.Fatalf("after rewrite: %v", got)
	}
	if !strings.Contains(out, `"caption":"우산"`) {
		t.Fatalf("caption lost: %s", out)
	}
	same, changed, err := RewriteIDs(doc, map[string]string{"other": "x"})
	if err != nil || changed || same != doc {
		t.Fatal("a map that touches nothing must return the document as it was")
	}
}

func TestKeepKnown(t *testing.T) {
	out, err := KeepKnown(doc, func(id string) bool { return id == "old-2" })
	if err != nil {
		t.Fatal(err)
	}
	got, _ := IDs(out)
	if strings.Join(got, ",") != "old-2" {
		t.Fatalf("kept %v, want only the known id", got)
	}
	if !strings.Contains(out, `"text":"[[illustration:old-1|우산]]"`) {
		t.Fatalf("an unknown id must come back as its placeholder text: %s", out)
	}
	if same, _ := KeepKnown(doc, func(string) bool { return true }); same != doc {
		t.Fatal("all known must leave the document as it was")
	}
}
