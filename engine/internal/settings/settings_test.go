package settings

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func newStoreOnTemp(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("LINETTA_HOME", dir)
	s, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestLoad_missingFileReturnsDefaults(t *testing.T) {
	s := newStoreOnTemp(t)
	got, err := s.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Provider != "claude-code-cli" {
		t.Errorf("provider = %q, want claude-code-cli", got.Provider)
	}
	if got.TypewriterDefault != false {
		t.Errorf("typewriter_default = %v", got.TypewriterDefault)
	}
	if got.BackupDir == "" {
		t.Error("backup_dir empty")
	}
}

func TestSet_partialPatchPreservesUntouchedFields(t *testing.T) {
	s := newStoreOnTemp(t)
	if _, err := s.Set(context.Background(), Patch{TypewriterDefault: boolPtr(true)}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, _ := s.Get(context.Background())
	if got.Provider != "claude-code-cli" {
		t.Errorf("provider mutated to %q", got.Provider)
	}
	if !got.TypewriterDefault {
		t.Errorf("typewriter_default not persisted")
	}
}

func TestSet_persistsToDisk(t *testing.T) {
	s := newStoreOnTemp(t)
	if _, err := s.Set(context.Background(), Patch{Provider: strPtr("openai-codex")}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	s2, err := New()
	if err != nil {
		t.Fatalf("re-New: %v", err)
	}
	got, _ := s2.Get(context.Background())
	if got.Provider != "openai-codex" {
		t.Errorf("provider not persisted across reload: %q", got.Provider)
	}
}

func TestSet_rejectsUnknownProvider(t *testing.T) {
	s := newStoreOnTemp(t)
	_, err := s.Set(context.Background(), Patch{Provider: strPtr("bad-provider")})
	if err == nil {
		t.Error("expected validation error")
	}
}

func TestLoad_corruptFileReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINETTA_HOME", dir)
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte("not-json"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s, err := New()
	if err != nil {
		t.Fatalf("New on corrupt: %v", err)
	}
	got, _ := s.Get(context.Background())
	if got.Provider != "claude-code-cli" {
		t.Errorf("did not fall back to defaults: %+v", got)
	}
}

func TestSet_concurrentSerialized(t *testing.T) {
	s := newStoreOnTemp(t)
	const N = 20
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			tw := i%2 == 0
			_, _ = s.Set(context.Background(), Patch{TypewriterDefault: &tw})
		}(i)
	}
	wg.Wait()
	// Reload from disk and verify it equals the in-memory state.
	s2, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	onDisk, _ := s2.Get(context.Background())
	inMem, _ := s.Get(context.Background())
	if onDisk.TypewriterDefault != inMem.TypewriterDefault {
		t.Fatalf("disk/memory mismatch after concurrent Set: disk=%v mem=%v", onDisk.TypewriterDefault, inMem.TypewriterDefault)
	}
}

func TestSet_focusDefault_persists(t *testing.T) {
	s := newStoreOnTemp(t)
	if _, err := s.Set(context.Background(), Patch{FocusDefault: boolPtr(true)}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, _ := s.Get(context.Background())
	if !got.FocusDefault {
		t.Errorf("focus_default not applied in-memory: %+v", got)
	}
	// Reload from disk and verify it survived.
	s2, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	reloaded, _ := s2.Get(context.Background())
	if !reloaded.FocusDefault {
		t.Errorf("focus_default not persisted across reload: %+v", reloaded)
	}
}

func boolPtr(v bool) *bool    { return &v }
func strPtr(v string) *string { return &v }
