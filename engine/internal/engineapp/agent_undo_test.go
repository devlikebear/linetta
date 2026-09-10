//go:build !mobile

package engineapp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/agenttest"
)

// agent.undo must tell the UI the outline changed.
//
// mcp.changed is the ONLY signal the workspace refreshes its outline from
// (useMcpChanges -> refreshOutlineFromEngine). Every other mutation path emits
// one, including the equivalent MCP tool (mcphost's linetta_undo_last_change).
// Without it here, the panel's undo button reverts the database and leaves the
// sidebar listing the chapters and scenes the batch created — a tree the
// writer can click into and get node_not_found from, immediately after their
// own action reported success.
func TestAgentUndo_tellsTheWorkspaceTheOutlineChanged(t *testing.T) {
	app := openApp(t)
	projectID, nodeID := seedProjectWithScene(t, app)

	consent := `{"provider":"anthropic","providers":{"anthropic":{"api_key":"sk-test","consented_at":1700000000000}}}`
	if _, rpcErr := call(t, app, "settings.set", consent); rpcErr != nil {
		t.Fatalf("settings.set: %+v", rpcErr)
	}

	// linetta_apply_story_ops is the one tool that returns an undo_batch_id,
	// so it is the only way to get a batch the panel's button could revert.
	app.SetProviderFactoryForTest(agenttest.NewScriptedClientFactory(
		agenttest.ScriptedTurn{ToolName: "linetta_apply_story_ops",
			ToolArgs: `{"project_id":"` + projectID + `","node_id":"` + nodeID + `","summary":"5장 추가",` +
				`"ops":[{"op":"create_scene","ref":"s5","after_node_id":"` + nodeID + `","label":"5장","title":"기록보관실"}]}`},
		agenttest.ScriptedTurn{Text: "5장을 추가했습니다."},
	))

	runLog := app.CaptureNotificationsForTest()
	if _, rpcErr := call(t, app, "agent.run",
		`{"project_id":"`+projectID+`","node_id":"`+nodeID+`","prompt":"5장을 추가해줘"}`); rpcErr != nil {
		t.Fatalf("agent.run: %+v", rpcErr)
	}
	waitForNotification(t, runLog, "agent.done")

	// The batch id the panel would render its 되돌리기 button from: it reaches
	// the panel only on agent.tool's resolving event.
	var tool struct {
		Name    string `json:"name"`
		State   string `json:"state"`
		BatchID string `json:"batch_id"`
	}
	if err := json.Unmarshal(runLog.paramsFor("agent.tool"), &tool); err != nil {
		t.Fatalf("decode agent.tool: %v", err)
	}
	if tool.BatchID == "" {
		t.Fatalf("agent.tool %s/%s carried no batch_id; there is nothing for undo to revert",
			tool.Name, tool.State)
	}
	before := outlineSize(t, app, projectID)

	// A fresh log, so the assertion below cannot pass on the mcp.changed the
	// apply itself emitted a moment ago.
	undoLog := app.CaptureNotificationsForTest()
	if _, rpcErr := call(t, app, "agent.undo", `{"batch_id":"`+tool.BatchID+`"}`); rpcErr != nil {
		t.Fatalf("agent.undo: %+v", rpcErr)
	}

	// The revert really happened...
	after := outlineSize(t, app, projectID)
	if after >= before {
		t.Fatalf("outline did not shrink: %d -> %d; the batch was not reverted", before, after)
	}

	// ...and the UI was told, or it goes on showing the nodes just deleted.
	if !undoLog.saw("mcp.changed") {
		t.Fatal("agent.undo emitted no mcp.changed; the sidebar would keep listing the reverted nodes")
	}
	var changed struct {
		Tool    string `json:"tool"`
		BatchID string `json:"batch_id"`
		Source  string `json:"source"`
	}
	if err := json.Unmarshal(undoLog.paramsFor("mcp.changed"), &changed); err != nil {
		t.Fatalf("decode mcp.changed: %v", err)
	}
	if changed.Tool != "linetta_undo_last_change" {
		t.Errorf("mcp.changed tool = %q, want linetta_undo_last_change", changed.Tool)
	}
	if changed.BatchID != tool.BatchID {
		t.Errorf("mcp.changed batch_id = %q, want %q", changed.BatchID, tool.BatchID)
	}
	// Same reason the write path carries it: the workspace must not tell the
	// writer "an external agent changed this" about their own undo click.
	if changed.Source != "agent" {
		t.Errorf("mcp.changed source = %q, want agent", changed.Source)
	}
}

// A failed undo must not claim the outline changed — a refresh is harmless,
// but a conflict banner naming the agent for a revert that never happened is
// not.
func TestAgentUndo_saysNothingChangedWhenTheBatchIsGone(t *testing.T) {
	app := openApp(t)
	log := app.CaptureNotificationsForTest()

	_, rpcErr := call(t, app, "agent.undo", `{"batch_id":"no-such-batch"}`)
	if rpcErr == nil {
		t.Fatal("agent.undo accepted a batch id that was never applied")
	}
	if log.saw("mcp.changed") {
		t.Fatalf("a refused undo emitted mcp.changed: %s", log.paramsFor("mcp.changed"))
	}
}

