package note

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/google/uuid"
)

var ErrNotFound = errors.New("note not found")

type Repo struct{ s *store.Store }

func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

func (r *Repo) Create(ctx context.Context, in NewInput, now int64) (Note, error) {
	if in.NodeID == "" {
		return Note{}, fmt.Errorf("create note: node_id required")
	}
	if in.Body == "" {
		return Note{}, fmt.Errorf("create note: body required")
	}
	id := uuid.NewString()
	if _, err := r.s.DB().ExecContext(ctx, `
INSERT INTO notes (id, node_id, anchor, body, created_at)
VALUES (?, ?, ?, ?, ?)`, id, in.NodeID, in.Anchor, in.Body, now); err != nil {
		return Note{}, err
	}
	return r.Get(ctx, id)
}

func (r *Repo) Get(ctx context.Context, id string) (Note, error) {
	row := r.s.DB().QueryRowContext(ctx, baseSelect+` WHERE id = ?`, id)
	n, err := scan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Note{}, ErrNotFound
	}
	return n, err
}

func (r *Repo) ListForNode(ctx context.Context, nodeID string) ([]Note, error) {
	rows, err := r.s.DB().QueryContext(ctx, baseSelect+` WHERE node_id = ? ORDER BY anchor ASC`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Note
	for rows.Next() {
		n, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (r *Repo) Update(ctx context.Context, in UpdateInput) error {
	if in.ID == "" {
		return fmt.Errorf("update note: id required")
	}
	if in.Body == "" {
		return fmt.Errorf("update note: body required (use Delete to remove)")
	}
	res, err := r.s.DB().ExecContext(ctx, `UPDATE notes SET body = ? WHERE id = ?`, in.Body, in.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repo) Delete(ctx context.Context, id string) error {
	res, err := r.s.DB().ExecContext(ctx, `DELETE FROM notes WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

const baseSelect = `SELECT id, node_id, anchor, body, created_at FROM notes`

type scanner interface{ Scan(...any) error }

func scan(row scanner) (Note, error) {
	var n Note
	if err := row.Scan(&n.ID, &n.NodeID, &n.Anchor, &n.Body, &n.CreatedAt); err != nil {
		return Note{}, err
	}
	return n, nil
}
