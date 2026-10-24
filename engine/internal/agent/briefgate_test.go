//go:build !mobile

package agent

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/mcphost"
	"github.com/devlikebear/tars/pkg/llm"
)

// briefTools installs a stand-in scene write, counting the writes that ran,
// and — when withBrief — a stand-in story brief.
func briefTools(withBrief bool, writes *atomic.Int32) RegisterTools {
	return func(s *mcp.Server, _ mcphost.ToolGroups) {
		if withBrief {
			mcp.AddTool(s, &mcp.Tool{Name: storyContextTool, Description: "the brief"},
				func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, struct{}, error) {
					return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "style notes: 단문 나열 금지"}}}, struct{}{}, nil
				})
		}
		mcp.AddTool(s, &mcp.Tool{Name: "linetta_revise_scene", Description: "revise"},
			func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, struct{}, error) {
				writes.Add(1)
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "revised"}}}, struct{}{}, nil
			})
	}
}

func newBriefService(t *testing.T, c llm.Client, rec *recorder, register RegisterTools) *Service {
	t.Helper()
	st := openStoreForAgentTests(t)
	svc := New(Deps{
		Providers: fakeProviders{client: c},
		History:   companion.NewHistoryRepo(st.DB()),
		Scope:     fakeScope{titles: map[string]string{"p1": "제목"}},
		Register:  register,
		Notify:    rec.notify,
		Language:  func() string { return "ko" },
		Clock:     func() int64 { return 1700000000000 },
	})
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

func toolMessages(msgs []llm.ChatMessage) []string {
	var out []string
	for _, m := range msgs {
		if m.Role == "tool" {
			out = append(out, m.Content)
		}
	}
	return out
}

// A revision turn that goes straight to the write is turned back to read the
// brief first; once it has, the same write goes through.
func TestRun_aProseWriteWaitsForTheTurnToReadTheBrief(t *testing.T) {
	rec := &recorder{}
	var writes atomic.Int32
	c := &scriptedClient{responses: []llm.ChatResponse{
		toolReply("linetta_revise_scene", `{}`),
		toolReply(storyContextTool, `{}`),
		toolReply("linetta_revise_scene", `{}`),
		textReply("고쳤어요"),
	}}
	svc := newBriefService(t, c, rec, briefTools(true, &writes))
	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "그 줄 고쳐 줘"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "agent.done", func() bool { return rec.has("agent.done") })

	if got := writes.Load(); got != 1 {
		t.Errorf("revise ran %d times, want once — after the brief, not before", got)
	}
	results := toolMessages(c.messages())
	if len(results) != 3 {
		t.Fatalf("tool results = %q, want three", results)
	}
	if !strings.Contains(results[0], storyContextTool) || !strings.HasPrefix(results[0], "Not written.") {
		t.Errorf("first write's result = %q, want the refusal naming %s", results[0], storyContextTool)
	}
	if results[2] != "revised" {
		t.Errorf("second write's result = %q, want it to have run", results[2])
	}
}

// A tool list without the brief cannot satisfy the gate, so it is not armed:
// every write refused would be a turn that can never write.
func TestRun_withoutTheBriefToolProseWritesAreNotHeldBack(t *testing.T) {
	rec := &recorder{}
	var writes atomic.Int32
	c := &scriptedClient{responses: []llm.ChatResponse{
		toolReply("linetta_revise_scene", `{}`),
		textReply("고쳤어요"),
	}}
	svc := newBriefService(t, c, rec, briefTools(false, &writes))
	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "그 줄 고쳐 줘"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "agent.done", func() bool { return rec.has("agent.done") })

	if got := writes.Load(); got != 1 {
		t.Errorf("revise ran %d times, want once", got)
	}
}

func TestBriefGate_holdsOnlyProseWrites(t *testing.T) {
	g := newBriefGate([]llm.ToolSchema{{Function: llm.ToolFunctionSchema{Name: storyContextTool}}})
	for _, name := range []string{"linetta_read_scene", "linetta_write_summary", "linetta_apply_story_ops"} {
		if r := g.refusal(name); r != "" {
			t.Errorf("%s was held back: %q", name, r)
		}
	}
	if g.refusal("linetta_write_scene") == "" {
		t.Error("write_scene went through before the brief was read")
	}
	g.ran(storyContextTool, false)
	if g.refusal("linetta_write_scene") == "" {
		t.Error("a brief call that failed opened the gate")
	}
	g.ran(storyContextTool, true)
	if r := g.refusal("linetta_write_scene"); r != "" {
		t.Errorf("write_scene still held after the brief: %q", r)
	}
}
