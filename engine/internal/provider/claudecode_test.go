//go:build !mas && !mobile

package provider

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/tars/pkg/llm"
)

// recordingClient keeps the last Chat call it was given.
type recordingClient struct {
	msgs []llm.ChatMessage
	opts llm.ChatOptions
}

func (c *recordingClient) Ask(context.Context, string) (string, error) {
	return "", errors.New("Ask must not be reached: it would bypass the lock")
}

func (c *recordingClient) Chat(_ context.Context, msgs []llm.ChatMessage, opts llm.ChatOptions) (llm.ChatResponse, error) {
	c.msgs, c.opts = msgs, opts
	return llm.ChatResponse{Message: llm.ChatMessage{Role: "assistant", Content: " OK "}}, nil
}

func claudeCodeSource(t *testing.T, cliPath string) (*Source, *settings.Store, *recordingClient, *llm.ProviderOptions) {
	t.Helper()
	src, st, _ := newSource(t)
	rec := &recordingClient{}
	var built llm.ProviderOptions
	src.WithCLIDetector(func(context.Context) string { return cliPath }).
		WithCLIWorkDir(filepath.Join(t.TempDir(), "agent-cli")).
		WithFactory(func(o llm.ProviderOptions) (llm.Client, error) { built = o; return rec, nil })
	if _, err := st.Set(context.Background(), settings.Patch{
		Provider:  strPtr(settings.ProviderClaudeCode),
		Providers: map[string]settings.ProviderPatch{settings.ProviderClaudeCode: {Model: strPtr("opus"), ConsentedAt: int64Ptr(1700000000000)}},
	}); err != nil {
		t.Fatal(err)
	}
	return src, st, rec, &built
}

