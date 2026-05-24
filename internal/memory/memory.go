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
