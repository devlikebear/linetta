package companion

import (
	"context"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/storycontext"
)

// referenceContextLimit matches gatherContext's own cap on injected references.
const referenceContextLimit = 40

// The companion is currently the only place that gathers Fact Book cards,
// remembered facts, and writer-attached references. storycontext grew optional
// source interfaces for those sections in Phase 1 of the MCP-first pivot (#47);
// these adapters connect the two so the MCP story brief carries everything the
// companion's own prompt does.
//
// When this package is removed (pivot Phase 6), these three methods move to
// whatever owns the underlying repos — the fact and reference repos are
// already independent, and memory recall follows the remember op into
// storyops.

var (
	_ storycontext.FactSource      = (*Service)(nil)
	_ storycontext.MemorySource    = (*Service)(nil)
	_ storycontext.ReferenceSource = (*Service)(nil)
)

// ContextFacts returns Fact Book cards for the brief, preferring cards
// attached to the current scene, exactly as gatherContext does.
func (s *Service) ContextFacts(ctx context.Context, projectID, nodeID string) ([]storycontext.FactBrief, error) {
	if s.facts == nil {
		return nil, nil
	}
	filter := fact.ListFilter{ProjectID: projectID, Limit: factContextLimit}
	if strings.TrimSpace(nodeID) != "" {
		filter.NodeID = &nodeID
	}
	cards, err := s.facts.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	out := make([]storycontext.FactBrief, 0, len(cards))
	for _, c := range cards {
		brief := storycontext.FactBrief{
			ID:       c.ID,
			Status:   c.Status,
			Claim:    c.Claim,
			Category: c.Category,
			Result:   c.Result,
		}
		for _, src := range c.Sources {
			if strings.TrimSpace(src.URL) == "" {
				continue
			}
			brief.Sources = append(brief.Sources, storycontext.FactSourceBrief{
				Title: src.Title,
				URL:   src.URL,
			})
		}
		out = append(out, brief)
	}
	return out, nil
}

// ContextMemories returns recent remembered facts for the brief.
func (s *Service) ContextMemories(projectID string) []string {
	return s.Recall(projectID, "", recallLimit)
}

// ContextReferences returns the writer-attached material for the brief,
// skipping disabled entries and using the same prompt text (summary vs full
// content) the companion sends.
func (s *Service) ContextReferences(ctx context.Context, projectID, nodeID string) ([]storycontext.ReferenceBrief, error) {
	if s.references == nil {
		return nil, nil
	}
	refs, err := s.ListReferences(ctx, ReferenceQuery{
		ProjectID: projectID,
		NodeID:    nodeID,
		Limit:     referenceContextLimit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]storycontext.ReferenceBrief, 0, len(refs))
	for _, r := range refs {
		if r.Status == ReferenceStatusDisabled {
			continue
		}
		text := strings.TrimSpace(referencePromptText(r))
		if text == "" {
			continue
		}
		out = append(out, storycontext.ReferenceBrief{
			Title:   strings.TrimSpace(r.Title),
			Purpose: strings.TrimSpace(r.Purpose),
			Body:    text,
		})
	}
	return out, nil
}
