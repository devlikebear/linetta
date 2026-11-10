//go:build !mobile

package engineapp

import (
	"encoding/json"
	"strings"
	"testing"
)

// projects.submission_check measures the work from the store: episodes with
// text, those short of the work's minimum, the whole work's length, and the
// synopsis (episode-length Phase 3).
func TestSubmissionCheckRPC(t *testing.T) {
	app := openTestApp(t, t.TempDir())
	raw, rpcErr := call(t, app, "projects.create",
		`{"title":"공모전","genres":[],"length_target":"series","default_pov":"first","outline_preset":"webnovel"}`)
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
	doc := func(chars int) string {
		text := strings.Repeat("가", chars)
		b, _ := json.Marshal(map[string]any{"id": proj.LastOpenedNodeID,
			"doc": `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + text + `"}]}]}`})
		return string(b)
	}
	if _, rpcErr := call(t, app, "nodes.update_content", doc(3925)); rpcErr != nil {
		t.Fatalf("nodes.update_content: %+v", rpcErr)
	}
	if _, rpcErr := call(t, app, "projects.update",
		`{"id":"`+proj.ID+`","submission_min_episodes":7,"submission_total_min":30000,"submission_needs_synopsis":true}`); rpcErr != nil {
		t.Fatalf("projects.update: %+v", rpcErr)
	}
	if _, rpcErr := call(t, app, "projects.update",
		`{"id":"`+proj.ID+`","submission_total_min":50000,"submission_total_max":30000}`); rpcErr == nil {
		t.Fatal("a total minimum past the maximum was saved")
	}

	raw, rpcErr = call(t, app, "projects.submission_check", `{"id":"`+proj.ID+`"}`)
	if rpcErr != nil {
		t.Fatalf("projects.submission_check: %+v", rpcErr)
	}
	var got struct {
		HasRule     bool `json:"has_rule"`
		Episodes    int  `json:"episodes"`
		MinEpisodes int  `json:"min_episodes"`
		Short       []struct {
			Label string `json:"label"`
			Delta int    `json:"delta"`
		} `json:"short"`
		TotalChars  int    `json:"total_chars"`
		TotalState  string `json:"total_state"`
		HasSynopsis bool   `json:"has_synopsis"`
		Ready       bool   `json:"ready"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if !got.HasRule || got.Episodes != 1 || got.MinEpisodes != 7 || got.Ready || got.HasSynopsis {
		t.Errorf("report = %+v", got)
	}
	if len(got.Short) != 1 || got.Short[0].Label != "1화" || got.Short[0].Delta != -1075 {
		t.Errorf("short = %+v, want 1화 -1075", got.Short)
	}
	if got.TotalChars != 3925 || got.TotalState != "short" {
		t.Errorf("total = %d %s, want 3925 short", got.TotalChars, got.TotalState)
	}

	if _, rpcErr := call(t, app, "projects.submission_check", `{"id":"nope"}`); rpcErr == nil {
		t.Error("an unknown work was checked")
	}
}
