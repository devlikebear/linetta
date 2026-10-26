package proposals

import (
	"context"
	"errors"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/node"
)

// moveToReverting is a white-box test helper that reproduces exactly what
// Undo's own step 1 does — flip status to reverting, leave result_json and
// undo_batch_id untouched — without going anywhere near storyops. Tests in
// this file use it to put a batch's items into the state reconcileBatch
// must resolve from DB facts alone, while keeping full control over
// whether storyops' in-memory undo window for that batch is still alive.
func moveToReverting(t *testing.T, env *testEnv, itemIDs []string) {
	t.Helper()
	ctx := context.Background()
	for _, id := range itemIDs {
		if err := env.svc.updateItemStatusOnly(ctx, env.svc.store.DB(), id, StatusReverting, now()); err != nil {
			t.Fatalf("moveToReverting %s: %v", id, err)
		}
	}
}

// TestReconcileBatch_nodeStillExists_restoresToAcceptedAndWindowStaysRetryable
// covers the first reconcile scenario: a batch's items are stuck at
// reverting (as if an earlier Undo call's step 2/3 failed) and the
// outline fact is unambiguous — the structural item's created node is
// still there. reconcileBatch must restore every covered item to
// accepted+original result+batchID, purely from that fact, and — since
// this test never actually called storyops.UndoApply — storyops' own
// undo window for the batch is still alive, so a normal Undo retry right
// afterward succeeds and actually reverts the outline.
func TestReconcileBatch_nodeStillExists_restoresToAcceptedAndWindowStaysRetryable(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	batchID, itemIDs, sceneID, beatID, th := setUpAcceptedStructuralBatch(t, env)

	before := map[string]Item{}
	for _, id := range itemIDs {
		it, err := env.svc.getItem(ctx, env.svc.store.DB(), id)
		if err != nil {
			t.Fatalf("getItem %s (before): %v", id, err)
		}
		before[id] = it
	}

	moveToReverting(t, env, itemIDs)

	status, err := env.svc.reconcileBatch(ctx, now(), env.project.ID, batchID)
	if err != nil {
		t.Fatalf("reconcileBatch: %v", err)
	}
	if status != StatusAccepted {
		t.Fatalf("reconcileBatch status = %q, want %q", status, StatusAccepted)
	}

	for _, id := range itemIDs {
		it, err := env.svc.getItem(ctx, env.svc.store.DB(), id)
		if err != nil {
			t.Fatalf("getItem %s (after reconcile): %v", id, err)
		}
		if it.Status != StatusAccepted {
			t.Errorf("item %s status = %q, want accepted", id, it.Status)
		}
		if it.UndoBatchID != batchID {
			t.Errorf("item %s UndoBatchID = %q, want %q", id, it.UndoBatchID, batchID)
		}
		if it.Result.CreatedID != before[id].Result.CreatedID {
			t.Errorf("item %s result = %+v, want unchanged %+v", id, it.Result, before[id].Result)
		}
	}

	// Calling reconcileBatch again changes nothing — the items are no
	// longer reverting, so it has nothing to resolve.
	status2, err := env.svc.reconcileBatch(ctx, now(), env.project.ID, batchID)
	if err != nil {
		t.Fatalf("reconcileBatch (second call): %v", err)
	}
	if status2 != "" {
		t.Errorf("reconcileBatch (second call) status = %q, want \"\" (nothing reverting left)", status2)
	}
	for _, id := range itemIDs {
		it, err := env.svc.getItem(ctx, env.svc.store.DB(), id)
		if err != nil {
			t.Fatalf("getItem %s (after second reconcile): %v", id, err)
		}
		if it.Status != StatusAccepted {
			t.Errorf("item %s status = %q after second reconcile, want unchanged accepted", id, it.Status)
		}
	}

	// storyops' own window was never touched by this test — the batch is
	// still retryable through a completely normal Undo call.
	if err := env.svc.Undo(ctx, now(), env.project.ID, batchID); err != nil {
		t.Fatalf("Undo after reconcile: %v", err)
	}
	if _, err := env.nodes.Get(ctx, sceneID); err == nil {
		t.Error("scene still exists after the retried Undo")
	} else if !errors.Is(err, node.ErrNotFound) {
		t.Errorf("Get scene after retried Undo: %v, want ErrNotFound", err)
	}
	beats, err := env.beats.ListByThread(ctx, th.ID)
	if err != nil {
		t.Fatalf("ListByThread: %v", err)
	}
	for _, b := range beats {
		if b.ID == beatID {
			t.Error("beat still exists after the retried Undo finished")
		}
	}
}

