package export

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func newExportFixture(t *testing.T) (*store.Store, *project.Repo, *node.Repo, *entity.Repo, *relationship.Repo) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, project.NewRepo(s), node.NewRepo(s), entity.NewRepo(s), relationship.NewRepo(s)
}

func TestExportProject_buildsTreeWithHeadingsAndMetadataAppendix(t *testing.T) {
	_, pr, nr, er, rr := newExportFixture(t)
	ctx := context.Background()
	p, err := pr.Create(ctx, 1, project.NewInput{
		Title: "조용한 도시", Genres: []string{"문학"}, LengthTarget: "novella", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	preset := project.OutlinePresetWebNovel
	if _, err := pr.Update(ctx, 2, project.UpdateInput{ID: p.ID, OutlinePreset: &preset}); err != nil {
		t.Fatalf("set outline preset: %v", err)
	}
	// p auto-creates 씬 1 leaf at root. We add 1부 → 1장 → 씬 1·씬 2.
	bu1, err := nr.CreateSibling(ctx, *p.LastOpenedNodeID, "container", "1부", "", 10)
	if err != nil {
		t.Fatalf("create 1부: %v", err)
	}
	ch1, err := nr.CreateChild(ctx, bu1.ID, "container", "1장", "", 20)
	if err != nil {
		t.Fatalf("create 1장: %v", err)
	}
	scene1, err := nr.CreateChild(ctx, ch1.ID, "leaf", "씬 1", "해변에서", 30)
	if err != nil {
		t.Fatalf("create 씬 1: %v", err)
	}
	if err := nr.UpdateContent(ctx, scene1.ID,
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"파도 소리"}]}]}`, 100); err != nil {
		t.Fatalf("update scene1: %v", err)
	}
	scene2, err := nr.CreateChild(ctx, ch1.ID, "leaf", "씬 2", "", 40)
	if err != nil {
		t.Fatalf("create 씬 2: %v", err)
	}
	if err := nr.UpdateContent(ctx, scene2.ID,
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"두 번째 장면"}]}]}`, 101); err != nil {
		t.Fatalf("update scene2: %v", err)
	}

	character, _ := er.Create(ctx, 1, entity.NewInput{ProjectID: p.ID, Name: "해진", Role: "POV", Kind: "character"})
	place, _ := er.Create(ctx, 2, entity.NewInput{ProjectID: p.ID, Name: "항구", Role: "메인무대", Kind: "place"})
	_, _ = rr.CreateOne(ctx, relationship.NewInput{
		ProjectID: p.ID, FromID: character.ID, ToID: place.ID, Label: "거주지", Notes: "자주 머문다",
	})

	out, err := ExportProject(ctx, pr, nr, er, rr, p.ID, "")
	if err != nil {
		t.Fatalf("ExportProject: %v", err)
	}
	if !strings.Contains(out.Markdown, "linetta:\n") {
		t.Fatalf("missing linetta frontmatter; doc=\n%s", out.Markdown)
	}
	if !strings.Contains(out.Markdown, "outline_preset: webnovel\n") {
		t.Fatalf("missing outline preset metadata; doc=\n%s", out.Markdown)
	}
	if !strings.Contains(out.Markdown, "entities:\n") || !strings.Contains(out.Markdown, "relationships:\n") {
		t.Fatalf("missing metadata entities/relationships; doc=\n%s", out.Markdown)
	}
	if !strings.Contains(out.Markdown, "\n# 조용한 도시\n\n") {
		t.Errorf("missing project H1 prefix; got prefix %q", out.Markdown[:min(40, len(out.Markdown))])
	}
	if !strings.Contains(out.Markdown, "## 1부") {
		t.Errorf("missing 1부 heading; doc=\n%s", out.Markdown)
	}
	if !strings.Contains(out.Markdown, "### 1장") {
		t.Errorf("missing 1장 heading")
	}
	if !strings.Contains(out.Markdown, "### 씬 1 — 해변에서") {
		t.Errorf("missing scene 1 heading with title")
	}
	if !strings.Contains(out.Markdown, "파도 소리") {
		t.Errorf("missing scene body")
	}
	if !strings.Contains(out.Markdown, "## 등장인물") {
		t.Errorf("missing entities appendix")
	}
	if !strings.Contains(out.Markdown, "해진") || !strings.Contains(out.Markdown, "항구") {
		t.Errorf("missing entity name in appendix")
	}
	if !strings.Contains(out.Markdown, "## 관계") || !strings.Contains(out.Markdown, "거주지") {
		t.Errorf("missing relationship appendix")
	}
	if out.SuggestedFilename != "조용한-도시.md" {
		t.Errorf("filename = %q, want 조용한-도시.md", out.SuggestedFilename)
	}
}

