//go:build mobile

package engineapp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/devlikebear/linetta/engine/internal/settings"
)

// Mobile builds cannot host a local server, so MCP is compiled out entirely —
// the SDK is never linked into the mobile engine.
const mcpAvailable = false

type mcpDeps struct {
	settingsStore *settings.Store
	home          string
	tools         any
}

// mcpController answers status queries with a disabled state and refuses
// mutations, so the frontend gets a clear message rather than a missing method
// if it ever calls through on a build where the pane should be hidden.
type mcpController struct{}

var errMCPUnavailable = errors.New("mcp is not available in this build")

func setupMCP(mcpDeps) (*mcpController, func() error) {
	return &mcpController{}, func() error { return nil }
}

func (c *mcpController) Status() (json.RawMessage, error) {
	return json.Marshal(struct {
		Running bool   `json:"running"`
		Mode    string `json:"mode"`
	}{Running: false, Mode: "off"})
}

func (c *mcpController) Enable(context.Context) error  { return errMCPUnavailable }
func (c *mcpController) Disable(context.Context) error { return nil }

func (c *mcpController) RegenerateToken(context.Context) (json.RawMessage, error) {
	return nil, errMCPUnavailable
}

func (c *mcpController) Activity(context.Context, int) (json.RawMessage, error) {
	return json.Marshal([]struct{}{})
}
