package contextualedit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/fact"
	"github.com/devlikebear/linetta/engine/internal/manuscriptedit"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/relationship"
)

const (
	ChangeTypeRename  = "rename"
	ChangeTypeSetting = "setting"

	metadataKindEntityName = "entity_name"
	reviewKindFact         = "fact_review"
	reviewKindRelationship = "relationship_review"

	IssueRemainingOldTerm = "remaining_old_term"
	IssueMetadataStale    = "metadata_stale"
	IssueReviewNeeded     = "review_needed"
)

var (
	ErrInvalidInput = errors.New("invalid contextual edit input")
	ErrInvalidPlan  = errors.New("invalid contextual edit plan")
)

// langPick returns en when lang selects English, otherwise ko (the default).
func langPick(lang, ko, en string) string {
	if strings.HasPrefix(lang, "en") {
		return en
	}
	return ko
}

type ResolveTargetInput struct {
	ProjectID    string `json:"project_id"`
	EntityID     string `json:"entity_id,omitempty"`
	FactID       string `json:"fact_id,omitempty"`
	SelectedText string `json:"selected_text,omitempty"`
	Query        string `json:"query,omitempty"`
}

type Target struct {
	CanonicalName   string   `json:"canonical_name"`
	Aliases         []string `json:"aliases,omitempty"`
	Kind            string   `json:"kind"`
	EntityIDs       []string `json:"entity_ids,omitempty"`
	FactIDs         []string `json:"fact_ids,omitempty"`
	RelationshipIDs []string `json:"relationship_ids,omitempty"`
}

type ChangeInput struct {
	ProjectID    string   `json:"project_id"`
	EntityID     string   `json:"entity_id,omitempty"`
	FactID       string   `json:"fact_id,omitempty"`
	SelectedText string   `json:"selected_text,omitempty"`
	Query        string   `json:"query,omitempty"`
	Type         string   `json:"type"`
	OldTerms     []string `json:"old_terms,omitempty"`
	NewTerms     []string `json:"new_terms,omitempty"`
	ReviewOnly   bool     `json:"review_only,omitempty"`
	Language     string   `json:"language,omitempty"`
}

type MetadataCandidate struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	TargetID string `json:"target_id"`
	Label    string `json:"label"`
	Before   string `json:"before"`
	After    string `json:"after"`
	Selected bool   `json:"selected"`
}

type ReviewCandidate struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	TargetID string `json:"target_id"`
	Label    string `json:"label"`
	Snippet  string `json:"snippet"`
	Selected bool   `json:"selected"`
}

type ChangePlan struct {
	ID                 string                       `json:"id"`
	ProjectID          string                       `json:"project_id"`
	Target             Target                       `json:"target"`
	Type               string                       `json:"type"`
	OldTerms           []string                     `json:"old_terms"`
	NewTerms           []string                     `json:"new_terms"`
	MetadataCandidates []MetadataCandidate          `json:"metadata_candidates"`
	ManuscriptPlans    []manuscriptedit.ReplacePlan `json:"manuscript_plans"`
	ReviewCandidates   []ReviewCandidate            `json:"review_candidates"`
	Warnings           []string                     `json:"warnings,omitempty"`
}

type ApplySelection struct {
	MetadataCandidateIDs   []string            `json:"metadata_candidate_ids,omitempty"`
	ManuscriptCandidateIDs map[string][]string `json:"manuscript_candidate_ids,omitempty"`
}

type ApplyResult struct {
	MetadataApplied int                               `json:"metadata_applied"`
	Manuscript      manuscriptedit.ApplyReplaceResult `json:"manuscript"`
	Failures        []manuscriptedit.ApplyFailure     `json:"failures,omitempty"`
}

type ConsistencyInput struct {
	ProjectID        string   `json:"project_id"`
	OldTerms         []string `json:"old_terms"`
	NewTerms         []string `json:"new_terms,omitempty"`
	ChangedEntityIDs []string `json:"changed_entity_ids,omitempty"`
	Language         string   `json:"language,omitempty"`
}

