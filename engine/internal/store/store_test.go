package store

import (
	"context"
	"path/filepath"
	"testing"
	"testing/fstest"
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

func TestOpenWith_injectedChain(t *testing.T) {
	ctx := context.Background()
	fsys := fstest.MapFS{
		"0001_a.sql": {Data: []byte(`CREATE TABLE a (id INTEGER PRIMARY KEY);`)},
		"0002_b.sql": {Data: []byte(`CREATE TABLE b (id INTEGER PRIMARY KEY);`)},
	}
	s, err := OpenWith(ctx, filepath.Join(t.TempDir(), "x.db"), fsys)
	if err != nil {
		t.Fatalf("OpenWith: %v", err)
	}
	defer s.Close()

	count := func(q string) int {
		var n int
		if err := s.DB().QueryRowContext(ctx, q).Scan(&n); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return n
	}
	for _, tbl := range []string{"a", "b"} {
		if count(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='`+tbl+`'`) != 1 {
			t.Errorf("table %s missing", tbl)
		}
	}
	if n := count(`SELECT COUNT(*) FROM schema_migrations WHERE version IN (1,2)`); n != 2 {
		t.Errorf("schema_migrations rows = %d, want 2", n)
	}
	if n := count(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='projects'`); n != 0 {
		t.Errorf("linetta table projects must not exist")
	}
}
