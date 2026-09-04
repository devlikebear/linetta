//go:build !mobile

package engineapp

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/agent"
	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/mcphost"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/provider"
	"github.com/devlikebear/linetta/engine/internal/rpc"
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
	// tools is the composed ToolDeps the agent registers on its own server —
	// kept here (unexported, same package) purely so agent_wiring_test.go can
	// confirm the three deliberate differences from the external host's deps
	// (Source, Limiter, Story) actually landed. None of the five agent.*
	// methods surfaces them on the wire when no provider is configured, so
	// without this a regression that dropped one of the three would compile,
	// pass every other test, and only show up as a writer's agent starving
	// against — or reverting — an external client's work.
	tools mcphost.ToolDeps
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
	return &agentController{svc: svc, tools: tools}, svc.Close
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

// Undo reverts a structural batch the agent applied. A batch that has aged
// out of storyops' in-memory undo window is not the writer's mistake — it is
// the ordinary result of a restart or a few more turns — so it gets its own
// reason code rather than surfacing storyops' English sentence verbatim.
func (c *agentController) Undo(ctx context.Context, batchID string) error {
	if err := c.svc.Undo(ctx, batchID); err != nil {
		if errors.Is(err, storyops.ErrUndoBatchNotFound) {
			return &rpc.ReasonError{Reason: rpc.ReasonAgentUndoUnavailable, Err: err}
		}
		return err
	}
	return nil
}

// SetProviderFactoryForTest replaces the client factory so a test can drive
// the loop without a network. Test-only: production wires llm.NewProvider in
// provider.NewSource.
func (a *App) SetProviderFactoryForTest(f provider.ClientFactory) {
	a.providerSrc.WithFactory(f)
}

// notificationLog records what the engine emitted. The agent's contract is
// largely a notification contract — a run id comes back immediately and
// everything after it arrives as agent.* — so a test that cannot see the
// notifications can only assert on side effects, and would pass on an engine
// that did the work but told the panel nothing.
type notificationLog struct {
	mu     sync.Mutex
	events []string
}

func (n *notificationLog) add(method string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.events = append(n.events, method)
}

func (n *notificationLog) saw(method string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, e := range n.events {
		if e == method {
			return true
		}
	}
	return false
}

// CaptureNotificationsForTest routes notifications into a log. It REPLACES
// the current notifier rather than chaining: in a test the notifier is the
// stdio default, which nothing is reading, and a getter on rpc.Server exists
// only to serve a chain nobody needs.
func (a *App) CaptureNotificationsForTest() *notificationLog {
	log := &notificationLog{}
	a.SetNotifier(func(method string, _ json.RawMessage) { log.add(method) })
	return log
}
