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
		for _, name := range []string{storyContextTool, "linetta_read_scene", "linetta_list_ledger", "linetta_edit_ledger", "linetta_report_continuity"} {
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
	st := openStoreForAgentTests(t)
	var writes atomic.Int32
	svc := New(Deps{
		Providers:         fakeProviders{client: c},
		History:           companion.NewHistoryRepo(st.DB()),
		Scope:             fakeScope{titles: map[string]string{"p1": "제목"}, labels: map[string]string{"n1": "1화", "n2": "2화", "n3": "3화", "n4": "4화", "n5": "5화"}},
		Register:          episodeLedgerTools(&writes),
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
