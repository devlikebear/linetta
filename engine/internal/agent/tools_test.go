//go:build !mobile

package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/mcphost"
)

type echoIn struct {
	Text string `json:"text"`
}

type echoOut struct {
	UndoBatchID  string   `json:"undo_batch_id,omitempty"`
	ChangedNodes []string `json:"changed_nodes,omitempty"`
}

// stubTools installs one tool that echoes its input back, reports the run id
// it saw, and can be told to fail or to return an oversized body.
func stubTools(seenRunID *string) RegisterTools {
	return func(s *mcp.Server) {
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
				}, echoOut{UndoBatchID: "batch-1", ChangedNodes: []string{"n1", "n2"}}, nil
			})
	}
}

func newSession(t *testing.T, register RegisterTools) *toolSession {
	t.Helper()
	s, err := connectTools(context.Background(), register)
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
