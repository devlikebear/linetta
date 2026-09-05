package companion

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/google/uuid"
)

// seedWork opens a throwaway library and creates one work in it. Every test
// below needs a project row before it can append a message — companion_messages
// has a foreign key to projects — and none of them is about how that row got
// there.
func seedWork(t *testing.T) (context.Context, *store.Store, *node.Repo, project.Project) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	p, err := project.NewRepo(st).Create(ctx, 1, project.NewInput{
		Title: "작품", Genres: []string{"fantasy"}, LengthTarget: "novel", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	return ctx, st, node.NewRepo(st), p
}

func TestHistoryRepoListScopesMessagesByScene(t *testing.T) {
	ctx, st, nodes, p := seedWork(t)
	sceneA, err := nodes.CreateRoot(ctx, p.ID, node.KindLeaf, "씬 1", "식탁 위 고지서", 10)
	if err != nil {
		t.Fatalf("create scene A: %v", err)
	}
	sceneB, err := nodes.CreateRoot(ctx, p.ID, node.KindLeaf, "씬 2", "퇴근 선언", 20)
	if err != nil {
		t.Fatalf("create scene B: %v", err)
	}

	repo := NewHistoryRepo(st.DB())
	if err := repo.Append(ctx, HistoryMessage{
		ID: "m1", ProjectID: p.ID, NodeID: sceneA.ID, RunID: "r1",
		Role: "user", Scope: HistoryScopeScene, Intent: "scene_write", Status: HistoryStatusDone,
		Content: "이 씬 작성해줘", CreatedAt: 100,
	}); err != nil {
		t.Fatalf("append scene A user: %v", err)
	}
	if err := repo.Append(ctx, HistoryMessage{
		ID: "m2", ProjectID: p.ID, NodeID: sceneA.ID, RunID: "r1",
		Role: "assistant", Scope: HistoryScopeScene, Intent: "scene_write", Status: HistoryStatusApplied,
		Content: "현재 씬 본문을 반영했습니다.", CreatedAt: 110,
	}); err != nil {
		t.Fatalf("append scene A assistant: %v", err)
	}
	if err := repo.Append(ctx, HistoryMessage{
		ID: "m3", ProjectID: p.ID, NodeID: sceneB.ID, RunID: "r2",
		Role: "user", Scope: HistoryScopeScene, Intent: "scene_write", Status: HistoryStatusDone,
		Content: "다음 씬 작성해줘", CreatedAt: 120,
	}); err != nil {
		t.Fatalf("append scene B user: %v", err)
	}

	sceneMsgs, err := repo.List(ctx, HistoryQuery{ProjectID: p.ID, NodeID: sceneA.ID, Scope: HistoryViewScene, Limit: 20})
	if err != nil {
		t.Fatalf("list scene: %v", err)
	}
	if len(sceneMsgs) != 2 {
		t.Fatalf("scene message count = %d, want 2: %+v", len(sceneMsgs), sceneMsgs)
	}
	for _, msg := range sceneMsgs {
		if msg.NodeID != sceneA.ID {
			t.Fatalf("scene list included node %q, want %q", msg.NodeID, sceneA.ID)
		}
		if msg.NodeLabel != "식탁 위 고지서" {
			t.Fatalf("node label = %q, want title", msg.NodeLabel)
		}
	}

	projectMsgs, err := repo.List(ctx, HistoryQuery{ProjectID: p.ID, Scope: HistoryViewProject, Limit: 20})
	if err != nil {
		t.Fatalf("list project: %v", err)
	}
	if len(projectMsgs) != 3 {
		t.Fatalf("project message count = %d, want 3: %+v", len(projectMsgs), projectMsgs)
	}
	if projectMsgs[2].NodeID != sceneB.ID || projectMsgs[2].NodeLabel != "퇴근 선언" {
		t.Fatalf("project list missing scene B metadata: %+v", projectMsgs[2])
	}
}

func TestList_ordersRowsWrittenInTheSameMillisecondByInsertion(t *testing.T) {
	// A turn writes assistant then tool then tool, and on a coarse clock —
	// Windows' timer granularity is ~15ms — all three land on one
	// millisecond. Ordering by the uuid id then decides at random, so the
	// panel can draw a reply before the tool chips that produced it.
	// Insertion order is the only thing that is actually true here.
	ctx, st, nodes, p := seedWork(t)
	scene, err := nodes.CreateRoot(ctx, p.ID, node.KindLeaf, "씬 1", "식탁 위 고지서", 10)
	if err != nil {
		t.Fatalf("create scene: %v", err)
	}

	repo := NewHistoryRepo(st.DB())
	const sameMillis = int64(1_000_000)
	// IDs are real uuid v4s, exactly as production rows get them, so an
	// id-based tie-break has real randomness to exploit across -count=20 runs
	// instead of the coincidental alphabetical order a hand-picked id would give.
	wantOrder := []string{"reply", "tool call 1", "tool call 2"}
	roles := []string{"assistant", "tool", "tool"}
	for i, content := range wantOrder {
		if err := repo.Append(ctx, HistoryMessage{
			ID: uuid.NewString(), ProjectID: p.ID, NodeID: scene.ID, RunID: "r1",
			Role: roles[i], Scope: HistoryScopeScene, Intent: "chat", Status: HistoryStatusDone,
			Content: content, CreatedAt: sameMillis,
		}); err != nil {
			t.Fatalf("append %q: %v", content, err)
		}
	}

	msgs, err := repo.List(ctx, HistoryQuery{ProjectID: p.ID, NodeID: scene.ID, Scope: HistoryViewScene, Limit: 20})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("message count = %d, want 3: %+v", len(msgs), msgs)
	}
	for i, want := range wantOrder {
		if msgs[i].Content != want {
			var gotContents []string
			for _, m := range msgs {
				gotContents = append(gotContents, m.Content)
			}
			t.Fatalf("order = %v, want %v", gotContents, wantOrder)
		}
	}
}
