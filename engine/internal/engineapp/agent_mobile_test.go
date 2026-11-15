//go:build mobile

package engineapp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// The phone has the built-in agent but no MCP server: the agent reaches its
// tools over an in-memory pipe, and it connects with an API key or through
// the Linetta Works connection (the trial) — never a browser sign-in that
// needs a local listener, never a subprocess.
func TestAgentIsAvailableOnMobileWithAPIKeyProvidersOnly(t *testing.T) {
	app := openTestApp(t, t.TempDir())
	raw, err := app.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"diagnostics.version","params":null}`))
	if err != nil || !strings.Contains(string(raw), `"agent_available":true`) || !strings.Contains(string(raw), `"mcp_available":false`) {
		t.Fatalf("diagnostics.version: %s %v", raw, err)
	}

	raw, err = app.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":2,"method":"providers.list","params":null}`))
	if err != nil {
		t.Fatalf("providers.list: %v", err)
	}
	var resp struct {
		Result []struct {
			ID   string `json:"id"`
			Auth string `json:"auth"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("providers.list: %s %v", raw, err)
	}
	var ids []string
	for _, p := range resp.Result {
		ids = append(ids, p.ID)
		if p.Auth != "api_key" && p.Auth != "works" {
			t.Errorf("%s connects with %q on mobile, want api_key or works", p.ID, p.Auth)
		}
	}
	if strings.Join(ids, ",") != "linetta-trial,anthropic,gemini-native,openai" {
		t.Errorf("providers = %v, want linetta-trial, anthropic, gemini-native, openai", ids)
	}
}
