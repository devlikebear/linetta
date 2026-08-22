package plot

import (
	"context"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

// Builder assembles a Spine from the node/beat/thread repos.
type Builder struct {
	nodes   *node.Repo
	beats   *beat.Repo
	threads *thread.Repo
}

// NewBuilder returns a Builder backed by the given repos.
func NewBuilder(nodes *node.Repo, beats *beat.Repo, threads *thread.Repo) *Builder {
	return &Builder{nodes: nodes, beats: beats, threads: threads}
}

// Build returns the plot spine centered on nodeID. If nodeID is a container (or
// otherwise absent from the leaf ordering), Prev/Next are nil and Current holds
// whatever beats are bound directly to nodeID.
func (b *Builder) Build(ctx context.Context, nodeID string) (Spine, error) {
	cur, err := b.nodes.Get(ctx, nodeID)
	if err != nil {
		return Spine{}, err
	}
	all, err := b.nodes.ListByProject(ctx, cur.ProjectID)
	if err != nil {
		return Spine{}, err
	}
	byID := make(map[string]node.Node, len(all))
	children := map[string][]node.Node{}
	for _, n := range all {
		byID[n.ID] = n
		key := ""
		if n.ParentID != nil {
			key = *n.ParentID
		}
		children[key] = append(children[key], n)
	}
	var leaves []node.Node
	var walk func(parent string)
	walk = func(parent string) {
		for _, c := range children[parent] {
			if c.Kind == "leaf" {
				leaves = append(leaves, c)
			}
			walk(c.ID)
		}
	}
	walk("")

	curIdx := -1
	for i, l := range leaves {
		if l.ID == cur.ID {
			curIdx = i
			break
		}
	}

	threadCache := map[string]thread.Thread{}
	sceneOf := func(n node.Node) (SceneBeats, error) {
		sb := SceneBeats{NodeID: n.ID, Label: node.BreadcrumbLabel(byID, n), Beats: []Beat{}}
		bs, err := b.beats.ListByNode(ctx, n.ID)
		if err != nil {
			return SceneBeats{}, err
		}
		for _, bt := range bs {
			th, ok := threadCache[bt.ThreadID]
			if !ok {
				t, err := b.threads.Get(ctx, bt.ThreadID)
				if err != nil {
					continue // benign: stale thread ref; skip the beat
				}
				th = t
				threadCache[bt.ThreadID] = t
			}
			sb.Beats = append(sb.Beats, Beat{
				ID: bt.ID, ThreadID: bt.ThreadID, ThreadName: th.Name, ThreadColor: th.Color,
				Label: bt.Label, Description: bt.Description, Intensity: bt.Intensity, Ordinal: bt.Ordinal,
			})
		}
		return sb, nil
	}

	out := Spine{}
	if curIdx >= 0 {
		out.Current, err = sceneOf(leaves[curIdx])
	} else {
		out.Current, err = sceneOf(cur)
	}
	if err != nil {
		return Spine{}, err
	}
	if curIdx > 0 {
		prev, err := sceneOf(leaves[curIdx-1])
		if err != nil {
			return Spine{}, err
		}
		out.Prev = &prev
	}
	if curIdx >= 0 && curIdx+1 < len(leaves) {
		next, err := sceneOf(leaves[curIdx+1])
		if err != nil {
			return Spine{}, err
		}
		out.Next = &next
	}
	return out, nil
}
