package export

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/project"
)

type fakeImages map[string][]byte

func (f fakeImages) IllustrationImage(_ context.Context, id string) (string, []byte, error) {
	data, ok := f[id]
	if !ok {
		return "", nil, fmt.Errorf("wrapped: %w", ErrIllustrationMissing)
	}
	return "image/jpeg", data, nil
}

func pictureDoc(parts ...string) string {
	var blocks []string
	for _, p := range parts {
		if id, ok := strings.CutPrefix(p, "pic:"); ok {
			id, caption, _ := strings.Cut(id, "|")
			blocks = append(blocks, fmt.Sprintf(`{"type":"illustration","attrs":{"id":%q,"caption":%q}}`, id, caption))
			continue
		}
		blocks = append(blocks, `{"type":"paragraph","content":[{"type":"text","text":"`+p+`"}]}`)
	}
	return `{"type":"doc","content":[` + strings.Join(blocks, ",") + `]}`
}

// One scene, two pictures (one used twice, one deleted from the gallery).
func exportWithPictures(t *testing.T, imageDir string) Payload {
	t.Helper()
	_, pr, nr, er, rr := newExportFixture(t)
	ctx := context.Background()
	p, err := pr.Create(ctx, 1, project.NewInput{Title: "비 오는 밤", LengthTarget: "short", DefaultPOV: "first"})
	if err != nil {
		t.Fatal(err)
	}
	doc := pictureDoc("비가 내렸다.", "pic:a|우산 [노란] 하나", "그는 걸었다.", "pic:gone|x", "pic:a|again")
	if err := nr.UpdateContent(ctx, *p.LastOpenedNodeID, doc, 2); err != nil {
		t.Fatal(err)
	}
	out, err := ExportProject(ctx, Sources{
		Projects: pr, Nodes: nr, Entities: er, Relationships: rr,
		Illustrations: fakeImages{"a": []byte("JPEGDATA")},
		ImageDir:      imageDir,
	}, p.ID, "ko")
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestExportProject_embedsPicturesOnceAtTheEnd(t *testing.T) {
	out := exportWithPictures(t, "")
	md := out.Markdown
	ref := `![우산 \[노란\] 하나][illustration-a]`
	if !strings.Contains(md, "비가 내렸다.\n\n"+ref+"\n\n그는 걸었다.") {
		t.Fatalf("picture not in place:\n%s", md)
	}
	if strings.Contains(md, "illustration-gone") {
		t.Fatalf("a deleted picture left a reference:\n%s", md)
	}
	def := "[illustration-a]: data:image/jpeg;base64," + base64.StdEncoding.EncodeToString([]byte("JPEGDATA"))
	if strings.Count(md, def) != 1 {
		t.Fatalf("want exactly one definition %q:\n%s", def, md)
	}
	if strings.Index(md, def) < strings.LastIndex(md, "그는 걸었다.") {
		t.Fatal("the definition must come after the manuscript")
	}
	if len(out.Images) != 0 {
		t.Fatal("an embedded export returns no files")
	}
}

func TestExportProject_linksPicturesToFilesForSync(t *testing.T) {
	out := exportWithPictures(t, "work.images")
	if !strings.Contains(out.Markdown, `![우산 \[노란\] 하나](work.images/a.jpg)`) {
		t.Fatalf("no file link:\n%s", out.Markdown)
	}
	if strings.Contains(out.Markdown, "base64") {
		t.Fatal("a sync export must not embed bytes")
	}
	if len(out.Images) != 1 || out.Images[0].Name != "a.jpg" || string(out.Images[0].Data) != "JPEGDATA" {
		t.Fatalf("files = %+v", out.Images)
	}
}

func TestExportProject_withoutASourceLeavesPicturesOut(t *testing.T) {
	_, pr, nr, er, rr := newExportFixture(t)
	ctx := context.Background()
	p, _ := pr.Create(ctx, 1, project.NewInput{Title: "t", LengthTarget: "short", DefaultPOV: "first"})
	_ = nr.UpdateContent(ctx, *p.LastOpenedNodeID, pictureDoc("비", "pic:a|우산"), 2)
	out, err := ExportProject(ctx, Sources{Projects: pr, Nodes: nr, Entities: er, Relationships: rr}, p.ID, "ko")
	if err != nil || strings.Contains(out.Markdown, "illustration") || strings.Contains(out.Markdown, "우산") {
		t.Fatalf("markdown = %q, %v", out.Markdown, err)
	}
}

func TestExportNode_embedsTheScenesPictures(t *testing.T) {
	_, pr, nr, _, _ := newExportFixture(t)
	ctx := context.Background()
	p, _ := pr.Create(ctx, 1, project.NewInput{Title: "t", LengthTarget: "short", DefaultPOV: "first"})
	_ = nr.UpdateContent(ctx, *p.LastOpenedNodeID, pictureDoc("비", "pic:a|우산"), 2)
	out, err := ExportNode(ctx, nr, *p.LastOpenedNodeID, fakeImages{"a": []byte("J")})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Markdown, "![우산][illustration-a]") || !strings.Contains(out.Markdown, "[illustration-a]: data:image/jpeg;base64,Sg==") {
		t.Fatalf("markdown = %q", out.Markdown)
	}
}

func TestWriteImageDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "work.images")
	if err := WriteImageDir(dir, []ImageFile{{Name: "a.jpg", Data: []byte("A")}, {Name: "b.jpg", Data: []byte("B")}}); err != nil {
		t.Fatal(err)
	}
	stamp := func(name string) int64 {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return info.ModTime().UnixNano()
	}
	// A second sync with a.jpg still used and b.jpg dropped: a.jpg is not
	// rewritten, b.jpg goes. The old timestamp shows a rewrite if one happens.
	old := time.Unix(1, 0)
	if err := os.Chtimes(filepath.Join(dir, "a.jpg"), old, old); err != nil {
		t.Fatal(err)
	}
	before := stamp("a.jpg")
	if err := WriteImageDir(dir, []ImageFile{{Name: "a.jpg", Data: []byte("A")}}); err != nil {
		t.Fatal(err)
	}
	if stamp("a.jpg") != before {
		t.Fatal("an unchanged picture was rewritten")
	}
	if _, err := os.Stat(filepath.Join(dir, "b.jpg")); !os.IsNotExist(err) {
		t.Fatalf("an unused picture stayed: %v", err)
	}
	// No pictures left: the folder goes too.
	if err := WriteImageDir(dir, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("the empty folder stayed: %v", err)
	}
	// And a work that never had pictures makes no folder.
	if err := WriteImageDir(filepath.Join(t.TempDir(), "none.images"), nil); err != nil {
		t.Fatal(err)
	}
}
