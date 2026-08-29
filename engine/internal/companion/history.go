package companion

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	HistoryScopeScene   = "scene"
	HistoryScopeProject = "project"
	HistoryScopeGlobal  = "global"

	HistoryViewScene   = "scene"
	HistoryViewProject = "project"

	HistoryStatusStreaming = "streaming"
	HistoryStatusDone      = "done"
	HistoryStatusApplied   = "applied"
	HistoryStatusFailed    = "failed"
	HistoryStatusCancelled = "cancelled"
	HistoryStatusCompacted = "compacted"
)

// HistoryMessage is the Linetta-owned companion transcript row used for UI
// restoration and scene/project filtering. It intentionally carries metadata
// that TARS session.Message cannot store.
type HistoryMessage struct {
	ID        string
	ProjectID string
	NodeID    string
	NodeLabel string
	RunID     string
	Role      string
	Scope     string
	Intent    string
	Status    string
	Content   string
	CreatedAt int64
}

type HistoryQuery struct {
	ProjectID string
	NodeID    string
	Scope     string
	Limit     int
}

type HistoryRepo struct {
	db *sql.DB
}

func NewHistoryRepo(db *sql.DB) *HistoryRepo {
	return &HistoryRepo{db: db}
}

func (r *HistoryRepo) Append(ctx context.Context, msg HistoryMessage) error {
	if r == nil {
		return nil
	}
	msg = normalizeHistoryMessage(msg)
	_, err := r.db.ExecContext(ctx, `
INSERT INTO companion_messages
  (id, project_id, node_id, run_id, role, scope, intent, status, content, created_at)
VALUES (?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?, ?, ?)`,
		msg.ID, msg.ProjectID, msg.NodeID, msg.RunID, msg.Role, msg.Scope, msg.Intent, msg.Status, msg.Content, msg.CreatedAt)
	if err != nil {
		return fmt.Errorf("append companion history: %w", err)
	}
	return nil
}

func (r *HistoryRepo) List(ctx context.Context, q HistoryQuery) ([]HistoryMessage, error) {
	if r == nil {
		return nil, nil
	}
	q.Scope = normalizeHistoryView(q.Scope)
	if q.Limit <= 0 {
		q.Limit = 100
	}
	if q.Scope == HistoryViewScene && strings.TrimSpace(q.NodeID) != "" {
		return r.list(ctx, `
SELECT m.id, m.project_id, COALESCE(m.node_id, ''), COALESCE(NULLIF(n.title, ''), n.label, ''),
       COALESCE(m.run_id, ''), m.role, m.scope, m.intent, m.status, m.content, m.created_at
  FROM (
    SELECT * FROM companion_messages
     WHERE project_id = ? AND node_id = ? AND scope = ?
     ORDER BY created_at DESC, CASE role WHEN 'assistant' THEN 0 ELSE 1 END, id DESC
     LIMIT ?
  ) m
  LEFT JOIN nodes n ON n.id = m.node_id
 ORDER BY m.created_at ASC, CASE m.role WHEN 'user' THEN 0 ELSE 1 END, m.id ASC`, q.ProjectID, q.NodeID, HistoryScopeScene, q.Limit)
	}
	return r.list(ctx, `
SELECT m.id, m.project_id, COALESCE(m.node_id, ''), COALESCE(NULLIF(n.title, ''), n.label, ''),
       COALESCE(m.run_id, ''), m.role, m.scope, m.intent, m.status, m.content, m.created_at
  FROM (
    SELECT * FROM companion_messages
     WHERE project_id = ?
     ORDER BY created_at DESC, CASE role WHEN 'assistant' THEN 0 ELSE 1 END, id DESC
     LIMIT ?
  ) m
  LEFT JOIN nodes n ON n.id = m.node_id
 ORDER BY m.created_at ASC, CASE m.role WHEN 'user' THEN 0 ELSE 1 END, m.id ASC`, q.ProjectID, q.Limit)
}

