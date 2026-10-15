//go:build !mas && !mobile && windows

package clidetect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Windows reports mode 0666 for regular files, so an executable test based on
// the Unix permission bits rejects every real claude.exe. Guard against that
// regression: a plain file with an executable extension must pass.
func TestIsExecutable_acceptsExeWithoutUnixPermissionBits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "claude.exe")
	if err := os.WriteFile(path, []byte("stub"), 0o644); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat stub: %v", err)
	}
	if info.Mode()&0o111 != 0 {
		t.Skip("this Windows filesystem reports Unix execute bits; the regression cannot occur here")
	}
	if !isExecutable(path) {
		t.Fatalf("isExecutable(%q) = false, want true", path)
	}
}

func TestIsExecutable_rejectsNonExecutableExtensions(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"claude", "claude.txt", "claude.md"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("stub"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		if isExecutable(path) {
			t.Errorf("isExecutable(%q) = true, want false", path)
		}
	}
}

func TestIsExecutable_rejectsDirectoriesAndMissingPaths(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "claude.exe")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if isExecutable(sub) {
		t.Error("isExecutable(dir) = true, want false")
	}
	if isExecutable(filepath.Join(dir, "missing.exe")) {
		t.Error("isExecutable(missing) = true, want false")
	}
	if isExecutable("   ") {
		t.Error("isExecutable(blank) = true, want false")
	}
}

func TestIsExecutable_honoursPathExt(t *testing.T) {
	t.Setenv("PATHEXT", ".COM;.EXE;.PS1")
	dir := t.TempDir()
	path := filepath.Join(dir, "claude.ps1")
	if err := os.WriteFile(path, []byte("stub"), 0o644); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	if !isExecutable(path) {
		t.Fatalf("isExecutable(%q) = false, want true with PATHEXT override", path)
	}
}

// Bare `claude` is not runnable on Windows, so every candidate needs a suffix.
func TestKnownPaths_allCarryAnExecutableExtension(t *testing.T) {
	paths := knownPaths(`C:\Users\writer`)
	if len(paths) == 0 {
		t.Fatal("knownPaths returned no candidates")
	}
	for _, p := range paths {
		if filepath.Ext(p) == "" {
			t.Errorf("candidate %q has no extension", p)
		}
		if !strings.EqualFold(strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)), "claude") {
			t.Errorf("candidate %q does not point at claude", p)
		}
	}
}