type ConsistencyIssue struct {
	Severity   string `json:"severity"`
	Kind       string `json:"kind"`
	NodeID     string `json:"node_id,omitempty"`
	Breadcrumb string `json:"breadcrumb,omitempty"`
	Snippet    string `json:"snippet,omitempty"`
	Message    string `json:"message"`
}

type ConsistencyReport struct {
	OK     bool               `json:"ok"`
	Issues []ConsistencyIssue `json:"issues"`
}

type Service struct {
	entities   *entity.Repo
	facts      *fact.Repo
	rels       *relationship.Repo
	manuscript *manuscriptedit.Service
	nodes      *node.Repo
}

func NewService(
	entities *entity.Repo,
	facts *fact.Repo,
	rels *relationship.Repo,
	manuscript *manuscriptedit.Service,
	nodes *node.Repo,
) *Service {
	return &Service{
		entities:   entities,
		facts:      facts,
		rels:       rels,
		manuscript: manuscript,
		nodes:      nodes,
	}
}

func (s *Service) ResolveTarget(ctx context.Context, in ResolveTargetInput) (Target, error) {
	projectID := strings.TrimSpace(in.ProjectID)
	if s == nil || projectID == "" {
		return Target{}, fmt.Errorf("%w: project_id required", ErrInvalidInput)
	}
	if id := strings.TrimSpace(in.EntityID); id != "" {
		e, err := s.entities.Get(ctx, id)
		if err != nil {
			return Target{}, err
		}
		if e.ProjectID != projectID {
			return Target{}, fmt.Errorf("%w: entity does not belong to project", ErrInvalidInput)
		}
		target := Target{
			CanonicalName: e.Name,
			Aliases:       append([]string{}, e.Aliases...),
			Kind:          e.Kind,
			EntityIDs:     []string{e.ID},
		}
		if s.rels != nil {
			rels, err := s.rels.ListByProject(ctx, projectID)
			if err != nil {
				return Target{}, err
			}
			for _, rel := range rels {
				if rel.FromID == e.ID || rel.ToID == e.ID {
					target.RelationshipIDs = append(target.RelationshipIDs, rel.ID)
				}
			}
		}
		return target, nil
	}
	if id := strings.TrimSpace(in.FactID); id != "" {
		card, err := s.facts.Get(ctx, id)
		if err != nil {
			return Target{}, err
		}
		if card.ProjectID != projectID {
			return Target{}, fmt.Errorf("%w: fact does not belong to project", ErrInvalidInput)
		}
		return Target{CanonicalName: firstNonEmpty(card.Claim, card.Result), Kind: "fact", FactIDs: []string{card.ID}}, nil
	}
	text := firstNonEmpty(in.SelectedText, in.Query)
	if text == "" {
		return Target{}, fmt.Errorf("%w: target required", ErrInvalidInput)
	}
	return Target{CanonicalName: text, Kind: "free_text"}, nil
}

