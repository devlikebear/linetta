package agentmemory

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/devlikebear/linetta/engine/internal/ftsquery"
)

// ActionArchive moves a line of a work's notes into the archive. It is not one
// of Apply's actions because it writes a second table, so it has its own entry
// point, Repo.Archive.
const ActionArchive = "archive"

const (
	// DefaultArchiveLimit is how many archived lines a listing returns.
	DefaultArchiveLimit = 200
	archiveSearchLimit  = 10
)

// ErrNoArchivedLine means the id names no archived line of this work.
var ErrNoArchivedLine = errors.New("agentmemory: no archived line with that id in this work")

// Archived is one line that left a work's notes.
type Archived struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Body      string `json:"body"`
	// SourceMessageID is the conversation message the line came from, when
	// the agent named one. It may no longer resolve: the conversation stays on
	// the device that had it.
	SourceMessageID string `json:"source_message_id,omitempty"`
	ArchivedAt      int64  `json:"archived_at"`
}

// ArchivedHit is an archived line that matched a search. Body is the whole
// line: one line is short, and a snippet of it would only lose the end.
type ArchivedHit = Archived

// Archive moves the one line matching find out of the work's notes. Both writes
// are one transaction, for the reason Edit gives and because a line must not
// end up in both places or in neither. It is remove, not a new budget rule: the
// result is always shorter, so it needs no budget check of its own.
func (r *Repo) Archive(ctx context.Context, projectID, find, sourceMessageID string, now int64) (Document, error) {
	arg, err := projectArg(ScopeWorkNotes, projectID)
	if err != nil {
		return Document{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Document{}, err
	}
	defer func() { _ = tx.Rollback() }()

	current, err := bodyBefore(ctx, tx, ScopeWorkNotes, arg)
	if err != nil {
		return Document{}, err
	}
	i, err := matchLine(current, find)
	if err != nil {
		return Document{}, err
	}
	line := splitLines(current)[i]
	next, err := Apply(ScopeWorkNotes, current, ActionRemove, find, "")
	if err != nil {
		return Document{}, err
	}
	if err := replaceBody(ctx, tx, ScopeWorkNotes, arg, next, now); err != nil {
		return Document{}, asWriteError(err, projectID)
	}
	if err := insertArchived(ctx, tx, uuid.NewString(), projectID, line, sourceMessageID, now); err != nil {
		return Document{}, asWriteError(err, projectID)
	}
	if err := tx.Commit(); err != nil {
		return Document{}, err
	}
	return Document{
		Scope: ScopeWorkNotes, ProjectID: strings.TrimSpace(projectID), Body: next,
		CharsUsed: utf8.RuneCountInString(next), CharsBudget: ScopeWorkNotes.Budget(), UpdatedAt: now,
	}, nil
}

func insertArchived(ctx context.Context, q interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, id, projectID, body, sourceMessageID string, now int64) error {
	_, err := q.ExecContext(ctx, `
INSERT OR IGNORE INTO agent_memory_archive (id, project_id, body, source_message_id, archived_at)
VALUES (?, ?, ?, ?, ?)`, id, strings.TrimSpace(projectID), body, strings.TrimSpace(sourceMessageID), now)
	return err
}

// Remember records a line straight into the archive. It is what the remember
// story op does: a fact worth finding later, with no claim on the notes' budget.
func (r *Repo) Remember(ctx context.Context, projectID, text string, now int64) error {
	line, err := oneLine(text)
	if err != nil {
		return err
	}
	if err := insertArchived(ctx, r.db, uuid.NewString(), projectID, line, "", now); err != nil {
		return asWriteError(err, projectID)
	}
	return nil
}

// ImportArchived records a line from the old experiences.jsonl log. The id comes
// from the work and the line, so importing the same line twice keeps one row.
func (r *Repo) ImportArchived(ctx context.Context, projectID, text string, now int64) error {
	line, err := oneLine(text)
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(projectID) + "\x00" + line))
	if err := insertArchived(ctx, r.db, "import-"+hex.EncodeToString(sum[:16]), projectID, line, "", now); err != nil {
		return asWriteError(err, projectID)
	}
	return nil
}

