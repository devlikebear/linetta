package thread

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/google/uuid"
)

// ErrNotFound is returned when a thread id does not exist.
var ErrNotFound = errors.New("thread not found")

// Repo persists Threads in SQLite.
type Repo struct {
	s *store.Store
}

func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

// Create inserts a new thread. Empty color defaults to #666.
func (r *Repo) Create(ctx context.Context, in NewInput) (Thread, error) {
	if in.ProjectID == "" || in.Name == "" {
		return Thread{}, fmt.Errorf("create thread: project_id and name required")
	}
	color := in.Color
	if color == "" {
		color = "#666"
	}
	id := uuid.NewString()
	if _, err := r.s.DB().ExecContext(ctx, `
INSERT INTO threads (id, project_id, name, color, summary, closed_at)
VALUES (?, ?, ?, ?, '', NULL)`, id, in.ProjectID, in.Name, color); err != nil {
		return Thread{}, err
	}
	return r.Get(ctx, id)
}

// Get returns one thread by id.
func (r *Repo) Get(ctx context.Context, id string) (Thread, error) {
	row := r.s.DB().QueryRowContext(ctx, baseSelect+` WHERE id = ?`, id)
	th, err := scan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Thread{}, ErrNotFound
	}
	return th, err
}

// ListByProject returns threads ordered by name. When includeClosed is false,
// rows with a non-null closed_at are filtered out.
func (r *Repo) ListByProject(ctx context.Context, projectID string, includeClosed bool) ([]Thread, error) {
	q := baseSelect + ` WHERE project_id = ?`
	if !includeClosed {
		q += ` AND closed_at IS NULL`
	}
	q += ` ORDER BY name COLLATE NOCASE`
	rows, err := r.s.DB().QueryContext(ctx, q, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Thread
	for rows.Next() {
		th, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, th)
	}
	return out, rows.Err()
}

// Update applies a partial input. Empty strings leave fields alone.
func (r *Repo) Update(ctx context.Context, in UpdateInput) error {
	if in.ID == "" {
		return fmt.Errorf("update thread: id required")
	}
	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	row := tx.QueryRowContext(ctx, baseSelect+` WHERE id = ?`, in.ID)
	cur, err := scan(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if in.Name != "" {
		cur.Name = in.Name
	}
	if in.Color != "" {
		cur.Color = in.Color
	}
	cur.Summary = in.Summary
	if _, err := tx.ExecContext(ctx, `
UPDATE threads SET name = ?, color = ?, summary = ? WHERE id = ?`,
		cur.Name, cur.Color, cur.Summary, in.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// Close stamps closed_at on the row.
func (r *Repo) Close(ctx context.Context, id string, now int64) error {
	res, err := r.s.DB().ExecContext(ctx, `UPDATE threads SET closed_at = ? WHERE id = ?`, now, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Reopen clears closed_at on the row.
func (r *Repo) Reopen(ctx context.Context, id string) error {
	res, err := r.s.DB().ExecContext(ctx, `UPDATE threads SET closed_at = NULL WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

const baseSelect = `
SELECT id, project_id, name, color, summary, closed_at
FROM threads`

type scanner interface{ Scan(...any) error }

func scan(row scanner) (Thread, error) {
	var (
		th     Thread
		closed sql.NullInt64
	)
	if err := row.Scan(&th.ID, &th.ProjectID, &th.Name, &th.Color, &th.Summary, &closed); err != nil {
		return Thread{}, err
	}
	if closed.Valid {
		v := closed.Int64
		th.ClosedAt = &v
	}
	return th, nil
}
