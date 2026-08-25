package handlers

import (
	"context"
	"encoding/json"
	"time"

	"github.com/devlikebear/linetta/engine/internal/entity"
	"github.com/devlikebear/linetta/engine/internal/export"
	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/project"
	"github.com/devlikebear/linetta/engine/internal/relationship"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

type exportProjectParams struct {
	ProjectID string `json:"project_id"`
}

// ExportProject returns a handler for export.project.
func ExportProject(pr *project.Repo, nr *node.Repo, er *entity.Repo, rr *relationship.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p exportProjectParams
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id required"}
		}
		out, err := export.ExportProject(ctx, pr, nr, er, rr, p.ProjectID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(out)
	}
}

// ExportCompanionHistory returns a handler for export.companion_history.
//
// The pivot retires the built-in companion, so the writer gets a way to keep
// the conversations before the removal lands. No parameters: this is an
// archive of everything, not a per-project view.
func ExportCompanionHistory(
	pr *project.Repo,
	history export.CompanionHistorySource,
	mem export.CompanionMemorySource,
) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		out, err := export.ExportCompanion(ctx, pr, history, mem, time.Now())
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(out)
	}
}

type exportNodeParams struct {
	NodeID string `json:"node_id"`
}

// ExportNode returns a handler for export.node.
func ExportNode(nr *node.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p exportNodeParams
		if err := json.Unmarshal(params, &p); err != nil || p.NodeID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node_id required"}
		}
		out, err := export.ExportNode(ctx, nr, p.NodeID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(out)
	}
}

// ExportNodeText returns platform-paste plain text for a leaf or subtree.
func ExportNodeText(nr *node.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p exportNodeParams
		if err := json.Unmarshal(params, &p); err != nil || p.NodeID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node_id required"}
		}
		out, err := export.ExportNodeText(ctx, nr, p.NodeID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(out)
	}
}
