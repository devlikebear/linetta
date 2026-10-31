package proposals

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrNotFound is returned when a proposal or item id does not exist.
var ErrNotFound = errors.New("proposal not found")

// List returns every batch for projectID whose items match statuses (an
// empty list means every status), newest first — proposals.list's data.
func (s *Service) List(ctx context.Context, projectID string, statuses []string) ([]ProposalWithItems, error) {
	list, err := s.listProposals(ctx, projectID, statuses)
	if err != nil {
		return nil, err
	}
	if err := s.refreshDuplicates(ctx, projectID, list); err != nil {
		return nil, err
	}
	return list, nil
}

// ProjectOfUndoBatch names the work a structural batch from Accept belongs
// to. ok is false for any batch Accept did not make, which is how
// linetta_undo_last_change tells a proposal's outline change, whose items have
// to go back to pending with it, from a plain storyops batch.
func (s *Service) ProjectOfUndoBatch(ctx context.Context, batchID string) (projectID string, ok bool, err error) {
	err = s.store.DB().QueryRowContext(ctx,
		`SELECT project_id FROM story_proposal_items WHERE undo_batch_id = ? LIMIT 1`, batchID).Scan(&projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return projectID, true, nil
}

// PendingCount returns how many pending items projectID has —
// proposals.pending_count and CanonPanel's badge.
func (s *Service) PendingCount(ctx context.Context, projectID string) (int, error) {
	return s.countPending(ctx, projectID)
}

// dbTx is satisfied by both *sql.DB and *sql.Tx — every helper below takes
// one so Submit/Accept can run their reads and writes in a single
// transaction while a read-only caller (List, pending_count) can pass the
// raw DB.
type dbTx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (s *Service) insertProposal(ctx context.Context, tx dbTx, p Proposal) error {
	bindingsJSON, err := marshalStringMap(p.RefBindings)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO story_proposals (id, project_id, purpose, forecast_id, source, summary, ref_bindings_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.ProjectID, p.Purpose, nullableString(p.ForecastID), p.Source, p.Summary, bindingsJSON, p.CreatedAt)
	return err
}

// getProposal returns one proposal's header row — Accept needs Purpose to
// decide whether a proposal's structural items batch together
// (purpose=forecast) or apply one at a time like every other purpose.
func (s *Service) getProposal(ctx context.Context, tx dbTx, id string) (Proposal, error) {
	row := tx.QueryRowContext(ctx,
		`SELECT id, project_id, purpose, forecast_id, source, summary, ref_bindings_json, created_at
		   FROM story_proposals WHERE id = ?`, id)
	p, err := scanProposal(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Proposal{}, ErrNotFound
	}
	return p, err
}

func (s *Service) insertItem(ctx context.Context, tx dbTx, it Item) error {
	opJSON, err := json.Marshal(it.Op)
	if err != nil {
		return err
	}
	dependsJSON, err := marshalStringSlice(it.DependsOn)
	if err != nil {
		return err
	}
	evidenceJSON, err := json.Marshal(nonNilEvidence(it.Evidence))
	if err != nil {
		return err
	}
	fromNodesJSON, err := marshalStringSlice(it.FromNodeIDs)
	if err != nil {
		return err
	}
	dupesJSON, err := json.Marshal(nonNilDuplicates(it.Duplicates))
	if err != nil {
		return err
	}
	resultJSON, err := json.Marshal(it.Result)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO story_proposal_items
		    (id, proposal_id, project_id, ordinal, op_json, depends_on_json, evidence_json,
		     from_node_ids_json, duplicates_json, status, result_json, status_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		it.ID, it.ProposalID, it.ProjectID, it.Ordinal, string(opJSON), dependsJSON, string(evidenceJSON),
		fromNodesJSON, string(dupesJSON), it.Status, string(resultJSON), it.StatusAt)
	return err
}

const itemColumns = `id, proposal_id, project_id, ordinal, op_json, depends_on_json, evidence_json,
       from_node_ids_json, duplicates_json, status, result_json, status_at, undo_batch_id`

func (s *Service) getItem(ctx context.Context, tx dbTx, id string) (Item, error) {
	row := tx.QueryRowContext(ctx, `SELECT `+itemColumns+` FROM story_proposal_items WHERE id = ?`, id)
	it, err := scanItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	return it, err
}

// getItemsByIDs returns every requested item that exists and belongs to
// projectID, keyed by id. A requested id that does not resolve (wrong
// project, or does not exist at all) is simply absent from the map — the
// caller (Accept) is the one that turns "absent" into a refusal naming the
// id, the same division continuity.Validate's RefLookup follows.
func (s *Service) getItemsByIDs(ctx context.Context, projectID string, ids []string) (map[string]Item, error) {
	out := map[string]Item{}
	if len(ids) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+1)
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	args = append(args, projectID)
	rows, err := s.store.DB().QueryContext(ctx,
		`SELECT `+itemColumns+` FROM story_proposal_items WHERE id IN (`+strings.Join(placeholders, ",")+`) AND project_id = ?`,
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out[it.ID] = it
	}
	return out, rows.Err()
}

