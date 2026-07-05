package companion

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/ai"
	"github.com/google/uuid"
)

const (
	ReferenceSourceText      = "text"
	ReferenceSourceClipboard = "clipboard"
	ReferenceSourceMarkdown  = "markdown"
	ReferenceSourceFile      = "file"

	ReferencePurposeStyle      = "style"
	ReferencePurposeContent    = "content"
	ReferencePurposeCanon      = "canon"
	ReferencePurposeConstraint = "constraint"

	ReferenceStatusActive     = "active"
	ReferenceStatusSummarized = "summarized"
	ReferenceStatusDisabled   = "disabled"
)

const (
	referenceAutoSummaryRunes = 4000
	referenceSummaryRunes     = 1600
	referencePromptRunes      = 2200
)

type Reference struct {
	ID            string `json:"id"`
	ProjectID     string `json:"project_id"`
	NodeID        string `json:"node_id,omitempty"`
	SourceType    string `json:"source_type"`
	Purpose       string `json:"purpose"`
	Title         string `json:"title"`
	Content       string `json:"content"`
	Summary       string `json:"summary"`
	CharCount     int    `json:"char_count"`
	TokenEstimate int    `json:"token_estimate"`
	Status        string `json:"status"`
	CreatedAt     int64  `json:"created_at"`
	UpdatedAt     int64  `json:"updated_at"`
}

type ReferenceInput struct {
	ProjectID  string
	NodeID     string
	SourceType string
	Purpose    string
	Title      string
	Content    string
	Summary    string
	Status     string
}

type ReferencePatch struct {
	ProjectID  string
	ID         string
	NodeID     *string
	SourceType *string
	Purpose    *string
	Title      *string
	Content    *string
	Summary    *string
	Status     *string
}

type ReferenceQuery struct {
	ProjectID       string
	NodeID          string
	IncludeDisabled bool
	ProjectWideOnly bool
	Limit           int
}

type ReferenceRepo struct {
	db *sql.DB
}

func NewReferenceRepo(db *sql.DB) *ReferenceRepo {
	return &ReferenceRepo{db: db}
}

