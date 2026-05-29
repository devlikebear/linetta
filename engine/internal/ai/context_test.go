package ai

import (
	"context"
	"fmt"
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

func TestBuildContext_projectMetaPopulated(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	pr := project.NewRepo(s)
	p, _ := pr.Create(context.Background(), 1000, project.NewInput{
		Title: "t", Genres: []string{"판타지", "미스터리"}, LengthTarget: "novel", DefaultPOV: "first",
	})
	mr := mention.NewRepo(s)
	nodes := node.NewRepo(s)
	nodes.SetMentionResyncer(func(ctx context.Context, nodeID, doc string) error {
		return mr.ResyncForNode(ctx, nodeID, mention.Collect([]byte(doc)))
	})

	builder := NewContextBuilder(pr, nodes, mr, thread.NewRepo(s), beat.NewRepo(s), note.NewRepo(s))
	c, err := builder.Build(context.Background(), *p.LastOpenedNodeID, "user prompt", Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(c.Project.Genres) != 2 || c.Project.Genres[0] != "판타지" {
		t.Fatalf("Project.Genres=%v", c.Project.Genres)
	}
	if c.Project.LengthTarget != "novel" {
		t.Fatalf("Project.LengthTarget=%q", c.Project.LengthTarget)
	}
	if c.Project.DefaultPOV != "first" {
		t.Fatalf("Project.DefaultPOV=%q", c.Project.DefaultPOV)
	}
}

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

func TestBuildContext_entityDossier_populatesRecentFromOtherLeaves(t *testing.T) {
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
	er := entity.NewRepo(s)
	mr := mention.NewRepo(s)
	nodes := node.NewRepo(s)
	nodes.SetMentionResyncer(func(ctx context.Context, nodeID, doc string) error {
		return mr.ResyncForNode(ctx, nodeID, mention.Collect([]byte(doc)))
	})

	e, _ := er.Create(context.Background(), 1050, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "해진"})

	first := *p.LastOpenedNodeID
	doc := func(text string) string {
		return `{"type":"doc","content":[{"type":"paragraph","content":[
			{"type":"text","text":"` + text + ` "},
			{"type":"mention","attrs":{"id":"` + e.ID + `","label":"해진"}}
		]}]}`
	}
	_ = nodes.UpdateContent(context.Background(), first, doc("씬 1에서"), 1100)
	gotFirst, _ := nodes.Get(context.Background(), first)
	_ = nodes.SetSummary(context.Background(), first, "해진은 모래에 처음 도착했다.\n계속 ...", gotFirst.ContentVersion)

	second, _ := nodes.CreateSibling(context.Background(), first, "leaf", "씬 2", "", 1200)
	_ = nodes.UpdateContent(context.Background(), second.ID, doc("씬 2의 현재"), 1300)

	builder := NewContextBuilder(pr, nodes, mr, thread.NewRepo(s), beat.NewRepo(s), note.NewRepo(s))
	got, err := builder.Build(context.Background(), second.ID, "확장", Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(got.Entities) != 1 {
		t.Fatalf("entities = %d", len(got.Entities))
	}
	if len(got.Entities[0].Recent) != 1 || got.Entities[0].Recent[0] != "해진은 모래에 처음 도착했다." {
		t.Errorf("dossier = %+v, want one line", got.Entities[0].Recent)
	}
}

func TestBuildContext_relatedScenes_returnsCoMentionTop3(t *testing.T) {
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
	er := entity.NewRepo(s)
	mr := mention.NewRepo(s)
	nodes := node.NewRepo(s)
	nodes.SetMentionResyncer(func(ctx context.Context, nodeID, doc string) error {
		return mr.ResyncForNode(ctx, nodeID, mention.Collect([]byte(doc)))
	})

	e1, _ := er.Create(context.Background(), 1050, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "해진"})
	e2, _ := er.Create(context.Background(), 1060, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "민호"})

	withBoth := func(text string) string {
		return `{"type":"doc","content":[{"type":"paragraph","content":[
			{"type":"text","text":"` + text + `"},
			{"type":"mention","attrs":{"id":"` + e1.ID + `","label":"해진"}},
			{"type":"mention","attrs":{"id":"` + e2.ID + `","label":"민호"}}
		]}]}`
	}
	withOne := func(text, eid, lbl string) string {
		return `{"type":"doc","content":[{"type":"paragraph","content":[
			{"type":"text","text":"` + text + `"},
			{"type":"mention","attrs":{"id":"` + eid + `","label":"` + lbl + `"}}
		]}]}`
	}

	// Co-mention leaf: both entities together. Placed FIRST so the current
	// node's nearby window (2 prior + 1 next in DFS order) doesn't sweep it
	// in — `co` ends up 3+ leaves before `cur` once we insert filler.
	first := *p.LastOpenedNodeID
	_ = nodes.UpdateContent(context.Background(), first, withBoth("co — "), 1100)
	gotCo, _ := nodes.Get(context.Background(), first)
	_ = nodes.SetSummary(context.Background(), first, "co 요약", gotCo.ContentVersion)
	co := first

	// Single-entity leaf: should NOT appear (only 1 shared entity).
	solo, _ := nodes.CreateSibling(context.Background(), co, "leaf", "씬 solo", "", 1200)
	_ = nodes.UpdateContent(context.Background(), solo.ID, withOne("solo — ", e1.ID, "해진"), 1210)
	gotSolo, _ := nodes.Get(context.Background(), solo.ID)
	_ = nodes.SetSummary(context.Background(), solo.ID, "solo 요약", gotSolo.ContentVersion)

	// Filler leaves so `co` is not in cur's nearby (2 prior + 1 next) window.
	filler1, _ := nodes.CreateSibling(context.Background(), solo.ID, "leaf", "씬 f1", "", 1220)
	_ = nodes.UpdateContent(context.Background(), filler1.ID, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"f1"}]}]}`, 1225)
	filler2, _ := nodes.CreateSibling(context.Background(), filler1.ID, "leaf", "씬 f2", "", 1230)
	_ = nodes.UpdateContent(context.Background(), filler2.ID, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"f2"}]}]}`, 1235)

	// Current node: also mentions both entities.
	curN, _ := nodes.CreateSibling(context.Background(), filler2.ID, "leaf", "현재", "", 1300)
	cur := curN.ID
	_ = nodes.UpdateContent(context.Background(), cur, withBoth("현재 — "), 1310)

	builder := NewContextBuilder(pr, nodes, mr, thread.NewRepo(s), beat.NewRepo(s), note.NewRepo(s))
	got, err := builder.Build(context.Background(), cur, "확장", Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(got.RelatedScenes) != 1 || got.RelatedScenes[0].NodeID != co {
		t.Errorf("related = %+v, want [co]", got.RelatedScenes)
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

// fakeRefresher is a stand-in for *summarizer.Summarizer used by the Plan 16
// integration tests. It bypasses the LLM by writing a deterministic body back
// into nodes.summary at the current content_version.
type fakeRefresher struct {
	nodes *node.Repo
	body  func(n node.Node) string
}

func (f *fakeRefresher) RefreshNow(ctx context.Context, nodeID string) {
	n, err := f.nodes.Get(ctx, nodeID)
	if err != nil {
		return
	}
	if n.Summary != "" && n.SummaryForVersion == n.ContentVersion {
		return
	}
	_ = f.nodes.SetSummary(ctx, nodeID, f.body(n), n.ContentVersion)
}

// TestBuildContext_hierarchicalRetrieval — Test 7-1.
//
// Builds a 2부 / 2장 / 2씬 tree. Seeds leaf summaries directly via SetSummary but
// leaves every container summary empty. The injected fakeRefresher fills the
// stale container rollups synchronously, simulating the summarizer.
func TestBuildContext_hierarchicalRetrieval(t *testing.T) {
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

	// 1부 → {1장 → [씬1, 씬2-current], 2장 → [씬3, 씬4]}
	// 2부 → {3장 → [씬5, 씬6]}
	part1, _ := nodes.CreateSibling(context.Background(), *p.LastOpenedNodeID, "container", "1부", "", 1100)
	chap1, _ := nodes.CreateChild(context.Background(), part1.ID, "container", "1장", "", 1110)
	s1, _ := nodes.CreateChild(context.Background(), chap1.ID, "leaf", "씬 1", "", 1120)
	cur, _ := nodes.CreateChild(context.Background(), chap1.ID, "leaf", "씬 2", "", 1130) // current
	chap2, _ := nodes.CreateChild(context.Background(), part1.ID, "container", "2장", "", 1140)
	s3, _ := nodes.CreateChild(context.Background(), chap2.ID, "leaf", "씬 3", "", 1150)
	s4, _ := nodes.CreateChild(context.Background(), chap2.ID, "leaf", "씬 4", "", 1160)
	part2, _ := nodes.CreateSibling(context.Background(), part1.ID, "container", "2부", "", 1170)
	chap3, _ := nodes.CreateChild(context.Background(), part2.ID, "container", "3장", "", 1180)
	s5, _ := nodes.CreateChild(context.Background(), chap3.ID, "leaf", "씬 5", "", 1190)
	s6, _ := nodes.CreateChild(context.Background(), chap3.ID, "leaf", "씬 6", "", 1200)

	docOf := func(text string) string {
		return `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + text + `"}]}]}`
	}
	// Write bodies on all leaves so ancestor content_versions bump.
	for _, id := range []string{s1.ID, cur.ID, s3.ID, s4.ID, s5.ID, s6.ID} {
		_ = nodes.UpdateContent(context.Background(), id, docOf("body of "+id), 1300)
	}

	// Seed leaf summaries directly (no LLM needed).
	seedLeaf := func(id, body string) {
		got, _ := nodes.Get(context.Background(), id)
		_ = nodes.SetSummary(context.Background(), id, body, got.ContentVersion)
	}
	seedLeaf(s1.ID, "씬1 요약")
	seedLeaf(s3.ID, "씬3 요약")
	seedLeaf(s4.ID, "씬4 요약")
	seedLeaf(s5.ID, "씬5 요약")
	seedLeaf(s6.ID, "씬6 요약")
	// Note: container summaries (chap1/chap2/chap3/part1/part2) are deliberately
	// NOT seeded. The fakeRefresher fills them on demand.

	calls := map[string]int{}
	ref := &fakeRefresher{
		nodes: nodes,
		body: func(n node.Node) string {
			calls[n.ID]++
			return fmt.Sprintf("ROLLUP[%s]", n.Label)
		},
	}

	builder := NewContextBuilder(pr, nodes, mr, thread.NewRepo(s), beat.NewRepo(s), note.NewRepo(s)).
		WithSummaryRefresher(ref)
	got, err := builder.Build(context.Background(), cur.ID, "확장", Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// NearbyLeafSummaries: cur is at index 1 in DFS order [s1, cur, s3, s4, s5, s6].
	// 2 prior would be (curIdx-2 = -1, skipped) and (curIdx-1 = s1), plus 1 next (s3).
	// At edge → expect 2 entries: s1 and s3.
	nearbyIDs := map[string]bool{}
	for _, ss := range got.Hierarchical.NearbyLeafSummaries {
		nearbyIDs[ss.NodeID] = true
	}
	if !nearbyIDs[s1.ID] || !nearbyIDs[s3.ID] {
		t.Errorf("nearby missing s1/s3: got %+v", got.Hierarchical.NearbyLeafSummaries)
	}
	if nearbyIDs[cur.ID] {
		t.Errorf("nearby leaked current: %+v", got.Hierarchical.NearbyLeafSummaries)
	}

	// SameChapterSummaries excludes current + nearby. cur's parent is chap1; the
	// only other chap1 leaf is s1, which is in nearby — so SameChapter should be
	// empty here.
	for _, ss := range got.Hierarchical.SameChapterSummaries {
		if nearbyIDs[ss.NodeID] || ss.NodeID == cur.ID {
			t.Errorf("same_chapter leaked nearby/self: %s", ss.NodeID)
		}
	}

	// OtherChapterSummaries should include chap2 (sibling of chap1 under part1).
	foundChap2 := false
	for _, ch := range got.Hierarchical.OtherChapterSummaries {
		if ch.NodeID == chap2.ID && strings.Contains(ch.Body, "ROLLUP[2장]") {
			foundChap2 = true
		}
	}
	if !foundChap2 {
		t.Errorf("other_chapter_summaries missing 2장 rollup: %+v", got.Hierarchical.OtherChapterSummaries)
	}

	// OtherPartSummaries should include part2.
	foundPart2 := false
	for _, ps := range got.Hierarchical.OtherPartSummaries {
		if ps.NodeID == part2.ID && strings.Contains(ps.Body, "ROLLUP[2부]") {
			foundPart2 = true
		}
	}
	if !foundPart2 {
		t.Errorf("other_part_summaries missing 2부 rollup: %+v", got.Hierarchical.OtherPartSummaries)
	}

	// ProjectSynopsis non-empty (there's the seeded default leaf at root plus
	// 1부 and 2부 containers → multi-root branch concatenates 부 rollups).
	if got.Hierarchical.ProjectSynopsis == "" {
		t.Errorf("project_synopsis empty; tree has 부 containers")
	}

	// fakeRefresher was actually invoked for at least one container.
	if calls[chap2.ID] == 0 && calls[part2.ID] == 0 {
		t.Errorf("refresher was never invoked on a container: %+v", calls)
	}
}

// TestBuildContext_entityDossier — Test 7-2.
//
// Three past leaves all mention 해진; build context against a 4th leaf that also
// mentions 해진. Recent should hold the 3 prior first-lines, most-recent first.
func TestBuildContext_entityDossier(t *testing.T) {
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
	er := entity.NewRepo(s)
	mr := mention.NewRepo(s)
	nodes := node.NewRepo(s)
	nodes.SetMentionResyncer(func(ctx context.Context, nodeID, doc string) error {
		return mr.ResyncForNode(ctx, nodeID, mention.Collect([]byte(doc)))
	})

	e, _ := er.Create(context.Background(), 1050, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "해진"})

	doc := func(text string) string {
		return `{"type":"doc","content":[{"type":"paragraph","content":[
			{"type":"text","text":"` + text + ` "},
			{"type":"mention","attrs":{"id":"` + e.ID + `","label":"해진"}}
		]}]}`
	}

	// Three past leaves, each mentions 해진. Updated in ascending time order so
	// the LAST (third) one has the highest updated_at and should top Recent[0].
	leaf1 := *p.LastOpenedNodeID
	_ = nodes.UpdateContent(context.Background(), leaf1, doc("씬 1"), 1100)
	got1, _ := nodes.Get(context.Background(), leaf1)
	_ = nodes.SetSummary(context.Background(), leaf1, "첫 등장.\n계속", got1.ContentVersion)

	leaf2, _ := nodes.CreateSibling(context.Background(), leaf1, "leaf", "씬 2", "", 1110)
	_ = nodes.UpdateContent(context.Background(), leaf2.ID, doc("씬 2"), 1200)
	got2, _ := nodes.Get(context.Background(), leaf2.ID)
	_ = nodes.SetSummary(context.Background(), leaf2.ID, "두 번째 만남.\n계속", got2.ContentVersion)

	leaf3, _ := nodes.CreateSibling(context.Background(), leaf2.ID, "leaf", "씬 3", "", 1120)
	_ = nodes.UpdateContent(context.Background(), leaf3.ID, doc("씬 3"), 1300)
	got3, _ := nodes.Get(context.Background(), leaf3.ID)
	_ = nodes.SetSummary(context.Background(), leaf3.ID, "세 번째 사건.\n계속", got3.ContentVersion)

	// Current (4th) leaf — also mentions 해진.
	leaf4, _ := nodes.CreateSibling(context.Background(), leaf3.ID, "leaf", "씬 4", "", 1130)
	_ = nodes.UpdateContent(context.Background(), leaf4.ID, doc("현재"), 1400)

	builder := NewContextBuilder(pr, nodes, mr, thread.NewRepo(s), beat.NewRepo(s), note.NewRepo(s))
	got, err := builder.Build(context.Background(), leaf4.ID, "확장", Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(got.Entities) != 1 || got.Entities[0].Name != "해진" {
		t.Fatalf("entities = %+v", got.Entities)
	}
	recent := got.Entities[0].Recent
	if len(recent) != 3 {
		t.Fatalf("Recent len = %d, want 3: %+v", len(recent), recent)
	}
	// Most recently-updated leaf (leaf3) should be first.
	if recent[0] != "세 번째 사건." {
		t.Errorf("Recent[0] = %q, want %q", recent[0], "세 번째 사건.")
	}
}

// TestBuildContext_topologyRAG — Test 7-3.
//
// Two entities. Past leaf A mentions both. Current leaf mentions both. Other
// past leaves mention only one (should NOT surface — k < 2).
func TestBuildContext_topologyRAG(t *testing.T) {
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
	er := entity.NewRepo(s)
	mr := mention.NewRepo(s)
	nodes := node.NewRepo(s)
	nodes.SetMentionResyncer(func(ctx context.Context, nodeID, doc string) error {
		return mr.ResyncForNode(ctx, nodeID, mention.Collect([]byte(doc)))
	})

	e1, _ := er.Create(context.Background(), 1050, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "해진"})
	e2, _ := er.Create(context.Background(), 1060, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "민호"})

	withBoth := func(text string) string {
		return `{"type":"doc","content":[{"type":"paragraph","content":[
			{"type":"text","text":"` + text + `"},
			{"type":"mention","attrs":{"id":"` + e1.ID + `","label":"해진"}},
			{"type":"mention","attrs":{"id":"` + e2.ID + `","label":"민호"}}
		]}]}`
	}
	withOne := func(text, eid, lbl string) string {
		return `{"type":"doc","content":[{"type":"paragraph","content":[
			{"type":"text","text":"` + text + `"},
			{"type":"mention","attrs":{"id":"` + eid + `","label":"` + lbl + `"}}
		]}]}`
	}

	// Past leaf A (the seeded root leaf): mentions BOTH entities.
	leafA := *p.LastOpenedNodeID
	_ = nodes.UpdateContent(context.Background(), leafA, withBoth("A — "), 1100)
	gotA, _ := nodes.Get(context.Background(), leafA)
	_ = nodes.SetSummary(context.Background(), leafA, "A 요약", gotA.ContentVersion)

	// Single-entity leaves — must NOT surface.
	soloHae, _ := nodes.CreateSibling(context.Background(), leafA, "leaf", "씬 해진only", "", 1200)
	_ = nodes.UpdateContent(context.Background(), soloHae.ID, withOne("hae — ", e1.ID, "해진"), 1210)
	gotSH, _ := nodes.Get(context.Background(), soloHae.ID)
	_ = nodes.SetSummary(context.Background(), soloHae.ID, "해진only 요약", gotSH.ContentVersion)

	soloMin, _ := nodes.CreateSibling(context.Background(), soloHae.ID, "leaf", "씬 민호only", "", 1220)
	_ = nodes.UpdateContent(context.Background(), soloMin.ID, withOne("min — ", e2.ID, "민호"), 1230)
	gotSM, _ := nodes.Get(context.Background(), soloMin.ID)
	_ = nodes.SetSummary(context.Background(), soloMin.ID, "민호only 요약", gotSM.ContentVersion)

	// Filler so the current leaf's nearby window (2 prior + 1 next) does not
	// sweep leafA in — without this, leafA would be filtered out as "nearby".
	filler1, _ := nodes.CreateSibling(context.Background(), soloMin.ID, "leaf", "f1", "", 1240)
	_ = nodes.UpdateContent(context.Background(), filler1.ID, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"f1"}]}]}`, 1245)
	filler2, _ := nodes.CreateSibling(context.Background(), filler1.ID, "leaf", "f2", "", 1250)
	_ = nodes.UpdateContent(context.Background(), filler2.ID, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"f2"}]}]}`, 1255)

	// Current leaf — mentions BOTH entities.
	curN, _ := nodes.CreateSibling(context.Background(), filler2.ID, "leaf", "현재", "", 1300)
	_ = nodes.UpdateContent(context.Background(), curN.ID, withBoth("cur — "), 1310)

	builder := NewContextBuilder(pr, nodes, mr, thread.NewRepo(s), beat.NewRepo(s), note.NewRepo(s))
	got, err := builder.Build(context.Background(), curN.ID, "확장", Options{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(got.RelatedScenes) != 1 {
		t.Fatalf("related = %+v, want exactly [leafA]", got.RelatedScenes)
	}
	if got.RelatedScenes[0].NodeID != leafA {
		t.Errorf("related[0] = %s, want leafA=%s", got.RelatedScenes[0].NodeID, leafA)
	}
}
