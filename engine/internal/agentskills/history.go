package agentskills

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// History persists skill_snapshots — one row per write, so the daily VACUUM
// INTO backup (which copies this database and nothing else under
// LINETTA_HOME) carries every version of every skill, even though it never
// copies the live <home>/skills/ directory. See
// 0019_skill_snapshots.sql for the fuller rationale; this file is the Go
// side of the same decision, mirroring internal/snapshot's Repo over
// node_snapshots without reusing it — node_snapshots.node_id is NOT NULL
// with a hard FK to nodes, its previews walk Tiptap JSON, and its retention
// partitions by node. A skill has none of those.
//
// There is deliberately no Thin here, unlike internal/snapshot. Skills
// change far less often than prose and each row is small (bounded by
// MaxBodyRunes), so there is no accumulation problem yet; a thinning pass
// for this table is #99, not an oversight.
type History struct{ db *sql.DB }

// NewHistory builds a History over db. It takes the raw *sql.DB, not a
// *store.Store, because it has no need of anything else the Store carries —
// unlike internal/snapshot.Repo, which predates this convention.
func NewHistory(db *sql.DB) *History { return &History{db: db} }

// Reasons a version was recorded. They mirror the store's write path: a
// version is recorded every time a skill is created, edited, or deleted.
const (
	ReasonCreated = "created"
	ReasonEdited  = "edited"
	ReasonDeleted = "deleted"
)

// validReason reports whether reason is one of the three above. Record
// refuses anything else rather than accepting an arbitrary string into a
// column callers (a version-history UI) will render and rely on.
func validReason(reason string) bool {
	switch reason {
	case ReasonCreated, ReasonEdited, ReasonDeleted:
		return true
	default:
		return false
	}
}

// ErrInvalidReason is Record's refusal of a reason outside the three
// constants above.
var ErrInvalidReason = errors.New("agentskills: invalid version reason")

// defaultHistoryLimit is how many versions List returns when the caller
// passes limit <= 0 ("I didn't say"), matching the house naming convention
// for this kind of bound (mcphost.DefaultActivityLimit, companion.recallLimit).
const defaultHistoryLimit = 20

// maxHistoryLimit is the ceiling List clamps any caller-supplied limit to.
// skill_snapshots carries no retention/thinning pass (that's #99, per the
// migration's comment), so nothing else keeps the table small; the RPC
// handler Task 8 adds will pass a client-supplied limit straight through,
// and this is what stops that from ever meaning "give me the whole table."
const maxHistoryLimit = 200

// ErrVersionNotFound is Get's refusal of an id that names no row. It is
// distinct from the package's ErrNotFound (store.go), which is a skill
// missing from the filesystem — a different kind of "not found" that would
// be confusing to conflate with a missing history id.
var ErrVersionNotFound = errors.New("agentskills: version not found")

// Version is one row of a skill's history: the skill as it looked right
// after the write that produced this row, tagged with why it was written.
type Version struct {
	ID        string
	Skill     Skill
	Reason    string
	CreatedAt int64
}

// projectArg maps a scope's project id onto skill_snapshots' nullable
// column, the same rule agentmemory.projectArg and Store.Dir both follow:
// the writer scope is global and stores NULL, never "" — SQLite exempts
// NULL from the foreign key, and "" would have to match a real project id.
func projectArg(scope Scope, projectID string) (any, error) {
	id := strings.TrimSpace(projectID)
	switch scope {
	case ScopeWriter:
		if id != "" {
			return nil, fmt.Errorf("agentskills: writer skills are global; they take no work id (got %q)", id)
		}
		return nil, nil
	case ScopeWork:
		if id == "" {
			return nil, errors.New("agentskills: work skills need the work they belong to")
		}
		return id, nil
	default:
		return nil, fmt.Errorf("agentskills: unknown scope %q; use %q or %q", scope, ScopeWriter, ScopeWork)
	}
}

