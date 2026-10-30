//go:build !mobile

package mcphost

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/proposals"
)

// ---------- linetta_propose_story_ops ----------
//
// story-assist Phase 3. An agent's create_entity, create_relationship,
// create_fact_card, create_thread/update_thread and add_beat/update_beat
// candidates land here, not in Story World: nothing is applied until the
// writer accepts an item through the app's proposals.accept RPC — no MCP
// tool accepts one. This is the path for anything pulled from the
// manuscript, or more than a few items at once; a directly requested small
// change still goes through linetta_apply_story_ops, which this tool does
// not replace.

// proposeStoryOpsInput is proposals.SubmitInput under a locally-defined
// name: a method cannot be added to a type from another package, even via
// an alias, and scope() is what record() uses to log what a call touched.
type proposeStoryOpsInput proposals.SubmitInput

func (in proposeStoryOpsInput) scope() (string, string) { return in.ProjectID, "" }

// proposeStoryOpsStatus is the only status a submission can carry back:
// nothing an agent does moves a proposal further than waiting for the
// writer, the same convention continuity.ReportStatusWaitingForWriter sets.
const proposeStoryOpsStatus = "waiting_for_writer"

type proposeStoryOpsOutput struct {
	ProposalID         string           `json:"proposal_id"`
	Items              []proposals.Item `json:"items"`
	PossibleDuplicates int              `json:"possible_duplicates"`
	Status             string           `json:"status"`
	// Progress is set only for purpose=import_extraction: how far a
	// manuscript sweep has gotten and which episode to read next, so the
	// agent can tell the writer "6/40, continuing from 7화" without a
	// separate RPC round trip it has no tool for anyway.
	Progress *proposeStoryOpsProgress `json:"progress,omitempty"`
}

type proposeStoryOpsProgress struct {
	Covered          int    `json:"covered"`
	Total            int    `json:"total"`
	NextEpisodeLabel string `json:"next_episode_label,omitempty"`
}

func (d ToolDeps) registerProposalWriteTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_propose_story_ops",
		Description: "Suggest world elements (characters, places, items, concepts, relationships, fact " +
			"cards, storylines, beats) for the writer to accept one by one. Nothing is applied. Use it " +
			"for anything drawn from the manuscript or for more than a few items; reuse existing entity " +
			"ids from linetta_list_characters instead of proposing duplicates. Cite evidence from " +
			"linetta_read_scene numbered=true where you can.",
	}, record(d, "linetta_propose_story_ops", d.proposeStoryOps))
}

func (d ToolDeps) proposeStoryOps(ctx context.Context, _ *mcp.CallToolRequest, in proposeStoryOpsInput) (*mcp.CallToolResult, proposeStoryOpsOutput, error) {
	if d.Proposals == nil {
		return toolErr("proposals are not available in this build"), proposeStoryOpsOutput{}, nil
	}
	res, problems, err := d.Proposals.Submit(ctx, d.now(), proposals.SubmitInput(in), d.sourceOrExternal())
	if err != nil {
		return nil, proposeStoryOpsOutput{}, err
	}
	if len(problems) > 0 {
		return toolErr("%s", proposalProblemsText(problems)), proposeStoryOpsOutput{}, nil
	}

	out := proposeStoryOpsOutput{
		ProposalID:         res.ProposalID,
		Items:              res.Items,
		PossibleDuplicates: res.PossibleDuplicates,
		Status:             proposeStoryOpsStatus,
	}
	if strings.TrimSpace(in.Purpose) == proposals.PurposeImportExtraction {
		if progress, perr := d.Proposals.ExtractionProgress(ctx, in.ProjectID); perr == nil {
			label := ""
			if progress.NextEpisode != nil {
				label = progress.NextEpisode.Label
			}
			out.Progress = &proposeStoryOpsProgress{
				Covered: progress.CoveredEpisodes, Total: progress.TotalEpisodes, NextEpisodeLabel: label,
			}
		}
	}

	d.notifyChanged(in.ProjectID, "linetta_propose_story_ops", nil, "")
	return nil, out, nil
}

// proposalProblemsText joins every proposals.Problem into one refusal,
// numbered so a model fixing several at once can tell them apart — the
// same convention continuity's problemsText follows for its own Problem
// type (an identical shape in a different package, so no shared helper).
func proposalProblemsText(problems []proposals.Problem) string {
	if len(problems) == 1 {
		return problems[0].Message
	}
	lines := make([]string, len(problems))
	for i, p := range problems {
		lines[i] = p.Message
	}
	return strings.Join(lines, "\n")
}
