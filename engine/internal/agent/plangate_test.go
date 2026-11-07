package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/mcphost"
	"github.com/devlikebear/tars/pkg/llm"
)

// fakePlans is a work's scenes as the gate sees them.
type fakePlans struct {
	mu     sync.Mutex
	scenes map[string]*fakeScene
}

type fakeScene struct{ empty, planned bool }

func (f *fakePlans) PlanState(_ context.Context, id string) (bool, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.scenes[id]
	if !ok {
		return false, false, nil
	}
	return s.empty, s.planned, nil
}

func (f *fakePlans) plan(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := f.scenes[id]; ok {
		s.planned = true
	}
}

// planTools installs the brief, a scene write and — when withSummary — the
// summary tool, whose as_plan call gives the scene its plan.
func planTools(plans *fakePlans, withSummary bool, writes *atomic.Int32) RegisterTools {
	return func(s *mcp.Server, _ mcphost.ToolGroups) {
		mcp.AddTool(s, &mcp.Tool{Name: storyContextTool, Description: "the brief"},
			func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, struct{}, error) {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "brief"}}}, struct{}{}, nil
			})
		type sceneArgs struct {
			NodeID  string   `json:"node_id,omitempty"`
			NodeIDs []string `json:"node_ids,omitempty"`
			AsPlan  bool     `json:"as_plan,omitempty"`
		}
		for _, name := range []string{"linetta_write_scene", "linetta_revise_scene"} {
			mcp.AddTool(s, &mcp.Tool{Name: name, Description: "prose"},
				func(context.Context, *mcp.CallToolRequest, sceneArgs) (*mcp.CallToolResult, struct{}, error) {
					writes.Add(1)
					return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "written"}}}, struct{}{}, nil
				})
		}
		if withSummary {
			mcp.AddTool(s, &mcp.Tool{Name: planTool, Description: "summary"},
				func(_ context.Context, _ *mcp.CallToolRequest, in sceneArgs) (*mcp.CallToolResult, struct{}, error) {
					if in.AsPlan {
						plans.plan(in.NodeID)
					}
					return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "planned"}}}, struct{}{}, nil
				})
		}
	}
}

type gateLog struct {
	mu   sync.Mutex
	rows []GateEvent
}

func (g *gateLog) record(_ context.Context, e GateEvent) {
	g.mu.Lock()
	g.rows = append(g.rows, e)
	g.mu.Unlock()
}

func (g *gateLog) all() []GateEvent {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]GateEvent(nil), g.rows...)
}

func newPlanService(t *testing.T, providers ProviderSource, rec *recorder, plans *fakePlans, register RegisterTools, log *gateLog) *Service {
	t.Helper()
	st := openStoreForAgentTests(t)
	svc := New(Deps{
		Providers:  providers,
		History:    companion.NewHistoryRepo(st.DB()),
		Scope:      fakeScope{titles: map[string]string{"p1": "제목"}},
		Register:   register,
		Notify:     rec.notify,
		Language:   func() string { return "ko" },
		Clock:      func() int64 { return 1700000000000 },
		Plans:      plans,
		RecordGate: log.record,
	})
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

func runPlanTurn(t *testing.T, plans *fakePlans, withSummary bool, replies ...llm.ChatResponse) (writes int32, results []string, log *gateLog) {
	t.Helper()
	rec := &recorder{}
	var w atomic.Int32
	log = &gateLog{}
	c := &scriptedClient{responses: append([]llm.ChatResponse{toolReply(storyContextTool, `{}`)}, replies...)}
	svc := newPlanService(t, fakeProviders{client: c}, rec, plans, planTools(plans, withSummary, &w), log)
	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "3화 써 줘"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "agent.done", func() bool { return rec.has("agent.done") })
	return w.Load(), toolMessages(c.messages()), log
}

// An empty episode with no plan is turned back to write one; once it has, the
// same write goes through in the same turn.
func TestRun_aNewEpisodeWaitsForItsPlan(t *testing.T) {
	plans := &fakePlans{scenes: map[string]*fakeScene{"n3": {empty: true}}}
	writes, results, log := runPlanTurn(t, plans, true,
		toolReply("linetta_write_scene", `{"node_id":"n3"}`),
		toolReply(planTool, `{"node_id":"n3","as_plan":true}`),
		toolReply("linetta_write_scene", `{"node_id":"n3"}`),
		textReply("썼어요"),
	)
	if writes != 1 {
		t.Errorf("write ran %d times, want once — after the plan", writes)
	}
	if len(results) != 4 || !strings.HasPrefix(results[1], "Not written.") || !strings.Contains(results[1], "as_plan=true") {
		t.Fatalf("tool results = %q, want the second to be the plan refusal", results)
	}
	if results[3] != "written" {
		t.Errorf("the write after the plan = %q", results[3])
	}
	rows := log.all()
	if len(rows) != 1 || rows[0].Kind != GatePlan || rows[0].NodeID != "n3" || rows[0].OK {
		t.Errorf("activity rows = %+v, want one plan_gate refusal for n3", rows)
	}
}

