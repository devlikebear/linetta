package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func setupImportFixture(t *testing.T) (*project.Repo, *node.Repo) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return project.NewRepo(s), node.NewRepo(s)
}

func TestImportMarkdownHandler_createsProjectFromContent(t *testing.T) {
	pr, nr := setupImportFixture(t)
	h := ImportMarkdown(pr, nr, func() int64 { return 5000 })

	params := json.RawMessage(`{"file_name":"my-work.md","content":"# Imported Work\n## Part A\n### Chapter 1\n#### Scene 1\nhello\n"}`)
	res, err := h(context.Background(), params)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var out struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal(res, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.ProjectID == "" {
		t.Fatalf("no project id")
	}
	p, err := pr.Get(context.Background(), out.ProjectID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if p.Title != "Imported Work" {
		t.Errorf("title: %q", p.Title)
	}
	nodes, _ := nr.ListByProject(context.Background(), p.ID)
	if len(nodes) < 3 {
		t.Errorf("expected 3+ nodes, got %d", len(nodes))
	}
}

func TestImportMarkdown_resultIncludesCountsAndWarnings(t *testing.T) {
	pr, nr := setupImportFixture(t)
	h := ImportMarkdown(pr, nr, func() int64 { return 7000 })
	ctx := context.Background()

	md := "# 작품\n## 1부\n### 1장\n#### 씬 1\n본문\n"
	params, _ := json.Marshal(map[string]any{
		"file_name": "x.md",
		"content":   md,
	})
	raw, err := h(ctx, params)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got struct {
		ProjectID      string   `json:"project_id"`
		ContainerCount int      `json:"container_count"`
		LeafCount      int      `json:"leaf_count"`
		Warnings       []string `json:"warnings"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ProjectID == "" {
		t.Fatal("project_id empty")
	}
	if got.ContainerCount != 2 { // 1부, 1장
		t.Fatalf("ContainerCount=%d want 2", got.ContainerCount)
	}
	if got.LeafCount != 1 { // 씬 1
		t.Fatalf("LeafCount=%d want 1", got.LeafCount)
	}
	if len(got.Warnings) != 0 {
		t.Fatalf("warnings=%v want none", got.Warnings)
	}
}

func TestImportMarkdownHandler_fallbackTitleFromFileName(t *testing.T) {
	pr, nr := setupImportFixture(t)
	h := ImportMarkdown(pr, nr, func() int64 { return 6000 })

	// No H1 in content → title should come from file_name (stripped of .md).
	params := json.RawMessage(`{"file_name":"my-novel.md","content":"## Part A\n### Chapter\n#### Scene\nbody\n"}`)
	res, err := h(context.Background(), params)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var out struct {
		ProjectID string `json:"project_id"`
	}
	_ = json.Unmarshal(res, &out)
	p, err := pr.Get(context.Background(), out.ProjectID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if p.Title != "my-novel" {
		t.Errorf("title: %q", p.Title)
	}
}
