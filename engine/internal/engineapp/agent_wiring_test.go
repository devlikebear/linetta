//go:build !mobile

package engineapp

import (
	"encoding/json"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/mcphost"
)

// The five agent.* registrations, the agent_available capability flag, and
// the three deliberate ToolDeps differences from the external host (Source,
// Limiter, Story) are the one link nothing else covers: a typo in a method
// name, or a future refactor that quietly drops one of the three
// differences, compiles, passes every other package test, and only shows up
// as a writer's agent starving against — or reverting — an external client's
// work. This drives a real *App through app.Handle the way the renderer
// does, mirroring providers_wiring_test.go's and mcp_wiring_test.go's
// existing pattern for this package.
func TestAgentMethodsAreRegisteredOnAFreshInstall(t *testing.T) {
	app := openApp(t)

	// agent.run: no provider is configured on a fresh install, so this must
	// reach the same reason code providers.list_models/test hit — proving
	// both that the method is wired and that the consent gate sits in front
	// of it, without dialling anyone.
	_, rpcErr := call(t, app, "agent.run", `{"project_id":"p1","prompt":"hello"}`)
	if rpcErr == nil {
		t.Fatal("agent.run: expected a refusal on a fresh install with no provider configured")
	}
	if got := string(rpcErr.Data); got != `{"reason":"provider_not_configured"}` {
		t.Errorf("agent.run: error data = %s, want a provider_not_configured reason", got)
	}

	// agent.cancel: an unknown run id is not an error — the writer's stop
	// click can land after the turn already finished.
	if _, rpcErr := call(t, app, "agent.cancel", `{"run_id":"no-such-run"}`); rpcErr != nil {
		t.Fatalf("agent.cancel: %+v", rpcErr)
	}

	// agent.history: an empty conversation is a JSON array, not null — the
	// panel renders this directly.
	result, rpcErr := call(t, app, "agent.history", `{"project_id":"p1"}`)
	if rpcErr != nil {
		t.Fatalf("agent.history: %+v", rpcErr)
	}
	if string(result) != "[]" {
		t.Errorf("agent.history: result = %s, want []", result)
	}

	// agent.clear: dropping an already-empty conversation is not an error.
	if _, rpcErr := call(t, app, "agent.clear", `{"project_id":"p1"}`); rpcErr != nil {
		t.Fatalf("agent.clear: %+v", rpcErr)
	}

	// agent.undo: no batch exists (the undo window is empty on a fresh
	// install), so this is the ordinary shape of "nothing to undo" — it must
	// reach the agent_undo_unavailable reason code (fix round 1), not
	// storyops' raw English sentence.
	_, rpcErr = call(t, app, "agent.undo", `{"batch_id":"no-such-batch"}`)
	if rpcErr == nil {
		t.Fatal("agent.undo: expected a refusal for an unknown batch")
	}
	if got := string(rpcErr.Data); got != `{"reason":"agent_undo_unavailable"}` {
		t.Errorf("agent.undo: error data = %s, want an agent_undo_unavailable reason", got)
	}
}

// agent_available must reach diagnostics.get so the panel knows whether to
// offer itself at all.
func TestAgentAvailableIsTrueOnThisBuild(t *testing.T) {
	app := openApp(t)
	result, rpcErr := call(t, app, "diagnostics.get", "")
	if rpcErr != nil {
		t.Fatalf("diagnostics.get: %+v", rpcErr)
	}
	var got struct {
		AgentAvailable bool `json:"agent_available"`
	}
	if err := json.Unmarshal(result, &got); err != nil {
		t.Fatalf("decode diagnostics.get: %v", err)
	}
	if !got.AgentAvailable {
		t.Error("agent_available = false, want true on a !mobile build")
	}
}

// The agent's tool deps must be its own, not the external host's: sharing a
// limiter lets a busy Claude Desktop session starve the panel and vice versa,
// sharing a storyops service lets the panel's undo button revert an external
// agent's batch, and the Source field is what exempts the agent from the
// mcp_project_id restriction meant for external clients. None of this has a
// wire-visible effect on a fresh install with no provider configured, so it
// is asserted directly against the unexported fields agent_enabled.go and
// engineapp.go carry for exactly this test (same package, no accessor
// invented for it).
func TestAgentToolDepsDifferFromTheExternalHosts(t *testing.T) {
	app := openApp(t)

	if app.agentCtrl == nil {
		t.Fatal("app.agentCtrl is nil — setupAgent did not run")
	}
	agentTools := app.agentCtrl.tools
	externalTools := app.mcpTools

	if agentTools.Source != mcphost.SourceAgent {
		t.Errorf("agent tools.Source = %q, want %q", agentTools.Source, mcphost.SourceAgent)
	}
	if externalTools.Source == mcphost.SourceAgent {
		t.Error("external tools.Source must stay empty (external), not the agent's")
	}

	if agentTools.Limiter == nil {
		t.Fatal("agent tools.Limiter is nil")
	}
	if externalTools.Limiter == nil {
		t.Fatal("external tools.Limiter is nil")
	}
	if agentTools.Limiter == externalTools.Limiter {
		t.Error("agent and external tool deps share one limiter instance — a busy external client would starve the panel, and vice versa")
	}

	if agentTools.Story == nil {
		t.Fatal("agent tools.Story is nil")
	}
	if externalTools.Story == nil {
		t.Fatal("external tools.Story is nil")
	}
	if agentTools.Story == externalTools.Story {
		t.Error("agent and external tool deps share one storyops.Service instance — the panel's undo button could revert an external agent's batch")
	}
}
