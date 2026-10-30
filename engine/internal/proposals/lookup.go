package proposals

import (
	"context"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/style"
)

// refLookup answers whether a ref Submit is checking actually belongs to
// the project being validated — continuity.RefLookup's convention, so a
// proposal cannot be submitted against another work's ids.
type refLookup struct {
	ctx       context.Context
	projectID string
	s         *Service
}

func (s *Service) lookupFor(ctx context.Context, projectID string) refLookup {
	return refLookup{ctx: ctx, projectID: projectID, s: s}
}

func (l refLookup) HasEntity(id string) bool {
	if l.s.entities == nil || id == "" {
		return false
	}
	e, err := l.s.entities.Get(l.ctx, id)
	return err == nil && e.ProjectID == l.projectID
}

func (l refLookup) HasThread(id string) bool {
	if l.s.threads == nil || id == "" {
		return false
	}
	th, err := l.s.threads.Get(l.ctx, id)
	return err == nil && th.ProjectID == l.projectID
}

func (l refLookup) HasBeat(id string) bool {
	if l.s.beats == nil || l.s.threads == nil || id == "" {
		return false
	}
	b, err := l.s.beats.Get(l.ctx, id)
	if err != nil {
		return false
	}
	th, err := l.s.threads.Get(l.ctx, b.ThreadID)
	return err == nil && th.ProjectID == l.projectID
}

// HasNode reports whether nodeID is ANY node (container or leaf) of this
// project — create_scene's after_node_*/create_outline_node's
// parent_node_*/after_node_* (story-assist Phase 4, purpose=forecast) can
// legitimately point at a container, unlike IsLeafNode's scene-only check
// below.
func (l refLookup) HasNode(nodeID string) bool {
	if l.s.nodes == nil || nodeID == "" {
		return false
	}
	n, err := l.s.nodes.Get(l.ctx, nodeID)
	return err == nil && n.ProjectID == l.projectID
}

// IsLeafNode reports whether nodeID is a leaf scene of this project —
// from_node_ids, add_beat/create_fact_card's optional node binding, and
// evidence's node_id all require this.
func (l refLookup) IsLeafNode(nodeID string) bool {
	if l.s.nodes == nil || nodeID == "" {
		return false
	}
	n, err := l.s.nodes.Get(l.ctx, nodeID)
	return err == nil && n.ProjectID == l.projectID && n.Kind == node.KindLeaf
}

// SceneParagraphs resolves a leaf scene's paragraphs for evidence
// verification, the same split style.Paragraphs gives continuity.
func (l refLookup) SceneParagraphs(nodeID string) ([]string, bool) {
	if l.s.nodes == nil || nodeID == "" {
		return nil, false
	}
	n, err := l.s.nodes.Get(l.ctx, nodeID)
	if err != nil || n.ProjectID != l.projectID || n.Kind != node.KindLeaf {
		return nil, false
	}
	if n.ContentDoc == nil {
		return []string{}, true
	}
	return style.Paragraphs(*n.ContentDoc), true
}
