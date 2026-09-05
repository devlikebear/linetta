// Package settings persists user-controlled preferences in $LINETTA_HOME/settings.json.
// Sensitive credentials are stored through SecretStore instead of the JSON file.
package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/devlikebear/linetta/engine/internal/paths"
)

// Provider ids the built-in agent can use (#90). Each maps 1:1 to a tars
// pkg/llm provider name. Codex authenticates with an OAuth login the app
// performs itself (#92); the other three take an API key. "openai" is the
// OpenAI-compatible family: base_url points it at OpenRouter, Ollama, LM Studio.
const (
	ProviderOpenAICodex  = "openai-codex"
	ProviderAnthropic    = "anthropic"
	ProviderGeminiNative = "gemini-native"
	ProviderOpenAI       = "openai"
)

// AIDataSharingConsentVersion increments whenever the disclosed third-party
// data-sharing terms materially change and renewed consent is required.
const AIDataSharingConsentVersion = 1

// DefaultOpenAICodexModel is the ChatGPT-account compatible Codex default.
// Leaving the model empty lets tars fall back to gpt-5.3-codex, which is only
// supported for API-backed Codex and returns 400 for ChatGPT account auth.
const DefaultOpenAICodexModel = "gpt-5.3-codex-spark"

func validLanguages() []string { return []string{"ko", "en", "ja"} }

func validThemes() []string { return []string{"system", "light", "dark"} }

// Palettes pick which set of colours the UI uses; theme picks which end of
// that set. The two are independent. "hanji" is the default; "paper" is the
// original burnt-sienna palette, kept as an option.
func validPalettes() []string { return []string{"hanji", "paper", "bone", "press"} }

const defaultPalette = "hanji"

func validCopyProfiles() []string { return []string{"plain", "munpia", "series", "joara"} }

// ProviderConfig is one provider's stored entry (#90/#91).
//
// APIKey is legacy on-disk input only: a pre-1.0 settings.json may still carry
// one, load() moves it into the SecretStore and it is never written back.
// Patches carry keys through ProviderPatch instead.
type ProviderConfig struct {
	Model       string `json:"model,omitempty"`
	APIKey      string `json:"api_key,omitempty"`
	APIKeySet   bool   `json:"api_key_set,omitempty"`  // settings.get presence flag, never persisted
	BaseURL     string `json:"base_url,omitempty"`     // openai only
	ConsentedAt int64  `json:"consented_at,omitempty"` // per-provider data-sharing consent; 0 = none
}

// ProviderPatch is one provider's entry in Patch.Providers. Nil leaves the
// field alone. An empty APIKey deletes the stored key; ConsentedAt 0 revokes.
type ProviderPatch struct {
	Model       *string `json:"model,omitempty"`
	BaseURL     *string `json:"base_url,omitempty"`
	APIKey      *string `json:"api_key,omitempty"`
	ConsentedAt *int64  `json:"consented_at,omitempty"`
}

// Config is the on-disk JSON. backup_dir is computed at Load time and not
// persisted (the field is omitted from JSON marshalling on write).
type Config struct {
	Language string `json:"language"`
	// The built-in agent's provider (#90). Provider is the active id; Providers
	// holds each id's model, base URL and consent. Keys live in the SecretStore.
	Provider                    string                    `json:"provider"`
	Providers                   map[string]ProviderConfig `json:"providers,omitempty"`
	TypewriterDefault           bool                      `json:"typewriter_default"`
	FocusDefault                bool                      `json:"focus_default"`
	Theme                       string                    `json:"theme"`
	Palette                     string                    `json:"palette"`
	EditorFontSize              int                       `json:"editor_font_size"`
	EditorLineHeight            float64                   `json:"editor_line_height"`
	CopyProfile                 string                    `json:"copy_profile"`
	BackupDir                   string                    `json:"backup_dir,omitempty"`
	GitSyncDir                  string                    `json:"git_sync_dir"`
	GitSyncCommitTemplate       string                    `json:"git_sync_commit_template"`
	FolderSyncDir               string                    `json:"folder_sync_dir"`
	FolderSyncEnabled           bool                      `json:"folder_sync_enabled"`
	SafetyChecklistDismissed    bool                      `json:"safety_checklist_dismissed"`
	OnboardingTourEnabled       bool                      `json:"onboarding_tour_enabled"`
	OnboardingTourSeenVersion   string                    `json:"onboarding_tour_seen_version"`
	AIDataSharingConsentVersion int                       `json:"ai_data_sharing_consent_version"`
	AIDataSharingConsentedAt    int64                     `json:"ai_data_sharing_consented_at"`
	WebSearchProvider           string                    `json:"web_search_provider"`
	WebSearchAPIKey             string                    `json:"web_search_api_key,omitempty"`     // write-only in settings.set; redacted from settings.get and disk
	WebSearchAPIKeySet          bool                      `json:"web_search_api_key_set,omitempty"` // read-only presence flag for settings.get
	MCPMode                     string                    `json:"mcp_mode"`                         // off | read_only | full; off means no listener binds
	MCPPort                     int                       `json:"mcp_port"`                         // fixed so saved client configs survive restarts
	MCPProjectID                string                    `json:"mcp_project_id"`                   // empty means every work is reachable
	MCPConsentVersion           int                       `json:"mcp_consent_version"`
	MCPConsentedAt              int64                     `json:"mcp_consented_at"`
	MCPTokenSet                 bool                      `json:"mcp_token_set,omitempty"` // read-only presence flag for settings.get
}

