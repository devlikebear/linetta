package provider

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/devlikebear/linetta/engine/internal/clidetect"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/tars/pkg/llm"
)

// The Claude Code CLI provider ("claude-code"). Linetta does not talk to an
// API here: it runs the writer's own `claude` binary, which signs in with
// their own Claude login and runs its own tool loop. Two things follow, and
// this file is where both are settled once for every caller.
//
//   - Claude Code is a whole agent, with file, shell and web tools, the
//     writer's CLAUDE.md, hooks, plugins and MCP servers. The consent
//     sentence promises the manuscript goes to Anthropic and nowhere else, so
//     the client Source.Client hands out is locked down by construction
//     (lockedClaudeCode): no built-in tools, none of the writer's
//     customisations, no MCP server but the one the agent passes, and every
//     permission prompt refused unless it is for one of Linetta's own tools.
//     A caller cannot widen it — there is no option that reaches past the
//     wrapper.
//   - tars knows the provider as "claude-code-cli" and finds the binary
//     through CLAUDE_CODE_CLI_PATH. A desktop app opened from the Finder has
//     a PATH without Homebrew or npm in it, so clidetect looks in the login
//     shell and the usual install locations instead.

// tarsClaudeCodeProvider is tars' name for the provider Linetta calls
// settings.ProviderClaudeCode.
const tarsClaudeCodeProvider = "claude-code-cli"

// claudeCodeCLIPathEnv is where tars looks for the binary before PATH.
const claudeCodeCLIPathEnv = "CLAUDE_CODE_CLI_PATH"

// claudeCodeToolPrefix is how Claude Code names a tool from the MCP server
// the agent registers as "linetta": mcp__<server>__<tool>.
const claudeCodeToolPrefix = "mcp__linetta__"

// claudeCodeModels is what providers.list_models offers. The CLI has no model
// list to ask; these are its own aliases, each following Anthropic's newest
// model of that tier, and any full model id typed in settings still works.
var claudeCodeModels = []string{"haiku", "opus", "sonnet"}

// IsSelfExecuting reports whether a provider runs its own tool loop, so the
// agent must hand it tools over MCP rather than execute ToolCalls itself.
func IsSelfExecuting(id string) bool { return id == settings.ProviderClaudeCode }

// cliDetector caches where the binary is. providers.list resolves every
// provider on every call, and a detection can start a login shell — so a
// found path is kept, and a miss is retried at most every cliRedetectAfter so
// a writer who installs Claude Code while Linetta is open is not asked to
// restart it.
type cliDetector struct {
	detect func(context.Context) string

	mu      sync.Mutex
	path    string
	checked time.Time
}

const cliRedetectAfter = 5 * time.Second

func newCLIDetector() *cliDetector { return &cliDetector{detect: clidetect.Detect} }

func (d *cliDetector) Path() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.path != "" {
		if isFile(d.path) {
			return d.path
		}
		// Uninstalled or moved since: look again right away.
		d.path, d.checked = "", time.Time{}
	}
	if !d.checked.IsZero() && time.Since(d.checked) < cliRedetectAfter {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	d.path, d.checked = d.detect(ctx), time.Now()
	return d.path
}

// Forget drops the cached answer, for the settings pane's "check again".
func (d *cliDetector) Forget() {
	d.mu.Lock()
	d.path, d.checked = "", time.Time{}
	d.mu.Unlock()
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// pointTarsAtCLI makes the detected binary the one tars runs. Process-wide,
// because tars reads it from the environment; there is only ever one CLI.
//
// The binary's own directory also goes on PATH. An npm install is a script
// whose first line asks env for `node`, which lives next to it — in
// /opt/homebrew/bin, say — and is not on the PATH a Finder-launched app gets.
func pointTarsAtCLI(path string) {
	_ = os.Setenv(claudeCodeCLIPathEnv, path)
	dir := filepath.Dir(path)
	current := os.Getenv("PATH")
	if slices.Contains(filepath.SplitList(current), dir) {
		return
	}
	if current == "" {
		_ = os.Setenv("PATH", dir)
		return
	}
	_ = os.Setenv("PATH", dir+string(os.PathListSeparator)+current)
}

// lockedClaudeCode is the only shape a claude-code client leaves this package
// in. See the file comment for why; each line of Chat below closes one way
// the manuscript could reach something other than Anthropic.
type lockedClaudeCode struct{ inner llm.Client }

func (c lockedClaudeCode) Ask(ctx context.Context, prompt string) (string, error) {
	// Not inner.Ask: that would reach inner.Chat without passing through
	// the lock below.
	resp, err := c.Chat(ctx, []llm.ChatMessage{{Role: "user", Content: prompt}}, llm.ChatOptions{})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Message.Content), nil
}

