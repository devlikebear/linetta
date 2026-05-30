package beat

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

func openFixture(t *testing.T) (*store.Store, project.Project, thread.Thread) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	pr := project.NewRepo(s)
	p, _ := pr.Create(context.Background(), 1, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first",
	})
	tr := thread.NewRepo(s)
	th, _ := tr.Create(context.Background(), thread.NewInput{ProjectID: p.ID, Name: "T"})
	return s, p, th
}

func TestRepo_Create_assignsAscendingOrdinals(t *testing.T) {
	s, p, th := openFixture(t)
	r := NewRepo(s)
	ctx := context.Background()
	b1, err := r.Create(ctx, NewInput{ThreadID: th.ID, NodeID: p.LastOpenedNodeID, Label: "첫 마디", Intensity: 1})
	if err != nil {
		t.Fatalf("Create b1: %v", err)
	}
	b2, err := r.Create(ctx, NewInput{ThreadID: th.ID, Label: "둘째 마디", Intensity: 2})
	if err != nil {
		t.Fatalf("Create b2: %v", err)
	}
	if b1.Ordinal != 1 || b2.Ordinal != 2 {
		t.Errorf("ordinals = %d,%d want 1,2", b1.Ordinal, b2.Ordinal)
	}
	if b2.NodeID != nil {
		t.Errorf("NodeID = %v, want nil", b2.NodeID)
	}
}

func TestRepo_Create_intensityClampedAndDefaulted(t *testing.T) {
	s, _, th := openFixture(t)
	r := NewRepo(s)
	ctx := context.Background()
	b0, _ := r.Create(ctx, NewInput{ThreadID: th.ID, Label: "기본"})
	if b0.Intensity != 1 {
		t.Errorf("default intensity = %d, want 1", b0.Intensity)
	}
	bHi, _ := r.Create(ctx, NewInput{ThreadID: th.ID, Label: "초과", Intensity: 99})
	if bHi.Intensity != 3 {
		t.Errorf("clamp-high = %d, want 3", bHi.Intensity)
	}
}

func TestRepo_ListByThread_ordered(t *testing.T) {
	s, _, th := openFixture(t)
	r := NewRepo(s)
	ctx := context.Background()
	_, _ = r.Create(ctx, NewInput{ThreadID: th.ID, Label: "1"})
	_, _ = r.Create(ctx, NewInput{ThreadID: th.ID, Label: "2"})
	_, _ = r.Create(ctx, NewInput{ThreadID: th.ID, Label: "3"})
	got, err := r.ListByThread(ctx, th.ID)
	if err != nil {
		t.Fatalf("ListByThread: %v", err)
	}
	if len(got) != 3 || got[0].Label != "1" || got[2].Label != "3" {
		t.Errorf("order = %+v", got)
	}
}

func TestRepo_ListByNode_returnsBeatsFromManyThreads(t *testing.T) {
	s, p, th := openFixture(t)
	tr := thread.NewRepo(s)
	thOther, _ := tr.Create(context.Background(), thread.NewInput{ProjectID: p.ID, Name: "다른 스토리"})
	r := NewRepo(s)
	ctx := context.Background()
	nodeID := *p.LastOpenedNodeID
	_, _ = r.Create(ctx, NewInput{ThreadID: th.ID, NodeID: &nodeID, Label: "A"})
	_, _ = r.Create(ctx, NewInput{ThreadID: thOther.ID, NodeID: &nodeID, Label: "B"})
	_, _ = r.Create(ctx, NewInput{ThreadID: th.ID, Label: "Z"}) // unbound, must NOT appear
	got, err := r.ListByNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("ListByNode: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len = %d, want 2", len(got))
	}
}

