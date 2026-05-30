package relationship

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/google/uuid"
)

// ErrNotFound is returned when a relationship id does not exist.
var ErrNotFound = errors.New("relationship not found")

// Repo persists Relationships in SQLite.
type Repo struct {
	s *store.Store
}

func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

// CreateOne inserts a singleton row (pair_id = NULL).
func (r *Repo) CreateOne(ctx context.Context, in NewInput) (Relationship, error) {
	if in.ProjectID == "" || in.FromID == "" || in.ToID == "" || in.Label == "" {
		return Relationship{}, fmt.Errorf("create relationship: project_id, from_id, to_id, label required")
	}
	id := uuid.NewString()
	if _, err := r.s.DB().ExecContext(ctx, `
INSERT INTO relationships (id, project_id, from_id, to_id, label, notes, pair_id)
VALUES (?, ?, ?, ?, ?, ?, NULL)`,
		id, in.ProjectID, in.FromID, in.ToID, in.Label, in.Notes); err != nil {
		return Relationship{}, err
	}
	return r.Get(ctx, id)
}

// CreatePair inserts two rows in one transaction, sharing a fresh pair_id.
// Return order: [forward(A→B, Label), inverse(B→A, InverseLabel)].
func (r *Repo) CreatePair(ctx context.Context, in NewPairInput) ([]Relationship, error) {
	if in.ProjectID == "" || in.FromID == "" || in.ToID == "" ||
		in.Label == "" || in.InverseLabel == "" {
		return nil, fmt.Errorf("create pair: project_id, from_id, to_id, label, inverse_label required")
	}
	pairID := uuid.NewString()
	forwardID := uuid.NewString()
	inverseID := uuid.NewString()

	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
INSERT INTO relationships (id, project_id, from_id, to_id, label, notes, pair_id)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		forwardID, in.ProjectID, in.FromID, in.ToID, in.Label, in.Notes, pairID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO relationships (id, project_id, from_id, to_id, label, notes, pair_id)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		inverseID, in.ProjectID, in.ToID, in.FromID, in.InverseLabel, "", pairID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	forward, err := r.Get(ctx, forwardID)
	if err != nil {
		return nil, err
	}
	inverse, err := r.Get(ctx, inverseID)
	if err != nil {
		return nil, err
	}
	return []Relationship{forward, inverse}, nil
}

// Get returns one relationship by id.
func (r *Repo) Get(ctx context.Context, id string) (Relationship, error) {
	row := r.s.DB().QueryRowContext(ctx, baseSelect+` WHERE id = ?`, id)
	rel, err := scan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Relationship{}, ErrNotFound
	}
	return rel, err
}

// ListByEntity returns rows where from_id = entityID, ordered by id (UUID order
// is stable and close enough to insertion order for this scope).
func (r *Repo) ListByEntity(ctx context.Context, entityID string) ([]Relationship, error) {
	rows, err := r.s.DB().QueryContext(ctx,
		baseSelect+` WHERE from_id = ? ORDER BY id`, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Relationship
	for rows.Next() {
		rel, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rel)
	}
	return out, rows.Err()
}

// ListByProject returns every relationship row in the project, ordered by id.
// Both directions of a pair are returned as separate rows; callers dedupe by
// pair_id if needed.
func (r *Repo) ListByProject(ctx context.Context, projectID string) ([]Relationship, error) {
	rows, err := r.s.DB().QueryContext(ctx,
		baseSelect+` WHERE project_id = ? ORDER BY id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Relationship
	for rows.Next() {
		rel, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rel)
	}
	return out, rows.Err()
}

// Update patches a single row. The paired side keeps its own label/notes.
func (r *Repo) Update(ctx context.Context, in UpdateInput) error {
	if in.ID == "" {
		return fmt.Errorf("update relationship: id required")
	}
	res, err := r.s.DB().ExecContext(ctx,
		`UPDATE relationships SET label = ?, notes = ? WHERE id = ?`,
		in.Label, in.Notes, in.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes the row. If pair_id is not NULL, both rows of the pair are
// removed in one transaction; otherwise only the one row is removed.
func (r *Repo) Delete(ctx context.Context, id string) error {
	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var pairID sql.NullString
	if err := tx.QueryRowContext(ctx,
		`SELECT pair_id FROM relationships WHERE id = ?`, id).Scan(&pairID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if pairID.Valid && pairID.String != "" {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM relationships WHERE pair_id = ?`, pairID.String); err != nil {
			return err
		}
	} else {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM relationships WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

const baseSelect = `
SELECT id, project_id, from_id, to_id, label, notes, pair_id
FROM relationships`

type scanner interface{ Scan(...any) error }

func scan(row scanner) (Relationship, error) {
	var (
		rel    Relationship
		pairID sql.NullString
	)
	if err := row.Scan(&rel.ID, &rel.ProjectID, &rel.FromID, &rel.ToID,
		&rel.Label, &rel.Notes, &pairID); err != nil {
		return Relationship{}, err
	}
	if pairID.Valid && pairID.String != "" {
		v := pairID.String
		rel.PairID = &v
	}
	return rel, nil
}
