package summarizer

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/node"
)

// fakeModel stands in for the writer's provider.
type fakeModel struct {
	calls atomic.Int32
	mu    sync.Mutex
	got   []ModelRequest
	// answer runs instead of the default reply when set.
	answer func(ctx context.Context, req ModelRequest) (ModelResult, error)
}

func (m *fakeModel) Summarize(ctx context.Context, req ModelRequest) (ModelResult, error) {
	m.calls.Add(1)
	m.mu.Lock()
	m.got = append(m.got, req)
	m.mu.Unlock()
	if m.answer != nil {
		return m.answer(ctx, req)
	}
	return ModelResult{Text: "생성된 요약.", Input: 120, Output: 30}, nil
}

type recorded struct {
	mu     sync.Mutex
	events []Event
}

func (r *recorded) record(_ context.Context, e Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *recorded) all() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Event(nil), r.events...)
}

var longScene = strings.Repeat("비가 오는 밤, 해진은 탑의 문을 열었다. ", 20)

type modelFixture struct {
	nodes  *node.Repo
	sum    *Summarizer
	model  *fakeModel
	on     atomic.Bool
	usage  *UsageRepo
	events *recorded
	nodeID string
	pID    string
}

func newModelFixture(t *testing.T) *modelFixture {
	t.Helper()
	st, nodes, p := newFixture(t)
	f := &modelFixture{nodes: nodes, model: &fakeModel{}, usage: NewUsageRepo(st.DB()), events: &recorded{}, nodeID: *p.LastOpenedNodeID, pID: p.ID}
	f.sum = New(nodes).WithModel(f.model, f.on.Load, f.usage, f.events.record)
	return f
}

// save writes the scene and runs it through the save queue.
func (f *modelFixture) save(t *testing.T, text string, now int64) node.Node {
	t.Helper()
	ctx := context.Background()
	if err := f.nodes.UpdateContent(ctx, f.nodeID, longDoc(text), now); err != nil {
		t.Fatal(err)
	}
	stop := f.sum.Start(ctx)
	f.sum.Enqueue(f.nodeID)
	waitFor(t, func() bool {
		n, _ := f.nodes.Get(ctx, f.nodeID)
		return n.SummaryFresh()
	}, "summary lands")
	stop()
	n, _ := f.nodes.Get(ctx, f.nodeID)
	return n
}

// The contract that matters most: off (the default) means no provider call on
// any path, and the scene still gets the opening cut.
func TestModel_offSendsNothing(t *testing.T) {
	f := newModelFixture(t)
	n := f.save(t, longScene, 1100)
	f.sum.RefreshNow(context.Background(), f.nodeID)
	if _, err := f.sum.WriteSynopsis(context.Background(), f.pID); !errors.Is(err, ErrModelOff) {
		t.Fatalf("WriteSynopsis = %v, want ErrModelOff", err)
	}
	if c := f.model.calls.Load(); c != 0 {
		t.Fatalf("provider called %d times while off", c)
	}
	if n.Summary == "" || n.Summary == "생성된 요약." {
		t.Fatalf("summary = %q, want the opening cut", n.Summary)
	}
}

func TestModel_onSummarizesALongSceneAndKeepsThePreviousSummary(t *testing.T) {
	f := newModelFixture(t)
	first := f.save(t, longScene, 1100) // off: the opening cut
	f.on.Store(true)
	n := f.save(t, longScene+"그리고 끝.", 1200)
	if n.Summary != "생성된 요약." || n.SummaryWritten {
		t.Fatalf("summary = %q written=%v, want the provider's, not marked as written", n.Summary, n.SummaryWritten)
	}
	if prev, _ := f.nodes.PreviousSummary(context.Background(), f.nodeID); prev != first.Summary {
		t.Fatalf("previous = %q, want %q", prev, first.Summary)
	}
	if f.model.got[0].Kind != KindScene || !strings.Contains(f.model.got[0].Text, "해진은 탑의 문을") {
		t.Fatalf("request = %+v", f.model.got[0])
	}
	u, _ := f.usage.Get(context.Background())
	if u != (Usage{Calls: 1, Input: 120, Output: 30}) {
		t.Fatalf("usage = %+v", u)
	}
	ev := f.events.all()
	if len(ev) != 1 || !ev[0].OK || ev[0].Tool != ToolSummarizeScene || ev[0].NodeID != f.nodeID || ev[0].ProjectID != f.pID {
		t.Fatalf("events = %+v", ev)
	}
}

func TestModel_shortScenesAndRefreshNowNeverAsk(t *testing.T) {
	f := newModelFixture(t)
	f.on.Store(true)
	f.save(t, "짧은 씬.", 1100)
	if err := f.nodes.UpdateContent(context.Background(), f.nodeID, longDoc(longScene), 1200); err != nil {
		t.Fatal(err)
	}
	f.sum.RefreshNow(context.Background(), f.nodeID)
	if c := f.model.calls.Load(); c != 0 {
		t.Fatalf("provider called %d times", c)
	}
}

