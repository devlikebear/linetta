package handlers

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// ProviderService is the slice of internal/provider the RPC layer needs.
// Declared as an interface with plain types because handlers must not link
// tars/pkg/llm (scripts/validate-story-core-deps.sh) and the provider
// package does. List hands back JSON the way MCPController.Status does.
type ProviderService interface {
	List(ctx context.Context) (json.RawMessage, error)
	ListModels(ctx context.Context, id string) ([]string, error)
	Test(ctx context.Context, id string) error
}

type providerParams struct {
	Provider string `json:"provider,omitempty"` // empty means the active provider
}

func decodeProviderParams(params json.RawMessage) providerParams {
	var p providerParams
	if len(params) > 0 {
		_ = json.Unmarshal(params, &p)
	}
	return p
}

// ProvidersList returns a handler for providers.list: every provider this
// build offers and where each one stands (configured, consented, active).
func ProvidersList(svc ProviderService) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		out, err := svc.List(ctx)
		if err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		return out, nil
	}
}

// ProvidersListModels returns a handler for providers.list_models. A failure
// is an RPC error; the pane falls back to free-text entry.
func ProvidersListModels(svc ProviderService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		p := decodeProviderParams(params)
		models, err := svc.ListModels(ctx, p.Provider)
		if err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		if models == nil {
			models = []string{}
		}
		return json.Marshal(struct {
			Models []string `json:"models"`
		}{models})
	}
}

// ProvidersTest returns a handler for providers.test: one short prompt,
// consent-gated on the provider side.
func ProvidersTest(svc ProviderService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		p := decodeProviderParams(params)
		if err := svc.Test(ctx, p.Provider); err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}
