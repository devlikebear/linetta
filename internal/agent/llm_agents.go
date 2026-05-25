package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/internal/llm"
	"github.com/devlikebear/linetta/internal/memory"
	"github.com/devlikebear/linetta/internal/work"
)

// agentContext is the shared context the seven agents need. It's assembled
// once per run and passed to each agent's prompt builder.
type agentContext struct {
	Work      work.Work
	Episode   work.Episode
	Blueprint work.EpisodeBlueprint
	Canon     []memory.Item
}

// runAgentLLM invokes the LLM for a given task with a role-specific system
// prompt. Returns ("", false) if no LLM client is available or the call fails —
// the caller should fall back to the deterministic stub.
func runAgentLLM(ctx context.Context, client *llm.Client, taskID string, ac agentContext, prior map[ArtifactKind]string) (string, bool) {
	if client == nil {
		return "", false
	}
	sys, temp := agentPrompt(ArtifactKind(taskID))
	if sys == "" {
		return "", false
	}
	user := agentUserPrompt(ArtifactKind(taskID), ac, prior)
	text, err := client.ChatText(ctx, []llm.Message{
		{Role: "system", Content: sys},
		{Role: "user", Content: user},
	}, temp)
	if err != nil {
		return "", false
	}
	out := strings.TrimSpace(text)
	if out == "" {
		return "", false
	}
	return out, true
}

// agentPrompt returns the system prompt and temperature for each role.
// The Writer gets a fairly long target length; Critic/Editor are tighter.
func agentPrompt(kind ArtifactKind) (string, float64) {
	switch kind {
	case ArtifactKindMuseNotes:
		return `You are the "Muse" agent for serial fiction.
Given the writer's blueprint and Canon memory, return 5-8 bullet-point sparks: scene possibilities, sensory hooks, conflict seeds, character moments.
Reply in plain markdown — short bullets, no headers, no preamble.
Respond in the same language the blueprint/work premise uses (Korean if Korean).`, 0.9

	case ArtifactKindPlotOutline:
		return `You are the "Plot Architect" agent.
Take the writer's blueprint, the Muse notes (provided), and Canon, and produce a scene-by-scene outline of 5-8 scenes.
For each scene: one numbered line with 1-2 sentences describing what happens + the function it serves.
End with one line: "Closing image: ..." describing the final beat.
No headers, no preamble. Respond in the input language.`, 0.7

	case ArtifactKindCanonReview:
		return `You are the "Canon Keeper" agent.
Review the plot outline against the Canon memory provided.
Report concisely:
1. Conflicts: outline points that contradict any Canon item (cite by title).
2. New entities: characters/places/objects/events mentioned that aren't in Canon yet (so the writer can decide whether to canonize them).
3. Gaps: missing context that the outline assumes.
Plain markdown, no preamble. Respond in the input language. If nothing is found in a section, write "(none)".`, 0.4

	case ArtifactKindResearch:
		return `You are the "Researcher" agent.
List 5-10 items the writer should verify or research before publishing this episode. Group by:
- Factual (real-world references, dates, terminology)
- Sensory (places, textures, sounds, smells to ground the scene)
- Plausibility (logic, timing, character behavior)
Each item: one bullet, concrete and actionable. Plain markdown. Respond in the input language.`, 0.6

	case ArtifactKindDraft:
		return `You are the "Writer" agent producing a complete episode draft.
Use the blueprint, plot outline, research notes, and Canon as scaffolding — but write the actual prose.
Target length: 1500-3000 words. Use sensory detail. Show, don't tell. Avoid info-dump exposition; reveal canon through action.
Match the work's genre (provided) and language of the input.
Output the prose ONLY — no headers, no meta-comments, no "Draft:" prefix.`, 0.85

	case ArtifactKindCritique:
		return `You are the "Critic" agent.
Review the draft for: pacing, tension, character motivation, originality, dialogue quality, opening hook, closing image.
Output 5-7 specific observations, each with a concrete example pulled from the draft (a phrase or scene).
Be honest and constructive. Plain markdown bullets, no preamble. Respond in the draft's language.`, 0.6

	case ArtifactKindEditedDraft:
		return `You are the "Editor" agent.
Produce a revised version of the draft that addresses the critique.
Preserve the voice and story events. Improve sentence rhythm, transitions, dialogue cadence, and word choice.
Same length as the original (±15%). Cut redundancies. Tighten where the critique flagged.
Output the revised prose ONLY — no headers, no meta-comments.`, 0.7

	default:
		return "", 0
	}
}