// A summary an agent writes while the provider is busy is the one that stays.
func TestModel_neverReplacesAnAgentsSummary(t *testing.T) {
	f := newModelFixture(t)
	f.on.Store(true)
	f.model.answer = func(ctx context.Context, _ ModelRequest) (ModelResult, error) {
		n, _ := f.nodes.Get(ctx, f.nodeID)
		_ = f.nodes.SetWrittenSummary(ctx, f.nodeID, "에이전트 요약", n.ContentVersion)
		return ModelResult{Text: "생성된 요약.", Input: 1, Output: 1}, nil
	}
	n := f.save(t, longScene, 1100)
	if n.Summary != "에이전트 요약" || !n.SummaryWritten {
		t.Fatalf("summary = %q written=%v", n.Summary, n.SummaryWritten)
	}
	// And once it is written, a later save does not even ask.
	f.model.answer = nil
	before := f.model.calls.Load()
	if err := f.nodes.UpdateContent(context.Background(), f.nodeID, longDoc(longScene+"더."), 1200); err != nil {
		t.Fatal(err)
	}
	f.sum.summarizeOne(context.Background(), f.nodeID) // the save queue's own entry point
	if f.model.calls.Load() != before {
		t.Fatal("asked the provider about a scene whose summary an agent wrote")
	}
}

// No consent, no credential, a self-executing provider: nothing was sent, so
// the opening cut stands and there is nothing to log.
func TestModel_unavailableProviderFallsBackSilently(t *testing.T) {
	f := newModelFixture(t)
	f.on.Store(true)
	f.model.answer = func(context.Context, ModelRequest) (ModelResult, error) {
		return ModelResult{}, ErrModelUnavailable
	}
	n := f.save(t, longScene, 1100)
	if n.Summary == "" || n.Summary == "생성된 요약." {
		t.Fatalf("summary = %q, want the opening cut", n.Summary)
	}
	if ev := f.events.all(); len(ev) != 0 {
		t.Fatalf("logged %+v for a request never sent", ev)
	}
	if u, _ := f.usage.Get(context.Background()); u.Calls != 0 {
		t.Fatalf("usage = %+v", u)
	}
}

func TestModel_failedRequestIsLoggedAndFallsBack(t *testing.T) {
	f := newModelFixture(t)
	f.on.Store(true)
	f.model.answer = func(context.Context, ModelRequest) (ModelResult, error) {
		return ModelResult{}, errors.New("rate limited")
	}
	n := f.save(t, longScene, 1100)
	if n.Summary == "" {
		t.Fatal("no summary at all after a failed request")
	}
	ev := f.events.all()
	if len(ev) != 1 || ev[0].OK || !strings.Contains(ev[0].Detail, "rate limited") {
		t.Fatalf("events = %+v", ev)
	}
}

// Engine shutdown cancels a request in flight, waits for it, and writes
// nothing after: the store closes right behind it.
func TestModel_stopCancelsAnInFlightRequestAndWritesNothing(t *testing.T) {
	f := newModelFixture(t)
	f.on.Store(true)
	started := make(chan struct{})
	f.model.answer = func(ctx context.Context, _ ModelRequest) (ModelResult, error) {
		close(started)
		<-ctx.Done()
		return ModelResult{}, ctx.Err()
	}
	ctx := context.Background()
	if err := f.nodes.UpdateContent(ctx, f.nodeID, longDoc(longScene), 1100); err != nil {
		t.Fatal(err)
	}
	stop := f.sum.Start(ctx)
	f.sum.Enqueue(f.nodeID)
	<-started
	stop()
	n, _ := f.nodes.Get(ctx, f.nodeID)
	if n.SummaryFresh() {
		t.Fatalf("wrote %q after shutdown began", n.Summary)
	}
	if ev := f.events.all(); len(ev) != 0 {
		t.Fatalf("logged %+v during shutdown", ev)
	}
}

func TestModel_writeSynopsisFromTheTopLevelSummaries(t *testing.T) {
	f := newModelFixture(t)
	f.on.Store(true)
	ctx := context.Background()
	if err := f.nodes.SetWrittenSummary(ctx, f.nodeID, "1화 요약", 0); err != nil {
		t.Fatal(err)
	}
	f.model.answer = func(_ context.Context, req ModelRequest) (ModelResult, error) {
		return ModelResult{Text: " 시놉시스 ", Input: 50, Output: 20}, nil
	}
	got, err := f.sum.WriteSynopsis(ctx, f.pID)
	if err != nil || got != "시놉시스" {
		t.Fatalf("WriteSynopsis = %q, %v", got, err)
	}
	req := f.model.got[0]
	if req.Kind != KindSynopsis || !strings.Contains(req.Text, "1화 요약") {
		t.Fatalf("request = %+v", req)
	}
	ev := f.events.all()
	if len(ev) != 1 || ev[0].Tool != ToolRewriteSynopsis || !ev[0].OK {
		t.Fatalf("events = %+v", ev)
	}
}

func TestModel_writeSynopsisOfAnEmptyWorkSendsNothing(t *testing.T) {
	f := newModelFixture(t)
	f.on.Store(true)
	if _, err := f.sum.WriteSynopsis(context.Background(), f.pID); !errors.Is(err, ErrNothingToSummarize) {
		t.Fatalf("WriteSynopsis = %v, want ErrNothingToSummarize", err)
	}
	if f.model.calls.Load() != 0 {
		t.Fatal("asked the provider about an empty work")
	}
}
