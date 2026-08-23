//go:build !mobile

package engineapp

import (
	"encoding/json"
	"strings"
	"testing"
)

// seedScene writes prose through the MCP write path and returns the scene id.
func seedScene(t *testing.T, c *mcpClient, nodeID, text string) {
	t.Helper()
	_, v := readScene(t, c, nodeID)
	if r := c.callTool("linetta_write_scene", map[string]any{
		"node_id": nodeID, "text": text, "expected_content_version": v,
	}); isToolError(r) {
		t.Fatalf("seed write: %v", r)
	}
}

// A rename is the case this tool exists for: change a name everywhere without
// resending whole scene bodies.
func TestMCPReviseSceneReplacesAcrossScenes(t *testing.T) {
	_, c, projectID, nodeID := startWritableMCP(t)
	seedScene(t, c, nodeID, "서린은 골목에 서 있었다. 서린의 그림자는 없었다.")

	result := c.callTool("linetta_revise_scene", map[string]any{
		"project_id": projectID, "find": "서린", "replace": "지한",
	})
	if isToolError(result) {
		t.Fatalf("revise_scene: %v", result)
	}
	var out struct {
		Applied      int      `json:"applied"`
		ChangedNodes []string `json:"changed_nodes"`
		Matches      []struct {
			Occurrences int `json:"occurrences"`
		} `json:"matches"`
	}
	if err := json.Unmarshal([]byte(structuredJSON(t, result)), &out); err != nil {
		t.Fatalf("decode revise: %v", err)
	}
	if out.Applied != 1 || len(out.ChangedNodes) != 1 {
		t.Fatalf("result = %+v, want one scene revised", out)
	}
	if len(out.Matches) == 0 || out.Matches[0].Occurrences < 2 {
		t.Errorf("both occurrences should be reported: %+v", out.Matches)
	}

	body, _ := readScene(t, c, nodeID)
	if strings.Contains(body, "서린") {
		t.Fatalf("the old name survived: %q", body)
	}
	if !strings.Contains(body, "지한") {
		t.Fatalf("the new name is missing: %q", body)
	}
}

// dry_run is how an agent checks the blast radius of a common phrase before
// committing to it.
func TestMCPReviseSceneDryRunChangesNothing(t *testing.T) {
	_, c, projectID, nodeID := startWritableMCP(t)
	const original = "그는 문을 열었다. 문은 잠겨 있지 않았다."
	seedScene(t, c, nodeID, original)

	result := c.callTool("linetta_revise_scene", map[string]any{
		"project_id": projectID, "find": "문", "replace": "창", "dry_run": true,
	})
	if isToolError(result) {
		t.Fatalf("dry run: %v", result)
	}
	var out struct {
		Applied int  `json:"applied"`
		DryRun  bool `json:"dry_run"`
		Matches []struct {
			After string `json:"after"`
		} `json:"matches"`
	}
	if err := json.Unmarshal([]byte(structuredJSON(t, result)), &out); err != nil {
		t.Fatalf("decode dry run: %v", err)
	}
	if !out.DryRun || out.Applied != 0 {
		t.Fatalf("result = %+v, want a preview that applied nothing", out)
	}
	if len(out.Matches) == 0 || out.Matches[0].After == "" {
		t.Error("a dry run must show what the text would become")
	}

	body, _ := readScene(t, c, nodeID)
	if body != original {
		t.Fatalf("dry run changed the scene:\n got %q\nwant %q", body, original)
	}
}

func TestMCPReviseSceneReportsNoMatch(t *testing.T) {
	_, c, projectID, nodeID := startWritableMCP(t)
	seedScene(t, c, nodeID, "짧은 본문.")

	result := c.callTool("linetta_revise_scene", map[string]any{
		"project_id": projectID, "find": "존재하지않는문구", "replace": "무엇이든",
	})
	if !isToolError(result) {
		t.Fatal("a revision with no match must report an error")
	}
	if msg := toolErrorText(result); !strings.Contains(msg, "linetta_search_manuscript") {
		t.Errorf("the message should point at the search tool: %s", msg)
	}
}

// The scene is snapshotted before a revision, so the writer can restore it.
func TestMCPReviseSceneSnapshots(t *testing.T) {
	app, c, projectID, nodeID := startWritableMCP(t)
	seedScene(t, c, nodeID, "고쳐질 문장이 여기 있다.")

	if r := c.callTool("linetta_revise_scene", map[string]any{
		"project_id": projectID, "find": "고쳐질", "replace": "고쳐진",
	}); isToolError(r) {
		t.Fatalf("revise: %v", r)
	}

	raw, rpcErr := call(t, app, "snapshots.list_for_node", `{"node_id":"`+nodeID+`"}`)
	if rpcErr != nil {
		t.Fatalf("snapshots.list_for_node: %+v", rpcErr)
	}
	var entries []struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("decode snapshots: %v", err)
	}
	if len(entries) < 2 {
		t.Fatalf("expected a snapshot from the write and one from the revision, got %d", len(entries))
	}
}
