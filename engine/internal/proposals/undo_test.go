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
