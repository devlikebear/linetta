package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/relgraph"
	"github.com/devlikebear/linetta/engine/internal/store/storetest"
)

func TestRelationshipGraphHandler(t *testing.T) {
	ctx := context.Background()
	s, err := storetest.Open(ctx, t.TempDir()+"/g.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	pr := project.NewRepo(s)
	p, _ := pr.Create(ctx, 1, project.NewInput{Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first"})
	er := entity.NewRepo(s)
	a, _ := er.Create(ctx, 2, entity.NewInput{ProjectID: p.ID, Kind: entity.KindCharacter, Name: "하린"})
	b, _ := er.Create(ctx, 3, entity.NewInput{ProjectID: p.ID, Kind: entity.KindCharacter, Name: "유나"})
	rr := relationship.NewRepo(s)
	if _, err := rr.CreatePair(ctx, relationship.NewPairInput{ProjectID: p.ID, FromID: a.ID, ToID: b.ID, Label: "동료", InverseLabel: "동료"}); err != nil {
		t.Fatal(err)
	}
	h := RelationshipGraph(RelationshipGraphDeps{Projects: pr, Nodes: node.NewRepo(s), Entities: er, Relationships: rr, Mentions: mention.NewRepo(s)})
	res, err := h(ctx, json.RawMessage(`{"project_id":"`+p.ID+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	var g relgraph.Graph
	if err := json.Unmarshal(res, &g); err != nil {
		t.Fatal(err)
	}
	if len(g.Nodes) != 2 || len(g.Edges) != 1 || g.Edges[0].Label != "동료" || g.Edges[0].InverseLabel != "동료" {
		t.Fatalf("graph = %+v", g)
	}
	if _, err := h(ctx, json.RawMessage(`{}`)); err == nil {
		t.Fatal("missing project_id accepted")
	}
}
