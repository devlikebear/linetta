package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/rpc"
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

func TestEntityScenesHandler(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "t.db")
	s, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	pr := project.NewRepo(s)
	nr := node.NewRepo(s)
	er := entity.NewRepo(s)
	mr := mention.NewRepo(s)

	now := int64(1000)
	p, err := pr.Create(ctx, now, project.NewInput{Title: "t", Genres: []string{}, LengthTarget: "short", DefaultPOV: "first"})
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	// pr.Create seeds a "씬 1" leaf as p.LastOpenedNodeID. Build: 1부 → 1장 → 씬A, 씬B
	bu, err := nr.CreateSibling(ctx, *p.LastOpenedNodeID, "container", "1부", "", now)
	if err != nil {
		t.Fatalf("CreateSibling 1부: %v", err)
	}
	ch, err := nr.CreateChild(ctx, bu.ID, "container", "1장", "", now)
	if err != nil {
		t.Fatalf("CreateChild 1장: %v", err)
	}
	scA, err := nr.CreateChild(ctx, ch.ID, "leaf", "씬A", "", now)
	if err != nil {
		t.Fatalf("CreateChild 씬A: %v", err)
	}
	scB, err := nr.CreateChild(ctx, ch.ID, "leaf", "씬B", "", now)
	if err != nil {
		t.Fatalf("CreateChild 씬B: %v", err)
	}

	e, err := er.Create(ctx, now, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "해진"})
	if err != nil {
		t.Fatalf("entity: %v", err)
	}

	// 씬A: 1 mention; 씬B: 2 mentions (dup → still 1 scene)
	if err := mr.ResyncForNode(ctx, scA.ID, []mention.Found{{EntityID: e.ID, Position: 1, Surface: "해진"}}); err != nil {
		t.Fatalf("resync A: %v", err)
	}
	if err := mr.ResyncForNode(ctx, scB.ID, []mention.Found{{EntityID: e.ID, Position: 1, Surface: "해진"}, {EntityID: e.ID, Position: 9, Surface: "해진"}}); err != nil {
		t.Fatalf("resync B: %v", err)
	}

	h := EntityScenes(mr, nr)
	params, _ := json.Marshal(map[string]any{"entity_id": e.ID})
	raw, err := h(ctx, params)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got []SceneMention
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 scenes, got %d: %+v", len(got), got)
	}
	if got[0].Label != "1부 / 1장 / 씬A" {
		t.Fatalf("got[0].Label=%q want '1부 / 1장 / 씬A'", got[0].Label)
	}
	if got[1].Label != "1부 / 1장 / 씬B" {
		t.Fatalf("got[1].Label=%q want '1부 / 1장 / 씬B'", got[1].Label)
	}
	if got[0].NodeID != scA.ID || got[1].NodeID != scB.ID {
		t.Fatalf("node ids mismatch: %+v", got)
	}
}

func TestEntityScenesHandler_noMentions(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "t.db")
	s, _ := store.Open(ctx, dbPath)
	defer s.Close()
	pr := project.NewRepo(s)
	er := entity.NewRepo(s)
	mr := mention.NewRepo(s)
	nr := node.NewRepo(s)
	p, _ := pr.Create(ctx, 1000, project.NewInput{Title: "t", Genres: []string{}, LengthTarget: "short", DefaultPOV: "first"})
	e, _ := er.Create(ctx, 1000, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "무명"})

	h := EntityScenes(mr, nr)
	params, _ := json.Marshal(map[string]any{"entity_id": e.ID})
	raw, err := h(ctx, params)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got []SceneMention
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty, got %+v", got)
	}
}

func TestEntityScenesHandler_emptyID(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "t.db")
	s, _ := store.Open(ctx, dbPath)
	defer s.Close()
	h := EntityScenes(mention.NewRepo(s), node.NewRepo(s))
	params, _ := json.Marshal(map[string]any{"entity_id": ""})
	_, err := h(ctx, params)
	if err == nil {
		t.Fatal("expected InvalidParams for empty entity_id")
	}
	var mErr *rpc.MethodError
	if !errors.As(err, &mErr) || mErr.Code != rpc.CodeInvalidParams {
		t.Fatalf("expected rpc.MethodError CodeInvalidParams, got %T %v", err, err)
	}
}
