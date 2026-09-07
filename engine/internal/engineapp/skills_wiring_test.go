//go:build !mobile

package engineapp

import (
	"encoding/json"
	"strings"
	"testing"
)

// The six skills.* registrations are the one link nothing else in this
// package covers. The two reflective guards in agent_wiring_test.go walk
// mcphost.ToolDeps and agent.Deps — the collaborators a TURN reads — and
// neither knows anything about the RPC registry, so a skills handler left
// unregistered in engineapp.go compiles, passes every handlers-package test
// (they call the handler function directly), and shows up only as a
// Settings pane whose every button answers "method not found".
//
// So this drives a real *App through app.Handle the way the renderer does,
// mirroring agent_wiring_test.go and mcp_wiring_test.go, and walks one skill
// through its whole life: write, list, read, history, restore, delete.
func TestSkillMethodsAreRegisteredOnAFreshInstall(t *testing.T) {
	app := openApp(t)

	// skills.list on a fresh install, before any work is open: the Settings
	// pane's first call. Empty, but both collections present.
	result, rpcErr := call(t, app, "skills.list", `{}`)
	if rpcErr != nil {
		t.Fatalf("skills.list: %+v", rpcErr)
	}
	if !strings.Contains(string(result), `"skills":[]`) || !strings.Contains(string(result), `"diagnostics":[]`) {
		t.Errorf("skills.list on a fresh install = %s, want empty arrays the pane can map over", result)
	}

	// skills.write: create.
	result, rpcErr = call(t, app, "skills.write",
		`{"scope":"writer","name":"fight-scenes","description":"싸움 장면을 쓸 때","body":"짧은 문장.\n"}`)
	if rpcErr != nil {
		t.Fatalf("skills.write: %+v", rpcErr)
	}
	var written struct {
		Name      string `json:"name"`
		Body      string `json:"body"`
		Versioned bool   `json:"versioned"`
	}
	if err := json.Unmarshal(result, &written); err != nil {
		t.Fatalf("decode skills.write: %v", err)
	}
	if written.Name != "fight-scenes" || written.Body != "짧은 문장.\n" {
		t.Fatalf("skills.write = %+v", written)
	}
	// The version row must land through the production wiring too: a store
	// wired without its history would save the file and leave the writer
	// unable to revert it, with nothing failing.
	if !written.Versioned {
		t.Error("versioned = false — skills.write is wired to a history that does not record")
	}

	// skills.list now sees it.
	result, rpcErr = call(t, app, "skills.list", `{}`)
	if rpcErr != nil {
		t.Fatalf("skills.list: %+v", rpcErr)
	}
	if !strings.Contains(string(result), "fight-scenes") {
		t.Errorf("skills.list = %s, want the skill just written", result)
	}

	// skills.read opens the body.
	result, rpcErr = call(t, app, "skills.read", `{"scope":"writer","name":"fight-scenes"}`)
	if rpcErr != nil {
		t.Fatalf("skills.read: %+v", rpcErr)
	}
	if !strings.Contains(string(result), "짧은 문장") {
		t.Errorf("skills.read = %s", result)
	}

	// skills.history has the created row, and its id is what restore takes.
	result, rpcErr = call(t, app, "skills.history", `{"scope":"writer","name":"fight-scenes"}`)
	if rpcErr != nil {
		t.Fatalf("skills.history: %+v", rpcErr)
	}
	var history struct {
		Versions []struct {
			ID     string `json:"id"`
			Reason string `json:"reason"`
		} `json:"versions"`
	}
	if err := json.Unmarshal(result, &history); err != nil {
		t.Fatalf("decode skills.history: %v", err)
	}
	if len(history.Versions) != 1 || history.Versions[0].Reason != "created" {
		t.Fatalf("skills.history = %s, want one row marked created", result)
	}

	// An edit, then skills.restore back to the first version.
	if _, rpcErr := call(t, app, "skills.write",
		`{"scope":"writer","name":"fight-scenes","description":"싸움 장면을 쓸 때","body":"고친 본문\n"}`); rpcErr != nil {
		t.Fatalf("skills.write (edit): %+v", rpcErr)
	}
	result, rpcErr = call(t, app, "skills.restore",
		`{"id":`+mustQuote(t, history.Versions[0].ID)+`}`)
	if rpcErr != nil {
		t.Fatalf("skills.restore: %+v", rpcErr)
	}
	if !strings.Contains(string(result), "짧은 문장") {
		t.Errorf("skills.restore = %s, want the first body back", result)
	}

	// skills.delete.
	if _, rpcErr := call(t, app, "skills.delete", `{"scope":"writer","name":"fight-scenes"}`); rpcErr != nil {
		t.Fatalf("skills.delete: %+v", rpcErr)
	}
	result, rpcErr = call(t, app, "skills.list", `{}`)
	if rpcErr != nil {
		t.Fatalf("skills.list: %+v", rpcErr)
	}
	if strings.Contains(string(result), "fight-scenes") {
		t.Errorf("skills.list = %s, want the deleted skill gone", result)
	}
}

// A refusal the writer can act on must arrive as invalid params (-32602),
// not as an internal error the pane can only show as a 500. The zero-width
// space is the case worth pinning: it is invisible in the editor, so the
// message naming the code point is the only way the writer finds it.
func TestSkillWriteRefusalIsReadableThroughTheWholeStack(t *testing.T) {
	app := openApp(t)
	_, rpcErr := call(t, app, "skills.write",
		"{\"scope\":\"writer\",\"name\":\"pasted\",\"description\":\"붙여넣기\",\"body\":\"보이지 않는 문자​가 있다\"}")
	if rpcErr == nil {
		t.Fatal("a body with a zero-width space must be refused")
	}
	if rpcErr.Code != -32602 {
		t.Errorf("code = %d, want -32602 (invalid params)", rpcErr.Code)
	}
	if !strings.Contains(rpcErr.Message, "U+200B") {
		t.Errorf("message = %q, want the code point the writer has to delete", rpcErr.Message)
	}
}

func mustQuote(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal %q: %v", s, err)
	}
	return string(b)
}
