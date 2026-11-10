package ledger

import (
	"fmt"
	"slices"
	"testing"
)

func settlements(f fixture, since int64, on *bool) *Settlements {
	return NewSettlements(f.repo.s, f.nodes, func() (int64, bool) { return since, *on })
}

// Five episodes, written at 100..104; settlement switched on at 102.
func TestPendingCountsOnlyWhatWasWrittenSinceSettlementWasOn(t *testing.T) {
	f := setup(t, 5)
	on := true
	s := settlements(f, 102, &on)
	ids := func(idx ...int) []string {
		var out []string
		for _, i := range idx {
			out = append(out, f.scenes[i].ID)
		}
		return out
	}

	got, err := s.Pending(f.ctx, f.pid, f.scenes[4].ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := ids(2, 3); !slices.Equal(got, want) {
		t.Fatalf("pending before 5화 = %v, want 3화 and 4화 only — 1화 and 2화 predate the switch", got)
	}
	if got, _ := s.Pending(f.ctx, f.pid, ""); !slices.Equal(got, ids(2, 3, 4)) {
		t.Errorf("pending in the whole work = %v", got)
	}
	if got, _ := s.Pending(f.ctx, f.pid, f.part.ID); len(got) != 0 {
		t.Errorf("pending before the part's first scene = %v, want none", got)
	}

	// 3화 settled at its version leaves the queue; a timeout does not.
	v3, _ := s.Version(f.ctx, f.scenes[2].ID)
	v4, _ := s.Version(f.ctx, f.scenes[3].ID)
	if err := s.Record(f.ctx, f.pid, f.scenes[2].ID, v3, SettlementDone, 200); err != nil {
		t.Fatal(err)
	}
	if err := s.Record(f.ctx, f.pid, f.scenes[3].ID, v4, SettlementTimeout, 200); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Pending(f.ctx, f.pid, f.scenes[4].ID); !slices.Equal(got, ids(3)) {
		t.Errorf("after settling 3화 = %v, want 4화", got)
	}

	// A later pass that did not finish does not unsettle the same version.
	if err := s.Record(f.ctx, f.pid, f.scenes[2].ID, v3, SettlementFailed, 300); err != nil {
		t.Fatal(err)
	}
	if outcome, _, _, _ := s.Settled(f.ctx, f.scenes[2].ID); outcome != SettlementDone {
		t.Errorf("3화 after a failed rerun = %q, want it still done", outcome)
	}

	// Editing the prose makes it a new version, waiting again — even 1화,
	// which predated the switch until it was touched.
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"고쳐 쓴 문장."}]}]}`
	if err := f.nodes.UpdateContent(f.ctx, f.scenes[2].ID, doc, 400); err != nil {
		t.Fatal(err)
	}
	if err := f.nodes.UpdateContent(f.ctx, f.scenes[0].ID, doc, 400); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Pending(f.ctx, f.pid, f.scenes[4].ID); !slices.Equal(got, ids(0, 2, 3)) {
		t.Errorf("after edits = %v, want 1화, 3화 and 4화", got)
	}

	on = false
	if got, _ := s.Pending(f.ctx, f.pid, f.scenes[4].ID); len(got) != 0 {
		t.Errorf("pending with settlement off = %v, want none", got)
	}
	if err := s.Record(f.ctx, f.pid, f.scenes[0].ID, 1, "maybe", 1); err == nil {
		t.Error("an unknown outcome was recorded")
	}
}

// Relinking a name as a mention rewrites the document and bumps the version,
// but the words are the same: a settled scene stays settled. A pass recorded
// after the scene had already moved on knows only the version, so the same
// relink sends that scene back to the queue.
func TestPendingIgnoresAVersionThatChangedNoWords(t *testing.T) {
	f := setup(t, 3)
	on := true
	s := settlements(f, 0, &on)
	relinked := func(i int) string {
		return fmt.Sprintf(`{"type":"doc","content":[{"type":"paragraph","content":[`+
			`{"type":"mention","attrs":{"id":"e1","label":"명패"}},`+
			`{"type":"text","text":"의 금박이 %d화에서 벗겨졌다."}]}]}`, i+1)
	}

	v1, _ := s.Version(f.ctx, f.scenes[0].ID)
	if err := s.Record(f.ctx, f.pid, f.scenes[0].ID, v1, SettlementDone, 200); err != nil {
		t.Fatal(err)
	}
	// 2화 changed while its pass ran: the pass is recorded at the old version.
	v2, _ := s.Version(f.ctx, f.scenes[1].ID)
	moved := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"명패의 금박이 2화에서 벗겨졌다."}]}]}`
	if err := f.nodes.UpdateContent(f.ctx, f.scenes[1].ID, moved, 150); err != nil {
		t.Fatal(err)
	}
	if err := s.Record(f.ctx, f.pid, f.scenes[1].ID, v2, SettlementDone, 200); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		if err := f.nodes.UpdateContent(f.ctx, f.scenes[i].ID, relinked(i), 300); err != nil {
			t.Fatal(err)
		}
	}
	if after, _ := s.Version(f.ctx, f.scenes[0].ID); after == v1 {
		t.Fatal("the relink did not bump 1화's version; the test proves nothing")
	}
	got, err := s.Pending(f.ctx, f.pid, "")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{f.scenes[1].ID, f.scenes[2].ID}; !slices.Equal(got, want) {
		t.Errorf("pending after relinking = %v, want 2화 and 3화 — 1화 reads the same as when it was settled", got)
	}

	// A real change to the words waits again.
	changed := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"명패는 끝내 벗겨지지 않았다."}]}]}`
	if err := f.nodes.UpdateContent(f.ctx, f.scenes[0].ID, changed, 400); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Pending(f.ctx, f.pid, ""); !slices.Contains(got, f.scenes[0].ID) {
		t.Errorf("pending after rewriting 1화 = %v, want 1화 back in the queue", got)
	}
}
