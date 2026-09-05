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

// ErrUnknownProject is returned when work notes name a work that is not in
// this library. projectArg cannot catch it — only the database knows which
// ids exist — so it surfaces as SQLite refusing the INSERT on
// agent_memory.project_id's foreign key, and Save and Edit translate that
// here. Without the translation the caller gets the driver's own
// "constraint failed: FOREIGN KEY constraint failed", which tells a writer
// (and an agent reading a tool error) nothing they can act on, unlike every
// other refusal on this path.
var ErrUnknownProject = errors.New("agentmemory: no such work")

// sqliteConstraintForeignKey is SQLITE_CONSTRAINT_FOREIGNKEY, SQLite's
// extended result code for a write that would leave a foreign key dangling.
// It is part of SQLite's published C API, so it is stable in a way the
// driver's message text is not.
const sqliteConstraintForeignKey = 787

// isForeignKeyViolation reports whether err is that refusal.
//
// It matches on the numeric result code, never on the driver's wording: the
// number is SQLite's own contract, the string is modernc.org/sqlite's
// phrasing and could change in any release. The `Code() int` interface is
// declared here rather than importing modernc.org/sqlite and asserting on
// *sqlite.Error, so agentmemory stays free of a dependency on one particular
// driver — any driver whose error carries the sqlite result code satisfies
// it. TestSaveOnAnUnknownWorkSaysSo drives this against the real driver, so
// a driver that stopped reporting the code would turn that test red.
func isForeignKeyViolation(err error) bool {
	var coded interface{ Code() int }
	if !errors.As(err, &coded) {
		return false
	}
	return coded.Code() == sqliteConstraintForeignKey
}

// asWriteError translates a failed write. Only the foreign-key refusal is
// reinterpreted; everything else (a disk error, a closed database) is a
// genuine fault and is passed through untouched.
func asWriteError(err error, projectID string) error {
	if isForeignKeyViolation(err) {
		return fmt.Errorf("%w: %q is not a work in this library; open the work first",
			ErrUnknownProject, strings.TrimSpace(projectID))
	}
	return err
}

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

// rowQuerier is satisfied by both *sql.DB and *sql.Tx. bodyBefore takes one
// of these — never r.db directly — so Save and Edit can run it against the
// very transaction that goes on to perform the write; see Save's doc comment
// for why that has to be the same transaction rather than a plain query.
type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// bodyBefore returns the body currently stored for scope/arg, or "" if there
// is no row. A missing row is the normal case for a writer who has never
// recorded anything, so it is not an error here either.
func bodyBefore(ctx context.Context, q rowQuerier, scope Scope, arg any) (string, error) {
	var body string
	row := q.QueryRowContext(ctx,
		`SELECT body FROM agent_memory WHERE scope = ? AND project_id IS ?`, string(scope), arg)
	if err := row.Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return body, nil
}

// bodyRuneLenBefore returns the rune length of the document currently stored
// for scope/arg, or 0 if there is none. It backs Save's shrink-only escape
// hatch below: agentmemory.Apply's own budgeted() helper accepts a result
// that is over budget as long as it is shorter than what it replaces, so an
// agent can dig out of a document that is already too big. Save has to agree
// with that exactly. Apply never writes, and its only production caller is
// Edit, which writes through replaceBody rather than through Save — but the
// two rules still have to be exact complements, because Save is the path the
// Settings textarea takes and a body one of them accepted must not be
// refused a moment later by the other.
func bodyRuneLenBefore(ctx context.Context, q rowQuerier, scope Scope, arg any) (int, error) {
	body, err := bodyBefore(ctx, q, scope, arg)
	if err != nil {
		return 0, err
	}
	return utf8.RuneCountInString(body), nil
}

// replaceBody performs the write itself. Save and Edit share it so the two
// paths cannot drift into writing the row differently.
func replaceBody(ctx context.Context, tx *sql.Tx, scope Scope, arg any, body string, now int64) error {
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM agent_memory WHERE scope = ? AND project_id IS ?`, string(scope), arg); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO agent_memory (scope, project_id, body, updated_at) VALUES (?, ?, ?, ?)`,
		string(scope), arg, body, now)
	return err
}

// saveRaceHookKey lets a test attach a callback that runs immediately after
// Save reads the rune length of the currently stored document — inside the
// same transaction that goes on to perform the write — and before that value
// is used to decide anything. It exists solely so a test can force a
// deterministic interleaving of two concurrent Saves without relying on
// sleep-based timing. Production code never sets this key: ctx.Value always
// returns nil outside a test, so runSaveRaceHook is a no-op there.
type saveRaceHookKey struct{}

