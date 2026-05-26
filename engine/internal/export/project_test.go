package export

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func newExportFixture(t *testing.T) (*store.Store, *project.Repo, *node.Repo, *entity.Repo) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, project.NewRepo(s), node.NewRepo(s), entity.NewRepo(s)
}

func TestExportProject_buildsTreeWithHeadingsAndEntitiesAppendix(t *testing.T) {
	_, pr, nr, er := newExportFixture(t)
	ctx := context.Background()
	p, err := pr.Create(ctx, 1, project.NewInput{
		Title: "조용한 도시", Genres: []string{"문학"}, LengthTarget: "novella", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
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

	_, _ = er.Create(ctx, 1, entity.NewInput{ProjectID: p.ID, Name: "해진", Role: "POV", Kind: "character"})

	out, err := ExportProject(ctx, pr, nr, er, p.ID)
	if err != nil {
		t.Fatalf("ExportProject: %v", err)
	}
	if !strings.HasPrefix(out.Markdown, "# 조용한 도시\n\n") {
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
	if !strings.Contains(out.Markdown, "해진") {
		t.Errorf("missing entity name in appendix")
	}
	if out.SuggestedFilename != "조용한-도시.md" {
		t.Errorf("filename = %q, want 조용한-도시.md", out.SuggestedFilename)
	}
}

func TestExportNode_returnsLeafBodyOnly(t *testing.T) {
	_, pr, nr, _ := newExportFixture(t)
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
