package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

func Open(path string) (*DB, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("store path is required")
	}
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
	}

	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db := &DB{conn: conn}
	if _, err := db.conn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := db.conn.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func (db *DB) Close() error {
	if db == nil || db.conn == nil {
		return nil
	}
	return db.conn.Close()
}

func (db *DB) Migrate(ctx context.Context) error {
	if db == nil || db.conn == nil {
		return fmt.Errorf("store is not open")
	}
	for _, statement := range migrationStatements {
		if _, err := db.conn.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) Conn() *sql.DB {
	if db == nil {
		return nil
	}
	return db.conn
}

var migrationStatements = []string{
	`CREATE TABLE IF NOT EXISTS works (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		genre TEXT NOT NULL DEFAULT '',
		premise TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'active',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS episodes (
		id TEXT PRIMARY KEY,
		work_id TEXT NOT NULL,
		title TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'idea',
		position INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		FOREIGN KEY (work_id) REFERENCES works(id) ON DELETE CASCADE
	)`,
	`CREATE INDEX IF NOT EXISTS idx_episodes_work_position ON episodes(work_id, position, created_at)`,
	`CREATE TABLE IF NOT EXISTS episode_blueprints (
		id TEXT PRIMARY KEY,
		work_id TEXT NOT NULL,
		episode_id TEXT NOT NULL,
		premise TEXT NOT NULL DEFAULT '',
		theme TEXT NOT NULL DEFAULT '',
		situation TEXT NOT NULL DEFAULT '',
		must_include TEXT NOT NULL DEFAULT '',
		must_avoid TEXT NOT NULL DEFAULT '',
		structure_notes TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		FOREIGN KEY (work_id) REFERENCES works(id) ON DELETE CASCADE,
		FOREIGN KEY (episode_id) REFERENCES episodes(id) ON DELETE CASCADE,
		UNIQUE(work_id, episode_id)
	)`,
	`CREATE TABLE IF NOT EXISTS agent_runs (
		id TEXT PRIMARY KEY,
		work_id TEXT NOT NULL,
		episode_id TEXT,
		status TEXT NOT NULL,
		tessera_run_id TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		closed_at TEXT,
		FOREIGN KEY (work_id) REFERENCES works(id) ON DELETE CASCADE,
		FOREIGN KEY (episode_id) REFERENCES episodes(id) ON DELETE SET NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_agent_runs_work_episode ON agent_runs(work_id, episode_id, created_at)`,
	`CREATE TABLE IF NOT EXISTS agent_run_events (
		id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		seq INTEGER NOT NULL,
		event_json TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY (run_id) REFERENCES agent_runs(id) ON DELETE CASCADE,
		UNIQUE(run_id, seq)
	)`,
	`CREATE TABLE IF NOT EXISTS artifacts (
		id TEXT PRIMARY KEY,
		work_id TEXT NOT NULL,
		episode_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		kind TEXT NOT NULL,
		title TEXT NOT NULL,
		body TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY (work_id) REFERENCES works(id) ON DELETE CASCADE,
		FOREIGN KEY (episode_id) REFERENCES episodes(id) ON DELETE CASCADE,
		FOREIGN KEY (run_id) REFERENCES agent_runs(id) ON DELETE CASCADE
	)`,
	`CREATE INDEX IF NOT EXISTS idx_artifacts_run_kind ON artifacts(run_id, kind, created_at)`,
	`CREATE TABLE IF NOT EXISTS canon_items (
		id TEXT PRIMARY KEY,
		work_id TEXT NOT NULL,
		kind TEXT NOT NULL,
		title TEXT NOT NULL,
		body TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL,
		importance TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		FOREIGN KEY (work_id) REFERENCES works(id) ON DELETE CASCADE
	)`,
	`CREATE INDEX IF NOT EXISTS idx_canon_items_work_kind_status ON canon_items(work_id, kind, status, updated_at)`,
	`CREATE TABLE IF NOT EXISTS canon_decisions (
		id TEXT PRIMARY KEY,
		work_id TEXT NOT NULL,
		canon_item_id TEXT NOT NULL,
		decision_type TEXT NOT NULL,
		reason TEXT NOT NULL DEFAULT '',
		actor TEXT NOT NULL DEFAULT 'human',
		created_at TEXT NOT NULL,
		FOREIGN KEY (work_id) REFERENCES works(id) ON DELETE CASCADE,
		FOREIGN KEY (canon_item_id) REFERENCES canon_items(id) ON DELETE CASCADE
	)`,
	`CREATE INDEX IF NOT EXISTS idx_canon_decisions_work_created ON canon_decisions(work_id, created_at)`,
	`CREATE TABLE IF NOT EXISTS memory_links (
		id TEXT PRIMARY KEY,
		work_id TEXT NOT NULL,
		from_item_id TEXT NOT NULL,
		to_item_id TEXT NOT NULL,
		relation TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY (work_id) REFERENCES works(id) ON DELETE CASCADE,
		FOREIGN KEY (from_item_id) REFERENCES canon_items(id) ON DELETE CASCADE,
		FOREIGN KEY (to_item_id) REFERENCES canon_items(id) ON DELETE CASCADE
	)`,
}
