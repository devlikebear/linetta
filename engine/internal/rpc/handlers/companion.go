package handlers

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/ai"
	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

type companionSendParams struct {
	ProjectID string                      `json:"project_id"`
	NodeID    string                      `json:"node_id"`
	Text      string                      `json:"text"`
	Options   companion.SendOptions       `json:"options"`
	Images    []companion.ImageAttachment `json:"images"`
}

// CompanionSend returns a handler for companion.send.
func CompanionSend(svc *companion.Service, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p companionSendParams
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" || p.Text == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id and text required"}
		}
		runID, err := svc.SendWithCompanionOptionsAndImages(ctx, p.ProjectID, p.NodeID, p.Text, p.Options, p.Images, func() int64 { return now() })
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(map[string]string{"run_id": runID})
	}
}

type companionPreviewContextParams struct {
	ProjectID string     `json:"project_id"`
	NodeID    string     `json:"node_id"`
	Options   ai.Options `json:"options"`
}

// CompanionPreviewContext returns inspectable companion context sections before
// a turn starts, mirroring the AI context checklist behavior.
func CompanionPreviewContext(svc *companion.Service) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p companionPreviewContextParams
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id required"}
		}
		preview, err := svc.PreviewContext(ctx, p.ProjectID, p.NodeID, p.Options.Context)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(preview)
	}
}

type companionHistoryParams struct {
	ProjectID string `json:"project_id"`
	NodeID    string `json:"node_id"`
	Scope     string `json:"scope"`
	Limit     int    `json:"limit"`
}

type companionMessage struct {
	ID        string `json:"id,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	NodeID    string `json:"node_id,omitempty"`
	NodeLabel string `json:"node_label,omitempty"`
	RunID     string `json:"run_id,omitempty"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
	Scope     string `json:"scope,omitempty"`
	Intent    string `json:"intent,omitempty"`
	Status    string `json:"status,omitempty"`
}

// CompanionHistory returns a handler for companion.history.
func CompanionHistory(svc *companion.Service) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p companionHistoryParams
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id required"}
		}
		msgs, err := svc.HistoryView(ctx, companion.HistoryQuery{
			ProjectID: p.ProjectID,
			NodeID:    p.NodeID,
			Scope:     p.Scope,
			Limit:     p.Limit,
		})
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return marshalCompanionMessages(msgs)
	}
}

// CompanionCompact returns a handler for companion.compact.
func CompanionCompact(svc *companion.Service, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p companionHistoryParams
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id required"}
		}
		msgs, err := svc.CompactHistoryView(ctx, companion.HistoryQuery{
			ProjectID: p.ProjectID,
			NodeID:    p.NodeID,
			Scope:     p.Scope,
			Limit:     p.Limit,
		}, func() int64 { return now() })
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return marshalCompanionMessages(msgs)
	}
}

// CompanionClear returns a handler for companion.clear.
func CompanionClear(svc *companion.Service) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p companionHistoryParams
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id required"}
		}
		if err := svc.ClearHistoryView(ctx, companion.HistoryQuery{
			ProjectID: p.ProjectID,
			NodeID:    p.NodeID,
			Scope:     p.Scope,
		}); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}

func marshalCompanionMessages(msgs []companion.HistoryMessage) (json.RawMessage, error) {
	out := make([]companionMessage, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, companionMessage{
			ID:        m.ID,
			ProjectID: m.ProjectID,
			NodeID:    m.NodeID,
			NodeLabel: m.NodeLabel,
			RunID:     m.RunID,
			Role:      m.Role,
			Content:   m.Content,
			Timestamp: m.CreatedAt,
			Scope:     m.Scope,
			Intent:    m.Intent,
			Status:    m.Status,
		})
	}
	return json.Marshal(map[string][]companionMessage{"messages": out})
}

type companionCancelParams struct {
	RunID string `json:"run_id"`
}

// CompanionCancel returns a handler for companion.cancel.
func CompanionCancel(svc *companion.Service) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p companionCancelParams
		if err := json.Unmarshal(params, &p); err != nil || p.RunID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "run_id required"}
		}
		if err := svc.Cancel(p.RunID); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}