func agentUserPrompt(kind ArtifactKind, ac agentContext, prior map[ArtifactKind]string) string {
	var b strings.Builder

	// Always include the basic context for every agent.
	fmt.Fprintf(&b, "Work title: %s\n", ac.Work.Title)
	if ac.Work.Genre != "" {
		fmt.Fprintf(&b, "Genre: %s\n", ac.Work.Genre)
	}
	if ac.Work.Premise != "" {
		fmt.Fprintf(&b, "Work-level premise: %s\n", ac.Work.Premise)
	}
	if ac.Episode.Title != "" {
		fmt.Fprintf(&b, "Episode title: %s\n", ac.Episode.Title)
	}

	// Blueprint matters for almost every agent.
	if kind != ArtifactKindEditedDraft && kind != ArtifactKindCritique {
		fmt.Fprintf(&b, "\nBlueprint:\n")
		fmt.Fprintf(&b, "  premise: %s\n", safeOr(ac.Blueprint.Premise, "(empty)"))
		fmt.Fprintf(&b, "  theme: %s\n", safeOr(ac.Blueprint.Theme, "(empty)"))
		fmt.Fprintf(&b, "  situation: %s\n", safeOr(ac.Blueprint.Situation, "(empty)"))
		fmt.Fprintf(&b, "  must_include: %s\n", safeOr(ac.Blueprint.MustInclude, "(empty)"))
		fmt.Fprintf(&b, "  must_avoid: %s\n", safeOr(ac.Blueprint.MustAvoid, "(empty)"))
		fmt.Fprintf(&b, "  structure_notes: %s\n", safeOr(ac.Blueprint.StructureNotes, "(empty)"))
	}

	// Canon memory: include for Muse, Architect, Canon Keeper, Writer.
	if kind == ArtifactKindMuseNotes || kind == ArtifactKindPlotOutline || kind == ArtifactKindCanonReview || kind == ArtifactKindDraft {
		if canon := compactCanon(ac.Canon, 40); canon != "" {
			fmt.Fprintf(&b, "\nCanon memory (current):\n%s", canon)
		} else {
			fmt.Fprintf(&b, "\nCanon memory: (empty — generate consistent material; treat first mentions as canonizable)\n")
		}
	}

	// Wire in prior artifacts each agent needs.
	switch kind {
	case ArtifactKindPlotOutline:
		appendPrior(&b, prior, ArtifactKindMuseNotes, "Muse notes")
	case ArtifactKindCanonReview:
		appendPrior(&b, prior, ArtifactKindPlotOutline, "Plot outline")
	case ArtifactKindResearch:
		appendPrior(&b, prior, ArtifactKindPlotOutline, "Plot outline")
		appendPrior(&b, prior, ArtifactKindCanonReview, "Canon Keeper review")
	case ArtifactKindDraft:
		appendPrior(&b, prior, ArtifactKindPlotOutline, "Plot outline")
		appendPrior(&b, prior, ArtifactKindCanonReview, "Canon Keeper review")
		appendPrior(&b, prior, ArtifactKindResearch, "Research notes")
	case ArtifactKindCritique:
		appendPrior(&b, prior, ArtifactKindDraft, "Draft")
	case ArtifactKindEditedDraft:
		appendPrior(&b, prior, ArtifactKindDraft, "Draft")
		appendPrior(&b, prior, ArtifactKindCritique, "Critique")
	default:
		// No-op for ArtifactKindMuseNotes.
	}

	return b.String()
}

func compactCanon(items []memory.Item, max int) string {
	if len(items) == 0 {
		return ""
	}
	if len(items) > max {
		items = items[:max]
	}
	var b strings.Builder
	for _, it := range items {
		fmt.Fprintf(&b, "  - [%s] %s", it.Kind, it.Title)
		body := strings.TrimSpace(it.Body)
		if body != "" {
			if len(body) > 200 {
				body = body[:200] + "…"
			}
			fmt.Fprintf(&b, " — %s", body)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func appendPrior(b *strings.Builder, prior map[ArtifactKind]string, kind ArtifactKind, label string) {
	text := strings.TrimSpace(prior[kind])
	if text == "" {
		return
	}
	fmt.Fprintf(b, "\n%s:\n%s\n", label, text)
}

func safeOr(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
