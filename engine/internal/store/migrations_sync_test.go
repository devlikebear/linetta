package store

import (
	"context"
	"database/sql"
	"io/fs"
	"path"
	"sort"
	"testing"
)

// applyThrough stops short of a migration, so a test can put rows in the shape an
// upgrading library has them.
func applyThrough(t *testing.T, db *sql.DB, last int) {
	t.Helper()
	ctx := context.Background()
	if err := ensureMigrationTable(ctx, db); err != nil {
		t.Fatal(err)
	}
	entries, err := fs.ReadDir(MigrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		version, err := parseVersion(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if version > last {
			break
		}
		body, err := fs.ReadFile(MigrationsFS, path.Join("migrations", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := applyOne(ctx, db, version, string(body), migrationChecksum(body)); err != nil {
			t.Fatalf("%s: %v", entry.Name(), err)
		}
	}
}

func TestSyncTargetsMigrationKeepsEveryBookmark(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	applyThrough(t, db, 39)
	for _, statement := range []string{
		`INSERT INTO projects (id, title, genres, length_target, default_pov, created_at, updated_at) VALUES ('p', 't', '[]', 'long', '3rd', 1, 2)`,
		`INSERT INTO vault_items (project_id, item_id, base_revision, last_hash, last_synced_at) VALUES ('p', 'item-1', 7, 'hash', 99)`,
		`INSERT INTO vault_deletions (item_id, project_id, title, queued_at) VALUES ('item-2', 'gone', '지운 작품', 5)`,
		`INSERT INTO sync_removed (project_id, removed_at) VALUES ('p', 6)`,
		`INSERT INTO vault_library (target, base_revision, last_hash) VALUES ('works', 3, 'lib')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}

	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}

	var target, item, hash string
	var base int
	var synced int64
	err = db.QueryRow(`SELECT target, item_id, base_revision, last_hash, last_synced_at FROM vault_items WHERE project_id = 'p'`).Scan(&target, &item, &base, &hash, &synced)
	if err != nil || target != "works" || item != "item-1" || base != 7 || hash != "hash" || synced != 99 {
		t.Fatalf("link = %s %s %d %s %d, %v", target, item, base, hash, synced, err)
	}
	if _, err := db.Exec(`INSERT INTO vault_items (project_id, target, item_id) VALUES ('p', 'folder', 'item-1')`); err != nil {
		t.Fatalf("a second target for the work: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO vault_items (project_id, target, item_id) VALUES ('p', 'works', 'item-3')`); err == nil {
		t.Fatal("two links to one target were accepted")
	}
	var title string
	if err := db.QueryRow(`SELECT title FROM vault_deletions WHERE target = 'works' AND item_id = 'item-2'`).Scan(&title); err != nil || title != "지운 작품" {
		t.Fatalf("deletion = %q, %v", title, err)
	}
	if err := db.QueryRow(`SELECT target FROM sync_removed WHERE project_id = 'p'`).Scan(&target); err != nil || target != "works" {
		t.Fatalf("removed = %q, %v", target, err)
	}
	if _, err := db.Exec(`DELETE FROM projects WHERE id = 'p'`); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := db.QueryRow(`SELECT COUNT(*) FROM vault_items`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("links left after the work went: %d, %v", left, err)
	}
}
