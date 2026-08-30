//go:build !mobile

package engineapp

import (
	"encoding/json"
	"strings"
	"testing"
)

// #73: the agent's only sources of truth are the schemas and the error
// messages. These tests pin the ergonomics an agent needs to make correct
// calls without reading Linetta's source.

// The read→edit→write round trip must survive an agent copying field names
// from one tool's output into the next tool's input: read_scene speaks
// "text", exactly like linetta_write_scene's input and set_scene_text.
func TestMCPReadSceneReturnsTextNotBody(t *testing.T) {
	_, c, _, nodeID := startWritableMCP(t)
	if r := c.callTool("linetta_write_scene", map[string]any{
		"node_id": nodeID, "text": "첫 문장.", "expected_content_version": 0,
	}); isToolError(r) {
		t.Fatalf("seed write: %v", r)
	}

	result := c.callTool("linetta_read_scene", map[string]any{"node_id": nodeID})
	if isToolError(result) {
		t.Fatalf("read_scene: %v", result)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(structuredJSON(t, result)), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := out["text"]; !ok {
		t.Error("read_scene output has no \"text\" field")
	}
	if _, ok := out["body"]; ok {
		t.Error("read_scene still emits \"body\"; the write side asks for \"text\"")
	}
}

// project_id is the argument agents habitually attach because most tools want
// one. Node-scoped tools must accept it — and refuse a mismatch loudly, since
// a quietly ignored wrong work becomes a wrong-work edit later.
func TestMCPNodeToolsAcceptOptionalProjectID(t *testing.T) {
	_, c, projectID, nodeID := startWritableMCP(t)

	for _, tool := range []string{"linetta_read_scene", "linetta_get_plot", "linetta_get_story_context"} {
		right := c.callTool(tool, map[string]any{"node_id": nodeID, "project_id": projectID})
		if isToolError(right) {
			t.Errorf("%s refused a matching project_id: %v", tool, right)
		}
		wrong := c.callTool(tool, map[string]any{"node_id": nodeID, "project_id": "some-other-work"})
		if !isToolError(wrong) {
			t.Errorf("%s accepted a mismatched project_id silently", tool)
		} else if msg := errorText(wrong); !strings.Contains(msg, projectID) {
			t.Errorf("%s mismatch error should name the real work: %s", tool, msg)
		}
	}
}

// An agent that guesses an op name must be taught the real names by the
// error, and must be able to read them from the schema in the first place.
func TestMCPApplyStoryOpsTeachesTheOpCatalogue(t *testing.T) {
	_, c, projectID, _ := startWritableMCP(t)

	// The schema advertises the op names — on the op field itself, not
	// merely somewhere in the listing. The SDK dropping a nested struct's
	// jsonschema tag is exactly the failure this guards against: the field
	// rendered with no description at all before #73.
	envelope := c.rpc("tools/list", map[string]any{})
	opDesc := opFieldDescription(t, envelope)
	for _, op := range []string{"create_entity", "add_beat", "create_fact_card"} {
		if !strings.Contains(opDesc, op) {
			t.Errorf("apply_story_ops' op field does not advertise %q; description: %s", op, opDesc)
		}
	}

	// And the unknown-op error carries the catalogue too.
	result := c.callTool("linetta_apply_story_ops", map[string]any{
		"project_id": projectID,
		"summary":    "잘못된 op 테스트",
		"ops":        []map[string]any{{"op": "add_character", "name": "호루"}},
	})
	if !isToolError(result) {
		t.Fatal("unknown op must be a tool error")
	}
	if msg := errorText(result); !strings.Contains(msg, "create_entity") {
		t.Errorf("unknown-op error does not name the valid ops: %s", msg)
	}
}

// opFieldDescription digs tools/list down to
// linetta_apply_story_ops.inputSchema.properties.ops.items.properties.op.description.
func opFieldDescription(t *testing.T, envelope map[string]any) string {
	t.Helper()
	result, _ := envelope["result"].(map[string]any)
	tools, _ := result["tools"].([]any)
	for _, raw := range tools {
		tool, _ := raw.(map[string]any)
		if tool["name"] != "linetta_apply_story_ops" {
			continue
		}
		schema, _ := tool["inputSchema"].(map[string]any)
		props, _ := schema["properties"].(map[string]any)
		ops, _ := props["ops"].(map[string]any)
		items, _ := ops["items"].(map[string]any)
		itemProps, _ := items["properties"].(map[string]any)
		op, _ := itemProps["op"].(map[string]any)
		desc, _ := op["description"].(string)
		return desc
	}
	t.Fatal("linetta_apply_story_ops missing from tools/list")
	return ""
}

// errorText pulls the human-readable message from a tool error, which lives
// in the content blocks (structuredContent is a zero output on errors).
func errorText(result map[string]any) string {
	content, _ := result["content"].([]any)
	for _, raw := range content {
		block, _ := raw.(map[string]any)
		if text, ok := block["text"].(string); ok {
			return text
		}
	}
	return ""
}

// revise_scene snapshots every scene it touches; the ids must reach the
// agent, or its only recovery from a bad sweep is asking the writer to dig
// through version history.
func TestMCPReviseReturnsSnapshotIDsThatUndoAccepts(t *testing.T) {
	_, c, projectID, nodeID := startWritableMCP(t)
	if r := c.callTool("linetta_write_scene", map[string]any{
		"node_id": nodeID, "text": "호루는 삼도천 나루에 서 있었다.", "expected_content_version": 0,
	}); isToolError(r) {
		t.Fatalf("seed write: %v", r)
	}

	applied := c.callTool("linetta_revise_scene", map[string]any{
		"project_id": projectID,
		"find":       "삼도천",
		"replace":    "서천",
	})
	if isToolError(applied) {
		t.Fatalf("revise_scene: %v", applied)
	}
	var out struct {
		Applied     int               `json:"applied"`
		SnapshotIDs map[string]string `json:"snapshot_ids"`
	}
	if err := json.Unmarshal([]byte(structuredJSON(t, applied)), &out); err != nil {
		t.Fatalf("decode revise: %v", err)
	}
	if out.Applied != 1 {
		t.Fatalf("applied = %d, want 1", out.Applied)
	}
	snapID := out.SnapshotIDs[nodeID]
	if snapID == "" {
		t.Fatalf("no snapshot id for the changed scene: %+v", out.SnapshotIDs)
	}

	// The id must be directly usable for a one-scene restore.
	undo := c.callTool("linetta_undo_last_change", map[string]any{"snapshot_id": snapID})
	if isToolError(undo) {
		t.Fatalf("undo with revise snapshot id: %v", undo)
	}
	text, _ := readScene(t, c, nodeID)
	if !strings.Contains(text, "삼도천") {
		t.Errorf("undo did not restore the original wording: %q", text)
	}
}