// The scenario #112 exists for: a scene write, the most common agent action,
// must be revertible from the tool line the same way an outline batch is —
// through snapshot_id rather than batch_id. A successful revert must still
// tell the workspace the scene changed, or the open editor keeps showing the
// agent's prose after the writer's own undo reported success.
func TestAgentUndo_withASnapshotID_tellsTheWorkspaceTheSceneChanged(t *testing.T) {
	app := openApp(t)
	projectID, nodeID := seedProjectWithScene(t, app)
	before := nodeContent(t, app, nodeID)

	consent := `{"provider":"anthropic","providers":{"anthropic":{"api_key":"sk-test","consented_at":1700000000000}}}`
	if _, rpcErr := call(t, app, "settings.set", consent); rpcErr != nil {
		t.Fatalf("settings.set: %+v", rpcErr)
	}

	app.SetProviderFactoryForTest(agenttest.NewScriptedClientFactory(
		agenttest.ScriptedTurn{ToolName: "linetta_write_scene",
			ToolArgs: `{"node_id":"` + nodeID + `","text":"새로 쓴 문장.","expected_content_version":0}`},
		agenttest.ScriptedTurn{Text: "1장을 다시 썼습니다."},
	))

	runLog := app.CaptureNotificationsForTest()
	if _, rpcErr := call(t, app, "agent.run",
		`{"project_id":"`+projectID+`","node_id":"`+nodeID+`","prompt":"1장을 다시 써줘"}`); rpcErr != nil {
		t.Fatalf("agent.run: %+v", rpcErr)
	}
	waitForNotification(t, runLog, "agent.done")

	// The snapshot id the panel would render its 되돌리기 button from: it
	// reaches the panel only on agent.tool's resolving event, exactly like
	// batch_id does for an outline batch.
	var tool struct {
		Name       string `json:"name"`
		State      string `json:"state"`
		SnapshotID string `json:"snapshot_id"`
	}
	if err := json.Unmarshal(runLog.paramsFor("agent.tool"), &tool); err != nil {
		t.Fatalf("decode agent.tool: %v", err)
	}
	if tool.SnapshotID == "" {
		t.Fatalf("agent.tool %s/%s carried no snapshot_id; there is nothing for undo to revert",
			tool.Name, tool.State)
	}
	if got := nodeContent(t, app, nodeID); !strings.Contains(got, "새로 쓴 문장") {
		t.Fatalf("scene was not written: %s", got)
	}

	// A fresh log, so the assertion below cannot pass on the mcp.changed the
	// write itself emitted a moment ago.
	undoLog := app.CaptureNotificationsForTest()
	if _, rpcErr := call(t, app, "agent.undo", `{"snapshot_id":"`+tool.SnapshotID+`"}`); rpcErr != nil {
		t.Fatalf("agent.undo: %+v", rpcErr)
	}

	// The revert really happened...
	if got := nodeContent(t, app, nodeID); got != before {
		t.Fatalf("scene was not restored: got %q, want %q", got, before)
	}

	// ...and the UI was told, or the open editor keeps showing the agent's
	// prose after the writer's own undo reported success.
	if !undoLog.saw("mcp.changed") {
		t.Fatal("agent.undo emitted no mcp.changed; the editor would keep showing the reverted text")
	}
	var changed struct {
		Tool      string   `json:"tool"`
		ProjectID string   `json:"project_id"`
		NodeIDs   []string `json:"node_ids"`
		Source    string   `json:"source"`
	}
	if err := json.Unmarshal(undoLog.paramsFor("mcp.changed"), &changed); err != nil {
		t.Fatalf("decode mcp.changed: %v", err)
	}
	if changed.Tool != "linetta_undo_last_change" {
		t.Errorf("mcp.changed tool = %q, want linetta_undo_last_change", changed.Tool)
	}
	if changed.ProjectID != projectID {
		t.Errorf("mcp.changed project_id = %q, want %q", changed.ProjectID, projectID)
	}
	if len(changed.NodeIDs) != 1 || changed.NodeIDs[0] != nodeID {
		t.Errorf("mcp.changed node_ids = %v, want [%q]", changed.NodeIDs, nodeID)
	}
	if changed.Source != "agent" {
		t.Errorf("mcp.changed source = %q, want agent", changed.Source)
	}
}

// A snapshot the writer's own cleanup (or an id typed by hand) has already
// dropped is the same ordinary case a stale batch id is: refused with
// agent_undo_unavailable, not a raw error, and mcp.changed must stay silent.
func TestAgentUndo_saysNothingChangedWhenTheSnapshotIsGone(t *testing.T) {
	app := openApp(t)
	log := app.CaptureNotificationsForTest()

	_, rpcErr := call(t, app, "agent.undo", `{"snapshot_id":"no-such-snapshot"}`)
	if rpcErr == nil {
		t.Fatal("agent.undo accepted a snapshot id that does not exist")
	}
	if rpcErr.Data == nil || !strings.Contains(string(rpcErr.Data), "agent_undo_unavailable") {
		t.Errorf("rpc error data = %s, want it to carry agent_undo_unavailable", rpcErr.Data)
	}
	if log.saw("mcp.changed") {
		t.Fatalf("a refused undo emitted mcp.changed: %s", log.paramsFor("mcp.changed"))
	}
}

// outlineSize counts the work's outline nodes.
func outlineSize(t *testing.T, app *App, projectID string) int {
	t.Helper()
	out, rpcErr := call(t, app, "nodes.list_tree", `{"project_id":"`+projectID+`"}`)
	if rpcErr != nil {
		t.Fatalf("nodes.list_tree: %+v", rpcErr)
	}
	var nodes []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &nodes); err != nil {
		t.Fatalf("decode tree: %v", err)
	}
	return len(nodes)
}
