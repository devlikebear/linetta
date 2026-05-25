package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func newEntityFixture(t *testing.T) (*entity.Repo, project.Project) {
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
	return entity.NewRepo(s), p
}

func TestCreateEntityHandler(t *testing.T) {
	r, p := newEntityFixture(t)
	h := CreateEntity(r, func() int64 { return 1234 })
	params := json.RawMessage(`{"project_id":"` + p.ID + `","kind":"character","name":"해진","role":"POV"}`)
	res, err := h(context.Background(), params)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var e entity.Entity
	_ = json.Unmarshal(res, &e)
	if e.Name != "해진" || e.Role != "POV" || e.CreatedAt != 1234 {
		t.Errorf("entity = %+v", e)
	}
}

func TestSearchEntityHandler(t *testing.T) {
	r, p := newEntityFixture(t)
	_, _ = r.Create(context.Background(), 100, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "해진"})
	_, _ = r.Create(context.Background(), 110, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "해진의 친구"})

	h := SearchEntities(r)
	res, err := h(context.Background(), json.RawMessage(`{"project_id":"`+p.ID+`","query":"해","limit":10}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got []entity.Entity
	_ = json.Unmarshal(res, &got)
	if len(got) != 2 || got[0].Name != "해진" {
		t.Errorf("results = %+v", got)
	}
}

func TestGetEntityHandler(t *testing.T) {
	r, p := newEntityFixture(t)
	created, _ := r.Create(context.Background(), 100, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "해진"})
	h := GetEntity(r)
	res, err := h(context.Background(), json.RawMessage(`{"id":"`+created.ID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var e entity.Entity
	_ = json.Unmarshal(res, &e)
	if e.Name != "해진" {
		t.Errorf("name = %q", e.Name)
	}
}

func TestUpdateEntityHandler(t *testing.T) {
	r, p := newEntityFixture(t)
	created, _ := r.Create(context.Background(), 100, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "해진"})
	h := UpdateEntity(r, func() int64 { return 5000 })
	params := json.RawMessage(`{"id":"` + created.ID + `","name":"해진","role":"POV","summary":"사진작가","attributes":{"나이":"32"}}`)
	if _, err := h(context.Background(), params); err != nil {
		t.Fatalf("handler: %v", err)
	}
	got, _ := r.Get(context.Background(), created.ID)
	if got.Role != "POV" || got.Attributes["나이"] != "32" {
		t.Errorf("update missed: %+v", got)
	}
}
