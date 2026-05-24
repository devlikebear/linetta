package work

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/internal/store"
)

func TestRepositoryCreatesAndListsWorks(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)

	first, err := repo.CreateWork(ctx, CreateWorkInput{
		Title:   "  Green Harbor  ",
		Genre:   "climate fiction",
		Premise: "A caretaker hears a forgotten machine singing.",
	})
	if err != nil {
		t.Fatalf("CreateWork() error = %v", err)
	}
	if first.ID == "" {
		t.Fatal("CreateWork() returned empty ID")
	}
	if first.Title != "Green Harbor" {
		t.Fatalf("Title = %q, want trimmed Green Harbor", first.Title)
	}
	if first.Status != WorkStatusActive {
		t.Fatalf("Status = %q, want %q", first.Status, WorkStatusActive)
	}

	second, err := repo.CreateWork(ctx, CreateWorkInput{Title: "Signal Rain"})
	if err != nil {
		t.Fatalf("CreateWork(second) error = %v", err)
	}

	works, err := repo.ListWorks(ctx)
	if err != nil {
		t.Fatalf("ListWorks() error = %v", err)
	}
	if len(works) != 2 {
		t.Fatalf("works len = %d, want 2", len(works))
	}
	if works[0].ID != second.ID || works[1].ID != first.ID {
		t.Fatalf("works order = [%s %s], want newest first [%s %s]", works[0].ID, works[1].ID, second.ID, first.ID)
	}
}

func TestRepositoryRejectsBlankWorkTitle(t *testing.T) {
	repo := newTestRepository(t)

	_, err := repo.CreateWork(context.Background(), CreateWorkInput{Title: "   "})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("CreateWork() error = %v, want ErrInvalidInput", err)
	}
}

func TestRepositoryGetsWorkAndReportsMissingWork(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)

	created, err := repo.CreateWork(ctx, CreateWorkInput{Title: "Memory City"})
	if err != nil {
		t.Fatalf("CreateWork() error = %v", err)
	}
	got, err := repo.GetWork(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetWork() error = %v", err)
	}
	if got.ID != created.ID || got.Title != created.Title {
		t.Fatalf("GetWork() = %+v, want %+v", got, created)
	}

	_, err = repo.GetWork(ctx, "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetWork(missing) error = %v, want ErrNotFound", err)
	}
}

func TestRepositoryKeepsEpisodesScopedToWork(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepository(t)

	first, err := repo.CreateWork(ctx, CreateWorkInput{Title: "First Work"})
	if err != nil {
		t.Fatalf("CreateWork(first) error = %v", err)
	}
	second, err := repo.CreateWork(ctx, CreateWorkInput{Title: "Second Work"})
	if err != nil {
		t.Fatalf("CreateWork(second) error = %v", err)
	}

	firstEpisode, err := repo.CreateEpisode(ctx, first.ID, "Opening Bell")
	if err != nil {
		t.Fatalf("CreateEpisode(first) error = %v", err)
	}
	if _, err := repo.CreateEpisode(ctx, second.ID, "Other Opening"); err != nil {
		t.Fatalf("CreateEpisode(second) error = %v", err)
	}

	episodes, err := repo.ListEpisodes(ctx, first.ID)
	if err != nil {
		t.Fatalf("ListEpisodes(first) error = %v", err)
	}
	if len(episodes) != 1 {
		t.Fatalf("episodes len = %d, want 1", len(episodes))
	}
	if episodes[0].ID != firstEpisode.ID || episodes[0].WorkID != first.ID {
		t.Fatalf("episodes[0] = %+v, want first work episode %+v", episodes[0], firstEpisode)
	}
}

func TestRepositoryRejectsEpisodeForMissingWork(t *testing.T) {
	repo := newTestRepository(t)

	_, err := repo.CreateEpisode(context.Background(), "missing-work", "Ghost Episode")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("CreateEpisode() error = %v, want ErrNotFound", err)
	}
}

func newTestRepository(t *testing.T) *Repository {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "linetta.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	return NewRepository(db)
}
