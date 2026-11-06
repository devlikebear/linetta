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

func TestHomeFor(t *testing.T) {
	cases := []struct {
		name, appID, envVar, envVal string
		wantEnv                     bool
	}{
		{"env override wins", "com.example.clio", "CLIO_HOME", "/custom/clio", true},
		{"env unset falls back to OS default", "com.example.clio", "CLIO_HOME", "", false},
		{"empty envVar means no override", "com.example.clio", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.envVar != "" {
				t.Setenv(c.envVar, c.envVal)
			}
			// A stray LINETTA_HOME must not leak into other apps.
			t.Setenv("LINETTA_HOME", "/linetta/should/not/apply")
			got, err := HomeFor(c.appID, c.envVar)
			if err != nil {
				t.Fatalf("HomeFor: %v", err)
			}
			if c.wantEnv {
				if got != c.envVal {
					t.Errorf("got %q, want %q", got, c.envVal)
				}
				return
			}
			if filepath.Base(got) != c.appID {
				t.Errorf("got %q, want base %q", got, c.appID)
			}
		})
	}
}
