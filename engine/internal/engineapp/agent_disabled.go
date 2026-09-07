//go:build mobile

package engineapp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/devlikebear/linetta/engine/internal/agentmemory"
	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/provider"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/storyops"
)

// The built-in agent needs the MCP tool layer, which mobile does not compile.
// The controller stays so register() is identical on every build and the
// frontend gets a clear refusal rather than a missing method.
const agentAvailable = false

type agentDeps struct {
	tools    agentToolDeps
	story    *storyops.Service
	history  *companion.HistoryRepo
	projects *project.Repo
	nodes    *node.Repo
	settings *settings.Store
	src      *provider.Source
	memory   *agentmemory.Repo
	notify   func(method string, params any)
	clock    func() int64
}

type agentController struct{}

var errAgentUnavailable = errors.New("the built-in agent is not available in this build")

func setupAgent(agentDeps) (*agentController, func() error) {
	return &agentController{}, func() error { return nil }
}

func (c *agentController) Run(context.Context, string, string, string) (string, error) {
	return "", errAgentUnavailable
}
func (c *agentController) Cancel(context.Context, string) error { return nil }
func (c *agentController) History(context.Context, string, int) (json.RawMessage, error) {
	return json.RawMessage(`[]`), nil
}
func (c *agentController) Clear(context.Context, string) error { return nil }
func (c *agentController) Undo(context.Context, string) error  { return errAgentUnavailable }
