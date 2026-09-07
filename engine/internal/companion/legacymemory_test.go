package companion

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeWorkspace lays down dir/memory/experiences.jsonl with body.
func writeWorkspace(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "memory"), 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "memory", "experiences.jsonl"), []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", dir, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func mustNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("%s still exists (err=%v)", path, err)
	}
}

// The whole point of the issue: an orphaned workspace is moved, byte for byte.
func TestMigrateLegacyMemoryMovesOrphanedWorkspace(t *testing.T) {
	home := t.TempDir()
	const id = "6f1b0e2a-0000-4000-8000-000000000001"
	const body = "{\"summary\":\"Mira never drinks before a job\"}\n{\"summary\":\"the harbour bell is cracked\"}\n"
	writeWorkspace(t, filepath.Join(home, "companion", id), body)

	rep := MigrateLegacyMemory(home)

	if got := rep.Moved; len(got) != 1 || got[0] != id {
		t.Fatalf("Moved = %v, want [%s]", got, id)
	}
	if len(rep.Failed) != 0 {
		t.Fatalf("Failed = %v, want none", rep.Failed)
	}
	moved := filepath.Join(home, id, "memory", "experiences.jsonl")
	if got := readFile(t, moved); got != body {
		t.Fatalf("moved content = %q, want %q", got, body)
	}
	mustNotExist(t, filepath.Join(home, "companion"))
	// The memRoot the service actually reads is where it landed.
	if want := memRoot(home, id); want != filepath.Join(home, id) {
		t.Fatalf("memRoot(%s, %s) = %s, want the migration destination", home, id, want)
	}
}

// The migration must never merge. Both files keep exactly what they held.
func TestMigrateLegacyMemoryLeavesExistingDestinationAlone(t *testing.T) {
	home := t.TempDir()
	const id = "6f1b0e2a-0000-4000-8000-000000000002"
	const oldBody = "{\"summary\":\"remembered before 1.0\"}\n"
	const newBody = "{\"summary\":\"remembered after 1.0\"}\n"
	writeWorkspace(t, filepath.Join(home, "companion", id), oldBody)
	writeWorkspace(t, filepath.Join(home, id), newBody)

	rep := MigrateLegacyMemory(home)

	if len(rep.Moved) != 0 {
		t.Fatalf("Moved = %v, want none", rep.Moved)
	}
	if len(rep.Skipped) != 1 || !strings.Contains(rep.Skipped[0], "already exists") {
		t.Fatalf("Skipped = %v, want one already-exists line", rep.Skipped)
	}
	if got := readFile(t, filepath.Join(home, "companion", id, "memory", "experiences.jsonl")); got != oldBody {
		t.Fatalf("legacy content = %q, want %q", got, oldBody)
	}
	if got := readFile(t, filepath.Join(home, id, "memory", "experiences.jsonl")); got != newBody {
		t.Fatalf("current content = %q, want %q", got, newBody)
	}
	if rep.RootRemoved {
		t.Fatal("RootRemoved = true, but the old root still holds the untouched workspace")
	}
}

