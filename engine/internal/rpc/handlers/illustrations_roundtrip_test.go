package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/export"
	"github.com/devlikebear/linetta/engine/internal/illustration"
	"github.com/devlikebear/linetta/engine/internal/illustref"
	"github.com/devlikebear/linetta/engine/internal/importmd"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/store"
)

type galleryImages struct{ repo *illustration.Repo }

func (g galleryImages) IllustrationImage(ctx context.Context, id string) (string, []byte, error) {
	img, err := g.repo.Get(ctx, id)
	if errors.Is(err, illustration.ErrNotFound) {
		return "", nil, export.ErrIllustrationMissing
	}
	if err != nil {
		return "", nil, err
	}
	return img.MIME, img.Data, nil
}

type pictureLibrary struct {
	projects      *project.Repo
	nodes         *node.Repo
	entities      *entity.Repo
	relationships *relationship.Repo
	gallery       *illustration.Repo
}

func openPictureLibrary(t *testing.T, name string) pictureLibrary {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return pictureLibrary{
		projects: project.NewRepo(st), nodes: node.NewRepo(st),
		entities: entity.NewRepo(st), relationships: relationship.NewRepo(st),
		gallery: illustration.NewRepo(st),
	}
}

func (l pictureLibrary) importMarkdown(t *testing.T, md string) (string, []string) {
	t.Helper()
	params, _ := json.Marshal(map[string]string{"file_name": "work.md", "content": md})
	raw, err := ImportMarkdown(l.projects, l.nodes, l.entities, l.relationships, importmd.Extras{}, l.gallery, func() int64 { return 9000 })(context.Background(), params)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	var res struct {
		ProjectID string   `json:"project_id"`
		Warnings  []string `json:"warnings"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatal(err)
	}
	return res.ProjectID, res.Warnings
}

func (l pictureLibrary) sceneDoc(t *testing.T, projectID string) string {
	t.Helper()
	nodes, err := l.nodes.ListByProject(context.Background(), projectID)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range nodes {
		if n.Kind == node.KindLeaf && n.ContentDoc != nil {
			return *n.ContentDoc
		}
	}
	t.Fatal("no scene")
	return ""
}

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// A work exported with its pictures comes back with them: the pictures are in
// the new work's gallery and the scene shows them where it did.
func TestExportImportRoundTrip_keepsPictures(t *testing.T) {
	ctx := context.Background()
	src := openPictureLibrary(t, "src.db")
	p, err := src.projects.Create(ctx, 1, project.NewInput{Title: "비 오는 밤", LengthTarget: "short", DefaultPOV: "first"})
	if err != nil {
		t.Fatal(err)
	}
	pic, err := src.gallery.Add(ctx, 1, illustration.NewInput{ProjectID: p.ID, Caption: "우산", Data: tinyPNG(t)})
	if err != nil {
		t.Fatal(err)
	}
	doc := fmt.Sprintf(`{"type":"doc","content":[`+
		`{"type":"paragraph","content":[{"type":"text","text":"비가 내렸다."}]},`+
		`{"type":"illustration","attrs":{"id":%q,"caption":"노란 [우산]"}},`+
		`{"type":"paragraph","content":[{"type":"text","text":"그는 걸었다."}]}]}`, pic.ID)
	if err := src.nodes.UpdateContent(ctx, *p.LastOpenedNodeID, doc, 2); err != nil {
		t.Fatal(err)
	}
	out, err := export.ExportProject(ctx, export.Sources{
		Projects: src.projects, Nodes: src.nodes, Entities: src.entities, Relationships: src.relationships,
		Illustrations: galleryImages{src.gallery},
	}, p.ID, "ko")
	if err != nil {
		t.Fatal(err)
	}

	dst := openPictureLibrary(t, "dst.db")
	projectID, warnings := dst.importMarkdown(t, out.Markdown)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	pics, err := dst.gallery.ListByProject(ctx, projectID)
	if err != nil || len(pics) != 1 {
		t.Fatalf("gallery = %+v, %v", pics, err)
	}
	got, err := dst.gallery.Get(ctx, pics[0].ID)
	if err != nil || !bytes.Equal(got.Data, tinyPNG(t)) {
		t.Fatalf("picture bytes changed: %v", err)
	}
	scene := dst.sceneDoc(t, projectID)
	ids, _ := illustref.IDs(scene)
	if len(ids) != 1 || ids[0] != pics[0].ID {
		t.Fatalf("scene shows %v, gallery has %s:\n%s", ids, pics[0].ID, scene)
	}
	if !strings.Contains(scene, `"caption":"노란 [우산]"`) {
		t.Fatalf("caption lost:\n%s", scene)
	}
	if strings.Index(scene, "비가 내렸다.") > strings.Index(scene, pics[0].ID) || strings.Index(scene, pics[0].ID) > strings.Index(scene, "그는 걸었다.") {
		t.Fatalf("picture moved:\n%s", scene)
	}
}

// A reference the file does not define, and a picture the gallery refuses,
// stay in the scene as placeholder lines and are warned about.
func TestImportMarkdown_picturesItCannotRestore(t *testing.T) {
	dst := openPictureLibrary(t, "dst.db")
	md := "# 작품\n\n## 씬 1\n\n![없는 그림][illustration-gone]\n\n![글자][illustration-text]\n\n" +
		"[illustration-text]: data:image/png;base64,aGVsbG8=\n"
	projectID, warnings := dst.importMarkdown(t, md)
	if strings.Join(warnings, ",") != importmd.WarnIllustrationsMissing+","+importmd.WarnIllustrationsRejected {
		t.Fatalf("warnings = %v", warnings)
	}
	scene := dst.sceneDoc(t, projectID)
	if ids, _ := illustref.IDs(scene); len(ids) != 0 {
		t.Fatalf("an unrestored picture is still a picture: %s", scene)
	}
	for _, want := range []string{"[[illustration:gone|없는 그림]]", "[[illustration:text|글자]]"} {
		if !strings.Contains(scene, want) {
			t.Fatalf("missing placeholder %q:\n%s", want, scene)
		}
	}
	if pics, _ := dst.gallery.ListByProject(context.Background(), projectID); len(pics) != 0 {
		t.Fatalf("gallery = %+v", pics)
	}
}
