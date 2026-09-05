package agentmemory

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// The three things an agent can do to a memory. There is no read action: the
// documents are already in the prompt and in the story brief, and every edit
// returns the resulting body.
const (
	ActionAdd     = "add"
	ActionReplace = "replace"
	ActionRemove  = "remove"
)

var (
	ErrNoMatch   = errors.New("agentmemory: no line matches")
	ErrAmbiguous = errors.New("agentmemory: more than one line matches")
	ErrEmptyText = errors.New("agentmemory: text is required")
	ErrEmptyFind = errors.New("agentmemory: find is required")
	ErrBadAction = errors.New("agentmemory: unknown action")
)

// Apply performs one edit and returns the new body. It does not write: the
// caller saves, so Repo.Save stays the one place a budget is enforced against
// what actually lands.
func Apply(scope Scope, body, action, find, text string) (string, error) {
	switch action {
	case ActionAdd:
		line, err := oneLine(text)
		if err != nil {
			return "", err
		}
		return budgeted(scope, body, appendLine(body, line))
	case ActionReplace:
		line, err := oneLine(text)
		if err != nil {
			return "", err
		}
		i, err := matchLine(body, find)
		if err != nil {
			return "", err
		}
		lines := splitLines(body)
		lines[i] = line
		return budgeted(scope, body, strings.Join(lines, "\n"))
	case ActionRemove:
		i, err := matchLine(body, find)
		if err != nil {
			return "", err
		}
		lines := splitLines(body)
		return budgeted(scope, body, strings.Join(append(lines[:i:i], lines[i+1:]...), "\n"))
	}
	return "", fmt.Errorf("%w: %q; use add, replace or remove", ErrBadAction, action)
}

// oneLine screens the incoming text, collapses it to a single line, and then
// screens the collapsed result too. One memory is one line so that find
// stays unambiguous; a text with a newline in it is the agent's intent
// expressed awkwardly, not an error.
//
// Screening only the raw text is not enough, because collapsing can turn
// text that Screen would wave through into something it would have refused.
// strings.Fields (used below) splits on unicode.IsSpace and discards the
// separators entirely, so it strips leading whitespace along with internal
// runs of it. Four or more leading spaces before a "#" defeat the heading
// pattern's own 0-3-space allowance (see headingPattern's comment) — but
// once those leading spaces are gone, "#" lands at the very start of the
// stored line, which the heading pattern does match. Screening only the raw
// text would let that through; screening the collapsed result too catches it,
// because the invariant this function keeps is "what is stored has been
// screened", not "what was typed has been screened".
//
// The reverse simplification — screening ONLY the collapsed result, and
// dropping the raw screen — would look equivalent but is not: strings.Fields
// also treats "\r" as whitespace and discards it as a field separator, so a
// bare "\r" never survives into the collapsed text for Screen to see there.
// Screen's control-character check has no exemption for "\r" (only "\n" and
// "\t" are allowed through), so without the raw screen a "\r" would be
// silently swallowed by the collapse and ErrControl would never fire on it.
// Keeping both screens is what makes oneLine strictly stricter than a single
// screen of either the raw or the collapsed text alone.
//
// One consequence, called out explicitly rather than left for a reader to
// puzzle over: the raw screen alone can already refuse text whose newline
// would have collapsed away a heading marker that was never going to reach
// the stored line — see TestOneLineStillRefusesAHeadingThatOnlyExistsBefore
// Collapsing. That is an over-refusal, not a regression from adding the
// second screen, and the safe direction to err in: this function is not in
// the business of trying to predict a text's post-collapse shape before
// screening the pre-collapse shape too.
func oneLine(text string) (string, error) {
	if err := Screen(text); err != nil {
		return "", err
	}
	flat := strings.Join(strings.Fields(strings.ReplaceAll(text, "\n", " ")), " ")
	if flat == "" {
		return "", ErrEmptyText
	}
	if err := Screen(flat); err != nil {
		return "", err
	}
	return flat, nil
}

func splitLines(body string) []string {
	if strings.TrimSpace(body) == "" {
		return nil
	}
	return strings.Split(body, "\n")
}

func appendLine(body, line string) string {
	if strings.TrimSpace(body) == "" {
		return line
	}
	return body + "\n" + line
}

// matchLine finds the one line containing find. Zero and many are both errors:
// taking the first of several would edit a line the agent did not mean, and it
// would have no way to notice.
func matchLine(body, find string) (int, error) {
	needle := strings.TrimSpace(find)
	if needle == "" {
		return 0, ErrEmptyFind
	}
	lines := splitLines(body)
	hits := []int{}
	for i, l := range lines {
		if strings.Contains(l, needle) {
			hits = append(hits, i)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		return 0, fmt.Errorf("%w for %q", ErrNoMatch, needle)
	default:
		return 0, fmt.Errorf("%w: %q is in %d lines; use a longer piece of the one you mean",
			ErrAmbiguous, needle, len(hits))
	}
}

// budgeted refuses a result over budget — unless it is smaller than what was
// there, which is how an agent digs out of a body that is already too big.
//
// Repo.Save has to agree with this escape hatch exactly (see its own budget
// check), because Apply never writes: the tool that calls Apply always saves
// what comes back through Save, and a body this function accepts must not be
// refused a moment later at the one place that actually persists it.
func budgeted(scope Scope, before, after string) (string, error) {
	used := utf8.RuneCountInString(after)
	if used <= scope.Budget() || used < utf8.RuneCountInString(before) {
		return after, nil
	}
	return "", fmt.Errorf("%w: this would be %d characters and %s holds %d — replace or remove a line first",
		ErrOverBudget, used, scope, scope.Budget())
}
