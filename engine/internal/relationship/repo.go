package relationship

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

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
	if err := insert(ctx, r.s.DB(), row{
		id: id, projectID: in.ProjectID, fromID: in.FromID, toID: in.ToID,
		label: in.Label, notes: in.Notes, since: in.SinceNodeID, quote: in.SinceQuote,
	}); err != nil {
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

	if err := insert(ctx, tx, row{
		id: forwardID, projectID: in.ProjectID, fromID: in.FromID, toID: in.ToID,
		label: in.Label, notes: in.Notes, pairID: pairID, since: in.SinceNodeID, quote: in.SinceQuote,
	}); err != nil {
		return nil, err
	}
	if err := insert(ctx, tx, row{
		id: inverseID, projectID: in.ProjectID, fromID: in.ToID, toID: in.FromID,
		label: in.InverseLabel, pairID: pairID, since: in.SinceNodeID, quote: in.SinceQuote,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.getAll(ctx, forwardID, inverseID)
}

// ErrAlreadyChanged is Change on a row a later one already took over from.
var ErrAlreadyChanged = errors.New("relationship already changed later in the story; change the newer one")

// Change ends the row, and its partner, at in.AtNodeID and starts their
// successors there in one transaction. Return order matches CreatePair:
// [the row's successor, the partner's successor when paired]. Whether
// AtNodeID is a scene after the row's start is the caller's to check: the
// repo does not know the story order.
func (r *Repo) Change(ctx context.Context, in ChangeInput) ([]Relationship, error) {
	if in.ID == "" || in.AtNodeID == "" || in.Label == "" {
		return nil, fmt.Errorf("change relationship: id, at_node_id, label required")
	}
	old, err := r.Get(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	if old.UntilNodeID != nil {
		return nil, ErrAlreadyChanged
	}
	if old.SinceNodeID != nil && *old.SinceNodeID == in.AtNodeID {
		return nil, fmt.Errorf("change relationship: it already starts in that scene; edit its label instead")
	}
	var partner *Relationship
	if old.PairID != nil {
		rows, err := r.listWhere(ctx, ` WHERE pair_id = ? AND id <> ?`, *old.PairID, old.ID)
		if err != nil {
			return nil, err
		}
		if len(rows) > 0 {
			partner = &rows[0]
		}
	}

	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	ids := []string{uuid.NewString()}
	next := row{
		id: ids[0], projectID: old.ProjectID, fromID: old.FromID, toID: old.ToID,
		label: in.Label, notes: old.Notes, since: in.AtNodeID, quote: in.Quote, supersedes: old.ID,
	}
	ended := []string{old.ID}
	if partner != nil {
		next.pairID = uuid.NewString()
		inverse := strings.TrimSpace(in.InverseLabel)
		if inverse == "" {
			inverse = partner.Label
		}
		ids = append(ids, uuid.NewString())
		if err := insert(ctx, tx, row{
			id: ids[1], projectID: old.ProjectID, fromID: partner.FromID, toID: partner.ToID,
			label: inverse, notes: partner.Notes, pairID: next.pairID, since: in.AtNodeID, quote: in.Quote,
			supersedes: partner.ID,
		}); err != nil {
			return nil, err
		}
		ended = append(ended, partner.ID)
	}
	if err := insert(ctx, tx, next); err != nil {
		return nil, err
	}
	for _, id := range ended {
		if _, err := tx.ExecContext(ctx, `UPDATE relationships SET until_node_id = ? WHERE id = ?`, in.AtNodeID, id); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.getAll(ctx, ids...)
}

// SetSpan writes where a row sits in the story as given.
func (r *Repo) SetSpan(ctx context.Context, in SpanInput) error {
	res, err := r.s.DB().ExecContext(ctx,
		`UPDATE relationships SET since_node_id = ?, until_node_id = ?, since_quote = ?, supersedes_id = ? WHERE id = ?`,
		nullable(in.SinceNodeID), nullable(in.UntilNodeID), in.SinceQuote, nullable(in.SupersedesID), in.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func nullable(p *string) any {
	if p == nil || *p == "" {
		return nil
	}
	return *p
}

// row is one insert. Empty pairID, since and supersedes are NULL.
type row struct {
	id, projectID, fromID, toID, label, notes string
	pairID, since, quote, supersedes          string
}

type execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func insert(ctx context.Context, db execer, in row) error {
	null := func(s string) any {
		if s == "" {
			return nil
		}
		return s
	}
	_, err := db.ExecContext(ctx, `
INSERT INTO relationships (id, project_id, from_id, to_id, label, notes, pair_id, since_node_id, since_quote, supersedes_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.id, in.projectID, in.fromID, in.toID, in.label, in.notes,
		null(in.pairID), null(in.since), in.quote, null(in.supersedes))
	return err
}

func (r *Repo) getAll(ctx context.Context, ids ...string) ([]Relationship, error) {
	out := make([]Relationship, 0, len(ids))
	for _, id := range ids {
		rel, err := r.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, rel)
	}
	return out, nil
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
// is stable and close enough to insertion order for this scope). Rows a change
// ended are included; Current or AsOf narrows them.
func (r *Repo) ListByEntity(ctx context.Context, entityID string) ([]Relationship, error) {
	return r.listWhere(ctx, ` WHERE from_id = ? ORDER BY id`, entityID)
}

// ListByProject returns every relationship row in the project, ordered by id.
// Both directions of a pair are returned as separate rows; callers dedupe by
// pair_id if needed. Rows a change ended are included too.
func (r *Repo) ListByProject(ctx context.Context, projectID string) ([]Relationship, error) {
	return r.listWhere(ctx, ` WHERE project_id = ? ORDER BY id`, projectID)
}

func (r *Repo) listWhere(ctx context.Context, where string, args ...any) ([]Relationship, error) {
	rows, err := r.s.DB().QueryContext(ctx, baseSelect+where, args...)
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

// Removal captures what Delete would take: the row and its pair, if any.
func (r *Repo) Removal(ctx context.Context, id string) (store.RowSet, error) {
	return r.s.Capture(ctx, store.Slice{
		Table: "relationships",
		Where: "id = ? OR pair_id = (SELECT pair_id FROM relationships WHERE id = ?)",
		Args:  []any{id, id},
	})
}

const baseSelect = `
SELECT id, project_id, from_id, to_id, label, notes, pair_id, since_node_id, until_node_id, since_quote, supersedes_id
FROM relationships`

type scanner interface{ Scan(...any) error }

func scan(row scanner) (Relationship, error) {
	var (
		rel                            Relationship
		pairID, since, until, replaced sql.NullString
	)
	if err := row.Scan(&rel.ID, &rel.ProjectID, &rel.FromID, &rel.ToID,
		&rel.Label, &rel.Notes, &pairID, &since, &until, &rel.SinceQuote, &replaced); err != nil {
		return Relationship{}, err
	}
	rel.PairID = ptr(pairID)
	rel.SinceNodeID = ptr(since)
	rel.UntilNodeID = ptr(until)
	rel.SupersedesID = ptr(replaced)
	return rel, nil
}

func ptr(v sql.NullString) *string {
	if !v.Valid || v.String == "" {
		return nil
	}
	s := v.String
	return &s
}
