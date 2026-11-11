//go:build !mobile

package engineapp

import (
	"encoding/json"
	"strings"
	"testing"
)

// projects.manuscript_pages adds every scene's manuscript lines before turning
// them into sheets, so scenes run on from one another (episode-length Phase 4).
func TestManuscriptPagesRPC(t *testing.T) {
	app := openTestApp(t, t.TempDir())
	raw, rpcErr := call(t, app, "projects.create",
		`{"title":"단편","genres":[],"length_target":"short","default_pov":"first","outline_preset":"novel"}`)
	if rpcErr != nil {
		t.Fatalf("projects.create: %+v", rpcErr)
	}
	var proj struct {
		ID               string `json:"id"`
		LastOpenedNodeID string `json:"last_opened_node_id"`
	}
	if err := json.Unmarshal(raw, &proj); err != nil {
		t.Fatal(err)
	}
	write := func(nodeID string, chars int) {
		t.Helper()
		text := strings.Repeat("가", chars)
		b, _ := json.Marshal(map[string]any{"id": nodeID,
			"doc": `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + text + `"}]}]}`})
		if _, rpcErr := call(t, app, "nodes.update_content", string(b)); rpcErr != nil {
			t.Fatalf("nodes.update_content: %+v", rpcErr)
		}
	}
	pages := func() (int, int) {
		t.Helper()
		raw, rpcErr := call(t, app, "projects.manuscript_pages", `{"id":"`+proj.ID+`"}`)
		if rpcErr != nil {
			t.Fatalf("projects.manuscript_pages: %+v", rpcErr)
		}
		var got struct {
			Lines int `json:"lines"`
			Pages int `json:"pages"`
		}
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		return got.Lines, got.Pages
	}

	if lines, sheets := pages(); lines != 0 || sheets != 0 {
		t.Errorf("an empty work = %d lines %d sheets, want 0 0", lines, sheets)
	}

	write(proj.LastOpenedNodeID, 219) // 220 cells: 11 lines
	raw, rpcErr = call(t, app, "nodes.create_sibling",
		`{"reference_id":"`+proj.LastOpenedNodeID+`","kind":"leaf","label":"씬 2"}`)
	if rpcErr != nil {
		t.Fatalf("nodes.create_sibling: %+v", rpcErr)
	}
	var second struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &second); err != nil {
		t.Fatal(err)
	}
	write(second.ID, 179) // 180 cells: 9 lines

	// Two sheets, not three: the second scene runs on from the first.
	if lines, sheets := pages(); lines != 20 || sheets != 2 {
		t.Errorf("work = %d lines %d sheets, want 20 lines 2 sheets", lines, sheets)
	}

	if _, rpcErr := call(t, app, "projects.manuscript_pages", `{"id":"nope"}`); rpcErr == nil {
		t.Error("an unknown work was measured")
	}
	if _, rpcErr := call(t, app, "projects.manuscript_pages", `{}`); rpcErr == nil {
		t.Error("a request without a work was measured")
	}
}
