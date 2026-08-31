package restore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/backup"
	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/note"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/snapshot"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

// TestMergeProjectFromBackup builds a work carrying every copied table, snapshots
// the library, then merges the work back in and verifies the copy is complete
// and purely additive.
func TestMergeProjectFromBackup(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(home, "library.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	now := int64(1000)
	projects := project.NewRepo(st)
	nodes := node.NewRepo(st)
	entities := entity.NewRepo(st)
	relationships := relationship.NewRepo(st)
	threads := thread.NewRepo(st)
	beats := beat.NewRepo(st)
	notes := note.NewRepo(st)
	facts := fact.NewRepo(st)
	snaps := snapshot.NewRepo(st)

	p, err := projects.Create(ctx, now, project.NewInput{
		Title: "원본", Genres: []string{"sf"}, LengthTarget: "short", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	leafID := *p.LastOpenedNodeID
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"본문"}]}]}`
	if err := nodes.UpdateContent(ctx, leafID, doc, now); err != nil {
		t.Fatalf("content: %v", err)
	}
	child, err := nodes.CreateSibling(ctx, leafID, "leaf", "씬 2", "", now)
	if err != nil {
		t.Fatalf("sibling: %v", err)
	}
	hero, err := entities.Create(ctx, now, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "주인공"})
	if err != nil {
		t.Fatalf("entity: %v", err)
	}
	villain, err := entities.Create(ctx, now, entity.NewInput{ProjectID: p.ID, Kind: "character", Name: "적수"})
	if err != nil {
		t.Fatalf("entity2: %v", err)
	}
	if _, err := relationships.CreatePair(ctx, relationship.NewPairInput{
		ProjectID: p.ID, FromID: hero.ID, ToID: villain.ID, Label: "맞선다", InverseLabel: "몰아붙인다",
	}); err != nil {
		t.Fatalf("relationship pair: %v", err)
	}
	th, err := threads.Create(ctx, thread.NewInput{ProjectID: p.ID, Name: "주 갈등", Color: "#333"})
	if err != nil {
		t.Fatalf("thread: %v", err)
	}
	if _, err := beats.Create(ctx, beat.NewInput{ThreadID: th.ID, NodeID: &child.ID, Label: "격돌", Intensity: 3}); err != nil {
		t.Fatalf("beat: %v", err)
	}
	if _, err := notes.Create(ctx, note.NewInput{NodeID: leafID, Anchor: 1, Body: "메모"}, now); err != nil {
		t.Fatalf("note: %v", err)
	}
	if _, err := facts.Create(ctx, now, fact.NewInput{
		ProjectID: p.ID, Claim: "사실", Result: "확인됨", Status: fact.StatusVerified,
		Sources: []fact.SourceInput{{URL: "https://example.com", AccessedAt: now}},
	}); err != nil {
		t.Fatalf("fact: %v", err)
	}
	if _, err := snaps.Create(ctx, leafID, doc, "manual", now); err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	res, err := backup.RunManualRecovery(ctx, st.DB(), home, time.UnixMilli(2000))
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	list, err := ListBackups(home)
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("no backups listed")
	}
	if err := ValidateBackupPath(home, res.Path); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if err := ValidateBackupPath(home, filepath.Join(home, "library.db")); err == nil {
		t.Fatal("path outside backups folder accepted")
	}

	peeked, err := PeekProjects(ctx, res.Path, "")
	if err != nil {
		t.Fatalf("peek: %v", err)
	}
	if len(peeked) != 1 || peeked[0].Title != "원본" {
		t.Fatalf("peeked = %+v", peeked)
	}

	merged, err := MergeProject(ctx, st, res.Path, "", p.ID, " (복원)", time.UnixMilli(3000))
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if merged.Title != "원본 (복원)" {
		t.Errorf("merged title = %q", merged.Title)
	}
	if merged.ProjectID == p.ID {
		t.Fatal("merge reused the original project id")
	}

	// Original untouched.
	orig, err := projects.Get(ctx, p.ID)
	if err != nil || orig.Title != "원본" {
		t.Fatalf("original mutated: %+v err=%v", orig, err)
	}

	got, err := projects.Get(ctx, merged.ProjectID)
	if err != nil {
		t.Fatalf("get merged: %v", err)
	}
	if len(got.Genres) != 1 || got.Genres[0] != "sf" {
		t.Errorf("genres = %v", got.Genres)
	}
	if got.LastOpenedNodeID == nil {
		t.Error("last_opened_node_id not remapped")
	}

	mergedNodes, err := nodes.ListByProject(ctx, merged.ProjectID)
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	if len(mergedNodes) != 2 {
		t.Fatalf("nodes = %d, want 2", len(mergedNodes))
	}
	var mergedLeafID, mergedChildID string
	for _, n := range mergedNodes {
		if n.ID == leafID || n.ID == child.ID {
			t.Fatalf("node id %s reused", n.ID)
		}
		switch n.Label {
		case "씬 2":
			mergedChildID = n.ID
		default:
			mergedLeafID = n.ID
			if n.ContentDoc == nil || *n.ContentDoc != doc {
				t.Errorf("content lost on %q", n.Label)
			}
		}
	}

	ents, err := entities.ListByProject(ctx, merged.ProjectID)
	if err != nil || len(ents) != 2 {
		t.Fatalf("entities = %d err=%v", len(ents), err)
	}
	rels, err := relationships.ListByProject(ctx, merged.ProjectID)
	if err != nil || len(rels) != 2 {
		t.Fatalf("relationships = %d err=%v", len(rels), err)
	}
	if rels[0].PairID == nil || rels[1].PairID == nil || *rels[0].PairID != *rels[1].PairID {
		t.Errorf("pair_id not preserved as a pair: %+v", rels)
	}

	ths, err := threads.ListByProject(ctx, merged.ProjectID, true)
	if err != nil || len(ths) != 1 {
		t.Fatalf("threads = %d err=%v", len(ths), err)
	}
	bts, err := beats.ListByThread(ctx, ths[0].ID)
	if err != nil || len(bts) != 1 {
		t.Fatalf("beats = %d err=%v", len(bts), err)
	}
	if bts[0].NodeID == nil || *bts[0].NodeID != mergedChildID {
		t.Errorf("beat node = %v, want %s", bts[0].NodeID, mergedChildID)
	}

	nts, err := notes.ListForNode(ctx, mergedLeafID)
	if err != nil || len(nts) != 1 || nts[0].Body != "메모" {
		t.Fatalf("notes = %+v err=%v", nts, err)
	}

	cards, err := facts.List(ctx, fact.ListFilter{ProjectID: merged.ProjectID})
	if err != nil || len(cards) != 1 || len(cards[0].Sources) != 1 {
		t.Fatalf("cards = %+v err=%v", cards, err)
	}

	snapEntries, err := snaps.ListForNode(ctx, mergedLeafID)
	if err != nil || len(snapEntries) == 0 {
		t.Fatalf("snapshots = %d err=%v", len(snapEntries), err)
	}
}
