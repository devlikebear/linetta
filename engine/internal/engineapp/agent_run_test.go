//go:build !mobile

package engineapp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/agent"
)

// The full loop against the real tool layer: the agent reads the work, writes
// a scene, and everything the writer relies on afterwards is in place.
//
// The scripted model itself (agent.ScriptedTurn / agent.NewScriptedClientFactoryForTest)
// lives in internal/agent, not here: scripts/validate-story-core-deps.sh
// confines tars/pkg/llm to internal/provider and internal/agent, and this
// package must not import it at all — not even from a _test.go file, since Go
// does not let one package's tests import another package's test-only
// declarations. The scripted model is the only fake in this test — the MCP
// server, the tools and the database are real.
func TestAgentRun_writesThroughTheRealMCPToolsAndIsAudited(t *testing.T) {
	app := openApp(t)

	// A real work with a real scene.
	projectID, nodeID := seedProjectWithScene(t, app)

	consent := `{"provider":"anthropic","providers":{"anthropic":{"api_key":"sk-test","consented_at":1700000000000}}}`
	if _, rpcErr := call(t, app, "settings.set", consent); rpcErr != nil {
		t.Fatalf("settings.set: %+v", rpcErr)
	}

	// The scenario from the issue: read the context, write the scene, refresh
	// the summary. A scripted model cannot read a tool's reply, so the
	// versions are hard-coded — a fresh scene is at content_version 0, and
	// the write above bumps it to 1. If write_scene ever stops bumping the
	// version, this test is what notices.
	factory := agent.NewScriptedClientFactoryForTest(
		agent.ScriptedTurn{ToolName: "linetta_get_story_context",
			ToolArgs: `{"project_id":"` + projectID + `","node_id":"` + nodeID + `"}`},
		agent.ScriptedTurn{ToolName: "linetta_write_scene",
			ToolArgs: `{"node_id":"` + nodeID + `","text":"비가 내렸다. 그는 우산을 펴지 않았다.","expected_content_version":0}`},
		agent.ScriptedTurn{ToolName: "linetta_write_summary",
			ToolArgs: `{"node_id":"` + nodeID + `","summary":"비 오는 날 우산을 펴지 않는 남자가 등장한다.","expected_content_version":1}`},
		agent.ScriptedTurn{Text: "1장을 썼습니다."},
	)
	app.SetProviderFactoryForTest(factory)

	changed := app.CaptureNotificationsForTest()

	out, rpcErr := call(t, app, "agent.run",
		`{"project_id":"`+projectID+`","node_id":"`+nodeID+`","prompt":"1장 초고를 써줘"}`)
	if rpcErr != nil {
		t.Fatalf("agent.run: %+v", rpcErr)
	}
	var run struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(out, &run); err != nil {
		t.Fatalf("decode run: %v", err)
	}
	if run.RunID == "" {
		t.Fatal("agent.run returned no run id")
	}

	waitForNotification(t, changed, "agent.done")

	// 1. The manuscript really changed.
	body := nodeContent(t, app, nodeID)
	if !strings.Contains(body, "비가 내렸다") {
		t.Errorf("the scene was not written: %q", body)
	}

	// 2. The activity log knows who did it and which turn it belonged to.
	actRaw, rpcErr := call(t, app, "mcp.activity", `{"limit":50}`)
	if rpcErr != nil {
		t.Fatalf("mcp.activity: %+v", rpcErr)
	}
	var entries []struct {
		Tool   string `json:"tool"`
		Source string `json:"source"`
		RunID  string `json:"run_id"`
	}
	if err := json.Unmarshal(actRaw, &entries); err != nil {
		t.Fatalf("decode activity: %v", err)
	}
	var wrote bool
	for _, e := range entries {
		if e.Tool != "linetta_write_scene" {
			continue
		}
		wrote = true
		if e.Source != "agent" {
			t.Errorf("activity source = %q, want agent", e.Source)
		}
		if e.RunID != run.RunID {
			t.Errorf("activity run_id = %q, want %q", e.RunID, run.RunID)
		}
	}
	if !wrote {
		t.Error("the write never reached the activity log")
	}

	// 3. The UI refresh path fired — the agent's writes reuse mcp.changed
	//    rather than adding a second refresh channel.
	if !changed.saw("mcp.changed") {
		t.Error("no mcp.changed; the workspace would show stale text")
	}

	// 4. The panel can restore the turn.
	histRaw, rpcErr := call(t, app, "agent.history", `{"project_id":"`+projectID+`","limit":50}`)
	if rpcErr != nil {
		t.Fatalf("agent.history: %+v", rpcErr)
	}
	var hist []struct {
		Role  string `json:"role"`
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(histRaw, &hist); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	roles := map[string]int{}
	for _, m := range hist {
		roles[m.Role]++
	}
	if roles["user"] != 1 || roles["tool"] != 3 || roles["assistant"] < 1 {
		t.Errorf("transcript roles = %v, want one prompt, three tool events and a reply", roles)
	}
}

// Without consent nothing is constructed that could send a byte.
func TestAgentRun_refusesWithoutProviderConsent(t *testing.T) {
	app := openApp(t)
	projectID, _ := seedProjectWithScene(t, app)
	noConsent := `{"provider":"anthropic","providers":{"anthropic":{"api_key":"sk-test"}}}`
	if _, rpcErr := call(t, app, "settings.set", noConsent); rpcErr != nil {
		t.Fatalf("settings.set: %+v", rpcErr)
	}
	app.SetProviderFactoryForTest(agent.NewFailingClientFactoryForTest(t.Fatal))
	_, rpcErr := call(t, app, "agent.run", `{"project_id":"`+projectID+`","prompt":"써줘"}`)
	if rpcErr == nil {
		t.Fatal("agent.run without consent must fail")
	}
	if !strings.Contains(string(rpcErr.Data), "provider_consent_required") {
		t.Errorf("error data = %s, want a provider_consent_required reason", rpcErr.Data)
	}
}

// External MCP being off must not disable the panel: the built-in agent is a
// client of a second, in-memory server that never binds a port.
func TestAgentRun_worksWhileExternalMCPIsOff(t *testing.T) {
	app := openApp(t)
	projectID, nodeID := seedProjectWithScene(t, app)

	st, rpcErr := call(t, app, "mcp.status", "")
	if rpcErr != nil {
		t.Fatalf("mcp.status: %+v", rpcErr)
	}
	var status mcpStatus
	if err := json.Unmarshal(st, &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status.Running {
		t.Fatal("this test needs external MCP off")
	}

	consent := `{"provider":"anthropic","providers":{"anthropic":{"api_key":"sk-test","consented_at":1700000000000}}}`
	if _, rpcErr := call(t, app, "settings.set", consent); rpcErr != nil {
		t.Fatalf("settings.set: %+v", rpcErr)
	}
	factory := agent.NewScriptedClientFactoryForTest(
		agent.ScriptedTurn{ToolName: "linetta_read_scene", ToolArgs: `{"node_id":"` + nodeID + `"}`},
		agent.ScriptedTurn{Text: "읽었습니다."},
	)
	app.SetProviderFactoryForTest(factory)
	changed := app.CaptureNotificationsForTest()

	if _, rpcErr := call(t, app, "agent.run", `{"project_id":"`+projectID+`","prompt":"읽어줘"}`); rpcErr != nil {
		t.Fatalf("agent.run: %+v", rpcErr)
	}
	waitForNotification(t, changed, "agent.done")
}

// seedProjectWithScene creates a work and returns it with its first scene.
// projects.create seeds an outline and reports the opened scene directly, so
// there is no tree to walk — mcp_write_test.go's fixture does the same.
func seedProjectWithScene(t *testing.T, app *App) (string, string) {
	t.Helper()
	out, rpcErr := call(t, app, "projects.create",
		`{"title":"테스트 작품","genres":["fantasy"],"length_target":"short","default_pov":"first"}`)
	if rpcErr != nil {
		t.Fatalf("projects.create: %+v", rpcErr)
	}
	var proj struct {
		ID               string  `json:"id"`
		LastOpenedNodeID *string `json:"last_opened_node_id"`
	}
	if err := json.Unmarshal(out, &proj); err != nil {
		t.Fatalf("decode project: %v", err)
	}
	if proj.LastOpenedNodeID == nil {
		t.Fatal("projects.create seeded no scene to write into")
	}
	return proj.ID, *proj.LastOpenedNodeID
}

// nodeContent returns a scene's stored body. content_doc is the editor's own
// JSON document, so the prose is asserted with Contains rather than equality.
func nodeContent(t *testing.T, app *App, nodeID string) string {
	t.Helper()
	out, rpcErr := call(t, app, "nodes.get", `{"id":"`+nodeID+`"}`)
	if rpcErr != nil {
		t.Fatalf("nodes.get: %+v", rpcErr)
	}
	var n struct {
		ContentDoc *string `json:"content_doc"`
	}
	if err := json.Unmarshal(out, &n); err != nil {
		t.Fatalf("decode node: %v", err)
	}
	if n.ContentDoc == nil {
		return ""
	}
	return *n.ContentDoc
}

func waitForNotification(t *testing.T, log *notificationLog, method string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !log.saw(method) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", method)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
