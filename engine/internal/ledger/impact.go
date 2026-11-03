package ledger

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"unicode"

	"github.com/devlikebear/linetta/engine/internal/storycontext"
)

// impactMaxScenes is how many scenes a change names. A fact about the hero is on
// every page, so the list has to be a ranking, and past this it stops helping.
const impactMaxScenes = 12

// Reasons a scene is a candidate.
const (
	ReasonRecorded = "recorded"
	ReasonNamed    = "named"
	ReasonWords    = "words"
)

// Candidate is a written scene a change to a fact may touch.
type Candidate struct {
	NodeID  string   `json:"node_id"`
	Label   string   `json:"label"`
	Reasons []string `json:"reasons"`
	Score   int      `json:"score"`
}

// Impact lists the scenes to read when a fact changes, in story order, and how
// many it left out. It works from the prose rather than the mention links, which
// only exist where a name was registered before the scene was written: the first
// scenes of a work are often unlinked, and they are where its premises are set.
// It decides nothing: a candidate is a scene worth reading.
func (r *Repo) Impact(ctx context.Context, entryID string) ([]Candidate, int, error) {
	e, err := r.Get(ctx, entryID)
	if err != nil {
		return nil, 0, err
	}
	all, err := r.nodes.ListByProject(ctx, e.ProjectID)
	if err != nil {
		return nil, 0, err
	}
	entries, err := r.List(ctx, Filter{ProjectID: e.ProjectID})
	if err != nil {
		return nil, 0, err
	}
	t := newTree(all)

	recorded := map[string]bool{e.PlantedNodeID: true, e.DueNodeID: true, e.PaidNodeID: true}
	for _, other := range entries {
		if other.Subject == e.Subject && other.Attribute == e.Attribute && (e.Subject != "" || e.Attribute != "") {
			recorded[other.PlantedNodeID] = true
		}
	}
	names := r.subjectNames(ctx, e)
	words := impactWords(e.Subject + " " + e.Body)
	phrases := impactPhrases(e.Body)

	var out []Candidate
	for _, leaf := range t.leaves {
		text := storycontext.PlainText(leaf.ContentDoc)
		if strings.TrimSpace(text) == "" {
			continue
		}
		c := Candidate{NodeID: leaf.ID, Label: leaf.Label}
		if recorded[leaf.ID] {
			c.Reasons = append(c.Reasons, ReasonRecorded)
			c.Score += 8
		}
		if n := countAny(text, names); n > 0 {
			c.Reasons = append(c.Reasons, ReasonNamed)
			c.Score += min(n, 3)
		}
		// Words of the fact itself weigh more than its subject's name: the hero
		// is on every page, and what the fact says about him is not.
		if n := countDistinct(text, words) + 2*countDistinct(text, phrases); n > 0 {
			c.Reasons = append(c.Reasons, ReasonWords)
			c.Score += 2 * n
		}
		if c.Score > 0 {
			out = append(out, c)
		}
	}
	omitted := 0
	if len(out) > impactMaxScenes {
		slices.SortStableFunc(out, func(a, b Candidate) int { return b.Score - a.Score })
		omitted = len(out) - impactMaxScenes
		out = out[:impactMaxScenes]
	}
	slices.SortFunc(out, func(a, b Candidate) int { return cmp.Compare(t.index[a.NodeID], t.index[b.NodeID]) })
	return out, omitted, nil
}

func (r *Repo) subjectNames(ctx context.Context, e Entry) []string {
	if strings.TrimSpace(e.Subject) == "" {
		return nil
	}
	if r.subjects != nil {
		if names := r.subjects.Names(ctx, e.ProjectID, e.Subject); len(names) > 0 {
			return names
		}
	}
	return []string{e.Subject}
}

// impactTokens splits a fact into words.
func impactTokens(body string) []string {
	return strings.FieldsFunc(body, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsPunct(r) })
}

