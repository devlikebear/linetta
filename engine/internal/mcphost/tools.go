//go:build !mobile

package mcphost

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/manuscript"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/plot"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/snapshot"
	"github.com/devlikebear/linetta/engine/internal/storycontext"
	"github.com/devlikebear/linetta/engine/internal/storyops"
)

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

	// Write-side collaborators. Snapshots make every body change revertible;
	// EnqueueSummary keeps agent prose in the summarizer's queue; Notify tells
	// the running UI that something outside it changed the manuscript.
	Snapshots      *snapshot.Repo
	Story          *storyops.Service
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

// ChangedPayload is the body of an "mcp.changed" notification: what an external
// agent just altered, so the UI can refetch instead of showing stale text.
type ChangedPayload struct {
	ProjectID string   `json:"project_id"`
	Tool      string   `json:"tool"`
	NodeIDs   []string `json:"node_ids,omitempty"`
	BatchID   string   `json:"batch_id,omitempty"`
}

func (d ToolDeps) notifyChanged(projectID, tool string, nodeIDs []string, batchID string) {
	if d.Notify == nil {
		return
	}
	d.Notify("mcp.changed", ChangedPayload{
		ProjectID: projectID, Tool: tool, NodeIDs: nodeIDs, BatchID: batchID,
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
		d.recordActivity(ctx, tool, projectID, targetID, ok, detail)
		return res, out, err
	}
}

func (d ToolDeps) recordActivity(ctx context.Context, tool, projectID, targetID string, ok bool, detail string) {
	if d.Activity == nil {
		return
	}
	if err := d.Activity.Record(ctx, ActivityEntry{
		Tool:      tool,
		ProjectID: projectID,
		TargetID:  targetID,
		OK:        ok,
		Detail:    detail,
	}); err != nil {
		logf("activity log: %v", err)
	}
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
	restricted := ""
	if d.Settings != nil {
		restricted = strings.TrimSpace(d.Settings.MCPProjectID())
	}
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

// allowedProjectID returns the restriction, or "" when every work is reachable.
func (d ToolDeps) allowedProjectID() string {
	if d.Settings == nil {
		return ""
	}
	return strings.TrimSpace(d.Settings.MCPProjectID())
}

func entityKindFilter(kind string) string {
	return strings.ToLower(strings.TrimSpace(kind))
}
