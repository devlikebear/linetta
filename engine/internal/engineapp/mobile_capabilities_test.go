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

// Linetta Works sync ships on iOS too. Only Linetta Works can be the target:
// there is no folder of the writer's here, and no git.
func TestVaultIsAvailableOnMobileWithLinettaWorksOnly(t *testing.T) {
	app := openTestApp(t, t.TempDir())
	raw, err := app.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"diagnostics.version","params":null}`))
	if err != nil || !strings.Contains(string(raw), `"vault_available":true`) {
		t.Fatalf("diagnostics.version: %s %v", raw, err)
	}
	raw, err = app.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"sync.targets","params":{}}`))
	if err != nil || !strings.Contains(string(raw), `"id":"works"`) {
		t.Fatalf("sync.targets: %s %v", raw, err)
	}
	for _, absent := range []string{`"id":"folder"`, `"id":"git"`} {
		if strings.Contains(string(raw), absent) {
			t.Errorf("sync.targets offers %s on iOS: %s", absent, raw)
		}
	}
	raw, err = app.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"sync.set_targets","params":{"primary":"works","mirrors":["folder"]}}`))
	if err != nil || !strings.Contains(string(raw), `"error"`) {
		t.Fatalf("a folder mirror was accepted on iOS: %s %v", raw, err)
	}
}
