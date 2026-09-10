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
	"errors"
	"fmt"
	"strings"
	"sync"

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
//
// The tool budget (#99) is passed in rather than read by the registrar,
// because the turn has already read it: Run resolves the switches once and
// that same value builds the server, labels the session and bounds the
// prompt. A registrar that read the store again would answer a settings.Set
// landing in between, and the session — cached under the label from the FIRST
// reading — would serve the disagreement to every later turn.
type RegisterTools func(*mcp.Server, mcphost.ToolGroups)

// hostGroups is the turn's tool budget in the form the tool layer takes. The
// conversion lives here rather than in prompt.go so the prompt stays free of
// the tool layer, and it exists so the value that bounded the prompt is
// literally the value that builds the server — rather than the two being read
// separately and hoped to agree.
func (g toolGroups) hostGroups() mcphost.ToolGroups {
	return mcphost.ToolGroups{Memory: g.memory, Skills: g.skills}
}

// toolSession is one connected client/server pair. Normally one per engine:
// the tools are stateless, and a session per run would re-handshake on every
// turn. It is rebuilt only when the writer's tool budget changes, because the
// tool set is fixed at registration time and a live server cannot be
// re-registered — see Service.session.
type toolSession struct {
	client *mcp.ClientSession
	server *mcp.ServerSession
	// groups is the tool budget (#99) the server was BUILT for. The session
	// is cached across turns, so this is the only thing that can tell a later
	// turn whether the cached tools still match the switches its own prompt
	// is about to describe.
	groups toolGroups

	// mu guards the retirement bookkeeping. A superseded session outlives the
	// call that swapped it out: a turn already in flight holds it for its
	// whole life, and its self-review for longer still. So a rebuild may not
	// close it — it retires it, and the last holder to let go does the
	// closing. Cancelling those turns instead would be a switch flip that
	// kills the message the writer is waiting on.
	mu      sync.Mutex
	refs    int
	retired bool
	closed  bool
}

// acquire takes one reference. It has two callers, and only one of them holds
// Service.toolsMu:
//
//   - Service.session, under toolsMu, taking the FIRST reference — which is
//     what makes it safe to hand a turn a session that is not already closed:
//     nothing can retire it between the check and the reference.
//   - startSelfReview, holding no lock at all. Safe for a different reason:
//     it acquires on the turn's own goroutine, against a session that turn is
//     still holding, so the refcount cannot be at zero and a concurrent retire
//     can only mark the session, not close it.
//
// Acquiring a session nobody else holds and nobody has retired from outside
// the lock would be the unsafe case, and there is no such caller.
func (s *toolSession) acquire() {
	s.mu.Lock()
	s.refs++
	s.mu.Unlock()
}

// release drops one reference, closing the session if it has been retired and
// this was the last holder. Every acquire owes exactly one release — and only
// one.
//
// A second release for the same reference is a bookkeeping bug in this file
// (a missing handedOff guard, a defer on the wrong goroutine), and it is
// reported rather than absorbed. It cannot double-close: `closed` is what
// actually stops that, and the extra release therefore does no visible harm
// today. But it would leave refs at -1, and a NEGATIVE refcount is a loaded
// gun — the next holder's release would then see refs drop to -1 again from
// its own legitimate acquire/release pair, so a retire could close a session
// another holder is still calling tools on. Clamping at zero keeps the count
// meaning what it says; the returned error is how the caller's logf makes the
// bug visible instead of leaving it to be found by a closed pipe months later.
func (s *toolSession) release() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	s.refs--
	if s.refs < 0 {
		s.refs = 0
		s.mu.Unlock()
		return errors.New("agent: tool session released more times than it was acquired")
	}
	last := s.retired && s.refs <= 0 && !s.closed
	if last {
		s.closed = true
	}
	s.mu.Unlock()
	if !last {
		return nil
	}
	return s.Close()
}

// retire marks the session superseded so no further turn is handed it, and
// closes it right away when nobody is holding it. A held session is closed by
// its last release instead.
func (s *toolSession) retire() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	s.retired = true
	now := s.refs <= 0 && !s.closed
	if now {
		s.closed = true
	}
	s.mu.Unlock()
	if !now {
		return nil
	}
	return s.Close()
}

// connectTools builds the server, registers the tools and dials it in memory.
// groups does two jobs here, and they are the same reading: it BUILDS the tool
// set (passed to register) and it LABELS the session (stored on it, so a later
// turn can tell whether the cached tools still match what its own prompt is
// about to describe). One value doing both is what makes the label honest —
// while register read the switches for itself, the two could come from
// readings either side of a settings.Set, and the label is the cache key.
func connectTools(ctx context.Context, register RegisterTools, groups toolGroups) (*toolSession, error) {
	if register == nil {
		return nil, fmt.Errorf("agent: no tool registration")
	}
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    mcphost.ServerName,
		Title:   "Linetta",
		Version: mcphost.ServerVersion,
	}, nil)
	register(srv, groups.hostGroups())

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
	return &toolSession{client: cs, server: ss, groups: groups}, nil
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
	Text       string
	IsError    bool
	BatchID    string
	SnapshotID string
	NodeIDs    []string
	Truncated  bool
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
	out.BatchID, out.SnapshotID, out.NodeIDs = writeMetadata(res)
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

// writeMetadata pulls the undo batch, the pre-write snapshot, and the touched
// scenes out of a write tool's structured output, so the panel can offer
// "undo this" without the loop knowing which tools are writes.
//
// snapshot_id is read the same way undo_batch_id is: linetta_apply_story_ops
// (an outline batch) sets undo_batch_id, while linetta_write_scene and
// linetta_revise_scene set snapshot_id instead — a tool call carries at most
// one of the two, never both (#112).
func writeMetadata(res *mcp.CallToolResult) (string, string, []string) {
	if res.StructuredContent == nil {
		return "", "", nil
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		return "", "", nil
	}
	var meta struct {
		UndoBatchID  string   `json:"undo_batch_id"`
		SnapshotID   string   `json:"snapshot_id"`
		ChangedNodes []string `json:"changed_nodes"`
		NodeID       string   `json:"node_id"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return "", "", nil
	}
	nodes := meta.ChangedNodes
	if len(nodes) == 0 && meta.NodeID != "" {
		nodes = []string{meta.NodeID}
	}
	return meta.UndoBatchID, meta.SnapshotID, nodes
}
