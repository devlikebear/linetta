package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/agentmemory"
)

// memStub stands in for the repo so this test needs no database. The handler
// takes an interface for the same reason MCPController does: handlers must
// stay linkable on every build tag.
type memStub struct {
	bodies map[string]string
	saveFn func(scope agentmemory.Scope, projectID, body string) error
}

func (m *memStub) Load(_ context.Context, scope agentmemory.Scope, projectID string) (agentmemory.Document, error) {
	body := m.bodies[string(scope)+"|"+projectID]
	return agentmemory.Document{
		Scope: scope, ProjectID: projectID, Body: body,
		CharsUsed: len([]rune(body)), CharsBudget: scope.Budget(),
	}, nil
}

func (m *memStub) Save(_ context.Context, scope agentmemory.Scope, projectID, body string, now int64) (agentmemory.Document, error) {
	if m.saveFn != nil {
		if err := m.saveFn(scope, projectID, body); err != nil {
			return agentmemory.Document{}, err
		}
	}
	if m.bodies == nil {
		m.bodies = map[string]string{}
	}
	m.bodies[string(scope)+"|"+projectID] = body
	return agentmemory.Document{
		Scope: scope, ProjectID: projectID, Body: body,
		CharsUsed: len([]rune(body)), CharsBudget: scope.Budget(), UpdatedAt: now,
	}, nil
}

func TestGetMemoryReturnsBothDocuments(t *testing.T) {
	store := &memStub{bodies: map[string]string{
		"writer_profile|": "프로필",
		"work_notes|p1":   "노트",
	}}
	raw, err := GetMemory(store)(context.Background(), json.RawMessage(`{"project_id":"p1"}`))
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	var got struct {
		WriterProfile agentmemory.Document `json:"writer_profile"`
		WorkNotes     agentmemory.Document `json:"work_notes"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.WriterProfile.Body != "프로필" || got.WorkNotes.Body != "노트" {
		t.Errorf("got %+v", got)
	}
	if got.WorkNotes.CharsBudget != 2200 {
		t.Errorf("CharsBudget = %d", got.WorkNotes.CharsBudget)
	}
}

func TestGetMemoryWithNoWorkStillReturnsTheProfile(t *testing.T) {
	store := &memStub{bodies: map[string]string{"writer_profile|": "프로필"}}
	raw, err := GetMemory(store)(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if !strings.Contains(string(raw), "프로필") {
		t.Errorf("got %s", raw)
	}
}

func TestSetMemoryRejectsAnUnknownScope(t *testing.T) {
	_, err := SetMemory(&memStub{}, func() int64 { return 1 }, nil)(
		context.Background(), json.RawMessage(`{"scope":"nonsense","body":"x"}`))
	if err == nil {
		t.Fatal("want an error")
	}
}

// A writer pasting text with a zero-width space, or overrunning the budget,
// must get a usable message — not an opaque internal error.
func TestSetMemorySurfacesARefusalUsefully(t *testing.T) {
	store := &memStub{saveFn: func(agentmemory.Scope, string, string) error { return agentmemory.ErrOverBudget }}
	_, err := SetMemory(store, func() int64 { return 1 }, nil)(
		context.Background(), json.RawMessage(`{"scope":"writer_profile","body":"x"}`))
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "budget") {
		t.Errorf("the message must say what went wrong; got %v", err)
	}
}

func TestSetMemoryNotifies(t *testing.T) {
	var method string
	notify := func(m string, _ any) { method = m }
	if _, err := SetMemory(&memStub{}, func() int64 { return 1 }, notify)(
		context.Background(), json.RawMessage(`{"scope":"writer_profile","body":"x"}`)); err != nil {
		t.Fatalf("SetMemory: %v", err)
	}
	if method != "memory.changed" {
		t.Errorf("method = %q — another window would show a stale textarea", method)
	}
}
