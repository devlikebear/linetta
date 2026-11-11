package storetest

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenStartsANewPathFullyMigrated(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "nested", "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var n int
	if err := st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM projects`).Scan(&n); err != nil {
		t.Fatalf("projects table missing: %v", err)
	}
}

func TestSeedLeavesAnExistingFileAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.db")
	if err := os.WriteFile(path, []byte("legacy"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Seed(path); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "legacy" {
		t.Fatalf("Seed overwrote an existing file: %q", got)
	}
}

// Each copy is its own database: what one test writes, another never sees.
func TestCopiesDoNotShareRows(t *testing.T) {
	ctx := context.Background()
	a, err := Open(ctx, filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if _, err := a.DB().ExecContext(ctx, `CREATE TABLE only_in_a (x INTEGER)`); err != nil {
		t.Fatal(err)
	}
	b, err := Open(ctx, filepath.Join(t.TempDir(), "b.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	var n int
	if err := b.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE name = 'only_in_a'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("a table created in one copy showed up in another")
	}
}
