package ledger

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// factKey is what makes two facts about the same thing.
type factKey struct{ subject, attribute string }

// holdsAt reports whether a fact is true at story position cur. A fact starts at
// the scene it was planted in, or from the start without one; it stops at the
// scene it runs to, where the next value takes over.
func (t tree) holdsAt(e Entry, cur int) bool {
	if e.Attribute == "" || e.Status != StatusOpen || e.SupersededBy != "" || t.after(e.PlantedNodeID, cur) {
		return false
	}
	if i, ok := t.index[e.ValidToNodeID]; ok && i <= cur {
		return false
	}
	return true
}

// plantedAt is a fact's place in the story, -1 when it has none.
func (t tree) plantedAt(e Entry) int {
	if i, ok := t.index[e.PlantedNodeID]; ok {
		return i
	}
	return -1
}

// current narrows entries to what is true at cur: the latest fact for each
// subject and attribute, except what a person knows, which piles up.
func (t tree) current(entries []Entry, cur int) []Entry {
	latest := map[factKey]Entry{}
	var out []Entry
	for _, e := range entries {
		if !t.holdsAt(e, cur) {
			continue
		}
		if e.Attribute == AttributeKnows {
			out = append(out, e)
			continue
		}
		k := factKey{e.Subject, e.Attribute}
		if prior, ok := latest[k]; !ok || t.plantedAt(e) >= t.plantedAt(prior) {
			latest[k] = e
		}
	}
	for _, e := range latest {
		out = append(out, e)
	}
	slices.SortFunc(out, func(a, b Entry) int {
		return cmp.Or(cmp.Compare(a.Subject, b.Subject), cmp.Compare(a.Attribute, b.Attribute), cmp.Compare(a.CreatedAt, b.CreatedAt), cmp.Compare(a.ID, b.ID))
	})
	return out
}

// AsOf returns what the ledger says is true at a scene.
func (r *Repo) AsOf(ctx context.Context, projectID, nodeID string) ([]Entry, error) {
	all, err := r.nodes.ListByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	t := newTree(all)
	cur, onStory := t.index[nodeID]
	if !onStory {
		cur = len(t.leaves)
	}
	entries, err := r.List(ctx, Filter{ProjectID: projectID, Status: StatusOpen})
	if err != nil {
		return nil, err
	}
	return t.current(entries, cur), nil
}

// stateLines renders the current state, one line a subject, the subjects this
// scene talks about first. It returns how many subjects did not fit.
func (t tree) stateLines(entries []Entry, nodeID string, cur int) ([]string, int) {
	now := t.current(entries, cur)
	values := map[string][]string{}
	var subjects []string
	for _, e := range now {
		if e.Pinned {
			continue
		}
		if _, seen := values[e.Subject]; !seen {
			subjects = append(subjects, e.Subject)
		}
		value := e.Body
		if e.Strength == StrengthSoft {
			value += " (미확정)"
		}
		values[e.Subject] = append(values[e.Subject], e.Attribute+"="+clip(value, briefStateValueRunes))
	}
	text := t.sceneText(nodeID)
	slices.SortStableFunc(subjects, func(a, b string) int { return subjectHits(b, text) - subjectHits(a, text) })
	room, omitted := briefStateRunes, 0
	var lines []string
	for _, subject := range subjects {
		line := fmt.Sprintf("%s: %s", subject, strings.Join(values[subject], "; "))
		cost := utf8.RuneCountInString(line)
		if cost > room {
			omitted++
			continue
		}
		room -= cost
		lines = append(lines, line)
	}
	return lines, omitted
}

func clip(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit]) + "…"
}
