package settings

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func newStoreOnTemp(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("LINETTA_HOME", dir)
	s, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestLoad_missingFileReturnsDefaults(t *testing.T) {
	s := newStoreOnTemp(t)
	got, err := s.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Provider != "openai-codex" {
		t.Errorf("provider = %q, want openai-codex", got.Provider)
	}
	if got.Language != "ko" {
		t.Errorf("language = %q, want ko", got.Language)
	}
	if got.TypewriterDefault != false {
		t.Errorf("typewriter_default = %v", got.TypewriterDefault)
	}
	if !got.OnboardingTourEnabled {
		t.Errorf("onboarding_tour_enabled = false, want true")
	}
	if got.OnboardingTourSeenVersion != "" {
		t.Errorf("onboarding_tour_seen_version = %q, want empty", got.OnboardingTourSeenVersion)
	}
	if got.BackupDir == "" {
		t.Error("backup_dir empty")
	}
	if got.Theme != "system" {
		t.Errorf("theme = %q, want system", got.Theme)
	}
	if got.EditorFontSize != 20 {
		t.Errorf("editor_font_size = %d, want 20", got.EditorFontSize)
	}
	if got.EditorLineHeight != 1.92 {
		t.Errorf("editor_line_height = %v, want 1.92", got.EditorLineHeight)
	}
	if got.CopyProfile != "plain" {
		t.Errorf("copy_profile = %q, want plain", got.CopyProfile)
	}
}

func TestSet_partialPatchPreservesUntouchedFields(t *testing.T) {
	s := newStoreOnTemp(t)
	if _, err := s.Set(context.Background(), Patch{TypewriterDefault: boolPtr(true)}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, _ := s.Get(context.Background())
	if got.Provider != "openai-codex" {
		t.Errorf("provider mutated to %q", got.Provider)
	}
	if !got.TypewriterDefault {
		t.Errorf("typewriter_default not persisted")
	}
}

func TestSet_persistsToDisk(t *testing.T) {
	s := newStoreOnTemp(t)
	if _, err := s.Set(context.Background(), Patch{Provider: strPtr("openai-codex")}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	s2, err := New()
	if err != nil {
		t.Fatalf("re-New: %v", err)
	}
	got, _ := s2.Get(context.Background())
	if got.Provider != "openai-codex" {
		t.Errorf("provider not persisted across reload: %q", got.Provider)
	}
}

func TestSet_rejectsUnknownProvider(t *testing.T) {
	s := newStoreOnTemp(t)
	_, err := s.Set(context.Background(), Patch{Provider: strPtr("bad-provider")})
	if err == nil {
		t.Error("expected validation error")
	}
}

func TestSet_language_persists(t *testing.T) {
	s := newStoreOnTemp(t)
	if _, err := s.Set(context.Background(), Patch{Language: strPtr("ja")}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, _ := s.Get(context.Background())
	if got.Language != "ja" {
		t.Errorf("language in-memory = %q, want ja", got.Language)
	}
	s2, err := New()
	if err != nil {
		t.Fatalf("re-New: %v", err)
	}
	reloaded, _ := s2.Get(context.Background())
	if reloaded.Language != "ja" {
		t.Errorf("language not persisted across reload: %q", reloaded.Language)
	}
}

func TestSet_rejectsUnknownLanguage(t *testing.T) {
	s := newStoreOnTemp(t)
	_, err := s.Set(context.Background(), Patch{Language: strPtr("fr")})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestLoad_corruptFileReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINETTA_HOME", dir)
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte("not-json"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s, err := New()
	if err != nil {
		t.Fatalf("New on corrupt: %v", err)
	}
	got, _ := s.Get(context.Background())
	if got.Provider != "openai-codex" {
		t.Errorf("did not fall back to defaults: %+v", got)
	}
}

func TestSet_concurrentSerialized(t *testing.T) {
	s := newStoreOnTemp(t)
	const N = 20
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			tw := i%2 == 0
			_, _ = s.Set(context.Background(), Patch{TypewriterDefault: &tw})
		}(i)
	}
	wg.Wait()
	// Reload from disk and verify it equals the in-memory state.
	s2, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	onDisk, _ := s2.Get(context.Background())
	inMem, _ := s.Get(context.Background())
	if onDisk.TypewriterDefault != inMem.TypewriterDefault {
		t.Fatalf("disk/memory mismatch after concurrent Set: disk=%v mem=%v", onDisk.TypewriterDefault, inMem.TypewriterDefault)
	}
}

func TestSet_focusDefault_persists(t *testing.T) {
	s := newStoreOnTemp(t)
	if _, err := s.Set(context.Background(), Patch{FocusDefault: boolPtr(true)}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, _ := s.Get(context.Background())
	if !got.FocusDefault {
		t.Errorf("focus_default not applied in-memory: %+v", got)
	}
	// Reload from disk and verify it survived.
	s2, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	reloaded, _ := s2.Get(context.Background())
	if !reloaded.FocusDefault {
		t.Errorf("focus_default not persisted across reload: %+v", reloaded)
	}
}

func TestSet_editorPreferences_persistAndValidate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINETTA_HOME", dir)
	secrets := NewMemorySecretStore()
	s, err := NewWithSecretStore(secrets)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	if _, err := s.Set(ctx, Patch{
		Theme:            strPtr("dark"),
		EditorFontSize:   intPtr(18),
		EditorLineHeight: floatPtr(2.05),
		CopyProfile:      strPtr("munpia"),
	}); err != nil {
		t.Fatalf("Set editor prefs: %v", err)
	}
	got, _ := s.Get(ctx)
	if got.Theme != "dark" || got.EditorFontSize != 18 || got.EditorLineHeight != 2.05 || got.CopyProfile != "munpia" {
		t.Fatalf("editor prefs not applied in memory: %+v", got)
	}
	s2, err := NewWithSecretStore(secrets)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	reloaded, _ := s2.Get(ctx)
	if reloaded.Theme != "dark" || reloaded.EditorFontSize != 18 || reloaded.EditorLineHeight != 2.05 || reloaded.CopyProfile != "munpia" {
		t.Fatalf("editor prefs not persisted: %+v", reloaded)
	}
	if _, err := s.Set(ctx, Patch{Theme: strPtr("sepia")}); err == nil {
		t.Fatalf("expected invalid theme error")
	}
	if _, err := s.Set(ctx, Patch{EditorFontSize: intPtr(30)}); err == nil {
		t.Fatalf("expected invalid font size error")
	}
	if _, err := s.Set(ctx, Patch{EditorLineHeight: floatPtr(3.0)}); err == nil {
		t.Fatalf("expected invalid line height error")
	}
	if _, err := s.Set(ctx, Patch{CopyProfile: strPtr("fax")}); err == nil {
		t.Fatalf("expected invalid copy profile error")
	}
}

func TestSet_gitSync_persists(t *testing.T) {
	s := newStoreOnTemp(t)
	dir := "/tmp/linetta-test-repo"
	tmpl := "Synced {date}"
	if _, err := s.Set(context.Background(), Patch{
		GitSyncDir:            strPtr(dir),
		GitSyncCommitTemplate: strPtr(tmpl),
	}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, _ := s.Get(context.Background())
	if got.GitSyncDir != dir {
		t.Errorf("git_sync_dir in-memory = %q, want %q", got.GitSyncDir, dir)
	}
	if got.GitSyncCommitTemplate != tmpl {
		t.Errorf("git_sync_commit_template in-memory = %q, want %q", got.GitSyncCommitTemplate, tmpl)
	}
	// Reload from disk and verify it survived.
	s2, err := New()
	if err != nil {
		t.Fatalf("re-New: %v", err)
	}
	reloaded, _ := s2.Get(context.Background())
	if reloaded.GitSyncDir != dir {
		t.Errorf("git_sync_dir not persisted across reload: %q", reloaded.GitSyncDir)
	}
	if reloaded.GitSyncCommitTemplate != tmpl {
		t.Errorf("git_sync_commit_template not persisted across reload: %q", reloaded.GitSyncCommitTemplate)
	}
}

func TestSet_gitSync_emptyMeansDisabled(t *testing.T) {
	s := newStoreOnTemp(t)
	got, err := s.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.GitSyncDir != "" {
		t.Errorf("default git_sync_dir = %q, want empty", got.GitSyncDir)
	}
}

func TestSet_safetyChecklistDismissed_persists(t *testing.T) {
	s := newStoreOnTemp(t)
	if _, err := s.Set(context.Background(), Patch{SafetyChecklistDismissed: boolPtr(true)}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, _ := s.Get(context.Background())
	if !got.SafetyChecklistDismissed {
		t.Errorf("safety_checklist_dismissed not applied in-memory: %+v", got)
	}

	s2, err := New()
	if err != nil {
		t.Fatalf("re-New: %v", err)
	}
	reloaded, _ := s2.Get(context.Background())
	if !reloaded.SafetyChecklistDismissed {
		t.Errorf("safety_checklist_dismissed not persisted across reload: %+v", reloaded)
	}
}

func TestLoad_legacyFileDefaultsOnboardingTourEnabled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINETTA_HOME", dir)
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"provider":"openai","language":"en"}`), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, _ := s.Get(context.Background())
	if !got.OnboardingTourEnabled {
		t.Fatalf("legacy settings should default onboarding_tour_enabled to true: %+v", got)
	}
}

func TestSet_onboardingTour_persistsAndMerges(t *testing.T) {
	s := newStoreOnTemp(t)
	ctx := context.Background()
	if _, err := s.Set(ctx, Patch{
		OnboardingTourEnabled:     boolPtr(false),
		OnboardingTourSeenVersion: strPtr("library-workspace-v1"),
	}); err != nil {
		t.Fatalf("Set onboarding: %v", err)
	}
	if _, err := s.Set(ctx, Patch{Language: strPtr("ja")}); err != nil {
		t.Fatalf("Set language: %v", err)
	}
	got, _ := s.Get(ctx)
	if got.OnboardingTourEnabled {
		t.Errorf("onboarding_tour_enabled not applied in-memory: %+v", got)
	}
	if got.OnboardingTourSeenVersion != "library-workspace-v1" {
		t.Errorf("onboarding_tour_seen_version in-memory = %q", got.OnboardingTourSeenVersion)
	}
	if got.Language != "ja" {
		t.Errorf("language patch did not merge: %+v", got)
	}

	s2, err := New()
	if err != nil {
		t.Fatalf("re-New: %v", err)
	}
	reloaded, _ := s2.Get(ctx)
	if reloaded.OnboardingTourEnabled || reloaded.OnboardingTourSeenVersion != "library-workspace-v1" || reloaded.Language != "ja" {
		t.Errorf("onboarding tour settings not persisted across reload: %+v", reloaded)
	}
}

func TestSet_webSearchConfig_persists(t *testing.T) {
	s := newStoreOnTemp(t)
	if _, err := s.Set(context.Background(), Patch{
		WebSearchProvider: strPtr("perplexity"),
		WebSearchAPIKey:   strPtr("secret-key"),
	}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, _ := s.Get(context.Background())
	if got.WebSearchProvider != "perplexity" {
		t.Errorf("web_search_provider in-memory = %q", got.WebSearchProvider)
	}
	if got.WebSearchAPIKey != "" {
		t.Errorf("web_search_api_key should be redacted in settings view, got %q", got.WebSearchAPIKey)
	}
	if !got.WebSearchAPIKeySet {
		t.Errorf("web_search_api_key_set = false")
	}
	if s.WebSearchAPIKey() != "secret-key" {
		t.Errorf("web search secret not resolved")
	}

	s2, err := NewWithSecretStore(s.secrets)
	if err != nil {
		t.Fatalf("re-New: %v", err)
	}
	reloaded, _ := s2.Get(context.Background())
	if reloaded.WebSearchProvider != "perplexity" || !reloaded.WebSearchAPIKeySet || s2.WebSearchAPIKey() != "secret-key" {
		t.Errorf("web search config not persisted: %+v", reloaded)
	}
}

func TestSet_rejectsUnknownWebSearchProvider(t *testing.T) {
	s := newStoreOnTemp(t)
	if _, err := s.Set(context.Background(), Patch{WebSearchProvider: strPtr("unknown")}); err == nil {
		t.Fatal("expected validation error")
	}
}

func boolPtr(v bool) *bool        { return &v }
func intPtr(v int) *int           { return &v }
func floatPtr(v float64) *float64 { return &v }
func strPtr(v string) *string     { return &v }

func TestProvidersBackwardCompatLoadAndResolve(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINETTA_HOME", dir)
	// Legacy file with no `providers` key must still load and resolve.
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"provider":"anthropic"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rp := s.Resolve()
	if rp.Provider != "anthropic" {
		t.Fatalf("provider=%q, want anthropic", rp.Provider)
	}
	if rp.Model != "" || rp.APIKey != "" || rp.CliPath != "" {
		t.Fatalf("expected empty per-provider fields, got %+v", rp)
	}
}

func TestResolveOpenAICodexDefaultsToChatGPTAccountModel(t *testing.T) {
	s := newStoreOnTemp(t)

	rp := s.Resolve()
	if rp.Provider != ProviderOpenAICodex {
		t.Fatalf("provider=%q, want %s", rp.Provider, ProviderOpenAICodex)
	}
	if rp.Model != DefaultOpenAICodexModel {
		t.Fatalf("model=%q, want %q", rp.Model, DefaultOpenAICodexModel)
	}
}

func TestResolveOpenAICodexKeepsExplicitModel(t *testing.T) {
	s := newStoreOnTemp(t)
	ctx := context.Background()
	custom := "gpt-5.5"
	if _, err := s.Set(ctx, Patch{
		Providers: map[string]ProviderConfig{
			ProviderOpenAICodex: {Model: custom},
		},
	}); err != nil {
		t.Fatal(err)
	}

	if got := s.Resolve().Model; got != custom {
		t.Fatalf("model=%q, want explicit %q", got, custom)
	}
}

func TestSetOpenAICodexReplacesUnsupportedLegacyModel(t *testing.T) {
	s := newStoreOnTemp(t)
	ctx := context.Background()
	if _, err := s.Set(ctx, Patch{
		Providers: map[string]ProviderConfig{
			ProviderOpenAICodex: {Model: "gpt-5.3-codex"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	cfg, err := s.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Providers[ProviderOpenAICodex].Model; got != DefaultOpenAICodexModel {
		t.Fatalf("stored model=%q, want %q", got, DefaultOpenAICodexModel)
	}
	if got := s.Resolve().Model; got != DefaultOpenAICodexModel {
		t.Fatalf("resolved model=%q, want %q", got, DefaultOpenAICodexModel)
	}
}

func TestSetProviderConfigMergePerKey(t *testing.T) {
	s := newStoreOnTemp(t)
	ctx := context.Background()
	if _, err := s.Set(ctx, Patch{Providers: map[string]ProviderConfig{"anthropic": {Model: "claude-3", APIKey: "k1"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Set(ctx, Patch{Providers: map[string]ProviderConfig{"openai": {Model: "gpt-4o", APIKey: "k2"}}}); err != nil {
		t.Fatal(err)
	}
	cfg, _ := s.Get(ctx)
	if cfg.Providers["anthropic"].Model != "claude-3" {
		t.Fatalf("anthropic clobbered: %+v", cfg.Providers)
	}
	if cfg.Providers["openai"].Model != "gpt-4o" {
		t.Fatalf("openai missing: %+v", cfg.Providers)
	}
	if !cfg.Providers["anthropic"].APIKeySet || !cfg.Providers["openai"].APIKeySet {
		t.Fatalf("api key set flags missing: %+v", cfg.Providers)
	}
}

func TestSetRejectsUnknownProvider(t *testing.T) {
	s := newStoreOnTemp(t)
	bad := "bedrock"
	if _, err := s.Set(context.Background(), Patch{Provider: &bad}); err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestResolveReturnsActiveProviderConfig(t *testing.T) {
	s := newStoreOnTemp(t)
	ctx := context.Background()
	active := "gemini-native"
	if _, err := s.Set(ctx, Patch{Provider: &active, Providers: map[string]ProviderConfig{"gemini-native": {Model: "gemini-2.5-pro", APIKey: "gk"}}}); err != nil {
		t.Fatal(err)
	}
	rp := s.Resolve()
	if rp.Provider != "gemini-native" || rp.Model != "gemini-2.5-pro" || rp.APIKey != "gk" {
		t.Fatalf("resolve mismatch: %+v", rp)
	}
}

func TestSecretsAreNotWrittenToSettingsJSON(t *testing.T) {
	s := newStoreOnTemp(t)
	ctx := context.Background()
	if _, err := s.Set(ctx, Patch{
		Providers: map[string]ProviderConfig{
			"anthropic": {Model: "claude-3", APIKey: "provider-secret"},
		},
		WebSearchAPIKey: strPtr("web-secret"),
	}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	body, err := os.ReadFile(filepath.Join(s.dir, "settings.json"))
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	for _, secret := range []string{"provider-secret", "web-secret", "api_key", "web_search_api_key"} {
		if strings.Contains(string(body), secret) {
			t.Fatalf("settings.json contains secret marker %q: %s", secret, body)
		}
	}

	cfg, _ := s.Get(ctx)
	if cfg.Providers["anthropic"].APIKey != "" || cfg.WebSearchAPIKey != "" {
		t.Fatalf("secrets leaked through settings view: %+v", cfg)
	}
	if !cfg.Providers["anthropic"].APIKeySet || !cfg.WebSearchAPIKeySet {
		t.Fatalf("secret presence flags missing: %+v", cfg)
	}
	if s.ProviderConfigFor("anthropic").APIKey != "provider-secret" || s.WebSearchAPIKey() != "web-secret" {
		t.Fatalf("secrets not resolved through runtime accessors")
	}
}

func TestPlaintextSecretsMigrateOutOfSettingsJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINETTA_HOME", dir)
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{
		"provider":"anthropic",
		"providers":{"anthropic":{"model":"claude-3","api_key":"legacy-provider-secret"}},
		"web_search_provider":"brave",
		"web_search_api_key":"legacy-web-secret"
	}`), 0o600); err != nil {
		t.Fatal(err)
	}

	secrets := NewMemorySecretStore()
	s, err := NewWithSecretStore(secrets)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rp := s.Resolve()
	if rp.APIKey != "legacy-provider-secret" || s.WebSearchAPIKey() != "legacy-web-secret" {
		t.Fatalf("legacy secrets not migrated: rp=%+v web=%q", rp, s.WebSearchAPIKey())
	}
	body, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "legacy-provider-secret") || strings.Contains(string(body), "legacy-web-secret") {
		t.Fatalf("legacy secrets still on disk: %s", body)
	}
}

func TestSetClearsStoredSecrets(t *testing.T) {
	s := newStoreOnTemp(t)
	ctx := context.Background()
	if _, err := s.Set(ctx, Patch{
		Providers: map[string]ProviderConfig{
			"anthropic": {APIKey: "provider-secret"},
		},
		WebSearchAPIKey: strPtr("web-secret"),
	}); err != nil {
		t.Fatalf("set secrets: %v", err)
	}
	if _, err := s.Set(ctx, Patch{
		Providers: map[string]ProviderConfig{
			"anthropic": {ClearAPIKey: true},
		},
		WebSearchAPIKey: strPtr(""),
	}); err != nil {
		t.Fatalf("clear secrets: %v", err)
	}

	cfg, _ := s.Get(ctx)
	if cfg.Providers["anthropic"].APIKeySet || cfg.WebSearchAPIKeySet {
		t.Fatalf("secret flags not cleared: %+v", cfg)
	}
	if s.ProviderConfigFor("anthropic").APIKey != "" || s.WebSearchAPIKey() != "" {
		t.Fatalf("runtime secrets not cleared")
	}
}
