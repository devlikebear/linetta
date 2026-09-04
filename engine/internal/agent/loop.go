//go:build !mobile

package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/tars/pkg/llm"
)

// maxIterations caps tool calls per turn. A runaway agent should hit a wall
// the writer can see in the activity log, not rewrite forty scenes.
const maxIterations = 24

// maxRepeatedToolErrors ends a turn when the same tool returns the same error
// this many times running. A tool error is normally the model's to recover
// from — it re-reads and retries — but a model that has failed identically
// three times is not learning, and a fourth attempt spends the writer's money
// to prove it.
const maxRepeatedToolErrors = 3

// RunRequest is one message from the panel.
type RunRequest struct {
	ProjectID string
	NodeID    string
	Prompt    string
}

// Run starts a turn and returns its id immediately; progress arrives as
// agent.* notifications. Everything that can be refused synchronously — no
// consent, no credential, another turn already running — is refused here, so
// the panel gets an error it can render instead of a notification it has to
// correlate.
func (s *Service) Run(ctx context.Context, req RunRequest) (string, error) {
	projectID := strings.TrimSpace(req.ProjectID)
	if projectID == "" {
		return "", &rpc.ReasonError{
			Reason: rpc.ReasonProjectNotFound,
			Err:    errors.New("agent: project_id is required"),
		}
	}
	client, resolved, err := s.deps.Providers.Client("")
	if err != nil {
		return "", err
	}
	tools, err := s.session(ctx)
	if err != nil {
		return "", err
	}
	schemas, err := tools.schemas(ctx)
	if err != nil {
		return "", err
	}

	runID := newRunID()
	// Deliberately NOT derived from the caller's ctx: that context belongs to
	// one JSON-RPC call and is cancelled the moment Run returns, which would
	// kill the turn before its first token.
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	if err := s.runs.start(projectID, runID, cancel); err != nil {
		cancel()
		if errors.Is(err, ErrBusy) {
			return "", &rpc.ReasonError{Reason: rpc.ReasonAgentBusy, Err: err}
		}
		return "", err
	}

	if err := s.tr.appendUser(runCtx, projectID, req.NodeID, runID, req.Prompt); err != nil {
		s.runs.finish(projectID, runID)
		cancel()
		return "", err
	}

	go func() {
		defer cancel()
		defer s.runs.finish(projectID, runID)
		s.loop(runCtx, loopState{
			runID:    runID,
			req:      req,
			client:   client,
			model:    resolved.Model,
			session:  tools,
			schemas:  schemas,
			language: s.language(),
		})
	}()
	return runID, nil
}

type loopState struct {
	runID    string
	req      RunRequest
	client   llm.Client
	model    string
	session  *toolSession
	schemas  []llm.ToolSchema
	language string
}

type deltaPayload struct {
	RunID string `json:"run_id"`
	Text  string `json:"text"`
}

type toolPayload struct {
	RunID   string   `json:"run_id"`
	Name    string   `json:"name"`
	State   string   `json:"state"` // started | done | error
	Summary string   `json:"summary,omitempty"`
	BatchID string   `json:"batch_id,omitempty"`
	NodeIDs []string `json:"node_ids,omitempty"`
}

type usagePayload struct {
	Input  int `json:"input"`
	Output int `json:"output"`
}

type donePayload struct {
	RunID string       `json:"run_id"`
	Model string       `json:"model,omitempty"`
	Usage usagePayload `json:"usage"`
}

