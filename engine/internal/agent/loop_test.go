//go:build !mobile

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/provider"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/devlikebear/tars/pkg/llm"
)

// scriptedClient replays a fixed list of responses, one per Chat call, and
// records what it was asked. The last response repeats if the loop asks for
// more, which is how the iteration cap is exercised.
type scriptedClient struct {
	mu        sync.Mutex
	responses []llm.ChatResponse
	calls     int
	lastMsgs  []llm.ChatMessage
	lastOpts  llm.ChatOptions
}

func (c *scriptedClient) Ask(context.Context, string) (string, error) { return "", nil }

func (c *scriptedClient) Chat(ctx context.Context, msgs []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return llm.ChatResponse{}, err
	}
	c.calls++
	c.lastMsgs = msgs
	c.lastOpts = opts
	i := c.calls - 1
	if i >= len(c.responses) {
		i = len(c.responses) - 1
	}
	resp := c.responses[i]
	if opts.OnDelta != nil && resp.Message.Content != "" {
		opts.OnDelta(resp.Message.Content)
	}
	return resp, nil
}

func (c *scriptedClient) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func (c *scriptedClient) messages() []llm.ChatMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]llm.ChatMessage(nil), c.lastMsgs...)
}

func (c *scriptedClient) options() llm.ChatOptions {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastOpts
}

type fakeProviders struct {
	client llm.Client
	err    error
}

func (f fakeProviders) Client(string) (llm.Client, provider.Resolved, error) {
	return f.client, provider.Resolved{ID: "anthropic", Model: "test-model"}, f.err
}

// recorder collects notifications so a test can assert on the stream the
// panel will see.
type recorder struct {
	mu   sync.Mutex
	seen []struct {
		Method string
		Params any
	}
}

func (r *recorder) notify(method string, params any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = append(r.seen, struct {
		Method string
		Params any
	}{method, params})
}

func (r *recorder) methods() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.seen))
	for _, e := range r.seen {
		out = append(out, e.Method)
	}
	return out
}

func (r *recorder) has(method string) bool {
	for _, m := range r.methods() {
		if m == method {
			return true
		}
	}
	return false
}

func textReply(text string) llm.ChatResponse {
	return llm.ChatResponse{
		Message: llm.ChatMessage{Role: "assistant", Content: text},
		Usage:   llm.Usage{InputTokens: 10, OutputTokens: 5},
	}
}

func toolReply(name, args string) llm.ChatResponse {
	return llm.ChatResponse{
		Message: llm.ChatMessage{
			Role:      "assistant",
			ToolCalls: []llm.ToolCall{{ID: "call-1", Name: name, Arguments: args}},
		},
	}
}

