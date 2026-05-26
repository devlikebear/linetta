package thread

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
	th, err := r.Create(ctx, NewInput{ProjectID: p.ID, Name: "잃어버린 시간", Color: "#c08a3e"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if th.ID == "" || th.Name != "잃어버린 시간" || th.Color != "#c08a3e" {
		t.Errorf("unexpected thread: %+v", th)
	}
	got, err := r.Get(ctx, th.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "잃어버린 시간" || got.ClosedAt != nil {
		t.Errorf("Get = %+v", got)
	}
}

func TestRepo_Create_defaultsColorWhenEmpty(t *testing.T) {
	s, p := openStoreAndProject(t)
	r := NewRepo(s)
	th, _ := r.Create(context.Background(), NewInput{ProjectID: p.ID, Name: "T"})
	if th.Color != "#666" {
		t.Errorf("default color = %q, want #666", th.Color)
	}
}

func TestRepo_ListByProject_excludesClosedByDefault(t *testing.T) {
	s, p := openStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()
	open, _ := r.Create(ctx, NewInput{ProjectID: p.ID, Name: "열린"})
	closed, _ := r.Create(ctx, NewInput{ProjectID: p.ID, Name: "닫힌"})
	if err := r.Close(ctx, closed.ID, 5000); err != nil {
		t.Fatalf("Close: %v", err)
	}

	got, err := r.ListByProject(ctx, p.ID, false)
	if err != nil {
		t.Fatalf("ListByProject: %v", err)
	}
	if len(got) != 1 || got[0].ID != open.ID {
		t.Errorf("open-only = %+v", got)
	}

	all, _ := r.ListByProject(ctx, p.ID, true)
	if len(all) != 2 {
		t.Errorf("include-closed got %d, want 2", len(all))
	}
}

func TestRepo_Update_partial(t *testing.T) {
	s, p := openStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()
	th, _ := r.Create(ctx, NewInput{ProjectID: p.ID, Name: "원본"})
	if err := r.Update(ctx, UpdateInput{ID: th.ID, Name: "수정됨", Summary: "요약 한 줄"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := r.Get(ctx, th.ID)
	if got.Name != "수정됨" || got.Summary != "요약 한 줄" || got.Color != "#666" {
		t.Errorf("update missed: %+v", got)
	}
}

func TestRepo_CloseAndReopen(t *testing.T) {
	s, p := openStoreAndProject(t)
	r := NewRepo(s)
	ctx := context.Background()
	th, _ := r.Create(ctx, NewInput{ProjectID: p.ID, Name: "T"})
	if err := r.Close(ctx, th.ID, 5000); err != nil {
		t.Fatalf("Close: %v", err)
	}
	closed, _ := r.Get(ctx, th.ID)
	if closed.ClosedAt == nil || *closed.ClosedAt != 5000 {
		t.Errorf("ClosedAt = %v", closed.ClosedAt)
	}
	if err := r.Reopen(ctx, th.ID); err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	reopened, _ := r.Get(ctx, th.ID)
	if reopened.ClosedAt != nil {
		t.Errorf("ClosedAt after reopen = %v", reopened.ClosedAt)
	}
}

func TestRepo_Get_notFound(t *testing.T) {
	s, _ := openStoreAndProject(t)
	r := NewRepo(s)
	if _, err := r.Get(context.Background(), "no-such-id"); err == nil {
		t.Error("expected ErrNotFound")
	}
}