type errorPayload struct {
	RunID   string `json:"run_id"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type cancelledPayload struct {
	RunID string `json:"run_id"`
}

// loop is the turn. It ends when the model stops asking for tools, when it
// hits the iteration cap, when the same tool fails repeatedly, or when the
// writer cancels.
func (s *Service) loop(ctx context.Context, st loopState) {
	msgs := s.openingMessages(ctx, st)

	var usage usagePayload
	lastToolError := ""
	repeats := 0

	for iter := 0; iter < maxIterations; iter++ {
		resp, err := st.client.Chat(ctx, msgs, llm.ChatOptions{
			Tools:   st.schemas,
			OnDelta: func(text string) { s.notify("agent.delta", deltaPayload{st.runID, text}) },
		})
		if err != nil {
			s.endWithError(ctx, st, err)
			return
		}
		usage.Input += resp.Usage.InputTokens
		usage.Output += resp.Usage.OutputTokens

		if text := strings.TrimSpace(resp.Message.Content); text != "" {
			if err := s.tr.appendAssistant(ctx, st.req.ProjectID, st.req.NodeID, st.runID,
				resp.Message.Content, companion.HistoryStatusDone); err != nil {
				logf("transcript: %v", err)
			}
		}

		if len(resp.Message.ToolCalls) == 0 {
			s.notify("agent.done", donePayload{RunID: st.runID, Model: st.model, Usage: usage})
			return
		}

		msgs = append(msgs, resp.Message)
		for _, call := range resp.Message.ToolCalls {
			if ctx.Err() != nil {
				s.endCancelled(ctx, st)
				return
			}
			result := s.runTool(ctx, st, call)
			msgs = append(msgs, llm.ChatMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    result.Text,
			})

			if !result.IsError {
				lastToolError, repeats = "", 0
				continue
			}
			signature := call.Name + "\x00" + result.Text
			if signature == lastToolError {
				repeats++
			} else {
				lastToolError, repeats = signature, 1
			}
			if repeats >= maxRepeatedToolErrors {
				s.endWithReason(ctx, st, rpc.ReasonAgentIterationLimit,
					fmt.Sprintf("%s failed the same way %d times running", call.Name, repeats))
				return
			}
		}
	}

	s.endWithReason(ctx, st, rpc.ReasonAgentIterationLimit,
		fmt.Sprintf("stopped after %d tool calls in one turn", maxIterations))
}

// openingMessages is system prompt + budgeted history + the scope-prefixed
// message. History is loaded BEFORE the user row is counted, because Run has
// already written it — replaying it here would double it.
func (s *Service) openingMessages(ctx context.Context, st loopState) []llm.ChatMessage {
	msgs := []llm.ChatMessage{{Role: "system", Content: systemPrompt(st.language)}}

	prior, err := s.tr.load(ctx, st.req.ProjectID, 200)
	if err != nil {
		logf("history: %v", err)
	}
	// Drop the row Run just appended: it is delivered below, with its scope line.
	if n := len(prior); n > 0 && prior[n-1].RunID == st.runID && prior[n-1].Role == "user" {
		prior = prior[:n-1]
	}
	msgs = append(msgs, priorMessages(prior)...)

	scope := scopeLine(ctx, s.deps.Scope, st.req.ProjectID, st.req.NodeID)
	return append(msgs, llm.ChatMessage{
		Role:    "user",
		Content: scope + "\n\n" + st.req.Prompt,
	})
}

// runTool executes one call, telling the panel before and after. The activity
// log entry is written server-side by mcphost.record, which reads the run id
// off the request's _meta.
func (s *Service) runTool(ctx context.Context, st loopState, call llm.ToolCall) toolResult {
	s.notify("agent.tool", toolPayload{
		RunID: st.runID, Name: call.Name, State: "started",
		Summary: summarizeArgs(call.Arguments),
	})

	result := st.session.call(ctx, st.runID, call.Name, call.Arguments)

	state := "done"
	if result.IsError {
		state = "error"
	}
	s.notify("agent.tool", toolPayload{
		RunID: st.runID, Name: call.Name, State: state,
		Summary: summarize(result.Text), BatchID: result.BatchID, NodeIDs: result.NodeIDs,
	})
	if err := s.tr.appendToolEvent(ctx, st.req.ProjectID, st.req.NodeID, st.runID, toolEvent{
		Name: call.Name, Summary: summarize(result.Text), OK: !result.IsError,
		BatchID: result.BatchID, NodeIDs: result.NodeIDs,
	}); err != nil {
		logf("transcript: %v", err)
	}
	return result
}

func (s *Service) endCancelled(ctx context.Context, st loopState) {
	if err := s.tr.markRun(ctx, st.runID, companion.HistoryStatusCancelled); err != nil {
		logf("transcript: %v", err)
	}
	s.notify("agent.cancelled", cancelledPayload{st.runID})
}

// endWithError maps a provider failure to a reason code. The provider's own
// body never becomes UI text — v0.8.5's lesson.
func (s *Service) endWithError(ctx context.Context, st loopState, err error) {
	if errors.Is(err, context.Canceled) || ctx.Err() != nil {
		s.endCancelled(ctx, st)
		return
	}
	reason := rpc.ReasonProviderUnreachable
	var re *rpc.ReasonError
	if errors.As(err, &re) {
		reason = re.Reason
	}
	s.endWithReason(ctx, st, reason, err.Error())
}

func (s *Service) endWithReason(ctx context.Context, st loopState, reason, message string) {
	if err := s.tr.markRun(ctx, st.runID, companion.HistoryStatusFailed); err != nil {
		logf("transcript: %v", err)
	}
	s.notify("agent.error", errorPayload{RunID: st.runID, Reason: reason, Message: message})
}

func summarize(s string) string {
	const max = 160
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

func summarizeArgs(args string) string { return summarize(args) }
