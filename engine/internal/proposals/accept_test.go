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

// Review fixup (f1): editing an item to drop its ref-based dependency must
// not keep it waiting on that dependency. The relationship item below
// originally depends on the entity item via from_ref/to_ref; the edit
// replaces both with real existing entity ids and removes the refs
// entirely, so the dependency no longer applies.
func TestAccept_editDroppingARefDependencyIsNotBlockedByTheStaleDependency(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	e1, err := env.entities.Create(ctx, now(), entity.NewInput{ProjectID: env.project.ID, Name: "실제 인물 1"})
	if err != nil {
		t.Fatalf("seed entity 1: %v", err)
	}
	e2, err := env.entities.Create(ctx, now(), entity.NewInput{ProjectID: env.project.ID, Name: "실제 인물 2"})
	if err != nil {
		t.Fatalf("seed entity 2: %v", err)
	}

	sub := mustSubmit(t, env, SubmitInput{
		ProjectID: env.project.ID,
		Items: []ItemInput{
			{Op: storyops.Op{Type: "create_entity", Ref: "e1", Name: "하린"}},
			{Op: storyops.Op{Type: "create_relationship", FromRef: "e1", ToRef: "e1", Label: "자기자신"}},
		},
	})
	relationshipItem := sub.Items[1]
	if len(relationshipItem.DependsOn) == 0 {
		t.Fatal("test setup: relationship item must depend on the entity item")
	}

	// Accept ONLY the relationship item, with an edit that drops the refs
	// in favor of two real, unrelated existing entities — the entity item
	// (sub.Items[0]) is deliberately left pending/unaccepted.
	edits := map[string]storyops.Op{
		relationshipItem.ID: {Type: "create_relationship", From: e1.ID, To: e2.ID, Label: "동료"},
	}
	res, err := env.svc.Accept(ctx, now(), env.project.ID, []string{relationshipItem.ID}, edits)
	if err != nil {
		t.Fatalf("Accept error: %v", err)
	}
	if res.Items[0].Status != StatusAccepted {
		t.Fatalf("item = %+v, want accepted — the edit no longer references the unaccepted entity item", res.Items[0])
	}
}

// Review fixup (f3): a previously failed item must be retryable, not stuck
// forever — the inbox's [편집] action on a failed row promises exactly this.
func TestAccept_retriesAPreviouslyFailedItem(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	sub := mustSubmit(t, env, SubmitInput{
		ProjectID: env.project.ID,
		Items: []ItemInput{
			{Op: storyops.Op{Type: "create_entity", Ref: "e1", Name: "하린"}},
			{Op: storyops.Op{Type: "create_relationship", FromRef: "e1", ToRef: "e1", Label: "자기자신"}},
		},
	})
	relationshipItem := sub.Items[1]

	// First call: the relationship fails because its dependency was not
	// accepted alongside it.
	first, err := env.svc.Accept(ctx, now(), env.project.ID, []string{relationshipItem.ID}, nil)
	if err != nil {
		t.Fatalf("first Accept error: %v", err)
	}
	if first.Items[0].Status != StatusFailed {
		t.Fatalf("setup: item = %+v, want failed", first.Items[0])
	}

	// Second call: accept the dependency and retry the failed item together.
	second, err := env.svc.Accept(ctx, now(), env.project.ID, []string{sub.Items[0].ID, relationshipItem.ID}, nil)
	if err != nil {
		t.Fatalf("second Accept error: %v", err)
	}
	for _, it := range second.Items {
		if it.Status != StatusAccepted {
			t.Fatalf("item %+v, want accepted on retry", it)
		}
	}
}

// Review fixup (f4): a create_entity item converted by edit into
// update_entity{entity_id} (the alias-conversion path) must still let a
// sibling relationship resolve its from_ref/to_ref to the entity the alias
// now points at.
func TestAccept_aliasConvertedItemStillSeedsItsRefForSiblings(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	existing, err := env.entities.Create(ctx, now(), entity.NewInput{ProjectID: env.project.ID, Name: "김민호", Kind: "character"})
	if err != nil {
		t.Fatalf("seed entity: %v", err)
	}

	sub := mustSubmit(t, env, SubmitInput{
		ProjectID: env.project.ID,
		Items: []ItemInput{
			{Op: storyops.Op{Type: "create_entity", Ref: "e1", Name: "김 민호", Kind: "character"}},
			{Op: storyops.Op{Type: "create_relationship", FromRef: "e1", ToRef: "e1", Label: "자기자신"}},
		},
	})
	createItem, relItem := sub.Items[0], sub.Items[1]

	edits := map[string]storyops.Op{
		createItem.ID: {Type: "update_entity", EntityID: existing.ID, Aliases: []string{"김 민호"}},
	}
	res, err := env.svc.Accept(ctx, now(), env.project.ID, []string{createItem.ID, relItem.ID}, edits)
	if err != nil {
		t.Fatalf("Accept error: %v", err)
	}
	var createOut, relOut Item
	for _, it := range res.Items {
		switch it.ID {
		case createItem.ID:
			createOut = it
		case relItem.ID:
			relOut = it
		}
	}
	if createOut.Status != StatusAccepted {
		t.Fatalf("create item = %+v, want accepted", createOut)
	}
	if createOut.Result.CreatedID != existing.ID {
		t.Fatalf("create item result = %+v, want created_id = %q (the aliased entity)", createOut.Result, existing.ID)
	}
	if relOut.Status != StatusAccepted {
		t.Fatalf("relationship item = %+v, want accepted — from_ref/to_ref should resolve via the alias conversion", relOut)
	}
}

// Review fixup (f4), cross-call case: the alias conversion happens in one
// Accept call; a sibling relationship is accepted in a LATER call. The
// seeded ref must be rebuilt correctly from storage (seedRefFrom reading
// back Result.CreatedID), not just carried in memory within one call.
func TestAccept_aliasConvertedRefSurvivesAcrossSeparateAcceptCalls(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	existing, err := env.entities.Create(ctx, now(), entity.NewInput{ProjectID: env.project.ID, Name: "김민호", Kind: "character"})
	if err != nil {
		t.Fatalf("seed entity: %v", err)
	}
	sub := mustSubmit(t, env, SubmitInput{
		ProjectID: env.project.ID,
		Items: []ItemInput{
			{Op: storyops.Op{Type: "create_entity", Ref: "e1", Name: "김 민호", Kind: "character"}},
			{Op: storyops.Op{Type: "create_relationship", FromRef: "e1", ToRef: "e1", Label: "자기자신"}},
		},
	})
	createItem, relItem := sub.Items[0], sub.Items[1]

	edits := map[string]storyops.Op{
		createItem.ID: {Type: "update_entity", EntityID: existing.ID, Aliases: []string{"김 민호"}},
	}
	if _, err := env.svc.Accept(ctx, now(), env.project.ID, []string{createItem.ID}, edits); err != nil {
		t.Fatalf("first Accept error: %v", err)
	}

	second, err := env.svc.Accept(ctx, now(), env.project.ID, []string{relItem.ID}, nil)
	if err != nil {
		t.Fatalf("second Accept error: %v", err)
	}
	if second.Items[0].Status != StatusAccepted {
		t.Fatalf("relationship item (later call) = %+v, want accepted", second.Items[0])
	}
}
