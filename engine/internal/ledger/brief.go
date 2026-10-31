package ledger

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/storycontext"
)

const (
	// briefMaxRunes bounds the ledger's share of the brief, due items first.
	briefMaxRunes = 3000
	// dueHorizon is how many scenes ahead counts as coming due.
	dueHorizon = 3
	// neglectedAfter is how many scenes an open promise may sit before the
	// brief says so.
	neglectedAfter = 20
)

// ContextLedger is the ledger as the brief for one scene sees it. Nothing
// planted after that scene is shown: a brief must not read ahead of the story.
func (r *Repo) ContextLedger(ctx context.Context, projectID, nodeID string) (storycontext.LedgerBrief, error) {
	var out storycontext.LedgerBrief
	all, err := r.nodes.ListByProject(ctx, projectID)
	if err != nil {
		return out, err
	}
	t := newTree(all)
	cur, onStory := t.index[nodeID]
	if !onStory {
		cur = len(t.leaves)
	}
	root := t.root(nodeID)

	arcs, err := r.ListArcs(ctx, projectID)
	if err != nil {
		return out, err
	}
	if a, ok := arcs[root]; ok {
		out.Part = &storycontext.PartArcBrief{
			Label: t.byID[root].Label, Role: a.Role, Goal: a.Goal, Tension: a.Tension,
			ScenesLeft: t.scenesLeft(root, cur),
		}
	}

	entries, err := r.List(ctx, Filter{ProjectID: projectID, Status: StatusOpen})
	if err != nil {
		return out, err
	}
	budget := briefMaxRunes
	take := func(line string, into *[]string) bool {
		budget -= utf8.RuneCountInString(line)
		if budget < 0 {
			out.Omitted++
			return false
		}
		*into = append(*into, line)
		return true
	}
	var owed, settled []Entry
	for _, e := range entries {
		if e.Kind == KindCutPlan || t.after(e.PlantedNodeID, cur) {
			continue
		}
		if Tracked(e.Kind) {
			owed = append(owed, e)
		} else {
			settled = append(settled, e)
		}
	}
	for _, e := range owed {
		if t.due(e, cur, root) {
			take(t.line(e, cur), &out.Due)
		}
	}
	for _, e := range owed {
		if !t.due(e, cur, root) {
			take(t.line(e, cur), &out.Open)
		}
	}
	// Newest first: what was settled last is likeliest to bear on this scene.
	for i := len(settled) - 1; i >= 0; i-- {
		take(t.line(settled[i], cur), &out.Settled)
	}
	return out, nil
}

// tree is the outline in story order.
type tree struct {
	byID   map[string]node.Node
	leaves []node.Node
	index  map[string]int
}

func newTree(all []node.Node) tree {
	t := tree{byID: make(map[string]node.Node, len(all)), index: map[string]int{}}
	children := map[string][]node.Node{}
	for _, n := range all {
		t.byID[n.ID] = n
		key := ""
		if n.ParentID != nil {
			key = *n.ParentID
		}
		children[key] = append(children[key], n)
	}
	var walk func(string)
	walk = func(parent string) {
		for _, c := range children[parent] {
			if c.Kind == node.KindLeaf {
				t.index[c.ID] = len(t.leaves)
				t.leaves = append(t.leaves, c)
			}
			walk(c.ID)
		}
	}
	walk("")
	return t
}

// root returns the top-level node above id.
func (t tree) root(id string) string {
	n, ok := t.byID[id]
	for hops := 0; ok && n.ParentID != nil && hops < 64; hops++ {
		parent, found := t.byID[*n.ParentID]
		if !found {
			break
		}
		n = parent
	}
	return n.ID
}

// lastScene returns the story index of the last scene at or under id.
func (t tree) lastScene(id string) (int, bool) {
	if i, ok := t.index[id]; ok {
		return i, true
	}
	last, found := 0, false
	for i, l := range t.leaves {
		for at := l; at.ParentID != nil; {
			if *at.ParentID == id {
				last, found = i, true
				break
			}
			parent, ok := t.byID[*at.ParentID]
			if !ok {
				break
			}
			at = parent
		}
	}
	return last, found
}

func (t tree) scenesLeft(part string, cur int) int {
	last, ok := t.lastScene(part)
	if !ok || last < cur {
		return 0
	}
	return last - cur
}

func (t tree) after(planted string, cur int) bool {
	i, ok := t.index[planted]
	return ok && i > cur
}

// due says whether an owed entry should be paid about now: at or past its
// scene, within the horizon, or promised to the part being written.
func (t tree) due(e Entry, cur int, root string) bool {
	if e.DueNodeID == "" {
		return false
	}
	if e.DueNodeID == root {
		return true
	}
	last, ok := t.lastScene(e.DueNodeID)
	return ok && last <= cur+dueHorizon
}

var kindLabels = map[string]string{
	KindPromise: "복선", KindGag: "반복 개그", KindQuestion: "독자의 질문",
	KindDecision: "결정", KindNumber: "수치", KindTimeline: "시간선", KindKnowledge: "인물 지식",
}

func (t tree) line(e Entry, cur int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[%s] ", kindLabels[e.Kind])
	if e.Subject != "" {
		b.WriteString(e.Subject + ": ")
	}
	b.WriteString(e.Body)
	var notes []string
	if n, ok := t.byID[e.PlantedNodeID]; ok {
		notes = append(notes, n.Label+"에서 심음")
	}
	if n, ok := t.byID[e.DueNodeID]; ok {
		notes = append(notes, n.Label+"까지")
	}
	if i, ok := t.index[e.PlantedNodeID]; ok && Tracked(e.Kind) && cur-i >= neglectedAfter {
		notes = append(notes, fmt.Sprintf("%d화째 열려 있음", cur-i))
	}
	if len(notes) > 0 {
		b.WriteString(" (" + strings.Join(notes, ", ") + ")")
	}
	return b.String()
}
