//go:build !mobile

package engineapp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/devlikebear/linetta/engine/internal/agenttest"
)

// The full loop against the real tool layer: the agent reads the work, writes
// a scene, and everything the writer relies on afterwards is in place.
//
// The scripted model itself (agenttest.ScriptedTurn / agenttest.NewScriptedClientFactory)
// lives in internal/agenttest, not here: scripts/validate-story-core-deps.sh
// confines tars/pkg/llm to internal/provider, internal/agent and
// internal/agenttest, and this package must not import it at all — not even
// from a _test.go file, since Go does not let one package's tests import
// another package's test-only declarations. The scripted model is the only
// fake in this test — the MCP server, the tools and the database are real.
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
	// the write above bumps it to 1. This is the canary for that bump: if
	// write_scene ever stops advancing content_version, linetta_write_summary
	// below is called against a version that no longer matches (it expects
	// 1; the still-0 scene refuses it), so its activity row below turns up
	// present but not ok, and this test fails.
	factory := agenttest.NewScriptedClientFactory(
		agenttest.ScriptedTurn{ToolName: "linetta_get_story_context",
			ToolArgs: `{"project_id":"` + projectID + `","node_id":"` + nodeID + `"}`},
		agenttest.ScriptedTurn{ToolName: "linetta_write_scene",
			ToolArgs: `{"node_id":"` + nodeID + `","text":"비가 내렸다. 그는 우산을 펴지 않았다.","expected_content_version":0}`},
		agenttest.ScriptedTurn{ToolName: "linetta_write_summary",
			ToolArgs: `{"node_id":"` + nodeID + `","summary":"비 오는 날 우산을 펴지 않는 남자가 등장한다.","expected_content_version":1}`},
		agenttest.ScriptedTurn{Text: "1장을 썼습니다."},
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

	// 2. The activity log knows who did it, which turn it belonged to, and
	//    that EACH of the three scripted calls actually succeeded — not just
	//    that a row with the right tool name exists. A row can be present and
	//    still record a refused call (a stale expected_content_version, a
	//    broken tool), so "found" alone is not enough; every one of these
	//    must also read ok:true.
	actRaw, rpcErr := call(t, app, "mcp.activity", `{"limit":50}`)
	if rpcErr != nil {
		t.Fatalf("mcp.activity: %+v", rpcErr)
	}
	var entries []struct {
		Tool   string `json:"tool"`
		Source string `json:"source"`
		RunID  string `json:"run_id"`
		OK     bool   `json:"ok"`
	}
	if err := json.Unmarshal(actRaw, &entries); err != nil {
		t.Fatalf("decode activity: %v", err)
	}
	var readContext, wroteScene, wroteSummary bool
	for _, e := range entries {
		switch e.Tool {
		case "linetta_get_story_context":
			readContext = true
			if !e.OK {
				t.Error("linetta_get_story_context activity row is not ok")
			}
		case "linetta_write_scene":
			wroteScene = true
			if e.Source != "agent" {
				t.Errorf("activity source = %q, want agent", e.Source)
			}
			if e.RunID != run.RunID {
				t.Errorf("activity run_id = %q, want %q", e.RunID, run.RunID)
			}
			if !e.OK {
				t.Error("linetta_write_scene activity row is not ok")
			}
		case "linetta_write_summary":
			wroteSummary = true
			if !e.OK {
				t.Error("linetta_write_summary activity row is not ok — this is the content_version canary: " +
					"a write_scene that stopped bumping the version would leave this call's " +
					"expected_content_version stale and refused")
			}
		}
	}
	if !readContext {
		t.Error("linetta_get_story_context never reached the activity log")
	}
	if !wroteScene {
		t.Error("the write never reached the activity log")
	}
	if !wroteSummary {
		t.Error("linetta_write_summary never reached the activity log")
	}

	// 3. The UI refresh path fired — the agent's writes reuse mcp.changed
	//    rather than adding a second refresh channel — and it says WHO wrote,
	//    so the workspace does not tell the writer that "an external agent
	//    changed this scene" about a revision they asked their own panel for.
	//    Like the activity log's source, it comes from the composed ToolDeps
	//    and cannot be claimed off the wire by an external client.
	if !changed.saw("mcp.changed") {
		t.Fatal("no mcp.changed; the workspace would show stale text")
	}
	var mcpChanged struct {
		Source  string   `json:"source"`
		NodeIDs []string `json:"node_ids"`
	}
	if err := json.Unmarshal(changed.paramsFor("mcp.changed"), &mcpChanged); err != nil {
		t.Fatalf("decode mcp.changed: %v", err)
	}
	if mcpChanged.Source != "agent" {
		t.Errorf("mcp.changed source = %q, want agent", mcpChanged.Source)
	}

	// 4. The panel can restore the turn, and every row of it belongs to this
	//    run — the linkage the panel groups a turn by.
	histRaw, rpcErr := call(t, app, "agent.history", `{"project_id":"`+projectID+`","limit":50}`)
	if rpcErr != nil {
		t.Fatalf("agent.history: %+v", rpcErr)
	}
	var hist []struct {
		Role   string `json:"role"`
		RunID  string `json:"run_id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(histRaw, &hist); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	roles := map[string]int{}
	for _, m := range hist {
		roles[m.Role]++
		if m.RunID != run.RunID {
			t.Errorf("history row role=%s run_id = %q, want %q", m.Role, m.RunID, run.RunID)
		}
		// status reaches the panel over the wire, and it is what decides
		// whether a turn gets a retry button. A turn that ran to completion
		// carries "done" on every row.
		if m.Status != "done" {
			t.Errorf("history row role=%s status = %q, want done", m.Role, m.Status)
		}
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
	app.SetProviderFactoryForTest(agenttest.NewFailingClientFactory(t.Fatal))
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

	// A known, distinctive label to read back, so "the tool actually
	// returned the seeded scene" can be checked against real content rather
	// than just an ok flag. This uses the node's LABEL, not its body text:
	// linetta_read_scene's JSON reply is marshalled from a map (its keys come
	// back alphabetised — content_version, label, node_id, project_id,
	// status, summary_is_stale, text, word_count — not struct declaration
	// order), and the transcript only keeps the first 160 runes of it
	// (agent/loop.go's summarize). With two real UUIDs (node_id, project_id)
	// ahead of it, "text" itself never survives that truncation — but
	// "label" is the second key and always does, so it is what a genuine
	// round trip can actually be checked against here.
	const seededLabel = "표식-장면-9F2"
	if _, rpcErr := call(t, app, "nodes.rename",
		`{"id":"`+nodeID+`","label":"`+seededLabel+`"}`); rpcErr != nil {
		t.Fatalf("nodes.rename: %+v", rpcErr)
	}

	consent := `{"provider":"anthropic","providers":{"anthropic":{"api_key":"sk-test","consented_at":1700000000000}}}`
	if _, rpcErr := call(t, app, "settings.set", consent); rpcErr != nil {
		t.Fatalf("settings.set: %+v", rpcErr)
	}
	factory := agenttest.NewScriptedClientFactory(
		agenttest.ScriptedTurn{ToolName: "linetta_read_scene", ToolArgs: `{"node_id":"` + nodeID + `"}`},
		agenttest.ScriptedTurn{Text: "읽었습니다."},
	)
	app.SetProviderFactoryForTest(factory)
	changed := app.CaptureNotificationsForTest()

	out, rpcErr := call(t, app, "agent.run", `{"project_id":"`+projectID+`","prompt":"읽어줘"}`)
	if rpcErr != nil {
		t.Fatalf("agent.run: %+v", rpcErr)
	}
	var run struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(out, &run); err != nil {
		t.Fatalf("decode run: %v", err)
	}
	waitForNotification(t, changed, "agent.done")

	// The tool call really reached the second, in-memory server and read
	// real content back — not just that some call named "linetta_read_scene"
	// was attempted. A server registered with zero tools (e.g. the agent's
	// tool registration accidentally gated behind settings.MCPMode, the exact
	// mistake agent_enabled.go's "Always full" comment warns against) would
	// fail this call; the model would still say it's done next turn, so
	// waiting on agent.done alone would not have noticed.
	histRaw, rpcErr := call(t, app, "agent.history", `{"project_id":"`+projectID+`","limit":50}`)
	if rpcErr != nil {
		t.Fatalf("agent.history: %+v", rpcErr)
	}
	var hist []struct {
		Role    string `json:"role"`
		RunID   string `json:"run_id"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(histRaw, &hist); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	var sawRead bool
	for _, m := range hist {
		if m.Role != "tool" || m.RunID != run.RunID {
			continue
		}
		var ev struct {
			Name    string `json:"name"`
			Summary string `json:"summary"`
			OK      bool   `json:"ok"`
		}
		if err := json.Unmarshal([]byte(m.Content), &ev); err != nil {
			t.Fatalf("decode tool event: %v", err)
		}
		if ev.Name != "linetta_read_scene" {
			continue
		}
		sawRead = true
		if !ev.OK {
			t.Errorf("linetta_read_scene did not succeed: %s", ev.Summary)
		}
		if !strings.Contains(ev.Summary, `"label":"`+seededLabel+`"`) {
			t.Errorf("linetta_read_scene did not return the seeded scene: %s", ev.Summary)
		}
	}
	if !sawRead {
		t.Fatal("linetta_read_scene never reached the transcript")
	}

	// External MCP is still off: the built-in agent never touched the HTTP
	// host or a port.
	st2, rpcErr := call(t, app, "mcp.status", "")
	if rpcErr != nil {
		t.Fatalf("mcp.status: %+v", rpcErr)
	}
	var status2 mcpStatus
	if err := json.Unmarshal(st2, &status2); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status2.Running {
		t.Error("mcp.status.running = true after an agent run; external MCP must stay off")
	}
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

// waitForNotification waits for method, but also fails fast — with the
// reason and message it carried — if the turn ends in agent.error instead:
// an opaque 10s timeout would otherwise be the only symptom of a broken loop.
func waitForNotification(t *testing.T, log *notificationLog, method string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if log.saw(method) {
			return
		}
		if method != "agent.error" {
			if params := log.paramsFor("agent.error"); params != nil {
				t.Fatalf("waiting for %s: turn ended in agent.error instead: %s", method, params)
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", method)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
