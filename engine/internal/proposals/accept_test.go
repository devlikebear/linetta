package proposals

import (
	"context"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/storyops"
)

func mustSubmit(t *testing.T, env *testEnv, in SubmitInput) SubmitResult {
	t.Helper()
	res, problems, err := env.svc.Submit(context.Background(), now(), in, SourceAgent)
	if err != nil || len(problems) != 0 {
		t.Fatalf("Submit() problems=%+v err=%v", problems, err)
	}
	return res
}

func TestAccept_withNoDependency(t *testing.T) {
	env := newTestEnv(t)
	sub := mustSubmit(t, env, SubmitInput{
		ProjectID: env.project.ID,
		Items:     []ItemInput{{Op: storyops.Op{Type: "create_entity", Name: "하린", Kind: "character"}}},
	})
	res, err := env.svc.Accept(context.Background(), now(), env.project.ID, []string{sub.Items[0].ID}, nil)
	if err != nil {
		t.Fatalf("Accept error: %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].Status != StatusAccepted || res.Items[0].Result.CreatedID == "" {
		t.Fatalf("result = %+v", res.Items)
	}
	ent, err := env.entities.Get(context.Background(), res.Items[0].Result.CreatedID)
	if err != nil || ent.Name != "하린" {
		t.Fatalf("entity = %+v, err = %v", ent, err)
	}
}

func TestAccept_withDependencyAcceptedTogether(t *testing.T) {
	env := newTestEnv(t)
	sub := mustSubmit(t, env, SubmitInput{
		ProjectID: env.project.ID,
		Items: []ItemInput{
			{Op: storyops.Op{Type: "create_entity", Ref: "e1", Name: "하린"}},
			{Op: storyops.Op{Type: "create_entity", Ref: "e2", Name: "유나"}},
			{Op: storyops.Op{Type: "create_relationship", FromRef: "e1", ToRef: "e2", Label: "스승"}},
		},
	})
	ids := []string{sub.Items[0].ID, sub.Items[1].ID, sub.Items[2].ID}
	res, err := env.svc.Accept(context.Background(), now(), env.project.ID, ids, nil)
	if err != nil {
		t.Fatalf("Accept error: %v", err)
	}
	for _, it := range res.Items {
		if it.Status != StatusAccepted {
			t.Fatalf("item %+v not accepted", it)
		}
	}
}

func TestAccept_refusedWhenDependencyMissing(t *testing.T) {
	env := newTestEnv(t)
	sub := mustSubmit(t, env, SubmitInput{
		ProjectID: env.project.ID,
		Items: []ItemInput{
			{Op: storyops.Op{Type: "create_entity", Ref: "e1", Name: "하린"}},
			{Op: storyops.Op{Type: "create_relationship", FromRef: "e1", ToRef: "e1", Label: "자기자신"}},
		},
	})
	// Only accept the relationship item, not its dependency.
	res, err := env.svc.Accept(context.Background(), now(), env.project.ID, []string{sub.Items[1].ID}, nil)
	if err != nil {
		t.Fatalf("Accept error: %v", err)
	}
	if res.Items[0].Status != StatusFailed || !strings.Contains(res.Items[0].Result.Error, "has not been accepted") {
		t.Fatalf("item = %+v, want failed naming the missing dependency", res.Items[0])
	}
}

func TestAccept_usesPreviouslyAcceptedRef(t *testing.T) {
	env := newTestEnv(t)
	sub := mustSubmit(t, env, SubmitInput{
		ProjectID: env.project.ID,
		Items: []ItemInput{
			{Op: storyops.Op{Type: "create_entity", Ref: "e1", Name: "하린"}},
			{Op: storyops.Op{Type: "create_entity", Ref: "e2", Name: "유나"}},
			{Op: storyops.Op{Type: "create_relationship", FromRef: "e1", ToRef: "e2", Label: "스승"}},
		},
	})
	ctx := context.Background()
	// Accept the two entities first, in a separate call...
	first, err := env.svc.Accept(ctx, now(), env.project.ID, []string{sub.Items[0].ID, sub.Items[1].ID}, nil)
	if err != nil {
		t.Fatalf("first Accept error: %v", err)
	}
	for _, it := range first.Items {
		if it.Status != StatusAccepted {
			t.Fatalf("setup item not accepted: %+v", it)
		}
	}
	// ...then the relationship in a LATER call, which must resolve against
	// the already-accepted entities' real ids.
	second, err := env.svc.Accept(ctx, now(), env.project.ID, []string{sub.Items[2].ID}, nil)
	if err != nil {
		t.Fatalf("second Accept error: %v", err)
	}
	if second.Items[0].Status != StatusAccepted {
		t.Fatalf("relationship item = %+v, want accepted using the earlier call's created refs", second.Items[0])
	}
}

func TestAccept_withEditedOp(t *testing.T) {
	env := newTestEnv(t)
	sub := mustSubmit(t, env, SubmitInput{
		ProjectID: env.project.ID,
		Items:     []ItemInput{{Op: storyops.Op{Type: "create_entity", Name: "하린", Summary: "원래 요약"}}},
	})
	edits := map[string]storyops.Op{
		sub.Items[0].ID: {Type: "create_entity", Name: "하린", Summary: "고친 요약"},
	}
	res, err := env.svc.Accept(context.Background(), now(), env.project.ID, []string{sub.Items[0].ID}, edits)
	if err != nil {
		t.Fatalf("Accept error: %v", err)
	}
	if res.Items[0].Status != StatusAccepted {
		t.Fatalf("item = %+v", res.Items[0])
	}
	ent, err := env.entities.Get(context.Background(), res.Items[0].Result.CreatedID)
	if err != nil || ent.Summary != "고친 요약" {
		t.Fatalf("entity = %+v, err = %v, want the edited summary applied", ent, err)
	}
}

func TestAccept_aliasConversionEdit(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	existing, err := env.entities.Create(ctx, now(), entity.NewInput{ProjectID: env.project.ID, Name: "김민호", Kind: "character"})
	if err != nil {
		t.Fatalf("seed entity: %v", err)
	}
	sub := mustSubmit(t, env, SubmitInput{
		ProjectID: env.project.ID,
		Items:     []ItemInput{{Op: storyops.Op{Type: "create_entity", Name: "김 민호", Kind: "character"}}},
	})
	if len(sub.Items[0].Duplicates) == 0 {
		t.Fatal("expected a duplicate flag against the existing entity")
	}
	// The writer chooses "convert to alias of the existing entity" instead
	// of creating a new one.
	edits := map[string]storyops.Op{
		sub.Items[0].ID: {Type: "update_entity", EntityID: existing.ID, Aliases: []string{"김 민호"}},
	}
	res, err := env.svc.Accept(ctx, now(), env.project.ID, []string{sub.Items[0].ID}, edits)
	if err != nil {
		t.Fatalf("Accept error: %v", err)
	}
	if res.Items[0].Status != StatusAccepted {
		t.Fatalf("item = %+v", res.Items[0])
	}
	got, err := env.entities.Get(ctx, existing.ID)
	if err != nil {
		t.Fatalf("get entity: %v", err)
	}
	found := false
	for _, a := range got.Aliases {
		if a == "김 민호" {
			found = true
		}
	}
	if !found {
		t.Fatalf("aliases = %+v, want 김 민호 added", got.Aliases)
	}
}

func TestAccept_partialFailureStillAppliesTheRest(t *testing.T) {
	env := newTestEnv(t)
	sub := mustSubmit(t, env, SubmitInput{
		ProjectID: env.project.ID,
		Items: []ItemInput{
			{Op: storyops.Op{Type: "create_entity", Name: "하린"}},
			{Op: storyops.Op{Type: "create_entity", Name: "유나"}},
		},
	})
	// Edit the second item into something that fails storyops validation
	// (missing required name) while the first stays untouched.
	edits := map[string]storyops.Op{
		sub.Items[1].ID: {Type: "create_entity", Name: ""},
	}
	res, err := env.svc.Accept(context.Background(), now(), env.project.ID,
		[]string{sub.Items[0].ID, sub.Items[1].ID}, edits)
	if err != nil {
		t.Fatalf("Accept error: %v", err)
	}
	if res.Items[0].Status != StatusAccepted {
		t.Fatalf("item 0 = %+v, want accepted despite item 1 failing", res.Items[0])
	}
	if res.Items[1].Status != StatusFailed {
		t.Fatalf("item 1 = %+v, want failed", res.Items[1])
	}
}

func TestReject_marksPendingItemsRejected(t *testing.T) {
	env := newTestEnv(t)
	sub := mustSubmit(t, env, SubmitInput{
		ProjectID: env.project.ID,
		Items:     []ItemInput{{Op: storyops.Op{Type: "create_entity", Name: "하린"}}},
	})
	if err := env.svc.Reject(context.Background(), now(), env.project.ID, []string{sub.Items[0].ID}); err != nil {
		t.Fatalf("Reject error: %v", err)
	}
	all, err := env.svc.listProposals(context.Background(), env.project.ID, []string{StatusRejected})
	if err != nil || len(all) != 1 || all[0].Items[0].Status != StatusRejected {
		t.Fatalf("listProposals = %+v, err = %v", all, err)
	}
}

func TestReject_doesNotCascadeToDependents(t *testing.T) {
	env := newTestEnv(t)
	sub := mustSubmit(t, env, SubmitInput{
		ProjectID: env.project.ID,
		Items: []ItemInput{
			{Op: storyops.Op{Type: "create_entity", Ref: "e1", Name: "하린"}},
			{Op: storyops.Op{Type: "create_relationship", FromRef: "e1", ToRef: "e1", Label: "자기자신"}},
		},
	})
	if err := env.svc.Reject(context.Background(), now(), env.project.ID, []string{sub.Items[0].ID}); err != nil {
		t.Fatalf("Reject error: %v", err)
	}
	all, err := env.svc.listProposals(context.Background(), env.project.ID, nil)
	if err != nil || len(all) != 1 {
		t.Fatalf("listProposals = %+v, err = %v", all, err)
	}
	var dependent Item
	for _, it := range all[0].Items {
		if it.ID == sub.Items[1].ID {
			dependent = it
		}
	}
	if dependent.Status != StatusPending {
		t.Fatalf("dependent item = %+v, want left pending (not cascaded)", dependent)
	}
}
