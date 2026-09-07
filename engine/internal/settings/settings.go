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
	// AgentSelfReviewEnabled governs the built-in agent's self-improvement
	// pass (#98 Task 10): after a turn that actually did work, the agent is
	// asked separately whether anything is worth recording as a skill. It
	// defaults to ON — the loop the feature is named for is the feature —
	// and the writer can switch it off in Settings → Skills. Off means no
	// second provider call and no skill written without the writer asking.
	AgentSelfReviewEnabled bool `json:"agent_self_review_enabled"`
	// MemoryToolsEnabled and SkillToolsEnabled are the writer's tool budget
	// (#99). Every tool's whole JSON schema travels in every request, so the
	// nineteen tools Linetta serves are a standing cost on every turn — the
	// memory and skills tools alone are about a fifth of it. These two
	// switches remove a group from what any agent is offered: the built-in
	// panel's server and the MCP server an external client connects to both
	// read them, so there is one answer to "which tools exist", not two.
	//
	// Both default to ON: a writer who never opens this pane keeps exactly
	// the tool set 1.2 shipped.
	//
	// Off removes the tools AND the instructions that name them. A prompt
	// that tells an agent to record a skill with a tool it has not been given
	// is worse than the tool's cost — see agent/prompt.go. What survives
	// differs by group, and deliberately: the memory documents are still
	// pasted (they are content, and the writer still edits them in Settings),
	// while the skills LIST goes with its tools, because a list of names is
	// only ever a pointer to bodies linetta_read_skill would fetch.
	MemoryToolsEnabled bool `json:"memory_tools_enabled"`
	SkillToolsEnabled  bool `json:"skill_tools_enabled"`
	// ToolBudget is what those two switches actually cost, measured from the
	// tool set the engine would register right now — never a table written
	// down here. Derived at settings.get time and never persisted, for the
	// same reason as the legacy-plaintext fields below: a number on disk
	// outlives the tool set it described.
	//
	// Nil on a build that wired no measurement (mobile, and every test that
	// does not ask for one), which is why the Settings pane treats it as
	// optional rather than as a promise.
	ToolBudget *ToolBudget `json:"tool_budget,omitempty"`
	// The four fields below report one thing settings.get cannot get from
	// anywhere else: a pre-1.0 plaintext api_key that is *still in
	// settings.json* because load() had nowhere to move it to (#113).
	//
	// They are derived at load time and never persisted — deliberately absent
	// from persist()'s allowlist. A flag written to disk would outlive the
	// condition it describes: it survives a reinstall onto a platform that
	// does have a secret store, and it survives the writer deleting the key,
	// leaving a warning about a file that no longer says what it claims.
	// Recomputing it every load costs nothing and can never be stale.
	//
	// LegacyPlaintextProviders is the sorted list of provider ids whose key
	// is still in the file.
	LegacyPlaintextProviders []string `json:"legacy_plaintext_providers,omitempty"`
	// LegacyPlaintextWebSearch says the same about the web-search key.
	LegacyPlaintextWebSearch bool `json:"legacy_plaintext_web_search,omitempty"`
	// LegacyPlaintextReason is "unsupported" when this platform has no secret
	// backend at all, "error" when it has one and it refused. The two need
	// different sentences: the first is permanent and the writer's only move
	// is to delete the key, the second may work on the next launch.
	LegacyPlaintextReason string `json:"legacy_plaintext_reason,omitempty"`
	// LegacyPlaintextPath is the settings.json holding them, so the notice can
	// name the file instead of asking the writer to guess where it lives.
	LegacyPlaintextPath string `json:"legacy_plaintext_path,omitempty"`
}

// Why a legacy plaintext key could not be moved into the secret store.
const (
	// LegacyPlaintextReasonUnsupported: this build has no secret backend.
	// Permanent until someone writes one; see secrets_other.go.
	LegacyPlaintextReasonUnsupported = "unsupported"
	// LegacyPlaintextReasonError: there is a backend and it refused — a
	// locked or access-denied Keychain, say. Possibly transient.
	LegacyPlaintextReasonError = "error"
)

