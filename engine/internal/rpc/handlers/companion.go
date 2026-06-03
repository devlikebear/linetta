package handlers

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/tars/pkg/session"
)

type companionSendParams struct {
	ProjectID string `json:"project_id"`
	NodeID    string `json:"node_id"`
	Text      string `json:"text"`
}

// CompanionSend returns a handler for companion.send.
func CompanionSend(svc *companion.Service, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p companionSendParams
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" || p.Text == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id and text required"}
		}
		runID, err := svc.Send(ctx, p.ProjectID, p.NodeID, p.Text, func() int64 { return now() })
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(map[string]string{"run_id": runID})
	}
}

type companionHistoryParams struct {
	ProjectID string `json:"project_id"`
}

type companionMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

// CompanionHistory returns a handler for companion.history.
func CompanionHistory(svc *companion.Service) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p companionHistoryParams
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id required"}
		}
		msgs, err := svc.History(ctx, p.ProjectID)
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
		msgs, err := svc.CompactHistory(ctx, p.ProjectID, func() int64 { return now() })
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
		if err := svc.ClearHistory(ctx, p.ProjectID); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}

func marshalCompanionMessages(msgs []session.Message) (json.RawMessage, error) {
	out := make([]companionMessage, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, companionMessage{Role: m.Role, Content: m.Content, Timestamp: m.Timestamp.UnixMilli()})
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
