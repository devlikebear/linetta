package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenCreatesParentDirectoryAndEnablesForeignKeys(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nested", "linetta.db")

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	var enabled int
	if err := db.conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&enabled); err != nil {
		t.Fatalf("PRAGMA foreign_keys scan error = %v", err)
	}
	if enabled != 1 {
		t.Fatalf("foreign_keys = %d, want 1", enabled)
	}
}

func TestMigrateCreatesLibraryTablesIdempotently(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "linetta.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	for i := 0; i < 2; i++ {
		if err := db.Migrate(ctx); err != nil {
			t.Fatalf("Migrate() attempt %d error = %v", i+1, err)
		}
	}

	for _, table := range []string{"works", "episodes", "agent_runs", "agent_run_events"} {
		if !tableExists(t, db, table) {
			t.Fatalf("expected table %q to exist", table)
		}
	}
}

func tableExists(t *testing.T, db *DB, name string) bool {
	t.Helper()
	var got string
	err := db.conn.QueryRowContext(
		context.Background(),
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`,
		name,
	).Scan(&got)
	return err == nil && got == name
}