func (s *Service) PlanContextChange(ctx context.Context, in ChangeInput) (ChangePlan, error) {
	projectID := strings.TrimSpace(in.ProjectID)
	if s == nil || projectID == "" {
		return ChangePlan{}, fmt.Errorf("%w: project_id required", ErrInvalidInput)
	}
	changeType := strings.TrimSpace(in.Type)
	if changeType == "" {
		changeType = ChangeTypeRename
	}
	target, err := s.ResolveTarget(ctx, ResolveTargetInput{
		ProjectID:    projectID,
		EntityID:     in.EntityID,
		FactID:       in.FactID,
		SelectedText: in.SelectedText,
		Query:        in.Query,
	})
	if err != nil {
		return ChangePlan{}, err
	}
	oldTerms := normalizeTerms(in.OldTerms)
	if len(oldTerms) == 0 {
		oldTerms = targetTerms(target)
	}
	newTerms := normalizeTerms(in.NewTerms)
	if len(oldTerms) == 0 || (changeType == ChangeTypeRename && len(newTerms) == 0) {
		return ChangePlan{}, fmt.Errorf("%w: old_terms and new_terms required", ErrInvalidInput)
	}

	plan := ChangePlan{
		ID:                 planID(changeType, projectID, oldTerms, newTerms),
		ProjectID:          projectID,
		Target:             target,
		Type:               changeType,
		OldTerms:           oldTerms,
		NewTerms:           newTerms,
		MetadataCandidates: []MetadataCandidate{},
		ManuscriptPlans:    []manuscriptedit.ReplacePlan{},
		ReviewCandidates:   []ReviewCandidate{},
	}
	if !in.ReviewOnly && changeType == ChangeTypeRename && len(target.EntityIDs) > 0 && len(newTerms) > 0 {
		plan.MetadataCandidates = append(plan.MetadataCandidates, MetadataCandidate{
			ID:       "entity:" + target.EntityIDs[0] + ":name",
			Kind:     metadataKindEntityName,
			TargetID: target.EntityIDs[0],
			Label:    langPick(in.Language, "자료집 이름", "Dossier name"),
			Before:   target.CanonicalName,
			After:    newTerms[0],
			Selected: true,
		})
	}
	if s.manuscript != nil && len(newTerms) > 0 {
		for i, oldTerm := range oldTerms {
			replacement := replacementFor(i, newTerms)
			if replacement == "" || oldTerm == replacement {
				continue
			}
			rp, err := s.manuscript.PlanReplace(ctx, manuscriptedit.ReplacePlanRequest{
				ProjectID:   projectID,
				Query:       oldTerm,
				Replacement: replacement,
			})
			if err != nil {
				return ChangePlan{}, err
			}
			if len(rp.Candidates) > 0 {
				plan.ManuscriptPlans = append(plan.ManuscriptPlans, rp)
			}
		}
	}
	review, err := s.reviewCandidates(ctx, projectID, oldTerms)
	if err != nil {
		return ChangePlan{}, err
	}
	plan.ReviewCandidates = review
	if len(plan.MetadataCandidates) == 0 && len(plan.ManuscriptPlans) == 0 && len(plan.ReviewCandidates) == 0 {
		plan.Warnings = append(plan.Warnings, langPick(in.Language, "변경 후보를 찾지 못했습니다.", "No change candidates were found."))
	}
	return plan, nil
}

func (s *Service) ApplyContextChange(ctx context.Context, plan ChangePlan, selection ApplySelection, now int64) (ApplyResult, error) {
	if s == nil || strings.TrimSpace(plan.ProjectID) == "" || len(plan.OldTerms) == 0 {
		return ApplyResult{}, fmt.Errorf("%w: plan required", ErrInvalidPlan)
	}
	result := ApplyResult{
		Manuscript: manuscriptedit.ApplyReplaceResult{Failures: []manuscriptedit.ApplyFailure{}, ChangedNodeIDs: []string{}},
		Failures:   []manuscriptedit.ApplyFailure{},
	}
	selectedMetadata := selectedSet(selection.MetadataCandidateIDs)
	for _, c := range plan.MetadataCandidates {
		if !selectedMetadata[c.ID] {
			continue
		}
		if err := s.applyMetadata(ctx, c, plan.ProjectID, now); err != nil {
			result.Failures = append(result.Failures, manuscriptedit.ApplyFailure{
				CandidateID: c.ID,
				Reason:      "metadata_update_failed",
				Message:     err.Error(),
			})
			continue
		}
		result.MetadataApplied++
	}
	for _, rp := range plan.ManuscriptPlans {
		ids := selection.ManuscriptCandidateIDs[rp.ID]
		if len(ids) == 0 {
			continue
		}
		applied, err := s.manuscript.ApplyReplace(ctx, rp, ids, now)
		if err != nil {
			return result, err
		}
		result.Manuscript.Applied += applied.Applied
		result.Manuscript.Skipped += applied.Skipped
		result.Manuscript.Failures = append(result.Manuscript.Failures, applied.Failures...)
		result.Manuscript.ChangedNodeIDs = append(result.Manuscript.ChangedNodeIDs, applied.ChangedNodeIDs...)
	}
	return result, nil
}

