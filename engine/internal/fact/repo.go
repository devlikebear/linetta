package fact

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/google/uuid"
)

var ErrNotFound = errors.New("fact card not found")

type Repo struct{ s *store.Store }

func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

func (r *Repo) Create(ctx context.Context, now int64, in NewInput) (Card, error) {
	if strings.TrimSpace(in.ProjectID) == "" {
		return Card{}, fmt.Errorf("create fact: project_id required")
	}
	claim := strings.TrimSpace(in.Claim)
	if claim == "" {
		return Card{}, fmt.Errorf("create fact: claim required")
	}
	result := strings.TrimSpace(in.Result)
	if result == "" {
		return Card{}, fmt.Errorf("create fact: result required")
	}
	status := normalizeStatus(in.Status)
	if !ValidStatus(status) {
		return Card{}, fmt.Errorf("create fact: invalid status %q", in.Status)
	}
	sources := normalizedSources(in.Sources, now)
	if len(sources) == 0 {
		return Card{}, fmt.Errorf("create fact: at least one source URL required")
	}

	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return Card{}, err
	}
	defer tx.Rollback()

	id := uuid.NewString()
	if _, err := tx.ExecContext(ctx, `
INSERT INTO fact_cards (id, project_id, node_id, claim, result, status, category, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, in.ProjectID, in.NodeID, claim, result, status, strings.TrimSpace(in.Category), now, now); err != nil {
		return Card{}, err
	}
	for _, src := range sources {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO fact_sources (id, card_id, url, title, snippet, accessed_at)
VALUES (?, ?, ?, ?, ?, ?)`,
			uuid.NewString(), id, src.URL, src.Title, src.Snippet, src.AccessedAt); err != nil {
			return Card{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Card{}, err
	}
	return r.Get(ctx, id)
}

func (r *Repo) Get(ctx context.Context, id string) (Card, error) {
	row := r.s.DB().QueryRowContext(ctx, cardSelect+` WHERE id = ?`, id)
	card, err := scanCard(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Card{}, ErrNotFound
	}
	if err != nil {
		return Card{}, err
	}
	card.Sources, err = r.listSources(ctx, card.ID)
	return card, err
}

func (r *Repo) List(ctx context.Context, f ListFilter) ([]Card, error) {
	if strings.TrimSpace(f.ProjectID) == "" {
		return nil, fmt.Errorf("list facts: project_id required")
	}
	limit := f.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var (
		rows *sql.Rows
		err  error
	)
	if f.NodeID != nil && strings.TrimSpace(*f.NodeID) != "" {
		rows, err = r.s.DB().QueryContext(ctx, cardSelect+`
WHERE project_id = ? AND (node_id = ? OR node_id IS NULL)
ORDER BY updated_at DESC, created_at DESC
LIMIT ?`, f.ProjectID, *f.NodeID, limit)
	} else {
		rows, err = r.s.DB().QueryContext(ctx, cardSelect+`
WHERE project_id = ?
ORDER BY updated_at DESC, created_at DESC
LIMIT ?`, f.ProjectID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Card{}
	for rows.Next() {
		card, err := scanCard(rows)
		if err != nil {
			return nil, err
		}
		card.Sources, err = r.listSources(ctx, card.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, card)
	}
	return out, rows.Err()
}

func (r *Repo) Update(ctx context.Context, now int64, in UpdateInput) (Card, error) {
	if strings.TrimSpace(in.ID) == "" {
		return Card{}, fmt.Errorf("update fact: id required")
	}
	cur, err := r.Get(ctx, in.ID)
	if err != nil {
		return Card{}, err
	}
	if in.Claim != nil {
		cur.Claim = strings.TrimSpace(*in.Claim)
	}
	if in.Result != nil {
		cur.Result = strings.TrimSpace(*in.Result)
	}
	if in.Status != nil {
		cur.Status = normalizeStatus(*in.Status)
	}
	if in.Category != nil {
		cur.Category = strings.TrimSpace(*in.Category)
	}
	if cur.Claim == "" {
		return Card{}, fmt.Errorf("update fact: claim required")
	}
	if cur.Result == "" {
		return Card{}, fmt.Errorf("update fact: result required")
	}
	if !ValidStatus(cur.Status) {
		return Card{}, fmt.Errorf("update fact: invalid status %q", cur.Status)
	}
	res, err := r.s.DB().ExecContext(ctx, `
UPDATE fact_cards
   SET claim = ?, result = ?, status = ?, category = ?, updated_at = ?
 WHERE id = ?`, cur.Claim, cur.Result, cur.Status, cur.Category, now, in.ID)
	if err != nil {
		return Card{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Card{}, ErrNotFound
	}
	return r.Get(ctx, in.ID)
}

func (r *Repo) Delete(ctx context.Context, id string) error {
	res, err := r.s.DB().ExecContext(ctx, `DELETE FROM fact_cards WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repo) listSources(ctx context.Context, cardID string) ([]Source, error) {
	rows, err := r.s.DB().QueryContext(ctx, `
SELECT id, card_id, url, title, snippet, accessed_at
  FROM fact_sources
 WHERE card_id = ?
 ORDER BY rowid ASC`, cardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Source{}
	for rows.Next() {
		var s Source
		if err := rows.Scan(&s.ID, &s.CardID, &s.URL, &s.Title, &s.Snippet, &s.AccessedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func normalizedSources(srcs []SourceInput, now int64) []SourceInput {
	out := []SourceInput{}
	for _, src := range srcs {
		src.URL = strings.TrimSpace(src.URL)
		if src.URL == "" {
			continue
		}
		src.Title = strings.TrimSpace(src.Title)
		src.Snippet = strings.TrimSpace(src.Snippet)
		if src.AccessedAt <= 0 {
			src.AccessedAt = now
		}
		out = append(out, src)
	}
	return out
}

func normalizeStatus(status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return StatusUncertain
	}
	return status
}

const cardSelect = `
SELECT id, project_id, node_id, claim, result, status, category, created_at, updated_at
FROM fact_cards`

type scanner interface{ Scan(...any) error }

func scanCard(row scanner) (Card, error) {
	var (
		card   Card
		nodeID sql.NullString
	)
	if err := row.Scan(&card.ID, &card.ProjectID, &nodeID, &card.Claim, &card.Result, &card.Status, &card.Category, &card.CreatedAt, &card.UpdatedAt); err != nil {
		return Card{}, err
	}
	if nodeID.Valid {
		v := nodeID.String
		card.NodeID = &v
	}
	return card, nil
}