// legacyPlaintextKeys is the pre-1.0 plaintext credentials load() found in
// settings.json and could not move into the SecretStore.
//
// It lives on the Store rather than in Config on purpose. Config's copies get
// blanked by sanitizeConfigForMemory the first time anything is saved, and a
// blanked copy is exactly what persist() would then write back over the file —
// silently destroying the credential. Keeping the values here, out of every
// sanitize path, is what lets persist() put them back byte for byte.
type legacyPlaintextKeys struct {
	providers map[string]string // provider id → the key still in settings.json
	webSearch string            // the web-search key still in settings.json
	reason    string            // LegacyPlaintextReason*
}

func (l legacyPlaintextKeys) any() bool {
	return len(l.providers) > 0 || l.webSearch != ""
}

// clone returns a copy that is safe to hold after s.mu is released.
//
// Assigning the struct is not enough: the copy shares the same providers map,
// and forgetLegacyPlaintextFor deletes from it under mu.Lock. A settings.get
// arriving on another goroutine — RPC dispatch is one goroutine per message
// (internal/rpc/server.go) — would then be iterating that map while it is
// written, which Go does not merely race on but aborts the process for:
// "fatal error: concurrent map iteration and map write". Every read of
// s.legacyPlaintext that outlives the lock must go through here.
func (l legacyPlaintextKeys) clone() legacyPlaintextKeys {
	if len(l.providers) == 0 {
		// Nil rather than an empty map, so a clone of nothing allocates
		// nothing and compares the way the zero value does.
		l.providers = nil
		return l
	}
	providers := make(map[string]string, len(l.providers))
	for id, key := range l.providers {
		providers[id] = key
	}
	l.providers = providers
	return l
}

func (l legacyPlaintextKeys) providerIDs() []string {
	if len(l.providers) == 0 {
		return nil
	}
	ids := make([]string, 0, len(l.providers))
	for id := range l.providers {
		ids = append(ids, id)
	}
	// Sorted because map order is randomized and this list reaches the UI:
	// an unordered one would reshuffle the notice on every settings.get.
	slices.Sort(ids)
	return ids
}

// note records one setSecret failure. A real error outranks "unsupported":
// if any part of the migration failed for a reason a backend could recover
// from, the writer should be told that rather than that their platform has
// no storage — which would be a lie on macOS.
func (l *legacyPlaintextKeys) note(err error) {
	if errors.Is(err, ErrSecretStoreUnsupported) {
		if l.reason == "" {
			l.reason = LegacyPlaintextReasonUnsupported
		}
		return
	}
	log.Printf("settings: moving a legacy plaintext key into the secret store failed; leaving it in settings.json: %v", err)
	l.reason = LegacyPlaintextReasonError
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
	AgentSelfReviewEnabled    *bool                    `json:"agent_self_review_enabled,omitempty"`
	MemoryToolsEnabled        *bool                    `json:"memory_tools_enabled,omitempty"`
	SkillToolsEnabled         *bool                    `json:"skill_tools_enabled,omitempty"`
	// ClearLegacyPlaintextKeys, when true, deletes the plaintext keys load()
	// could not migrate from settings.json (#113). One-way and destructive:
	// on a platform with no secret store the file is the only place those
	// keys exist, so this must never fire on its own — the Settings notice
	// puts it behind a button the writer has to press.
	ClearLegacyPlaintextKeys *bool `json:"clear_legacy_plaintext_keys,omitempty"`
}

