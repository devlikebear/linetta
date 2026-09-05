//go:build !mobile

package agent

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/tars/pkg/llm"
)

// historyBudget caps how much of the earlier conversation is replayed, in
// CHARACTERS (runes), filled newest-first. No summarisation: a compaction
// pass that quietly rewrites what the writer said is worse than an honest
// cut, and session search (sub-project 4) is where this gets revisited.
//
// Runes, not bytes: Linetta's writers work mostly in Korean and Japanese, and
// 40,000 bytes of Hangul is about 13,300 characters — a third of the budget
// the spec asks for, so a Korean conversation would lose its earlier turns
// three times sooner than an English one. tools.go's capText and loop.go's
// summarize already count runes for the same reason.
const historyBudget = 40000

// ScopeLookup resolves the names in the scope line. An interface rather than
// the repos themselves so this package stays out of project/node — and so the
// prompt can be tested without a database.
type ScopeLookup interface {
	ProjectTitle(ctx context.Context, projectID string) string
	NodeLabel(ctx context.Context, nodeID string) string
}

// systemPrompt is deliberately short. The story brief is NOT pasted in: the
// agent fetches it with linetta_get_story_context exactly as an external
// agent does, which is the only way the tool descriptions stay honest — if
// the workflow they describe stops working, this agent notices first.
//
// There is no per-work language field in Linetta, so the manuscript rule is
// stated as "match what is already written" rather than naming a language.
func systemPrompt(lang string) string {
	return fmt.Sprintf(`You are Linetta's writing agent. You work inside the writer's own app, on their manuscript, with the writer holding the final say on every word.

Reply to the writer in %q (their app language). Write manuscript prose in the language the existing manuscript is written in — read a scene first if you are unsure.

How you work:
- Call linetta_get_story_context before drafting anything, so you write from the work's actual state rather than a guess.
- After writing or revising a scene, refresh its summary so the rest of the work stays accurate.
- Before a large rewrite you are not certain about, call linetta_create_checkpoint first so the writer can get their version back.`, lang)
}

// scopeLine prefixes the writer's message with the work and the scene the
// panel is open on. Its whole job is to stop the agent asking "which scene?"
// — anything more than that it finds out with tools.
func scopeLine(ctx context.Context, look ScopeLookup, projectID, nodeID string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[work: %s %q]", projectID, titleOr(look, ctx, projectID))
	if strings.TrimSpace(nodeID) != "" {
		fmt.Fprintf(&b, " [open scene: %s %q]", nodeID, labelOr(look, ctx, nodeID))
	}
	return b.String()
}

func titleOr(look ScopeLookup, ctx context.Context, id string) string {
	if look == nil {
		return ""
	}
	return look.ProjectTitle(ctx, id)
}

func labelOr(look ScopeLookup, ctx context.Context, id string) string {
	if look == nil {
		return ""
	}
	return look.NodeLabel(ctx, id)
}

// priorMessages replays earlier turns within the budget, newest first, then
// restores chronological order. Tool rows are skipped: they are the bulkiest
// thing in the transcript, and the model already stated their outcome in the
// reply that followed them.
func priorMessages(msgs []companion.HistoryMessage) []llm.ChatMessage {
	kept := make([]llm.ChatMessage, 0, len(msgs))
	used := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		if strings.TrimSpace(m.Content) == "" {
			continue
		}
		cost := utf8.RuneCountInString(m.Content)
		if used+cost > historyBudget {
			break
		}
		used += cost
		kept = append(kept, llm.ChatMessage{Role: m.Role, Content: m.Content})
	}
	for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
		kept[i], kept[j] = kept[j], kept[i]
	}
	return kept
}
