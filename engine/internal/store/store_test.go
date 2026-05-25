package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpen_appliesMigrations_andEnablesFKs(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	ctx := context.Background()

	s, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	// projects table exists?
	if _, err := s.DB().ExecContext(ctx, `
INSERT INTO projects (id, title, genres, length_target, default_pov, created_at, updated_at)
VALUES ('p1', 'Hello', '["SF"]', 'novel', 'first', 0, 0)`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// foreign_keys ON?
	var fk int
	if err := s.DB().QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fk); err != nil {
		t.Fatalf("pragma: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}

	// journal_mode WAL?
	var jm string
	if err := s.DB().QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&jm); err != nil {
		t.Fatalf("pragma: %v", err)
	}
	if jm != "wal" {
		t.Errorf("journal_mode = %q, want wal", jm)
	}
}

func TestOpen_idempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	ctx := context.Background()

	s1, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open #1: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open #2: %v", err)
	}
	defer s2.Close()
}
