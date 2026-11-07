package mcphost

import (
	"testing"

	"github.com/devlikebear/linetta/engine/internal/ledger"
)

// An agent can write a question's answer and point a knows fact at it, and the
// listing shows the answer: whoever reads the ledger reads it for the writer.
func TestAgentRecordsAnAnswerAndWhatAKnowsFactIsAbout(t *testing.T) {
	ctx, d, pid := ledgerDeps(t)
	kind, body, answer := ledger.KindQuestion, "하람은 누구인가", "왕자다"
	_, out, err := d.editLedger(ctx, nil, editLedgerInput{ProjectID: pid, Kind: kind, Body: &body, Answer: &answer})
	if err != nil || out.Entry == nil || out.Entry.Answer != answer {
		t.Fatalf("question = %+v, err %v", out.Entry, err)
	}
	q := *out.Entry

	subject, attr, knows := "서진", ledger.AttributeKnows, "하람의 정체를 앎"
	yes := ledger.KnowsYes
	_, out, err = d.editLedger(ctx, nil, editLedgerInput{ProjectID: pid, Kind: ledger.KindKnowledge, Subject: &subject,
		Attribute: &attr, Body: &knows, About: &q.ID, ValueNum: &yes})
	if err != nil || out.Entry == nil || out.Entry.About != q.ID {
		t.Fatalf("knows = %+v, err %v", out.Entry, err)
	}

	res, _, err := d.editLedger(ctx, nil, editLedgerInput{ProjectID: pid, Kind: ledger.KindKnowledge, Subject: &subject,
		Attribute: &attr, Body: &knows, About: &q.ID})
	if err != nil || res == nil || !res.IsError {
		t.Fatalf("a knows fact about a question without 1 or 0 must be refused: %+v, %v", res, err)
	}

	_, list, err := d.listLedger(ctx, nil, listLedgerInput{ProjectID: pid, Kinds: []string{ledger.KindQuestion}})
	if err != nil || len(list.Entries) != 1 || list.Entries[0].Answer != answer {
		t.Fatalf("listing = %+v, err %v", list.Entries, err)
	}
}

// Once the writer pins a question, its answer is theirs.
func TestAgentCannotChangeTheAnswerOfAPinnedQuestion(t *testing.T) {
	ctx, d, pid := ledgerDeps(t)
	q, err := d.Ledger.Add(ctx, 1, ledger.Entry{ProjectID: pid, Kind: ledger.KindQuestion, Body: "하람은 누구인가", Answer: "왕자다"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Ledger.SetPinned(ctx, 2, q.ID, true); err != nil {
		t.Fatal(err)
	}
	other := "거지다"
	res, _, err := d.editLedger(ctx, nil, editLedgerInput{ProjectID: pid, ID: q.ID, Answer: &other})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || !res.IsError {
		t.Fatal("the answer of a pinned question must be refused")
	}
	if got, _ := d.Ledger.Get(ctx, q.ID); got.Answer != "왕자다" {
		t.Fatalf("answer = %q", got.Answer)
	}
}
