package project

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/google/uuid"
)

// ErrNotFound is returned by Get when the project id doesn't exist.
var ErrNotFound = errors.New("project not found")

var (
	ErrInvalidInput             = errors.New("invalid project input")
	ErrInvalidLengthTarget      = errors.New("invalid length_target")
	ErrInvalidDefaultPOV        = errors.New("invalid default_pov")
	ErrInvalidOutlinePreset     = errors.New("invalid outline_preset")
	ErrInvalidEpisodeCharTarget = errors.New("invalid episode_char_target")
)

// Repo persists Projects (and the auto-created first leaf node) in SQLite.
type Repo struct {
	s *store.Store
}

// NewRepo returns a Repo backed by the given Store.
func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

// Create inserts a new project at the given timestamp, plus its first leaf
// node ("씬 1"), and returns the resulting Project (with last_opened_node_id set).
func (r *Repo) Create(ctx context.Context, now int64, in NewInput) (Project, error) {
	if in.Title == "" {
		return Project{}, fmt.Errorf("%w: title required", ErrInvalidInput)
	}
	if !ValidLengthTarget(in.LengthTarget) {
		return Project{}, ErrInvalidLengthTarget
	}
	if !ValidDefaultPOV(in.DefaultPOV) {
		return Project{}, ErrInvalidDefaultPOV
	}
	outlinePreset := in.OutlinePreset
	if outlinePreset == "" {
		outlinePreset = OutlinePresetNovel
	}
	if !ValidOutlinePreset(outlinePreset) {
		return Project{}, ErrInvalidOutlinePreset
	}

	projectID := uuid.NewString()
	nodeID := uuid.NewString()
	genresJSON, err := json.Marshal(in.Genres)
	if err != nil {
		return Project{}, err
	}

	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return Project{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
INSERT INTO projects (id, title, genres, length_target, default_pov, style_notes, outline,
                      outline_preset, word_count, last_opened_node_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, '', '', ?, 0, ?, ?, ?)`,
		projectID, in.Title, string(genresJSON), in.LengthTarget, in.DefaultPOV,
		outlinePreset, nodeID, now, now); err != nil {
		return Project{}, err
	}
	// Empty Tiptap doc: single empty paragraph.
	const emptyDoc = `{"type":"doc","content":[{"type":"paragraph"}]}`
	if outlinePreset == OutlinePresetWebNovel {
		// Webnovel projects open on a 1화 episode leaf under a 1권 arc so the
		// outline matches the preset from the first keystroke.
		arcID := uuid.NewString()
		if _, err := tx.ExecContext(ctx, `
INSERT INTO nodes (id, project_id, parent_id, ordinal, kind, label, title,
                   content_doc, status, word_count, created_at, updated_at)
VALUES (?, ?, NULL, 0, 'container', '1권', '', NULL, 'draft', 0, ?, ?)`,
			arcID, projectID, now, now); err != nil {
			return Project{}, err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO nodes (id, project_id, parent_id, ordinal, kind, label, title,
                   content_doc, status, word_count, created_at, updated_at)
VALUES (?, ?, ?, 0, 'leaf', '1화', '', ?, 'draft', 0, ?, ?)`,
			nodeID, projectID, arcID, emptyDoc, now, now); err != nil {
			return Project{}, err
		}
	} else if _, err := tx.ExecContext(ctx, `
INSERT INTO nodes (id, project_id, parent_id, ordinal, kind, label, title,
                   content_doc, status, word_count, created_at, updated_at)
VALUES (?, ?, NULL, 0, 'leaf', '씬 1', '', ?, 'draft', 0, ?, ?)`,
		nodeID, projectID, emptyDoc, now, now); err != nil {
		return Project{}, err
	}
	if err := tx.Commit(); err != nil {
		return Project{}, err
	}

	return r.Get(ctx, projectID)
}

// Get returns the project by id, or ErrNotFound.
func (r *Repo) Get(ctx context.Context, id string) (Project, error) {
	row := r.s.DB().QueryRowContext(ctx, baseSelect+` WHERE id = ?`, id)
	p, err := scan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	return p, err
}

