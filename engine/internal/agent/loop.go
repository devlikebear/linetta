//go:build !mobile

package agent

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/agentmemory"
	"github.com/devlikebear/linetta/engine/internal/agentskills"
	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/provider"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/tars/pkg/llm"
)

// maxIterations is the default cap on the number of tool calls EXECUTED in a
// turn — not the number of Chat round-trips. A single model response can
// legally ask for many tool calls at once, and letting each one through
// uncounted is how a runaway agent rewrites forty scenes instead of hitting
// the wall the writer is supposed to see in the activity log.
//
// The writer can move the cap in Settings (Deps.MaxToolCalls); this is what
// a turn gets when nothing says otherwise, and it matches
// settings.DefaultAgentMaxToolCalls.
//
// 48 rather than the original 24: an ordinary batch request — "summarise every
// scene" — costs a read and a write per scene, so 24 cut a twelve-scene work
// off partway through a job the writer asked for by name.
const maxIterations = 48

// wrapUpPrompt is the last thing a turn that ran into a wall is told. It is a
// user-role message because that is the only role every provider accepts
// after a tool result; %s is the wall's own description of itself.
const wrapUpPrompt = "[system notice] This turn was stopped before it finished: %s. " +
	"You cannot call any more tools in this turn. In the writer's language, say briefly " +
	"what you finished, what is still left, and that they can ask you to continue."

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
	// Selection is a passage the writer highlighted in the open scene and
	// sent along with the prompt, or "". See withSelection.
	Selection string
	// Images are pictures attached to the message. They go to the model
	// with this message only; the transcript keeps a marker (withImages).
	Images []Image
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
	images, err := checkImages(req.Images)
	if err != nil {
		return "", err
	}
	req.Images = images
	client, resolved, err := s.deps.Providers.Client(ctx, "")
	if err != nil {
		return "", err
	}
	if len(images) > 0 && provider.IsSelfExecuting(resolved.ID) {
		return "", errImagesUnsupported
	}
	// The two tool-budget switches (#99) are read ONCE, here, and the same
	// value then decides every part of the turn that has an opinion about
	// which tools exist: the server the session is built with (connectTools
	// hands this value to Deps.Register — the registrar does not read the
	// store), the label that session is cached under, and which tools the
	// prompt is allowed to name (openingMessages reads st.groups). Any second
	// reading reopens the same hole: a settings.Set landing between two
	// readings produces a turn whose prompt names a tool its own server never
	// registered, and because the label is the cache key, that turn's mistake
	// is cached for every turn after it.
	groups := resolveToolGroups(s.deps.MemoryToolsEnabled, s.deps.SkillToolsEnabled, s.deps.ReviewToolsEnabled)
	if s.deps.VisualWriteToolsDisabled != nil {
		groups.disableVisualWrites = s.deps.VisualWriteToolsDisabled()
	}
	tools, err := s.session(ctx, groups)
	if err != nil {
		return "", err
	}
	// The turn now holds a reference to the session, and owes exactly one
	// release. Handed to the goroutine below on success; dropped here on
	// every path that never gets that far, or a session the writer's next
	// flip retires would never be closed.
	handedOff := false
	defer func() {
		if handedOff {
			return
		}
		if err := tools.release(); err != nil {
			logf("agent: releasing the tool session: %v", err)
		}
	}()

	schemas, err := tools.schemas(ctx)
	if err != nil {
		return "", err
	}
	schemas = append(schemas, askWriterSchema())

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
	// to maxIterations tool calls and every round-trip between them), not one
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

	// The passage is folded into the prompt before anything sees it, so the
	// transcript row, the replay of later turns and this turn's message all
	// carry the same text: a follow-up "now shorten it" still knows what "it" is.
	req.Prompt = withImages(len(req.Images), withSelection(req.Selection, req.Prompt))
	s.runs.attach(runID, req.Images)
	if err := s.tr.appendUser(runCtx, projectID, req.NodeID, runID, req.Prompt); err != nil {
		s.runs.finish(projectID, runID)
		s.leave()
		cancel()
		return "", err
	}

	st := loopState{
		runID: runID,
		// The trimmed id — the same value runs.start and appendUser key on,
		// not req.ProjectID's raw form. Every agent.* notification carries it
		// (#95 Task 7 review round 3), so a padded id can never make the
		// panel's own equality check disagree with the engine's.
		projectID:    projectID,
		req:          req,
		client:       client,
		provider:     resolved.ID,
		model:        resolved.Model,
		session:      tools,
		schemas:      schemas,
		groups:       groups,
		language:     s.language(),
		maxToolCalls: s.maxToolCalls(),
		brief:        newBriefGate(schemas),
	}

	handedOff = true
	go func() {
		defer cancel()
		defer s.runs.finish(projectID, runID)
		defer s.leave()
		// Declared AFTER leave so it runs BEFORE it: Close waits on the wait
		// group and then retires the session it snapshotted, so a release
		// that landed after leave could let Close see a reference count that
		// is not yet zero and skip the close entirely.
		defer releaseSession(st.session)
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
		if provider.IsSelfExecuting(st.provider) {
			s.loopCLI(runCtx, st)
			return
		}
		s.loop(runCtx, st)
	}()
	return runID, nil
}

