//go:build !mobile

package agent

import (
	"context"
	"errors"
	"net/http"
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

// passClaudeCode stands in for the CLI across a turn and the passes after it:
// the first Chat is the writer's turn and writes a scene, each one after is a
// pass, which it records and then tries to reach past what it was offered.
type passClaudeCode struct {
	mu      sync.Mutex
	systems []string
	offered [][]string
	refused []bool
	extra   int
	// ledgerTurn makes the first Chat record a decision instead of writing.
	ledgerTurn   bool
	lastMessages []llm.ChatMessage
	// finished counts the passes that ran to their end, which is what a test has
	// to wait for: closing the service cancels one still running.
	finished int
}

func (f *passClaudeCode) Ask(context.Context, string) (string, error) { return "", nil }

func (f *passClaudeCode) Chat(ctx context.Context, msgs []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	srv := opts.ClaudeCodeMCPServers[0]
	client := mcp.NewClient(&mcp.Implementation{Name: "fake-claude-code", Version: "0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   srv.URL,
		HTTPClient: &http.Client{Transport: headerTransport{headers: srv.Headers}},
	}, nil)
	if err != nil {
		return llm.ChatResponse{}, err
	}
	defer session.Close()
	list, err := session.ListTools(ctx, nil)
	if err != nil {
		return llm.ChatResponse{}, err
	}
	names := make([]string, 0, len(list.Tools))
	for _, tool := range list.Tools {
		names = append(names, tool.Name)
	}

	f.mu.Lock()
	first := len(f.systems) == 0
	f.systems = append(f.systems, msgs[0].Content)
	f.offered = append(f.offered, names)
	f.lastMessages = msgs
	f.mu.Unlock()

	if first && f.ledgerTurn {
		if _, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "linetta_edit_ledger", Arguments: map[string]any{"kind": "decision"}}); err != nil {
			return llm.ChatResponse{}, err
		}
		return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "바꿨어요"}}, nil
	}
	if first {
		for _, name := range []string{storyContextTool, "linetta_write_scene"} {
			if _, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: map[string]any{"node_id": "n1"}}); err != nil {
				return llm.ChatResponse{}, err
			}
		}
		return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "다 썼어요"}}, nil
	}
	if _, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "linetta_read_scene", Arguments: map[string]any{"node_id": "n1"}}); err != nil {
		return llm.ChatResponse{}, err
	}
	// A tool it was not offered does not exist on this bridge.
	_, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "linetta_write_scene", Arguments: map[string]any{"node_id": "n1"}})
	f.mu.Lock()
	f.refused = append(f.refused, err != nil)
	f.mu.Unlock()
	for range f.extra {
		if _, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "linetta_read_scene", Arguments: map[string]any{"node_id": "n1"}}); err != nil {
			return llm.ChatResponse{}, err
		}
	}
	f.mu.Lock()
	f.finished++
	f.mu.Unlock()
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "끝"}}, nil
}

func (f *passClaudeCode) passes() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.finished
}

func (f *passClaudeCode) chats() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.systems)
}

func passTools(reads, writes *atomic.Int32) RegisterTools {
	ok := func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, struct{}, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, struct{}{}, nil
	}
	return func(s *mcp.Server, _ mcphost.ToolGroups) {
		mcp.AddTool(s, &mcp.Tool{Name: storyContextTool, Description: "brief"}, ok)
		mcp.AddTool(s, &mcp.Tool{Name: "linetta_report_continuity", Description: "report"}, ok)
		mcp.AddTool(s, &mcp.Tool{Name: "linetta_list_ledger", Description: "ledger"}, ok)
		mcp.AddTool(s, &mcp.Tool{Name: "linetta_edit_ledger", Description: "ledger"},
			func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, struct{}, error) {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: `{"entry":{"id":"e1","kind":"decision"}}`}}}, struct{}{}, nil
			})
		mcp.AddTool(s, &mcp.Tool{Name: "linetta_read_scene", Description: "read"},
			func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, struct{}, error) {
				reads.Add(1)
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "scene"}}}, struct{}{}, nil
			})
		mcp.AddTool(s, &mcp.Tool{Name: "linetta_write_scene", Description: "write"},
			func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, struct{}, error) {
				writes.Add(1)
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "written"}}}, struct{}{}, nil
			})
	}
}

