// Package agentmemory owns the two curated documents every agent working on a
// Linetta manuscript reads: a global writer profile and per-work notes.
//
// It must never import tars/pkg/llm. mcphost and storycontext import this
// package, and scripts/validate-story-core-deps.sh forbids a model client in
// their dependency graph.
package agentmemory

import (
	"errors"
	"fmt"
	"regexp"
	"unicode"
)

// Sentinel causes. The caller turns each into a tool error the agent reads, so
// each one has to say what to change.
var (
	ErrInvisible = errors.New("agentmemory: invisible character")
	ErrControl   = errors.New("agentmemory: control character")
	ErrDelimiter = errors.New("agentmemory: markdown heading")
)

// blockDelimiter documents the common case of how the injected block
// separates its sections: an ATX heading opened with "## ". Memory content
// that opens its own heading could claim the block ended and the rest is
// something else. headingPattern below is the check actually used — it also
// catches the ATX forms blockDelimiter's plain substring match would miss:
// 0-3 leading spaces, "#" through "######", and a heading that is the last
// line of a block ending in "\r\n" rather than "\n". storycontext/render.go
// only ever emits an unindented "## ", so today none of that is reachable
// from this app's own renderer — but the boundary should hold on its own
// terms, not on what happens to call it.
const blockDelimiter = "\n## "

// headingPattern matches a CommonMark ATX heading opener at the start of a
// line. "(?m)" makes "^" match at the start of the text and after every
// "\n"; a "\r" immediately before that "\n" is left as a trailing character
// on the previous line, so "\r\n" endings are handled without special-casing.
var headingPattern = regexp.MustCompile(`(?m)^[ ]{0,3}#{1,6}(?:[ \t]|$)`)

// Runes that render as nothing in ordinary fonts but do not fall under
// unicode.Cf (Go's "format" category), so the general Cf check below misses
// them. Hangul has three filler jamo used to hold a syllable-block position
// empty; Khmer has two "inherent vowel" signs that are invisible modifiers
// rather than visible vowels.
const (
	hangulChoseongFiller  = '\u115F'
	hangulJungseongFiller = '\u1160'
	hangulFiller          = '\u3164'
	khmerVowelInherentAq  = '\u17B4'
	khmerVowelInherentAa  = '\u17B5'
)

const zeroWidthJoiner = '\u200D'

// Screen refuses text that must not reach a prompt.
//
// It screens for characters a writer cannot see while reviewing their own
// memory but a model still reads: zero-width characters, bidi controls,
// Unicode tag characters, variation selectors, and the format category
// generally — plus a short list of characters outside that category which
// are just as invisible in practice (see the const block above).
//
// The zero-width joiner is a deliberate, narrow exception: U+200D is what
// makes family and profession emoji ("👩‍⚕️") a single glyph instead of three,
// and writers describing their own preferences legitimately paste those. A
// ZWJ is only accepted when it directly joins two runes that look like
// emoji; a ZWJ anywhere else — in particular between two ordinary letters,
// which is exactly the smuggling shape — is still refused. See isEmojiJoin.
//
// It deliberately does NOT match phrases like "ignore previous instructions".
// Linetta's users write novels: a thriller contains that sentence honestly,
// and a note like "민준은 이전 지시를 무시하는 인물" is ordinary character
// description. A phrase filter would fire on exactly the material this app
// exists to hold, and a rephrasing walks past it anyway. What stands in its
// place is containment — see the frame in agent/prompt.go and
// storycontext/render.go, which says the block is the writer's notes about the
// writing and does not change what the tools do.
func Screen(text string) error {
	if headingPattern.MatchString(text) {
		return fmt.Errorf("%w: a memory line may not start a markdown heading (\"#\" through \"######\")", ErrDelimiter)
	}
	runes := []rune(text)
	for i, r := range runes {
		switch {
		case r == '\n' || r == '\t':
			// The only two control characters memory is written with.
		case unicode.IsControl(r):
			return fmt.Errorf("%w: U+%04X is not allowed in a memory", ErrControl, r)
		case r == zeroWidthJoiner:
			if isEmojiJoin(runes, i) {
				continue
			}
			return fmt.Errorf("%w: U+200D is invisible and is not allowed in a memory outside an emoji sequence", ErrInvisible)
		case isTagChar(r):
			return fmt.Errorf("%w: U+%04X is invisible and is not allowed in a memory", ErrInvisible, r)
		case r >= 0xE0100 && r <= 0xE01EF:
			// Variation Selector Supplement: same invisible-payload shape as
			// the tag block just above, immediately adjacent in the plane.
			return fmt.Errorf("%w: U+%04X is invisible and is not allowed in a memory", ErrInvisible, r)
		case r >= 0xFE00 && r <= 0xFE0F:
			// Base Variation Selectors. VS15/VS16 (U+FE0E/U+FE0F) select
			// text vs. emoji presentation for the preceding character — a
			// two-way, visible-effect choice, not a hidden payload channel,
			// and VS16 is what puts color emoji writers actually type. The
			// rest of the block (VS1-14) has no legitimate use in this app's
			// languages and is refused with the rest of the invisible set.
			if r == '\uFE0E' || r == '\uFE0F' {
				continue
			}
			return fmt.Errorf("%w: U+%04X is invisible and is not allowed in a memory", ErrInvisible, r)
		case r == hangulChoseongFiller || r == hangulJungseongFiller || r == hangulFiller:
			return fmt.Errorf("%w: U+%04X renders as nothing and is not allowed in a memory", ErrInvisible, r)
		case r == khmerVowelInherentAq || r == khmerVowelInherentAa:
			return fmt.Errorf("%w: U+%04X renders as nothing and is not allowed in a memory", ErrInvisible, r)
		case unicode.Is(unicode.Cf, r) || r == '­':
			return fmt.Errorf("%w: U+%04X is invisible and is not allowed in a memory", ErrInvisible, r)
		}
	}
	return nil
}

// isTagChar reports the Unicode tag block (U+E0000..U+E007F), which encodes
// ASCII invisibly.
func isTagChar(r rune) bool { return r >= 0xE0000 && r <= 0xE007F }

// isEmojiJoin reports whether the zero-width joiner at runes[i] sits directly
// between two runes that look like emoji, i.e. it is doing the job ZWJ is for
// (gluing separate emoji into one glyph) rather than hiding between two
// ordinary characters.
//
// "Looks like emoji" is approximated as General_Category = So (Symbol,
// other): Go's unicode package has no Extended_Pictographic table (the
// property Unicode itself defines for this), and nearly every standalone
// emoji base character — a person, object, hand gesture, or symbol, which is
// what appears on either side of a ZWJ in a real sequence — is So. This is
// deliberately an under-approximation, not an exact match: it misses a
// handful of Extended_Pictographic code points that fall outside So (for
// example some digits and symbols used only in keycap sequences, which do not
// use ZWJ anyway), and it does not special-case skin-tone modifiers or
// presentation selectors that may sit between the joined emoji and the ZWJ
// in a longer sequence. Every gap makes this check stricter than the real
// property, so it can be widened later without becoming less safe; it must
// not be loosened to "not a letter", which would reopen the smuggling case.
func isEmojiJoin(runes []rune, i int) bool {
	if i <= 0 || i >= len(runes)-1 {
		return false
	}
	return unicode.Is(unicode.So, runes[i-1]) && unicode.Is(unicode.So, runes[i+1])
}
