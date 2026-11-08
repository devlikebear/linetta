package agent

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/mcphost"
	"github.com/devlikebear/tars/pkg/llm"
)

// episodeLedgerTools is what a writing turn and a settlement pass need.
func episodeLedgerTools(writes *atomic.Int32) RegisterTools {
	ok := func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, struct{}, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, struct{}{}, nil
	}
	return func(s *mcp.Server, _ mcphost.ToolGroups) {
		for _, name := range []string{storyContextTool, "linetta_read_scene", "linetta_list_ledger", "linetta_edit_ledger", "linetta_report_continuity", "linetta_revise_scene"} {
			mcp.AddTool(s, &mcp.Tool{Name: name, Description: name}, ok)
		}
		mcp.AddTool(s, &mcp.Tool{Name: "linetta_write_scene", Description: "write"},
			func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, struct{}, error) {
				writes.Add(1)
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "written"}}}, struct{}{}, nil
			})
	}
}

// fakeSettlements is the settlement record: scene versions, what was recorded,
// and the queue Pending answers with until a done takes a scene off it.
type fakeSettlements struct {
	mu       sync.Mutex
	versions map[string]int
	pending  []string
	records  []settlementRecord
	asked    []string // beforeNodeID of each Pending call
}

type settlementRecord struct {
	node    string
	version int
	outcome string
}

func (f *fakeSettlements) Version(_ context.Context, id string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.versions[id], nil
}

func (f *fakeSettlements) Record(_ context.Context, _, id string, version int, outcome string, _ int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records = append(f.records, settlementRecord{id, version, outcome})
	if outcome == passDone {
		f.pending = slices.DeleteFunc(f.pending, func(p string) bool { return p == id })
	}
	return nil
}

func (f *fakeSettlements) Pending(_ context.Context, _, before string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, before)
	return slices.Clone(f.pending), nil
}

func (f *fakeSettlements) recorded() []settlementRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.records)
}

// settleClient scripts a turn and the settlement passes apart, telling them by
// the settlement's own system prompt. mode decides how every pass ends.
type settleClient struct {
	mu       sync.Mutex
	turn     []llm.ChatResponse
	turns    int
	mode     string // done | timeout | failed
	passes   []string
	firstMsg string
	lastTurn []llm.ChatMessage
	calls    atomic.Int32
}

func (c *settleClient) Ask(context.Context, string) (string, error) { return "", nil }

func (c *settleClient) Chat(_ context.Context, msgs []llm.ChatMessage, _ llm.ChatOptions) (llm.ChatResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if strings.HasPrefix(msgs[0].Content, "You are settling") {
		if len(msgs) == 2 {
			c.passes = append(c.passes, msgs[1].Content)
		}
		switch c.mode {
		case passFailed:
			return llm.ChatResponse{}, errors.New("provider down")
		case passTimeout:
			// Never stops reading: the pass runs out of calls.
			n := c.calls.Add(1)
			return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{
				{ID: "r" + string(rune('a'+n%26)), Name: "linetta_read_scene", Arguments: `{}`},
			}}}, nil
		}
		return textReply(""), nil
	}
	if c.turns == 0 {
		c.firstMsg = msgs[len(msgs)-1].Content
	}
	c.lastTurn = slices.Clone(msgs)
	c.turns++
	return c.turn[pick(c.turns, len(c.turn))], nil
}

func (c *settleClient) settlementRequests() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.passes)
}

func newSettleService(t *testing.T, c llm.Client, rec *recorder, on bool, ledger *fakeSettlements, log *gateLog) *Service {
	t.Helper()
	var writes atomic.Int32
	return newSettleServiceWith(t, fakeProviders{client: c}, episodeLedgerTools(&writes), nil, rec, on, ledger, log)
}

