package companion

import (
	"context"
	"errors"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/google/uuid"
)

// ErrUndoBatchNotFound means the undo window has passed: the batch was already
// undone, or fell out of the in-memory list.
var ErrUndoBatchNotFound = errors.New("companion: undo batch not found")

// A batch that rearranges this many outline nodes is a structural change to the
// work, not an edit: the writer sees it before it lands. Var so tests can move
// the line.
var largeOutlineChangeThreshold = 6

// How many undo batches are kept in memory. The writer only ever undoes the
// change they just watched land, so a short list is enough.
const maxUndoBatches = 8

// previewTreeLimit caps how many rows a preview carries; a 200-node rewrite is
// already far past the point where more rows help the writer decide.
const previewTreeLimit = 200

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

// OutlinePreviewNode is one row of the preview tree.
type OutlinePreviewNode struct {
	Ref    string `json:"ref,omitempty"`
	NodeID string `json:"node_id,omitempty"`
	Label  string `json:"label,omitempty"`
	Title  string `json:"title,omitempty"`
	Kind   string `json:"kind,omitempty"`
	Depth  int    `json:"depth"`
	Action string `json:"action"`
}

// OutlineChangePreview is what the writer approves or discards.
type OutlineChangePreview struct {
	Summary   string               `json:"summary,omitempty"`
	Counts    OutlineChangeCounts  `json:"counts"`
	Tree      []OutlinePreviewNode `json:"tree,omitempty"`
	Truncated int                  `json:"truncated,omitempty"`
	Ops       []Op                 `json:"ops"`
}

func outlineOpAction(opType string) string {
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

// countOutlineChanges tallies a proposal by what it does to the tree.
func countOutlineChanges(p Proposal) OutlineChangeCounts {
	var c OutlineChangeCounts
	for _, op := range p.Ops {
		switch outlineOpAction(op.Type) {
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

// needsOutlineApproval reports whether a batch reshapes enough of the outline
// that the writer should see it first.
func needsOutlineApproval(p Proposal) bool {
	return countOutlineChanges(p).Structural() >= largeOutlineChangeThreshold
}

// buildOutlinePreview renders the batch as an indented list of what it would
// create, rename, delete, or move. Labels for existing nodes are looked up
// best-effort so a delete row reads as a chapter name, not an id.
func (s *Service) buildOutlinePreview(ctx context.Context, projectID string, p Proposal) OutlineChangePreview {
	preview := OutlineChangePreview{
		Summary: strings.TrimSpace(p.Summary),
		Counts:  countOutlineChanges(p),
		Ops:     p.Ops,
	}
	depthByRef := map[string]int{}
	for _, op := range p.Ops {
		action := outlineOpAction(op.Type)
		if action == "" {
			continue
		}
		row := OutlinePreviewNode{
			Ref:    strings.TrimSpace(op.Ref),
			NodeID: strings.TrimSpace(op.NodeID),
			Label:  strings.TrimSpace(op.Label),
			Title:  strings.TrimSpace(op.Title),
			Kind:   strings.TrimSpace(op.Kind),
			Action: action,
		}
		if action == "create" {
			row.Depth = createDepth(op, depthByRef)
			if row.Ref != "" {
				depthByRef[row.Ref] = row.Depth
			}
			if op.Type == "create_scene" && row.Kind == "" {
				row.Kind = "leaf"
			}
		}
		if row.Label == "" && row.NodeID != "" {
			row.Label = s.outlineNodeLabel(ctx, projectID, row.NodeID)
		}
		if len(preview.Tree) >= previewTreeLimit {
			preview.Truncated++
			continue
		}
		preview.Tree = append(preview.Tree, row)
	}
	return preview
}

// createDepth places a new node under the node it is being attached to. Nodes
// hung off an existing parent start one level in; roots start at zero.
func createDepth(op Op, depthByRef map[string]int) int {
	if parentRef := strings.TrimSpace(op.ParentNodeRef); parentRef != "" {
		if depth, ok := depthByRef[parentRef]; ok {
			return depth + 1
		}
		return 1
	}
	if strings.TrimSpace(op.ParentNodeID) != "" {
		return 1
	}
	return 0
}

func (s *Service) outlineNodeLabel(ctx context.Context, projectID, nodeID string) string {
	if s.nodes == nil || nodeID == "" {
		return ""
	}
	n, err := s.nodes.Get(ctx, nodeID)
	if err != nil || n.ProjectID != projectID {
		return ""
	}
	return n.Label
}

// undoBatch is the outline as it stood before an applied change.
type undoBatch struct {
	projectID string
	nodes     []node.Node
}

// rememberUndoBatch keeps the pre-change outline so the writer can put it back
// with one action. Batches live in memory only: undo is for the change you just
// watched land, not for history.
func (s *Service) rememberUndoBatch(projectID string, before []node.Node) string {
	if len(before) == 0 {
		return ""
	}
	id := uuid.NewString()
	s.undoMu.Lock()
	defer s.undoMu.Unlock()
	if s.undoBatches == nil {
		s.undoBatches = map[string]undoBatch{}
	}
	s.undoBatches[id] = undoBatch{projectID: projectID, nodes: before}
	s.undoOrder = append(s.undoOrder, id)
	for len(s.undoOrder) > maxUndoBatches {
		delete(s.undoBatches, s.undoOrder[0])
		s.undoOrder = s.undoOrder[1:]
	}
	return id
}

func (s *Service) takeUndoBatch(id string) (undoBatch, bool) {
	s.undoMu.Lock()
	defer s.undoMu.Unlock()
	batch, ok := s.undoBatches[id]
	if !ok {
		return undoBatch{}, false
	}
	delete(s.undoBatches, id)
	for i, existing := range s.undoOrder {
		if existing == id {
			s.undoOrder = append(s.undoOrder[:i], s.undoOrder[i+1:]...)
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
