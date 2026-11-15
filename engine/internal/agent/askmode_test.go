package agent

import (
	"context"
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

// askTools serves the cite tool beside passTools' reads and writes, so a
// question turn has something to offer and something to withhold.
func askTools(reads, writes *atomic.Int32) RegisterTools {
	pass := passTools(reads, writes)
	return func(s *mcp.Server, groups mcphost.ToolGroups) {
		pass(s, groups)
		mcp.AddTool(s, &mcp.Tool{Name: citeToolName, Description: "cite"},
			func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, struct{}, error) {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "1 sources verified"}}}, struct{}{}, nil
			})
	}
}

func newAskService(t *testing.T, c llm.Client, rec *recorder, register RegisterTools, providers ProviderSource) *Service {
	t.Helper()
	st := openStoreForAgentTests(t)
	svc := New(Deps{
		Providers:            providers,
		History:              companion.NewHistoryRepo(st.DB()),
		Scope:                fakeScope{titles: map[string]string{"p1": "제목"}},
		Register:             register,
		Notify:               rec.notify,
		Language:             func() string { return "ko" },
		SelfReviewEnabled:    func() bool { return true },
		EpisodeReviewEnabled: func() bool { return true },
		SettlementEnabled:    func() bool { return true },
		Clock:                func() int64 { return 1700000000000 },
		MaxToolCalls:         func() int { return 10 },
	})
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

func offeredNames(opts llm.ChatOptions) []string {
	names := make([]string, 0, len(opts.Tools))
	for _, tool := range opts.Tools {
		names = append(names, tool.Function.Name)
	}
	return names
}

// A question turn is offered the read tools and the source check, told to
// answer from the manuscript with sources, and nothing that writes.
func TestRun_askOffersOnlyReadToolsAndAsksForSources(t *testing.T) {
	rec := &recorder{}
	var reads, writes atomic.Int32
	client := &scriptedClient{responses: []llm.ChatResponse{textReply("12화입니다 [1].")}}
	svc := newAskService(t, client, rec, askTools(&reads, &writes), fakeProviders{client: client})

	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "검을 처음 받은 게 몇 화야?", Ask: true}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the turn to end", func() bool { return rec.has("agent.done") })
	waitIdle(t, svc)

	offered := offeredNames(client.options())
	for _, want := range []string{"linetta_read_scene", citeToolName, askWriterTool} {
		if !slices.Contains(offered, want) {
			t.Errorf("a question turn is not offered %s: %v", want, offered)
		}
	}
	for _, write := range []string{"linetta_write_scene", "linetta_edit_ledger", "linetta_report_continuity"} {
		if slices.Contains(offered, write) {
			t.Errorf("a question turn is offered %s: %v", write, offered)
		}
	}
	system := client.lastMsgs[0].Content
	for _, want := range []string{"question about the manuscript", citeToolName, "[1], [2]", "no basis"} {
		if !strings.Contains(system, want) {
			t.Errorf("system prompt lacks %q", want)
		}
	}
	if strings.Contains(system, "linetta_write_scene") || strings.Contains(system, "refresh its summary") {
		t.Error("a question turn's prompt still carries the writing habits")
	}
	if client.count() != 1 {
		t.Errorf("Chat calls = %d: a question turn starts no pass", client.count())
	}
}

// The model's call is not the guarantee: a write named in a question turn is
// refused before it reaches the tool.
func TestRun_askRefusesAWriteTheModelCallsAnyway(t *testing.T) {
	rec := &recorder{}
	var reads, writes atomic.Int32
	client := &scriptedClient{responses: []llm.ChatResponse{
		toolReply("linetta_write_scene", `{"node_id":"n1"}`),
		textReply("바꿀 수 없어요."),
	}}
	svc := newAskService(t, client, rec, askTools(&reads, &writes), fakeProviders{client: client})

	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "고쳐 줄 수 있어?", Ask: true}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the turn to end", func() bool { return rec.has("agent.done") })
	waitIdle(t, svc)

	got := finalToolPayload(t, rec, "linetta_write_scene")
	if got.State != "error" || !strings.Contains(client.lastMsgs[len(client.lastMsgs)-1].Content, "question") {
		t.Fatalf("a write in a question turn ran: %+v", got)
	}
	if writes.Load() != 0 {
		t.Fatalf("the write tool ran %d times", writes.Load())
	}
}

type askClaudeCode struct {
	mu      sync.Mutex
	systems []string
	offered [][]string
}

func (f *askClaudeCode) Ask(context.Context, string) (string, error) { return "", nil }

func (f *askClaudeCode) Chat(ctx context.Context, msgs []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	srv := opts.ClaudeCodeMCPServers[0]
	session, err := mcp.NewClient(&mcp.Implementation{Name: "fake-claude-code", Version: "0"}, nil).
		Connect(ctx, &mcp.StreamableClientTransport{
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
	names := []string{}
	for _, tool := range list.Tools {
		names = append(names, tool.Name)
	}
	f.mu.Lock()
	f.systems = append(f.systems, msgs[0].Content)
	f.offered = append(f.offered, names)
	f.mu.Unlock()
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "12화입니다."}}, nil
}

func (f *askClaudeCode) chats() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.systems)
}

// Claude Code gets the same cut through its bridge, and its passes, on in this
// service, stay off after a question.
func TestRunCLI_askBridgesOnlyReadToolsAndStartsNoPass(t *testing.T) {
	rec := &recorder{}
	var reads, writes atomic.Int32
	cc := &askClaudeCode{}
	svc := newAskService(t, cc, rec, askTools(&reads, &writes), cliProviders{client: cc})

	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "몇 화야?", Ask: true}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the turn to end", func() bool { return rec.has("agent.done") })
	waitIdle(t, svc)

	if cc.chats() != 1 {
		t.Fatalf("Claude Code ran %d times: a question turn starts no pass", cc.chats())
	}
	if slices.Contains(cc.offered[0], "linetta_write_scene") || !slices.Contains(cc.offered[0], citeToolName) {
		t.Fatalf("bridged tools = %v", cc.offered[0])
	}
	if !strings.Contains(cc.systems[0], "question about the manuscript") {
		t.Error("the bridged turn's prompt is not a question turn's")
	}
}

// The same service still writes and runs its passes on an ordinary turn, so
// the cut above is the question's and not the service's.
func TestRunCLI_aWritingTurnStillRunsItsPasses(t *testing.T) {
	rec := &recorder{}
	var reads, writes atomic.Int32
	c := &passClaudeCode{}
	svc := newAskService(t, c, rec, askTools(&reads, &writes), cliProviders{client: c})
	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "써 줘"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "both passes", func() bool { return c.passes() == 2 })
	if writes.Load() == 0 {
		t.Fatal("the writing turn did not write")
	}
}
