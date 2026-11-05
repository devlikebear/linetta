//go:build mas

package engineapp

import (
	"strings"
	"testing"
)

// The Mac App Store build publishes to Linetta Works but leaves sync out: the
// vault's own encryption is what its export declaration says it does not
// ship. This is the check on that line.
func TestStoreBuildPublishesButLeavesSyncOut(t *testing.T) {
	app := openApp(t)
	result, rpcErr := call(t, app, "diagnostics.get", "")
	if rpcErr != nil {
		t.Fatalf("diagnostics.get: %+v", rpcErr)
	}
	for _, want := range []string{`"publish_available":true`, `"vault_available":false`} {
		if !strings.Contains(string(result), want) {
			t.Fatalf("diagnostics.get = %s, want %s", result, want)
		}
	}
	for _, method := range []string{"vault.push", "sync.run_now"} {
		if _, rpcErr := call(t, app, method, `{}`); rpcErr == nil {
			t.Errorf("%s answered in the store build", method)
		}
	}
}
