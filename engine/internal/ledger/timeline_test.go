package ledger

import (
	"testing"

	"github.com/devlikebear/linetta/engine/internal/node"
)

func tlNode(id, parent, kind, label string, ordinal, words int) node.Node {
	n := node.Node{ID: id, Kind: kind, Label: label, Ordinal: ordinal, WordCount: words}
	if parent != "" {
		n.ParentID = &parent
	}
	return n
}

// 1권 holds 1화-2화, 2권 holds 3화-4화. 4화 has nothing written yet.
func timelineNodes() []node.Node {
	return []node.Node{
		tlNode("v2", "", node.KindContainer, "2권", 1, 0),
		tlNode("v1", "", node.KindContainer, "1권", 0, 0),
		tlNode("e1", "v1", node.KindContainer, "1화", 0, 0),
		tlNode("e2", "v1", node.KindContainer, "2화", 1, 0),
		tlNode("e3", "v2", node.KindContainer, "3화", 0, 0),
		tlNode("e4", "v2", node.KindContainer, "4화", 1, 0),
		tlNode("s1", "e1", node.KindLeaf, "씬 1", 0, 100),
		tlNode("s2", "e2", node.KindLeaf, "씬 1", 0, 100),
		tlNode("s3a", "e3", node.KindLeaf, "씬 1", 0, 100),
		tlNode("s3b", "e3", node.KindLeaf, "씬 2", 1, 100),
		tlNode("s4", "e4", node.KindLeaf, "씬 1", 0, 0),
	}
}

func TestBuildTimelinePlacesEachEntryOnTheEpisodeAxis(t *testing.T) {
	entries := []Entry{
		{ID: "key", Kind: KindPromise, Subject: "은빛 열쇠", Status: StatusPaid,
			PlantedNodeID: "s1", PlantedQuote: "열쇠가 빛났다", DueNodeID: "e3", PaidNodeID: "s3b", PaidQuote: "문이 열렸다"},
		{ID: "late", Kind: KindQuestion, Status: StatusOpen, PlantedNodeID: "s1", DueNodeID: "v1"},
		{ID: "waiting", Kind: KindGag, Status: StatusOpen, PlantedNodeID: "s2", DueNodeID: "v2"},
		{ID: "gone", Kind: KindPromise, Status: StatusDropped, PlantedNodeID: "s2"},
		{ID: "old", Kind: KindPromise, Status: StatusOpen, PlantedNodeID: "s1", SupersededBy: "key"},
		{ID: "fact", Kind: KindDecision, Status: StatusOpen, PlantedNodeID: "s1"},
	}
	tl := BuildTimeline(entries, timelineNodes(), "webnovel", nil, "s3a")

	if len(tl.Episodes) != 4 || tl.Episodes[0].Label != "1화" || tl.Episodes[3].Ordinal != 4 {
		t.Fatalf("episodes = %+v", tl.Episodes)
	}
	if tl.Latest != 3 || tl.Current != 3 {
		t.Fatalf("latest = %d, current = %d; want 3 (4화 is empty) and 3", tl.Latest, tl.Current)
	}
	if len(tl.Parts) != 2 || tl.Parts[0].Label != "1권" || tl.Parts[0].From != 1 || tl.Parts[0].To != 2 || tl.Parts[1].From != 3 || tl.Parts[1].To != 4 {
		t.Fatalf("parts = %+v", tl.Parts)
	}

	byID := map[string]TimelineEntry{}
	for _, e := range tl.Entries {
		byID[e.ID] = e
	}
	if len(tl.Entries) != 4 {
		t.Fatalf("entries = %+v; superseded and untracked kinds stay off the map", tl.Entries)
	}
	key := byID["key"]
	if key.Planted.Episode != 1 || key.Planted.Quote != "열쇠가 빛났다" || key.Due.Episode != 3 || key.Paid.Episode != 3 || key.Paid.Quote != "문이 열렸다" {
		t.Errorf("key = %+v %+v %+v", key.Planted, key.Due, key.Paid)
	}
	if key.Overdue || key.OpenFor != 0 {
		t.Errorf("a paid entry is neither overdue nor open: %+v", key)
	}
	// Due by the end of 1권 (2화); 3화 is written, so it is late.
	if late := byID["late"]; late.Due.Episode != 2 || !late.Overdue || late.OpenFor != 2 {
		t.Errorf("late = %+v due %+v", late, late.Due)
	}
	// Due by the end of 2권 (4화): not late yet.
	if w := byID["waiting"]; w.Due.Episode != 4 || w.Overdue || w.OpenFor != 1 {
		t.Errorf("waiting = %+v due %+v", w, w.Due)
	}
	if g := byID["gone"]; g.Overdue || g.OpenFor != 0 || g.Planted.Episode != 2 {
		t.Errorf("gone = %+v", g)
	}
}

func TestBuildTimelineFiltersKindsAndToleratesMissingScenes(t *testing.T) {
	entries := []Entry{
		{ID: "a", Kind: KindPromise, Status: StatusOpen, PlantedNodeID: "deleted-scene"},
		{ID: "b", Kind: KindGag, Status: StatusOpen},
	}
	tl := BuildTimeline(entries, timelineNodes(), "webnovel", []string{KindPromise}, "")
	if len(tl.Entries) != 1 || tl.Entries[0].ID != "a" || tl.Entries[0].Planted != nil || tl.Current != 0 {
		t.Fatalf("timeline = %+v", tl)
	}
	if empty := BuildTimeline(nil, nil, "novel", nil, ""); len(empty.Episodes) != 0 || empty.Entries == nil || empty.Parts == nil {
		t.Fatalf("an empty work still answers with empty lists: %+v", empty)
	}
}
