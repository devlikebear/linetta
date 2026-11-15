// Package relgraph builds the relationship graph (#386 B2): the work's
// entities, the relationships that hold at one episode, and who has appeared
// with whom by then. Episodes and positions are computed here, not in the
// app, so every client draws the same graph.
package relgraph

import (
	"cmp"
	"slices"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/episode"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/relationship"
)

// MaxCoAppearances caps the co-appearance edges: the pairs seen together
// most, so a long work's graph stays readable.
const MaxCoAppearances = 60

// Episode is one tick of the slider.
type Episode struct {
	Ordinal int    `json:"ordinal"`
	NodeID  string `json:"node_id"`
	Label   string `json:"label"`
	Title   string `json:"title,omitempty"`
}

// Node is an entity, sized by how many scenes it has appeared in by AsOf.
type Node struct {
	EntityID string `json:"entity_id"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	// Appearances counts the scenes up to and including the AsOf episode.
	Appearances int `json:"appearances"`
	// FirstEpisode is the episode it first appears in; 0 when it has not
	// appeared by AsOf.
	FirstEpisode int `json:"first_episode,omitempty"`
}

// Step is one label a relationship held: from SinceEpisode (0: the start)
// on, resting on Quote in scene SinceNodeID.
type Step struct {
	Label        string `json:"label"`
	InverseLabel string `json:"inverse_label,omitempty"`
	SinceEpisode int    `json:"since_episode,omitempty"`
	SinceNodeID  string `json:"since_node_id,omitempty"`
	Quote        string `json:"quote,omitempty"`
}

// Edge is a relationship as it stands at AsOf. A pair is one edge, its
// partner's label as InverseLabel.
type Edge struct {
	ID           string `json:"id"`
	FromID       string `json:"from_id"`
	ToID         string `json:"to_id"`
	Label        string `json:"label"`
	InverseLabel string `json:"inverse_label,omitempty"`
	// Changed marks a relationship that began or took this label in the AsOf
	// episode.
	Changed bool `json:"changed,omitempty"`
	// Steps are the labels it has held up to AsOf, oldest first.
	Steps []Step `json:"steps"`
}

// CoAppearance is two entities seen in the same scenes, Scenes times by AsOf.
type CoAppearance struct {
	FromID string `json:"from_id"`
	ToID   string `json:"to_id"`
	Scenes int    `json:"scenes"`
}

// Graph is the whole answer.
type Graph struct {
	Episodes []Episode `json:"episodes"`
	// Latest is the last episode with prose; AsOf the one the graph shows.
	Latest        int            `json:"latest"`
	AsOf          int            `json:"as_of"`
	Nodes         []Node         `json:"nodes"`
	Edges         []Edge         `json:"edges"`
	CoAppearances []CoAppearance `json:"co_appearances"`
}

// Input is what Build reads.
type Input struct {
	Nodes         []node.Node
	Preset        string
	Entities      []entity.Entity
	Relationships []relationship.Relationship
	Appearances   []mention.Appearance
	// AsOf is the episode to show; 0 means the episode CurrentNodeID is in,
	// or the latest written one.
	AsOf          int
	CurrentNodeID string
}

// Build lays the work out at one episode.
func Build(in Input) Graph {
	sorted := slices.Clone(in.Nodes)
	slices.SortStableFunc(sorted, func(a, b node.Node) int { return a.Ordinal - b.Ordinal })
	eps := episode.FromNodes(sorted, in.Preset)
	words := episode.WordCounts(in.Nodes)

	g := Graph{Episodes: []Episode{}, Nodes: []Node{}, Edges: []Edge{}, CoAppearances: []CoAppearance{}}
	of := map[string]int{} // scene → episode
	lastLeaf := map[int]string{}
	for _, ep := range eps {
		g.Episodes = append(g.Episodes, Episode{Ordinal: ep.Ordinal, NodeID: ep.NodeID, Label: ep.Label, Title: ep.Title})
		for _, leaf := range ep.LeafIDs {
			of[leaf] = ep.Ordinal
			lastLeaf[ep.Ordinal] = leaf
			if words[leaf] > 0 {
				g.Latest = max(g.Latest, ep.Ordinal)
			}
		}
	}
	g.AsOf = in.AsOf
	if g.AsOf <= 0 || g.AsOf > len(eps) {
		g.AsOf = cmp.Or(of[in.CurrentNodeID], g.Latest, len(eps))
	}

	// Who has appeared, and with whom, by the end of AsOf.
	scenes := map[string]map[string]bool{} // entity → scenes
	byScene := map[string][]string{}
	first := map[string]int{}
	for _, a := range in.Appearances {
		ep, ok := of[a.NodeID]
		if !ok || ep > g.AsOf {
			continue
		}
		if scenes[a.EntityID] == nil {
			scenes[a.EntityID] = map[string]bool{}
		}
		scenes[a.EntityID][a.NodeID] = true
		byScene[a.NodeID] = append(byScene[a.NodeID], a.EntityID)
		if f, seen := first[a.EntityID]; !seen || ep < f {
			first[a.EntityID] = ep
		}
	}
	for _, e := range in.Entities {
		g.Nodes = append(g.Nodes, Node{EntityID: e.ID, Kind: e.Kind, Name: e.Name, Appearances: len(scenes[e.ID]), FirstEpisode: first[e.ID]})
	}

	// The relationships as they stand at the end of AsOf: its last scene, or
	// the last one before it when nothing is written in it yet.
	at := ""
	for ep := g.AsOf; ep > 0 && at == ""; ep-- {
		at = lastLeaf[ep]
	}
	g.Edges = edges(in.Relationships, in.Nodes, at, of, g.AsOf)
	g.CoAppearances = coAppearances(byScene)
	return g
}

func edges(rels []relationship.Relationship, nodes []node.Node, at string, of map[string]int, asOf int) []Edge {
	byID := make(map[string]relationship.Relationship, len(rels))
	for _, r := range rels {
		byID[r.ID] = r
	}
	held := relationship.AsOf(rels, nodes, at)
	// A pair's partner, as it stands at the same scene.
	partner := map[string]relationship.Relationship{}
	for _, r := range held {
		if r.PairID != nil {
			for _, o := range held {
				if o.ID != r.ID && o.PairID != nil && *o.PairID == *r.PairID {
					partner[r.ID] = o
				}
			}
		}
	}
	since := func(r relationship.Relationship) int {
		if r.SinceNodeID == nil {
			return 0
		}
		return of[*r.SinceNodeID]
	}
	out := []Edge{}
	seenPair := map[string]bool{}
	for _, r := range held {
		if r.PairID != nil {
			if seenPair[*r.PairID] {
				continue
			}
			seenPair[*r.PairID] = true
		}
		e := Edge{ID: r.ID, FromID: r.FromID, ToID: r.ToID, Label: r.Label, Steps: []Step{}}
		if p, ok := partner[r.ID]; ok {
			e.InverseLabel = p.Label
		}
		// Begun or changed in the shown episode: either way it is new there.
		e.Changed = r.SinceNodeID != nil && since(r) == asOf && asOf > 0
		var chain []relationship.Relationship
		visited := map[string]bool{}
		for cur, ok := r, true; ok && !visited[cur.ID]; {
			visited[cur.ID] = true
			chain = append(chain, cur)
			if cur.SupersedesID == nil {
				break
			}
			cur, ok = byID[*cur.SupersedesID]
		}
		for i := len(chain) - 1; i >= 0; i-- {
			c := chain[i]
			step := Step{Label: c.Label, SinceEpisode: since(c), Quote: c.SinceQuote}
			if c.SinceNodeID != nil {
				step.SinceNodeID = *c.SinceNodeID
			}
			if c.PairID != nil {
				for _, o := range rels {
					if o.ID != c.ID && o.PairID != nil && *o.PairID == *c.PairID {
						step.InverseLabel = o.Label
					}
				}
			}
			e.Steps = append(e.Steps, step)
		}
		out = append(out, e)
	}
	return out
}

func coAppearances(byScene map[string][]string) []CoAppearance {
	type pair struct{ a, b string }
	count := map[pair]int{}
	for _, ents := range byScene {
		slices.Sort(ents)
		ents = slices.Compact(ents)
		for i := range ents {
			for j := i + 1; j < len(ents); j++ {
				count[pair{ents[i], ents[j]}]++
			}
		}
	}
	out := []CoAppearance{}
	for p, n := range count {
		if n >= 2 {
			out = append(out, CoAppearance{FromID: p.a, ToID: p.b, Scenes: n})
		}
	}
	slices.SortFunc(out, func(x, y CoAppearance) int {
		return cmp.Or(y.Scenes-x.Scenes, cmp.Compare(x.FromID, y.FromID), cmp.Compare(x.ToID, y.ToID))
	})
	if len(out) > MaxCoAppearances {
		out = out[:MaxCoAppearances]
	}
	return out
}
