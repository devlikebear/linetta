package store

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Store wraps the SQLite handle with helpers and a guaranteed migration pass.
type Store struct {
	db *sql.DB
}

// Open opens or creates the database at path, applies pragmas, runs migrations,
// and returns the Store. Closes any partial state on error.
func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	// SQLite allows only one writer at a time. With WAL and Go's connection
	// pool, two concurrent write transactions land on separate connections and
	// SQLite returns SQLITE_BUSY (BUSY_SNAPSHOT), which busy_timeout will not
	// retry. The engine runs every RPC request and several background jobs
	// (backup, snapshot thinning, summarizer) as goroutines sharing this pool,
	// so capping it to a single connection serializes all access and removes
	// the contention entirely. It also guarantees the per-connection pragmas
	// below stay applied.
	db.SetMaxOpenConns(1)
	for _, pragma := range []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA synchronous=NORMAL`,
		`PRAGMA foreign_keys=ON`,
	} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("pragma %q: %w", pragma, err)
		}
	}
	if err := ApplyMigrations(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

// DB exposes the raw *sql.DB for repos that need it.
func (s *Store) DB() *sql.DB { return s.db }

// Close shuts down the underlying connection pool.
func (s *Store) Close() error { return s.db.Close() }
