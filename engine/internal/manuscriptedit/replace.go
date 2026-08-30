package manuscriptedit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/snapshot"
)

var (
	ErrInvalidRequest = errors.New("invalid replace request")
	ErrInvalidPlan    = errors.New("invalid replace plan")
)

const (
	FailureVersionMismatch = "version_mismatch"
	FailureSnapshot        = "snapshot_failed"
	FailureUpdate          = "update_failed"
	FailureNotFound        = "candidate_not_found"
)

type ReplacePlanRequest struct {
	ProjectID     string   `json:"project_id"`
	Query         string   `json:"query"`
	Replacement   string   `json:"replacement"`
	NodeIDs       []string `json:"node_ids,omitempty"`
	MatchCase     bool     `json:"match_case,omitempty"`
	WholeWord     bool     `json:"whole_word,omitempty"`
	MaxCandidates int      `json:"max_candidates,omitempty"`
}

type ReplaceCandidate struct {
	ID             string `json:"id"`
	NodeID         string `json:"node_id"`
	Breadcrumb     string `json:"breadcrumb"`
	Before         string `json:"before"`
	After          string `json:"after"`
	Occurrences    int    `json:"occurrences"`
	Selected       bool   `json:"selected"`
	PreviewVersion int    `json:"preview_version"`
}

type ReplacePlan struct {
	ID            string             `json:"id,omitempty"`
	ProjectID     string             `json:"project_id"`
	Query         string             `json:"query"`
	Replacement   string             `json:"replacement"`
	Candidates    []ReplaceCandidate `json:"candidates"`
	GeneratedAt   int64              `json:"generated_at,omitempty"`
	MatchCase     bool               `json:"match_case,omitempty"`
	WholeWord     bool               `json:"whole_word,omitempty"`
	MaxCandidates int                `json:"max_candidates,omitempty"`
}

type ApplyFailure struct {
	CandidateID string `json:"candidate_id"`
	NodeID      string `json:"node_id,omitempty"`
	Breadcrumb  string `json:"breadcrumb,omitempty"`
	Reason      string `json:"reason"`
	Message     string `json:"message"`
}

type ApplyReplaceResult struct {
	Applied        int            `json:"applied"`
	Skipped        int            `json:"skipped"`
	Failures       []ApplyFailure `json:"failures"`
	ChangedNodeIDs []string       `json:"changed_node_ids"`
	// SnapshotIDs maps each changed node to the snapshot taken just before
	// its edit, so a caller (an MCP agent especially, #73) can restore any
	// one scene without hunting through version history.
	SnapshotIDs map[string]string `json:"snapshot_ids,omitempty"`
}

type nodeRepo interface {
	Get(ctx context.Context, id string) (node.Node, error)
	ListByProject(ctx context.Context, projectID string) ([]node.Node, error)
	UpdateContent(ctx context.Context, id string, doc string, now int64) error
}

type snapshotRepo interface {
	Create(ctx context.Context, nodeID, doc, reason string, now int64) (snapshot.Snapshot, error)
}

type Service struct {
	nodes nodeRepo
	snaps snapshotRepo
}

func NewService(nodes nodeRepo, snaps snapshotRepo) *Service {
	return &Service{nodes: nodes, snaps: snaps}
}

