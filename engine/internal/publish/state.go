package publish

import (
	"context"
	"database/sql"
	"errors"
)

// WorkState is the last thing this library knew about a work on the site.
type WorkState struct {
	ProjectID    string `json:"project_id"`
	RemoteURL    string `json:"remote_url"`
	Slug         string `json:"slug"`
	Blurb        string `json:"blurb"`
	SerialStatus string `json:"serial_status"`
	LastSyncedAt int64  `json:"last_synced_at"`
}

// EpisodeState is the last thing this library knew about one episode on the
// site, keyed by the node it was published from.
type EpisodeState struct {
	NodeID           string `json:"node_id"`
	ProjectID        string `json:"project_id"`
	RemoteURL        string `json:"remote_url"`
	Number           int    `json:"number"`
	Title            string `json:"title"`
	ContentHash      string `json:"content_hash"`
	RemoteStatus     string `json:"remote_status"`
	ModerationReason string `json:"moderation_reason"`
	PublishedAt      string `json:"published_at"`
}

// StateRepo keeps the publish_works and publish_episodes tables.
type StateRepo struct {
	db *sql.DB
}

// NewStateRepo returns a StateRepo over the library database.
func NewStateRepo(db *sql.DB) *StateRepo { return &StateRepo{db: db} }

// Work returns the stored state of a work and whether there is one.
func (r *StateRepo) Work(ctx context.Context, projectID string) (WorkState, bool, error) {
	w := WorkState{ProjectID: projectID}
	err := r.db.QueryRowContext(ctx, `
SELECT remote_url, slug, blurb, serial_status, last_synced_at
FROM publish_works WHERE project_id = ?`, projectID).
		Scan(&w.RemoteURL, &w.Slug, &w.Blurb, &w.SerialStatus, &w.LastSyncedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return w, false, nil
	}
	return w, err == nil, err
}

// Episodes returns a work's stored episodes by number.
func (r *StateRepo) Episodes(ctx context.Context, projectID string) ([]EpisodeState, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT node_id, remote_url, number, title, content_hash, remote_status, moderation_reason, published_at
FROM publish_episodes WHERE project_id = ? ORDER BY number, node_id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EpisodeState
	for rows.Next() {
		e := EpisodeState{ProjectID: projectID}
		if err := rows.Scan(&e.NodeID, &e.RemoteURL, &e.Number, &e.Title, &e.ContentHash,
			&e.RemoteStatus, &e.ModerationReason, &e.PublishedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Sync replaces everything stored about a work with what the site just said:
// the work row and the full episode list. One transaction, so the copy is
// never half old and half new.
func (r *StateRepo) Sync(ctx context.Context, w WorkState, episodes []EpisodeState) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := putWork(ctx, tx, w); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM publish_episodes WHERE project_id = ?`, w.ProjectID); err != nil {
		return err
	}
	for _, e := range episodes {
		e.ProjectID = w.ProjectID
		if err := putEpisode(ctx, tx, e); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// PutWork stores one work row, keeping its episodes.
func (r *StateRepo) PutWork(ctx context.Context, w WorkState) error {
	return putWork(ctx, r.db, w)
}

// PutEpisode stores one episode row.
func (r *StateRepo) PutEpisode(ctx context.Context, e EpisodeState) error {
	return putEpisode(ctx, r.db, e)
}

// Forget drops everything saved about one work: the site says it does not
// have it.
func (r *StateRepo) Forget(ctx context.Context, projectID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM publish_episodes WHERE project_id = ?`, projectID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM publish_works WHERE project_id = ?`, projectID); err != nil {
		return err
	}
	return tx.Commit()
}

// Clear forgets every work. Connecting a different account makes the old
// account's copy describe works this one cannot see.
func (r *StateRepo) Clear(ctx context.Context) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM publish_episodes`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM publish_works`); err != nil {
		return err
	}
	return tx.Commit()
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func putWork(ctx context.Context, db execer, w WorkState) error {
	if w.SerialStatus == "" {
		w.SerialStatus = SerialOngoing
	}
	_, err := db.ExecContext(ctx, `
INSERT INTO publish_works (project_id, remote_url, slug, blurb, serial_status, last_synced_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(project_id) DO UPDATE SET
  remote_url = excluded.remote_url, slug = excluded.slug, blurb = excluded.blurb,
  serial_status = excluded.serial_status, last_synced_at = excluded.last_synced_at`,
		w.ProjectID, w.RemoteURL, w.Slug, w.Blurb, w.SerialStatus, w.LastSyncedAt)
	return err
}

func putEpisode(ctx context.Context, db execer, e EpisodeState) error {
	_, err := db.ExecContext(ctx, `
INSERT INTO publish_episodes (node_id, project_id, remote_url, number, title, content_hash,
                              remote_status, moderation_reason, published_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(node_id) DO UPDATE SET
  project_id = excluded.project_id, remote_url = excluded.remote_url, number = excluded.number,
  title = excluded.title, content_hash = excluded.content_hash, remote_status = excluded.remote_status,
  moderation_reason = excluded.moderation_reason, published_at = excluded.published_at`,
		e.NodeID, e.ProjectID, e.RemoteURL, e.Number, e.Title, e.ContentHash,
		e.RemoteStatus, e.ModerationReason, e.PublishedAt)
	return err
}
