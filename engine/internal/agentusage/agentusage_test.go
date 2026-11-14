package agentusage

import (
	"context"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/store/storetest"
)

func newRepo(t *testing.T) *Repo {
	t.Helper()
	st, err := storetest.Open(context.Background(), t.TempDir()+"/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return NewRepo(st.DB())
}

// Requests add up by kind and model, cache tokens kept apart from input, and
// only what falls inside the window counts.
func TestSummary_sumsByKindAndModelSinceTheWindow(t *testing.T) {
	ctx := context.Background()
	r := newRepo(t)
	for _, e := range []Entry{
		{Kind: "turn", Provider: "anthropic", Model: "big", Input: 10, Output: 5, CacheRead: 900, CacheWrite: 100, At: 2000},
		{Kind: "turn", Provider: "anthropic", Model: "big", Input: 20, Output: 7, CacheRead: 1000, At: 3000},
		{Kind: "self_review", Provider: "anthropic", Model: "small", Input: 30, Output: 1, CacheWrite: 400, At: 3000},
		{Kind: "turn", Provider: "anthropic", Model: "big", Input: 999, Output: 999, At: 500},
	} {
		if err := r.Add(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	got, err := r.Summary(ctx, 1000)
	if err != nil {
		t.Fatal(err)
	}
	want := []Row{
		{Kind: "turn", Provider: "anthropic", Model: "big", Calls: 2, Input: 30, Output: 12, CacheRead: 1900, CacheWrite: 100},
		{Kind: "self_review", Provider: "anthropic", Model: "small", Calls: 1, Input: 30, Output: 1, CacheWrite: 400},
	}
	if len(got.Rows) != len(want) {
		t.Fatalf("rows = %+v, want %+v", got.Rows, want)
	}
	for i := range want {
		if got.Rows[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, got.Rows[i], want[i])
		}
	}
}

// A request with no counted tokens is not a row, and a row needs a kind.
func TestAdd_skipsUncountedRequestsAndRefusesNoKind(t *testing.T) {
	ctx := context.Background()
	r := newRepo(t)
	if err := r.Add(ctx, Entry{Kind: "turn", At: 1}); err != nil {
		t.Fatal(err)
	}
	if err := r.Add(ctx, Entry{Input: 3, At: 1}); err == nil {
		t.Error("a row without a kind was accepted")
	}
	got, err := r.Summary(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rows) != 0 {
		t.Errorf("rows = %+v, want none", got.Rows)
	}
}

func TestPrune_dropsOnlyOlderRows(t *testing.T) {
	ctx := context.Background()
	r := newRepo(t)
	for _, at := range []int64{100, 200, 300} {
		if err := r.Add(ctx, Entry{Kind: "turn", Input: 1, At: at}); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Prune(ctx, 200); err != nil {
		t.Fatal(err)
	}
	got, err := r.Summary(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rows) != 1 || got.Rows[0].Calls != 2 {
		t.Errorf("after prune = %+v, want the 2 rows at or after 200", got.Rows)
	}
}