func (s *Service) PlanReplace(ctx context.Context, req ReplacePlanRequest) (ReplacePlan, error) {
	projectID := strings.TrimSpace(req.ProjectID)
	query := strings.TrimSpace(req.Query)
	if s == nil || s.nodes == nil || projectID == "" || query == "" || strings.TrimSpace(req.Replacement) == "" {
		return ReplacePlan{}, ErrInvalidRequest
	}
	if req.WholeWord {
		return ReplacePlan{}, fmt.Errorf("%w: whole_word is not supported yet", ErrInvalidRequest)
	}

	nodes, err := s.candidateNodes(ctx, projectID, req.NodeIDs)
	if err != nil {
		return ReplacePlan{}, err
	}
	byID := map[string]node.Node{}
	for _, n := range nodes {
		byID[n.ID] = n
	}

	plan := ReplacePlan{
		ID:            planID(query, req.Replacement),
		ProjectID:     projectID,
		Query:         query,
		Replacement:   req.Replacement,
		MatchCase:     req.MatchCase,
		WholeWord:     req.WholeWord,
		MaxCandidates: req.MaxCandidates,
		Candidates:    []ReplaceCandidate{},
	}
	for _, n := range nodes {
		if n.Kind != node.KindLeaf || n.ContentDoc == nil {
			continue
		}
		result, err := replaceDocText(*n.ContentDoc, query, req.Replacement, req.MatchCase)
		if err != nil || result.Occurrences == 0 {
			continue
		}
		c := ReplaceCandidate{
			ID:             candidateID(n.ID, n.ContentVersion),
			NodeID:         n.ID,
			Breadcrumb:     node.BreadcrumbLabel(byID, n),
			Before:         result.BeforePlain,
			After:          result.AfterPlain,
			Occurrences:    result.Occurrences,
			Selected:       true,
			PreviewVersion: n.ContentVersion,
		}
		plan.Candidates = append(plan.Candidates, c)
		if req.MaxCandidates > 0 && len(plan.Candidates) >= req.MaxCandidates {
			break
		}
	}
	return plan, nil
}

func (s *Service) ApplyReplace(ctx context.Context, plan ReplacePlan, candidateIDs []string, now int64) (ApplyReplaceResult, error) {
	if s == nil || s.nodes == nil || s.snaps == nil || strings.TrimSpace(plan.ProjectID) == "" ||
		strings.TrimSpace(plan.Query) == "" || strings.TrimSpace(plan.Replacement) == "" {
		return ApplyReplaceResult{}, ErrInvalidPlan
	}
	selected := map[string]bool{}
	for _, id := range candidateIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			selected[id] = true
		}
	}
	if len(selected) == 0 {
		return ApplyReplaceResult{Skipped: len(plan.Candidates), Failures: []ApplyFailure{}, ChangedNodeIDs: []string{}}, nil
	}

	result := ApplyReplaceResult{Failures: []ApplyFailure{}, ChangedNodeIDs: []string{}}
	for _, c := range plan.Candidates {
		if !selected[c.ID] {
			result.Skipped++
			continue
		}
		n, err := s.nodes.Get(ctx, c.NodeID)
		if err != nil {
			result.Failures = append(result.Failures, ApplyFailure{
				CandidateID: c.ID,
				NodeID:      c.NodeID,
				Breadcrumb:  c.Breadcrumb,
				Reason:      FailureNotFound,
				Message:     err.Error(),
			})
			continue
		}
		if n.ProjectID != plan.ProjectID {
			result.Failures = append(result.Failures, ApplyFailure{
				CandidateID: c.ID,
				NodeID:      c.NodeID,
				Breadcrumb:  c.Breadcrumb,
				Reason:      FailureNotFound,
				Message:     "node does not belong to replace plan project",
			})
			continue
		}
		if n.ContentVersion != c.PreviewVersion {
			result.Failures = append(result.Failures, ApplyFailure{
				CandidateID: c.ID,
				NodeID:      c.NodeID,
				Breadcrumb:  c.Breadcrumb,
				Reason:      FailureVersionMismatch,
				Message:     "scene changed after preview; refresh preview",
			})
			continue
		}
		if n.ContentDoc == nil {
			result.Failures = append(result.Failures, ApplyFailure{
				CandidateID: c.ID,
				NodeID:      c.NodeID,
				Breadcrumb:  c.Breadcrumb,
				Reason:      FailureNotFound,
				Message:     "scene has no content",
			})
			continue
		}
		replaced, err := replaceDocText(*n.ContentDoc, plan.Query, plan.Replacement, plan.MatchCase)
		if err != nil || replaced.Occurrences == 0 {
			msg := "candidate no longer matches"
			if err != nil {
				msg = err.Error()
			}
			result.Failures = append(result.Failures, ApplyFailure{
				CandidateID: c.ID,
				NodeID:      c.NodeID,
				Breadcrumb:  c.Breadcrumb,
				Reason:      FailureNotFound,
				Message:     msg,
			})
			continue
		}
		snap, err := s.snaps.Create(ctx, n.ID, *n.ContentDoc, snapshot.ReasonManual, now)
		if err != nil {
			result.Failures = append(result.Failures, ApplyFailure{
				CandidateID: c.ID,
				NodeID:      c.NodeID,
				Breadcrumb:  c.Breadcrumb,
				Reason:      FailureSnapshot,
				Message:     err.Error(),
			})
			continue
		}
		if err := s.nodes.UpdateContent(ctx, n.ID, replaced.Doc, now); err != nil {
			result.Failures = append(result.Failures, ApplyFailure{
				CandidateID: c.ID,
				NodeID:      c.NodeID,
				Breadcrumb:  c.Breadcrumb,
				Reason:      FailureUpdate,
				Message:     err.Error(),
			})
			continue
		}
		result.Applied++
		result.ChangedNodeIDs = append(result.ChangedNodeIDs, n.ID)
		if result.SnapshotIDs == nil {
			result.SnapshotIDs = map[string]string{}
		}
		result.SnapshotIDs[n.ID] = snap.ID
	}
	return result, nil
}

