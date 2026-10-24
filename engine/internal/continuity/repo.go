package continuity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/devlikebear/linetta/engine/internal/store"
)

// ErrNotFound is returned when an observation id does not exist.
var ErrNotFound = errors.New("continuity observation not found")

// ErrNodeRequired is returned when a ReportInput or ListFilter names no
// scene.
var ErrNodeRequired = errors.New("continuity: node_id required")

// Repo stores continuity checks and observations in library.db, so they
// travel with the backup (restore.MergeProject carries both tables).
type Repo struct{ s *store.Store }

func NewRepo(s *store.Store) *Repo { return &Repo{s: s} }

// Record stores one scene's continuity reading: the check row and its
// observations (an empty slice is a clean check, not an error), in a single
// transaction. Any still-OPEN observation from an earlier check of the SAME
// scene is marked replaced first — dismissed and resolved observations are
// left exactly where the writer put them, because a judgement the writer
// already made is not something a new check gets to undo.
//
// in is assumed already validated (continuity.Validate) and in.NodeID
// resolved to in.ProjectID by the caller (mcphost's requireNodeInProject) —
// Record trusts both rather than re-deriving them, the same division of
// labor direction.Repo.SetSceneIntent follows with its own caller.
func (r *Repo) Record(ctx context.Context, now int64, in ReportInput, source string) (Check, error) {
	nodeID := strings.TrimSpace(in.NodeID)
	if nodeID == "" {
		return Check{}, ErrNodeRequired
	}
	projectID := strings.TrimSpace(in.ProjectID)
	if projectID == "" {
		return Check{}, fmt.Errorf("continuity: record: project_id required")
	}
	source = strings.TrimSpace(source)
	if source == "" {
		source = SourceExternal
	}

	tx, err := r.s.DB().BeginTx(ctx, nil)
	if err != nil {
		return Check{}, err
	}
	defer tx.Rollback()

	// Replace this scene's still-open observations before the new check
	// lands, so a reader querying in between never sees two checks both
	// "current" for the same scene.
	if _, err := tx.ExecContext(ctx,
		`UPDATE continuity_observations
		    SET status = ?, status_at = ?
		  WHERE node_id = ? AND status = ?`,
		StatusReplaced, now, nodeID, StatusOpen); err != nil {
		return Check{}, err
	}

	check := Check{
		ID:             uuid.NewString(),
		ProjectID:      projectID,
		NodeID:         nodeID,
		ContentVersion: in.SceneVersion,
		Source:         source,
		Note:           strings.TrimSpace(in.Note),
		CheckedAt:      now,
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO continuity_checks (id, project_id, node_id, content_version, source, note, checked_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		check.ID, check.ProjectID, check.NodeID, check.ContentVersion, check.Source, check.Note, check.CheckedAt,
	); err != nil {
		return Check{}, err
	}

	for _, obs := range in.Observations {
		refsJSON, err := marshalRefs(obs.Refs)
		if err != nil {
			return Check{}, err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO continuity_observations
			    (id, check_id, project_id, node_id, category, kind, severity, summary, suggestion,
			     paragraph_from, paragraph_to, quote, refs_json, status, status_at, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			uuid.NewString(), check.ID, projectID, nodeID,
			obs.Category, obs.Kind, obs.Severity, strings.TrimSpace(obs.Summary), strings.TrimSpace(obs.Suggestion),
			obs.ParagraphFrom, obs.ParagraphTo, strings.TrimSpace(obs.Quote), refsJSON,
			StatusOpen, now, now,
		); err != nil {
			return Check{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return Check{}, err
	}
	return check, nil
}

// List returns checks and their observations for a work, newest check
// first, each check's Stale computed against the scene's CURRENT
// content_version — so a writer who edited the scene since it was checked
// sees that immediately, without a second RPC round trip.
func (r *Repo) List(ctx context.Context, f ListFilter) ([]CheckWithObservations, error) {
	projectID := strings.TrimSpace(f.ProjectID)
	if projectID == "" {
		return nil, fmt.Errorf("continuity: list: project_id required")
	}

	query := `
SELECT c.id, c.project_id, c.node_id, c.content_version, c.source, c.note, c.checked_at, n.content_version
  FROM continuity_checks c
  JOIN nodes n ON n.id = c.node_id
 WHERE c.project_id = ?`
	args := []any{projectID}
	if nodeID := strings.TrimSpace(f.NodeID); nodeID != "" {
		query += " AND c.node_id = ?"
		args = append(args, nodeID)
	}
	query += " ORDER BY c.checked_at DESC"

	rows, err := r.s.DB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var checks []CheckWithObservations
	order := map[string]int{}
	for rows.Next() {
		var cwo CheckWithObservations
		var currentVersion int
		if err := rows.Scan(&cwo.ID, &cwo.ProjectID, &cwo.NodeID, &cwo.ContentVersion,
			&cwo.Source, &cwo.Note, &cwo.CheckedAt, &currentVersion); err != nil {
			return nil, err
		}
		cwo.Stale = currentVersion != cwo.ContentVersion
		cwo.Observations = []Observation{}
		order[cwo.ID] = len(checks)
		checks = append(checks, cwo)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(checks) == 0 {
		return checks, nil
	}

	obsQuery := `
SELECT id, check_id, project_id, node_id, category, kind, severity, summary, suggestion,
       paragraph_from, paragraph_to, quote, refs_json, status, status_at, created_at
  FROM continuity_observations
 WHERE project_id = ?`
	obsArgs := []any{projectID}
	if nodeID := strings.TrimSpace(f.NodeID); nodeID != "" {
		obsQuery += " AND node_id = ?"
		obsArgs = append(obsArgs, nodeID)
	}
	if len(f.Statuses) > 0 {
		placeholders := make([]string, len(f.Statuses))
		for i, st := range f.Statuses {
			placeholders[i] = "?"
			obsArgs = append(obsArgs, st)
		}
		obsQuery += " AND status IN (" + strings.Join(placeholders, ",") + ")"
	}
	obsQuery += " ORDER BY created_at ASC"

	obsRows, err := r.s.DB().QueryContext(ctx, obsQuery, obsArgs...)
	if err != nil {
		return nil, err
	}
	defer obsRows.Close()
	for obsRows.Next() {
		obs, err := scanObservation(obsRows)
		if err != nil {
			return nil, err
		}
		if i, ok := order[obs.CheckID]; ok {
			checks[i].Observations = append(checks[i].Observations, obs)
		}
	}
	if err := obsRows.Err(); err != nil {
		return nil, err
	}
	return checks, nil
}

// SetStatus moves one observation between open, dismissed and resolved —
// never into or out of replaced, which only Record assigns: replaced means
// a later check superseded it, and that fact does not change because the
// writer clicked something.
func (r *Repo) SetStatus(ctx context.Context, now int64, observationID, status string) (Observation, error) {
	if status == StatusReplaced {
		return Observation{}, fmt.Errorf("continuity: set_status: cannot set status to %q directly", StatusReplaced)
	}
	if !ValidStatus(status) {
		return Observation{}, fmt.Errorf("continuity: set_status: invalid status %q", status)
	}

	cur, err := r.getObservation(ctx, observationID)
	if err != nil {
		return Observation{}, err
	}
	if cur.Status == StatusReplaced {
		return Observation{}, fmt.Errorf("continuity: set_status: observation %q was replaced by a later check; its status cannot change", observationID)
	}

	res, err := r.s.DB().ExecContext(ctx,
		`UPDATE continuity_observations SET status = ?, status_at = ? WHERE id = ? AND status != ?`,
		status, now, observationID, StatusReplaced)
	if err != nil {
		return Observation{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Observation{}, ErrNotFound
	}
	return r.getObservation(ctx, observationID)
}

// OpenCounts returns, for every scene with at least one open observation in
// the work, how many it has — the badge ContextPanel shows for the scene
// currently being written.
func (r *Repo) OpenCounts(ctx context.Context, projectID string) (map[string]int, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, fmt.Errorf("continuity: open_counts: project_id required")
	}
	rows, err := r.s.DB().QueryContext(ctx,
		`SELECT node_id, COUNT(*) FROM continuity_observations WHERE project_id = ? AND status = ? GROUP BY node_id`,
		projectID, StatusOpen)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var nodeID string
		var n int
		if err := rows.Scan(&nodeID, &n); err != nil {
			return nil, err
		}
		out[nodeID] = n
	}
	return out, rows.Err()
}

func (r *Repo) getObservation(ctx context.Context, id string) (Observation, error) {
	row := r.s.DB().QueryRowContext(ctx,
		`SELECT id, check_id, project_id, node_id, category, kind, severity, summary, suggestion,
		        paragraph_from, paragraph_to, quote, refs_json, status, status_at, created_at
		   FROM continuity_observations WHERE id = ?`, id)
	obs, err := scanObservation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Observation{}, ErrNotFound
	}
	return obs, err
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows, so scanObservation
// serves getObservation's single-row read and List's multi-row read with
// one implementation.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanObservation(row rowScanner) (Observation, error) {
	var obs Observation
	var refsJSON string
	if err := row.Scan(&obs.ID, &obs.CheckID, &obs.ProjectID, &obs.NodeID,
		&obs.Category, &obs.Kind, &obs.Severity, &obs.Summary, &obs.Suggestion,
		&obs.ParagraphFrom, &obs.ParagraphTo, &obs.Quote, &refsJSON,
		&obs.Status, &obs.StatusAt, &obs.CreatedAt); err != nil {
		return Observation{}, err
	}
	refs, err := unmarshalRefs(refsJSON)
	if err != nil {
		return Observation{}, err
	}
	obs.Refs = refs
	return obs, nil
}

func marshalRefs(refs []Ref) (string, error) {
	if refs == nil {
		refs = []Ref{}
	}
	b, err := json.Marshal(refs)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func unmarshalRefs(raw string) ([]Ref, error) {
	var refs []Ref
	if raw == "" {
		return []Ref{}, nil
	}
	if err := json.Unmarshal([]byte(raw), &refs); err != nil {
		return nil, err
	}
	if refs == nil {
		refs = []Ref{}
	}
	return refs, nil
}
