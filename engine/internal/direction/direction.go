// Package direction holds the writer's own direction for a work: the
// current focus (a few episodes' worth of "what to concentrate on") and a
// scene's intent (its one-line goal, what to keep, what to avoid).
//
// This is an instruction the writer gives, not something the agent
// discovers or drafts — see the README's "Your direction" section.
// Nothing in this package calls a model; it only stores, bounds and reports
// staleness.
package direction

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// Limits. The story brief injects both the focus and a scene's intent into
// every prompt, so both are bounded the way style.MaxProfileRunes and
// agentmemory's work-notes budget are.
const (
	// MaxFocusRunes bounds the writer's current-focus note.
	MaxFocusRunes = 500
	// MaxGoalRunes bounds a scene's one-line goal.
	MaxGoalRunes = 200
	// MaxListItems bounds how many keep/avoid items a scene may carry.
	MaxListItems = 5
	// MaxItemRunes bounds one keep/avoid item.
	MaxItemRunes = 120
	// FocusStaleAfterEpisodes is how many episodes may pass, written, after
	// a focus was set before the app asks the writer if it still holds.
	FocusStaleAfterEpisodes = 3
)

// Focus is the writer's current focus for a work: what to concentrate on for
// the next few episodes.
type Focus struct {
	ProjectID string `json:"project_id"`
	Body      string `json:"body"`
	// AnchorNodeID is the episode node the focus was set at — the engine's
	// choice, not the writer's (episode.LastWritten at SetFocus time).
	AnchorNodeID string `json:"anchor_node_id,omitempty"`
	SetAt        int64  `json:"set_at"`
}

// FocusState is a Focus plus what GetFocus can tell about it right now.
type FocusState struct {
	Focus
	// Present is false when the work has no focus set. The other fields are
	// zero in that case.
	Present bool `json:"present"`
	// EpisodesSince is how many episodes, by ordinal, have been written
	// since the anchor episode. -1 when the anchor node was recorded but no
	// longer exists (it was deleted) — the UI shows "anchor episode not
	// found." A focus set before anything was written has no anchor node at
	// all; that is not this case, and reads as episode 0 instead.
	EpisodesSince int `json:"episodes_since"`
	// Stale is true once EpisodesSince >= FocusStaleAfterEpisodes, or the
	// anchor episode is gone.
	Stale bool `json:"stale"`
}

// SceneIntent is one scene's intent: its goal, and what to keep or avoid.
type SceneIntent struct {
	NodeID    string   `json:"node_id"`
	ProjectID string   `json:"project_id"`
	Goal      string   `json:"goal"`
	Keep      []string `json:"keep"`
	Avoid     []string `json:"avoid"`
	UpdatedAt int64    `json:"updated_at"`
}

var (
	// ErrFocusTooLong means the focus is over MaxFocusRunes. Refused, not
	// truncated: a direction cut off mid-sentence can mean something the
	// writer did not intend.
	ErrFocusTooLong = errors.New("focus is over the character limit")
	// ErrGoalTooLong means a scene's goal is over MaxGoalRunes.
	ErrGoalTooLong = errors.New("scene goal is over the character limit")
	// ErrTooManyItems means a keep/avoid list has more than MaxListItems.
	ErrTooManyItems = errors.New("at most 5 items")
	// ErrItemTooLong means one keep/avoid item is over MaxItemRunes.
	ErrItemTooLong     = errors.New("an item is over the character limit")
	ErrProjectNotFound = errors.New("project not found")
	ErrNodeNotFound    = errors.New("node not found")
	ErrNotScene        = errors.New("node is not a scene")
)

// normalizeList trims each item, drops blanks, folds case-insensitive
// duplicates and enforces the limits — style.NormalizePhrases's rule for a
// list of short free-text items, applied to keep/avoid. The result is never
// nil so an empty list serializes as [] on the wire, not null.
func normalizeList(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, item := range in {
		item = strings.Join(strings.Fields(item), " ")
		if item == "" {
			continue
		}
		if utf8.RuneCountInString(item) > MaxItemRunes {
			return nil, ErrItemTooLong
		}
		key := strings.ToLower(item)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	if len(out) > MaxListItems {
		return nil, ErrTooManyItems
	}
	return out, nil
}
