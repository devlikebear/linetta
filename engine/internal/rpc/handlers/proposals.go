package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/proposals"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/storyops"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

// proposals.* serve story-assist Phase 3's suggestions inbox: an agent's
// linetta_propose_story_ops candidates, and the writer's item-by-item
// accept/reject of them. Nothing here applies anything on its own —
// proposals.accept is the one path that does, by running the chosen items
// through the same storyops applier every other write goes through.

// proposalEvidenceOut adds the cited scene's breadcrumb to one evidence
// entry, so the screen never makes a second round trip per citation —
// continuity.go's continuityRefOut follows the same shape for the same
// reason.
type proposalEvidenceOut struct {
	proposals.EvidenceInput
	Breadcrumb string `json:"breadcrumb,omitempty"`
}

// proposalItemOut is one stored item plus what the screen needs to render
// a row without resolving anything itself: a human summary, cited
// evidence with breadcrumbs, and whether a dependency was rejected.
type proposalItemOut struct {
	ID                string                `json:"id"`
	ProposalID        string                `json:"proposal_id"`
	Ordinal           int                   `json:"ordinal"`
	OpType            string                `json:"op_type"`
	Op                storyops.Op           `json:"op"`
	Summary           string                `json:"summary"`
	DependsOn         []string              `json:"depends_on"`
	BlockedByRejected bool                  `json:"blocked_by_rejected"`
	Evidence          []proposalEvidenceOut `json:"evidence"`
	FromNodeIDs       []string              `json:"from_node_ids"`
	Duplicates        []proposals.Duplicate `json:"duplicates"`
	Status            string                `json:"status"`
	Result            proposals.ItemResult  `json:"result"`
	StatusAt          int64                 `json:"status_at"`
}

type proposalOut struct {
	ID        string            `json:"id"`
	ProjectID string            `json:"project_id"`
	Purpose   string            `json:"purpose"`
	Source    string            `json:"source"`
	Summary   string            `json:"summary"`
	CreatedAt int64             `json:"created_at"`
	Items     []proposalItemOut `json:"items"`
}

// ListProposals returns a handler for proposals.list.
func ListProposals(svc *proposals.Service, nodes *node.Repo, entities *entity.Repo, threads *thread.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string   `json:"project_id"`
			Statuses  []string `json:"statuses,omitempty"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		if svc == nil {
			return json.Marshal(struct {
				Proposals []proposalOut `json:"proposals"`
			}{[]proposalOut{}})
		}
		list, err := svc.List(ctx, in.ProjectID, in.Statuses)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}

		byNodeID := map[string]node.Node{}
		if nodes != nil {
			all, err := nodes.ListByProject(ctx, in.ProjectID)
			if err == nil {
				for _, n := range all {
					byNodeID[n.ID] = n
				}
			}
		}

		out := make([]proposalOut, 0, len(list))
		for _, p := range list {
			statusByID := map[string]string{}
			for _, it := range p.Items {
				statusByID[it.ID] = it.Status
			}
			items := make([]proposalItemOut, 0, len(p.Items))
			for _, it := range p.Items {
				blocked := false
				for _, dep := range it.DependsOn {
					if statusByID[dep] == proposals.StatusRejected {
						blocked = true
					}
				}
				evidence := make([]proposalEvidenceOut, 0, len(it.Evidence))
				for _, ev := range it.Evidence {
					breadcrumb := ""
					if n, ok := byNodeID[ev.NodeID]; ok {
						breadcrumb = node.BreadcrumbLabel(byNodeID, n)
					}
					evidence = append(evidence, proposalEvidenceOut{EvidenceInput: ev, Breadcrumb: breadcrumb})
				}
				items = append(items, proposalItemOut{
					ID: it.ID, ProposalID: it.ProposalID, Ordinal: it.Ordinal,
					OpType: it.Op.Type, Op: it.Op,
					Summary:           summarizeProposalOp(ctx, it.Op, p.Items, entities, threads),
					DependsOn:         it.DependsOn,
					BlockedByRejected: blocked,
					Evidence:          evidence,
					FromNodeIDs:       it.FromNodeIDs,
					Duplicates:        it.Duplicates,
					Status:            it.Status,
					Result:            it.Result,
					StatusAt:          it.StatusAt,
				})
			}
			out = append(out, proposalOut{
				ID: p.ID, ProjectID: p.ProjectID, Purpose: p.Purpose, Source: p.Source,
				Summary: p.Summary, CreatedAt: p.CreatedAt, Items: items,
			})
		}
		return json.Marshal(struct {
			Proposals []proposalOut `json:"proposals"`
		}{out})
	}
}

// AcceptProposals returns a handler for proposals.accept.
func AcceptProposals(svc *proposals.Service, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string                 `json:"project_id"`
			ItemIDs   []string               `json:"item_ids"`
			Edits     map[string]storyops.Op `json:"edits,omitempty"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" || len(in.ItemIDs) == 0 {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		if svc == nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: "proposals are not available"}
		}
		res, err := svc.Accept(ctx, now(), in.ProjectID, in.ItemIDs, in.Edits)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(res)
	}
}

