package mcphost

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/ledger"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store/storetest"
)

func ledgerDeps(t *testing.T) (context.Context, ToolDeps, string) {
	t.Helper()
	ctx := context.Background()
	st, err := storetest.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	projects := project.NewRepo(st)
	nodes := node.NewRepo(st)
	p, err := projects.Create(ctx, 1, project.NewInput{Title: "작품", LengthTarget: "novel", DefaultPOV: "first"})
	if err != nil {
		t.Fatal(err)
	}
	return ctx, ToolDeps{Projects: projects, Nodes: nodes, Ledger: ledger.NewRepo(st, nodes)}, p.ID
}

// A pin is the writer's. The tool tells the agent so rather than failing
// quietly, and no input on the wire can set one.
func TestAgentCannotChangeOrDeleteAPinnedEntry(t *testing.T) {
	ctx, d, pid := ledgerDeps(t)
	e, err := d.Ledger.Add(ctx, 1, ledger.Entry{ProjectID: pid, Kind: ledger.KindDecision, Body: "규칙"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Ledger.SetPinned(ctx, 2, e.ID, true); err != nil {
		t.Fatal(err)
	}
	body := "바꾼 규칙"
	for name, in := range map[string]editLedgerInput{
		"edit":   {ProjectID: pid, ID: e.ID, Body: &body},
		"delete": {ProjectID: pid, ID: e.ID, Delete: true},
	} {
		res, _, err := d.editLedger(ctx, nil, in)
		if err != nil {
			t.Fatal(err)
		}
		if res == nil || !res.IsError {
			t.Errorf("%s: a pinned entry must be refused", name)
		}
	}
	got, err := d.Ledger.Get(ctx, e.ID)
	if err != nil || got.Body != "규칙" || !got.Pinned {
		t.Fatalf("got %+v, err %v", got, err)
	}
}

func TestListLedgerPagesNewestFirst(t *testing.T) {
	ctx, d, pid := ledgerDeps(t)
	for i := range 40 {
		body := fmt.Sprintf("결정 %02d %s", i, string(make([]rune, 0)))
		for range 60 {
			body += " 내용을 길게 적은 문장"
		}
		if _, err := d.Ledger.Add(ctx, int64(i), ledger.Entry{ProjectID: pid, Kind: ledger.KindDecision, Body: body}); err != nil {
			t.Fatal(err)
		}
	}
	seen, cursor := map[string]bool{}, ""
	first := ""
	for {
		_, out, err := d.listLedger(ctx, nil, listLedgerInput{ProjectID: pid, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		if out.Total != 40 {
			t.Fatalf("total = %d", out.Total)
		}
		for _, e := range out.Entries {
			if first == "" {
				first = e.Body
			}
			if seen[e.ID] {
				t.Fatalf("%s came twice", e.ID)
			}
			seen[e.ID] = true
		}
		if out.NextCursor == "" {
			break
		}
		cursor = out.NextCursor
	}
	if len(seen) != 40 {
		t.Fatalf("read %d of 40", len(seen))
	}
	if first[:len("결정 39")] != "결정 39" {
		t.Fatalf("the newest entry must come first, got %q", first[:12])
	}
}

func TestListLedgerFiltersByQuery(t *testing.T) {
	ctx, d, pid := ledgerDeps(t)
	for _, subject := range []string{"낡은 패", "무관한 사실"} {
		if _, err := d.Ledger.Add(ctx, 1, ledger.Entry{ProjectID: pid, Kind: ledger.KindDecision, Subject: subject, Body: "내용"}); err != nil {
			t.Fatal(err)
		}
	}
	_, out, err := d.listLedger(ctx, nil, listLedgerInput{ProjectID: pid, Query: "낡은"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Entries) != 1 || out.Entries[0].Subject != "낡은 패" {
		t.Fatalf("entries = %+v", out.Entries)
	}
}

func TestEditLedgerReportsAConflictAndSavesAnyway(t *testing.T) {
	ctx, d, pid := ledgerDeps(t)
	subject, attribute, first, second := "주인공", "소지금", "은 넉 냥", "은 열 냥"
	if _, _, err := d.editLedger(ctx, nil, editLedgerInput{ProjectID: pid, Kind: ledger.KindNumber, Subject: &subject, Attribute: &attribute, Body: &first}); err != nil {
		t.Fatal(err)
	}
	res, out, err := d.editLedger(ctx, nil, editLedgerInput{ProjectID: pid, Kind: ledger.KindNumber, Subject: &subject, Attribute: &attribute, Body: &second})
	if err != nil || res != nil {
		t.Fatalf("res %+v, err %v", res, err)
	}
	if out.Entry == nil || len(out.Conflicts) != 1 || out.Conflicts[0].Body != first {
		t.Fatalf("out = %+v", out)
	}
}

func TestEditLedgerSupersedesAFact(t *testing.T) {
	ctx, d, pid := ledgerDeps(t)
	subject, attribute, first, second := "주인공", "소속", "없음", "표국"
	_, out, err := d.editLedger(ctx, nil, editLedgerInput{ProjectID: pid, Kind: ledger.KindNumber, Subject: &subject, Attribute: &attribute, Body: &first})
	if err != nil {
		t.Fatal(err)
	}
	old := out.Entry.ID
	_, out, err = d.editLedger(ctx, nil, editLedgerInput{ProjectID: pid, Kind: ledger.KindNumber, Subject: &subject, Attribute: &attribute, Body: &second, Supersedes: old})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Conflicts) != 0 {
		t.Fatalf("a replaced fact is no conflict: %+v", out.Conflicts)
	}
	got, err := d.Ledger.Get(ctx, old)
	if err != nil || got.SupersededBy != out.Entry.ID {
		t.Fatalf("got %+v, err %v", got, err)
	}
}

func TestSupersedingAPinnedFactIsRefusedBeforeAnythingIsSaved(t *testing.T) {
	ctx, d, pid := ledgerDeps(t)
	e, err := d.Ledger.Add(ctx, 1, ledger.Entry{ProjectID: pid, Kind: ledger.KindDecision, Body: "규칙"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Ledger.SetPinned(ctx, 2, e.ID, true); err != nil {
		t.Fatal(err)
	}
	body := "바꾼 규칙"
	res, _, err := d.editLedger(ctx, nil, editLedgerInput{ProjectID: pid, Kind: ledger.KindDecision, Body: &body, Supersedes: e.ID})
	if err != nil || res == nil || !res.IsError {
		t.Fatalf("res %+v, err %v", res, err)
	}
	all, _ := d.Ledger.List(ctx, ledger.Filter{ProjectID: pid})
	if len(all) != 1 {
		t.Fatalf("entries = %d", len(all))
	}
}
