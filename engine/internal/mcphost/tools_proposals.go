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
// candidates land here, not in Story World: nothing is applied until an item
// is accepted, by the writer in the inbox or by an agent the writer told to
// through linetta_accept_proposals below. This is the path for anything
// pulled from the manuscript, or more than a few items at once; a directly
// requested small change still goes through linetta_apply_story_ops, which
// this tool does not replace.

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

// ---------- linetta_list_proposals ----------
//
// The ids the two decision tools need. Open items only: a settled item has
// nothing left to decide, and a sweep's inbox can run to hundreds of rows.

const (
	defaultProposalListLimit = 50
	maxProposalListLimit     = 200
)

type listProposalsInput struct {
	ProjectID  string `json:"project_id"`
	ProposalID string `json:"proposal_id,omitempty" jsonschema:"only this batch's items"`
	Limit      int    `json:"limit,omitempty" jsonschema:"at most 200; default 50"`
	Offset     int    `json:"offset,omitempty"`
}

func (in listProposalsInput) scope() (string, string) { return in.ProjectID, "" }

type proposalBatchOut struct {
	ID      string `json:"id"`
	Summary string `json:"summary,omitempty"`
	Purpose string `json:"purpose"`
	Open    int    `json:"open"`
}

type proposalItemOut struct {
	ID                string                `json:"id"`
	ProposalID        string                `json:"proposal_id"`
	Status            string                `json:"status"`
	Summary           string                `json:"summary"`
	DependsOn         []string              `json:"depends_on,omitempty"`
	BlockedByRejected bool                  `json:"blocked_by_rejected,omitempty"`
	Duplicates        []proposals.Duplicate `json:"duplicates,omitempty"`
	Error             string                `json:"error,omitempty"`
}

type listProposalsOutput struct {
	Batches []proposalBatchOut `json:"batches"`
	Items   []proposalItemOut  `json:"items"`
	Total   int                `json:"total" jsonschema:"open items in all, including any past this page"`
}

// ---------- linetta_accept_proposals / linetta_reject_proposals ----------
//
// The writer can hand the decision over ("다 승인해"), so an agent can take it,
// through the same Accept and Reject the inbox calls. They are no reach beyond
// what linetta_apply_story_ops already gives a client in full mode; what keeps
// the decision the writer's is that the tool descriptions and the system
// prompt require the writer's own word for it.

type decideProposalsInput struct {
	ProjectID   string   `json:"project_id"`
	ItemIDs     []string `json:"item_ids,omitempty" jsonschema:"ids from linetta_list_proposals"`
	ProposalIDs []string `json:"proposal_ids,omitempty" jsonschema:"every open item of these batches"`
}

func (in decideProposalsInput) scope() (string, string) { return in.ProjectID, "" }

type acceptedItemOut struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	CreatedID string `json:"created_id,omitempty"`
	// LinkedExisting: the item's name was already in the work, so it became
	// that record, and CreatedID is not new.
	LinkedExisting bool   `json:"linked_existing,omitempty"`
	Error          string `json:"error,omitempty"`
}

type acceptProposalsOutput struct {
	Accepted     int               `json:"accepted"`
	Failed       int               `json:"failed"`
	Items        []acceptedItemOut `json:"items"`
	UndoBatchIDs []string          `json:"undo_batch_ids,omitempty" jsonschema:"revert an outline change with linetta_undo_last_change"`
}

type rejectProposalsOutput struct {
	Rejected int `json:"rejected"`
	Skipped  int `json:"skipped" jsonschema:"named items that were not open, so nothing changed for them"`
}

func (d ToolDeps) registerProposalReadTool(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_list_proposals",
		Description: "List the suggestions inbox's undecided items with their ids: pending ones and failed " +
			"ones, which can be retried. Each failed item says why.",
	}, record(d, "linetta_list_proposals", d.listProposals))
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

	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_accept_proposals",
		Description: "Accept inbox items, applying them to Story World. Only when the writer has told you to in " +
			"this conversation; never on your own judgement. Include what an item depends on. A new element " +
			"whose exact name and kind already exist links to that record, keeping what it says. One item " +
			"failing does not stop the rest, and failed items can be retried.",
	}, record(d, "linetta_accept_proposals", d.acceptProposals))

	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_reject_proposals",
		Description: "Reject inbox items so they leave the inbox, failed ones included. Only when the writer has " +
			"told you to in this conversation. Items depending on a rejected one stay, marked blocked.",
	}, record(d, "linetta_reject_proposals", d.rejectProposals))
}

