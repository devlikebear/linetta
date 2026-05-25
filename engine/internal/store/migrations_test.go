package store

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestApplyMigrations_appliesOnce(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("second apply (idempotent): %v", err)
	}

	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n < 1 {
		t.Errorf("expected at least one migration recorded, got %d", n)
	}
}

func TestApplyMigrations_createsProjectsTable(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply: %v", err)
	}
	// Insert a sentinel row matching the schema.
	_, err = db.ExecContext(ctx, `
INSERT INTO projects (id, title, genres, length_target, default_pov, created_at, updated_at)
VALUES ('p1', 'Test', '["SF"]', 'novel', 'first', 0, 0)`)
	if err != nil {
		t.Fatalf("insert into projects: %v", err)
	}
}
