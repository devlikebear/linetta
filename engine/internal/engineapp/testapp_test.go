package engineapp

import (
	"context"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/settings"
)

// openTestApp is how a test in this package opens an App: rooted at home, with
// an in-memory secret store, closed when the test ends. Use it instead of
// calling Open directly — the OS keychain is process-global and not scoped by
// Home, so an App opened without Secrets must not be what a test asserts on.
func openTestApp(t *testing.T, home string) *App {
	t.Helper()
	app, err := Open(context.Background(), Options{Home: home, Secrets: settings.NewMemorySecretStore()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return app
}
