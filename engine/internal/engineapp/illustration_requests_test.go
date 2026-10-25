//go:build !mobile

package engineapp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
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
