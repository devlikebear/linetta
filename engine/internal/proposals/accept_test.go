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
	if _, err := env.svc.Reject(context.Background(), now(), env.project.ID, []string{sub.Items[0].ID}); err != nil {
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
	if _, err := env.svc.Reject(context.Background(), now(), env.project.ID, []string{sub.Items[0].ID}); err != nil {
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

// An import sweep proposes the same cast episode after episode, so a name
// can already be in Story World by the time its item is accepted. entities
// has UNIQUE(project_id, name): this used to fail with the raw constraint
// text, and every relationship hanging off the item failed with it.
func TestAccept_linksExistingEntityAndItsDependentsResolve(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	existing, err := env.entities.Create(ctx, now(), entity.NewInput{
		ProjectID: env.project.ID, Kind: "character", Name: "백설화", Role: "주인공",
	})
	if err != nil {
		t.Fatalf("create entity: %v", err)
	}
	sub := mustSubmit(t, env, SubmitInput{
		ProjectID: env.project.ID,
		Items: []ItemInput{
			{Op: storyops.Op{Type: "create_entity", Ref: "e1", Name: "백설화", Kind: "character", Role: "공동 주인공",
				Summary: "검객", Aliases: []string{"설화"}}},
			{Op: storyops.Op{Type: "create_entity", Ref: "e2", Name: "한서린", Kind: "character"}},
			{Op: storyops.Op{Type: "create_relationship", FromRef: "e1", ToRef: "e2", Label: "동료"}},
		},
	})
	ids := []string{sub.Items[0].ID, sub.Items[1].ID, sub.Items[2].ID}
	res, err := env.svc.Accept(ctx, now(), env.project.ID, ids, nil)
	if err != nil {
		t.Fatalf("Accept error: %v", err)
	}
	for _, it := range res.Items {
		if it.Status != StatusAccepted {
			t.Fatalf("item %+v not accepted", it)
		}
	}
	if !res.Items[0].Result.LinkedExisting || res.Items[0].Result.CreatedID != existing.ID {
		t.Fatalf("result = %+v, want linked to %s", res.Items[0].Result, existing.ID)
	}
	if res.Items[1].Result.LinkedExisting {
		t.Fatalf("a new entity must not read as linked: %+v", res.Items[1].Result)
	}
	got, err := env.entities.Get(ctx, existing.ID)
	if err != nil {
		t.Fatalf("get entity: %v", err)
	}
	if got.Role != "주인공" {
		t.Errorf("role = %q; a candidate must not overwrite what the record already says", got.Role)
	}
	if got.Summary != "검객" {
		t.Errorf("summary = %q, want the blank field filled", got.Summary)
	}
	if len(got.Aliases) != 1 || got.Aliases[0] != "설화" {
		t.Errorf("aliases = %v, want the candidate's added", got.Aliases)
	}
}

// The inbox rows already stuck on the constraint error have to clear on a
// retry, along with the relationships that failed because of them.
func TestAccept_retriesAnItemThatFailedOnTheNameConstraint(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	sub := mustSubmit(t, env, SubmitInput{
		ProjectID: env.project.ID,
		Items: []ItemInput{
			{Op: storyops.Op{Type: "create_entity", Ref: "e1", Name: "곰배", Kind: "character"}},
			{Op: storyops.Op{Type: "create_relationship", FromRef: "e1", ToRef: "e1", Label: "자기자신"}},
		},
	})
	failed := ItemResult{Error: "constraint failed: UNIQUE constraint failed: entities.project_id, entities.name (2067)"}
	if err := env.svc.updateItemStatus(ctx, env.svc.store.DB(), sub.Items[0].ID, StatusFailed, failed, now()); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	if _, err := env.entities.Create(ctx, now(), entity.NewInput{
		ProjectID: env.project.ID, Kind: "character", Name: "곰배",
	}); err != nil {
		t.Fatalf("create entity: %v", err)
	}
	res, err := env.svc.Accept(ctx, now(), env.project.ID, []string{sub.Items[0].ID, sub.Items[1].ID}, nil)
	if err != nil {
		t.Fatalf("Accept error: %v", err)
	}
	for _, it := range res.Items {
		if it.Status != StatusAccepted || it.Result.Error != "" {
			t.Fatalf("item %+v not accepted", it)
		}
	}
}

// Two batches of one sweep both propose a name: whichever is accepted second
// meets the first's row, not an error.
func TestAccept_samePendingNameAcrossBatchesLinksTheSecond(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	in := SubmitInput{
		ProjectID: env.project.ID,
		Items:     []ItemInput{{Op: storyops.Op{Type: "create_entity", Name: "구양철", Kind: "character"}}},
	}
	first, second := mustSubmit(t, env, in), mustSubmit(t, env, in)
	res, err := env.svc.Accept(ctx, now(), env.project.ID, []string{first.Items[0].ID, second.Items[0].ID}, nil)
	if err != nil {
		t.Fatalf("Accept error: %v", err)
	}
	if res.Items[0].Status != StatusAccepted || res.Items[1].Status != StatusAccepted {
		t.Fatalf("items = %+v", res.Items)
	}
	if res.Items[0].Result.CreatedID != res.Items[1].Result.CreatedID {
		t.Fatalf("items resolved to different entities: %+v", res.Items)
	}
}

// A name taken by a different kind of thing is not the same record, so it is
// refused in the writer's terms rather than merged.
func TestAccept_refusesANameTakenByAnotherKind(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	if _, err := env.entities.Create(ctx, now(), entity.NewInput{
		ProjectID: env.project.ID, Kind: "place", Name: "청운",
	}); err != nil {
		t.Fatalf("create entity: %v", err)
	}
	sub := mustSubmit(t, env, SubmitInput{
		ProjectID: env.project.ID,
		Items:     []ItemInput{{Op: storyops.Op{Type: "create_entity", Name: "청운", Kind: "character"}}},
	})
	res, err := env.svc.Accept(ctx, now(), env.project.ID, []string{sub.Items[0].ID}, nil)
	if err != nil {
		t.Fatalf("Accept error: %v", err)
	}
	it := res.Items[0]
	if it.Status != StatusFailed || it.Result.Code != ErrorCodeNameTaken || strings.Contains(it.Result.Error, "constraint") {
		t.Fatalf("item = %+v, want a readable refusal", it)
	}
}

// Submit never required an entity to come before the relationship that names
// it, so the applying order must not depend on the order a model listed them.
func TestAccept_relationshipListedBeforeItsEntity(t *testing.T) {
	env := newTestEnv(t)
	sub := mustSubmit(t, env, SubmitInput{
		ProjectID: env.project.ID,
		Items: []ItemInput{
			{Op: storyops.Op{Type: "create_relationship", FromRef: "e1", ToRef: "e2", Label: "스승"}},
			{Op: storyops.Op{Type: "create_entity", Ref: "e1", Name: "하린"}},
			{Op: storyops.Op{Type: "create_entity", Ref: "e2", Name: "유나"}},
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

// A failed row has to be dismissible: Reject skipped everything but pending,
// which left failed items in the inbox for good.
func TestReject_clearsAFailedItem(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	sub := mustSubmit(t, env, SubmitInput{
		ProjectID: env.project.ID,
		Items:     []ItemInput{{Op: storyops.Op{Type: "create_entity", Name: "하린"}}},
	})
	if err := env.svc.updateItemStatus(ctx, env.svc.store.DB(), sub.Items[0].ID, StatusFailed, ItemResult{Error: "x"}, now()); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	n, err := env.svc.Reject(ctx, now(), env.project.ID, []string{sub.Items[0].ID})
	if err != nil {
		t.Fatalf("Reject error: %v", err)
	}
	if n != 1 {
		t.Fatalf("rejected = %d, want 1", n)
	}
}

// The flag on a stored row is what Submit saw. A name that reached Story
// World afterwards has to show on the row too, or the writer learns about it
// only from a failed accept.
func TestList_flagsAnEntityCreatedAfterSubmission(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	mustSubmit(t, env, SubmitInput{
		ProjectID: env.project.ID,
		Items:     []ItemInput{{Op: storyops.Op{Type: "create_entity", Name: "곰배", Kind: "character"}}},
	})
	ent, err := env.entities.Create(ctx, now(), entity.NewInput{
		ProjectID: env.project.ID, Kind: "character", Name: "곰배",
	})
	if err != nil {
		t.Fatalf("create entity: %v", err)
	}
	list, err := env.svc.List(ctx, env.project.ID, nil)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	dups := list[0].Items[0].Duplicates
	if len(dups) != 1 || dups[0].EntityID != ent.ID {
		t.Fatalf("duplicates = %+v, want the entity created after submission", dups)
	}
}