// List returns recent projects sorted by updated_at DESC.
func (r *Repo) List(ctx context.Context, f ListFilter) ([]Project, error) {
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := baseSelect
	if !f.IncludeArchived {
		q += ` WHERE archived_at IS NULL`
	}
	q += ` ORDER BY updated_at DESC LIMIT ?`

	rows, err := r.s.DB().QueryContext(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		p, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Archive soft-deletes by setting archived_at; idempotent on already-archived.
func (r *Repo) Archive(ctx context.Context, id string, now int64) error {
	res, err := r.s.DB().ExecContext(ctx, `
UPDATE projects SET archived_at = COALESCE(archived_at, ?), updated_at = ?
WHERE id = ?`, now, now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Restore reactivates an archived project by clearing archived_at.
func (r *Repo) Restore(ctx context.Context, id string, now int64) error {
	res, err := r.s.DB().ExecContext(ctx, `
UPDATE projects SET archived_at = NULL, updated_at = ?
WHERE id = ?`, now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete permanently removes a project and all rows that depend on it.
func (r *Repo) Delete(ctx context.Context, id string) error {
	res, err := r.s.DB().ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Update patches editable fields and bumps updated_at.
// The read of the current row and the UPDATE share a single transaction to
// avoid losing an interleaved write under concurrent access.
func (r *Repo) Update(ctx context.Context, now int64, in UpdateInput) (Project, error) {
	if in.ID == "" {
		return Project{}, fmt.Errorf("update project: id required")
	}
	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return Project{}, err
	}
	defer tx.Rollback()

	row := tx.QueryRowContext(ctx, baseSelect+` WHERE id = ?`, in.ID)
	cur, err := scan(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Project{}, ErrNotFound
		}
		return Project{}, err
	}
	if in.Outline != nil {
		cur.Outline = *in.Outline
	}
	if in.Synopsis != nil {
		cur.Synopsis = *in.Synopsis
	}
	if in.OutlinePreset != nil {
		if !ValidOutlinePreset(*in.OutlinePreset) {
			return Project{}, ErrInvalidOutlinePreset
		}
		cur.OutlinePreset = *in.OutlinePreset
	}
	if in.EpisodeCharTarget != nil {
		if *in.EpisodeCharTarget <= 0 {
			return Project{}, ErrInvalidEpisodeCharTarget
		}
		cur.EpisodeCharTarget = *in.EpisodeCharTarget
	}
	if in.Title != nil {
		if *in.Title == "" {
			return Project{}, fmt.Errorf("%w: title required", ErrInvalidInput)
		}
		cur.Title = *in.Title
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE projects SET title = ?, outline = ?, outline_preset = ?, episode_char_target = ?, synopsis = ?, updated_at = ? WHERE id = ?`,
		cur.Title, cur.Outline, cur.OutlinePreset, cur.EpisodeCharTarget, cur.Synopsis, now, in.ID); err != nil {
		return Project{}, err
	}
	if err := tx.Commit(); err != nil {
		return Project{}, err
	}
	return r.Get(ctx, in.ID)
}

const baseSelect = `
SELECT id, title, genres, length_target, default_pov, style_notes, outline, outline_preset, episode_char_target, synopsis,
       word_count, last_opened_node_id, created_at, updated_at, archived_at
FROM projects`

// scanner is the small subset of *sql.Row / *sql.Rows we need.
type scanner interface {
	Scan(...any) error
}

func scan(row scanner) (Project, error) {
	var (
		p          Project
		genresJSON string
		lastNode   sql.NullString
		archivedAt sql.NullInt64
	)
	if err := row.Scan(&p.ID, &p.Title, &genresJSON, &p.LengthTarget, &p.DefaultPOV,
		&p.StyleNotes, &p.Outline, &p.OutlinePreset, &p.EpisodeCharTarget, &p.Synopsis, &p.WordCount, &lastNode, &p.CreatedAt, &p.UpdatedAt, &archivedAt); err != nil {
		return Project{}, err
	}
	if err := json.Unmarshal([]byte(genresJSON), &p.Genres); err != nil {
		return Project{}, fmt.Errorf("decode genres: %w", err)
	}
	if lastNode.Valid {
		v := lastNode.String
		p.LastOpenedNodeID = &v
	}
	if archivedAt.Valid {
		v := archivedAt.Int64
		p.ArchivedAt = &v
	}
	return p, nil
}
