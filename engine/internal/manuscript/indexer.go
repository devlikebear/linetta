package manuscript

import (
	"context"
	"database/sql"
	"errors"
)

// Indexer keeps the app-managed FTS table synchronized with leaf manuscript
// prose. It stores plain text derived from Tiptap JSON.
type Indexer struct {
	db *sql.DB
}

func NewIndexer(db *sql.DB) *Indexer {
	return &Indexer{db: db}
}

func (i *Indexer) Upsert(ctx context.Context, projectID, nodeID, contentDoc string) error {
	if i == nil || i.db == nil {
		return nil
	}
	if _, err := i.db.ExecContext(ctx, `DELETE FROM manuscript_fts WHERE node_id = ?`, nodeID); err != nil {
		return err
	}
	_, err := i.db.ExecContext(ctx, `
INSERT INTO manuscript_fts (plain, node_id, project_id)
VALUES (?, ?, ?)`, docToPlainText(contentDoc), nodeID, projectID)
	return err
}

func (i *Indexer) Delete(ctx context.Context, nodeID string) error {
	if i == nil || i.db == nil {
		return nil
	}
	_, err := i.db.ExecContext(ctx, `DELETE FROM manuscript_fts WHERE node_id = ?`, nodeID)
	return err
}

func (i *Indexer) Rebuild(ctx context.Context, projectID string) error {
	if i == nil || i.db == nil {
		return nil
	}
	rows, err := i.db.QueryContext(ctx, `
SELECT id, COALESCE(content_doc, '')
  FROM nodes
 WHERE project_id = ? AND kind = 'leaf'
 ORDER BY ordinal`, projectID)
	if err != nil {
		return err
	}
	type entry struct {
		nodeID string
		doc    string
	}
	entries := []entry{}
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.nodeID, &e.doc); err != nil {
			rows.Close()
			return err
		}
		entries = append(entries, e)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}

	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM manuscript_fts WHERE project_id = ?`, projectID); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO manuscript_fts (plain, node_id, project_id)
VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, e := range entries {
		if _, err := stmt.ExecContext(ctx, docToPlainText(e.doc), e.nodeID, projectID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (i *Indexer) ProjectRowCount(ctx context.Context, projectID string) (int, error) {
	if i == nil || i.db == nil {
		return 0, nil
	}
	var count int
	err := i.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM manuscript_fts WHERE project_id = ?`, projectID).Scan(&count)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return count, err
}
