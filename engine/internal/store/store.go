package store

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/devlikebear/linetta/engine/internal/backup"
	_ "modernc.org/sqlite"
)

// Store wraps the SQLite handle with helpers and a guaranteed migration pass.
type Store struct {
	db *sql.DB
}

// Open opens or creates the database at path, applies pragmas, runs migrations,
// and returns the Store. Closes any partial state on error.
func Open(ctx context.Context, path string) (*Store, error) {
	fsys, err := fs.Sub(MigrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("embedded migrations: %w", err)
	}
	return OpenWith(ctx, path, fsys)
}

// OpenWith is Open with an injected migration chain. migrations holds the
// `NNNN_name.sql` files directly at its root (no subdirectory); the highest
// version found there is the target version used for the pre-migration backup.
// A sibling app supplies its own chain so its database never gets Linetta's
// tables.
func OpenWith(ctx context.Context, path string, migrations fs.FS) (*Store, error) {
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
	if pending, targetVersion, err := migrationPendingFS(ctx, db, migrations); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("inspect migrations: %w", err)
	} else if pending {
		home := filepath.Dir(path)
		if _, err := backup.RunPreMigration(ctx, db, home, targetVersion, time.Now()); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("pre-migration backup: %w", err)
		}
	}
	if err := ApplyMigrationsFS(ctx, db, migrations); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

// DB exposes the raw *sql.DB for repos that need it.
func (s *Store) DB() *sql.DB { return s.db }

// Close shuts down the underlying connection pool.
func (s *Store) Close() error { return s.db.Close() }
