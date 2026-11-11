package agent

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/mcphost"
	"github.com/devlikebear/tars/pkg/llm"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The settlement pass looks for an event before it records time since it, and
// reports time that runs wrong under its own prefix.
func TestSettlementPromptCoversElapsedTime(t *testing.T) {
	for _, want := range []string{"경과", "나이", "kind=timeline", "about its id", "value_num the days since it",
		"only when it is not there", "jump_days", "scenes_between", "시간 모순:", "Record one 경과 an event a scene"} {
		if !strings.Contains(settlementSystemPrompt, want) {
			t.Errorf("settlement prompt lacks %q", want)
		}
	}
	if settlementCallsPerScene != 14 {
		t.Errorf("settlementCallsPerScene = %d, want 14 until passes are measured", settlementCallsPerScene)
	}
}

func TestEpisodeReviewPromptHoldsTheProseAgainstStoryTime(t *testing.T) {
	for _, want := range []string{"## 작중 시간", "never shrinks", "as timeline"} {
		if !strings.Contains(episodeReviewSystemPrompt, want) {
			t.Errorf("review prompt lacks %q", want)
		}
	}
}

// toolLog keeps every call's arguments, by tool.
type toolLog struct {
	mu    sync.Mutex
	calls map[string][]string
}

func (l *toolLog) add(name, args string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls[name] = append(l.calls[name], args)
}

func (l *toolLog) of(name string) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.calls[name]...)
}

// A pass that records an elapsed time against an event and reports the time
// running wrong: both go out as tool calls the pass is allowed to make.
func TestSettlement_recordsElapsedTimeAndReportsATimeContradiction(t *testing.T) {
	log := &toolLog{calls: map[string][]string{}}
	register := func(s *mcp.Server, _ mcphost.ToolGroups) {
		for _, name := range []string{storyContextTool, "linetta_read_scene", "linetta_list_ledger", "linetta_edit_ledger", "linetta_report_continuity", "linetta_write_scene"} {
			mcp.AddTool(s, &mcp.Tool{Name: name, Description: name},
				func(_ context.Context, req *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, struct{}, error) {
					log.add(req.Params.Name, string(req.Params.Arguments))
					text := "ok"
					if req.Params.Name == "linetta_edit_ledger" {
						text = `{"entry":{"id":"e2"},"previous":{"entry_id":"e1","value_num":90,"delta":1005,"jump_days":1005,"scenes_between":4}}`
					}
					return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, struct{}{}, nil
				})
		}
	}
	elapsed := `{"project_id":"p1","kind":"timeline","subject":"등대지기 실종","attribute":"경과","body":"삼 년","about":"ev1","value_num":1095,"planted_node_id":"n1"}`
	report := `{"node_id":"n1","observations":[{"category":"timeline","summary":"시간 모순: 석 달 뒤 열이틀 만에 삼 년","quote":"삼 년이 흘렀다"}]}`
	c := &settleClient{turn: writingTurn("n1"), mode: passDone, script: []llm.ChatResponse{
		{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{
			{ID: "l", Name: "linetta_list_ledger", Arguments: `{"project_id":"p1","kinds":["timeline"]}`},
			{ID: "e", Name: "linetta_edit_ledger", Arguments: elapsed},
		}}},
		{Message: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "r", Name: "linetta_report_continuity", Arguments: report}}}},
		textReply("정산했습니다"),
	}}
	ledger := &fakeSettlements{versions: map[string]int{"n1": 1}}
	svc := newSettleServiceWith(t, fakeProviders{client: c}, register, nil, &recorder{}, true, ledger, &gateLog{})
	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "써 줘"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the settlement record", func() bool { return len(ledger.recorded()) == 1 })
	if got := log.of("linetta_edit_ledger"); len(got) != 1 || !strings.Contains(got[0], `"attribute":"경과"`) || !strings.Contains(got[0], `"about":"ev1"`) {
		t.Fatalf("edits = %q", got)
	}
	if got := log.of("linetta_report_continuity"); len(got) != 1 || !strings.Contains(got[0], "시간 모순:") {
		t.Fatalf("reports = %q", got)
	}
	if got := log.of("linetta_list_ledger"); len(got) != 1 {
		t.Fatalf("the event was not looked up first: %q", got)
	}
}