func TestRepo_Update_intensityAndLabel(t *testing.T) {
	s, _, th := openFixture(t)
	r := NewRepo(s)
	ctx := context.Background()
	b, _ := r.Create(ctx, NewInput{ThreadID: th.ID, Label: "원본", Intensity: 1})
	if err := r.Update(ctx, UpdateInput{ID: b.ID, Label: "수정", Intensity: 3}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := r.Get(ctx, b.ID)
	if got.Label != "수정" || got.Intensity != 3 {
		t.Errorf("update missed: %+v", got)
	}
}

func TestRepo_Reorder_rewritesOrdinals(t *testing.T) {
	s, _, th := openFixture(t)
	r := NewRepo(s)
	ctx := context.Background()
	b1, _ := r.Create(ctx, NewInput{ThreadID: th.ID, Label: "1"})
	b2, _ := r.Create(ctx, NewInput{ThreadID: th.ID, Label: "2"})
	b3, _ := r.Create(ctx, NewInput{ThreadID: th.ID, Label: "3"})
	if err := r.Reorder(ctx, th.ID, []string{b3.ID, b1.ID, b2.ID}); err != nil {
		t.Fatalf("Reorder: %v", err)
	}
	got, _ := r.ListByThread(ctx, th.ID)
	if got[0].ID != b3.ID || got[1].ID != b1.ID || got[2].ID != b2.ID {
		t.Errorf("post-reorder = %+v", got)
	}
	if got[0].Ordinal != 1 || got[1].Ordinal != 2 || got[2].Ordinal != 3 {
		t.Errorf("ordinals = %d,%d,%d", got[0].Ordinal, got[1].Ordinal, got[2].Ordinal)
	}
}

func TestRepo_Reorder_rejectsIncompletePermutation(t *testing.T) {
	s, _, th := openFixture(t)
	r := NewRepo(s)
	ctx := context.Background()
	b1, _ := r.Create(context.Background(), NewInput{ThreadID: th.ID, Label: "1"})
	_, _ = r.Create(ctx, NewInput{ThreadID: th.ID, Label: "2"})
	if err := r.Reorder(ctx, th.ID, []string{b1.ID}); err == nil {
		t.Error("expected error on partial permutation")
	}
}

func TestRepo_Delete(t *testing.T) {
	s, _, th := openFixture(t)
	r := NewRepo(s)
	ctx := context.Background()
	b, _ := r.Create(ctx, NewInput{ThreadID: th.ID, Label: "X"})
	if err := r.Delete(ctx, b.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := r.Get(ctx, b.ID); err == nil {
		t.Error("expected ErrNotFound after delete")
	}
}

func newBeatRepoWithThread(t *testing.T) (*Repo, string) {
	t.Helper()
	s, _, th := openFixture(t)
	return NewRepo(s), th.ID
}

func TestBeatDescriptionCRUD(t *testing.T) {
	repo, threadID := newBeatRepoWithThread(t)
	ctx := context.Background()
	b, err := repo.Create(ctx, NewInput{ThreadID: threadID, Label: "재회", Description: "항구에서 마주친다."})
	if err != nil {
		t.Fatal(err)
	}
	if b.Description != "항구에서 마주친다." {
		t.Fatalf("create description = %q", b.Description)
	}
	if err := repo.Update(ctx, UpdateInput{ID: b.ID, Label: "재회(수정)"}); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != "항구에서 마주친다." {
		t.Fatalf("nil description should preserve, got %q", got.Description)
	}
	if got.Label != "재회(수정)" {
		t.Fatalf("label patch failed: %q", got.Label)
	}
	empty := ""
	if err := repo.Update(ctx, UpdateInput{ID: b.ID, Description: &empty}); err != nil {
		t.Fatal(err)
	}
	got, err = repo.Get(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != "" {
		t.Fatalf("empty-pointer description should clear, got %q", got.Description)
	}
	body := "편지로 신분이 드러난다."
	if err := repo.Update(ctx, UpdateInput{ID: b.ID, Description: &body}); err != nil {
		t.Fatal(err)
	}
	got, err = repo.Get(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != body {
		t.Fatalf("description set failed: %q", got.Description)
	}
}

func TestRepo_BeatNodeIDNulledByCascade(t *testing.T) {
	// When the bound node is deleted, beats.node_id becomes NULL (ON DELETE SET NULL).
	// Verify by direct DELETE in SQL — the migration's FK already covers this.
	s, p, th := openFixture(t)
	r := NewRepo(s)
	ctx := context.Background()
	nodeID := *p.LastOpenedNodeID
	b, _ := r.Create(ctx, NewInput{ThreadID: th.ID, NodeID: &nodeID, Label: "B"})
	if _, err := s.DB().ExecContext(ctx, `DELETE FROM nodes WHERE id = ?`, nodeID); err != nil {
		t.Fatalf("delete node: %v", err)
	}
	got, err := r.Get(ctx, b.ID)
	if err != nil {
		t.Fatalf("Get after node delete: %v", err)
	}
	if got.NodeID != nil {
		t.Errorf("NodeID = %v, want nil after cascade", got.NodeID)
	}
}
