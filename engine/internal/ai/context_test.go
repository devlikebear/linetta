package ai

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/note"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

func TestBuildContext_includesSceneEntitiesAndStyleNotes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	pr := project.NewRepo(s)
	p, _ := pr.Create(context.Background(), 1000, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "novel", DefaultPOV: "first",
	})

	// Set style_notes directly.
	_, _ = s.DB().ExecContext(context.Background(), `UPDATE projects SET style_notes = ? WHERE id = ?`, "단문 위주", p.ID)

	er := entity.NewRepo(s)
	mr := mention.NewRepo(s)
	nodes := node.NewRepo(s)
	nodes.SetMentionResyncer(func(ctx context.Context, nodeID, doc string) error {
		return mr.ResyncForNode(ctx, nodeID, mention.Collect([]byte(doc)))
	})

	e, _ := er.Create(context.Background(), 1100, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "해진", Role: "POV"})

	// Write scene with the mention.
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[
		{"type":"text","text":"파도 소리. "},
		{"type":"mention","attrs":{"id":"` + e.ID + `","label":"해진"}},
		{"type":"text","text":"이 모래를 밟았다."}
	]}]}`
	if err := nodes.UpdateContent(context.Background(), *p.LastOpenedNodeID, doc, 2000); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}

	builder := NewContextBuilder(pr, nodes, mr, thread.NewRepo(s), beat.NewRepo(s), note.NewRepo(s))
	got, err := builder.Build(context.Background(), *p.LastOpenedNodeID, "재작성", Options{Tone: TonePresetMy})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got.SceneLabel != "씬 1" {
		t.Errorf("scene_label = %q", got.SceneLabel)
	}
	if !strings.Contains(got.SceneText, "파도 소리") {
		t.Errorf("scene_text missing prose: %q", got.SceneText)
	}
	if got.StyleNotes != "단문 위주" {
		t.Errorf("style_notes = %q", got.StyleNotes)
	}
	if len(got.Entities) != 1 || got.Entities[0].Name != "해진" {
		t.Errorf("entities = %+v", got.Entities)
	}
	if got.UserPrompt != "재작성" {
		t.Errorf("prompt = %q", got.UserPrompt)
	}
	if got.Options.Tone != TonePresetMy {
		t.Errorf("options not propagated: tone=%q", got.Options.Tone)
	}
}

func TestBuildContext_prevSummary_trims300chars(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	pr := project.NewRepo(s)
	p, _ := pr.Create(context.Background(), 1000, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "novel", DefaultPOV: "first",
	})
	mr := mention.NewRepo(s)
	nodes := node.NewRepo(s)
	nodes.SetMentionResyncer(func(ctx context.Context, nodeID, doc string) error {
		return mr.ResyncForNode(ctx, nodeID, mention.Collect([]byte(doc)))
	})

	// First leaf "씬 1" gets long content; add a second leaf "씬 2" and build
	// context for it — should pull a 300-char trim of 씬 1 as prev_summary.
	var long strings.Builder
	for i := 0; i < 400; i++ {
		long.WriteString("가")
	}
	docFirst := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + long.String() + `"}]}]}`
	_ = nodes.UpdateContent(context.Background(), *p.LastOpenedNodeID, docFirst, 1100)

	second, _ := nodes.CreateSibling(context.Background(), *p.LastOpenedNodeID, "leaf", "씬 2", "", 1200)

	builder := NewContextBuilder(pr, nodes, mr, thread.NewRepo(s), beat.NewRepo(s), note.NewRepo(s))
	got, err := builder.Build(context.Background(), second.ID, "확장", Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got.PrevSummary == "" {
		t.Fatal("prev_summary should be populated")
	}
	if r := []rune(got.PrevSummary); len(r) > 310 { // 300 + ellipsis slack
		t.Errorf("prev_summary too long: %d runes", len(r))
	}
}

func TestBuildContext_activeThreadsForCurrentNode(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	pr := project.NewRepo(s)
	p, _ := pr.Create(context.Background(), 1000, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "novel", DefaultPOV: "first",
	})
	mr := mention.NewRepo(s)
	nodes := node.NewRepo(s)
	nodes.SetMentionResyncer(func(ctx context.Context, nodeID, doc string) error {
		return mr.ResyncForNode(ctx, nodeID, mention.Collect([]byte(doc)))
	})
	tr := thread.NewRepo(s)
	br := beat.NewRepo(s)

	// Open thread bound to the current node via two beats.
	th, _ := tr.Create(context.Background(), thread.NewInput{ProjectID: p.ID, Name: "잃어버린 시간", Color: "#c08a3e"})
	_ = tr.Update(context.Background(), thread.UpdateInput{ID: th.ID, Summary: "요약"})
	nID := *p.LastOpenedNodeID
	_, _ = br.Create(context.Background(), beat.NewInput{ThreadID: th.ID, NodeID: &nID, Label: "마디 1"})
	_, _ = br.Create(context.Background(), beat.NewInput{ThreadID: th.ID, NodeID: &nID, Label: "마디 2"})

	// Closed thread bound to the same node — must NOT appear.
	closed, _ := tr.Create(context.Background(), thread.NewInput{ProjectID: p.ID, Name: "닫힌"})
	_, _ = br.Create(context.Background(), beat.NewInput{ThreadID: closed.ID, NodeID: &nID, Label: "닫힌 마디"})
	_ = tr.Close(context.Background(), closed.ID, 2000)

	builder := NewContextBuilder(pr, nodes, mr, tr, br, note.NewRepo(s))
	got, err := builder.Build(context.Background(), nID, "재작성", Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(got.ActiveThreads) != 1 {
		t.Fatalf("active = %d, want 1 (open only)", len(got.ActiveThreads))
	}
	at := got.ActiveThreads[0]
	if at.Name != "잃어버린 시간" || at.Color != "#c08a3e" || at.Summary != "요약" {
		t.Errorf("active = %+v", at)
	}
	if len(at.RecentBeats) != 2 || at.RecentBeats[0].Label != "마디 1" {
		t.Errorf("beats = %+v", at.RecentBeats)
	}
}

