// Package opsstatus persists the latest result of background and support jobs
// so the UI can show recovery-oriented status instead of relying on stderr.
package opsstatus

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/devlikebear/linetta/engine/internal/store"
)

const (
	JobBackup               = "backup.daily"
	JobGitSync              = "git_sync"
	JobFolderSync           = "folder_sync"
	JobSummarizer           = "summarizer"
	JobCompanionPersistence = "companion.persistence"
)

// Status mirrors one ops_status row.
type Status struct {
	JobName        string `json:"job_name"`
	LastStartedAt  *int64 `json:"last_started_at,omitempty"`
	LastFinishedAt *int64 `json:"last_finished_at,omitempty"`
	LastOK         bool   `json:"last_ok"`
	LastError      string `json:"last_error"`
	MetadataJSON   string `json:"metadata_json"`
}

// Repo stores the latest visible status for named operational jobs.
type Repo struct {
	s *store.Store
}

func NewRepo(s *store.Store) *Repo {
	return &Repo{s: s}
}

// Record upserts the latest status for a job. metadata is stored as compact
// JSON; nil metadata becomes an empty object so frontend parsing is stable.
func (r *Repo) Record(
	ctx context.Context,
	jobName string,
	startedAt int64,
	finishedAt int64,
	ok bool,
	errMsg string,
	metadata any,
) error {
	if r == nil || r.s == nil {
		return nil
	}
	if jobName == "" {
		return fmt.Errorf("job_name required")
	}
	metaJSON, err := marshalMetadata(metadata)
	if err != nil {
		return err
	}
	okInt := 0
	if ok {
		okInt = 1
	}
	_, err = r.s.DB().ExecContext(ctx, `
INSERT INTO ops_status (job_name, last_started_at, last_finished_at, last_ok, last_error, metadata_json)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(job_name) DO UPDATE SET
  last_started_at = excluded.last_started_at,
  last_finished_at = excluded.last_finished_at,
  last_ok = excluded.last_ok,
  last_error = excluded.last_error,
  metadata_json = excluded.metadata_json`,
		jobName, nullableTime(startedAt), nullableTime(finishedAt), okInt, errMsg, metaJSON)
	return err
}

func (r *Repo) Get(ctx context.Context) ([]Status, error) {
	if r == nil || r.s == nil {
		return nil, nil
	}
	rows, err := r.s.DB().QueryContext(ctx, `
SELECT job_name, last_started_at, last_finished_at, last_ok, last_error, metadata_json
FROM ops_status
ORDER BY job_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Status
	for rows.Next() {
		var (
			status      Status
			started     sql.NullInt64
			finished    sql.NullInt64
			okInt       int
			metadataSQL sql.NullString
		)
		if err := rows.Scan(
			&status.JobName,
			&started,
			&finished,
			&okInt,
			&status.LastError,
			&metadataSQL,
		); err != nil {
			return nil, err
		}
		if started.Valid {
			v := started.Int64
			status.LastStartedAt = &v
		}
		if finished.Valid {
			v := finished.Int64
			status.LastFinishedAt = &v
		}
		status.LastOK = okInt != 0
		status.MetadataJSON = metadataSQL.String
		if status.MetadataJSON == "" {
			status.MetadataJSON = "{}"
		}
		out = append(out, status)
	}
	return out, rows.Err()
}

func (r *Repo) ClearError(ctx context.Context, jobName string) error {
	if r == nil || r.s == nil {
		return nil
	}
	if jobName == "" {
		return fmt.Errorf("job_name required")
	}
	_, err := r.s.DB().ExecContext(ctx, `
UPDATE ops_status
SET last_error = ''
WHERE job_name = ?`, jobName)
	return err
}

func marshalMetadata(metadata any) (string, error) {
	if metadata == nil {
		return "{}", nil
	}
	if s, ok := metadata.(string); ok {
		if s == "" {
			return "{}", nil
		}
		if json.Valid([]byte(s)) {
			return s, nil
		}
		return "", fmt.Errorf("metadata_json must be valid JSON")
	}
	b, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("marshal metadata: %w", err)
	}
	if string(b) == "null" {
		return "{}", nil
	}
	return string(b), nil
}

func nullableTime(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}
