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
	// Mark is the manifest of the revision this link last matched, for a store
	// where two devices can write the same revision number.
	Mark string
	// quiet keeps a transfer off the progress bar. It is not stored.
	quiet bool
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func getLink(ctx context.Context, q queryer, target, projectID string) (Link, bool, error) {
	l := Link{ProjectID: projectID}
	err := q.QueryRowContext(ctx, `
SELECT item_id, base_revision, last_hash, last_synced_at, head_mark
FROM vault_items WHERE project_id = ? AND target = ?`, projectID, target).
		Scan(&l.ItemID, &l.BaseRevision, &l.LastHash, &l.LastSyncedAt, &l.Mark)
	if errors.Is(err, sql.ErrNoRows) {
		return l, false, nil
	}
	return l, err == nil, err
}

func listLinks(ctx context.Context, db *sql.DB, target string) ([]Link, error) {
	rows, err := db.QueryContext(ctx, `
SELECT project_id, item_id, base_revision, last_hash, last_synced_at, head_mark
FROM vault_items WHERE target = ?`, target)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Link
	for rows.Next() {
		var l Link
		if err := rows.Scan(&l.ProjectID, &l.ItemID, &l.BaseRevision, &l.LastHash, &l.LastSyncedAt, &l.Mark); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func deleteLinkByItem(ctx context.Context, db execer, target, itemID string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM vault_items WHERE target = ? AND item_id = ?`, target, itemID)
	return err
}

func putLink(ctx context.Context, db execer, target string, l Link) error {
	_, err := db.ExecContext(ctx, `
INSERT INTO vault_items (project_id, target, item_id, base_revision, last_hash, last_synced_at, head_mark)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(project_id, target) DO UPDATE SET
  item_id = excluded.item_id, base_revision = excluded.base_revision,
  last_hash = excluded.last_hash, last_synced_at = excluded.last_synced_at,
  head_mark = excluded.head_mark`,
		l.ProjectID, target, l.ItemID, l.BaseRevision, l.LastHash, l.LastSyncedAt, l.Mark)
	return err
}
