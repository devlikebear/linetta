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

	if err := s.storyops.UndoApply(ctx, batchID, func() int64 { return now }); err != nil {
		if errors.Is(err, storyops.ErrUndoBatchNotFound) {
			return ErrUndoBatchNotFound
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
		if err := s.updateItemStatus(ctx, s.store.DB(), it.ID, StatusPending, ItemResult{}, now); err != nil {
			return fmt.Errorf("persist item %s status: %w", it.ID, err)
		}
		if err := s.updateItemUndoBatch(ctx, s.store.DB(), it.ID, ""); err != nil {
			return fmt.Errorf("persist item %s undo batch: %w", it.ID, err)
		}
	}

	s.fireNotify("proposals.changed", proposalsChangedPayload{ProjectID: projectID, Kinds: sortedKinds(kinds)})
	return nil
}
