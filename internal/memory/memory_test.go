package memory

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/internal/store"
	"github.com/devlikebear/linetta/internal/work"
)

func TestRepositoryCreatesUpdatesAndArchivesCanonItemsWithDecisions(t *testing.T) {
	ctx := context.Background()
	repo, workID := newTestRepository(t)

	created, err := repo.CreateItem(ctx, CreateItemInput{
		WorkID:     workID,
		Kind:       KindCharacter,
		Title:      "Mira",
		Body:       "A tide-garden caretaker.",
		Importance: ImportanceHigh,
		Reason:     "Initial protagonist seed",
		Actor:      "human",
	})
	if err != nil {
		t.Fatalf("CreateItem() error = %v", err)
	}
	if created.ID == "" || created.Status != StatusDraft {
		t.Fatalf("created = %+v, want id and draft status", created)
	}

	updated, err := repo.UpdateItem(ctx, created.ID, UpdateItemInput{
		Title:      "Mira",
		Body:       "A tide-garden caretaker who hears old infrastructure singing.",
		Status:     StatusCanon,
		Importance: ImportanceHigh,
		Reason:     "Promoted protagonist details to canon",
		Actor:      "human",
	})
	if err != nil {
		t.Fatalf("UpdateItem() error = %v", err)
	}
	if updated.Status != StatusCanon {
		t.Fatalf("updated status = %q, want canon", updated.Status)
	}

	archived, err := repo.ArchiveItem(ctx, created.ID, "Replaced by a stronger character entry", "human")
	if err != nil {
		t.Fatalf("ArchiveItem() error = %v", err)
	}
	if archived.Status != StatusArchived {
		t.Fatalf("archived status = %q, want archived", archived.Status)
	}

	decisions, err := repo.ListDecisions(ctx, workID)
	if err != nil {
		t.Fatalf("ListDecisions() error = %v", err)
	}
	if len(decisions) != 3 {
		t.Fatalf("decisions len = %d, want 3", len(decisions))
	}
	for i, want := range []DecisionType{DecisionCreate, DecisionUpdate, DecisionArchive} {
		if decisions[i].Type != want {
			t.Fatalf("decision[%d].Type = %q, want %q", i, decisions[i].Type, want)
		}
	}
}

func TestRepositoryKeepsCanonItemsScopedToWork(t *testing.T) {
	ctx := context.Background()
	repo, firstWorkID := newTestRepository(t)
	secondWorkID := createWork(t, repo.workRepo, "Second Work")

	firstItem, err := repo.CreateItem(ctx, CreateItemInput{
		WorkID: firstWorkID,
		Kind:   KindWorldFact,
		Title:  "Harbor",
		Body:   "The harbor is powered by tide gardens.",
	})
	if err != nil {
		t.Fatalf("CreateItem(first) error = %v", err)
	}
	_, err = repo.CreateItem(ctx, CreateItemInput{
		WorkID: secondWorkID,
		Kind:   KindWorldFact,
		Title:  "Other Harbor",
		Body:   "This belongs to another work.",
	})
	if err != nil {
		t.Fatalf("CreateItem(second) error = %v", err)
	}

	items, err := repo.ListItems(ctx, firstWorkID, ListFilter{Kind: KindWorldFact})
	if err != nil {
		t.Fatalf("ListItems() error = %v", err)
	}
	if len(items) != 1 || items[0].ID != firstItem.ID {
		t.Fatalf("items = %+v, want only first work item %+v", items, firstItem)
	}
}

func TestRepositoryFiltersCanonItemsByStatus(t *testing.T) {
	ctx := context.Background()
	repo, workID := newTestRepository(t)

	_, err := repo.CreateItem(ctx, CreateItemInput{
		WorkID: workID,
		Kind:   KindPlotThread,
		Title:  "Singing Machine",
		Body:   "A machine keeps calling Mira.",
		Status: StatusDraft,
	})
	if err != nil {
		t.Fatalf("CreateItem(draft) error = %v", err)
	}
	canon, err := repo.CreateItem(ctx, CreateItemInput{
		WorkID: workID,
		Kind:   KindPlotThread,
		Title:  "Black-water Years",
		Body:   "A past climate disaster still shapes memory.",
		Status: StatusCanon,
	})
	if err != nil {
		t.Fatalf("CreateItem(canon) error = %v", err)
	}

	items, err := repo.ListItems(ctx, workID, ListFilter{Status: StatusCanon})
	if err != nil {
		t.Fatalf("ListItems(canon) error = %v", err)
	}
	if len(items) != 1 || items[0].ID != canon.ID {
		t.Fatalf("items = %+v, want canon item %+v", items, canon)
	}
}

func TestRepositoryReportsMissingCanonItem(t *testing.T) {
	repo, _ := newTestRepository(t)

	_, err := repo.GetItem(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetItem() error = %v, want ErrNotFound", err)
	}
}

func newTestRepository(t *testing.T) (*Repository, string) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "linetta.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	workRepo := work.NewRepository(db)
	workID := createWork(t, workRepo, "First Work")
	return NewRepository(db, workRepo), workID
}

func createWork(t *testing.T, repo *work.Repository, title string) string {
	t.Helper()
	created, err := repo.CreateWork(context.Background(), work.CreateWorkInput{Title: title})
	if err != nil {
		t.Fatalf("CreateWork(%q) error = %v", title, err)
	}
	return created.ID
}
