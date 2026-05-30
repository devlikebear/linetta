package relationship

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

type fixture struct {
	s    *store.Store
	r    *Repo
	pID  string
	a, b string // entity IDs
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
	p, err := pr.Create(context.Background(), 1000, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	er := entity.NewRepo(s)
	a, err := er.Create(context.Background(), 2000, entity.NewInput{
		ProjectID: p.ID, Kind: entity.KindCharacter, Name: "해진",
	})
	if err != nil {
		t.Fatalf("create A: %v", err)
	}
	b, err := er.Create(context.Background(), 2001, entity.NewInput{
		ProjectID: p.ID, Kind: entity.KindCharacter, Name: "아지",
	})
	if err != nil {
		t.Fatalf("create B: %v", err)
	}
	return fixture{s: s, r: NewRepo(s), pID: p.ID, a: a.ID, b: b.ID}
}

func TestRepo_CreateOne_singleton(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	r, err := f.r.CreateOne(ctx, NewInput{
		ProjectID: f.pID, FromID: f.a, ToID: f.b, Label: "엄마",
	})
	if err != nil {
		t.Fatalf("CreateOne: %v", err)
	}
	if r.ID == "" || r.Label != "엄마" || r.PairID != nil {
		t.Errorf("singleton mismatch: %+v", r)
	}
	got, err := f.r.Get(ctx, r.ID)
	if err != nil || got.PairID != nil {
		t.Errorf("Get singleton: %+v err=%v", got, err)
	}
}

func TestRepo_CreatePair_twoRowsShareNonNilPairID(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	rows, err := f.r.CreatePair(ctx, NewPairInput{
		ProjectID: f.pID, FromID: f.a, ToID: f.b,
		Label: "친구", InverseLabel: "친구",
	})
	if err != nil {
		t.Fatalf("CreatePair: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	if rows[0].PairID == nil || rows[1].PairID == nil {
		t.Fatalf("pair_id must be non-nil on both rows: %+v", rows)
	}
	if *rows[0].PairID != *rows[1].PairID {
		t.Errorf("pair_id mismatch: %q vs %q", *rows[0].PairID, *rows[1].PairID)
	}
	forward := rows[0]
	inverse := rows[1]
	if !(forward.FromID == f.a && forward.ToID == f.b) {
		t.Errorf("forward row not A→B: %+v", forward)
	}
	if !(inverse.FromID == f.b && inverse.ToID == f.a) {
		t.Errorf("inverse row not B→A: %+v", inverse)
	}
	if forward.Label != "친구" || inverse.Label != "친구" {
		t.Errorf("labels: forward=%q inverse=%q", forward.Label, inverse.Label)
	}
}

func TestRepo_CreatePair_distinctLabels(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	rows, err := f.r.CreatePair(ctx, NewPairInput{
		ProjectID: f.pID, FromID: f.a, ToID: f.b,
		Label: "부모", InverseLabel: "자녀",
	})
	if err != nil {
		t.Fatalf("CreatePair: %v", err)
	}
	if rows[0].Label != "부모" || rows[1].Label != "자녀" {
		t.Errorf("distinct labels not preserved: forward=%q inverse=%q",
			rows[0].Label, rows[1].Label)
	}
}

func TestRepo_ListByEntity_filtersByFromID_orderedByID(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.r.CreatePair(ctx, NewPairInput{
		ProjectID: f.pID, FromID: f.a, ToID: f.b, Label: "친구", InverseLabel: "친구",
	}); err != nil {
		t.Fatalf("pair: %v", err)
	}
	if _, err := f.r.CreateOne(ctx, NewInput{
		ProjectID: f.pID, FromID: f.a, ToID: f.b, Label: "엄마",
	}); err != nil {
		t.Fatalf("single: %v", err)
	}
	listA, err := f.r.ListByEntity(ctx, f.a)
	if err != nil {
		t.Fatalf("ListByEntity A: %v", err)
	}
	if len(listA) != 2 {
		t.Errorf("A list = %d rows, want 2 (paired A→B + singleton A→B)", len(listA))
	}
	listB, err := f.r.ListByEntity(ctx, f.b)
	if err != nil {
		t.Fatalf("ListByEntity B: %v", err)
	}
	if len(listB) != 1 || listB[0].FromID != f.b {
		t.Errorf("B list = %+v, want only the inverse half of the pair", listB)
	}
}

func TestRepo_Update_paired_onlyTouchesOneSide(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	rows, _ := f.r.CreatePair(ctx, NewPairInput{
		ProjectID: f.pID, FromID: f.a, ToID: f.b,
		Label: "친구", InverseLabel: "친구",
	})
	if err := f.r.Update(ctx, UpdateInput{
		ID: rows[0].ID, Label: "절친", Notes: "유년기부터",
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	gotForward, _ := f.r.Get(ctx, rows[0].ID)
	gotInverse, _ := f.r.Get(ctx, rows[1].ID)
	if gotForward.Label != "절친" || gotForward.Notes != "유년기부터" {
		t.Errorf("forward not updated: %+v", gotForward)
	}
	if gotInverse.Label != "친구" || gotInverse.Notes != "" {
		t.Errorf("inverse should be untouched: %+v", gotInverse)
	}
}

func TestRepo_Delete_paired_removesBothRowsAtomically(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	rows, _ := f.r.CreatePair(ctx, NewPairInput{
		ProjectID: f.pID, FromID: f.a, ToID: f.b,
		Label: "친구", InverseLabel: "친구",
	})
	if err := f.r.Delete(ctx, rows[0].ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := f.r.Get(ctx, rows[0].ID); err == nil {
		t.Error("forward row still exists")
	}
	if _, err := f.r.Get(ctx, rows[1].ID); err == nil {
		t.Error("inverse row still exists — pair delete must be atomic")
	}
}

func TestRepo_Delete_singleton_doesNotAffectOthers(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	single, _ := f.r.CreateOne(ctx, NewInput{
		ProjectID: f.pID, FromID: f.a, ToID: f.b, Label: "엄마",
	})
	pair, _ := f.r.CreatePair(ctx, NewPairInput{
		ProjectID: f.pID, FromID: f.a, ToID: f.b,
		Label: "친구", InverseLabel: "친구",
	})
	if err := f.r.Delete(ctx, single.ID); err != nil {
		t.Fatalf("Delete singleton: %v", err)
	}
	if _, err := f.r.Get(ctx, pair[0].ID); err != nil {
		t.Errorf("pair forward should survive: %v", err)
	}
	if _, err := f.r.Get(ctx, pair[1].ID); err != nil {
		t.Errorf("pair inverse should survive: %v", err)
	}
}

func TestRepo_Get_notFound(t *testing.T) {
	f := newFixture(t)
	if _, err := f.r.Get(context.Background(), "no-such-id"); err == nil {
		t.Error("expected ErrNotFound")
	}
}

func TestListByProject(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	rel, err := f.r.CreateOne(ctx, NewInput{
		ProjectID: f.pID, FromID: f.a, ToID: f.b, Label: "동료",
	})
	if err != nil {
		t.Fatalf("CreateOne: %v", err)
	}
	got, err := f.r.ListByProject(ctx, f.pID)
	if err != nil {
		t.Fatalf("ListByProject: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].ID != rel.ID {
		t.Errorf("got %q, want %q", got[0].ID, rel.ID)
	}
}
