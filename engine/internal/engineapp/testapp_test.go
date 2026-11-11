package engineapp

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/linetta/engine/internal/store/storetest"
)

// openTestApp is how a test in this package opens an App: rooted at home, with
// an in-memory secret store, closed when the test ends. Use it instead of
// calling Open directly — the OS keychain is process-global and not scoped by
// Home, so an App opened without Secrets must not be what a test asserts on.
// A home with no library yet gets an already-migrated one (see storetest).
func openTestApp(t *testing.T, home string) *App {
	t.Helper()
	if err := storetest.Seed(filepath.Join(home, "library.db")); err != nil {
		t.Fatalf("seed library: %v", err)
	}
	app, err := Open(context.Background(), Options{Home: home, Secrets: settings.NewMemorySecretStore()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return app
}
