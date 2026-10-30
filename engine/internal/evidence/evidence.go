// Package evidence holds the one way Linetta checks whether a model's
// quoted evidence is real: a 1-based [from,to] paragraph range is in bounds
// for the scene it is checked against, and the quote actually appears in
// those paragraphs once both sides are Unicode-normalized.
//
// Extracted from internal/continuity (story-assist Phase 2) as part of
// Phase 3 (3.A.1): the proposals package needs the same check for the
// evidence an agent cites on a proposal item, and a quote-verification rule
// that exists in two packages is a rule that can quietly drift between them.
// continuity.Validate is the original caller and keeps its exact wording —
// see validate.go — by inspecting the typed Error this package returns
// rather than depending on a generic message string.
package evidence

import (
	"fmt"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// ErrorKind says which of VerifyQuote's checks failed.
type ErrorKind int

const (
	// FromTooSmall means from < 1.
	FromTooSmall ErrorKind = iota
	// ToBeforeFrom means to < from.
	ToBeforeFrom
	// OutOfRange means to is past the last paragraph.
	OutOfRange
	// QuoteNotFound means the range was valid but the quote was not found in
	// it, once both are normalized.
	QuoteNotFound
)

// Error is VerifyQuote's failure, carrying the inputs a caller needs to
// phrase its own message — continuity's and proposals' wording differ (one
// says "observation N", the other "item N"), so this type exists to be
// switched on rather than parsed back out of a string.
type Error struct {
	Kind ErrorKind
	// From, To are the range VerifyQuote was asked to check.
	From, To int
	// N is the paragraph count VerifyQuote checked the range against.
	N int
	// Quote is the quote that was not found, set only when Kind is
	// QuoteNotFound.
	Quote string
}

func (e *Error) Error() string {
	switch e.Kind {
	case FromTooSmall:
		return fmt.Sprintf("paragraph_from %d must be at least 1", e.From)
	case ToBeforeFrom:
		return fmt.Sprintf("paragraph_to %d is before paragraph_from %d", e.To, e.From)
	case OutOfRange:
		return fmt.Sprintf("paragraph %d is out of range; the scene has %d paragraphs", e.To, e.N)
	case QuoteNotFound:
		return fmt.Sprintf("quote %q not found in paragraphs %d-%d", e.Quote, e.From, e.To)
	default:
		return "invalid evidence"
	}
}

// VerifyQuote checks a 1-based [from,to] paragraph range against paragraphs,
// and — when quote is non-empty after trimming — that quote appears
// (normalized) somewhere in that range. A range error is reported before any
// quote check: an out-of-bounds range makes a quote check meaningless.
//
// An empty (or whitespace-only) quote is not itself an error here — callers
// that require a quote check for that separately, at the point they also
// decide whether to run this range check at all. continuity's observation
// path always calls VerifyQuote so a bad range is still reported even when
// the quote is missing; continuity's per-ref path only calls it once it has
// already confirmed the quote is present, matching each caller's original
// behavior.
func VerifyQuote(paragraphs []string, from, to int, quote string) error {
	n := len(paragraphs)
	switch {
	case from < 1:
		return &Error{Kind: FromTooSmall, From: from, To: to, N: n}
	case to < from:
		return &Error{Kind: ToBeforeFrom, From: from, To: to, N: n}
	case to > n:
		return &Error{Kind: OutOfRange, From: from, To: to, N: n}
	}
	quote = strings.TrimSpace(quote)
	if quote == "" {
		return nil
	}
	haystack := strings.Join(paragraphs[from-1:to], "\n")
	if !ContainsNormalized(haystack, quote) {
		return &Error{Kind: QuoteNotFound, From: from, To: to, N: n, Quote: quote}
	}
	return nil
}

// ContainsNormalized reports whether quote appears in haystack once both are
// NFC-normalized and their whitespace runs are folded to a single space —
// so a quote that differs from the manuscript only in composed-vs-decomposed
// Hangul or in how many spaces sit between two words still passes, while a
// quote with different WORDS does not.
func ContainsNormalized(haystack, quote string) bool {
	return strings.Contains(Normalize(haystack), Normalize(quote))
}

// Normalize NFC-normalizes s and folds its whitespace runs to single spaces,
// the comparison both ContainsNormalized sides go through.
func Normalize(s string) string {
	return strings.Join(strings.Fields(norm.NFC.String(s)), " ")
}
