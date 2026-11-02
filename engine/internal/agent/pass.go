//go:build !mobile

package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/provider"
	"github.com/devlikebear/tars/pkg/llm"
)

// passTurnsOver is how many responses a pass may spend past its tool calls.
const passTurnsOver = 4

// runPass runs an unattended exchange after a turn, on whichever kind of
// provider the turn used. boundedPass runs the tool calls a model hands back,
// which Claude Code never does: it runs its own loop, so on that provider the
// same narrowed tools have to come to it over a bridge of their own.
func (s *Service) runPass(ctx context.Context, st loopState, name string, msgs []llm.ChatMessage, allowed map[string]bool, maxCalls int) {
	if provider.IsSelfExecuting(st.provider) {
		s.cliPass(ctx, st, name, msgs, allowed, maxCalls)
		return
	}
	s.boundedPass(ctx, st, name, msgs, allowed, maxCalls)
}

// passTurn answers a pass's tool calls up to its budget. It goes to the session
// directly rather than through runTool: nothing here belongs to the turn the
// writer read, so it sends no agent.tool events and carries no run id.
type passTurn struct {
	ctx      context.Context
	st       loopState
	maxCalls int

	mu    sync.Mutex
	calls int
}

func (t *passTurn) call(name string, arguments json.RawMessage) *mcp.CallToolResult {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.calls >= t.maxCalls {
		return toolText(true, "not run: this pass has used all its tool calls. Stop here.")
	}
	t.calls++
	result := t.st.session.call(t.ctx, "", name, strings.TrimSpace(string(arguments)))
	return toolText(result.IsError, result.Text)
}

// cliPass is boundedPass for a self-executing provider. The bridge offers the
// allowed tools and nothing else, so the model cannot name its way past them.
func (s *Service) cliPass(ctx context.Context, st loopState, name string, msgs []llm.ChatMessage, allowed map[string]bool, maxCalls int) {
	turn := &passTurn{ctx: ctx, st: st, maxCalls: maxCalls}
	bridge, err := startCLIBridge(turn, onlyTools(st.schemas, allowed))
	if err != nil {
		logf("%s after turn %s: %v", name, st.runID, err)
		return
	}
	defer bridge.close()
	_, err = st.client.Chat(ctx, msgs, llm.ChatOptions{
		OnDelta:              func(string) {},
		ClaudeCodeMCPServers: []llm.ClaudeCodeMCPServer{bridge.server()},
		ClaudeCodeHarness:    &llm.ClaudeCodeHarnessOptions{MaxTurns: maxCalls + passTurnsOver},
	})
	if err != nil {
		// As in boundedPass: the writer is not waiting on this, so it is a log
		// line, never an agent.error under a turn they have moved on from.
		logf("%s after turn %s: %v", name, st.runID, err)
	}
}
