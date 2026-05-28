package importmd

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func setupRepos(t *testing.T) (*project.Repo, *node.Repo) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return project.NewRepo(s), node.NewRepo(s)
}

func TestBuildProject_emptyMarkdownKeepsSeed(t *testing.T) {
	pr, nr := setupRepos(t)
	ctx := context.Background()
	o := ParseOutline("")
	res, err := BuildProject(ctx, pr, nr, 1000, o, "Fallback")
	if err != nil {
		t.Fatalf("BuildProject: %v", err)
	}
	p := res.Project
	if p.Title != "Fallback" {
		t.Errorf("title: %q", p.Title)
	}
	nodes, err := nr.ListByProject(ctx, p.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node (seed), got %d", len(nodes))
	}
	if nodes[0].Label != "씬 1" || nodes[0].Kind != "leaf" {
		t.Errorf("seed: %+v", nodes[0])
	}
}

func TestBuildProject_fullTree(t *testing.T) {
	pr, nr := setupRepos(t)
	ctx := context.Background()
	src := "# Title\n## Part A\n### Chapter 1\n#### Scene 1\nbody one\n#### Scene 2\nbody two\n## Part B\n### Chapter 2\n#### Scene 3\nbody three\n"
	o := ParseOutline(src)
	res, err := BuildProject(ctx, pr, nr, 2000, o, "ignored")
	if err != nil {
		t.Fatalf("BuildProject: %v", err)
	}
	p := res.Project
	if p.Title != "Title" {
		t.Errorf("title: %q", p.Title)
	}
	nodes, err := nr.ListByProject(ctx, p.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// Expect: Part A (container), Part B (container), Chapter 1, Chapter 2,
	// Scene 1, Scene 2, Scene 3 = 7 nodes (seed deleted).
	if len(nodes) != 7 {
		t.Fatalf("expected 7 nodes, got %d: %+v", len(nodes), nodes)
	}
	labels := map[string]node.Node{}
	for _, n := range nodes {
		labels[n.Label] = n
	}
	for _, want := range []string{"Part A", "Part B", "Chapter 1", "Chapter 2", "Scene 1", "Scene 2", "Scene 3"} {
		if _, ok := labels[want]; !ok {
			t.Errorf("missing label: %q", want)
		}
	}
	// Scene 1 must contain "body one"
	s1 := labels["Scene 1"]
	if s1.ContentDoc == nil || !strings.Contains(*s1.ContentDoc, "body one") {
		t.Errorf("Scene 1 doc: %v", s1.ContentDoc)
	}
	// Containers should have nil ContentDoc
	pa := labels["Part A"]
	if pa.Kind != "container" {
		t.Errorf("Part A kind: %s", pa.Kind)
	}
	if pa.ContentDoc != nil {
		t.Errorf("Part A content_doc should be nil: %v", pa.ContentDoc)
	}
}

func TestBuildProject_H3BodyNoH4_leaf(t *testing.T) {
	pr, nr := setupRepos(t)
	ctx := context.Background()
	src := "# T\n## P\n### Chapter only\nbody body body\n"
	o := ParseOutline(src)
	res, err := BuildProject(ctx, pr, nr, 3000, o, "x")
	if err != nil {
		t.Fatalf("BuildProject: %v", err)
	}
	p := res.Project
	nodes, _ := nr.ListByProject(ctx, p.ID)
	// Expect 2 nodes: Part P (container), Chapter only (leaf with body).
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d: %+v", len(nodes), nodes)
	}
	var chapter node.Node
	found := false
	for _, n := range nodes {
		if n.Label == "Chapter only" {
			chapter = n
			found = true
		}
	}
	if !found {
		t.Fatalf("Chapter only not found")
	}
	if chapter.Kind != "leaf" {
		t.Errorf("kind: %s", chapter.Kind)
	}
	if chapter.ContentDoc == nil || !strings.Contains(*chapter.ContentDoc, "body body body") {
		t.Errorf("body: %v", chapter.ContentDoc)
	}
}

