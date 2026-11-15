package relationship

import (
	"context"
	"errors"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/node"
)

// scenes adds n top-level scenes to the fixture's work, in reading order.
func (f fixture) scenes(t *testing.T, n int) []string {
	t.Helper()
	nr := node.NewRepo(f.s)
	ids := make([]string, n)
	for i := range ids {
		sc, err := nr.CreateRoot(context.Background(), f.pID, node.KindLeaf, "", "장면", int64(3000+i))
		if err != nil {
			t.Fatalf("create scene: %v", err)
		}
		ids[i] = sc.ID
	}
	return ids
}

func (f fixture) nodes(t *testing.T) []node.Node {
	t.Helper()
	all, err := node.NewRepo(f.s).ListByProject(context.Background(), f.pID)
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	return all
}

func labelsAt(rels []Relationship, all []node.Node, at string) []string {
	var out []string
	for _, r := range AsOf(rels, all, at) {
		out = append(out, r.Label)
	}
	return out
}

func TestChangePairEndsBothRowsAndStartsSuccessors(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	sc := f.scenes(t, 3)
	pair, err := f.r.CreatePair(ctx, NewPairInput{ProjectID: f.pID, FromID: f.a, ToID: f.b, Label: "동료", InverseLabel: "동료", Notes: "같은 부대"})
	if err != nil {
		t.Fatalf("CreatePair: %v", err)
	}
	next, err := f.r.Change(ctx, ChangeInput{ID: pair[0].ID, AtNodeID: sc[1], Label: "적", Quote: "이제 너와 나는 적이다."})
	if err != nil {
		t.Fatalf("Change: %v", err)
	}
	if len(next) != 2 || next[0].Label != "적" || next[1].Label != "동료" {
		t.Fatalf("successors = %+v, want 적 and the partner's old label", next)
	}
	if next[0].Notes != "같은 부대" || *next[0].SupersedesID != pair[0].ID || *next[1].SupersedesID != pair[1].ID {
		t.Fatalf("successor carries notes and names what it supersedes: %+v", next)
	}
	if next[0].PairID == nil || next[1].PairID == nil || *next[0].PairID != *next[1].PairID || *next[0].PairID == *pair[0].PairID {
		t.Fatalf("successors are a new pair: %+v", next)
	}
	if *next[0].SinceNodeID != sc[1] || next[0].SinceQuote != "이제 너와 나는 적이다." {
		t.Fatalf("successor starts at the scene with its quote: %+v", next[0])
	}
	for _, id := range []string{pair[0].ID, pair[1].ID} {
		old, _ := f.r.Get(ctx, id)
		if old.UntilNodeID == nil || *old.UntilNodeID != sc[1] {
			t.Fatalf("old row %s should end at the change scene: %+v", id, old)
		}
	}
	if _, err := f.r.Change(ctx, ChangeInput{ID: pair[0].ID, AtNodeID: sc[2], Label: "친구"}); !errors.Is(err, ErrAlreadyChanged) {
		t.Fatalf("changing an ended row: err = %v, want ErrAlreadyChanged", err)
	}

	rels, _ := f.r.ListByEntity(ctx, f.a)
	all := f.nodes(t)
	if got := labelsAt(rels, all, sc[0]); len(got) != 1 || got[0] != "동료" {
		t.Fatalf("before the change = %v, want [동료]", got)
	}
	for _, at := range []string{sc[1], sc[2], ""} {
		if got := labelsAt(rels, all, at); len(got) != 1 || got[0] != "적" {
			t.Fatalf("at %q = %v, want [적]", at, got)
		}
	}
}

func TestChangeInFlashbackHoldsThereAlone(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	sc := f.scenes(t, 3)
	if err := node.NewRepo(f.s).SetFlashback(ctx, sc[1], true); err != nil {
		t.Fatal(err)
	}
	rel, _ := f.r.CreateOne(ctx, NewInput{ProjectID: f.pID, FromID: f.a, ToID: f.b, Label: "적"})
	if _, err := f.r.Change(ctx, ChangeInput{ID: rel.ID, AtNodeID: sc[1], Label: "친구"}); err != nil {
		t.Fatal(err)
	}
	rels, _ := f.r.ListByProject(ctx, f.pID)
	all := f.nodes(t)
	want := map[string]string{sc[0]: "적", sc[1]: "친구", sc[2]: "적", "": "적"}
	for at, label := range want {
		if got := labelsAt(rels, all, at); len(got) != 1 || got[0] != label {
			t.Fatalf("at %q = %v, want [%s]", at, got, label)
		}
	}
}

func TestDeletedChangeSceneKeepsTheSuccessor(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	sc := f.scenes(t, 2)
	rel, _ := f.r.CreateOne(ctx, NewInput{ProjectID: f.pID, FromID: f.a, ToID: f.b, Label: "동료"})
	if _, err := f.r.Change(ctx, ChangeInput{ID: rel.ID, AtNodeID: sc[1], Label: "적"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB().ExecContext(ctx, `DELETE FROM nodes WHERE id = ?`, sc[1]); err != nil {
		t.Fatal(err)
	}
	rels, _ := f.r.ListByProject(ctx, f.pID)
	if got := labelsAt(rels, f.nodes(t), sc[0]); len(got) != 1 || got[0] != "적" {
		t.Fatalf("with the change scene gone = %v, want only the successor", got)
	}
}

func TestCreateFromSceneStartsThere(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	sc := f.scenes(t, 2)
	if _, err := f.r.CreateOne(ctx, NewInput{ProjectID: f.pID, FromID: f.a, ToID: f.b, Label: "연인", SinceNodeID: sc[1], SinceQuote: "좋아해."}); err != nil {
		t.Fatal(err)
	}
	rels, _ := f.r.ListByProject(ctx, f.pID)
	all := f.nodes(t)
	if got := labelsAt(rels, all, sc[0]); len(got) != 0 {
		t.Fatalf("before it starts = %v, want none", got)
	}
	if got := labelsAt(rels, all, sc[1]); len(got) != 1 {
		t.Fatalf("from its scene = %v, want [연인]", got)
	}
}

func TestSetSpanWritesAndClears(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	sc := f.scenes(t, 1)
	rel, _ := f.r.CreateOne(ctx, NewInput{ProjectID: f.pID, FromID: f.a, ToID: f.b, Label: "적"})
	if err := f.r.SetSpan(ctx, SpanInput{ID: rel.ID, UntilNodeID: &sc[0], SinceQuote: "q"}); err != nil {
		t.Fatal(err)
	}
	got, _ := f.r.Get(ctx, rel.ID)
	if got.UntilNodeID == nil || *got.UntilNodeID != sc[0] || got.SinceNodeID != nil || got.SinceQuote != "q" {
		t.Fatalf("span = %+v", got)
	}
	if err := f.r.SetSpan(ctx, SpanInput{ID: "missing"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing row: %v", err)
	}
}
