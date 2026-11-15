package relgraph

import (
	"fmt"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/mention"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
)

func ptr(s string) *string { return &s }

// Three episodes of one scene each; the third is not written yet.
func work() []node.Node {
	var out []node.Node
	for i := 1; i <= 3; i++ {
		ep := fmt.Sprintf("ep%d", i)
		words := 0
		if i < 3 {
			words = 100
		}
		out = append(out,
			node.Node{ID: ep, Kind: node.KindContainer, Label: fmt.Sprintf("%d화", i), Ordinal: i},
			node.Node{ID: fmt.Sprintf("s%d", i), ParentID: ptr(ep), Kind: node.KindLeaf, Label: "씬 1", Ordinal: 1, WordCount: words},
		)
	}
	return out
}

func input(asOf int) Input {
	return Input{
		Nodes:  work(),
		Preset: project.OutlinePresetWebNovel,
		Entities: []entity.Entity{
			{ID: "a", Kind: "character", Name: "하린"}, {ID: "b", Kind: "character", Name: "유나"}, {ID: "c", Kind: "character", Name: "서진"},
		},
		Relationships: []relationship.Relationship{
			{ID: "r1", FromID: "a", ToID: "b", Label: "동료", PairID: ptr("p1"), UntilNodeID: ptr("s2")},
			{ID: "r1i", FromID: "b", ToID: "a", Label: "동료", PairID: ptr("p1"), UntilNodeID: ptr("s2")},
			{ID: "r2", FromID: "a", ToID: "b", Label: "적", PairID: ptr("p2"), SinceNodeID: ptr("s2"), SinceQuote: "넌 적이야.", SupersedesID: ptr("r1")},
			{ID: "r2i", FromID: "b", ToID: "a", Label: "원수", PairID: ptr("p2"), SinceNodeID: ptr("s2"), SupersedesID: ptr("r1i")},
		},
		Appearances: []mention.Appearance{
			{NodeID: "s1", EntityID: "a"}, {NodeID: "s1", EntityID: "b"},
			{NodeID: "s2", EntityID: "a"}, {NodeID: "s2", EntityID: "b"}, {NodeID: "s2", EntityID: "c"},
		},
		AsOf: asOf,
	}
}

func TestBuildReadsTheGraphAtAnEpisode(t *testing.T) {
	g := Build(input(1))
	if len(g.Episodes) != 3 || g.Latest != 2 || g.AsOf != 1 {
		t.Fatalf("axis = %d episodes, latest %d, as of %d", len(g.Episodes), g.Latest, g.AsOf)
	}
	if len(g.Edges) != 1 || g.Edges[0].Label != "동료" || g.Edges[0].InverseLabel != "동료" || g.Edges[0].Changed {
		t.Fatalf("episode 1 edges = %+v", g.Edges)
	}
	// 서진 has not appeared yet.
	if n := g.Nodes[2]; n.Appearances != 0 || n.FirstEpisode != 0 {
		t.Fatalf("서진 at episode 1 = %+v", n)
	}
	if len(g.CoAppearances) != 0 {
		t.Fatalf("one shared scene is not a co-appearance yet: %+v", g.CoAppearances)
	}

	g = Build(input(2))
	e := g.Edges[0]
	if len(g.Edges) != 1 || e.Label != "적" || e.InverseLabel != "원수" || !e.Changed {
		t.Fatalf("episode 2 edge = %+v", e)
	}
	if len(e.Steps) != 2 || e.Steps[0].Label != "동료" || e.Steps[1].SinceEpisode != 2 || e.Steps[1].Quote != "넌 적이야." || e.Steps[1].InverseLabel != "원수" {
		t.Fatalf("steps = %+v", e.Steps)
	}
	if n := g.Nodes[0]; n.Appearances != 2 || n.FirstEpisode != 1 {
		t.Fatalf("하린 = %+v", n)
	}
	if len(g.CoAppearances) != 1 || g.CoAppearances[0] != (CoAppearance{FromID: "a", ToID: "b", Scenes: 2}) {
		t.Fatalf("co-appearances = %+v", g.CoAppearances)
	}
}

func TestBuildMarksATieThatBeginsInTheEpisode(t *testing.T) {
	in := input(2)
	in.Relationships = append(in.Relationships, relationship.Relationship{ID: "r3", FromID: "c", ToID: "a", Label: "스승", SinceNodeID: ptr("s2")})
	g := Build(in)
	for _, e := range g.Edges {
		if e.ID == "r3" && (!e.Changed || len(e.Steps) != 1 || e.Steps[0].SinceEpisode != 2) {
			t.Fatalf("a tie begun in episode 2 = %+v", e)
		}
	}
	in.AsOf = 1
	if g = Build(in); len(g.Edges) != 1 {
		t.Fatalf("before it begins, episode 1 edges = %+v", g.Edges)
	}
}

func TestBuildDefaultsToTheOpenEpisodeThenTheLatest(t *testing.T) {
	in := input(0)
	if g := Build(in); g.AsOf != 2 {
		t.Fatalf("no episode asked: as of %d, want the latest written (2)", g.AsOf)
	}
	in.CurrentNodeID = "s1"
	if g := Build(in); g.AsOf != 1 {
		t.Fatalf("scene s1 open: as of %d, want 1", g.AsOf)
	}
}

func TestBuildOnAnUnwrittenEpisodeKeepsTheLastRelationships(t *testing.T) {
	in := input(3)
	in.Nodes = in.Nodes[:5] // episode 3 has no scene
	g := Build(in)
	if g.AsOf != 3 || len(g.Edges) != 1 || g.Edges[0].Label != "적" || g.Edges[0].Changed {
		t.Fatalf("episode 3 = as of %d, edges %+v", g.AsOf, g.Edges)
	}
}