// Patch holds optional updates. Nil pointers mean "leave the field alone".
type Patch struct {
	Language                  *string                  `json:"language,omitempty"`
	Provider                  *string                  `json:"provider,omitempty"`
	Providers                 map[string]ProviderPatch `json:"providers,omitempty"`
	TypewriterDefault         *bool                    `json:"typewriter_default,omitempty"`
	FocusDefault              *bool                    `json:"focus_default,omitempty"`
	Theme                     *string                  `json:"theme,omitempty"`
	Palette                   *string                  `json:"palette,omitempty"`
	EditorFontSize            *int                     `json:"editor_font_size,omitempty"`
	EditorLineHeight          *float64                 `json:"editor_line_height,omitempty"`
	CopyProfile               *string                  `json:"copy_profile,omitempty"`
	GitSyncDir                *string                  `json:"git_sync_dir,omitempty"`
	GitSyncCommitTemplate     *string                  `json:"git_sync_commit_template,omitempty"`
	FolderSyncDir             *string                  `json:"folder_sync_dir,omitempty"`
	FolderSyncEnabled         *bool                    `json:"folder_sync_enabled,omitempty"`
	SafetyChecklistDismissed  *bool                    `json:"safety_checklist_dismissed,omitempty"`
	OnboardingTourEnabled     *bool                    `json:"onboarding_tour_enabled,omitempty"`
	OnboardingTourSeenVersion *string                  `json:"onboarding_tour_seen_version,omitempty"`
	MCPMode                   *string                  `json:"mcp_mode,omitempty"`
	MCPPort                   *int                     `json:"mcp_port,omitempty"`
	MCPProjectID              *string                  `json:"mcp_project_id,omitempty"`
	MCPConsentVersion         *int                     `json:"mcp_consent_version,omitempty"`
	MCPConsentedAt            *int64                   `json:"mcp_consented_at,omitempty"`
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
	home, err := paths.Home()
	if err != nil {
		return nil, err
	}
	return NewForHomeWithSecretStore(home, secrets)
}

// NewForHome constructs a Store rooted at home. Engine embedding uses this to
// keep tests and in-process runtimes from falling back to the process default.
func NewForHome(home string) (*Store, error) {
	return NewForHomeWithSecretStore(home, defaultSecretStore())
}

