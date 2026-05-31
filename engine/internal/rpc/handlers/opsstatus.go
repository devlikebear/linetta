package handlers

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/opsstatus"
	"github.com/devlikebear/linetta/engine/internal/rpc"
)

func GetOpsStatus(repo *opsstatus.Repo) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		statuses, err := repo.Get(ctx)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(statuses)
	}
}

type clearOpsStatusParams struct {
	JobName string `json:"job_name"`
}

func ClearOpsStatusError(repo *opsstatus.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p clearOpsStatusParams
		if err := json.Unmarshal(params, &p); err != nil || p.JobName == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "job_name required"}
		}
		if err := repo.ClearError(ctx, p.JobName); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}
