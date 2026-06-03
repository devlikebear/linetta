// Package settings persists user-controlled preferences in $LINETTA_HOME/settings.json.
// Sensitive credentials are stored through SecretStore instead of the JSON file.
package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/devlikebear/linetta/engine/internal/paths"
)

// Allowed provider IDs (must match tars/pkg/llm provider names + UI labels).
const (
	ProviderClaudeCodeCLI = "claude-code-cli"
	ProviderOpenAICodex   = "openai-codex"
	ProviderAnthropic     = "anthropic"
	ProviderOpenAI        = "openai"
	ProviderGeminiNative  = "gemini-native"
)

func validProviders() []string {
	return []string{
		ProviderClaudeCodeCLI,
		ProviderOpenAICodex,
		ProviderAnthropic,
		ProviderOpenAI,
		ProviderGeminiNative,
	}
}

func validWebSearchProviders() []string { return []string{"brave", "perplexity"} }

// ProviderConfig holds per-provider settings keyed by provider id in Config.Providers.
type ProviderConfig struct {
	Model       string `json:"model,omitempty"`         // selected model id; empty => provider default
	APIKey      string `json:"api_key,omitempty"`       // write-only in settings.set; redacted from settings.get and disk
	APIKeySet   bool   `json:"api_key_set,omitempty"`   // read-only presence flag for settings.get
	ClearAPIKey bool   `json:"clear_api_key,omitempty"` // write-only deletion flag for settings.set
	BaseURL     string `json:"base_url,omitempty"`      // custom endpoint for OpenAI/Anthropic-compatible providers (MiniMax, Kimi, ...)
	CliPath     string `json:"cli_path,omitempty"`      // claude-code-cli binary path override
}

// ProviderSettings is the resolved active-provider view consumed by the ai
// package (kept here so settings has no dependency on ai).
type ProviderSettings struct {
	Provider string
	Model    string
	APIKey   string
	BaseURL  string
	CliPath  string
}

// Config is the on-disk JSON. backup_dir is computed at Load time and not
// persisted (the field is omitted from JSON marshalling on write).
type Config struct {
	Provider                 string                    `json:"provider"`
	Providers                map[string]ProviderConfig `json:"providers,omitempty"`
	TypewriterDefault        bool                      `json:"typewriter_default"`
	FocusDefault             bool                      `json:"focus_default"`
	BackupDir                string                    `json:"backup_dir,omitempty"`
	GitSyncDir               string                    `json:"git_sync_dir"`
	GitSyncCommitTemplate    string                    `json:"git_sync_commit_template"`
	SafetyChecklistDismissed bool                      `json:"safety_checklist_dismissed"`
	WebSearchProvider        string                    `json:"web_search_provider"`
	WebSearchAPIKey          string                    `json:"web_search_api_key,omitempty"`     // write-only in settings.set; redacted from settings.get and disk
	WebSearchAPIKeySet       bool                      `json:"web_search_api_key_set,omitempty"` // read-only presence flag for settings.get
}

// Patch holds optional updates. Nil pointers mean "leave the field alone".
type Patch struct {
	Provider                 *string                   `json:"provider,omitempty"`
	Providers                map[string]ProviderConfig `json:"providers,omitempty"`
	TypewriterDefault        *bool                     `json:"typewriter_default,omitempty"`
	FocusDefault             *bool                     `json:"focus_default,omitempty"`
	GitSyncDir               *string                   `json:"git_sync_dir,omitempty"`
	GitSyncCommitTemplate    *string                   `json:"git_sync_commit_template,omitempty"`
	SafetyChecklistDismissed *bool                     `json:"safety_checklist_dismissed,omitempty"`
	WebSearchProvider        *string                   `json:"web_search_provider,omitempty"`
	WebSearchAPIKey          *string                   `json:"web_search_api_key,omitempty"`
}

// Store reads and writes the settings file with internal locking.
type Store struct {
	mu      sync.RWMutex // protects cfg reads
	writeMu sync.Mutex   // serializes Set: validation → cfg update → disk write
	cfg     Config
	dir     string
	secrets SecretStore
}

// New constructs a Store, ensuring $LINETTA_HOME exists and loading the file.
// Missing or corrupt files yield defaults (and a quiet rewrite on next Set).
func New() (*Store, error) {
	return NewWithSecretStore(defaultSecretStore())
}

