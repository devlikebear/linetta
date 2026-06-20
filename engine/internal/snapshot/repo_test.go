package snapshot

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func newRepoWithNode(t *testing.T) (*Repo, string) {
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

func TestCreate_returnsRowWithGeneratedID(t *testing.T) {
	r, nodeID := newRepoWithNode(t)
	got, err := r.Create(context.Background(), nodeID, `{"type":"doc"}`, ReasonManual, 5000)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.ID == "" {
		t.Error("missing id")
	}
	if got.NodeID != nodeID {
		t.Errorf("node_id = %q", got.NodeID)
	}
	if got.Reason != ReasonManual {
		t.Errorf("reason = %q", got.Reason)
	}
	if got.CreatedAt != 5000 {
		t.Errorf("created_at = %d", got.CreatedAt)
	}
}

func TestCreate_rejectsInvalidReason(t *testing.T) {
	r, nodeID := newRepoWithNode(t)
	_, err := r.Create(context.Background(), nodeID, `{"type":"doc"}`, "bad", 5000)
	if err != ErrInvalidReason {
		t.Errorf("err = %v, want ErrInvalidReason", err)
	}
}

func TestLatestForNode_returnsMostRecent(t *testing.T) {
	r, nodeID := newRepoWithNode(t)
	ctx := context.Background()
	_, _ = r.Create(ctx, nodeID, `{"v":1}`, ReasonAutosave, 1000)
	_, _ = r.Create(ctx, nodeID, `{"v":2}`, ReasonAutosave, 2000)
	_, _ = r.Create(ctx, nodeID, `{"v":3}`, ReasonManual, 3000)

	latest, err := r.LatestForNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("LatestForNode: %v", err)
	}
	if latest.CreatedAt != 3000 || latest.ContentDoc != `{"v":3}` {
		t.Errorf("latest = %+v, want v=3", latest)
	}
}

func TestLatestForNode_emptyReturnsNotFound(t *testing.T) {
	r, _ := newRepoWithNode(t)
	_, err := r.LatestForNode(context.Background(), "no-such-node")
	if err != ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestCreateIfChanged_skipsDuplicate(t *testing.T) {
	ctx := context.Background()
	r, nodeID := newRepoWithNode(t)

	first, created, err := r.CreateIfChanged(ctx, nodeID, `{"v":1}`, ReasonAutosave, 1000)
	if err != nil || !created {
		t.Fatalf("first CreateIfChanged: created=%v err=%v", created, err)
	}
	if first.ID == "" {
		t.Fatalf("expected a created snapshot")
	}

	_, created2, err := r.CreateIfChanged(ctx, nodeID, `{"v":1}`, ReasonAutosave, 2000)
	if err != nil {
		t.Fatalf("second CreateIfChanged: %v", err)
	}
	if created2 {
		t.Errorf("expected skip on identical content, got created=true")
	}

	_, created3, err := r.CreateIfChanged(ctx, nodeID, `{"v":2}`, ReasonAutosave, 3000)
	if err != nil || !created3 {
		t.Errorf("expected create on changed content: created=%v err=%v", created3, err)
	}
}

func TestListForNode_orderedDesc(t *testing.T) {
	r, nodeID := newRepoWithNode(t)
	ctx := context.Background()
	_, _ = r.Create(ctx, nodeID, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"오래된"}]}]}`, ReasonAutosave, 1000)
	_, _ = r.Create(ctx, nodeID, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"중간"}]}]}`, ReasonManual, 2000)
	_, _ = r.Create(ctx, nodeID, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"새 거"}]}]}`, ReasonCompanionBefore, 3000)

	got, err := r.ListForNode(context.Background(), nodeID)
	if err != nil {
		t.Fatalf("ListForNode: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Reason != ReasonCompanionBefore || got[2].Reason != ReasonAutosave {
		t.Errorf("ordering wrong: %+v", got)
	}
	if got[0].DocPreview != "새 거\n" {
		t.Errorf("preview = %q", got[0].DocPreview)
	}
}

func TestGetByID(t *testing.T) {
	r, nodeID := newRepoWithNode(t)
	created, _ := r.Create(context.Background(), nodeID, `{"v":1}`, ReasonManual, 1000)
	got, err := r.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("id mismatch: %q vs %q", got.ID, created.ID)
	}
}
