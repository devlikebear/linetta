//go:build !mobile

package mcphost

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/agentmemory"
	"github.com/devlikebear/linetta/engine/internal/agentskills"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/manuscript"
	"github.com/devlikebear/linetta/engine/internal/manuscriptedit"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/plot"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/snapshot"
	"github.com/devlikebear/linetta/engine/internal/storycontext"
	"github.com/devlikebear/linetta/engine/internal/storyops"
)

// MetaRunID is the MCP _meta key the built-in agent stamps each tool call
// with. A Go context value does not cross the in-memory transport — the
// server handler sees a fresh context — so this is how one turn's calls are
// tied together in the activity log.
const MetaRunID = "linetta/run_id"

// ToolDeps carries everything the tool layer reads from. Every field is a repo
// the UI already uses, so an agent sees exactly what the writer sees.
type ToolDeps struct {
	Projects   *project.Repo
	Nodes      *node.Repo
	Entities   *entity.Repo
	Mentions   *mention.Repo
	Facts      *fact.Repo
	Plot       *plot.Builder
	Manuscript *manuscript.Searcher
	Context    *storycontext.ContextBuilder
	Settings   *settings.Store
	Activity   *ActivityRepo

	// Memory is the two curated documents every agent reads. Nil in a build
	// with no database open; the tool refuses rather than panicking.
	Memory *agentmemory.Repo

	// Skills are the SKILL.md documents an agent writes for itself and the
	// writer can read, edit and revert. Skills is the filesystem store under
	// the Linetta home; SkillHistory is the version row every write lands, so
	// a bad edit is revertible. Both nil in a build with no home or database;
	// the tools refuse rather than panicking.
	Skills       *agentskills.Store
	SkillHistory *agentskills.History

	// Source names who is calling: SourceExternal (the HTTP host) or
	// SourceAgent (the built-in panel's in-memory server). It is a field on
	// the deps rather than something read off the wire, so an external client
	// cannot claim to be the agent. Empty means external.
	Source string

	// Write-side collaborators. Snapshots make every body change revertible;
	// EnqueueSummary keeps agent prose in the summarizer's queue; Notify tells
	// the running UI that something outside it changed the manuscript.
	Snapshots      *snapshot.Repo
	Story          *storyops.Service
	ManuscriptEdit *manuscriptedit.Service
	Limiter        *limiter
	EnqueueSummary func(nodeID string)
	Notify         func(method string, params any)
	Clock          func() int64
}

// now returns the wall clock the tools stamp writes with.
func (d ToolDeps) now() int64 {
	if d.Clock != nil {
		return d.Clock()
	}
	return time.Now().UnixMilli()
}

// ChangedPayload is the body of an "mcp.changed" notification: what an agent
// just altered, so the UI can refetch instead of showing stale text.
type ChangedPayload struct {
	ProjectID string   `json:"project_id"`
	Tool      string   `json:"tool"`
	NodeIDs   []string `json:"node_ids,omitempty"`
	BatchID   string   `json:"batch_id,omitempty"`

	// Source is who wrote it: SourceExternal (Claude Desktop and friends over
	// HTTP) or SourceAgent (the writer's own built-in panel). Without it the
	// UI cannot tell the two apart, and tells a writer whose unsaved scene the
	// panel just revised — at their own request — that "an external agent
	// changed this scene", possibly while the HTTP host is not even running.
	//
	// It comes from ToolDeps.Source, the same field that stamps the activity
	// log, so an external client cannot claim to be the agent: it is set when
	// the server is composed, never read off the wire.
	Source string `json:"source"`
}

func (d ToolDeps) notifyChanged(projectID, tool string, nodeIDs []string, batchID string) {
	if d.Notify == nil {
		return
	}
	d.Notify("mcp.changed", ChangedPayload{
		ProjectID: projectID, Tool: tool, NodeIDs: nodeIDs, BatchID: batchID,
		Source: d.sourceOrExternal(),
	})
}