func TestBuildProject_H2WithBodyAndChildren_syntheticScene(t *testing.T) {
	pr, nr := setupRepos(t)
	ctx := context.Background()
	// H2 has both body lines AND H3 children; should emit synthetic 씬 1 leaf
	// under the H2 container with the body text.
	src := "# T\n## P\nintro body for part\n### Chapter 1\nchapter body\n"
	o := ParseOutline(src)
	res, err := BuildProject(ctx, pr, nr, 4000, o, "x")
	if err != nil {
		t.Fatalf("BuildProject: %v", err)
	}
	p := res.Project
	nodes, _ := nr.ListByProject(ctx, p.ID)
	// Expect: P (container), synthetic 씬 1 leaf under P (with intro), Chapter 1 leaf.
	var partP node.Node
	var synthetic node.Node
	var chapter node.Node
	for _, n := range nodes {
		switch n.Label {
		case "P":
			partP = n
		case "씬 1":
			synthetic = n
		case "Chapter 1":
			chapter = n
		}
	}
	if partP.ID == "" || synthetic.ID == "" || chapter.ID == "" {
		t.Fatalf("missing nodes: %+v", nodes)
	}
	if partP.Kind != "container" {
		t.Errorf("P kind: %s", partP.Kind)
	}
	if synthetic.ParentID == nil || *synthetic.ParentID != partP.ID {
		t.Errorf("synthetic parent: %v want %s", synthetic.ParentID, partP.ID)
	}
	if synthetic.Kind != "leaf" {
		t.Errorf("synthetic kind: %s", synthetic.Kind)
	}
	if synthetic.ContentDoc == nil || !strings.Contains(*synthetic.ContentDoc, "intro body for part") {
		t.Errorf("synthetic body: %v", synthetic.ContentDoc)
	}
	if chapter.ParentID == nil || *chapter.ParentID != partP.ID {
		t.Errorf("chapter parent: %v want %s", chapter.ParentID, partP.ID)
	}
	if chapter.ContentDoc == nil || !strings.Contains(*chapter.ContentDoc, "chapter body") {
		t.Errorf("chapter body: %v", chapter.ContentDoc)
	}
	if res.ContainerCount != 1 {
		t.Fatalf("ContainerCount=%d want 1", res.ContainerCount)
	}
	if res.LeafCount != 2 { // synthetic 씬 1 + Chapter 1
		t.Fatalf("LeafCount=%d want 2", res.LeafCount)
	}
}

func TestBuildProject_returnsCountsAndWarnings(t *testing.T) {
	pr, nr := setupRepos(t)
	ctx := context.Background()

	md := "# 작품\n" +
		"## 1부\n" +
		"### 1장\n" +
		"#### 씬 1\n본문 1\n" +
		"#### 씬 2\n본문 2\n" +
		"### 2장\n" +
		"#### 씬 1\n본문 3\n"
	out := ParseOutline(md)
	res, err := BuildProject(ctx, pr, nr, 1000, out, "fallback")
	if err != nil {
		t.Fatalf("BuildProject: %v", err)
	}
	if res.ContainerCount != 3 { // 1부, 1장, 2장
		t.Fatalf("ContainerCount=%d want 3", res.ContainerCount)
	}
	if res.LeafCount != 3 { // 씬 1, 씬 2, 씬 1
		t.Fatalf("LeafCount=%d want 3", res.LeafCount)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("Warnings=%v want empty", res.Warnings)
	}
}

func TestBuildProject_warnsWhenNoHeadings(t *testing.T) {
	pr, nr := setupRepos(t)
	ctx := context.Background()

	out := ParseOutline("그냥 본문만 있고 헤딩이 없음.")
	res, err := BuildProject(ctx, pr, nr, 1000, out, "fallback")
	if err != nil {
		t.Fatalf("BuildProject: %v", err)
	}
	if res.LeafCount != 0 || res.ContainerCount != 0 {
		t.Fatalf("counts: c=%d l=%d want both 0", res.ContainerCount, res.LeafCount)
	}
	if len(res.Warnings) == 0 {
		t.Fatal("want warning for no headings")
	}
}
