package ledger

import (
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
