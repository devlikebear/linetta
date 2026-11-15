package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/mcphost"
	"github.com/devlikebear/tars/pkg/llm"
)

type citeStubOut struct {
	Citations []mcphost.CitedSource `json:"citations"`
}

var stubSource = mcphost.CitedSource{
	N: 1, NodeID: "n12", Breadcrumb: "1부 / 12화 / 씬 3", SceneTitle: "비 오는 음악실",
	ParagraphFrom: 4, ParagraphTo: 5, Quote: "검을 건네받은 손이 떨렸다",
}

// citeStubTools adds, beside stubTools' echo, a linetta_cite_sources that
// returns one source (or fails on "boom"), and a tool by another name that
// returns the same shape.
func citeStubTools() RegisterTools {
	return func(s *mcp.Server, groups mcphost.ToolGroups) {
		stubTools(nil)(s, groups)
		cite := func(_ context.Context, _ *mcp.CallToolRequest, in echoIn) (*mcp.CallToolResult, citeStubOut, error) {
			out := citeStubOut{Citations: []mcphost.CitedSource{stubSource}}
			if in.Text == "boom" {
				return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "quote not found"}}}, out, nil
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "1 sources verified"}}}, out, nil
		}
		mcp.AddTool(s, &mcp.Tool{Name: citeToolName, Description: "cite"}, cite)
		mcp.AddTool(s, &mcp.Tool{Name: "lookalike", Description: "returns citations it never checked"}, cite)
	}
}

// A source card says the engine checked the quote, so only the tool that
// checks it can put one on a line, and only when the check passed.
func TestCall_readsCitationsOnlyOffASuccessfulCiteCall(t *testing.T) {
	s := newSession(t, citeStubTools())

	got := s.call(context.Background(), "run-1", citeToolName, `{"text":"ok"}`)
	if len(got.Citations) != 1 || got.Citations[0] != stubSource {
		t.Fatalf("Citations = %+v", got.Citations)
	}
	if got := s.call(context.Background(), "run-1", citeToolName, `{"text":"boom"}`); !got.IsError || got.Citations != nil {
		t.Fatalf("a refused citation call carried %+v", got)
	}
	if got := s.call(context.Background(), "run-1", "lookalike", `{"text":"ok"}`); got.Citations != nil {
		t.Fatalf("another tool's citations were read: %+v", got.Citations)
	}
}

// The panel draws the cards from agent.tool while the turn runs, and from the
// transcript row when the conversation is opened again; both carry them.
func TestRun_carriesCitationsToThePanelAndTheTranscript(t *testing.T) {
	rec := &recorder{}
	client := &scriptedClient{responses: []llm.ChatResponse{
		toolReply(citeToolName, `{"text":"ok"}`),
		textReply("12화에서 받았습니다 [1]."),
	}}
	st := openStoreForAgentTests(t)
	svc := New(Deps{
		Providers: fakeProviders{client: client},
		History:   companion.NewHistoryRepo(st.DB()),
		Scope:     fakeScope{titles: map[string]string{"p1": "제목"}},
		Register:  citeStubTools(),
		Notify:    rec.notify,
		Language:  func() string { return "ko" },
		Clock:     func() int64 { return 1700000000000 },
	})
	t.Cleanup(func() { _ = svc.Close() })

	runAndWait(t, svc, rec, "주인공이 처음 검을 받은 게 몇 화였지?")

	live := finalToolPayload(t, rec, citeToolName)
	if live.State != "done" || len(live.Citations) != 1 || live.Citations[0] != stubSource {
		t.Fatalf("agent.tool = %+v", live)
	}
	msgs, err := svc.History(context.Background(), "p1", 100)
	if err != nil {
		t.Fatal(err)
	}
	var restored *toolEvent
	for _, m := range msgs {
		if m.Role != "tool" {
			continue
		}
		var ev toolEvent
		if err := json.Unmarshal([]byte(m.Content), &ev); err != nil {
			t.Fatalf("tool row %q: %v", m.Content, err)
		}
		if ev.Name == citeToolName {
			restored = &ev
		}
	}
	if restored == nil || len(restored.Citations) != 1 || restored.Citations[0] != stubSource {
		t.Fatalf("transcript row = %+v", restored)
	}
}
