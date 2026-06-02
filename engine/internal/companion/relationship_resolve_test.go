package companion

import (
	"context"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
)

// A create_relationship referencing pre-existing entities by NAME (not ref/id)
// must resolve instead of failing with a FOREIGN KEY constraint error.
func TestApplyOps_RelationshipResolvesByName(t *testing.T) {
	ctx := context.Background()
	svc, projectID, _ := newToolSvc(t)

	a, err := svc.entities.Create(ctx, 1, entity.NewInput{ProjectID: projectID, Kind: "character", Name: "우진"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.entities.Create(ctx, 1, entity.NewInput{ProjectID: projectID, Kind: "character", Name: "서아"})
	if err != nil {
		t.Fatal(err)
	}

	ops := `[{"op":"create_relationship","from":"우진","to":"서아","label":"관심과 연민"}]`
	p, _, err := ParseProposal("```linetta-proposal\n{\"ops\":" + ops + "}\n```")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	res := svc.ApplyOps(ctx, projectID, "", p, func() int64 { return 1 })
	if len(res.Failures) != 0 {
		t.Fatalf("expected 0 failures, got %+v", res.Failures)
	}

	rels, err := svc.relationships.ListByEntity(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) == 0 {
		t.Fatalf("expected a relationship from %q to %q", a.Name, b.Name)
	}
}