// Only a memory workspace moves. Anything else under the old root stays.
func TestMigrateLegacyMemorySkipsNonWorkspaces(t *testing.T) {
	home := t.TempDir()
	legacy := filepath.Join(home, "companion")
	// A directory with no marker file.
	if err := os.MkdirAll(filepath.Join(legacy, "notes", "memory"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// A plain file.
	if err := os.WriteFile(filepath.Join(legacy, "stray.txt"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	rep := MigrateLegacyMemory(home)

	if len(rep.Moved) != 0 {
		t.Fatalf("Moved = %v, want none", rep.Moved)
	}
	mustNotExist(t, filepath.Join(home, "notes"))
	mustNotExist(t, filepath.Join(home, "stray.txt"))
	if _, err := os.Stat(filepath.Join(legacy, "notes")); err != nil {
		t.Fatalf("notes/ should still be under the old root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(legacy, "stray.txt")); err != nil {
		t.Fatalf("stray.txt should still be under the old root: %v", err)
	}
	if rep.RootRemoved {
		t.Fatal("RootRemoved = true, but the old root still holds leftovers")
	}
}

// A symlinked entry is not followed and not moved.
func TestMigrateLegacyMemoryRefusesSymlinkedEntry(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	const id = "6f1b0e2a-0000-4000-8000-000000000003"
	writeWorkspace(t, filepath.Join(outside, id), "{\"summary\":\"outside the home\"}\n")
	if err := os.MkdirAll(filepath.Join(home, "companion"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, id), filepath.Join(home, "companion", id)); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	rep := MigrateLegacyMemory(home)

	if len(rep.Moved) != 0 {
		t.Fatalf("Moved = %v, want none", rep.Moved)
	}
	if len(rep.Skipped) != 1 || !strings.Contains(rep.Skipped[0], "symlink") {
		t.Fatalf("Skipped = %v, want one symlink line", rep.Skipped)
	}
	mustNotExist(t, filepath.Join(home, id))
	// The link and its target are both untouched.
	if _, err := os.Lstat(filepath.Join(home, "companion", id)); err != nil {
		t.Fatalf("the symlink should still be there: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, id, "memory", "experiences.jsonl")); err != nil {
		t.Fatalf("the target should be untouched: %v", err)
	}
}

// The marker must be the real log. A symlink in its place does not vouch for
// a directory, for the same reason a symlinked entry is not moved.
func TestMigrateLegacyMemoryRefusesSymlinkedMarker(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	const id = "6f1b0e2a-0000-4000-8000-000000000007"
	target := filepath.Join(outside, "experiences.jsonl")
	if err := os.WriteFile(target, []byte("{\"summary\":\"elsewhere\"}\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	dir := filepath.Join(home, "companion", id, "memory")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "experiences.jsonl")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	rep := MigrateLegacyMemory(home)

	if len(rep.Moved) != 0 {
		t.Fatalf("Moved = %v, want none", rep.Moved)
	}
	if len(rep.Skipped) != 1 || !strings.Contains(rep.Skipped[0], "no memory") {
		t.Fatalf("Skipped = %v, want one no-marker line", rep.Skipped)
	}
	mustNotExist(t, filepath.Join(home, id))
}

// A reserved name is never a destination, even when nothing occupies it yet.
func TestMigrateLegacyMemoryRefusesReservedNames(t *testing.T) {
	home := t.TempDir()
	for name := range reservedHomeNames {
		if name == "companion" {
			continue // that is the old root itself, not an entry under it
		}
		writeWorkspace(t, filepath.Join(home, "companion", name), "{\"summary\":\""+name+"\"}\n")
	}

	rep := MigrateLegacyMemory(home)

	if len(rep.Moved) != 0 {
		t.Fatalf("Moved = %v, want none", rep.Moved)
	}
	for name := range reservedHomeNames {
		if name == "companion" {
			continue
		}
		mustNotExist(t, filepath.Join(home, name))
		if _, err := os.Stat(filepath.Join(home, "companion", name, "memory", "experiences.jsonl")); err != nil {
			t.Fatalf("%s should still be under the old root: %v", name, err)
		}
	}
	// library.db is on the list because this migration runs before the store
	// creates it: existence alone would not have stopped that rename.
	if !reservedHomeNames["library.db"] || !reservedHomeNames["skills"] {
		t.Fatal("library.db and skills must both be reserved")
	}
}

// Path safety, at the level the code can be asked about directly: nothing but
// a plain single segment may name a destination.
func TestPlainSegment(t *testing.T) {
	good := []string{"6f1b0e2a-0000-4000-8000-000000000004", "a", ".hidden", "with space"}
	bad := []string{"", ".", "..", "../escape", "a/b", `a\b`, "/abs", string(filepath.Separator)}
	for _, name := range good {
		if !plainSegment(name) {
			t.Errorf("plainSegment(%q) = false, want true", name)
		}
	}
	for _, name := range bad {
		if plainSegment(name) {
			t.Errorf("plainSegment(%q) = true, want false", name)
		}
	}
}

func TestMigrateLegacyMemoryNoLegacyRootIsANoOp(t *testing.T) {
	home := t.TempDir()

	rep := MigrateLegacyMemory(home)

	if !rep.Empty() {
		t.Fatalf("report = %+v, want empty", rep)
	}
	if len(rep.Lines()) != 0 {
		t.Fatalf("Lines() = %v, want none", rep.Lines())
	}
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("home gained %d entries, want none", len(entries))
	}
}

// An empty old root is removed; the two mixed cases in one pass.
func TestMigrateLegacyMemoryRemovesEmptiedRoot(t *testing.T) {
	home := t.TempDir()
	const id = "6f1b0e2a-0000-4000-8000-000000000005"
	writeWorkspace(t, filepath.Join(home, "companion", id), "{\"summary\":\"only tenant\"}\n")

	rep := MigrateLegacyMemory(home)

	if !rep.RootRemoved {
		t.Fatalf("RootRemoved = false, report = %+v", rep)
	}
	mustNotExist(t, filepath.Join(home, "companion"))
}

// One entry failing to move must not stop the others, and must be reported.
func TestMigrateLegacyMemoryReportsFailureAndContinues(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores the permission bits this test relies on")
	}
	home := t.TempDir()
	const blocked = "6f1b0e2a-0000-4000-8000-000000000006"
	writeWorkspace(t, filepath.Join(home, "companion", blocked), "{\"summary\":\"unreachable\"}\n")
	// A read-only old root: entries can be listed and read, but nothing can be
	// renamed out of it.
	legacy := filepath.Join(home, "companion")
	t.Cleanup(func() { _ = os.Chmod(legacy, 0o700) })
	if err := os.Chmod(legacy, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	rep := MigrateLegacyMemory(home)

	if len(rep.Moved) != 0 {
		t.Fatalf("Moved = %v, want none", rep.Moved)
	}
	if len(rep.Failed) != 1 || !strings.Contains(rep.Failed[0], blocked) {
		t.Fatalf("Failed = %v, want one line naming %s", rep.Failed, blocked)
	}
	if rep.Empty() {
		t.Fatal("Empty() = true, but the pass had a failure to report")
	}
	if lines := rep.Lines(); len(lines) != 1 || !strings.Contains(lines[0], "could not move") {
		t.Fatalf("Lines() = %v, want one could-not-move line", lines)
	}
	// The data is still where it was.
	if _, err := os.Stat(filepath.Join(legacy, blocked, "memory", "experiences.jsonl")); err != nil {
		t.Fatalf("the source should be intact: %v", err)
	}
}

// A real directory whose memory/ is a symlink pointing out of the app data
// directory. One Lstat of "<dir>/memory/experiences.jsonl" resolves memory/
// and only leaves the last component alone, so this entry used to pass the
// marker check and get moved — after which <home>/<id>/memory was a link out
// of the home that every later memory write followed (#114 review).
func TestMigrateLegacyMemoryRefusesSymlinkedMemoryDirectory(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	const id = "6f1b0e2a-0000-4000-8000-000000000008"
	if err := os.WriteFile(filepath.Join(outside, "experiences.jsonl"), []byte("{\"summary\":\"elsewhere\"}\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	entry := filepath.Join(home, "companion", id)
	if err := os.MkdirAll(entry, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(entry, "memory")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	rep := MigrateLegacyMemory(home)

	if len(rep.Moved) != 0 {
		t.Fatalf("Moved = %v, want none — moving this makes <home>/%s/memory a link out of the app data directory", rep.Moved, id)
	}
	if len(rep.Skipped) != 1 || !strings.Contains(rep.Skipped[0], "no memory") {
		t.Fatalf("Skipped = %v, want one no-marker line", rep.Skipped)
	}
	mustNotExist(t, filepath.Join(home, id))
	// And the entry it refused is untouched, link and all.
	if _, err := os.Lstat(filepath.Join(entry, "memory")); err != nil {
		t.Fatalf("the refused entry was disturbed: %v", err)
	}
}