func newPassService(t *testing.T, c llm.Client, rec *recorder, review, settle bool, register RegisterTools) *Service {
	t.Helper()
	st := openStoreForAgentTests(t)
	svc := New(Deps{
		Providers:            cliProviders{client: c},
		History:              companion.NewHistoryRepo(st.DB()),
		Scope:                fakeScope{titles: map[string]string{"p1": "제목"}},
		Register:             register,
		Notify:               rec.notify,
		Language:             func() string { return "ko" },
		EpisodeReviewEnabled: func() bool { return review },
		SettlementEnabled:    func() bool { return settle },
		Clock:                func() int64 { return 1700000000000 },
		MaxToolCalls:         func() int { return 10 },
	})
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

// Claude Code runs its own loop, so a pass written for a provider that hands
// tool calls back never ran on it: the switch was on and nothing happened.
func TestPassesRunOnClaudeCodeSettlingBeforeTheReview(t *testing.T) {
	rec := &recorder{}
	var reads, writes atomic.Int32
	c := &passClaudeCode{}
	svc := newPassService(t, c, rec, true, true, passTools(&reads, &writes))
	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "써 줘"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "both passes", func() bool { return c.passes() == 2 })
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.systems[1], "settling") || !strings.Contains(c.systems[2], "reviewing") {
		t.Fatalf("order: %.40q, %.40q", c.systems[1], c.systems[2])
	}
	if !slices.Contains(c.offered[1], "linetta_edit_ledger") || slices.Contains(c.offered[2], "linetta_edit_ledger") {
		t.Fatalf("offered: %v, %v", c.offered[1], c.offered[2])
	}
	for i, tools := range c.offered[1:] {
		if slices.Contains(tools, "linetta_write_scene") {
			t.Fatalf("pass %d was offered the prose: %v", i, tools)
		}
	}
	if !c.refused[0] || !c.refused[1] {
		t.Fatalf("a pass reached a tool it was not offered: %v", c.refused)
	}
	if writes.Load() != 1 {
		t.Fatalf("writes = %d, want the turn's own", writes.Load())
	}
	if reads.Load() != 2 {
		t.Fatalf("reads = %d", reads.Load())
	}
}

func TestPassesStayOffUntilTheWriterTurnsThemOn(t *testing.T) {
	rec := &recorder{}
	var reads, writes atomic.Int32
	c := &passClaudeCode{}
	svc := newPassService(t, c, rec, false, false, passTools(&reads, &writes))
	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "써 줘"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "agent.done", func() bool { return rec.has("agent.done") })
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
	if c.chats() != 1 {
		t.Fatalf("chats = %d: a pass the writer did not ask for is a cost on their plan", c.chats())
	}
}

func TestAPassStopsAtItsToolBudget(t *testing.T) {
	rec := &recorder{}
	var reads, writes atomic.Int32
	c := &passClaudeCode{extra: 40}
	svc := newPassService(t, c, rec, true, false, passTools(&reads, &writes))
	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "써 줘"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the review", func() bool { return c.passes() == 1 })
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
	if int(reads.Load()) > episodeReviewCallsPerScene {
		t.Fatalf("reads = %d past a budget of %d", reads.Load(), episodeReviewCallsPerScene)
	}
}

