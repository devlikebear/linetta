//go:build !mobile

package mcphost

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

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
	// Counted from the same scenes (#163): the raw material for describing
	// how this writer writes. It holds a style.Stats, typed as any on purpose:
	// a typed field would put a twelve-property schema — about 900B — into
	// every request of every turn, to describe keys that already say what
	// they are (sentence_mean, dialogue_share, endings).
	Stats any `json:"stats" jsonschema:"lengths in characters, shares 0-1"`
}

// ---------- linetta_propose_style_profile ----------
//
// The other half of #163. An agent can measure the writer's scenes and read
// them, and from that describe the style in words — but what it writes is its
// own reading, not the writer's wish. So it lands as a draft the writer sees
// in Story World, and reaches the style notes only through the app's own
// approve action. There is deliberately no MCP tool that approves.

type proposeStyleProfileInput struct {
	ProjectID string `json:"project_id" jsonschema:"id of the work"`
	Profile   string `json:"profile" jsonschema:"the style in prose, at most 2200 characters"`
}

func (in proposeStyleProfileInput) scope() (string, string) { return in.ProjectID, "" }

type proposeStyleProfileOutput struct {
	ProjectID  string `json:"project_id"`
	Status     string `json:"status"`
	Characters int    `json:"characters"`
	Limit      int    `json:"limit"`
}

// styleProfilePending is the only status a proposal can have: nothing an
// agent does moves it further.
const styleProfilePending = "pending_writer_approval"

func registerStyleTools(s *mcp.Server, d ToolDeps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_analyze_style",
		Description: "Check a scene, a chapter, or the whole work against the writer's style rules " +
			"(phrases to avoid, a sentence-length limit) and measure it (sentence lengths, dialogue " +
			"share, common endings). Each violation comes with its scene, paragraph and an excerpt. " +
			"Reads only. It cannot judge tone — hold the text against the style notes yourself.",
	}, record(d, "linetta_analyze_style", d.analyzeStyle))
}

func (d ToolDeps) registerStyleWriteTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_propose_style_profile",
		Description: "Save a style profile you drew from the writer's own scenes (measure them with " +
			"linetta_analyze_style, read a few) as a draft. The writer reviews it in Story World and it " +
			"joins the style notes only if they approve: say it is waiting, and do not treat it as their " +
			"preference. Replaces an earlier draft.",
	}, record(d, "linetta_propose_style_profile", d.proposeStyleProfile))
}

func (d ToolDeps) proposeStyleProfile(ctx context.Context, _ *mcp.CallToolRequest, in proposeStyleProfileInput) (*mcp.CallToolResult, proposeStyleProfileOutput, error) {
	if d.Style == nil {
		return toolErr("style profiles are not available in this build"), proposeStyleProfileOutput{}, nil
	}
	p, errResult := d.requireProject(ctx, in.ProjectID)
	if errResult != nil {
		return errResult, proposeStyleProfileOutput{}, nil
	}
	draft, err := d.Style.SaveDraft(ctx, d.now(), p.ID, in.Profile, d.sourceOrExternal())
	switch {
	case errors.Is(err, style.ErrDraftEmpty):
		return toolErr("profile is empty; describe the style in prose"), proposeStyleProfileOutput{}, nil
	case errors.Is(err, style.ErrDraftTooLong):
		return toolErr("profile is %d characters; the limit is %d — shorten it and call again",
			utf8.RuneCountInString(strings.TrimSpace(in.Profile)), style.MaxProfileRunes), proposeStyleProfileOutput{}, nil
	case err != nil:
		return nil, proposeStyleProfileOutput{}, err
	}
	d.notifyChanged(p.ID, "linetta_propose_style_profile", nil, "")
	return nil, proposeStyleProfileOutput{
		ProjectID:  p.ID,
		Status:     styleProfilePending,
		Characters: utf8.RuneCountInString(draft.Body),
		Limit:      style.MaxProfileRunes,
	}, nil
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
	scenes := style.ScenesUnder(nodes, in.NodeID)
	rep := style.Check(scenes, rules)

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
		Stats:         style.Measure(scenes),
	}, nil
}
