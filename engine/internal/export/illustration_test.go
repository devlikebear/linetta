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

const twoPictures = `{"type":"doc","content":[` +
	`{"type":"illustration","attrs":{"id":"pic-2","caption":"노란 [우산]"}},` +
	`{"type":"paragraph","content":[{"type":"text","text":"밤이 왔다."}]},` +
	`{"type":"illustration","attrs":{"id":"pic-1","caption":"우산"}},` +
	`{"type":"illustration","attrs":{"id":"gone","caption":"지운 그림"}}]}`

// Published text shows each gallery picture where the scene does, and lists
// the pictures for upload in first-use order, each once.
func TestExportEpisodeMarkdown_showsGalleryPictures(t *testing.T) {
	known := map[string]bool{"pic-1": true, "pic-2": true}
	_, export := episodeFixtureKnowing(t, known, withPicture, twoPictures)
	got := export()
	want := "비가 내렸다.\n\n![우산](illustration:pic-1)\n\n그는 걸었다." + SceneBreak +
		"![노란 \\[우산\\]](illustration:pic-2)\n\n밤이 왔다.\n\n![우산](illustration:pic-1)"
	if got.Markdown != want {
		t.Fatalf("markdown =\n%q\nwant\n%q", got.Markdown, want)
	}
	if strings.Join(got.Illustrations, ",") != "pic-1,pic-2" {
		t.Fatalf("illustrations = %v", got.Illustrations)
	}
	if got.ContentHash != ContentHash(want) {
		t.Fatal("the hash is not of the text with its pictures")
	}
}

// Without the gallery (or for pictures it no longer has) the text is what it
// was before pictures were published.
func TestExportEpisodeMarkdown_leavesOutUnknownPictures(t *testing.T) {
	_, export := episodeFixture(t, withPicture, paragraphs("둘째 씬"))
	got := export()
	if strings.Contains(got.Markdown, "illustration") || len(got.Illustrations) != 0 {
		t.Fatalf("published text carries an unknown picture: %q %v", got.Markdown, got.Illustrations)
	}
	_, plain := episodeFixture(t, paragraphs("그림 없음"))
	if len(plain().Illustrations) != 0 {
		t.Fatal("an episode without pictures lists some")
	}
}
