//go:build !mobile

package engineapp

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/agent"
	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/mcphost"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/provider"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/storyops"
)

const agentAvailable = true

// agentDeps is what setupAgent needs from register().
type agentDeps struct {
	tools    mcphost.ToolDeps // the external host's deps, cloned below
	story    *storyops.Service
	history  *companion.HistoryRepo
	projects *project.Repo
	nodes    *node.Repo
	settings *settings.Store
	src      *provider.Source
	notify   func(method string, params any)
	clock    func() int64
}

// scopeLookup answers the two names the scope line carries. Failures are
// silent on purpose: a missing title must not cost the writer their turn.
type scopeLookup struct {
	projects *project.Repo
	nodes    *node.Repo
}

func (s scopeLookup) ProjectTitle(ctx context.Context, id string) string {
	p, err := s.projects.Get(ctx, id)
	if err != nil {
		return ""
	}
	return p.Title
}

func (s scopeLookup) NodeLabel(ctx context.Context, id string) string {
	if id == "" {
		return ""
	}
	n, err := s.nodes.Get(ctx, id)
	if err != nil {
		return ""
	}
	if n.Title != "" {
		return n.Title
	}
	return n.Label
}

// agentController adapts *agent.Service to handlers.AgentController.
type agentController struct {
	svc *agent.Service
}

func setupAgent(deps agentDeps) (*agentController, func() error) {
	// The agent's own tool deps: same repos, three deliberate differences.
	//
	//  - Source marks every row it writes in the activity log, and exempts it
	//    from the mcp_project_id restriction meant for external clients.
	//  - Its own limiter: sharing the external one lets a busy Claude Desktop
	//    session starve the panel, and vice versa.
	//  - Its own storyops service: undo batches live in memory on the
	//    service, so this is what stops the panel's undo button reverting an
	//    external agent's batch.
	tools := deps.tools
	tools.Source = mcphost.SourceAgent
	tools.Limiter = mcphost.NewLimiter()
	tools.Story = deps.story

	svc := agent.New(agent.Deps{
		Providers: deps.src,
		History:   deps.history,
		Scope:     scopeLookup{projects: deps.projects, nodes: deps.nodes},
		Register: func(s *mcp.Server) {
			// Always full: an agent that cannot write the manuscript has no
			// reason to exist. settings.MCPMode governs external clients only.
			tools.Register(s, settings.MCPModeFull)
		},
		Notify:   deps.notify,
		Language: deps.settings.Language,
		Undo: func(ctx context.Context, batchID string) error {
			return deps.story.UndoApply(ctx, batchID, deps.clock)
		},
		Clock: deps.clock,
	})
	return &agentController{svc: svc}, svc.Close
}

func (c *agentController) Run(ctx context.Context, projectID, nodeID, prompt string) (string, error) {
	return c.svc.Run(ctx, agent.RunRequest{ProjectID: projectID, NodeID: nodeID, Prompt: prompt})
}

func (c *agentController) Cancel(_ context.Context, runID string) error {
	return c.svc.Cancel(runID)
}

func (c *agentController) History(ctx context.Context, projectID string, limit int) (json.RawMessage, error) {
	msgs, err := c.svc.History(ctx, projectID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]agentHistoryRow, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, agentHistoryRow{
			ID: m.ID, ProjectID: m.ProjectID, NodeID: m.NodeID, NodeLabel: m.NodeLabel,
			RunID: m.RunID, Role: m.Role, Status: m.Status, Content: m.Content, CreatedAt: m.CreatedAt,
		})
	}
	return json.Marshal(out)
}

// agentHistoryRow is the panel's shape. companion.HistoryMessage has no JSON
// tags — it is an internal row — so it is projected rather than marshalled.
type agentHistoryRow struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	NodeID    string `json:"node_id,omitempty"`
	NodeLabel string `json:"node_label,omitempty"`
	RunID     string `json:"run_id,omitempty"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	Content   string `json:"content"`
	CreatedAt int64  `json:"created_at"`
}

func (c *agentController) Clear(ctx context.Context, projectID string) error {
	return c.svc.Clear(ctx, projectID)
}

func (c *agentController) Undo(ctx context.Context, batchID string) error {
	return c.svc.Undo(ctx, batchID)
}
