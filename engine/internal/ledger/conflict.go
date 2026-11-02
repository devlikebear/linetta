package ledger

import (
	"context"
	"fmt"
	"strings"
)

// Conflict is a recorded fact a new one disagrees with. It is a report, never a
// refusal: the story may have a reason, and only the writer can tell.
type Conflict struct {
	EntryID string `json:"entry_id"`
	Body    string `json:"body"`
	Reason  string `json:"reason"`
}

// Previous is the last number on record before a fact, in story order.
type Previous struct {
	EntryID  string  `json:"entry_id"`
	ValueNum float64 `json:"value_num"`
	Delta    float64 `json:"delta"`
}

// Conflicts compares a fact with what the ledger holds for the same subject and
// attribute at the same point in the story. It works from values alone, so it
// finds what is written under the same name and nothing subtler.
func (r *Repo) Conflicts(ctx context.Context, e Entry) ([]Conflict, *Previous, error) {
	if e.Attribute == "" || e.Attribute == AttributeKnows {
		return nil, nil, nil
	}
	all, err := r.nodes.ListByProject(ctx, e.ProjectID)
	if err != nil {
		return nil, nil, err
	}
	t := newTree(all)
	cur, onStory := t.index[e.PlantedNodeID]
	if !onStory {
		cur = len(t.leaves)
	}
	entries, err := r.List(ctx, Filter{ProjectID: e.ProjectID, Status: StatusOpen})
	if err != nil {
		return nil, nil, err
	}

	var conflicts []Conflict
	var prior *Entry
	for i := range entries {
		other := entries[i]
		if other.ID == e.ID || other.Subject != e.Subject || other.Attribute != e.Attribute || other.SupersededBy != "" {
			continue
		}
		if other.ValueNum != nil && e.ValueNum != nil && t.plantedAt(other) <= cur &&
			(prior == nil || t.plantedAt(other) >= t.plantedAt(*prior)) {
			prior = &entries[i]
		}
		if disagrees(other, e) && t.holdsAt(other, cur) {
			conflicts = append(conflicts, Conflict{EntryID: other.ID, Body: other.Body,
				Reason: fmt.Sprintf("%s의 %s가 다르게 기록되어 있습니다", e.Subject, e.Attribute)})
		}
	}
	var previous *Previous
	if prior != nil {
		previous = &Previous{EntryID: prior.ID, ValueNum: *prior.ValueNum, Delta: *e.ValueNum - *prior.ValueNum}
		if e.Attribute == AttributeDate && previous.Delta < 0 {
			conflicts = append(conflicts, Conflict{EntryID: prior.ID, Body: prior.Body,
				Reason: "작중 날짜가 이야기 순서보다 뒤로 갑니다"})
		}
	}
	return conflicts, previous, nil
}

// disagrees reports whether two facts about one thing cannot both stand. A date
// is meant to move, and is judged by its order below. What a character says or a
// rumour carries may be false without the story being wrong.
func disagrees(a, b Entry) bool {
	if a.Attribute == AttributeDate || a.Strength == StrengthSoft || b.Strength == StrengthSoft {
		return false
	}
	return !sameValue(a, b)
}

func sameValue(a, b Entry) bool {
	if a.ValueNum != nil && b.ValueNum != nil {
		return *a.ValueNum == *b.ValueNum
	}
	return strings.TrimSpace(a.Body) == strings.TrimSpace(b.Body)
}

// Supersede closes an old fact at the new one's scene and records what replaced
// it. Both stay in the ledger: what was true before still was.
func (r *Repo) Supersede(ctx context.Context, now int64, oldID string, next Entry) error {
	old, err := r.Get(ctx, oldID)
	if err != nil {
		return err
	}
	if old.ProjectID != next.ProjectID {
		return ErrWrongWork
	}
	if old.Pinned {
		return ErrSupersedePinned
	}
	_, err = r.s.DB().ExecContext(ctx, `
UPDATE story_ledger SET superseded_by = ?, valid_to_node_id = COALESCE(NULLIF(?, ''), valid_to_node_id), updated_at = ?
 WHERE id = ?`, next.ID, next.PlantedNodeID, now, oldID)
	return err
}
