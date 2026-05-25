package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// AIRunStatus mirrors ai_runs.status values.
type AIRunStatus string

const (
	AIRunStreaming AIRunStatus = "streaming"
	AIRunDone      AIRunStatus = "done"
	AIRunError     AIRunStatus = "error"
	AIRunCancelled AIRunStatus = "cancelled"
)

// AIRun mirrors one row of ai_runs.
type AIRun struct {
	ID          string          `json:"id"`
	ProjectID   string          `json:"project_id"`
	NodeID      *string         `json:"node_id,omitempty"`
	Provider    string          `json:"provider"`
	Prompt      string          `json:"prompt"`
	ContextJSON json.RawMessage `json:"context_json"`
	Output      string          `json:"output"`
	Status      AIRunStatus     `json:"status"`
	ErrorMsg    *string         `json:"error,omitempty"`
	StartedAt   int64           `json:"started_at"`
	EndedAt     *int64          `json:"ended_at,omitempty"`
}

// AIRunsRepo persists ai_runs rows.
type AIRunsRepo struct{ s *Store }

// NewAIRunsRepo returns a repo backed by the given Store.
func NewAIRunsRepo(s *Store) *AIRunsRepo { return &AIRunsRepo{s: s} }

// Insert creates the row with status=streaming. The caller (runner) calls
// UpdateStatus once the run terminates.
func (r *AIRunsRepo) Insert(ctx context.Context, run AIRun) error {
	if run.ID == "" || run.ProjectID == "" {
		return errors.New("ai_run: id and project_id required")
	}
	if run.ContextJSON == nil {
		run.ContextJSON = []byte("{}")
	}
	var nodeID any
	if run.NodeID != nil {
		nodeID = *run.NodeID
	}
	_, err := r.s.DB().ExecContext(ctx, `
INSERT INTO ai_runs (id, project_id, node_id, provider, prompt, context_json,
                     output, status, started_at)
VALUES (?, ?, ?, ?, ?, ?, '', ?, ?)`,
		run.ID, run.ProjectID, nodeID, run.Provider, run.Prompt,
		string(run.ContextJSON), string(run.Status), run.StartedAt)
	return err
}

// UpdateStatus finalizes a run. Pass status / output / error / endedAt.
func (r *AIRunsRepo) UpdateStatus(ctx context.Context, id string, status AIRunStatus, output string, errMsg string, endedAt int64) error {
	var errArg any
	if errMsg != "" {
		errArg = errMsg
	}
	_, err := r.s.DB().ExecContext(ctx, `
UPDATE ai_runs SET status = ?, output = ?, error = ?, ended_at = ?
 WHERE id = ?`, string(status), output, errArg, endedAt, id)
	return err
}

// ListRecent returns at most limit recent runs for a project.
func (r *AIRunsRepo) ListRecent(ctx context.Context, projectID string, limit int) ([]AIRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 30
	}
	rows, err := r.s.DB().QueryContext(ctx, `
SELECT id, project_id, node_id, provider, prompt, context_json, output, status,
       error, started_at, ended_at
  FROM ai_runs
 WHERE project_id = ?
 ORDER BY started_at DESC
 LIMIT ?`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AIRun
	for rows.Next() {
		var (
			run         AIRun
			nodeID      sql.NullString
			contextJSON string
			errMsg      sql.NullString
			endedAt     sql.NullInt64
		)
		if err := rows.Scan(&run.ID, &run.ProjectID, &nodeID, &run.Provider,
			&run.Prompt, &contextJSON, &run.Output, &run.Status,
			&errMsg, &run.StartedAt, &endedAt); err != nil {
			return nil, err
		}
		if nodeID.Valid {
			v := nodeID.String
			run.NodeID = &v
		}
		run.ContextJSON = json.RawMessage(contextJSON)
		if errMsg.Valid {
			v := errMsg.String
			run.ErrorMsg = &v
		}
		if endedAt.Valid {
			v := endedAt.Int64
			run.EndedAt = &v
		}
		out = append(out, run)
	}
	return out, rows.Err()
}
