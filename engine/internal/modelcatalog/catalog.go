// Package modelcatalog lists the models available for a provider, using the
// tars model fetcher. It is a thin, injectable wrapper so handlers and tests do
// not depend on tars construction directly.
package modelcatalog

import (
	"context"
	"strings"

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
func (c *Catalog) List(ctx context.Context, provider, apiKey, baseURL string) ([]string, error) {
	if strings.TrimSpace(provider) == "claude-code-cli" {
		return []string{}, nil
	}
	return c.fetcher.FetchModels(ctx, llm.ProviderOptions{
		Provider: provider,
		APIKey:   apiKey,
		BaseURL:  baseURL,
	})
}