// Register installs the tool set for a mode. Read tools are always present;
// write tools (Phase 3) are registered only for settings.MCPModeFull, so
// read_only does not merely refuse writes — the tools are absent from
// tools/list and cannot be called at all.
//
// The mode is captured when the listener starts. Changing it goes through
// Host.Restart (see mcpController.Enable), which builds a fresh server, so a
// running server never serves a stale tool set.
func (d ToolDeps) Register(s *mcp.Server, mode string) {
	d.registerReadTools(s)
	if mode == settings.MCPModeFull {
		d.registerWriteTools(s)
	}
}

// scopedInput is implemented by tool inputs that name a work and/or a target,
// so the activity log can record what was touched without every tool repeating it.
type scopedInput interface {
	scope() (projectID, targetID string)
}

// record wraps a typed tool handler so every call — success or failure — lands
// in the activity log the writer can inspect. Wrapping at registration time
// means no tool can forget to report itself.
func record[In, Out any](d ToolDeps, tool string, h mcp.ToolHandlerFor[In, Out]) mcp.ToolHandlerFor[In, Out] {
	h = limited(d.Limiter, h)
	return func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		res, out, err := h(ctx, req, in)

		projectID, targetID := "", ""
		if s, ok := any(in).(scopedInput); ok {
			projectID, targetID = s.scope()
		}
		ok := err == nil && (res == nil || !res.IsError)
		detail := ""
		if err != nil {
			detail = err.Error()
		} else if res != nil && res.IsError {
			detail = firstText(res)
		}
		d.recordActivity(ctx, tool, projectID, targetID, ok, detail, d.runIDOf(req))
		return res, out, err
	}
}

// runIDOf reads the built-in agent's run id off the request. Only an agent
// server reads it: external callers control their own _meta, and a forged
// run id would put their edits under the panel's undo button.
func (d ToolDeps) runIDOf(req *mcp.CallToolRequest) string {
	if d.Source != SourceAgent || req == nil || req.Params == nil {
		return ""
	}
	id, _ := req.Params.GetMeta()[MetaRunID].(string)
	return id
}

func (d ToolDeps) recordActivity(ctx context.Context, tool, projectID, targetID string, ok bool, detail, runID string) {
	if d.Activity == nil {
		return
	}
	if err := d.Activity.Record(ctx, ActivityEntry{
		Tool:      tool,
		ProjectID: projectID,
		TargetID:  targetID,
		OK:        ok,
		Detail:    detail,
		Source:    d.sourceOrExternal(),
		RunID:     runID,
	}); err != nil {
		logf("activity log: %v", err)
	}
}

func (d ToolDeps) sourceOrExternal() string {
	if d.Source == "" {
		return SourceExternal
	}
	return d.Source
}

func firstText(res *mcp.CallToolResult) string {
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

// toolErr returns a tool-level error result. Agents recover from these; a Go
// error would surface as a transport failure they cannot act on.
func toolErr(format string, args ...any) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(format, args...)}},
	}
}

// requireProject resolves the work a call targets and enforces the optional
// single-work restriction. Every tool funnels through here so the restriction
// cannot be bypassed by a tool that forgot to check.
func (d ToolDeps) requireProject(ctx context.Context, projectID string) (project.Project, *mcp.CallToolResult) {
	projectID = strings.TrimSpace(projectID)
	restricted := d.allowedProjectID()
	if restricted != "" {
		if projectID == "" {
			projectID = restricted
		} else if projectID != restricted {
			return project.Project{}, toolErr(
				"this Linetta server is restricted to a single work; work %q is not available", projectID)
		}
	}
	if projectID == "" {
		return project.Project{}, toolErr("project_id is required; call linetta_list_works first")
	}
	p, err := d.Projects.Get(ctx, projectID)
	if err != nil {
		return project.Project{}, toolErr("work %q not found", projectID)
	}
	return p, nil
}

