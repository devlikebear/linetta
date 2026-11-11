package draft

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/devlikebear/linetta/engine/internal/store"
)

const cols = `id, body, platform, kind, content_version, published_at, created_at, updated_at`

// Repo persists drafts. The store has a single connection, so every method
// finishes with its Rows before it returns.
type Repo struct {
	st  *store.Store
	now func() time.Time
}

// NewRepo returns a Repo over st.
func NewRepo(st *store.Store) *Repo { return &Repo{st: st, now: time.Now} }

// Create stores a new draft at content_version 1.
func (r *Repo) Create(ctx context.Context, platform, kind, body string) (Draft, error) {
	ts := r.now().UnixMilli()
	res, err := r.st.DB().ExecContext(ctx,
		`INSERT INTO drafts (body, platform, kind, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		body, platform, kind, ts, ts)
	if err != nil {
		return Draft{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Draft{}, err
	}
	return r.Get(ctx, id)
}

// Get returns one draft or ErrNotFound.
func (r *Repo) Get(ctx context.Context, id int64) (Draft, error) {
	var d Draft
	err := r.st.DB().QueryRowContext(ctx, `SELECT `+cols+` FROM drafts WHERE id = ?`, id).
		Scan(&d.ID, &d.Body, &d.Platform, &d.Kind, &d.ContentVersion, &d.PublishedAt, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Draft{}, ErrNotFound
	}
	return d, err
}

// List returns drafts, most recently updated first.
func (r *Repo) List(ctx context.Context) ([]Draft, error) {
	rows, err := r.st.DB().QueryContext(ctx, `SELECT `+cols+` FROM drafts ORDER BY updated_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Draft{}
	for rows.Next() {
		var d Draft
		if err := rows.Scan(&d.ID, &d.Body, &d.Platform, &d.Kind, &d.ContentVersion, &d.PublishedAt, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Update writes the draft and bumps content_version only when expectedVersion
// matches the stored one. A stale version returns ErrConflict, a missing id
// ErrNotFound.
func (r *Repo) Update(ctx context.Context, id, expectedVersion int64, body, platform, kind string) (Draft, error) {
	res, err := r.st.DB().ExecContext(ctx,
		`UPDATE drafts SET body = ?, platform = ?, kind = ?, content_version = content_version + 1, updated_at = ?
		 WHERE id = ? AND content_version = ?`,
		body, platform, kind, r.now().UnixMilli(), id, expectedVersion)
	if err != nil {
		return Draft{}, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Draft{}, err
	}
	if n == 0 {
		if _, err := r.Get(ctx, id); err != nil {
			return Draft{}, err
		}
		return Draft{}, ErrConflict
	}
	return r.Get(ctx, id)
}

// Delete removes a draft or returns ErrNotFound.
func (r *Repo) Delete(ctx context.Context, id int64) error {
	res, err := r.st.DB().ExecContext(ctx, `DELETE FROM drafts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
