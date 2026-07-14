package handlers

import (
	"context"
	"encoding/json"
	"errors"
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

func TestSetStatusHandler(t *testing.T) {
	f := newNodeFixture(t)
	h := SetNodeStatus(f.nodes, func() int64 { return 9999 })
	params := json.RawMessage(`{"id":"` + f.nID + `","status":"published"}`)
	if _, err := h(context.Background(), params); err != nil {
		t.Fatalf("handler: %v", err)
	}
	got, err := f.nodes.Get(context.Background(), f.nID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != node.StatusPublished {
		t.Errorf("status = %q, want published", got.Status)
	}
}

func TestSetStatusHandler_rejectsInvalidStatus(t *testing.T) {
	f := newNodeFixture(t)
	h := SetNodeStatus(f.nodes, func() int64 { return 9999 })
	_, err := h(context.Background(), json.RawMessage(`{"id":"`+f.nID+`","status":"queued"}`))
	if err == nil {
		t.Fatal("expected error")
	}
	var methodErr *rpc.MethodError
	if !errors.As(err, &methodErr) || methodErr.Code != rpc.CodeInvalidParams {
		t.Fatalf("err = %v, want invalid params", err)
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

func TestMoveToParentHandler(t *testing.T) {
	f := newNodeFixture(t)
	ctx := context.Background()
	part, _ := f.nodes.CreateSibling(ctx, f.nID, "container", "1부", "", 2000)
	chapter, _ := f.nodes.CreateChild(ctx, part.ID, "container", "1장", "", 3000)

	h := MoveToParent(f.nodes, func() int64 { return 4000 })
	if _, err := h(ctx, json.RawMessage(`{"id":"`+f.nID+`","parent_id":"`+chapter.ID+`"}`)); err != nil {
		t.Fatalf("MoveToParent handler: %v", err)
	}

	got, _ := f.nodes.Get(ctx, f.nID)
	if got.ParentID == nil || *got.ParentID != chapter.ID {
		t.Fatalf("parent = %v, want %s", got.ParentID, chapter.ID)
	}
}

func TestMoveToHandler(t *testing.T) {
	f := newNodeFixture(t)
	ctx := context.Background()
	second, _ := f.nodes.CreateSibling(ctx, f.nID, "leaf", "씬 2", "", 2000)

	payload, _ := json.Marshal(map[string]any{
		"id":        second.ID,
		"parent_id": nil,
		"ordinal":   0,
	})
	h := MoveTo(f.nodes, func() int64 { return 3000 })
	if _, err := h(ctx, payload); err != nil {
		t.Fatalf("MoveTo handler: %v", err)
	}

	tree, _ := f.nodes.ListByProject(ctx, f.pID)
	if tree[0].ID != second.ID || tree[1].ID != f.nID {
		t.Fatalf("order after MoveTo = %q,%q", tree[0].Label, tree[1].Label)
	}
}

func TestMoveToRootHandler(t *testing.T) {
	f := newNodeFixture(t)
	ctx := context.Background()
	part, _ := f.nodes.CreateSibling(ctx, f.nID, "container", "1부", "", 2000)
	chapter, _ := f.nodes.CreateChild(ctx, part.ID, "container", "1장", "경계의 틈", 3000)

	h := MoveToRoot(f.nodes, func() int64 { return 4000 })
	if _, err := h(ctx, json.RawMessage(`{"id":"`+chapter.ID+`"}`)); err != nil {
		t.Fatalf("MoveToRoot handler: %v", err)
	}

	got, _ := f.nodes.Get(ctx, chapter.ID)
	if got.ParentID != nil {
		t.Fatalf("parent = %v, want root", got.ParentID)
	}
	if got.Title != "경계의 틈" {
		t.Fatalf("title should be preserved: %+v", got)
	}
}

func TestConvertToContainerHandler(t *testing.T) {
	f := newNodeFixture(t)
	ctx := context.Background()
	leaf, _ := f.nodes.CreateSibling(ctx, f.nID, "leaf", "1장 - 경계의 틈", "표시 제목", 2000)

	h := ConvertToContainer(f.nodes, func() int64 { return 3000 })
	if _, err := h(ctx, json.RawMessage(`{"id":"`+leaf.ID+`"}`)); err != nil {
		t.Fatalf("ConvertToContainer handler: %v", err)
	}

	got, _ := f.nodes.Get(ctx, leaf.ID)
	if got.Kind != node.KindContainer || got.Title != "표시 제목" {
		t.Fatalf("converted node mismatch: %+v", got)
	}
}

func TestRestoreOutlineHandler(t *testing.T) {
	f := newNodeFixture(t)
	ctx := context.Background()
	chapter, _ := f.nodes.CreateSibling(ctx, f.nID, "container", "1장", "", 2000)
	snapshot, _ := f.nodes.ListByProject(ctx, f.pID)
	_ = f.nodes.MoveToParent(ctx, f.nID, chapter.ID, 3000)

	payload, _ := json.Marshal(map[string]any{
		"project_id": f.pID,
		"nodes":      snapshot,
	})
	h := RestoreOutline(f.nodes, func() int64 { return 4000 })
	if _, err := h(ctx, payload); err != nil {
		t.Fatalf("RestoreOutline handler: %v", err)
	}
	got, _ := f.nodes.Get(ctx, f.nID)
	if got.ParentID != nil {
		t.Fatalf("node should be restored to root: %+v", got)
	}
}

func TestUpdateNodeContentHandler_callsPostUpdateAfterSuccess(t *testing.T) {
	f := newNodeFixture(t)
	var got []string
	hook := func(id string) { got = append(got, id) }
	h := UpdateNodeContent(f.nodes, func() int64 { return 10_000 }, hook)
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
	h := UpdateNodeContent(f.nodes, func() int64 { return 10_000 }, hook)
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
	updateContent := UpdateNodeContent(f.nodes, func() int64 { return 3 }, nil)
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

func TestUpdateNodeContent_returnsConflictForStaleVersion(t *testing.T) {
	f := newNodeFixture(t)
	h := UpdateNodeContent(f.nodes, func() int64 { return 10_000 }, nil)
	ctx := context.Background()

	if _, err := h(ctx, json.RawMessage(`{"id":"`+f.nID+`","doc":"{}","expected_content_version":0}`)); err != nil {
		t.Fatalf("first update: %v", err)
	}
	_, err := h(ctx, json.RawMessage(`{"id":"`+f.nID+`","doc":"{\"stale\":true}","expected_content_version":0}`))
	if !rpcErrorCode(err, rpc.CodeContentConflict) {
		t.Fatalf("stale update err = %v, want content conflict", err)
	}
}

func rpcErrorCode(err error, code int) bool {
	if err == nil {
		return false
	}
	me, ok := err.(*rpc.MethodError)
	return ok && me.Code == code
}