func TestNoteChangedFactKeepsDecisionsAndReplacements(t *testing.T) {
	changed := &[]string{}
	noteChangedFact(changed, "linetta_edit_ledger", `{}`, `{"entry":{"id":"a","kind":"decision"}}`)
	noteChangedFact(changed, "linetta_edit_ledger", `{"supersedes":"x"}`, `{"entry":{"id":"b","kind":"number"}}`)
	noteChangedFact(changed, "linetta_edit_ledger", `{}`, `{"entry":{"id":"c","kind":"number"}}`)
	noteChangedFact(changed, "linetta_edit_ledger", `{}`, `{"entry":{"id":"d","kind":"promise"}}`)
	noteChangedFact(changed, "linetta_read_scene", `{}`, `{"entry":{"id":"e","kind":"decision"}}`)
	if strings.Join(*changed, ",") != "a,b" {
		t.Fatalf("changed = %v", *changed)
	}
}

// What the writer asked for is a reader who has seen only these scenes: no
// conversation, no brief, nothing but reading and reporting.
func TestSampleCheckCarriesNoConversationAndOnlyReadsAndReports(t *testing.T) {
	rec := &recorder{}
	var reads, writes atomic.Int32
	c := &passClaudeCode{systems: []string{"이미 턴을 쓴 것으로 친다"}, offered: [][]string{nil}}
	svc := newPassService(t, c, rec, false, false, passTools(&reads, &writes))
	seedConversation(t, svc, 3, 100)
	if err := svc.StartSampleCheck(context.Background(), "p1", "Compare these pairs", 4); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the check", func() bool { return c.passes() == 1 })
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.systems[1], "know nothing") {
		t.Fatalf("system = %.60q", c.systems[1])
	}
	if len(c.lastMessages) != 2 {
		t.Fatalf("messages = %d: nothing of the conversation may come along", len(c.lastMessages))
	}
	if got := strings.Join(c.offered[1], ","); got != "linetta_read_scene,linetta_report_continuity" && got != "linetta_report_continuity,linetta_read_scene" {
		t.Fatalf("offered %q", got)
	}
}

func TestSampleCheckWaitsForTheTurnToFinish(t *testing.T) {
	rec := &recorder{}
	var reads, writes atomic.Int32
	svc := newPassService(t, &passClaudeCode{}, rec, false, false, passTools(&reads, &writes))
	svc.runs.start("p1", "r1", func() {})
	defer svc.runs.finish("p1", "r1")
	if err := svc.StartSampleCheck(context.Background(), "p1", "x", 4); !errors.Is(err, ErrTurnRunning) {
		t.Fatalf("err = %v", err)
	}
}

func TestImpactPassFollowsATurnThatChangedADecision(t *testing.T) {
	rec := &recorder{}
	var reads, writes atomic.Int32
	c := &passClaudeCode{ledgerTurn: true}
	st := openStoreForAgentTests(t)
	svc := New(Deps{
		Providers: cliProviders{client: c},
		History:   companion.NewHistoryRepo(st.DB()),
		Scope:     fakeScope{titles: map[string]string{"p1": "제목"}},
		Register: func(s *mcp.Server, g mcphost.ToolGroups) {
			passTools(&reads, &writes)(s, g)
			mcp.AddTool(s, &mcp.Tool{Name: "linetta_find_impact", Description: "impact"},
				func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, struct{}, error) {
					return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "{}"}}}, struct{}{}, nil
				})
		},
		Notify:              rec.notify,
		Language:            func() string { return "ko" },
		ImpactReviewEnabled: func() bool { return true },
		Clock:               func() int64 { return 1700000000000 },
		MaxToolCalls:        func() int { return 10 },
	})
	t.Cleanup(func() { _ = svc.Close() })
	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "설정을 바꿔 줘"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the impact pass", func() bool { return c.passes() == 1 })
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.systems[1], "has just changed") {
		t.Fatalf("system = %.60q", c.systems[1])
	}
	if !slices.Contains(c.offered[1], "linetta_find_impact") || slices.Contains(c.offered[1], "linetta_edit_ledger") {
		t.Fatalf("offered %v", c.offered[1])
	}
}
