package export

import (
	"context"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/project"
)

func TestDocToPlainText_removesMarkdownAndKeepsMentionLabel(t *testing.T) {
	doc := []byte(`{"type":"doc","content":[
		{"type":"paragraph","content":[
			{"type":"text","text":"해진은 ","marks":[{"type":"bold"}]},
			{"type":"mention","attrs":{"id":"e1","label":"해진"}},
			{"type":"text","text":"을 봤다."}
		]},
		{"type":"paragraph","content":[{"type":"text","text":"다음 문단","marks":[{"type":"italic"}]}]}
	]}`)

	text, err := DocToPlainText(doc)
	if err != nil {
		t.Fatalf("DocToPlainText: %v", err)
	}
	if text != "해진은 해진을 봤다.\n\n다음 문단" {
		t.Fatalf("text = %q", text)
	}
}

func TestExportNodeText_leaf(t *testing.T) {
	_, pr, nr, _, _ := newExportFixture(t)
	ctx := context.Background()
	p, _ := pr.Create(ctx, 1, newProjectInput())
	if err := nr.UpdateContent(ctx, *p.LastOpenedNodeID,
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"문단 하나"}]}]}`, 100); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}

	out, err := ExportNodeText(ctx, nr, *p.LastOpenedNodeID)
	if err != nil {
		t.Fatalf("ExportNodeText: %v", err)
	}
	if out.Text != "문단 하나" {
		t.Fatalf("text = %q", out.Text)
	}
	if out.CharCount != 5 {
		t.Fatalf("char_count = %d, want 5", out.CharCount)
	}
}

func TestExportNodeText_containerSeparatesScenes(t *testing.T) {
	_, pr, nr, _, _ := newExportFixture(t)
	ctx := context.Background()
	p, _ := pr.Create(ctx, 1, newProjectInput())
	episode, err := nr.CreateSibling(ctx, *p.LastOpenedNodeID, "container", "1화", "", 10)
	if err != nil {
		t.Fatalf("CreateSibling: %v", err)
	}
	scene1, err := nr.CreateChild(ctx, episode.ID, "leaf", "씬 1", "", 20)
	if err != nil {
		t.Fatalf("CreateChild scene1: %v", err)
	}
	scene2, err := nr.CreateChild(ctx, episode.ID, "leaf", "씬 2", "", 30)
	if err != nil {
		t.Fatalf("CreateChild scene2: %v", err)
	}
	if err := nr.UpdateContent(ctx, scene1.ID,
		`{"type":"doc","content":[
			{"type":"paragraph","content":[{"type":"text","text":"첫 문단"}]},
			{"type":"paragraph","content":[{"type":"text","text":"둘째 문단"}]}
		]}`, 100); err != nil {
		t.Fatalf("UpdateContent scene1: %v", err)
	}
	if err := nr.UpdateContent(ctx, scene2.ID,
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"다른 씬"}]}]}`, 101); err != nil {
		t.Fatalf("UpdateContent scene2: %v", err)
	}

	out, err := ExportNodeText(ctx, nr, episode.ID)
	if err != nil {
		t.Fatalf("ExportNodeText: %v", err)
	}
	if out.Text != "첫 문단\n\n둘째 문단\n\n\n다른 씬" {
		t.Fatalf("text = %q", out.Text)
	}
	if out.CharCount != 13 {
		t.Fatalf("char_count = %d, want 13", out.CharCount)
	}
}

func newProjectInput() project.NewInput {
	return project.NewInput{
		Title:        "T",
		Genres:       []string{"판타지"},
		LengthTarget: "series",
		DefaultPOV:   "third_limited",
	}
}
