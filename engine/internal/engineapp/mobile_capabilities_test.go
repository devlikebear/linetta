//go:build mobile

package engineapp

import (
	"context"
	"strings"
	"testing"
)

// Publishing to Linetta Works ships on iOS: diagnostics says so, which is
// what shows the Settings entry and the publish actions.
func TestPublishIsAvailableOnMobile(t *testing.T) {
	app := openTestApp(t, t.TempDir())
	raw, err := app.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"diagnostics.version","params":null}`))
	if err != nil || !strings.Contains(string(raw), `"publish_available":true`) {
		t.Fatalf("diagnostics.version: %s %v", raw, err)
	}
}

// iOS keeps the library in the app's private container, so diagnostics tells
// the UI to leave out folder sync and the open-folder actions.
func TestFoldersAreUnavailableOnMobile(t *testing.T) {
	app := openTestApp(t, t.TempDir())
	raw, err := app.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"diagnostics.version","params":null}`))
	if err != nil || !strings.Contains(string(raw), `"folders_available":false`) {
		t.Fatalf("diagnostics.version: %s %v", raw, err)
	}
}

func TestVaultIsUnavailableOnMobile(t *testing.T) {
	app := openTestApp(t, t.TempDir())
	raw, err := app.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"diagnostics.version","params":null}`))
	if err != nil || !strings.Contains(string(raw), `"vault_available":false`) {
		t.Fatalf("diagnostics.version: %s %v", raw, err)
	}
	for _, method := range []string{"vault.status", "vault.work_status", "vault.setup", "vault.push", "vault.pull", "vault.resolve_conflict", "vault.delete_item"} {
		raw, err := app.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"`+method+`","params":{}}`))
		if err != nil || !strings.Contains(string(raw), "not available in this build") {
			t.Fatalf("%s: %s %v", method, raw, err)
		}
	}
}
