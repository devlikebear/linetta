package entity

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func openStoreAndProject(t *testing.T) (*store.Store, project.Project) {
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

func TestRepo_Create_thenGet(t *testing.T) {
	s, p := openStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()
	e, err := r.Create(ctx, 100, NewInput{ProjectID: p.ID, Kind: KindCharacter, Name: "해진", Role: "POV"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if e.ID == "" || e.Name != "해진" || e.Kind != "character" {
		t.Errorf("unexpected entity: %+v", e)
	}
	got, err := r.Get(ctx, e.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "해진" {
		t.Errorf("Get name = %q", got.Name)
	}
	if got.Attributes == nil {
		t.Error("Attributes should be a non-nil empty map on a fresh entity")
	}
}

func TestRepo_Create_rejectsDuplicateName(t *testing.T) {
	s, p := openStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()
	if _, err := r.Create(ctx, 100, NewInput{ProjectID: p.ID, Kind: KindCharacter, Name: "해진"}); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if _, err := r.Create(ctx, 200, NewInput{ProjectID: p.ID, Kind: KindCharacter, Name: "해진"}); err == nil {
		t.Error("duplicate name should have failed")
	}
}

func TestRepo_Search_caseInsensitiveSubstring_shortFirst(t *testing.T) {
	s, p := openStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()
	_, _ = r.Create(ctx, 100, NewInput{ProjectID: p.ID, Kind: KindCharacter, Name: "해진"})
	_, _ = r.Create(ctx, 110, NewInput{ProjectID: p.ID, Kind: KindCharacter, Name: "해진의 친구"})
	_, _ = r.Create(ctx, 120, NewInput{ProjectID: p.ID, Kind: KindPlace, Name: "동해 해변"})

	got, err := r.Search(ctx, p.ID, "해", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].Name != "해진" || got[1].Name != "해진의 친구" || got[2].Name != "동해 해변" {
		t.Errorf("ordering = %q,%q,%q", got[0].Name, got[1].Name, got[2].Name)
	}
}

func TestRepo_Update_partial(t *testing.T) {
	s, p := openStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()
	e, _ := r.Create(ctx, 100, NewInput{ProjectID: p.ID, Kind: KindCharacter, Name: "해진"})
	attrs := map[string]string{"나이": "32", "직업": "사진작가"}
	if err := r.Update(ctx, 200, UpdateInput{ID: e.ID, Role: "POV", Summary: "사진을 찍는 사람", Attributes: &attrs}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := r.Get(ctx, e.ID)
	if got.Role != "POV" || got.Summary != "사진을 찍는 사람" {
		t.Errorf("update missed: %+v", got)
	}
	if got.Attributes["나이"] != "32" {
		t.Errorf("attributes not stored: %+v", got.Attributes)
	}
}

func TestRepo_Search_emptyQuery_returnsRecent(t *testing.T) {
	s, p := openStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()
	_, _ = r.Create(ctx, 100, NewInput{ProjectID: p.ID, Kind: KindCharacter, Name: "A"})
	_, _ = r.Create(ctx, 200, NewInput{ProjectID: p.ID, Kind: KindCharacter, Name: "B"})

	got, err := r.Search(ctx, p.ID, "", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("empty query: got %d, want 2", len(got))
	}
}
