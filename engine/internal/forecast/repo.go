package forecast

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/episode"
	"github.com/devlikebear/linetta/engine/internal/store"
	"github.com/google/uuid"
)

// ErrNotFound means the forecast id does not exist in this project.
var ErrNotFound = errors.New("forecast not found")

// Service is the forecast package's entry point: Save validates and
// stores, List reports every forecast with freshly recomputed staleness,
// and Select/ClearSelection/Delete are the writer's own direct-RPC
// actions — no LLM turn involved (phase-4-forecast.md's Goal section).
type Service struct {
	store *store.Store
	deps  Deps
}

// New builds a Service. deps is the same set of repos engineapp already
// wires for the brief builder and MCP tools.
func New(s *store.Store, deps Deps) *Service {
	return &Service{store: s, deps: deps}
}

// Save validates in against every rule in 4.A.2, and only when nothing is
// wrong with it, assigns branch ids b1..bN, anchors to
// episode.LastWritten, computes the input fingerprint and stores the
// result. Mirrors proposals.Submit/continuity.Validate's own shape:
// problems are collected and returned together, and a submission with any
// problem at all stores nothing.
func (s *Service) Save(ctx context.Context, now int64, in SaveInput, source string) (Forecast, []Problem, error) {
	projectID := strings.TrimSpace(in.ProjectID)
	if projectID == "" {
		return Forecast{}, []Problem{{Index: GlobalIndex, Message: "project_id is required"}}, nil
	}

	lookup := refLookup{ctx: ctx, projectID: projectID, entities: s.deps.Entities, facts: s.deps.Facts, nodes: s.deps.Nodes}
	if problems := Validate(lookup, in); len(problems) > 0 {
		return Forecast{}, problems, nil
	}

	source = strings.TrimSpace(source)
	if source == "" {
		source = SourceExternal
	}

	proj, err := s.deps.Projects.Get(ctx, projectID)
	if err != nil {
		return Forecast{}, nil, fmt.Errorf("forecast save: load project: %w", err)
	}

	baseNodeID := ""
	if ep, found, err := episode.LastWritten(ctx, s.deps.Nodes, projectID, proj.OutlinePreset); err != nil {
		return Forecast{}, nil, fmt.Errorf("forecast save: last written episode: %w", err)
	} else if found {
		baseNodeID = ep.NodeID
	}

	hashes, err := Inputs(ctx, s.deps, projectID)
	if err != nil {
		return Forecast{}, nil, fmt.Errorf("forecast save: input fingerprint: %w", err)
	}

	branches := make([]Branch, len(in.Branches))
	for i, b := range in.Branches {
		branches[i] = Branch{ID: fmt.Sprintf("b%d", i+1), BranchInput: b}
	}

	f := Forecast{
		ID:          uuid.NewString(),
		ProjectID:   projectID,
		Divergence:  strings.TrimSpace(in.Divergence),
		BaseNodeID:  baseNodeID,
		Horizon:     in.Horizon,
		Branches:    branches,
		Fingerprint: hashes.Fingerprint(),
		InputHashes: hashes,
		Source:      source,
		CreatedAt:   now,
	}
	if err := s.insert(ctx, f); err != nil {
		return Forecast{}, nil, fmt.Errorf("forecast save: %w", err)
	}
	return f, nil, nil
}

// List returns every forecast for projectID, newest first, each with its
// staleness recomputed against the project's canon RIGHT NOW — never a
// cached flag, so a forecast that looks stale and is listed again after
// canon reverts reports fresh again (unlike InkOS's one-way flag; §3).
func (s *Service) List(ctx context.Context, projectID string) ([]ListedForecast, error) {
	rows, err := s.listByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	current, err := Inputs(ctx, s.deps, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]ListedForecast, 0, len(rows))
	for _, f := range rows {
		reasons := Diff(f.InputHashes, current)
		out = append(out, ListedForecast{Forecast: f, Stale: len(reasons) > 0, Reasons: reasons})
	}
	return out, nil
}

