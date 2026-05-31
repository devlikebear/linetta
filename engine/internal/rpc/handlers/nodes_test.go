package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/snapshot"
	"github.com/devlikebear/linetta/engine/internal/store"
)

type nodeFixture struct {
	store *store.Store
	proj  *project.Repo
	nodes *node.Repo
	snaps *snapshot.Repo
	pID   string
	nID   string
}

func newNodeFixture(t *testing.T) nodeFixture {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	pr := project.NewRepo(s)
	p, err := pr.Create(context.Background(), 1000, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return nodeFixture{
		store: s, proj: pr, nodes: node.NewRepo(s), snaps: snapshot.NewRepo(s),
		pID: p.ID, nID: *p.LastOpenedNodeID,
	}
}

func TestGetNodeHandler(t *testing.T) {
	f := newNodeFixture(t)
	h := GetNode(f.nodes)
	res, err := h(context.Background(), json.RawMessage(`{"id":"`+f.nID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var n node.Node
	if err := json.Unmarshal(res, &n); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if n.Label != "씬 1" {
		t.Errorf("label = %q", n.Label)
	}
}

func TestUpdateNodeContentHandler_createsAutosaveSnapshotOnFirstSave(t *testing.T) {
	f := newNodeFixture(t)
	clock := int64(10_000)
	h := UpdateNodeContent(f.nodes, f.snaps, func() int64 { return clock }, nil)

	res, err := h(context.Background(), json.RawMessage(`{"id":"`+f.nID+`","doc":"{\"type\":\"doc\",\"content\":[{\"type\":\"paragraph\",\"content\":[{\"type\":\"text\",\"text\":\"파도 소리\"}]}]}"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var out node.Node
	if err := json.Unmarshal(res, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.WordCount != 5 {
		t.Errorf("word_count = %d, want 5", out.WordCount)
	}

	// First save → snapshot must be created.
	at, ok, err := f.snaps.LatestAutosaveTime(context.Background(), f.nID)
	if err != nil || !ok {
		t.Fatalf("expected an autosave snapshot to exist; ok=%v err=%v", ok, err)
	}
	if at != 10_000 {
		t.Errorf("autosave at = %d, want 10000", at)
	}
}

func TestUpdateNodeContentHandler_noSnapshotWithin60s(t *testing.T) {
	f := newNodeFixture(t)
	clock := int64(10_000)
	h := UpdateNodeContent(f.nodes, f.snaps, func() int64 { return clock }, nil)

	if _, err := h(context.Background(), json.RawMessage(`{"id":"`+f.nID+`","doc":"{}"}`)); err != nil {
		t.Fatalf("save 1: %v", err)
	}
	clock = 30_000 // 20 seconds later
	if _, err := h(context.Background(), json.RawMessage(`{"id":"`+f.nID+`","doc":"{}"}`)); err != nil {
		t.Fatalf("save 2: %v", err)
	}
	// Should still only be the first snapshot (no new one within 60s).
	at, _, _ := f.snaps.LatestAutosaveTime(context.Background(), f.nID)
	if at != 10_000 {
		t.Errorf("autosave at = %d, want 10000 (snapshot should not have been refreshed)", at)
	}

	clock = 80_000 // > 60s after last snapshot
	if _, err := h(context.Background(), json.RawMessage(`{"id":"`+f.nID+`","doc":"{}"}`)); err != nil {
		t.Fatalf("save 3: %v", err)
	}
	at, _, _ = f.snaps.LatestAutosaveTime(context.Background(), f.nID)
	if at != 80_000 {
		t.Errorf("autosave at = %d, want 80000", at)
	}
}

func TestSetLastOpenedHandler(t *testing.T) {
	f := newNodeFixture(t)
	h := SetLastOpened(f.nodes, func() int64 { return 9999 })
	params := json.RawMessage(`{"project_id":"` + f.pID + `","node_id":"` + f.nID + `"}`)
	if _, err := h(context.Background(), params); err != nil {
		t.Fatalf("handler: %v", err)
	}
	got, err := f.proj.Get(context.Background(), f.pID)
	if err != nil {
		t.Fatalf("project Get: %v", err)
	}
	if got.LastOpenedNodeID == nil || *got.LastOpenedNodeID != f.nID {
		t.Errorf("last_opened = %v", got.LastOpenedNodeID)
	}
}

func TestListTreeHandler(t *testing.T) {
	f := newNodeFixture(t)
	chapter, _ := f.nodes.CreateSibling(context.Background(), f.nID, "container", "1장", "", 2000)
	_, _ = f.nodes.CreateChild(context.Background(), chapter.ID, "leaf", "씬 A", "", 3000)

	h := ListTree(f.nodes)
	res, err := h(context.Background(), json.RawMessage(`{"project_id":"`+f.pID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got []node.Node
	if err := json.Unmarshal(res, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
}

func TestCreateSiblingHandler(t *testing.T) {
	f := newNodeFixture(t)
	h := CreateSibling(f.nodes, func() int64 { return 1234 })
	params := json.RawMessage(`{"reference_id":"` + f.nID + `","kind":"leaf","label":"씬 2","title":""}`)
	res, err := h(context.Background(), params)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var n node.Node
	_ = json.Unmarshal(res, &n)
	if n.Label != "씬 2" || n.Kind != "leaf" {
		t.Errorf("got %+v", n)
	}
	if n.CreatedAt != 1234 {
		t.Errorf("clock not injected: %d", n.CreatedAt)
	}
}

func TestCreateChildHandler(t *testing.T) {
	f := newNodeFixture(t)
	chapter, _ := f.nodes.CreateSibling(context.Background(), f.nID, "container", "1장", "", 2000)
	h := CreateChild(f.nodes, func() int64 { return 5000 })
	params := json.RawMessage(`{"parent_id":"` + chapter.ID + `","kind":"leaf","label":"씬 A","title":""}`)
	res, err := h(context.Background(), params)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var n node.Node
	_ = json.Unmarshal(res, &n)
	if n.ParentID == nil || *n.ParentID != chapter.ID {
		t.Errorf("parent mismatch: %v", n.ParentID)
	}
}

func TestRenameHandler(t *testing.T) {
	f := newNodeFixture(t)
	h := RenameNode(f.nodes, func() int64 { return 9999 })
	if _, err := h(context.Background(), json.RawMessage(`{"id":"`+f.nID+`","label":"프롤로그","title":"별이 떨어지는 밤"}`)); err != nil {
		t.Fatalf("handler: %v", err)
	}
	got, _ := f.nodes.Get(context.Background(), f.nID)
	if got.Label != "프롤로그" || got.Title != "별이 떨어지는 밤" {
		t.Errorf("rename failed: %+v", got)
	}
}

func TestDeleteHandler(t *testing.T) {
	f := newNodeFixture(t)
	// Create a second leaf so the project still has a node after the delete.
	other, _ := f.nodes.CreateSibling(context.Background(), f.nID, "leaf", "씬 2", "", 2000)
	_ = other

	h := DeleteNode(f.nodes, func() int64 { return 1 })
	if _, err := h(context.Background(), json.RawMessage(`{"id":"`+f.nID+`"}`)); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if _, err := f.nodes.Get(context.Background(), f.nID); err != node.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestMoveHandlers(t *testing.T) {
	f := newNodeFixture(t)
	second, _ := f.nodes.CreateSibling(context.Background(), f.nID, "leaf", "씬 2", "", 2000)

	up := MoveUp(f.nodes, func() int64 { return 3000 })
	if _, err := up(context.Background(), json.RawMessage(`{"id":"`+second.ID+`"}`)); err != nil {
		t.Fatalf("MoveUp handler: %v", err)
	}
	tree, _ := f.nodes.ListByProject(context.Background(), f.pID)
	if tree[0].Label != "씬 2" || tree[1].Label != "씬 1" {
		t.Errorf("order after MoveUp: %q,%q", tree[0].Label, tree[1].Label)
	}

	down := MoveDown(f.nodes, func() int64 { return 4000 })
	if _, err := down(context.Background(), json.RawMessage(`{"id":"`+second.ID+`"}`)); err != nil {
		t.Fatalf("MoveDown handler: %v", err)
	}
	tree, _ = f.nodes.ListByProject(context.Background(), f.pID)
	if tree[0].Label != "씬 1" || tree[1].Label != "씬 2" {
		t.Errorf("order after MoveDown: %q,%q", tree[0].Label, tree[1].Label)
	}
}

func TestUpdateNodeContentHandler_callsPostUpdateAfterSuccess(t *testing.T) {
	f := newNodeFixture(t)
	var got []string
	hook := func(id string) { got = append(got, id) }
	h := UpdateNodeContent(f.nodes, f.snaps, func() int64 { return 10_000 }, hook)
	if _, err := h(context.Background(), json.RawMessage(`{"id":"`+f.nID+`","doc":"{\"type\":\"doc\"}"}`)); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if len(got) != 1 || got[0] != f.nID {
		t.Errorf("postUpdate calls = %v, want [%q]", got, f.nID)
	}
}

func TestUpdateNodeContentHandler_doesNotCallPostUpdateOnError(t *testing.T) {
	f := newNodeFixture(t)
	called := 0
	hook := func(id string) { called++ }
	h := UpdateNodeContent(f.nodes, f.snaps, func() int64 { return 10_000 }, hook)
	if _, err := h(context.Background(), json.RawMessage(`{"id":"no-such","doc":"{}"}`)); err == nil {
		t.Fatal("expected error")
	}
	if called != 0 {
		t.Errorf("postUpdate called %d times on error", called)
	}
}

func TestNodeHandlers_returnInvalidParamsForIntegrityErrors(t *testing.T) {
	f := newNodeFixture(t)
	ctx := context.Background()

	createSibling := CreateSibling(f.nodes, func() int64 { return 1 })
	if _, err := createSibling(ctx, json.RawMessage(`{"reference_id":"`+f.nID+`","kind":"folder"}`)); !rpcErrorCode(err, rpc.CodeInvalidParams) {
		t.Fatalf("CreateSibling err = %v, want invalid params", err)
	}

	chapter, err := f.nodes.CreateSibling(ctx, f.nID, "container", "1장", "", 2)
	if err != nil {
		t.Fatalf("CreateSibling container: %v", err)
	}
	updateContent := UpdateNodeContent(f.nodes, f.snaps, func() int64 { return 3 }, nil)
	if _, err := updateContent(ctx, json.RawMessage(`{"id":"`+chapter.ID+`","doc":"{}"}`)); !rpcErrorCode(err, rpc.CodeInvalidParams) {
		t.Fatalf("UpdateContent err = %v, want invalid params", err)
	}

	otherProject, err := f.proj.Create(ctx, 4, project.NewInput{Title: "Other", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first"})
	if err != nil {
		t.Fatalf("create other project: %v", err)
	}
	setLastOpened := SetLastOpened(f.nodes, func() int64 { return 5 })
	if _, err := setLastOpened(ctx, json.RawMessage(`{"project_id":"`+f.pID+`","node_id":"`+*otherProject.LastOpenedNodeID+`"}`)); !rpcErrorCode(err, rpc.CodeInvalidParams) {
		t.Fatalf("SetLastOpened err = %v, want invalid params", err)
	}
}

func rpcErrorCode(err error, code int) bool {
	if err == nil {
		return false
	}
	me, ok := err.(*rpc.MethodError)
	return ok && me.Code == code
}
