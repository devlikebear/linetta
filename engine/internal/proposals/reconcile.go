package proposals

import (
	"context"
	"errors"
	"fmt"

	"github.com/devlikebear/linetta/engine/internal/node"
)

// reconcileBatch resolves one batchID's reverting items from the outline's
// own facts, never from a guess: if ANY of the batch's structural items
// (create_outline_node/create_scene) still has its created node in the
// outline (node.Repo.Get succeeds), storyops.UndoApply for this batch
// never actually took effect — whatever failed was either step 2 itself
// or this package's own compensating write afterward — so every item the
// batch covers goes back to accepted+original result+batchID, exactly
// what restoreItems already does for Undo's own failure path. If every
// structural item's node is gone (node.ErrNotFound), the outline really
// was put back; only this package's bookkeeping afterward failed, so
// every item is finalized to pending (finalizeReverted — same helper
// Undo's own success path uses, so the two can never disagree).
//
// Returns the status every item in the batch now carries — StatusAccepted
// or StatusPending — or "" when there was nothing reverting to reconcile,
// or when the batch has no structural item to check a fact against (an
// add_beat-only batch should not exist in practice, but reconcile must
// not guess either way when it has nothing to ask the outline about). A
// transient failure to even read the fact (anything but node.ErrNotFound)
// aborts with an error instead of silently leaving the batch reverting
// without saying why — a caller that got "" with a nil error knows that
// was a deliberate "nothing to check", not a failure.
func (s *Service) reconcileBatch(ctx context.Context, now int64, projectID, batchID string) (string, error) {
	items, err := s.listItemsByUndoBatch(ctx, s.store.DB(), projectID, batchID)
	if err != nil {
		return "", err
	}

	var reverting []Item
	for _, it := range items {
		if it.Status == StatusReverting {
			reverting = append(reverting, it)
		}
	}
	if len(reverting) == 0 {
		return "", nil
	}

	sawStructural := false
	anyNodeExists := false
	for _, it := range reverting {
		if it.Op.Type != "create_outline_node" && it.Op.Type != "create_scene" {
			continue
		}
		if it.Result.CreatedID == "" {
			continue
		}
		sawStructural = true
		if _, gerr := s.nodes.Get(ctx, it.Result.CreatedID); gerr == nil {
			anyNodeExists = true
		} else if !errors.Is(gerr, node.ErrNotFound) {
			return "", fmt.Errorf("check node %s: %w", it.Result.CreatedID, gerr)
		}
	}
	if !sawStructural {
		return "", nil
	}

	if anyNodeExists {
		// restoreItems writes back exactly the Status each Item struct
		// carries — fine for Undo's own use (its items are read BEFORE
		// step 1 moves them to reverting, so Status is still the
		// original "accepted"), but reverting here was just read
		// straight from the DB, so Status reads "reverting". Set it
		// explicitly, or this would write "reverting" right back over
		// itself and report accepted without actually changing anything.
		for i := range reverting {
			reverting[i].Status = StatusAccepted
		}
		if err := s.restoreItems(ctx, reverting); err != nil {
			return "", fmt.Errorf("restore to accepted: %w", err)
		}
		s.fireNotify("proposals.changed", proposalsChangedPayload{ProjectID: projectID, Kinds: sortedKinds(map[string]bool{"proposals": true, "outline": true})})
		return StatusAccepted, nil
	}

	if err := s.finalizeReverted(ctx, now, reverting); err != nil {
		return "", fmt.Errorf("finalize to pending: %w", err)
	}
	s.fireNotify("proposals.changed", proposalsChangedPayload{ProjectID: projectID, Kinds: []string{"proposals"}})
	return StatusPending, nil
}

// ReconcileReverting resolves every item left at StatusReverting across
// every project, grouped by (project, undo_batch_id) so a batch's items
// are always reconciled together — never one accepted while a sibling
// stays reverting. Meant to run once at engine startup (register, before
// the RPC server takes requests): storyops' in-memory undo window is gone
// on restart (storyops.maxUndoBatches's own doc comment), so anything a
// crash left reverting can never resolve through Undo's own retry path —
// a later proposals.undo call for that batch id would get
// storyops.ErrUndoBatchNotFound immediately and reconcile exactly as it
// would mid-session, but there is no reason to make the writer click undo
// again just to trigger that.
//
// Best-effort across batches: one batch's reconcile failing (e.g. a
// transient DB read) does not stop the others from being resolved. Errors
// are joined and returned for the caller to log; a caller that cannot
// afford to block startup on this should still call it synchronously —
// its own worst case is "a reverting item is still reverting", not a
// destructive write, since reconcileBatch only writes once it has a DB
// fact to write from.
func (s *Service) ReconcileReverting(ctx context.Context, now int64) error {
	items, err := s.listItemsByStatus(ctx, s.store.DB(), StatusReverting)
	if err != nil {
		return fmt.Errorf("list reverting items: %w", err)
	}

	type batchKey struct {
		projectID string
		batchID   string
	}
	seen := map[batchKey]bool{}
	var errs []error
	for _, it := range items {
		if it.UndoBatchID == "" {
			// Should not happen — Undo always sets reverting alongside a
			// batch id — but reconcile has nothing to group this item
			// under, so skip it rather than guess.
			continue
		}
		k := batchKey{it.ProjectID, it.UndoBatchID}
		if seen[k] {
			continue
		}
		seen[k] = true
		if _, err := s.reconcileBatch(ctx, now, it.ProjectID, it.UndoBatchID); err != nil {
			errs = append(errs, fmt.Errorf("reconcile batch %s (project %s): %w", it.UndoBatchID, it.ProjectID, err))
		}
	}
	return errors.Join(errs...)
}