// Select sets forecastID's chosen branch, clearing any other selection in
// the same project first — one selection per project. A stale forecast may
// still be selected: the engine does not gate this (phase-4-forecast.md's
// 4.C.1 "정책을 한 곳(엔진)에 둔다"); callers surface the staleness as a
// warning, not a refusal.
func (s *Service) Select(ctx context.Context, now int64, forecastID, branchID string) (Forecast, error) {
	f, err := s.get(ctx, forecastID)
	if err != nil {
		return Forecast{}, err
	}
	branchID = strings.TrimSpace(branchID)
	found := false
	for _, b := range f.Branches {
		if b.ID == branchID {
			found = true
			break
		}
	}
	if !found {
		return Forecast{}, fmt.Errorf("forecast select: branch %q not found in forecast %q", branchID, forecastID)
	}

	tx, err := s.store.DB().BeginTx(ctx, nil)
	if err != nil {
		return Forecast{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`UPDATE forecasts SET selected_branch = NULL, selected_at = NULL WHERE project_id = ?`, f.ProjectID); err != nil {
		return Forecast{}, err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE forecasts SET selected_branch = ?, selected_at = ? WHERE id = ?`, branchID, now, forecastID); err != nil {
		return Forecast{}, err
	}
	if err := tx.Commit(); err != nil {
		return Forecast{}, err
	}
	f.SelectedBranch = branchID
	f.SelectedAt = now
	return f, nil
}

// ClearSelection removes projectID's selection, if any. Not an error when
// there was none.
func (s *Service) ClearSelection(ctx context.Context, projectID string) error {
	_, err := s.store.DB().ExecContext(ctx,
		`UPDATE forecasts SET selected_branch = NULL, selected_at = NULL WHERE project_id = ? AND selected_branch IS NOT NULL`,
		projectID)
	return err
}

// Delete removes one forecast. Not an error when it did not exist —
// proposals.Reject's own "retrying a batch request" tolerance, applied to
// a single delete.
func (s *Service) Delete(ctx context.Context, forecastID string) error {
	_, err := s.store.DB().ExecContext(ctx, `DELETE FROM forecasts WHERE id = ?`, forecastID)
	return err
}

// Selected returns projectID's currently selected forecast (there is at
// most one — Select enforces that), and whether one exists. This is what
// storycontext's DirectionSource reads for DirectionBrief.PlannedBranch.
func (s *Service) Selected(ctx context.Context, projectID string) (Forecast, bool, error) {
	row := s.store.DB().QueryRowContext(ctx, selectColumns+
		` FROM forecasts WHERE project_id = ? AND selected_branch IS NOT NULL LIMIT 1`, projectID)
	f, err := scanForecastRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Forecast{}, false, nil
	}
	if err != nil {
		return Forecast{}, false, err
	}
	return f, true, nil
}

// Stale reports whether f is stale against its project's canon RIGHT NOW,
// and why — the single-forecast version of what List computes for every
// row at once. storycontext's direction adapter uses this to decide
// whether the brief's planned-branch note needs the "canon changed" line.
func (s *Service) Stale(ctx context.Context, f Forecast) ([]StaleReason, error) {
	current, err := Inputs(ctx, s.deps, f.ProjectID)
	if err != nil {
		return nil, err
	}
	return Diff(f.InputHashes, current), nil
}

const selectColumns = `SELECT id, project_id, divergence, base_node_id, horizon, branches_json,
       fingerprint, input_hashes_json, source, selected_branch, selected_at, created_at`

func (s *Service) get(ctx context.Context, id string) (Forecast, error) {
	row := s.store.DB().QueryRowContext(ctx, selectColumns+` FROM forecasts WHERE id = ?`, id)
	f, err := scanForecastRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Forecast{}, ErrNotFound
	}
	return f, err
}

func (s *Service) listByProject(ctx context.Context, projectID string) ([]Forecast, error) {
	rows, err := s.store.DB().QueryContext(ctx, selectColumns+
		` FROM forecasts WHERE project_id = ? ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Forecast{}
	for rows.Next() {
		f, err := scanForecastRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Service) insert(ctx context.Context, f Forecast) error {
	branchesJSON, err := json.Marshal(f.Branches)
	if err != nil {
		return err
	}
	hashesJSON, err := json.Marshal(f.InputHashes)
	if err != nil {
		return err
	}
	_, err = s.store.DB().ExecContext(ctx, `
INSERT INTO forecasts (id, project_id, divergence, base_node_id, horizon, branches_json,
                        fingerprint, input_hashes_json, source, selected_branch, selected_at, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, ?)`,
		f.ID, f.ProjectID, f.Divergence, nullable(f.BaseNodeID), f.Horizon, string(branchesJSON),
		f.Fingerprint, string(hashesJSON), f.Source, f.CreatedAt)
	return err
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanForecastRow(row scanner) (Forecast, error) {
	var (
		f                          Forecast
		baseNodeID, selectedBranch sql.NullString
		selectedAt                 sql.NullInt64
		branchesJSON, hashesJSON   string
	)
	if err := row.Scan(&f.ID, &f.ProjectID, &f.Divergence, &baseNodeID, &f.Horizon, &branchesJSON,
		&f.Fingerprint, &hashesJSON, &f.Source, &selectedBranch, &selectedAt, &f.CreatedAt); err != nil {
		return Forecast{}, err
	}
	f.BaseNodeID = baseNodeID.String
	f.SelectedBranch = selectedBranch.String
	f.SelectedAt = selectedAt.Int64
	if err := json.Unmarshal([]byte(branchesJSON), &f.Branches); err != nil {
		return Forecast{}, fmt.Errorf("forecast %s: decode branches: %w", f.ID, err)
	}
	if err := json.Unmarshal([]byte(hashesJSON), &f.InputHashes); err != nil {
		return Forecast{}, fmt.Errorf("forecast %s: decode input hashes: %w", f.ID, err)
	}
	return f, nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
