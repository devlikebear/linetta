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

func TestApplyMigrations_createsFactBookTablesWithSourceCascade(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys=ON`); err != nil {
		t.Fatalf("pragma: %v", err)
	}
	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply: %v", err)
	}

	_, err = db.ExecContext(ctx, `
INSERT INTO projects (id, title, genres, length_target, default_pov, created_at, updated_at)
VALUES ('p1', 'Test', '["SF"]', 'novel', 'first', 0, 0)`)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO fact_cards (id, project_id, claim, result, status, category, created_at, updated_at)
VALUES ('f1', 'p1', '주장', '검증 결과', 'verified', 'world', 10, 10)`)
	if err != nil {
		t.Fatalf("insert fact card: %v", err)
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO fact_sources (id, card_id, url, title, snippet, accessed_at)
VALUES ('s1', 'f1', 'https://example.com', 'Example', 'short excerpt', 10)`)
	if err != nil {
		t.Fatalf("insert fact source: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM fact_cards WHERE id = 'f1'`); err != nil {
		t.Fatalf("delete fact card: %v", err)
	}
	var sourceCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM fact_sources WHERE card_id = 'f1'`).Scan(&sourceCount); err != nil {
		t.Fatalf("count sources: %v", err)
	}
	if sourceCount != 0 {
		t.Fatalf("source count after card delete = %d, want 0", sourceCount)
	}
}
