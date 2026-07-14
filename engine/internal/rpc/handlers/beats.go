package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/devlikebear/linetta/engine/internal/beat"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// CreateBeat returns a handler for beats.create.
func CreateBeat(repo *beat.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in beat.NewInput
		if err := json.Unmarshal(params, &in); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		if in.ThreadID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "thread_id required"}
		}
		b, err := repo.Create(ctx, in)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(b)
	}
}

type listBeatsByThreadParams struct {
	ThreadID string `json:"thread_id"`
}

// ListBeatsByThread returns a handler for beats.list_by_thread.
func ListBeatsByThread(repo *beat.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p listBeatsByThreadParams
		if err := json.Unmarshal(params, &p); err != nil || p.ThreadID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "thread_id required"}
		}
		list, err := repo.ListByThread(ctx, p.ThreadID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if list == nil {
			list = []beat.Beat{}
		}
		return json.Marshal(list)
	}
}

type listBeatsByNodeParams struct {
	NodeID string `json:"node_id"`
}

// ListBeatsByNode returns a handler for beats.list_by_node.
func ListBeatsByNode(repo *beat.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p listBeatsByNodeParams
		if err := json.Unmarshal(params, &p); err != nil || p.NodeID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "node_id required"}
		}
		list, err := repo.ListByNode(ctx, p.NodeID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if list == nil {
			list = []beat.Beat{}
		}
		return json.Marshal(list)
	}
}

// UpdateBeat returns a handler for beats.update.
func UpdateBeat(repo *beat.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in beat.UpdateInput
		if err := json.Unmarshal(params, &in); err != nil || in.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := repo.Update(ctx, in); err != nil {
			if errors.Is(err, beat.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "beat not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		got, err := repo.Get(ctx, in.ID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(got)
	}
}

type reorderBeatsParams struct {
	ThreadID string   `json:"thread_id"`
	IDs      []string `json:"ids"`
}

// ReorderBeats returns a handler for beats.reorder.
func ReorderBeats(repo *beat.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p reorderBeatsParams
		if err := json.Unmarshal(params, &p); err != nil || p.ThreadID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "thread_id and ids required"}
		}
		if err := repo.Reorder(ctx, p.ThreadID, p.IDs); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}

// DeleteBeat returns a handler for beats.delete.
func DeleteBeat(repo *beat.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := repo.Delete(ctx, p.ID); err != nil {
			if errors.Is(err, beat.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "beat not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}
