package handlers

import (
	"context"
	"encoding/json"
	"testing"

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
