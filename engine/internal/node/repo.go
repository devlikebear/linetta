package node

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/devlikebear/linetta/engine/internal/store"
)

// ErrNotFound is returned when a node id does not exist.
var ErrNotFound = errors.New("node not found")

// Repo persists Nodes in SQLite and keeps derived counts on projects in sync.
type Repo struct {
	s *store.Store
}

// NewRepo returns a Repo backed by the given Store.
func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

// Get returns a single node by id.
func (r *Repo) Get(ctx context.Context, id string) (Node, error) {
	row := r.s.DB().QueryRowContext(ctx, baseSelect+` WHERE id = ?`, id)
	n, err := scan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Node{}, ErrNotFound
	}
	return n, err
}

// UpdateContent replaces the leaf node's content_doc, recomputes its word_count,
// updates `projects.word_count` (= sum of leaf word_counts in that project),
// and touches `updated_at` on both rows.
func (r *Repo) UpdateContent(ctx context.Context, id string, doc string, now int64) error {
	count := CountChars([]byte(doc))

	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
UPDATE nodes
   SET content_doc = ?, word_count = ?, updated_at = ?
 WHERE id = ?`, doc, count, now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}

	// Recompute project total + touch its updated_at.
	if _, err := tx.ExecContext(ctx, `
UPDATE projects
   SET word_count = COALESCE((
        SELECT SUM(word_count) FROM nodes
         WHERE project_id = projects.id AND kind = 'leaf'), 0),
       updated_at = ?
 WHERE id = (SELECT project_id FROM nodes WHERE id = ?)`, now, id); err != nil {
		return fmt.Errorf("update project totals: %w", err)
	}

	return tx.Commit()
}

// SetLastOpened updates projects.last_opened_node_id and projects.updated_at.
func (r *Repo) SetLastOpened(ctx context.Context, projectID, nodeID string, now int64) error {
	res, err := r.s.DB().ExecContext(ctx, `
UPDATE projects SET last_opened_node_id = ?, updated_at = ?
 WHERE id = ?`, nodeID, now, projectID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

const baseSelect = `
SELECT id, project_id, parent_id, ordinal, kind, label, title,
       content_doc, status, word_count, created_at, updated_at
FROM nodes`

type scanner interface{ Scan(...any) error }

func scan(row scanner) (Node, error) {
	var (
		n          Node
		parentID   sql.NullString
		contentDoc sql.NullString
	)
	if err := row.Scan(&n.ID, &n.ProjectID, &parentID, &n.Ordinal, &n.Kind, &n.Label, &n.Title,
		&contentDoc, &n.Status, &n.WordCount, &n.CreatedAt, &n.UpdatedAt); err != nil {
		return Node{}, err
	}
	if parentID.Valid {
		v := parentID.String
		n.ParentID = &v
	}
	if contentDoc.Valid {
		v := contentDoc.String
		n.ContentDoc = &v
	}
	return n, nil
}
