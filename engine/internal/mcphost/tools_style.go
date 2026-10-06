//go:build !mobile

package mcphost

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/style"
)

// ---------- linetta_analyze_style ----------
//
// The style notes in the story brief tell an agent how the writer wants the
// prose to read; nothing so far told anyone whether a draft did (#162). This
// tool answers the part of that question a machine can settle — the writer's
// listed phrases and sentence limit — and leaves tone to whoever calls it.
// It reads. It does not touch the scene.

// defaultStyleViolations keeps a default report small enough to sit in an
// agent's context beside the scene it is about.
const defaultStyleViolations = 50

type analyzeStyleInput struct {
	ProjectID string `json:"project_id,omitempty" jsonschema:"id of the work; optional when node_id is given"`
	NodeID    string `json:"node_id,omitempty" jsonschema:"a scene, or a chapter to cover its scenes; omit for the whole work"`
	Limit     int    `json:"limit,omitempty" jsonschema:"violations to list, default 50, at most 200"`
}

func (in analyzeStyleInput) scope() (string, string) { return in.ProjectID, in.NodeID }

type styleRulesOut struct {
	AvoidPhrases     []string `json:"avoid_phrases"`
	MaxSentenceChars int      `json:"max_sentence_chars" jsonschema:"0 means no limit"`
}

type analyzeStyleOutput struct {
	ProjectID string        `json:"project_id"`
	Rules     styleRulesOut `json:"rules" jsonschema:"the writer's checkable rules; empty means nothing was checked"`
	// The report's own counts, so a capped list is never mistaken for all of it.
	ScenesChecked int               `json:"scenes_checked"`
	Total         int               `json:"total" jsonschema:"every violation found, listed or not"`
	Violations    []style.Violation `json:"violations"`
	Truncated     bool              `json:"truncated"`
}

func registerStyleTools(s *mcp.Server, d ToolDeps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_analyze_style",
		Description: "Check a scene, a chapter, or the whole work against the writer's style rules: phrases " +
			"to avoid and a sentence-length limit. Returns each violation with its scene, paragraph and an " +
			"excerpt. Reads only; it changes nothing. It cannot judge tone or rhythm — hold the text " +
			"against the style notes in the story brief for that yourself.",
	}, record(d, "linetta_analyze_style", d.analyzeStyle))
}

func (d ToolDeps) analyzeStyle(ctx context.Context, _ *mcp.CallToolRequest, in analyzeStyleInput) (*mcp.CallToolResult, analyzeStyleOutput, error) {
	if d.Style == nil {
		return toolErr("style rules are not available in this build"), analyzeStyleOutput{}, nil
	}
	projectID := in.ProjectID
	if in.NodeID != "" {
		n, errResult := d.requireNodeInProject(ctx, in.NodeID, in.ProjectID)
		if errResult != nil {
			return errResult, analyzeStyleOutput{}, nil
		}
		projectID = n.ProjectID
	}
	p, errResult := d.requireProject(ctx, projectID)
	if errResult != nil {
		return errResult, analyzeStyleOutput{}, nil
	}
	rules, err := d.Style.GetRules(ctx, p.ID)
	if err != nil {
		return nil, analyzeStyleOutput{}, err
	}
	nodes, err := d.Nodes.ListByProject(ctx, p.ID)
	if err != nil {
		return nil, analyzeStyleOutput{}, err
	}
	rep := style.Check(style.ScenesUnder(nodes, in.NodeID), rules)

	limit := in.Limit
	if limit <= 0 {
		limit = defaultStyleViolations
	}
	if limit < len(rep.Violations) {
		rep.Violations = rep.Violations[:limit]
		rep.Truncated = true
	}
	return nil, analyzeStyleOutput{
		ProjectID:     p.ID,
		Rules:         styleRulesOut{AvoidPhrases: rules.AvoidPhrases, MaxSentenceChars: rules.MaxSentenceChars},
		ScenesChecked: rep.ScenesChecked,
		Total:         rep.Total,
		Violations:    rep.Violations,
		Truncated:     rep.Truncated,
	}, nil
}