// NewWithSecretStore constructs a Store using an explicit secret backend.
// Tests inject an in-memory store; production uses the macOS Keychain backend.
func NewWithSecretStore(secrets SecretStore) (*Store, error) {
	if err := paths.EnsureHome(); err != nil {
		return nil, err
	}
	home, err := paths.Home()
	if err != nil {
		return nil, err
	}
	if secrets == nil {
		secrets = defaultSecretStore()
	}
	s := &Store{dir: home, cfg: defaults(home), secrets: secrets}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func defaults(home string) Config {
	return Config{
		Provider:          ProviderOpenAICodex,
		TypewriterDefault: false,
		BackupDir:         filepath.Join(home, "backups"),
		WebSearchProvider: "brave",
	}
}

func (s *Store) load() error {
	path := filepath.Join(s.dir, "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil // keep defaults
		}
		return err
	}
	var disk Config
	if err := json.Unmarshal(data, &disk); err != nil {
		log.Printf("settings: ignoring corrupt %s; falling back to defaults: %v", path, err)
		return nil // ignore corrupt file; defaults stand
	}
	s.mu.Lock()
	if disk.Provider != "" {
		s.cfg.Provider = disk.Provider
	}
	s.cfg.TypewriterDefault = disk.TypewriterDefault
	s.cfg.FocusDefault = disk.FocusDefault
	s.cfg.GitSyncDir = disk.GitSyncDir
	s.cfg.GitSyncCommitTemplate = disk.GitSyncCommitTemplate
	s.cfg.SafetyChecklistDismissed = disk.SafetyChecklistDismissed
	if disk.WebSearchProvider != "" {
		s.cfg.WebSearchProvider = disk.WebSearchProvider
	}
	s.cfg.WebSearchAPIKey = disk.WebSearchAPIKey
	migratedProviderKeys := false
	if len(disk.Providers) > 0 {
		providers, migrated, err := s.migrateProviderSecrets(disk.Providers)
		if err != nil {
			s.mu.Unlock()
			return err
		}
		s.cfg.Providers = providers
		if migrated {
			disk.Providers = providers
			migratedProviderKeys = true
		}
	}
	migratedWebKey := false
	if disk.WebSearchAPIKey != "" {
		if err := s.setSecret(webSearchAPIKeySecretName, disk.WebSearchAPIKey); err != nil {
			s.mu.Unlock()
			return err
		}
		s.cfg.WebSearchAPIKey = ""
		disk.WebSearchAPIKey = ""
		migratedWebKey = true
	}
	next := s.cfg
	s.mu.Unlock()
	if migratedWebKey || migratedProviderKeys {
		if err := s.persist(next); err != nil {
			return err
		}
	}
	return nil
}

// Get returns a copy of the current Config (with backup_dir filled in).
func (s *Store) Get(ctx context.Context) (Config, error) {
	s.mu.RLock()
	c := s.cfg
	s.mu.RUnlock()
	c.BackupDir = filepath.Join(s.dir, "backups")
	return s.redactedSettingsView(c), nil
}

// Provider returns the active provider id (cheap, lock-protected).
// ai.Runner calls this on every Start so provider changes take effect at once.
func (s *Store) Provider() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Provider
}

// Resolve returns the active provider plus its per-provider config. Consulted on
// every AI call so settings changes take effect without an engine restart.
func (s *Store) Resolve() ProviderSettings {
	s.mu.RLock()
	pc := s.cfg.Providers[s.cfg.Provider]
	provider := s.cfg.Provider
	s.mu.RUnlock()
	pc = s.runtimeProviderConfig(provider, pc)
	return ProviderSettings{
		Provider: provider,
		Model:    pc.Model,
		APIKey:   pc.APIKey,
		BaseURL:  pc.BaseURL,
		CliPath:  pc.CliPath,
	}
}

// ProviderConfigFor returns the stored config for a provider id (zero value if unset).
func (s *Store) ProviderConfigFor(id string) ProviderConfig {
	s.mu.RLock()
	cfg := s.cfg.Providers[id]
	s.mu.RUnlock()
	return s.runtimeProviderConfig(id, cfg)
}

func (s *Store) WebSearchProvider() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg.WebSearchProvider == "" {
		return "brave"
	}
	return s.cfg.WebSearchProvider
}

func (s *Store) WebSearchAPIKey() string {
	secret, ok, err := s.secrets.Get(webSearchAPIKeySecretName)
	if err != nil || !ok {
		return ""
	}
	return secret
}

// Set applies a partial patch, validates, persists atomically, returns the new Config.
func (s *Store) Set(ctx context.Context, p Patch) (Config, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	s.mu.RLock()
	next := s.cfg
	s.mu.RUnlock()

	if p.Provider != nil {
		if !slices.Contains(validProviders(), *p.Provider) {
			return Config{}, fmt.Errorf("settings: unknown provider %q", *p.Provider)
		}
		next.Provider = *p.Provider
	}
	if p.Providers != nil {
		merged := map[string]ProviderConfig{}
		maps.Copy(merged, next.Providers)
		for k, v := range p.Providers {
			if !slices.Contains(validProviders(), k) {
				return Config{}, fmt.Errorf("settings: unknown provider %q", k)
			}
			if err := s.applyProviderSecretPatch(k, &v); err != nil {
				return Config{}, err
			}
			merged[k] = v
		}
		next.Providers = merged
	}
	if p.TypewriterDefault != nil {
		next.TypewriterDefault = *p.TypewriterDefault
	}
	if p.FocusDefault != nil {
		next.FocusDefault = *p.FocusDefault
	}
	if p.GitSyncDir != nil {
		next.GitSyncDir = *p.GitSyncDir
	}
	if p.GitSyncCommitTemplate != nil {
		next.GitSyncCommitTemplate = *p.GitSyncCommitTemplate
	}
	if p.SafetyChecklistDismissed != nil {
		next.SafetyChecklistDismissed = *p.SafetyChecklistDismissed
	}
	if p.WebSearchProvider != nil {
		if !slices.Contains(validWebSearchProviders(), *p.WebSearchProvider) {
			return Config{}, fmt.Errorf("settings: unknown web_search_provider %q", *p.WebSearchProvider)
		}
		next.WebSearchProvider = *p.WebSearchProvider
	}
	if p.WebSearchAPIKey != nil {
		if err := s.applyWebSearchSecretPatch(*p.WebSearchAPIKey); err != nil {
			return Config{}, err
		}
		next.WebSearchAPIKey = ""
	}
	if next.WebSearchProvider == "" {
		next.WebSearchProvider = "brave"
	}

	if err := s.persist(next); err != nil {
		return Config{}, err
	}

	s.mu.Lock()
	s.cfg = sanitizeConfigForMemory(next)
	s.mu.Unlock()

	return s.Get(ctx)
}

