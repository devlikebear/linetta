//go:build !mobile

package agent

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/mcphost"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/tars/pkg/llm"
)

// A turn on a self-executing provider — the Claude Code CLI.
//
// Every other provider hands back ToolCalls and the loop runs them. Claude
// Code runs its own tool loop and hands back nothing to run, so the tools
// have to come to it: for the length of one turn the agent serves them over
// MCP on loopback, and Claude Code (told about that server alone, and allowed
// nothing else — see provider.lockedClaudeCode) calls them there.
//
// The server is a proxy, not a second tool layer. Each call it receives goes
// through runTool, the same path the loop uses, so the parts of a turn the
// panel and the transcript depend on — the agent.tool events, the run id on
// the activity log, the undo batch and snapshot ids, the tool-call cap and
// the repeated-error wall — are the loop's own, not a re-implementation of
// them.

// cliTurn is the state a CLI turn shares between the Chat call and the tool
// calls arriving on the bridge while it runs.
type cliTurn struct {
	s  *Service
	st loopState
	// ctx is the turn's context. Tool calls run under it rather than under
	// the HTTP request's, so the writer pressing stop reaches a tool the same
	// way it does in the loop.
	ctx context.Context

	// mu serialises the tool calls: Claude Code may send several at once,
	// and the cap, the wall and the transcript order are all counted one
	// call at a time in the loop too.
	mu        sync.Mutex
	pending   strings.Builder // reply text not yet a transcript row
	streamed  bool
	toolCalls int
	lastFail  string
	repeats   int
	wall      string // why the turn ran into a wall; "" while it has not
	asked     bool   // a card is waiting for the writer
}

// delta streams reply text and keeps it until it becomes a transcript row.
//
// Whatever Claude Code says after a card is dropped. Asked not to, it still
// announces the card under it, which only pushes the card up the log; the loop
// ends the turn before a model could.
func (t *cliTurn) delta(text string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.asked {
		return
	}
	t.s.streamDelta(t.st, text)
	t.pending.WriteString(text)
	t.streamed = true
}

// flush turns the text streamed since the last tool call into its own
// transcript row, so a restored conversation reads in the order it happened:
// what Claude said, the tool it called, what it said next. Called with mu
// held.
func (t *cliTurn) flush() {
	text := t.pending.String()
	t.pending.Reset()
	if strings.TrimSpace(text) != "" {
		if err := t.s.tr.appendAssistant(context.WithoutCancel(t.ctx), t.st.req.ProjectID, t.st.req.NodeID, t.st.runID,
			text, companion.HistoryStatusDone); err != nil {
			logf("transcript: %v", err)
		}
	}
	t.s.runs.committed(t.st.runID)
}

// call runs one tool Claude Code asked for.
func (t *cliTurn) call(name string, arguments json.RawMessage) *mcp.CallToolResult {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.flush()

	// The loop ends the turn at a card. Claude Code cannot be stopped there
	// without losing the session, so it is told instead.
	if t.asked {
		return toolText(true, "not run: "+askedReply)
	}
	if t.wall == "" && t.toolCalls >= t.st.maxToolCalls {
		t.wall = fmt.Sprintf("stopped after %d tool calls in one turn", t.toolCalls)
	}
	if t.wall != "" {
		// The loop ends the turn here and asks for a wrap-up. Claude Code
		// cannot be stopped mid-turn without losing what it would say, so
		// it is told instead, and its next words are the wrap-up.
		return toolText(true, "not run: "+t.wall+". Do not call any more tools; tell the writer how far you got.")
	}

	args := strings.TrimSpace(string(arguments))
	result := t.s.runTool(t.ctx, t.st, llm.ToolCall{Name: name, Arguments: args})
	t.toolCalls++
	t.asked = result.Ask != nil

	if !result.IsError {
		t.lastFail, t.repeats = "", 0
	} else {
		if name == t.lastFail {
			t.repeats++
		} else {
			t.lastFail, t.repeats = name, 1
		}
		if t.repeats >= maxRepeatedToolErrors {
			t.wall = fmt.Sprintf("%s failed %d times in a row", name, t.repeats)
		}
	}
	return toolText(result.IsError, result.Text)
}

