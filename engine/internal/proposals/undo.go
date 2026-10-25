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

	// Carry every covered item to pending — committed before storyops ever
	// runs. The store's single connection (store.go's SetMaxOpenConns(1))
	// means a transaction here cannot also call into storyops: storyops'
	// own repo calls would block on that same connection and deadlock
	// (hit in Phase 3). So this has to be two separate steps, which is
	// exactly the gap the old code fell into — it called UndoApply first
	// and only afterward wrote status=pending, so a DB failure between
	// "outline restored" and "last item written" left the remaining items
	// stuck at accepted+batchID with no second chance (UndoApply itself
	// had already dropped the batch and answers ErrUndoBatchNotFound on
	// retry).
	//
	// Flipping the order closes that hole: once this transaction commits,
	// the outline hasn't moved yet, so if it fails outright nothing has
	// changed anywhere and the caller can simply retry Undo. The new risk
	// this shifts in is the opposite case — DB write ok, then
	// UndoApply fails — handled below by writing every item straight
	// back to its pre-undo state (status=accepted, same result,
	// undo_batch_id=batchID) so the batch is still exactly what
	// storyops.UndoApply expects next time.
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
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit pending items: %w", err)
	}

	if err := s.storyops.UndoApply(ctx, batchID, func() int64 { return now }); err != nil {
		// The outline never moved (or storyops already rolled its own
		// side back — RestoreOutline is itself all-or-nothing). Put every
		// item back to exactly what it was before the transaction above,
		// so a retry with the same batchID finds the batch still in
		// storyops' undo window and the items still tagged for it.
		//
		// KNOWN GAP (tracked as a follow-up, not fixed here): if this
		// compensating write ALSO fails, the item is left at pending with
		// result cleared and no undo_batch_id, while the outline node the
		// now-failed UndoApply never touched (its own transaction rolled
		// back) is still sitting there, orphaned — no proposal item
		// points at it any more. Accept() only skips an item whose status
		// is accepted (see its committed[it.ID] gate in accept.go), so a
		// writer who simply retries "accept" on that pending item gets a
		// SECOND scene/node created alongside the orphaned first one,
		// not a clean retry. Recovering automatically from a double DB
		// failure needs a state beyond plain pending (e.g. a terminal
		// "needs_review" status) and a migration to add it; out of scope
		// for this fix.
		restoreErr := s.restoreItems(ctx, items)
		if errors.Is(err, storyops.ErrUndoBatchNotFound) {
			err = ErrUndoBatchNotFound
		}
		if restoreErr != nil {
			return errors.Join(err, fmt.Errorf("restore items after failed undo: %w", restoreErr))
		}
		return err
	}

	kinds := map[string]bool{"proposals": true, "outline": true}
	for _, it := range items {
		if it.Op.Type == "add_beat" && it.Result.CreatedID != "" {
			// Best-effort: the beat may already be gone (e.g. a retry
			// after a partial failure) — Undo's job is to leave the
			// proposal item retryable, not to treat a missing beat as
			// fatal.
			_ = s.beats.Delete(ctx, it.Result.CreatedID)
		}
	}

	s.fireNotify("proposals.changed", proposalsChangedPayload{ProjectID: projectID, Kinds: sortedKinds(kinds)})
	return nil
}

// restoreItems writes every item in items back to exactly the status,
// result and undo_batch_id it carried on entry to Undo — the compensation
// that runs when the "carry to pending" transaction committed but
// storyops.UndoApply then failed, so the batch stays retryable instead of
// stranding the writer with items at pending but no batch left in
// storyops' undo window to retry against.
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
