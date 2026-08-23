//go:build !mobile

package engineapp

import (
	"encoding/json"
	"strings"
	"testing"
)

// The exit criterion for the write phase: an agent writes prose and can put it
// back. Body reverts go through the snapshot, not the batch id.
func TestMCPWriteSceneThenUndoRestoresTheOriginalBytes(t *testing.T) {
	_, c, _, nodeID := startWritableMCP(t)

	const original = "원래 있던 문장이다.\n\n두 번째 문단."
	_, v0 := readScene(t, c, nodeID)
	if r := c.callTool("linetta_write_scene", map[string]any{
		"node_id": nodeID, "text": original, "expected_content_version": v0,
	}); isToolError(r) {
		t.Fatalf("seed write: %v", r)
	}
	before, v1 := readScene(t, c, nodeID)

	result := c.callTool("linetta_write_scene", map[string]any{
		"node_id": nodeID, "text": "에이전트가 완전히 새로 쓴 원고.", "expected_content_version": v1,
	})
	if isToolError(result) {
		t.Fatalf("write_scene: %v", result)
	}
	var wrote struct {
		SnapshotID string `json:"snapshot_id"`
	}
	if err := json.Unmarshal([]byte(structuredJSON(t, result)), &wrote); err != nil {
		t.Fatalf("decode write_scene: %v", err)
	}
	if wrote.SnapshotID == "" {
		t.Fatal("write_scene must return the snapshot id that restores the previous text")
	}

	undo := c.callTool("linetta_undo_last_change", map[string]any{"snapshot_id": wrote.SnapshotID})
	if isToolError(undo) {
		t.Fatalf("undo: %v", undo)
	}
	var undone struct {
		Reverted string `json:"reverted"`
	}
	if err := json.Unmarshal([]byte(structuredJSON(t, undo)), &undone); err != nil {
		t.Fatalf("decode undo: %v", err)
	}
	if undone.Reverted != "scene" {
		t.Errorf("reverted = %q, want scene", undone.Reverted)
	}

	after, _ := readScene(t, c, nodeID)
	if after != before {
		t.Fatalf("undo did not restore the original bytes:\n got %q\nwant %q", after, before)
	}
}

// A structural batch applies atomically and its batch id puts the outline back.
func TestMCPApplyStoryOpsAndUndoBatch(t *testing.T) {
	_, c, projectID, nodeID := startWritableMCP(t)

	countScenes := func() int {
		t.Helper()
		outline := c.callTool("linetta_get_outline", map[string]any{"project_id": projectID})
		var out struct {
			Outline []struct {
				Kind string `json:"kind"`
			} `json:"outline"`
		}
		if err := json.Unmarshal([]byte(structuredJSON(t, outline)), &out); err != nil {
			t.Fatalf("decode outline: %v", err)
		}
		return len(out.Outline)
	}
	before := countScenes()

	result := c.callTool("linetta_apply_story_ops", map[string]any{
		"project_id": projectID,
		"node_id":    nodeID,
		"summary":    "2화와 인물 추가",
		"ops": []map[string]any{
			{"op": "create_scene", "ref": "s2", "after_node_id": nodeID, "label": "2화", "title": "기록보관실"},
			{"op": "create_entity", "ref": "hayun", "kind": "character", "name": "하윤", "role": "조력자"},
		},
	})
	if isToolError(result) {
		t.Fatalf("apply_story_ops: %v", result)
	}
	var applied struct {
		Applied     int    `json:"applied"`
		UndoBatchID string `json:"undo_batch_id"`
	}
	if err := json.Unmarshal([]byte(structuredJSON(t, result)), &applied); err != nil {
		t.Fatalf("decode apply: %v", err)
	}
	if applied.Applied != 2 {
		t.Fatalf("applied = %d, want 2", applied.Applied)
	}
	if applied.UndoBatchID == "" {
		t.Fatal("a structural batch must return an undo_batch_id")
	}
	if countScenes() != before+1 {
		t.Fatalf("outline did not gain the new scene: %d -> %d", before, countScenes())
	}

	chars := c.callTool("linetta_list_characters", map[string]any{"project_id": projectID})
	if !strings.Contains(structuredJSON(t, chars), "하윤") {
		t.Error("the created character is not readable through the read tools")
	}

	if undo := c.callTool("linetta_undo_last_change", map[string]any{
		"batch_id": applied.UndoBatchID,
	}); isToolError(undo) {
		t.Fatalf("undo batch: %v", undo)
	}
	if got := countScenes(); got != before {
		t.Fatalf("undo did not restore the outline: %d, want %d", got, before)
	}
}