func (s *Service) candidateNodes(ctx context.Context, projectID string, nodeIDs []string) ([]node.Node, error) {
	if len(nodeIDs) == 0 {
		return s.nodes.ListByProject(ctx, projectID)
	}
	out := make([]node.Node, 0, len(nodeIDs))
	for _, id := range nodeIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		n, err := s.nodes.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		if n.ProjectID == projectID {
			out = append(out, n)
		}
	}
	return out, nil
}

func candidateID(nodeID string, contentVersion int) string {
	return nodeID + ":" + strconv.Itoa(contentVersion)
}

func planID(query string, replacement string) string {
	return "replace:" + strconv.QuoteToASCII(query) + "->" + strconv.QuoteToASCII(replacement)
}

type docReplaceResult struct {
	Doc         string
	BeforePlain string
	AfterPlain  string
	Occurrences int
}

func replaceDocText(raw string, query string, replacement string, matchCase bool) (docReplaceResult, error) {
	var doc any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return docReplaceResult{}, err
	}
	beforePlain := plainText(doc)
	occurrences := replaceTextNodes(doc, query, replacement, matchCase)
	if occurrences == 0 {
		return docReplaceResult{Doc: raw, BeforePlain: beforePlain, AfterPlain: beforePlain}, nil
	}
	afterPlain := plainText(doc)
	encoded, err := json.Marshal(doc)
	if err != nil {
		return docReplaceResult{}, err
	}
	return docReplaceResult{
		Doc:         string(encoded),
		BeforePlain: beforePlain,
		AfterPlain:  afterPlain,
		Occurrences: occurrences,
	}, nil
}

func replaceTextNodes(v any, query string, replacement string, matchCase bool) int {
	switch t := v.(type) {
	case map[string]any:
		kind, _ := t["type"].(string)
		if kind == "text" {
			text, _ := t["text"].(string)
			next, count := replaceText(text, query, replacement, matchCase)
			if count > 0 {
				t["text"] = next
			}
			return count
		}
		count := 0
		if content, ok := t["content"].([]any); ok {
			for _, child := range content {
				count += replaceTextNodes(child, query, replacement, matchCase)
			}
		}
		return count
	case []any:
		count := 0
		for _, child := range t {
			count += replaceTextNodes(child, query, replacement, matchCase)
		}
		return count
	default:
		return 0
	}
}

func replaceText(text string, query string, replacement string, matchCase bool) (string, int) {
	if query == "" {
		return text, 0
	}
	if matchCase {
		return strings.ReplaceAll(text, query, replacement), strings.Count(text, query)
	}
	lowerText := strings.ToLower(text)
	lowerQuery := strings.ToLower(query)
	var out strings.Builder
	count := 0
	cursor := 0
	for {
		at := strings.Index(lowerText[cursor:], lowerQuery)
		if at < 0 {
			out.WriteString(text[cursor:])
			break
		}
		at += cursor
		out.WriteString(text[cursor:at])
		out.WriteString(replacement)
		cursor = at + len(query)
		count++
	}
	return out.String(), count
}

func plainText(v any) string {
	var sb strings.Builder
	writePlain(v, &sb)
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
