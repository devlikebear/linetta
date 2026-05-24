package work

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/internal/store"
)

func TestRepositorySavesAndUpdatesEpisodeBlueprint(t *testing.T) {
	ctx := context.Background()
	repo := newBlueprintTestRepository(t)
	workItem, episode := createBlueprintTestEpisode(t, repo)

	saved, err := repo.SaveBlueprint(ctx, workItem.ID, episode.ID, SaveBlueprintInput{
		Premise:        "Mira hears the harbor singing.",
		Theme:          "Memory as civic infrastructure",
		Situation:      "A pump changes rhythm before sunset.",
		MustInclude:    "The lullaby clue",
		MustAvoid:      "No exposition dump",
		StructureNotes: "Open with a ritual, end with a message.",
	})
	if err != nil {
		t.Fatalf("SaveBlueprint() error = %v", err)
	}
	if saved.ID == "" || saved.WorkID != workItem.ID || saved.EpisodeID != episode.ID {
		t.Fatalf("saved = %+v, want ids populated", saved)
	}

	updated, err := repo.SaveBlueprint(ctx, workItem.ID, episode.ID, SaveBlueprintInput{
		Premise:        "Mira follows the song beneath the tide gardens.",
		Theme:          saved.Theme,
		Situation:      saved.Situation,
		MustInclude:    saved.MustInclude,
		MustAvoid:      saved.MustAvoid,
		StructureNotes: saved.StructureNotes,
	})
	if err != nil {
		t.Fatalf("SaveBlueprint(update) error = %v", err)
	}
	if updated.ID != saved.ID {
		t.Fatalf("updated ID = %q, want same %q", updated.ID, saved.ID)
	}
	if updated.Premise != "Mira follows the song beneath the tide gardens." {
		t.Fatalf("updated premise = %q", updated.Premise)
	}

	got, err := repo.GetBlueprint(ctx, workItem.ID, episode.ID)
	if err != nil {
		t.Fatalf("GetBlueprint() error = %v", err)
	}
	if got.ID != saved.ID || got.Premise != updated.Premise {
		t.Fatalf("got = %+v, want updated %+v", got, updated)
	}
}

func TestRepositoryRejectsBlueprintForEpisodeOwnedByAnotherWork(t *testing.T) {
	ctx := context.Background()
	repo := newBlueprintTestRepository(t)
	first, episode := createBlueprintTestEpisode(t, repo)
	second, err := repo.CreateWork(ctx, CreateWorkInput{Title: "Second Work"})
	if err != nil {
		t.Fatalf("CreateWork(second) error = %v", err)
	}

	_, err = repo.SaveBlueprint(ctx, second.ID, episode.ID, SaveBlueprintInput{Premise: "Wrong owner"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("SaveBlueprint() error = %v, want ErrNotFound", err)
	}

	_, err = repo.GetBlueprint(ctx, first.ID, "missing-episode")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetBlueprint(missing episode) error = %v, want ErrNotFound", err)
	}
}

func newBlueprintTestRepository(t *testing.T) *Repository {
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

func createBlueprintTestEpisode(t *testing.T, repo *Repository) (Work, Episode) {
	t.Helper()
	ctx := context.Background()
	workItem, err := repo.CreateWork(ctx, CreateWorkInput{Title: "Blueprint Work"})
	if err != nil {
		t.Fatalf("CreateWork() error = %v", err)
	}
	episode, err := repo.CreateEpisode(ctx, workItem.ID, "Episode 1")
	if err != nil {
		t.Fatalf("CreateEpisode() error = %v", err)
	}
	return workItem, episode
}