type companionRememberParams struct {
	ProjectID string `json:"project_id"`
	Text      string `json:"text"`
	Category  string `json:"category"`
}

// CompanionRemember returns a handler for companion.remember.
func CompanionRemember(svc *companion.Service) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p companionRememberParams
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" || p.Text == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id and text required"}
		}
		if err := svc.Remember(p.ProjectID, p.Text, p.Category); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}

type companionApplyOpsParams struct {
	ProjectID string         `json:"project_id"`
	NodeID    string         `json:"node_id"`
	Summary   string         `json:"summary"`
	Ops       []companion.Op `json:"ops"`
}

// CompanionApplyOps returns a handler for companion.apply_ops.
func CompanionApplyOps(svc *companion.Service, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p companionApplyOpsParams
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" || len(p.Ops) == 0 {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id and ops required"}
		}
		result := svc.ApplyOps(ctx, p.ProjectID, p.NodeID, companion.Proposal{
			Summary: p.Summary,
			Ops:     p.Ops,
		}, func() int64 { return now() })
		return json.Marshal(result)
	}
}

type companionReferencesListParams struct {
	ProjectID       string `json:"project_id"`
	NodeID          string `json:"node_id"`
	IncludeDisabled bool   `json:"include_disabled"`
	Limit           int    `json:"limit"`
}

// CompanionReferencesList returns user-managed companion references.
func CompanionReferencesList(svc *companion.Service) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p companionReferencesListParams
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id required"}
		}
		refs, err := svc.ListReferences(ctx, companion.ReferenceQuery{
			ProjectID:       p.ProjectID,
			NodeID:          p.NodeID,
			IncludeDisabled: p.IncludeDisabled,
			Limit:           p.Limit,
		})
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(map[string][]companion.Reference{"references": refs})
	}
}

type companionReferencesCreateParams struct {
	ProjectID  string `json:"project_id"`
	NodeID     string `json:"node_id"`
	SourceType string `json:"source_type"`
	Purpose    string `json:"purpose"`
	Title      string `json:"title"`
	Content    string `json:"content"`
	Summary    string `json:"summary"`
	Status     string `json:"status"`
}

// CompanionReferencesCreate persists one user-managed reference.
func CompanionReferencesCreate(svc *companion.Service, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p companionReferencesCreateParams
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" || p.Content == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id and content required"}
		}
		ref, err := svc.CreateReference(ctx, companion.ReferenceInput{
			ProjectID:  p.ProjectID,
			NodeID:     p.NodeID,
			SourceType: p.SourceType,
			Purpose:    p.Purpose,
			Title:      p.Title,
			Content:    p.Content,
			Summary:    p.Summary,
			Status:     p.Status,
		}, func() int64 { return now() })
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(ref)
	}
}

type companionReferencesUpdateParams struct {
	ProjectID  string  `json:"project_id"`
	ID         string  `json:"id"`
	NodeID     *string `json:"node_id"`
	SourceType *string `json:"source_type"`
	Purpose    *string `json:"purpose"`
	Title      *string `json:"title"`
	Content    *string `json:"content"`
	Summary    *string `json:"summary"`
	Status     *string `json:"status"`
}

// CompanionReferencesUpdate patches one user-managed reference.
func CompanionReferencesUpdate(svc *companion.Service, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p companionReferencesUpdateParams
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id and id required"}
		}
		ref, err := svc.UpdateReference(ctx, companion.ReferencePatch{
			ProjectID:  p.ProjectID,
			ID:         p.ID,
			NodeID:     p.NodeID,
			SourceType: p.SourceType,
			Purpose:    p.Purpose,
			Title:      p.Title,
			Content:    p.Content,
			Summary:    p.Summary,
			Status:     p.Status,
		}, func() int64 { return now() })
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(ref)
	}
}

type companionReferencesDeleteParams struct {
	ProjectID string `json:"project_id"`
	ID        string `json:"id"`
}

// CompanionReferencesDelete removes one user-managed reference.
func CompanionReferencesDelete(svc *companion.Service) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p companionReferencesDeleteParams
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id and id required"}
		}
		if err := svc.DeleteReference(ctx, p.ProjectID, p.ID); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}
