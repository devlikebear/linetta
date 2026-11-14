//go:build !mobile

package engineapp

import (
	"encoding/json"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/agenttest"
)

// A turn's request lands in agent_usage, and agent.usage reads it back summed
// by kind and model with the cache tokens the provider reported.
func TestAgentUsage_aTurnsRequestsShowUpInTheSummary(t *testing.T) {
	app := openApp(t)
	projectID, nodeID := seedProjectWithScene(t, app)
	consent := `{"provider":"anthropic","providers":{"anthropic":{"api_key":"sk-test","consented_at":1700000000000}}}`
	if _, rpcErr := call(t, app, "settings.set", consent); rpcErr != nil {
		t.Fatalf("settings.set: %+v", rpcErr)
	}
	app.SetProviderFactoryForTest(agenttest.NewScriptedClientFactory(agenttest.ScriptedTurn{
		Text:        "네.",
		InputTokens: 40, OutputTokens: 6, CacheReadTokens: 12000, CacheWriteTokens: 800,
	}))
	changed := app.CaptureNotificationsForTest()
	if _, rpcErr := call(t, app, "agent.run",
		`{"project_id":"`+projectID+`","node_id":"`+nodeID+`","prompt":"안녕"}`); rpcErr != nil {
		t.Fatalf("agent.run: %+v", rpcErr)
	}
	waitForNotification(t, changed, "agent.done")

	raw, rpcErr := call(t, app, "agent.usage", `{"days":7}`)
	if rpcErr != nil {
		t.Fatalf("agent.usage: %+v", rpcErr)
	}
	var got struct {
		Rows []struct {
			Kind, Provider       string
			Calls, Input, Output int64
			CacheRead            int64 `json:"cache_read"`
			CacheWrite           int64 `json:"cache_write"`
		}
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	if len(got.Rows) != 1 {
		t.Fatalf("rows = %s, want the one turn request", raw)
	}
	r := got.Rows[0]
	if r.Kind != "turn" || r.Provider != "anthropic" || r.Calls != 1 || r.Input != 40 || r.Output != 6 || r.CacheRead != 12000 || r.CacheWrite != 800 {
		t.Errorf("row = %+v, want the turn's request as the provider reported it", r)
	}
}