func runSaveRaceHook(ctx context.Context) {
	if hook, ok := ctx.Value(saveRaceHookKey{}).(func()); ok && hook != nil {
		hook()
	}
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
// Screening the body and comparing its length against the scope's fixed
// budget don't depend on what's currently stored, so both run before any
// transaction opens. But the escape hatch's "is this shorter than what's
// there?" question does depend on the stored row, and reading that row with
// a plain query before opening the transaction is not safe: the store's pool
// is capped to one connection (store.Store, see its own comment), which
// guarantees no two statements run AT THE SAME INSTANT, but it does not make
// "read the current row, decide, then delete-then-insert" one atomic unit.
// The plain read releases the connection the moment it returns; another
// Save's entire delete-then-insert can land in the gap between that read and
// this call's own BeginTx, so the decision here would be made against a row
// that Save then goes on to silently clobber. Reading bodyRuneLenBefore
// through the same tx that performs the write closes that gap: once BeginTx
// succeeds, this call holds the pool's one connection until Commit or
// Rollback, so no other Save's BeginTx can proceed until this one is done.
func (r *Repo) Save(ctx context.Context, scope Scope, projectID, body string, now int64) (Document, error) {
	arg, err := projectArg(scope, projectID)
	if err != nil {
		return Document{}, err
	}
	if err := Screen(body); err != nil {
		return Document{}, err
	}
	used := utf8.RuneCountInString(body)
	overBudget := used > scope.Budget()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Document{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if overBudget {
		before, err := bodyRuneLenBefore(ctx, tx, scope, arg)
		if err != nil {
			return Document{}, err
		}
		runSaveRaceHook(ctx)
		if used >= before {
			return Document{}, fmt.Errorf("%w: %d characters, and %s holds %d", ErrOverBudget, used, scope, scope.Budget())
		}
	}
	if err := replaceBody(ctx, tx, scope, arg, body, now); err != nil {
		return Document{}, asWriteError(err, projectID)
	}
	if err := tx.Commit(); err != nil {
		return Document{}, err
	}
	return Document{
		Scope: scope, ProjectID: strings.TrimSpace(projectID), Body: body,
		CharsUsed: used, CharsBudget: scope.Budget(), UpdatedAt: now,
	}, nil
}

// editRaceHookKey is the Edit counterpart of saveRaceHookKey: a test attaches
// a callback that runs immediately after Edit has read the current body —
// inside the very transaction that goes on to perform the write — and before
// Apply computes the new one. It exists solely so a test can force a
// deterministic interleaving of two concurrent Edits without sleep-based
// timing. Production code never sets this key, so runEditRaceHook is a no-op
// there.
type editRaceHookKey struct{}

func runEditRaceHook(ctx context.Context) {
	if hook, ok := ctx.Value(editRaceHookKey{}).(func()); ok && hook != nil {
		hook()
	}
}

// Edit is read-modify-write as ONE unit: it reads the current body, applies
// one edit to it, and writes the result, all inside a single transaction.
//
// Save is the right call when the caller means to replace a whole document
// it already has in hand — the Settings textarea, where overwriting is what
// the writer asked for. Edit is the right call when the new body is a
// function of the stored one, which is every agent edit. Doing that as
// Load-then-Apply-then-Save loses data: two callers both load the same body,
// both append their line, and the second Save discards the first caller's
// line while telling it that it succeeded. This memory has two writers by
// design — the built-in agent and a connected external client — so that is a
// real interleaving, not a theoretical one. Save's own transaction does not
// help, because it only protects Save's shrink-only decision about a body the
// caller had already computed from a stale read.
//
// The store's pool is capped to one connection (see store.Store), so once
// BeginTx succeeds this call holds it until Commit or Rollback and no other
// Save or Edit can read the row in between.
//
// The budget rule is Apply's, evaluated against the body read inside this
// transaction, and that is exactly Save's rule rather than merely a similar
// one: Apply's budgeted() accepts when used <= budget || used < before, whose
// complement is Save's refusal when used > budget && used >= before, over the
// same `before`. Screen runs on the result too, keeping Save's invariant that
// what is stored has been screened — Apply screens the incoming line, but a
// remove leaves the rest of the body unexamined.
func (r *Repo) Edit(ctx context.Context, scope Scope, projectID, action, find, text string, now int64) (Document, error) {
	arg, err := projectArg(scope, projectID)
	if err != nil {
		return Document{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Document{}, err
	}
	defer func() { _ = tx.Rollback() }()

	current, err := bodyBefore(ctx, tx, scope, arg)
	if err != nil {
		return Document{}, err
	}
	runEditRaceHook(ctx)
	next, err := Apply(scope, current, action, find, text)
	if err != nil {
		return Document{}, err
	}
	if err := Screen(next); err != nil {
		return Document{}, err
	}
	if err := replaceBody(ctx, tx, scope, arg, next, now); err != nil {
		return Document{}, asWriteError(err, projectID)
	}
	if err := tx.Commit(); err != nil {
		return Document{}, err
	}
	return Document{
		Scope: scope, ProjectID: strings.TrimSpace(projectID), Body: next,
		CharsUsed: utf8.RuneCountInString(next), CharsBudget: scope.Budget(), UpdatedAt: now,
	}, nil
}