// requireNode resolves a node and verifies it belongs to an allowed work, so a
// node id from another work cannot be read through a restricted server.
func (d ToolDeps) requireNode(ctx context.Context, nodeID string) (node.Node, *mcp.CallToolResult) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return node.Node{}, toolErr("node_id is required; call linetta_get_outline to find one")
	}
	n, err := d.Nodes.Get(ctx, nodeID)
	if err != nil {
		return node.Node{}, toolErr("scene or outline node %q not found", nodeID)
	}
	if _, errResult := d.requireProject(ctx, n.ProjectID); errResult != nil {
		return node.Node{}, errResult
	}
	return n, nil
}

// requireNodeInProject is requireNode plus the courtesy of accepting an
// optional project_id (#73): agents habitually pass it because most tools
// want one, and rejecting the extra key failed otherwise-correct calls. When
// given, a mismatch is refused loudly — it means the agent is confused about
// which work it is in, and silence there becomes a wrong-work edit later.
func (d ToolDeps) requireNodeInProject(ctx context.Context, nodeID, projectID string) (node.Node, *mcp.CallToolResult) {
	n, errResult := d.requireNode(ctx, nodeID)
	if errResult != nil {
		return node.Node{}, errResult
	}
	projectID = strings.TrimSpace(projectID)
	if projectID != "" && projectID != n.ProjectID {
		return node.Node{}, toolErr(
			"node %q belongs to work %q, not %q; drop project_id or fix it", n.ID, n.ProjectID, projectID)
	}
	return n, nil
}

// requireSkillTarget resolves the scope, work and name that one skill tool
// call addresses, funnelling the work through requireProject so a skill in
// another work cannot be reached through a restricted server.
//
// It deliberately does NOT apply the pin to a writer-scoped skill: reading a
// global skill is harmless, and only the WRITE path has a reason to refuse
// one — see editSkill, where that refusal lives.
//
// The name is validated here rather than left to the store so the refusal is
// a sentence about slugs that an agent can act on, instead of a path error.
func (d ToolDeps) requireSkillTarget(ctx context.Context, scopeArg, projectID, name string) (agentskills.Scope, string, string, *mcp.CallToolResult) {
	scope, err := agentskills.ParseScope(scopeArg)
	if err != nil {
		return "", "", "", toolErr("%v", err)
	}
	projectID = strings.TrimSpace(projectID)
	switch scope {
	case agentskills.ScopeWork:
		p, errResult := d.requireProject(ctx, projectID)
		if errResult != nil {
			return "", "", "", errResult
		}
		// requireProject fills in the pinned work when the caller omitted
		// one, so take the id it resolved rather than the raw input.
		projectID = p.ID
	case agentskills.ScopeWriter:
		if projectID != "" {
			return "", "", "", toolErr(
				"a %q skill is global and belongs to no work; drop project_id, or use scope %q for a skill about one work",
				agentskills.ScopeWriter, agentskills.ScopeWork)
		}
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", "", toolErr(`name is required: the skill's name, a lowercase slug such as "fight-scenes"`)
	}
	if !agentskills.ValidName(name) {
		return "", "", "", toolErr(
			`skill name %q is not usable: use lowercase letters, digits and hyphens only, `+
				`starting and ending with a letter or digit (e.g. "fight-scenes")`, name)
	}
	return scope, projectID, name, nil
}

// allowedProjectID returns the restriction, or "" when every work is reachable.
//
// The built-in agent is never restricted: mcp_project_id exists so a writer
// can hand an external client one work only, and the panel's scope is
// whichever work it is open on. Tying the exemption to Source keeps it in the
// same place as the log stamp, so the two cannot drift apart.
func (d ToolDeps) allowedProjectID() string {
	if d.Settings == nil || d.Source == SourceAgent {
		return ""
	}
	return strings.TrimSpace(d.Settings.MCPProjectID())
}

func entityKindFilter(kind string) string {
	return strings.ToLower(strings.TrimSpace(kind))
}
