//go:build !mobile

package agent

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/mcphost"
	"github.com/devlikebear/tars/pkg/llm"
)

// episodeClient scripts the turn and the review apart, telling them by the
// review's own system prompt.
type episodeClient struct {
	mu       sync.Mutex
	turn     []llm.ChatResponse
	turns    int
	requests []string
	offered  []string
	entered  chan struct{}
	once     sync.Once
}

func (c *episodeClient) Ask(context.Context, string) (string, error) { return "", nil }

func (c *episodeClient) Chat(_ context.Context, msgs []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if strings.HasPrefix(msgs[0].Content, "You are reviewing a chapter") {
		c.requests = append(c.requests, msgs[1].Content)
		for _, t := range opts.Tools {
			c.offered = append(c.offered, t.Function.Name)
		}
		c.once.Do(func() { close(c.entered) })
		return textReply(""), nil
	}
	c.turns++
	return c.turn[pick(c.turns, len(c.turn))], nil
}

func writeCall(id, node string) llm.ToolCall {
	return llm.ToolCall{ID: id, Name: "linetta_write_scene", Arguments: `{"node_id":"` + node + `"}`}
}

func episodeTools(writes *atomic.Int32) RegisterTools {
	ok := func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, struct{}, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, struct{}{}, nil
	}
	return func(s *mcp.Server, _ mcphost.ToolGroups) {
		mcp.AddTool(s, &mcp.Tool{Name: storyContextTool, Description: "brief"}, ok)
		mcp.AddTool(s, &mcp.Tool{Name: "linetta_report_continuity", Description: "report"}, ok)
		mcp.AddTool(s, &mcp.Tool{Name: "linetta_read_scene", Description: "read"}, ok)
		mcp.AddTool(s, &mcp.Tool{Name: "linetta_write_scene", Description: "write"},
			func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, struct{}, error) {
				writes.Add(1)
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "written"}}}, struct{}{}, nil
			})
	}
}

func newEpisodeService(t *testing.T, c llm.Client, rec *recorder, on bool, register RegisterTools) *Service {
	t.Helper()
	st := openStoreForAgentTests(t)
	svc := New(Deps{
		Providers:            fakeProviders{client: c},
		History:              companion.NewHistoryRepo(st.DB()),
		Scope:                fakeScope{titles: map[string]string{"p1": "제목"}},
		Register:             register,
		Notify:               rec.notify,
		Language:             func() string { return "ko" },
		EpisodeReviewEnabled: func() bool { return on },
		Clock:                func() int64 { return 1700000000000 },
	})
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

func writingTurn(nodes ...string) []llm.ChatResponse {
	calls := []llm.ToolCall{{ID: "b", Name: storyContextTool, Arguments: `{}`}}
	for _, n := range nodes {
		calls = append(calls, writeCall("w-"+n, n))
	}
	return []llm.ChatResponse{
		{Message: llm.ChatMessage{Role: "assistant", ToolCalls: calls}},
		textReply("다 썼어요"),
	}
}

func TestEpisodeReview_readsWhatTheTurnWroteWithReadOnlyTools(t *testing.T) {
	rec := &recorder{}
	var writes atomic.Int32
	c := &episodeClient{turn: writingTurn("n1", "n2"), entered: make(chan struct{})}
	svc := newEpisodeService(t, c, rec, true, episodeTools(&writes))
	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "go"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "the review", func() bool { c.mu.Lock(); defer c.mu.Unlock(); return len(c.requests) > 0 })
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.requests[0], "n1") || !strings.Contains(c.requests[0], "n2") {
		t.Fatalf("request = %q", c.requests[0])
	}
	for _, name := range c.offered {
		if name == "linetta_write_scene" {
			t.Fatal("the review was handed a tool that changes the manuscript")
		}
	}
}

func TestEpisodeReview_doesNotRunWhenOffOrWhenNothingWasWritten(t *testing.T) {
	for name, tc := range map[string]struct {
		on   bool
		turn []llm.ChatResponse
	}{
		"off":      {false, writingTurn("n1")},
		"no prose": {true, writingTurn()},
	} {
		t.Run(name, func(t *testing.T) {
			rec := &recorder{}
			var writes atomic.Int32
			c := &episodeClient{turn: tc.turn, entered: make(chan struct{})}
			svc := newEpisodeService(t, c, rec, tc.on, episodeTools(&writes))
			if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "go"}); err != nil {
				t.Fatalf("Run: %v", err)
			}
			waitFor(t, "the run to end", func() bool { return rec.has("agent.done") })
			if err := svc.Close(); err != nil {
				t.Fatal(err)
			}
			if len(c.requests) != 0 {
				t.Fatalf("reviewed: %v", c.requests)
			}
		})
	}
}

func TestEpisodeReview_coversOnlyTheLastScenesOfALongTurn(t *testing.T) {
	var written []string
	for _, id := range []string{"a", "b", "c", "d", "a"} {
		noteWrittenScene(&written, "linetta_write_scene", `{"node_id":"`+id+`"}`)
	}
	noteWrittenScene(&written, "linetta_read_scene", `{"node_id":"x"}`)
	if strings.Join(written, "") != "abcd" {
		t.Fatalf("written = %v", written)
	}
}