// listItemsByProposal returns one proposal's items in ordinal order.
func (s *Service) listItemsByProposal(ctx context.Context, tx dbTx, proposalID string) ([]Item, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT `+itemColumns+` FROM story_proposal_items WHERE proposal_id = ? ORDER BY ordinal ASC`, proposalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// listPendingCreateEntityItems returns every pending create_entity item for
// projectID across every proposal — dedupe.go's "pending" half of a new
// candidate's comparison set.
func (s *Service) listPendingCreateEntityItems(ctx context.Context, projectID string) ([]Item, error) {
	rows, err := s.store.DB().QueryContext(ctx,
		`SELECT `+itemColumns+` FROM story_proposal_items WHERE project_id = ? AND status = ?`,
		projectID, StatusPending)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		if it.Op.Type != "create_entity" {
			continue
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// listProposals returns every batch for projectID whose items match
// statuses (a batch with no matching item is omitted), newest first, each
// with only the matching items attached. An empty statuses list means every
// status.
func (s *Service) listProposals(ctx context.Context, projectID string, statuses []string) ([]ProposalWithItems, error) {
	rows, err := s.store.DB().QueryContext(ctx,
		`SELECT id, project_id, purpose, forecast_id, source, summary, ref_bindings_json, created_at
		   FROM story_proposals WHERE project_id = ? ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	var proposals []Proposal
	for rows.Next() {
		p, err := scanProposal(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		proposals = append(proposals, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	statusSet := map[string]bool{}
	for _, st := range statuses {
		statusSet[st] = true
	}

	out := make([]ProposalWithItems, 0, len(proposals))
	for _, p := range proposals {
		items, err := s.listItemsByProposal(ctx, s.store.DB(), p.ID)
		if err != nil {
			return nil, err
		}
		if len(statusSet) > 0 {
			filtered := items[:0:0]
			for _, it := range items {
				if statusSet[it.Status] {
					filtered = append(filtered, it)
				}
			}
			items = filtered
		}
		if len(items) == 0 {
			continue
		}
		out = append(out, ProposalWithItems{Proposal: p, Items: items})
	}
	return out, nil
}

// countPending returns how many pending items projectID has — the badge
// CanonPanel and proposals.pending_count read.
func (s *Service) countPending(ctx context.Context, projectID string) (int, error) {
	var n int
	err := s.store.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM story_proposal_items WHERE project_id = ? AND status = ?`,
		projectID, StatusPending).Scan(&n)
	return n, err
}

func (s *Service) updateItemStatus(ctx context.Context, tx dbTx, itemID, status string, result ItemResult, now int64) error {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		`UPDATE story_proposal_items SET status = ?, result_json = ?, status_at = ? WHERE id = ?`,
		status, string(resultJSON), now, itemID)
	return err
}

// updateItemStatusOnly changes status and status_at ONLY — leaving
// result_json and undo_batch_id exactly as they were. This is
// proposals.Undo's first transaction, carrying every item a batch covers
// to StatusReverting before storyops.UndoApply ever runs: the point of
// reverting is that result_json (the created id, undone or not) and
// undo_batch_id (the handle a retry or ReconcileReverting needs to find
// these same items again) must survive untouched until the outcome is
// actually known, so this helper — unlike updateItemStatus — must never
// touch those two columns.
func (s *Service) updateItemStatusOnly(ctx context.Context, tx dbTx, itemID, status string, now int64) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE story_proposal_items SET status = ?, status_at = ? WHERE id = ?`,
		status, now, itemID)
	return err
}

// updateItemUndoBatch tags itemID with batchID (or clears it when batchID
// is ""). Set alongside updateItemStatus, never instead of it: the two
// columns are independent so a status update to pending (proposals.undo)
// and a status update to accepted (Accept) can each touch only the column
// they mean to.
func (s *Service) updateItemUndoBatch(ctx context.Context, tx dbTx, itemID, batchID string) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE story_proposal_items SET undo_batch_id = ? WHERE id = ?`, nullableString(batchID), itemID)
	return err
}

// restoreItemFull writes back an item's status, result, status_at and
// undo_batch_id together in one statement — proposals.Undo's compensation
// path, where a failed storyops.UndoApply means every column the earlier
// "carry to pending" transaction touched must return to exactly what it
// was, not just the ones a narrower helper happens to cover.
func (s *Service) restoreItemFull(ctx context.Context, tx dbTx, it Item) error {
	resultJSON, err := json.Marshal(it.Result)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		`UPDATE story_proposal_items SET status = ?, result_json = ?, status_at = ?, undo_batch_id = ? WHERE id = ?`,
		it.Status, string(resultJSON), it.StatusAt, nullableString(it.UndoBatchID), it.ID)
	return err
}

// listItemsByUndoBatch returns every item tagged with batchID in
// projectID — proposals.undo's own "what does this batch cover" query.
func (s *Service) listItemsByUndoBatch(ctx context.Context, tx dbTx, projectID, batchID string) ([]Item, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT `+itemColumns+` FROM story_proposal_items WHERE project_id = ? AND undo_batch_id = ? ORDER BY ordinal ASC`,
		projectID, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// listItemsByStatus returns every item across every project carrying
// status, ordered so items from the same undo batch land together —
// ReconcileReverting's own "what needs reconciling" query at engine
// startup, where there is no single project to scope to yet (unlike
// listItemsByUndoBatch, which proposals.Undo already knows the project and
// batch id for).
func (s *Service) listItemsByStatus(ctx context.Context, tx dbTx, status string) ([]Item, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT `+itemColumns+` FROM story_proposal_items WHERE status = ? ORDER BY project_id ASC, undo_batch_id ASC, ordinal ASC`,
		status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func (s *Service) updateProposalRefBindings(ctx context.Context, tx dbTx, proposalID string, bindings map[string]string) error {
	bindingsJSON, err := marshalStringMap(bindings)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		`UPDATE story_proposals SET ref_bindings_json = ? WHERE id = ?`, bindingsJSON, proposalID)
	return err
}

func (s *Service) getProposalRefBindings(ctx context.Context, tx dbTx, proposalID string) (map[string]string, error) {
	var raw string
	err := tx.QueryRowContext(ctx, `SELECT ref_bindings_json FROM story_proposals WHERE id = ?`, proposalID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return unmarshalStringMap(raw)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanItem(row rowScanner) (Item, error) {
	var it Item
	var opJSON, dependsJSON, evidenceJSON, fromNodesJSON, dupesJSON, resultJSON string
	var undoBatchID sql.NullString
	if err := row.Scan(&it.ID, &it.ProposalID, &it.ProjectID, &it.Ordinal, &opJSON, &dependsJSON,
		&evidenceJSON, &fromNodesJSON, &dupesJSON, &it.Status, &resultJSON, &it.StatusAt, &undoBatchID); err != nil {
		return Item{}, err
	}
	it.UndoBatchID = undoBatchID.String
	if err := json.Unmarshal([]byte(opJSON), &it.Op); err != nil {
		return Item{}, fmt.Errorf("decode op: %w", err)
	}
	deps, err := unmarshalStringSlice(dependsJSON)
	if err != nil {
		return Item{}, fmt.Errorf("decode depends_on: %w", err)
	}
	it.DependsOn = deps
	if err := json.Unmarshal([]byte(evidenceJSON), &it.Evidence); err != nil {
		return Item{}, fmt.Errorf("decode evidence: %w", err)
	}
	fromNodes, err := unmarshalStringSlice(fromNodesJSON)
	if err != nil {
		return Item{}, fmt.Errorf("decode from_node_ids: %w", err)
	}
	it.FromNodeIDs = fromNodes
	if err := json.Unmarshal([]byte(dupesJSON), &it.Duplicates); err != nil {
		return Item{}, fmt.Errorf("decode duplicates: %w", err)
	}
	if err := json.Unmarshal([]byte(resultJSON), &it.Result); err != nil {
		return Item{}, fmt.Errorf("decode result: %w", err)
	}
	return it, nil
}

func scanProposal(row rowScanner) (Proposal, error) {
	var p Proposal
	var bindingsJSON string
	var forecastID sql.NullString
	if err := row.Scan(&p.ID, &p.ProjectID, &p.Purpose, &forecastID, &p.Source, &p.Summary, &bindingsJSON, &p.CreatedAt); err != nil {
		return Proposal{}, err
	}
	p.ForecastID = forecastID.String
	bindings, err := unmarshalStringMap(bindingsJSON)
	if err != nil {
		return Proposal{}, fmt.Errorf("decode ref_bindings: %w", err)
	}
	p.RefBindings = bindings
	return p, nil
}

// nullableString turns an empty string into a SQL NULL so an optional
// column (forecast_id) reads back empty rather than "" vs NULL mattering
// to a future query.
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func marshalStringSlice(v []string) (string, error) {
	if v == nil {
		v = []string{}
	}
	b, err := json.Marshal(v)
	return string(b), err
}

func unmarshalStringSlice(raw string) ([]string, error) {
	var out []string
	if strings.TrimSpace(raw) == "" {
		return []string{}, nil
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []string{}
	}
	return out, nil
}

func marshalStringMap(v map[string]string) (string, error) {
	if v == nil {
		v = map[string]string{}
	}
	b, err := json.Marshal(v)
	return string(b), err
}

func unmarshalStringMap(raw string) (map[string]string, error) {
	out := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]string{}
	}
	return out, nil
}

func nonNilEvidence(v []EvidenceInput) []EvidenceInput {
	if v == nil {
		return []EvidenceInput{}
	}
	return v
}

func nonNilDuplicates(v []Duplicate) []Duplicate {
	if v == nil {
		return []Duplicate{}
	}
	return v
}
