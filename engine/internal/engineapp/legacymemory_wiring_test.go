package engineapp

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/settings"
)

func seedLegacyWorkspace(t *testing.T, home, id, body string) {
	t.Helper()
	dir := filepath.Join(home, "companion", id, "memory")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "experiences.jsonl"), []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// Open recovers the orphaned workspace on the way up, before anything reads
// the new location.
func TestOpenMigratesLegacyCompanionMemory(t *testing.T) {
	home := t.TempDir()
	const id = "0f3a5c11-0000-4000-8000-0000000000a1"
	const body = "{\"summary\":\"the lighthouse keeper is her brother\"}\n"
	seedLegacyWorkspace(t, home, id, body)

	app, err := Open(context.Background(), Options{Home: home, Secrets: settings.NewMemorySecretStore()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer app.Close()

	got, err := os.ReadFile(filepath.Join(home, id, "memory", "experiences.jsonl"))
	if err != nil {
		t.Fatalf("the workspace was not moved: %v", err)
	}
	if string(got) != body {
		t.Fatalf("content = %q, want %q", got, body)
	}
	if _, err := os.Lstat(filepath.Join(home, "companion")); !os.IsNotExist(err) {
		t.Fatalf("the emptied old root should be gone (err=%v)", err)
	}
}

// A migration that cannot do its job must not keep the app from starting.
func TestOpenSurvivesFailedLegacyMemoryMigration(t *testing.T) {
	// Both skips are about how the failure is INDUCED, not about what is under
	// test: Open surviving a failed migration is platform-independent, and
	// CI's Linux leg exercises it. Windows does not honour a directory's
	// permission bits, so the rename below simply succeeds -- and the test
	// then fails on its own premise, looking for a source it expected to still
	// be there.
	if runtime.GOOS == "windows" {
		t.Skip("windows ignores directory permission bits, so a read-only old root does not block the rename")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores the permission bits this test relies on")
	}
	home := t.TempDir()
	const id = "0f3a5c11-0000-4000-8000-0000000000a2"
	seedLegacyWorkspace(t, home, id, "{\"summary\":\"stuck\"}\n")
	legacy := filepath.Join(home, "companion")
	t.Cleanup(func() { _ = os.Chmod(legacy, 0o700) })
	if err := os.Chmod(legacy, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	app, err := Open(context.Background(), Options{Home: home, Secrets: settings.NewMemorySecretStore()})
	if err != nil {
		t.Fatalf("Open must survive a failed migration, got: %v", err)
	}
	defer app.Close()

	resp, err := app.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if want := `"pong"`; !strings.Contains(string(resp), want) {
		t.Fatalf("ping response = %s, want it to contain %s", resp, want)
	}
	// Nothing was lost on the way.
	if _, err := os.Stat(filepath.Join(legacy, id, "memory", "experiences.jsonl")); err != nil {
		t.Fatalf("the source should be intact: %v", err)
	}
}
