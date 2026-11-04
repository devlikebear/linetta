//go:build mobile

package engineapp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/devlikebear/linetta/engine/internal/mcphost"
)

// Mobile builds cannot host a local server, so no external agent can connect:
// the settings pane and the top-bar toggle hide on this flag. The built-in
// agent still has the tools (see setupMCP below).
const mcpAvailable = false

// mcpController answers status queries with a disabled state and refuses
// mutations, so the frontend gets a clear message rather than a missing method
// if it ever calls through on a build where the pane should be hidden.
type mcpController struct{}

var errMCPUnavailable = errors.New("mcp is not available in this build")

// setupMCP hosts nothing on mobile, but the built-in agent still gets the
// whole tool set: it reaches the tools over an in-memory pipe, not a port.
func setupMCP(deps mcpDeps) (*mcpController, agentToolDeps, func() error) {
	return &mcpController{}, buildToolDeps(deps, mcphost.NewActivityRepo(deps.repos.db)), func() error { return nil }
}

func (c *mcpController) Status() (json.RawMessage, error) {
	return json.Marshal(struct {
		Running bool   `json:"running"`
		Mode    string `json:"mode"`
	}{Running: false, Mode: "off"})
}

func (c *mcpController) Enable(context.Context) error { return errMCPUnavailable }

func (c *mcpController) EnableToken(context.Context) (json.RawMessage, error) {
	return nil, errMCPUnavailable
}
func (c *mcpController) Disable(context.Context) error { return nil }

func (c *mcpController) RegenerateToken(context.Context) (json.RawMessage, error) {
	return nil, errMCPUnavailable
}

func (c *mcpController) Activity(context.Context, int) (json.RawMessage, error) {
	return json.Marshal([]struct{}{})
}
