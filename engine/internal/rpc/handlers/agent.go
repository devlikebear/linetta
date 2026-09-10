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
	// Undo reverts a structural batch (batchID) or restores a scene's
	// pre-write snapshot (snapshotID). Exactly one is non-empty; AgentUndo
	// validates that before calling this (#112).
	Undo(ctx context.Context, batchID, snapshotID string) error
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
	// Exactly one. A batch id undoes an outline batch from
	// linetta_apply_story_ops; a snapshot id restores a scene write or
	// revise's pre-write text — mirrors mcphost's undoInput (tools_batch.go).
	BatchID    string `json:"batch_id,omitempty"`
	SnapshotID string `json:"snapshot_id,omitempty"`
}

// AgentUndo returns a handler for agent.undo: the panel's revert button.
func AgentUndo(ctrl AgentController) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p agentUndoParams
		if len(params) > 0 {
			_ = json.Unmarshal(params, &p)
		}
		batchID := strings.TrimSpace(p.BatchID)
		snapshotID := strings.TrimSpace(p.SnapshotID)
		switch {
		case batchID == "" && snapshotID == "":
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "pass batch_id or snapshot_id"}
		case batchID != "" && snapshotID != "":
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "pass either batch_id or snapshot_id, not both"}
		}
		if err := ctrl.Undo(ctx, batchID, snapshotID); err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}
