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
//
// It reports how the pass ended: passDone when the model stopped of its own
// accord, passTimeout when the pass ran out of time, round trips or tool calls,
// passFailed when the provider failed it.
func (s *Service) runPass(ctx context.Context, st loopState, name string, msgs []llm.ChatMessage, allowed map[string]bool, maxCalls int) string {
	if provider.IsSelfExecuting(st.provider) {
		return s.cliPass(ctx, st, name, msgs, allowed, maxCalls)
	}
	return s.boundedPass(ctx, st, name, msgs, allowed, maxCalls)
}

// How a pass ended. The strings are the ones scene_settlements keeps.
const (
	passDone    = "done"
	passTimeout = "timeout"
	passFailed  = "failed"
)

// passTurn answers a pass's tool calls up to its budget. It goes to the session
// directly rather than through runTool: nothing here belongs to the turn the
// writer read, so it sends no agent.tool events and carries no run id.
type passTurn struct {
	ctx      context.Context
	st       loopState
	maxCalls int

	mu    sync.Mutex
	calls int
	// capped says a call was turned away because the budget was spent.
	capped bool
}

func (t *passTurn) call(name string, arguments json.RawMessage) *mcp.CallToolResult {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.calls >= t.maxCalls {
		t.capped = true
		return toolText(true, "not run: this pass has used all its tool calls. Stop here.")
	}
	t.calls++
	result := t.st.session.call(t.ctx, "", name, strings.TrimSpace(string(arguments)))
	return toolText(result.IsError, result.Text)
}

// cliPass is boundedPass for a self-executing provider. The bridge offers the
// allowed tools and nothing else, so the model cannot name its way past them.
func (s *Service) cliPass(ctx context.Context, st loopState, name string, msgs []llm.ChatMessage, allowed map[string]bool, maxCalls int) string {
	turn := &passTurn{ctx: ctx, st: st, maxCalls: maxCalls}
	bridge, err := startCLIBridge(turn, onlyTools(st.schemas, allowed))
	if err != nil {
		logf("%s after turn %s: %v", name, st.runID, err)
		return passFailed
	}
	defer bridge.close()
	resp, err := st.passer().Chat(ctx, msgs, llm.ChatOptions{
		OnDelta:              func(string) {},
		ClaudeCodeMCPServers: []llm.ClaudeCodeMCPServer{bridge.server()},
		ClaudeCodeHarness:    &llm.ClaudeCodeHarnessOptions{MaxTurns: maxCalls + passTurnsOver},
	})
	s.recordUsage(ctx, st, usageKind(name), st.passerModel(), resp.Usage)
	if err != nil {
		// As in boundedPass: the writer is not waiting on this, so it is a log
		// line, never an agent.error under a turn they have moved on from.
		logf("%s after turn %s: %v", name, st.runID, err)
	}
	turn.mu.Lock()
	capped := turn.capped
	turn.mu.Unlock()
	switch {
	case ctx.Err() != nil || capped:
		return passTimeout
	case err != nil:
		return passFailed
	}
	return passDone
}

// StartSampleCheck compares scenes in the background, with no conversation and
// no brief: what the writer asked for is a reader who has seen only these
// scenes. It is a pass with no turn before it, so it sets up what a turn would.
func (s *Service) StartSampleCheck(ctx context.Context, projectID, request string, maxCalls int) error {
	projectID = strings.TrimSpace(projectID)
	if s.runs.status(projectID).Running {
		return ErrTurnRunning
	}
	client, resolved, err := s.deps.Providers.Client(ctx, "")
	if err != nil {
		return err
	}
	groups := resolveToolGroups(s.deps.MemoryToolsEnabled, s.deps.SkillToolsEnabled, s.deps.ReviewToolsEnabled).
		withTrial(s.deps.TrialToolsEnabled)
	tools, err := s.session(ctx, groups)
	if err != nil {
		return err
	}
	defer func() {
		if err := tools.release(); err != nil {
			logf("agent: releasing the tool session: %v", err)
		}
	}()
	schemas, err := tools.schemas(ctx)
	if err != nil {
		return err
	}
	st := loopState{
		runID: newRunID(), projectID: projectID, req: RunRequest{ProjectID: projectID},
		provider: resolved.ID, client: client, model: resolved.Model, session: tools, schemas: schemas, groups: groups,
	}
	s.startPasses(ctx, st, []func(context.Context){func(ctx context.Context) {
		s.runPass(ctx, st, "sample check", []llm.ChatMessage{
			{Role: "system", Content: sampleSystemPrompt},
			{Role: "user", Content: request},
		}, sampleTools, maxCalls)
	}})
	return nil
}
