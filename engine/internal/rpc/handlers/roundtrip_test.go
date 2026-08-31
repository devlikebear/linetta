package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/export"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/importmd"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/note"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/ptrutil"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

type library struct {
	projects      *project.Repo
	nodes         *node.Repo
	entities      *entity.Repo
	relationships *relationship.Repo
	threads       *thread.Repo
	beats         *beat.Repo
	notes         *note.Repo
	facts         *fact.Repo
}

func openLibrary(t *testing.T, name string) library {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), name))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return library{
		projects:      project.NewRepo(st),
		nodes:         node.NewRepo(st),
		entities:      entity.NewRepo(st),
		relationships: relationship.NewRepo(st),
		threads:       thread.NewRepo(st),
		beats:         beat.NewRepo(st),
		notes:         note.NewRepo(st),
		facts:         fact.NewRepo(st),
	}
}

func (l library) exportSources() export.Sources {
	return export.Sources{
		Projects:      l.projects,
		Nodes:         l.nodes,
		Entities:      l.entities,
		Relationships: l.relationships,
		Extras:        export.Extras{Threads: l.threads, Beats: l.beats, Notes: l.notes, Facts: l.facts},
	}
}

func (l library) importExtras() importmd.Extras {
	return importmd.Extras{Threads: l.threads, Beats: l.beats, Notes: l.notes, Facts: l.facts}
}

