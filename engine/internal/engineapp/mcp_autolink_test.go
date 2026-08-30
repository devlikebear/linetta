//go:build !mobile

package engineapp

import (
	"encoding/json"
	"strings"
	"testing"
)

// #72: an agent that registers characters and then writes a scene naming
// them must find its own world connected — where_does_appear populated, the
// brief carrying the cast — without the writer clicking scene-scan first.
func TestMCPWriteSceneLinksRegisteredElements(t *testing.T) {
	_, c, projectID, nodeID := startWritableMCP(t)

	created := c.callTool("linetta_apply_story_ops", map[string]any{
		"project_id": projectID,
		"summary":    "인물과 장소 등록",
		"ops": []map[string]any{
			{"op": "create_entity", "kind": "character", "name": "호루", "role": "주인공"},
			{"op": "create_entity", "kind": "place", "name": "삼도천 나루"},
			{"op": "create_entity", "kind": "concept", "name": "검은 낙인"},
		},
	})
	if isToolError(created) {
		t.Fatalf("create entities: %v", created)
	}

	written := c.callTool("linetta_write_scene", map[string]any{
		"node_id":                  nodeID,
		"text":                     "호루는 삼도천 나루에 서 있었다.\n\n호루의 손등이 뜨거웠다.",
		"expected_content_version": 0,
	})
	if isToolError(written) {
		t.Fatalf("write_scene: %v", written)
	}
	var writeOut struct {
		LinkedElements []struct {
			EntityID string `json:"entity_id"`
			Name     string `json:"name"`
			Count    int    `json:"count"`
		} `json:"linked_elements"`
	}
	if err := json.Unmarshal([]byte(structuredJSON(t, written)), &writeOut); err != nil {
		t.Fatalf("decode write: %v", err)
	}
	linkedByName := map[string]int{}
	horuID := ""
	for _, l := range writeOut.LinkedElements {
		linkedByName[l.Name] = l.Count
		if l.Name == "호루" {
			horuID = l.EntityID
		}
	}
	if linkedByName["호루"] != 2 || linkedByName["삼도천 나루"] != 1 {
		t.Fatalf("linked_elements = %+v", writeOut.LinkedElements)
	}
	// 검은 낙인 is registered but absent from the text: it must NOT be linked.
	if _, ok := linkedByName["검은 낙인"]; ok {
		t.Fatal("an element absent from the text was linked")
	}

	// The whole point: the agent's own view is now connected.
	appears := c.callTool("linetta_where_does_appear", map[string]any{"entity_id": horuID})
	if isToolError(appears) {
		t.Fatalf("where_does_appear: %v", appears)
	}
	if !strings.Contains(structuredJSON(t, appears), nodeID) {
		t.Fatalf("scene missing from appearances: %s", structuredJSON(t, appears))
	}

	brief := c.callTool("linetta_get_story_context", map[string]any{"node_id": nodeID})
	if isToolError(brief) {
		t.Fatalf("get_story_context: %v", brief)
	}
	var briefOut struct {
		Brief              string `json:"brief"`
		ElementsNotInBrief []struct {
			Name string `json:"name"`
			Kind string `json:"kind"`
		} `json:"elements_not_in_brief"`
	}
	if err := json.Unmarshal([]byte(structuredJSON(t, brief)), &briefOut); err != nil {
		t.Fatalf("decode brief: %v", err)
	}
	if !strings.Contains(briefOut.Brief, "삼도천 나루") {
		t.Errorf("mentioned place missing from the brief")
	}
	// And the silence is gone: the unmentioned, role-less concept is named as
	// left out instead of vanishing (#45's failure mode, in a new coat).
	foundUnlisted := false
	for _, u := range briefOut.ElementsNotInBrief {
		if u.Name == "검은 낙인" {
			foundUnlisted = true
		}
		if u.Name == "삼도천 나루" || u.Name == "호루" {
			t.Errorf("%s is in the brief and must not be reported as left out", u.Name)
		}
	}
	if !foundUnlisted {
		t.Errorf("elements_not_in_brief misses 검은 낙인: %+v", briefOut.ElementsNotInBrief)
	}
}