func (s *Store) persist(next Config) error {
	next = sanitizeConfigForDisk(next)
	persistable := Config{
		Provider:                 next.Provider,
		Providers:                next.Providers,
		TypewriterDefault:        next.TypewriterDefault,
		FocusDefault:             next.FocusDefault,
		GitSyncDir:               next.GitSyncDir,
		GitSyncCommitTemplate:    next.GitSyncCommitTemplate,
		SafetyChecklistDismissed: next.SafetyChecklistDismissed,
		WebSearchProvider:        next.WebSearchProvider,
	}
	body, err := json.MarshalIndent(persistable, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(s.dir, "settings.json.tmp")
	target := filepath.Join(s.dir, "settings.json")
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		return err
	}
	return nil
}

func (s *Store) migrateProviderSecrets(providers map[string]ProviderConfig) (map[string]ProviderConfig, bool, error) {
	out := map[string]ProviderConfig{}
	migrated := false
	for id, cfg := range providers {
		if cfg.APIKey != "" {
			if err := s.setSecret(providerAPIKeySecretName(id), cfg.APIKey); err != nil {
				return nil, false, err
			}
			cfg.APIKey = ""
			migrated = true
		}
		cfg.APIKeySet = false
		cfg.ClearAPIKey = false
		out[id] = cfg
	}
	return out, migrated, nil
}

func (s *Store) applyProviderSecretPatch(provider string, cfg *ProviderConfig) error {
	switch {
	case cfg.ClearAPIKey:
		if err := s.secrets.Delete(providerAPIKeySecretName(provider)); err != nil {
			return err
		}
		cfg.APIKey = ""
		cfg.APIKeySet = false
	case cfg.APIKey != "":
		if err := s.setSecret(providerAPIKeySecretName(provider), cfg.APIKey); err != nil {
			return err
		}
		cfg.APIKey = ""
		cfg.APIKeySet = true
	default:
		cfg.APIKey = ""
		_, ok, err := s.secrets.Get(providerAPIKeySecretName(provider))
		if err != nil {
			return err
		}
		cfg.APIKeySet = ok
	}
	cfg.ClearAPIKey = false
	return nil
}

func (s *Store) applyWebSearchSecretPatch(value string) error {
	if value == "" {
		return s.secrets.Delete(webSearchAPIKeySecretName)
	}
	return s.setSecret(webSearchAPIKeySecretName, value)
}

func (s *Store) setSecret(name, value string) error {
	if value == "" {
		return s.secrets.Delete(name)
	}
	return s.secrets.Set(name, value)
}

func (s *Store) runtimeProviderConfig(provider string, cfg ProviderConfig) ProviderConfig {
	cfg.APIKey = ""
	cfg.APIKeySet = false
	cfg.ClearAPIKey = false
	secret, ok, err := s.secrets.Get(providerAPIKeySecretName(provider))
	if err != nil || !ok {
		return cfg
	}
	cfg.APIKey = secret
	cfg.APIKeySet = true
	return cfg
}

func (s *Store) redactedSettingsView(c Config) Config {
	c = sanitizeConfigForMemory(c)
	providers := map[string]ProviderConfig{}
	for id, cfg := range c.Providers {
		_, ok, err := s.secrets.Get(providerAPIKeySecretName(id))
		if err == nil {
			cfg.APIKeySet = ok
		}
		providers[id] = cfg
	}
	c.Providers = providers
	_, webKeySet, err := s.secrets.Get(webSearchAPIKeySecretName)
	if err == nil {
		c.WebSearchAPIKeySet = webKeySet
	}
	return c
}

func sanitizeConfigForMemory(c Config) Config {
	c.WebSearchAPIKey = ""
	providers := map[string]ProviderConfig{}
	for id, cfg := range c.Providers {
		cfg.APIKey = ""
		cfg.ClearAPIKey = false
		providers[id] = cfg
	}
	c.Providers = providers
	return c
}

func sanitizeConfigForDisk(c Config) Config {
	c = sanitizeConfigForMemory(c)
	c.WebSearchAPIKeySet = false
	providers := map[string]ProviderConfig{}
	for id, cfg := range c.Providers {
		cfg.APIKeySet = false
		providers[id] = cfg
	}
	c.Providers = providers
	return c
}