func TestExportNode_returnsLeafBodyOnly(t *testing.T) {
	_, pr, nr, _, _ := newExportFixture(t)
	ctx := context.Background()
	p, _ := pr.Create(ctx, 1, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first",
	})
	_ = nr.UpdateContent(ctx, *p.LastOpenedNodeID,
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"단편"}]}]}`, 100)
	out, err := ExportNode(ctx, nr, *p.LastOpenedNodeID)
	if err != nil {
		t.Fatalf("ExportNode: %v", err)
	}
	if strings.Contains(out.Markdown, "#") {
		t.Errorf("node export should not contain headings; got:\n%s", out.Markdown)
	}
	if !strings.Contains(out.Markdown, "단편") {
		t.Errorf("missing body")
	}
	if out.SuggestedFilename != "씬-1.md" {
		t.Errorf("filename = %q", out.SuggestedFilename)
	}
}

func TestSyncFilename_keepsProjectsWithSameTitleDistinct(t *testing.T) {
	first := SyncFilename("Quiet City", "11111111-1111-1111-1111-111111111111")
	second := SyncFilename("Quiet City", "22222222-2222-2222-2222-222222222222")
	if first == second {
		t.Fatalf("sync filenames collided: %q", first)
	}
	if first != "quiet-city--11111111-1111-1111-1111-111111111111.md" {
		t.Fatalf("first filename = %q", first)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestExportProject_appendixHeadingsFollowTheReader(t *testing.T) {
	_, pr, nr, er, rr := newExportFixture(t)
	ctx := context.Background()
	p, err := pr.Create(ctx, 1, project.NewInput{Title: "Quiet City", LengthTarget: "novella", DefaultPOV: "first"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	a, _ := er.Create(ctx, 1, entity.NewInput{ProjectID: p.ID, Name: "Hae-jin", Kind: "character"})
	b, _ := er.Create(ctx, 2, entity.NewInput{ProjectID: p.ID, Name: "Harbour", Kind: "place"})
	if _, err := rr.CreateOne(ctx, relationship.NewInput{
		ProjectID: p.ID, FromID: a.ID, ToID: b.ID, Label: "lives in",
	}); err != nil {
		t.Fatalf("create relationship: %v", err)
	}

	// The bug: an English writer exporting their own novel found "## 등장인물"
	// in the middle of it, because the engine wrote the heading and the engine
	// has no idea who is reading.
	for _, tc := range []struct{ language, characters, relationships string }{
		{"en", "## Characters", "## Relationships"},
		{"ja", "## 登場人物", "## 関係"},
		{"ko", "## 등장인물", "## 관계"},
		{"", "## 등장인물", "## 관계"},
	} {
		out, err := ExportProject(ctx, pr, nr, er, rr, p.ID, tc.language)
		if err != nil {
			t.Fatalf("ExportProject(%q): %v", tc.language, err)
		}
		if !strings.Contains(out.Markdown, tc.characters) {
			t.Errorf("language %q: missing %q; doc=\n%s", tc.language, tc.characters, out.Markdown)
		}
		if !strings.Contains(out.Markdown, tc.relationships) {
			t.Errorf("language %q: missing %q", tc.language, tc.relationships)
		}
	}
}

func TestExportProject_translatesOnlyTheHeadings(t *testing.T) {
	_, pr, nr, er, rr := newExportFixture(t)
	ctx := context.Background()
	p, err := pr.Create(ctx, 1, project.NewInput{Title: "조용한 도시", LengthTarget: "novella", DefaultPOV: "first"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := er.Create(ctx, 1, entity.NewInput{
		ProjectID: p.ID, Name: "해진", Role: "주인공", Kind: "character",
	}); err != nil {
		t.Fatalf("create entity: %v", err)
	}

	out, err := ExportProject(ctx, pr, nr, er, rr, p.ID, "en")
	if err != nil {
		t.Fatalf("ExportProject: %v", err)
	}
	// What the writer typed is theirs. Only the section label is ours.
	for _, theirs := range []string{"조용한 도시", "해진", "주인공"} {
		if !strings.Contains(out.Markdown, theirs) {
			t.Errorf("English export dropped the writer's own %q; doc=\n%s", theirs, out.Markdown)
		}
	}
}