// setupPrevSummaryFixture seeds two leaves and returns the project, repos, and
// the second leaf's id — shared by the three cache-path tests below.
func setupPrevSummaryFixture(t *testing.T) (*project.Repo, *node.Repo, *mention.Repo, *thread.Repo, *beat.Repo, *note.Repo, string, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	pr := project.NewRepo(s)
	p, _ := pr.Create(context.Background(), 1000, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "novel", DefaultPOV: "first",
	})
	mr := mention.NewRepo(s)
	nodes := node.NewRepo(s)
	nodes.SetMentionResyncer(func(ctx context.Context, nodeID, doc string) error {
		return mr.ResyncForNode(ctx, nodeID, mention.Collect([]byte(doc)))
	})
	var long strings.Builder
	for i := 0; i < 400; i++ {
		long.WriteString("가")
	}
	docFirst := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + long.String() + `"}]}]}`
	_ = nodes.UpdateContent(context.Background(), *p.LastOpenedNodeID, docFirst, 1100)
	second, _ := nodes.CreateSibling(context.Background(), *p.LastOpenedNodeID, "leaf", "씬 2", "", 1200)
	return pr, nodes, mr, thread.NewRepo(s), beat.NewRepo(s), note.NewRepo(s), *p.LastOpenedNodeID, second.ID
}

