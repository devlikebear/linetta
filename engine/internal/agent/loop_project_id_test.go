//go:build !mobile

package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/devlikebear/tars/pkg/llm"
)

// projectIDOf reads project_id off whichever agent.* payload this is. A type
// switch rather than reflection, so a sixth payload type added later fails
// this check loudly instead of silently returning ok=false.
func projectIDOf(params any) (string, bool) {
	switch p := params.(type) {
	case deltaPayload:
		return p.ProjectID, true
	case toolPayload:
		return p.ProjectID, true
	case donePayload:
		return p.ProjectID, true
	case errorPayload:
		return p.ProjectID, true
	case cancelledPayload:
		return p.ProjectID, true
	}
	return "", false
}

// agentNotifications returns every agent.* entry, so a test can assert on all
// of them at once rather than on the ones it happens to remember naming.
func (r *recorder) agentNotifications() []struct {
	Method string
	Params any
} {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]struct {
		Method string
		Params any
	}, 0, len(r.seen))
	for _, e := range r.seen {
		if strings.HasPrefix(e.Method, "agent.") {
			out = append(out, e)
		}
	}
	return out
}

// A run id says which turn an event belongs to; it does not say whose
// conversation that turn is. A turn outlives the RPC call that started it
// (#93), so its events keep arriving after the writer has jumped to another
// work — and the panel has to refuse them without reconstructing ownership
// from its own memory of which runs it started and which it abandoned. So
// every notification names the project (#95 Task 7 review round 3).
func TestRun_everyNotificationNamesItsProject(t *testing.T) {
	t.Run("a turn that streams, calls a tool, and finishes", func(t *testing.T) {
		rec := &recorder{}
		svc := newService(t, &scriptedClient{responses: []llm.ChatResponse{
			toolReply("linetta_write_scene", `{"node_id":"n1"}`),
			textReply("다 썼습니다"),
		}}, rec)

		runID, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", NodeID: "n1", Prompt: "써줘"})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		waitFor(t, "the turn to end", func() bool { return rec.hasTerminalFor(runID) })

		seen := map[string]bool{}
		for _, e := range rec.agentNotifications() {
			got, ok := projectIDOf(e.Params)
			if !ok {
				t.Fatalf("%s: params %T is not a payload projectIDOf knows", e.Method, e.Params)
			}
			if got != "p1" {
				t.Errorf("%s: project_id = %q, want p1", e.Method, got)
			}
			seen[e.Method] = true
		}
		// Guards the loop above against passing vacuously, and pins that the
		// delta and the tool events really are among the ones it checked.
		for _, m := range []string{"agent.delta", "agent.tool", "agent.done"} {
			if !seen[m] {
				t.Errorf("no %s was emitted; this turn did not exercise it", m)
			}
		}
	})

	t.Run("a turn the writer cancels", func(t *testing.T) {
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
		waitFor(t, "the turn to end", func() bool { return rec.hasTerminalFor(runID) })
		close(release)

		if !rec.has("agent.cancelled") {
			t.Fatal("no agent.cancelled was emitted")
		}
		for _, e := range rec.agentNotifications() {
			if got, _ := projectIDOf(e.Params); got != "p1" {
				t.Errorf("%s: project_id = %q, want p1", e.Method, got)
			}
		}
	})

	t.Run("a turn that fails inside the provider", func(t *testing.T) {
		rec := &recorder{}
		svc := newService(t, &httpFailingClient{err: &llm.ProviderError{StatusCode: 401, Message: "nope"}}, rec)

		runID, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "안녕"})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		waitFor(t, "agent.error", func() bool { return rec.errorFor(runID) != nil })
		if got := rec.errorFor(runID).ProjectID; got != "p1" {
			t.Errorf("agent.error project_id = %q, want p1", got)
		}
	})

	// The panel compares this against its own projectId prop, so a padded id
	// on the way in must not reach the wire padded: Run trims before it starts
	// the run, and the notification has to carry that same trimmed value.
	t.Run("the id on the wire is the trimmed one", func(t *testing.T) {
		rec := &recorder{}
		svc := newService(t, &scriptedClient{responses: []llm.ChatResponse{textReply("네")}}, rec)

		runID, err := svc.Run(context.Background(), RunRequest{ProjectID: "  p1  ", Prompt: "안녕"})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		waitFor(t, "the turn to end", func() bool { return rec.hasTerminalFor(runID) })
		for _, e := range rec.agentNotifications() {
			if got, _ := projectIDOf(e.Params); got != "p1" {
				t.Errorf("%s: project_id = %q, want p1 (trimmed)", e.Method, got)
			}
		}
	})

	// The field name is the contract, not the Go identifier: the panel reads
	// project_id, spelled the way mcp.changed already spells it.
	t.Run("the JSON key is project_id", func(t *testing.T) {
		for _, payload := range []any{
			deltaPayload{RunID: "r", ProjectID: "p1", Text: "t"},
			toolPayload{RunID: "r", ProjectID: "p1", Name: "n", State: "started"},
			donePayload{RunID: "r", ProjectID: "p1"},
			errorPayload{RunID: "r", ProjectID: "p1", Reason: "x"},
			cancelledPayload{RunID: "r", ProjectID: "p1"},
		} {
			b, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("marshal %T: %v", payload, err)
			}
			var got map[string]any
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatalf("unmarshal %T: %v", payload, err)
			}
			if got["project_id"] != "p1" {
				t.Errorf("%T: project_id = %v, want p1 (JSON was %s)", payload, got["project_id"], b)
			}
		}
	})
}
