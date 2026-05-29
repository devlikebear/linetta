package handlers

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/ai"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

type runAIParams struct {
	NodeID        string     `json:"node_id"`
	Prompt        string     `json:"prompt"`
	SelectionText string     `json:"selection_text"`
	Options       ai.Options `json:"options"`
}

// RunAI returns a handler for ai.run. It builds the Context via the supplied
// ContextBuilder and asks the Runner to start; returns the run id immediately.
func RunAI(builder *ai.ContextBuilder, runner *ai.Runner, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p runAIParams
		if err := json.Unmarshal(params, &p); err != nil || p.NodeID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node_id required"}
		}
		c, err := builder.Build(ctx, p.NodeID, p.Prompt, p.SelectionText, p.Options)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		runID, err := runner.Start(ctx, c, func() int64 { return now() })
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(map[string]string{"run_id": runID})
	}
}

type cancelAIParams struct {
	RunID string `json:"run_id"`
}

// CancelAI returns a handler for ai.cancel.
func CancelAI(runner *ai.Runner) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p cancelAIParams
		if err := json.Unmarshal(params, &p); err != nil || p.RunID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "run_id required"}
		}
		if err := runner.Cancel(p.RunID); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}
