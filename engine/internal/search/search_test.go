package search

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

type fixture struct {
	store    *store.Store
	projects *project.Repo
	nodes    *node.Repo
	project  project.Project
	nodeID   string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	projects := project.NewRepo(s)
	p, err := projects.Create(ctx, 1_000, project.NewInput{
		Title:        "바다 도시",
		Genres:       []string{"mystery"},
		LengthTarget: project.LengthShort,
		DefaultPOV:   project.POVFirst,
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	return fixture{
		store:    s,
		projects: projects,
		nodes:    node.NewRepo(s),
		project:  p,
		nodeID:   *p.LastOpenedNodeID,
	}
}

func tiptapDoc(text string) string {
	return `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + text + `"}]}]}`
}

func TestQueryFindsNodeContentWithPreview(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	if err := f.nodes.Rename(ctx, f.nodeID, "씬 1", "정박지", 1_100); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if err := f.nodes.UpdateContent(ctx, f.nodeID, tiptapDoc("푸른 항구의 단서를 숨겼다."), 1_200); err != nil {
		t.Fatalf("update content: %v", err)
	}

	results, err := NewRepo(f.store).Query(ctx, "단서", 10)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len = %d, want 1: %#v", len(results), results)
	}
	got := results[0]
	if got.ProjectID != f.project.ID || got.ProjectTitle != "바다 도시" {
		t.Errorf("project mismatch: %#v", got)
	}
	if got.NodeID != f.nodeID || got.NodeTitle != "정박지" || got.NodeLabel != "씬 1" {
		t.Errorf("node mismatch: %#v", got)
	}
	if !strings.Contains(got.Preview, "단서") {
		t.Errorf("preview = %q, want query text", got.Preview)
	}
}

func TestQueryFindsTitlesAndSkipsArchivedProjects(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	if err := f.nodes.Rename(ctx, f.nodeID, "프롤로그", "붉은 등대", 1_100); err != nil {
		t.Fatalf("rename: %v", err)
	}

	results, err := NewRepo(f.store).Query(ctx, "등대", 10)
	if err != nil {
		t.Fatalf("query title: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len = %d, want 1", len(results))
	}

	if err := f.projects.Archive(ctx, f.project.ID, 1_300); err != nil {
		t.Fatalf("archive: %v", err)
	}
	results, err = NewRepo(f.store).Query(ctx, "등대", 10)
	if err != nil {
		t.Fatalf("query archived: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("len = %d, want archived projects hidden", len(results))
	}
}
