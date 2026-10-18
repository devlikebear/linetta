package export

import (
	"context"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/episode"
	"github.com/devlikebear/linetta/engine/internal/project"
)

func paragraphs(texts ...string) string {
	var blocks []string
	for _, t := range texts {
		blocks = append(blocks, `{"type":"paragraph","content":[{"type":"text","text":"`+t+`"}]}`)
	}
	return `{"type":"doc","content":[` + strings.Join(blocks, ",") + `]}`
}

func episodeFixture(t *testing.T, docs ...string) (episode.Episode, func() EpisodePayload) {
	t.Helper()
	_, pr, nr, _, _ := newExportFixture(t)
	ctx := context.Background()
	p, err := pr.Create(ctx, 1, project.NewInput{
		Title: "작품", Genres: []string{}, LengthTarget: "series", DefaultPOV: "first",
		OutlinePreset: project.OutlinePresetWebNovel,
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := nr.Get(ctx, *p.LastOpenedNodeID)
	if err != nil {
		t.Fatal(err)
	}
	// 1권 > 3화 (container) > one scene per doc.
	ep, err := nr.CreateSibling(ctx, first.ID, "container", "3화", "첫 출근", 2)
	if err != nil {
		t.Fatal(err)
	}
	for i, doc := range docs {
		scene, err := nr.CreateChild(ctx, ep.ID, "leaf", "씬", "", int64(3+i))
		if err != nil {
			t.Fatal(err)
		}
		if err := nr.UpdateContent(ctx, scene.ID, doc, int64(10+i)); err != nil {
			t.Fatal(err)
		}
	}
	resolved, err := episode.Resolve(ctx, nr, project.OutlinePresetWebNovel, ep.ID)
	if err != nil {
		t.Fatal(err)
	}
	return resolved, func() EpisodePayload {
		out, err := ExportEpisodeMarkdown(ctx, nr, resolved, p.Title)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
}

func TestExportEpisodeMarkdown_oneScene(t *testing.T) {
	_, export := episodeFixture(t, paragraphs("첫 문단", "둘째 문단"))
	got := export()
	want := "첫 문단\n\n둘째 문단"
	if got.Markdown != want {
		t.Fatalf("markdown = %q, want %q", got.Markdown, want)
	}
	if got.Title != "첫 출근" {
		t.Errorf("title = %q", got.Title)
	}
	if got.CharCount != 7 {
		t.Errorf("char count = %d, want 7", got.CharCount)
	}
	// What linetta-artworks' contentHash() gives for the same string.
	if got.ContentHash != "4540b219cb36b0edbd608b1602395ab1e60a741aba163922898e8eccd6616013" {
		t.Errorf("hash = %q", got.ContentHash)
	}
}

func TestExportEpisodeMarkdown_scenesJoinedByABreakAndEmptyScenesSkipped(t *testing.T) {
	_, export := episodeFixture(t,
		paragraphs("하나"),
		`{"type":"doc","content":[{"type":"paragraph"}]}`,
		paragraphs("둘"),
	)
	if got := export().Markdown; got != "하나\n\n* * *\n\n둘" {
		t.Fatalf("markdown = %q", got)
	}
}

func TestExportEpisodeMarkdown_dropsEditingMarks(t *testing.T) {
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[
	  {"type":"text","text":"오늘은 "},
	  {"type":"mention","attrs":{"id":"e1","label":"해진"},"marks":[{"type":"bold"}]},
	  {"type":"noteMarker","attrs":{"noteId":"n1"}},
	  {"type":"text","text":"이 웃었다."}
	]}]}`
	got := export(t, doc)
	if got != "오늘은 **해진**이 웃었다." {
		t.Fatalf("markdown = %q", got)
	}
	// The writer's own export keeps the mention as it always has.
	own, err := DocToMarkdown([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(own, "@해진") {
		t.Fatalf("DocToMarkdown changed: %q", own)
	}
}

func export(t *testing.T, doc string) string {
	t.Helper()
	_, run := episodeFixture(t, doc)
	return run().Markdown
}

func TestEpisodeTitle(t *testing.T) {
	cases := []struct {
		ep   episode.Episode
		want string
	}{
		{episode.Episode{Label: "3화", Title: " 첫 출근 "}, "첫 출근"},
		{episode.Episode{Label: "3화"}, ""},
		{episode.Episode{Label: "프롤로그"}, "프롤로그"},
		{episode.Episode{Label: "씬 1", Whole: true}, "작품"},
	}
	for _, c := range cases {
		if got := EpisodeTitle(c.ep, "작품"); got != c.want {
			t.Errorf("EpisodeTitle(%+v) = %q, want %q", c.ep, got, c.want)
		}
	}
}
