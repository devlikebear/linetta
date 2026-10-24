// Package provider turns the writer's settings into a tars llm.Client for the
// built-in agent (#90). It is the only package besides internal/agent allowed
// to import tars/pkg/llm — scripts/validate-story-core-deps.sh enforces that,
// so the story core and the MCP tool layer stay model-free for every agent.
package provider

import (
	"context"
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
	CLIPath     string // claude-code only: the detected `claude` binary, or ""
	WorkDir     string // claude-code only: the empty directory the CLI runs in
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
	if r.ID == settings.ProviderClaudeCode {
		// The binary is all Linetta can see. Whether the writer is signed
		// in to it is something only the CLI knows, and providers.test is
		// how the pane finds out.
		return r.CLIPath != ""
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
	return fmt.Sprintf("provider.Resolved{ID:%s Model:%s APIKey:%s BaseURL:%s CodexHome:%s CLIPath:%s ConsentedAt:%d}",
		r.ID, r.Model, key, r.BaseURL, r.CodexHome, r.CLIPath, r.ConsentedAt)
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
	switch r.ID {
	case settings.ProviderOpenAICodex:
		opts.AuthConfig.CodexHome = r.CodexHome
	case settings.ProviderClaudeCode:
		// No key and no base URL: the CLI signs in on its own and talks
		// only to Anthropic.
		opts.Provider = tarsClaudeCodeProvider
		opts.BaseURL = ""
		opts.WorkDir = r.WorkDir
	default:
		opts.APIKey = strings.TrimSpace(r.APIKey)
	}
	if r.ID == settings.ProviderAnthropic {
		opts.MaxTokens = anthropicMaxTokens(opts.Model)
	}
	return opts
}

// anthropicOutputCeiling is the output limit Linetta asks Anthropic for, in
// place of the 16000 tars gives a client that names none.
//
// tars keeps its default low for callers that do not stream. The agent does,
// and its unit of work is a scene: a response that rewrites three of them in
// one go, with the model's thinking billed against the same limit, runs past
// 16000 and is cut off mid tool call — which ends the turn with nothing
// written. 64000 is the lowest ceiling any family below documents, so one
// number is valid for all of them.
const anthropicOutputCeiling = 64000

// anthropicLargeOutputFamilies are the model families known to accept
// anthropicOutputCeiling, matched by prefix so a dated or point release
// ("claude-opus-5-5", "claude-haiku-4-5-20251001") counts.
//
// An explicit limit reaches the provider as written, and Anthropic rejects one
// above the model's own — so a model this list does not name gets no explicit
// limit at all and keeps tars' conservative default, rather than a request
// that fails outright. That includes anything reached through a gateway under
// a name of its own.
//
// This is the one place Linetta knows model names. It mirrors the families in
// tars' internal/llmdefaults, which pkg/llm does not export; a family missing
// here costs only the old, lower limit.
var anthropicLargeOutputFamilies = []string{
	"claude-fable-5",
	"claude-mythos-5",
	"claude-opus-5",
	"claude-opus-4-8",
	"claude-opus-4-7",
	"claude-opus-4-6",
	"claude-sonnet-5",
	"claude-sonnet-4-6",
	"claude-haiku-4-5",
}

// anthropicMaxTokens is the explicit output limit for model, or 0 to leave the
// choice to tars.
func anthropicMaxTokens(model string) int {
	normalized := strings.ToLower(strings.TrimSpace(model))
	for _, family := range anthropicLargeOutputFamilies {
		if strings.HasPrefix(normalized, family) {
			return anthropicOutputCeiling
		}
	}
	return 0
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
	cli       *cliDetector
	// codexCache is the Codex model list a turn picks its default from.
	codexCache codexModelCache
	// cliWorkDir is where the Claude Code CLI runs: an empty directory of
	// Linetta's own. A Finder-launched app's working directory is "/", and
	// the CLI's file tools — switched off, but the directory is still its
	// root — have no business there.
	cliWorkDir string
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
		cli:       newCLIDetector(),
	}
}

// WithCLIWorkDir sets the directory the Claude Code CLI runs in.
func (s *Source) WithCLIWorkDir(dir string) *Source {
	s.cliWorkDir = dir
	return s
}

// WithCLIDetector replaces how the Claude Code CLI is found (tests).
func (s *Source) WithCLIDetector(detect func(context.Context) string) *Source {
	s.cli = &cliDetector{detect: detect}
	return s
}

// RedetectCLI forgets where the Claude Code CLI was (or that it was not
// found), so the next resolve looks again.
func (s *Source) RedetectCLI() { s.cli.Forget() }

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
	r := Resolved{
		ID:          id,
		Model:       cfg.Model,
		APIKey:      cfg.APIKey,
		BaseURL:     cfg.BaseURL,
		ConsentedAt: cfg.ConsentedAt,
	}
	// Only Codex reads CodexHome, and resolving it stats up to two paths on
	// every call. Resolve runs on every agent turn for every provider, so the
	// lookup belongs behind the one branch that uses its answer.
	if id == settings.ProviderOpenAICodex {
		r.CodexHome = resolveCodexHome(s.codexHome)
	}
	if id == settings.ProviderClaudeCode {
		r.CLIPath = s.cli.Path()
		r.WorkDir = s.cliWorkDir
	}
	return r, nil
}

// Client builds a client for id ("" = active). It refuses before any network
// activity when the provider is not configured or not consented to. The
// consent rule is "not a byte leaves without it", and this is the only path
// that produces something able to send one, so the check lives here.
//
// For Codex with no model chosen, the returned Resolved carries the model the
// account's list picked (see codexModel), so the caller logs what was used.
func (s *Source) Client(ctx context.Context, id string) (llm.Client, Resolved, error) {
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
	if r.ID == settings.ProviderOpenAICodex {
		r.Model = s.codexModel(ctx, r)
	}
	if r.ID == settings.ProviderClaudeCode {
		pointTarsAtCLI(r.CLIPath)
		if r.WorkDir != "" {
			if err := os.MkdirAll(r.WorkDir, 0o700); err != nil {
				return nil, r, Classify(r.ID, err)
			}
		}
	}
	c, err := s.factory(Options(r))
	if err != nil {
		return nil, r, Classify(r.ID, err)
	}
	if r.ID == settings.ProviderClaudeCode {
		// The only way a claude-code client leaves this package.
		c = lockedClaudeCode{inner: c}
	}
	return c, r, nil
}
