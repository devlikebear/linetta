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

func TestApplyMigrations_rejectsChangedAppliedMigrationChecksum(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE schema_migrations SET checksum = 'tampered' WHERE version = (SELECT MIN(version) FROM schema_migrations)`); err != nil {
		t.Fatalf("tamper checksum: %v", err)
	}
	if err := ApplyMigrations(ctx, db); err == nil {
		t.Fatal("expected changed migration checksum to be rejected")
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

func TestApplyMigrations_createsCompanionMessagesWithProjectCascade(t *testing.T) {
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
INSERT INTO nodes (id, project_id, parent_id, ordinal, kind, label, title, content_doc, created_at, updated_at)
VALUES ('n1', 'p1', NULL, 0, 'leaf', '씬 1', '시작', '{}', 0, 0)`)
	if err != nil {
		t.Fatalf("insert node: %v", err)
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO companion_messages (id, project_id, node_id, run_id, role, scope, intent, status, content, created_at)
VALUES ('m1', 'p1', 'n1', 'r1', 'user', 'scene', 'scene_write', 'done', '써줘', 10)`)
	if err != nil {
		t.Fatalf("insert companion message: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM projects WHERE id = 'p1'`); err != nil {
		t.Fatalf("delete project: %v", err)
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM companion_messages WHERE project_id = 'p1'`).Scan(&n); err != nil {
		t.Fatalf("count companion messages: %v", err)
	}
	if n != 0 {
		t.Fatalf("companion message count after project delete = %d, want 0", n)
	}
}

func TestApplyMigrations_createsCompanionReferencesWithProjectCascade(t *testing.T) {
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
INSERT INTO companion_references
  (id, project_id, source_type, purpose, title, content, summary, char_count, token_estimate, status, created_at, updated_at)
VALUES ('r1', 'p1', 'text', 'style', 'Tone', 'body', '', 4, 2, 'active', 10, 10)`)
	if err != nil {
		t.Fatalf("insert companion reference: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM projects WHERE id = 'p1'`); err != nil {
		t.Fatalf("delete project: %v", err)
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM companion_references WHERE project_id = 'p1'`).Scan(&n); err != nil {
		t.Fatalf("count companion references: %v", err)
	}
	if n != 0 {
		t.Fatalf("companion reference count after project delete = %d, want 0", n)
	}
}

func TestApplyMigrations_createsManuscriptFTS(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply: %v", err)
	}

	var sqlText string
	if err := db.QueryRowContext(ctx, `
SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'manuscript_fts'`).Scan(&sqlText); err != nil {
		t.Fatalf("manuscript_fts schema: %v", err)
	}
	if sqlText == "" {
		t.Fatal("manuscript_fts schema is empty")
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO manuscript_fts (plain, node_id, project_id)
VALUES ('진홍빛 눈동자', 'n1', 'p1')`); err != nil {
		t.Fatalf("insert manuscript_fts: %v", err)
	}
	var nodeID string
	if err := db.QueryRowContext(ctx, `
SELECT node_id FROM manuscript_fts WHERE manuscript_fts MATCH '진홍빛'`).Scan(&nodeID); err != nil {
		t.Fatalf("match manuscript_fts: %v", err)
	}
	if nodeID != "n1" {
		t.Fatalf("nodeID = %q, want n1", nodeID)
	}
}

func TestMigrationPending_detectsMissingEmbeddedVersion(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("apply: %v", err)
	}
	pending, latest, err := migrationPending(ctx, db)
	if err != nil || pending {
		t.Fatalf("fully migrated = pending %v latest %d err %v", pending, latest, err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = ?`, latest); err != nil {
		t.Fatalf("delete latest version: %v", err)
	}
	pending, gotLatest, err := migrationPending(ctx, db)
	if err != nil || !pending || gotLatest != latest {
		t.Fatalf("missing migration = pending %v latest %d err %v", pending, gotLatest, err)
	}
}