// TestExportImportRoundTripCompleteness is the #83 acceptance test: a project
// carrying every kind of metadata — synopsis, plot outline, style notes, node
// status/summary/title, threads with beats, margin notes, fact cards — must
// survive export → import into a fresh library without loss.
func TestExportImportRoundTripCompleteness(t *testing.T) {
	ctx := context.Background()
	now := int64(1000)
	src := openLibrary(t, "src.db")

	p, err := src.projects.Create(ctx, now, project.NewInput{
		Title:        "완전한 작품",
		Genres:       []string{"fantasy", "mystery"},
		LengthTarget: "novella",
		DefaultPOV:   "third_limited",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := src.projects.Update(ctx, now, project.UpdateInput{
		ID:                p.ID,
		StyleNotes:        ptrutil.To("건조한 문체"),
		Synopsis:          ptrutil.To("주인공이 잃어버린 도시를 찾는 이야기."),
		Outline:           ptrutil.To("1부: 출발\n2부: 귀환"),
		EpisodeCharTarget: ptrutil.To(4200),
	}); err != nil {
		t.Fatalf("update project: %v", err)
	}

	// Outline: 프롤로그(leaf, titled) → 1장(container) → [씬 A, 씬 B].
	seedID := *p.LastOpenedNodeID
	if err := src.nodes.Rename(ctx, seedID, "프롤로그", "새벽의 항구", now); err != nil {
		t.Fatalf("rename seed: %v", err)
	}
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"프롤로그 본문."}]}]}`
	if err := src.nodes.UpdateContent(ctx, seedID, doc, now); err != nil {
		t.Fatalf("seed content: %v", err)
	}
	if err := src.nodes.SetStatus(ctx, seedID, node.StatusRevision, now); err != nil {
		t.Fatalf("seed status: %v", err)
	}
	seed, err := src.nodes.Get(ctx, seedID)
	if err != nil {
		t.Fatalf("get seed: %v", err)
	}
	if err := src.nodes.SetSummary(ctx, seedID, "항구에서 여정이 시작된다.", seed.ContentVersion); err != nil {
		t.Fatalf("seed summary: %v", err)
	}
	chapter, err := src.nodes.CreateSibling(ctx, seedID, "container", "1장", "", now)
	if err != nil {
		t.Fatalf("create chapter: %v", err)
	}
	sceneA, err := src.nodes.CreateChild(ctx, chapter.ID, "leaf", "씬 A", "", now)
	if err != nil {
		t.Fatalf("create scene A: %v", err)
	}
	if err := src.nodes.UpdateContent(ctx, sceneA.ID, doc, now); err != nil {
		t.Fatalf("scene A content: %v", err)
	}
	if _, err := src.nodes.CreateChild(ctx, chapter.ID, "leaf", "씬 B", "", now); err != nil {
		t.Fatalf("create scene B: %v", err)
	}

	hero, err := src.entities.Create(ctx, now, entity.NewInput{
		ProjectID: p.ID, Kind: "character", Name: "리나", Role: "주인공",
	})
	if err != nil {
		t.Fatalf("create entity: %v", err)
	}
	if err := src.entities.Update(ctx, now, entity.UpdateInput{
		ID:      hero.ID,
		Aliases: ptrutil.To([]string{"항해사"}),
		Summary: ptrutil.To("도시를 찾는 항해사."),
	}); err != nil {
		t.Fatalf("update entity: %v", err)
	}
	port, err := src.entities.Create(ctx, now, entity.NewInput{
		ProjectID: p.ID, Kind: "place", Name: "잿빛 항구",
	})
	if err != nil {
		t.Fatalf("create place: %v", err)
	}
	if _, err := src.relationships.CreateOne(ctx, relationship.NewInput{
		ProjectID: p.ID, FromID: hero.ID, ToID: port.ID, Label: "출신지", Notes: "떠나온 곳",
	}); err != nil {
		t.Fatalf("create relationship: %v", err)
	}

	th, err := src.threads.Create(ctx, thread.NewInput{ProjectID: p.ID, Name: "복수의 실", Color: "#a33"})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	if err := src.threads.Update(ctx, thread.UpdateInput{ID: th.ID, Summary: ptrutil.To("복수가 이어진다")}); err != nil {
		t.Fatalf("update thread: %v", err)
	}
	if _, err := src.beats.Create(ctx, beat.NewInput{
		ThreadID: th.ID, NodeID: &sceneA.ID, Label: "첫 단서", Description: "지도를 발견", Intensity: 2,
	}); err != nil {
		t.Fatalf("create beat: %v", err)
	}

	if _, err := src.notes.Create(ctx, note.NewInput{NodeID: seedID, Anchor: 3, Body: "여기 묘사 보강"}, now); err != nil {
		t.Fatalf("create note: %v", err)
	}

	if _, err := src.facts.Create(ctx, now, fact.NewInput{
		ProjectID: p.ID, NodeID: &sceneA.ID,
		Claim: "17세기 항해술", Result: "실제로 존재", Status: fact.StatusVerified, Category: "역사",
		Sources: []fact.SourceInput{{URL: "https://example.com/nav", Title: "항해술 자료", Snippet: "…", AccessedAt: now}},
	}); err != nil {
		t.Fatalf("create fact: %v", err)
	}

	exportParams, _ := json.Marshal(map[string]string{"project_id": p.ID})
	exportedRaw, err := ExportProject(src.exportSources(), nil)(ctx, exportParams)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var payload export.Payload
	if err := json.Unmarshal(exportedRaw, &payload); err != nil {
		t.Fatalf("unmarshal export: %v", err)
	}

	// ---- import into a fresh library ----
	dst := openLibrary(t, "dst.db")
	importParams, _ := json.Marshal(map[string]string{"file_name": payload.SuggestedFilename, "content": payload.Markdown})
	resultRaw, err := ImportMarkdown(dst.projects, dst.nodes, dst.entities, dst.relationships, dst.importExtras(), func() int64 { return 9000 })(ctx, importParams)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	var result struct {
		ProjectID string   `json:"project_id"`
		Warnings  []string `json:"warnings"`
	}
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		t.Fatalf("unmarshal import result: %v", err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("roundtrip warnings = %v, want none", result.Warnings)
	}

	got, err := dst.projects.Get(ctx, result.ProjectID)
	if err != nil {
		t.Fatalf("get imported project: %v", err)
	}
	if got.Title != "완전한 작품" {
		t.Errorf("title = %q", got.Title)
	}
	if len(got.Genres) != 2 || got.Genres[0] != "fantasy" || got.Genres[1] != "mystery" {
		t.Errorf("genres = %v", got.Genres)
	}
	if got.LengthTarget != "novella" || got.DefaultPOV != "third_limited" {
		t.Errorf("length/pov = %q/%q", got.LengthTarget, got.DefaultPOV)
	}
	if got.StyleNotes != "건조한 문체" {
		t.Errorf("style notes = %q", got.StyleNotes)
	}
	if got.Synopsis != "주인공이 잃어버린 도시를 찾는 이야기." {
		t.Errorf("synopsis = %q", got.Synopsis)
	}
	if got.Outline != "1부: 출발\n2부: 귀환" {
		t.Errorf("outline = %q", got.Outline)
	}
	if got.EpisodeCharTarget != 4200 {
		t.Errorf("episode char target = %d", got.EpisodeCharTarget)
	}

	nodes, err := dst.nodes.ListByProject(ctx, result.ProjectID)
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	byLabel := map[string]node.Node{}
	for _, n := range nodes {
		byLabel[n.Label] = n
	}
	prologue, ok := byLabel["프롤로그"]
	if !ok {
		t.Fatalf("프롤로그 missing; have %v", labels(nodes))
	}
	if prologue.Title != "새벽의 항구" {
		t.Errorf("prologue title = %q", prologue.Title)
	}
	if prologue.Status != node.StatusRevision {
		t.Errorf("prologue status = %q", prologue.Status)
	}
	if prologue.Summary != "항구에서 여정이 시작된다." {
		t.Errorf("prologue summary = %q", prologue.Summary)
	}
	if _, ok := byLabel["씬 A"]; !ok {
		t.Fatalf("씬 A missing; have %v", labels(nodes))
	}

	ents, err := dst.entities.ListByProject(ctx, result.ProjectID)
	if err != nil {
		t.Fatalf("list entities: %v", err)
	}
	if len(ents) != 2 {
		t.Fatalf("entities = %d, want 2", len(ents))
	}
	for _, e := range ents {
		if e.Name == "리나" {
			if len(e.Aliases) != 1 || e.Aliases[0] != "항해사" {
				t.Errorf("aliases = %v", e.Aliases)
			}
			if e.Summary != "도시를 찾는 항해사." {
				t.Errorf("entity summary = %q", e.Summary)
			}
		}
	}
	rels, err := dst.relationships.ListByProject(ctx, result.ProjectID)
	if err != nil {
		t.Fatalf("list relationships: %v", err)
	}
	if len(rels) != 1 || rels[0].Label != "출신지" || rels[0].Notes != "떠나온 곳" {
		t.Fatalf("relationships = %+v", rels)
	}

	threads, err := dst.threads.ListByProject(ctx, result.ProjectID, true)
	if err != nil {
		t.Fatalf("list threads: %v", err)
	}
	if len(threads) != 1 || threads[0].Name != "복수의 실" || threads[0].Color != "#a33" || threads[0].Summary != "복수가 이어진다" {
		t.Fatalf("threads = %+v", threads)
	}
	beats, err := dst.beats.ListByThread(ctx, threads[0].ID)
	if err != nil {
		t.Fatalf("list beats: %v", err)
	}
	if len(beats) != 1 || beats[0].Label != "첫 단서" || beats[0].Description != "지도를 발견" || beats[0].Intensity != 2 {
		t.Fatalf("beats = %+v", beats)
	}
	if beats[0].NodeID == nil || *beats[0].NodeID != byLabel["씬 A"].ID {
		t.Errorf("beat node link = %v, want 씬 A (%s)", beats[0].NodeID, byLabel["씬 A"].ID)
	}

	importedNotes, err := dst.notes.ListForNode(ctx, prologue.ID)
	if err != nil {
		t.Fatalf("list notes: %v", err)
	}
	if len(importedNotes) != 1 || importedNotes[0].Body != "여기 묘사 보강" || importedNotes[0].Anchor != 3 {
		t.Fatalf("notes = %+v", importedNotes)
	}

	cards, err := dst.facts.List(ctx, fact.ListFilter{ProjectID: result.ProjectID})
	if err != nil {
		t.Fatalf("list facts: %v", err)
	}
	if len(cards) != 1 {
		t.Fatalf("fact cards = %d, want 1", len(cards))
	}
	card := cards[0]
	if card.Claim != "17세기 항해술" || card.Result != "실제로 존재" || card.Status != fact.StatusVerified || card.Category != "역사" {
		t.Errorf("card = %+v", card)
	}
	if card.NodeID == nil || *card.NodeID != byLabel["씬 A"].ID {
		t.Errorf("card node link = %v", card.NodeID)
	}
	if len(card.Sources) != 1 || card.Sources[0].URL != "https://example.com/nav" || card.Sources[0].Title != "항해술 자료" {
		t.Errorf("card sources = %+v", card.Sources)
	}
}

// TestImportV1FrontmatterStillWorks: a version-1 export (entities and
// relationships only) must keep importing exactly as before.
func TestImportV1FrontmatterStillWorks(t *testing.T) {
	ctx := context.Background()
	dst := openLibrary(t, "v1.db")
	content := "---\n" +
		"linetta:\n" +
		"  version: 1\n" +
		"  entities:\n" +
		"    - id: e1\n" +
		"      kind: character\n" +
		"      name: 리나\n" +
		"---\n\n# 옛 작품\n\n## 씬 1\n\n본문.\n"
	params, _ := json.Marshal(map[string]string{"file_name": "old.md", "content": content})
	raw, err := ImportMarkdown(dst.projects, dst.nodes, dst.entities, dst.relationships, dst.importExtras(), func() int64 { return 1000 })(ctx, params)
	if err != nil {
		t.Fatalf("import v1: %v", err)
	}
	var result struct {
		ProjectID   string `json:"project_id"`
		EntityCount int    `json:"entity_count"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.EntityCount != 1 {
		t.Fatalf("entity count = %d, want 1", result.EntityCount)
	}
}

func labels(nodes []node.Node) []string {
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, n.Label)
	}
	return out
}
