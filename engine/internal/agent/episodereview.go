//go:build !mobile

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
	episodeReviewTimeout       = 5 * time.Minute
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
   - timeline, fact: numbers, dates or sequences that contradict the text or the ledger. Work the arithmetic.
   - character: who knows what, and speech that does not sound like them.
   - motive: a choice no sensible person would make here; a villain who gives way with no resistance, pressure or calculation shown.
   - readability: a line compressed past understanding, or a number or term the reader cannot place.
   - plot: a promise in the ledger this scene breaks, or a setup with no reason to exist.
   - pacing: an opening with no hook, no turn in the scene, or an ending that is merely cut off.
3. Call linetta_report_continuity once per scene, source quotes copied exactly. An empty list is a good result.

Report only what you can point at in the text. Taste is not a finding. Do not suggest prose longer than a sentence.`

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

// maybeEpisodeReview follows a turn that wrote prose with an independent read of
// it. Like the self-improvement pass it never blocks the turn. It runs only when
// the writer switched it on, and only if this turn holds the tool it reports with.
func (s *Service) maybeEpisodeReview(ctx context.Context, st loopState) {
	if st.written == nil || len(*st.written) == 0 {
		return
	}
	if s.deps.EpisodeReviewEnabled == nil || !s.deps.EpisodeReviewEnabled() {
		return
	}
	if !offers(st.schemas, "linetta_report_continuity") {
		return
	}
	scenes := *st.written
	if len(scenes) > episodeReviewMaxScenes {
		scenes = scenes[len(scenes)-episodeReviewMaxScenes:]
	}
	s.startEpisodeReview(ctx, st, slices.Clone(scenes))
}

func (s *Service) startEpisodeReview(ctx context.Context, st loopState, scenes []string) {
	reviewCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), episodeReviewTimeout)
	reviewID := newRunID()
	s.runs.track(reviewID, cancel)
	if err := s.enter(); err != nil {
		s.runs.untrack(reviewID)
		cancel()
		return
	}
	st.session.acquire()
	go func() {
		defer cancel()
		defer s.runs.untrack(reviewID)
		defer s.leave()
		defer releaseSession(st.session)
		defer func() {
			if r := recover(); r != nil {
				logf("panic in episode review after turn %s: %v\n%s", st.runID, r, debug.Stack())
			}
		}()
		msgs := []llm.ChatMessage{
			{Role: "system", Content: episodeReviewSystemPrompt},
			{Role: "user", Content: episodeReviewRequest(st.req.ProjectID, scenes)},
		}
		s.boundedPass(reviewCtx, st, "episode review", msgs, episodeReviewTools, episodeReviewCallsPerScene*len(scenes))
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
