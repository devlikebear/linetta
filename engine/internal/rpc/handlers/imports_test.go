package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/importmd"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
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

func setupImportFixtureFull(t *testing.T) (*project.Repo, *node.Repo, *entity.Repo, *relationship.Repo) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return project.NewRepo(s), node.NewRepo(s), entity.NewRepo(s), relationship.NewRepo(s)
}

func TestImportMarkdownHandler_createsProjectFromContent(t *testing.T) {
	pr, nr := setupImportFixture(t)
	h := ImportMarkdown(pr, nr, nil, nil, importmd.Extras{}, func() int64 { return 5000 })

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
	for _, n := range nodes {
		if !node.ValidKind(n.Kind) {
			t.Fatalf("import produced invalid node kind: %+v", n)
		}
	}
}

func TestImportMarkdownHandler_removesPartialProjectWhenMetadataRestoreFails(t *testing.T) {
	pr, nr, er, rr := setupImportFixtureFull(t)
	h := ImportMarkdown(pr, nr, er, rr, importmd.Extras{}, func() int64 { return 5000 })
	ctx := context.Background()
	md := "---\n" +
		"linetta:\n" +
		"  version: 1\n" +
		"  entities:\n" +
		"    - id: first\n" +
		"      name: 중복 이름\n" +
		"    - id: second\n" +
		"      name: 중복 이름\n" +
		"---\n\n" +
		"# 실패해야 하는 가져오기\n## 1부\n### 씬 1\n본문\n"
	params, _ := json.Marshal(map[string]any{"file_name": "broken.md", "content": md})

	if _, err := h(ctx, params); err == nil {
		t.Fatal("expected metadata restore failure")
	}
	projects, err := pr.List(ctx, project.ListFilter{IncludeArchived: true, Limit: 10})
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	if len(projects) != 0 {
		t.Fatalf("partial import remained after failure: %+v", projects)
	}
}

func TestImportMarkdownHandler_restoresLinettaMetadataWithoutAppendixNodes(t *testing.T) {
	pr, nr, er, rr := setupImportFixtureFull(t)
	h := ImportMarkdown(pr, nr, er, rr, importmd.Extras{}, func() int64 { return 5000 })
	ctx := context.Background()

	md := "---\n" +
		"linetta:\n" +
		"  version: 1\n" +
		"  entities:\n" +
		"    - id: char-1\n" +
		"      kind: character\n" +
		"      name: 해진\n" +
		"      role: 주인공\n" +
		"      summary: 사진작가\n" +
		"    - id: place-1\n" +
		"      kind: place\n" +
		"      name: 항구\n" +
		"      role: 메인무대\n" +
		"      summary: 오래된 부두\n" +
		"  relationships:\n" +
		"    - from_id: char-1\n" +
		"      to_id: place-1\n" +
		"      label: 거주지\n" +
		"      notes: 자주 머문다\n" +
		"---\n\n" +
		"# 복원작\n" +
		"## 1부\n" +
		"### 1장\n" +
		"#### 씬 1\n본문\n\n" +
		"## 등장인물\n\n" +
		"- **해진** (character) · 주인공 — 사진작가\n" +
		"- **항구** (place) · 메인무대 — 오래된 부두\n"
	params, _ := json.Marshal(map[string]any{
		"file_name": "roundtrip.md",
		"content":   md,
	})
	raw, err := h(ctx, params)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var out struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	nodes, err := nr.ListByProject(ctx, out.ProjectID)
	if err != nil {
		t.Fatalf("nodes: %v", err)
	}
	for _, n := range nodes {
		if n.Label == "등장인물" {
			t.Fatalf("metadata appendix restored as a node: %+v", nodes)
		}
	}
	ents, err := er.ListByProject(ctx, out.ProjectID)
	if err != nil {
		t.Fatalf("entities: %v", err)
	}
	if len(ents) != 2 {
		t.Fatalf("entities=%d want 2: %+v", len(ents), ents)
	}
	var foundPlace bool
	for _, e := range ents {
		if e.Name == "항구" && e.Kind == entity.KindPlace && e.Summary == "오래된 부두" {
			foundPlace = true
		}
	}
	if !foundPlace {
		t.Fatalf("place entity not restored: %+v", ents)
	}
	rels, err := rr.ListByProject(ctx, out.ProjectID)
	if err != nil {
		t.Fatalf("relationships: %v", err)
	}
	if len(rels) != 1 || rels[0].Label != "거주지" || rels[0].Notes != "자주 머문다" {
		t.Fatalf("relationship not restored: %+v", rels)
	}
}

