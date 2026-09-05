//go:build !mobile

package engineapp

import (
	"encoding/json"
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
