//go:build !mobile

package engineapp

import (
	"encoding/json"
	"strings"
	"testing"
)

// A web novel bounds its episodes (episode-length Phase 2): a write that
// leaves an episode short says so from the episode's last scene only, one
// that runs past the maximum says so from any scene, and the outline tells
// an external client which episodes are short.
func TestMCPEpisodeLengthWarningsAndOutline(t *testing.T) {
	app, c, _, _ := startWritableMCP(t)
	created, rpcErr := call(t, app, "projects.create",
		`{"title":"회차 분량","genres":["fantasy"],"length_target":"series","default_pov":"first","outline_preset":"webnovel"}`)
	if rpcErr != nil {
		t.Fatalf("projects.create: %+v", rpcErr)
	}
	var proj struct {
		ID               string `json:"id"`
		LastOpenedNodeID string `json:"last_opened_node_id"`
	}
	if err := json.Unmarshal(created, &proj); err != nil {
		t.Fatal(err)
	}
	first := proj.LastOpenedNodeID // 1화, a scene of its own

	newNode := func(method, params string) string {
		t.Helper()
		raw, rpcErr := call(t, app, method, params)
		if rpcErr != nil {
			t.Fatalf("%s: %+v", method, rpcErr)
		}
		var n struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &n); err != nil {
			t.Fatal(err)
		}
		return n.ID
	}
	ep2 := newNode("nodes.create_sibling", `{"reference_id":"`+first+`","kind":"container","label":"2화"}`)
	sceneA := newNode("nodes.create_child", `{"parent_id":"`+ep2+`","kind":"leaf","label":"씬 1"}`)
	sceneB := newNode("nodes.create_child", `{"parent_id":"`+ep2+`","kind":"leaf","label":"씬 2"}`)

	lengthWarning := func(nodeID string, chars int) string {
		t.Helper()
		_, version := readScene(t, c, nodeID)
		result := c.callTool("linetta_write_scene", map[string]any{
			"node_id": nodeID, "text": strings.Repeat("가", chars), "expected_content_version": version,
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
		found := ""
		for _, w := range out.Warnings {
			if strings.Contains(w, "characters (including spaces)") {
				if found != "" {
					t.Fatalf("two length warnings: %q", out.Warnings)
				}
				found = w
			}
		}
		return found
	}

	if got := lengthWarning(first, 3925); !strings.Contains(got, "episode 1화 is 3,925 characters") ||
		!strings.Contains(got, "the minimum is 5,000 — 1,075 short") {
		t.Errorf("a short one-scene episode's warning = %q", got)
	}
	if got := lengthWarning(sceneA, 2100); got != "" {
		t.Errorf("the first of two scenes warned before the episode was done: %q", got)
	}
	if got := lengthWarning(sceneB, 1825); !strings.Contains(got, "episode 2화 is 3,925 characters") {
		t.Errorf("the last scene's warning = %q, want 2화 short", got)
	}
	if got := lengthWarning(sceneB, 3000); got != "" {
		t.Errorf("an episode that reached the minimum warned: %q", got)
	}

	if _, rpcErr := call(t, app, "projects.update", `{"id":"`+proj.ID+`","episode_char_max":5300}`); rpcErr != nil {
		t.Fatalf("projects.update: %+v", rpcErr)
	}
	if got := lengthWarning(sceneA, 2400); !strings.Contains(got, "over the 5,300 maximum by 100") {
		t.Errorf("an episode past the maximum warned %q, from its first scene too", got)
	}

	result := c.callTool("linetta_get_outline", map[string]any{"project_id": proj.ID})
	if isToolError(result) {
		t.Fatalf("get_outline: %v", result)
	}
	var outline struct {
		EpisodeCharMin int `json:"episode_char_min"`
		EpisodeCharMax int `json:"episode_char_max"`
		Outline        []struct {
			NodeID       string `json:"node_id"`
			EpisodeChars *int   `json:"episode_chars"`
			LengthState  string `json:"length_state"`
			LengthDelta  int    `json:"length_delta"`
		} `json:"outline"`
	}
	if err := json.Unmarshal([]byte(structuredJSON(t, result)), &outline); err != nil {
		t.Fatal(err)
	}
	if outline.EpisodeCharMin != 5000 || outline.EpisodeCharMax != 5300 {
		t.Errorf("bounds = %d..%d, want 5000..5300", outline.EpisodeCharMin, outline.EpisodeCharMax)
	}
	type judged struct {
		chars *int
		state string
		delta int
	}
	rows := map[string]judged{}
	for _, r := range outline.Outline {
		rows[r.NodeID] = judged{r.EpisodeChars, r.LengthState, r.LengthDelta}
	}
	if r := rows[first]; r.chars == nil || *r.chars != 3925 || r.state != "short" || r.delta != -1075 {
		t.Errorf("1화 row = %+v, want 3925 short -1075", r)
	}
	if r := rows[ep2]; r.chars == nil || *r.chars != 5400 || r.state != "long" || r.delta != 100 {
		t.Errorf("2화 row = %+v, want 5400 long +100", r)
	}
	for _, id := range []string{sceneA, sceneB} {
		if r := rows[id]; r.chars != nil || r.state != "" {
			t.Errorf("a scene row carries an episode length: %+v", r)
		}
	}
}

// A work that sets no bounds writes and lists as it did before.
func TestMCPUnboundedWorkHasNoEpisodeLength(t *testing.T) {
	_, c, projectID, first := startWritableMCP(t) // a novel: no minimum
	_, version := readScene(t, c, first)
	result := c.callTool("linetta_write_scene", map[string]any{
		"node_id": first, "text": "짧다.", "expected_content_version": version,
	})
	if strings.Contains(structuredJSON(t, result), "characters (") {
		t.Errorf("an unbounded work's write warned about length: %s", structuredJSON(t, result))
	}
	outline := structuredJSON(t, c.callTool("linetta_get_outline", map[string]any{"project_id": projectID}))
	for _, field := range []string{"episode_char_min", "episode_chars", "length_state"} {
		if strings.Contains(outline, field) {
			t.Errorf("an unbounded work's outline has %s: %s", field, outline)
		}
	}
}
