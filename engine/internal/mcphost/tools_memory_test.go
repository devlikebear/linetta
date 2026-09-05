//go:build !mobile

package mcphost

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/agentmemory"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/store"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func newMemoryDeps(t *testing.T) (context.Context, ToolDeps, string) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	p, err := project.NewRepo(st).Create(ctx, 42, project.NewInput{
		Title: "작품", Genres: []string{"fantasy"}, LengthTarget: "novel", DefaultPOV: "first",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	d := ToolDeps{
		Memory:   agentmemory.NewRepo(st.DB()),
		Activity: NewActivityRepo(st.DB()),
		Projects: project.NewRepo(st),
		Source:   SourceAgent,
		Clock:    func() int64 { return 42 },
	}
	return ctx, d, p.ID
}

func TestEditMemoryAddsAndReturnsTheBody(t *testing.T) {
	ctx, d, projectID := newMemoryDeps(t)
	res, out, err := d.editMemory(ctx, nil, editMemoryInput{
		Scope: "work_notes", Action: "add", Text: "민준은 3화부터 존댓말", ProjectID: projectID,
	})
	if err != nil || (res != nil && res.IsError) {
		t.Fatalf("editMemory: err=%v res=%+v", err, res)
	}
	if out.Body != "민준은 3화부터 존댓말" {
		t.Errorf("Body = %q", out.Body)
	}
	if out.CharsBudget != 2200 {
		t.Errorf("CharsBudget = %d, want 2200 so the agent can manage its own space", out.CharsBudget)
	}
	if out.CharsUsed == 0 {
		t.Error("CharsUsed must be filled")
	}
}

func TestEditMemoryPersists(t *testing.T) {
	ctx, d, projectID := newMemoryDeps(t)
	if _, _, err := d.editMemory(ctx, nil, editMemoryInput{
		Scope: "work_notes", Action: "add", Text: "하나", ProjectID: projectID}); err != nil {
		t.Fatalf("first add: %v", err)
	}
	_, out, err := d.editMemory(ctx, nil, editMemoryInput{
		Scope: "work_notes", Action: "add", Text: "둘", ProjectID: projectID})
	if err != nil {
		t.Fatalf("second add: %v", err)
	}
	if out.Body != "하나\n둘" {
		t.Fatalf("Body = %q — the second call must have loaded what the first wrote", out.Body)
	}
}

// Recoverable failures come back as a tool RESULT with a nil Go error, so the
// model reads the message and retries. A transport error would end the turn.
func TestEditMemoryFailuresAreToolErrorsNotTransportErrors(t *testing.T) {
	ctx, d, projectID := newMemoryDeps(t)
	cases := map[string]editMemoryInput{
		"unknown scope":  {Scope: "nonsense", Action: "add", Text: "x"},
		"unknown action": {Scope: "work_notes", Action: "rewrite", Text: "x", ProjectID: projectID},
		"no match":       {Scope: "work_notes", Action: "remove", Find: "없음", ProjectID: projectID},
		"invisible char": {Scope: "work_notes", Action: "add", Text: "안녕​", ProjectID: projectID},
		"notes, no work": {Scope: "work_notes", Action: "add", Text: "x"},
		"profile + work": {Scope: "writer_profile", Action: "add", Text: "x", ProjectID: projectID},
		"unknown work":   {Scope: "work_notes", Action: "add", Text: "x", ProjectID: "no-such-work"},
	}
	for name, in := range cases {
		res, _, err := d.editMemory(ctx, nil, in)
		if err != nil {
			t.Errorf("%s: got a transport error %v; want a tool error result", name, err)
		}
		if res == nil || !res.IsError {
			t.Errorf("%s: want an error result, got %+v", name, res)
		}
	}
}

func TestEditMemoryOverBudgetSaysWhatToDo(t *testing.T) {
	ctx, d, _ := newMemoryDeps(t)
	if _, _, err := d.editMemory(ctx, nil, editMemoryInput{
		Scope: "writer_profile", Action: "add", Text: strings.Repeat("가", 1400)}); err != nil {
		t.Fatalf("filling the profile: %v", err)
	}
	res, _, err := d.editMemory(ctx, nil, editMemoryInput{
		Scope: "writer_profile", Action: "add", Text: "한 줄 더"})
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatal("want an error result")
	}
	if !strings.Contains(firstText(res), "1400") {
		t.Errorf("the message must name the budget; got %q", firstText(res))
	}
}

func TestEditMemoryNotifiesWithItsSource(t *testing.T) {
	ctx, d, projectID := newMemoryDeps(t)
	var gotMethod string
	var gotParams any
	d.Notify = func(method string, params any) { gotMethod, gotParams = method, params }
	if _, _, err := d.editMemory(ctx, nil, editMemoryInput{
		Scope: "work_notes", Action: "add", Text: "x", ProjectID: projectID}); err != nil {
		t.Fatalf("editMemory: %v", err)
	}
	if gotMethod != "memory.changed" {
		t.Fatalf("method = %q, want memory.changed — Settings would show a stale textarea otherwise", gotMethod)
	}
	p, ok := gotParams.(memoryChangedPayload)
	if !ok {
		t.Fatalf("payload type %T", gotParams)
	}
	if p.Source != SourceAgent || p.Scope != "work_notes" || p.ProjectID != projectID {
		t.Errorf("payload = %+v", p)
	}
}

// A failed edit must not tell the UI something changed.
func TestEditMemoryDoesNotNotifyOnFailure(t *testing.T) {
	ctx, d, _ := newMemoryDeps(t)
	notified := false
	d.Notify = func(string, any) { notified = true }
	if _, _, err := d.editMemory(ctx, nil, editMemoryInput{
		Scope: "nonsense", Action: "add", Text: "x"}); err != nil {
		t.Fatalf("editMemory: %v", err)
	}
	if notified {
		t.Error("a refused edit must not emit memory.changed")
	}
}

func TestEditMemoryScopesTheActivityEntry(t *testing.T) {
	ctx, d, projectID := newMemoryDeps(t)
	h := record(d, "linetta_edit_memory", d.editMemory)
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "linetta_edit_memory"}}
	if _, _, err := h(ctx, req, editMemoryInput{
		Scope: "work_notes", Action: "add", Text: "x", ProjectID: projectID}); err != nil {
		t.Fatalf("handler: %v", err)
	}
	rows, err := d.Activity.List(ctx, 10)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 activity row, got %d", len(rows))
	}
	if rows[0].ProjectID != projectID || rows[0].Tool != "linetta_edit_memory" || !rows[0].OK {
		t.Errorf("row = %+v", rows[0])
	}
}

func TestEditMemoryIsRegisteredAsAWriteTool(t *testing.T) {
	found := false
	for _, n := range WriteToolNames {
		if n == "linetta_edit_memory" {
			found = true
		}
	}
	if !found {
		t.Fatal("linetta_edit_memory must be in WriteToolNames — a read_only server must not hand out a way to write the writer's memory")
	}
	for _, n := range ReadToolNames {
		if n == "linetta_edit_memory" {
			t.Fatal("it must not also be a read tool")
		}
	}
}
