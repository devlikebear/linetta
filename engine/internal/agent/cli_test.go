//go:build !mobile

package agent

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/provider"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/tars/pkg/llm"
)

// cliProviders resolves to the Claude Code CLI, so Run takes loopCLI.
type cliProviders struct{ client llm.Client }

func (f cliProviders) Client(context.Context, string) (llm.Client, provider.Resolved, error) {
	return f.client, provider.Resolved{ID: settings.ProviderClaudeCode, Model: "sonnet"}, nil
}

// fakeClaudeCode stands in for the CLI: it reads the MCP server the turn
// handed it, connects to it over HTTP the way Claude Code would, and calls
// the tools in calls between bits of streamed text.
type fakeClaudeCode struct {
	calls []string // the echo argument of each call, in order
	seen  []*mcp.CallToolResult
	// unauthorized is the status a request without the token got.
	unauthorized int
}

func (f *fakeClaudeCode) Ask(context.Context, string) (string, error) { return "", nil }

func (f *fakeClaudeCode) Chat(ctx context.Context, _ []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	if len(opts.ClaudeCodeMCPServers) != 1 {
		return llm.ChatResponse{}, &llm.ProviderError{Provider: "claude-code-cli", Operation: "request"}
	}
	srv := opts.ClaudeCodeMCPServers[0]

	if resp, err := http.Post(srv.URL, "application/json", strings.NewReader(`{}`)); err == nil {
		f.unauthorized = resp.StatusCode
		_ = resp.Body.Close()
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "fake-claude-code", Version: "0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   srv.URL,
		HTTPClient: &http.Client{Transport: headerTransport{headers: srv.Headers}},
	}, nil)
	if err != nil {
		return llm.ChatResponse{}, err
	}
	defer session.Close()

	opts.OnDelta("먼저 읽어 볼게요.")
	for _, text := range f.calls {
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{"text": text}})
		if err != nil {
			return llm.ChatResponse{}, err
		}
		f.seen = append(f.seen, res)
	}
	opts.OnDelta("다 했어요.")
	return llm.ChatResponse{
		Message: llm.ChatMessage{Role: "assistant", Content: "먼저 읽어 볼게요.다 했어요."},
		Usage:   llm.Usage{InputTokens: 30, OutputTokens: 7},
	}, nil
}

type headerTransport struct{ headers map[string]string }

func (h headerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	for k, v := range h.headers {
		r.Header.Set(k, v)
	}
	return http.DefaultTransport.RoundTrip(r)
}

