package manuscriptedit

import (
	"strings"
	"testing"
)

// Replacing across a scene edits text and nothing else: a picture between the
// paragraphs comes through with its id and caption, even when the caption
// contains the word being replaced.
func TestReplaceDocTextKeepsIllustrations(t *testing.T) {
	doc := `{"type":"doc","content":[` +
		`{"type":"paragraph","content":[{"type":"text","text":"민호가 웃었다."}]},` +
		`{"type":"illustration","attrs":{"caption":"민호의 우산","id":"pic-1"}},` +
		`{"type":"paragraph","content":[{"type":"text","text":"민호는 갔다."}]}]}`
	out, err := replaceDocText(doc, "민호", "민준", true)
	if err != nil {
		t.Fatal(err)
	}
	if out.Occurrences != 2 {
		t.Fatalf("occurrences = %d, want 2 (the caption is not prose)", out.Occurrences)
	}
	if !strings.Contains(out.Doc, `{"attrs":{"caption":"민호의 우산","id":"pic-1"},"type":"illustration"}`) {
		t.Fatalf("the picture changed: %s", out.Doc)
	}
	if strings.Index(out.Doc, "민준가 웃었다") > strings.Index(out.Doc, "pic-1") || strings.Index(out.Doc, "pic-1") > strings.Index(out.Doc, "민준는 갔다") {
		t.Fatalf("the picture moved: %s", out.Doc)
	}
}
