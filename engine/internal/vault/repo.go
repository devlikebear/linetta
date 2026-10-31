package vault

import (
	"context"
	"database/sql"
	"errors"
)

// Link is this device's bookmark for one work (the vault_items table).
type Link struct {
	ProjectID    string
	ItemID       string
	BaseRevision int
	LastHash     string
	LastSyncedAt int64
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func getLink(ctx context.Context, q queryer, projectID string) (Link, bool, error) {
	l := Link{ProjectID: projectID}
	err := q.QueryRowContext(ctx, `
SELECT item_id, base_revision, last_hash, last_synced_at
FROM vault_items WHERE project_id = ?`, projectID).
		Scan(&l.ItemID, &l.BaseRevision, &l.LastHash, &l.LastSyncedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return l, false, nil
	}
	return l, err == nil, err
}

func listLinks(ctx context.Context, db *sql.DB) ([]Link, error) {
	rows, err := db.QueryContext(ctx, `
SELECT project_id, item_id, base_revision, last_hash, last_synced_at FROM vault_items`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Link
	for rows.Next() {
		var l Link
		if err := rows.Scan(&l.ProjectID, &l.ItemID, &l.BaseRevision, &l.LastHash, &l.LastSyncedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func putLink(ctx context.Context, db execer, l Link) error {
	_, err := db.ExecContext(ctx, `
INSERT INTO vault_items (project_id, item_id, base_revision, last_hash, last_synced_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(project_id) DO UPDATE SET
  item_id = excluded.item_id, base_revision = excluded.base_revision,
  last_hash = excluded.last_hash, last_synced_at = excluded.last_synced_at`,
		l.ProjectID, l.ItemID, l.BaseRevision, l.LastHash, l.LastSyncedAt)
	return err
}
