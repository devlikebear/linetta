//go:build !mobile

package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/tars/pkg/llm"
)

// errorFor returns the agent.error payload for a run, or nil if that run
// never emitted one.
func (r *recorder) errorFor(runID string) *errorPayload {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.seen {
		if e.Method != "agent.error" {
			continue
		}
		if p, ok := e.Params.(errorPayload); ok && p.RunID == runID {
			found := p
			return &found
		}
	}
	return nil
}

// httpFailingClient fails every Chat with a real *llm.ProviderError — the
// shape tars' newHTTPError builds from a non-2xx response. Two properties of
// it are the whole point of the first test below: its Message field IS the
// raw response body, and its Unwrap() returns a nil Cause, so a bare
// errors.As(err, **rpc.ReasonError) never matches it.
type httpFailingClient struct{ err *llm.ProviderError }

func (c *httpFailingClient) Ask(context.Context, string) (string, error) { return "", c.err }

func (c *httpFailingClient) Chat(context.Context, []llm.ChatMessage, llm.ChatOptions) (llm.ChatResponse, error) {
	return llm.ChatResponse{}, c.err
}

// A provider failure raised INSIDE a turn — the writer's key revoked between
// the pre-flight and the first token — must reach the panel as the same
// reason code the settings connection test would give it, with the
// provider's response body nowhere near the message.
//
// loop_test.go's TestRun_providerFailureBecomesAReasonCode covers the
// pre-flight Client() path only, which already hands back a *rpc.ReasonError.
// This is the Chat path, where nothing has classified the error yet.
func TestRun_providerFailureInsideATurnIsClassified(t *testing.T) {
	const body = `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`
	cases := []struct {
		name   string
		status int
		want   string
	}{
		{"a revoked key", 401, rpc.ReasonProviderAuthFailed},
		{"a rate limit", 429, rpc.ReasonProviderRateLimited},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recorder{}
			svc := newService(t, &httpFailingClient{err: &llm.ProviderError{
				Provider:   "anthropic",
				Operation:  "request",
				StatusCode: tc.status,
				Message:    body,
			}}, rec)

			runID, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "hi"})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			waitFor(t, "the run to end", func() bool { return rec.hasTerminalFor(runID) })

			p := rec.errorFor(runID)
			if p == nil {
				t.Fatalf("the turn did not end in agent.error: %v", rec.methods())
			}
			if p.Reason != tc.want {
				t.Errorf("reason = %q, want %q — an HTTP %d must not reach the writer as a "+
					"network problem they are told to fix by checking their connection",
					p.Reason, tc.want, tc.status)
			}
			for _, leak := range []string{body, "x-api-key", "authentication_error"} {
				if strings.Contains(p.Message, leak) {
					t.Errorf("agent.error message carries the provider's raw body (%q): %q", leak, p.Message)
				}
			}
		})
	}
}

// statusesOf returns the transcript statuses of one run's rows.
func statusesOf(t *testing.T, svc *Service, projectID, runID string) []string {
	t.Helper()
	msgs, err := svc.History(context.Background(), projectID, 100)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	var out []string
	for _, m := range msgs {
		if m.RunID == runID {
			out = append(out, m.Status)
		}
	}
	if len(out) == 0 {
		t.Fatalf("run %s left no transcript rows", runID)
	}
	return out
}

func assertAllStatus(t *testing.T, got []string, want string) {
	t.Helper()
	for _, s := range got {
		if s != want {
			t.Errorf("row status = %q, want every row of the run at %q: %v", s, want, got)
			return
		}
	}
}

// The status column is what the panel reads to decide whether to offer a
// retry (spec §10). A provider failure earns one. Nothing else on this branch
// asserts it, so a MarkRunStatus regression — the wrong column, or a status
// normalizeHistoryStatus silently rejects and turns into "done" — would be
// invisible.
func TestRun_providerFailureLeavesTheRunFailed(t *testing.T) {
	rec := &recorder{}
	svc := newService(t, &httpFailingClient{err: &llm.ProviderError{
		Provider: "anthropic", Operation: "request", StatusCode: 500, Message: "boom",
	}}, rec)

	runID, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "hi"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "the run to end", func() bool { return rec.hasTerminalFor(runID) })
	assertAllStatus(t, statusesOf(t, svc, "p1", runID), companion.HistoryStatusFailed)
}

