package companion

import (
	"context"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/storyops"
)

// ErrUndoBatchNotFound re-exports the storyops sentinel so existing handler
// errors.Is checks keep matching.
var ErrUndoBatchNotFound = storyops.ErrUndoBatchNotFound

// OutlineChangeCounts moved to internal/storyops with the applier.
type OutlineChangeCounts = storyops.OutlineChangeCounts

// A batch that rearranges this many outline nodes is a structural change to the
// work, not an edit: the writer sees it before it lands. Var so tests can move
// the line.
var largeOutlineChangeThreshold = 6

// previewTreeLimit caps how many rows a preview carries; a 200-node rewrite is
// already far past the point where more rows help the writer decide.
const previewTreeLimit = 200

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

// needsOutlineApproval reports whether a batch reshapes enough of the outline
// that the writer should see it first.
func needsOutlineApproval(p Proposal) bool {
	return storyops.CountOutlineChanges(p).Structural() >= largeOutlineChangeThreshold
}

// buildOutlinePreview renders the batch as an indented list of what it would
// create, rename, delete, or move. Labels for existing nodes are looked up
// best-effort so a delete row reads as a chapter name, not an id.
func (s *Service) buildOutlinePreview(ctx context.Context, projectID string, p Proposal) OutlineChangePreview {
	preview := OutlineChangePreview{
		Summary: strings.TrimSpace(p.Summary),
		Counts:  storyops.CountOutlineChanges(p),
		Ops:     p.Ops,
	}
	depthByRef := map[string]int{}
	for _, op := range p.Ops {
		action := storyops.OutlineOpAction(op.Type)
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

// UndoApply puts the outline back the way it was before the applied batch.
func (s *Service) UndoApply(ctx context.Context, batchID string, now func() int64) error {
	return s.story.UndoApply(ctx, batchID, now)
}
