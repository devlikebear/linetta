// Package modelcatalog lists the models available for a provider, using the
// tars model fetcher. It is a thin, injectable wrapper so handlers and tests do
// not depend on tars construction directly.
package modelcatalog

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/tars/pkg/llm"
)

// Catalog wraps a tars model fetcher.
type Catalog struct{ fetcher llm.ModelFetcher }

// New builds a Catalog over an explicit fetcher (used in tests).
func New(f llm.ModelFetcher) *Catalog { return &Catalog{fetcher: f} }

// Default builds a Catalog backed by tars' live model fetcher.
func Default() *Catalog { return &Catalog{fetcher: llm.NewModelFetcher()} }

// List returns model ids for a provider. claude-code-cli has no list API and
// returns an empty slice (callers fall back to free-text entry). baseURL is the
// optional custom endpoint for OpenAI/Anthropic-compatible providers; empty uses
// the provider default.
//
// OAuth providers (openai-codex) authenticate against the chat backend, but the
// model-list endpoint (api.openai.com/v1/models) requires the api.model.read
// scope that ChatGPT OAuth tokens lack, so it always returns 401/403. Mirroring
// tars' own console handler, we treat that as a soft failure: return an empty
// list so the UI falls back to manual model entry rather than surfacing a raw
// permission error. Other failures still propagate.
func (c *Catalog) List(ctx context.Context, provider, apiKey, baseURL string) ([]string, error) {
	if strings.TrimSpace(provider) == "claude-code-cli" {
		return []string{}, nil
	}
	if strings.TrimSpace(provider) == settings.ProviderOpenRouter {
		provider = settings.ProviderOpenAI
		if strings.TrimSpace(baseURL) == "" {
			baseURL = settings.OpenRouterBaseURL
		}
	}
	models, err := c.fetcher.FetchModels(ctx, llm.ProviderOptions{
		Provider: provider,
		APIKey:   apiKey,
		BaseURL:  baseURL,
	})
	if err != nil {
		var pe *llm.ProviderError
		if errors.As(err, &pe) && (pe.StatusCode == http.StatusUnauthorized || pe.StatusCode == http.StatusForbidden) {
			return []string{}, nil
		}
		return nil, err
	}
	return models, nil
}
