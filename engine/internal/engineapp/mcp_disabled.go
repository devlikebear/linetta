//go:build mobile

package engineapp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

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
)

// Mobile builds cannot host a local server, so MCP is compiled out entirely —
// the SDK is never linked into the mobile engine.
const mcpAvailable = false

type mcpDeps struct {
	settingsStore *settings.Store
	home          string
	repos         mcpToolRepos
}

// mcpToolRepos mirrors the enabled build's shape so register() compiles
// unchanged; mobile never reads these.
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
	enqueue    func(nodeID string)
	notify     func(method string, params any)
	clock      func() int64
	db         *sql.DB
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
