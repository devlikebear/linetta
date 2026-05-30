package plot

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

func openTestStore(t *testing.T) (*store.Store, project.Project) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	pr := project.NewRepo(s)
	p, err := pr.Create(context.Background(), 1, project.NewInput{
		Title: "Test Novel", Genres: []string{"SF"}, LengthTarget: "short", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("project.Create: %v", err)
	}
	return s, p
}

func TestSpinePrevCurrentNext(t *testing.T) {
	ctx := context.Background()
	s, p := openTestStore(t)

	nodes := node.NewRepo(s)
	beats := beat.NewRepo(s)
	threads := thread.NewRepo(s)
	builder := NewBuilder(nodes, beats, threads)

	// The project was created with a first leaf "씬 1"; its ID is LastOpenedNodeID.
	firstLeafID := *p.LastOpenedNodeID

	// Add two sibling leaves after the first.
	leaf2, err := nodes.CreateSibling(ctx, firstLeafID, "leaf", "씬 2", "", 2)
	if err != nil {
		t.Fatalf("CreateSibling leaf2: %v", err)
	}
	leaf3, err := nodes.CreateSibling(ctx, leaf2.ID, "leaf", "씬 3", "", 3)
	if err != nil {
		t.Fatalf("CreateSibling leaf3: %v", err)
	}

	// Create a thread.
	th, err := threads.Create(ctx, thread.NewInput{ProjectID: p.ID, Name: "주인공 arc", Color: "#ff0000"})
	if err != nil {
		t.Fatalf("thread.Create: %v", err)
	}

	// Create one beat bound to each leaf.
	_, err = beats.Create(ctx, beat.NewInput{ThreadID: th.ID, NodeID: &firstLeafID, Label: "발단", Intensity: 1})
	if err != nil {
		t.Fatalf("beat for leaf1: %v", err)
	}
	secondLeafID := leaf2.ID
	_, err = beats.Create(ctx, beat.NewInput{ThreadID: th.ID, NodeID: &secondLeafID, Label: "전개", Intensity: 2})
	if err != nil {
		t.Fatalf("beat for leaf2: %v", err)
	}
	thirdLeafID := leaf3.ID
	_, err = beats.Create(ctx, beat.NewInput{ThreadID: th.ID, NodeID: &thirdLeafID, Label: "절정", Intensity: 3})
	if err != nil {
		t.Fatalf("beat for leaf3: %v", err)
	}

	// Build spine centered on the second leaf.
	sp, err := builder.Build(ctx, secondLeafID)
	if err != nil {
		t.Fatalf("Build(second): %v", err)
	}

	// Current should be the second leaf.
	if sp.Current.NodeID != secondLeafID {
		t.Errorf("Current.NodeID = %q, want %q", sp.Current.NodeID, secondLeafID)
	}
	if len(sp.Current.Beats) != 1 {
		t.Errorf("Current beats len = %d, want 1", len(sp.Current.Beats))
	} else {
		if sp.Current.Beats[0].Label != "전개" {
			t.Errorf("Current beat label = %q, want \"전개\"", sp.Current.Beats[0].Label)
		}
		if sp.Current.Beats[0].ThreadName != "주인공 arc" {
			t.Errorf("ThreadName = %q, want \"주인공 arc\"", sp.Current.Beats[0].ThreadName)
		}
		if sp.Current.Beats[0].ThreadColor != "#ff0000" {
			t.Errorf("ThreadColor = %q, want \"#ff0000\"", sp.Current.Beats[0].ThreadColor)
		}
	}

	// Prev should be the first leaf.
	if sp.Prev == nil {
		t.Fatal("Prev is nil, want first leaf")
	}
	if sp.Prev.NodeID != firstLeafID {
		t.Errorf("Prev.NodeID = %q, want %q", sp.Prev.NodeID, firstLeafID)
	}

	// Next should be the third leaf.
	if sp.Next == nil {
		t.Fatal("Next is nil, want third leaf")
	}
	if sp.Next.NodeID != thirdLeafID {
		t.Errorf("Next.NodeID = %q, want %q", sp.Next.NodeID, thirdLeafID)
	}

	// Build on first leaf → Prev must be nil.
	spFirst, err := builder.Build(ctx, firstLeafID)
	if err != nil {
		t.Fatalf("Build(first): %v", err)
	}
	if spFirst.Prev != nil {
		t.Errorf("first leaf Prev = %+v, want nil", spFirst.Prev)
	}
	if spFirst.Next == nil || spFirst.Next.NodeID != secondLeafID {
		t.Errorf("first leaf Next.NodeID = %v, want %q", spFirst.Next, secondLeafID)
	}

	// Build on last leaf → Next must be nil.
	spLast, err := builder.Build(ctx, thirdLeafID)
	if err != nil {
		t.Fatalf("Build(last): %v", err)
	}
	if spLast.Next != nil {
		t.Errorf("last leaf Next = %+v, want nil", spLast.Next)
	}
	if spLast.Prev == nil || spLast.Prev.NodeID != secondLeafID {
		t.Errorf("last leaf Prev.NodeID = %v, want %q", spLast.Prev, secondLeafID)
	}
}

func TestSpineEmptyBeats(t *testing.T) {
	ctx := context.Background()
	s, p := openTestStore(t)

	nodes := node.NewRepo(s)
	beats := beat.NewRepo(s)
	threads := thread.NewRepo(s)
	builder := NewBuilder(nodes, beats, threads)

	// Project has only the auto-created first leaf; no beats.
	firstLeafID := *p.LastOpenedNodeID

	sp, err := builder.Build(ctx, firstLeafID)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if sp.Current.NodeID != firstLeafID {
		t.Errorf("Current.NodeID = %q, want %q", sp.Current.NodeID, firstLeafID)
	}
	// Beats must be an empty (non-nil) slice.
	if sp.Current.Beats == nil {
		t.Error("Current.Beats is nil, want empty slice")
	}
	if len(sp.Current.Beats) != 0 {
		t.Errorf("Current.Beats len = %d, want 0", len(sp.Current.Beats))
	}
	if sp.Prev != nil {
		t.Errorf("Prev = %+v, want nil for only leaf", sp.Prev)
	}
	if sp.Next != nil {
		t.Errorf("Next = %+v, want nil for only leaf", sp.Next)
	}
}