func toolText(isError bool, text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: isError, Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

// cliBridge is the turn's MCP server on loopback.
type cliBridge struct {
	url   string
	token string
	srv   *http.Server
}

// startCLIBridge serves schemas — the turn's own tool list, the one the
// prompt describes — with every call going to turn.
func startCLIBridge(turn *cliTurn, schemas []llm.ToolSchema) (*cliBridge, error) {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "linetta",
		Title:   "Linetta",
		Version: mcphost.ServerVersion,
	}, nil)
	for _, schema := range schemas {
		name := schema.Function.Name
		server.AddTool(&mcp.Tool{
			Name:        name,
			Description: schema.Function.Description,
			InputSchema: json.RawMessage(schema.Function.Parameters),
		}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var args json.RawMessage
			if req != nil && req.Params != nil {
				args = req.Params.Arguments
			}
			return turn.call(name, args), nil
		})
	}

	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("agent: bridge listen: %w", err)
	}
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	srv := &http.Server{Handler: bridgeAuth(token, handler), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logf("bridge: %v", err)
		}
	}()
	return &cliBridge{url: "http://" + ln.Addr().String() + "/mcp", token: token, srv: srv}, nil
}

func (b *cliBridge) server() llm.ClaudeCodeMCPServer {
	return llm.ClaudeCodeMCPServer{
		Name:      "linetta",
		Transport: "http",
		URL:       b.url,
		Headers:   map[string]string{"Authorization": "Bearer " + b.token},
	}
}

func (b *cliBridge) close() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = b.srv.Shutdown(ctx)
}

// bridgeAuth admits only the CLI this turn started. The token is fresh per
// turn and travels only in the MCP config tars writes 0600 to a temp file.
// A request carrying an Origin came from a browser — the CLI sends none — so
// a page that guessed the port is turned away before the token is compared.
func bridgeAuth(token string, next http.Handler) http.Handler {
	want := []byte("Bearer " + token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), want) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("agent: bridge token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// loopCLI is loop for a self-executing provider: one Chat call, with the
// tools reachable over the bridge for as long as it runs.
func (s *Service) loopCLI(ctx context.Context, st loopState) {
	msgs := s.openingMessages(ctx, st)
	turn := &cliTurn{s: s, st: st, ctx: ctx}

	bridge, err := startCLIBridge(turn, st.schemas)
	if err != nil {
		logf("run %s: %v", st.runID, err)
		s.endWithReason(ctx, st, rpc.ReasonAgentInternalError, err.Error(), companion.HistoryStatusFailed)
		return
	}
	defer bridge.close()

	resp, err := st.client.Chat(ctx, msgs, llm.ChatOptions{
		OnDelta:              turn.delta,
		ClaudeCodeMCPServers: []llm.ClaudeCodeMCPServer{bridge.server()},
		// Claude Code's own turn count, one per model response. The tool
		// cap is enforced call by call above; this only keeps a turn that
		// ignores the "stop" answer from going on forever.
		ClaudeCodeHarness: &llm.ClaudeCodeHarnessOptions{MaxTurns: st.maxToolCalls + 8},
	})

	turn.mu.Lock()
	// Nothing streamed but there is a reply: the CLI said it only in its
	// final result. Show it, the way a streamed one would have been.
	if err == nil && !turn.streamed && !turn.asked {
		if text := strings.TrimSpace(resp.Message.Content); text != "" {
			s.streamDelta(st, text)
			turn.pending.WriteString(text)
		}
	}
	turn.flush()
	wall := turn.wall
	// A card is a reply: nothing need follow it.
	replied := turn.asked || turn.streamed || strings.TrimSpace(resp.Message.Content) != ""
	turn.mu.Unlock()

	if err != nil {
		s.endWithError(ctx, st, err)
		return
	}
	if ctx.Err() != nil {
		s.endCancelled(ctx, st)
		return
	}
	if wall != "" {
		s.endAtWall(ctx, st, wall)
		return
	}
	if !replied {
		s.endWithReason(ctx, st, rpc.ReasonAgentEmptyReply,
			"the model ended the turn without a reply (stop reason: "+resp.StopReason+")", companion.HistoryStatusDone)
		return
	}
	s.notify("agent.done", donePayload{
		RunID: st.runID, ProjectID: st.projectID, Seq: s.runs.next(st.runID), Model: st.model,
		Usage: usagePayload{Input: resp.Usage.InputTokens, Output: resp.Usage.OutputTokens},
	})
	// No self-review: it is a second loop of its own that runs the tool
	// calls a model hands back, which this provider never does — and a
	// second CLI turn the writer did not ask for is a cost on their Claude
	// plan they cannot see.
}
