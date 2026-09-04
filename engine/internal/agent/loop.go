//go:build !mobile

package agent

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/provider"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/tars/pkg/llm"
)

// maxIterations caps the number of tool calls EXECUTED in a turn — not the
// number of Chat round-trips. A single model response can legally ask for
// many tool calls at once, and letting each one through uncounted is how a
// runaway agent rewrites forty scenes instead of hitting the wall the writer
// is supposed to see in the activity log.
const maxIterations = 24

// maxRepeatedToolErrors ends a turn when the same TOOL — by name alone,
// regardless of the exact error text — fails this many times in a row. Real
// tool errors normally carry changing detail (a version conflict counts up:
// "expected 7, got 8", then "8, got 9"), so keying the wall on the literal
// error text let a genuinely stuck loop run forever, since every attempt
// looked like a "new" failure. A tool error is normally the model's to
// recover from — it re-reads and retries — but a model that has failed the
// same tool three times running is not learning, and a fourth attempt spends
// the writer's money to prove it.
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

	// The cancel func is registered BEFORE enter, and that order is the whole
	// point. Close sets closed, then takes a SNAPSHOT of the cancel map, then
	// waits on the wait group — so a run that entered the group before
	// registering its cancel would be invisible to that snapshot, never
	// cancelled at all, and Close would block for the run's entire turn (up
	// to 24 tool calls and every round-trip between them), not one
	// round-trip. Registering first leaves only the harmless ordering: a
	// Close landing in the gap misses this run's cancel in its snapshot, but
	// enter below then sees closed and refuses, so no turn ever starts.
	if err := s.runs.start(projectID, runID, cancel); err != nil {
		cancel()
		if errors.Is(err, ErrBusy) {
			return "", &rpc.ReasonError{Reason: rpc.ReasonAgentBusy, Err: err}
		}
		return "", err
	}

	// Registered under the same lock session() uses for its closed check: a
	// Close racing this Run either finishes first (Run then already failed
	// at session, above, or fails here) or is guaranteed to wait for the turn
	// enter admits.
	if err := s.enter(); err != nil {
		s.runs.finish(projectID, runID)
		cancel()
		return "", err
	}

	if err := s.tr.appendUser(runCtx, projectID, req.NodeID, runID, req.Prompt); err != nil {
		s.runs.finish(projectID, runID)
		s.leave()
		cancel()
		return "", err
	}

	st := loopState{
		runID:    runID,
		req:      req,
		client:   client,
		provider: resolved.ID,
		model:    resolved.Model,
		session:  tools,
		schemas:  schemas,
		language: s.language(),
	}

	go func() {
		defer cancel()
		defer s.runs.finish(projectID, runID)
		defer s.leave()
		// The highest-risk goroutine in the feature: nothing here may take
		// the whole engine process down with it. A panic ends this one turn
		// with an agent.error instead.
		defer func() {
			if r := recover(); r != nil {
				logf("panic in turn %s: %v\n%s", runID, r, debug.Stack())
				s.endWithReason(runCtx, st, rpc.ReasonAgentInternalError,
					fmt.Sprintf("internal error: %v", r), companion.HistoryStatusFailed)
			}
		}()
		s.loop(runCtx, st)
	}()
	return runID, nil
}

