package settings

import (
	"context"
	"testing"
)

func TestFolderSyncPatchRoundTrip(t *testing.T) {
	t.Setenv("LINETTA_HOME", t.TempDir())
	st, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	dir := "/tmp/linetta-folder-sync"
	enabled := true
	if _, err := st.Set(ctx, Patch{FolderSyncDir: &dir, FolderSyncEnabled: &enabled}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	cfg, err := st.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if cfg.FolderSyncDir != dir {
		t.Errorf("FolderSyncDir = %q, want %q", cfg.FolderSyncDir, dir)
	}
	if !cfg.FolderSyncEnabled {
		t.Errorf("FolderSyncEnabled = false, want true")
	}
}