// RejectProposals returns a handler for proposals.reject.
func RejectProposals(svc *proposals.Service, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string   `json:"project_id"`
			ItemIDs   []string `json:"item_ids"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" || len(in.ItemIDs) == 0 {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		if svc == nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: "proposals are not available"}
		}
		if err := svc.Reject(ctx, now(), in.ProjectID, in.ItemIDs); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(struct {
			OK bool `json:"ok"`
		}{true})
	}
}

// ProposalsPendingCount returns a handler for proposals.pending_count —
// CanonPanel's "제안함 N" badge.
func ProposalsPendingCount(svc *proposals.Service) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		if svc == nil {
			return json.Marshal(struct {
				Count int `json:"count"`
			}{0})
		}
		n, err := svc.PendingCount(ctx, in.ProjectID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(struct {
			Count int `json:"count"`
		}{n})
	}
}

// ExtractionEstimate returns a handler for proposals.extraction_estimate —
// the Library import dialog's size/token preview before a sweep starts.
func ExtractionEstimate(svc *proposals.Service) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		if svc == nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: "proposals are not available"}
		}
		est, err := svc.ExtractionEstimate(ctx, in.ProjectID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(est)
	}
}

// ExtractionProgress returns a handler for proposals.extraction_progress —
// how far a purpose=import_extraction sweep has gotten, and which episode
// to continue from.
func ExtractionProgress(svc *proposals.Service) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if err := json.Unmarshal(params, &in); err != nil || strings.TrimSpace(in.ProjectID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: errIDRequired}
		}
		if svc == nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: "proposals are not available"}
		}
		progress, err := svc.ExtractionProgress(ctx, in.ProjectID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(progress)
	}
}

// summarizeProposalOp renders one op the way the inbox row's headline
// reads it ("새 인물: 하린 — 검술 사범"), resolving a referenced entity/
// thread's name either from the work (an update op's real id) or from a
// sibling create_entity/create_thread item in the SAME proposal (a ref that
// has not been accepted yet, so it has no real id to look up).
func summarizeProposalOp(ctx context.Context, op storyops.Op, siblings []proposals.Item, entities *entity.Repo, threads *thread.Repo) string {
	resolveEntity := func(idOrRef string) string {
		if idOrRef == "" {
			return ""
		}
		if entities != nil {
			if e, err := entities.Get(ctx, idOrRef); err == nil {
				return e.Name
			}
		}
		for _, sib := range siblings {
			if sib.Op.Type == "create_entity" && sib.Op.Ref == idOrRef {
				return sib.Op.Name
			}
		}
		return idOrRef
	}
	resolveThread := func(idOrRef string) string {
		if idOrRef == "" {
			return ""
		}
		if threads != nil {
			if th, err := threads.Get(ctx, idOrRef); err == nil {
				return th.Name
			}
		}
		for _, sib := range siblings {
			if sib.Op.Type == "create_thread" && sib.Op.Ref == idOrRef {
				return sib.Op.Name
			}
		}
		return idOrRef
	}

	switch op.Type {
	case "create_entity":
		if op.Role != "" {
			return fmt.Sprintf("새 %s: %s — %s", entityKindLabelKo(op.Kind), op.Name, op.Role)
		}
		return fmt.Sprintf("새 %s: %s", entityKindLabelKo(op.Kind), op.Name)
	case "update_entity":
		name := op.Name
		if name == "" {
			name = resolveEntity(op.EntityID)
		}
		return fmt.Sprintf("%s 정보 수정", name)
	case "create_relationship":
		from := op.From
		if from == "" {
			from = op.FromRef
		}
		to := op.To
		if to == "" {
			to = op.ToRef
		}
		return fmt.Sprintf("관계: %s → %s (%s)", resolveEntity(from), resolveEntity(to), op.Label)
	case "create_fact_card":
		return fmt.Sprintf("팩트: %s", op.Claim)
	case "create_thread":
		return fmt.Sprintf("새 스토리라인: %s", op.Name)
	case "update_thread":
		name := op.Name
		if name == "" {
			name = resolveThread(op.ThreadID)
		}
		return fmt.Sprintf("스토리라인 수정: %s", name)
	case "add_beat":
		return fmt.Sprintf("비트 추가: %s", op.Label)
	case "update_beat":
		return fmt.Sprintf("비트 수정: %s", op.Label)
	default:
		return op.Type
	}
}

func entityKindLabelKo(kind string) string {
	switch kind {
	case "place":
		return "장소"
	case "item":
		return "사물"
	case "concept":
		return "개념"
	default:
		return "인물"
	}
}
