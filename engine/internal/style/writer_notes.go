package style

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode/utf8"
)

// The writer's default style in prose (#182): what cannot be put as a rule —
// tone, rhythm, point-of-view habits — written once for every work. It goes
// into every brief as the writer's own direction, ahead of the work's style
// notes, which win where the two disagree. A work that does not follow the
// writer's default style gets neither these notes nor the rules.
//
// Only the writer sets the notes. An agent may propose a draft of them (#183),
// which the writer approves in the app, as with a work's style profile.

// MaxWriterNotesRunes bounds the notes. They ride in every brief of every
// work, so they are kept well under a work's own 2,200.
const MaxWriterNotesRunes = 1000

// ErrWriterNotesTooLong refuses notes, or an approved draft, over the limit.
// Refused rather than cut, like a work's style notes.
var ErrWriterNotesTooLong = errors.New("the writer's style notes would be over 1000 characters")

// GetWriterNotes returns the writer's style notes, "" until some are saved.
func (r *Repo) GetWriterNotes(ctx context.Context) (string, error) {
	var notes string
	err := r.s.DB().QueryRowContext(ctx, "SELECT notes FROM writer_style WHERE id = 1").Scan(&notes)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return notes, err
}

// SetWriterNotes replaces the writer's style notes. The rules beside them are
// left as they are.
func (r *Repo) SetWriterNotes(ctx context.Context, now int64, notes string) (string, error) {
	notes = strings.TrimSpace(notes)
	if utf8.RuneCountInString(notes) > MaxWriterNotesRunes {
		return "", ErrWriterNotesTooLong
	}
	_, err := r.s.DB().ExecContext(ctx, `INSERT INTO writer_style (id, notes, updated_at) VALUES (1,?,?)
ON CONFLICT(id) DO UPDATE SET notes=excluded.notes, updated_at=excluded.updated_at`, notes, now)
	return notes, err
}

// WriterNotesFor returns the writer's style notes as a brief of the work
// should carry them: none for a work that does not follow the writer's
// default style.
func (r *Repo) WriterNotesFor(ctx context.Context, projectID string) (string, error) {
	work, err := r.GetRules(ctx, projectID)
	if err != nil || work.IgnoreWriterStyle {
		return "", err
	}
	return r.GetWriterNotes(ctx)
}

// GetWriterDraft returns the proposed writer notes and whether there is one.
func (r *Repo) GetWriterDraft(ctx context.Context) (Draft, bool, error) {
	var d Draft
	err := r.s.DB().QueryRowContext(ctx, "SELECT body, author, created_at FROM writer_style_draft WHERE id = 1").
		Scan(&d.Body, &d.Author, &d.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Draft{}, false, nil
	}
	if err != nil {
		return Draft{}, false, err
	}
	return d, true, nil
}

// SaveWriterDraft stores an agent's proposal for the writer's style notes,
// replacing an earlier one. Nothing reads it until the writer approves.
func (r *Repo) SaveWriterDraft(ctx context.Context, now int64, body, author string) (Draft, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return Draft{}, ErrDraftEmpty
	}
	if utf8.RuneCountInString(body) > MaxWriterNotesRunes {
		return Draft{}, ErrWriterNotesTooLong
	}
	d := Draft{Body: body, Author: author, CreatedAt: now}
	_, err := r.s.DB().ExecContext(ctx, `INSERT INTO writer_style_draft (id, body, author, created_at) VALUES (1,?,?,?)
ON CONFLICT(id) DO UPDATE SET body=excluded.body, author=excluded.author, created_at=excluded.created_at`,
		d.Body, d.Author, d.CreatedAt)
	return d, err
}

// DiscardWriterDraft drops the proposal. Discarding none is not an error.
func (r *Repo) DiscardWriterDraft(ctx context.Context) error {
	_, err := r.s.DB().ExecContext(ctx, "DELETE FROM writer_style_draft WHERE id = 1")
	return err
}

// ApproveWriterDraft is the writer's approval: body — the draft or their edit
// of it — replaces the notes or is added after them, and the draft is removed,
// in one transaction. It is the only path from a draft to the notes, and it is
// on the app's RPC surface, not an MCP tool.
func (r *Repo) ApproveWriterDraft(ctx context.Context, now int64, body, mode string) (string, error) {
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
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM writer_style_draft WHERE id = 1").Scan(&pending); err != nil {
		return "", err
	}
	if pending == 0 {
		return "", ErrNoDraft
	}
	var current string
	if err := tx.QueryRowContext(ctx, "SELECT notes FROM writer_style WHERE id = 1").Scan(&current); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	notes := body
	if mode == ApproveAppend && strings.TrimSpace(current) != "" {
		notes = strings.TrimSpace(current) + "\n\n" + body
	}
	if utf8.RuneCountInString(notes) > MaxWriterNotesRunes {
		return "", ErrWriterNotesTooLong
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO writer_style (id, notes, updated_at) VALUES (1,?,?)
ON CONFLICT(id) DO UPDATE SET notes=excluded.notes, updated_at=excluded.updated_at`, notes, now); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM writer_style_draft WHERE id = 1"); err != nil {
		return "", err
	}
	return notes, tx.Commit()
}
