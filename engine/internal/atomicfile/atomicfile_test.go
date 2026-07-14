package atomicfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWritePublishesCompleteReplacementWithoutTempFiles(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "project.md")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	content := strings.Repeat("new manuscript\n", 1024)
	if err := Write(target, []byte(content), 0o644); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != content {
		t.Fatalf("published content length = %d, want %d", len(got), len(content))
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "project.md" {
		t.Fatalf("unexpected files after publish: %v", entries)
	}
}
