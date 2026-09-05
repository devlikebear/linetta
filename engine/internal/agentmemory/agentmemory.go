package agentmemory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Scope names one of the two documents.
type Scope string

const (
	// ScopeWriterProfile is global: how this writer works, across every work.
	ScopeWriterProfile Scope = "writer_profile"
	// ScopeWorkNotes is per-work: what an agent has learned about one book.
	ScopeWorkNotes Scope = "work_notes"
)

// Budgets in RUNES. 2200 is referencePromptRunes (companion/references.go:34),
// this codebase's established size for one prompt-injected block. The profile
// is smaller because it is in every turn of every work.
const (
	writerProfileBudget = 1400
	workNotesBudget     = 2200
)

func (s Scope) Budget() int {
	switch s {
	case ScopeWriterProfile:
		return writerProfileBudget
	case ScopeWorkNotes:
		return workNotesBudget
	}
	return 0
}

func (s Scope) Valid() bool { return s.Budget() > 0 }

// ParseScope converts a value off the wire. An unknown scope is an error
// rather than a zero value, so a typo from a model is told, not ignored.
func ParseScope(v string) (Scope, error) {
	s := Scope(strings.TrimSpace(v))
	if !s.Valid() {
		return "", fmt.Errorf("agentmemory: unknown scope %q; use %q or %q", v, ScopeWriterProfile, ScopeWorkNotes)
	}
	return s, nil
}

// ErrOverBudget is returned when a save would exceed the scope's budget. The
// answer is to replace or remove a line first, which is why this surfaces
// rather than truncating: a silent truncation would drop the end of what the
// writer just said to remember.
var ErrOverBudget = errors.New("agentmemory: over budget")

// Document is one memory, carrying enough of its budget that a caller can
// render a capacity line without asking twice.
type Document struct {
	Scope       Scope  `json:"scope"`
	ProjectID   string `json:"project_id,omitempty"`
	Body        string `json:"body"`
	CharsUsed   int    `json:"chars_used"`
	CharsBudget int    `json:"chars_budget"`
	UpdatedAt   int64  `json:"updated_at"`
}

// Repo reads and writes the agent_memory table.
type Repo struct{ db *sql.DB }

func NewRepo(db *sql.DB) *Repo { return &Repo{db: db} }

// projectArg maps the scope's project id onto the nullable column. The global
// row is NULL, not ”: SQLite exempts NULL from the foreign key, and ” would
// have to match a real project id.
func projectArg(scope Scope, projectID string) (any, error) {
	id := strings.TrimSpace(projectID)
	switch scope {
	case ScopeWriterProfile:
		if id != "" {
			return nil, fmt.Errorf("agentmemory: the writer profile is global; it takes no work id (got %q)", id)
		}
		return nil, nil
	case ScopeWorkNotes:
		if id == "" {
			return nil, errors.New("agentmemory: work notes need the work they belong to")
		}
		return id, nil
	}
	return nil, fmt.Errorf("agentmemory: unknown scope %q", scope)
}

// Load returns the document. A row that is not there is an empty document, not
// an error: a writer who has never recorded anything is the normal case.
func (r *Repo) Load(ctx context.Context, scope Scope, projectID string) (Document, error) {
	arg, err := projectArg(scope, projectID)
	if err != nil {
		return Document{}, err
	}
	doc := Document{Scope: scope, ProjectID: strings.TrimSpace(projectID), CharsBudget: scope.Budget()}
	row := r.db.QueryRowContext(ctx,
		`SELECT body, updated_at FROM agent_memory
		  WHERE scope = ? AND project_id IS ?`, string(scope), arg)
	if err := row.Scan(&doc.Body, &doc.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return doc, nil
		}
		return Document{}, err
	}
	doc.CharsUsed = utf8.RuneCountInString(doc.Body)
	return doc, nil
}

// bodyRuneLenBefore returns the rune length of the document currently stored
// for scope/arg, or 0 if there is none. It backs Save's shrink-only escape
// hatch below: agentmemory.Apply's own budgeted() helper accepts a result
// that is over budget as long as it is shorter than what it replaces, so an
// agent can dig out of a document that is already too big. Save has to agree
// with that exactly, because Apply never writes — the tool that calls it
// always saves what comes back through Save, and a body Apply accepted must
// not be refused a moment later here.
func (r *Repo) bodyRuneLenBefore(ctx context.Context, scope Scope, arg any) (int, error) {
	var body string
	row := r.db.QueryRowContext(ctx,
		`SELECT body FROM agent_memory WHERE scope = ? AND project_id IS ?`, string(scope), arg)
	if err := row.Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	return utf8.RuneCountInString(body), nil
}

// Save screens and budget-checks, then replaces the document. Both checks run
// before the write, so a refused save leaves the previous memory intact.
//
// The budget check has the same shrink-only escape hatch as
// agentmemory.Apply's budgeted(): a body over the scope's budget is still
// allowed through if it is shorter (in runes) than the document it replaces.
// Without that, an agent could never claw its way out of a document that
// somehow got over budget already (a shrunk budget, or a hand-edited row) —
// every remove or replace it tries would itself be refused as over budget.
// A save that GROWS an already-over-budget document, or that is not
// shrinking at all, is still refused.
//
// Delete-then-insert in one transaction rather than an upsert: the global row
// and a work's row conflict on two DIFFERENT partial unique indexes, so a
// single ON CONFLICT target cannot cover both. store.Store caps the pool at
// one connection, so this cannot interleave with another writer.
func (r *Repo) Save(ctx context.Context, scope Scope, projectID, body string, now int64) (Document, error) {
	arg, err := projectArg(scope, projectID)
	if err != nil {
		return Document{}, err
	}
	if err := Screen(body); err != nil {
		return Document{}, err
	}
	used := utf8.RuneCountInString(body)
	if used > scope.Budget() {
		before, err := r.bodyRuneLenBefore(ctx, scope, arg)
		if err != nil {
			return Document{}, err
		}
		if used >= before {
			return Document{}, fmt.Errorf("%w: %d characters, and %s holds %d", ErrOverBudget, used, scope, scope.Budget())
		}
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Document{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM agent_memory WHERE scope = ? AND project_id IS ?`, string(scope), arg); err != nil {
		return Document{}, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO agent_memory (scope, project_id, body, updated_at) VALUES (?, ?, ?, ?)`,
		string(scope), arg, body, now); err != nil {
		return Document{}, err
	}
	if err := tx.Commit(); err != nil {
		return Document{}, err
	}
	return Document{
		Scope: scope, ProjectID: strings.TrimSpace(projectID), Body: body,
		CharsUsed: used, CharsBudget: scope.Budget(), UpdatedAt: now,
	}, nil
}
