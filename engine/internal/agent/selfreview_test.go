//go:build !mobile

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/tars/pkg/llm"
)

// ---------- fixtures ----------

// isSelfReviewCall tells a review's Chat from a turn's by the system prompt it
// carries. It keys on selfReviewMarker — a sentence of the real prompt — rather
// than on a copy of the wording, so an edit to the prompt that dropped the
// review's framing breaks these tests instead of silently letting them keep
// matching a string nothing sends any more.
func isSelfReviewCall(msgs []llm.ChatMessage) bool {
	return len(msgs) > 0 && strings.Contains(msgs[0].Content, selfReviewMarker)
}

// reviewClient scripts the turn and the review separately, and can park the
// review's Chat under the test's own control — no timers anywhere. The last
// response of each script repeats, matching scriptedClient.
type reviewClient struct {
	mu             sync.Mutex
	turn           []llm.ChatResponse
	review         []llm.ChatResponse
	turnCalls      int
	reviewCalls    int
	reviewRequests []string
	sawCancel      bool
	unwound        bool

	entered     chan struct{}
	enteredOnce sync.Once

	// release, when non-nil, parks every review Chat until the test closes it.
	// waitForCancel additionally makes the parked call wait for its context to
	// be cancelled first and record that it saw it — which is how a test knows
	// Close has run cancelAll and is now inside wg.Wait().
	release       chan struct{}
	releaseOnce   sync.Once
	waitForCancel bool
}

// letGo releases a parked review, once. Every test that parks one registers
// this with t.Cleanup as well as calling it at the right moment: a t.Fatal
// between the two would otherwise leave the review parked forever, and
// Service.Close — which correctly waits for it — would hang the whole package
// instead of reporting the failure.
func (c *reviewClient) letGo() {
	if c.release == nil {
		return
	}
	c.releaseOnce.Do(func() { close(c.release) })
}

func newReviewClient(turn, review []llm.ChatResponse) *reviewClient {
	return &reviewClient{turn: turn, review: review, entered: make(chan struct{})}
}

func (c *reviewClient) Ask(context.Context, string) (string, error) { return "", nil }

func (c *reviewClient) Chat(ctx context.Context, msgs []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	if isSelfReviewCall(msgs) {
		return c.reviewChat(ctx, msgs)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return llm.ChatResponse{}, err
	}
	c.turnCalls++
	resp := c.turn[pick(c.turnCalls, len(c.turn))]
	if opts.OnDelta != nil && resp.Message.Content != "" {
		opts.OnDelta(resp.Message.Content)
	}
	return resp, nil
}

func (c *reviewClient) reviewChat(ctx context.Context, msgs []llm.ChatMessage) (llm.ChatResponse, error) {
	c.mu.Lock()
	c.reviewCalls++
	resp := c.review[pick(c.reviewCalls, len(c.review))]
	if len(msgs) > 1 {
		c.reviewRequests = append(c.reviewRequests, msgs[1].Content)
	}
	release, waitForCancel := c.release, c.waitForCancel
	c.mu.Unlock()

	c.enteredOnce.Do(func() { close(c.entered) })

	if release != nil {
		if waitForCancel {
			// Deliberately NOT returning on cancellation: a provider request
			// already on the wire does not unwind the instant the context is
			// cancelled, and a Close that does not wait for this is exactly
			// the hazard.
			<-ctx.Done()
			c.mu.Lock()
			c.sawCancel = true
			c.mu.Unlock()
		}
		<-release
		c.mu.Lock()
		c.unwound = true
		c.mu.Unlock()
	}
	return resp, nil
}

func pick(nth, total int) int {
	if nth-1 >= total {
		return total - 1
	}
	return nth - 1
}

func (c *reviewClient) reviews() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reviewCalls
}

func (c *reviewClient) requests() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.reviewRequests...)
}

func (c *reviewClient) cancelSeen() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sawCancel
}

func (c *reviewClient) hasUnwound() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.unwound
}

// settingProbe stands in for settings.agent_self_review_enabled AND doubles as
// the test's synchronisation point for "was a review considered at all". It is
// consulted synchronously on the turn's own goroutine, after the threshold
// check and before any goroutine is spawned — so a Close (which waits for that
// goroutine) is a hard barrier: once it returns, this counter is final.
type settingProbe struct {
	mu    sync.Mutex
	calls int
	on    bool
}

func (p *settingProbe) enabled() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	return p.on
}

func (p *settingProbe) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

// skillCounter records what the stub skill tools were asked to do.
type skillCounter struct {
	mu    sync.Mutex
	edits int
	reads int
}