func (r *ReferenceRepo) Create(ctx context.Context, input ReferenceInput, now int64) (Reference, error) {
	if r == nil {
		return Reference{}, errors.New("reference repo is nil")
	}
	ref := normalizeReference(Reference{
		ID:         uuid.NewString(),
		ProjectID:  input.ProjectID,
		NodeID:     input.NodeID,
		SourceType: input.SourceType,
		Purpose:    input.Purpose,
		Title:      input.Title,
		Content:    input.Content,
		Summary:    input.Summary,
		Status:     input.Status,
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if ref.ProjectID == "" || ref.Content == "" {
		return Reference{}, errors.New("project_id and content required")
	}
	if _, err := r.db.ExecContext(ctx, `
INSERT INTO companion_references
  (id, project_id, node_id, source_type, purpose, title, content, summary, char_count, token_estimate, status, created_at, updated_at)
VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ref.ID, ref.ProjectID, ref.NodeID, ref.SourceType, ref.Purpose, ref.Title, ref.Content, ref.Summary,
		ref.CharCount, ref.TokenEstimate, ref.Status, ref.CreatedAt, ref.UpdatedAt); err != nil {
		return Reference{}, fmt.Errorf("create companion reference: %w", err)
	}
	return ref, nil
}

func (r *ReferenceRepo) List(ctx context.Context, q ReferenceQuery) ([]Reference, error) {
	if r == nil {
		return nil, nil
	}
	projectID := strings.TrimSpace(q.ProjectID)
	if projectID == "" {
		return nil, errors.New("project_id required")
	}
	if q.Limit <= 0 {
		q.Limit = 40
	}
	args := []any{projectID}
	where := []string{"project_id = ?"}
	if strings.TrimSpace(q.NodeID) != "" {
		args = append(args, strings.TrimSpace(q.NodeID))
		where = append(where, "(node_id IS NULL OR node_id = ?)")
	} else if q.ProjectWideOnly {
		where = append(where, "node_id IS NULL")
	}
	if !q.IncludeDisabled {
		where = append(where, "status <> 'disabled'")
	}
	args = append(args, q.Limit)
	rows, err := r.db.QueryContext(ctx, `
SELECT id, project_id, COALESCE(node_id, ''), source_type, purpose, title, content, summary,
       char_count, token_estimate, status, created_at, updated_at
  FROM companion_references
 WHERE `+strings.Join(where, " AND ")+`
 ORDER BY updated_at DESC, id DESC
 LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Reference
	for rows.Next() {
		var ref Reference
		if err := rows.Scan(&ref.ID, &ref.ProjectID, &ref.NodeID, &ref.SourceType, &ref.Purpose, &ref.Title,
			&ref.Content, &ref.Summary, &ref.CharCount, &ref.TokenEstimate, &ref.Status, &ref.CreatedAt, &ref.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}

func (r *ReferenceRepo) Update(ctx context.Context, patch ReferencePatch, now int64) (Reference, error) {
	if r == nil {
		return Reference{}, errors.New("reference repo is nil")
	}
	current, err := r.Get(ctx, patch.ProjectID, patch.ID)
	if err != nil {
		return Reference{}, err
	}
	if patch.NodeID != nil {
		current.NodeID = *patch.NodeID
	}
	if patch.SourceType != nil {
		current.SourceType = *patch.SourceType
	}
	if patch.Purpose != nil {
		current.Purpose = *patch.Purpose
	}
	if patch.Title != nil {
		current.Title = *patch.Title
	}
	if patch.Content != nil {
		current.Content = *patch.Content
	}
	if patch.Summary != nil {
		current.Summary = *patch.Summary
	}
	if patch.Status != nil {
		current.Status = *patch.Status
	}
	current.UpdatedAt = now
	current = normalizeReference(current)
	if _, err := r.db.ExecContext(ctx, `
UPDATE companion_references
   SET node_id = NULLIF(?, ''), source_type = ?, purpose = ?, title = ?, content = ?,
       summary = ?, char_count = ?, token_estimate = ?, status = ?, updated_at = ?
 WHERE project_id = ? AND id = ?`,
		current.NodeID, current.SourceType, current.Purpose, current.Title, current.Content, current.Summary,
		current.CharCount, current.TokenEstimate, current.Status, current.UpdatedAt, current.ProjectID, current.ID); err != nil {
		return Reference{}, fmt.Errorf("update companion reference: %w", err)
	}
	return current, nil
}

func (r *ReferenceRepo) Get(ctx context.Context, projectID, id string) (Reference, error) {
	if r == nil {
		return Reference{}, errors.New("reference repo is nil")
	}
	var ref Reference
	err := r.db.QueryRowContext(ctx, `
SELECT id, project_id, COALESCE(node_id, ''), source_type, purpose, title, content, summary,
       char_count, token_estimate, status, created_at, updated_at
  FROM companion_references
 WHERE project_id = ? AND id = ?`, strings.TrimSpace(projectID), strings.TrimSpace(id)).
		Scan(&ref.ID, &ref.ProjectID, &ref.NodeID, &ref.SourceType, &ref.Purpose, &ref.Title,
			&ref.Content, &ref.Summary, &ref.CharCount, &ref.TokenEstimate, &ref.Status, &ref.CreatedAt, &ref.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Reference{}, errors.New("reference not found")
		}
		return Reference{}, err
	}
	return ref, nil
}

func (r *ReferenceRepo) Delete(ctx context.Context, projectID, id string) error {
	if r == nil {
		return nil
	}
	res, err := r.db.ExecContext(ctx, `DELETE FROM companion_references WHERE project_id = ? AND id = ?`,
		strings.TrimSpace(projectID), strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("reference not found")
	}
	return nil
}

func normalizeReference(ref Reference) Reference {
	ref.ProjectID = strings.TrimSpace(ref.ProjectID)
	ref.NodeID = strings.TrimSpace(ref.NodeID)
	ref.SourceType = normalizeReferenceSource(ref.SourceType)
	ref.Purpose = normalizeReferencePurpose(ref.Purpose)
	ref.Title = strings.TrimSpace(ref.Title)
	ref.Content = strings.TrimSpace(ref.Content)
	ref.Summary = strings.TrimSpace(ref.Summary)
	ref.Status = normalizeReferenceStatus(ref.Status)
	if ref.Title == "" {
		ref.Title = defaultReferenceTitle(ref.SourceType)
	}
	ref.CharCount = ai.EstimateChars(ref.Content)
	ref.TokenEstimate = ai.EstimateTokens(ref.Content)
	if ref.Summary == "" && ref.CharCount > referenceAutoSummaryRunes {
		ref.Summary = deterministicReferenceSummary(ref)
		if ref.Status == ReferenceStatusActive {
			ref.Status = ReferenceStatusSummarized
		}
	}
	if ref.Status == ReferenceStatusSummarized && ref.Summary == "" {
		ref.Summary = deterministicReferenceSummary(ref)
	}
	return ref
}

func normalizeReferenceSource(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case ReferenceSourceClipboard, ReferenceSourceMarkdown, ReferenceSourceFile:
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return ReferenceSourceText
	}
}

func normalizeReferencePurpose(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case ReferencePurposeStyle, ReferencePurposeCanon, ReferencePurposeConstraint:
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return ReferencePurposeContent
	}
}

func normalizeReferenceStatus(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case ReferenceStatusSummarized, ReferenceStatusDisabled:
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return ReferenceStatusActive
	}
}

func defaultReferenceTitle(source string) string {
	switch source {
	case ReferenceSourceClipboard:
		return "클립보드 레퍼런스"
	case ReferenceSourceMarkdown:
		return "마크다운 레퍼런스"
	case ReferenceSourceFile:
		return "파일 레퍼런스"
	default:
		return "텍스트 레퍼런스"
	}
}

func deterministicReferenceSummary(ref Reference) string {
	body := trimRunesLocal(strings.Join(strings.Fields(ref.Content), " "), referenceSummaryRunes)
	if body == "" {
		return ""
	}
	return fmt.Sprintf("%s · %s · 원문 %d자 중 발췌 요약\n%s",
		purposeLabel(ref.Purpose, ""), ref.Title, ref.CharCount, body)
}

func referencePromptText(ref Reference) string {
	if ref.Status == ReferenceStatusSummarized && strings.TrimSpace(ref.Summary) != "" {
		return trimRunesLocal(ref.Summary, referencePromptRunes)
	}
	if strings.TrimSpace(ref.Summary) != "" && ref.CharCount > referenceAutoSummaryRunes {
		return trimRunesLocal(ref.Summary, referencePromptRunes)
	}
	return trimRunesLocal(ref.Content, referencePromptRunes)
}
