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
	// JumpDays is how many days an elapsed time or an age grew since Previous
	// when neither scene's story date is known, and ScenesBetween how many
	// scenes on it was. The engine does not judge such a jump: the prose may
	// say the time passed. The settlement pass reads it and decides.
	JumpDays      float64 `json:"jump_days,omitempty"`
	ScenesBetween int     `json:"scenes_between,omitempty"`
}

// Conflicts compares a fact with what the ledger holds for the same subject and
// attribute at the same point in the story. It works from values alone, so it
// finds what is written under the same name and nothing subtler.
//
// What a person knows piles up, so a knows fact is compared only when it names
// the question it is about: then the last one before it, about the same
// question, says what it may not say.
//
// An elapsed time is compared only with others from the same event, and an age
// in years with the same person's: see elapsedConflicts. An elapsed time with
// no event is kept and not compared. A scene marked a flashback is left out of
// every comparison in story order.
func (r *Repo) Conflicts(ctx context.Context, e Entry) ([]Conflict, *Previous, error) {
	if e.Attribute == AttributeKnows && e.About != "" {
		conflicts, err := r.knowsConflicts(ctx, e)
		return conflicts, nil, err
	}
	if e.Attribute == AttributeElapsed || (e.Attribute == AttributeAge && e.ValueNum != nil) {
		return r.elapsedConflicts(ctx, e)
	}
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
		if other.ValueNum != nil && e.ValueNum != nil && t.plantedAt(other) <= cur && t.ordered(other, cur) &&
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

// knowsConflicts compares a knows fact about a question with the last one
// before it for the same person and question. Learning is no conflict; knowing
// and then not knowing is, and so is learning again what was already known. A
// fact that replaces the earlier one (amnesia, say) was meant, and is not.
func (r *Repo) knowsConflicts(ctx context.Context, e Entry) ([]Conflict, error) {
	if e.ValueNum == nil || e.Strength == StrengthSoft {
		return nil, nil
	}
	all, err := r.nodes.ListByProject(ctx, e.ProjectID)
	if err != nil {
		return nil, err
	}
	t := newTree(all)
	cur, onStory := t.index[e.PlantedNodeID]
	if !onStory {
		cur = len(t.leaves)
	}
	entries, err := r.List(ctx, Filter{ProjectID: e.ProjectID, Kinds: []string{KindKnowledge}, Status: StatusOpen})
	if err != nil {
		return nil, err
	}
	var prior *Entry
	for i := range entries {
		other := entries[i]
		if other.ID == e.ID || other.Subject != e.Subject || other.Attribute != AttributeKnows ||
			other.About != e.About || other.ValueNum == nil || other.Strength == StrengthSoft {
			continue
		}
		if other.SupersededBy == e.ID {
			return nil, nil
		}
		at := t.plantedAt(other)
		if other.SupersededBy != "" || at > cur || (at == cur && other.CreatedAt > e.CreatedAt) || !t.ordered(other, cur) {
			continue
		}
		if prior == nil || t.storyOrder(other, *prior) >= 0 {
			prior = &entries[i]
		}
	}
	if prior == nil || *prior.ValueNum != KnowsYes {
		return nil, nil
	}
	where := "앞에서"
	if n, ok := t.byID[prior.PlantedNodeID]; ok {
		where = n.Label + "에서"
	}
	reason := fmt.Sprintf("%s는 이미 %s 알았습니다", e.Subject, where)
	if *e.ValueNum == KnowsNo {
		reason = fmt.Sprintf("%s는 %s 안 것을 모르는 것으로 서술됩니다", e.Subject, where)
	}
	return []Conflict{{EntryID: prior.ID, Body: prior.Body, Reason: reason}}, nil
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
