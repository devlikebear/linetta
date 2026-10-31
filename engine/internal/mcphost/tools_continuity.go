//go:build !mobile

package mcphost

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/continuity"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/style"
)

// ---------- linetta_report_continuity ----------
//
// story-assist Phase 2. An agent reads a scene, holds it against the work's
// own record (Story World, fact cards, earlier scenes, the writer's
// direction), and records what it found. The engine verifies every quote
// and every ref before any of it is stored — continuity.Validate — so a
// writer reading these observations in Story World is reading claims the
// engine itself checked, not claims the model merely asserted. Nothing here
// touches the manuscript.

// reportContinuityInput is continuity.ReportInput under a locally-defined
// name: a method cannot be added to a type from another package, even via
// an alias, and scope() is what record() uses to log what a call touched.
type reportContinuityInput continuity.ReportInput

func (in reportContinuityInput) scope() (string, string) { return in.ProjectID, in.NodeID }

type reportContinuityOutput struct {
	CheckID  string `json:"check_id"`
	Recorded int    `json:"recorded"`
	Status   string `json:"status"`
}

// ---------- linetta_list_continuity ----------
//
// The ids linetta_update_continuity needs. Open observations by default: the
// rest are settled, and a whole work's checks can run to hundreds of rows.

const (
	defaultContinuityListLimit = 50
	maxContinuityListLimit     = 200
)

type listContinuityInput struct {
	ProjectID string   `json:"project_id"`
	NodeID    string   `json:"node_id,omitempty" jsonschema:"only this scene"`
	Statuses  []string `json:"statuses,omitempty" jsonschema:"any of open, dismissed, resolved; default open"`
	Limit     int      `json:"limit,omitempty" jsonschema:"at most 200; default 50"`
	Offset    int      `json:"offset,omitempty"`
}

func (in listContinuityInput) scope() (string, string) { return in.ProjectID, in.NodeID }

type continuityObservationOut struct {
	ID            string           `json:"id"`
	CheckID       string           `json:"check_id"`
	NodeID        string           `json:"node_id"`
	Scene         string           `json:"scene"`
	Stale         bool             `json:"stale,omitempty" jsonschema:"the scene's text changed since this was recorded"`
	Status        string           `json:"status"`
	Category      string           `json:"category"`
	Kind          string           `json:"kind"`
	Severity      string           `json:"severity"`
	Summary       string           `json:"summary"`
	Suggestion    string           `json:"suggestion,omitempty"`
	ParagraphFrom int              `json:"paragraph_from"`
	ParagraphTo   int              `json:"paragraph_to"`
	Quote         string           `json:"quote"`
	Refs          []continuity.Ref `json:"refs,omitempty"`
}

type listContinuityOutput struct {
	Observations []continuityObservationOut `json:"observations"`
	Total        int                        `json:"total" jsonschema:"matching observations in all, including any past this page"`
}

// ---------- linetta_update_continuity ----------
//
// The writer can hand the decision over ("연속성 점검 정리해 줘"), the same way
// they can with the inbox: this is the Repo the Story World buttons call, and
// what keeps the decision theirs is the description's and the system prompt's
// requirement of their word for it.

const (
	continuityActionDismiss = "dismiss"
	continuityActionResolve = "resolve"
	continuityActionReopen  = "reopen"
	continuityActionDelete  = "delete"
)

type updateContinuityInput struct {
	ProjectID      string   `json:"project_id"`
	Action         string   `json:"action" jsonschema:"one of: dismiss, resolve, reopen, delete"`
	ObservationIDs []string `json:"observation_ids,omitempty" jsonschema:"ids from linetta_list_continuity"`
	CheckIDs       []string `json:"check_ids,omitempty" jsonschema:"whole checks: their open observations for dismiss and resolve, settled ones for reopen, all for delete"`
}

func (in updateContinuityInput) scope() (string, string) { return in.ProjectID, "" }

type updateContinuityOutput struct {
	Action  string `json:"action"`
	Changed int    `json:"changed"`
	Skipped int    `json:"skipped" jsonschema:"named items left as they were: unknown, replaced by a later check, or already there"`
}

func (d ToolDeps) registerContinuityReadTool(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_list_continuity",
		Description: "List the work's recorded continuity observations with their ids, newest check first. " +
			"Each says what it found, where, and whether the scene has changed since.",
	}, record(d, "linetta_list_continuity", d.listContinuity))
}

