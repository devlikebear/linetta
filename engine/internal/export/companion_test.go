package export

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/project"
)

type stubHistory struct {
	byProject map[string][]companion.HistoryMessage
	queries   []companion.HistoryQuery
}

func (s *stubHistory) List(_ context.Context, q companion.HistoryQuery) ([]companion.HistoryMessage, error) {
	s.queries = append(s.queries, q)
	return s.byProject[q.ProjectID], nil
}

type stubMemory struct {
	facts map[string][]string
	root  string
}

func (s *stubMemory) Recall(projectID, _ string, limit int) []string {
	got := s.facts[projectID]
	if limit > 0 && len(got) > limit {
		return got[:limit]
	}
	return got
}

func (s *stubMemory) MemoryRoot(string) string { return s.root }

func exportedAt() time.Time { return time.Date(2026, 8, 25, 9, 30, 0, 0, time.UTC) }

func TestExportCompanion_archivesTranscriptAndFacts(t *testing.T) {
	_, pr, _, _, _ := newExportFixture(t)
	ctx := context.Background()
	p, err := pr.Create(ctx, 1, project.NewInput{
		Title: "조용한 도시", Genres: []string{"문학"}, LengthTarget: "novella", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	at := exportedAt().UnixMilli()
	history := &stubHistory{byProject: map[string][]companion.HistoryMessage{
		p.ID: {
			{Role: "user", Content: "3장 분위기를 어떻게 잡을까", Status: companion.HistoryStatusDone, CreatedAt: at},
			{Role: "assistant", Content: "비 오는 저녁으로 시작해 보세요", Status: companion.HistoryStatusDone, CreatedAt: at, NodeLabel: "씬 3"},
		},
	}}
	mem := &stubMemory{facts: map[string][]string{p.ID: {"주인공은 커피를 마시지 않는다"}}}

	out, err := ExportCompanion(ctx, pr, history, mem, exportedAt())
	if err != nil {
		t.Fatalf("ExportCompanion: %v", err)
	}

	for _, want := range []string{
		"조용한 도시",
		"3장 분위기를 어떻게 잡을까",
		"비 오는 저녁으로 시작해 보세요",
		"씬 3",
		"주인공은 커피를 마시지 않는다",
		"2026-08-25",
	} {
		if !strings.Contains(out.Markdown, want) {
			t.Fatalf("markdown missing %q:\n%s", want, out.Markdown)
		}
	}
	// The archive is the thing that survives the companion's removal, so it has
	// to name who said what without Linetta to interpret it.
	if !strings.Contains(out.Markdown, "나") || !strings.Contains(out.Markdown, "컴패니언") {
		t.Fatalf("speakers not labelled:\n%s", out.Markdown)
	}
	if out.SuggestedFilename != "linetta-companion-20260825.md" {
		t.Fatalf("suggested_filename = %q", out.SuggestedFilename)
	}
}

func TestExportCompanion_skipsProjectsWithNothingToKeep(t *testing.T) {
	_, pr, _, _, _ := newExportFixture(t)
	ctx := context.Background()
	quiet, err := pr.Create(ctx, 1, project.NewInput{
		Title: "말 없는 작품", Genres: []string{"문학"}, LengthTarget: "short", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	out, err := ExportCompanion(ctx, pr, &stubHistory{}, &stubMemory{}, exportedAt())
	if err != nil {
		t.Fatalf("ExportCompanion: %v", err)
	}
	if strings.Contains(out.Markdown, quiet.Title) {
		t.Fatalf("project with no companion data appears in the archive:\n%s", out.Markdown)
	}
	if !strings.Contains(out.Markdown, "남아 있는 컴패니언 대화나 기억이 없습니다") {
		t.Fatalf("empty archive does not say so:\n%s", out.Markdown)
	}
}

func TestExportCompanion_pointsAtTheRawLogWhenFactsAreTruncated(t *testing.T) {
	_, pr, _, _, _ := newExportFixture(t)
	ctx := context.Background()
	p, err := pr.Create(ctx, 1, project.NewInput{
		Title: "기억이 많은 작품", Genres: []string{"문학"}, LengthTarget: "novel", DefaultPOV: "third_limited",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	many := make([]string, CompanionExportMemoryLimit+5)
	for i := range many {
		many[i] = "사실"
	}
	mem := &stubMemory{facts: map[string][]string{p.ID: many}, root: "/home/w/companion/p1"}

	out, err := ExportCompanion(ctx, pr, &stubHistory{}, mem, exportedAt())
	if err != nil {
		t.Fatalf("ExportCompanion: %v", err)
	}
	// Silent truncation in a preservation path is the failure this whole task
	// exists to avoid, so a full list has to say where the rest is.
	if !strings.Contains(out.Markdown, "experiences.jsonl") {
		t.Fatalf("truncated fact list does not point at the raw log:\n%s", out.Markdown)
	}
	if !strings.Contains(out.Markdown, "/home/w/companion/p1") {
		t.Fatalf("truncated fact list does not name the memory directory:\n%s", out.Markdown)
	}
}

func TestExportCompanion_readsWholeProjectScope(t *testing.T) {
	_, pr, _, _, _ := newExportFixture(t)
	ctx := context.Background()
	if _, err := pr.Create(ctx, 1, project.NewInput{
		Title: "범위 확인", Genres: []string{"문학"}, LengthTarget: "short", DefaultPOV: "first",
	}); err != nil {
		t.Fatalf("create project: %v", err)
	}

	history := &stubHistory{}
	if _, err := ExportCompanion(ctx, pr, history, nil, exportedAt()); err != nil {
		t.Fatalf("ExportCompanion: %v", err)
	}
	if len(history.queries) != 1 {
		t.Fatalf("queries = %d, want 1", len(history.queries))
	}
	// Scene scope would drop every message the writer sent from the project
	// view — an archive has to ask for all of it.
	if history.queries[0].Scope != companion.HistoryViewProject {
		t.Fatalf("scope = %q, want %q", history.queries[0].Scope, companion.HistoryViewProject)
	}
	if history.queries[0].NodeID != "" {
		t.Fatalf("node_id = %q, want empty", history.queries[0].NodeID)
	}
}