type loopState struct {
	runID string
	req   RunRequest
	// provider is the resolved provider id ("anthropic", "openai-codex", …).
	// It names the failure in an agent.error message, the same way
	// provider.Classify names it for the settings connection test.
	provider string
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
	toolCalls := 0
	lastFailedTool := ""
	repeats := 0

	for {
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
			// Written with a context that survives the turn's own
			// cancellation, the same as the tool-event write below: the
			// reply already left the model by the time this runs.
			if err := s.tr.appendAssistant(context.WithoutCancel(ctx), st.req.ProjectID, st.req.NodeID, st.runID,
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
			// maxIterations bounds the TOTAL tool calls executed this turn,
			// not the number of responses that asked for them — a single
			// response can legally pack many calls into one, and this check
			// runs before every one of them, not just once per round-trip.
			if toolCalls >= maxIterations {
				s.endAtWall(ctx, st, fmt.Sprintf("stopped after %d tool calls in one turn", toolCalls))
				return
			}

			result := s.runTool(ctx, st, call)
			toolCalls++
			msgs = append(msgs, llm.ChatMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    result.Text,
			})

			if !result.IsError {
				lastFailedTool, repeats = "", 0
				continue
			}
			if call.Name == lastFailedTool {
				repeats++
			} else {
				lastFailedTool, repeats = call.Name, 1
			}
			if repeats >= maxRepeatedToolErrors {
				s.endAtWall(ctx, st, fmt.Sprintf("%s failed %d times in a row", call.Name, repeats))
				return
			}
		}

		// A response that used its entire allotment does not earn another
		// round-trip just to be told no on its first tool call.
		if toolCalls >= maxIterations {
			s.endAtWall(ctx, st, fmt.Sprintf("stopped after %d tool calls in one turn", toolCalls))
			return
		}
	}
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
	// Drop this turn's own user row: it is delivered below, with its scope
	// line. Filtered by run id rather than assumed position — HistoryRepo.List
	// sorts user rows before assistant rows at equal timestamps, so under a
	// frozen or coarse-grained clock (tests; Windows) this turn's row is not
	// guaranteed to be the last item load returns, and dropping the wrong
	// (or no) row either repeats the prompt or reorders the prior reply.
	filtered := make([]companion.HistoryMessage, 0, len(prior))
	for _, m := range prior {
		if m.Role == "user" && m.RunID == st.runID {
			continue
		}
		filtered = append(filtered, m)
	}
	msgs = append(msgs, priorMessages(filtered)...)

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
		Summary: summarize(call.Arguments),
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
	// Recorded with a context that survives the turn's own cancellation,
	// unlike the tool call above: the call already ran — and may already
	// have written the manuscript — by the time this executes, so a
	// cancelled ctx must not cost the writer the transcript row carrying the
	// undo batch id for a write that actually landed. Same reasoning as
	// markRun (transcript.go).
	if err := s.tr.appendToolEvent(context.WithoutCancel(ctx), st.req.ProjectID, st.req.NodeID, st.runID, toolEvent{
		Name: call.Name, Summary: summarize(result.Text), OK: !result.IsError,
		BatchID: result.BatchID, NodeIDs: result.NodeIDs,
	}); err != nil {
		logf("transcript: %v", err)
	}
	return result
}

// endAtWall ends a turn that ran into one of the loop's own limits — the
// per-turn tool-call cap, or the same tool failing three times running.
//
// The rows stay "done", unlike a provider error's. The turn stopped short,
// and agent.error says so with agent_iteration_limit, but everything it did
// before the wall actually happened: the scenes are written, the tool rows
// carry live undo batch ids, and the reply the model already produced is a
// real partial answer. Stamping those rows "failed" makes a turn that landed
// twenty-four successful tool calls look identical to one that crashed before
// its first token, and hangs a retry button under work the writer can see in
// their manuscript.
func (s *Service) endAtWall(ctx context.Context, st loopState, message string) {
	s.endWithReason(ctx, st, rpc.ReasonAgentIterationLimit, message, companion.HistoryStatusDone)
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
	reason, message := classifyProviderError(st.provider, err)
	s.endWithReason(ctx, st, reason, message, companion.HistoryStatusFailed)
}

// classifyProviderError reduces a failure raised inside a turn to the reason
// code the panel translates and a message safe to sit beside it.
//
// The reason comes from provider.Classify — the same mapper the settings
// connection test uses, so an expired key reads "check your key" in both
// places. Going through it is what makes an HTTP 401 provider_auth_failed
// rather than provider_unreachable: st.client.Chat returns tars'
// *llm.ProviderError, whose Unwrap() is a nil Cause, so a bare
// errors.As(err, **rpc.ReasonError) never matches it. An error the pre-flight
// already classified passes through Classify untouched and keeps its reason.
//
// The message is rebuilt here rather than taken from Classify. Classify keeps
// the failure's first line capped at 200 characters, which stops a multi-line
// JSON error page but not a compact single-line one — and *llm.ProviderError's
// Message field IS the raw response body. So an HTTP failure is described by
// the two parts that can carry neither the writer's manuscript nor their key:
// the provider and the status.
func classifyProviderError(id string, err error) (reason, message string) {
	classified := provider.Classify(id, err)
	reason = rpc.ReasonProviderUnreachable
	var re *rpc.ReasonError
	if errors.As(classified, &re) {
		reason = re.Reason
	}
	var pe *llm.ProviderError
	if errors.As(err, &pe) && pe.StatusCode > 0 {
		return reason, fmt.Sprintf("%s: HTTP %d", id, pe.StatusCode)
	}
	return reason, classified.Error()
}

// endWithReason ends a turn with a reason code the panel renders. status is
// what every row of the run is stamped with, and it is NOT always "failed":
// a turn that hit the iteration wall after twenty-four successful tool calls
// produced real work, and marking it failed puts a retry button under a reply
// that already landed (spec §10 gives the retry button to failed turns).
// Provider errors and panics keep "failed"; walls end "done".
func (s *Service) endWithReason(ctx context.Context, st loopState, reason, message, status string) {
	if err := s.tr.markRun(ctx, st.runID, status); err != nil {
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
