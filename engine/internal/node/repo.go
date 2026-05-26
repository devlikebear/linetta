package node

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/google/uuid"
)

// ErrNotFound is returned when a node id does not exist.
var ErrNotFound = errors.New("node not found")

// MentionResyncer is called after a successful UpdateContent. Typical impl:
// mention.Repo.ResyncForNode + mention.Collect. The callback returning an error
// surfaces as an UpdateContent failure.
type MentionResyncer func(ctx context.Context, nodeID, doc string) error

// Repo persists Nodes in SQLite and keeps derived counts on projects in sync.
type Repo struct {
	s      *store.Store
	resync MentionResyncer
}

// NewRepo returns a Repo backed by the given Store.
func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

// SetMentionResyncer wires the optional callback. If unset, UpdateContent skips
// the resync (used in tests that don't care about mentions).
func (r *Repo) SetMentionResyncer(fn MentionResyncer) {
	r.resync = fn
}

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
   SET content_doc = ?, word_count = ?, updated_at = ?,
       content_version = content_version + 1
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

	if err := tx.Commit(); err != nil {
		return err
	}
	if r.resync != nil {
		if err := r.resync(ctx, id, doc); err != nil {
			return err
		}
	}
	return nil
}

// SetSummary writes the LLM-generated summary and the version it was generated
// for. Does NOT touch updated_at — derived field, not user content.
func (r *Repo) SetSummary(ctx context.Context, id string, summary string, forVersion int) error {
	res, err := r.s.DB().ExecContext(ctx, `
UPDATE nodes SET summary = ?, summary_for_version = ? WHERE id = ?`,
		summary, forVersion, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
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
       content_doc, status, word_count, summary, content_version,
       summary_for_version, created_at, updated_at
FROM nodes`

type scanner interface{ Scan(...any) error }

func scan(row scanner) (Node, error) {
	var (
		n          Node
		parentID   sql.NullString
		contentDoc sql.NullString
	)
	if err := row.Scan(&n.ID, &n.ProjectID, &parentID, &n.Ordinal, &n.Kind, &n.Label, &n.Title,
		&contentDoc, &n.Status, &n.WordCount, &n.Summary, &n.ContentVersion,
		&n.SummaryForVersion, &n.CreatedAt, &n.UpdatedAt); err != nil {
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

// ListByProject returns every node belonging to the project, sorted by
// (parent_id NULLS FIRST, ordinal). Callers build the tree in memory.
func (r *Repo) ListByProject(ctx context.Context, projectID string) ([]Node, error) {
	rows, err := r.s.DB().QueryContext(ctx, baseSelect+`
WHERE project_id = ?
ORDER BY (parent_id IS NULL) DESC, parent_id, ordinal`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Node
	for rows.Next() {
		n, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// CreateSibling inserts a new node after referenceID at the same parent + level.
// The new ordinal is referenceOrdinal + 1; existing siblings at >= that ordinal
// are shifted forward by 1.
func (r *Repo) CreateSibling(ctx context.Context, referenceID, kind, label, title string, now int64) (Node, error) {
	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return Node{}, err
	}
	defer tx.Rollback()

	row := tx.QueryRowContext(ctx, baseSelect+` WHERE id = ?`, referenceID)
	ref, err := scan(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return Node{}, ErrNotFound
		}
		return Node{}, err
	}
	parentID := ref.ParentID
	refOrd := ref.Ordinal

	// Shift downstream siblings.
	if parentID == nil {
		if _, err := tx.ExecContext(ctx, `
UPDATE nodes SET ordinal = ordinal + 1
 WHERE project_id = ? AND parent_id IS NULL AND ordinal > ?`,
			ref.ProjectID, refOrd); err != nil {
			return Node{}, err
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
UPDATE nodes SET ordinal = ordinal + 1
 WHERE parent_id = ? AND ordinal > ?`, *parentID, refOrd); err != nil {
			return Node{}, err
		}
	}

	newID := uuid.NewString()
	var contentDoc any
	if kind == "leaf" {
		contentDoc = `{"type":"doc","content":[{"type":"paragraph"}]}`
	} else {
		contentDoc = nil
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO nodes (id, project_id, parent_id, ordinal, kind, label, title,
                   content_doc, status, word_count, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'draft', 0, ?, ?)`,
		newID, ref.ProjectID, parentID, refOrd+1, kind, label, title, contentDoc, now, now); err != nil {
		return Node{}, err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE projects SET updated_at = ? WHERE id = ?`, now, ref.ProjectID); err != nil {
		return Node{}, err
	}
	if err := tx.Commit(); err != nil {
		return Node{}, err
	}
	return r.Get(ctx, newID)
}