// NewForHomeWithSecretStore constructs a Store rooted at home using an
// explicit secret backend.
func NewForHomeWithSecretStore(home string, secrets SecretStore) (*Store, error) {
	if home == "" {
		return NewWithSecretStore(secrets)
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
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
		Language:              "ko",
		Provider:              ProviderOpenAICodex,
		Providers:             map[string]ProviderConfig{ProviderOpenAICodex: {Model: DefaultOpenAICodexModel}},
		TypewriterDefault:     false,
		Theme:                 "system",
		Palette:               defaultPalette,
		EditorFontSize:        20,
		EditorLineHeight:      1.92,
		CopyProfile:           "plain",
		BackupDir:             filepath.Join(home, "backups"),
		OnboardingTourEnabled: true,
		WebSearchProvider:     "brave",
		MCPMode:               MCPModeOff,
		MCPPort:               DefaultMCPPort,
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
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		raw = map[string]json.RawMessage{}
	}
	s.mu.Lock()
	if disk.Language != "" && slices.Contains(validLanguages(), disk.Language) {
		s.cfg.Language = disk.Language
	}
	if disk.Provider != "" {
		s.cfg.Provider = disk.Provider
	}
	s.cfg.TypewriterDefault = disk.TypewriterDefault
	s.cfg.FocusDefault = disk.FocusDefault
	if disk.Theme != "" && slices.Contains(validThemes(), disk.Theme) {
		s.cfg.Theme = disk.Theme
	}
	if disk.Palette != "" && slices.Contains(validPalettes(), disk.Palette) {
		s.cfg.Palette = disk.Palette
	}
	if disk.EditorFontSize >= 15 && disk.EditorFontSize <= 22 {
		s.cfg.EditorFontSize = disk.EditorFontSize
	}
	if disk.EditorLineHeight >= 1.6 && disk.EditorLineHeight <= 2.2 {
		s.cfg.EditorLineHeight = disk.EditorLineHeight
	}
	if disk.CopyProfile != "" && slices.Contains(validCopyProfiles(), disk.CopyProfile) {
		s.cfg.CopyProfile = disk.CopyProfile
	}
	s.cfg.GitSyncDir = disk.GitSyncDir
	s.cfg.GitSyncCommitTemplate = disk.GitSyncCommitTemplate
	s.cfg.FolderSyncDir = disk.FolderSyncDir
	s.cfg.FolderSyncEnabled = disk.FolderSyncEnabled
	s.cfg.SafetyChecklistDismissed = disk.SafetyChecklistDismissed
	if _, ok := raw["onboarding_tour_enabled"]; ok {
		s.cfg.OnboardingTourEnabled = disk.OnboardingTourEnabled
	}
	s.cfg.OnboardingTourSeenVersion = disk.OnboardingTourSeenVersion
	s.cfg.AIDataSharingConsentVersion = disk.AIDataSharingConsentVersion
	s.cfg.AIDataSharingConsentedAt = disk.AIDataSharingConsentedAt
	if disk.WebSearchProvider != "" {
		s.cfg.WebSearchProvider = disk.WebSearchProvider
	}
	// MCP settings written by a newer build must survive a reload. Blank or
	// out-of-range values (including a file written by a build that predates
	// these keys) keep the defaults, and normalizeMCPPreferences below is the
	// final guard that an unrecognized mode never becomes an open server.
	if disk.MCPMode != "" {
		s.cfg.MCPMode = disk.MCPMode
	}
	if disk.MCPPort != 0 {
		s.cfg.MCPPort = disk.MCPPort
	}
	s.cfg.MCPProjectID = disk.MCPProjectID
	s.cfg.MCPConsentVersion = disk.MCPConsentVersion
	s.cfg.MCPConsentedAt = disk.MCPConsentedAt
	s.cfg = normalizeMCPPreferences(s.cfg)
	migratedProviderKeys, migratedWebKey, err := s.migrateLegacySecrets(&disk)
	if err != nil {
		s.mu.Unlock()
		return err
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

func (s *Store) migrateLegacySecrets(disk *Config) (bool, bool, error) {
	migratedProviderKeys := false
	if len(disk.Providers) > 0 {
		providers, migrated, err := s.migrateProviderSecrets(disk.Providers)
		if err != nil {
			return false, false, err
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
			return false, false, err
		}
		s.cfg.WebSearchAPIKey = ""
		disk.WebSearchAPIKey = ""
		migratedWebKey = true
	}
	return migratedProviderKeys, migratedWebKey, nil
}

// Get returns a copy of the current Config (with backup_dir filled in).
func (s *Store) Get(ctx context.Context) (Config, error) {
	s.mu.RLock()
	c := s.cfg
	s.mu.RUnlock()
	c.BackupDir = filepath.Join(s.dir, "backups")
	return s.redactedSettingsView(c), nil
}

// ProviderConfigFor returns the stored config for a provider id (zero value if unset).
func (s *Store) ProviderConfigFor(id string) ProviderConfig {
	s.mu.RLock()
	cfg := s.cfg.Providers[id]
	s.mu.RUnlock()
	return s.runtimeProviderConfig(id, cfg)
}

// Language is the app's UI language (ko/en/ja). The built-in agent replies in
// it, so it is read per turn rather than captured at start-up.
func (s *Store) Language() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Language
}

// Set applies a partial patch, validates, persists atomically, returns the new Config.
func (s *Store) Set(ctx context.Context, p Patch) (Config, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	s.mu.RLock()
	next := s.cfg
	s.mu.RUnlock()
	if p.Language != nil {
		if !slices.Contains(validLanguages(), *p.Language) {
			return Config{}, fmt.Errorf("settings: unknown language %q", *p.Language)
		}
		next.Language = *p.Language
	}
	if p.Provider != nil {
		if !slices.Contains(ValidProviders(), *p.Provider) {
			return Config{}, fmt.Errorf("settings: unknown provider %q", *p.Provider)
		}
		next.Provider = *p.Provider
	}
	// Provider key writes are collected here and applied only after every
	// validation below has passed. The SecretStore is the one side effect Set
	// cannot roll back: writing a key (or deleting one, which an empty api_key
	// means) before an invalid theme or an out-of-range mcp_port aborts the
	// call would leave the keychain changed while the caller is told nothing
	// happened. See applyPendingSecrets below the validations.
	var pendingSecrets []pendingSecret
	if len(p.Providers) > 0 {
		// Validate every id before mutating anything. Map iteration order is
		// randomized, and a partial pass would let a later invalid id abort
		// the call after an earlier valid one had already been merged —
		// breaking the "reject means nothing changed" guarantee.
		for id := range p.Providers {
			if !slices.Contains(ValidProviders(), id) {
				return Config{}, fmt.Errorf("settings: unknown provider %q", id)
			}
		}
		merged := map[string]ProviderConfig{}
		for id, cfg := range next.Providers {
			merged[id] = cfg
		}
		for id, pp := range p.Providers {
			cfg := merged[id]
			if pp.Model != nil {
				cfg.Model = strings.TrimSpace(*pp.Model)
			}
			if pp.BaseURL != nil {
				base := strings.TrimSpace(*pp.BaseURL)
				// base_url decides where the request actually goes. Consent is
				// recorded per provider and the consent sentence names that
				// provider, so a base_url on "anthropic" would let the real
				// destination diverge from the name the writer agreed to. Only
				// "openai" — the OpenAI-compatible family, whose whole purpose
				// is pointing at OpenRouter/Ollama/LM Studio — may carry one.
				// An empty string stays allowed so a writer can clear it.
				if base != "" && id != ProviderOpenAI {
					return Config{}, fmt.Errorf("settings: base_url is only supported for provider %q, not %q", ProviderOpenAI, id)
				}
				cfg.BaseURL = base
			}
			if pp.ConsentedAt != nil {
				cfg.ConsentedAt = *pp.ConsentedAt
			}
			if pp.APIKey != nil {
				// Deferred, not written here: setSecret deletes on "", so one
				// field both sets and clears, and either is irreversible.
				pendingSecrets = append(pendingSecrets, pendingSecret{
					name:  providerAPIKeySecretName(id),
					value: strings.TrimSpace(*pp.APIKey),
				})
			}
			merged[id] = normalizeProviderConfig(id, cfg)
		}
		next.Providers = merged
	}
	if p.TypewriterDefault != nil {
		next.TypewriterDefault = *p.TypewriterDefault
	}
	if p.FocusDefault != nil {
		next.FocusDefault = *p.FocusDefault
	}
	if p.Theme != nil {
		if !slices.Contains(validThemes(), *p.Theme) {
			return Config{}, fmt.Errorf("settings: unknown theme %q", *p.Theme)
		}
		next.Theme = *p.Theme
	}
	if p.Palette != nil {
		if !slices.Contains(validPalettes(), *p.Palette) {
			return Config{}, fmt.Errorf("settings: unknown palette %q", *p.Palette)
		}
		next.Palette = *p.Palette
	}
	if p.EditorFontSize != nil {
		if *p.EditorFontSize < 15 || *p.EditorFontSize > 22 {
			return Config{}, fmt.Errorf("settings: editor_font_size out of range")
		}
		next.EditorFontSize = *p.EditorFontSize
	}
	if p.EditorLineHeight != nil {
		if *p.EditorLineHeight < 1.6 || *p.EditorLineHeight > 2.2 {
			return Config{}, fmt.Errorf("settings: editor_line_height out of range")
		}
		next.EditorLineHeight = *p.EditorLineHeight
	}
	if p.CopyProfile != nil {
		if !slices.Contains(validCopyProfiles(), *p.CopyProfile) {
			return Config{}, fmt.Errorf("settings: unknown copy_profile %q", *p.CopyProfile)
		}
		next.CopyProfile = *p.CopyProfile
	}
	if p.GitSyncDir != nil {
		next.GitSyncDir = *p.GitSyncDir
	}
	if p.GitSyncCommitTemplate != nil {
		next.GitSyncCommitTemplate = *p.GitSyncCommitTemplate
	}
	if p.FolderSyncDir != nil {
		next.FolderSyncDir = *p.FolderSyncDir
	}
	if p.FolderSyncEnabled != nil {
		next.FolderSyncEnabled = *p.FolderSyncEnabled
	}
	if p.SafetyChecklistDismissed != nil {
		next.SafetyChecklistDismissed = *p.SafetyChecklistDismissed
	}
	if p.OnboardingTourEnabled != nil {
		next.OnboardingTourEnabled = *p.OnboardingTourEnabled
	}
	if p.OnboardingTourSeenVersion != nil {
		next.OnboardingTourSeenVersion = *p.OnboardingTourSeenVersion
	}
	if p.MCPMode != nil {
		if !slices.Contains(ValidMCPModes(), *p.MCPMode) {
			return Config{}, fmt.Errorf("settings: unknown mcp_mode %q", *p.MCPMode)
		}
		next.MCPMode = *p.MCPMode
	}
	if p.MCPPort != nil {
		if *p.MCPPort < 1024 || *p.MCPPort > 65535 {
			return Config{}, fmt.Errorf("settings: mcp_port %d out of range (1024-65535)", *p.MCPPort)
		}
		next.MCPPort = *p.MCPPort
	}
	if p.MCPProjectID != nil {
		next.MCPProjectID = *p.MCPProjectID
	}
	if p.MCPConsentVersion != nil {
		next.MCPConsentVersion = *p.MCPConsentVersion
	}
	if p.MCPConsentedAt != nil {
		next.MCPConsentedAt = *p.MCPConsentedAt
	}
	if next.WebSearchProvider == "" {
		next.WebSearchProvider = "brave"
	}
	if next.Language == "" {
		next.Language = "ko"
	}
	next = normalizeEditorPreferences(next)

	if err := s.persist(next); err != nil {
		return Config{}, err
	}

	s.mu.Lock()
	s.cfg = sanitizeConfigForMemory(next)
	s.mu.Unlock()

	// Last, past every path that can still fail with nothing written. A
	// SecretStore failure here (a locked keychain) is reported to the caller
	// with the rest of the patch already applied — the ordering that keeps
	// memory and disk in step; only the key did not take.
	if err := s.applyPendingSecrets(pendingSecrets); err != nil {
		return Config{}, err
	}

	return s.Get(ctx)
}

// pendingSecret is one deferred SecretStore write from a Set patch.
type pendingSecret struct{ name, value string }

func (s *Store) applyPendingSecrets(pending []pendingSecret) error {
	for _, ps := range pending {
		// setSecret deletes on "", so one field both sets and clears.
		if err := s.setSecret(ps.name, ps.value); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) persist(next Config) error {
	next = sanitizeConfigForDisk(next)
	persistable := Config{
		Language:                    next.Language,
		Provider:                    next.Provider,
		Providers:                   next.Providers,
		TypewriterDefault:           next.TypewriterDefault,
		FocusDefault:                next.FocusDefault,
		Theme:                       next.Theme,
		Palette:                     next.Palette,
		EditorFontSize:              next.EditorFontSize,
		EditorLineHeight:            next.EditorLineHeight,
		CopyProfile:                 next.CopyProfile,
		GitSyncDir:                  next.GitSyncDir,
		GitSyncCommitTemplate:       next.GitSyncCommitTemplate,
		FolderSyncDir:               next.FolderSyncDir,
		FolderSyncEnabled:           next.FolderSyncEnabled,
		SafetyChecklistDismissed:    next.SafetyChecklistDismissed,
		OnboardingTourEnabled:       next.OnboardingTourEnabled,
		OnboardingTourSeenVersion:   next.OnboardingTourSeenVersion,
		AIDataSharingConsentVersion: next.AIDataSharingConsentVersion,
		AIDataSharingConsentedAt:    next.AIDataSharingConsentedAt,
		WebSearchProvider:           next.WebSearchProvider,
		MCPMode:                     next.MCPMode,
		MCPPort:                     next.MCPPort,
		MCPProjectID:                next.MCPProjectID,
		MCPConsentVersion:           next.MCPConsentVersion,
		MCPConsentedAt:              next.MCPConsentedAt,
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
		cfg = normalizeProviderConfig(id, cfg)
		out[id] = cfg
	}
	return out, migrated, nil
}

func (s *Store) setSecret(name, value string) error {
	if value == "" {
		return s.secrets.Delete(name)
	}
	return s.secrets.Set(name, value)
}

func (s *Store) runtimeProviderConfig(provider string, cfg ProviderConfig) ProviderConfig {
	cfg = normalizeProviderConfig(provider, cfg)
	cfg.APIKey = ""
	cfg.APIKeySet = false
	secret, ok, err := s.secrets.Get(providerAPIKeySecretName(provider))
	if err != nil {
		// A locked or access-denied keychain currently reads downstream as
		// "no key": providers.list reports the provider unconfigured and
		// providers.test fails with provider_not_configured, so the writer's
		// rational move is to retype a key the store is still refusing. Log it
		// so the failure is at least diagnosable. Telling the two apart in the
		// UI needs a reason code and a pane to show it in — that belongs to
		// #94, which builds the provider settings UI.
		log.Printf("settings: reading the stored key for provider %q failed; treating it as unset: %v", provider, err)
		return cfg
	}
	if !ok {
		return cfg
	}
	cfg.APIKey = secret
	cfg.APIKeySet = true
	return cfg
}

func normalizeProviderConfig(provider string, cfg ProviderConfig) ProviderConfig {
	if provider == ProviderOpenAICodex && (cfg.Model == "" || cfg.Model == "gpt-5.3-codex") {
		cfg.Model = DefaultOpenAICodexModel
	}
	// base_url is not scrubbed here on purpose. Set rejects it for every id but
	// "openai" (see the merge loop), which is the path the app writes through.
	// Doing it here too would also strip the base_url off a retired 1.0 entry
	// like "openrouter", which TestSet_leavesRetiredCompanionSettingsOnDisk
	// requires be left exactly as found.
	return cfg
}

func normalizeEditorPreferences(c Config) Config {
	if !slices.Contains(validThemes(), c.Theme) {
		c.Theme = "system"
	}
	if !slices.Contains(validPalettes(), c.Palette) {
		c.Palette = defaultPalette
	}
	if c.EditorFontSize < 15 || c.EditorFontSize > 22 {
		c.EditorFontSize = 20
	}
	if c.EditorLineHeight < 1.6 || c.EditorLineHeight > 2.2 {
		c.EditorLineHeight = 1.92
	}
	if !slices.Contains(validCopyProfiles(), c.CopyProfile) {
		c.CopyProfile = "plain"
	}
	return normalizeMCPPreferences(c)
}

// normalizeMCPPreferences keeps MCP settings safe by construction: an
// unrecognized mode falls back to off (never to an open server), and an
// out-of-range port falls back to the default so a bad value cannot make the
// server unreachable in a way the writer cannot see.
func normalizeMCPPreferences(c Config) Config {
	if !slices.Contains(ValidMCPModes(), c.MCPMode) {
		c.MCPMode = MCPModeOff
	}
	if c.MCPPort < 1024 || c.MCPPort > 65535 {
		c.MCPPort = DefaultMCPPort
	}
	return c
}

func (s *Store) redactedSettingsView(c Config) Config {
	c = normalizeEditorPreferences(c)
	c = sanitizeConfigForMemory(c)
	providers := map[string]ProviderConfig{}
	for id, cfg := range c.Providers {
		ok, err := s.secrets.Exists(providerAPIKeySecretName(id))
		if err == nil {
			cfg.APIKeySet = ok
		}
		cfg = normalizeProviderConfig(id, cfg)
		providers[id] = cfg
	}
	c.Providers = providers
	webKeySet, err := s.secrets.Exists(webSearchAPIKeySecretName)
	if err == nil {
		c.WebSearchAPIKeySet = webKeySet
	}
	// Presence only — never the value: settings.get must not read secrets, and
	// the check has to see the 0600 file fallback too.
	c.MCPTokenSet = s.MCPTokenExists()
	return c
}

func sanitizeConfigForMemory(c Config) Config {
	c.WebSearchAPIKey = ""
	providers := map[string]ProviderConfig{}
	for id, cfg := range c.Providers {
		cfg.APIKey = ""
		providers[id] = cfg
	}
	c.Providers = providers
	return c
}

func sanitizeConfigForDisk(c Config) Config {
	c = sanitizeConfigForMemory(c)
	c.WebSearchAPIKeySet = false
	c.MCPTokenSet = false
	providers := map[string]ProviderConfig{}
	for id, cfg := range c.Providers {
		cfg.APIKeySet = false
		providers[id] = cfg
	}
	c.Providers = providers
	return c
}
