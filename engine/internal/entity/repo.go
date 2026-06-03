package entity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/google/uuid"
)

// ErrNotFound is returned when an entity id does not exist.
var ErrNotFound = errors.New("entity not found")

// Repo persists Entities in SQLite.
type Repo struct {
	s *store.Store
}

func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

// Create inserts a new entity. Returns an error if the unique(project_id, name)
// constraint is violated.
func (r *Repo) Create(ctx context.Context, now int64, in NewInput) (Entity, error) {
	if in.ProjectID == "" || in.Name == "" {
		return Entity{}, fmt.Errorf("create entity: project_id and name required")
	}
	kind := in.Kind
	if kind == "" {
		kind = KindCharacter
	}
	id := uuid.NewString()
	_, err := r.s.DB().ExecContext(ctx, `
INSERT INTO entities (id, project_id, kind, name, aliases, role, summary, attributes,
                      created_at, updated_at)
VALUES (?, ?, ?, ?, '[]', ?, '', '{}', ?, ?)`,
		id, in.ProjectID, kind, in.Name, in.Role, now, now)
	if err != nil {
		return Entity{}, err
	}
	return r.Get(ctx, id)
}

// Get returns one entity by id.
func (r *Repo) Get(ctx context.Context, id string) (Entity, error) {
	row := r.s.DB().QueryRowContext(ctx, baseSelect+` WHERE id = ?`, id)
	e, err := scan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Entity{}, ErrNotFound
	}
	return e, err
}

// Update applies a partial input. Empty strings leave fields alone; an Attributes
// pointer to a (possibly empty) map overwrites the JSON column.
func (r *Repo) Update(ctx context.Context, now int64, in UpdateInput) error {
	if in.ID == "" {
		return fmt.Errorf("update entity: id required")
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

	if in.Kind != "" {
		cur.Kind = in.Kind
	}
	if in.Name != "" {
		cur.Name = in.Name
	}
	cur.Role = in.Role
	cur.Summary = in.Summary
	if in.Attributes != nil {
		cur.Attributes = *in.Attributes
	}

	attrsJSON, err := json.Marshal(cur.Attributes)
	if err != nil {
		return err
	}
	aliasesJSON, err := json.Marshal(cur.Aliases)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE entities
   SET kind = ?, name = ?, aliases = ?, role = ?, summary = ?, attributes = ?, updated_at = ?
 WHERE id = ?`, cur.Kind, cur.Name, string(aliasesJSON), cur.Role, cur.Summary,
		string(attrsJSON), now, in.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// Search returns entities whose name contains `query` (case-insensitive), shortest
// name first; with an empty query it returns the project's most-recently-updated
// entities. Limit is capped at 50.
func (r *Repo) Search(ctx context.Context, projectID, query string, limit int) ([]Entity, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	q := strings.ToLower(strings.TrimSpace(query))

	var (
		rows *sql.Rows
		err  error
	)
	if q == "" {
		rows, err = r.s.DB().QueryContext(ctx, baseSelect+`
WHERE project_id = ?
ORDER BY updated_at DESC
LIMIT ?`, projectID, limit)
	} else {
		rows, err = r.s.DB().QueryContext(ctx, baseSelect+`
WHERE project_id = ? AND LOWER(name) LIKE ?
ORDER BY (LOWER(name) NOT LIKE ?) ASC, LENGTH(name) ASC, updated_at DESC
LIMIT ?`, projectID, "%"+q+"%", q+"%", limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entity
	for rows.Next() {
		e, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repo) ListByProject(ctx context.Context, projectID string) ([]Entity, error) {
	rows, err := r.s.DB().QueryContext(ctx, baseSelect+`
WHERE project_id = ?
ORDER BY updated_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	return ScanAll(rows)
}

// ListCoreByProject returns entities with story-skeleton roles such as
// 주인공/빌런/메인무대/특별한 장소, newest first within that core set.
func (r *Repo) ListCoreByProject(ctx context.Context, projectID string, limit int) ([]Entity, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := r.s.DB().QueryContext(ctx, `
SELECT id, project_id, kind, name, aliases, role, summary, attributes,
       created_at, updated_at
  FROM entities
 WHERE project_id = ?
 ORDER BY updated_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Entity{}
	for rows.Next() {
		e, err := scan(rows)
		if err != nil {
			return nil, err
		}
		if !IsCoreEntity(e) {
			continue
		}
		out = append(out, e)
		if len(out) >= limit {
			break
		}
	}
	return out, rows.Err()
}

const baseSelect = `
SELECT id, project_id, kind, name, aliases, role, summary, attributes,
       created_at, updated_at
FROM entities`

type scanner interface{ Scan(...any) error }

// Scan consumes one row whose columns match baseSelect.
func Scan(row scanner) (Entity, error) {
	return scan(row)
}

// ScanAll consumes a *sql.Rows (or compatible) whose columns match baseSelect
// and returns the full slice. Exposed for cross-package callers (e.g. mention.Repo).
func ScanAll(rows interface {
	Next() bool
	Scan(...any) error
	Close() error
	Err() error
}) ([]Entity, error) {
	defer rows.Close()
	var out []Entity
	for rows.Next() {
		e, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func scan(row scanner) (Entity, error) {
	var (
		e        Entity
		aliases  string
		attrsRaw string
	)
	if err := row.Scan(&e.ID, &e.ProjectID, &e.Kind, &e.Name, &aliases, &e.Role,
		&e.Summary, &attrsRaw, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return Entity{}, err
	}
	if aliases == "" {
		e.Aliases = []string{}
	} else if err := json.Unmarshal([]byte(aliases), &e.Aliases); err != nil {
		return Entity{}, fmt.Errorf("decode aliases: %w", err)
	}
	if attrsRaw == "" {
		e.Attributes = map[string]string{}
	} else if err := json.Unmarshal([]byte(attrsRaw), &e.Attributes); err != nil {
		return Entity{}, fmt.Errorf("decode attributes: %w", err)
	}
	if e.Aliases == nil {
		e.Aliases = []string{}
	}
	if e.Attributes == nil {
		e.Attributes = map[string]string{}
	}
	return e, nil
}
