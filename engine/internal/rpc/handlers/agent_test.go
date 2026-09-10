package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/rpc"
)

type fakeAgent struct {
	runID      string
	runErr     error
	sawProject string
	sawPrompt  string
	sawNode    string

	cancelErr error
	cancelled string

	historyErr        error
	sawHistoryProject string
	sawLimit          int

	clearErr error
	cleared  string

	undoErr    error
	undone     string
	undoneSnap string
}

func (f *fakeAgent) Run(_ context.Context, projectID, nodeID, prompt string) (string, error) {
	f.sawProject, f.sawPrompt, f.sawNode = projectID, prompt, nodeID
	return f.runID, f.runErr
}
func (f *fakeAgent) Cancel(_ context.Context, runID string) error {
	f.cancelled = runID
	return f.cancelErr
}
func (f *fakeAgent) History(_ context.Context, projectID string, limit int) (json.RawMessage, error) {
	f.sawHistoryProject, f.sawLimit = projectID, limit
	if f.historyErr != nil {
		return nil, f.historyErr
	}
	return json.RawMessage(`[]`), nil
}
func (f *fakeAgent) Clear(_ context.Context, projectID string) error {
	f.cleared = projectID
	return f.clearErr
}
func (f *fakeAgent) Undo(_ context.Context, batchID, snapshotID string) error {
	f.undone, f.undoneSnap = batchID, snapshotID
	return f.undoErr
}

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
	if f.sawProject != "p1" || f.sawPrompt != "안녕" || f.sawNode != "n1" {
		t.Errorf("controller saw project=%q prompt=%q node=%q", f.sawProject, f.sawPrompt, f.sawNode)
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

// A handler that dropped the ctrl error and returned nil, or forwarded a raw
// (non-MethodError) err, would still satisfy the type checker — this pins
// the MethodErrorFrom translation down for the three handlers whose fake
// never errors in the happy-path test above.
func TestAgentCancelClearUndo_translateControllerErrors(t *testing.T) {
	sentinel := &rpc.ReasonError{Reason: rpc.ReasonAgentUndoUnavailable, Err: errors.New("gone")}
	wantData := `{"reason":"agent_undo_unavailable"}`

	assertTranslated := func(t *testing.T, name string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s: want an error", name)
		}
		var me *rpc.MethodError
		if !errors.As(err, &me) {
			t.Fatalf("%s: err = %T, want *rpc.MethodError", name, err)
		}
		if string(me.Data) != wantData {
			t.Errorf("%s: data = %s, want %s", name, me.Data, wantData)
		}
	}

	_, err := AgentCancel(&fakeAgent{cancelErr: sentinel})(context.Background(), json.RawMessage(`{"run_id":"r1"}`))
	assertTranslated(t, "AgentCancel", err)

	_, err = AgentClear(&fakeAgent{clearErr: sentinel})(context.Background(), json.RawMessage(`{"project_id":"p1"}`))
	assertTranslated(t, "AgentClear", err)

	_, err = AgentUndo(&fakeAgent{undoErr: sentinel})(context.Background(), json.RawMessage(`{"batch_id":"b1"}`))
	assertTranslated(t, "AgentUndo", err)
}

func TestAgentUndo_requiresABatchID(t *testing.T) {
	f := &fakeAgent{}
	if _, err := AgentUndo(f)(context.Background(), json.RawMessage(`{"batch_id":"  "}`)); err == nil {
		t.Fatal("an empty batch_id must be refused before the controller is called")
	}
	if f.undone != "" {
		t.Errorf("controller was reached with an empty batch_id: undone = %q", f.undone)
	}
}

// A scene write's undo goes through snapshot_id instead of batch_id (#112);
// AgentUndo must accept it and hand it to the controller unchanged.
func TestAgentUndo_acceptsASnapshotID(t *testing.T) {
	f := &fakeAgent{}
	if _, err := AgentUndo(f)(context.Background(), json.RawMessage(`{"snapshot_id":"s1"}`)); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if f.undoneSnap != "s1" || f.undone != "" {
		t.Errorf("controller saw batch=%q snapshot=%q, want batch empty and snapshot s1", f.undone, f.undoneSnap)
	}
}

// Exactly one of batch_id, snapshot_id is valid: neither and both are both
// refused before the controller is reached.
func TestAgentUndo_requiresExactlyOneOfBatchOrSnapshot(t *testing.T) {
	f := &fakeAgent{}
	if _, err := AgentUndo(f)(context.Background(), json.RawMessage(`{}`)); err == nil {
		t.Fatal("neither batch_id nor snapshot_id must be refused")
	}
	if _, err := AgentUndo(f)(context.Background(), json.RawMessage(`{"batch_id":"b1","snapshot_id":"s1"}`)); err == nil {
		t.Fatal("both batch_id and snapshot_id must be refused")
	}
	if f.undone != "" || f.undoneSnap != "" {
		t.Errorf("controller was reached with invalid params: batch=%q snapshot=%q", f.undone, f.undoneSnap)
	}
}

func TestAgentHistory_returnsAJSONArray(t *testing.T) {
	f := &fakeAgent{}
	out, err := AgentHistory(f)(context.Background(), json.RawMessage(`{"project_id":"p1","limit":5}`))
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if string(out) != `[]` {
		t.Errorf("out = %s", out)
	}
	if f.sawHistoryProject != "p1" || f.sawLimit != 5 {
		t.Errorf("controller saw project=%q limit=%d, want p1/5", f.sawHistoryProject, f.sawLimit)
	}
}

func TestAgentHistory_translatesControllerErrors(t *testing.T) {
	f := &fakeAgent{historyErr: &rpc.ReasonError{Reason: rpc.ReasonProjectNotFound, Err: errors.New("gone")}}
	_, err := AgentHistory(f)(context.Background(), json.RawMessage(`{"project_id":"nope"}`))
	if err == nil {
		t.Fatal("want an error")
	}
	var me *rpc.MethodError
	if !errors.As(err, &me) || string(me.Data) != `{"reason":"project_not_found"}` {
		t.Errorf("err = %v, want a MethodError carrying project_not_found", err)
	}
}
