package export

import (
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/illustref"
)

const withPicture = `{"type":"doc","content":[` +
	`{"type":"paragraph","content":[{"type":"text","text":"비가 내렸다."}]},` +
	`{"type":"illustration","attrs":{"id":"pic-1","caption":"우산"}},` +
	`{"type":"paragraph","content":[{"type":"text","text":"그는 걸었다."}]}]}`

// Until exports write image files, a picture is left out of every export
// rather than leaking a placeholder into the writer's files; only a caller
// that asks for it gets one.
func TestIllustrationIsOmittedFromExportsUnlessAsked(t *testing.T) {
	md, err := DocToMarkdown([]byte(withPicture))
	if err != nil {
		t.Fatal(err)
	}
	if md != "비가 내렸다.\n\n그는 걸었다.\n\n" {
		t.Fatalf("markdown = %q", md)
	}
	md, err = DocToMarkdownWith([]byte(withPicture), MarkdownOptions{Illustration: illustref.Placeholder})
	if err != nil {
		t.Fatal(err)
	}
	if md != "비가 내렸다.\n\n[[illustration:pic-1|우산]]\n\n그는 걸었다.\n\n" {
		t.Fatalf("markdown with placeholder = %q", md)
	}
	text, err := DocToPlainText([]byte(withPicture))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "illustration") || strings.Contains(text, "우산") || !strings.Contains(text, "그는 걸었다.") {
		t.Fatalf("plain text = %q", text)
	}
}

func TestExportEpisodeMarkdown_countsTheOmittedPictures(t *testing.T) {
	_, export := episodeFixture(t, withPicture, paragraphs("둘째 씬"))
	got := export()
	if got.IllustrationsOmitted != 1 {
		t.Fatalf("omitted = %d, want 1", got.IllustrationsOmitted)
	}
	if strings.Contains(got.Markdown, "illustration") {
		t.Fatalf("published text carries the picture: %q", got.Markdown)
	}
	_, plain := episodeFixture(t, paragraphs("그림 없음"))
	if plain().IllustrationsOmitted != 0 {
		t.Fatal("an episode without pictures reports some")
	}
}