// ListArchived returns the work's archived lines, newest first.
func (r *Repo) ListArchived(ctx context.Context, projectID string, limit int) ([]Archived, error) {
	if limit <= 0 {
		limit = DefaultArchiveLimit
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT id, project_id, body, source_message_id, archived_at
  FROM agent_memory_archive
 WHERE project_id = ?
 ORDER BY archived_at DESC, id
 LIMIT ?`, strings.TrimSpace(projectID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanArchived(rows)
}

func scanArchived(rows *sql.Rows) ([]Archived, error) {
	out := []Archived{}
	for rows.Next() {
		var a Archived
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.Body, &a.SourceMessageID, &a.ArchivedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SearchArchived returns the archived lines holding every word of query, best
// match first. Words under three characters go through LIKE, as everywhere the
// trigram index is searched.
func (r *Repo) SearchArchived(ctx context.Context, projectID, query string, limit int) ([]ArchivedHit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []ArchivedHit{}, nil
	}
	if limit <= 0 || limit > archiveSearchLimit {
		limit = archiveSearchLimit
	}
	terms := ftsquery.Terms(query)
	var (
		rows *sql.Rows
		err  error
	)
	if match, ok := ftsquery.TrigramMatch(terms); ok {
		rows, err = r.db.QueryContext(ctx, `
SELECT a.id, a.project_id, a.body, a.source_message_id, a.archived_at
  FROM agent_memory_archive_fts
  JOIN agent_memory_archive a ON a.id = agent_memory_archive_fts.archive_id
 WHERE agent_memory_archive_fts MATCH ? AND agent_memory_archive_fts.project_id = ?
 ORDER BY bm25(agent_memory_archive_fts)
 LIMIT ?`, match, strings.TrimSpace(projectID), limit)
	} else {
		likes := make([]string, 0, len(terms))
		args := []any{strings.TrimSpace(projectID)}
		for _, term := range terms {
			likes = append(likes, `body LIKE ? ESCAPE '\'`)
			args = append(args, "%"+ftsquery.EscapeLike(term)+"%")
		}
		rows, err = r.db.QueryContext(ctx, `
SELECT id, project_id, body, source_message_id, archived_at
  FROM agent_memory_archive
 WHERE project_id = ? AND `+strings.Join(likes, " AND ")+`
 ORDER BY archived_at DESC, id
 LIMIT ?`, append(args, limit)...)
	}
	if err != nil {
		return nil, fmt.Errorf("search archived memory: %w", err)
	}
	defer rows.Close()
	return scanArchived(rows)
}

// DeleteArchived forgets an archived line for good.
func (r *Repo) DeleteArchived(ctx context.Context, projectID, id string) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM agent_memory_archive WHERE id = ? AND project_id = ?`, strings.TrimSpace(id), strings.TrimSpace(projectID))
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNoArchivedLine
	}
	return nil
}

// RestoreArchived moves an archived line back to the end of the notes. The
// budget is the notes' ordinary one: a full document refuses, and the line stays
// archived.
func (r *Repo) RestoreArchived(ctx context.Context, projectID, id string, now int64) (Document, error) {
	arg, err := projectArg(ScopeWorkNotes, projectID)
	if err != nil {
		return Document{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Document{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var line string
	err = tx.QueryRowContext(ctx, `SELECT body FROM agent_memory_archive WHERE id = ? AND project_id = ?`,
		strings.TrimSpace(id), strings.TrimSpace(projectID)).Scan(&line)
	if errors.Is(err, sql.ErrNoRows) {
		return Document{}, ErrNoArchivedLine
	}
	if err != nil {
		return Document{}, err
	}
	current, err := bodyBefore(ctx, tx, ScopeWorkNotes, arg)
	if err != nil {
		return Document{}, err
	}
	next, err := Apply(ScopeWorkNotes, current, ActionAdd, "", line)
	if err != nil {
		return Document{}, err
	}
	if err := replaceBody(ctx, tx, ScopeWorkNotes, arg, next, now); err != nil {
		return Document{}, asWriteError(err, projectID)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM agent_memory_archive WHERE id = ?`, strings.TrimSpace(id)); err != nil {
		return Document{}, err
	}
	if err := tx.Commit(); err != nil {
		return Document{}, err
	}
	return Document{
		Scope: ScopeWorkNotes, ProjectID: strings.TrimSpace(projectID), Body: next,
		CharsUsed: utf8.RuneCountInString(next), CharsBudget: ScopeWorkNotes.Budget(), UpdatedAt: now,
	}, nil
}
