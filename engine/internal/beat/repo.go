package beat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/google/uuid"
)

// ErrNotFound is returned when a beat id does not exist.
var ErrNotFound = errors.New("beat not found")

// Repo persists Beats in SQLite.
type Repo struct {
	s *store.Store
}

func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

// Create assigns the next ordinal within the thread (atomic in a transaction
// to avoid collisions on concurrent inserts) and inserts the row. Intensity
// is clamped to 1..3; 0 defaults to 1.
func (r *Repo) Create(ctx context.Context, in NewInput) (Beat, error) {
	if in.ThreadID == "" {
		return Beat{}, fmt.Errorf("create beat: thread_id required")
	}
	intensity := clampIntensity(in.Intensity)

	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return Beat{}, err
	}
	defer tx.Rollback()

	var maxOrd sql.NullInt64
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(ordinal), 0) FROM beats WHERE thread_id = ?`,
		in.ThreadID).Scan(&maxOrd); err != nil {
		return Beat{}, err
	}
	ordinal := int(maxOrd.Int64) + 1

	id := uuid.NewString()
	if _, err := tx.ExecContext(ctx, `
INSERT INTO beats (id, thread_id, node_id, ordinal, label, description, intensity)
VALUES (?, ?, ?, ?, ?, ?, ?)`, id, in.ThreadID, nullStr(in.NodeID), ordinal, in.Label, in.Description, intensity); err != nil {
		return Beat{}, err
	}
	if err := tx.Commit(); err != nil {
		return Beat{}, err
	}
	return r.Get(ctx, id)
}

// Get returns one beat by id.
func (r *Repo) Get(ctx context.Context, id string) (Beat, error) {
	row := r.s.DB().QueryRowContext(ctx, baseSelect+` WHERE id = ?`, id)
	b, err := scan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Beat{}, ErrNotFound
	}
	return b, err
}

// ListByThread returns the thread's beats ordered by ordinal ASC.
func (r *Repo) ListByThread(ctx context.Context, threadID string) ([]Beat, error) {
	rows, err := r.s.DB().QueryContext(ctx,
		baseSelect+` WHERE thread_id = ? ORDER BY ordinal ASC`, threadID)
	if err != nil {
		return nil, err
	}
	return scanAll(rows)
}

// ListByNode returns every beat bound to the node, ordered by (thread_id, ordinal).
// Beats unbound (node_id IS NULL) are excluded.
func (r *Repo) ListByNode(ctx context.Context, nodeID string) ([]Beat, error) {
	rows, err := r.s.DB().QueryContext(ctx,
		baseSelect+` WHERE node_id = ? ORDER BY thread_id, ordinal`, nodeID)
	if err != nil {
		return nil, err
	}
	return scanAll(rows)
}

// Update applies a partial input.
func (r *Repo) Update(ctx context.Context, in UpdateInput) error {
	if in.ID == "" {
		return fmt.Errorf("update beat: id required")
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
	if in.Label != "" {
		cur.Label = in.Label
	}
	if in.Description != nil {
		cur.Description = *in.Description
	}
	if in.Intensity != 0 {
		cur.Intensity = clampIntensity(in.Intensity)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE beats SET label = ?, description = ?, intensity = ? WHERE id = ?`,
		cur.Label, cur.Description, cur.Intensity, in.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// Reorder rewrites the thread's beat ordinals according to the supplied id slice.
// The slice MUST be a permutation of the thread's existing beat ids; otherwise
// the function returns an error and leaves the table untouched.
func (r *Repo) Reorder(ctx context.Context, threadID string, ids []string) error {
	existing, err := r.ListByThread(ctx, threadID)
	if err != nil {
		return err
	}
	if len(existing) != len(ids) {
		return fmt.Errorf("reorder: got %d ids, thread has %d beats", len(ids), len(existing))
	}
	have := map[string]bool{}
	for _, b := range existing {
		have[b.ID] = true
	}
	for _, id := range ids {
		if !have[id] {
			return fmt.Errorf("reorder: id %s not in thread %s", id, threadID)
		}
	}

	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Two-phase: bump every ordinal by a large offset first, then write the
	// final values. This sidesteps any future UNIQUE(thread_id, ordinal)
	// constraint without changing semantics today.
	if _, err := tx.ExecContext(ctx,
		`UPDATE beats SET ordinal = ordinal + 1000000 WHERE thread_id = ?`, threadID); err != nil {
		return err
	}
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx,
			`UPDATE beats SET ordinal = ? WHERE id = ?`, i+1, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Delete removes the beat.
func (r *Repo) Delete(ctx context.Context, id string) error {
	res, err := r.s.DB().ExecContext(ctx, `DELETE FROM beats WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

const baseSelect = `
SELECT id, thread_id, node_id, ordinal, label, description, intensity
FROM beats`

type scanner interface{ Scan(...any) error }

func scan(row scanner) (Beat, error) {
	var (
		b      Beat
		nodeID sql.NullString
	)
	if err := row.Scan(&b.ID, &b.ThreadID, &nodeID, &b.Ordinal, &b.Label, &b.Description, &b.Intensity); err != nil {
		return Beat{}, err
	}
	if nodeID.Valid {
		v := nodeID.String
		b.NodeID = &v
	}
	return b, nil
}

func scanAll(rows *sql.Rows) ([]Beat, error) {
	defer rows.Close()
	var out []Beat
	for rows.Next() {
		b, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func clampIntensity(v int) int {
	if v <= 0 {
		return 1
	}
	if v > 3 {
		return 3
	}
	return v
}

func nullStr(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}
