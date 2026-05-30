package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/plot"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

func newPlotFixture(t *testing.T) (*plot.Builder, string /* leafNodeID */) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	pr := project.NewRepo(s)
	p, err := pr.Create(context.Background(), 1, project.NewInput{
		Title: "T", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	nr := node.NewRepo(s)
	br := beat.NewRepo(s)
	tr := thread.NewRepo(s)

	builder := plot.NewBuilder(nr, br, tr)

	// project.Create seeds a root leaf node; use it as our leaf.
	leafID := *p.LastOpenedNodeID

	return builder, leafID
}

func TestPlotSpinePanel_EmptyNodeID(t *testing.T) {
	builder, _ := newPlotFixture(t)
	h := PlotSpinePanel(builder)

	_, err := h(context.Background(), json.RawMessage(`{"node_id":""}`))
	if err == nil {
		t.Fatal("expected error for empty node_id, got nil")
	}
	me, ok := err.(*rpc.MethodError)
	if !ok {
		t.Fatalf("expected *rpc.MethodError, got %T: %v", err, err)
	}
	if me.Code != rpc.CodeInvalidParams {
		t.Errorf("code = %d, want CodeInvalidParams (%d)", me.Code, rpc.CodeInvalidParams)
	}
}

func TestPlotSpinePanel_UnknownNodeID(t *testing.T) {
	builder, _ := newPlotFixture(t)
	h := PlotSpinePanel(builder)

	_, err := h(context.Background(), json.RawMessage(`{"node_id":"00000000-0000-0000-0000-000000000000"}`))
	if err == nil {
		t.Fatal("expected error for unknown node_id, got nil")
	}
	me, ok := err.(*rpc.MethodError)
	if !ok {
		t.Fatalf("expected *rpc.MethodError, got %T: %v", err, err)
	}
	if me.Code != rpc.CodeInvalidParams {
		t.Errorf("code = %d, want CodeInvalidParams (%d)", me.Code, rpc.CodeInvalidParams)
	}
}

func TestPlotSpinePanel_ValidLeaf(t *testing.T) {
	builder, leafID := newPlotFixture(t)
	h := PlotSpinePanel(builder)

	res, err := h(context.Background(), json.RawMessage(`{"node_id":"`+leafID+`"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var spine struct {
		Current struct {
			NodeID string `json:"node_id"`
		} `json:"current"`
	}
	if err := json.Unmarshal(res, &spine); err != nil {
		t.Fatalf("unmarshal: %v (raw=%s)", err, string(res))
	}
	if spine.Current.NodeID == "" {
		t.Errorf("current.node_id is empty; want %q", leafID)
	}
}
