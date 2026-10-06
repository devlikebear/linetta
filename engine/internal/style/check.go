package style

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Rule names as they appear in a Violation.
const (
	RuleAvoidPhrase     = "avoid_phrase"
	RuleSentenceTooLong = "sentence_too_long"
)

// MaxViolations caps one report. A check that finds more says how many it
// found and returns the first MaxViolations in reading order.
const MaxViolations = 200

// excerptContext is how many characters of surrounding text an excerpt keeps
// on each side of the match.
const excerptContext = 20

// Scene is one scene's prose as the check reads it: paragraphs in order.
type Scene struct {
	NodeID     string
	Label      string
	Paragraphs []string
}

// Violation is one place a scene breaks one rule.
type Violation struct {
	NodeID string `json:"node_id"`
	Label  string `json:"label"`
	Rule   string `json:"rule"`
	// Paragraph is 1-based, counting the scene's non-empty paragraphs.
	Paragraph int `json:"paragraph"`
	// Phrase is the phrase found, for RuleAvoidPhrase.
	Phrase string `json:"phrase,omitempty"`
	// Excerpt is the offending text with a little context.
	Excerpt string `json:"excerpt"`
	// Length and Limit are set for RuleSentenceTooLong, in characters.
	Length int `json:"length,omitempty"`
	Limit  int `json:"limit,omitempty"`
}

// Report is the result of checking scenes against rules.
type Report struct {
	ScenesChecked int `json:"scenes_checked"`
	// Total counts every violation found; Violations holds at most
	// MaxViolations of them, and Truncated says when it holds fewer.
	Total      int         `json:"total"`
	Violations []Violation `json:"violations"`
	Truncated  bool        `json:"truncated"`
}

// Check holds scenes against rules. It is a pure function of its inputs: the
// same scene and rules always produce the same report.
func Check(scenes []Scene, rules Rules) Report {
	rep := Report{ScenesChecked: len(scenes), Violations: []Violation{}}
	add := func(v Violation) {
		rep.Total++
		if len(rep.Violations) < MaxViolations {
			rep.Violations = append(rep.Violations, v)
			return
		}
		rep.Truncated = true
	}
	phrases := make([]string, 0, len(rules.AvoidPhrases))
	for _, p := range rules.AvoidPhrases {
		if strings.TrimSpace(p) != "" {
			phrases = append(phrases, p)
		}
	}
	for _, sc := range scenes {
		para := 0
		for _, text := range sc.Paragraphs {
			if strings.TrimSpace(text) == "" {
				continue
			}
			para++
			for _, hit := range findPhrases(text, phrases) {
				add(Violation{
					NodeID: sc.NodeID, Label: sc.Label, Rule: RuleAvoidPhrase, Paragraph: para,
					Phrase: hit.phrase, Excerpt: excerpt(text, hit.start, hit.end),
				})
			}
			if rules.MaxSentenceChars > 0 {
				for _, s := range Sentences(text) {
					if n := utf8.RuneCountInString(s); n > rules.MaxSentenceChars {
						add(Violation{
							NodeID: sc.NodeID, Label: sc.Label, Rule: RuleSentenceTooLong, Paragraph: para,
							Excerpt: clip(s, excerptContext*3), Length: n, Limit: rules.MaxSentenceChars,
						})
					}
				}
			}
		}
	}
	return rep
}

type phraseHit struct {
	phrase     string
	start, end int // byte offsets into the paragraph
}

// findPhrases returns every occurrence of every phrase, in reading order.
// Matching ignores letter case; it does not ignore spacing, so "할 수 있었다"
// is not found in "할수있었다" — the writer lists the forms they mean.
func findPhrases(text string, phrases []string) []phraseHit {
	if len(phrases) == 0 {
		return nil
	}
	folded := foldCase(text)
	var hits []phraseHit
	for _, p := range phrases {
		needle := foldCase(p)
		for from := 0; from < len(folded); {
			i := strings.Index(folded[from:], needle)
			if i < 0 {
				break
			}
			start := from + i
			hits = append(hits, phraseHit{phrase: p, start: start, end: start + len(needle)})
			from = start + len(needle)
		}
	}
	// Reading order, so a report reads top to bottom whatever order the
	// phrases were listed in.
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0 && hits[j].start < hits[j-1].start; j-- {
			hits[j], hits[j-1] = hits[j-1], hits[j]
		}
	}
	return hits
}

// foldCase lowercases s only where doing so keeps every byte offset valid.
// A rune whose lowercase form has a different encoded length is left as it
// is, so an offset found in the folded string indexes the original too.
func foldCase(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if l := unicode.ToLower(r); utf8.RuneLen(l) == utf8.RuneLen(r) {
			b.WriteRune(l)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// excerpt returns text[start:end] with up to excerptContext characters of
// context on each side, marking a cut with an ellipsis.
func excerpt(text string, start, end int) string {
	if start < 0 || end > len(text) || start > end {
		return clip(text, excerptContext*3)
	}
	from := start
	for n := 0; n < excerptContext && from > 0; n++ {
		_, size := utf8.DecodeLastRuneInString(text[:from])
		from -= size
	}
	to := end
	for n := 0; n < excerptContext && to < len(text); n++ {
		_, size := utf8.DecodeRuneInString(text[to:])
		to += size
	}
	out := strings.TrimSpace(text[from:to])
	if from > 0 {
		out = "…" + out
	}
	if to < len(text) {
		out += "…"
	}
	return out
}

func clip(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// Sentences splits a paragraph into sentences. A sentence ends at ., !, ?, …
// or their full-width forms — with any closing quotes or brackets that follow
// — when whitespace or the end of the text comes next, and at a line break.
// "3.14" and "example.com" therefore stay whole.
func Sentences(text string) []string {
	var out []string
	flush := func(s string) {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	runes := []rune(text)
	start := 0
	for i := 0; i < len(runes); i++ {
		if runes[i] == '\n' {
			flush(string(runes[start:i]))
			start = i + 1
			continue
		}
		if !isTerminator(runes[i]) {
			continue
		}
		end := i + 1
		for end < len(runes) && (isTerminator(runes[end]) || isCloser(runes[end])) {
			end++
		}
		if end == len(runes) || unicode.IsSpace(runes[end]) {
			flush(string(runes[start:end]))
			start = end
		}
		i = end - 1
	}
	flush(string(runes[start:]))
	return out
}

func isTerminator(r rune) bool {
	switch r {
	case '.', '!', '?', '…', '。', '！', '？':
		return true
	}
	return false
}

func isCloser(r rune) bool {
	switch r {
	case '"', '\'', ')', ']', '”', '’', '」', '』', '》', '〉':
		return true
	}
	return false
}