// maxToolCalls is the cap for the turn about to start.
func (s *Service) maxToolCalls() int {
	if s.deps.MaxToolCalls != nil {
		if n := s.deps.MaxToolCalls(); n > 0 {
			return n
		}
	}
	return maxIterations
}

type loopState struct {
	runID string
	// written collects the scenes this turn wrote, for the review that may
	// follow it. A pointer because the state is passed by value.
	written *[]string
	// projectID is the work this turn belongs to, trimmed. It is on every
	// notification this file emits rather than only in the transcript,
	// because a run id alone does not say whose conversation an event is —
	// and a panel that has to reconstruct that from its own bookkeeping
	// (which run it started, which it abandoned, which send is still out)
	// is reconstructing a fact the emitter already knows.
	projectID string
	req       RunRequest
	// provider is the resolved provider id ("anthropic", "openai-codex", …).
	// It names the failure in an agent.error message, the same way
	// provider.Classify names it for the settings connection test.
	provider string
	client   llm.Client
	model    string
	session  *toolSession
	schemas  []llm.ToolSchema
	// groups is the tool budget this turn was started with (#99), read once
	// in Run. session was built for it and schemas is the tool list that came
	// back from it, so a prompt built from this same value is the only way
	// the two can be guaranteed to describe the same tools.
	groups   toolGroups
	language string
	// maxToolCalls is this turn's tool-call cap, read once in Run so a
	// settings change mid-turn cannot move a wall the turn is already
	// counting towards.
	maxToolCalls int
	// brief holds a prose write back until the turn has read the story
	// brief (briefgate.go).
	brief *briefGate
}

// releaseSession drops one reference and logs a close that failed. A helper
// only so `defer` can carry it: a deferred method value would be fine, but
// the error a close returns would then go nowhere at all.
func releaseSession(ts *toolSession) {
	if err := ts.release(); err != nil {
		logf("agent: releasing the tool session: %v", err)
	}
}

// Every payload below carries project_id beside run_id, spelled the same way
// mcphost.ChangedPayload spells it. A turn outlives the RPC call that started
// it (#93), so its events keep arriving after the writer has moved to another
// work — and a panel told only the run id has to infer ownership from what it
// remembers starting, which is exactly the inference three review rounds of
// #95 kept finding a new gap in. The emitter knows the answer; it says it.

// Seq numbers the turn's notifications from 1, in the order they were emitted.
// agent.status reports the last one, so a panel that mounts mid-turn can drop
// the events its status-and-transcript read already accounts for.

type deltaPayload struct {
	RunID     string `json:"run_id"`
	ProjectID string `json:"project_id"`
	Seq       int64  `json:"seq"`
	Text      string `json:"text"`
}

