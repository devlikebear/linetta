// Package charcount counts a manuscript's characters the way the writer chose
// for the work: with spaces or without them. Korean serial sites and contests
// set length limits on one or the other, so every screen that shows a count,
// and every length the agents aim for, follows the work's one basis.
package charcount

import (
	"encoding/json"
	"unicode"
)

const (
	// WithSpaces counts every visible character, spaces included. It is the
	// default and what a work counted before the basis was a choice.
	WithSpaces = "with_spaces"
	// WithoutSpaces leaves out whitespace.
	WithoutSpaces = "without_spaces"
)

// Valid reports whether basis is one of the two bases.
func Valid(basis string) bool {
	return basis == WithSpaces || basis == WithoutSpaces
}

// Normalize maps an unknown or empty basis to WithSpaces.
func Normalize(basis string) string {
	if basis == WithoutSpaces {
		return WithoutSpaces
	}
	return WithSpaces
}

// Text counts the runes of s under basis.
func Text(s, basis string) int {
	if Normalize(basis) == WithSpaces {
		n := 0
		for range s {
			n++
		}
		return n
	}
	n := 0
	for _, r := range s {
		if !unicode.IsSpace(r) {
			n++
		}
	}
	return n
}

// Doc walks a Tiptap doc (raw JSON) and counts the text of its text nodes
// under basis. Returns 0 for any malformed or empty input.
func Doc(rawDoc []byte, basis string) int {
	if len(rawDoc) == 0 {
		return 0
	}
	var v any
	if err := json.Unmarshal(rawDoc, &v); err != nil {
		return 0
	}
	return walk(v, Normalize(basis))
}

func walk(v any, basis string) int {
	switch t := v.(type) {
	case map[string]any:
		// A text node is {"type":"text","text":"..."}.
		if kind, _ := t["type"].(string); kind == "text" {
			if s, ok := t["text"].(string); ok {
				return Text(s, basis)
			}
			return 0
		}
		if content, ok := t["content"].([]any); ok {
			return walk(content, basis)
		}
		return 0
	case []any:
		n := 0
		for _, c := range t {
			n += walk(c, basis)
		}
		return n
	}
	return 0
}
