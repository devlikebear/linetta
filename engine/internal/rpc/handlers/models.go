package handlers

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/clidetect"
	"github.com/devlikebear/linetta/engine/internal/modelcatalog"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/settings"
)

type listModelsParams struct {
	Provider string `json:"provider"`
}

type listModelsResult struct {
	Models []string `json:"models"`
}

// ListModels returns a handler for providers.list_models. It resolves the API
// key for the requested provider from settings and asks the catalog for the
// live model list. With no provider in the request, the active provider is used.
func ListModels(store *settings.Store, catalog *modelcatalog.Catalog) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p listModelsParams
		if len(params) > 0 {
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
			}
		}
		provider := p.Provider
		if provider == "" {
			provider = store.Provider()
		}
		key := store.ProviderConfigFor(provider).APIKey
		models, err := catalog.List(ctx, provider, key)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(listModelsResult{Models: models})
	}
}

type detectCLIResult struct {
	Path string `json:"path"`
}

// DetectCLI returns a handler for providers.detect_cli. It locates the Claude
// Code CLI executable (PATH, login shell, then known install dirs) and returns
// the resolved path, or an empty string when not found.
func DetectCLI() rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		return json.Marshal(detectCLIResult{Path: clidetect.Detect(ctx)})
	}
}