type toolPayload struct {
	RunID     string `json:"run_id"`
	ProjectID string `json:"project_id"`
	Seq       int64  `json:"seq"`
	Name      string `json:"name"`
	State     string `json:"state"` // started | done | error
	Summary   string `json:"summary,omitempty"`
	BatchID   string `json:"batch_id,omitempty"`
	// SnapshotID is the pre-write snapshot a scene write or revise left
	// behind, offered by the panel the same way BatchID is — the two are
	// never both set for one call (#112).
	SnapshotID string   `json:"snapshot_id,omitempty"`
	NodeIDs    []string `json:"node_ids,omitempty"`
	// RequestID is the illustration request the call left (illustration
	// Phase 4); the panel shows its approval card under the line.
	RequestID string `json:"request_id,omitempty"`
	// Ask is the card linetta_ask_writer left for the writer to answer.
	Ask *askCard `json:"ask,omitempty"`
}

type usagePayload struct {
	Input  int `json:"input"`
	Output int `json:"output"`
}

type donePayload struct {
	RunID     string       `json:"run_id"`
	ProjectID string       `json:"project_id"`
	Seq       int64        `json:"seq"`
	Model     string       `json:"model,omitempty"`
	Usage     usagePayload `json:"usage"`
}

type errorPayload struct {
	RunID     string `json:"run_id"`
	ProjectID string `json:"project_id"`
	Seq       int64  `json:"seq"`
	Reason    string `json:"reason"`
	Message   string `json:"message"`
}

type cancelledPayload struct {
	RunID     string `json:"run_id"`
	ProjectID string `json:"project_id"`
	Seq       int64  `json:"seq"`
}

// historyCutPayload tells a panel that is already open that this turn's replay
// left more of the conversation out. A panel opened later gets the same fact
// from agent.history, where it can draw the boundary at the message itself.
type historyCutPayload struct {
	RunID     string `json:"run_id"`
	ProjectID string `json:"project_id"`
	Seq       int64  `json:"seq"`
}

// loop is the turn. It ends when the model stops asking for tools, when it
// hits the iteration cap, when the same tool fails repeatedly, or when the
// writer cancels.
func (s *Service) loop(ctx context.Context, st loopState) {
	msgs := s.openingMessages(ctx, st)

	var usage usagePayload
	toolCalls := 0
	// The tool NAMES this turn executed, in order. This is the whole of what
	// the self-review is told about the turn (see selfreview.go): names, never
	// arguments or results, so a background call the writer did not ask for
	// carries none of their manuscript.
	usedTools := make([]string, 0, st.maxToolCalls)
	st.written = &[]string{}
	lastFailedTool := ""
	repeats := 0

	for {
		clearOldToolResults(msgs)
		resp, err := st.client.Chat(ctx, msgs, llm.ChatOptions{
			Tools:   st.schemas,
			OnDelta: func(text string) { s.streamDelta(st, text) },
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
		// The message is over either way: what streamed is now a transcript
		// row, or was nothing worth one.
		s.runs.committed(st.runID)

		if len(resp.Message.ToolCalls) == 0 {
			// A turn that ends here with nothing to show for its last
			// round-trip did not finish — it stopped. Reporting it as done
			// leaves the panel with a column of tool lines and a live send
			// button, which reads as the agent having silently given up.
			if reason, message, stopped := silentStop(resp); stopped {
				s.endWithReason(ctx, st, reason, message, companion.HistoryStatusDone)
				s.maybeSelfReview(ctx, st, toolCalls, usedTools)
				s.maybeEpisodeReview(ctx, st)
				return
			}
			s.endDone(ctx, st, usage, toolCalls, usedTools)
			return
		}

		msgs = append(msgs, resp.Message)
		for i, call := range resp.Message.ToolCalls {
			if ctx.Err() != nil {
				s.endCancelled(ctx, st)
				return
			}
			// The cap bounds the TOTAL tool calls executed this turn,
			// not the number of responses that asked for them — a single
			// response can legally pack many calls into one, and this check
			// runs before every one of them, not just once per round-trip.
			if toolCalls >= st.maxToolCalls {
				s.stopAtWall(ctx, st, msgs, resp.Message.ToolCalls[i:],
					fmt.Sprintf("stopped after %d tool calls in one turn", toolCalls), toolCalls, usedTools)
				return
			}

			result := s.runTool(ctx, st, call)
			toolCalls++
			usedTools = append(usedTools, call.Name)
			if !result.IsError {
				noteWrittenScene(st.written, call.Name, call.Arguments)
			}
			msgs = append(msgs, llm.ChatMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    result.Text,
			})

			// The card waits for the writer, so there is nothing to go back to
			// the model with. Calls that came after it are not run: they were
			// chosen without the answers.
			if result.Ask != nil {
				s.endDone(ctx, st, usage, toolCalls, usedTools)
				return
			}
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
				s.stopAtWall(ctx, st, msgs, resp.Message.ToolCalls[i+1:],
					fmt.Sprintf("%s failed %d times in a row", call.Name, repeats), toolCalls, usedTools)
				return
			}
		}

		// A response that used its entire allotment does not earn another
		// tool-bearing round-trip just to be told no on its first tool call.
		if toolCalls >= st.maxToolCalls {
			s.stopAtWall(ctx, st, msgs, nil,
				fmt.Sprintf("stopped after %d tool calls in one turn", toolCalls), toolCalls, usedTools)
			return
		}
	}
}

