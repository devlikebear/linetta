package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/devlikebear/tars/pkg/llm"
)

// An episode's review may read each scene it covers, the brief, the ledger and a
// few earlier scenes, and record what it finds. A turn that wrote many episodes
// reviews the last few rather than running for as long as the turn did.
const (
	episodeReviewMaxScenes     = 3
	episodeReviewCallsPerScene = 6
	// episodeReviewTimeout covers everything that follows a turn: settlement
	// and review share it.
	episodeReviewTimeout = 10 * time.Minute
)

// episodeReviewTools can read and can record a finding, and nothing else. The
// review judges prose it did not write; it must not be able to change it.
var episodeReviewTools = map[string]bool{
	"linetta_read_scene":        true,
	"linetta_get_story_context": true,
	"linetta_search_manuscript": true,
	"linetta_list_ledger":       true,
	"linetta_report_continuity": true,
}

// episodeReviewSystemPrompt deliberately knows nothing of the turn. The agent
// that drafted a scene reads it with its intentions filled in; this one has only
// the page, which is why it can see what the writer has been catching by hand.
const episodeReviewSystemPrompt = `You are reviewing a chapter of fiction you did not write, for its author. You have only what is on the page and the work's own record. Your reply goes nowhere: what you find reaches the author only through linetta_report_continuity.

For each scene you are given:
1. Read it with linetta_read_scene numbered=true, and read the brief with linetta_get_story_context. Look up earlier scenes when something depends on them.
2. Read it as someone who has seen only this episode, then as someone who has read everything before it. Look for:
   - timeline, fact: numbers, dates or sequences that contradict the text or the ledger. Work the arithmetic. Hold each "how long since" in the prose against the brief's ## 작중 시간: time since one event never shrinks, and grows only by what has passed. Report one that does not fit as timeline.
   - character: who knows what, and speech that does not sound like them.
   - motive: a choice no sensible person would make here; a villain who gives way with no resistance, pressure or calculation shown.
   - readability: a line compressed past understanding, or a number or term the reader cannot place.
   - plot: a promise in the ledger this scene breaks, or a setup with no reason to exist.
   - pacing: an opening with no hook, no turn in the scene, or an ending that is merely cut off.
3. Call linetta_report_continuity once per scene, source quotes copied exactly. An empty list is a good result.

Report only what you can point at in the text. Taste is not a finding. Do not suggest prose longer than a sentence.` + passLanguage

// passLanguage ends every pass prompt. The prompts are in English and the
// writer reads what a pass reports in the panel: on a real model (2026-11-09)
// the review wrote its findings in English about a Korean scene.
const passLanguage = "\n\nWrite every summary, note and suggestion in the language the scene is written in."

// noteWrittenScene remembers which scene a tool call wrote.
func noteWrittenScene(written *[]string, name, args string) {
	if written == nil || name != "linetta_write_scene" {
		return
	}
	var in struct {
		NodeID string `json:"node_id"`
	}
	if json.Unmarshal([]byte(args), &in) != nil || strings.TrimSpace(in.NodeID) == "" {
		return
	}
	if !slices.Contains(*written, in.NodeID) {
		*written = append(*written, in.NodeID)
	}
}

// noteChangedFact remembers an entry a tool call changed in a way the prose may
// not agree with: a decision, or a fact replacing another.
func noteChangedFact(changed *[]string, name, args, result string) {
	if changed == nil || name != "linetta_edit_ledger" {
		return
	}
	var in struct {
		Supersedes string `json:"supersedes"`
	}
	var out struct {
		Entry struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		} `json:"entry"`
	}
	if json.Unmarshal([]byte(args), &in) != nil || json.Unmarshal([]byte(result), &out) != nil || out.Entry.ID == "" {
		return
	}
	if (in.Supersedes != "" || out.Entry.Kind == "decision") && !slices.Contains(*changed, out.Entry.ID) {
		*changed = append(*changed, out.Entry.ID)
	}
}

// maybeEpisodeReview follows a turn with what the writer has switched on: a
// settlement of what its scenes changed, an independent read of them, and a look
// for prose a changed fact no longer agrees with. Like the self-improvement pass
// it never blocks the turn. Each runs only if this turn holds the tool it records
// with.
func (s *Service) maybeEpisodeReview(ctx context.Context, st loopState) {
	var scenes, entries []string
	if st.written != nil {
		scenes = *st.written
	}
	if st.changed != nil {
		entries = *st.changed
	}
	enabled := func(f func() bool) bool { return f != nil && f() }
	var passes []func(context.Context)
	if len(scenes) > episodeReviewMaxScenes {
		scenes = scenes[len(scenes)-episodeReviewMaxScenes:]
	}
	scenes = slices.Clone(scenes)
	// Settlement first, so the review reads a ledger that has this scene in it.
	if len(scenes) > 0 && enabled(s.deps.SettlementEnabled) && offers(st.schemas, "linetta_edit_ledger") {
		passes = append(passes, func(ctx context.Context) {
			s.settle(ctx, st, "settlement", scenes)
		})
	}
	if len(scenes) > 0 && enabled(s.deps.EpisodeReviewEnabled) && offers(st.schemas, "linetta_report_continuity") {
		passes = append(passes, func(ctx context.Context) {
			s.runPass(ctx, st, "episode review", []llm.ChatMessage{
				{Role: "system", Content: episodeReviewSystemPrompt},
				{Role: "user", Content: episodeReviewRequest(st.req.ProjectID, scenes)},
			}, episodeReviewTools, episodeReviewCallsPerScene*len(scenes))
		})
	}
	if len(entries) > 0 && enabled(s.deps.ImpactReviewEnabled) && offers(st.schemas, "linetta_find_impact") {
		if len(entries) > impactMaxEntries {
			entries = entries[len(entries)-impactMaxEntries:]
		}
		entries = slices.Clone(entries)
		passes = append(passes, func(ctx context.Context) {
			s.runPass(ctx, st, "impact review", []llm.ChatMessage{
				{Role: "system", Content: impactSystemPrompt},
				{Role: "user", Content: impactRequest(st.req.ProjectID, entries)},
			}, impactTools, impactCallsPerEntry*len(entries))
		})
	}
	if len(passes) > 0 {
		s.startPasses(ctx, st, passes)
	}
}

// startPasses runs passes in order in the background, holding the session.
func (s *Service) startPasses(ctx context.Context, st loopState, passes []func(context.Context)) {
	passCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), episodeReviewTimeout)
	id := newRunID()
	s.runs.track(id, cancel)
	if err := s.enter(); err != nil {
		s.runs.untrack(id)
		cancel()
		return
	}
	st.session.acquire()
	go func() {
		defer cancel()
		defer s.runs.untrack(id)
		defer s.leave()
		defer releaseSession(st.session)
		defer func() {
			if r := recover(); r != nil {
				logf("panic in a pass after turn %s: %v\n%s", st.runID, r, debug.Stack())
			}
		}()
		for _, pass := range passes {
			pass(passCtx)
		}
	}()
}

func episodeReviewRequest(projectID string, scenes []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Work: %s\nReview these scenes, in order:\n", projectID)
	for _, id := range scenes {
		b.WriteString("- " + id + "\n")
	}
	return b.String()
}
