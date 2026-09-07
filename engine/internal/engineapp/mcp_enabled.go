//go:build !mobile

package engineapp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/devlikebear/linetta/engine/internal/agentmemory"
	"github.com/devlikebear/linetta/engine/internal/agentskills"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/manuscript"
	"github.com/devlikebear/linetta/engine/internal/manuscriptedit"
	"github.com/devlikebear/linetta/engine/internal/mcphost"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/plot"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/rpc/handlers"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/snapshot"
	"github.com/devlikebear/linetta/engine/internal/storycontext"
	"github.com/devlikebear/linetta/engine/internal/storyops"
)

// MCP ships on desktop and on the Mac App Store. It is deliberately NOT gated
// on `mas` the way git sync is: once the companion is removed, MCP is the only
// AI path a MAS build has. Mobile cannot host a server, so it gets the
// disabled twin.
const mcpAvailable = true

// mcpDeps is what setupMCP needs from register().
type mcpDeps struct {
	settingsStore *settings.Store
	home          string
	repos         mcpToolRepos
}

// mcpToolRepos collects what the tool layer reads from. The context builder is
// a second instance wired with fact/memory/reference sources — the builder the
// AI runner uses stays untouched so its prompts do not change.
type mcpToolRepos struct {
	projects   *project.Repo
	nodes      *node.Repo
	entities   *entity.Repo
	mentions   *mention.Repo
	facts      *fact.Repo
	plot       *plot.Builder
	manuscript *manuscript.Searcher
	context    *storycontext.ContextBuilder
	snapshots  *snapshot.Repo
	story      *storyops.Service
	msEdit     *manuscriptedit.Service
	// memory is the two curated documents linetta_edit_memory writes. The
	// same *agentmemory.Repo the companion reads them back through and the
	// settings pane edits: one row per document, one writer per row at a
	// time, so an agent's edit and a writer's textarea cannot fork.
	memory *agentmemory.Repo
	// skills are the SKILL.md documents linetta_edit_skill writes and
	// linetta_read_skill opens: a filesystem store under the Linetta home,
	// plus the version history every write lands a row in so the writer can
	// revert what an agent wrote. Both are shared with the built-in agent's
	// server, the same way memory is — one home directory, one table.
	skills       *agentskills.Store
	skillHistory *agentskills.History
	enqueue      func(nodeID string)
	notify       func(method string, params any)
	clock        func() int64
	db           *sql.DB
}

// mcpController adapts *mcphost.Host to handlers.MCPController, translating
// host errors into the sentinels the RPC layer turns into reason codes.
type mcpController struct {
	host     *mcphost.Host
	set      *settings.Store
	activity *mcphost.ActivityRepo
}

// agentToolDeps is what setupMCP hands the built-in agent to clone for its
// own in-memory server (#93). An alias rather than the type itself because
// register() is compiled on every build tag and mobile has no mcphost.
type agentToolDeps = mcphost.ToolDeps

// setupMCP builds the external HTTP host and returns the tool deps it was
// built from, so the agent's server is wired from the same place rather than
// from a second copy of this list.
func setupMCP(deps mcpDeps) (*mcpController, agentToolDeps, func() error) {
	activity := mcphost.NewActivityRepo(deps.repos.db)
	tools := mcphost.ToolDeps{
		Projects:   deps.repos.projects,
		Nodes:      deps.repos.nodes,
		Entities:   deps.repos.entities,
		Mentions:   deps.repos.mentions,
		Facts:      deps.repos.facts,
		Plot:       deps.repos.plot,
		Manuscript: deps.repos.manuscript,
		Context:    deps.repos.context,
		Settings:   deps.settingsStore,
		Activity:   activity,
		Memory:     deps.repos.memory,

		Skills:       deps.repos.skills,
		SkillHistory: deps.repos.skillHistory,

		Snapshots:      deps.repos.snapshots,
		Story:          deps.repos.story,
		ManuscriptEdit: deps.repos.msEdit,
		Limiter:        mcphost.NewLimiter(),
		EnqueueSummary: deps.repos.enqueue,
		Notify:         deps.repos.notify,
		Clock:          deps.repos.clock,
	}
	host := mcphost.New(mcphost.Deps{
		Settings: deps.settingsStore,
		Home:     deps.home,
		Tools:    tools.Register,
	})
	ctrl := &mcpController{host: host, set: deps.settingsStore, activity: activity}
	// Start honors the persisted mode: a writer who left MCP on finds it
	// running after a restart, and mode off binds nothing.
	if err := host.Start(context.Background()); err != nil {
		fmt.Printf("mcp: start skipped: %v\n", err)
	}
	return ctrl, tools, host.Stop
}

func (c *mcpController) Status() (json.RawMessage, error) {
	return json.Marshal(c.host.Status())
}

func (c *mcpController) Enable(ctx context.Context) error {
	if err := c.host.Restart(ctx); err != nil {
		return translateMCPError(err)
	}
	return nil
}

// EnableToken returns the bearer token alongside the status. Enabling mints
// the token server-side, and settings.get redacts it forever after, so this is
// the one moment the settings pane can put it into a copyable client command.
// Mirrors RegenerateToken's response shape.
func (c *mcpController) EnableToken(ctx context.Context) (json.RawMessage, error) {
	if err := c.Enable(ctx); err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Token  string         `json:"token"`
		Status mcphost.Status `json:"status"`
	}{Token: c.set.MCPToken(), Status: c.host.Status()})
}

func (c *mcpController) Disable(ctx context.Context) error {
	return c.host.Stop()
}

func (c *mcpController) RegenerateToken(ctx context.Context) (json.RawMessage, error) {
	token, err := c.set.RegenerateMCPToken()
	if err != nil {
		return nil, err
	}
	// The listener holds the old token in memory and the discovery file still
	// advertises it, so a running server must be cycled for the new token to
	// take effect everywhere.
	if c.host.Status().Running {
		if err := c.host.Restart(ctx); err != nil {
			return nil, translateMCPError(err)
		}
	}
	return json.Marshal(struct {
		Token  string         `json:"token"`
		Status mcphost.Status `json:"status"`
	}{Token: token, Status: c.host.Status()})
}

func (c *mcpController) Activity(ctx context.Context, limit int) (json.RawMessage, error) {
	if c.activity == nil {
		return json.Marshal([]mcphost.ActivityEntry{})
	}
	entries, err := c.activity.List(ctx, limit)
	if err != nil {
		return nil, err
	}
	return json.Marshal(entries)
}

func translateMCPError(err error) error {
	switch {
	case errors.Is(err, mcphost.ErrPortInUse):
		return fmt.Errorf("%w: %v", handlers.ErrMCPPortInUse, err)
	case errors.Is(err, mcphost.ErrConsentRequired):
		return fmt.Errorf("%w: %v", handlers.ErrMCPConsentRequired, err)
	default:
		return err
	}
}