func (s *Service) CheckAfterChange(ctx context.Context, in ConsistencyInput) (ConsistencyReport, error) {
	projectID := strings.TrimSpace(in.ProjectID)
	oldTerms := normalizeTerms(in.OldTerms)
	if s == nil || projectID == "" || len(oldTerms) == 0 {
		return ConsistencyReport{}, fmt.Errorf("%w: project_id and old_terms required", ErrInvalidInput)
	}
	report := ConsistencyReport{OK: true, Issues: []ConsistencyIssue{}}
	nodes, err := s.nodes.ListByProject(ctx, projectID)
	if err != nil {
		return report, err
	}
	byID := map[string]node.Node{}
	for _, n := range nodes {
		byID[n.ID] = n
	}
	for _, n := range nodes {
		if n.Kind != node.KindLeaf || n.ContentDoc == nil {
			continue
		}
		plain := plainTextFromDoc(*n.ContentDoc)
		if term, ok := containsAny(plain, oldTerms); ok {
			report.Issues = append(report.Issues, ConsistencyIssue{
				Severity:   "warning",
				Kind:       IssueRemainingOldTerm,
				NodeID:     n.ID,
				Breadcrumb: node.BreadcrumbLabel(byID, n),
				Snippet:    snippetAround(plain, term),
				Message:    langPick(in.Language, "원고에 이전 표현이 남아 있습니다.", "The old term still appears in the manuscript."),
			})
		}
	}
	entities, err := s.entities.ListByProject(ctx, projectID)
	if err != nil {
		return report, err
	}
	for _, e := range entities {
		text := strings.Join([]string{e.Name, strings.Join(e.Aliases, " "), e.Role, e.Summary, attrsText(e.Attributes)}, " ")
		if term, ok := containsAny(text, oldTerms); ok {
			report.Issues = append(report.Issues, ConsistencyIssue{
				Severity: "warning",
				Kind:     IssueMetadataStale,
				Snippet:  snippetAround(text, term),
				Message:  langPick(in.Language, "자료집 항목에 이전 표현이 남아 있습니다.", "The old term still appears in a dossier entry."),
			})
		}
	}
	facts, err := s.facts.List(ctx, fact.ListFilter{ProjectID: projectID, Limit: 100})
	if err != nil {
		return report, err
	}
	for _, card := range facts {
		text := card.Claim + " " + card.Result
		if term, ok := containsAny(text, oldTerms); ok {
			report.Issues = append(report.Issues, ConsistencyIssue{
				Severity: "info",
				Kind:     IssueReviewNeeded,
				Snippet:  snippetAround(text, term),
				Message:  langPick(in.Language, "팩트 카드가 변경 후에도 맞는지 확인이 필요합니다.", "Check whether this fact card is still correct after the change."),
			})
		}
	}
	report.OK = len(report.Issues) == 0
	return report, nil
}

func (s *Service) applyMetadata(ctx context.Context, c MetadataCandidate, projectID string, now int64) error {
	if c.Kind != metadataKindEntityName {
		return nil
	}
	e, err := s.entities.Get(ctx, c.TargetID)
	if err != nil {
		return err
	}
	if e.ProjectID != projectID {
		return fmt.Errorf("%w: metadata candidate belongs to another project", ErrInvalidPlan)
	}
	attrs := e.Attributes
	return s.entities.Update(ctx, now, entity.UpdateInput{
		ID:         e.ID,
		Kind:       e.Kind,
		Name:       c.After,
		Role:       e.Role,
		Summary:    e.Summary,
		Attributes: &attrs,
	})
}

