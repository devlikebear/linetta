package relationship

import (
	"errors"
	"fmt"

	"github.com/devlikebear/linetta/engine/internal/node"
)

// AsOf narrows rels to what holds at the scene nodeID, by the ledger's rule
// for facts: a row starts at its since scene (or the start) and stops at its
// until scene, where its successor takes over. A row started in a flashback
// holds in that scene alone, and a change made in one does not end the row
// it changes anywhere else: it was so then, not now. nodeID "" or a scene not
// in the story reads the end of the story. all is the work's nodes as
// node.Repo.ListByProject returns them.
func AsOf(rels []Relationship, all []node.Node, nodeID string) []Relationship {
	leaves := node.ReadingOrder(all)
	index := make(map[string]int, len(leaves))
	for i, n := range leaves {
		index[n.ID] = i
	}
	flashback := func(i int) bool { return i >= 0 && i < len(leaves) && leaves[i].Flashback }
	at := func(id *string) (int, bool) {
		if id == nil {
			return -1, false
		}
		i, ok := index[*id]
		return i, ok
	}
	cur, ok := index[nodeID]
	if !ok {
		cur = len(leaves)
	}
	holds := func(r Relationship) bool {
		if s, ok := at(r.SinceNodeID); ok && (s > cur || (s != cur && flashback(s))) {
			return false
		}
		if u, ok := at(r.UntilNodeID); ok && u <= cur && (u == cur || !flashback(u)) {
			return false
		}
		return true
	}
	var held []Relationship
	replaced := map[string]bool{}
	for _, r := range rels {
		if holds(r) {
			held = append(held, r)
			if r.SupersedesID != nil {
				replaced[*r.SupersedesID] = true
			}
		}
	}
	// A deleted scene leaves both ends of a change NULL, so both rows hold;
	// the successor is the one that does.
	out := held[:0]
	for _, r := range held {
		if !replaced[r.ID] {
			out = append(out, r)
		}
	}
	return out
}

// ErrChangeBeforeStart is a change at or before the scene the relationship
// starts in: it would end before it began.
var ErrChangeBeforeStart = errors.New("the relationship starts in that scene or a later one; change it in a scene after where it starts")

// ErrSceneNotInWork is a change at a scene the work does not have.
var ErrSceneNotInWork = errors.New("not a scene of this work")

// SceneAfter checks that at is a scene of the work all holds and, when since
// is set, comes after it in reading order.
func SceneAfter(all []node.Node, at string, since *string) error {
	pos := map[string]int{}
	for i, n := range node.ReadingOrder(all) {
		pos[n.ID] = i
	}
	i, ok := pos[at]
	if !ok {
		return fmt.Errorf("scene %q: %w", at, ErrSceneNotInWork)
	}
	if since != nil {
		if j, ok := pos[*since]; ok && j >= i {
			return ErrChangeBeforeStart
		}
	}
	return nil
}