func fakeCLI(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestClaudeCode_isConfiguredByTheBinaryAlone(t *testing.T) {
	src, _, _, _ := claudeCodeSource(t, "")
	r, err := src.Resolve("")
	if err != nil {
		t.Fatal(err)
	}
	if r.Configured() {
		t.Error("configured without a CLI")
	}
	if _, _, err := src.Client(context.Background(), ""); reasonOf(t, err) != rpc.ReasonProviderNotConfigured {
		t.Errorf("Client without a CLI: %v", err)
	}

	cli := fakeCLI(t)
	src, _, _, _ = claudeCodeSource(t, cli)
	r, _ = src.Resolve("")
	if !r.Configured() || r.CLIPath != cli {
		t.Errorf("resolved = %v", r)
	}
	statuses := src.List()
	last := statuses[len(statuses)-1]
	if last.ID != settings.ProviderClaudeCode || last.Auth != "cli" || last.CLIPath != cli || !last.Active {
		t.Errorf("status = %+v", last)
	}
}

func TestClaudeCode_mapsToTarsAndPointsItAtTheBinary(t *testing.T) {
	cli := fakeCLI(t)
	src, _, _, built := claudeCodeSource(t, cli)
	if _, _, err := src.Client(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if built.Provider != "claude-code-cli" || built.Model != "opus" || built.APIKey != "" || built.BaseURL != "" {
		t.Errorf("options = %+v", *built)
	}
	if info, err := os.Stat(built.WorkDir); err != nil || !info.IsDir() {
		t.Errorf("work dir %q was not created: %v", built.WorkDir, err)
	}
	if got := os.Getenv(claudeCodeCLIPathEnv); got != cli {
		t.Errorf("%s = %q", claudeCodeCLIPathEnv, got)
	}
	if !strings.HasPrefix(os.Getenv("PATH"), filepath.Dir(cli)) {
		t.Errorf("the binary's directory is not on PATH: %s", os.Getenv("PATH"))
	}
}

// Whatever a caller asks for, the client that leaves Source.Client runs
// Claude Code with nothing but Linetta's own MCP server.
func TestClaudeCode_isLockedDownWhateverTheCallerAsks(t *testing.T) {
	src, _, rec, _ := claudeCodeSource(t, fakeCLI(t))
	c, _, err := src.Client(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Chat(context.Background(), []llm.ChatMessage{
		{Role: "system", Content: "Linetta's long system prompt"},
		{Role: "user", Content: "1화를 다듬어 줘"},
	}, llm.ChatOptions{
		ClaudeCodeMCPServers: []llm.ClaudeCodeMCPServer{
			{Name: "linetta", Transport: "http", URL: "http://127.0.0.1:1/mcp"},
			{Name: "filesystem", Command: "npx"},
		},
		ClaudeCodePermissionMode:  "bypassPermissions",
		ClaudeCodePermissionAllow: []string{"Bash"},
		ClaudeCodeHarness:         &llm.ClaudeCodeHarnessOptions{Tools: []string{"Bash", "Edit"}, MaxTurns: 9},
		WorkDir:                   "/",
		PersistSession:            true,
	})
	if err != nil {
		t.Fatal(err)
	}
	o := rec.opts
	if len(o.ClaudeCodeMCPServers) != 1 || o.ClaudeCodeMCPServers[0].Name != "linetta" {
		t.Errorf("servers = %+v", o.ClaudeCodeMCPServers)
	}
	h := o.ClaudeCodeHarness
	if h == nil || !h.Restricted || h.SafeMode || !h.StrictMCP || !h.IsolateEnvironment || !h.DisableChrome {
		t.Fatalf("harness = %+v", h)
	}
	if h.Tools == nil || len(h.Tools) != 0 {
		t.Errorf("built-in tools = %#v, want an empty non-nil list (--tools \"\")", h.Tools)
	}
	if h.MaxTurns != 9 {
		t.Errorf("max turns = %d, want the caller's", h.MaxTurns)
	}
	if o.ClaudeCodePermissionMode != "default" || o.ClaudeCodePermissionAllow != nil || o.WorkDir != "" || o.PersistSession {
		t.Errorf("options widened: %+v", o)
	}
	if o.ClaudeCodePermissionHandler == nil {
		t.Fatal("no permission handler")
	}
	for name, allow := range map[string]bool{
		"mcp__linetta__linetta_read_scene": true,
		"mcp__filesystem__read_file":       false,
		"Bash":                             false,
		"WebFetch":                         false,
	} {
		d, err := o.ClaudeCodePermissionHandler(context.Background(), llm.ClaudeCodePermissionRequest{ToolName: name})
		if err != nil || d.Allow != allow {
			t.Errorf("%s: allow = %v (%v), want %v", name, d.Allow, err, allow)
		}
	}
	// The system prompt rides in the conversation (stdin), not on argv.
	if len(rec.msgs) != 2 || rec.msgs[0].Role != "user" || !strings.Contains(rec.msgs[0].Content, "Linetta's long system prompt") {
		t.Errorf("messages = %+v", rec.msgs)
	}

	// Ask goes through the same lock rather than the inner client's Ask.
	if out, err := c.Ask(context.Background(), "ping"); err != nil || out != "OK" {
		t.Errorf("Ask = %q, %v", out, err)
	}
	if rec.opts.ClaudeCodeHarness == nil || !rec.opts.ClaudeCodeHarness.Restricted {
		t.Error("Ask bypassed the lock")
	}
}

func TestClaudeCode_offersItsAliasesAsModels(t *testing.T) {
	src, _, _, _ := claudeCodeSource(t, fakeCLI(t))
	models, err := src.ListModels(context.Background(), settings.ProviderClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(models, ",") != "haiku,opus,sonnet" {
		t.Errorf("models = %v", models)
	}
}

func TestClassify_readsTheCLIsOwnWords(t *testing.T) {
	cases := map[string]string{
		"claude-code-cli executable not found in PATH or ~/.local/bin":                       rpc.ReasonProviderNotConfigured,
		"claude-code-cli request: Invalid API key · Please run /login":                       rpc.ReasonProviderAuthFailed,
		"claude-code-cli request: OAuth token has expired":                                   rpc.ReasonProviderAuthFailed,
		"claude-code-cli request: Claude AI usage limit reached|1760000000":                  rpc.ReasonProviderRateLimited,
		"claude-code-cli request: Credit balance is too low":                                 rpc.ReasonProviderRateLimited,
		"claude-code-cli request: cli timed out: no output for 15m0s":                        rpc.ReasonProviderTimeout,
		"claude-code-cli request: cli failed: exit status 1: something we have not seen yet": rpc.ReasonProviderUnreachable,
	}
	for msg, want := range cases {
		var re *rpc.ReasonError
		if !errors.As(Classify(settings.ProviderClaudeCode, errors.New(msg)), &re) || re.Reason != want {
			t.Errorf("%q → %v, want %s", msg, re, want)
		}
	}
}