func (s *Service) reviewCandidates(ctx context.Context, projectID string, terms []string) ([]ReviewCandidate, error) {
	out := []ReviewCandidate{}
	if s.facts != nil {
		cards, err := s.facts.List(ctx, fact.ListFilter{ProjectID: projectID, Limit: 100})
		if err != nil {
			return nil, err
		}
		for _, card := range cards {
			text := strings.TrimSpace(card.Claim + " " + card.Result)
			if term, ok := containsAny(text, terms); ok {
				out = append(out, ReviewCandidate{
					ID:       "fact:" + card.ID,
					Kind:     reviewKindFact,
					TargetID: card.ID,
					Label:    card.Claim,
					Snippet:  snippetAround(text, term),
					Selected: false,
				})
			}
		}
	}
	if s.rels != nil {
		rels, err := s.rels.ListByProject(ctx, projectID)
		if err != nil {
			return nil, err
		}
		for _, rel := range rels {
			text := strings.TrimSpace(rel.Label + " " + rel.Notes)
			if term, ok := containsAny(text, terms); ok {
				out = append(out, ReviewCandidate{
					ID:       "relationship:" + rel.ID,
					Kind:     reviewKindRelationship,
					TargetID: rel.ID,
					Label:    rel.Label,
					Snippet:  snippetAround(text, term),
					Selected: false,
				})
			}
		}
	}
	return out, nil
}

func targetTerms(target Target) []string {
	terms := []string{}
	if target.CanonicalName != "" {
		terms = append(terms, target.CanonicalName)
	}
	terms = append(terms, target.Aliases...)
	return normalizeTerms(terms)
}

func normalizeTerms(terms []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" || seen[term] {
			continue
		}
		seen[term] = true
		out = append(out, term)
	}
	return out
}

func selectedSet(ids []string) map[string]bool {
	out := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			out[id] = true
		}
	}
	return out
}

func replacementFor(i int, terms []string) string {
	if len(terms) == 0 {
		return ""
	}
	if i < len(terms) {
		return terms[i]
	}
	return terms[0]
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

func planID(changeType, projectID string, oldTerms, newTerms []string) string {
	return "context:" + changeType + ":" + projectID + ":" + strings.Join(oldTerms, "|") + "->" + strings.Join(newTerms, "|")
}

func containsAny(text string, terms []string) (string, bool) {
	for _, term := range terms {
		if term != "" && strings.Contains(text, term) {
			return term, true
		}
	}
	return "", false
}

func snippetAround(text, term string) string {
	text = strings.TrimSpace(text)
	if text == "" || term == "" {
		return text
	}
	idx := strings.Index(text, term)
	if idx < 0 {
		if len([]rune(text)) <= 140 {
			return text
		}
		return string([]rune(text)[:140])
	}
	runes := []rune(text)
	termRunes := []rune(term)
	startRune := len([]rune(text[:idx])) - 35
	if startRune < 0 {
		startRune = 0
	}
	endRune := len([]rune(text[:idx])) + len(termRunes) + 70
	if endRune > len(runes) {
		endRune = len(runes)
	}
	return strings.TrimSpace(string(runes[startRune:endRune]))
}

func attrsText(attrs map[string]string) string {
	parts := []string{}
	for k, v := range attrs {
		parts = append(parts, k, v)
	}
	return strings.Join(parts, " ")
}

func plainTextFromDoc(raw string) string {
	var doc any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return raw
	}
	var sb strings.Builder
	writePlain(doc, &sb)
	return sb.String()
}

func writePlain(v any, sb *strings.Builder) {
	switch t := v.(type) {
	case map[string]any:
		kind, _ := t["type"].(string)
		if kind == "mention" {
			attrs, _ := t["attrs"].(map[string]any)
			if label, _ := attrs["label"].(string); label != "" {
				sb.WriteString("@")
				sb.WriteString(label)
			}
			return
		}
		if kind == "text" {
			if text, ok := t["text"].(string); ok {
				sb.WriteString(text)
			}
			return
		}
		if content, ok := t["content"].([]any); ok {
			for _, child := range content {
				writePlain(child, sb)
			}
		}
		if kind == "paragraph" || kind == "heading" || kind == "blockquote" {
			sb.WriteString("\n")
		}
	case []any:
		for _, child := range t {
			writePlain(child, sb)
		}
	}
}
