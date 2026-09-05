//go:build !mobile

package mcphost

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/note"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/storycontext"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

// fakeCurated stands in for companion.Service, which mcphost must not import.
type fakeCurated struct{ profile, notes string }

func (f fakeCurated) CuratedMemory(_ context.Context, _ string) (string, string) {
	return f.profile, f.notes
}

// newStoryContextDeps wires the real ContextBuilder over a temp store, the way
// engineapp does, and hands back the scene the brief will be built for.
func newStoryContextDeps(t *testing.T, curated storycontext.CuratedMemorySource) (context.Context, ToolDeps, string) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	mentions := mention.NewRepo(st)
	nodes := node.NewRepo(st)
	nodes.SetMentionResyncer(func(ctx context.Context, nodeID, doc string) error {
		return mentions.ResyncForNode(ctx, nodeID, mention.Collect([]byte(doc)))
	})
	projects := project.NewRepo(st)
	p, err := projects.Create(ctx, 1, project.NewInput{
		Title: "작품", Genres: []string{"fantasy"}, LengthTarget: "novel", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if p.LastOpenedNodeID == nil {
		t.Fatal("a new work must come with a scene to build a brief for")
	}
	builder := storycontext.NewContextBuilder(
		projects, nodes, mentions, thread.NewRepo(st), beat.NewRepo(st),
		note.NewRepo(st), relationship.NewRepo(st))
	if curated != nil {
		builder = builder.WithCuratedMemorySource(curated)
	}
	return ctx, ToolDeps{
		Projects: projects,
		Nodes:    nodes,
		Context:  builder,
		Source:   SourceAgent,
		Clock:    func() int64 { return 42 },
	}, *p.LastOpenedNodeID
}

// languageSettings is a store with no keychain behind it, set to one language.
func languageSettings(t *testing.T, lang string) *settings.Store {
	t.Helper()
	t.Setenv("LINETTA_HOME", t.TempDir())
	st, err := settings.NewWithSecretStore(settings.NewMemorySecretStore())
	if err != nil {
		t.Fatalf("settings: %v", err)
	}
	if _, err := st.Set(context.Background(), settings.Patch{Language: &lang}); err != nil {
		t.Fatalf("set language %q: %v", lang, err)
	}
	return st
}

// The brief is the only thing an external client reads, and storycontext falls
// back to Korean when Options.Language is empty. Until the tool passed the app
// language through, every client got a Korean brief no matter what the writer
// had the app set to — and the English and Japanese memory frames could never
// render at all.
func TestGetStoryContextRendersInTheAppLanguage(t *testing.T) {
	ctx, d, nodeID := newStoryContextDeps(t, fakeCurated{profile: "no em dashes"})
	d.Settings = languageSettings(t, "en")

	res, out, err := d.getStoryContext(ctx, nil, getStoryContextInput{NodeID: nodeID})
	if err != nil || (res != nil && res.IsError) {
		t.Fatalf("getStoryContext: err=%v res=%+v", err, res)
	}
	for _, want := range []string{
		"## Project Settings",
		"## What is remembered about this writer and this work",
		"These are notes recorded for this writer and this work.",
	} {
		if !strings.Contains(out.Brief, want) {
			t.Errorf("English brief missing %q; got:\n%s", want, out.Brief)
		}
	}
	for _, gone := range []string{"## 작품 설정", "기록되어 온 메모"} {
		if strings.Contains(out.Brief, gone) {
			t.Errorf("English brief still carries the Korean %q", gone)
		}
	}
}

// The Korean default has to survive the wiring: an unset language is still ko.
func TestGetStoryContextStillDefaultsToKorean(t *testing.T) {
	ctx, d, nodeID := newStoryContextDeps(t, fakeCurated{profile: "줄표 쓰지 않기"})
	d.Settings = languageSettings(t, "ko")

	_, out, err := d.getStoryContext(ctx, nil, getStoryContextInput{NodeID: nodeID})
	if err != nil {
		t.Fatalf("getStoryContext: %v", err)
	}
	if !strings.Contains(out.Brief, "## 작가와 작품에 대해 기억해 둔 것") {
		t.Errorf("Korean brief missing the memory heading; got:\n%s", out.Brief)
	}
}

// Nil Settings is a build with no store open. The tool must still answer.
func TestGetStoryContextToleratesNilSettings(t *testing.T) {
	ctx, d, nodeID := newStoryContextDeps(t, nil)

	res, out, err := d.getStoryContext(ctx, nil, getStoryContextInput{NodeID: nodeID})
	if err != nil || (res != nil && res.IsError) {
		t.Fatalf("getStoryContext with nil Settings: err=%v res=%+v", err, res)
	}
	if strings.TrimSpace(out.Brief) == "" {
		t.Error("the brief must still build without a settings store")
	}
}

// The report is the client's inventory of the brief. A writer profile in the
// brief with "memories" listed empty tells a client to skip a section it can
// see, which is worse than saying nothing.
func TestSectionReportCountsTheCuratedMemories(t *testing.T) {
	cases := []struct {
		name string
		c    storycontext.Context
		want bool
	}{
		{"nothing", storycontext.Context{}, false},
		{"recall only", storycontext.Context{Memories: []string{"오래된 기억"}}, true},
		{"profile only", storycontext.Context{WriterProfile: "줄표 쓰지 않기"}, true},
		{"notes only", storycontext.Context{WorkNotes: "민준은 존댓말"}, true},
		{"blank profile", storycontext.Context{WriterProfile: "   \n"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			included, empty := sectionReport(tc.c)
			if got := slices.Contains(included, "memories"); got != tc.want {
				t.Errorf("included has memories = %v, want %v (included=%v empty=%v)",
					got, tc.want, included, empty)
			}
			if slices.Contains(empty, "memories") == tc.want {
				t.Errorf("memories is in both or neither list: included=%v empty=%v", included, empty)
			}
		})
	}
}

// The toggle is one switch for everything memory. With it off, the report must
// say memories is empty even though the Context still carries the documents.
func TestSectionReportHonoursTheMemoriesToggle(t *testing.T) {
	off := false
	c := storycontext.Context{
		WriterProfile: "줄표 쓰지 않기",
		WorkNotes:     "민준은 존댓말",
		Memories:      []string{"오래된 기억"},
		Options:       storycontext.Options{Context: storycontext.ContextSelection{Memories: &off}},
	}
	included, empty := sectionReport(c)
	if slices.Contains(included, "memories") || !slices.Contains(empty, "memories") {
		t.Errorf("memories survived the toggle: included=%v empty=%v", included, empty)
	}
}