func newCLIService(t *testing.T, client llm.Client, rec *recorder, maxToolCalls int) *Service {
	t.Helper()
	st := openStoreForAgentTests(t)
	svc := New(Deps{
		Providers:    cliProviders{client: client},
		History:      companion.NewHistoryRepo(st.DB()),
		Scope:        fakeScope{titles: map[string]string{"p1": "제목"}},
		Register:     stubTools(nil),
		Notify:       rec.notify,
		Language:     func() string { return "ko" },
		Clock:        func() int64 { return 1700000000000 },
		MaxToolCalls: func() int { return maxToolCalls },
	})
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

// The CLI runs its own loop, so the tools reach it over the bridge — and a
// call made there has to land exactly like one the loop made itself: an
// agent.tool pair with the undo ids, a transcript row in the order it
// happened, and a done event with the usage.
func TestRunCLI_servesTheToolsAndRecordsTheTurnLikeTheLoop(t *testing.T) {
	rec := &recorder{}
	cc := &fakeClaudeCode{calls: []string{"hello"}}
	svc := newCLIService(t, cc, rec, 10)

	runID, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "1화를 다듬어 줘"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "the run to end", func() bool { return rec.hasTerminalFor(runID) })

	if cc.unauthorized != http.StatusUnauthorized {
		t.Errorf("a request without the token got %d, want 401", cc.unauthorized)
	}
	if len(cc.seen) != 1 || cc.seen[0].IsError || textOf(cc.seen[0]) != "echo: hello" {
		t.Fatalf("tool result over the bridge = %+v", cc.seen)
	}

	var started, done *toolPayload
	rec.mu.Lock()
	for _, e := range rec.seen {
		if p, ok := e.Params.(toolPayload); ok && p.RunID == runID {
			p := p
			switch p.State {
			case "started":
				started = &p
			case "done":
				done = &p
			}
		}
	}
	rec.mu.Unlock()
	if started == nil || done == nil {
		t.Fatalf("no agent.tool pair: %v", rec.methods())
	}
	if done.BatchID != "batch-1" || done.SnapshotID != "snap-1" {
		t.Errorf("undo ids lost on the way: %+v", done)
	}

	msgs, err := svc.History(context.Background(), "p1", 100)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, m := range msgs {
		if m.RunID != runID {
			continue
		}
		order = append(order, m.Role+":"+strings.TrimSpace(m.Content))
	}
	want := []string{"user:1화를 다듬어 줘", "assistant:먼저 읽어 볼게요.", "tool", "assistant:다 했어요."}
	if len(order) != len(want) {
		t.Fatalf("transcript = %q, want the shape %q", order, want)
	}
	for i, w := range want {
		if w == "tool" {
			if !strings.HasPrefix(order[i], "tool") {
				t.Errorf("row %d = %q, want the tool row", i, order[i])
			}
			continue
		}
		if order[i] != w {
			t.Errorf("row %d = %q, want %q", i, order[i], w)
		}
	}

	rec.mu.Lock()
	var gotDone bool
	for _, e := range rec.seen {
		if p, ok := e.Params.(donePayload); ok && p.RunID == runID {
			gotDone = true
			if p.Usage.Input != 30 || p.Usage.Output != 7 || p.Model != "sonnet" {
				t.Errorf("done payload = %+v", p)
			}
		}
	}
	rec.mu.Unlock()
	if !gotDone {
		t.Errorf("no agent.done: %v", rec.methods())
	}
}

// The tool-call cap holds on the bridge too. Claude Code cannot be stopped
// mid-turn without losing its wrap-up, so a call past the cap is answered
// "not run" and the turn ends at the wall once it does.
func TestRunCLI_holdsTheToolCallCap(t *testing.T) {
	rec := &recorder{}
	cc := &fakeClaudeCode{calls: []string{"one", "two"}}
	svc := newCLIService(t, cc, rec, 1)

	runID, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "go"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "the run to end", func() bool { return rec.hasTerminalFor(runID) })

	if len(cc.seen) != 2 {
		t.Fatalf("calls answered = %d", len(cc.seen))
	}
	if cc.seen[0].IsError {
		t.Errorf("first call should have run: %s", textOf(cc.seen[0]))
	}
	if !cc.seen[1].IsError || !strings.HasPrefix(textOf(cc.seen[1]), "not run:") {
		t.Errorf("second call = %q, want it refused at the cap", textOf(cc.seen[1]))
	}
	p := rec.errorFor(runID)
	if p == nil || p.Reason != rpc.ReasonAgentIterationLimit {
		t.Fatalf("turn did not end at the wall: %+v / %v", p, rec.methods())
	}
}

// Three failures of the same tool in a row stop the turn, as in the loop.
func TestRunCLI_stopsAtRepeatedToolErrors(t *testing.T) {
	rec := &recorder{}
	cc := &fakeClaudeCode{calls: []string{"boom", "boom", "boom", "boom"}}
	svc := newCLIService(t, cc, rec, 10)

	runID, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "go"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "the run to end", func() bool { return rec.hasTerminalFor(runID) })

	if got := textOf(cc.seen[3]); !strings.HasPrefix(got, "not run:") {
		t.Errorf("fourth call = %q, want it refused after three failures", got)
	}
	if p := rec.errorFor(runID); p == nil || p.Reason != rpc.ReasonAgentIterationLimit {
		t.Fatalf("turn did not end at the wall: %+v", p)
	}
}
