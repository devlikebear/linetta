//go:build !mobile

package engineapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/devlikebear/linetta/engine/internal/mcphost"
	"github.com/devlikebear/linetta/engine/internal/rpc/handlers"
	"github.com/devlikebear/linetta/engine/internal/settings"
)

// MCP ships on desktop and on the Mac App Store. It is deliberately NOT gated
// on `mas` the way git sync is: once the companion is removed, MCP is the only
// AI path a MAS build has. Mobile cannot host a server, so it gets the
// disabled twin.
const mcpAvailable = true

// mcpController adapts *mcphost.Host to handlers.MCPController, translating
// host errors into the sentinels the RPC layer turns into reason codes.
type mcpController struct {
	host     *mcphost.Host
	set      *settings.Store
	activity *mcphost.ActivityRepo
}

// setupMCP builds the external HTTP host and returns the tool deps it was
// built from, so the agent's server is wired from the same place rather than
// from a second copy of this list.
func setupMCP(deps mcpDeps) (*mcpController, agentToolDeps, func() error) {
	activity := mcphost.NewActivityRepo(deps.repos.db)
	tools := buildToolDeps(deps, activity)
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
