package ledger

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"
)

type fixture struct {
	ctx    context.Context
	repo   *Repo
	nodes  *node.Repo
	pid    string
	part   node.Node
	scenes []node.Node
}

func setup(t *testing.T, episodes int) fixture {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
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
	part, err := nodes.CreateRoot(ctx, p.ID, node.KindContainer, "1부", "", 3)
	if err != nil {
		t.Fatal(err)
	}
	f := fixture{ctx: ctx, repo: NewRepo(st, nodes), nodes: nodes, pid: p.ID, part: part}
	for i := range episodes {
		n, err := nodes.CreateChild(ctx, part.ID, node.KindLeaf, fmt.Sprintf("%d화", i+1), "", int64(10+i))
		if err != nil {
			t.Fatal(err)
		}
		doc := fmt.Sprintf(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"명패의 금박이 %d화에서 벗겨졌다."}]}]}`, i+1)
		if err := nodes.UpdateContent(ctx, n.ID, doc, int64(100+i)); err != nil {
			t.Fatal(err)
		}
		f.scenes = append(f.scenes, n)
	}
	return f
}

func TestQuotesAreCheckedAgainstTheScene(t *testing.T) {
	f := setup(t, 2)
	_, err := f.repo.Add(f.ctx, 1, Entry{ProjectID: f.pid, Kind: KindPromise, Body: "금박", PlantedNodeID: f.scenes[0].ID, PlantedQuote: "지어낸 문장"})
	if !errors.Is(err, ErrQuoteNotFound) {
		t.Fatalf("err = %v", err)
	}
	e, err := f.repo.Add(f.ctx, 1, Entry{ProjectID: f.pid, Kind: KindPromise, Body: "금박", PlantedNodeID: f.scenes[0].ID, PlantedQuote: "금박이 1화에서"})
	if err != nil {
		t.Fatal(err)
	}
	status := StatusPaid
	if _, err := f.repo.Update(f.ctx, 2, e.ID, Patch{Status: &status}); !errors.Is(err, ErrPaidNeedsNode) {
		t.Fatalf("paid with no scene: %v", err)
	}
	paid, quote := f.scenes[1].ID, "금박이 2화에서"
	got, err := f.repo.Update(f.ctx, 2, e.ID, Patch{Status: &status, PaidNodeID: &paid, PaidQuote: &quote})
	if err != nil || got.Status != StatusPaid {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestAnEntryCannotPointIntoAnotherWork(t *testing.T) {
	f := setup(t, 1)
	other := setup(t, 1)
	_ = other
	_, err := f.repo.Add(f.ctx, 1, Entry{ProjectID: "elsewhere", Kind: KindDecision, Body: "x", DueNodeID: f.scenes[0].ID})
	if !errors.Is(err, ErrWrongWork) {
		t.Fatalf("err = %v", err)
	}
}

func TestBriefNeverReadsAheadAndPutsDueFirst(t *testing.T) {
	f := setup(t, 10)
	add := func(e Entry) {
		t.Helper()
		e.ProjectID = f.pid
		if _, err := f.repo.Add(f.ctx, 1, e); err != nil {
			t.Fatal(err)
		}
	}
	add(Entry{Kind: KindPromise, Subject: "금박", Body: "진짜 명패임이 드러난다", PlantedNodeID: f.scenes[0].ID, DueNodeID: f.scenes[6].ID})
	add(Entry{Kind: KindGag, Body: "명패 숨기기", PlantedNodeID: f.scenes[1].ID})
	add(Entry{Kind: KindPromise, Body: "뒤에 심은 복선", PlantedNodeID: f.scenes[8].ID})
	add(Entry{Kind: KindDecision, Body: "서린은 남자", PlantedNodeID: f.scenes[0].ID})
	add(Entry{Kind: KindCutPlan, Body: "지운 계획"})
	got, err := f.repo.ContextLedger(f.ctx, f.pid, f.scenes[4].ID)
	if err != nil {
		t.Fatal(err)
	}
	all := strings.Join(append(append(append([]string{}, got.Due...), got.Open...), got.Settled...), "\n")
	if len(got.Due) != 1 || !strings.Contains(got.Due[0], "진짜 명패") {
		t.Errorf("due = %q", got.Due)
	}
	if strings.Contains(all, "뒤에 심은") {
		t.Errorf("read ahead: %s", all)
	}
	if strings.Contains(all, "지운 계획") {
		t.Errorf("a cut plan is not story state: %s", all)
	}
	if len(got.Settled) != 1 || !strings.Contains(got.Settled[0], "서린은 남자") {
		t.Errorf("settled = %q", got.Settled)
	}
}

func TestBriefFlagsAPromiseLeftOpenAndAPartDue(t *testing.T) {
	f := setup(t, 30)
	if _, err := f.repo.Add(f.ctx, 1, Entry{ProjectID: f.pid, Kind: KindPromise, Body: "오래된 복선", PlantedNodeID: f.scenes[0].ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.Add(f.ctx, 1, Entry{ProjectID: f.pid, Kind: KindQuestion, Body: "이 부의 약속", DueNodeID: f.part.ID}); err != nil {
		t.Fatal(err)
	}
	got, err := f.repo.ContextLedger(f.ctx, f.pid, f.scenes[25].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Open) != 1 || !strings.Contains(got.Open[0], "25화째 열려 있음") {
		t.Errorf("open = %q", got.Open)
	}
	if len(got.Due) != 1 || !strings.Contains(got.Due[0], "이 부의 약속") {
		t.Errorf("due = %q", got.Due)
	}
}

func TestBriefStaysInBudgetAndSaysWhatItLeftOut(t *testing.T) {
	f := setup(t, 2)
	for i := range 80 {
		if _, err := f.repo.Add(f.ctx, int64(i), Entry{ProjectID: f.pid, Kind: KindNumber, Body: strings.Repeat("수치 ", 40)}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := f.repo.ContextLedger(f.ctx, f.pid, f.scenes[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Omitted == 0 || len(got.Settled) == 0 {
		t.Fatalf("omitted %d settled %d", got.Omitted, len(got.Settled))
	}
}

func TestPartArcInTheBrief(t *testing.T) {
	f := setup(t, 5)
	if _, err := f.repo.SetArc(f.ctx, 1, Arc{NodeID: f.part.ID, Role: "승", Goal: "기연을 얻는다", Tension: 6}); err != nil {
		t.Fatal(err)
	}
	got, err := f.repo.ContextLedger(f.ctx, f.pid, f.scenes[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Part == nil || got.Part.Label != "1부" || got.Part.ScenesLeft != 3 || got.Part.Tension != 6 {
		t.Fatalf("part = %+v", got.Part)
	}
	if _, err := f.repo.SetArc(f.ctx, 1, Arc{NodeID: f.scenes[0].ID}); !errors.Is(err, ErrNotAPart) {
		t.Fatalf("arc on a scene: %v", err)
	}
	if _, err := f.repo.SetArc(f.ctx, 1, Arc{NodeID: f.part.ID, Tension: 11}); !errors.Is(err, ErrTension) {
		t.Fatalf("tension: %v", err)
	}
}

func TestCutPlansAreKeptWithTheirLabels(t *testing.T) {
	f := setup(t, 1)
	n := f.scenes[0]
	n.Plan = "[계획] 사라질 화"
	labels, err := f.repo.RecordCutPlans(f.ctx, 1, []node.Node{n, f.part})
	if err != nil {
		t.Fatal(err)
	}
	if len(labels) != 1 || labels[0] != "1화" {
		t.Fatalf("labels = %v", labels)
	}
	got, _ := f.repo.List(f.ctx, Filter{ProjectID: f.pid, Kinds: []string{KindCutPlan}})
	if len(got) != 1 || got[0].Body != n.Plan {
		t.Fatalf("got %+v", got)
	}
}
