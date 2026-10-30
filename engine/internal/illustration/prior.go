package illustration

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// MaxPriorReferences bounds the earlier illustrations sent with one drawing,
// after the characters' reference images. Two is enough to carry a face and
// an art style; more mostly costs upload time and invites the provider to
// copy a composition.
const MaxPriorReferences = 2

// priorNote is added to the prompt when earlier illustrations go along, so
// the provider reads them as a model sheet rather than a picture to redraw.
// It names no count: a prompt that already carries it (a drawing sent again)
// is not given it twice.
const priorNote = "Earlier illustrations:\n" +
	"The last attached images are earlier illustrations of this work. Keep the characters' faces, hair, " +
	"outfits, art style and coloring consistent with them, but draw a new composition for this scene; do not copy them."

// WithPriorNote is the prompt sent when earlier illustrations go along.
func WithPriorNote(prompt string) string {
	if strings.Contains(prompt, priorNote) {
		return prompt
	}
	return strings.TrimRight(prompt, "\n") + "\n\n" + priorNote
}

// SuggestPrior picks the earlier illustrations of a work worth sending with
// a new picture of the given characters, best first, at most limit.
//
// A picture counts as showing a character when the request that drew it
// named them, or when its prompt or caption names them (every composed
// prompt lists its cast by name). Pictures showing more of the characters
// come first, then pictures the writer kept (put in a scene, or made a
// scene's or episode's picture), then newer ones. With characters named and
// none of them in any picture, only the first of that order is offered, for
// the art style; with no characters named, up to limit of them are.
func (r *Repo) SuggestPrior(ctx context.Context, projectID string, characterIDs []string, limit int) ([]Meta, error) {
	if limit <= 0 || limit > MaxPriorReferences {
		limit = MaxPriorReferences
	}
	all, err := r.ListByProject(ctx, projectID)
	if err != nil || len(all) == 0 {
		return []Meta{}, err
	}
	names, err := r.characterNames(ctx, projectID, characterIDs)
	if err != nil {
		return nil, err
	}
	drawnFor, err := r.requestCharacters(ctx, projectID)
	if err != nil {
		return nil, err
	}
	kept, err := r.keptIDs(ctx, projectID)
	if err != nil {
		return nil, err
	}

	type candidate struct {
		meta    Meta
		matches int
		kept    bool
	}
	candidates := make([]candidate, 0, len(all))
	for _, m := range all {
		c := candidate{meta: m, kept: kept[m.ID]}
		text := m.Prompt + "\n" + m.Caption
		for _, id := range characterIDs {
			if drawnFor[m.ID][id] || mentionsAny(text, names[id]) {
				c.matches++
			}
		}
		candidates = append(candidates, c)
	}
	// ListByProject is newest first; a stable sort keeps that order among
	// equals.
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].matches != candidates[j].matches {
			return candidates[i].matches > candidates[j].matches
		}
		return candidates[i].kept && !candidates[j].kept
	})

	out := []Meta{}
	if len(characterIDs) > 0 && candidates[0].matches > 0 {
		for _, c := range candidates {
			if c.matches == 0 || len(out) == limit {
				break
			}
			out = append(out, c.meta)
		}
		return out, nil
	}
	if len(characterIDs) > 0 {
		limit = 1
	}
	for _, c := range candidates[:min(limit, len(candidates))] {
		out = append(out, c.meta)
	}
	return out, nil
}

// checkPrior confirms earlier pictures chosen by hand are few enough and in
// the work's own gallery.
func (r *Repo) checkPrior(ctx context.Context, projectID string, ids []string) error {
	if len(ids) > MaxPriorReferences {
		return fmt.Errorf("%w: at most %d earlier illustrations go along", ErrInvalidInput, MaxPriorReferences)
	}
	for _, id := range ids {
		var exists bool
		if err := r.s.DB().QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM illustrations WHERE id = ? AND project_id = ?)", id, projectID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("%w: illustration %q is not in this work's gallery", ErrInvalidInput, id)
		}
	}
	return nil
}

// characterNames maps each character to its name and aliases.
func (r *Repo) characterNames(ctx context.Context, projectID string, ids []string) (map[string][]string, error) {
	out := map[string][]string{}
	for _, id := range ids {
		var name, aliases string
		err := r.s.DB().QueryRowContext(ctx, "SELECT name, aliases FROM entities WHERE id = ? AND project_id = ?", id, projectID).Scan(&name, &aliases)
		if err != nil {
			return nil, fmt.Errorf("%w: character %q is not in this work", ErrInvalidInput, id)
		}
		list := []string{name}
		var more []string
		if json.Unmarshal([]byte(aliases), &more) == nil {
			list = append(list, more...)
		}
		out[id] = list
	}
	return out, nil
}

// requestCharacters maps each picture an agent's request drew to the
// characters that request named.
func (r *Repo) requestCharacters(ctx context.Context, projectID string) (map[string]map[string]bool, error) {
	rows, err := r.s.DB().QueryContext(ctx, `SELECT illustration_id, character_ids_json FROM illustration_requests
		WHERE project_id = ? AND illustration_id IS NOT NULL`, projectID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]map[string]bool{}
	for rows.Next() {
		var id, raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		var ids []string
		if json.Unmarshal([]byte(raw), &ids) != nil {
			continue
		}
		if out[id] == nil {
			out[id] = map[string]bool{}
		}
		for _, c := range ids {
			out[id][c] = true
		}
	}
	return out, rows.Err()
}

// keptIDs are the pictures the writer kept: shown in a scene, or standing
// for a scene or an episode.
func (r *Repo) keptIDs(ctx context.Context, projectID string) (map[string]bool, error) {
	usage, err := r.UsageByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	reps, err := r.Representatives(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for id := range usage {
		out[id] = true
	}
	for _, id := range reps {
		out[id] = true
	}
	return out, nil
}

func mentionsAny(text string, names []string) bool {
	text = strings.ToLower(text)
	for _, n := range names {
		if n = strings.ToLower(strings.TrimSpace(n)); n != "" && strings.Contains(text, n) {
			return true
		}
	}
	return false
}