func TestImportMarkdownHandler_restoresRelationshipPairs(t *testing.T) {
	pr, nr, er, rr := setupImportFixtureFull(t)
	h := ImportMarkdown(pr, nr, er, rr, importmd.Extras{}, func() int64 { return 5000 })
	ctx := context.Background()

	md := "---\n" +
		"linetta:\n" +
		"  version: 1\n" +
		"  entities:\n" +
		"    - id: a\n" +
		"      kind: character\n" +
		"      name: 해진\n" +
		"    - id: b\n" +
		"      kind: character\n" +
		"      name: 아지\n" +
		"    - id: c\n" +
		"      kind: place\n" +
		"      name: 등대\n" +
		"  relationships:\n" +
		"    - pair_id: pair-1\n" +
		"      from_id: a\n" +
		"      to_id: b\n" +
		"      label: 보호자\n" +
		"      notes: 정서적 보호자\n" +
		"    - pair_id: pair-1\n" +
		"      from_id: b\n" +
		"      to_id: a\n" +
		"      label: 피보호자\n" +
		"    - from_id: a\n" +
		"      to_id: c\n" +
		"      label: 자주 찾는 곳\n" +
		"---\n\n" +
		"# 복원작\n" +
		"## 1부\n" +
		"### 씬 1\n본문\n"
	params, _ := json.Marshal(map[string]any{
		"file_name": "pairs.md",
		"content":   md,
	})
	raw, err := h(ctx, params)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var out struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	rels, err := rr.ListByProject(ctx, out.ProjectID)
	if err != nil {
		t.Fatalf("relationships: %v", err)
	}
	if len(rels) != 3 {
		t.Fatalf("rels=%d want 3: %+v", len(rels), rels)
	}
	paired := []relationship.Relationship{}
	var foundSingle bool
	for _, rel := range rels {
		if rel.PairID != nil {
			paired = append(paired, rel)
		}
		if rel.Label == "자주 찾는 곳" {
			foundSingle = true
		}
	}
	if len(paired) != 2 || *paired[0].PairID != *paired[1].PairID {
		t.Fatalf("pair id not restored: %+v", rels)
	}
	if !foundSingle {
		t.Fatalf("single relationship not restored after pair: %+v", rels)
	}
}

func TestImportMarkdownHandler_restoresOutlinePreset(t *testing.T) {
	pr, nr := setupImportFixture(t)
	h := ImportMarkdown(pr, nr, nil, nil, importmd.Extras{}, func() int64 { return 5000 })
	ctx := context.Background()

	md := "---\n" +
		"linetta:\n" +
		"  version: 1\n" +
		"  outline_preset: webnovel\n" +
		"---\n\n" +
		"# 웹소설\n" +
		"## 1권\n" +
		"### 1화\n" +
		"#### 씬 1\n본문\n"
	params, _ := json.Marshal(map[string]any{
		"file_name": "webnovel.md",
		"content":   md,
	})
	raw, err := h(ctx, params)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var out struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	p, err := pr.Get(ctx, out.ProjectID)
	if err != nil {
		t.Fatalf("get project: %v", err)
	}
	if p.OutlinePreset != project.OutlinePresetWebNovel {
		t.Fatalf("outline_preset=%q want %q", p.OutlinePreset, project.OutlinePresetWebNovel)
	}
}

func TestImportMarkdown_resultIncludesCountsAndWarnings(t *testing.T) {
	pr, nr := setupImportFixture(t)
	h := ImportMarkdown(pr, nr, nil, nil, importmd.Extras{}, func() int64 { return 7000 })
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
	if strings.Contains(string(raw), `"warnings":null`) {
		t.Fatalf("warnings should serialize as [] not null, got %s", raw)
	}
}

func TestImportMarkdownHandler_fallbackTitleFromFileName(t *testing.T) {
	pr, nr := setupImportFixture(t)
	h := ImportMarkdown(pr, nr, nil, nil, importmd.Extras{}, func() int64 { return 6000 })

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

func TestImportPreview_returnsTreeNoDBWrite(t *testing.T) {
	ctx := context.Background()
	pr, _ := setupImportFixture(t)

	h := ImportPreview()

	md := "# 작품\n## 1부\n### 1장\n#### 씬 1\n본문\n"
	params, _ := json.Marshal(map[string]any{
		"file_name": "novel.md",
		"content":   md,
	})
	raw, err := h(ctx, params)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got struct {
		Title          string   `json:"title"`
		ContainerCount int      `json:"container_count"`
		LeafCount      int      `json:"leaf_count"`
		Warnings       []string `json:"warnings"`
		Roots          []any    `json:"roots"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Title != "작품" {
		t.Fatalf("title=%q", got.Title)
	}
	if got.ContainerCount != 2 || got.LeafCount != 1 {
		t.Fatalf("counts: c=%d l=%d", got.ContainerCount, got.LeafCount)
	}
	if len(got.Roots) != 1 {
		t.Fatalf("roots len=%d", len(got.Roots))
	}

	// Confirm no project rows created — Preview must be read-only.
	projects, err := pr.List(ctx, project.ListFilter{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 0 {
		t.Fatalf("preview must not write DB, found %d projects", len(projects))
	}

	// JSON shape stability: warnings should be [] not null, roots should be [...] not null.
	rawStr := string(raw)
	if strings.Contains(rawStr, `"warnings":null`) {
		t.Fatalf("warnings should be [] not null, got %s", rawStr)
	}
	if strings.Contains(rawStr, `"roots":null`) {
		t.Fatalf("roots should be [...] not null, got %s", rawStr)
	}
}
