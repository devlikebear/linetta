//go:build !mobile

package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/tars/pkg/llm"
)

const askArgs = `{"questions":[
  {"header":"삼도약의 연도","question":"누가 천마를 쓰러뜨렸나요?","options":[
    {"label":"백가 선대","description":"묵로는 두 번째 주인","recommended":true},
    {"label":"묵로 본인"}]},
  {"header":"1·2화 수정","question":"어디를 손볼까요?","multi_select":true,"options":[
    {"label":"1화 복선"},{"label":"2화 표현"}]}]}`

func TestParseAskCard_refusesWhatTheCardCannotDraw(t *testing.T) {
	option := `{"label":"가"},{"label":"나"}`
	question := func(options string) string {
		return `{"header":"결정","question":"무엇을?","options":[` + options + `]}`
	}
	cases := map[string]string{
		"not json":      `{`,
		"no questions":  `{"questions":[]}`,
		"seven":         `{"questions":[` + strings.TrimSuffix(strings.Repeat(question(option)+",", 7), ",") + `]}`,
		"no header":     `{"questions":[{"question":"무엇을?","options":[` + option + `]}]}`,
		"long header":   `{"questions":[{"header":"` + strings.Repeat("가", 41) + `","question":"무엇을?","options":[` + option + `]}]}`,
		"one option":    `{"questions":[` + question(`{"label":"가"}`) + `]}`,
		"blank label":   `{"questions":[` + question(`{"label":"가"},{"label":"  "}`) + `]}`,
		"same label":    `{"questions":[` + question(`{"label":"가"},{"label":" 가 "}`) + `]}`,
		"seven options": `{"questions":[` + question(`{"label":"1"},{"label":"2"},{"label":"3"},{"label":"4"},{"label":"5"},{"label":"6"},{"label":"7"}`) + `]}`,
	}
	for name, args := range cases {
		if _, err := parseAskCard(args); err == nil {
			t.Errorf("%s: parseAskCard accepted %s", name, args)
		}
	}
	if _, err := parseAskCard(askArgs); err != nil {
		t.Fatalf("a good card was refused: %v", err)
	}
}

func finalToolPayload(t *testing.T, rec *recorder, name string) toolPayload {
	t.Helper()
	rec.mu.Lock()
	defer rec.mu.Unlock()
	for _, e := range rec.seen {
		if p, ok := e.Params.(toolPayload); ok && p.Name == name && p.State != "started" {
			return p
		}
	}
	t.Fatalf("no resolved agent.tool for %s", name)
	return toolPayload{}
}

func runAndWait(t *testing.T, svc *Service, rec *recorder, prompt string) string {
	t.Helper()
	runID, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: prompt})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "the run to end", func() bool { return rec.hasTerminalFor(runID) })
	return runID
}

// The card waits for the writer, so the turn must not spend another round-trip
// or run what the model queued behind a question it has not had answered.
func TestRun_aCardEndsTheTurnWithoutAnotherRoundTrip(t *testing.T) {
	rec := &recorder{}
	reply := toolReply(askWriterTool, askArgs)
	reply.Message.Content = "세 가지를 정해 주세요."
	reply.Message.ToolCalls = append(reply.Message.ToolCalls, llm.ToolCall{ID: "call-2", Name: "echo", Arguments: `{"text":"too soon"}`})
	client := &scriptedClient{responses: []llm.ChatResponse{reply}}
	svc := newService(t, client, rec)

	runAndWait(t, svc, rec, "3화를 고쳐 줘")

	if client.count() != 1 {
		t.Errorf("Chat calls = %d, want 1", client.count())
	}
	if !rec.has("agent.done") {
		t.Errorf("the turn did not end done: %v", rec.methods())
	}
	if rec.toolStarts() != 1 {
		t.Errorf("tool starts = %d, want only the card", rec.toolStarts())
	}
	got := finalToolPayload(t, rec, askWriterTool)
	if got.State != "done" || got.Ask == nil || len(got.Ask.Questions) != 2 {
		t.Fatalf("agent.tool = %+v", got)
	}
	if !got.Ask.Questions[0].Options[0].Recommended || !got.Ask.Questions[1].MultiSelect {
		t.Errorf("the card lost its marks: %+v", got.Ask)
	}
}

func TestRun_offersTheCardToTheModel(t *testing.T) {
	rec := &recorder{}
	client := &scriptedClient{responses: []llm.ChatResponse{textReply("좋아요")}}
	svc := newService(t, client, rec)
	runAndWait(t, svc, rec, "안녕")
	if !offers(client.options().Tools, askWriterTool) {
		t.Fatalf("%s is not among the turn's tools", askWriterTool)
	}
	var schema map[string]any
	if err := json.Unmarshal(askWriterSchema().Function.Parameters, &schema); err != nil {
		t.Fatalf("the tool's schema is not JSON: %v", err)
	}
}

