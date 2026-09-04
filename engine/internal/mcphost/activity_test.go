//go:build !mobile

package mcphost

import (
	"context"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/store"
)

func newActivityRepo(t *testing.T) *ActivityRepo {
	t.Helper()
	st, err := store.Open(context.Background(), t.TempDir()+"/test.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return NewActivityRepo(st.DB())
}

// A row written by an external client carries no run id and reads back as
// "external" — which is also what every row written before 0017 becomes.
func TestRecord_defaultsToExternal(t *testing.T) {
	r := newActivityRepo(t)
	if err := r.Record(context.Background(), ActivityEntry{Tool: "linetta_read_scene", OK: true}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	got, err := r.List(context.Background(), 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1", len(got))
	}
	if got[0].Source != SourceExternal {
		t.Errorf("Source = %q, want %q", got[0].Source, SourceExternal)
	}
	if got[0].RunID != "" {
		t.Errorf("RunID = %q, want empty", got[0].RunID)
	}
}

// The built-in agent's rows are distinguishable and carry the turn they
// belong to, which is what the panel groups its undo button by.
func TestRecord_keepsSourceAndRunID(t *testing.T) {
	r := newActivityRepo(t)
	if err := r.Record(context.Background(), ActivityEntry{
		Tool: "linetta_write_scene", OK: true, Source: SourceAgent, RunID: "run-7",
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	got, err := r.List(context.Background(), 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got[0].Source != SourceAgent || got[0].RunID != "run-7" {
		t.Errorf("entry = %+v, want source=agent run_id=run-7", got[0])
	}
}
