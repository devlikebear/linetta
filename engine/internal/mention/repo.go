package mention

import (
	"context"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/google/uuid"
)

// Repo persists mentions rows.
type Repo struct {
	s *store.Store
}

func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

// ResyncForNode replaces the node's mention set with `found`. Any Found whose
// EntityID does not exist in the entities table is silently dropped (the body
// might reference a stale entity id after a deletion — that's allowed).
func (r *Repo) ResyncForNode(ctx context.Context, nodeID string, found []Found) error {
	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM mentions WHERE node_id = ?`, nodeID); err != nil {
		return err
	}
	for _, f := range found {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM entities WHERE id = ?`, f.EntityID).Scan(&exists); err != nil {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO mentions (id, node_id, entity_id, position, surface)
VALUES (?, ?, ?, ?, ?)`, uuid.NewString(), nodeID, f.EntityID, f.Position, f.Surface); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListForNode returns raw mention rows ordered by position.
func (r *Repo) ListForNode(ctx context.Context, nodeID string) ([]Mention, error) {
	rows, err := r.s.DB().QueryContext(ctx, `
SELECT id, node_id, entity_id, position, surface
  FROM mentions
 WHERE node_id = ?
 ORDER BY position ASC`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Mention
	for rows.Next() {
		var m Mention
		if err := rows.Scan(&m.ID, &m.NodeID, &m.EntityID, &m.Position, &m.Surface); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListEntitiesForNode returns the distinct entities mentioned in the node,
// hydrated with their full fields, in first-appearance order.
func (r *Repo) ListEntitiesForNode(ctx context.Context, nodeID string) ([]entity.Entity, error) {
	rows, err := r.s.DB().QueryContext(ctx, `
SELECT e.id, e.project_id, e.kind, e.name, e.aliases, e.role, e.summary, e.attributes,
       e.created_at, e.updated_at
  FROM entities e
  JOIN (
    SELECT entity_id, MIN(position) AS pos
      FROM mentions
     WHERE node_id = ?
     GROUP BY entity_id
  ) m ON m.entity_id = e.id
 ORDER BY m.pos ASC`, nodeID)
	if err != nil {
		return nil, err
	}
	return entity.ScanAll(rows)
}
