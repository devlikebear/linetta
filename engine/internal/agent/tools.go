//go:build !mobile

// Package agent is Linetta's built-in writing agent. It is a client of
// Linetta's own MCP server rather than a second tool layer: the same
// ToolDeps that serve Claude Desktop over HTTP are registered on a second
// mcp.Server here and reached over an in-memory pipe — no port, no token, no
// Origin check, and no second set of tool descriptions to keep in step.
//
// Part of the built-in BYOK agent (#90, issue #93).
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/mcphost"
	"github.com/devlikebear/tars/pkg/llm"
)

// maxToolResultChars caps one tool result before it enters the model's
// context. linetta_read_scene returning a long scene is normal, so the cap is
// generous; it exists so a loop cannot paste an entire manuscript into every
// subsequent request.
const maxToolResultChars = 24000

// RegisterTools installs the tool set on a fresh server. The caller supplies
// mcphost.ToolDeps.Register bound to full mode — the built-in agent is always
// full: an agent that cannot write the manuscript has no reason to exist.
type RegisterTools func(*mcp.Server)

// toolSession is one connected client/server pair. One per engine: the tools
// are stateless, and a session per run would re-handshake on every turn.
type toolSession struct {
	client *mcp.ClientSession
	server *mcp.ServerSession
}

// connectTools builds the server, registers the tools and dials it in memory.
func connectTools(ctx context.Context, register RegisterTools) (*toolSession, error) {
	if register == nil {
		return nil, fmt.Errorf("agent: no tool registration")
	}
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    mcphost.ServerName,
		Title:   "Linetta",
		Version: mcphost.ServerVersion,
	}, nil)
	register(srv)

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, serverTransport, nil)
	if err != nil {
		return nil, fmt.Errorf("agent: connect tool server: %w", err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{
		Name: "linetta-builtin-agent", Version: mcphost.ServerVersion,
	}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		_ = ss.Close()
		return nil, fmt.Errorf("agent: connect tool client: %w", err)
	}
	return &toolSession{client: cs, server: ss}, nil
}

func (s *toolSession) Close() error {
	if s == nil {
		return nil
	}
	err := s.client.Close()
	if serr := s.server.Close(); err == nil {
		err = serr
	}
	return err
}

// schemas converts the server's tool list into what the model wants. The
// descriptions come straight from the MCP layer, so the workflow they already
// spell out ("read the context before drafting", "refresh the summary after
// writing") reaches the model without being written twice.
func (s *toolSession) schemas(ctx context.Context) ([]llm.ToolSchema, error) {
	list, err := s.client.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("agent: list tools: %w", err)
	}
	out := make([]llm.ToolSchema, 0, len(list.Tools))
	for _, t := range list.Tools {
		// The SDK hands the client the input schema as a map[string]any; the
		// model's schema field is raw JSON.
		params, err := json.Marshal(t.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("agent: schema for %s: %w", t.Name, err)
		}
		out = append(out, llm.ToolSchema{
			Type: "function",
			Function: llm.ToolFunctionSchema{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			},
		})
	}
	return out, nil
}

// toolResult is one tool call reduced to what both the model and the panel
// need: the text to feed back, and the write metadata the undo button uses.
type toolResult struct {
	Text      string
	IsError   bool
	BatchID   string
	NodeIDs   []string
	Truncated bool
}

// call runs one tool. Every failure the model could have caused — a bad name,
// malformed arguments, a version conflict — comes back as an error *result*,
// not a Go error: the model reads it, corrects itself and tries again. Only
// the loop's own cancellation ends a turn.
func (s *toolSession) call(ctx context.Context, runID, name, arguments string) toolResult {
	args := map[string]any{}
	if trimmed := strings.TrimSpace(arguments); trimmed != "" && trimmed != "null" {
		if err := json.Unmarshal([]byte(trimmed), &args); err != nil {
			return toolResult{
				IsError: true,
				Text:    fmt.Sprintf("arguments for %s are not a JSON object: %v", name, err),
			}
		}
	}

	params := &mcp.CallToolParams{Name: name, Arguments: args}
	if runID != "" {
		params.SetMeta(map[string]any{mcphost.MetaRunID: runID})
	}
	res, err := s.client.CallTool(ctx, params)
	if err != nil {
		// A cancelled turn is the caller's business, not the model's.
		if ctx.Err() != nil {
			return toolResult{IsError: true, Text: "the writer stopped this turn"}
		}
		return toolResult{IsError: true, Text: fmt.Sprintf("tool %s could not be called: %v", name, err)}
	}

	out := toolResult{IsError: res.IsError}
	out.Text, out.Truncated = capText(textOf(res))
	out.BatchID, out.NodeIDs = writeMetadata(res)
	return out
}

func textOf(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func capText(s string) (string, bool) {
	r := []rune(s)
	if len(r) <= maxToolResultChars {
		return s, false
	}
	return string(r[:maxToolResultChars]) +
		"\n\n[truncated: the result was longer than this tool's limit. " +
		"Narrow the request if you need the rest.]", true
}

// writeMetadata pulls the undo batch and the touched scenes out of a write
// tool's structured output, so the panel can offer "undo this" without the
// loop knowing which tools are writes.
func writeMetadata(res *mcp.CallToolResult) (string, []string) {
	if res.StructuredContent == nil {
		return "", nil
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		return "", nil
	}
	var meta struct {
		UndoBatchID  string   `json:"undo_batch_id"`
		ChangedNodes []string `json:"changed_nodes"`
		NodeID       string   `json:"node_id"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return "", nil
	}
	nodes := meta.ChangedNodes
	if len(nodes) == 0 && meta.NodeID != "" {
		nodes = []string{meta.NodeID}
	}
	return meta.UndoBatchID, nodes
}
