package forecast

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/direction"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

// testEnv is the same shape proposals' own tests build: one isolated SQLite
// file, every repo forecast.Deps needs, and a project with its default
// opening scene — Phase 3's newTestEnv convention.
type testEnv struct {
	st            *store.Store
	deps          Deps
	projects      *project.Repo
	nodes         *node.Repo
	entities      *entity.Repo
	relationships *relationship.Repo
	facts         *fact.Repo
	threads       *thread.Repo
	beats         *beat.Repo
	direction     *direction.Repo
	project       project.Project
	sceneID       string
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	projects := project.NewRepo(st)
	nodes := node.NewRepo(st)
	entities := entity.NewRepo(st)
	relationships := relationship.NewRepo(st)
	facts := fact.NewRepo(st)
	threads := thread.NewRepo(st)
	beats := beat.NewRepo(st)
	dir := direction.NewRepo(st, nodes, projects)

	p, err := projects.Create(ctx, 1_000, project.NewInput{
		Title: "예측 테스트", Genres: []string{"fantasy"}, LengthTarget: "short", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	sceneID := *p.LastOpenedNodeID

	return &testEnv{
		st: st,
		deps: Deps{
			Projects: projects, Nodes: nodes, Entities: entities, Relationships: relationships,
			Facts: facts, Threads: threads, Beats: beats, Direction: dir,
		},
		projects: projects, nodes: nodes, entities: entities, relationships: relationships,
		facts: facts, threads: threads, beats: beats, direction: dir,
		project: p, sceneID: sceneID,
	}
}

func (e *testEnv) writeScene(t *testing.T, nodeID, text string, version int64) {
	t.Helper()
	if err := e.nodes.UpdateContent(context.Background(), nodeID, plainDoc(text), 1_000+version); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}
}

// plainDoc wraps text as a minimal Tiptap doc — the same shape
// storyops.PlainTextToTiptapDoc produces, inlined here so this package does
// not need to import storyops just for test fixtures.
func plainDoc(text string) string {
	return `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + text + `"}]}]}`
}