func newSettleServiceWith(t *testing.T, providers ProviderSource, register RegisterTools, plans PlanLookup, rec *recorder, on bool, ledger *fakeSettlements, log *gateLog) *Service {
	t.Helper()
	st := openStoreForAgentTests(t)
	svc := New(Deps{
		Providers:         providers,
		Plans:             plans,
		History:           companion.NewHistoryRepo(st.DB()),
		Scope:             fakeScope{titles: map[string]string{"p1": "제목"}, labels: map[string]string{"n1": "1화", "n2": "2화", "n3": "3화", "n4": "4화", "n5": "5화", "n6": "6화"}},
		Register:          register,
		Notify:            rec.notify,
		Language:          func() string { return "ko" },
		SettlementEnabled: func() bool { return on },
		Settlements:       ledger,
		RecordGate:        log.record,
		Clock:             func() int64 { return 1700000000000 },
	})
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

// A pass after a turn records, for each scene it covered, how it ended and at
// which version — all three endings.
func TestSettlement_recordsHowThePassEnded(t *testing.T) {
	for _, mode := range []string{passDone, passTimeout, passFailed} {
		t.Run(mode, func(t *testing.T) {
			rec := &recorder{}
			c := &settleClient{turn: writingTurn("n1", "n2"), mode: mode}
			ledger := &fakeSettlements{versions: map[string]int{"n1": 4, "n2": 7}}
			svc := newSettleService(t, c, rec, true, ledger, &gateLog{})
			if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "써 줘"}); err != nil {
				t.Fatal(err)
			}
			waitFor(t, "the record", func() bool { return len(ledger.recorded()) == 2 })
			want := []settlementRecord{{"n1", 4, mode}, {"n2", 7, mode}}
			if got := ledger.recorded(); !slices.Equal(got, want) {
				t.Fatalf("records = %+v, want %+v", got, want)
			}
		})
	}
}

func (c *settleClient) turnResults() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return toolMessages(c.lastTurn)
}

// catchUpTurn runs one turn and closes the service once passes settlement
// passes have reached the provider. agent.done goes out BEFORE the passes
// after the turn are started (endDone, then afterReply), and Close refuses
// any pass not yet started — so a test that closes on agent.done alone races
// the turn's own settlement, and loses it on a loaded machine.
func catchUpTurn(t *testing.T, on bool, mode string, turn []llm.ChatResponse, pending []string, passes int) (*settleClient, *fakeSettlements, *gateLog) {
	t.Helper()
	rec := &recorder{}
	c := &settleClient{turn: turn, mode: mode}
	ledger := &fakeSettlements{versions: map[string]int{"n1": 1, "n2": 2, "n3": 3, "n4": 4, "n5": 5, "n6": 6}, pending: pending}
	log := &gateLog{}
	svc := newSettleService(t, c, rec, on, ledger, log)
	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", NodeID: "n1", Prompt: "다음 화 써 줘"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "agent.done", func() bool { return rec.has("agent.done") })
	waitFor(t, "the settlement passes", func() bool { return len(c.settlementRequests()) >= passes })
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
	return c, ledger, log
}

// A turn that writes no prose never waits on a settlement, whether the writer
// has it on or off — even with a scene open and episodes waiting.
func TestCatchUp_aTurnWithNoProseWriteCatchesNothingUp(t *testing.T) {
	for _, on := range []bool{false, true} {
		c, ledger, log := catchUpTurn(t, on, passDone, []llm.ChatResponse{textReply("네")}, []string{"n2"}, 0)
		if got := c.settlementRequests(); len(got) != 0 {
			t.Fatalf("on=%v: passes = %q, want none", on, got)
		}
		if len(ledger.asked) != 0 || len(log.all()) != 0 {
			t.Errorf("on=%v: Pending asked %v, activity %+v", on, ledger.asked, log.all())
		}
	}
}

// Settlement off: a writing turn makes no call before its write either.
func TestCatchUp_nothingRunsWhenSettlementIsOff(t *testing.T) {
	c, ledger, _ := catchUpTurn(t, false, passDone, writingTurn("n5"), []string{"n2"}, 0)
	if got := c.settlementRequests(); len(got) != 0 {
		t.Fatalf("passes = %q, want none", got)
	}
	if len(ledger.asked) != 0 {
		t.Errorf("Pending was asked %v with settlement off", ledger.asked)
	}
}

