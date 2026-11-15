package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/devlikebear/tars/pkg/llm"
)

// The organize pass (#387): the writer's own prose, read for what the work's
// record does not hold yet — people, places, things, ideas, how two people
// stand, what the story now owes — and proposed into the suggestions inbox
// with the line each rests on. It changes nothing: every item waits for the
// writer to accept it.

// organizeCallsPerScene covers reading the scene, looking up what is already
// recorded, and one proposal batch.
const organizeCallsPerScene = 6

// organizeTools read and propose, and nothing else.
var organizeTools = map[string]bool{
	"linetta_read_scene":        true,
	"linetta_get_story_context": true,
	"linetta_list_characters":   true,
	"linetta_list_ledger":       true,
	"linetta_search_manuscript": true,
	"linetta_propose_story_ops": true,
}

// ErrOrganizeTrial: the trial is paid for out of Linetta's own key, and an
// organize pass is a request the writer makes of their own provider.
var ErrOrganizeTrial = errors.New("agent: organizing needs your own AI provider; the trial does not run it")

// ErrOrganizeUnavailable: this build's tool session has no proposal tool.
var ErrOrganizeUnavailable = errors.New("agent: the suggestions inbox is not available, so there is nowhere to propose to")

// OrganizeScene is one scene to read. LedgerSettled says the settlement pass
// already took this version into the ledger, so its foreshadowing is left to
// that.
type OrganizeScene struct {
	NodeID        string
	LedgerSettled bool
}

// OrganizeRequest is one pass over an episode's changed scenes.
type OrganizeRequest struct {
	ProjectID    string
	EpisodeLabel string
	Scenes       []OrganizeScene
	// Language is the app's, for the batch summary the inbox shows.
	Language string
}

// StartOrganize runs the organize pass in the background and calls done with
// how it ended ("done", "timeout" or "failed") once it has.
func (s *Service) StartOrganize(ctx context.Context, req OrganizeRequest, done func(ctx context.Context, outcome string)) error {
	if len(req.Scenes) == 0 {
		return nil
	}
	st, release, err := s.standalone(ctx, req.ProjectID)
	if err != nil {
		return err
	}
	defer release()
	if st.groups.trial {
		return ErrOrganizeTrial
	}
	if !offers(st.schemas, "linetta_propose_story_ops") {
		return ErrOrganizeUnavailable
	}
	s.startPasses(ctx, st, []func(context.Context){func(ctx context.Context) {
		outcome := s.runPass(ctx, st, "organize", []llm.ChatMessage{
			{Role: "system", Content: organizeSystemPrompt},
			{Role: "user", Content: organizeRequest(req)},
		}, organizeTools, organizeCallsPerScene*len(req.Scenes)+2)
		if done != nil {
			done(context.WithoutCancel(ctx), outcome)
		}
	}})
	return nil
}

const organizeSystemPrompt = `You are tidying up a work of fiction's records after its author wrote a chapter by hand. You did not write it. Your reply goes nowhere: what you find reaches the author only as proposals in their suggestions inbox, through linetta_propose_story_ops, and nothing changes until they accept it.

1. Read the work's characters and places with linetta_list_characters (include_relationships true) and its open promises with linetta_list_ledger, so you do not propose what is already there.
2. Read each scene you are given with linetta_read_scene numbered=true.
3. Propose, in ONE call to linetta_propose_story_ops with purpose "organize" and the summary you are given:
   - create_entity for a person, place, thing or idea that is named here for the first time and will plainly matter again. Not a passer-by, not a one-off object. Reuse an existing entity's id rather than proposing a near-duplicate name.
   - create_relationship for a tie the scene establishes between two entities, and change_relationship (relationship_id, the new label, node_id the scene it changes in) when a recorded tie turns into another here.
   - a ledger item (promise, gag or question) for something the story now owes the reader: a setup left open, a running joke, a question the reader will carry. A ledger item with entry_id and status paid (paid_node_id, paid_quote) when this scene pays one already recorded. Skip ledger items for scenes marked "ledger already settled".
   Every item cites its evidence: node_id, the paragraph number, and a quote copied EXACTLY from that paragraph.
4. Propose only what you can point at in the text. Fewer, surer items are better than many. Proposing nothing is a good result; then do not call the tool.` + passLanguage

func organizeRequest(req OrganizeRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Work: %s\nEpisode: %s\nSummary to use: %s · %s\nRead these scenes, in order:\n",
		req.ProjectID, req.EpisodeLabel, organizeSummaryWord(req.Language), req.EpisodeLabel)
	for _, sc := range req.Scenes {
		b.WriteString("- " + sc.NodeID)
		if sc.LedgerSettled {
			b.WriteString(" (ledger already settled)")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// organizeSummaryWord starts the batch summary, "자동 정리 · 12화", in the
// language the inbox is read in.
func organizeSummaryWord(language string) string {
	switch {
	case strings.HasPrefix(language, "en"):
		return "Organize"
	case strings.HasPrefix(language, "ja"):
		return "自動整理"
	}
	return "자동 정리"
}
