//go:build !mobile

package engineapp

import (
	"encoding/json"
	"reflect"
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

// The three differences above are the deliberate ones. Everything else in
// ToolDeps must simply BE there — and until this test existed, one of them
// was not: ToolDeps.Memory was never assigned in mcp_enabled.go, so
// linetta_edit_memory answered "memory is unavailable in this build" for the
// external host and for the built-in agent alike, while the system prompt
// told the agent to record what it learns with that tool every single turn.
// Nothing caught it, because every other test builds its own ToolDeps
// literal carrying exactly the fields that test needs.
//
// So this asserts the shape rather than the one field: a large struct
// literal where a forgotten field is silently a nil, and the nil surfaces
// only as a runtime tool error, needs a guard that catches the NEXT
// forgotten field too. Reflection walks every nilable field, so a
// collaborator added to ToolDeps and not wired here fails this test instead
// of shipping dead.
//
// Fields whose zero value is correct in production are named in
// skipToolDep, with the reason — a short list that has to be argued for,
// which is the point of keeping it explicit.
func TestProductionToolDepsCarryEveryCollaborator(t *testing.T) {
	app := openApp(t)

	if app.agentCtrl == nil {
		t.Fatal("app.agentCtrl is nil — setupAgent did not run")
	}
	assertToolDepsWired(t, "external host", app.mcpTools)
	assertToolDepsWired(t, "built-in agent", app.agentCtrl.tools)

	// Memory in particular must be the SAME repo on both, not two instances.
	// The two documents are one row each and the settings pane edits those
	// same rows, so a second repo here would still work — the repo is
	// stateless — but it would mean someone built one, which is the drift
	// this whole test exists to catch.
	if app.mcpTools.Memory != app.agentCtrl.tools.Memory {
		t.Error("agent and external tool deps hold different memory repos — the curated documents have exactly one home")
	}
}

// skipToolDep explains why a ToolDeps field may be left at its zero value in
// production, or returns "" when the field must be wired.
func skipToolDep(name string) string {
	if name == "Source" {
		// Empty means external — that is how sourceOrExternal reads it, and
		// TestAgentToolDepsDifferFromTheExternalHosts asserts both sides of
		// it directly. It is a string besides, so it is never nil.
		return "empty means the external host"
	}
	return ""
}

func assertToolDepsWired(t *testing.T, label string, deps mcphost.ToolDeps) {
	t.Helper()
	v := reflect.ValueOf(deps)
	for i := 0; i < v.NumField(); i++ {
		name := v.Type().Field(i).Name
		if skipToolDep(name) != "" {
			continue
		}
		switch f := v.Field(i); f.Kind() {
		case reflect.Ptr, reflect.Func, reflect.Interface, reflect.Map, reflect.Slice:
			if f.IsNil() {
				t.Errorf("%s tool deps: ToolDeps.%s is nil — the tools that read it refuse at runtime, and nothing else notices",
					label, name)
			}
		}
	}
}
