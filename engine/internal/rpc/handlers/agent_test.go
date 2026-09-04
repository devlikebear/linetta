package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/rpc"
)

type fakeAgent struct {
	runID     string
	runErr    error
	sawPrompt string
	sawNode   string
	cancelled string
	cleared   string
	undone    string
}

func (f *fakeAgent) Run(_ context.Context, projectID, nodeID, prompt string) (string, error) {
	f.sawPrompt, f.sawNode = prompt, nodeID
	return f.runID, f.runErr
}
func (f *fakeAgent) Cancel(_ context.Context, runID string) error { f.cancelled = runID; return nil }
func (f *fakeAgent) History(context.Context, string, int) (json.RawMessage, error) {
	return json.RawMessage(`[]`), nil
}
func (f *fakeAgent) Clear(_ context.Context, projectID string) error { f.cleared = projectID; return nil }
func (f *fakeAgent) Undo(_ context.Context, batchID string) error    { f.undone = batchID; return nil }

func TestAgentRun_returnsTheRunID(t *testing.T) {
	f := &fakeAgent{runID: "run-1"}
	out, err := AgentRun(f)(context.Background(),
		json.RawMessage(`{"project_id":"p1","node_id":"n1","prompt":"안녕"}`))
	if err != nil {
		t.Fatalf("AgentRun: %v", err)
	}
	var got struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.RunID != "run-1" {
		t.Errorf("run_id = %q", got.RunID)
	}
	if f.sawPrompt != "안녕" || f.sawNode != "n1" {
		t.Errorf("controller saw prompt=%q node=%q", f.sawPrompt, f.sawNode)
	}
}

// The busy reason has to survive the handler, or the panel prints the
// engine's English sentence instead of a translated one.
func TestAgentRun_carriesTheBusyReasonThrough(t *testing.T) {
	f := &fakeAgent{runErr: &rpc.ReasonError{Reason: rpc.ReasonAgentBusy, Err: errors.New("busy")}}
	_, err := AgentRun(f)(context.Background(), json.RawMessage(`{"project_id":"p1","prompt":"x"}`))
	if err == nil {
		t.Fatal("want an error")
	}
	var me *rpc.MethodError
	if !errors.As(err, &me) {
		t.Fatalf("err = %T, want *rpc.MethodError", err)
	}
	if string(me.Data) != `{"reason":"agent_busy"}` {
		t.Errorf("data = %s", me.Data)
	}
}

func TestAgentRun_requiresAPrompt(t *testing.T) {
	f := &fakeAgent{runID: "run-1"}
	if _, err := AgentRun(f)(context.Background(), json.RawMessage(`{"project_id":"p1","prompt":"  "}`)); err == nil {
		t.Fatal("an empty prompt must be refused before a provider is dialled")
	}
}

func TestAgentCancelAndClearAndUndoReachTheController(t *testing.T) {
	f := &fakeAgent{}
	if _, err := AgentCancel(f)(context.Background(), json.RawMessage(`{"run_id":"run-3"}`)); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := AgentClear(f)(context.Background(), json.RawMessage(`{"project_id":"p1"}`)); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, err := AgentUndo(f)(context.Background(), json.RawMessage(`{"batch_id":"b1"}`)); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if f.cancelled != "run-3" || f.cleared != "p1" || f.undone != "b1" {
		t.Errorf("controller saw cancel=%q clear=%q undo=%q", f.cancelled, f.cleared, f.undone)
	}
}

func TestAgentHistory_returnsAJSONArray(t *testing.T) {
	out, err := AgentHistory(&fakeAgent{})(context.Background(), json.RawMessage(`{"project_id":"p1"}`))
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if string(out) != `[]` {
		t.Errorf("out = %s", out)
	}
}
