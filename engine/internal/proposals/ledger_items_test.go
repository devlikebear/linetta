package proposals

import (
	"context"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/ledger"
	"github.com/devlikebear/linetta/engine/internal/storyops"
)

func problemText(ps []Problem) string {
	var b strings.Builder
	for _, p := range ps {
		b.WriteString(p.Message + "\n")
	}
	return b.String()
}

// A promise proposed with its planting quote waits in the inbox and lands in
// the ledger when accepted, the item's ledger fields surviving the store.
func TestLedgerItem_addsAPromiseWhenAccepted(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	res, problems, err := e.svc.Submit(ctx, now(), SubmitInput{
		ProjectID: e.project.ID,
		Purpose:   PurposeOrganize,
		Items: []ItemInput{{Ledger: &LedgerItem{
			Kind: ledger.KindPromise, Subject: " 은빛 검 ", Body: "검을 뽑은 손의 비밀",
			PlantedNodeID: e.sceneID, PlantedQuote: "오른손으로 검을 뽑았다",
		}}},
	}, SourceAgent)
	if err != nil || len(problems) != 0 {
		t.Fatalf("submit: err=%v problems=%s", err, problemText(problems))
	}
	list, err := e.svc.List(ctx, e.project.ID, []string{StatusPending})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Purpose != PurposeOrganize || len(list[0].Items) != 1 {
		t.Fatalf("inbox = %+v", list)
	}
	stored := list[0].Items[0]
	if stored.Ledger == nil || stored.Ledger.Subject != "은빛 검" || stored.Op.Type != "" {
		t.Fatalf("stored item = %+v (ledger %+v)", stored, stored.Ledger)
	}
	if got := e.svc.SummarizeItem(ctx, stored, list[0].Items); got != "복선 추가: 은빛 검 — 검을 뽑은 손의 비밀" || ItemOpType(stored) != LedgerOpType {
		t.Errorf("summary = %q, op type = %q", got, ItemOpType(stored))
	}
	if entries, _ := e.ledger.List(ctx, ledger.Filter{ProjectID: e.project.ID}); len(entries) != 0 {
		t.Fatalf("a proposal wrote to the ledger before it was accepted: %+v", entries)
	}

	out, err := e.svc.Accept(ctx, now()+1, e.project.ID, []string{res.Items[0].ID}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 1 || out.Items[0].Status != StatusAccepted || out.Items[0].Result.CreatedID == "" {
		t.Fatalf("accept = %+v", out.Items)
	}
	got, err := e.ledger.Get(ctx, out.Items[0].Result.CreatedID)
	if err != nil || got.Kind != ledger.KindPromise || got.PlantedQuote != "오른손으로 검을 뽑았다" || got.Source != ledger.SourceAgent {
		t.Fatalf("ledger entry = %+v err=%v", got, err)
	}
}

// Marking a promise paid goes through the same checks linetta_edit_ledger
// makes: the scene that pays it is required, and its quote has to be there.
func TestLedgerItem_marksAPromisePaidAndChecksItsScene(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	entry, err := e.ledger.Add(ctx, now(), ledger.Entry{
		ProjectID: e.project.ID, Kind: ledger.KindPromise, Body: "검의 주인", PlantedNodeID: e.sceneID,
	})
	if err != nil {
		t.Fatal(err)
	}
	submit := func(l LedgerItem) ([]Problem, SubmitResult) {
		t.Helper()
		res, problems, err := e.svc.Submit(ctx, now(), SubmitInput{ProjectID: e.project.ID, Items: []ItemInput{{Ledger: &l}}}, SourceAgent)
		if err != nil {
			t.Fatal(err)
		}
		return problems, res
	}

	if problems, _ := submit(LedgerItem{EntryID: entry.ID, Status: ledger.StatusPaid}); !strings.Contains(problemText(problems), "the scene that pays it") {
		t.Errorf("paid with no scene = %s", problemText(problems))
	}
	if problems, _ := submit(LedgerItem{EntryID: entry.ID, Status: ledger.StatusPaid, PaidNodeID: e.sceneID, PaidQuote: "왼손으로 쥐었다"}); !strings.Contains(problemText(problems), "quote is not in that scene") {
		t.Errorf("paid with a quote not in the scene = %s", problemText(problems))
	}
	problems, res := submit(LedgerItem{EntryID: entry.ID, Status: ledger.StatusPaid, PaidNodeID: e.sceneID, PaidQuote: "두 번째 문단"})
	if len(problems) != 0 {
		t.Fatalf("a good paid item was refused: %s", problemText(problems))
	}
	if got := e.svc.SummarizeItem(ctx, res.Items[0], res.Items); got != "회수로 표시: 검의 주인" {
		t.Errorf("summary = %q", got)
	}
	out, err := e.svc.Accept(ctx, now()+1, e.project.ID, []string{res.Items[0].ID}, nil)
	if err != nil || out.Items[0].Status != StatusAccepted || out.Items[0].Result.CreatedID != entry.ID {
		t.Fatalf("accept = %+v err=%v", out.Items, err)
	}
	got, _ := e.ledger.Get(ctx, entry.ID)
	if got.Status != ledger.StatusPaid || got.PaidNodeID != e.sceneID || got.Body != "검의 주인" {
		t.Fatalf("entry after paid = %+v", got)
	}
}

func TestLedgerItem_refusesWhatTheLedgerWouldNot(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	other, err := e.ledger.Add(ctx, now(), ledger.Entry{ProjectID: e.project.ID, Kind: ledger.KindDecision, Body: "결정"})
	if err != nil {
		t.Fatal(err)
	}
	_, problems, err := e.svc.Submit(ctx, now(), SubmitInput{
		ProjectID: e.project.ID,
		Items: []ItemInput{
			{Ledger: &LedgerItem{Kind: ledger.KindNumber, Body: "수치"}},
			{Ledger: &LedgerItem{Kind: ledger.KindPromise}},
			{Ledger: &LedgerItem{EntryID: "missing", Status: ledger.StatusDropped}},
			{Ledger: &LedgerItem{EntryID: other.ID, Kind: ledger.KindPromise}},
			{Op: storyops.Op{Type: "create_entity", Kind: "character", Name: "하린"}, Ledger: &LedgerItem{Kind: ledger.KindGag, Body: "개그"}},
		},
	}, SourceAgent)
	if err != nil {
		t.Fatal(err)
	}
	text := problemText(problems)
	for _, want := range []string{
		`item 1: ledger kind "number" cannot be proposed`,
		"item 2: ",
		`item 3: ledger entry "missing" is not in this work`,
		"item 4: an entry's kind cannot be changed",
		"item 5: give op or ledger, not both",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("problems lack %q:\n%s", want, text)
		}
	}

	_, problems, err = e.svc.Submit(ctx, now(), SubmitInput{
		ProjectID: e.project.ID, Purpose: PurposeForecast, ForecastID: e.savedSelectedForecast(t).ID,
		Items: []ItemInput{{Ledger: &LedgerItem{Kind: ledger.KindGag, Body: "개그"}}},
	}, SourceAgent)
	if err != nil || !strings.Contains(problemText(problems), "a forecast batch carries outline items only") {
		t.Fatalf("forecast with a ledger item: err=%v %s", err, problemText(problems))
	}
}

// The scene can change between proposing and accepting; accept checks again.
func TestLedgerItem_failsAtAcceptWhenTheQuoteIsGone(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	res, problems, err := e.svc.Submit(ctx, now(), SubmitInput{
		ProjectID: e.project.ID,
		Items: []ItemInput{{Ledger: &LedgerItem{
			Kind: ledger.KindQuestion, Body: "누구의 검인가", PlantedNodeID: e.sceneID, PlantedQuote: "두 번째 문단",
		}}},
	}, SourceAgent)
	if err != nil || len(problems) != 0 {
		t.Fatalf("submit: %v %s", err, problemText(problems))
	}
	doc, err := storyops.PlainTextToTiptapDoc("완전히 다시 쓴 장면이다.")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.nodes.UpdateContent(ctx, e.sceneID, doc, 1_002); err != nil {
		t.Fatal(err)
	}
	out, err := e.svc.Accept(ctx, now()+1, e.project.ID, []string{res.Items[0].ID}, nil)
	if err != nil || out.Items[0].Status != StatusFailed || !strings.Contains(out.Items[0].Result.Error, "quote is not in that scene") {
		t.Fatalf("accept after the scene changed = %+v err=%v", out.Items, err)
	}
}