// The turn's first prose write settles what waits before its scene, once;
// the second write in the same turn does not run it again.
func TestCatchUp_runsOnceBeforeTheFirstWrite(t *testing.T) {
	c, ledger, log := catchUpTurn(t, true, passDone, writingTurn("n5", "n6"), []string{"n2", "n3"}, 2)
	// The pass after the turn settles what it wrote; the first is the catch-up.
	got := c.settlementRequests()
	if len(got) != 2 || !strings.Contains(got[0], "n2") || !strings.Contains(got[0], "n3") || strings.Contains(got[1], "n2") {
		t.Fatalf("passes = %q, want the catch-up over n2 and n3, then the turn's own", got)
	}
	if len(ledger.asked) != 2 || ledger.asked[0] != "n5" || ledger.asked[1] != "n5" {
		t.Errorf("Pending asked before %v, want the first write's scene, before and after the pass", ledger.asked)
	}
	if want := []settlementRecord{{"n2", 2, passDone}, {"n3", 3, passDone}}; !slices.Equal(ledger.recorded()[:2], want) {
		t.Errorf("records = %+v", ledger.recorded())
	}
	results := c.turnResults()
	if len(results) != 3 || results[1] != "written" || results[2] != "written" {
		t.Errorf("tool results = %q, want both writes run with no notice once nothing waits", results)
	}
	if strings.Contains(c.opening(), "Settlement pending") {
		t.Errorf("opening = %q, want nothing before the writer's message", c.opening())
	}
	rows := log.all()
	if len(rows) != 1 || rows[0].Kind != SettlementCatchUp || !rows[0].OK || rows[0].NodeID != "n2" ||
		!strings.Contains(rows[0].Detail, "scenes=2 left=0") {
		t.Errorf("activity rows = %+v", rows)
	}
}

func (c *settleClient) opening() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.firstMsg
}

// A revise that sweeps the whole work names no scene, so it runs no catch-up
// and does not use up the turn's one: the write after it does.
func TestCatchUp_aSweepingReviseLeavesTheCatchUpToTheNextWrite(t *testing.T) {
	turn := []llm.ChatResponse{
		toolReply(storyContextTool, `{}`),
		toolReply("linetta_revise_scene", `{"find":"a","replace":"b"}`),
		toolReply("linetta_write_scene", `{"node_id":"n5"}`),
		toolReply("linetta_write_scene", `{"node_id":"n6"}`),
		textReply("썼어요"),
	}
	c, ledger, _ := catchUpTurn(t, true, passFailed, turn, []string{"n2"}, 2)
	got := c.settlementRequests()
	if len(got) < 1 || !strings.Contains(got[0], "n2") {
		t.Fatalf("passes = %q, want the catch-up over n2 first", got)
	}
	if want := (settlementRecord{"n2", 2, passFailed}); ledger.recorded()[0] != want {
		t.Errorf("records = %+v", ledger.recorded())
	}
	if ledger.asked[0] != "n5" {
		t.Errorf("Pending asked before %v, want the write after the sweep", ledger.asked)
	}
	results := c.turnResults()
	if len(results) != 4 || strings.Contains(results[1], "Settlement") ||
		!strings.Contains(results[2], "Settlement pending: 2화.") || strings.Contains(results[3], "Settlement") {
		t.Errorf("tool results = %q, want the notice on the first write_scene only", results)
	}
}

// Past three scenes, the rest are named on the write's own result.
func TestCatchUp_settlesThreeAndNamesTheRestOnTheWrite(t *testing.T) {
	c, _, _ := catchUpTurn(t, true, passDone, writingTurn("n6"), []string{"n1", "n2", "n3", "n4", "n5"}, 2)
	got := c.settlementRequests()
	if len(got) != 2 || !strings.Contains(got[0], "n3") || strings.Contains(got[0], "n4") {
		t.Fatalf("passes = %q, want the catch-up over n1..n3, then the turn's own", got)
	}
	results := c.turnResults()
	if len(results) != 2 || !strings.HasPrefix(results[1], "written\n") || !strings.Contains(results[1], "Settlement pending: 4화, 5화.") {
		t.Errorf("write result = %q, want the write's own result and then 4화, 5화", results)
	}
}

