// Package ftsquery turns what a writer or an agent typed into something the
// trigram indexes can answer. Every search over Korean or Japanese text has the
// same shape: a trigram index cannot see a word under three characters, and
// most Korean names are that short, so such a query goes through LIKE instead.
package ftsquery

import (
	"strings"
	"unicode/utf8"
)

// Terms splits q into words. When nothing survives sanitizing, q itself is the
// one term, so a query of only punctuation still finds itself.
func Terms(q string) []string {
	terms := []string{}
	for _, field := range strings.Fields(q) {
		if term := sanitizeTerm(field); term != "" {
			terms = append(terms, term)
		}
	}
	if len(terms) == 0 {
		return []string{q}
	}
	return terms
}

// TrigramMatch builds the MATCH expression for terms. It reports false when any
// term is too short for the trigram index: dropping it would search for less
// than was asked.
func TrigramMatch(terms []string) (string, bool) {
	quoted := make([]string, 0, len(terms))
	for _, term := range terms {
		if utf8.RuneCountInString(term) < 3 {
			return "", false
		}
		quoted = append(quoted, `"`+strings.ReplaceAll(term, `"`, `""`)+`"`)
	}
	return strings.Join(quoted, " AND "), true
}

func sanitizeTerm(term string) string {
	term = strings.TrimSpace(term)
	term = strings.Trim(term, `"'()[]{}:,^*`)
	switch strings.ToUpper(term) {
	case "AND", "OR", "NOT":
		return ""
	default:
		return term
	}
}

// EscapeLike escapes s for a LIKE pattern written with ESCAPE '\'.
func EscapeLike(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\', '%', '_':
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// LikeSnippet cuts plain down to the first needle found and radius runes on
// each side of it. Needles are tried in order, so a whole phrase wins over its
// words.
func LikeSnippet(plain string, needles []string, radius int) string {
	lower := strings.ToLower(plain)
	start, width := 0, 0
	for _, needle := range needles {
		if i := strings.Index(lower, strings.ToLower(needle)); i >= 0 {
			start = utf8.RuneCountInString(lower[:i])
			width = utf8.RuneCountInString(needle)
			break
		}
	}
	runes := []rune(plain)
	from := max(start-radius, 0)
	to := min(start+width+radius, len(runes))
	snippet := string(runes[from:to])
	if from > 0 {
		snippet = "..." + snippet
	}
	if to < len(runes) {
		snippet += "..."
	}
	return snippet
}