func (d ToolDeps) listProposals(ctx context.Context, _ *mcp.CallToolRequest, in listProposalsInput) (*mcp.CallToolResult, listProposalsOutput, error) {
	if d.Proposals == nil {
		return toolErr("proposals are not available in this build"), listProposalsOutput{}, nil
	}
	p, errResult := d.requireProject(ctx, in.ProjectID)
	if errResult != nil {
		return errResult, listProposalsOutput{}, nil
	}
	batches, err := d.Proposals.List(ctx, p.ID, nil)
	if err != nil {
		return nil, listProposalsOutput{}, err
	}
	limit := in.Limit
	if limit <= 0 {
		limit = defaultProposalListLimit
	}
	limit = min(limit, maxProposalListLimit)
	offset := max(in.Offset, 0)
	proposalID := strings.TrimSpace(in.ProposalID)

	out := listProposalsOutput{Batches: []proposalBatchOut{}, Items: []proposalItemOut{}}
	for _, b := range batches {
		if proposalID != "" && b.ID != proposalID {
			continue
		}
		open := 0
		for _, it := range b.Items {
			if !proposals.IsOpen(it) {
				continue
			}
			open++
			out.Total++
			if out.Total <= offset || len(out.Items) >= limit {
				continue
			}
			out.Items = append(out.Items, proposalItemOut{
				ID: it.ID, ProposalID: b.ID, Status: it.Status,
				Summary:           d.Proposals.Summarize(ctx, it.Op, b.Items),
				DependsOn:         it.DependsOn,
				BlockedByRejected: proposals.DependencyRejected(it, b.Items),
				Duplicates:        it.Duplicates,
				Error:             it.Result.Error,
			})
		}
		if open > 0 {
			out.Batches = append(out.Batches, proposalBatchOut{ID: b.ID, Summary: b.Summary, Purpose: b.Purpose, Open: open})
		}
	}
	return nil, out, nil
}

// decisionItemIDs resolves what a decision tool was pointed at: the named
// items plus every open item of the named batches, each once.
func (d ToolDeps) decisionItemIDs(ctx context.Context, projectID string, in decideProposalsInput) ([]string, *mcp.CallToolResult, error) {
	if len(in.ItemIDs) == 0 && len(in.ProposalIDs) == 0 {
		return nil, toolErr("pass item_ids or proposal_ids; linetta_list_proposals shows them"), nil
	}
	seen := map[string]bool{}
	ids := make([]string, 0, len(in.ItemIDs))
	add := func(id string) {
		if id = strings.TrimSpace(id); id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, id := range in.ItemIDs {
		add(id)
	}
	if len(in.ProposalIDs) > 0 {
		batches, err := d.Proposals.List(ctx, projectID, nil)
		if err != nil {
			return nil, nil, err
		}
		for _, proposalID := range in.ProposalIDs {
			proposalID = strings.TrimSpace(proposalID)
			known := false
			for _, b := range batches {
				if b.ID != proposalID {
					continue
				}
				known = true
				for _, it := range b.Items {
					if proposals.IsOpen(it) {
						add(it.ID)
					}
				}
			}
			if !known {
				return nil, toolErr("no batch %q in this work; linetta_list_proposals shows them", proposalID), nil
			}
		}
	}
	return ids, nil, nil
}

func (d ToolDeps) acceptProposals(ctx context.Context, _ *mcp.CallToolRequest, in decideProposalsInput) (*mcp.CallToolResult, acceptProposalsOutput, error) {
	if d.Proposals == nil {
		return toolErr("proposals are not available in this build"), acceptProposalsOutput{}, nil
	}
	p, errResult := d.requireProject(ctx, in.ProjectID)
	if errResult != nil {
		return errResult, acceptProposalsOutput{}, nil
	}
	ids, errResult, err := d.decisionItemIDs(ctx, p.ID, in)
	if errResult != nil || err != nil {
		return errResult, acceptProposalsOutput{}, err
	}
	res, err := d.Proposals.Accept(ctx, d.now(), p.ID, ids, nil)
	if err != nil {
		return nil, acceptProposalsOutput{}, err
	}
	out := acceptProposalsOutput{Items: make([]acceptedItemOut, 0, len(res.Items)), UndoBatchIDs: res.UndoBatchIDs}
	for _, it := range res.Items {
		switch it.Status {
		case proposals.StatusAccepted:
			out.Accepted++
		case proposals.StatusFailed:
			out.Failed++
		}
		out.Items = append(out.Items, acceptedItemOut{
			ID: it.ID, Status: it.Status, CreatedID: it.Result.CreatedID,
			LinkedExisting: it.Result.LinkedExisting, Error: it.Result.Error,
		})
	}
	d.notifyChanged(p.ID, "linetta_accept_proposals", nil, "")
	return nil, out, nil
}

func (d ToolDeps) rejectProposals(ctx context.Context, _ *mcp.CallToolRequest, in decideProposalsInput) (*mcp.CallToolResult, rejectProposalsOutput, error) {
	if d.Proposals == nil {
		return toolErr("proposals are not available in this build"), rejectProposalsOutput{}, nil
	}
	p, errResult := d.requireProject(ctx, in.ProjectID)
	if errResult != nil {
		return errResult, rejectProposalsOutput{}, nil
	}
	ids, errResult, err := d.decisionItemIDs(ctx, p.ID, in)
	if errResult != nil || err != nil {
		return errResult, rejectProposalsOutput{}, err
	}
	rejected, err := d.Proposals.Reject(ctx, d.now(), p.ID, ids)
	if err != nil {
		return nil, rejectProposalsOutput{}, err
	}
	d.notifyChanged(p.ID, "linetta_reject_proposals", nil, "")
	return nil, rejectProposalsOutput{Rejected: rejected, Skipped: len(ids) - rejected}, nil
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
