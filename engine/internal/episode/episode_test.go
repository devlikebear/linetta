package episode

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

const webnovel = project.OutlinePresetWebNovel

func n(id, parent string, ordinal int, kind, label string) node.Node {
	out := node.Node{ID: id, ProjectID: "p", Ordinal: ordinal, Kind: kind, Label: label}
	if parent != "" {
		out.ParentID = &parent
	}
	return out
}

func leaf(id, parent string, ordinal int, label string) node.Node {
	return n(id, parent, ordinal, node.KindLeaf, label)
}

func container(id, parent string, ordinal int, label string) node.Node {
	return n(id, parent, ordinal, node.KindContainer, label)
}

func resolvedID(t *testing.T, nodes []node.Node, preset, id string) string {
	t.Helper()
	ep, err := resolve(nodes, preset, id)
	if err != nil {
		t.Fatalf("resolve(%s): %v", id, err)
	}
	return ep.NodeID
}

// The cases below are outlineEpisode.test.ts's, so the Go port and the UI
// answer the same way.

func TestResolve_sceneInsideAnEpisodeContainer(t *testing.T) {
	nodes := []node.Node{
		container("arc-1", "", 0, "1권"),
		container("ep-1", "arc-1", 0, "1화"),
		leaf("scene-1", "ep-1", 0, "씬 1"),
	}
	if got := resolvedID(t, nodes, webnovel, "scene-1"); got != "ep-1" {
		t.Fatalf("got %q, want ep-1", got)
	}
}

func TestResolve_leafEpisodeDirectlyUnderAnArc(t *testing.T) {
	nodes := []node.Node{
		container("arc-1", "", 0, "1권"),
		leaf("ep-leaf", "arc-1", 0, "1화"),
		leaf("ep-leaf-2", "arc-1", 1, "2화"),
	}
	if got := resolvedID(t, nodes, webnovel, "ep-leaf"); got != "ep-leaf" {
		t.Fatalf("got %q, want ep-leaf", got)
	}
}

func TestResolve_rootSeedSceneIsTheWholeWork(t *testing.T) {
	nodes := []node.Node{leaf("seed-1", "", 0, "씬 1")}
	ep, err := resolve(nodes, webnovel, "seed-1")
	if err != nil {
		t.Fatal(err)
	}
	if ep.NodeID != "seed-1" || !ep.Whole {
		t.Fatalf("got %+v, want the whole work as seed-1", ep)
	}
}

func TestResolve_unlabelledMidLevelContainerIsTheEpisode(t *testing.T) {
	nodes := []node.Node{
		container("arc-1", "", 0, "1권"),
		container("mid-1", "arc-1", 0, "막간"),
		leaf("scene-1", "mid-1", 0, "씬 1"),
	}
	if got := resolvedID(t, nodes, webnovel, "scene-1"); got != "mid-1" {
		t.Fatalf("got %q, want mid-1", got)
	}
}

