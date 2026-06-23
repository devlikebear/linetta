// Package modelcatalog lists the models available for a provider, using the
// tars model fetcher. It is a thin, injectable wrapper so handlers and tests do
// not depend on tars construction directly.
package modelcatalog

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/openrouter"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/tars/pkg/llm"
)

// Catalog wraps a tars model fetcher.
type Catalog struct {
	fetcher          llm.ModelFetcher
	openRouterModels func(context.Context, string) ([]openrouter.Model, error)
}

// New builds a Catalog over an explicit fetcher (used in tests).
func New(f llm.ModelFetcher) *Catalog { return NewWithOpenRouter(f, openrouter.FetchModels) }

// NewWithOpenRouter builds a Catalog with an explicit OpenRouter fetcher.
func NewWithOpenRouter(f llm.ModelFetcher, openRouterModels func(context.Context, string) ([]openrouter.Model, error)) *Catalog {
	if openRouterModels == nil {
		openRouterModels = openrouter.FetchModels
	}
	return &Catalog{fetcher: f, openRouterModels: openRouterModels}
}

// Default builds a Catalog backed by tars' live model fetcher.
func Default() *Catalog { return New(llm.NewModelFetcher()) }

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
		return c.listOpenRouter(ctx, apiKey)
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

func (c *Catalog) listOpenRouter(ctx context.Context, apiKey string) ([]string, error) {
	models, err := c.openRouterModels(ctx, apiKey)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(models)+1)
	seen := map[string]bool{}
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] || isOpenRouterNonTextModel(id) {
			return
		}
		seen[id] = true
		ids = append(ids, id)
	}
	add(settings.DefaultOpenRouterModel)
	add(settings.OpenRouterFastModel)
	add(settings.OpenRouterAutoModel)
	for _, model := range models {
		add(model.ID)
	}
	return ids, nil
}

func isOpenRouterNonTextModel(id string) bool {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return true
	}
	mediaMarkers := []string{
		"image",
		"img",
		"tts",
		"audio",
		"speech",
		"music",
		"video",
		"veo",
		"lyria",
		"stable-diffusion",
		"flux",
		"dall-e",
		"midjourney",
	}
	for _, marker := range mediaMarkers {
		if strings.Contains(id, marker) {
			return true
		}
	}
	return false
}
