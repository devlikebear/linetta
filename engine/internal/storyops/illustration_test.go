package storyops

import (
	"context"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/illustref"
)

type fakeIndex map[string]bool

func (f fakeIndex) IDsInProject(context.Context, string) (map[string]bool, error) { return f, nil }

func TestPlainTextToTiptapDocTurnsPlaceholderLinesIntoPictures(t *testing.T) {
	for name, text := range map[string]string{
		"own block":      "비가 내렸다.\n\n[[illustration:pic-1|우산]]\n\n그는 걸었다.",
		"inside a block": "비가 내렸다.\n[[illustration:pic-1|우산]]\n그는 걸었다.",
	} {
		t.Run(name, func(t *testing.T) {
			doc, err := PlainTextToTiptapDoc(text)
			if err != nil {
				t.Fatal(err)
			}
			want := `[{"type":"paragraph","content":[{"type":"text","text":"비가 내렸다."}]},` +
				`{"type":"illustration","attrs":{"caption":"우산","id":"pic-1"}},` +
				`{"type":"paragraph","content":[{"type":"text","text":"그는 걸었다."}]}]`
			if !strings.Contains(doc, want) {
				t.Fatalf("doc = %s", doc)
			}
		})
	}
	// A placeholder in the middle of a sentence is prose.
	doc, _ := PlainTextToTiptapDoc("그 장면은 [[illustration:pic-1]] 처럼 보였다.")
	if strings.Contains(doc, `"type":"illustration"`) {
		t.Fatalf("an inline mention became a picture: %s", doc)
	}
	// Hard breaks between ordinary lines survive.
	doc, _ = PlainTextToTiptapDoc("한 줄\n두 줄")
	if !strings.Contains(doc, `{"type":"hardBreak"}`) {
		t.Fatalf("hard break lost: %s", doc)
	}
}

// set_scene_text verifies what it wrote by reading it back; a placeholder
// written on a line next to prose comes back as its own block and must still
// count as the same text. Only the work's own pictures become pictures.
func TestApplyOpsSetSceneTextKeepsKnownIllustrations(t *testing.T) {
	svc, nodes, _, projectID, sceneID := newTestService(t)
	svc.WithIllustrations(fakeIndex{"pic-1": true})
	ctx := context.Background()

	res := svc.ApplyOps(ctx, projectID, sceneID, Proposal{Ops: []Op{{
		Type: "set_scene_text",
		Text: "비가 내렸다.\n[[illustration:pic-1|우산]]\n[[illustration:not-ours]]\n그는 걸었다.",
	}}}, now())
	if res.IsError() {
		t.Fatalf("failures = %+v", res.Failures)
	}
	got, _ := nodes.Get(ctx, sceneID)
	ids, err := illustref.IDs(*got.ContentDoc)
	if err != nil || len(ids) != 1 || ids[0] != "pic-1" {
		t.Fatalf("pictures = %v (%v) in %s", ids, err, *got.ContentDoc)
	}
	if !strings.Contains(*got.ContentDoc, "[[illustration:not-ours]]") {
		t.Fatalf("the unknown placeholder must stay as text: %s", *got.ContentDoc)
	}
}
