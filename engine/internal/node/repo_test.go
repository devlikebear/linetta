package node

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func newStoreAndProject(t *testing.T) (*store.Store, project.Project) {
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
	return s, p
}

func TestRepo_Get_firstLeaf(t *testing.T) {
	s, p := newStoreAndProject(t)
	if p.LastOpenedNodeID == nil {
		t.Fatal("project has no first leaf")
	}
	r := NewRepo(s)
	n, err := r.Get(context.Background(), *p.LastOpenedNodeID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if n.Kind != "leaf" {
		t.Errorf("kind = %q, want leaf", n.Kind)
	}
	if n.Label != "씬 1" {
		t.Errorf("label = %q, want 씬 1", n.Label)
	}
	if n.ContentDoc == nil {
		t.Fatal("first leaf has no content_doc")
	}
}

func TestRepo_UpdateContent_updatesWordCount_andProjectCount(t *testing.T) {
	s, p := newStoreAndProject(t)
	r := NewRepo(s)
	pr := project.NewRepo(s)
	ctx := context.Background()

	// Insert content with 5 visible characters.
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"안녕 세계"}]}]}`
	if err := r.UpdateContent(ctx, *p.LastOpenedNodeID, doc, 9999); err != nil {
		t.Fatalf("UpdateContent: %v", err)
	}

	got, err := r.Get(ctx, *p.LastOpenedNodeID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.WordCount != 5 {
		t.Errorf("node.word_count = %d, want 5", got.WordCount)
	}
	if got.UpdatedAt != 9999 {
		t.Errorf("node.updated_at = %d, want 9999", got.UpdatedAt)
	}

	pp, err := pr.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("project Get: %v", err)
	}
	if pp.WordCount != 5 {
		t.Errorf("project.word_count = %d, want 5", pp.WordCount)
	}
	if pp.UpdatedAt != 9999 {
		t.Errorf("project.updated_at = %d, want 9999", pp.UpdatedAt)
	}
}

func TestRepo_SetLastOpened(t *testing.T) {
	s, p := newStoreAndProject(t)
	r := NewRepo(s)
	pr := project.NewRepo(s)
	ctx := context.Background()

	original := *p.LastOpenedNodeID
	if err := r.SetLastOpened(ctx, p.ID, original, 1234); err != nil {
		t.Fatalf("SetLastOpened: %v", err)
	}

	pp, err := pr.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("project Get: %v", err)
	}
	if pp.LastOpenedNodeID == nil || *pp.LastOpenedNodeID != original {
		t.Errorf("last_opened_node_id = %v, want %q", pp.LastOpenedNodeID, original)
	}
	if pp.UpdatedAt != 1234 {
		t.Errorf("project.updated_at = %d, want 1234", pp.UpdatedAt)
	}
}

func TestRepo_UpdateContent_rejectsMissingNode(t *testing.T) {
	s, _ := newStoreAndProject(t)
	r := NewRepo(s)
	err := r.UpdateContent(context.Background(), "no-such-id", `{"type":"doc"}`, 1)
	if err != ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
