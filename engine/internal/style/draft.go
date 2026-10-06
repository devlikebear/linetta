package style

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode/utf8"
)

// MaxProfileRunes bounds a style profile, proposed or approved. It is
// agentmemory's work-notes budget: the size this codebase has settled on for
// one block the story brief injects into every prompt.
const MaxProfileRunes = 2200

// How an approved draft joins the work's existing style notes.
const (
	ApproveReplace = "replace"
	ApproveAppend  = "append"
)

var (
	ErrDraftEmpty    = errors.New("style profile is empty")
	ErrDraftTooLong  = errors.New("style profile is over 2200 characters")
	ErrNoDraft       = errors.New("no style profile is waiting for approval")
	ErrApproveMode   = errors.New("approve mode must be replace or append")
	ErrNotesTooLong  = errors.New("style notes would be over 2200 characters")
	errDraftNotFound = sql.ErrNoRows
)

// Draft is a style profile waiting for the writer. It is not part of the
// work's style until the writer approves it: nothing reads this table when a
// story brief is built.
type Draft struct {
	ProjectID string `json:"project_id"`
	Body      string `json:"body"`
	// Author is who proposed it: "agent" for the built-in panel, "external"
	// for a client over MCP.
	Author    string `json:"author"`
	CreatedAt int64  `json:"created_at"`
}

// GetDraft returns the pending draft and whether there is one.
func (r *Repo) GetDraft(ctx context.Context, projectID string) (Draft, bool, error) {
	d := Draft{ProjectID: projectID}
	err := r.s.DB().QueryRowContext(ctx,
		"SELECT body, author, created_at FROM project_style_drafts WHERE project_id = ?", projectID).
		Scan(&d.Body, &d.Author, &d.CreatedAt)
	if errors.Is(err, errDraftNotFound) {
		return d, false, nil
	}
	return d, err == nil, err
}

// SaveDraft stores a proposed profile, replacing any earlier proposal. A body
// over the limit is refused rather than cut: a profile that stops mid-sentence
// is worse than one the proposer was told to shorten.
func (r *Repo) SaveDraft(ctx context.Context, now int64, projectID, body, author string) (Draft, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return Draft{}, ErrDraftEmpty
	}
	if utf8.RuneCountInString(body) > MaxProfileRunes {
		return Draft{}, ErrDraftTooLong
	}
	var exists bool
	if err := r.s.DB().QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM projects WHERE id = ?)", projectID).Scan(&exists); err != nil {
		return Draft{}, err
	}
	if !exists {
		return Draft{}, ErrProjectNotFound
	}
	d := Draft{ProjectID: projectID, Body: body, Author: author, CreatedAt: now}
	_, err := r.s.DB().ExecContext(ctx, `INSERT INTO project_style_drafts (project_id, body, author, created_at) VALUES (?,?,?,?)
ON CONFLICT(project_id) DO UPDATE SET body=excluded.body, author=excluded.author, created_at=excluded.created_at`,
		d.ProjectID, d.Body, d.Author, d.CreatedAt)
	return d, err
}

// DiscardDraft removes the pending draft. Discarding when there is none is
// not an error: the writer's intent — no draft — already holds.
func (r *Repo) DiscardDraft(ctx context.Context, projectID string) error {
	_, err := r.s.DB().ExecContext(ctx, "DELETE FROM project_style_drafts WHERE project_id = ?", projectID)
	return err
}

// ApproveDraft is the writer accepting a profile. body is the text they
// approved — the draft as proposed, or their edit of it — and mode says
// whether it replaces the work's style notes or is added after them. The
// notes are written and the draft removed in one transaction, and the new
// style notes are returned.
//
// This is the only path from a draft to the style notes, and so to the story
// brief. It is reachable from the app's RPC surface, not from an MCP tool.
func (r *Repo) ApproveDraft(ctx context.Context, now int64, projectID, body, mode string) (string, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", ErrDraftEmpty
	}
	if mode != ApproveReplace && mode != ApproveAppend {
		return "", ErrApproveMode
	}
	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	var pending int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM project_style_drafts WHERE project_id = ?", projectID).Scan(&pending); err != nil {
		return "", err
	}
	if pending == 0 {
		return "", ErrNoDraft
	}
	var current string
	if err := tx.QueryRowContext(ctx, "SELECT style_notes FROM projects WHERE id = ?", projectID).Scan(&current); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrProjectNotFound
		}
		return "", err
	}
	notes := body
	if mode == ApproveAppend && strings.TrimSpace(current) != "" {
		notes = strings.TrimSpace(current) + "\n\n" + body
	}
	if utf8.RuneCountInString(notes) > MaxProfileRunes {
		return "", ErrNotesTooLong
	}
	if _, err := tx.ExecContext(ctx, "UPDATE projects SET style_notes = ?, updated_at = ? WHERE id = ?", notes, now, projectID); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM project_style_drafts WHERE project_id = ?", projectID); err != nil {
		return "", err
	}
	return notes, tx.Commit()
}
