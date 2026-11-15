package proposals

import (
	"context"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/storyops"
)

func TestChangeRelationshipIsProposedAndAccepted(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	a, _ := env.entities.Create(ctx, 1, entity.NewInput{ProjectID: env.project.ID, Kind: "character", Name: "하린"})
	b, _ := env.entities.Create(ctx, 1, entity.NewInput{ProjectID: env.project.ID, Kind: "character", Name: "유나"})
	rels := relationship.NewRepo(env.svc.store)
	rel, err := rels.CreateOne(ctx, relationship.NewInput{ProjectID: env.project.ID, FromID: a.ID, ToID: b.ID, Label: "동료"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := env.nodes.CreateSibling(ctx, env.sceneID, "leaf", "", "배신", 2)
	if err != nil {
		t.Fatal(err)
	}

	_, problems, err := env.svc.Submit(ctx, now(), SubmitInput{ProjectID: env.project.ID, Items: []ItemInput{
		{Op: storyops.Op{Type: "change_relationship", RelationshipID: rel.ID, Label: "적"}},
	}}, SourceAgent)
	if err != nil || len(problems) != 1 || !strings.Contains(problems[0].Message, "needs node_id") {
		t.Fatalf("without a scene: problems = %+v, err = %v", problems, err)
	}

	sub := mustSubmit(t, env, SubmitInput{ProjectID: env.project.ID, Items: []ItemInput{
		{Op: storyops.Op{Type: "change_relationship", RelationshipID: rel.ID, Label: "적", NodeID: second.ID, Quote: "넌 이제 적이야."}},
	}})
	if got := env.svc.Summarize(ctx, sub.Items[0].Op, nil); got != "관계 변화: 하린 → 유나 (동료 → 적, 배신부터)" {
		t.Fatalf("summary = %q", got)
	}
	res, err := env.svc.Accept(ctx, now(), env.project.ID, []string{sub.Items[0].ID}, nil)
	if err != nil || res.Items[0].Status != StatusAccepted {
		t.Fatalf("accept = %+v, err = %v", res.Items, err)
	}
	old, _ := rels.Get(ctx, rel.ID)
	if old.UntilNodeID == nil || *old.UntilNodeID != second.ID {
		t.Fatalf("old row should end at the change scene: %+v", old)
	}

	_, problems, _ = env.svc.Submit(ctx, now(), SubmitInput{ProjectID: env.project.ID, Items: []ItemInput{
		{Op: storyops.Op{Type: "change_relationship", RelationshipID: rel.ID, Label: "친구", NodeID: second.ID}},
	}}, SourceAgent)
	if len(problems) != 1 || !strings.Contains(problems[0].Message, "already changed") {
		t.Fatalf("changing an ended row: problems = %+v", problems)
	}
}
