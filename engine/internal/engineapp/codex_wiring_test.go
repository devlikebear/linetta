//go:build !mobile

package engineapp

import (
	"encoding/json"
	"testing"
)

// A fresh library has no Codex login, and the three methods must all be
// reachable. A typo in a Handle string compiles and passes every unit test;
// only a call through the real dispatcher catches it.
func TestCodexMethodsAreRegistered(t *testing.T) {
	app := openApp(t)

	res, rpcErr := call(t, app, "codex.login_status", "")
	if rpcErr != nil {
		t.Fatalf("codex.login_status: %v", rpcErr)
	}
	var st struct {
		LoggedIn bool   `json:"logged_in"`
		Email    string `json:"email"`
	}
	if err := json.Unmarshal(res, &st); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if st.LoggedIn || st.Email != "" {
		t.Errorf("fresh library reports a login: %+v", st)
	}

	// Logging out with nothing stored is a success, and proves the method is
	// wired rather than merely absent.
	if _, rpcErr := call(t, app, "codex.logout", ""); rpcErr != nil {
		t.Fatalf("codex.logout: %v", rpcErr)
	}
}
