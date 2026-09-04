//go:build !mobile

// Package agenttest is test-only scaffolding: a scripted tars/pkg/llm.Client
// for driving the built-in agent's loop without a network. It is a separate
// package from internal/agent — not a file inside it — precisely so nothing
// but a test can ever import it: scripts/validate-story-core-deps.sh confines
// tars/pkg/llm to internal/provider, internal/agent, and this package, and
// production code has no reason to import a package named "agenttest". CI's
// `go list -deps ./cmd/...` check (see the gate script) confirms it never
// reaches a shipped binary's dependency graph.
//
// This exists because Go does not let one package's tests import another
// package's _test.go declarations, and engineapp's full-stack agent test
// (which drives a REAL App, REAL MCP tools and REAL repos, faking only the
// model) needs a concrete tars/pkg/llm.Client — but engineapp itself must
// never import tars/pkg/llm, not even from a _test.go file.
package agenttest

import (
	"context"
	"sync"

	"github.com/devlikebear/linetta/engine/internal/provider"
	"github.com/devlikebear/tars/pkg/llm"
)

// ScriptedTurn is one step of a canned model response: a tool call (ToolName
// set) or a final assistant reply (Text set, ToolName empty).
type ScriptedTurn struct {
	ToolName string
	ToolArgs string
	Text     string
}

// NewScriptedClientFactory returns a provider.ClientFactory that plays turns
// back in order on successive Chat calls, repeating the final turn for any
// call past the end of the script — the same replay behaviour internal/agent's
// own loop_test.go scriptedClient uses.
func NewScriptedClientFactory(turns ...ScriptedTurn) provider.ClientFactory {
	return func(llm.ProviderOptions) (llm.Client, error) {
		return &scriptedClient{turns: turns}, nil
	}
}

// NewFailingClientFactory returns a provider.ClientFactory that must never be
// invoked: calling it reports fail (typically a *testing.T's Fatal method
// value) instead of building a client. It asserts, e.g., that no client is
// constructed when the writer has not consented.
func NewFailingClientFactory(fail func(args ...any)) provider.ClientFactory {
	return func(llm.ProviderOptions) (llm.Client, error) {
		fail("a client must not be built here")
		return nil, nil
	}
}

type scriptedClient struct {
	mu    sync.Mutex
	turn  int
	turns []ScriptedTurn
}

func (c *scriptedClient) Ask(context.Context, string) (string, error) { return "", nil }

func (c *scriptedClient) Chat(ctx context.Context, _ []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return llm.ChatResponse{}, err
	}
	i := c.turn
	if i >= len(c.turns) {
		i = len(c.turns) - 1
	}
	c.turn++
	t := c.turns[i]
	msg := llm.ChatMessage{Role: "assistant", Content: t.Text}
	if t.ToolName != "" {
		msg.ToolCalls = []llm.ToolCall{{ID: "c" + t.ToolName, Name: t.ToolName, Arguments: t.ToolArgs}}
	}
	if opts.OnDelta != nil && msg.Content != "" {
		opts.OnDelta(msg.Content)
	}
	return llm.ChatResponse{Message: msg}, nil
}
