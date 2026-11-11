package ledger

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store/storetest"
)

// A new work starts as scenes at its top level, with no part around them. Such
// a scene belongs to no arc, so a promise planted in one is still owed, and its
// answer still a secret, scenes later. Found by the 2026-11-09 synthetic run:
// every unpinned question vanished from the brief past the scene it was planted
// in, because each top-level scene counted as an arc of its own.
func TestBriefKeepsPromisesInAWorkWithNoParts(t *testing.T) {
	ctx := context.Background()
	st, err := storetest.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	p, err := project.NewRepo(st).Create(ctx, 1, project.NewInput{Title: "T", LengthTarget: "series", DefaultPOV: "first"})
	if err != nil {
		t.Fatal(err)
	}
	nodes := node.NewRepo(st)
	if err := nodes.Delete(ctx, *p.LastOpenedNodeID, 2); err != nil {
		t.Fatal(err)
	}
	var scenes []node.Node
	for i := range 4 {
		n, err := nodes.CreateRoot(ctx, p.ID, node.KindLeaf, fmt.Sprintf("%d화", i+1), "", int64(10+i))
		if err != nil {
			t.Fatal(err)
		}
		scenes = append(scenes, n)
	}
	repo := NewRepo(st, nodes)
	if _, err := repo.Add(ctx, 1, Entry{ProjectID: p.ID, Kind: KindQuestion, Body: "아버지를 죽인 자는 누구인가",
		Answer: "백 선장", PlantedNodeID: scenes[0].ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Add(ctx, 1, Entry{ProjectID: p.ID, Kind: KindPromise, Body: "등대의 푸른빛", PlantedNodeID: scenes[0].ID}); err != nil {
		t.Fatal(err)
	}

	b, err := repo.ContextLedger(ctx, p.ID, scenes[2].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Secrets) != 1 || !strings.Contains(b.Secrets[0], "백 선장") {
		t.Fatalf("secrets two scenes on = %q", b.Secrets)
	}
	owed := strings.Join(append(append([]string{}, b.Open...), b.Due...), "\n")
	if !strings.Contains(owed, "등대의 푸른빛") || !strings.Contains(owed, "아버지를 죽인 자는 누구인가") {
		t.Fatalf("owed two scenes on = %q", owed)
	}
	if b.Omitted != 0 {
		t.Fatalf("omitted = %d, want nothing left out", b.Omitted)
	}
}
