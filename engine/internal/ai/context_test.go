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

	builder := NewContextBuilder(pr, nodes, mr, thread.NewRepo(s), beat.NewRepo(s))
	got, err := builder.Build(context.Background(), *p.LastOpenedNodeID, "재작성", Options{TonePreset: true})
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
	if !got.Options.TonePreset {
		t.Errorf("options not propagated")
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

	builder := NewContextBuilder(pr, nodes, mr, thread.NewRepo(s), beat.NewRepo(s))
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

	builder := NewContextBuilder(pr, nodes, mr, tr, br)
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
