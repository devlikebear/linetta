package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

func TestListMentionsForNodeHandler(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()
	pr := project.NewRepo(s)
	p, _ := pr.Create(context.Background(), 1000, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first",
	})
	er := entity.NewRepo(s)
	mr := mention.NewRepo(s)

	e, _ := er.Create(context.Background(), 2000, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "해진"})
	_ = mr.ResyncForNode(context.Background(), *p.LastOpenedNodeID, []mention.Found{
		{EntityID: e.ID, Position: 1, Surface: "해진"},
	})

	h := ListMentionsForNode(mr)
	res, err := h(context.Background(), json.RawMessage(`{"node_id":"`+*p.LastOpenedNodeID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got []entity.Entity
	_ = json.Unmarshal(res, &got)
	if len(got) != 1 || got[0].Name != "해진" {
		t.Errorf("got %+v", got)
	}
}
