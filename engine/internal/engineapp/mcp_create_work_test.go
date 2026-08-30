//go:build !mobile

package engineapp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// #79: starting a work must not require leaving the agent. Create → draft in
// one round trip, using the defaults the app's own new-work dialog uses.
func TestMCPCreateWorkIsImmediatelyDraftable(t *testing.T) {
	_, c, _, _ := startWritableMCP(t)

	created := c.callTool("linetta_create_work", map[string]any{
		"title":  "지워지지 않는 이름",
		"genres": []string{"현대판타지"},
	})
	if isToolError(created) {
		t.Fatalf("create_work: %v", created)
	}
	var out struct {
		ProjectID        string `json:"project_id"`
		Title            string `json:"title"`
		FirstSceneNodeID string `json:"first_scene_node_id"`
	}
	if err := json.Unmarshal([]byte(structuredJSON(t, created)), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.ProjectID == "" || out.FirstSceneNodeID == "" {
		t.Fatalf("ids missing: %+v", out)
	}

	// The returned scene is real and version 0: drafting starts right here.
	written := c.callTool("linetta_write_scene", map[string]any{
		"node_id": out.FirstSceneNodeID, "text": "첫 문장.", "expected_content_version": 0,
	})
	if isToolError(written) {
		t.Fatalf("write into the new work: %v", written)
	}

	works := c.callTool("linetta_list_works", map[string]any{})
	if !strings.Contains(structuredJSON(t, works), out.ProjectID) {
		t.Fatalf("new work missing from linetta_list_works")
	}
}

// A restricted server promised the writer "this work only". A work the agent
// creates but can never touch again would be a trap — refuse loudly.
func TestMCPCreateWorkRefusedOnARestrictedServer(t *testing.T) {
	app, c, projectID, _ := startWritableMCP(t)
	if _, rpcErr := call(t, app, "settings.set", fmt.Sprintf(`{"mcp_project_id":%q}`, projectID)); rpcErr != nil {
		t.Fatalf("settings.set: %+v", rpcErr)
	}

	result := c.callTool("linetta_create_work", map[string]any{"title": "몰래 만든 작품"})
	if !isToolError(result) {
		t.Fatal("create_work must refuse on a restricted server")
	}
	if msg := errorText(result); !strings.Contains(msg, "restricted") {
		t.Errorf("refusal should say why: %s", msg)
	}
}
