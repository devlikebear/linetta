package mention

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

type fixture struct {
	store *store.Store
	mr    *Repo
	er    *entity.Repo
	pID   string
	nID   string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	pr := project.NewRepo(s)
	p, _ := pr.Create(context.Background(), 1000, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first",
	})
	return fixture{store: s, mr: NewRepo(s), er: entity.NewRepo(s), pID: p.ID, nID: *p.LastOpenedNodeID}
}

func TestResyncForNode_insertsValidMentions_dropsUnknown(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	e, _ := f.er.Create(ctx, 100, entity.NewInput{ProjectID: f.pID, Kind: "character", Name: "해진"})

	found := []Found{
		{EntityID: e.ID, Position: 5, Surface: "해진"},
		{EntityID: "missing-uuid", Position: 10, Surface: "윤서"}, // dropped (FK invalid)
	}
	if err := f.mr.ResyncForNode(ctx, f.nID, found); err != nil {
		t.Fatalf("ResyncForNode: %v", err)
	}
	got, err := f.mr.ListForNode(ctx, f.nID)
	if err != nil {
		t.Fatalf("ListForNode: %v", err)
	}
	if len(got) != 1 || got[0].EntityID != e.ID {
		t.Errorf("after resync: %+v", got)
	}
}

func TestResyncForNode_replacesPreviousSet(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a, _ := f.er.Create(ctx, 100, entity.NewInput{ProjectID: f.pID, Kind: "character", Name: "A"})
	b, _ := f.er.Create(ctx, 110, entity.NewInput{ProjectID: f.pID, Kind: "character", Name: "B"})

	_ = f.mr.ResyncForNode(ctx, f.nID, []Found{{EntityID: a.ID, Position: 1, Surface: "A"}})
	_ = f.mr.ResyncForNode(ctx, f.nID, []Found{{EntityID: b.ID, Position: 1, Surface: "B"}})

	got, _ := f.mr.ListForNode(ctx, f.nID)
	if len(got) != 1 || got[0].EntityID != b.ID {
		t.Errorf("after re-resync: %+v", got)
	}
}

func TestListEntitiesForNode_hydratesEntities(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	e, _ := f.er.Create(ctx, 100, entity.NewInput{ProjectID: f.pID, Kind: "character", Name: "해진", Role: "POV"})
	_ = f.mr.ResyncForNode(ctx, f.nID, []Found{{EntityID: e.ID, Position: 1, Surface: "해진"}})

	got, err := f.mr.ListEntitiesForNode(ctx, f.nID)
	if err != nil {
		t.Fatalf("ListEntitiesForNode: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len %d", len(got))
	}
	if got[0].Name != "해진" || got[0].Role != "POV" {
		t.Errorf("entity hydration missed: %+v", got[0])
	}
}