func (s *skillCounter) editCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.edits
}

type skillIn struct {
	Name string `json:"name,omitempty"`
}

// skillTools is stubTools plus stand-ins for the two tools the review is
// allowed to call. The review refuses to start at all without
// linetta_edit_skill on the server (a review that cannot write a skill is a
// provider call with nothing to do), so every test here registers them.
func skillTools(rec *skillCounter) RegisterTools {
	base := stubTools(nil)
	return func(s *mcp.Server) {
		base(s)
		mcp.AddTool(s, &mcp.Tool{Name: "linetta_edit_skill", Description: "write a skill"},
			func(_ context.Context, _ *mcp.CallToolRequest, in skillIn) (*mcp.CallToolResult, struct{}, error) {
				rec.mu.Lock()
				rec.edits++
				rec.mu.Unlock()
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: "saved skill " + in.Name}},
				}, struct{}{}, nil
			})
		mcp.AddTool(s, &mcp.Tool{Name: "linetta_read_skill", Description: "read a skill"},
			func(_ context.Context, _ *mcp.CallToolRequest, in skillIn) (*mcp.CallToolResult, struct{}, error) {
				rec.mu.Lock()
				rec.reads++
				rec.mu.Unlock()
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: "body of " + in.Name}},
				}, struct{}{}, nil
			})
	}
}

// newReviewService wires a service whose skill tools exist and whose
// self-review setting the test controls. It deliberately does NOT register a
// t.Cleanup Close: every test here calls Close itself at the point that makes
// its assertion deterministic, and a second Close is a no-op.
func newReviewService(t *testing.T, c llm.Client, rec *recorder, probe *settingProbe, skills *skillCounter) *Service {
	t.Helper()
	st := openStoreForAgentTests(t)
	svc := New(Deps{
		Providers:         fakeProviders{client: c},
		History:           companion.NewHistoryRepo(st.DB()),
		Scope:             fakeScope{titles: map[string]string{"p1": "제목"}},
		Register:          skillTools(skills),
		Notify:            rec.notify,
		Language:          func() string { return "ko" },
		SelfReviewEnabled: probe.enabled,
		Clock:             func() int64 { return 1700000000000 },
	})
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

// burstReply is one response asking for n echo calls at once — the cheapest
// way to give a turn an exact executed-tool-call count.
func burstReply(n int) llm.ChatResponse {
	calls := make([]llm.ToolCall, n)
	for i := range calls {
		calls[i] = llm.ToolCall{ID: fmt.Sprintf("call-%d", i), Name: "echo", Arguments: `{"text":"hi"}`}
	}
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", ToolCalls: calls}}
}

func doneCount(rec *recorder) int {
	n := 0
	for _, m := range rec.methods() {
		if m == "agent.done" {
			n++
		}
	}
	return n
}

// ---------- the threshold ----------

// The trigger is "this turn did real work", and the number is 8. Both halves
// are asserted against the same barrier: Close waits for the turn's goroutine,
// and the turn's goroutine consults the setting synchronously — so once Close
// has returned, the probe's count is the final answer to "was a review
// considered", with no polling and no timer.
func TestSelfReview_startsAtEightExecutedToolCallsAndNotAtSeven(t *testing.T) {
	if selfReviewThreshold != 8 {
		t.Fatalf("selfReviewThreshold = %d, want 8 — the counts below are literals on purpose",
			selfReviewThreshold)
	}
	for _, tc := range []struct {
		name      string
		toolCalls int
		want      int
	}{
		// Literal 7 and 8, deliberately not selfReviewThreshold±1: a table
		// written in terms of the constant moves with it, so lowering the
		// threshold to 7 would still pass — which it did, the first time this
		// test was mutation-checked.
		{"one short of the threshold", 7, 0},
		{"exactly the threshold", 8, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recorder{}
			probe := &settingProbe{on: true}
			c := newReviewClient(
				[]llm.ChatResponse{burstReply(tc.toolCalls), textReply("끝")},
				[]llm.ChatResponse{textReply("")},
			)
			svc := newReviewService(t, c, rec, probe, &skillCounter{})

			if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "go"}); err != nil {
				t.Fatalf("Run: %v", err)
			}
			waitFor(t, "agent.done", func() bool { return rec.has("agent.done") })
			if err := svc.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}

			if got := probe.count(); got != tc.want {
				t.Errorf("a turn of %d tool calls considered a review %d times, want %d "+
					"(the threshold is %d)", tc.toolCalls, got, tc.want, selfReviewThreshold)
			}
			if tc.want == 0 && c.reviews() != 0 {
				t.Errorf("a turn below the threshold made %d review calls to the provider", c.reviews())
			}
			if tc.want > 0 {
				waitFor(t, "the review to reach the provider", func() bool { return c.reviews() > 0 })
			}
		})
	}
}