// Record lands one version row for s, tagged with reason and now.
//
// It always records the state s is IN at the moment of the call — the state
// a write just produced, never the state that preceded it. Restoring
// version N therefore gives back exactly what the skill looked like at
// version N; there is no separate "before" row to reach for.
//
// For reason == ReasonDeleted, the caller is expected to pass s as the
// skill's last known state (the body it held right before removal), not an
// empty Skill. That is a decision about what the CALLER passes, not
// something Record can enforce — but it is the point of recording a
// deletion at all: a version row marked "deleted" with the last body intact
// is a complete, restorable snapshot on its own. A writer browsing history
// can restore straight from the row marked deleted; they do not need to
// know to reach for the row one before it. An empty body on that row would
// make the deletion look unrecoverable when it is not.
func (h *History) Record(ctx context.Context, s Skill, reason string, now int64) error {
	if !validReason(reason) {
		return fmt.Errorf("%w: %q", ErrInvalidReason, reason)
	}
	arg, err := projectArg(s.Scope, s.ProjectID)
	if err != nil {
		return err
	}
	author := s.Author
	if author == "" {
		author = AuthorWriter
	}
	id := uuid.NewString()
	_, err = h.db.ExecContext(ctx, `
INSERT INTO skill_snapshots (id, scope, project_id, name, body, descript, author, reason, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, string(s.Scope), arg, s.Name, s.Body, s.Description, string(author), reason, now)
	if err != nil {
		return fmt.Errorf("agentskills: record version: %w", err)
	}
	return nil
}

// List returns a skill's versions, newest first.
//
// It filters on scope, project id AND name together — not name alone — so
// that two skills sharing a name in different scopes, or in different
// works, are correctly treated as separate histories.
//
// limit bounds how many rows come back. limit <= 0 (the caller said
// nothing) falls back to defaultHistoryLimit; anything above
// maxHistoryLimit is clamped to it. Task 8's RPC handler passes a
// client-supplied value straight through, and skill_snapshots has no
// retention job (that's #99) to bound how large the table can grow, so
// List itself has to refuse to hand back the whole table just because a
// caller passed 0 or something enormous.
func (h *History) List(ctx context.Context, scope Scope, projectID, name string, limit int) ([]Version, error) {
	arg, err := projectArg(scope, projectID)
	if err != nil {
		return nil, err
	}
	switch {
	case limit <= 0:
		limit = defaultHistoryLimit
	case limit > maxHistoryLimit:
		limit = maxHistoryLimit
	}
	rows, err := h.db.QueryContext(ctx, `
SELECT rowid, id, scope, project_id, name, body, descript, author, reason, created_at
  FROM skill_snapshots
 WHERE scope = ? AND project_id IS ? AND name = ?
 ORDER BY created_at DESC, rowid DESC
 LIMIT ?`, string(scope), arg, name, limit)
	if err != nil {
		return nil, fmt.Errorf("agentskills: list versions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]Version, 0)
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return nil, fmt.Errorf("agentskills: list versions: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("agentskills: list versions: %w", err)
	}
	return out, nil
}

// Get returns one version by id. A missing id is ErrVersionNotFound, not a
// zero Version silently returned, because a caller resolving a restore
// target has to be told the id no longer names anything.
func (h *History) Get(ctx context.Context, id string) (Version, error) {
	row := h.db.QueryRowContext(ctx, `
SELECT rowid, id, scope, project_id, name, body, descript, author, reason, created_at
  FROM skill_snapshots
 WHERE id = ?`, id)
	v, err := scanVersion(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Version{}, fmt.Errorf("%w: %q", ErrVersionNotFound, id)
		}
		return Version{}, fmt.Errorf("agentskills: get version: %w", err)
	}
	return v, nil
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows, so scanVersion
// serves List and Get alike.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanVersion(row rowScanner) (Version, error) {
	var (
		id, scope, name, body, descript, author, reason string
		projectID                                       sql.NullString
		createdAt, rowSeq                               int64
	)
	if err := row.Scan(&rowSeq, &id, &scope, &projectID, &name, &body, &descript, &author, &reason, &createdAt); err != nil {
		return Version{}, err
	}
	_ = rowSeq // selected only so List's ORDER BY tie-break names a real column
	v := Version{
		ID:        id,
		Reason:    reason,
		CreatedAt: createdAt,
		Skill: Skill{
			Name:        name,
			Scope:       Scope(scope),
			Description: descript,
			Author:      Author(author),
			Body:        body,
			UpdatedAt:   createdAt,
			BodyRunes:   len([]rune(body)),
		},
	}
	if projectID.Valid {
		v.Skill.ProjectID = projectID.String
	}
	return v, nil
}