// One door per mutation type: set_scene_text through the batch tool would skip
// write_scene's version check entirely.
func TestMCPApplyStoryOpsRejectsSceneText(t *testing.T) {
	_, c, projectID, nodeID := startWritableMCP(t)
	result := c.callTool("linetta_apply_story_ops", map[string]any{
		"project_id": projectID,
		"node_id":    nodeID,
		"summary":    "본문 덮어쓰기 시도",
		"ops": []map[string]any{
			{"op": "set_scene_text", "node_id": nodeID, "text": "버전 검사를 우회한 본문"},
		},
	})
	if !isToolError(result) {
		t.Fatal("set_scene_text must be refused by the batch tool")
	}
	if msg := toolErrorText(result); !strings.Contains(msg, "linetta_write_scene") {
		t.Errorf("the refusal should point at write_scene: %s", msg)
	}
	body, _ := readScene(t, c, nodeID)
	if strings.Contains(body, "우회한 본문") {
		t.Fatal("the rejected op still changed the scene")
	}
}

// A failing op rolls the whole batch back: half a restructured outline is
// worse than none.
func TestMCPApplyStoryOpsRollsBackOnFailure(t *testing.T) {
	_, c, projectID, nodeID := startWritableMCP(t)
	result := c.callTool("linetta_apply_story_ops", map[string]any{
		"project_id": projectID,
		"node_id":    nodeID,
		"summary":    "실패하는 배치",
		"ops": []map[string]any{
			{"op": "create_outline_node", "kind": "container", "label": "1부"},
			{"op": "delete_outline_node", "node_id": "no-such-node"},
		},
	})
	if !isToolError(result) {
		t.Fatal("a batch with a failing op must report an error")
	}
	var out struct {
		Applied    int  `json:"applied"`
		RolledBack bool `json:"rolled_back"`
	}
	if err := json.Unmarshal([]byte(structuredJSON(t, result)), &out); err != nil {
		t.Fatalf("decode apply: %v", err)
	}
	if !out.RolledBack || out.Applied != 0 {
		t.Fatalf("result = %+v, want a rollback with nothing applied", out)
	}
}

func TestMCPCheckpointAndArgumentChecks(t *testing.T) {
	_, c, _, nodeID := startWritableMCP(t)
	_, v := readScene(t, c, nodeID)
	if r := c.callTool("linetta_write_scene", map[string]any{
		"node_id": nodeID, "text": "체크포인트 대상 원고.", "expected_content_version": v,
	}); isToolError(r) {
		t.Fatalf("write: %v", r)
	}

	result := c.callTool("linetta_create_checkpoint", map[string]any{"node_id": nodeID})
	if isToolError(result) {
		t.Fatalf("create_checkpoint: %v", result)
	}
	var cp struct {
		SnapshotID string `json:"snapshot_id"`
	}
	if err := json.Unmarshal([]byte(structuredJSON(t, result)), &cp); err != nil {
		t.Fatalf("decode checkpoint: %v", err)
	}
	if cp.SnapshotID == "" {
		t.Fatal("a checkpoint must return a snapshot id even when the text is unchanged")
	}

	if r := c.callTool("linetta_undo_last_change", map[string]any{}); !isToolError(r) {
		t.Error("undo with no id must be refused")
	}
	if r := c.callTool("linetta_undo_last_change", map[string]any{
		"batch_id": "a", "snapshot_id": "b",
	}); !isToolError(r) {
		t.Error("undo with both ids must be refused")
	}
	if r := c.callTool("linetta_undo_last_change", map[string]any{
		"snapshot_id": "no-such-snapshot",
	}); !isToolError(r) {
		t.Error("undo with an unknown snapshot must be refused")
	}
}
