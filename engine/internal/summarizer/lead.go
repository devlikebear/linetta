package summarizer

import "strings"

// leadSummaryMaxRunes is how much of a scene the deterministic lead keeps.
// Long enough to carry the opening beat, short enough that a story brief made
// of several of them stays readable.
const leadSummaryMaxRunes = 200

// sentenceEnders close a sentence in the languages Linetta ships in. The
// full-width forms matter: Korean and Japanese prose uses them.
const sentenceEnders = ".?!。？！…"

// minLeadRunes is how far into the budget a sentence break has to fall before
// the lead cuts there. Below it the lead would be a fragment, so the cut
// falls back to a word boundary near the budget instead.
const minLeadRunes = 60

// leadSummary is the summary Linetta writes when no better one exists.
//
// It is not a summary and does not pretend to be: it is the opening of the
// scene, cut at a sentence if one lands in range. That is the honest thing a
// writing tool can produce without a language model, and it matters because
// summaries feed the story brief — an empty summary section costs an agent
// its context, while a lead at least says what the scene opens on.
//
// An agent that writes a real summary through linetta_write_summary replaces
// this; the summarizer will not overwrite it, because summarizeOneDepth skips
// a node whose summary already matches its content version.
func leadSummary(plain string) string {
	text := strings.Join(strings.Fields(plain), " ")
	runes := []rune(text)
	if len(runes) <= leadSummaryMaxRunes {
		return text
	}

	window := runes[:leadSummaryMaxRunes]
	if cut := lastSentenceEnd(window); cut >= minLeadRunes {
		return strings.TrimSpace(string(window[:cut]))
	}
	if cut := strings.LastIndex(string(window), " "); cut > 0 {
		return strings.TrimSpace(string([]rune(string(window)[:cut]))) + "…"
	}
	return strings.TrimSpace(string(window)) + "…"
}

// lastSentenceEnd returns the index just past the final sentence terminator in
// window, or -1 when there is none.
func lastSentenceEnd(window []rune) int {
	for i := len(window) - 1; i >= 0; i-- {
		if strings.ContainsRune(sentenceEnders, window[i]) {
			return i + 1
		}
	}
	return -1
}
