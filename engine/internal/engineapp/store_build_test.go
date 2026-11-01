//go:build mas

package engineapp

import (
	"strings"
	"testing"
)

// The Mac App Store build sends nothing to a server the developer runs, and
// its export declaration and privacy label say so. This is the check on both.
func TestStoreBuildLeavesLinettaWorksOut(t *testing.T) {
	app := openApp(t)
	result, rpcErr := call(t, app, "diagnostics.get", "")
	if rpcErr != nil {
		t.Fatalf("diagnostics.get: %+v", rpcErr)
	}
	for _, want := range []string{`"publish_available":false`, `"vault_available":false`} {
		if !strings.Contains(string(result), want) {
			t.Fatalf("diagnostics.get = %s, want %s", result, want)
		}
	}
	for _, method := range []string{"publish.connect_start", "vault.push", "sync.run_now"} {
		if _, rpcErr := call(t, app, method, `{}`); rpcErr == nil {
			t.Errorf("%s answered in the store build", method)
		}
	}
}
