package storyops

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/google/uuid"
)

// ErrUndoBatchNotFound means the undo window has passed: the batch was already
// undone, or fell out of the in-memory list.
var ErrUndoBatchNotFound = errors.New("storyops: undo batch not found")

// How many undo batches are kept in memory. The writer only ever undoes the
// change they just watched land, so a short list is enough.
const maxUndoBatches = 8

// OutlineChangeCounts summarizes what a batch would do to the outline tree.
type OutlineChangeCounts struct {
	Created int `json:"created"`
	Renamed int `json:"renamed"`
	Deleted int `json:"deleted"`
	Moved   int `json:"moved"`
	// Other counts ops in the same batch that do not touch the tree (beats,
	// storylines, world-building, memories).
	Other int `json:"other"`
}

// Structural reports how many ops rearrange the outline tree.
func (c OutlineChangeCounts) Structural() int {
	return c.Created + c.Renamed + c.Deleted + c.Moved
}

// OutlineOpAction classifies an op type by what it does to the outline tree:
// "create", "rename", "delete", "move", or "" for ops that leave it alone.
func OutlineOpAction(opType string) string {
	switch opType {
	case "create_outline_node", "create_scene":
		return "create"
	case "rename_outline_node":
		return "rename"
	case "delete_outline_node":
		return "delete"
	case "move_outline_node":
		return "move"
	default:
		return ""
	}
}

// CountOutlineChanges tallies a proposal by what it does to the tree.
func CountOutlineChanges(p Proposal) OutlineChangeCounts {
	var c OutlineChangeCounts
	for _, op := range p.Ops {
		switch OutlineOpAction(op.Type) {
		case "create":
			c.Created++
		case "rename":
			c.Renamed++
		case "delete":
			c.Deleted++
		case "move":
			c.Moved++
		default:
			c.Other++
		}
	}
	return c
}

// undoBatch is the outline as it stood before an applied change.
type undoBatch struct {
	projectID string
	nodes     []node.Node
}

// undoState holds the outline snapshots taken before structural applies, kept
// so the writer can undo the change that just landed.
type undoState struct {
	mu      sync.Mutex
	batches map[string]undoBatch
	order   []string
}

// rememberUndoBatch keeps the pre-change outline so the writer can put it back
// with one action. Batches live in memory only: undo is for the change you just
// watched land, not for history.
func (s *Service) rememberUndoBatch(projectID string, before []node.Node) string {
	if len(before) == 0 {
		return ""
	}
	id := uuid.NewString()
	s.undo.mu.Lock()
	defer s.undo.mu.Unlock()
	if s.undo.batches == nil {
		s.undo.batches = map[string]undoBatch{}
	}
	s.undo.batches[id] = undoBatch{projectID: projectID, nodes: before}
	s.undo.order = append(s.undo.order, id)
	for len(s.undo.order) > maxUndoBatches {
		delete(s.undo.batches, s.undo.order[0])
		s.undo.order = s.undo.order[1:]
	}
	return id
}

func (s *Service) takeUndoBatch(id string) (undoBatch, bool) {
	s.undo.mu.Lock()
	defer s.undo.mu.Unlock()
	batch, ok := s.undo.batches[id]
	if !ok {
		return undoBatch{}, false
	}
	delete(s.undo.batches, id)
	for i, existing := range s.undo.order {
		if existing == id {
			s.undo.order = append(s.undo.order[:i], s.undo.order[i+1:]...)
			break
		}
	}
	return batch, true
}

// UndoApply puts the outline back the way it was before the applied batch.
func (s *Service) UndoApply(ctx context.Context, batchID string, now func() int64) error {
	batch, ok := s.takeUndoBatch(strings.TrimSpace(batchID))
	if !ok {
		return ErrUndoBatchNotFound
	}
	if s.nodes == nil {
		return ErrUndoBatchNotFound
	}
	return s.nodes.RestoreOutline(ctx, batch.projectID, batch.nodes, now())
}

// snapshotOutline captures the tree so a failed batch can be rolled back and a
// finished one can be undone.
func (s *Service) snapshotOutline(ctx context.Context, projectID string) []node.Node {
	if s.nodes == nil {
		return nil
	}
	before, err := s.nodes.ListByProject(ctx, projectID)
	if err != nil {
		return nil
	}
	return before
}
