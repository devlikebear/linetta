package provider

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/settings"
)

// Status is what the settings pane shows per provider. It never carries a
// secret: Configured says a key or login exists, not what it is.
type Status struct {
	ID         string `json:"id"`
	Auth       string `json:"auth"` // "oauth" | "api_key"
	Active     bool   `json:"active"`
	Configured bool   `json:"configured"`
	Consented  bool   `json:"consented"`
	Model      string `json:"model,omitempty"`
	BaseURL    string `json:"base_url,omitempty"`
}

// testTimeout bounds providers.test; a hung endpoint must not hang the pane.
const testTimeout = 30 * time.Second

// testPrompt is the one line providers.test sends. No manuscript text — but
// it still goes through Client, so it still needs consent.
const testPrompt = "Reply with the single word OK."

func authKind(id string) string {
	if id == settings.ProviderOpenAICodex {
		return "oauth"
	}
	return "api_key"
}

// List describes every provider this build offers, in whitelist order.
func (s *Source) List() []Status {
	active := s.settings.ActiveProvider()
	out := make([]Status, 0, len(settings.ValidProviders()))
	for _, id := range settings.ValidProviders() {
		r, err := s.Resolve(id)
		if err != nil {
			continue
		}
		out = append(out, Status{
			ID:         id,
			Auth:       authKind(id),
			Active:     id == active,
			Configured: r.Configured(),
			Consented:  r.Consented(),
			Model:      r.Model,
			BaseURL:    r.BaseURL,
		})
	}
	return out
}

// ListModels asks the provider for its model ids. It needs a credential but
// not consent: no manuscript text is involved, and the pane lists models
// before the writer has read the consent sentence.
func (s *Source) ListModels(ctx context.Context, id string) ([]string, error) {
	r, err := s.Resolve(id)
	if err != nil {
		return nil, err
	}
	if !r.Configured() {
		return nil, &rpc.ReasonError{
			Reason: rpc.ReasonProviderNotConfigured,
			Err:    fmt.Errorf("%s: no credential", r.ID),
		}
	}
	models, err := s.fetcher.FetchModels(ctx, Options(r))
	if err != nil {
		return nil, Classify(r.ID, err)
	}
	sort.Strings(models)
	return models, nil
}

// Test sends one short prompt through a real client. It runs through Client
// on purpose: even the connection test sends nothing before consent.
func (s *Source) Test(ctx context.Context, id string) error {
	c, r, err := s.Client(id)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, testTimeout)
	defer cancel()
	if _, err := c.Ask(ctx, testPrompt); err != nil {
		return Classify(r.ID, err)
	}
	return nil
}
