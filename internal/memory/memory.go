package memory

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/linetta/internal/store"
	"github.com/devlikebear/linetta/internal/work"
)

var (
	ErrInvalidInput = errors.New("invalid memory input")
	ErrNotFound     = errors.New("memory item not found")
)

type Kind string

const (
	KindCharacter     Kind = "character"
	KindWorldFact     Kind = "world_fact"
	KindTimelineEvent Kind = "timeline_event"
	KindPlotThread    Kind = "plot_thread"
	KindStyleRule     Kind = "style_rule"
	KindSource        Kind = "source"
)

type Status string

const (
	StatusDraft    Status = "draft"
	StatusCanon    Status = "canon"
	StatusArchived Status = "archived"
)

type Importance string

const (
	ImportanceLow    Importance = "low"
	ImportanceMedium Importance = "medium"
	ImportanceHigh   Importance = "high"
)

type DecisionType string

const (
	DecisionCreate  DecisionType = "create"
	DecisionUpdate  DecisionType = "update"
	DecisionArchive DecisionType = "archive"
	DecisionApprove DecisionType = "approve"
)

type Item struct {
	ID         string     `json:"id"`
	WorkID     string     `json:"work_id"`
	Kind       Kind       `json:"kind"`
	Title      string     `json:"title"`
	Body       string     `json:"body"`
	Status     Status     `json:"status"`
	Importance Importance `json:"importance"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type CreateItemInput struct {
	WorkID     string     `json:"work_id"`
	Kind       Kind       `json:"kind"`
	Title      string     `json:"title"`
	Body       string     `json:"body"`
	Status     Status     `json:"status"`
	Importance Importance `json:"importance"`
	Reason     string     `json:"reason"`
	Actor      string     `json:"actor"`
}

type UpdateItemInput struct {
	Title      string     `json:"title"`
	Body       string     `json:"body"`
	Status     Status     `json:"status"`
	Importance Importance `json:"importance"`
	Reason     string     `json:"reason"`
	Actor      string     `json:"actor"`
}

type ListFilter struct {
	Kind   Kind
	Status Status
	Query  string
}

type Decision struct {
	ID          string       `json:"id"`
	WorkID      string       `json:"work_id"`
	CanonItemID string       `json:"canon_item_id"`
	Type        DecisionType `json:"decision_type"`
	Reason      string       `json:"reason"`
	Actor       string       `json:"actor"`
	CreatedAt   time.Time    `json:"created_at"`
}

type ProposalChangeType string

const (
	ProposalChangeCreate  ProposalChangeType = "create"
	ProposalChangeUpdate  ProposalChangeType = "update"
	ProposalChangeArchive ProposalChangeType = "archive"
	ProposalChangeLink    ProposalChangeType = "link"
)

type ProposalStatus string

const (
	ProposalPending  ProposalStatus = "pending"
	ProposalApproved ProposalStatus = "approved"
	ProposalRejected ProposalStatus = "rejected"
	ProposalDeferred ProposalStatus = "deferred"
)

type Proposal struct {
	ID           string             `json:"id"`
	WorkID       string             `json:"work_id"`
	EpisodeID    string             `json:"episode_id"`
	RunID        string             `json:"run_id"`
	TargetItemID string             `json:"target_item_id"`
	ChangeType   ProposalChangeType `json:"change_type"`
	Kind         Kind               `json:"kind"`
	Title        string             `json:"title"`
	BeforeBody   string             `json:"before_body"`
	AfterBody    string             `json:"after_body"`
	Reason       string             `json:"reason"`
	Confidence   float64            `json:"confidence"`
	Status       ProposalStatus     `json:"status"`
	CreatedAt    time.Time          `json:"created_at"`
	DecidedAt    *time.Time         `json:"decided_at,omitempty"`
}

type IssueSeverity string

const (
	IssueInfo    IssueSeverity = "info"
	IssueWarning IssueSeverity = "warning"
	IssueBlocker IssueSeverity = "blocker"
)

type IssueStatus string

const (
	IssueOpen     IssueStatus = "open"
	IssueAccepted IssueStatus = "accepted"
	IssueResolved IssueStatus = "resolved"
	IssueIgnored  IssueStatus = "ignored"
)

type ContinuityIssue struct {
	ID             string        `json:"id"`
	WorkID         string        `json:"work_id"`
	EpisodeID      string        `json:"episode_id"`
	RunID          string        `json:"run_id"`
	Severity       IssueSeverity `json:"severity"`
	Title          string        `json:"title"`
	Body           string        `json:"body"`
	RelatedItemIDs string        `json:"related_item_ids"`
	Status         IssueStatus   `json:"status"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

type CreateIssueInput struct {
	WorkID         string        `json:"work_id"`
	EpisodeID      string        `json:"episode_id"`
	RunID          string        `json:"run_id"`
	Severity       IssueSeverity `json:"severity"`
	Title          string        `json:"title"`
	Body           string        `json:"body"`
	RelatedItemIDs string        `json:"related_item_ids"`
}

type CreateProposalInput struct {
	WorkID       string             `json:"work_id"`
	EpisodeID    string             `json:"episode_id"`
	RunID        string             `json:"run_id"`
	TargetItemID string             `json:"target_item_id"`
	ChangeType   ProposalChangeType `json:"change_type"`
	Kind         Kind               `json:"kind"`
	Title        string             `json:"title"`
	BeforeBody   string             `json:"before_body"`
	AfterBody    string             `json:"after_body"`
	Reason       string             `json:"reason"`
	Confidence   float64            `json:"confidence"`
}

type Repository struct {
	db       *store.DB
	workRepo *work.Repository
}

func NewRepository(db *store.DB, workRepo *work.Repository) *Repository {
	return &Repository{db: db, workRepo: workRepo}
}

func (r *Repository) CreateItem(ctx context.Context, input CreateItemInput) (Item, error) {
	input = normalizeCreateInput(input)
	if input.WorkID == "" || input.Kind == "" || input.Title == "" {
		return Item{}, fmt.Errorf("%w: work id, kind, and title are required", ErrInvalidInput)
	}
	if err := r.requireWork(ctx, input.WorkID); err != nil {
		return Item{}, err
	}

	now := time.Now().UTC()
	item := Item{
		ID:         newID("canon"),
		WorkID:     input.WorkID,
		Kind:       input.Kind,
		Title:      input.Title,
		Body:       input.Body,
		Status:     input.Status,
		Importance: input.Importance,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	_, err := r.conn().ExecContext(ctx, `
		INSERT INTO canon_items (id, work_id, kind, title, body, status, importance, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, item.ID, item.WorkID, item.Kind, item.Title, item.Body, item.Status, item.Importance, formatTime(item.CreatedAt), formatTime(item.UpdatedAt))
	if err != nil {
		return Item{}, err
	}
	if err := r.RecordDecision(ctx, Decision{
		WorkID:      item.WorkID,
		CanonItemID: item.ID,
		Type:        DecisionCreate,
		Reason:      input.Reason,
		Actor:       input.Actor,
	}); err != nil {
		return Item{}, err
	}
	return item, nil
}

func (r *Repository) UpdateItem(ctx context.Context, id string, input UpdateItemInput) (Item, error) {
	current, err := r.GetItem(ctx, id)
	if err != nil {
		return Item{}, err
	}
	input = normalizeUpdateInput(input, current)
	if input.Title == "" {
		return Item{}, fmt.Errorf("%w: title is required", ErrInvalidInput)
	}

	now := time.Now().UTC()
	_, err = r.conn().ExecContext(ctx, `
		UPDATE canon_items
		SET title = ?, body = ?, status = ?, importance = ?, updated_at = ?
		WHERE id = ?
	`, input.Title, input.Body, input.Status, input.Importance, formatTime(now), current.ID)
	if err != nil {
		return Item{}, err
	}
	if err := r.RecordDecision(ctx, Decision{
		WorkID:      current.WorkID,
		CanonItemID: current.ID,
		Type:        DecisionUpdate,
		Reason:      input.Reason,
		Actor:       input.Actor,
	}); err != nil {
		return Item{}, err
	}
	return r.GetItem(ctx, current.ID)
}

func (r *Repository) ArchiveItem(ctx context.Context, id, reason, actor string) (Item, error) {
	current, err := r.GetItem(ctx, id)
	if err != nil {
		return Item{}, err
	}
	now := time.Now().UTC()
	_, err = r.conn().ExecContext(ctx, `
		UPDATE canon_items
		SET status = ?, updated_at = ?
		WHERE id = ?
	`, StatusArchived, formatTime(now), current.ID)
	if err != nil {
		return Item{}, err
	}
	if err := r.RecordDecision(ctx, Decision{
		WorkID:      current.WorkID,
		CanonItemID: current.ID,
		Type:        DecisionArchive,
		Reason:      strings.TrimSpace(reason),
		Actor:       normalizeActor(actor),
	}); err != nil {
		return Item{}, err
	}
	return r.GetItem(ctx, current.ID)
}

func (r *Repository) ListItems(ctx context.Context, workID string, filter ListFilter) ([]Item, error) {
	workID = strings.TrimSpace(workID)
	if err := r.requireWork(ctx, workID); err != nil {
		return nil, err
	}

	query := `SELECT id, work_id, kind, title, body, status, importance, created_at, updated_at FROM canon_items WHERE work_id = ?`
	args := []any{workID}
	if filter.Kind != "" {
		query += ` AND kind = ?`
		args = append(args, filter.Kind)
	}
	if filter.Status != "" {
		query += ` AND status = ?`
		args = append(args, filter.Status)
	}
	if strings.TrimSpace(filter.Query) != "" {
		query += ` AND (title LIKE ? OR body LIKE ?)`
		term := "%" + strings.TrimSpace(filter.Query) + "%"
		args = append(args, term, term)
	}
	query += ` ORDER BY updated_at DESC, created_at DESC, id DESC`

	rows, err := r.conn().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Item
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) GetItem(ctx context.Context, id string) (Item, error) {
	row := r.conn().QueryRowContext(ctx, `
		SELECT id, work_id, kind, title, body, status, importance, created_at, updated_at
		FROM canon_items
		WHERE id = ?
	`, strings.TrimSpace(id))
	item, err := scanItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, err
	}
	return item, nil
}

func (r *Repository) RecordDecision(ctx context.Context, decision Decision) error {
	decision.WorkID = strings.TrimSpace(decision.WorkID)
	decision.CanonItemID = strings.TrimSpace(decision.CanonItemID)
	decision.Actor = normalizeActor(decision.Actor)
	decision.Reason = strings.TrimSpace(decision.Reason)
	if decision.WorkID == "" || decision.CanonItemID == "" || decision.Type == "" {
		return fmt.Errorf("%w: work id, canon item id, and decision type are required", ErrInvalidInput)
	}
	if decision.ID == "" {
		decision.ID = newID("decision")
	}
	if decision.CreatedAt.IsZero() {
		decision.CreatedAt = time.Now().UTC()
	}
	_, err := r.conn().ExecContext(ctx, `
		INSERT INTO canon_decisions (id, work_id, canon_item_id, decision_type, reason, actor, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, decision.ID, decision.WorkID, decision.CanonItemID, decision.Type, decision.Reason, decision.Actor, formatTime(decision.CreatedAt))
	return err
}

func (r *Repository) ListDecisions(ctx context.Context, workID string) ([]Decision, error) {
	workID = strings.TrimSpace(workID)
	if err := r.requireWork(ctx, workID); err != nil {
		return nil, err
	}
	rows, err := r.conn().QueryContext(ctx, `
		SELECT id, work_id, canon_item_id, decision_type, reason, actor, created_at
		FROM canon_decisions
		WHERE work_id = ?
		ORDER BY created_at ASC, id ASC
	`, workID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var decisions []Decision
	for rows.Next() {
		decision, err := scanDecision(rows)
		if err != nil {
			return nil, err
		}
		decisions = append(decisions, decision)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return decisions, nil
}

func (r *Repository) CreateProposal(ctx context.Context, input CreateProposalInput) (Proposal, error) {
	input = normalizeProposalInput(input)
	if input.WorkID == "" || input.EpisodeID == "" || input.RunID == "" || input.ChangeType == "" || input.Kind == "" || input.Title == "" {
		return Proposal{}, fmt.Errorf("%w: work id, episode id, run id, change type, kind, and title are required", ErrInvalidInput)
	}
	if err := r.requireWork(ctx, input.WorkID); err != nil {
		return Proposal{}, err
	}
	now := time.Now().UTC()
	proposal := Proposal{
		ID:           newID("proposal"),
		WorkID:       input.WorkID,
		EpisodeID:    input.EpisodeID,
		RunID:        input.RunID,
		TargetItemID: input.TargetItemID,
		ChangeType:   input.ChangeType,
		Kind:         input.Kind,
		Title:        input.Title,
		BeforeBody:   input.BeforeBody,
		AfterBody:    input.AfterBody,
		Reason:       input.Reason,
		Confidence:   input.Confidence,
		Status:       ProposalPending,
		CreatedAt:    now,
	}
	_, err := r.conn().ExecContext(ctx, `
		INSERT INTO canon_change_proposals (
			id, work_id, episode_id, run_id, target_item_id, change_type, kind, title, before_body, after_body, reason, confidence, status, created_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, proposal.ID, proposal.WorkID, proposal.EpisodeID, proposal.RunID, proposal.TargetItemID, proposal.ChangeType, proposal.Kind, proposal.Title, proposal.BeforeBody, proposal.AfterBody, proposal.Reason, proposal.Confidence, proposal.Status, formatTime(proposal.CreatedAt))
	if err != nil {
		return Proposal{}, err
	}
	return proposal, nil
}

func (r *Repository) GetProposal(ctx context.Context, id string) (Proposal, error) {
	row := r.conn().QueryRowContext(ctx, `
		SELECT id, work_id, episode_id, run_id, target_item_id, change_type, kind, title, before_body, after_body, reason, confidence, status, created_at, decided_at
		FROM canon_change_proposals
		WHERE id = ?
	`, strings.TrimSpace(id))
	proposal, err := scanProposal(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Proposal{}, ErrNotFound
	}
	if err != nil {
		return Proposal{}, err
	}
	return proposal, nil
}

func (r *Repository) ListProposals(ctx context.Context, workID string, status ProposalStatus) ([]Proposal, error) {
	workID = strings.TrimSpace(workID)
	if err := r.requireWork(ctx, workID); err != nil {
		return nil, err
	}
	query := `
		SELECT id, work_id, episode_id, run_id, target_item_id, change_type, kind, title, before_body, after_body, reason, confidence, status, created_at, decided_at
		FROM canon_change_proposals
		WHERE work_id = ?`
	args := []any{workID}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY created_at DESC, id DESC`
	rows, err := r.conn().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var proposals []Proposal
	for rows.Next() {
		proposal, err := scanProposal(rows)
		if err != nil {
			return nil, err
		}
		proposals = append(proposals, proposal)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return proposals, nil
}

func (r *Repository) ApproveProposal(ctx context.Context, id, actor string) (Proposal, error) {
	proposal, err := r.GetProposal(ctx, id)
	if err != nil {
		return Proposal{}, err
	}
	if proposal.Status != ProposalPending && proposal.Status != ProposalDeferred {
		return Proposal{}, fmt.Errorf("%w: proposal is not pending", ErrInvalidInput)
	}
	actor = normalizeActor(actor)
	targetID := proposal.TargetItemID
	switch proposal.ChangeType {
	case ProposalChangeCreate:
		item, err := r.CreateItem(ctx, CreateItemInput{
			WorkID:     proposal.WorkID,
			Kind:       proposal.Kind,
			Title:      proposal.Title,
			Body:       proposal.AfterBody,
			Status:     StatusCanon,
			Importance: ImportanceMedium,
			Reason:     proposal.Reason,
			Actor:      actor,
		})
		if err != nil {
			return Proposal{}, err
		}
		targetID = item.ID
	case ProposalChangeUpdate:
		if targetID == "" {
			return Proposal{}, fmt.Errorf("%w: update proposal requires target item", ErrInvalidInput)
		}
		item, err := r.UpdateItem(ctx, targetID, UpdateItemInput{
			Title:  proposal.Title,
			Body:   proposal.AfterBody,
			Status: StatusCanon,
			Reason: proposal.Reason,
			Actor:  actor,
		})
		if err != nil {
			return Proposal{}, err
		}
		targetID = item.ID
	case ProposalChangeArchive:
		if targetID == "" {
			return Proposal{}, fmt.Errorf("%w: archive proposal requires target item", ErrInvalidInput)
		}
		item, err := r.ArchiveItem(ctx, targetID, proposal.Reason, actor)
		if err != nil {
			return Proposal{}, err
		}
		targetID = item.ID
	case ProposalChangeLink:
		// Link proposals are reserved for a later richer graph UI. Approval still records the human decision.
	default:
		return Proposal{}, fmt.Errorf("%w: unsupported proposal change type", ErrInvalidInput)
	}
	if targetID != "" {
		if err := r.RecordDecision(ctx, Decision{
			WorkID:      proposal.WorkID,
			CanonItemID: targetID,
			Type:        DecisionApprove,
			Reason:      proposal.Reason,
			Actor:       actor,
		}); err != nil {
			return Proposal{}, err
		}
	}
	return r.updateProposalStatus(ctx, proposal.ID, ProposalApproved, targetID)
}

func (r *Repository) RejectProposal(ctx context.Context, id, _ string) (Proposal, error) {
	proposal, err := r.GetProposal(ctx, id)
	if err != nil {
		return Proposal{}, err
	}
	return r.updateProposalStatus(ctx, proposal.ID, ProposalRejected, proposal.TargetItemID)
}

func (r *Repository) DeferProposal(ctx context.Context, id, _ string) (Proposal, error) {
	proposal, err := r.GetProposal(ctx, id)
	if err != nil {
		return Proposal{}, err
	}
	return r.updateProposalStatus(ctx, proposal.ID, ProposalDeferred, proposal.TargetItemID)
}

func (r *Repository) CreateIssue(ctx context.Context, input CreateIssueInput) (ContinuityIssue, error) {
	input = normalizeIssueInput(input)
	if input.WorkID == "" || input.EpisodeID == "" || input.RunID == "" || input.Severity == "" || input.Title == "" {
		return ContinuityIssue{}, fmt.Errorf("%w: work id, episode id, run id, severity, and title are required", ErrInvalidInput)
	}
	if err := r.requireWork(ctx, input.WorkID); err != nil {
		return ContinuityIssue{}, err
	}
	now := time.Now().UTC()
	issue := ContinuityIssue{
		ID:             newID("issue"),
		WorkID:         input.WorkID,
		EpisodeID:      input.EpisodeID,
		RunID:          input.RunID,
		Severity:       input.Severity,
		Title:          input.Title,
		Body:           input.Body,
		RelatedItemIDs: input.RelatedItemIDs,
		Status:         IssueOpen,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	_, err := r.conn().ExecContext(ctx, `
		INSERT INTO continuity_issues (
			id, work_id, episode_id, run_id, severity, title, body, related_item_ids, status, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, issue.ID, issue.WorkID, issue.EpisodeID, issue.RunID, issue.Severity, issue.Title, issue.Body, issue.RelatedItemIDs, issue.Status, formatTime(issue.CreatedAt), formatTime(issue.UpdatedAt))
	if err != nil {
		return ContinuityIssue{}, err
	}
	return issue, nil
}

func (r *Repository) ListIssues(ctx context.Context, workID, episodeID string) ([]ContinuityIssue, error) {
	workID = strings.TrimSpace(workID)
	episodeID = strings.TrimSpace(episodeID)
	if err := r.requireWork(ctx, workID); err != nil {
		return nil, err
	}
	rows, err := r.conn().QueryContext(ctx, `
		SELECT id, work_id, episode_id, run_id, severity, title, body, related_item_ids, status, created_at, updated_at
		FROM continuity_issues
		WHERE work_id = ? AND episode_id = ?
		ORDER BY created_at DESC, id DESC
	`, workID, episodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var issues []ContinuityIssue
	for rows.Next() {
		issue, err := scanIssue(rows)
		if err != nil {
			return nil, err
		}
		issues = append(issues, issue)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return issues, nil
}

func (r *Repository) UpdateIssueStatus(ctx context.Context, id string, status IssueStatus) (ContinuityIssue, error) {
	id = strings.TrimSpace(id)
	if id == "" || status == "" {
		return ContinuityIssue{}, fmt.Errorf("%w: issue id and status are required", ErrInvalidInput)
	}
	now := time.Now().UTC()
	res, err := r.conn().ExecContext(ctx, `
		UPDATE continuity_issues
		SET status = ?, updated_at = ?
		WHERE id = ?
	`, status, formatTime(now), id)
	if err != nil {
		return ContinuityIssue{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return ContinuityIssue{}, err
	}
	if affected == 0 {
		return ContinuityIssue{}, ErrNotFound
	}
	return r.GetIssue(ctx, id)
}

func (r *Repository) GetIssue(ctx context.Context, id string) (ContinuityIssue, error) {
	row := r.conn().QueryRowContext(ctx, `
		SELECT id, work_id, episode_id, run_id, severity, title, body, related_item_ids, status, created_at, updated_at
		FROM continuity_issues
		WHERE id = ?
	`, strings.TrimSpace(id))
	issue, err := scanIssue(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ContinuityIssue{}, ErrNotFound
	}
	if err != nil {
		return ContinuityIssue{}, err
	}
	return issue, nil
}

func (r *Repository) updateProposalStatus(ctx context.Context, id string, status ProposalStatus, targetItemID string) (Proposal, error) {
	now := time.Now().UTC()
	_, err := r.conn().ExecContext(ctx, `
		UPDATE canon_change_proposals
		SET status = ?, target_item_id = ?, decided_at = ?
		WHERE id = ?
	`, status, strings.TrimSpace(targetItemID), formatTime(now), id)
	if err != nil {
		return Proposal{}, err
	}
	return r.GetProposal(ctx, id)
}

func (r *Repository) requireWork(ctx context.Context, workID string) error {
	if workID == "" {
		return fmt.Errorf("%w: work id is required", ErrInvalidInput)
	}
	if r.workRepo == nil {
		return nil
	}
	if _, err := r.workRepo.GetWork(ctx, workID); err != nil {
		if errors.Is(err, work.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func (r *Repository) conn() *sql.DB {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Conn()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanItem(row scanner) (Item, error) {
	var item Item
	var createdAt, updatedAt string
	if err := row.Scan(&item.ID, &item.WorkID, &item.Kind, &item.Title, &item.Body, &item.Status, &item.Importance, &createdAt, &updatedAt); err != nil {
		return Item{}, err
	}
	var err error
	item.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return Item{}, err
	}
	item.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return Item{}, err
	}
	return item, nil
}

func scanDecision(row scanner) (Decision, error) {
	var decision Decision
	var createdAt string
	if err := row.Scan(&decision.ID, &decision.WorkID, &decision.CanonItemID, &decision.Type, &decision.Reason, &decision.Actor, &createdAt); err != nil {
		return Decision{}, err
	}
	var err error
	decision.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return Decision{}, err
	}
	return decision, nil
}

func scanProposal(row scanner) (Proposal, error) {
	var proposal Proposal
	var createdAt string
	var decidedAt sql.NullString
	if err := row.Scan(
		&proposal.ID,
		&proposal.WorkID,
		&proposal.EpisodeID,
		&proposal.RunID,
		&proposal.TargetItemID,
		&proposal.ChangeType,
		&proposal.Kind,
		&proposal.Title,
		&proposal.BeforeBody,
		&proposal.AfterBody,
		&proposal.Reason,
		&proposal.Confidence,
		&proposal.Status,
		&createdAt,
		&decidedAt,
	); err != nil {
		return Proposal{}, err
	}
	var err error
	proposal.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return Proposal{}, err
	}
	if decidedAt.Valid {
		parsed, err := parseTime(decidedAt.String)
		if err != nil {
			return Proposal{}, err
		}
		proposal.DecidedAt = &parsed
	}
	return proposal, nil
}

func scanIssue(row scanner) (ContinuityIssue, error) {
	var issue ContinuityIssue
	var createdAt, updatedAt string
	if err := row.Scan(
		&issue.ID,
		&issue.WorkID,
		&issue.EpisodeID,
		&issue.RunID,
		&issue.Severity,
		&issue.Title,
		&issue.Body,
		&issue.RelatedItemIDs,
		&issue.Status,
		&createdAt,
		&updatedAt,
	); err != nil {
		return ContinuityIssue{}, err
	}
	var err error
	issue.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return ContinuityIssue{}, err
	}
	issue.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return ContinuityIssue{}, err
	}
	return issue, nil
}

func normalizeCreateInput(input CreateItemInput) CreateItemInput {
	input.WorkID = strings.TrimSpace(input.WorkID)
	input.Title = strings.TrimSpace(input.Title)
	input.Body = strings.TrimSpace(input.Body)
	input.Reason = strings.TrimSpace(input.Reason)
	input.Actor = normalizeActor(input.Actor)
	if input.Status == "" {
		input.Status = StatusDraft
	}
	if input.Importance == "" {
		input.Importance = ImportanceMedium
	}
	return input
}

func normalizeUpdateInput(input UpdateItemInput, current Item) UpdateItemInput {
	input.Title = strings.TrimSpace(input.Title)
	input.Body = strings.TrimSpace(input.Body)
	input.Reason = strings.TrimSpace(input.Reason)
	input.Actor = normalizeActor(input.Actor)
	if input.Title == "" {
		input.Title = current.Title
	}
	if input.Status == "" {
		input.Status = current.Status
	}
	if input.Importance == "" {
		input.Importance = current.Importance
	}
	return input
}

func normalizeActor(actor string) string {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return "human"
	}
	return actor
}

func normalizeProposalInput(input CreateProposalInput) CreateProposalInput {
	input.WorkID = strings.TrimSpace(input.WorkID)
	input.EpisodeID = strings.TrimSpace(input.EpisodeID)
	input.RunID = strings.TrimSpace(input.RunID)
	input.TargetItemID = strings.TrimSpace(input.TargetItemID)
	input.Title = strings.TrimSpace(input.Title)
	input.BeforeBody = strings.TrimSpace(input.BeforeBody)
	input.AfterBody = strings.TrimSpace(input.AfterBody)
	input.Reason = strings.TrimSpace(input.Reason)
	return input
}

func normalizeIssueInput(input CreateIssueInput) CreateIssueInput {
	input.WorkID = strings.TrimSpace(input.WorkID)
	input.EpisodeID = strings.TrimSpace(input.EpisodeID)
	input.RunID = strings.TrimSpace(input.RunID)
	input.Title = strings.TrimSpace(input.Title)
	input.Body = strings.TrimSpace(input.Body)
	input.RelatedItemIDs = strings.TrimSpace(input.RelatedItemIDs)
	if input.Severity == "" {
		input.Severity = IssueInfo
	}
	return input
}

func newID(prefix string) string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(buf[:])
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}