func TestResolve_missingNode(t *testing.T) {
	nodes := []node.Node{leaf("seed-1", "", 0, "씬 1")}
	if _, err := resolve(nodes, webnovel, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestResolve_arcHoldingSeveralEpisodesResolvesToNone(t *testing.T) {
	nodes := []node.Node{
		container("arc-1", "", 0, "1권"),
		leaf("ep-1", "arc-1", 0, "1화"),
		leaf("ep-2", "arc-1", 1, "2화"),
	}
	if _, err := resolve(nodes, webnovel, "arc-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestResolve_presetDecidesWhatAChapterLabelIs(t *testing.T) {
	// "1화" is an episode label only under the web-novel preset. Under the
	// novel preset a root-level "1화" is just a scene, and a work made only
	// of those has no episode structure.
	nodes := []node.Node{leaf("a", "", 0, "1화"), leaf("b", "", 1, "2화")}
	if got := resolvedID(t, nodes, webnovel, "b"); got != "b" {
		t.Fatalf("webnovel: got %q, want b", got)
	}
	ep, err := resolve(nodes, project.OutlinePresetNovel, "b")
	if err != nil {
		t.Fatal(err)
	}
	if !ep.Whole || ep.NodeID != "a" {
		t.Fatalf("novel: got %+v, want the whole work as a", ep)
	}
}

func TestList_containerAndLeafEpisodesInOutlineOrder(t *testing.T) {
	// ListByProject's order: roots first, then grouped by parent.
	nodes := []node.Node{
		container("arc-1", "", 0, "1권"),
		container("arc-2", "", 1, "2권"),
		container("ep-1", "arc-1", 0, "1화"),
		leaf("ep-2", "arc-1", 1, "2화"),
		container("ep-3", "arc-2", 0, "3화"),
		leaf("s-1", "ep-1", 0, "씬 1"),
		leaf("s-2", "ep-1", 1, "씬 2"),
	}
	got := episodes(nodes, webnovel)
	want := []Episode{
		{NodeID: "ep-1", Label: "1화", Ordinal: 1, LeafIDs: []string{"s-1", "s-2"}},
		{NodeID: "ep-2", Label: "2화", Ordinal: 2, LeafIDs: []string{"ep-2"}},
		// Nothing written in 3화 yet; it is listed all the same.
		{NodeID: "ep-3", Label: "3화", Ordinal: 3},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestList_shortStoryIsOneEpisode(t *testing.T) {
	nodes := []node.Node{
		container("part", "", 0, "본문"),
		leaf("s-1", "part", 0, "씬 1"),
		leaf("s-2", "part", 1, "씬 2"),
	}
	got := episodes(nodes, project.OutlinePresetNovel)
	want := []Episode{{NodeID: "part", Label: "본문", Ordinal: 1, Whole: true, LeafIDs: []string{"s-1", "s-2"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestList_strayRootSceneIsAnEpisodeOfItsOwn(t *testing.T) {
	nodes := []node.Node{
		leaf("note", "", 0, "작가의 말"),
		container("arc", "", 1, "1권"),
		leaf("ep-1", "arc", 0, "1화"),
	}
	got := episodes(nodes, webnovel)
	if len(got) != 2 || got[0].NodeID != "note" || got[1].NodeID != "ep-1" {
		t.Fatalf("got %+v, want note then ep-1", got)
	}
}

func TestIsNumberedLabel(t *testing.T) {
	for label, want := range map[string]bool{
		"3화": true, "제3화": true, "Chapter 3": true, "ep.3": false, "EP 3": true,
		"1장 시작": true, "프롤로그": false, "외전": false, "3화차": false,
	} {
		if got := IsNumberedLabel(label); got != want {
			t.Errorf("IsNumberedLabel(%q) = %v, want %v", label, got, want)
		}
	}
}

func TestListAndResolve_readTheStore(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	p, err := project.NewRepo(s).Create(ctx, 1000, project.NewInput{
		Title: "T", Genres: []string{}, LengthTarget: "series", DefaultPOV: "first",
		OutlinePreset: webnovel,
	})
	if err != nil {
		t.Fatal(err)
	}
	nr := node.NewRepo(s)
	first := *p.LastOpenedNodeID
	second, err := nr.CreateSibling(ctx, first, node.KindLeaf, "2화", "", 2000)
	if err != nil {
		t.Fatal(err)
	}

	list, err := List(ctx, nr, p.ID, webnovel)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].NodeID != first || list[1].NodeID != second.ID || list[1].Ordinal != 2 {
		t.Fatalf("List = %+v", list)
	}
	ep, err := Resolve(ctx, nr, webnovel, second.ID)
	if err != nil || ep.NodeID != second.ID {
		t.Fatalf("Resolve = %+v, %v", ep, err)
	}
	if _, err := Resolve(ctx, nr, webnovel, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Resolve(missing) err = %v", err)
	}
}
