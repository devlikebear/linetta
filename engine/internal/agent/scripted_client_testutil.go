//go:build !mobile

package agent

import (
	"context"
	"sync"

	"github.com/devlikebear/linetta/engine/internal/provider"
	"github.com/devlikebear/tars/pkg/llm"
)

// ScriptedTurn is one step of a canned model response for tests: a tool call
// (ToolName set) or a final assistant reply (Text set, ToolName empty).
//
// This file is deliberately NOT a _test.go file, even though every symbol in
// it exists only to support engineapp's full-stack agent test. Go does not
// let one package's tests import another package's _test.go declarations, and
// scripts/validate-story-core-deps.sh confines tars/pkg/llm to this package
// and internal/provider — engineapp must not import it at all, including in
// its own tests. So the fake model has to live here, behind a plain (non-llm)
// type, and engineapp scripts a []ScriptedTurn without ever naming an llm
// type itself. Nothing in this file is called from production code; grep for
// its two exported names finds only test files.
type ScriptedTurn struct {
	ToolName string
	ToolArgs string
	Text     string
}

// NewScriptedClientFactoryForTest returns a provider.ClientFactory that plays
// turns back in order on successive Chat calls, repeating the final turn for
// any call past the end of the script — the same replay behaviour this
// package's own loop_test.go scriptedClient uses.
func NewScriptedClientFactoryForTest(turns ...ScriptedTurn) provider.ClientFactory {
	return func(llm.ProviderOptions) (llm.Client, error) {
		return &scriptedTestClient{turns: turns}, nil
	}
}

// NewFailingClientFactoryForTest returns a provider.ClientFactory that must
// never be invoked: calling it reports fail (typically a *testing.T's Fatal
// method value) instead of building a client. It asserts, e.g., that no
// client is constructed when the writer has not consented.
func NewFailingClientFactoryForTest(fail func(args ...any)) provider.ClientFactory {
	return func(llm.ProviderOptions) (llm.Client, error) {
		fail("a client must not be built here")
		return nil, nil
	}
}

type scriptedTestClient struct {
	mu    sync.Mutex
	turn  int
	turns []ScriptedTurn
}

func (c *scriptedTestClient) Ask(context.Context, string) (string, error) { return "", nil }

func (c *scriptedTestClient) Chat(ctx context.Context, _ []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
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
