//go:build mobile

package engineapp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// The phone has the built-in agent but no MCP server: the agent reaches its
// tools over an in-memory pipe, and it connects with an API key only.
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
		if p.Auth != "api_key" {
			t.Errorf("%s connects with %q on mobile, want api_key", p.ID, p.Auth)
		}
	}
	if strings.Join(ids, ",") != "anthropic,gemini-native,openai" {
		t.Errorf("providers = %v, want anthropic, gemini-native, openai", ids)
	}
}
