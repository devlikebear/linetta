package handlers

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/companion"
	"github.com/devlikebear/linetta/engine/internal/rpc"
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
		out := make([]companionMessage, 0, len(msgs))
		for _, m := range msgs {
			out = append(out, companionMessage{Role: m.Role, Content: m.Content, Timestamp: m.Timestamp.UnixMilli()})
		}
		return json.Marshal(map[string][]companionMessage{"messages": out})
	}
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
