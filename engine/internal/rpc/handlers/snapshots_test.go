package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/snapshot"
)

func TestCreateManualSnapshotHandler(t *testing.T) {
	f := newNodeFixture(t)
	h := CreateManualSnapshot(f.snaps, func() int64 { return 7777 })

	res, err := h(context.Background(), json.RawMessage(`{"node_id":"`+f.nID+`","doc":"{\"v\":1}"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got snapshot.Snapshot
	if err := json.Unmarshal(res, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Reason != snapshot.ReasonManual {
		t.Errorf("reason = %q", got.Reason)
	}
	if got.CreatedAt != 7777 {
		t.Errorf("created_at = %d", got.CreatedAt)
	}
}

func TestListForNodeHandler(t *testing.T) {
	f := newNodeFixture(t)
	ctx := context.Background()
	_, _ = f.snaps.Create(ctx, f.nID, `{"v":1}`, snapshot.ReasonManual, 1000)
	_, _ = f.snaps.Create(ctx, f.nID, `{"v":2}`, snapshot.ReasonAutosave, 2000)

	h := ListSnapshotsForNode(f.snaps)
	res, err := h(ctx, json.RawMessage(`{"node_id":"`+f.nID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var entries []snapshot.Entry
	if err := json.Unmarshal(res, &entries); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries", len(entries))
	}
}

func TestRestoreSnapshotHandler_writesCurrentAsManualThenRestores(t *testing.T) {
	f := newNodeFixture(t)
	ctx := context.Background()
	// Seed current content + an older snapshot.
	if err := f.nodes.UpdateContent(ctx, f.nID,
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"현재"}]}]}`, 5000); err != nil {
		t.Fatalf("seed update: %v", err)
	}
	older, _ := f.snaps.Create(ctx, f.nID,
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"옛 본문"}]}]}`,
		snapshot.ReasonManual, 1000)

	h := RestoreSnapshot(f.nodes, f.snaps, func() int64 { return 9999 })
	res, err := h(ctx, json.RawMessage(`{"snapshot_id":"`+older.ID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got node.Node
	if err := json.Unmarshal(res, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ContentDoc == nil || !strings.Contains(*got.ContentDoc, "옛 본문") {
		t.Errorf("restore did not apply; doc=%v", got.ContentDoc)
	}
	// New manual snapshot capturing the pre-restore "현재" body must exist.
	entries, err := f.snaps.ListForNode(ctx, f.nID)
	if err != nil {
		t.Fatalf("ListForNode: %v", err)
	}
	var hasPreRestoreManual bool
	for _, e := range entries {
		if e.Reason == snapshot.ReasonManual && e.CreatedAt == 9999 {
			hasPreRestoreManual = true
		}
	}
	if !hasPreRestoreManual {
		t.Errorf("missing pre-restore manual snapshot at 9999; got %+v", entries)
	}
}
