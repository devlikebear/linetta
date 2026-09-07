package settings

import (
	"context"
	"encoding/json"
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

func boolPtr(v bool) *bool        { return &v }
func intPtr(v int) *int           { return &v }
func floatPtr(v float64) *float64 { return &v }
func strPtr(v string) *string     { return &v }

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
	if got.Palette != "hanji" {
		t.Errorf("palette = %q, want hanji", got.Palette)
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
	if _, err := s.Set(context.Background(), Patch{Theme: strPtr("dark")}); err != nil {
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
		Palette:          strPtr("press"),
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
	if got.Palette != "press" {
		t.Fatalf("palette not applied in memory: %+v", got)
	}
	s2, err := NewWithSecretStore(secrets)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	reloaded, _ := s2.Get(ctx)
	if reloaded.Theme != "dark" || reloaded.EditorFontSize != 18 || reloaded.EditorLineHeight != 2.05 || reloaded.CopyProfile != "munpia" {
		t.Fatalf("editor prefs not persisted: %+v", reloaded)
	}
	if reloaded.Palette != "press" {
		t.Fatalf("palette not persisted: %+v", reloaded)
	}
	if _, err := s.Set(ctx, Patch{Theme: strPtr("sepia")}); err == nil {
		t.Fatalf("expected invalid theme error")
	}
	if _, err := s.Set(ctx, Patch{Palette: strPtr("neon")}); err == nil {
		t.Fatalf("expected invalid palette error")
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
	_ = s
	if got, ok, err := secrets.Get(webSearchAPIKeySecretName); err != nil || !ok || got != "legacy-web-secret" {
		t.Fatalf("web search secret not migrated: (%q, %v, %v)", got, ok, err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "legacy-provider-secret") || strings.Contains(string(body), "legacy-web-secret") {
		t.Fatalf("legacy secrets still on disk: %s", body)
	}
}

// The companion's settings are no longer settable, but persist() writes an
// enumerated struct — dropping them from that list would delete a writer's
// stored provider config on the next unrelated save. This is the Phase 2
// mcp_* bug in reverse, and the reason those fields stay in Config.
func TestSet_leavesRetiredCompanionSettingsOnDisk(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINETTA_HOME", dir)
	path := filepath.Join(dir, "settings.json")
	seed := `{"language":"ko","provider":"openrouter",` +
		`"providers":{"openrouter":{"model":"openai/gpt-5.4","base_url":"https://example.test"}},` +
		`"ai_data_sharing_consent_version":1,"ai_data_sharing_consented_at":1700000000000,` +
		`"web_search_provider":"perplexity"}`
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatalf("seed settings.json: %v", err)
	}

	s, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := s.Set(context.Background(), Patch{Theme: strPtr("dark")}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	var onDisk Config
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if onDisk.Provider != "openrouter" {
		t.Errorf("provider = %q, want it untouched", onDisk.Provider)
	}
	if got := onDisk.Providers["openrouter"].Model; got != "openai/gpt-5.4" {
		t.Errorf("provider model = %q, want it untouched", got)
	}
	if got := onDisk.Providers["openrouter"].BaseURL; got != "https://example.test" {
		t.Errorf("provider base_url = %q, want it untouched", got)
	}
	if onDisk.AIDataSharingConsentVersion != 1 || onDisk.AIDataSharingConsentedAt != 1700000000000 {
		t.Errorf("consent fields were rewritten: version=%d at=%d",
			onDisk.AIDataSharingConsentVersion, onDisk.AIDataSharingConsentedAt)
	}
	if onDisk.WebSearchProvider != "perplexity" {
		t.Errorf("web_search_provider = %q, want it untouched", onDisk.WebSearchProvider)
	}
	if onDisk.Theme != "dark" {
		t.Errorf("theme = %q, want the patch applied", onDisk.Theme)
	}
}

// The self-improvement loop is on unless the writer says otherwise (#98 Task
// 10), and a deliberate "otherwise" has to survive a restart. The three ways
// this field can be silently lost are all covered here: missing from defaults
// (a fresh install ships the feature off), missing from persist()'s allowlist
// (the writer's choice lives until the next restart and no further), and a
// plain assignment in load() instead of the presence guard (a settings.json
// written before the key existed reads as a deliberate false).
func TestAgentSelfReviewEnabled_defaultsOnAndSurvivesADeliberateFalse(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINETTA_HOME", dir)
	ctx := context.Background()

	s, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, _ := s.Get(ctx)
	if !got.AgentSelfReviewEnabled {
		t.Fatalf("agent_self_review_enabled defaults to false; the loop the feature is named "+
			"for would never run on a fresh install: %+v", got)
	}
	if !s.AgentSelfReviewEnabled() {
		t.Error("the accessor the agent reads disagrees with the config")
	}

	if _, err := s.Set(ctx, Patch{AgentSelfReviewEnabled: boolPtr(false)}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if s.AgentSelfReviewEnabled() {
		t.Error("switching the self-review off did not take effect in memory")
	}

	s2, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatalf("re-New: %v", err)
	}
	if s2.AgentSelfReviewEnabled() {
		t.Error("a deliberate false did not survive a reload — either persist() never wrote it, " +
			"or load() overwrote it with the default")
	}

	// And back on. This direction is the one that catches a field missing from
	// persist()'s allowlist, and it is the ONLY direction that does: the JSON
	// tag has no omitempty, so an unlisted field is still written — as its
	// zero value, false. A test that only ever switched the setting off would
	// read that accident as success and ship a switch that cannot be turned
	// back on across a restart.
	if _, err := s2.Set(ctx, Patch{AgentSelfReviewEnabled: boolPtr(true)}); err != nil {
		t.Fatalf("Set back on: %v", err)
	}
	s3, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatalf("re-New: %v", err)
	}
	if !s3.AgentSelfReviewEnabled() {
		t.Error("switching the self-review back on did not survive a reload — " +
			"agent_self_review_enabled is missing from persist()'s allowlist")
	}

	// A settings.json written by a build that predates the key must still get
	// the default, not the zero value.
	legacy := t.TempDir()
	t.Setenv("LINETTA_HOME", legacy)
	if err := os.WriteFile(filepath.Join(legacy, "settings.json"), []byte(`{"language":"en"}`), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s4, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatalf("legacy New: %v", err)
	}
	if !s4.AgentSelfReviewEnabled() {
		t.Error("a settings.json written before this key existed reads as a deliberate false")
	}
}
