// Package charcount counts a manuscript's characters the way the writer chose
// for the work: with spaces or without them. Korean serial sites and contests
// set length limits on one or the other, so every screen that shows a count,
// and every length the agents aim for, follows the work's one basis.
package charcount

import (
	"encoding/json"
	"strconv"
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

// ValidEpisodeRange reports whether an episode length rule holds together:
// the bounds are 0 (none) or positive, the minimum is not past the maximum,
// and the target is positive and within both. A work and a saved preset are
// held to the same rule.
func ValidEpisodeRange(min, target, max int) bool {
	switch {
	case min < 0 || max < 0 || target <= 0:
		return false
	case max > 0 && min > max:
		return false
	case target < min:
		return false
	case max > 0 && target > max:
		return false
	}
	return true
}

// Episode length states, the same four the desktop gauge shows
// (apps/desktop/src/lib/episodeLength.ts).
const (
	// LengthShort is an episode under the work's minimum.
	LengthShort = "short"
	// LengthOK is an episode within the bounds the work sets.
	LengthOK = "ok"
	// LengthLong is an episode over the work's maximum.
	LengthLong = "long"
	// LengthProgress is an episode in a work with no minimum: it is only on
	// its way to the target, and nothing says it is too short.
	LengthProgress = "progress"
)

// EpisodeLength is where an episode of some length stands against the
// work's rule.
type EpisodeLength struct {
	State string `json:"state"`
	// Delta is how far under the minimum (negative) or over the maximum
	// (positive) the episode is; 0 otherwise.
	Delta int `json:"delta,omitempty"`
}

// JudgeEpisode judges an episode of chars characters. Reaching a bound
// exactly is within it. episodeLength.test.ts holds the same cases, so the
// gauge and the agent's warnings agree.
func JudgeEpisode(chars, min, max int) EpisodeLength {
	if max > 0 && chars > max {
		return EpisodeLength{State: LengthLong, Delta: chars - max}
	}
	if min > 0 {
		if chars < min {
			return EpisodeLength{State: LengthShort, Delta: chars - min}
		}
		return EpisodeLength{State: LengthOK}
	}
	return EpisodeLength{State: LengthProgress}
}

// Group writes n with thousands separators ("5,300"), the way briefs and
// warnings show lengths.
func Group(n int) string {
	sign := ""
	if n < 0 {
		sign, n = "-", -n
	}
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return sign + s
}
