//go:build !mobile

package agent

import (
	"context"
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"github.com/devlikebear/linetta/engine/internal/agentskills"
	"github.com/devlikebear/tars/pkg/llm"
)

// selfReviewThreshold is how many EXECUTED tool calls a turn has to have made
// before it is worth asking what was learned. Eight is "the agent actually did
// something": a turn that read the story context, opened two scenes and
// rewrote one lands around there, while a question answered from the prompt
// lands at zero. Below the threshold there is no technique to record, and
// asking anyway spends the writer's money on a provider call that reliably
// answers "nothing".
const selfReviewThreshold = 8

// selfReviewMaxToolCalls caps the review at four tool calls, plus the one
// round trip after them in which the model says what it did. This is a
// janitor, not a second turn: reading one skill and rewriting it is two calls,
// and a review that wants twenty is a review that has started working on the
// manuscript again.
const selfReviewMaxToolCalls = 4

// selfReviewTimeout bounds the whole review. It exists because the review's
// context is deliberately detached from the turn's (see startSelfReview): with
// nothing else to end it, a provider that never answers would hold Close open
// forever, since Close waits on the same wait group the review joins.
const selfReviewTimeout = 3 * time.Minute

// selfReviewTools are the only tools the review may call. Not the full turn
// set: the review runs after the reply has already gone, unattended, with no
// writer reading its output — the last thing it should be able to do is decide
// the manuscript needs one more edit.
var selfReviewTools = map[string]bool{
	"linetta_read_skill": true,
	"linetta_edit_skill": true,
}

// selfReviewSystemPrompt frames the pass. It is a separate Chat, not a
// continuation of the turn: the turn's transcript is not replayed, and the
// writer's own message is not repeated here — what reaches the model is the
// list of tool NAMES the turn called and the skills that already exist.
//
// The contents of those calls are deliberately left out. A review is a
// background provider call the writer did not ask for and will not read, so
// what it sends is exactly what it needs to answer the question: "which tools,
// in which order" is enough to recognise a procedure worth recording, and it
// carries none of the manuscript.
//
// "otherwise say nothing" is load-bearing. A review that always writes
// something fills the writer's skill list with restatements of the tool
// descriptions within a week, and every one of those lines then costs every
// later turn's system prompt.
const selfReviewSystemPrompt = `You have just finished a turn of work for the writer. Your reply has already been sent; the writer is not waiting on you and will not see anything you say here.

This is a review of your own working method, not of the manuscript. Answer one question: did this turn teach you a technique worth recording as a skill, or did it show that a skill you already have is wrong?

- Worth recording: a procedure you worked out and would want to follow again — the order of steps that made a revision go well, a way of handling this work's structure, a check that caught a mistake. Skills hold methods. Facts about the story belong in the fact book, and what you learn about the writer belongs in memory; do not record either here.
- Worth correcting: a skill you followed this turn whose steps did not match what the tools actually do. Patch it now, while you know what was wrong.

If either applies, call linetta_edit_skill once. Read a skill first with linetta_read_skill if you are about to change it. Otherwise reply with nothing at all — most turns teach nothing new, and a skill list padded with restatements of the tool descriptions costs every later turn.

Do not touch the manuscript, and do not answer the writer: this pass has no reply.`

// selfReviewMarker is a sentence of the prompt above, named so a test can
// recognise a review's Chat call for what it is without hard-coding a copy of
// the wording that would then quietly stop matching when the prompt is edited.
const selfReviewMarker = "This is a review of your own working method"

// maybeSelfReview is the seam the loop calls at both ends of a turn that
// produced work — the agent.done exit and endAtWall. It decides whether a
// review is warranted and, if so, hands it to its own goroutine; it never
// blocks the turn, which by then has already sent the writer's reply.
func (s *Service) maybeSelfReview(ctx context.Context, st loopState, toolCalls int, used []string) {
	if toolCalls < selfReviewThreshold {
		return
	}
	if s.deps.SelfReviewEnabled != nil && !s.deps.SelfReviewEnabled() {
		return
	}
	if !offers(st.schemas, "linetta_edit_skill") {
		// A review that cannot write a skill has nothing to do but cost a
		// provider call. Not hypothetical: linetta_edit_skill is only
		// registered on a server built with a skills store.
		return
	}
	s.startSelfReview(ctx, st, used)
}

// startSelfReview launches the review, closing the two hazards that come with
// running anything after a turn ends.
//
// The context: Run's `defer cancel()` fires the moment loop returns, so a
// review derived from the turn's ctx would be cancelled before its first
// token. It gets context.WithoutCancel plus its own timeout — the same shape
// appendAssistant and appendToolEvent use for the same reason.
//
// The wait group: the review takes s.enter() and holds it for its whole life,
// so Close's wg.Wait() blocks until it has unwound. Without that, a review
// would go on calling the provider and writing skill files while the caller,
// having had Close return, is free to close the store underneath it — the
// hazard Service.wg's own comment describes.
//
// The registration order is Run's, and for Run's reason: the cancel func goes
// into the registry BEFORE enter, so a Close landing in the gap either takes a
// snapshot that already contains this review's cancel, or sets closed first
// and makes enter refuse. A review registered after the snapshot and admitted
// to the wait group would be invisible to cancelAll and hold Close open for
// its entire timeout.
func (s *Service) startSelfReview(ctx context.Context, st loopState, used []string) {
	reviewCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), selfReviewTimeout)
	reviewID := newRunID()
	// track, not start: the review must not claim the work. See runs.track.
	s.runs.track(reviewID, cancel)
	if err := s.enter(); err != nil {
		s.runs.untrack(reviewID)
		cancel()
		return
	}
	go func() {
		defer cancel()
		defer s.runs.untrack(reviewID)
		defer s.leave()
		// Same reasoning as the turn's own recover: a panic in a background
		// goroutine takes the engine process down with it. Unlike the turn's,
		// this one has nobody to tell — there is no agent.error for a pass the
		// writer never asked for — so it goes to the log and stops there.
		defer func() {
			if r := recover(); r != nil {
				logf("panic in self-review after turn %s: %v\n%s", st.runID, r, debug.Stack())
			}
		}()
		s.selfReview(reviewCtx, st, used)
	}()
}