func (c lockedClaudeCode) Chat(ctx context.Context, messages []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	maxTurns := 0
	if opts.ClaudeCodeHarness != nil {
		maxTurns = opts.ClaudeCodeHarness.MaxTurns
	}
	locked := llm.ChatOptions{
		OnDelta:        opts.OnDelta,
		OnProviderTool: opts.OnProviderTool,
		// Only the agent's own server, and only under the name the
		// permission rule below recognises.
		ClaudeCodeMCPServers: linettaServersOnly(opts.ClaudeCodeMCPServers),
		// "default" so that every tool call not already allowed becomes a
		// prompt, and every prompt reaches onlyLinettaTools.
		ClaudeCodePermissionMode:    "default",
		ClaudeCodePermissionHandler: onlyLinettaTools,
		ClaudeCodeHarness: &llm.ClaudeCodeHarnessOptions{
			// No CLAUDE.md, skills, plugins, hooks or the writer's own MCP
			// servers: their setup is for their code, not this manuscript.
			SafeMode:  true,
			StrictMCP: true,
			// No browser integration, and only the environment a login
			// needs — none of the shell's tokens or proxies ride along.
			DisableChrome:      true,
			IsolateEnvironment: true,
			// Empty, not nil: `--tools ""`, no built-in tool at all. Reading
			// and writing the manuscript is what Linetta's MCP tools are
			// for; Bash, Edit, Read and WebFetch would reach the disk and
			// the network around them.
			Tools:    []string{},
			MaxTurns: maxTurns,
		},
	}
	return c.inner.Chat(ctx, instructionsAsPrompt(messages), locked)
}

func linettaServersOnly(servers []llm.ClaudeCodeMCPServer) []llm.ClaudeCodeMCPServer {
	var out []llm.ClaudeCodeMCPServer
	for _, s := range servers {
		if s.Name == "linetta" {
			out = append(out, s)
		}
	}
	return out
}

func onlyLinettaTools(_ context.Context, req llm.ClaudeCodePermissionRequest) (llm.ClaudeCodePermissionDecision, error) {
	if strings.HasPrefix(req.ToolName, claudeCodeToolPrefix) {
		return llm.ClaudeCodePermissionDecision{Allow: true}, nil
	}
	return llm.ClaudeCodePermissionDecision{
		Message: "Only Linetta's own tools (" + claudeCodeToolPrefix + "*) are available here.",
	}, nil
}

// instructionsAsPrompt moves the system message into the conversation.
//
// tars passes a system message to the CLI on its command line, where Windows
// allows 32,767 characters for everything and Linux 128 KB per argument — and
// Linetta's system prompt, with the writer's memory documents, can pass
// either. The conversation itself goes over stdin (the permission handler
// puts tars in stream-json control mode), which has no such limit. What stays
// on the command line is tars' own three-line preamble.
func instructionsAsPrompt(messages []llm.ChatMessage) []llm.ChatMessage {
	out := make([]llm.ChatMessage, 0, len(messages))
	for _, m := range messages {
		if strings.EqualFold(strings.TrimSpace(m.Role), "system") {
			if text := strings.TrimSpace(m.Content); text != "" {
				out = append(out, llm.ChatMessage{
					Role:    "user",
					Content: "[Instructions from Linetta, the writing app you are working inside. Follow them as your system instructions.]\n\n" + text,
				})
			}
			continue
		}
		out = append(out, m)
	}
	return out
}