// A failed catch-up does not stop the write.
func TestCatchUp_aFailedPassDoesNotStopTheWrite(t *testing.T) {
	c, ledger, log := catchUpTurn(t, true, passFailed, writingTurn("n5"), []string{"n2"}, 2)
	if want := []settlementRecord{{"n2", 2, passFailed}}; !slices.Equal(ledger.recorded()[:1], want) {
		t.Errorf("records = %+v", ledger.recorded())
	}
	results := c.turnResults()
	if len(results) != 2 || !strings.HasPrefix(results[1], "written") || !strings.Contains(results[1], "Settlement pending: 2화.") {
		t.Errorf("write result = %q", results)
	}
	if rows := log.all(); len(rows) != 1 || rows[0].OK {
		t.Errorf("activity rows = %+v, want one catch-up that did not finish", rows)
	}
}

// A write the plan gate turns back runs no catch-up; the write after the plan
// does.
func TestCatchUp_waitsForTheWriteThePlanGateLetsThrough(t *testing.T) {
	rec := &recorder{}
	plans := &fakePlans{scenes: map[string]*fakeScene{"n5": {empty: true}}}
	// The pass fails, so the waiting scene is named on whichever write ran the
	// catch-up: it must be the one after the plan.
	c := &settleClient{mode: passFailed, turn: []llm.ChatResponse{
		toolReply(storyContextTool, `{}`),
		toolReply("linetta_write_scene", `{"node_id":"n5"}`),
		toolReply(planTool, `{"node_id":"n5","as_plan":true}`),
		toolReply("linetta_write_scene", `{"node_id":"n5"}`),
		textReply("썼어요"),
	}}
	ledger := &fakeSettlements{versions: map[string]int{"n2": 2}, pending: []string{"n2"}}
	var writes atomic.Int32
	svc := newSettleServiceWith(t, fakeProviders{client: c}, settlePlanTools(plans, &writes), plans, rec, true, ledger, &gateLog{})
	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", NodeID: "n1", Prompt: "써 줘"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "agent.done", func() bool { return rec.has("agent.done") })
	// The pass after the turn starts after agent.done; see catchUpTurn.
	waitFor(t, "the settlement passes", func() bool { return len(c.settlementRequests()) >= 2 })
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
	if got := c.settlementRequests(); len(got) != 2 || !strings.Contains(got[0], "n2") {
		t.Fatalf("passes = %q, want one catch-up (before the write after the plan), then the turn's own", got)
	}
	results := c.turnResults()
	if len(results) != 4 || !strings.HasPrefix(results[1], "Not written.") || strings.Contains(results[1], "Settlement") {
		t.Fatalf("tool results = %q, want the refusal without a catch-up", results)
	}
	if !strings.HasPrefix(results[3], "written") || !strings.Contains(results[3], "Settlement pending: 2화.") || writes.Load() != 1 {
		t.Errorf("write after the plan = %q (writes %d)", results[3], writes.Load())
	}
}

// settlePlanTools is planTools plus what a settlement pass needs.
func settlePlanTools(plans *fakePlans, writes *atomic.Int32) RegisterTools {
	base := planTools(plans, true, writes)
	ok := func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, struct{}, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, struct{}{}, nil
	}
	return func(s *mcp.Server, g mcphost.ToolGroups) {
		base(s, g)
		for _, name := range []string{"linetta_read_scene", "linetta_list_ledger", "linetta_edit_ledger", "linetta_report_continuity"} {
			mcp.AddTool(s, &mcp.Tool{Name: name, Description: name}, ok)
		}
	}
}

// settlingClaudeCode is claudeCodeCalls that also answers a settlement pass:
// it stops at once, done.
type settlingClaudeCode struct {
	claudeCodeCalls
	mu     sync.Mutex
	passes int
}

func (f *settlingClaudeCode) Chat(ctx context.Context, msgs []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	if strings.HasPrefix(msgs[0].Content, "You are settling") {
		f.mu.Lock()
		f.passes++
		f.mu.Unlock()
		return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "끝"}}, nil
	}
	return f.claudeCodeCalls.Chat(ctx, msgs, opts)
}

