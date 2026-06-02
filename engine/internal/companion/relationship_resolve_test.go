package companion

import (
	"context"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/thread"
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

// update_entity referencing the target by NAME must resolve instead of failing
// with "entity not found".
func TestApplyOps_UpdateEntityResolvesByName(t *testing.T) {
	ctx := context.Background()
	svc, projectID, _ := newToolSvc(t)

	ent, err := svc.entities.Create(ctx, 1, entity.NewInput{ProjectID: projectID, Kind: "character", Name: "우진"})
	if err != nil {
		t.Fatal(err)
	}

	ops := `[{"op":"update_entity","entity_id":"우진","summary":"마을 뒷산의 비밀 아지트를 가진 소년"}]`
	p, _, err := ParseProposal("```linetta-proposal\n{\"ops\":" + ops + "}\n```")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	res := svc.ApplyOps(ctx, projectID, "", p, func() int64 { return 1 })
	if len(res.Failures) != 0 {
		t.Fatalf("expected 0 failures, got %+v", res.Failures)
	}

	got, err := svc.entities.Get(ctx, ent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Summary != "마을 뒷산의 비밀 아지트를 가진 소년" {
		t.Fatalf("summary not updated: %q", got.Summary)
	}
}

// add_beat with a real thread id placed in thread_ref (the model conflates
// thread_id/thread_ref) must resolve instead of failing validation with
// "thread_ref ... not declared by any create_thread.ref".
func TestApplyOps_AddBeatResolvesThreadIdInRefField(t *testing.T) {
	ctx := context.Background()
	svc, projectID, nodeID := newToolSvc(t)
	th, err := svc.threads.Create(ctx, thread.NewInput{ProjectID: projectID, Name: "메인 미스터리"})
	if err != nil {
		t.Fatal(err)
	}

	ops := `[{"op":"add_beat","thread_ref":"` + th.ID + `","label":"단서 발견"}]`
	p, _, err := ParseProposal("```linetta-proposal\n{\"ops\":" + ops + "}\n```")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	res := svc.ApplyOps(ctx, projectID, nodeID, p, func() int64 { return 1 })
	if len(res.Failures) != 0 {
		t.Fatalf("expected 0 failures, got %+v", res.Failures)
	}
	beats, err := svc.beats.ListByThread(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(beats) == 0 {
		t.Fatal("expected a beat on the thread")
	}
}

// create_relationship with real entity ids placed in from_ref/to_ref must
// resolve (the model uses from_ref but with an id, not a declared ref).
func TestApplyOps_RelationshipResolvesIdInRefField(t *testing.T) {
	ctx := context.Background()
	svc, projectID, _ := newToolSvc(t)
	a, _ := svc.entities.Create(ctx, 1, entity.NewInput{ProjectID: projectID, Kind: "character", Name: "강진우"})
	b, _ := svc.entities.Create(ctx, 1, entity.NewInput{ProjectID: projectID, Kind: "character", Name: "서윤아"})

	ops := `[{"op":"create_relationship","from_ref":"` + a.ID + `","to_ref":"` + b.ID + `","label":"신뢰"}]`
	p, _, err := ParseProposal("```linetta-proposal\n{\"ops\":" + ops + "}\n```")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	res := svc.ApplyOps(ctx, projectID, "", p, func() int64 { return 1 })
	if len(res.Failures) != 0 {
		t.Fatalf("expected 0 failures, got %+v", res.Failures)
	}
	rels, _ := svc.relationships.ListByEntity(ctx, a.ID)
	if len(rels) == 0 {
		t.Fatalf("expected relationship from %q", b.Name)
	}
}
