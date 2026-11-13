//go:build !mobile

package engineapp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func createWork(t *testing.T, app *App, title string) (string, string) {
	t.Helper()
	out, rpcErr := call(t, app, "projects.create",
		fmt.Sprintf(`{"title":%q,"genres":["fantasy"],"length_target":"short","default_pov":"first"}`, title))
	if rpcErr != nil {
		t.Fatalf("projects.create: %+v", rpcErr)
	}
	var p struct {
		ID               string  `json:"id"`
		LastOpenedNodeID *string `json:"last_opened_node_id"`
	}
	if err := json.Unmarshal(out, &p); err != nil {
		t.Fatal(err)
	}
	return p.ID, *p.LastOpenedNodeID
}

// #182: the writer's notes reach the brief of every work that follows the
// writer's default style, and no other; the limit is refused, not cut.
func TestWriterStyleNotes_reachEveryFollowingWorksBrief(t *testing.T) {
	app, c, _, nodeA := startWritableMCP(t)
	projectB, nodeB := createWork(t, app, "둘째 작품")
	const notes = "모든 작품은 현재형으로 쓴다."
	if _, rpcErr := call(t, app, "style.set_writer_notes", fmt.Sprintf(`{"notes":%q}`, notes)); rpcErr != nil {
		t.Fatalf("set_writer_notes: %+v", rpcErr)
	}
	for _, n := range []string{nodeA, nodeB} {
		if brief := storyBrief(t, c, n); strings.Count(brief, notes) != 1 {
			t.Fatalf("brief of %s carries the writer's notes %d times:\n%s", n, strings.Count(brief, notes), brief)
		}
	}

	if _, rpcErr := call(t, app, "style.set_rules", fmt.Sprintf(`{"project_id":%q,"avoid_phrases":[],"ignore_writer_style":true}`, projectB)); rpcErr != nil {
		t.Fatalf("set_rules: %+v", rpcErr)
	}
	if brief := storyBrief(t, c, nodeB); strings.Contains(brief, notes) {
		t.Fatalf("an opted-out work's brief has the writer's notes:\n%s", brief)
	}

	_, rpcErr := call(t, app, "style.set_writer_notes", fmt.Sprintf(`{"notes":%q}`, strings.Repeat("가", 1001)))
	if rpcErr == nil || !strings.Contains(string(rpcErr.Data), "style_writer_notes_too_long") {
		t.Fatalf("over the limit: %+v", rpcErr)
	}
	raw, _ := call(t, app, "style.get_writer_rules", "")
	var w struct {
		Notes      string `json:"notes"`
		NotesLimit int    `json:"notes_limit"`
	}
	_ = json.Unmarshal(raw, &w)
	if w.Notes != notes || w.NotesLimit != 1000 {
		t.Fatalf("writer style after a refused save = %s", raw)
	}
}

// #183: an agent's proposal for the writer's style waits for approval, and
// only then reaches every work's brief.
func TestWriterStyleDraft_reachesTheBriefsOnlyAfterApproval(t *testing.T) {
	app, c, _, nodeA := startWritableMCP(t)
	_, nodeB := createWork(t, app, "둘째 작품")
	const proposal = "짧은 문장, 현재형, 감각 묘사를 아낀다."

	res := c.callTool("linetta_propose_style_profile", map[string]any{"scope": "writer", "profile": proposal})
	if isToolError(res) {
		t.Fatalf("propose writer: %v", res)
	}
	if out := structuredJSON(t, res); !strings.Contains(out, `"pending_writer_approval"`) || !strings.Contains(out, `"limit":1000`) {
		t.Fatalf("proposal result = %s", out)
	}
	if brief := storyBrief(t, c, nodeA); strings.Contains(brief, proposal) {
		t.Fatal("an unapproved writer draft reached a brief")
	}
	raw, rpcErr := call(t, app, "style.get_writer_draft", "")
	if rpcErr != nil || !strings.Contains(string(raw), `"pending":true`) || !strings.Contains(string(raw), proposal) {
		t.Fatalf("get_writer_draft = %s %+v", raw, rpcErr)
	}

	if _, rpcErr := call(t, app, "style.approve_writer_draft", fmt.Sprintf(`{"body":%q,"mode":"append"}`, proposal)); rpcErr != nil {
		t.Fatalf("approve: %+v", rpcErr)
	}
	for _, n := range []string{nodeA, nodeB} {
		if brief := storyBrief(t, c, n); !strings.Contains(brief, proposal) {
			t.Fatalf("approved writer style missing from %s's brief", n)
		}
	}
	raw, _ = call(t, app, "style.get_writer_draft", "")
	if !strings.Contains(string(raw), `"pending":false`) {
		t.Fatalf("draft still pending after approval: %s", raw)
	}

	// Too long for the writer's limit, though short enough for a work's.
	res = c.callTool("linetta_propose_style_profile", map[string]any{"scope": "writer", "profile": strings.Repeat("가", 1200)})
	if !isToolError(res) || !strings.Contains(errorText(res), "1000") {
		t.Fatalf("over-long writer proposal: %v", res)
	}
}

// A client pinned to one work may read the writer's style but not propose it.
func TestWriterStyleDraft_refusedForAPinnedClient(t *testing.T) {
	app, c, projectID, _ := startWritableMCP(t)
	if _, rpcErr := call(t, app, "settings.set", fmt.Sprintf(`{"mcp_project_id":%q}`, projectID)); rpcErr != nil {
		t.Fatalf("settings.set: %+v", rpcErr)
	}
	res := c.callTool("linetta_propose_style_profile", map[string]any{"scope": "writer", "profile": "현재형."})
	if !isToolError(res) || !strings.Contains(errorText(res), "restricted") || !strings.Contains(errorText(res), "scope work") {
		t.Fatalf("pinned writer proposal: %v", res)
	}
	raw, _ := call(t, app, "style.get_writer_draft", "")
	if !strings.Contains(string(raw), `"pending":false`) {
		t.Fatalf("a refused proposal was saved: %s", raw)
	}
	// Its own work's profile is still its to propose.
	if res := c.callTool("linetta_propose_style_profile", map[string]any{"profile": "현재형."}); isToolError(res) {
		t.Fatalf("pinned work proposal: %v", res)
	}
}