// impactWords are a fact's longer words. A Korean word may carry a particle the
// text does not, so the last rune goes; a word of two runes is too short to lose
// one and too common to mean much, which is why phrases count for more.
func impactWords(body string) []string {
	var out []string
	for _, word := range impactTokens(body) {
		runes := []rune(word)
		if len(runes) < 3 {
			continue
		}
		stem := string(runes[:len(runes)-1])
		if !slices.Contains(out, stem) {
			out = append(out, stem)
		}
	}
	return out
}

// impactPhrases are a fact's neighbouring words, as they stand. A phrase found
// is far better evidence than a word: it is how "never held a sword" finds the
// page that says otherwise when neither word alone is rare.
func impactPhrases(body string) []string {
	tokens := impactTokens(body)
	out := make([]string, 0, len(tokens))
	for i := 1; i < len(tokens); i++ {
		out = append(out, tokens[i-1]+" "+tokens[i])
	}
	return out
}

func countAny(text string, names []string) int {
	n := 0
	for _, name := range names {
		if name != "" {
			n += strings.Count(text, name)
		}
	}
	return n
}

func countDistinct(text string, words []string) int {
	n := 0
	for _, word := range words {
		if strings.Contains(text, word) {
			n++
		}
	}
	return n
}

// SampleScene is one written scene, in story order, as a sample check sees it.
type SampleScene struct {
	NodeID  string
	Label   string
	Version int
	// Anchor is set on the first scene under each container and on the work's
	// first scene, where what later scenes rest on is set down.
	Anchor bool
}

// SampleScenes lists a work's written scenes in story order.
func (r *Repo) SampleScenes(ctx context.Context, projectID string) ([]SampleScene, error) {
	all, err := r.nodes.ListByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	t := newTree(all)
	opened := map[string]bool{}
	var out []SampleScene
	for _, leaf := range t.leaves {
		if strings.TrimSpace(storycontext.PlainText(leaf.ContentDoc)) == "" {
			continue
		}
		anchor := len(out) == 0
		for at := leaf; at.ParentID != nil; {
			if !opened[*at.ParentID] {
				opened[*at.ParentID] = true
				anchor = true
			}
			parent, ok := t.byID[*at.ParentID]
			if !ok {
				break
			}
			at = parent
		}
		out = append(out, SampleScene{NodeID: leaf.ID, Label: leaf.Label, Version: leaf.ContentVersion, Anchor: anchor})
	}
	return out, nil
}

// Neighbour is the earlier scene that shares the most uncommon names with a
// scene, among those not excluded, or "". A name in every scene says nothing
// about two of them, so rarer names count for more.
func (r *Repo) Neighbour(ctx context.Context, projectID, nodeID string, exclude map[string]bool) (string, error) {
	all, err := r.nodes.ListByProject(ctx, projectID)
	if err != nil {
		return "", err
	}
	if r.subjects == nil {
		return "", nil
	}
	groups := r.subjects.AllNames(ctx, projectID)
	t := newTree(all)
	at, ok := t.index[nodeID]
	if !ok {
		return "", nil
	}
	present := make([]map[int]bool, len(t.leaves))
	df := make(map[int]int, len(groups))
	for i, leaf := range t.leaves[:at+1] {
		text := storycontext.PlainText(leaf.ContentDoc)
		present[i] = map[int]bool{}
		for g, names := range groups {
			if countAny(text, names) > 0 {
				present[i][g] = true
				df[g]++
			}
		}
	}
	best, bestScore := "", 0.0
	for i := at - 1; i >= 0; i-- {
		if exclude[t.leaves[i].ID] {
			continue
		}
		score := 0.0
		for g := range present[i] {
			if present[at][g] {
				score += 1 / float64(df[g])
			}
		}
		if score > bestScore {
			best, bestScore = t.leaves[i].ID, score
		}
	}
	return best, nil
}

// PinnedBodies lists what the writer pinned.
func (r *Repo) PinnedBodies(ctx context.Context, projectID string) ([]string, error) {
	entries, err := r.List(ctx, Filter{ProjectID: projectID, Status: StatusOpen})
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.Pinned {
			out = append(out, e.Body)
		}
	}
	return out, nil
}