// TestReconcileBatch_nodeGone_finalizesToPendingAndCleansBeat covers the
// second reconcile scenario: storyops.UndoApply actually succeeded (the
// node is gone) but this package's own finalize write never landed, so
// the items are stuck at reverting with their stale result still
// pointing at a node that no longer exists. reconcileBatch must finish
// the job — pending, result cleared, undo_batch_id cleared — and clean up
// the now-orphaned beat, purely from the fact that the node is gone.
func TestReconcileBatch_nodeGone_finalizesToPendingAndCleansBeat(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	batchID, itemIDs, sceneID, beatID, th := setUpAcceptedStructuralBatch(t, env)

	// Move to reverting first (as Undo's step 1 would), then actually run
	// storyops.UndoApply for real — this is exactly "UndoApply succeeded,
	// the finalize write after it never ran" (it consumes storyops' own
	// window entry for batchID too, matching the real post-success
	// state).
	moveToReverting(t, env, itemIDs)
	if err := env.storyops.UndoApply(ctx, batchID, func() int64 { return now() }); err != nil {
		t.Fatalf("UndoApply: %v", err)
	}
	if _, err := env.nodes.Get(ctx, sceneID); err == nil {
		t.Fatal("scene still exists after UndoApply — test setup is wrong")
	}

	status, err := env.svc.reconcileBatch(ctx, now(), env.project.ID, batchID)
	if err != nil {
		t.Fatalf("reconcileBatch: %v", err)
	}
	if status != StatusPending {
		t.Fatalf("reconcileBatch status = %q, want %q", status, StatusPending)
	}

	for _, id := range itemIDs {
		it, err := env.svc.getItem(ctx, env.svc.store.DB(), id)
		if err != nil {
			t.Fatalf("getItem %s (after reconcile): %v", id, err)
		}
		if it.Status != StatusPending {
			t.Errorf("item %s status = %q, want pending", id, it.Status)
		}
		if it.UndoBatchID != "" {
			t.Errorf("item %s UndoBatchID = %q, want cleared", id, it.UndoBatchID)
		}
		if it.Result.CreatedID != "" || it.Result.Error != "" {
			t.Errorf("item %s result = %+v, want cleared", id, it.Result)
		}
	}
	beats, err := env.beats.ListByThread(ctx, th.ID)
	if err != nil {
		t.Fatalf("ListByThread: %v", err)
	}
	for _, b := range beats {
		if b.ID == beatID {
			t.Error("beat still exists after reconcile finalized the batch to pending")
		}
	}

	// Calling reconcileBatch again changes nothing — the items are no
	// longer reverting.
	status2, err := env.svc.reconcileBatch(ctx, now(), env.project.ID, batchID)
	if err != nil {
		t.Fatalf("reconcileBatch (second call): %v", err)
	}
	if status2 != "" {
		t.Errorf("reconcileBatch (second call) status = %q, want \"\" (nothing reverting left)", status2)
	}
}

