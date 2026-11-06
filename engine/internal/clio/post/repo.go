package post

import (
	"context"
	"fmt"
	"time"

	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/google/uuid"
)

// Repo persists posts. The store has a single connection, so every method
// finishes with its Rows before it returns.
type Repo struct {
	st  *store.Store
	now func() time.Time
}

// NewRepo returns a Repo over st.
func NewRepo(st *store.Store) *Repo { return &Repo{st: st, now: time.Now} }

// InsertMany stores the posts in one transaction. A post whose body already
// exists on the same platform is skipped, not an error.
func (r *Repo) InsertMany(ctx context.Context, posts []NewPost) (inserted, skipped int, err error) {
	tx, err := r.st.DB().BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	// ON CONFLICT DO NOTHING, not INSERT OR IGNORE: the latter would also
	// swallow CHECK and NOT NULL violations as "skipped".
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO posts
		(id, body, platform, kind, source, external_id, posted_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(platform, body) DO NOTHING`)
	if err != nil {
		return 0, 0, err
	}
	defer stmt.Close()
	created := r.now().UnixMilli()
	for _, p := range posts {
		res, execErr := stmt.ExecContext(ctx, uuid.NewString(), p.Body, p.Platform, p.Kind, p.Source, p.ExternalID, p.PostedAt, created)
		if execErr != nil {
			return 0, 0, fmt.Errorf("insert post: %w", execErr)
		}
		n, raErr := res.RowsAffected()
		if raErr != nil {
			return 0, 0, raErr
		}
		if n == 1 {
			inserted++
		} else {
			skipped++
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, 0, err
	}
	return inserted, skipped, nil
}

// List returns posts in insertion order.
func (r *Repo) List(ctx context.Context, p ListParams) ([]Post, error) {
	q := `SELECT id, body, platform, kind, source, external_id, posted_at, created_at FROM posts WHERE 1=1`
	var args []any
	if p.Platform != "" {
		q += ` AND platform = ?`
		args = append(args, p.Platform)
	}
	if p.Kind != "" {
		q += ` AND kind = ?`
		args = append(args, p.Kind)
	}
	q += ` ORDER BY created_at, rowid`
	if p.Limit > 0 {
		q += ` LIMIT ? OFFSET ?`
		args = append(args, p.Limit, p.Offset)
	} else if p.Offset > 0 {
		q += ` LIMIT -1 OFFSET ?`
		args = append(args, p.Offset)
	}
	rows, err := r.st.DB().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Post{}
	for rows.Next() {
		var x Post
		if err := rows.Scan(&x.ID, &x.Body, &x.Platform, &x.Kind, &x.Source, &x.ExternalID, &x.PostedAt, &x.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// Counts returns how many posts each platform/kind context holds.
func (r *Repo) Counts(ctx context.Context) ([]ContextCount, error) {
	rows, err := r.st.DB().QueryContext(ctx, `SELECT platform, kind, COUNT(*) FROM posts GROUP BY platform, kind ORDER BY platform, kind`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ContextCount{}
	for rows.Next() {
		var c ContextCount
		if err := rows.Scan(&c.Platform, &c.Kind, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