// Over the CLI bridge the same: once, before the first write, and the rest on
// that write's result.
func TestRunCLI_catchUpRunsOnceBeforeTheFirstWrite(t *testing.T) {
	rec := &recorder{}
	plans := &fakePlans{scenes: map[string]*fakeScene{}}
	var writes atomic.Int32
	cc := &settlingClaudeCode{claudeCodeCalls: claudeCodeCalls{calls: [][2]string{
		{storyContextTool, `{}`},
		{"linetta_write_scene", `{"node_id":"n5"}`},
		{"linetta_write_scene", `{"node_id":"n6"}`},
	}}}
	ledger := &fakeSettlements{versions: map[string]int{"n1": 1, "n2": 2, "n3": 3, "n4": 4}, pending: []string{"n1", "n2", "n3", "n4"}}
	svc := newSettleServiceWith(t, cliProviders{client: cc}, settlePlanTools(plans, &writes), plans, rec, true, ledger, &gateLog{})
	runID, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "써 줘"})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the run to end", func() bool { return rec.hasTerminalFor(runID) })
	cc.mu.Lock()
	passes := cc.passes
	cc.mu.Unlock()
	// The settlement after the turn (settlement switched on, two scenes
	// written) is a second pass; wait for it so Close does not race it.
	waitFor(t, "the passes", func() bool { cc.mu.Lock(); defer cc.mu.Unlock(); return cc.passes >= 2 })
	if passes < 1 {
		t.Fatalf("no catch-up ran before the first write")
	}
	if len(cc.seen) != 3 || writes.Load() != 2 {
		t.Fatalf("calls answered = %d, writes %d", len(cc.seen), writes.Load())
	}
	if first := textOf(cc.seen[1]); !strings.Contains(first, "Settlement pending: 4화.") {
		t.Errorf("first write over the bridge = %q, want 4화 named", first)
	}
	if second := textOf(cc.seen[2]); strings.Contains(second, "Settlement") {
		t.Errorf("second write = %q, want no second catch-up", second)
	}
	if want := []settlementRecord{{"n1", 1, passDone}, {"n2", 2, passDone}, {"n3", 3, passDone}}; !slices.Equal(ledger.recorded()[:3], want) {
		t.Errorf("records = %+v", ledger.recorded())
	}
}

// The same over the CLI bridge: a sweeping revise first, then the write that
// runs the catch-up.
func TestRunCLI_aSweepingReviseLeavesTheCatchUpToTheNextWrite(t *testing.T) {
	rec := &recorder{}
	plans := &fakePlans{scenes: map[string]*fakeScene{}}
	var writes atomic.Int32
	cc := &settlingClaudeCode{claudeCodeCalls: claudeCodeCalls{calls: [][2]string{
		{storyContextTool, `{}`},
		{"linetta_revise_scene", `{}`},
		{"linetta_write_scene", `{"node_id":"n5"}`},
	}}}
	ledger := &fakeSettlements{versions: map[string]int{"n2": 2}, pending: []string{"n2"}}
	svc := newSettleServiceWith(t, cliProviders{client: cc}, settlePlanTools(plans, &writes), plans, rec, true, ledger, &gateLog{})
	runID, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "써 줘"})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the run to end", func() bool { return rec.hasTerminalFor(runID) })
	waitFor(t, "the passes", func() bool { cc.mu.Lock(); defer cc.mu.Unlock(); return cc.passes >= 2 })
	if len(cc.seen) != 3 {
		t.Fatalf("calls answered = %d", len(cc.seen))
	}
	if len(ledger.asked) == 0 || ledger.asked[0] != "n5" {
		t.Errorf("Pending asked before %v, want the write after the sweep", ledger.asked)
	}
	if got := ledger.recorded(); len(got) == 0 || got[0] != (settlementRecord{"n2", 2, passDone}) {
		t.Errorf("records = %+v, want the catch-up over n2 first", got)
	}
}