// TestUndo_retryAfterFinalizeFailureReconcilesToPendingViaUndo exercises
// the SAME finalize-failure-then-retry scenario through the public Undo
// entry point, not reconcileBatch directly, so Undo's own wiring (it must
// call reconcileBatch when storyops answers ErrUndoBatchNotFound, and
// return nil — not ErrUndoBatchNotFound — when reconcile resolves to
// pending) is covered, not just reconcileBatch in isolation.
func TestUndo_retryAfterFinalizeFailureReconcilesToPendingViaUndo(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	batchID, itemIDs, sceneID, beatID, _ := setUpAcceptedStructuralBatch(t, env)

	if _, err := env.svc.store.DB().ExecContext(ctx,
		`CREATE TRIGGER fail_finalize_to_pending BEFORE UPDATE ON story_proposal_items
		 WHEN NEW.status = 'pending'
		 BEGIN SELECT RAISE(ABORT, 'forced failure for test'); END;`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	if err := env.svc.Undo(ctx, now(), env.project.ID, batchID); err == nil {
		t.Fatal("Undo succeeded despite a forced finalize failure")
	}
	// Sanity: UndoApply really did succeed this first time (the outline
	// moved), so the retry below exercises storyops answering
	// ErrUndoBatchNotFound, not a transient real failure.
	if _, err := env.nodes.Get(ctx, sceneID); err == nil {
		t.Fatal("scene still exists — the first Undo call's UndoApply should have succeeded")
	}

	if _, err := env.svc.store.DB().ExecContext(ctx, `DROP TRIGGER fail_finalize_to_pending`); err != nil {
		t.Fatalf("drop trigger: %v", err)
	}

	// Retry: storyops no longer has this batch (UndoApply already
	// consumed it the first time) — Undo must reconcile from the outline
	// fact (node gone) and return nil, not ErrUndoBatchNotFound.
	if err := env.svc.Undo(ctx, now(), env.project.ID, batchID); err != nil {
		t.Fatalf("retry Undo: %v, want nil (reconciled to pending)", err)
	}

	for _, id := range itemIDs {
		it, err := env.svc.getItem(ctx, env.svc.store.DB(), id)
		if err != nil {
			t.Fatalf("getItem %s: %v", id, err)
		}
		if it.Status != StatusPending {
			t.Errorf("item %s status = %q, want pending", id, it.Status)
		}
		if it.UndoBatchID != "" {
			t.Errorf("item %s UndoBatchID = %q, want cleared", id, it.UndoBatchID)
		}
	}
	if _, err := env.beats.Get(ctx, beatID); err == nil {
		t.Error("beat still exists after the retried Undo reconciled the batch to pending")
	}
}

// TestReconcileReverting_resolvesEveryBatchFromItsOwnFact exercises the
// orchestration ReconcileReverting does on top of reconcileBatch — group
// every reverting item by (project, undo_batch_id) and resolve each
// group once — with two independent batches left reverting for two
// different reasons at once: one whose node is still there (must end up
// accepted) and one whose node is already gone (must end up pending, beat
// cleaned). Neither batch's resolution may affect the other's.
func TestReconcileReverting_resolvesEveryBatchFromItsOwnFact(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	batchA, itemsA, sceneA, _, _ := setUpAcceptedStructuralBatch(t, env)
	batchB, itemsB, sceneB, beatB, _ := setUpAcceptedStructuralBatch(t, env)

	moveToReverting(t, env, itemsA) // left as-is: node A still exists
	moveToReverting(t, env, itemsB)
	if err := env.storyops.UndoApply(ctx, batchB, func() int64 { return now() }); err != nil {
		t.Fatalf("UndoApply batch B: %v", err)
	}

	if err := env.svc.ReconcileReverting(ctx, now()); err != nil {
		t.Fatalf("ReconcileReverting: %v", err)
	}

	for _, id := range itemsA {
		it, err := env.svc.getItem(ctx, env.svc.store.DB(), id)
		if err != nil {
			t.Fatalf("getItem %s: %v", id, err)
		}
		if it.Status != StatusAccepted {
			t.Errorf("batch A item %s status = %q, want accepted", id, it.Status)
		}
		if it.UndoBatchID != batchA {
			t.Errorf("batch A item %s UndoBatchID = %q, want %q", id, it.UndoBatchID, batchA)
		}
	}
	if _, err := env.nodes.Get(ctx, sceneA); err != nil {
		t.Errorf("Get scene A: %v, want it to still exist", err)
	}

	for _, id := range itemsB {
		it, err := env.svc.getItem(ctx, env.svc.store.DB(), id)
		if err != nil {
			t.Fatalf("getItem %s: %v", id, err)
		}
		if it.Status != StatusPending {
			t.Errorf("batch B item %s status = %q, want pending", id, it.Status)
		}
		if it.UndoBatchID != "" {
			t.Errorf("batch B item %s UndoBatchID = %q, want cleared", id, it.UndoBatchID)
		}
	}
	if _, err := env.nodes.Get(ctx, sceneB); err == nil {
		t.Error("scene B still exists — UndoApply should have removed it")
	}
	if _, err := env.beats.Get(ctx, beatB); err == nil {
		t.Error("beat B still exists after ReconcileReverting finalized its batch to pending")
	}
}