// The review's request carries the tool NAMES the turn called — not their
// arguments and not their results. It is a background provider call the writer
// did not ask for and will never read; sending the manuscript through it would
// be a second copy of their work leaving the machine for nothing.
func TestSelfReview_sendsTheToolNamesAndNotTheirContents(t *testing.T) {
	rec := &recorder{}
	probe := &settingProbe{on: true}
	c := newReviewClient(
		[]llm.ChatResponse{burstReply(8), textReply("끝")},
		[]llm.ChatResponse{textReply("")},
	)
	svc := newReviewService(t, c, rec, probe, &skillCounter{})

	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "비밀 원고"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "the review to reach the provider", func() bool { return c.reviews() > 0 })

	reqs := c.requests()
	if len(reqs) == 0 {
		t.Fatal("the review sent no request")
	}
	if !strings.Contains(reqs[0], "echo") {
		t.Errorf("the review was not told which tools the turn called:\n%s", reqs[0])
	}
	// "hi" is echo's argument and "echo: hi" is its result; neither may travel.
	if strings.Contains(reqs[0], "echo: hi") || strings.Contains(reqs[0], `"text":"hi"`) {
		t.Errorf("the review carries the turn's tool arguments or results:\n%s", reqs[0])
	}
	if strings.Contains(reqs[0], "비밀 원고") {
		t.Errorf("the review carries the writer's own message:\n%s", reqs[0])
	}
}

// ---------- the setting ----------

// Off means off at every count, including a turn that ran to the iteration
// wall. The setting is read at the END of the turn, so this also pins that a
// writer who switches it off gets no review out of the turn in flight.
func TestSelfReview_doesNotStartWhenTheSettingIsOff(t *testing.T) {
	rec := &recorder{}
	probe := &settingProbe{on: false}
	c := newReviewClient(
		[]llm.ChatResponse{burstReply(maxIterations + 4), textReply("끝")},
		[]llm.ChatResponse{textReply("")},
	)
	svc := newReviewService(t, c, rec, probe, &skillCounter{})

	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "go"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// A burst larger than the cap ends at the wall, which is the other seam a
	// review attaches to — so this covers "at any count" in the direction that
	// matters.
	waitFor(t, "the run to end", func() bool { return rec.has("agent.error") })
	if err := svc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if probe.count() == 0 {
		t.Fatal("the setting was never consulted — this test would pass even if the switch were ignored")
	}
	if c.reviews() != 0 {
		t.Errorf("the review ran %d times with the setting off", c.reviews())
	}
}

// A turn that ends at the iteration wall produced work — twenty-four tool
// calls of it — and the wall ends "done" on purpose. It gets a review.
func TestSelfReview_alsoRunsForATurnThatEndedAtTheWall(t *testing.T) {
	rec := &recorder{}
	probe := &settingProbe{on: true}
	c := newReviewClient(
		[]llm.ChatResponse{burstReply(maxIterations + 4), textReply("끝")},
		[]llm.ChatResponse{textReply("")},
	)
	svc := newReviewService(t, c, rec, probe, &skillCounter{})

	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "go"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "the wall", func() bool { return rec.has("agent.error") })
	waitFor(t, "the review to reach the provider", func() bool { return c.reviews() > 0 })
}

// ---------- invisibility ----------