// endDone ends a turn that finished.
func (s *Service) endDone(ctx context.Context, st loopState, usage usagePayload, toolCalls int, usedTools []string) {
	s.notify("agent.done", donePayload{RunID: st.runID, ProjectID: st.projectID, Seq: s.runs.next(st.runID), Model: st.model, Usage: usage})
	// After the reply has gone, and only for a turn that did real work: ask
	// what was learned. It runs on its own goroutine with its own context, so
	// this returns immediately and the writer's next message never waits on it.
	s.maybeSelfReview(ctx, st, toolCalls, usedTools)
	s.maybeEpisodeReview(ctx, st)
}

// streamDelta forwards reply text to the panel as it arrives, keeping a copy
// for a panel that is not there to receive it (see liveRun).
func (s *Service) streamDelta(st loopState, text string) {
	seq := s.runs.streamed(st.runID, text)
	s.notify("agent.delta", deltaPayload{RunID: st.runID, ProjectID: st.projectID, Seq: seq, Text: text})
}

// silentStop reports whether a response that asked for no tools ended the
// turn without the model actually finishing, and with which reason.
//
// Two shapes do. The provider cut the response off at its output ceiling —
// most often partway through a tool call carrying a whole scene, which leaves
// no text and no usable call. Or the response is simply empty: no text, no
// tool calls, which is what a ceiling spent entirely on reasoning looks like
// from here. A reply that was cut off after some text is still reported, since
// the writer is looking at a sentence that stops mid-way.
func silentStop(resp llm.ChatResponse) (reason, message string, stopped bool) {
	switch strings.ToLower(strings.TrimSpace(resp.StopReason)) {
	case "max_tokens", "length", "max_output_tokens":
		return rpc.ReasonAgentOutputTruncated, "the response hit the provider's output limit", true
	}
	if strings.TrimSpace(resp.Message.Content) == "" {
		return rpc.ReasonAgentEmptyReply, "the model ended the turn without a reply (stop reason: " + resp.StopReason + ")", true
	}
	return "", "", false
}

// stopAtWall ends a turn that ran into one of the loop's own limits, after
// letting the model say where it stopped. pending is whatever the last
// response asked for and the wall kept from running.
func (s *Service) stopAtWall(ctx context.Context, st loopState, msgs []llm.ChatMessage, pending []llm.ToolCall, why string, toolCalls int, usedTools []string) {
	s.wrapUp(ctx, st, msgs, pending, why)
	// The writer pressing stop during the wrap-up is still the writer
	// pressing stop.
	if ctx.Err() != nil {
		s.endCancelled(ctx, st)
		return
	}
	s.endAtWall(ctx, st, why)
	// The wall ends the turn "done" on purpose (see endAtWall): a turn that
	// got this far did work, and work is exactly what the review is for.
	s.maybeSelfReview(ctx, st, toolCalls, usedTools)
	s.maybeEpisodeReview(ctx, st)
}

