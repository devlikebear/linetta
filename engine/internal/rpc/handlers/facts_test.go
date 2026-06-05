package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

type factsFixture struct {
	repo      *fact.Repo
	projectID string
	nodeID    string
}

func newFactsFixture(t *testing.T) factsFixture {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	p, err := project.NewRepo(s).Create(context.Background(), 1, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("project.Create: %v", err)
	}
	return factsFixture{repo: fact.NewRepo(s), projectID: p.ID, nodeID: *p.LastOpenedNodeID}
}

func TestCreateFactHandler(t *testing.T) {
	f := newFactsFixture(t)
	res, err := CreateFact(f.repo, func() int64 { return 100 })(context.Background(),
		json.RawMessage(`{"project_id":"`+f.projectID+`","node_id":"`+f.nodeID+`","claim":"경찰 총기 휴대","result":"국가마다 다르다","status":"verified","sources":[{"url":"https://example.com","title":"Example","accessed_at":100}]}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var card fact.Card
	if err := json.Unmarshal(res, &card); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if card.ProjectID != f.projectID || card.NodeID == nil || *card.NodeID != f.nodeID || len(card.Sources) != 1 {
		t.Fatalf("card = %+v", card)
	}
}

func TestCreateFactHandlerRequiresSource(t *testing.T) {
	f := newFactsFixture(t)
	if _, err := CreateFact(f.repo, func() int64 { return 100 })(context.Background(),
		json.RawMessage(`{"project_id":"`+f.projectID+`","claim":"출처 없는 주장","result":"x","status":"verified"}`)); err == nil {
		t.Fatal("expected source-required error")
	}
}

func TestListFactsHandlerReturnsEmptyArray(t *testing.T) {
	f := newFactsFixture(t)
	res, err := ListFacts(f.repo)(context.Background(), json.RawMessage(`{"project_id":"`+f.projectID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var list []fact.Card
	if err := json.Unmarshal(res, &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if list == nil || len(list) != 0 {
		t.Fatalf("list = %+v, want empty non-nil array", list)
	}
}

func TestUpdateAndDeleteFactHandlers(t *testing.T) {
	f := newFactsFixture(t)
	card, err := f.repo.Create(context.Background(), 10, fact.NewInput{
		ProjectID: f.projectID,
		Claim:    "원본",
		Result:   "검증",
		Status:   fact.StatusVerified,
		Sources:  []fact.SourceInput{{URL: "https://example.com", AccessedAt: 10}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	res, err := UpdateFact(f.repo, func() int64 { return 20 })(context.Background(),
		json.RawMessage(`{"id":"`+card.ID+`","status":"stale"}`))
	if err != nil {
		t.Fatalf("Update handler: %v", err)
	}
	var updated fact.Card
	_ = json.Unmarshal(res, &updated)
	if updated.Status != fact.StatusStale {
		t.Fatalf("updated = %+v", updated)
	}
	if _, err := DeleteFact(f.repo)(context.Background(), json.RawMessage(`{"id":"`+card.ID+`"}`)); err != nil {
		t.Fatalf("Delete handler: %v", err)
	}
}
