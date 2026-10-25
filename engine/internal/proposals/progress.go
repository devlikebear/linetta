package proposals

import (
	"context"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/episode"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/storycontext"
	"github.com/devlikebear/linetta/engine/internal/storyops"
)

// Progress is where a purpose=import_extraction sweep stands: which
// episodes already have at least one pending/accepted/rejected/failed item
// drawn from one of their scenes, and which episode to read next.
type Progress struct {
	TotalEpisodes   int              `json:"total_episodes"`
	CoveredEpisodes int              `json:"covered_episodes"`
	NextEpisode     *episode.Episode `json:"next_episode,omitempty"`
	PendingItems    int              `json:"pending_items"`
}

// Estimate is what Library's "Story World 채우기" dialog shows before the
// writer starts an extraction sweep: the whole manuscript's size and a
// rough input-token budget. It is deliberately an estimate, not a cost
// preview — see the package's Non-goals in
// docs/plans/story-assist/phase-3-proposals-import.md.
type Estimate struct {
	Episodes    int `json:"episodes"`
	Chars       int `json:"chars"`
	InputTokens int `json:"input_tokens"`
}

// ExtractionProgress reports which episodes a purpose=import_extraction
// sweep has already drawn at least one item from (any status — a
// rejected/failed item still means the agent read that episode), and which
// to read next. "Covered" means an item's from_node_ids, OR one of its
// cited evidence scenes, overlaps the episode's own LeafIDs, computed from
// episode.List so this agrees with every other episode-aware feature in
// the codebase.
//
// Evidence counts too, not just from_node_ids: Submit does not require an
// item to set from_node_ids (it is the progress mechanism's own hint, not
// something storyops or Accept needs), so an agent that cited a scene as
// evidence without also filling from_node_ids must still mark that
// episode read — otherwise NextEpisode keeps naming the same episode
// forever and the agent burns its tool budget re-reading it.
//
// This still cannot see an episode with nothing worth proposing AT ALL
// (zero items, so zero evidence too) — Submit refuses an empty batch, so
// there is no signal an agent can send for "I read it, there was
// nothing." That gap is a known limitation, not something this function
// can close on its own.
func (s *Service) ExtractionProgress(ctx context.Context, projectID string) (Progress, error) {
	episodes, err := s.episodesFor(ctx, projectID)
	if err != nil {
		return Progress{}, err
	}
	items, err := s.listItemsForPurpose(ctx, projectID, PurposeImportExtraction)
	if err != nil {
		return Progress{}, err
	}
	coveredLeaf := map[string]bool{}
	for _, it := range items {
		for _, leaf := range it.FromNodeIDs {
			coveredLeaf[leaf] = true
		}
		for _, ev := range it.Evidence {
			if ev.NodeID != "" {
				coveredLeaf[ev.NodeID] = true
			}
		}
	}

	covered := 0
	var next *episode.Episode
	for i := range episodes {
		ep := episodes[i]
		hasCoverage := false
		for _, leaf := range ep.LeafIDs {
			if coveredLeaf[leaf] {
				hasCoverage = true
				break
			}
		}
		if hasCoverage {
			covered++
		} else if next == nil {
			ep := ep
			next = &ep
		}
	}

	pending, err := s.countPending(ctx, projectID)
	if err != nil {
		return Progress{}, err
	}
	return Progress{
		TotalEpisodes: len(episodes), CoveredEpisodes: covered, NextEpisode: next, PendingItems: pending,
	}, nil
}

// ExtractionEstimate sizes the WHOLE manuscript — not just what remains —
// since the dialog that shows this appears right after an import, before
// any extraction has happened at all. InputTokens is
// storycontext.EstimateTokens's rough per-character estimate times 1.5, for
// the brief and tool round trips an extraction turn costs beyond the raw
// text (both roadmap.md §"왜 새 경로가 필요한가" and the work order call this an
// estimate, shown to the writer as one, not a billed cost).
func (s *Service) ExtractionEstimate(ctx context.Context, projectID string) (Estimate, error) {
	episodes, err := s.episodesFor(ctx, projectID)
	if err != nil {
		return Estimate{}, err
	}
	nodes, err := s.nodes.ListByProject(ctx, projectID)
	if err != nil {
		return Estimate{}, err
	}
	var sb strings.Builder
	for _, n := range nodes {
		if n.Kind != node.KindLeaf || n.ContentDoc == nil {
			continue
		}
		sb.WriteString(storyops.PlainTextFromDoc(n.ContentDoc))
		sb.WriteString("\n")
	}
	text := sb.String()
	chars := storycontext.EstimateChars(text)
	tokens := int(float64(storycontext.EstimateTokens(text)) * 1.5)
	return Estimate{Episodes: len(episodes), Chars: chars, InputTokens: tokens}, nil
}

// listItemsForPurpose returns every item (any status) for projectID whose
// proposal has the given purpose.
func (s *Service) listItemsForPurpose(ctx context.Context, projectID, purpose string) ([]Item, error) {
	rows, err := s.store.DB().QueryContext(ctx,
		`SELECT i.id, i.proposal_id, i.project_id, i.ordinal, i.op_json, i.depends_on_json, i.evidence_json,
		        i.from_node_ids_json, i.duplicates_json, i.status, i.result_json, i.status_at
		   FROM story_proposal_items i
		   JOIN story_proposals p ON p.id = i.proposal_id
		  WHERE i.project_id = ? AND p.purpose = ?`,
		projectID, purpose)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}
