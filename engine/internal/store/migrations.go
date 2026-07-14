package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ApplyMigrations runs every embedded migration whose version is greater than
// the highest version already recorded in schema_migrations. Idempotent: a
// second call is a no-op.
func ApplyMigrations(ctx context.Context, db *sql.DB) error {
	if err := ensureMigrationTable(ctx, db); err != nil {
		return err
	}
	applied, err := appliedVersions(ctx, db)
	if err != nil {
		return err
	}

	entries, err := fs.ReadDir(MigrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		version, err := parseVersion(e.Name())
		if err != nil {
			return err
		}
		body, err := fs.ReadFile(MigrationsFS, path.Join("migrations", e.Name()))
		if err != nil {
			return fmt.Errorf("read %s: %w", e.Name(), err)
		}
		checksum := migrationChecksum(body)
		if recorded, done := applied[version]; done {
			if recorded == "" {
				if _, err := db.ExecContext(ctx, `UPDATE schema_migrations SET checksum = ? WHERE version = ?`, checksum, version); err != nil {
					return fmt.Errorf("backfill checksum for %s: %w", e.Name(), err)
				}
				continue
			}
			if recorded != checksum {
				return fmt.Errorf("migration %s checksum changed after it was applied", e.Name())
			}
			continue
		}
		if err := applyOne(ctx, db, version, string(body), checksum); err != nil {
			return fmt.Errorf("apply %s: %w", e.Name(), err)
		}
	}
	return nil
}

func migrationPending(ctx context.Context, db *sql.DB) (bool, int, error) {
	entries, err := fs.ReadDir(MigrationsFS, "migrations")
	if err != nil {
		return false, 0, fmt.Errorf("read embedded migrations: %w", err)
	}
	latest := 0
	versions := make([]int, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		version, err := parseVersion(entry.Name())
		if err != nil {
			return false, 0, err
		}
		versions = append(versions, version)
		if version > latest {
			latest = version
		}
	}
	var hasMigrationTable int
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'`).Scan(&hasMigrationTable); err != nil {
		return false, latest, err
	}
	if hasMigrationTable == 0 {
		var userTables int
		if err := db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM sqlite_master
 WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`).Scan(&userTables); err != nil {
			return false, latest, err
		}
		return userTables > 0, latest, nil
	}
	applied, err := appliedVersionNumbers(ctx, db)
	if err != nil {
		return false, latest, err
	}
	for version := range applied {
		if version > latest {
			return false, latest, fmt.Errorf("database schema version %d is newer than this app supports (%d)", version, latest)
		}
	}
	for _, version := range versions {
		if _, ok := applied[version]; !ok {
			return true, latest, nil
		}
	}
	return false, latest, nil
}

func appliedVersionNumbers(ctx context.Context, db *sql.DB) (map[int]struct{}, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]struct{}{}
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		out[version] = struct{}{}
	}
	return out, rows.Err()
}

func ensureMigrationTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
  version    INTEGER PRIMARY KEY,
  applied_at INTEGER NOT NULL,
  checksum   TEXT NOT NULL DEFAULT ''
)`)
	if err != nil {
		return err
	}
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(schema_migrations)`)
	if err != nil {
		return err
	}
	hasChecksum := false
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		if name == "checksum" {
			hasChecksum = true
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if !hasChecksum {
		_, err = db.ExecContext(ctx, `ALTER TABLE schema_migrations ADD COLUMN checksum TEXT NOT NULL DEFAULT ''`)
	}
	return err
}

func appliedVersions(ctx context.Context, db *sql.DB) (map[int]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT version, checksum FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]string{}
	for rows.Next() {
		var v int
		var checksum string
		if err := rows.Scan(&v, &checksum); err != nil {
			return nil, err
		}
		out[v] = checksum
	}
	return out, rows.Err()
}

func applyOne(ctx context.Context, db *sql.DB, version int, body, checksum string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, body); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at, checksum) VALUES(?, ?, ?)`,
		version, time.Now().UnixMilli(), checksum); err != nil {
		return err
	}
	return tx.Commit()
}

func migrationChecksum(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// parseVersion accepts filenames like "0001_init.sql" and returns 1.
func parseVersion(name string) (int, error) {
	base := strings.TrimSuffix(name, ".sql")
	prefix := base
	if i := strings.Index(base, "_"); i >= 0 {
		prefix = base[:i]
	}
	v, err := strconv.Atoi(prefix)
	if err != nil {
		return 0, fmt.Errorf("bad migration filename %q: %w", name, err)
	}
	return v, nil
}
