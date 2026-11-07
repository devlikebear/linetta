//go:build !mobile

package engineapp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// An external client is never refused for writing a new episode with no plan,
// or for writing after episodes the ledger has not caught up with (longform
// Phase 6). The write goes through and says what it went ahead without.
func TestMCPWritesWarnInsteadOfRefusing(t *testing.T) {
	app, c, projectID, first := startWritableMCP(t)
	write := func(nodeID, text string) []string {
		t.Helper()
		_, version := readScene(t, c, nodeID)
		result := c.callTool("linetta_write_scene", map[string]any{
			"node_id": nodeID, "text": text, "expected_content_version": version,
		})
		if isToolError(result) {
			t.Fatalf("write_scene was refused: %v", result)
		}
		var out struct {
			Warnings []string `json:"warnings"`
		}
		if err := json.Unmarshal([]byte(structuredJSON(t, result)), &out); err != nil {
			t.Fatal(err)
		}
		return out.Warnings
	}

	got := write(first, "비가 내렸다.")
	if len(got) != 1 || !strings.Contains(got[0], "no plan") {
		t.Fatalf("first write's warnings = %q, want the missing plan", got)
	}
	if got := write(first, "비가 그쳤다."); len(got) != 0 {
		t.Errorf("rewriting a scene with prose warned: %q", got)
	}

	created, rpcErr := call(t, app, "nodes.create_sibling", `{"reference_id":"`+first+`","kind":"leaf","label":"2화"}`)
	if rpcErr != nil {
		t.Fatalf("nodes.create_sibling: %+v", rpcErr)
	}
	var second struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created, &second); err != nil {
		t.Fatal(err)
	}
	if res := c.callTool("linetta_write_summary", map[string]any{"node_id": second.ID, "summary": "형을 만난다.", "as_plan": true}); isToolError(res) {
		t.Fatalf("write_summary as_plan: %v", res)
	}

	time.Sleep(5 * time.Millisecond)
	if _, rpcErr := call(t, app, "settings.set", `{"agent_settlement_enabled":true}`); rpcErr != nil {
		t.Fatalf("settings.set: %+v", rpcErr)
	}
	time.Sleep(5 * time.Millisecond)
	write(first, "비가 다시 내렸다.") // changed since settlement was switched on: waiting

	got = write(second.ID, "형은 문 앞에 서 있었다.")
	if len(got) != 1 || !strings.Contains(got[0], "not settled") {
		t.Fatalf("second write's warnings = %q, want only the waiting episode (it has a plan)", got)
	}

	result := c.callTool("linetta_revise_scene", map[string]any{
		"project_id": projectID, "find": "문 앞에", "replace": "대문 앞에", "node_ids": []string{second.ID},
	})
	if isToolError(result) {
		t.Fatalf("revise_scene was refused: %v", result)
	}
	var revised struct {
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(structuredJSON(t, result)), &revised); err != nil {
		t.Fatal(err)
	}
	if len(revised.Warnings) != 1 || !strings.Contains(revised.Warnings[0], "not settled") {
		t.Errorf("revise warnings = %q, want the waiting episode", revised.Warnings)
	}
}