// openStoreForAgentTests opens a real on-disk store and seeds the minimal
// parent rows companion_messages needs. store.Open turns on
// PRAGMA foreign_keys=ON, and companion_messages has FOREIGN KEYs to
// projects and nodes (see engine/internal/store/migrations/0001_init.sql and
// 0013_companion_messages.sql), so a bare "p1"/"n1" insert would fail
// without them. transcript_test.go's newTranscript helper solves the same
// problem the same way.
func openStoreForAgentTests(t *testing.T) *store.Store {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir()+"/test.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	if _, err := st.DB().ExecContext(ctx, `
INSERT INTO projects (id, title, genres, length_target, default_pov, created_at, updated_at)
VALUES ('p1', 'Test', '["SF"]', 'novel', 'first', 0, 0)`); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if _, err := st.DB().ExecContext(ctx, `
INSERT INTO nodes (id, project_id, parent_id, ordinal, kind, label, title, content_doc, created_at, updated_at)
VALUES ('n1', 'p1', NULL, 0, 'leaf', 'scene 1', 'opening', '{}', 0, 0)`); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	return st
}

func newService(t *testing.T, client llm.Client, rec *recorder) *Service {
	t.Helper()
	st := openStoreForAgentTests(t)
	svc := New(Deps{
		Providers: fakeProviders{client: client},
		History:   companion.NewHistoryRepo(st.DB()),
		Scope:     fakeScope{titles: map[string]string{"p1": "제목"}},
		Register:  stubTools(nil),
		Notify:    rec.notify,
		Language:  func() string { return "ko" },
		Clock:     func() int64 { return 1700000000000 },
	})
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

// waitFor polls until cond holds or the deadline passes. Run is asynchronous
// by contract — it returns a run id and the work happens in a goroutine — so
// every assertion about the outcome has to wait for it.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestRun_returnsARunIDImmediatelyAndStreams(t *testing.T) {
	rec := &recorder{}
	svc := newService(t, &scriptedClient{responses: []llm.ChatResponse{textReply("좋아요")}}, rec)

	runID, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "안녕"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if runID == "" {
		t.Fatal("Run must return a run id the panel can cancel by")
	}
	waitFor(t, "agent.done", func() bool { return rec.has("agent.done") })
	if !rec.has("agent.delta") {
		t.Error("no agent.delta was emitted; the panel would show nothing until the end")
	}
}

// The scope line is why the agent does not have to ask "which scene?".
func TestRun_prefixesThePromptWithTheScopeLine(t *testing.T) {
	rec := &recorder{}
	c := &scriptedClient{responses: []llm.ChatResponse{textReply("ok")}}
	svc := newService(t, c, rec)

	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", NodeID: "n1", Prompt: "이 씬 고쳐줘"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "agent.done", func() bool { return rec.has("agent.done") })

	msgs := c.messages()
	if len(msgs) == 0 {
		t.Fatal("no messages reached the model")
	}
	last := msgs[len(msgs)-1]
	if !strings.Contains(last.Content, "[work: p1") || !strings.Contains(last.Content, "이 씬 고쳐줘") {
		t.Errorf("last message = %q, want the scope line then the prompt", last.Content)
	}
	if msgs[0].Role != "system" {
		t.Errorf("first message role = %q, want system", msgs[0].Role)
	}
}

// The whole design in one assertion: the model is offered the MCP tools,
// with the MCP layer's own descriptions.
func TestRun_offersTheMCPToolsToTheModel(t *testing.T) {
	rec := &recorder{}
	c := &scriptedClient{responses: []llm.ChatResponse{textReply("ok")}}
	svc := newService(t, c, rec)
	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "hi"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "agent.done", func() bool { return rec.has("agent.done") })

	tools := c.options().Tools
	if len(tools) == 0 {
		t.Fatal("the model was offered no tools")
	}
	if tools[0].Function.Description == "" {
		t.Error("tool descriptions must reach the model; they carry the workflow")
	}
}

func TestRun_executesAToolCallAndFeedsTheResultBack(t *testing.T) {
	rec := &recorder{}
	c := &scriptedClient{responses: []llm.ChatResponse{
		toolReply("echo", `{"text":"hi"}`),
		textReply("다 했어요"),
	}}
	svc := newService(t, c, rec)
	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "go"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "agent.done", func() bool { return rec.has("agent.done") })

	if c.count() != 2 {
		t.Fatalf("model was called %d times, want 2", c.count())
	}
	msgs := c.messages()
	var sawToolRole bool
	for _, m := range msgs {
		if m.Role == "tool" && strings.Contains(m.Content, "echo: hi") {
			sawToolRole = true
		}
	}
	if !sawToolRole {
		t.Errorf("the tool result never reached the model: %+v", msgs)
	}
	if !rec.has("agent.tool") {
		t.Error("no agent.tool notification; the panel would show no activity")
	}
}

// A runaway agent must hit a wall before it rewrites forty scenes.
func TestRun_stopsAtTheIterationCap(t *testing.T) {
	rec := &recorder{}
	c := &scriptedClient{responses: []llm.ChatResponse{toolReply("echo", `{"text":"hi"}`)}}
	svc := newService(t, c, rec)
	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "loop"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "the run to end", func() bool { return rec.has("agent.done") || rec.has("agent.error") })

	if c.count() > maxIterations {
		t.Errorf("model called %d times, want at most %d", c.count(), maxIterations)
	}
	if !rec.has("agent.error") {
		t.Error("hitting the cap must be reported, not silently swallowed")
	}
}

// Three identical failures in a row means the model is not learning from the
// error. Handing it a fourth wastes the writer's tokens.
func TestRun_stopsAfterTheSameToolFailsThreeTimes(t *testing.T) {
	rec := &recorder{}
	c := &scriptedClient{responses: []llm.ChatResponse{toolReply("echo", `{"text":"boom"}`)}}
	svc := newService(t, c, rec)
	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "fail"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "the run to end", func() bool { return rec.has("agent.done") || rec.has("agent.error") })

	if c.count() > maxRepeatedToolErrors+1 {
		t.Errorf("model called %d times, want the loop cut at %d", c.count(), maxRepeatedToolErrors+1)
	}
}

func TestRun_secondRunOnTheSameWorkIsBusy(t *testing.T) {
	rec := &recorder{}
	release := make(chan struct{})
	blocking := &blockingClient{release: release}
	svc := newService(t, blocking, rec)

	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "first"}); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	waitFor(t, "the first run to reach the model", func() bool { return blocking.entered() })

	_, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "second"})
	if err == nil {
		t.Fatal("a second run on the same work must be refused")
	}
	var re *rpc.ReasonError
	if !errors.As(err, &re) || re.Reason != rpc.ReasonAgentBusy {
		t.Fatalf("err = %v, want a %s reason", err, rpc.ReasonAgentBusy)
	}
	close(release)
}

func TestRun_cancelEndsTheTurnAndReportsIt(t *testing.T) {
	rec := &recorder{}
	release := make(chan struct{})
	blocking := &blockingClient{release: release}
	svc := newService(t, blocking, rec)

	runID, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "long"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "the run to reach the model", func() bool { return blocking.entered() })

	if err := svc.Cancel(runID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	waitFor(t, "agent.cancelled", func() bool { return rec.has("agent.cancelled") })
	close(release)

	// The work is free again once the cancelled run tears down.
	waitFor(t, "the work to be released", func() bool {
		_, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "next"})
		return err == nil
	})
}

// blockingClient parks inside Chat until released or cancelled.
type blockingClient struct {
	mu      sync.Mutex
	arrived bool
	release chan struct{}
}

func (b *blockingClient) entered() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.arrived
}

func (b *blockingClient) Ask(context.Context, string) (string, error) { return "", nil }

func (b *blockingClient) Chat(ctx context.Context, _ []llm.ChatMessage, _ llm.ChatOptions) (llm.ChatResponse, error) {
	b.mu.Lock()
	b.arrived = true
	b.mu.Unlock()
	select {
	case <-ctx.Done():
		return llm.ChatResponse{}, ctx.Err()
	case <-b.release:
		return textReply("released"), nil
	}
}

// A provider failure is a reason code, never the provider's raw body.
func TestRun_providerFailureBecomesAReasonCode(t *testing.T) {
	rec := &recorder{}
	st := openStoreForAgentTests(t)
	svc := New(Deps{
		Providers: fakeProviders{err: &rpc.ReasonError{Reason: rpc.ReasonProviderConsentRequired}},
		History:   companion.NewHistoryRepo(st.DB()),
		Scope:     fakeScope{},
		Register:  stubTools(nil),
		Notify:    rec.notify,
		Language:  func() string { return "ko" },
	})
	t.Cleanup(func() { _ = svc.Close() })

	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "hi"}); err == nil {
		t.Fatal("a run without consent must be refused")
	}
}

func TestRun_writesTheTurnToTheTranscript(t *testing.T) {
	rec := &recorder{}
	c := &scriptedClient{responses: []llm.ChatResponse{
		toolReply("echo", `{"text":"hi"}`),
		textReply("끝"),
	}}
	svc := newService(t, c, rec)
	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "go"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "agent.done", func() bool { return rec.has("agent.done") })

	msgs, err := svc.History(context.Background(), "p1", 50)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	roles := map[string]int{}
	for _, m := range msgs {
		roles[m.Role]++
	}
	if roles["user"] != 1 || roles["assistant"] < 1 || roles["tool"] != 1 {
		t.Errorf("transcript roles = %v, want one user, one tool and a reply", roles)
	}
	for _, m := range msgs {
		if m.Role != "tool" {
			continue
		}
		var ev toolEvent
		if err := json.Unmarshal([]byte(m.Content), &ev); err != nil {
			t.Errorf("tool row is not JSON: %s", m.Content)
		}
	}
}
