//go:build !mas

package provider

import (
	"os"
	"path/filepath"
	"testing"
)

func writeAuth(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(`{"tokens":{}}`), 0o600); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
}

func TestResolveCodexHome_prefersLinettasOwnLogin(t *testing.T) {
	fakeHome := t.TempDir()
	isolateHomeDir(t, fakeHome)
	linetta := filepath.Join(t.TempDir(), "codex")
	writeAuth(t, linetta)
	writeAuth(t, filepath.Join(fakeHome, ".codex"))

	if got := resolveCodexHome(linetta); got != linetta {
		t.Errorf("resolveCodexHome = %q, want Linetta's own %q", got, linetta)
	}
}

func TestResolveCodexHome_fallsBackToTheCodexCLI(t *testing.T) {
	fakeHome := t.TempDir()
	isolateHomeDir(t, fakeHome)
	linetta := filepath.Join(t.TempDir(), "codex") // no auth.json written
	cli := filepath.Join(fakeHome, ".codex")
	writeAuth(t, cli)

	if got := resolveCodexHome(linetta); got != cli {
		t.Errorf("resolveCodexHome = %q, want the CLI's %q", got, cli)
	}
}

func TestResolveCodexHome_withNoLoginAnywhereReturnsLinettas(t *testing.T) {
	isolateHomeDir(t, t.TempDir())
	linetta := filepath.Join(t.TempDir(), "codex")

	// A future login has to land in Linetta's own directory, so that is the
	// answer when neither side has a credential yet.
	if got := resolveCodexHome(linetta); got != linetta {
		t.Errorf("resolveCodexHome = %q, want %q", got, linetta)
	}
}

func TestResolveCodexHome_emptyInputStaysEmpty(t *testing.T) {
	isolateHomeDir(t, t.TempDir())
	if got := resolveCodexHome(""); got != "" {
		t.Errorf("resolveCodexHome(\"\") = %q, want empty so Configured() stays false", got)
	}
}
