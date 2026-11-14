// Package agentusage keeps what the built-in agent's requests cost, one row per
// request, in agent_usage. The agent records; Settings reads the sums.
package agentusage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// Retention is how long a row is kept. Settings shows the last 30 days; the
// rest is there to compare against.
const Retention int64 = 90 * 24 * 60 * 60 * 1000

// Entry is one request. The token fields are the provider's own report (tars
// llm.Usage): Cached is a part of Input, CacheRead and CacheWrite are on top of
// it.
type Entry struct {
	RunID     string
	ProjectID string
	// Kind is what sent the request: "turn" for the writer's own turn, or the
	// background pass after it ("self_review", "settlement", …).
	Kind       string
	Provider   string
	Model      string
	Input      int
	Output     int
	Cached     int
	CacheRead  int
	CacheWrite int
	At         int64
}

// Row is the sum of one kind of request on one model.
type Row struct {
	Kind       string `json:"kind"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	Calls      int64  `json:"calls"`
	Input      int64  `json:"input"`
	Output     int64  `json:"output"`
	Cached     int64  `json:"cached"`
	CacheRead  int64  `json:"cache_read"`
	CacheWrite int64  `json:"cache_write"`
}

// Summary is every row since a moment, most requested first.
type Summary struct {
	Since int64 `json:"since"`
	Rows  []Row `json:"rows"`
}

// Repo reads and writes agent_usage.
type Repo struct{ db *sql.DB }

// NewRepo returns a repo on db.
func NewRepo(db *sql.DB) *Repo { return &Repo{db: db} }

// Add records one request. A request that reported no tokens at all — a
// provider that does not count them — leaves no row, since it would only say
// that a call happened.
func (r *Repo) Add(ctx context.Context, e Entry) error {
	if r == nil || r.db == nil {
		return nil
	}
	kind := strings.TrimSpace(e.Kind)
	if kind == "" {
		return errors.New("agentusage: kind required")
	}
	if e.Input == 0 && e.Output == 0 && e.Cached == 0 && e.CacheRead == 0 && e.CacheWrite == 0 {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO agent_usage (run_id, project_id, kind, provider, model,
                         input_tokens, output_tokens, cached_tokens,
                         cache_read_tokens, cache_write_tokens, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.RunID, e.ProjectID, kind, e.Provider, e.Model,
		e.Input, e.Output, e.Cached, e.CacheRead, e.CacheWrite, e.At)
	return err
}

// Summary sums the requests made at or after since, by kind and model.
func (r *Repo) Summary(ctx context.Context, since int64) (Summary, error) {
	out := Summary{Since: since, Rows: []Row{}}
	rows, err := r.db.QueryContext(ctx, `
SELECT kind, provider, model, COUNT(*),
       SUM(input_tokens), SUM(output_tokens), SUM(cached_tokens),
       SUM(cache_read_tokens), SUM(cache_write_tokens)
  FROM agent_usage
 WHERE created_at >= ?
 GROUP BY kind, provider, model
 ORDER BY COUNT(*) DESC, kind, provider, model`, since)
	if err != nil {
		return Summary{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var row Row
		if err := rows.Scan(&row.Kind, &row.Provider, &row.Model, &row.Calls,
			&row.Input, &row.Output, &row.Cached, &row.CacheRead, &row.CacheWrite); err != nil {
			return Summary{}, err
		}
		out.Rows = append(out.Rows, row)
	}
	return out, rows.Err()
}

// Prune drops the rows older than before.
func (r *Repo) Prune(ctx context.Context, before int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM agent_usage WHERE created_at < ?`, before)
	return err
}
