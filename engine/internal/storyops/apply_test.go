package storyops

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/snapshot"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

func newTestService(t *testing.T) (*Service, *node.Repo, *snapshot.Repo, string, string) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	projects := project.NewRepo(st)
	nodes := node.NewRepo(st)
	snaps := snapshot.NewRepo(st)
	svc := New(projects, nodes, thread.NewRepo(st), beat.NewRepo(st),
		entity.NewRepo(st), relationship.NewRepo(st)).
		WithFacts(fact.NewRepo(st)).
		WithSnapshots(snaps)

	p, err := projects.Create(ctx, 1_000, project.NewInput{
		Title: "적용기 테스트", Genres: []string{"fantasy"}, LengthTarget: "short", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	return svc, nodes, snaps, p.ID, *p.LastOpenedNodeID
}

func now() func() int64 {
	var t int64 = 10_000
	return func() int64 { t++; return t }
}

// A structural batch applies atomically and leaves the writer one undo away
// from the pre-change outline.
func TestApplyOpsStructuralBatchAndUndo(t *testing.T) {
	svc, nodes, _, projectID, _ := newTestService(t)
	ctx := context.Background()
	clock := now()

	before, err := nodes.ListByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("list before: %v", err)
	}

	res := svc.ApplyOps(ctx, projectID, "", Proposal{
		Summary: "1부 뼈대",
		Ops: []Op{
			{Type: "create_outline_node", Ref: "p1", Kind: "container", Label: "1부"},
			{Type: "create_outline_node", Ref: "s1", Kind: "leaf", ParentNodeRef: "p1", Label: "씬 1"},
		},
	}, clock)
	if res.IsError() {
		t.Fatalf("failures = %+v", res.Failures)
	}
	if res.Applied != 2 || res.UndoBatchID == "" {
		t.Fatalf("applied = %d, undo = %q; want 2 applied with an undo id", res.Applied, res.UndoBatchID)
	}
	after, _ := nodes.ListByProject(ctx, projectID)
	if len(after) != len(before)+2 {
		t.Fatalf("node count = %d, want %d", len(after), len(before)+2)
	}

	if err := svc.UndoApply(ctx, res.UndoBatchID, clock); err != nil {
		t.Fatalf("UndoApply: %v", err)
	}
	restored, _ := nodes.ListByProject(ctx, projectID)
	if len(restored) != len(before) {
		t.Fatalf("after undo node count = %d, want %d", len(restored), len(before))
	}
	if err := svc.UndoApply(ctx, res.UndoBatchID, clock); err != ErrUndoBatchNotFound {
		t.Fatalf("second undo = %v, want ErrUndoBatchNotFound", err)
	}
}

// A structural batch with a failing op is rolled back wholesale: half a
// restructured outline is worse than none.
func TestApplyOpsRollsBackFailedStructuralBatch(t *testing.T) {
	svc, nodes, _, projectID, _ := newTestService(t)
	ctx := context.Background()

	before, _ := nodes.ListByProject(ctx, projectID)
	res := svc.ApplyOps(ctx, projectID, "", Proposal{
		Ops: []Op{
			{Type: "create_outline_node", Kind: "container", Label: "1부"},
			{Type: "delete_outline_node", NodeID: "no-such-node"},
		},
	}, now())
	if !res.IsError() || !res.RolledBack {
		t.Fatalf("result = %+v, want failure with rollback", res)
	}
	if res.Applied != 0 || res.UndoBatchID != "" {
		t.Fatalf("rolled-back result must report nothing applied: %+v", res)
	}
	after, _ := nodes.ListByProject(ctx, projectID)
	if len(after) != len(before) {
		t.Fatalf("outline changed despite rollback: %d -> %d nodes", len(before), len(after))
	}
}

// set_scene_text records a companion-before snapshot when snapshots are wired,
// so every applied body change stays one restore away.
func TestApplyOpsSetSceneTextSnapshots(t *testing.T) {
	svc, nodes, snaps, projectID, sceneID := newTestService(t)
	ctx := context.Background()
	clock := now()

	seed, err := PlainTextToTiptapDoc("원래 본문")
	if err != nil {
		t.Fatalf("seed doc: %v", err)
	}
	if err := nodes.UpdateContent(ctx, sceneID, seed, clock()); err != nil {
		t.Fatalf("seed content: %v", err)
	}

	res := svc.ApplyOps(ctx, projectID, sceneID, Proposal{
		Ops: []Op{{Type: "set_scene_text", Text: "고쳐 쓴 본문"}},
	}, clock)
	if res.IsError() {
		t.Fatalf("failures = %+v", res.Failures)
	}
	if len(res.ChangedNodes) != 1 || res.ChangedNodes[0].NodeID != sceneID {
		t.Fatalf("changed nodes = %+v", res.ChangedNodes)
	}

	got, _ := nodes.Get(ctx, sceneID)
	if !strings.Contains(*got.ContentDoc, "고쳐 쓴 본문") {
		t.Fatalf("scene body not replaced: %s", *got.ContentDoc)
	}
	list, err := snaps.ListForNode(ctx, sceneID)
	if err != nil {
		t.Fatalf("list snapshots: %v", err)
	}
	found := false
	for _, sn := range list {
		if sn.Reason == snapshot.ReasonCompanionBefore {
			found = true
		}
	}
	if !found {
		t.Fatalf("no companion-before snapshot recorded; got %+v", list)
	}
}

// Without a memory recorder the remember op must fail with a clear message,
// not a nil-pointer panic — the MCP applier is built without companion memory.
func TestApplyOpsRememberWithoutMemory(t *testing.T) {
	svc, _, _, projectID, _ := newTestService(t)
	res := svc.ApplyOps(context.Background(), projectID, "", Proposal{
		Ops: []Op{{Type: "remember", Text: "작가는 건조한 문체를 선호한다"}},
	}, now())
	if !res.IsError() {
		t.Fatal("remember without memory should fail")
	}
	if !strings.Contains(res.Failures[0].Error, "memory is not available") {
		t.Fatalf("failure = %+v, want a clear memory-unavailable message", res.Failures[0])
	}
}
