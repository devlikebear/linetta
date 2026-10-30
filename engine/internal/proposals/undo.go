package proposals

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/storyops"
)

// ErrUndoBatchNotFound means batchID names no story_proposal_items in this
// project — either it was already undone, or storyops' own in-memory undo
// window (at most 8 batches, gone on restart — storyops.maxUndoBatches)
// never held it in the first place. Both read the same to the writer: the
// revert button simply isn't available for this batch any more.
var ErrUndoBatchNotFound = errors.New("proposals: undo batch not found")

// Undo reverts one purpose=forecast structural batch: the
// create_outline_node/create_scene items Accept applied together in a
// single ApplyOpsWithRefs call (AcceptResult.UndoBatchIDs), plus every
// add_beat item that depended on a node one of them created. It puts the
// outline back (storyops.Service.UndoApply), deletes those add_beat items'
// created beats (RestoreOutline only reverts nodes; beats.node_id is
// ON DELETE SET NULL, not CASCADE — see migration 0029's comment), and
// carries every covered item back to pending so the writer can accept a
// corrected version instead of being stuck with a terminal failure.
//
// Scoped to a single storyops batch id on purpose, not "undo the whole
// proposal": a proposal's non-structural items (add_beat not tied to this
// batch) are untouched, matching the rest of this package's "one unit at a
// time" discipline (3.B.3).
//
// This runs in three steps, never two, because the store's single
// connection (store.go's SetMaxOpenConns(1)) means a transaction here
// cannot also call into storyops — storyops' own repo calls would block on
// that same connection and deadlock (hit in Phase 3):
//
//  1. Every covered item moves to StatusReverting — status and status_at
//     only; result_json and undo_batch_id are left exactly as Accept
//     wrote them (updateItemStatusOnly). Committed before storyops ever
//     runs, so if THIS write fails outright, nothing has changed anywhere
//     and the caller can simply retry Undo.
//  2. storyops.UndoApply actually puts the outline back.
//  3. On success, finalizeReverted deletes every covered add_beat item's
//     beat and carries every item the rest of the way to pending
//     (result cleared, undo_batch_id cleared) in its own transaction. On
//     failure, restoreItems compensates by writing every item straight
//     back to accepted+original result+batchID, so the batch is still
//     exactly what storyops.UndoApply expects on a later retry.
//
// The gap the two-step version of this left open — if step 3 ALSO fails,
// which half of it failed? Did storyops actually move the outline or
// not? — is why reverting exists as its own status instead of jumping
// straight to pending or back to accepted: an item stuck at reverting
// still carries its original result_json (so ReconcileReverting knows
// which node to ask about) and undo_batch_id (so it can still be found by
// that batch id), and Accept refuses to touch it (see accept.go's
// retryable check), so a writer cannot create a duplicate node/scene
// while the outcome is still undetermined. Resolving a reverting item
// that got stuck here is ReconcileReverting's job (reconcile.go), reading
// the outline's own facts rather than guessing — run below when storyops
// itself reports it has nothing left to retry (ErrUndoBatchNotFound), and
// once at engine startup for anything a crash left behind.
func (s *Service) Undo(ctx context.Context, now int64, projectID, batchID string) error {
	projectID = strings.TrimSpace(projectID)
	batchID = strings.TrimSpace(batchID)
	if projectID == "" || batchID == "" {
		return fmt.Errorf("proposals undo: project_id and batch_id are required")
	}

	items, err := s.listItemsByUndoBatch(ctx, s.store.DB(), projectID, batchID)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return ErrUndoBatchNotFound
	}

	// Step 1: reverting only — result_json and undo_batch_id untouched.
	// See the doc comment above for why this is its own status rather
	// than jumping straight to pending.
	tx, err := s.store.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for _, it := range items {
		if err := s.updateItemStatusOnly(ctx, tx, it.ID, StatusReverting, now); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("persist item %s status: %w", it.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reverting items: %w", err)
	}

	// Step 2: actually put the outline back.
	if err := s.storyops.UndoApply(ctx, batchID, func() int64 { return now }); err != nil {
		if errors.Is(err, storyops.ErrUndoBatchNotFound) {
			// storyops has nothing left in its undo window for this
			// batch id. Since items is non-empty (checked above), this
			// is NOT "the batch never existed" — that case already
			// returned above. It means either an earlier Undo call's
			// own UndoApply already succeeded here and consumed the
			// window entry, but this package's own step 3 failed before
			// recording that (reconcile's whole reason to exist), or the
			// engine restarted and storyops' in-memory window (at most
			// maxUndoBatches, gone on restart) simply never held this
			// batch across the restart. Either way, blindly restoring to
			// accepted here would be a guess about which of those
			// happened — ask the outline itself instead.
			status, rerr := s.reconcileBatch(ctx, now, projectID, batchID)
			if rerr != nil {
				return errors.Join(ErrUndoBatchNotFound, fmt.Errorf("reconcile batch %s: %w", batchID, rerr))
			}
			if status == StatusPending {
				// The outline fact says this batch really is gone —
				// reconcileBatch already finished the job and notified.
				return nil
			}
			// status is StatusAccepted (the outline fact says the batch
			// never actually left) or "" (reconcile had nothing to check
			// a fact against) — either way storyops has no window entry
			// left for this batch id, so there is nothing left to retry.
			return ErrUndoBatchNotFound
		}
		// A real failure, not "batch not found" — storyops' own window
		// entry is still there (RestoreOutline's own transaction rolled
		// back cleanly), so put every item back to exactly what it was
		// before step 1 and let the caller retry the same batch id.
		restoreErr := s.restoreItems(ctx, items)
		if restoreErr != nil {
			return errors.Join(err, fmt.Errorf("restore items after failed undo: %w", restoreErr))
		}
		return err
	}

	// Step 3: the outline actually moved — finish writing that down.
	if err := s.finalizeReverted(ctx, now, items); err != nil {
		// The outline really did change; only this package's own
		// bookkeeping failed. Nothing to compensate here — the items
		// stay at reverting with their original result+batchID (step 1
		// already committed that), exactly the state ReconcileReverting
		// resolves to pending from, on the next retry of this same batch
		// id (storyops will answer ErrUndoBatchNotFound above, since its
		// window entry is already consumed) or at the next engine start.
		return fmt.Errorf("finalize reverted items: %w", err)
	}

	s.fireNotify("proposals.changed", proposalsChangedPayload{ProjectID: projectID, Kinds: sortedKinds(map[string]bool{"proposals": true, "outline": true})})
	return nil
}

