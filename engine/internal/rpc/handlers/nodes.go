package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/snapshot"
)

// AutosaveIntervalMillis controls how often UpdateNodeContent inserts a fresh
// autosave snapshot. Exposed for tests; production passes time.Now().UnixMilli.
const AutosaveIntervalMillis int64 = 60_000

type updateContentParams struct {
	ID  string `json:"id"`
	Doc string `json:"doc"`
}

// GetNode returns a handler for nodes.get.
func GetNode(nodes *node.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		n, err := nodes.Get(ctx, p.ID)
		if errors.Is(err, node.ErrNotFound) {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node not found"}
		}
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(n)
	}
}

// UpdateNodeContent returns a handler for nodes.update_content. After saving,
// if more than AutosaveIntervalMillis have elapsed since the last autosave for
// this node, a fresh autosave snapshot is inserted.
func UpdateNodeContent(nodes *node.Repo, snaps *snapshot.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p updateContentParams
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id and doc required"}
		}
		t := now()
		if err := nodes.UpdateContent(ctx, p.ID, p.Doc, t); err != nil {
			if errors.Is(err, node.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}

		// Maybe-snapshot.
		last, ok, err := snaps.LatestAutosaveTime(ctx, p.ID)
		if err == nil && (!ok || t-last >= AutosaveIntervalMillis) {
			_, _ = snaps.Create(ctx, p.ID, p.Doc, snapshot.ReasonAutosave, t)
		}

		got, err := nodes.Get(ctx, p.ID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(got)
	}
}

type setLastOpenedParams struct {
	ProjectID string `json:"project_id"`
	NodeID    string `json:"node_id"`
}

// SetLastOpened returns a handler for nodes.set_last_opened.
func SetLastOpened(nodes *node.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p setLastOpenedParams
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" || p.NodeID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id and node_id required"}
		}
		if err := nodes.SetLastOpened(ctx, p.ProjectID, p.NodeID, now()); err != nil {
			if errors.Is(err, node.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}

// --- Tree ops ---

type projectIDParam struct {
	ProjectID string `json:"project_id"`
}

func ListTree(nodes *node.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p projectIDParam
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id required"}
		}
		list, err := nodes.ListByProject(ctx, p.ProjectID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if list == nil {
			list = []node.Node{}
		}
		return json.Marshal(list)
	}
}

type createSiblingParams struct {
	ReferenceID string `json:"reference_id"`
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	Title       string `json:"title"`
}

func CreateSibling(nodes *node.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p createSiblingParams
		if err := json.Unmarshal(params, &p); err != nil || p.ReferenceID == "" || p.Kind == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "reference_id and kind required"}
		}
		n, err := nodes.CreateSibling(ctx, p.ReferenceID, p.Kind, p.Label, p.Title, now())
		if err != nil {
			if errors.Is(err, node.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "reference not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(n)
	}
}

type createChildParams struct {
	ParentID string `json:"parent_id"`
	Kind     string `json:"kind"`
	Label    string `json:"label"`
	Title    string `json:"title"`
}

func CreateChild(nodes *node.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p createChildParams
		if err := json.Unmarshal(params, &p); err != nil || p.ParentID == "" || p.Kind == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "parent_id and kind required"}
		}
		n, err := nodes.CreateChild(ctx, p.ParentID, p.Kind, p.Label, p.Title, now())
		if err != nil {
			if errors.Is(err, node.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "parent not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(n)
	}
}

type renameParams struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Title string `json:"title"`
}

func RenameNode(nodes *node.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p renameParams
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := nodes.Rename(ctx, p.ID, p.Label, p.Title, now()); err != nil {
			if errors.Is(err, node.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}

func DeleteNode(nodes *node.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := nodes.Delete(ctx, p.ID, now()); err != nil {
			if errors.Is(err, node.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}

func MoveUp(nodes *node.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := nodes.MoveUp(ctx, p.ID, now()); err != nil {
			if errors.Is(err, node.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}

func MoveDown(nodes *node.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := nodes.MoveDown(ctx, p.ID, now()); err != nil {
			if errors.Is(err, node.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}