func (d ToolDeps) registerContinuityWriteTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_report_continuity",
		Description: "Record what you found checking one scene against the work's own record (Story " +
			"World, fact cards, earlier scenes, the writer's direction). Cite paragraphs from " +
			"linetta_read_scene numbered=true and copy quotes exactly; every contradiction must point " +
			"at what it contradicts. Taste is not a finding. An empty list records a clean check. The " +
			"writer reviews these; nothing in the manuscript changes.",
	}, record(d, "linetta_report_continuity", d.reportContinuity))

	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_update_continuity",
		Description: "Settle continuity observations in bulk: dismiss, resolve, reopen, or delete. Only when the " +
			"writer has told you to in this conversation; if they hand you the cleanup, judge each one yourself. " +
			"Delete cannot be undone; the others can be reversed with reopen. Nothing in the manuscript changes.",
	}, record(d, "linetta_update_continuity", d.updateContinuity))
}

func (d ToolDeps) listContinuity(ctx context.Context, _ *mcp.CallToolRequest, in listContinuityInput) (*mcp.CallToolResult, listContinuityOutput, error) {
	if d.Continuity == nil {
		return toolErr("continuity checks are not available in this build"), listContinuityOutput{}, nil
	}
	p, errResult := d.requireProject(ctx, in.ProjectID)
	if errResult != nil {
		return errResult, listContinuityOutput{}, nil
	}
	nodeID := strings.TrimSpace(in.NodeID)
	if nodeID != "" {
		if _, errResult := d.requireNodeInProject(ctx, nodeID, p.ID); errResult != nil {
			return errResult, listContinuityOutput{}, nil
		}
	}
	statuses := in.Statuses
	if len(statuses) == 0 {
		statuses = []string{continuity.StatusOpen}
	}
	for _, st := range statuses {
		if !continuity.ValidStatus(st) {
			return toolErr("status %q is not one of open, dismissed, resolved", st), listContinuityOutput{}, nil
		}
	}
	checks, err := d.Continuity.List(ctx, continuity.ListFilter{ProjectID: p.ID, NodeID: nodeID, Statuses: statuses})
	if err != nil {
		return nil, listContinuityOutput{}, err
	}
	nodes, err := d.Nodes.ListByProject(ctx, p.ID)
	if err != nil {
		return nil, listContinuityOutput{}, err
	}
	byID := make(map[string]node.Node, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}

	limit := in.Limit
	if limit <= 0 {
		limit = defaultContinuityListLimit
	}
	limit = min(limit, maxContinuityListLimit)
	offset := max(in.Offset, 0)
	out := listContinuityOutput{Observations: []continuityObservationOut{}}
	for _, c := range checks {
		for _, o := range c.Observations {
			out.Total++
			if out.Total <= offset || len(out.Observations) >= limit {
				continue
			}
			out.Observations = append(out.Observations, continuityObservationOut{
				ID: o.ID, CheckID: o.CheckID, NodeID: o.NodeID,
				Scene: node.BreadcrumbLabel(byID, byID[o.NodeID]), Stale: c.Stale, Status: o.Status,
				Category: o.Category, Kind: o.Kind, Severity: o.Severity,
				Summary: o.Summary, Suggestion: o.Suggestion,
				ParagraphFrom: o.ParagraphFrom, ParagraphTo: o.ParagraphTo, Quote: o.Quote, Refs: o.Refs,
			})
		}
	}
	return nil, out, nil
}

func (d ToolDeps) updateContinuity(ctx context.Context, _ *mcp.CallToolRequest, in updateContinuityInput) (*mcp.CallToolResult, updateContinuityOutput, error) {
	if d.Continuity == nil {
		return toolErr("continuity checks are not available in this build"), updateContinuityOutput{}, nil
	}
	p, errResult := d.requireProject(ctx, in.ProjectID)
	if errResult != nil {
		return errResult, updateContinuityOutput{}, nil
	}
	if len(in.ObservationIDs) == 0 && len(in.CheckIDs) == 0 {
		return toolErr("pass observation_ids or check_ids; linetta_list_continuity shows them"), updateContinuityOutput{}, nil
	}
	sel := continuity.Selection{ProjectID: p.ID, ObservationIDs: in.ObservationIDs, CheckIDs: in.CheckIDs}
	var (
		outcome continuity.Outcome
		err     error
	)
	switch action := strings.TrimSpace(in.Action); action {
	case continuityActionDismiss:
		outcome, err = d.Continuity.SetStatusMany(ctx, d.now(), sel, continuity.StatusDismissed)
	case continuityActionResolve:
		outcome, err = d.Continuity.SetStatusMany(ctx, d.now(), sel, continuity.StatusResolved)
	case continuityActionReopen:
		outcome, err = d.Continuity.SetStatusMany(ctx, d.now(), sel, continuity.StatusOpen)
	case continuityActionDelete:
		outcome, err = d.Continuity.DeleteMany(ctx, sel)
	default:
		return toolErr("action must be one of dismiss, resolve, reopen, delete"), updateContinuityOutput{}, nil
	}
	if err != nil {
		return nil, updateContinuityOutput{}, err
	}
	if outcome.Changed > 0 {
		d.notifyChanged(p.ID, "linetta_update_continuity", nil, "")
	}
	return nil, updateContinuityOutput{Action: in.Action, Changed: outcome.Changed, Skipped: outcome.Skipped}, nil
}