// Store reads and writes the settings file with internal locking.
type Store struct {
	mu      sync.RWMutex // protects cfg reads
	writeMu sync.Mutex   // serializes Set: validation → cfg update → disk write
	cfg     Config
	dir     string
	secrets SecretStore
	// legacyPlaintext is what load() could not move out of settings.json.
	// Guarded by mu. Empty on every healthy install; non-empty only on a
	// platform with no secret backend, or after one refused. See #113.
	legacyPlaintext legacyPlaintextKeys
	// toolBudget measures what the tool set costs, for settings.get (#99).
	// Guarded by mu; nil until WithToolBudget wires one. See toolbudget.go.
	toolBudget ToolBudgetFunc
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
		// On by default: an agent that never notices what it learned is the
		// feature not shipping. See Config.AgentSelfReviewEnabled.
		AgentSelfReviewEnabled: true,
		// On by default: a writer who never opens the tool-budget pane keeps
		// exactly the tool set 1.2 shipped. See Config.MemoryToolsEnabled.
		MemoryToolsEnabled: true,
		SkillToolsEnabled:  true,
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
	// Presence-guarded, exactly like onboarding_tour_enabled above and for the
	// same reason: the default is true, so a plain assignment would read a
	// deliberate `false` and a settings.json written by a build that predates
	// the key as the same thing — and silently turn the writer's switch back
	// on at every restart.
	if _, ok := raw["agent_self_review_enabled"]; ok {
		s.cfg.AgentSelfReviewEnabled = disk.AgentSelfReviewEnabled
	}
	// The two tool-budget switches (#99), presence-guarded for the same
	// reason: both default to true, so a plain assignment would read the
	// writer's deliberate `false` and a settings.json from a build that
	// predates the key as the same thing, and hand the tools back at every
	// restart.
	if _, ok := raw["memory_tools_enabled"]; ok {
		s.cfg.MemoryToolsEnabled = disk.MemoryToolsEnabled
	}
	if _, ok := raw["skill_tools_enabled"]; ok {
		s.cfg.SkillToolsEnabled = disk.SkillToolsEnabled
	}
	s.cfg = normalizeMCPPreferences(s.cfg)
	// Migration cannot fail the load any more (#113). A pre-1.0 plaintext key
	// on a platform with no secret store used to return an error from here,
	// which meant the writer could not open their settings at all — while the
	// key stayed in the file regardless. What comes back instead is a record
	// of what could not be moved, which Get() surfaces so Settings can say so
	// and offer to delete it.
	migratedProviderKeys, migratedWebKey := s.migrateLegacySecrets(&disk)
	next := s.cfg
	legacy := s.legacyPlaintext.clone()
	s.mu.Unlock()
	if migratedWebKey || migratedProviderKeys {
		// persistWith, not persist: the rewrite that drops the keys that *did*
		// move must put back the ones that did not, or a partly-failed
		// migration would delete the keys it could not save.
		if err := s.persistWith(next, legacy); err != nil {
			return err
		}
	}
	return nil
}

// migrateLegacySecrets moves pre-1.0 plaintext keys out of settings.json and
// into the SecretStore. It reports what moved (so load() knows whether the
// file needs rewriting) and records what could not in s.legacyPlaintext.
//
// Called with s.mu held.
func (s *Store) migrateLegacySecrets(disk *Config) (bool, bool) {
	legacy := legacyPlaintextKeys{}
	migratedProviderKeys := false
	if len(disk.Providers) > 0 {
		providers, migrated := s.migrateProviderSecrets(disk.Providers, &legacy)
		s.cfg.Providers = providers
		if migrated {
			disk.Providers = providers
			migratedProviderKeys = true
		}
	}
	migratedWebKey := false
	if disk.WebSearchAPIKey != "" {
		if err := s.setSecret(webSearchAPIKeySecretName, disk.WebSearchAPIKey); err != nil {
			legacy.note(err)
			legacy.webSearch = disk.WebSearchAPIKey
		} else {
			s.cfg.WebSearchAPIKey = ""
			disk.WebSearchAPIKey = ""
			migratedWebKey = true
		}
	}
	s.legacyPlaintext = legacy
	return migratedProviderKeys, migratedWebKey
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

// AgentSelfReviewEnabled reports whether the built-in agent may run its
// self-improvement pass after a working turn. Read per turn like Language,
// not captured at start-up: switching it off in Settings has to take effect
// on the writer's very next message, not after a restart.
func (s *Store) AgentSelfReviewEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.AgentSelfReviewEnabled
}