// restoreItems writes every item in items back to exactly the status,
// result and undo_batch_id it carried on entry to Undo — the compensation
// that runs when the "carry to reverting" transaction committed but
// storyops.UndoApply then failed (step 2), so the batch stays retryable
// instead of stranding the writer with items that have lost their batch
// id. Also reconcileBatch's own "the outline fact says this batch never
// actually left" path, for exactly the same reason.
func (s *Service) restoreItems(ctx context.Context, items []Item) error {
	tx, err := s.store.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for _, it := range items {
		if err := s.restoreItemFull(ctx, tx, it); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("restore item %s: %w", it.ID, err)
		}
	}
	return tx.Commit()
}

// finalizeReverted carries every item in items the rest of the way to
// pending — result cleared, undo_batch_id cleared — deleting each
// covered add_beat item's created beat first. Shared by Undo's own step 3
// (storyops.UndoApply just succeeded) and reconcileBatch's "the outline
// fact says this batch is actually gone" path, so the two can never
// finalize an item differently.
//
// Beat deletion runs before the transaction, not inside it, and is
// best-effort (errors ignored): the beat may already be gone — a retry
// after an earlier call got this far and committed the transaction below
// is impossible (reconcile/Undo would have found the item already
// pending, not reverting), but a retry after an earlier call deleted the
// beat and then failed the transaction is exactly what reverting is for,
// and must not treat a missing beat as fatal the second time around.
func (s *Service) finalizeReverted(ctx context.Context, now int64, items []Item) error {
	for _, it := range items {
		if it.Op.Type == "add_beat" && it.Result.CreatedID != "" {
			_ = s.beats.Delete(ctx, it.Result.CreatedID)
		}
	}

	tx, err := s.store.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for _, it := range items {
		if err := s.updateItemStatus(ctx, tx, it.ID, StatusPending, ItemResult{}, now); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("persist item %s status: %w", it.ID, err)
		}
		if err := s.updateItemUndoBatch(ctx, tx, it.ID, ""); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("persist item %s undo batch: %w", it.ID, err)
		}
	}
	return tx.Commit()
}
