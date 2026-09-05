//go:build !mobile

package mcphost

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// activityRetention caps how many rows the log keeps. The table trims itself
// after each insert rather than leaning on the nightly snapshot-thinning job:
// one fewer moving part, and the cap holds even if the app never idles.
const activityRetention = 500

// DefaultActivityLimit is how many entries mcp.activity returns when the
// caller does not ask for a specific count.
const DefaultActivityLimit = 100

// Who called a tool. External is every MCP client that reaches the HTTP host;
// agent is Linetta's own panel, which speaks to a second server instance over
// an in-memory transport.
const (
	SourceExternal = "external"
	SourceAgent    = "agent"
)

// ActivityEntry is one recorded tool call.
type ActivityEntry struct {
	ID        string `json:"id"`
	At        int64  `json:"at"`
	Tool      string `json:"tool"`
	ProjectID string `json:"project_id,omitempty"`
	TargetID  string `json:"target_id,omitempty"`
	OK        bool   `json:"ok"`
	Detail    string `json:"detail,omitempty"`
	Source    string `json:"source"`
	RunID     string `json:"run_id,omitempty"`
}

// ActivityRepo persists the MCP audit trail.
type ActivityRepo struct {
	db  *sql.DB
	now func() int64
}

// NewActivityRepo returns a repo over db.
func NewActivityRepo(db *sql.DB) *ActivityRepo {
	return &ActivityRepo{db: db, now: func() int64 { return time.Now().UnixMilli() }}
}

// Record appends one entry and trims the table to the retention cap. Recording
// is best-effort from the caller's perspective — a logging failure must never
// fail the tool call itself — so callers log the error and continue.
func (r *ActivityRepo) Record(ctx context.Context, e ActivityEntry) error {
	if r == nil || r.db == nil {
		return nil
	}
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	if e.At == 0 {
		e.At = r.now()
	}
	if e.Source == "" {
		e.Source = SourceExternal
	}
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO mcp_activity (id, at, tool, project_id, target_id, ok, detail, source, run_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.At, e.Tool, e.ProjectID, e.TargetID, boolToInt(e.OK), truncate(e.Detail, 500),
		e.Source, e.RunID,
	); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM mcp_activity WHERE id NOT IN (
		   SELECT id FROM mcp_activity ORDER BY at DESC, id DESC LIMIT ?
		 )`, activityRetention)
	return err
}

// List returns the most recent entries, newest first.
func (r *ActivityRepo) List(ctx context.Context, limit int) ([]ActivityEntry, error) {
	if r == nil || r.db == nil {
		return []ActivityEntry{}, nil
	}
	if limit <= 0 || limit > activityRetention {
		limit = DefaultActivityLimit
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, at, tool, project_id, target_id, ok, detail, source, run_id
		 FROM mcp_activity ORDER BY at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ActivityEntry{}
	for rows.Next() {
		var e ActivityEntry
		var ok int
		if err := rows.Scan(&e.ID, &e.At, &e.Tool, &e.ProjectID, &e.TargetID, &ok, &e.Detail,
			&e.Source, &e.RunID); err != nil {
			return nil, err
		}
		e.OK = ok != 0
		out = append(out, e)
	}
	return out, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
