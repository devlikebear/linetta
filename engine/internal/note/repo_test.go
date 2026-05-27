package note

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func newRepo(t *testing.T) (*Repo, string) {
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
		t.Fatalf("create project: %v", err)
	}
	return NewRepo(s), *p.LastOpenedNodeID
}

func TestRepo_Create_thenGet(t *testing.T) {
	r, nodeID := newRepo(t)
	ctx := context.Background()
	n, err := r.Create(ctx, NewInput{NodeID: nodeID, Anchor: 7, Body: "여기 분위기 바꾸기"}, 5000)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if n.ID == "" || n.Anchor != 7 || n.Body != "여기 분위기 바꾸기" || n.CreatedAt != 5000 {
		t.Errorf("unexpected note: %+v", n)
	}
	got, err := r.Get(ctx, n.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Body != "여기 분위기 바꾸기" {
		t.Errorf("Get = %+v", got)
	}
}

func TestRepo_Create_rejectsEmpty(t *testing.T) {
	r, nodeID := newRepo(t)
	if _, err := r.Create(context.Background(), NewInput{NodeID: "", Anchor: 0, Body: "x"}, 1); err == nil {
		t.Error("expected error for empty node_id")
	}
	if _, err := r.Create(context.Background(), NewInput{NodeID: nodeID, Anchor: 0, Body: ""}, 1); err == nil {
		t.Error("expected error for empty body")
	}
}

func TestRepo_ListForNode_orderedByAnchor(t *testing.T) {
	r, nodeID := newRepo(t)
	ctx := context.Background()
	_, _ = r.Create(ctx, NewInput{NodeID: nodeID, Anchor: 30, Body: "C"}, 1)
	_, _ = r.Create(ctx, NewInput{NodeID: nodeID, Anchor: 10, Body: "A"}, 2)
	_, _ = r.Create(ctx, NewInput{NodeID: nodeID, Anchor: 20, Body: "B"}, 3)
	got, err := r.ListForNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("ListForNode: %v", err)
	}
	if len(got) != 3 || got[0].Body != "A" || got[1].Body != "B" || got[2].Body != "C" {
		t.Errorf("order = %+v", got)
	}
}

func TestRepo_Update_bodyOnly(t *testing.T) {
	r, nodeID := newRepo(t)
	ctx := context.Background()
	n, _ := r.Create(ctx, NewInput{NodeID: nodeID, Anchor: 5, Body: "원본"}, 1)
	if err := r.Update(ctx, UpdateInput{ID: n.ID, Body: "수정됨"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := r.Get(ctx, n.ID)
	if got.Body != "수정됨" || got.Anchor != 5 {
		t.Errorf("Update changed too much: %+v", got)
	}
}

func TestRepo_Delete(t *testing.T) {
	r, nodeID := newRepo(t)
	ctx := context.Background()
	n, _ := r.Create(ctx, NewInput{NodeID: nodeID, Anchor: 1, Body: "x"}, 1)
	if err := r.Delete(ctx, n.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := r.Get(ctx, n.ID); err == nil {
		t.Error("expected ErrNotFound after delete")
	}
}

func TestRepo_Get_notFound(t *testing.T) {
	r, _ := newRepo(t)
	if _, err := r.Get(context.Background(), "no-such-id"); err == nil {
		t.Error("expected ErrNotFound")
	}
}
