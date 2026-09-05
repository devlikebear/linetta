package handlers

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// AgentController is the slice of the built-in agent the RPC layer needs.
// An interface with plain types for two reasons: this file compiles on every
// build tag (the agent itself is //go:build !mobile), and handlers must never
// link tars/pkg/llm.
type AgentController interface {
	Run(ctx context.Context, projectID, nodeID, prompt string) (string, error)
	Cancel(ctx context.Context, runID string) error
	History(ctx context.Context, projectID string, limit int) (json.RawMessage, error)
	Clear(ctx context.Context, projectID string) error
	Undo(ctx context.Context, batchID string) error
}

type agentRunParams struct {
	ProjectID string `json:"project_id"`
	NodeID    string `json:"node_id,omitempty"`
	Prompt    string `json:"prompt"`
}

type agentRunResult struct {
	RunID string `json:"run_id"`
}

// AgentRun returns a handler for agent.run. It hands back a run id at once;
// the turn itself arrives as agent.* notifications.
func AgentRun(ctrl AgentController) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p agentRunParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "agent.run: " + err.Error()}
		}
		if strings.TrimSpace(p.Prompt) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "prompt is required"}
		}
		runID, err := ctrl.Run(ctx, p.ProjectID, p.NodeID, p.Prompt)
		if err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		return json.Marshal(agentRunResult{RunID: runID})
	}
}

type agentCancelParams struct {
	RunID string `json:"run_id"`
}

// AgentCancel returns a handler for agent.cancel. Cancelling a run that has
// already finished is not an error — the stop click can land late.
func AgentCancel(ctrl AgentController) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p agentCancelParams
		if len(params) > 0 {
			_ = json.Unmarshal(params, &p)
		}
		if err := ctrl.Cancel(ctx, p.RunID); err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}

type agentHistoryParams struct {
	ProjectID string `json:"project_id"`
	Limit     int    `json:"limit,omitempty"`
}

// AgentHistory returns a handler for agent.history.
func AgentHistory(ctrl AgentController) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p agentHistoryParams
		if len(params) > 0 {
			_ = json.Unmarshal(params, &p)
		}
		out, err := ctrl.History(ctx, p.ProjectID, p.Limit)
		if err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		return out, nil
	}
}

type agentClearParams struct {
	ProjectID string `json:"project_id"`
}

// AgentClear returns a handler for agent.clear. It drops the conversation;
// the activity log is a separate record and stays.
func AgentClear(ctrl AgentController) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p agentClearParams
		if len(params) > 0 {
			_ = json.Unmarshal(params, &p)
		}
		if err := ctrl.Clear(ctx, p.ProjectID); err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}

type agentUndoParams struct {
	BatchID string `json:"batch_id"`
}

// AgentUndo returns a handler for agent.undo: the panel's revert button.
func AgentUndo(ctrl AgentController) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p agentUndoParams
		if len(params) > 0 {
			_ = json.Unmarshal(params, &p)
		}
		if strings.TrimSpace(p.BatchID) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "batch_id is required"}
		}
		if err := ctrl.Undo(ctx, p.BatchID); err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}
