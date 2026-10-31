// dedupe.go implements 3.B.2: when a proposal's create_entity item's name or
// alias normalizes to the same text as an existing entity, or another
// pending item's, the item is flagged "might be the same thing" — the
// writer decides, same as InkOS's arbiter deliberately not inferring
// semantic identity on its own.
package proposals

import (
	"context"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// normalizeForDedupe folds name/alias text to one comparable form: NFKC,
// lowercase, with whitespace, the middle dot (가운뎃점) and hyphens removed —
// so "김민호" / "김 민호", and "Ha-rin" / "harin", compare equal.
func normalizeForDedupe(s string) string {
	s = norm.NFKC.String(s)
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if unicode.IsSpace(r) || r == '·' || r == '-' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// dedupeCandidate is one thing a new create_entity item's name/aliases can
// match against: an existing entity, or another pending item.
type dedupeCandidate struct {
	EntityID string
	ItemID   string
	Name     string
	Kind     string
	Aliases  []string
}

// findDuplicates compares name/kind against every candidate's name and
// aliases, normalized, and reports every match. It never merges or chooses
// for the writer — see the package doc comment above.
func findDuplicates(name, kind string, candidates []dedupeCandidate) []Duplicate {
	target := normalizeForDedupe(name)
	if target == "" {
		return nil
	}
	var dups []Duplicate
	for _, c := range candidates {
		terms := make([]string, 0, len(c.Aliases)+1)
		terms = append(terms, c.Name)
		terms = append(terms, c.Aliases...)
		for _, term := range terms {
			if normalizeForDedupe(term) != target {
				continue
			}
			matched := "exact"
			if c.Kind != "" && kind != "" && c.Kind != kind {
				matched = "exact_kind_mismatch"
			}
			dups = append(dups, Duplicate{
				EntityID: c.EntityID, ItemID: c.ItemID, Name: c.Name, Kind: c.Kind, Matched: matched,
			})
			break
		}
	}
	return dups
}

// attachDuplicates fills Duplicates on every create_entity item in items by
// comparing it against the project's existing entities and every OTHER
// pending create_entity item (already stored ones, plus siblings in this
// same not-yet-stored batch) — 3.B.2's two comparison sets.
func (s *Service) attachDuplicates(ctx context.Context, projectID string, items []Item) error {
	existing, err := s.existingEntityCandidates(ctx, projectID)
	if err != nil {
		return err
	}
	pending, err := s.pendingEntityCandidates(ctx, projectID)
	if err != nil {
		return err
	}

	for i := range items {
		if items[i].Op.Type != "create_entity" {
			continue
		}
		candidates := make([]dedupeCandidate, 0, len(existing)+len(pending)+len(items)-1)
		candidates = append(candidates, existing...)
		candidates = append(candidates, pending...)
		for j := range items {
			if j == i || items[j].Op.Type != "create_entity" {
				continue
			}
			candidates = append(candidates, dedupeCandidate{
				ItemID: items[j].ID, Name: items[j].Op.Name, Kind: items[j].Op.Kind, Aliases: items[j].Op.Aliases,
			})
		}
		items[i].Duplicates = findDuplicates(items[i].Op.Name, items[i].Op.Kind, candidates)
	}
	return nil
}

// refreshDuplicates re-reads every undecided create_entity item's matches
// against the work as it stands now. What Submit stored was true at
// submission, but a name can reach Story World afterwards, from another batch
// or a direct apply, and a match against an entity since deleted is no use.
// Matches against other pending items are left as Submit recorded them.
func (s *Service) refreshDuplicates(ctx context.Context, projectID string, list []ProposalWithItems) error {
	existing, err := s.existingEntityCandidates(ctx, projectID)
	if err != nil {
		return err
	}
	for _, p := range list {
		for i := range p.Items {
			it := &p.Items[i]
			if it.Op.Type != "create_entity" || !IsOpen(*it) {
				continue
			}
			fresh := findDuplicates(it.Op.Name, it.Op.Kind, existing)
			for _, dup := range it.Duplicates {
				if dup.ItemID != "" {
					fresh = append(fresh, dup)
				}
			}
			it.Duplicates = nonNilDuplicates(fresh)
		}
	}
	return nil
}

func (s *Service) existingEntityCandidates(ctx context.Context, projectID string) ([]dedupeCandidate, error) {
	if s.entities == nil {
		return nil, nil
	}
	ents, err := s.entities.ListByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]dedupeCandidate, len(ents))
	for i, e := range ents {
		out[i] = dedupeCandidate{EntityID: e.ID, Name: e.Name, Kind: e.Kind, Aliases: e.Aliases}
	}
	return out, nil
}

func (s *Service) pendingEntityCandidates(ctx context.Context, projectID string) ([]dedupeCandidate, error) {
	items, err := s.listPendingCreateEntityItems(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]dedupeCandidate, len(items))
	for i, it := range items {
		out[i] = dedupeCandidate{ItemID: it.ID, Name: it.Op.Name, Kind: it.Op.Kind, Aliases: it.Op.Aliases}
	}
	return out, nil
}
