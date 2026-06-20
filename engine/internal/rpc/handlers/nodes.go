package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/devlikebear/linetta/engine/internal/node"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

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

// UpdateNodeContent returns a handler for nodes.update_content. Version
// snapshots are no longer created here; autosave checkpoints are idle-triggered
// from the renderer via snapshots.create_auto.
func UpdateNodeContent(nodes *node.Repo, now Clock, postUpdate func(nodeID string)) rpc.Handler {
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
			if errors.Is(err, node.ErrContentOnContainer) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}

		got, err := nodes.Get(ctx, p.ID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if postUpdate != nil {
			postUpdate(p.ID)
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
			if errors.Is(err, node.ErrNodeProjectMismatch) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
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
			if errors.Is(err, node.ErrInvalidKind) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
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
			if errors.Is(err, node.ErrInvalidKind) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(n)
	}
}

type moveToParentParams struct {
	ID       string `json:"id"`
	ParentID string `json:"parent_id"`
}

type moveToParams struct {
	ID       string  `json:"id"`
	ParentID *string `json:"parent_id"`
	Ordinal  int     `json:"ordinal"`
}

func MoveTo(nodes *node.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p moveToParams
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := nodes.MoveTo(ctx, p.ID, p.ParentID, p.Ordinal, now()); err != nil {
			if errors.Is(err, node.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node or parent not found"}
			}
			if errors.Is(err, node.ErrNodeProjectMismatch) ||
				errors.Is(err, node.ErrContentOnContainer) ||
				errors.Is(err, node.ErrInvalidMove) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}

func MoveToParent(nodes *node.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p moveToParentParams
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" || p.ParentID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id and parent_id required"}
		}
		if err := nodes.MoveToParent(ctx, p.ID, p.ParentID, now()); err != nil {
			if errors.Is(err, node.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node or parent not found"}
			}
			if errors.Is(err, node.ErrNodeProjectMismatch) ||
				errors.Is(err, node.ErrContentOnContainer) ||
				errors.Is(err, node.ErrInvalidMove) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}

func MoveToRoot(nodes *node.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := nodes.MoveToRoot(ctx, p.ID, now()); err != nil {
			if errors.Is(err, node.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}

func ConvertToContainer(nodes *node.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := nodes.ConvertLeafToContainer(ctx, p.ID, now()); err != nil {
			if errors.Is(err, node.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node not found"}
			}
			if errors.Is(err, node.ErrInvalidMove) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}

type restoreOutlineParams struct {
	ProjectID string      `json:"project_id"`
	Nodes     []node.Node `json:"nodes"`
}

func RestoreOutline(nodes *node.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p restoreOutlineParams
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" || len(p.Nodes) == 0 {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id and nodes required"}
		}
		if err := nodes.RestoreOutline(ctx, p.ProjectID, p.Nodes, now()); err != nil {
			if errors.Is(err, node.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project not found"}
			}
			if errors.Is(err, node.ErrInvalidMove) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}

type renameParams struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Title string `json:"title"`
}

type setStatusParams struct {
	ID     string `json:"id"`
	Status string `json:"status"`
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

func SetNodeStatus(nodes *node.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p setStatusParams
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" || p.Status == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id and status required"}
		}
		if err := nodes.SetStatus(ctx, p.ID, p.Status, now()); err != nil {
			if errors.Is(err, node.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node not found"}
			}
			if errors.Is(err, node.ErrInvalidStatus) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
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
