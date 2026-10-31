//go:build !mobile

package agent

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/mcphost"
	"github.com/devlikebear/tars/pkg/llm"
)

type echoIn struct {
	Text string `json:"text"`
}

type echoOut struct {
	UndoBatchID  string   `json:"undo_batch_id,omitempty"`
	SnapshotID   string   `json:"snapshot_id,omitempty"`
	ChangedNodes []string `json:"changed_nodes,omitempty"`
}

// stubTools installs one tool that echoes its input back, reports the run id
// it saw, and can be told to fail or to return an oversized body.
// It ignores the tool budget: the one tool it installs is in no group, so
// there is nothing for the switches to remove. That is exactly why the
// budget's own tests use the real mcphost registration instead — see
// budgetService in toolbudget_test.go.
func stubTools(seenRunID *string) RegisterTools {
	return func(s *mcp.Server, _ mcphost.ToolGroups) {
		mcp.AddTool(s, &mcp.Tool{Name: "echo", Description: "echo the text back"},
			func(_ context.Context, req *mcp.CallToolRequest, in echoIn) (*mcp.CallToolResult, echoOut, error) {
				if seenRunID != nil && req != nil && req.Params != nil {
					id, _ := req.Params.GetMeta()[mcphost.MetaRunID].(string)
					*seenRunID = id
				}
				switch in.Text {
				case "boom":
					return &mcp.CallToolResult{
						IsError: true,
						Content: []mcp.Content{&mcp.TextContent{Text: "version conflict; re-read and retry"}},
					}, echoOut{}, nil
				case "huge":
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: strings.Repeat("x", maxToolResultChars+500)}},
					}, echoOut{}, nil
				}
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: "echo: " + in.Text}},
				}, echoOut{UndoBatchID: "batch-1", SnapshotID: "snap-1", ChangedNodes: []string{"n1", "n2"}}, nil
			})
	}
}

func newSession(t *testing.T, register RegisterTools) *toolSession {
	t.Helper()
	s, err := connectTools(context.Background(), register, allToolGroups())
	if err != nil {
		t.Fatalf("connectTools: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// The model must see the tool descriptions the MCP layer already writes —
// keeping a second copy of them is the thing this design exists to avoid.
func TestSchemas_carryNameDescriptionAndParameters(t *testing.T) {
	s := newSession(t, stubTools(nil))
	got, err := s.schemas(context.Background())
	if err != nil {
		t.Fatalf("schemas: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d schemas, want 1", len(got))
	}
	if got[0].Type != "function" {
		t.Errorf("Type = %q, want function", got[0].Type)
	}
	if got[0].Function.Name != "echo" {
		t.Errorf("Name = %q", got[0].Function.Name)
	}
	if got[0].Function.Description != "echo the text back" {
		t.Errorf("Description = %q", got[0].Function.Description)
	}
	// The SDK hands the client a map[string]any; the model wants JSON.
	if !strings.Contains(string(got[0].Function.Parameters), `"text"`) {
		t.Errorf("Parameters = %s, want the input schema", got[0].Function.Parameters)
	}
}

func TestCall_returnsTextAndTheWriteMetadata(t *testing.T) {
	s := newSession(t, stubTools(nil))
	got := s.call(context.Background(), "run-1", "echo", `{"text":"hi"}`)
	if got.IsError {
		t.Fatalf("unexpected error result: %+v", got)
	}
	if !strings.Contains(got.Text, "echo: hi") {
		t.Errorf("Text = %q", got.Text)
	}
	if got.BatchID != "batch-1" {
		t.Errorf("BatchID = %q, want batch-1", got.BatchID)
	}
	// A write tool never sets both in practice (#112: linetta_apply_story_ops
	// sets undo_batch_id, linetta_write_scene/linetta_revise_scene set
	// snapshot_id), but writeMetadata reads either independently, and this
	// stub sets both to prove neither field's presence depends on the other's
	// absence.
	if got.SnapshotID != "snap-1" {
		t.Errorf("SnapshotID = %q, want snap-1", got.SnapshotID)
	}
	if len(got.NodeIDs) != 2 || got.NodeIDs[0] != "n1" {
		t.Errorf("NodeIDs = %v", got.NodeIDs)
	}
}

// The run id is what ties a turn's writes together in the activity log.
func TestCall_stampsTheRunIDOnTheRequest(t *testing.T) {
	var seen string
	s := newSession(t, stubTools(&seen))
	s.call(context.Background(), "run-9", "echo", `{"text":"hi"}`)
	if seen != "run-9" {
		t.Errorf("server saw run id %q, want run-9", seen)
	}
}

// A tool error is the model's to recover from, not a transport failure: it
// comes back as a result the loop can hand straight to the model.
func TestCall_toolErrorIsAResultNotAFailure(t *testing.T) {
	s := newSession(t, stubTools(nil))
	got := s.call(context.Background(), "run-1", "echo", `{"text":"boom"}`)
	if !got.IsError {
		t.Fatal("want IsError")
	}
	if !strings.Contains(got.Text, "version conflict") {
		t.Errorf("Text = %q, want the tool's own message", got.Text)
	}
}

// linetta_read_scene legitimately returns long scenes, so the cap is generous
// — but a loop that keeps pasting a novel into the context has to hit a wall.
func TestCall_truncatesAnOversizedResult(t *testing.T) {
	s := newSession(t, stubTools(nil))
	got := s.call(context.Background(), "run-1", "echo", `{"text":"huge"}`)
	if !got.Truncated {
		t.Error("want Truncated")
	}
	if len([]rune(got.Text)) > maxToolResultChars+200 {
		t.Errorf("text is %d runes, want it capped near %d", len([]rune(got.Text)), maxToolResultChars)
	}
	if !strings.Contains(got.Text, "truncated") {
		t.Error("the model must be told the result was cut")
	}
}

// A name the server does not serve is the model's mistake. Reporting it as a
// result keeps the turn alive; a Go error would end it.
func TestCall_unknownToolComesBackAsAnErrorResult(t *testing.T) {
	s := newSession(t, stubTools(nil))
	got := s.call(context.Background(), "run-1", "linetta_nonexistent", `{}`)
	if !got.IsError {
		t.Fatal("want IsError")
	}
	if got.Text == "" {
		t.Error("want a message the model can act on")
	}
}

// Malformed arguments come from the model, not from Linetta.
func TestCall_malformedArgumentsComeBackAsAnErrorResult(t *testing.T) {
	s := newSession(t, stubTools(nil))
	got := s.call(context.Background(), "run-1", "echo", `{"text":`)
	if !got.IsError {
		t.Fatal("want IsError")
	}
}

// linetta_request_illustration's request_id rides on the tool line, so the
// panel can put the writer's approval card under it (illustration Phase 4).
func TestWriteMetadata_readsAnIllustrationRequest(t *testing.T) {
	res := &mcp.CallToolResult{StructuredContent: map[string]any{
		"request_id": "req-1", "status": "waiting_for_writer", "prompt": "p",
	}}
	batch, snap, nodes, request := writeMetadata(res)
	if request != "req-1" || batch != "" || snap != "" || len(nodes) != 0 {
		t.Fatalf("got batch=%q snap=%q nodes=%v request=%q", batch, snap, nodes, request)
	}
	if _, _, _, request := writeMetadata(&mcp.CallToolResult{}); request != "" {
		t.Fatalf("no structured content gave request %q", request)
	}
}

func toolTurn(sizes ...int) []llm.ChatMessage {
	msgs := []llm.ChatMessage{{Role: "system", Content: "s"}, {Role: "user", Content: "u"}}
	for i, size := range sizes {
		msgs = append(msgs,
			llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: fmt.Sprint(i), Name: "echo"}}},
			llm.ChatMessage{Role: "tool", ToolCallID: fmt.Sprint(i), Content: strings.Repeat("가", size)},
		)
	}
	return msgs
}

