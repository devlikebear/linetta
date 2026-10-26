package proposals

import (
	"context"
	"errors"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/storyops"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

// setUpAcceptedStructuralBatch submits and accepts a create_scene +
// add_beat forecast batch, returning enough to drive Undo at it: the
// batch id, the ids of both covered items, and the scene/beat ids Undo is
// expected to either revert or leave alone depending on where a forced
// failure lands.
func setUpAcceptedStructuralBatch(t *testing.T, env *testEnv) (batchID string, itemIDs []string, sceneID, beatID string, th thread.Thread) {
	t.Helper()
	ctx := context.Background()
	th, err := env.threads.Create(ctx, thread.NewInput{ProjectID: env.project.ID, Name: "주 갈래"})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	sub, _ := mustSubmitForecast(t, env, []ItemInput{
		{Op: storyops.Op{Type: "create_scene", Ref: "s1", Label: "맞선 장면", AfterNodeID: env.sceneID}},
		{Op: storyops.Op{Type: "add_beat", ThreadID: th.ID, NodeRef: "s1", Label: "맞선 비트"}},
	})
	itemIDs = []string{sub.Items[0].ID, sub.Items[1].ID}

	res, err := env.svc.Accept(ctx, now(), env.project.ID, itemIDs, nil)
	if err != nil {
		t.Fatalf("Accept error: %v", err)
	}
	if len(res.UndoBatchIDs) != 1 {
		t.Fatalf("UndoBatchIDs = %+v, want exactly one", res.UndoBatchIDs)
	}
	return res.UndoBatchIDs[0], itemIDs, res.Items[0].Result.CreatedID, res.Items[1].Result.CreatedID, th
}

// TestUndo_statusWriteFailureLeavesOutlineAndItemsUntouched covers the
// first half of the atomicity fix: if the transaction that carries the
// batch's items to pending never commits, storyops.UndoApply must never
// have been called either — the outline (and the beat UndoApply never
// gets asked to drop) stays exactly as Accept left it, and the items stay
// at their pre-Undo accepted+batchID, so a plain retry (once whatever
// broke the write is fixed) can still succeed.
func TestUndo_statusWriteFailureLeavesOutlineAndItemsUntouched(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	batchID, itemIDs, sceneID, beatID, th := setUpAcceptedStructuralBatch(t, env)

	// Force the "carry to pending" transaction to fail on its very first
	// write, before Undo ever reaches storyops.
	if _, err := env.svc.store.DB().ExecContext(ctx,
		`CREATE TRIGGER fail_item_update BEFORE UPDATE ON story_proposal_items
		 BEGIN SELECT RAISE(ABORT, 'forced failure for test'); END;`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	if err := env.svc.Undo(ctx, now(), env.project.ID, batchID); err == nil {
		t.Fatal("Undo succeeded despite a forced status-write failure")
	}

	if _, err := env.svc.store.DB().ExecContext(ctx, `DROP TRIGGER fail_item_update`); err != nil {
		t.Fatalf("drop trigger: %v", err)
	}

	// The outline never moved: UndoApply was never reached.
	if _, err := env.nodes.Get(ctx, sceneID); err != nil {
		t.Errorf("Get scene: %v, want it to still exist — UndoApply must not run before items commit to pending", err)
	}
	beats, err := env.beats.ListByThread(ctx, th.ID)
	if err != nil {
		t.Fatalf("ListByThread: %v", err)
	}
	found := false
	for _, b := range beats {
		if b.ID == beatID {
			found = true
		}
	}
	if !found {
		t.Error("beat missing — it must not be deleted before the outline is actually restored")
	}

	// Both items are exactly where Accept left them.
	for _, id := range itemIDs {
		it, err := env.svc.getItem(ctx, env.svc.store.DB(), id)
		if err != nil {
			t.Fatalf("getItem %s: %v", id, err)
		}
		if it.Status != StatusAccepted {
			t.Errorf("item %s status = %q, want accepted (unchanged)", id, it.Status)
		}
		if it.UndoBatchID != batchID {
			t.Errorf("item %s UndoBatchID = %q, want %q (unchanged)", id, it.UndoBatchID, batchID)
		}
	}

	// Retryable: with the trigger gone, the same batch id still undoes
	// cleanly.
	if err := env.svc.Undo(ctx, now(), env.project.ID, batchID); err != nil {
		t.Fatalf("retry Undo: %v", err)
	}
	if _, err := env.nodes.Get(ctx, sceneID); err == nil {
		t.Error("scene still exists after the retried Undo")
	}
}

// TestUndo_undoApplyFailureRestoresItemsForRetry covers the second half:
// the "carry to pending" transaction commits, but storyops.UndoApply then
// fails to restore the outline (its own transaction — see node.Repo's
// RestoreOutline — rolls back cleanly, so the outline is untouched too).
// The items must be written straight back to accepted+batchID so the same
// batch id is still valid for a later retry, instead of being stranded at
// pending with no batch left in storyops' undo window.
func TestUndo_undoApplyFailureRestoresItemsForRetry(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	batchID, itemIDs, sceneID, beatID, th := setUpAcceptedStructuralBatch(t, env)

	// Force storyops.UndoApply's own outline write to fail: RestoreOutline
	// deletes the newly created scene node to put the outline back, so a
	// trigger that blocks DELETE on nodes makes that call fail.
	if _, err := env.svc.store.DB().ExecContext(ctx,
		`CREATE TRIGGER fail_node_delete BEFORE DELETE ON nodes
		 BEGIN SELECT RAISE(ABORT, 'forced failure for test'); END;`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	err := env.svc.Undo(ctx, now(), env.project.ID, batchID)
	if err == nil {
		t.Fatal("Undo succeeded despite a forced outline-restore failure")
	}
	if errors.Is(err, ErrUndoBatchNotFound) {
		t.Fatalf("Undo error = %v, want a real failure, not ErrUndoBatchNotFound — the batch still exists, it just failed to apply", err)
	}

	if _, err := env.svc.store.DB().ExecContext(ctx, `DROP TRIGGER fail_node_delete`); err != nil {
		t.Fatalf("drop trigger: %v", err)
	}

	// The outline was never actually touched (RestoreOutline's own
	// transaction rolled back), and the beat Undo would otherwise have
	// deleted must not have been touched either — that only happens after
	// UndoApply succeeds.
	if _, err := env.nodes.Get(ctx, sceneID); err != nil {
		t.Errorf("Get scene: %v, want it to still exist — RestoreOutline's own transaction must have rolled back", err)
	}
	beats, err := env.beats.ListByThread(ctx, th.ID)
	if err != nil {
		t.Fatalf("ListByThread: %v", err)
	}
	found := false
	for _, b := range beats {
		if b.ID == beatID {
			found = true
		}
	}
	if !found {
		t.Error("beat missing after a failed UndoApply — it must only be deleted once the outline restore actually succeeds")
	}

	// Both items are restored to accepted+batchID — retryable, not
	// stranded at pending with a dead batch id.
	for _, id := range itemIDs {
		it, err := env.svc.getItem(ctx, env.svc.store.DB(), id)
		if err != nil {
			t.Fatalf("getItem %s: %v", id, err)
		}
		if it.Status != StatusAccepted {
			t.Errorf("item %s status = %q, want accepted (restored for retry)", id, it.Status)
		}
		if it.UndoBatchID != batchID {
			t.Errorf("item %s UndoBatchID = %q, want %q (restored for retry)", id, it.UndoBatchID, batchID)
		}
	}

	// Retrying the very same batch id now succeeds.
	if err := env.svc.Undo(ctx, now(), env.project.ID, batchID); err != nil {
		t.Fatalf("retry Undo: %v", err)
	}
	if _, err := env.nodes.Get(ctx, sceneID); err == nil {
		t.Error("scene still exists after the retried Undo")
	} else if !errors.Is(err, node.ErrNotFound) {
		t.Errorf("Get scene after retried Undo: %v, want ErrNotFound", err)
	}
}

// TestUndo_doubleFailureLeavesItemRevertingAndBlocksReaccept covers the
// gap PR #236 left open: step 2 (storyops.UndoApply) fails AND the
// compensating write back to accepted (step 2's own restoreItems) also
// fails. The batch's items must be left at StatusReverting with their
// original result and undo_batch_id intact (step 1 only ever touches
// status/status_at) — not at pending with the outline's node still
// standing, which is what let a re-Accept create a duplicate node before
// this fix. Accept must refuse a reverting item outright, and a sibling
// item that still depends on it (not yet accepted) must fail with an
// unmet-dependency error rather than being allowed to apply ahead of it.
func TestUndo_doubleFailureLeavesItemRevertingAndBlocksReaccept(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	th, err := env.threads.Create(ctx, thread.NewInput{ProjectID: env.project.ID, Name: "주 갈래"})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}

	// Submit three items in one proposal: the structural scene, a beat
	// riding its ref (accepted together with it below), and a SECOND beat
	// on the same ref left pending — the dependent item whose fate this
	// test also checks.
	sub, _ := mustSubmitForecast(t, env, []ItemInput{
		{Op: storyops.Op{Type: "create_scene", Ref: "s1", Label: "맞선 장면", AfterNodeID: env.sceneID}},
		{Op: storyops.Op{Type: "add_beat", ThreadID: th.ID, NodeRef: "s1", Label: "맞선 비트"}},
		{Op: storyops.Op{Type: "add_beat", ThreadID: th.ID, NodeRef: "s1", Label: "두 번째 비트"}},
	})
	sceneItemID, beatItemID, pendingDependentID := sub.Items[0].ID, sub.Items[1].ID, sub.Items[2].ID

	res, err := env.svc.Accept(ctx, now(), env.project.ID, []string{sceneItemID, beatItemID}, nil)
	if err != nil {
		t.Fatalf("Accept error: %v", err)
	}
	if len(res.UndoBatchIDs) != 1 {
		t.Fatalf("UndoBatchIDs = %+v, want exactly one", res.UndoBatchIDs)
	}
	batchID := res.UndoBatchIDs[0]
	sceneID, beatID := res.Items[0].Result.CreatedID, res.Items[1].Result.CreatedID

	before, err := env.svc.getItem(ctx, env.svc.store.DB(), sceneItemID)
	if err != nil {
		t.Fatalf("getItem (before): %v", err)
	}

	// Force storyops.UndoApply's own outline write to fail (same as the
	// existing undoApplyFailure test), AND force the compensating write
	// back to accepted to fail too — scoped with WHEN so step 1's own
	// write to 'reverting' still succeeds.
	if _, err := env.svc.store.DB().ExecContext(ctx,
		`CREATE TRIGGER fail_node_delete BEFORE DELETE ON nodes
		 BEGIN SELECT RAISE(ABORT, 'forced failure for test'); END;`); err != nil {
		t.Fatalf("create node trigger: %v", err)
	}
	if _, err := env.svc.store.DB().ExecContext(ctx,
		`CREATE TRIGGER fail_restore_to_accepted BEFORE UPDATE ON story_proposal_items
		 WHEN NEW.status = 'accepted'
		 BEGIN SELECT RAISE(ABORT, 'forced failure for test'); END;`); err != nil {
		t.Fatalf("create item trigger: %v", err)
	}

	err = env.svc.Undo(ctx, now(), env.project.ID, batchID)
	if err == nil {
		t.Fatal("Undo succeeded despite forced UndoApply and compensation failures")
	}
	if errors.Is(err, ErrUndoBatchNotFound) {
		t.Fatalf("Undo error = %v, want a real failure, not ErrUndoBatchNotFound", err)
	}

	// The outline and the beat are both untouched — RestoreOutline's own
	// transaction rolled back.
	if _, err := env.nodes.Get(ctx, sceneID); err != nil {
		t.Errorf("Get scene: %v, want it to still exist", err)
	}
	if _, err := env.beats.Get(ctx, beatID); err != nil {
		t.Errorf("Get beat: %v, want it to still exist", err)
	}

	// Both covered items are stuck at reverting, not pending — with their
	// ORIGINAL result and undo_batch_id intact, exactly as step 1 left
	// them.
	for _, id := range []string{sceneItemID, beatItemID} {
		it, err := env.svc.getItem(ctx, env.svc.store.DB(), id)
		if err != nil {
			t.Fatalf("getItem %s: %v", id, err)
		}
		if it.Status != StatusReverting {
			t.Errorf("item %s status = %q, want reverting", id, it.Status)
		}
		if it.UndoBatchID != batchID {
			t.Errorf("item %s UndoBatchID = %q, want %q (preserved)", id, it.UndoBatchID, batchID)
		}
	}
	after, err := env.svc.getItem(ctx, env.svc.store.DB(), sceneItemID)
	if err != nil {
		t.Fatalf("getItem (after): %v", err)
	}
	if after.Result.CreatedID != before.Result.CreatedID {
		t.Errorf("scene item result = %+v, want unchanged %+v", after.Result, before.Result)
	}

	nodesBefore, err := env.nodes.ListByProject(ctx, env.project.ID)
	if err != nil {
		t.Fatalf("ListByProject: %v", err)
	}

	// Re-accepting the reverting items must not apply them again — no
	// second node, no status change.
	res2, err := env.svc.Accept(ctx, now(), env.project.ID, []string{sceneItemID, beatItemID}, nil)
	if err != nil {
		t.Fatalf("Accept (reverting items) error: %v", err)
	}
	for _, it := range res2.Items {
		if it.Status != StatusReverting {
			t.Errorf("re-Accept item %s status = %q, want reverting (untouched)", it.ID, it.Status)
		}
	}
	nodesAfter, err := env.nodes.ListByProject(ctx, env.project.ID)
	if err != nil {
		t.Fatalf("ListByProject: %v", err)
	}
	if len(nodesAfter) != len(nodesBefore) {
		t.Errorf("node count changed from %d to %d — re-Accept must not create a duplicate node", len(nodesBefore), len(nodesAfter))
	}

	// The sibling item still waiting on the scene's ref must fail with an
	// unmet-dependency error, not be allowed to apply ahead of it.
	res3, err := env.svc.Accept(ctx, now(), env.project.ID, []string{pendingDependentID}, nil)
	if err != nil {
		t.Fatalf("Accept (pending dependent) error: %v", err)
	}
	if len(res3.Items) != 1 || res3.Items[0].Status != StatusFailed {
		t.Fatalf("pending dependent Accept result = %+v, want exactly one failed item", res3.Items)
	}
	if res3.Items[0].Result.Error == "" {
		t.Error("pending dependent's failure has no error message")
	}
}

// TestUndo_finalizeFailureAfterSuccessfulUndoApplyLeavesItemReverting
// covers the other half of the double-failure gap: storyops.UndoApply
// (step 2) succeeds — the outline is actually restored — but the
// finalize write (step 3, carrying items the rest of the way to pending)
// fails. The items must stay at reverting with their ORIGINAL result and
// undo_batch_id (step 1 never touches those columns), and the covered
// add_beat item's beat must already be gone — finalizeReverted deletes it
// before attempting the write that then fails, exactly like a successful
// Undo would.
func TestUndo_finalizeFailureAfterSuccessfulUndoApplyLeavesItemReverting(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	batchID, itemIDs, sceneID, beatID, _ := setUpAcceptedStructuralBatch(t, env)

	before, err := env.svc.getItem(ctx, env.svc.store.DB(), itemIDs[0])
	if err != nil {
		t.Fatalf("getItem (before): %v", err)
	}

	// Scoped with WHEN so step 1's write to 'reverting' still succeeds —
	// only the finalize write to 'pending' is blocked.
	if _, err := env.svc.store.DB().ExecContext(ctx,
		`CREATE TRIGGER fail_finalize_to_pending BEFORE UPDATE ON story_proposal_items
		 WHEN NEW.status = 'pending'
		 BEGIN SELECT RAISE(ABORT, 'forced failure for test'); END;`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	err = env.svc.Undo(ctx, now(), env.project.ID, batchID)
	if err == nil {
		t.Fatal("Undo succeeded despite a forced finalize failure")
	}

	// The outline really did move: UndoApply itself succeeded.
	if _, err := env.nodes.Get(ctx, sceneID); err == nil {
		t.Error("scene still exists — UndoApply should have succeeded here")
	} else if !errors.Is(err, node.ErrNotFound) {
		t.Errorf("Get scene: %v, want ErrNotFound", err)
	}
	// The beat is already gone too — finalizeReverted deletes it before
	// the transaction that then fails.
	if _, err := env.beats.Get(ctx, beatID); err == nil {
		t.Error("beat still exists — finalizeReverted should have deleted it before the failed write")
	}

	for _, id := range itemIDs {
		it, err := env.svc.getItem(ctx, env.svc.store.DB(), id)
		if err != nil {
			t.Fatalf("getItem %s: %v", id, err)
		}
		if it.Status != StatusReverting {
			t.Errorf("item %s status = %q, want reverting", id, it.Status)
		}
		if it.UndoBatchID != batchID {
			t.Errorf("item %s UndoBatchID = %q, want %q (preserved)", id, it.UndoBatchID, batchID)
		}
	}
	after, err := env.svc.getItem(ctx, env.svc.store.DB(), itemIDs[0])
	if err != nil {
		t.Fatalf("getItem (after): %v", err)
	}
	if after.Result.CreatedID != before.Result.CreatedID {
		t.Errorf("scene item result = %+v, want unchanged %+v", after.Result, before.Result)
	}
}
