//go:build !mobile

package engineapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/mcphost"
	"github.com/devlikebear/linetta/engine/internal/rpc/handlers"
	"github.com/devlikebear/linetta/engine/internal/settings"
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
	tools         func(s *mcp.Server, mode string)
}

// mcpController adapts *mcphost.Host to handlers.MCPController, translating
// host errors into the sentinels the RPC layer turns into reason codes.
type mcpController struct {
	host *mcphost.Host
	set  *settings.Store
}

func setupMCP(deps mcpDeps) (*mcpController, func() error) {
	host := mcphost.New(mcphost.Deps{
		Settings: deps.settingsStore,
		Home:     deps.home,
		Tools:    deps.tools,
	})
	ctrl := &mcpController{host: host, set: deps.settingsStore}
	// Start honors the persisted mode: a writer who left MCP on finds it
	// running after a restart, and mode off binds nothing.
	if err := host.Start(context.Background()); err != nil {
		fmt.Printf("mcp: start skipped: %v\n", err)
	}
	return ctrl, host.Stop
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

func (c *mcpController) Disable(ctx context.Context) error {
	return c.host.Stop()
}

func (c *mcpController) RegenerateToken(ctx context.Context) (json.RawMessage, error) {
	token, err := c.set.RegenerateMCPToken()
	if err != nil {
		return nil, err
	}
	// The listener holds the old token in memory, so it must be cycled for the
	// new one to take effect.
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
	// The activity log lands with the tool layer (Task 2.6); until then this
	// reports an empty list rather than failing the settings pane.
	return json.Marshal([]struct{}{})
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
