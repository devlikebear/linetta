// Package blueprint generates suggested EpisodeBlueprint values for a work,
// combining any partial values the user has already typed with Canon memory
// for in-universe context. Calls an LLM when OPENAI_API_KEY is set; otherwise
// falls back to a deterministic template-based generator so the wizard still
// works in dev environments without API credentials.
package blueprint

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"

	"github.com/devlikebear/linetta/internal/llm"
	"github.com/devlikebear/linetta/internal/memory"
	"github.com/devlikebear/linetta/internal/work"
)

// Input bundles everything the suggester needs from the surrounding context.
type Input struct {
	Work      work.Work
	Episode   work.Episode
	Partial   work.EpisodeBlueprint // user-typed values; empty fields will be filled
	Canon     []memory.Item         // current Canon-status memory items for the work
	PriorMSS  []string              // optional: prior episode premises for continuity flavor
}

// Suggestion is the suggester's reply. Each field is what the suggester thinks
// the blueprint should be; the caller decides whether to overwrite empty user
// values only, or replace the whole form.
type Suggestion struct {
	Premise        string `json:"premise"`
	Theme          string `json:"theme"`
	Situation      string `json:"situation"`
	MustInclude    string `json:"must_include"`
	MustAvoid      string `json:"must_avoid"`
	StructureNotes string `json:"structure_notes"`
	Source         string `json:"source"` // "llm" or "fallback"
}

// Suggest returns a Suggestion. If an LLM client is available the suggestion
// is generated from a structured prompt; otherwise a deterministic templater
// produces something usable from Canon items + simple heuristics.
func Suggest(ctx context.Context, in Input) (Suggestion, error) {
	client, err := llm.NewFromEnv()
	if err != nil {
		if !errors.Is(err, llm.ErrNoAPIKey) {
			return Suggestion{}, err
		}
		s := fallback(in)
		s.Source = "fallback"
		return s, nil
	}
	suggestion, err := llmSuggest(ctx, client, in)
	if err != nil {
		// On LLM failure (timeout, network, malformed reply), degrade
		// gracefully to fallback rather than propagating to the user — the
		// wizard is a nice-to-have, not a hard dependency.
		s := fallback(in)
		s.Source = "fallback"
		return s, nil
	}
	suggestion.Source = "llm"
	return suggestion, nil
}

func llmSuggest(ctx context.Context, client *llm.Client, in Input) (Suggestion, error) {
	sys := `You are a creative writing assistant for serial fiction.
Given a work's premise + Canon memory + any partial blueprint the writer has typed, produce a coherent EpisodeBlueprint.
Reply ONLY with JSON of this exact shape:
{
  "premise":         "1-2 sentence episode-specific premise",
  "theme":           "one thematic phrase",
  "situation":       "physical/temporal setup",
  "must_include":    "concrete elements the episode must contain (comma-separated)",
  "must_avoid":      "concrete elements to keep out",
  "structure_notes": "free-form structural guidance"
}
Rules:
- Preserve every non-empty value from the writer's partial input EXACTLY in your reply for those fields.
- Fill empty fields so the whole blueprint reads coherently.
- Reference Canon people/places/objects by name when natural.
- Keep tone consistent with the work's genre and premise.
- Korean writers may submit Korean text — respond in the same language the partial input or work premise uses.`

	canonLines := canonContext(in.Canon, 24)
	userInput := strings.Builder{}
	fmt.Fprintf(&userInput, "Work title: %s\n", in.Work.Title)
	if in.Work.Genre != "" {
		fmt.Fprintf(&userInput, "Genre: %s\n", in.Work.Genre)
	}
	if in.Work.Premise != "" {
		fmt.Fprintf(&userInput, "Work premise: %s\n", in.Work.Premise)
	}
	if in.Episode.Title != "" {
		fmt.Fprintf(&userInput, "Episode title: %s\n", in.Episode.Title)
	}
	if canonLines != "" {
		fmt.Fprintf(&userInput, "\nCanon memory (current):\n%s\n", canonLines)
	}
	if len(in.PriorMSS) > 0 {
		fmt.Fprintf(&userInput, "\nPrior episode premises (recency-ordered):\n")
		for i, m := range in.PriorMSS {
			fmt.Fprintf(&userInput, "  %d. %s\n", i+1, m)
		}
	}
	fmt.Fprintf(&userInput, "\nPartial blueprint (preserve non-empty fields):\n")
	fmt.Fprintf(&userInput, "  premise:         %q\n", in.Partial.Premise)
	fmt.Fprintf(&userInput, "  theme:           %q\n", in.Partial.Theme)
	fmt.Fprintf(&userInput, "  situation:       %q\n", in.Partial.Situation)
	fmt.Fprintf(&userInput, "  must_include:    %q\n", in.Partial.MustInclude)
	fmt.Fprintf(&userInput, "  must_avoid:      %q\n", in.Partial.MustAvoid)
	fmt.Fprintf(&userInput, "  structure_notes: %q\n", in.Partial.StructureNotes)

	messages := []llm.Message{
		{Role: "system", Content: sys},
		{Role: "user", Content: userInput.String()},
	}
	temperature := 0.85
	if allFieldsEmpty(in.Partial) && len(in.Canon) == 0 {
		// No constraint at all → push the model to be more inventive.
		temperature = 1.0
	}
	var out Suggestion
	if err := client.ChatJSON(ctx, messages, temperature, &out); err != nil {
		return Suggestion{}, err
	}
	// Safety: if the model dropped a non-empty user field, restore it.
	out = mergePreserving(in.Partial, out)
	return out, nil
}

