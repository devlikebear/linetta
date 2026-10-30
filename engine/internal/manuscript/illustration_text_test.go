package manuscript

import (
	"strings"
	"testing"
)

// The search index holds the prose only; a picture's caption is not text the
// writer wrote in the scene.
func TestDocToPlainTextLeavesIllustrationsOut(t *testing.T) {
	got := docToPlainText(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"비"}]},` +
		`{"type":"illustration","attrs":{"id":"pic-1","caption":"우산"}}]}`)
	if got != "비" || strings.Contains(got, "우산") {
		t.Fatalf("indexed text = %q", got)
	}
}