// selfReview is the pass itself: one Chat with the skill tools, a small tool
// budget, and no output anywhere the writer's panel can see it.
//
// Nothing here notifies. No agent.delta (no OnDelta is passed), no agent.tool,
// no agent.done, and not one transcript row — a review that appended to the
// transcript would put a second "reply" under a turn the writer already read,
// and one that emitted agent.done would make the panel re-render a finished
// turn as if it had just ended. If the review writes a skill, linetta_edit_skill
// emits skills.changed on its own, and that notification — the one the Settings
// pane already listens to — is the entire visible footprint of this feature.
func (s *Service) selfReview(ctx context.Context, st loopState, used []string) {
	var skills []agentskills.Skill
	if s.deps.Skills != nil {
		skills = s.deps.Skills.Skills(ctx, st.req.ProjectID)
	}
	msgs := []llm.ChatMessage{
		{Role: "system", Content: selfReviewSystemPrompt},
		{Role: "user", Content: selfReviewRequest(st, used, skills)},
	}
	tools := onlySelfReviewTools(st.schemas)

	calls, rounds := 0, 0
	for {
		if ctx.Err() != nil {
			return
		}
		// A second wall, on ROUND TRIPS rather than tool calls. The tool cap
		// below cannot bound this loop on its own: a model that keeps asking
		// for a tool it was not offered gets a refusal instead of an execution,
		// so `calls` never moves and the loop would run until the review's
		// timeout — several minutes of provider calls for a pass the writer
		// never asked for. Same number, because four calls plus the round trip
		// after them is four round trips plus one either way.
		if rounds >= selfReviewMaxToolCalls+1 {
			logf("self-review after turn %s: stopped after %d round trips", st.runID, rounds)
			return
		}
		rounds++
		resp, err := st.client.Chat(ctx, msgs, llm.ChatOptions{Tools: tools})
		if err != nil {
			// The writer asked for none of this and is not waiting on it, so a
			// failed review is a log line, never an agent.error under a turn
			// they have already read and moved on from.
			logf("self-review after turn %s: %v", st.runID, err)
			return
		}
		if len(resp.Message.ToolCalls) == 0 {
			// The ordinary ending: "nothing to record" is the right answer for
			// most turns, and this is what it looks like. Whatever text came
			// back is dropped on purpose — there is nowhere for it to go.
			return
		}
		// The cap is checked here rather than only inside the inner loop, so a
		// review that has spent its whole budget gets exactly ONE more
		// round-trip — the one where it says what it did — and not a second
		// helping of tool calls.
		if calls >= selfReviewMaxToolCalls {
			logf("self-review after turn %s: stopped at %d tool calls", st.runID, calls)
			return
		}
		msgs = append(msgs, resp.Message)
		for _, call := range resp.Message.ToolCalls {
			if ctx.Err() != nil {
				return
			}
			if calls >= selfReviewMaxToolCalls {
				break
			}
			if !selfReviewTools[call.Name] {
				// The model was handed only the skill tools; a call for
				// anything else is answered rather than executed, so a review
				// can never reach the manuscript by naming a tool it was not
				// offered.
				msgs = append(msgs, llm.ChatMessage{
					Role: "tool", ToolCallID: call.ID,
					Content: fmt.Sprintf("%s is not available in this pass; only the skill tools are", call.Name),
				})
				continue
			}
			// An empty run id, not st.runID: the activity log's run id is what
			// ties a row to a turn in the panel, and this call is not part of
			// the turn the writer read. The row is still written, with
			// source=agent, so a skill that appears out of nowhere is at least
			// traceable.
			result := st.session.call(ctx, "", call.Name, call.Arguments)
			calls++
			msgs = append(msgs, llm.ChatMessage{
				Role: "tool", ToolCallID: call.ID, Content: result.Text,
			})
		}
	}
}

// selfReviewRequest is the one user message: what the turn did, and what
// skills already exist. Tool NAMES only — see selfReviewSystemPrompt.
func selfReviewRequest(st loopState, used []string, skills []agentskills.Skill) string {
	var b strings.Builder
	fmt.Fprintf(&b, "The turn you just finished called %d tools, in this order:\n", len(used))
	for _, name := range used {
		fmt.Fprintf(&b, "- %s\n", name)
	}
	fmt.Fprintf(&b, "\n[work: %s]\n", st.projectID)
	if block := skillsBlock(skills); block != "" {
		b.WriteString(block)
	} else {
		b.WriteString("\nYou have no skills recorded yet.\n")
	}
	return b.String()
}

// onlySelfReviewTools narrows the turn's schemas to the skill tools. It reuses
// the schemas the turn already fetched rather than listing the server again:
// the tool set does not change mid-session, and a second ListTools would be a
// round-trip to learn something already in hand.
func onlySelfReviewTools(schemas []llm.ToolSchema) []llm.ToolSchema {
	out := make([]llm.ToolSchema, 0, len(selfReviewTools))
	for _, sc := range schemas {
		if selfReviewTools[sc.Function.Name] {
			out = append(out, sc)
		}
	}
	return out
}

func offers(schemas []llm.ToolSchema, name string) bool {
	for _, sc := range schemas {
		if sc.Function.Name == name {
			return true
		}
	}
	return false
}