// fallback returns a usable Suggestion without calling any LLM. It draws
// concrete nouns (character / location / object Canon kinds) and combines them
// with a small set of theme/structure templates.
func fallback(in Input) Suggestion {
	rng := rand.New(rand.NewSource(int64(simpleSeed(in.Work.ID, in.Episode.ID))))

	people := canonByKind(in.Canon, memory.KindCharacter)
	places := canonByKind(in.Canon, memory.KindWorldFact)
	objects := canonByKind(in.Canon, memory.KindPlotThread)

	pick := func(items []memory.Item, fallback string) string {
		if len(items) == 0 {
			return fallback
		}
		return items[rng.Intn(len(items))].Title
	}
	person := pick(people, "the protagonist")
	place := pick(places, "the old harbor")
	object := pick(objects, "an unmarked envelope")

	themes := []string{"inheritance", "memory and listening", "small betrayals", "quiet resilience", "the cost of curiosity", "what stays after change"}
	situations := []string{"Late autumn, after a sudden rain.", "Pre-dawn, before anyone else is awake.", "Evening, just as the lights come on.", "Midday, in the middle of a routine errand."}
	structures := []string{"Open with a sensory anomaly. Push the protagonist toward a small decision. Close on an image the next episode can pick up.", "Cold open with dialogue. Reveal the situation through action, not exposition. End on a question."}

	s := Suggestion{
		Premise:        fmt.Sprintf("%s finds %s near %s, and a single decision tilts the day.", person, object, place),
		Theme:          themes[rng.Intn(len(themes))],
		Situation:      situations[rng.Intn(len(situations))],
		MustInclude:    fmt.Sprintf("%s, %s, a moment of silence", person, object),
		MustAvoid:      "On-the-nose backstory, info-dump exposition.",
		StructureNotes: structures[rng.Intn(len(structures))],
	}
	return mergePreserving(in.Partial, s)
}

func canonContext(items []memory.Item, max int) string {
	if len(items) == 0 {
		return ""
	}
	if len(items) > max {
		items = items[:max]
	}
	var b strings.Builder
	for _, it := range items {
		fmt.Fprintf(&b, "  - [%s] %s", it.Kind, it.Title)
		if it.Body != "" {
			body := strings.TrimSpace(it.Body)
			if len(body) > 140 {
				body = body[:140] + "…"
			}
			fmt.Fprintf(&b, " — %s", body)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func canonByKind(items []memory.Item, kind memory.Kind) []memory.Item {
	var out []memory.Item
	for _, it := range items {
		if it.Kind == kind {
			out = append(out, it)
		}
	}
	return out
}

func allFieldsEmpty(b work.EpisodeBlueprint) bool {
	return strings.TrimSpace(b.Premise) == "" &&
		strings.TrimSpace(b.Theme) == "" &&
		strings.TrimSpace(b.Situation) == "" &&
		strings.TrimSpace(b.MustInclude) == "" &&
		strings.TrimSpace(b.MustAvoid) == "" &&
		strings.TrimSpace(b.StructureNotes) == ""
}

func mergePreserving(user work.EpisodeBlueprint, gen Suggestion) Suggestion {
	out := gen
	if v := strings.TrimSpace(user.Premise); v != "" {
		out.Premise = user.Premise
	}
	if v := strings.TrimSpace(user.Theme); v != "" {
		out.Theme = user.Theme
	}
	if v := strings.TrimSpace(user.Situation); v != "" {
		out.Situation = user.Situation
	}
	if v := strings.TrimSpace(user.MustInclude); v != "" {
		out.MustInclude = user.MustInclude
	}
	if v := strings.TrimSpace(user.MustAvoid); v != "" {
		out.MustAvoid = user.MustAvoid
	}
	if v := strings.TrimSpace(user.StructureNotes); v != "" {
		out.StructureNotes = user.StructureNotes
	}
	return out
}

func simpleSeed(parts ...string) uint64 {
	var seed uint64 = 1469598103934665603 // FNV offset
	for _, p := range parts {
		for _, c := range []byte(p) {
			seed ^= uint64(c)
			seed *= 1099511628211 // FNV prime
		}
	}
	return seed
}
