// Package episode finds a work's episodes (회차): the installment a reader
// reads in one sitting, and the unit Linetta Works publishes.
//
// The rules are the desktop app's — findEpisodeNode in
// apps/desktop/src/lib/outlineEpisode.ts and the chapter patterns in
// outlineRepair.ts — ported so the engine can publish without the UI. The UI
// keeps its own copy for now; episode_test.go carries outlineEpisode.test.ts's
// cases so the two keep giving the same answers until they are merged.
package episode

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
)

// ErrNotFound means the node is not part of the work's tree.
var ErrNotFound = errors.New("episode: node not found")

// Episode is one installment, in outline order.
type Episode struct {
	// NodeID identifies the episode. For a work with no episode structure it
	// is the first node of the outline, which is also what the Linetta Works
	// importer used, so an imported short story keeps its identity.
	NodeID string `json:"node_id"`
	Label  string `json:"label"`
	Title  string `json:"title"`
	// Ordinal is the 1-based position among the work's episodes.
	Ordinal int `json:"ordinal"`
	// Whole marks the single episode of a work without episode structure.
	Whole bool `json:"whole,omitempty"`
	// LeafIDs are the scenes whose text makes up the episode, in outline
	// order. Empty for an episode container nothing has been written into.
	LeafIDs []string `json:"-"`
}

const boundary = `(?:[\s\p{Z}]|$|[-—–·:])`

var novelChapterPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^(?:제[\s\p{Z}]*)?\d+[\s\p{Z}]*(?:장|章)` + boundary),
	regexp.MustCompile(`(?i)^(?:장|章)[\s\p{Z}]*\d+` + boundary),
	regexp.MustCompile(`(?i)^(?:chapter|ch)[\s\p{Z}]*\d+` + boundary),
}

var webNovelChapterPatterns = append(append([]*regexp.Regexp{}, novelChapterPatterns...),
	regexp.MustCompile(`(?i)^(?:제[\s\p{Z}]*)?\d+[\s\p{Z}]*(?:화|話|회)`+boundary),
	regexp.MustCompile(`(?i)^(?:화|話|회)[\s\p{Z}]*\d+`+boundary),
	regexp.MustCompile(`(?i)^(?:episode|ep)[\s\p{Z}]*\d+`+boundary),
)

func chapterPatterns(preset string) []*regexp.Regexp {
	if preset == project.OutlinePresetWebNovel {
		return webNovelChapterPatterns
	}
	return novelChapterPatterns
}

// IsChapterLabel reports whether label names an episode under the outline
// preset (isStructuralChapterLabel in outlineRepair.ts).
func IsChapterLabel(label, preset string) bool {
	normalized := strings.ToLower(strings.TrimSpace(label))
	for _, p := range chapterPatterns(preset) {
		if p.MatchString(normalized) {
			return true
		}
	}
	return false
}

// IsNumberedLabel reports whether label is an episode number and nothing
// else worth showing as a title ("3화", "Chapter 3"), under either preset.
// A label like "프롤로그" is not, so it can stand in for a missing title.
func IsNumberedLabel(label string) bool {
	return IsChapterLabel(label, project.OutlinePresetWebNovel)
}

// tree is a work's outline with the lookups both functions need.
type tree struct {
	byID     map[string]node.Node
	preOrder []node.Node
}

// newTree orders nodes the way ListByProject returns them — grouped by
// parent, by ordinal within a parent — into outline pre-order.
func newTree(nodes []node.Node) tree {
	t := tree{byID: make(map[string]node.Node, len(nodes))}
	children := map[string][]node.Node{}
	for _, n := range nodes {
		t.byID[n.ID] = n
		key := ""
		if n.ParentID != nil {
			key = *n.ParentID
		}
		children[key] = append(children[key], n)
	}
	var walk func(key string)
	walk = func(key string) {
		for _, n := range children[key] {
			t.preOrder = append(t.preOrder, n)
			walk(n.ID)
		}
	}
	walk("")
	return t
}

// own returns the episode-like node a node belongs to, walking up from the
// node itself: the first one whose label is a chapter label, or that is a
// container below the top level. ok is false when no ancestor qualifies.
func (t tree) own(n node.Node, preset string) (node.Node, bool) {
	cur, found := n, true
	for found {
		if IsChapterLabel(cur.Label, preset) {
			return cur, true
		}
		if cur.Kind == node.KindContainer && cur.ParentID != nil {
			return cur, true
		}
		if cur.ParentID == nil {
			break
		}
		cur, found = t.byID[*cur.ParentID]
	}
	return node.Node{}, false
}

// episodes lists the work's episodes from its flat node list.
//
// Every scene belongs to the episode findEpisodeNode gives it; a scene with
// no episode-like ancestor is an episode of its own, as it is in the UI. A
// chapter-labelled container with nothing written in it yet is listed too, so
// a freshly made "3화" shows up before its first scene does. When no scene
// has an episode-like ancestor at all, the work has no episode structure and
// is one episode: a short story.
func episodes(nodes []node.Node, preset string) []Episode {
	t := newTree(nodes)
	if len(t.preOrder) == 0 {
		return nil
	}
	owner := map[string]string{}
	structured := false
	for _, n := range t.preOrder {
		if n.Kind != node.KindLeaf {
			continue
		}
		if ep, ok := t.own(n, preset); ok {
			owner[n.ID] = ep.ID
			structured = true
			continue
		}
		owner[n.ID] = n.ID
	}

	if !structured {
		first := t.preOrder[0]
		whole := Episode{NodeID: first.ID, Label: first.Label, Title: first.Title, Ordinal: 1, Whole: true}
		for _, n := range t.preOrder {
			if n.Kind == node.KindLeaf {
				whole.LeafIDs = append(whole.LeafIDs, n.ID)
			}
		}
		return []Episode{whole}
	}

	leaves := map[string][]string{}
	for _, n := range t.preOrder {
		if n.Kind == node.KindLeaf {
			leaves[owner[n.ID]] = append(leaves[owner[n.ID]], n.ID)
		}
	}
	var out []Episode
	for _, n := range t.preOrder {
		ids, owns := leaves[n.ID]
		if !owns && !(n.Kind == node.KindContainer && IsChapterLabel(n.Label, preset)) {
			continue
		}
		out = append(out, Episode{
			NodeID:  n.ID,
			Label:   n.Label,
			Title:   n.Title,
			Ordinal: len(out) + 1,
			LeafIDs: ids,
		})
	}
	return out
}

// List returns the work's episodes in outline order.
func List(ctx context.Context, nr *node.Repo, projectID, preset string) ([]Episode, error) {
	nodes, err := nr.ListByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return episodes(nodes, preset), nil
}

// LastWritten returns the last episode, in outline order, that has at least
// one scene with word_count > 0. found is false when nothing in the work has
// been written yet. Story-assist Phase 1 uses this as the anchor for a
// writer's focus (the project_focus table), and Phase 4's branch forecast
// uses it as the episode to forecast from — both want "where the writer
// actually is," not the outline's last node.
func LastWritten(ctx context.Context, nr *node.Repo, projectID, preset string) (Episode, bool, error) {
	nodes, err := nr.ListByProject(ctx, projectID)
	if err != nil {
		return Episode{}, false, err
	}
	return lastWrittenFromNodes(nodes, preset)
}

// lastWrittenFromNodes is LastWritten's pure core, taking the node list
// directly so it shares test fixtures with episodes/resolve.
func lastWrittenFromNodes(nodes []node.Node, preset string) (Episode, bool, error) {
	wordCount := make(map[string]int, len(nodes))
	for _, n := range nodes {
		wordCount[n.ID] = n.WordCount
	}
	var last Episode
	found := false
	for _, ep := range episodes(nodes, preset) {
		for _, leafID := range ep.LeafIDs {
			if wordCount[leafID] > 0 {
				last = ep
				found = true
				break
			}
		}
	}
	return last, found, nil
}

// Resolve returns the episode nodeID belongs to.
func Resolve(ctx context.Context, nr *node.Repo, preset, nodeID string) (Episode, error) {
	n, err := nr.Get(ctx, nodeID)
	if errors.Is(err, node.ErrNotFound) {
		return Episode{}, ErrNotFound
	}
	if err != nil {
		return Episode{}, err
	}
	nodes, err := nr.ListByProject(ctx, n.ProjectID)
	if err != nil {
		return Episode{}, err
	}
	return resolve(nodes, preset, nodeID)
}

// resolve finds the episode holding nodeID: the episode itself, one of its
// scenes, or a container between the two. A container above every episode
// (a 권) holds several and so resolves to none.
func resolve(nodes []node.Node, preset, nodeID string) (Episode, error) {
	t := newTree(nodes)
	if _, ok := t.byID[nodeID]; !ok {
		return Episode{}, ErrNotFound
	}
	list := episodes(nodes, preset)
	if len(list) == 1 && list[0].Whole {
		return list[0], nil
	}
	ids := map[string]int{}
	for i, ep := range list {
		ids[ep.NodeID] = i
	}
	cur, found := t.byID[nodeID], true
	for found {
		if i, ok := ids[cur.ID]; ok {
			return list[i], nil
		}
		if cur.ParentID == nil {
			break
		}
		cur, found = t.byID[*cur.ParentID]
	}
	return Episode{}, ErrNotFound
}