// Hazard 4. A review that writes a skill leaves exactly one trace: the
// skills.changed the tool emits for itself. No second agent.done, no
// agent.tool for its own calls, and not one transcript row — a review row
// would appear in the panel under a turn the writer has already read and moved
// on from.
func TestSelfReview_leavesNoTranscriptRowAndNoPanelNotification(t *testing.T) {
	rec := &recorder{}
	probe := &settingProbe{on: true}
	skills := &skillCounter{}
	c := newReviewClient(
		[]llm.ChatResponse{burstReply(8), textReply("끝")},
		[]llm.ChatResponse{
			toolReply("linetta_edit_skill", `{"name":"revising-a-scene"}`),
			textReply("스킬을 적어 두었습니다"),
		},
	)
	svc := newReviewService(t, c, rec, probe, skills)

	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "go"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "the review to write a skill", func() bool { return skills.editCount() > 0 })
	if err := svc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Positive control: without this the assertions below would pass on a
	// review that never ran at all.
	if skills.editCount() != 1 {
		t.Fatalf("the review made %d skill edits, want 1", skills.editCount())
	}

	if n := doneCount(rec); n != 1 {
		t.Errorf("agent.done was emitted %d times, want 1 — the review must not end a second turn", n)
	}
	for _, e := range rec.seen {
		p, ok := e.Params.(toolPayload)
		if !ok {
			continue
		}
		if p.Name == "linetta_edit_skill" {
			t.Errorf("the review emitted agent.tool for %s; the panel would draw its private call "+
				"into the writer's activity list", p.Name)
		}
	}

	msgs, err := svc.History(context.Background(), "p1", 100)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	for _, m := range msgs {
		if strings.Contains(m.Content, "스킬을 적어 두었습니다") {
			t.Errorf("the review's reply was written to the transcript: %+v", m)
		}
		if m.Role != "tool" {
			continue
		}
		var ev toolEvent
		if err := json.Unmarshal([]byte(m.Content), &ev); err != nil {
			continue
		}
		if ev.Name == "linetta_edit_skill" {
			t.Errorf("the review's tool call was written to the transcript: %+v", ev)
		}
	}
}

// ---------- hazard 2: Close ----------

// Close must not return while a review is still calling the provider: the
// caller is free to close the store the moment it does, and a review that
// outlived it would go on writing against that store.
//
// Synchronisation is entirely hand-controlled. The review's Chat parks; the
// test waits until that parked call has SEEN its context cancelled, which
// happens inside Close's own cancelAll — so at that point Close has started
// and is either inside wg.Wait() (correct) or already finished (the bug). Only
// then is the review released.
func TestSelfReview_closeWaitsForAReviewInFlight(t *testing.T) {
	rec := &recorder{}
	probe := &settingProbe{on: true}
	c := newReviewClient(
		[]llm.ChatResponse{burstReply(8), textReply("끝")},
		[]llm.ChatResponse{textReply("")},
	)
	c.release = make(chan struct{})
	c.waitForCancel = true
	svc := newReviewService(t, c, rec, probe, &skillCounter{})
	// Registered AFTER the service, so t.Cleanup's LIFO order releases the
	// parked review BEFORE Close waits for it. The other way round, a t.Fatal
	// anywhere below would deadlock the package in cleanup instead of
	// reporting the failure — which is precisely what Close waiting correctly
	// on a review buys you.
	t.Cleanup(c.letGo)

	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "go"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	<-c.entered // the review is inside Chat, parked

	type closeResult struct {
		err error
		// unwound is read the instant Close returns. Correct code cannot
		// observe false here: the review sets it before its Chat returns,
		// which is before selfReview returns, which is before s.leave().
		unwound bool
	}
	done := make(chan closeResult, 1)
	go func() {
		err := svc.Close()
		done <- closeResult{err: err, unwound: c.hasUnwound()}
	}()

	waitFor(t, "Close to cancel the review", func() bool { return c.cancelSeen() })
	select {
	case r := <-done:
		t.Fatalf("Close returned (err=%v) while the review was still inside the provider call", r.err)
	default:
	}

	c.letGo()
	r := <-done
	if r.err != nil {
		t.Fatalf("Close: %v", r.err)
	}
	if !r.unwound {
		t.Error("Close returned before the review had unwound; the caller is now free to close " +
			"the store underneath it")
	}
}

// ---------- hazard 3: the writer's next message ----------

// The review must never hold the work. It runs after the reply has gone, and a
// writer typing their next message immediately is the normal case — being told
// "this work already has a turn running" because a janitor is thinking would
// be a bug the writer feels on the very first turn that crosses the threshold.
func TestSelfReview_doesNotBlockTheWritersNextMessage(t *testing.T) {
	rec := &recorder{}
	probe := &settingProbe{on: true}
	c := newReviewClient(
		[]llm.ChatResponse{burstReply(8), textReply("끝")},
		[]llm.ChatResponse{textReply("")},
	)
	c.release = make(chan struct{})
	svc := newReviewService(t, c, rec, probe, &skillCounter{})
	t.Cleanup(c.letGo) // after the service: cleanup is LIFO, and Close waits

	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "first"}); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	<-c.entered // the review is in flight on p1, right now

	secondID, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "second"})
	if err != nil {
		var re *rpc.ReasonError
		if errors.As(err, &re) && re.Reason == rpc.ReasonAgentBusy {
			t.Fatalf("the writer's next message was refused with %s because a self-review was "+
				"running on the same work", rpc.ReasonAgentBusy)
		}
		t.Fatalf("second Run: %v", err)
	}
	waitFor(t, "the second turn to finish", func() bool { return rec.hasTerminalFor(secondID) })

	c.letGo()
	if err := svc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// ---------- hazard: the janitor stays a janitor ----------