// The writer's intent for the scene, or a beat of the chosen branch, is a plan
// too: the lookup says so, and the write is not held.
func TestRun_aSceneIntentIsEnoughOfAPlan(t *testing.T) {
	plans := &fakePlans{scenes: map[string]*fakeScene{"n3": {empty: true, planned: true}}}
	writes, _, log := runPlanTurn(t, plans, true,
		toolReply("linetta_write_scene", `{"node_id":"n3"}`),
		textReply("썼어요"),
	)
	if writes != 1 {
		t.Errorf("write ran %d times, want once", writes)
	}
	if rows := log.all(); len(rows) != 0 {
		t.Errorf("activity rows = %+v, want none", rows)
	}
}

// Revising an episode that already has prose is never held for a plan.
func TestRun_aSceneWithProseIsNotHeldForAPlan(t *testing.T) {
	plans := &fakePlans{scenes: map[string]*fakeScene{"n1": {empty: false}}}
	writes, _, _ := runPlanTurn(t, plans, true,
		toolReply("linetta_write_scene", `{"node_id":"n1"}`),
		toolReply("linetta_revise_scene", `{"node_ids":["n1"]}`),
		toolReply("linetta_revise_scene", `{}`),
		textReply("고쳤어요"),
	)
	if writes != 3 {
		t.Errorf("writes ran %d times, want 3", writes)
	}
}

// Without the summary tool the turn could never write the plan, so the gate
// is not armed.
func TestRun_withoutTheSummaryToolNewEpisodesAreNotHeld(t *testing.T) {
	plans := &fakePlans{scenes: map[string]*fakeScene{"n3": {empty: true}}}
	writes, _, _ := runPlanTurn(t, plans, false,
		toolReply("linetta_write_scene", `{"node_id":"n3"}`),
		textReply("썼어요"),
	)
	if writes != 1 {
		t.Errorf("write ran %d times, want once", writes)
	}
}

func TestPlanGate_revisingEmptyScenesByIDIsHeld(t *testing.T) {
	plans := &fakePlans{scenes: map[string]*fakeScene{"n1": {}, "n3": {empty: true}}}
	g := newPlanGate(plans, []llm.ToolSchema{{Function: llm.ToolFunctionSchema{Name: planTool}}})
	if r, id := g.refusal(context.Background(), "linetta_revise_scene", `{"node_ids":["n1","n3"]}`); r == "" || id != "n3" {
		t.Errorf("revise naming an empty scene: refusal %q for %q", r, id)
	}
	for _, name := range []string{"linetta_read_scene", planTool, "linetta_apply_story_ops"} {
		if r, _ := g.refusal(context.Background(), name, `{"node_id":"n3"}`); r != "" {
			t.Errorf("%s was held: %q", name, r)
		}
	}
	if newPlanGate(nil, []llm.ToolSchema{{Function: llm.ToolFunctionSchema{Name: planTool}}}) != nil {
		t.Error("a gate with no lookup was armed")
	}
}

// claudeCodeCalls stands in for the CLI, calling the named tools over the
// bridge in order.
type claudeCodeCalls struct {
	calls [][2]string // name, JSON arguments
	seen  []*mcp.CallToolResult
}

func (f *claudeCodeCalls) Ask(context.Context, string) (string, error) { return "", nil }

func (f *claudeCodeCalls) Chat(ctx context.Context, _ []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	srv := opts.ClaudeCodeMCPServers[0]
	client := mcp.NewClient(&mcp.Implementation{Name: "fake-claude-code", Version: "0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   srv.URL,
		HTTPClient: &http.Client{Transport: headerTransport{headers: srv.Headers}},
	}, nil)
	if err != nil {
		return llm.ChatResponse{}, err
	}
	defer session.Close()
	for _, c := range f.calls {
		var args map[string]any
		if err := json.Unmarshal([]byte(c[1]), &args); err != nil {
			return llm.ChatResponse{}, err
		}
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: c[0], Arguments: args})
		if err != nil {
			return llm.ChatResponse{}, err
		}
		f.seen = append(f.seen, res)
	}
	opts.OnDelta("썼어요")
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "썼어요"}}, nil
}

// The CLI's calls come over the bridge, and pass the same gate.
func TestRunCLI_aNewEpisodeWaitsForItsPlan(t *testing.T) {
	plans := &fakePlans{scenes: map[string]*fakeScene{"n3": {empty: true}}}
	rec := &recorder{}
	var writes atomic.Int32
	cc := &claudeCodeCalls{calls: [][2]string{
		{storyContextTool, `{}`},
		{"linetta_write_scene", `{"node_id":"n3"}`},
		{planTool, `{"node_id":"n3","as_plan":true}`},
		{"linetta_write_scene", `{"node_id":"n3"}`},
	}}
	svc := newPlanService(t, cliProviders{client: cc}, rec, plans, planTools(plans, true, &writes), &gateLog{})
	runID, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "3화 써 줘"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "the run to end", func() bool { return rec.hasTerminalFor(runID) })
	if len(cc.seen) != 4 {
		t.Fatalf("calls answered = %d", len(cc.seen))
	}
	if !cc.seen[1].IsError || !strings.HasPrefix(textOf(cc.seen[1]), "Not written.") {
		t.Errorf("first write over the bridge = %q, want the plan refusal", textOf(cc.seen[1]))
	}
	if cc.seen[3].IsError || writes.Load() != 1 {
		t.Errorf("write after the plan = %q (writes %d)", textOf(cc.seen[3]), writes.Load())
	}
}