func TestClearOldToolResults_leavesATurnWithinTheHeadroomAlone(t *testing.T) {
	msgs := toolTurn(toolResultBudget, toolResultHeadroom)
	clearOldToolResults(msgs)
	for _, m := range msgs {
		if m.Content == clearedToolResult {
			t.Fatal("a result was cleared inside the headroom")
		}
	}
}

func TestClearOldToolResults_keepsTheNewestWithinBudget(t *testing.T) {
	msgs := toolTurn(30000, 30000, 30000, 30000, 30000)
	clearOldToolResults(msgs)

	kept := []string{}
	for _, m := range msgs {
		if m.Role != "tool" {
			continue
		}
		if m.Content == clearedToolResult {
			kept = append(kept, "cleared")
		} else {
			kept = append(kept, "kept")
		}
	}
	want := []string{"cleared", "cleared", "cleared", "kept", "kept"}
	if !slices.Equal(kept, want) {
		t.Fatalf("results = %v, want %v", kept, want)
	}
	if msgs[2].ToolCalls == nil || msgs[3].ToolCallID != "0" {
		t.Fatal("clearing a result must leave the call and its result paired")
	}
}

// One response can ask for many calls at once. The model has read none of
// their results yet, so none of them may be cleared, whatever they add up to.
func TestClearOldToolResults_keepsResultsTheModelHasNotRead(t *testing.T) {
	msgs := toolTurn(1000, 1)
	for i := range 6 {
		msgs = append(msgs, llm.ChatMessage{Role: "tool", ToolCallID: fmt.Sprint(i), Content: strings.Repeat("가", 24000)})
	}
	clearOldToolResults(msgs)

	if msgs[3].Content != clearedToolResult {
		t.Error("the older result the model already read was not cleared")
	}
	for _, m := range msgs[5:] {
		if m.Content == clearedToolResult {
			t.Fatal("a result the model has not read was cleared")
		}
	}
}

func TestRun_clearsOldToolResultsBeforeTheNextRequest(t *testing.T) {
	rec := &recorder{}
	responses := make([]llm.ChatResponse, 0, 7)
	for range 6 {
		responses = append(responses, toolReply("echo", `{"text":"huge"}`))
	}
	c := &scriptedClient{responses: append(responses, textReply("끝"))}
	svc := newService(t, c, rec)

	if _, err := svc.Run(context.Background(), RunRequest{ProjectID: "p1", Prompt: "다 읽어"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitFor(t, "agent.done", func() bool { return rec.has("agent.done") })

	cleared := 0
	for _, m := range c.messages() {
		if m.Role == "tool" && m.Content == clearedToolResult {
			cleared++
		}
	}
	if cleared == 0 {
		t.Fatal("six oversized results went to the provider with none cleared")
	}
}