// A bad card is the model's to fix, like any tool error.
func TestRun_aBadCardGoesBackToTheModel(t *testing.T) {
	rec := &recorder{}
	client := &scriptedClient{responses: []llm.ChatResponse{
		toolReply(askWriterTool, `{"questions":[]}`),
		textReply("다시 생각해 볼게요."),
	}}
	svc := newService(t, client, rec)

	runAndWait(t, svc, rec, "3화를 고쳐 줘")

	if client.count() != 2 {
		t.Errorf("Chat calls = %d, want 2", client.count())
	}
	if got := finalToolPayload(t, rec, askWriterTool); got.State != "error" || got.Ask != nil {
		t.Errorf("agent.tool = %+v", got)
	}
}

// The answers come back as the next message, and replay drops tool rows, so a
// card that is not replayed leaves the model reading answers to a question it
// has no record of asking.
func TestRun_theNextTurnSeesWhatItAsked(t *testing.T) {
	rec := &recorder{}
	client := &scriptedClient{responses: []llm.ChatResponse{toolReply(askWriterTool, askArgs), textReply("그렇게 할게요.")}}
	svc := newService(t, client, rec)

	runAndWait(t, svc, rec, "3화를 고쳐 줘")
	runAndWait(t, svc, rec, "삼도약의 연도: 백가 선대")

	var joined strings.Builder
	for _, m := range client.messages() {
		joined.WriteString(m.Role + ": " + m.Content + "\n")
	}
	for _, want := range []string{"assistant: [I asked the writer", "누가 천마를 쓰러뜨렸나요?", "백가 선대 (recommended): 묵로는 두 번째 주인", "(any number of options)"} {
		if !strings.Contains(joined.String(), want) {
			t.Errorf("the replay is missing %q:\n%s", want, joined.String())
		}
	}
}

func TestReplayableMessages_keepsCardsAndStillDropsOtherToolRows(t *testing.T) {
	card, err := parseAskCard(askArgs)
	if err != nil {
		t.Fatal(err)
	}
	withCard, err := json.Marshal(toolEvent{Name: askWriterTool, OK: true, Ask: card})
	if err != nil {
		t.Fatal(err)
	}
	got := replayableMessages([]companion.HistoryMessage{
		{ID: "1", Role: "tool", Content: `{"name":"linetta_read_scene","ok":true}`},
		{ID: "2", Role: "tool", Content: "not json"},
		{ID: "3", Role: "tool", Content: string(withCard)},
	})
	if len(got) != 1 || got[0].ID != "3" || got[0].Role != "assistant" {
		t.Fatalf("replayable = %+v", got)
	}
}

// askingClaudeCode asks once over the bridge, then tries to carry on.
type askingClaudeCode struct {
	card, after *mcp.CallToolResult
}

func (f *askingClaudeCode) Ask(context.Context, string) (string, error) { return "", nil }

func (f *askingClaudeCode) Chat(ctx context.Context, _ []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
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

	var args map[string]any
	if err := json.Unmarshal([]byte(askArgs), &args); err != nil {
		return llm.ChatResponse{}, err
	}
	if f.card, err = session.CallTool(ctx, &mcp.CallToolParams{Name: askWriterTool, Arguments: args}); err != nil {
		return llm.ChatResponse{}, err
	}
	if f.after, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{"text": "too soon"}}); err != nil {
		return llm.ChatResponse{}, err
	}
	opts.OnDelta("카드를 띄워 뒀습니다.")
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: "카드를 띄워 뒀습니다."}}, nil
}

// Claude Code cannot be stopped at the card the way the loop stops, so it is
// refused anything further, its words after the card go nowhere, and that
// leaving nothing to show is not a failed turn.
func TestRunCLI_aCardStopsTheToolsAndNeedsNoFurtherReply(t *testing.T) {
	rec := &recorder{}
	cc := &askingClaudeCode{}
	svc := newCLIService(t, cc, rec, 10)

	runAndWait(t, svc, rec, "3화를 고쳐 줘")

	if cc.card == nil || cc.card.IsError || !strings.Contains(textOf(cc.card), "End your turn") {
		t.Fatalf("the card's result = %+v", cc.card)
	}
	if cc.after == nil || !cc.after.IsError {
		t.Fatalf("a call after the card ran: %+v", cc.after)
	}
	if rec.toolStarts() != 1 {
		t.Errorf("tool starts = %d, want only the card", rec.toolStarts())
	}
	if !rec.has("agent.done") || rec.has("agent.error") {
		t.Errorf("the turn did not end done: %v", rec.methods())
	}
	if got := finalToolPayload(t, rec, askWriterTool); got.Ask == nil {
		t.Errorf("agent.tool carries no card: %+v", got)
	}
	if rec.has("agent.delta") {
		t.Error("text after the card reached the panel")
	}
	msgs, err := svc.History(context.Background(), "p1", 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range msgs {
		if m.Role == "assistant" {
			t.Errorf("text after the card reached the transcript: %q", m.Content)
		}
	}
}
