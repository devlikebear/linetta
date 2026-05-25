package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/snapshot"
	"github.com/devlikebear/linetta/engine/internal/store"
)

type nodeFixture struct {
	store *store.Store
	proj  *project.Repo
	nodes *node.Repo
	snaps *snapshot.Repo
	pID   string
	nID   string
}

func newNodeFixture(t *testing.T) nodeFixture {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	pr := project.NewRepo(s)
	p, err := pr.Create(context.Background(), 1000, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return nodeFixture{
		store: s, proj: pr, nodes: node.NewRepo(s), snaps: snapshot.NewRepo(s),
		pID: p.ID, nID: *p.LastOpenedNodeID,
	}
}

func TestGetNodeHandler(t *testing.T) {
	f := newNodeFixture(t)
	h := GetNode(f.nodes)
	res, err := h(context.Background(), json.RawMessage(`{"id":"`+f.nID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var n node.Node
	if err := json.Unmarshal(res, &n); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if n.Label != "씬 1" {
		t.Errorf("label = %q", n.Label)
	}
}

func TestUpdateNodeContentHandler_createsAutosaveSnapshotOnFirstSave(t *testing.T) {
	f := newNodeFixture(t)
	clock := int64(10_000)
	h := UpdateNodeContent(f.nodes, f.snaps, func() int64 { return clock })

	res, err := h(context.Background(), json.RawMessage(`{"id":"`+f.nID+`","doc":"{\"type\":\"doc\",\"content\":[{\"type\":\"paragraph\",\"content\":[{\"type\":\"text\",\"text\":\"파도 소리\"}]}]}"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var out node.Node
	if err := json.Unmarshal(res, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.WordCount != 5 {
		t.Errorf("word_count = %d, want 5", out.WordCount)
	}

	// First save → snapshot must be created.
	at, ok, err := f.snaps.LatestAutosaveTime(context.Background(), f.nID)
	if err != nil || !ok {
		t.Fatalf("expected an autosave snapshot to exist; ok=%v err=%v", ok, err)
	}
	if at != 10_000 {
		t.Errorf("autosave at = %d, want 10000", at)
	}
}

func TestUpdateNodeContentHandler_noSnapshotWithin60s(t *testing.T) {
	f := newNodeFixture(t)
	clock := int64(10_000)
	h := UpdateNodeContent(f.nodes, f.snaps, func() int64 { return clock })

	if _, err := h(context.Background(), json.RawMessage(`{"id":"`+f.nID+`","doc":"{}"}`)); err != nil {
		t.Fatalf("save 1: %v", err)
	}
	clock = 30_000 // 20 seconds later
	if _, err := h(context.Background(), json.RawMessage(`{"id":"`+f.nID+`","doc":"{}"}`)); err != nil {
		t.Fatalf("save 2: %v", err)
	}
	// Should still only be the first snapshot (no new one within 60s).
	at, _, _ := f.snaps.LatestAutosaveTime(context.Background(), f.nID)
	if at != 10_000 {
		t.Errorf("autosave at = %d, want 10000 (snapshot should not have been refreshed)", at)
	}

	clock = 80_000 // > 60s after last snapshot
	if _, err := h(context.Background(), json.RawMessage(`{"id":"`+f.nID+`","doc":"{}"}`)); err != nil {
		t.Fatalf("save 3: %v", err)
	}
	at, _, _ = f.snaps.LatestAutosaveTime(context.Background(), f.nID)
	if at != 80_000 {
		t.Errorf("autosave at = %d, want 80000", at)
	}
}

func TestSetLastOpenedHandler(t *testing.T) {
	f := newNodeFixture(t)
	h := SetLastOpened(f.nodes, func() int64 { return 9999 })
	params := json.RawMessage(`{"project_id":"` + f.pID + `","node_id":"` + f.nID + `"}`)
	if _, err := h(context.Background(), params); err != nil {
		t.Fatalf("handler: %v", err)
	}
	got, err := f.proj.Get(context.Background(), f.pID)
	if err != nil {
		t.Fatalf("project Get: %v", err)
	}
	if got.LastOpenedNodeID == nil || *got.LastOpenedNodeID != f.nID {
		t.Errorf("last_opened = %v", got.LastOpenedNodeID)
	}
}
