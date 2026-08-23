//go:build !mobile

package mcphost

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/manuscriptedit"
)

// ---------- linetta_revise_scene ----------

type reviseSceneInput struct {
	ProjectID string `json:"project_id" jsonschema:"id of the work"`
	Find      string `json:"find" jsonschema:"the exact text to replace"`
	Replace   string `json:"replace" jsonschema:"what to put in its place"`
	// NodeIDs narrows the edit to specific scenes. Omit to sweep the work —
	// useful for a rename, dangerous for a common phrase, so the tool reports
	// what it touched.
	NodeIDs   []string `json:"node_ids,omitempty" jsonschema:"scenes to edit; omit to search the whole work"`
	MatchCase bool     `json:"match_case,omitempty"`
	WholeWord bool     `json:"whole_word,omitempty"`
	// DryRun returns the matches without changing anything.
	DryRun bool `json:"dry_run,omitempty" jsonschema:"preview the matches without applying them"`
}

func (in reviseSceneInput) scope() (string, string) { return in.ProjectID, "" }

type reviseMatch struct {
	NodeID      string `json:"node_id"`
	Label       string `json:"label,omitempty"`
	Occurrences int    `json:"occurrences"`
	Before      string `json:"before,omitempty"`
	After       string `json:"after,omitempty"`
}

type reviseSceneOutput struct {
	Applied      int           `json:"applied"`
	DryRun       bool          `json:"dry_run"`
	Matches      []reviseMatch `json:"matches"`
	ChangedNodes []string      `json:"changed_nodes,omitempty"`
	Failures     []string      `json:"failures,omitempty"`
}

func (d ToolDeps) registerReviseTool(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_revise_scene",
		Description: "Replace exact text across one or more scenes — a renamed character, a corrected " +
			"term, a reworded line — without resending whole scene bodies. Every touched scene is " +
			"snapshotted first. Pass dry_run to see what would change before committing; omit node_ids " +
			"only when you mean to sweep the entire work.",
	}, record(d, "linetta_revise_scene", d.reviseScene))
}

func (d ToolDeps) reviseScene(ctx context.Context, _ *mcp.CallToolRequest, in reviseSceneInput) (*mcp.CallToolResult, reviseSceneOutput, error) {
	p, errResult := d.requireProject(ctx, in.ProjectID)
	if errResult != nil {
		return errResult, reviseSceneOutput{}, nil
	}
	find := strings.TrimSpace(in.Find)
	if find == "" {
		return toolErr("find is required"), reviseSceneOutput{}, nil
	}
	if strings.TrimSpace(in.Replace) == "" {
		return toolErr("replace is required; to delete text, replace it with the surrounding wording you want"),
			reviseSceneOutput{}, nil
	}
	if d.Manuscript == nil || d.ManuscriptEdit == nil {
		return toolErr("manuscript editing is unavailable in this build"), reviseSceneOutput{}, nil
	}
	// Every named scene must belong to an allowed work, or a restricted server
	// could be steered into another work by id.
	for _, nodeID := range in.NodeIDs {
		if _, errResult := d.requireNode(ctx, nodeID); errResult != nil {
			return errResult, reviseSceneOutput{}, nil
		}
	}

	plan, err := d.ManuscriptEdit.PlanReplace(ctx, manuscriptedit.ReplacePlanRequest{
		ProjectID:   p.ID,
		Query:       find,
		Replacement: in.Replace,
		NodeIDs:     in.NodeIDs,
		MatchCase:   in.MatchCase,
		WholeWord:   in.WholeWord,
	})
	if err != nil {
		return toolErr("could not plan the revision: %v", err), reviseSceneOutput{}, nil
	}
	if len(plan.Candidates) == 0 {
		return toolErr("no scene contains %q; check the exact wording with linetta_search_manuscript", find),
			reviseSceneOutput{}, nil
	}

	out := reviseSceneOutput{DryRun: in.DryRun}
	candidateIDs := make([]string, 0, len(plan.Candidates))
	for _, c := range plan.Candidates {
		out.Matches = append(out.Matches, reviseMatch{
			NodeID:      c.NodeID,
			Label:       c.Breadcrumb,
			Occurrences: c.Occurrences,
			Before:      c.Before,
			After:       c.After,
		})
		candidateIDs = append(candidateIDs, c.ID)
	}
	if in.DryRun {
		return nil, out, nil
	}

	result, err := d.ManuscriptEdit.ApplyReplace(ctx, plan, candidateIDs, d.now())
	if err != nil {
		return toolErr("could not apply the revision: %v", err), reviseSceneOutput{}, nil
	}
	out.Applied = result.Applied
	out.ChangedNodes = result.ChangedNodeIDs
	for _, f := range result.Failures {
		out.Failures = append(out.Failures, f.Message)
	}
	for _, nodeID := range result.ChangedNodeIDs {
		if d.EnqueueSummary != nil {
			d.EnqueueSummary(nodeID)
		}
	}
	if result.Applied > 0 {
		d.notifyChanged(p.ID, "linetta_revise_scene", result.ChangedNodeIDs, "")
	}
	if result.Applied == 0 && len(result.Failures) > 0 {
		return toolErr("nothing was revised: %s", strings.Join(out.Failures, "; ")), out, nil
	}
	return nil, out, nil
}
