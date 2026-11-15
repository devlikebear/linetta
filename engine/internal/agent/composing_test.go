package agent

import (
	"context"
	"testing"

	"github.com/devlikebear/tars/pkg/llm"
)

// The panel learns which call the model is writing before the call runs: one
// agent.composing per call, ahead of that call's agent.tool "started".
func TestRun_namesTheToolCallTheModelIsWriting(t *testing.T) {
	rec := &recorder{}
	c := &scriptedClient{responses: []llm.ChatResponse{
		toolReply("echo", `{"text":"hi"}`),
		textReply("다 했어요"),
	}}
	svc := newService(t, c, rec)
	runID, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "go"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "agent.done", func() bool { return rec.has("agent.done") })

	rec.mu.Lock()
	defer rec.mu.Unlock()
	composed, started := -1, -1
	for i, e := range rec.seen {
		switch p := e.Params.(type) {
		case composingPayload:
			if composed != -1 {
				t.Fatalf("composing reported twice for one call")
			}
			if p.Name != "echo" || p.RunID != runID || p.ProjectID != "p1" {
				t.Fatalf("composing payload = %+v", p)
			}
			composed = i
		case toolPayload:
			if p.State == "started" {
				started = i
			}
		}
	}
	if composed == -1 || started == -1 || composed > started {
		t.Fatalf("composing at %d, tool started at %d: want composing first (%v)", composed, started, rec.methods())
	}
}

// A reply with no tool calls composes nothing, and neither does the wall's
// wrap-up, whose tool calls are forbidden and never run.
func TestRun_composesNothingWithoutAToolCall(t *testing.T) {
	rec := &recorder{}
	c := &scriptedClient{responses: []llm.ChatResponse{textReply("네")}}
	svc := newService(t, c, rec)
	runAndWait(t, svc, rec, "안녕")
	if rec.has("agent.composing") {
		t.Fatalf("a text-only turn reported composing: %v", rec.methods())
	}
}
