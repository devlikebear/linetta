package project

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/store"
)

func openStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestRepo_Create_returnsProjectWithGeneratedID_andFirstLeafNode(t *testing.T) {
	s := openStore(t)
	r := NewRepo(s)
	ctx := context.Background()

	now := time.Now().UnixMilli()
	p, err := r.Create(ctx, now, NewInput{
		Title:        "은하의 노래",
		Genres:       []string{"SF", "문학"},
		LengthTarget: "novel",
		DefaultPOV:   "first",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.ID == "" {
		t.Error("Create: missing ID")
	}
	if p.Title != "은하의 노래" {
		t.Errorf("title = %q", p.Title)
	}
	if got, want := len(p.Genres), 2; got != want {
		t.Errorf("genres len = %d, want %d", got, want)
	}
	if p.LastOpenedNodeID == nil || *p.LastOpenedNodeID == "" {
		t.Error("Create: last_opened_node_id should point to the auto-created first leaf")
	}

	// First leaf node exists?
	var (
		nodeID, label, kind string
		nodeProjectID       string
	)
	err = s.DB().QueryRowContext(ctx, `
SELECT id, project_id, kind, label FROM nodes WHERE id = ?`, *p.LastOpenedNodeID).
		Scan(&nodeID, &nodeProjectID, &kind, &label)
	if err != nil {
		t.Fatalf("first leaf row: %v", err)
	}
	if nodeProjectID != p.ID {
		t.Errorf("node.project_id = %q, want %q", nodeProjectID, p.ID)
	}
	if kind != "leaf" {
		t.Errorf("node.kind = %q, want leaf", kind)
	}
	if label != "씬 1" {
		t.Errorf("node.label = %q, want %q", label, "씬 1")
	}
}

func TestRepo_Create_rejectsInvalidLengthAndPOV(t *testing.T) {
	s := openStore(t)
	r := NewRepo(s)
	ctx := context.Background()

	if _, err := r.Create(ctx, 1, NewInput{
		Title: "bad", Genres: []string{"SF"}, LengthTarget: "epic", DefaultPOV: "first",
	}); err != ErrInvalidLengthTarget {
		t.Errorf("length err = %v, want ErrInvalidLengthTarget", err)
	}
	if _, err := r.Create(ctx, 1, NewInput{
		Title: "bad", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "second",
	}); err != ErrInvalidDefaultPOV {
		t.Errorf("pov err = %v, want ErrInvalidDefaultPOV", err)
	}
}

func TestRepo_List_recentFirst(t *testing.T) {
	s := openStore(t)
	r := NewRepo(s)
	ctx := context.Background()

	_, _ = r.Create(ctx, 1000, NewInput{Title: "A", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first"})
	_, _ = r.Create(ctx, 2000, NewInput{Title: "B", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first"})
	_, _ = r.Create(ctx, 3000, NewInput{Title: "C", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first"})

	got, err := r.List(ctx, ListFilter{Limit: 5})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].Title != "C" || got[1].Title != "B" || got[2].Title != "A" {
		t.Errorf("order = %q,%q,%q; want C,B,A", got[0].Title, got[1].Title, got[2].Title)
	}
}

func TestRepo_List_excludesArchivedByDefault(t *testing.T) {
	s := openStore(t)
	r := NewRepo(s)
	ctx := context.Background()

	a, _ := r.Create(ctx, 1000, NewInput{Title: "kept", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first"})
	b, _ := r.Create(ctx, 2000, NewInput{Title: "gone", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first"})
	if err := r.Archive(ctx, b.ID, 9999); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	defaultList, err := r.List(ctx, ListFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(defaultList) != 1 || defaultList[0].ID != a.ID {
		t.Errorf("default list should be just a, got %d entries", len(defaultList))
	}

	allList, err := r.List(ctx, ListFilter{IncludeArchived: true, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(allList) != 2 {
		t.Errorf("with archived: got %d, want 2", len(allList))
	}
}

func TestProjectOutlineUpdate(t *testing.T) {
	s := openStore(t)
	repo := NewRepo(s)
	ctx := context.Background()
	p, err := repo.Create(ctx, 1000, NewInput{Title: "테스트작", Genres: []string{"판타지"}, LengthTarget: "novel", DefaultPOV: "third_limited"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Outline != "" {
		t.Fatalf("new project outline should be empty, got %q", p.Outline)
	}
	body := "한 줄 로그라인과 3막 개요."
	updated, err := repo.Update(ctx, 2000, UpdateInput{ID: p.ID, Outline: &body})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Outline != body {
		t.Fatalf("outline = %q", updated.Outline)
	}
	if updated.UpdatedAt != 2000 {
		t.Fatalf("updated_at not bumped: %d", updated.UpdatedAt)
	}
	again, err := repo.Update(ctx, 3000, UpdateInput{ID: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	if again.Outline != body {
		t.Fatalf("nil outline should preserve, got %q", again.Outline)
	}
}

func TestProjectTitleUpdate(t *testing.T) {
	s := openStore(t)
	repo := NewRepo(s)
	ctx := context.Background()
	p, err := repo.Create(ctx, 1000, NewInput{Title: "처음 제목", Genres: []string{"판타지"}, LengthTarget: "novel", DefaultPOV: "third_limited"})
	if err != nil {
		t.Fatal(err)
	}

	next := "바뀐 제목"
	updated, err := repo.Update(ctx, 2000, UpdateInput{ID: p.ID, Title: &next})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != next {
		t.Fatalf("title = %q, want %q", updated.Title, next)
	}
	if updated.UpdatedAt != 2000 {
		t.Fatalf("updated_at not bumped: %d", updated.UpdatedAt)
	}

	again, err := repo.Update(ctx, 3000, UpdateInput{ID: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	if again.Title != next {
		t.Fatalf("nil title should preserve, got %q", again.Title)
	}
}

func TestRepo_Get(t *testing.T) {
	s := openStore(t)
	r := NewRepo(s)
	ctx := context.Background()

	p, _ := r.Create(ctx, 1000, NewInput{Title: "x", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first"})
	got, err := r.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != "x" {
		t.Errorf("Get returned wrong project: %+v", got)
	}

	_, err = r.Get(ctx, "nonexistent")
	if err != ErrNotFound {
		t.Errorf("Get of missing: err = %v, want ErrNotFound", err)
	}
}
