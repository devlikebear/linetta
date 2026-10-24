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

func (d ToolDeps) registerContinuityWriteTools(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "linetta_report_continuity",
		Description: "Record what you found checking one scene against the work's own record (Story " +
			"World, fact cards, earlier scenes, the writer's direction). Cite paragraphs from " +
			"linetta_read_scene numbered=true and copy quotes exactly; every contradiction must point " +
			"at what it contradicts. Taste is not a finding. An empty list records a clean check. The " +
			"writer reviews these; nothing in the manuscript changes.",
	}, record(d, "linetta_report_continuity", d.reportContinuity))
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