// wrapUp gives a turn at the wall one last round-trip, with tool calls
// forbidden, to tell the writer how far it got. Without it the panel shows a
// column of tool lines and then the wall's notice, which promises a reply
// that says where the work stopped — and the follow-up turn, which replays
// replies but not tool rows (priorMessages), has nothing to continue from.
//
// Best effort: a wrap-up that fails changes nothing about how the turn ends.
func (s *Service) wrapUp(ctx context.Context, st loopState, msgs []llm.ChatMessage, pending []llm.ToolCall, why string) {
	// Every call the last response asked for needs a result before the
	// provider will take another message, including the ones that never ran.
	for _, call := range pending {
		msgs = append(msgs, llm.ChatMessage{
			Role:       "tool",
			ToolCallID: call.ID,
			Content:    "not run: " + why,
		})
	}
	msgs = append(msgs, llm.ChatMessage{Role: "user", Content: fmt.Sprintf(wrapUpPrompt, why)})
	clearOldToolResults(msgs)

	// The schemas stay on the request — providers reject a history holding
	// tool calls when no tools are declared — and the choice is what forbids
	// them. A call that comes back anyway is simply not run.
	resp, err := st.client.Chat(ctx, msgs, llm.ChatOptions{
		Tools:      st.schemas,
		ToolChoice: llm.ToolChoiceNone(),
		OnDelta:    func(text string) { s.streamDelta(st, text) },
	})
	// Whatever streamed is either about to become a transcript row or was cut
	// off with the call; it is no longer the reply in progress.
	defer s.runs.committed(st.runID)
	if err != nil {
		logf("wrap-up for turn %s: %v", st.runID, err)
		return
	}
	if strings.TrimSpace(resp.Message.Content) == "" {
		return
	}
	if err := s.tr.appendAssistant(context.WithoutCancel(ctx), st.req.ProjectID, st.req.NodeID, st.runID,
		resp.Message.Content, companion.HistoryStatusDone); err != nil {
		logf("transcript: %v", err)
	}
}

// openingMessages is system prompt + budgeted history + the scope-prefixed
// message. History is loaded BEFORE the user row is counted, because Run has
// already written it — replaying it here would double it.
func (s *Service) openingMessages(ctx context.Context, st loopState) []llm.ChatMessage {
	profile := agentmemory.Document{Scope: agentmemory.ScopeWriterProfile, CharsBudget: agentmemory.ScopeWriterProfile.Budget()}
	notes := agentmemory.Document{Scope: agentmemory.ScopeWorkNotes, CharsBudget: agentmemory.ScopeWorkNotes.Budget()}
	if s.deps.Memory != nil {
		profile, notes = s.deps.Memory.Memories(ctx, st.req.ProjectID)
	}
	// The tool budget is taken from the TURN, not read from the switches
	// again. Run read them once and built this turn's tool session for that
	// same value (Service.session), so the prompt below can only name tools
	// st.schemas actually contains. Re-reading here would put the flip back
	// in the gap between the two — the writer switching the skills tools on
	// mid-session would get a prompt telling the agent to record a skill with
	// a tool this turn was never offered.
	groups := st.groups
	var skills []agentskills.Skill
	// Not even asked for when the group is off: the list has no reader then
	// (systemPrompt drops the block), and a skills read is filesystem work on
	// the turn's critical path.
	if groups.skills && s.deps.Skills != nil {
		skills = s.deps.Skills.Skills(ctx, st.req.ProjectID)
	}
	msgs := []llm.ChatMessage{{Role: "system", Content: systemPrompt(st.language, profile, notes, skills, groups)}}

	prior, err := s.tr.load(ctx, st.req.ProjectID, historyLoadRows)
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
	replay, cut, moved := s.replayHistory(st.projectID, filtered)
	if cut {
		msgs[0].Content += historyCutNotice
	}
	if moved {
		s.notify("agent.history_cut", historyCutPayload{RunID: st.runID, ProjectID: st.projectID, Seq: s.runs.next(st.runID)})
	}
	msgs = append(msgs, replay...)

	scope := scopeLine(ctx, s.deps.Scope, st.req.ProjectID, st.req.NodeID)
	msg := llm.ChatMessage{Role: "user", Content: scope + "\n\n" + st.req.Prompt}
	if len(st.req.Images) > 0 {
		msg.ContentBlocks = append(msg.ContentBlocks, llm.ContentBlock{Type: "text", Text: msg.Content})
		for _, img := range st.req.Images {
			msg.ContentBlocks = append(msg.ContentBlocks, llm.ContentBlock{
				Type: "image", MediaType: img.MIME, Data: base64.StdEncoding.EncodeToString(img.Data),
			})
		}
	}
	return append(msgs, msg)
}

