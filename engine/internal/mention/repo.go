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

// RebuildAll reconstructs the derived mention table from every persisted leaf
// document. Documents are loaded before writes begin because the store uses a
// single SQLite connection.
func (r *Repo) RebuildAll(ctx context.Context) error {
	rows, err := r.s.DB().QueryContext(ctx, `
SELECT id, content_doc
  FROM nodes
 WHERE kind = 'leaf' AND content_doc IS NOT NULL`)
	if err != nil {
		return err
	}
	type document struct {
		nodeID string
		doc    string
	}
	documents := make([]document, 0)
	for rows.Next() {
		var item document
		if err := rows.Scan(&item.nodeID, &item.doc); err != nil {
			_ = rows.Close()
			return err
		}
		documents = append(documents, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range documents {
		if err := r.ResyncForNode(ctx, item.nodeID, Collect([]byte(item.doc))); err != nil {
			return err
		}
	}
	return nil
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

// MentionedNodeIDs returns the distinct node_ids where entityID is mentioned,
// plus the entity's project_id (resolved from the entities table so it works
// even when the entity has zero mentions).
func (r *Repo) MentionedNodeIDs(ctx context.Context, entityID string) (ids []string, projectID string, err error) {
	if err = r.s.DB().QueryRowContext(ctx,
		`SELECT project_id FROM entities WHERE id = ?`, entityID).Scan(&projectID); err != nil {
		return nil, "", err
	}
	rows, err := r.s.DB().QueryContext(ctx,
		`SELECT DISTINCT node_id FROM mentions WHERE entity_id = ?`, entityID)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	ids = []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, "", err
		}
		ids = append(ids, id)
	}
	return ids, projectID, rows.Err()
}

// CoMentionResult is one row of the topology-RAG result set.
type CoMentionResult struct {
	NodeID   string
	K        int     // distinct entities from the current node that appear here
	Rarity   float64 // sum of 1/(scenes the entity appears in) over those entities
	LastSeen int64   // nodes.updated_at
}

// CoMentionLeaves returns up to `limit` nodes that share at least 2 entities
// with currentNodeID, ranked by (rarity desc, updated_at desc). Excludes
// currentNodeID itself. Callers post-filter for additional exclusions
// (e.g. ids already shown in the nearby set).
//
// Rarity rather than the shared count: leads appear in nearly every scene of a
// long work, so the count saturates and stops telling scenes apart. Sharing a
// minor character or a prop says far more.
func (r *Repo) CoMentionLeaves(ctx context.Context, currentNodeID string, limit int) ([]CoMentionResult, error) {
	rows, err := r.s.DB().QueryContext(ctx, `
WITH cur_ents AS (
  SELECT DISTINCT entity_id FROM mentions WHERE node_id = ?
), ent_df AS (
  SELECT entity_id, COUNT(DISTINCT node_id) AS df
    FROM mentions
   WHERE entity_id IN (SELECT entity_id FROM cur_ents)
   GROUP BY entity_id
), shared AS (
  SELECT DISTINCT node_id, entity_id
    FROM mentions
   WHERE entity_id IN (SELECT entity_id FROM cur_ents)
     AND node_id != ?
)
SELECT s.node_id, COUNT(*) AS k, SUM(1.0 / d.df) AS rarity, n.updated_at AS last_seen
  FROM shared s
  JOIN ent_df d ON d.entity_id = s.entity_id
  JOIN nodes n ON n.id = s.node_id
 WHERE n.summary != ''
 GROUP BY s.node_id
HAVING k >= 2
 ORDER BY rarity DESC, last_seen DESC
 LIMIT ?`, currentNodeID, currentNodeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CoMentionResult
	for rows.Next() {
		var rec CoMentionResult
		if err := rows.Scan(&rec.NodeID, &rec.K, &rec.Rarity, &rec.LastSeen); err != nil {
			return nil, err
		}
		out = append(out, rec)
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

// ListCoreEntitiesForProject returns core story-bible entities independently of
// scene mentions so AI context can keep the global skeleton visible.
func (r *Repo) ListCoreEntitiesForProject(ctx context.Context, projectID string, limit int) ([]entity.Entity, error) {
	return entity.NewRepo(r.s).ListCoreByProject(ctx, projectID, limit)
}
