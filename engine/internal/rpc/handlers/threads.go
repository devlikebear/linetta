package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/thread"
)

// CreateThread returns a handler for threads.create.
func CreateThread(repo *thread.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in thread.NewInput
		if err := json.Unmarshal(params, &in); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		if in.ProjectID == "" || in.Name == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id and name required"}
		}
		th, err := repo.Create(ctx, in)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(th)
	}
}

type listThreadsParams struct {
	ProjectID     string `json:"project_id"`
	IncludeClosed bool   `json:"include_closed"`
}

// ListThreads returns a handler for threads.list.
func ListThreads(repo *thread.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p listThreadsParams
		if err := json.Unmarshal(params, &p); err != nil || p.ProjectID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "project_id required"}
		}
		list, err := repo.ListByProject(ctx, p.ProjectID, p.IncludeClosed)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if list == nil {
			list = []thread.Thread{}
		}
		return json.Marshal(list)
	}
}

// GetThread returns a handler for threads.get.
func GetThread(repo *thread.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		th, err := repo.Get(ctx, p.ID)
		if errors.Is(err, thread.ErrNotFound) {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "thread not found"}
		}
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(th)
	}
}

// UpdateThread returns a handler for threads.update.
func UpdateThread(repo *thread.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in thread.UpdateInput
		if err := json.Unmarshal(params, &in); err != nil || in.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := repo.Update(ctx, in); err != nil {
			if errors.Is(err, thread.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "thread not found"}
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

// CloseThread returns a handler for threads.close.
func CloseThread(repo *thread.Repo, now Clock) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := repo.Close(ctx, p.ID, now()); err != nil {
			if errors.Is(err, thread.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "thread not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		got, err := repo.Get(ctx, p.ID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(got)
	}
}

// ReopenThread returns a handler for threads.reopen.
func ReopenThread(repo *thread.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p idParam
		if err := json.Unmarshal(params, &p); err != nil || p.ID == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "id required"}
		}
		if err := repo.Reopen(ctx, p.ID); err != nil {
			if errors.Is(err, thread.ErrNotFound) {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "thread not found"}
			}
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		got, err := repo.Get(ctx, p.ID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(got)
	}
}