// A turn that ran into the loop's own wall is NOT a failure. It executed
// twenty-four real tool calls; the scenes are written and the tool rows carry
// live undo batch ids. Marking those rows failed offers a retry for work that
// already landed, and makes a partial turn indistinguishable from one that
// crashed before its first token.
func TestRun_iterationLimitLeavesTheRunDoneNotFailed(t *testing.T) {
	rec := &recorder{}
	c := &scriptedClient{responses: []llm.ChatResponse{toolReply("echo", `{"text":"hi"}`)}}
	svc := newService(t, c, rec)

	runID, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "loop"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "the run to end", func() bool { return rec.hasTerminalFor(runID) })

	p := rec.errorFor(runID)
	if p == nil || p.Reason != rpc.ReasonAgentIterationLimit {
		t.Fatalf("the turn did not end at the iteration wall: %+v", p)
	}
	assertAllStatus(t, statusesOf(t, svc, "p1", runID), companion.HistoryStatusDone)
}

// And a cancelled turn reads as cancelled, not failed: the writer stopped it
// on purpose and does not need a retry button telling them it broke.
func TestRun_cancelLeavesTheRunCancelled(t *testing.T) {
	rec := &recorder{}
	blocking := &blockingClient{release: make(chan struct{})}
	svc := newService(t, blocking, rec)

	runID, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "long"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "the run to reach the model", func() bool { return blocking.entered() })
	if err := svc.Cancel(runID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	waitFor(t, "the run to end", func() bool { return rec.hasTerminalFor(runID) })
	assertAllStatus(t, statusesOf(t, svc, "p1", runID), companion.HistoryStatusCancelled)
}

// A turn whose last response carries nothing — or was cut off at the
// provider's output ceiling — did not finish. It used to end as agent.done,
// which the panel draws as nothing at all: tool lines, then a live send
// button, and a writer asking whether it is still working.
func TestRun_aTurnThatStopsWithoutFinishingSaysSo(t *testing.T) {
	cases := []struct {
		name string
		last llm.ChatResponse
		want string
	}{
		{
			"an empty last response",
			llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant"}, StopReason: "end_turn"},
			rpc.ReasonAgentEmptyReply,
		},
		{
			"a response cut off at the output ceiling",
			llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant"}, StopReason: "max_tokens"},
			rpc.ReasonAgentOutputTruncated,
		},
		{
			"a reply cut off mid-sentence",
			llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "The scene opens with"}, StopReason: "length"},
			rpc.ReasonAgentOutputTruncated,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recorder{}
			svc := newService(t, &scriptedClient{responses: []llm.ChatResponse{tc.last}}, rec)

			runID, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "rewrite every scene"})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			waitFor(t, "the run to end", func() bool { return rec.hasTerminalFor(runID) })

			p := rec.errorFor(runID)
			if p == nil {
				t.Fatalf("the turn ended without an agent.error; the panel would show nothing")
			}
			if p.Reason != tc.want {
				t.Fatalf("reason = %q, want %q", p.Reason, tc.want)
			}
			// The rows stay "done", the same as a turn that hit the tool wall:
			// whatever it did before stopping really happened.
			rows, err := svc.History(context.Background(), "p1", 0)
			if err != nil {
				t.Fatalf("History: %v", err)
			}
			for _, row := range rows {
				if row.Status != companion.HistoryStatusDone {
					t.Fatalf("row %s/%s has status %q, want done", row.Role, row.ID, row.Status)
				}
			}
		})
	}
}

// midReplyClient streams a little text and then holds its Chat call open
// until released — a turn frozen mid-reply, which is what a panel mounting
// in the middle of one finds.
type midReplyClient struct {
	streamed chan struct{}
	release  chan struct{}
}

func (c *midReplyClient) Ask(context.Context, string) (string, error) { return "", nil }

func (c *midReplyClient) Chat(ctx context.Context, _ []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	opts.OnDelta("Reading the ")
	opts.OnDelta("outline")
	close(c.streamed)
	select {
	case <-c.release:
	case <-ctx.Done():
		return llm.ChatResponse{}, ctx.Err()
	}
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "Reading the outline. Done."}}, nil
}

// agent.status is how a panel that was not there for the start of a turn
// finds out one is running, and gets the reply streamed so far.
func TestStatus_reportsARunningTurnAndItsStreamedText(t *testing.T) {
	rec := &recorder{}
	client := &midReplyClient{streamed: make(chan struct{}), release: make(chan struct{})}
	svc := newService(t, client, rec)

	if st := svc.Status("p1"); st.Running {
		t.Fatalf("status before a run = %+v", st)
	}
	runID, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "hi"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	<-client.streamed

	st := svc.Status(" p1 ")
	if !st.Running || st.RunID != runID {
		t.Fatalf("status mid-turn = %+v, want run %s running", st, runID)
	}
	if st.Text != "Reading the outline" {
		t.Fatalf("streamed text = %q", st.Text)
	}
	if st.Seq != 2 {
		t.Fatalf("seq = %d, want 2 (one per delta)", st.Seq)
	}

	close(client.release)
	waitFor(t, "the run to end", func() bool { return rec.hasTerminalFor(runID) })
	waitFor(t, "the work to be released", func() bool { return !svc.Status("p1").Running })
}