// CreateChild inserts a new node as the last child of parentID.
func (r *Repo) CreateChild(ctx context.Context, parentID, kind, label, title string, now int64) (Node, error) {
	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return Node{}, err
	}
	defer tx.Rollback()

	row := tx.QueryRowContext(ctx, baseSelect+` WHERE id = ?`, parentID)
	parent, err := scan(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return Node{}, ErrNotFound
		}
		return Node{}, err
	}

	var maxOrd sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MAX(ordinal) FROM nodes WHERE parent_id = ?`, parentID).Scan(&maxOrd); err != nil {
		return Node{}, err
	}
	nextOrd := 0
	if maxOrd.Valid {
		nextOrd = int(maxOrd.Int64) + 1
	}

	newID := uuid.NewString()
	var contentDoc any
	if kind == "leaf" {
		contentDoc = `{"type":"doc","content":[{"type":"paragraph"}]}`
	} else {
		contentDoc = nil
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO nodes (id, project_id, parent_id, ordinal, kind, label, title,
                   content_doc, status, word_count, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'draft', 0, ?, ?)`,
		newID, parent.ProjectID, parentID, nextOrd, kind, label, title, contentDoc, now, now); err != nil {
		return Node{}, err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE projects SET updated_at = ? WHERE id = ?`, now, parent.ProjectID); err != nil {
		return Node{}, err
	}
	if err := tx.Commit(); err != nil {
		return Node{}, err
	}
	return r.Get(ctx, newID)
}

// Rename updates label and title (both can be empty strings to clear).
func (r *Repo) Rename(ctx context.Context, id, label, title string, now int64) error {
	res, err := r.s.DB().ExecContext(ctx, `
UPDATE nodes SET label = ?, title = ?, updated_at = ?
 WHERE id = ?`, label, title, now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes the node (children cascade via FK). Recomputes the project's
// word_count.
func (r *Repo) Delete(ctx context.Context, id string, now int64) error {
	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var projectID string
	if err := tx.QueryRowContext(ctx, `SELECT project_id FROM nodes WHERE id = ?`, id).Scan(&projectID); err != nil {
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM nodes WHERE id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE projects
   SET word_count = COALESCE((SELECT SUM(word_count) FROM nodes WHERE project_id = ? AND kind = 'leaf'), 0),
       updated_at  = ?
 WHERE id = ?`, projectID, now, projectID); err != nil {
		return err
	}
	return tx.Commit()
}

// MoveUp swaps the node with its previous sibling (same parent_id). No-op if
// the node is already first.
func (r *Repo) MoveUp(ctx context.Context, id string, now int64) error {
	return r.swap(ctx, id, "up", now)
}

// MoveDown swaps the node with its next sibling. No-op if last.
func (r *Repo) MoveDown(ctx context.Context, id string, now int64) error {
	return r.swap(ctx, id, "down", now)
}

func (r *Repo) swap(ctx context.Context, id, direction string, now int64) error {
	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	row := tx.QueryRowContext(ctx, baseSelect+` WHERE id = ?`, id)
	cur, err := scan(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		return err
	}

	var (
		neighborID  string
		neighborOrd int
	)
	var query string
	if direction == "up" {
		if cur.ParentID == nil {
			query = `SELECT id, ordinal FROM nodes
 WHERE project_id = ? AND parent_id IS NULL AND ordinal < ?
 ORDER BY ordinal DESC LIMIT 1`
		} else {
			query = `SELECT id, ordinal FROM nodes
 WHERE parent_id = ? AND ordinal < ?
 ORDER BY ordinal DESC LIMIT 1`
		}
	} else {
		if cur.ParentID == nil {
			query = `SELECT id, ordinal FROM nodes
 WHERE project_id = ? AND parent_id IS NULL AND ordinal > ?
 ORDER BY ordinal ASC LIMIT 1`
		} else {
			query = `SELECT id, ordinal FROM nodes
 WHERE parent_id = ? AND ordinal > ?
 ORDER BY ordinal ASC LIMIT 1`
		}
	}
	scope := any(cur.ProjectID)
	if cur.ParentID != nil {
		scope = *cur.ParentID
	}
	err = tx.QueryRowContext(ctx, query, scope, cur.Ordinal).Scan(&neighborID, &neighborOrd)
	if err == sql.ErrNoRows {
		// No neighbor — no-op.
		return tx.Commit()
	}
	if err != nil {
		return err
	}

	// Two-step swap (avoid unique constraint conflicts if any — schema has none here
	// but this is the safe pattern).
	tmp := -1 - cur.Ordinal
	if _, err := tx.ExecContext(ctx, `UPDATE nodes SET ordinal = ?, updated_at = ? WHERE id = ?`, tmp, now, cur.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE nodes SET ordinal = ?, updated_at = ? WHERE id = ?`, cur.Ordinal, now, neighborID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE nodes SET ordinal = ?, updated_at = ? WHERE id = ?`, neighborOrd, now, cur.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE projects SET updated_at = ? WHERE id = ?`, now, cur.ProjectID); err != nil {
		return err
	}
	return tx.Commit()
}