func (r *HistoryRepo) LoadForPrompt(ctx context.Context, q HistoryQuery) ([]HistoryMessage, error) {
	if r == nil {
		return nil, nil
	}
	if q.Limit <= 0 {
		q.Limit = 24
	}
	if normalizeHistoryView(q.Scope) == HistoryViewScene && strings.TrimSpace(q.NodeID) != "" {
		return r.List(ctx, HistoryQuery{
			ProjectID: q.ProjectID,
			NodeID:    q.NodeID,
			Scope:     HistoryViewScene,
			Limit:     q.Limit,
		})
	}
	return r.List(ctx, HistoryQuery{
		ProjectID: q.ProjectID,
		Scope:     HistoryViewProject,
		Limit:     q.Limit,
	})
}

func (r *HistoryRepo) list(ctx context.Context, query string, args ...any) ([]HistoryMessage, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HistoryMessage
	for rows.Next() {
		var msg HistoryMessage
		if err := rows.Scan(
			&msg.ID, &msg.ProjectID, &msg.NodeID, &msg.NodeLabel,
			&msg.RunID, &msg.Role, &msg.Scope, &msg.Intent, &msg.Status,
			&msg.Content, &msg.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	return out, rows.Err()
}

func (r *HistoryRepo) Clear(ctx context.Context, q HistoryQuery) error {
	if r == nil {
		return nil
	}
	if normalizeHistoryView(q.Scope) == HistoryViewScene && strings.TrimSpace(q.NodeID) != "" {
		_, err := r.db.ExecContext(ctx, `
DELETE FROM companion_messages
 WHERE project_id = ? AND node_id = ? AND scope = ?`, q.ProjectID, q.NodeID, HistoryScopeScene)
		return err
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM companion_messages WHERE project_id = ?`, q.ProjectID)
	return err
}

func (r *HistoryRepo) MarkRunStatus(ctx context.Context, runID, status string) error {
	if r == nil || strings.TrimSpace(runID) == "" {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE companion_messages
   SET status = ?
 WHERE run_id = ?`, normalizeHistoryStatus(status), runID)
	return err
}

func (r *HistoryRepo) ProjectMessageCount(ctx context.Context, projectID string) (int, error) {
	if r == nil {
		return 0, nil
	}
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM companion_messages WHERE project_id = ?`, projectID).Scan(&count)
	return count, err
}

func normalizeHistoryMessage(msg HistoryMessage) HistoryMessage {
	msg.ID = strings.TrimSpace(msg.ID)
	if msg.ID == "" {
		msg.ID = uuid.NewString()
	}
	msg.ProjectID = strings.TrimSpace(msg.ProjectID)
	msg.NodeID = strings.TrimSpace(msg.NodeID)
	msg.RunID = strings.TrimSpace(msg.RunID)
	msg.Role = strings.TrimSpace(msg.Role)
	msg.Scope = normalizeHistoryScope(msg.Scope, msg.NodeID)
	msg.Intent = strings.TrimSpace(msg.Intent)
	msg.Status = normalizeHistoryStatus(msg.Status)
	if msg.CreatedAt <= 0 {
		msg.CreatedAt = time.Now().UnixMilli()
	}
	return msg
}

func normalizeHistoryScope(scope, nodeID string) string {
	switch strings.TrimSpace(scope) {
	case HistoryScopeScene:
		if strings.TrimSpace(nodeID) != "" {
			return HistoryScopeScene
		}
		return HistoryScopeProject
	case HistoryScopeGlobal:
		return HistoryScopeGlobal
	default:
		return HistoryScopeProject
	}
}

func normalizeHistoryView(scope string) string {
	if strings.TrimSpace(scope) == HistoryViewScene {
		return HistoryViewScene
	}
	return HistoryViewProject
}

func normalizeHistoryStatus(status string) string {
	switch strings.TrimSpace(status) {
	case HistoryStatusStreaming, HistoryStatusApplied, HistoryStatusFailed, HistoryStatusCancelled, HistoryStatusCompacted:
		return strings.TrimSpace(status)
	default:
		return HistoryStatusDone
	}
}

func historyErr(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return err
}
