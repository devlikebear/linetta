package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHome_envOverride(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LINETTA_HOME", tmp)

	got, err := Home()
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	if got != tmp {
		t.Errorf("Home = %q, want %q", got, tmp)
	}
}

func TestHome_default_macos(t *testing.T) {
	t.Setenv("LINETTA_HOME", "")
	got, err := Home()
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	// We don't assert the exact value (it's user-dependent) — only that it
	// ends with the identifier and is under the user's home.
	home, _ := os.UserHomeDir()
	if !strings.HasPrefix(got, home) {
		t.Errorf("Home %q should be under user home %q", got, home)
	}
	if filepath.Base(got) != "com.devlikebear.linetta" {
		t.Errorf("Home base = %q, want com.devlikebear.linetta", filepath.Base(got))
	}
}

func TestDBPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LINETTA_HOME", tmp)
	got, err := DBPath()
	if err != nil {
		t.Fatalf("DBPath: %v", err)
	}
	want := filepath.Join(tmp, "library.db")
	if got != want {
		t.Errorf("DBPath = %q, want %q", got, want)
	}
}

func TestEnsureHome_createsDir(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "nested", "linetta")
	t.Setenv("LINETTA_HOME", tmp)

	if err := EnsureHome(); err != nil {
		t.Fatalf("EnsureHome: %v", err)
	}
	info, err := os.Stat(tmp)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("%q is not a directory", tmp)
	}
}
