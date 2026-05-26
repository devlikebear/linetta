package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/store"
)

type relFix struct {
	rr   *relationship.Repo
	pID  string
	a, b string
}

func newRelFixture(t *testing.T) relFix {
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
	er := entity.NewRepo(s)
	a, _ := er.Create(context.Background(), 2, entity.NewInput{
		ProjectID: p.ID, Kind: entity.KindCharacter, Name: "해진",
	})
	b, _ := er.Create(context.Background(), 3, entity.NewInput{
		ProjectID: p.ID, Kind: entity.KindCharacter, Name: "아지",
	})
	return relFix{rr: relationship.NewRepo(s), pID: p.ID, a: a.ID, b: b.ID}
}

func TestCreateOneRelationshipHandler(t *testing.T) {
	f := newRelFixture(t)
	res, err := CreateOneRelationship(f.rr)(context.Background(),
		json.RawMessage(`{"project_id":"`+f.pID+`","from_id":"`+f.a+`","to_id":"`+f.b+`","label":"엄마"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var rel relationship.Relationship
	_ = json.Unmarshal(res, &rel)
	if rel.Label != "엄마" || rel.PairID != nil {
		t.Errorf("singleton mismatch: %+v", rel)
	}
}

func TestCreatePairRelationshipHandler(t *testing.T) {
	f := newRelFixture(t)
	res, err := CreatePairRelationship(f.rr)(context.Background(),
		json.RawMessage(`{"project_id":"`+f.pID+`","from_id":"`+f.a+`","to_id":"`+f.b+
			`","label":"부모","inverse_label":"자녀"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var rows []relationship.Relationship
	_ = json.Unmarshal(res, &rows)
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	if rows[0].PairID == nil || rows[1].PairID == nil || *rows[0].PairID != *rows[1].PairID {
		t.Errorf("pair_id not shared: %+v", rows)
	}
	if rows[0].Label != "부모" || rows[1].Label != "자녀" {
		t.Errorf("labels swapped wrong: %+v", rows)
	}
}

func TestListRelationshipsByEntityHandler(t *testing.T) {
	f := newRelFixture(t)
	_, _ = f.rr.CreatePair(context.Background(), relationship.NewPairInput{
		ProjectID: f.pID, FromID: f.a, ToID: f.b,
		Label: "친구", InverseLabel: "친구",
	})
	res, err := ListRelationshipsByEntity(f.rr)(context.Background(),
		json.RawMessage(`{"entity_id":"`+f.a+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var list []relationship.Relationship
	_ = json.Unmarshal(res, &list)
	if len(list) != 1 || list[0].FromID != f.a {
		t.Errorf("list = %+v", list)
	}
}

func TestUpdateRelationshipHandler(t *testing.T) {
	f := newRelFixture(t)
	rel, _ := f.rr.CreateOne(context.Background(), relationship.NewInput{
		ProjectID: f.pID, FromID: f.a, ToID: f.b, Label: "친구",
	})
	res, err := UpdateRelationship(f.rr)(context.Background(),
		json.RawMessage(`{"id":"`+rel.ID+`","label":"절친","notes":"메모"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got relationship.Relationship
	_ = json.Unmarshal(res, &got)
	if got.Label != "절친" || got.Notes != "메모" {
		t.Errorf("update missed: %+v", got)
	}
}

func TestDeleteRelationshipHandler_pair(t *testing.T) {
	f := newRelFixture(t)
	rows, _ := f.rr.CreatePair(context.Background(), relationship.NewPairInput{
		ProjectID: f.pID, FromID: f.a, ToID: f.b,
		Label: "친구", InverseLabel: "친구",
	})
	if _, err := DeleteRelationship(f.rr)(context.Background(),
		json.RawMessage(`{"id":"`+rows[0].ID+`"}`)); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := f.rr.Get(context.Background(), rows[1].ID); err == nil {
		t.Error("inverse row should be gone (atomic pair delete)")
	}
}
