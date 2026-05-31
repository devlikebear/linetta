package handlers

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/search"
)

type searchParams struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

// Search returns an app-wide project/node search handler.
func Search(repo *search.Repo) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p searchParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
		}
		if strings.TrimSpace(p.Query) == "" {
			return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: "query required"}
		}
		results, err := repo.Query(ctx, p.Query, p.Limit)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		if results == nil {
			results = []search.Result{}
		}
		return json.Marshal(results)
	}
}