// runTool executes one call, telling the panel before and after. The activity
// log entry is written server-side by mcphost.record, which reads the run id
// off the request's _meta.
func (s *Service) runTool(ctx context.Context, st loopState, call llm.ToolCall) toolResult {
	s.notify("agent.tool", toolPayload{
		RunID: st.runID, ProjectID: st.projectID, Seq: s.runs.inTool(st.runID, call.Name), Name: call.Name, State: "started",
		Summary: summarize(call.Arguments),
	})

	var result toolResult
	if call.Name == askWriterTool {
		result = askWriter(call.Arguments)
	} else if refusal := st.brief.refusal(call.Name); refusal != "" {
		result = toolResult{IsError: true, Text: refusal}
	} else {
		result = st.session.call(ctx, st.runID, call.Name, call.Arguments)
		st.brief.ran(call.Name, !result.IsError)
	}

	state := "done"
	if result.IsError {
		state = "error"
	}
	s.notify("agent.tool", toolPayload{
		RunID: st.runID, ProjectID: st.projectID, Seq: s.runs.inTool(st.runID, ""), Name: call.Name, State: state,
		Summary: summarize(result.Text), BatchID: result.BatchID, SnapshotID: result.SnapshotID,
		NodeIDs: result.NodeIDs, RequestID: result.RequestID, Ask: result.Ask,
	})
	// Recorded with a context that survives the turn's own cancellation,
	// unlike the tool call above: the call already ran — and may already
	// have written the manuscript — by the time this executes, so a
	// cancelled ctx must not cost the writer the transcript row carrying the
	// undo batch id for a write that actually landed. Same reasoning as
	// markRun (transcript.go).
	if err := s.tr.appendToolEvent(context.WithoutCancel(ctx), st.req.ProjectID, st.req.NodeID, st.runID, toolEvent{
		Name: call.Name, Summary: summarize(result.Text), OK: !result.IsError,
		BatchID: result.BatchID, SnapshotID: result.SnapshotID, NodeIDs: result.NodeIDs, RequestID: result.RequestID,
		Ask: result.Ask,
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
// dozens of successful tool calls look identical to one that crashed before
// its first token, and hangs a retry button under work the writer can see in
// their manuscript.
func (s *Service) endAtWall(ctx context.Context, st loopState, message string) {
	s.endWithReason(ctx, st, rpc.ReasonAgentIterationLimit, message, companion.HistoryStatusDone)
}

func (s *Service) endCancelled(ctx context.Context, st loopState) {
	if err := s.tr.markRun(ctx, st.runID, companion.HistoryStatusCancelled); err != nil {
		logf("transcript: %v", err)
	}
	s.notify("agent.cancelled", cancelledPayload{RunID: st.runID, ProjectID: st.projectID, Seq: s.runs.next(st.runID)})
}

// endWithError maps a provider failure to a reason code. The provider's own
// body never becomes UI text — v0.8.5's lesson.
func (s *Service) endWithError(ctx context.Context, st loopState, err error) {
	if errors.Is(err, context.Canceled) || ctx.Err() != nil {
		s.endCancelled(ctx, st)
		return
	}
	reason, message := classifyProviderError(st.provider, err)
	// The panel only ever shows the reason. The cause goes on record here, in
	// Classify's form: its first line, capped, so a provider body cannot ride
	// along whole. Without it, a timeout and a dead network look the same
	// after the fact.
	logf("run %s failed (%s, model %q): %v", st.runID, reason, st.model, provider.Classify(st.provider, err))
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
// a turn that hit the iteration wall after dozens of successful tool calls
// produced real work, and marking it failed puts a retry button under a reply
// that already landed (spec §10 gives the retry button to failed turns).
// Provider errors and panics keep "failed"; walls end "done".
func (s *Service) endWithReason(ctx context.Context, st loopState, reason, message, status string) {
	if err := s.tr.markRun(ctx, st.runID, status); err != nil {
		logf("transcript: %v", err)
	}
	s.notify("agent.error", errorPayload{RunID: st.runID, ProjectID: st.projectID, Seq: s.runs.next(st.runID), Reason: reason, Message: message})
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