// A review whose model keeps asking for tools stops at its own cap — four
// calls, plus the one round trip in which it says what it did. Without that
// this is a second turn running unattended behind the writer's back, with no
// panel to show it and no stop button to end it.
func TestSelfReview_stopsAtItsOwnToolCap(t *testing.T) {
	// Literals below, not selfReviewMaxToolCalls: a test written in terms of
	// the cap moves with it and would pass at any value.
	const wantToolCalls, wantRoundTrips = 4, 5
	if selfReviewMaxToolCalls != wantToolCalls {
		t.Fatalf("selfReviewMaxToolCalls = %d, want %d", selfReviewMaxToolCalls, wantToolCalls)
	}
	rec := &recorder{}
	probe := &settingProbe{on: true}
	skills := &skillCounter{}
	c := newReviewClient(
		[]llm.ChatResponse{burstReply(8), textReply("끝")},
		// Repeats forever: the review has to be the one that stops.
		[]llm.ChatResponse{toolReply("linetta_edit_skill", `{"name":"again"}`)},
	)
	svc := newReviewService(t, c, rec, probe, skills)

	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "go"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Waits on the ROUND TRIP count, not the tool count: the review makes one
	// last request after its budget is spent, and waiting on the tool count
	// would let Close cancel that final round trip and turn this into a test
	// of when Close lands.
	waitFor(t, "the review to spend its tool budget", func() bool {
		return c.reviews() >= wantRoundTrips
	})
	if err := svc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if got := skills.editCount(); got != wantToolCalls {
		t.Errorf("the review made %d tool calls, want exactly %d", got, wantToolCalls)
	}
	// Four calls, then one last round trip, and no more: a fifth response
	// asking for tools must not buy a sixth request.
	if got := c.reviews(); got != wantRoundTrips {
		t.Errorf("the review made %d provider calls, want %d — its %d tool calls plus the one "+
			"round trip after them", got, wantRoundTrips, wantToolCalls)
	}
}

// A model that keeps asking for a tool it was not offered gets a refusal, not
// an execution — so the tool cap never moves, and only the round-trip wall
// stops the loop. Without it this is minutes of provider calls for a pass the
// writer never asked for.
func TestSelfReview_stopsEvenWhenItsCallsAreAllRefused(t *testing.T) {
	const wantRoundTrips = 5
	rec := &recorder{}
	probe := &settingProbe{on: true}
	skills := &skillCounter{}
	c := newReviewClient(
		[]llm.ChatResponse{burstReply(8), textReply("끝")},
		// A tool the review was never offered, forever.
		[]llm.ChatResponse{toolReply("linetta_update_scene", `{"node_id":"n1"}`)},
	)
	svc := newReviewService(t, c, rec, probe, skills)

	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "go"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "the review to hit its round-trip wall", func() bool { return c.reviews() >= wantRoundTrips })
	if err := svc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if got := c.reviews(); got != wantRoundTrips {
		t.Errorf("the review made %d provider calls, want %d", got, wantRoundTrips)
	}
	if got := skills.editCount(); got != 0 {
		t.Errorf("a tool the review was not offered was executed %d times", got)
	}
}

// The review is offered the skill tools and nothing else. It runs unattended
// after the writer has stopped reading; handing it linetta_update_scene would
// make it a second, invisible writing turn.
func TestSelfReview_isOfferedOnlyTheSkillTools(t *testing.T) {
	schemas := []llm.ToolSchema{
		{Function: llm.ToolFunctionSchema{Name: "linetta_update_scene"}},
		{Function: llm.ToolFunctionSchema{Name: "linetta_edit_skill"}},
		{Function: llm.ToolFunctionSchema{Name: "linetta_read_skill"}},
		{Function: llm.ToolFunctionSchema{Name: "linetta_delete_node"}},
	}
	got := onlySelfReviewTools(schemas)
	if len(got) != 2 {
		t.Fatalf("the review was offered %d tools, want the two skill tools: %+v", len(got), got)
	}
	for _, sc := range got {
		if !selfReviewTools[sc.Function.Name] {
			t.Errorf("the review was offered %s", sc.Function.Name)
		}
	}
}
