// Package provider turns the writer's settings into a tars llm.Client for the
// built-in agent (#90). It is the only package besides internal/agent allowed
// to import tars/pkg/llm — scripts/validate-story-core-deps.sh enforces that,
// so the story core and the MCP tool layer stay model-free for every agent.
package provider

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/tars/pkg/llm"
)

// codexRefreshStorageEnv is read by tars when it refreshes a Codex token. Its
// macOS default shells out to the `security` CLI, which a sandboxed App Store
// build cannot do, so every build uses the file store: auth.json at 0600 in
// Linetta's data directory, the way the Codex CLI itself keeps it.
const codexRefreshStorageEnv = "TARS_OPENAI_CODEX_REFRESH_TOKEN_STORAGE"

// codexAuthFile is the credential file tars reads under CodexHome.
const codexAuthFile = "auth.json"

// Resolved is one provider's effective configuration: settings plus the key
// the settings store keeps outside settings.json.
type Resolved struct {
	ID          string
	Model       string
	APIKey      string
	BaseURL     string
	CodexHome   string // openai-codex only: the directory holding auth.json
	ConsentedAt int64
}

// Configured reports whether the provider has a credential to call with: an
// API key, or for Codex a completed login (#92 writes the file; here it only
// has to exist).
func (r Resolved) Configured() bool {
	if r.ID == settings.ProviderOpenAICodex {
		// Without a CodexHome the Join collapses to a bare "auth.json" and
		// stats it against the process's working directory, so a stray file
		// there would read as a completed login. No home means no login.
		if r.CodexHome == "" {
			return false
		}
		_, err := os.Stat(filepath.Join(r.CodexHome, codexAuthFile))
		return err == nil
	}
	return strings.TrimSpace(r.APIKey) != ""
}

// String redacts APIKey. Resolved is passed between packages and logged with
// %+v; the built-in agent (#93) will carry it further still. Making the plain
// key unprintable is cheaper than auditing every future format verb.
func (r Resolved) String() string {
	key := "unset"
	if strings.TrimSpace(r.APIKey) != "" {
		key = "set"
	}
	return fmt.Sprintf("provider.Resolved{ID:%s Model:%s APIKey:%s BaseURL:%s CodexHome:%s ConsentedAt:%d}",
		r.ID, r.Model, key, r.BaseURL, r.CodexHome, r.ConsentedAt)
}

// Consented reports whether the writer agreed to send text to this provider.
func (r Resolved) Consented() bool { return r.ConsentedAt > 0 }

// Options is the tars-facing shape of a Resolved provider. Model stays empty
// when unset so tars applies its own default; Linetta keeps no model catalog.
func Options(r Resolved) llm.ProviderOptions {
	opts := llm.ProviderOptions{
		Provider: r.ID,
		Model:    strings.TrimSpace(r.Model),
		BaseURL:  strings.TrimSpace(r.BaseURL),
	}
	if r.ID == settings.ProviderOpenAICodex {
		opts.AuthConfig.CodexHome = r.CodexHome
	} else {
		opts.APIKey = strings.TrimSpace(r.APIKey)
	}
	return opts
}

// ClientFactory builds an llm.Client. Production is llm.NewProvider; tests
// inject one that never dials.
type ClientFactory func(opts llm.ProviderOptions) (llm.Client, error)

// Source resolves providers from the settings store.
type Source struct {
	settings  *settings.Store
	codexHome string
	factory   ClientFactory
	fetcher   llm.ModelFetcher
}

// NewSource wires the store. codexHome is where Codex's auth.json lives —
// $LINETTA_HOME/codex on every platform (#92 adds the ~/.codex fallback).
func NewSource(st *settings.Store, codexHome string) *Source {
	_ = os.Setenv(codexRefreshStorageEnv, "file")
	return &Source{
		settings:  st,
		codexHome: codexHome,
		factory:   llm.NewProvider,
		fetcher:   llm.NewModelFetcher(),
	}
}

// WithFactory replaces the client factory (tests).
func (s *Source) WithFactory(f ClientFactory) *Source {
	s.factory = f
	return s
}

// WithFetcher replaces the model lister (tests).
func (s *Source) WithFetcher(f llm.ModelFetcher) *Source {
	s.fetcher = f
	return s
}

// Resolve returns the effective config for id, or for the active provider
// when id is empty. Read on every call so a settings change applies to the
// next agent turn without a restart.
func (s *Source) Resolve(id string) (Resolved, error) {
	if id == "" {
		id = s.settings.ActiveProvider()
	}
	if !slices.Contains(settings.ValidProviders(), id) {
		return Resolved{}, &rpc.ReasonError{
			Reason: rpc.ReasonProviderNotConfigured,
			Err:    fmt.Errorf("unknown provider %q", id),
		}
	}
	cfg := s.settings.ProviderConfigFor(id)
	return Resolved{
		ID:          id,
		Model:       cfg.Model,
		APIKey:      cfg.APIKey,
		BaseURL:     cfg.BaseURL,
		CodexHome:   resolveCodexHome(s.codexHome),
		ConsentedAt: cfg.ConsentedAt,
	}, nil
}

// Client builds a client for id ("" = active). It refuses before any network
// activity when the provider is not configured or not consented to. The
// consent rule is "not a byte leaves without it", and this is the only path
// that produces something able to send one, so the check lives here.
func (s *Source) Client(id string) (llm.Client, Resolved, error) {
	r, err := s.Resolve(id)
	if err != nil {
		return nil, Resolved{}, err
	}
	if !r.Configured() {
		return nil, r, &rpc.ReasonError{
			Reason: rpc.ReasonProviderNotConfigured,
			Err:    fmt.Errorf("%s: no credential", r.ID),
		}
	}
	if !r.Consented() {
		return nil, r, &rpc.ReasonError{
			Reason: rpc.ReasonProviderConsentRequired,
			Err:    fmt.Errorf("%s: consent required", r.ID),
		}
	}
	c, err := s.factory(Options(r))
	if err != nil {
		return nil, r, Classify(r.ID, err)
	}
	return c, r, nil
}
