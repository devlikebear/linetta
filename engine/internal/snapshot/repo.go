package snapshot

import (
	"context"
	"database/sql"
	"errors"

	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/google/uuid"
)

// ErrNotFound is returned when no snapshot exists for a query.
var ErrNotFound = errors.New("snapshot not found")

// Repo persists node_snapshots rows.
type Repo struct {
	s *store.Store
}

// NewRepo returns a Repo backed by the given Store.
func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

// Create inserts a snapshot and returns it with a generated id.
func (r *Repo) Create(ctx context.Context, nodeID, doc, reason string, now int64) (Snapshot, error) {
	id := uuid.NewString()
	if _, err := r.s.DB().ExecContext(ctx, `
INSERT INTO node_snapshots (id, node_id, content_doc, reason, created_at)
VALUES (?, ?, ?, ?, ?)`, id, nodeID, doc, reason, now); err != nil {
		return Snapshot{}, err
	}
	return Snapshot{ID: id, NodeID: nodeID, ContentDoc: doc, Reason: reason, CreatedAt: now}, nil
}

// LatestForNode returns the most recent snapshot for the node (any reason).
func (r *Repo) LatestForNode(ctx context.Context, nodeID string) (Snapshot, error) {
	row := r.s.DB().QueryRowContext(ctx, `
SELECT id, node_id, content_doc, reason, created_at
  FROM node_snapshots
 WHERE node_id = ?
 ORDER BY created_at DESC
 LIMIT 1`, nodeID)
	var s Snapshot
	if err := row.Scan(&s.ID, &s.NodeID, &s.ContentDoc, &s.Reason, &s.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Snapshot{}, ErrNotFound
		}
		return Snapshot{}, err
	}
	return s, nil
}

// LatestAutosaveTime returns (created_at, true) of the most recent autosave
// for the node, or (0, false) if none.
func (r *Repo) LatestAutosaveTime(ctx context.Context, nodeID string) (int64, bool, error) {
	var t sql.NullInt64
	err := r.s.DB().QueryRowContext(ctx, `
SELECT MAX(created_at) FROM node_snapshots
 WHERE node_id = ? AND reason = 'autosave'`, nodeID).Scan(&t)
	if err != nil {
		return 0, false, err
	}
	if !t.Valid {
		return 0, false, nil
	}
	return t.Int64, true, nil
}
