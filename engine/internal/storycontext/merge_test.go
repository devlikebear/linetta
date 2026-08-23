package storycontext

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/note"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

// newTestBuilder opens a temp store with one project and returns a builder plus
// the project's first scene node id. No summary refresher is wired, matching a
// provider-less installation.
func newTestBuilder(t *testing.T) (*ContextBuilder, string) {
	t.Helper()
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	pr := project.NewRepo(s)
	p, err := pr.Create(context.Background(), 1000, project.NewInput{
		Title: "병합 테스트", Genres: []string{"판타지"}, LengthTarget: "short", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	mr := mention.NewRepo(s)
	nodes := node.NewRepo(s)
	builder := NewContextBuilder(pr, nodes, mr, thread.NewRepo(s), beat.NewRepo(s), note.NewRepo(s), relationship.NewRepo(s))
	return builder, *p.LastOpenedNodeID
}

// Task 1.3 of the MCP-first pivot (#47): facts, memories, and references —
// which today only the companion's own prompt path gathers — become optional
// sections of the shared story brief, completing the ContextSelection toggles
// that already existed for them.

type fakeFactSource struct{ facts []FactBrief }

func (f fakeFactSource) ContextFacts(context.Context, string, string) ([]FactBrief, error) {
	return f.facts, nil
}

type fakeMemorySource struct{ memories []string }

func (m fakeMemorySource) ContextMemories(string) []string { return m.memories }

type fakeReferenceSource struct{ refs []ReferenceBrief }

func (r fakeReferenceSource) ContextReferences(context.Context, string, string) ([]ReferenceBrief, error) {
	return r.refs, nil
}

func mergedContext() Context {
	return Context{
		SceneLabel: "씬 1",
		SceneText:  "본문",
		UserPrompt: "이어서",
		Options:    Options{Language: "ko"},
		Facts: []FactBrief{{
			ID: "f1", Status: "verified", Claim: "1920년대 경성에는 전차가 다녔다",
			Sources: []FactSourceBrief{{Title: "경성 교통사", URL: "https://example.test/tram"}},
		}},
		Memories:   []string{"작가는 건조한 문체를 선호한다"},
		References: []ReferenceBrief{{Title: "톤 레퍼런스", Purpose: "style", Body: "짧고 차가운 문장."}},
	}
}

// The renderer must carry all three merged sections into the user prompt.
func TestRenderIncludesMergedSections(t *testing.T) {
	_, user := Render(mergedContext())
	for _, want := range []string{
		"## 기억", "작가는 건조한 문체를 선호한다",
		"## 팩트 자료집", "1920년대 경성에는 전차가 다녔다", "https://example.test/tram",
		"## 추가 레퍼런스", "짧고 차가운 문장.",
	} {
		if !strings.Contains(user, want) {
			t.Errorf("user prompt missing %q", want)
		}
	}
}

// Empty sections must not leave stray headings behind.
func TestRenderOmitsEmptyMergedSections(t *testing.T) {
	c := mergedContext()
	c.Facts, c.Memories, c.References = nil, nil, nil
	_, user := Render(c)
	for _, heading := range []string{"## 기억", "## 팩트 자료집", "## 추가 레퍼런스"} {
		if strings.Contains(user, heading) {
			t.Errorf("empty section rendered heading %q", heading)
		}
	}
}

// The pre-existing ContextSelection toggles must actually clear the sections.
func TestContextSelectionClearsMergedSections(t *testing.T) {
	off := false
	c := mergedContext()
	c.Options.Context = ContextSelection{Facts: &off, Memories: &off, References: &off}
	got := ApplyContextSelection(c)
	if got.Facts != nil || got.Memories != nil || got.References != nil {
		t.Fatalf("disabled sections survived: facts=%v memories=%v references=%v",
			got.Facts, got.Memories, got.References)
	}
	_, user := Render(c)
	if strings.Contains(user, "팩트 자료집") || strings.Contains(user, "## 기억") {
		t.Error("disabled sections leaked into the rendered prompt")
	}
}

// Wired sources populate BuildFull output; the bring-your-own-agent premise
// also demands a complete, error-free brief when no LLM provider exists —
// summaries may be empty, everything else must be present.
func TestBuildFullMergesSourcesWithoutProvider(t *testing.T) {
	b, nodeID := newTestBuilder(t)
	b.WithFactSource(fakeFactSource{facts: []FactBrief{{ID: "f1", Status: "verified", Claim: "사실"}}}).
		WithMemorySource(fakeMemorySource{memories: []string{"기억 한 줄"}}).
		WithReferenceSource(fakeReferenceSource{refs: []ReferenceBrief{{Title: "참고", Body: "본문"}}})

	c, err := b.BuildFull(context.Background(), nodeID, "이어서 써줘", "", Options{Language: "ko"})
	if err != nil {
		t.Fatalf("BuildFull: %v", err)
	}
	if len(c.Facts) != 1 || len(c.Memories) != 1 || len(c.References) != 1 {
		t.Fatalf("merged sections not populated: facts=%d memories=%d references=%d",
			len(c.Facts), len(c.Memories), len(c.References))
	}
	_, user := Render(c)
	if !strings.Contains(user, "기억 한 줄") || !strings.Contains(user, "사실") {
		t.Errorf("merged sections missing from rendered brief")
	}
}