// MemoryToolsEnabled reports whether linetta_edit_memory is offered (#99).
//
// Read where the tool set is built, not captured at start-up, for a stronger
// reason than Language's: the built-in agent builds a fresh in-memory server
// for every run, and the MCP host builds one per HTTP session, so reading it
// there is what makes the switch take effect on the writer's next message and
// their client's next connection rather than at the next restart.
func (s *Store) MemoryToolsEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.MemoryToolsEnabled
}

// SkillToolsEnabled reports whether linetta_read_skill and linetta_edit_skill
// are offered (#99). Read per server build, like MemoryToolsEnabled.
func (s *Store) SkillToolsEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.SkillToolsEnabled
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
	if p.AgentSelfReviewEnabled != nil {
		next.AgentSelfReviewEnabled = *p.AgentSelfReviewEnabled
	}
	if p.MemoryToolsEnabled != nil {
		next.MemoryToolsEnabled = *p.MemoryToolsEnabled
	}
	if p.SkillToolsEnabled != nil {
		next.SkillToolsEnabled = *p.SkillToolsEnabled
	}
	if next.WebSearchProvider == "" {
		next.WebSearchProvider = "brave"
	}
	if next.Language == "" {
		next.Language = "ko"
	}
	next = normalizeEditorPreferences(next)

	// The one path that deletes a credential the writer owns (#113). The file
	// is rewritten without the plaintext keys first and the in-memory record
	// dropped only once that write has landed: the other order would take the
	// notice off the screen while the keys were still on disk.
	s.mu.RLock()
	legacy := s.legacyPlaintext.clone()
	s.mu.RUnlock()
	clearLegacy := p.ClearLegacyPlaintextKeys != nil && *p.ClearLegacyPlaintextKeys
	if clearLegacy {
		legacy = legacyPlaintextKeys{}
	}
	if err := s.persistWith(next, legacy); err != nil {
		return Config{}, err
	}

	s.mu.Lock()
	s.cfg = sanitizeConfigForMemory(next)
	if clearLegacy {
		s.legacyPlaintext = legacyPlaintextKeys{}
	}
	s.mu.Unlock()

	// Last, past every path that can still fail with nothing written. A
	// SecretStore failure here (a locked keychain) is reported to the caller
	// with the rest of the patch already applied — the ordering that keeps
	// memory and disk in step; only the key did not take.
	if err := s.applyPendingSecrets(pendingSecrets); err != nil {
		return Config{}, err
	}
	// A key the writer has just stored or cleared supersedes the plaintext one
	// still in the file for that provider, so the file stops carrying it. This
	// is after applyPendingSecrets because only a write that actually landed
	// counts, and it is why the rewrite is a second one.
	if s.forgetLegacyPlaintextFor(pendingSecrets) {
		if err := s.persist(next); err != nil {
			return Config{}, err
		}
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

// persist writes next, carrying forward whatever pre-1.0 plaintext keys the
// file still holds. Every caller but the one destructive path wants this.
func (s *Store) persist(next Config) error {
	s.mu.RLock()
	legacy := s.legacyPlaintext.clone()
	s.mu.RUnlock()
	return s.persistWith(next, legacy)
}

// persistWith writes next plus exactly the legacy plaintext keys given.
// Passing an empty legacy is how the keys get deleted from the file, and the
// only caller that does is the explicit clear_legacy_plaintext_keys patch.
func (s *Store) persistWith(next Config, legacy legacyPlaintextKeys) error {
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
		// The line everyone forgets: a field missing from this literal is
		// never written to disk, so the writer's choice survives until the
		// next restart and no further.
		AgentSelfReviewEnabled: next.AgentSelfReviewEnabled,
		MemoryToolsEnabled:     next.MemoryToolsEnabled,
		SkillToolsEnabled:      next.SkillToolsEnabled,
	}
	// sanitizeConfigForDisk has just blanked every api_key, which is right for
	// every key the SecretStore holds. It is wrong for the pre-1.0 ones it
	// refused: for those the file is the only copy, and a save of an unrelated
	// preference must not be what destroys one (#113). Put them back.
	persistable = withLegacyPlaintextKeys(persistable, legacy)
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

// withLegacyPlaintextKeys puts the un-migrated plaintext keys back into a
// config on its way to disk, so the file keeps saying what it already said.
func withLegacyPlaintextKeys(c Config, legacy legacyPlaintextKeys) Config {
	if !legacy.any() {
		return c
	}
	if legacy.webSearch != "" {
		c.WebSearchAPIKey = legacy.webSearch
	}
	if len(legacy.providers) > 0 {
		providers := map[string]ProviderConfig{}
		for id, cfg := range c.Providers {
			providers[id] = cfg
		}
		for id, key := range legacy.providers {
			cfg := providers[id]
			cfg.APIKey = key
			providers[id] = cfg
		}
		c.Providers = providers
	}
	return c
}

// forgetLegacyPlaintextFor drops the record of any plaintext key the writer
// has just deliberately replaced or cleared through settings.set. Storing a
// new key for a provider — or clearing that provider's key, which is the same
// button saying the opposite — is an explicit decision about that credential,
// and after it the plaintext copy is not something to preserve. It returns
// whether anything changed, so the caller knows to rewrite the file.
func (s *Store) forgetLegacyPlaintextFor(pending []pendingSecret) bool {
	if len(pending) == 0 {
		return false
	}
	names := map[string]bool{}
	for _, ps := range pending {
		names[ps.name] = true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for id := range s.legacyPlaintext.providers {
		if names[providerAPIKeySecretName(id)] {
			delete(s.legacyPlaintext.providers, id)
			changed = true
		}
	}
	if !s.legacyPlaintext.any() {
		s.legacyPlaintext = legacyPlaintextKeys{}
	}
	return changed
}

func (s *Store) migrateProviderSecrets(providers map[string]ProviderConfig, legacy *legacyPlaintextKeys) (map[string]ProviderConfig, bool) {
	out := map[string]ProviderConfig{}
	migrated := false
	for id, cfg := range providers {
		if cfg.APIKey != "" {
			if err := s.setSecret(providerAPIKeySecretName(id), cfg.APIKey); err != nil {
				// The key is recorded and left exactly where it is — in cfg
				// and in the file. Blanking it in memory while it stayed on
				// disk would make the two disagree about what the writer has;
				// blanking it on disk would be the app destroying a
				// credential nobody asked it to touch (#113).
				legacy.note(err)
				if legacy.providers == nil {
					legacy.providers = map[string]string{}
				}
				legacy.providers[id] = cfg.APIKey
			} else {
				cfg.APIKey = ""
				migrated = true
			}
		}
		cfg.APIKeySet = false
		cfg = normalizeProviderConfig(id, cfg)
		out[id] = cfg
	}
	return out, migrated
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
	// What the tool set the two switches govern actually costs (#99).
	// Measured here rather than stored, so it can never describe a tool set
	// the engine no longer serves.
	c.ToolBudget = s.toolBudgetView(c.MemoryToolsEnabled, c.SkillToolsEnabled)
	// Which keys are still in plain text in settings.json, and why (#113).
	// Names and a reason only — the values stay where they are, and the point
	// of the notice is that the writer already has them.
	s.mu.RLock()
	legacy := s.legacyPlaintext.clone()
	s.mu.RUnlock()
	if legacy.any() {
		c.LegacyPlaintextProviders = legacy.providerIDs()
		c.LegacyPlaintextWebSearch = legacy.webSearch != ""
		c.LegacyPlaintextReason = legacy.reason
		c.LegacyPlaintextPath = filepath.Join(s.dir, "settings.json")
	}
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
	// Derived, never written: a measurement on disk outlives the tool set it
	// measured. persist()'s allowlist already leaves it out; this is the
	// second lock on the same door, matching the two flags above.
	c.ToolBudget = nil
	providers := map[string]ProviderConfig{}
	for id, cfg := range c.Providers {
		cfg.APIKeySet = false
		providers[id] = cfg
	}
	c.Providers = providers
	return c
}
