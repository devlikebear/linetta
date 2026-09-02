package engineapp

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/provider"
)

// providerService adapts *provider.Source to handlers.ProviderService. The
// handlers see JSON and strings only, so they never link tars/pkg/llm.
type providerService struct {
	src *provider.Source
}

func (p providerService) List(context.Context) (json.RawMessage, error) {
	return json.Marshal(p.src.List())
}

func (p providerService) ListModels(ctx context.Context, id string) ([]string, error) {
	return p.src.ListModels(ctx, id)
}

func (p providerService) Test(ctx context.Context, id string) error {
	return p.src.Test(ctx, id)
}
