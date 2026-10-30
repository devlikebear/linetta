//go:build !mobile

package engineapp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/illustration"
)

// The request methods are wired, answer an empty work, and explain a missing
// request with its reason code rather than a raw message.
func TestIllustrationRequestMethods(t *testing.T) {
	app := openTestApp(t, t.TempDir())
	created, rpcErr := call(t, app, "projects.create",
		`{"title":"요청 시험","genres":[],"length_target":"short","default_pov":"first"}`)
	if rpcErr != nil {
		t.Fatalf("projects.create: %+v", rpcErr)
	}
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created, &p); err != nil {
		t.Fatal(err)
	}
	raw, rpcErr := call(t, app, "illustration_requests.list", fmt.Sprintf(`{"project_id":%q}`, p.ID))
	if rpcErr != nil || strings.TrimSpace(string(raw)) != "[]" {
		t.Fatalf("list = %s, %+v", raw, rpcErr)
	}
	for _, method := range []string{"illustration_requests.get", "illustration_requests.approve", "illustration_requests.reject"} {
		_, rpcErr := call(t, app, method, `{"id":"missing"}`)
		if rpcErr == nil || !strings.Contains(string(rpcErr.Data), "illustration_request_not_found") {
			t.Fatalf("%s: %+v", method, rpcErr)
		}
		if _, rpcErr := call(t, app, method, `{}`); rpcErr == nil {
			t.Fatalf("%s accepted no id", method)
		}
	}
}

// The writer's own representative-picture choice goes through two methods:
// representatives lists them and set_representative chooses or clears one.
func TestIllustrationRepresentativeMethods(t *testing.T) {
	app := openTestApp(t, t.TempDir())
	created, rpcErr := call(t, app, "projects.create",
		`{"title":"대표 시험","genres":[],"length_target":"short","default_pov":"first"}`)
	if rpcErr != nil {
		t.Fatalf("projects.create: %+v", rpcErr)
	}
	var p struct {
		ID               string `json:"id"`
		LastOpenedNodeID string `json:"last_opened_node_id"`
	}
	if err := json.Unmarshal(created, &p); err != nil {
		t.Fatal(err)
	}
	pic, err := app.mcpTools.Gallery.Add(context.Background(), 1, illustration.NewInput{ProjectID: p.ID, Prompt: "p", Provider: "x", Model: "m", Data: tinyJPEG(t)})
	if err != nil {
		t.Fatal(err)
	}
	if _, rpcErr := call(t, app, "illustrations.set_representative", fmt.Sprintf(`{"node_id":%q,"illustration_id":%q}`, p.LastOpenedNodeID, pic.ID)); rpcErr != nil {
		t.Fatalf("set: %+v", rpcErr)
	}
	raw, rpcErr := call(t, app, "illustrations.representatives", fmt.Sprintf(`{"project_id":%q}`, p.ID))
	var reps map[string]string
	if rpcErr != nil || json.Unmarshal(raw, &reps) != nil || reps[p.LastOpenedNodeID] != pic.ID {
		t.Fatalf("representatives = %s, %+v", raw, rpcErr)
	}
	raw, rpcErr = call(t, app, "illustrations.set_representative", fmt.Sprintf(`{"node_id":%q}`, p.LastOpenedNodeID))
	if rpcErr != nil || !strings.Contains(string(raw), pic.ID) {
		t.Fatalf("clear = %s, %+v", raw, rpcErr)
	}
	if _, rpcErr := call(t, app, "illustrations.set_representative", fmt.Sprintf(`{"node_id":%q,"illustration_id":"nope"}`, p.LastOpenedNodeID)); rpcErr == nil || !strings.Contains(string(rpcErr.Data), "illustration_not_found") {
		t.Fatalf("unknown picture: %+v", rpcErr)
	}
}