func (d ToolDeps) reportContinuity(ctx context.Context, _ *mcp.CallToolRequest, in reportContinuityInput) (*mcp.CallToolResult, reportContinuityOutput, error) {
	if d.Continuity == nil {
		return toolErr("continuity checks are not available in this build"), reportContinuityOutput{}, nil
	}
	n, errResult := d.requireNodeInProject(ctx, in.NodeID, in.ProjectID)
	if errResult != nil {
		return errResult, reportContinuityOutput{}, nil
	}
	if n.Kind != node.KindLeaf {
		return toolErr("node %q is a container (%s), not a scene; only scenes can be continuity-checked", n.ID, n.Label),
			reportContinuityOutput{}, nil
	}
	if in.SceneVersion != n.ContentVersion {
		return toolErr("the scene changed since you read it (version %d, now %d); read it again with "+
			"numbered=true", in.SceneVersion, n.ContentVersion), reportContinuityOutput{}, nil
	}

	paragraphs := []string{}
	if n.ContentDoc != nil {
		paragraphs = style.Paragraphs(*n.ContentDoc)
	}
	report := continuity.ReportInput(in)
	lookup := continuityRefLookup{ctx: ctx, projectID: n.ProjectID, deps: d}
	if problems := continuity.Validate(report, paragraphs, lookup); len(problems) > 0 {
		return toolErr("%s", problemsText(problems)), reportContinuityOutput{}, nil
	}

	report.ProjectID = n.ProjectID
	check, err := d.Continuity.Record(ctx, d.now(), report, d.sourceOrExternal())
	if err != nil {
		return nil, reportContinuityOutput{}, err
	}
	d.notifyChanged(n.ProjectID, "linetta_report_continuity", []string{n.ID}, "")
	return nil, reportContinuityOutput{
		CheckID:  check.ID,
		Recorded: len(report.Observations),
		Status:   continuity.ReportStatusWaitingForWriter,
	}, nil
}

// problemsText joins every continuity.Problem into one refusal, numbered so
// a model fixing several at once can tell them apart, and so it fixes every
// one of them before calling again rather than discovering them one retry
// at a time.
func problemsText(problems []continuity.Problem) string {
	if len(problems) == 1 {
		return problems[0].Message
	}
	lines := make([]string, len(problems))
	for i, p := range problems {
		lines[i] = p.Message
	}
	return strings.Join(lines, "\n")
}

// continuityRefLookup implements continuity.RefLookup against the real
// repos, scoped to one work. continuity.Validate takes no context of its
// own — see its doc comment — so this adapter, built fresh for the single
// Validate call inside reportContinuity, is where ctx is allowed to live.
type continuityRefLookup struct {
	ctx       context.Context
	projectID string
	deps      ToolDeps
}

func (l continuityRefLookup) HasEntity(id string) bool {
	if l.deps.Entities == nil || strings.TrimSpace(id) == "" {
		return false
	}
	e, err := l.deps.Entities.Get(l.ctx, id)
	return err == nil && e.ProjectID == l.projectID
}

func (l continuityRefLookup) HasFact(id string) bool {
	if l.deps.Facts == nil || strings.TrimSpace(id) == "" {
		return false
	}
	c, err := l.deps.Facts.Get(l.ctx, id)
	return err == nil && c.ProjectID == l.projectID
}

func (l continuityRefLookup) HasBeat(id string) bool {
	if l.deps.Beats == nil || l.deps.Threads == nil || strings.TrimSpace(id) == "" {
		return false
	}
	b, err := l.deps.Beats.Get(l.ctx, id)
	if err != nil {
		return false
	}
	th, err := l.deps.Threads.Get(l.ctx, b.ThreadID)
	return err == nil && th.ProjectID == l.projectID
}

func (l continuityRefLookup) SceneParagraphs(nodeID string) ([]string, bool) {
	if l.deps.Nodes == nil || strings.TrimSpace(nodeID) == "" {
		return nil, false
	}
	n, err := l.deps.Nodes.Get(l.ctx, nodeID)
	if err != nil || n.ProjectID != l.projectID || n.Kind != node.KindLeaf {
		return nil, false
	}
	if n.ContentDoc == nil {
		return []string{}, true
	}
	return style.Paragraphs(*n.ContentDoc), true
}