func TestBuildContext_prevSummary_usesFreshCache(t *testing.T) {
	pr, nodes, mr, tr, br, nr, prevID, secondID := setupPrevSummaryFixture(t)

	prevN, _ := nodes.Get(context.Background(), prevID)
	if err := nodes.SetSummary(context.Background(), prevID, "캐시된 요약", prevN.ContentVersion); err != nil {
		t.Fatalf("SetSummary: %v", err)
	}

	builder := NewContextBuilder(pr, nodes, mr, tr, br, nr)
	got, err := builder.Build(context.Background(), secondID, "확장", Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got.PrevSummary != "캐시된 요약" {
		t.Errorf("prev_summary = %q, want cached %q", got.PrevSummary, "캐시된 요약")
	}
}

func TestBuildContext_prevSummary_fallsBackWhenStale(t *testing.T) {
	pr, nodes, mr, tr, br, nr, prevID, secondID := setupPrevSummaryFixture(t)

	// Seed a summary stamped for an older content_version (0 — the doc has been
	// updated once, so content_version is 1).
	_ = nodes.SetSummary(context.Background(), prevID, "오래된 요약", 0)

	builder := NewContextBuilder(pr, nodes, mr, tr, br, nr)
	got, err := builder.Build(context.Background(), secondID, "확장", Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got.PrevSummary == "오래된 요약" {
		t.Errorf("stale cache used: %q", got.PrevSummary)
	}
	if got.PrevSummary == "" {
		t.Errorf("fallback trim did not run")
	}
}

func TestBuildContext_prevSummary_fallsBackWhenEmpty(t *testing.T) {
	pr, nodes, mr, tr, br, nr, _, secondID := setupPrevSummaryFixture(t)

	builder := NewContextBuilder(pr, nodes, mr, tr, br, nr)
	got, err := builder.Build(context.Background(), secondID, "확장", Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got.PrevSummary == "" {
		t.Errorf("empty cache should have fallen back to trim, got empty")
	}
}

func TestBuildContext_hierarchical_populatesNearbySameChapterAndPart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	pr := project.NewRepo(s)
	p, _ := pr.Create(context.Background(), 1000, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "novel", DefaultPOV: "first",
	})
	mr := mention.NewRepo(s)
	nodes := node.NewRepo(s)
	nodes.SetMentionResyncer(func(ctx context.Context, nodeID, doc string) error {
		return mr.ResyncForNode(ctx, nodeID, mention.Collect([]byte(doc)))
	})

	// 1부 → 1장 → {씬 1, 씬 2, 씬 3 (current), 씬 4}, plus 2부 → 2장 → 씬 5.
	part1, _ := nodes.CreateSibling(context.Background(), *p.LastOpenedNodeID, "container", "1부", "", 1100)
	chap1, _ := nodes.CreateChild(context.Background(), part1.ID, "container", "1장", "", 1110)
	s1, _ := nodes.CreateChild(context.Background(), chap1.ID, "leaf", "씬 1", "", 1120)
	s2, _ := nodes.CreateChild(context.Background(), chap1.ID, "leaf", "씬 2", "", 1130)
	s3, _ := nodes.CreateChild(context.Background(), chap1.ID, "leaf", "씬 3", "", 1140)
	s4, _ := nodes.CreateChild(context.Background(), chap1.ID, "leaf", "씬 4", "", 1150)
	part2, _ := nodes.CreateSibling(context.Background(), part1.ID, "container", "2부", "", 1160)
	chap2, _ := nodes.CreateChild(context.Background(), part2.ID, "container", "2장", "", 1170)
	_, _ = nodes.CreateChild(context.Background(), chap2.ID, "leaf", "씬 5", "", 1180)

	// Also delete the project's seeded default leaf so we have a clean
	// two-part tree at the root (otherwise the synopsis branch sees > 1
	// root container — still fine, but the assertion is cleaner this way).
	docOf := func(text string) string {
		return `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + text + `"}]}]}`
	}
	_ = nodes.UpdateContent(context.Background(), s1.ID, docOf("씬1 본문"), 1200)
	_ = nodes.UpdateContent(context.Background(), s2.ID, docOf("씬2 본문"), 1210)
	_ = nodes.UpdateContent(context.Background(), s3.ID, docOf("씬3 본문 — 현재 씬"), 1220)
	_ = nodes.UpdateContent(context.Background(), s4.ID, docOf("씬4 본문"), 1230)

	// Seed fresh summaries on leaves we want layered into the result, and on
	// every container so the builder reads them without invoking the refresher.
	seedFresh := func(id, body string) {
		got, _ := nodes.Get(context.Background(), id)
		_ = nodes.SetSummary(context.Background(), id, body, got.ContentVersion)
	}
	seedFresh(s1.ID, "씬1 요약")
	seedFresh(s2.ID, "씬2 요약")
	seedFresh(s4.ID, "씬4 요약")
	seedFresh(chap1.ID, "1장 요약")
	seedFresh(part1.ID, "1부 요약")
	seedFresh(chap2.ID, "2장 요약")
	seedFresh(part2.ID, "2부 요약")

	builder := NewContextBuilder(pr, nodes, mr, thread.NewRepo(s), beat.NewRepo(s), note.NewRepo(s))
	got, err := builder.Build(context.Background(), s3.ID, "확장", Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	gotNearby := map[string]bool{}
	for _, ss := range got.Hierarchical.NearbyLeafSummaries {
		gotNearby[ss.NodeID] = true
	}
	for _, want := range []string{s1.ID, s2.ID, s4.ID} {
		if !gotNearby[want] {
			t.Errorf("nearby missing %s; got %+v", want, got.Hierarchical.NearbyLeafSummaries)
		}
	}
	for _, ss := range got.Hierarchical.SameChapterSummaries {
		if gotNearby[ss.NodeID] || ss.NodeID == s3.ID {
			t.Errorf("same_chapter leaked nearby/self: %s", ss.NodeID)
		}
	}
	foundPart := false
	for _, ps := range got.Hierarchical.OtherPartSummaries {
		if ps.NodeID == part2.ID && ps.Body == "2부 요약" {
			foundPart = true
		}
	}
	if !foundPart {
		t.Errorf("other_part_summaries missing 2부: %+v", got.Hierarchical.OtherPartSummaries)
	}
	if !strings.Contains(got.Hierarchical.ProjectSynopsis, "1부") {
		t.Errorf("project_synopsis = %q, want to mention 1부", got.Hierarchical.ProjectSynopsis)
	}
	// Breadcrumb sanity check on one nearby entry.
	for _, ss := range got.Hierarchical.NearbyLeafSummaries {
		if ss.NodeID == s2.ID && !strings.Contains(ss.Label, " / ") {
			t.Errorf("nearby s2 label missing breadcrumb separator: %q", ss.Label)
		}
	}
}

func TestBuildContext_includesNotesForNode(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	pr := project.NewRepo(s)
	p, _ := pr.Create(context.Background(), 1000, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "novel", DefaultPOV: "first",
	})
	mr := mention.NewRepo(s)
	nodes := node.NewRepo(s)
	nodes.SetMentionResyncer(func(ctx context.Context, nodeID, doc string) error {
		return mr.ResyncForNode(ctx, nodeID, mention.Collect([]byte(doc)))
	})
	nr := note.NewRepo(s)
	_, _ = nr.Create(context.Background(), note.NewInput{NodeID: *p.LastOpenedNodeID, Anchor: 7, Body: "톤 바꾸기"}, 1000)

	builder := NewContextBuilder(pr, nodes, mr, thread.NewRepo(s), beat.NewRepo(s), nr)
	got, err := builder.Build(context.Background(), *p.LastOpenedNodeID, "확장", Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(got.Notes) != 1 || got.Notes[0].Body != "톤 바꾸기" || got.Notes[0].Anchor != 7 {
		t.Errorf("notes = %+v", got.Notes)
	}
}
