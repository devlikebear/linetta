package settings

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func int64Ptr(v int64) *int64 { return &v }

func TestValidProviders_isExactlyTheFour(t *testing.T) {
	want := []string{"openai-codex", "anthropic", "gemini-native", "openai"}
	if got := ValidProviders(); !slices.Equal(got, want) {
		t.Fatalf("ValidProviders = %v, want %v", got, want)
	}
}

func TestSet_providerPatch_roundTrips(t *testing.T) {
	s := newStoreOnTemp(t)
	ctx := context.Background()
	if _, err := s.Set(ctx, Patch{
		Provider: strPtr("anthropic"),
		Providers: map[string]ProviderPatch{
			"anthropic": {Model: strPtr("claude-sonnet-4-5"), ConsentedAt: int64Ptr(1700000000000)},
			"openai":    {BaseURL: strPtr("https://openrouter.ai/api/v1")},
		},
	}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	// Reload from disk with a fresh store: what survived is what was persisted.
	s2, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	got, _ := s2.Get(ctx)
	if got.Provider != "anthropic" {
		t.Errorf("provider = %q, want anthropic", got.Provider)
	}
	if got.Providers["anthropic"].Model != "claude-sonnet-4-5" {
		t.Errorf("anthropic model = %q", got.Providers["anthropic"].Model)
	}
	if got.Providers["anthropic"].ConsentedAt != 1700000000000 {
		t.Errorf("anthropic consented_at = %d", got.Providers["anthropic"].ConsentedAt)
	}
	if got.Providers["openai"].BaseURL != "https://openrouter.ai/api/v1" {
		t.Errorf("openai base_url = %q", got.Providers["openai"].BaseURL)
	}
}

func TestSet_providerPatch_mergesPerKeyAndPerField(t *testing.T) {
	s := newStoreOnTemp(t)
	ctx := context.Background()
	if _, err := s.Set(ctx, Patch{Providers: map[string]ProviderPatch{
		"anthropic": {Model: strPtr("claude-sonnet-4-5"), ConsentedAt: int64Ptr(1)},
	}}); err != nil {
		t.Fatalf("Set 1: %v", err)
	}
	// A patch for another provider, and a partial patch for the same one,
	// must not wipe what was there.
	if _, err := s.Set(ctx, Patch{Providers: map[string]ProviderPatch{
		"openai":    {BaseURL: strPtr("http://localhost:11434/v1")},
		"anthropic": {Model: strPtr("claude-opus-4-1")},
	}}); err != nil {
		t.Fatalf("Set 2: %v", err)
	}
	got, _ := s.Get(ctx)
	if got.Providers["anthropic"].Model != "claude-opus-4-1" {
		t.Errorf("anthropic model = %q, want the second patch", got.Providers["anthropic"].Model)
	}
	if got.Providers["anthropic"].ConsentedAt != 1 {
		t.Errorf("anthropic consent was lost by a partial patch: %d", got.Providers["anthropic"].ConsentedAt)
	}
	if got.Providers["openai"].BaseURL != "http://localhost:11434/v1" {
		t.Errorf("openai base_url = %q", got.Providers["openai"].BaseURL)
	}
}

func TestSet_providerAPIKey_livesOnlyInTheSecretStore(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINETTA_HOME", dir)
	secrets := NewMemorySecretStore()
	s, err := NewWithSecretStore(secrets)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	if _, err := s.Set(ctx, Patch{Providers: map[string]ProviderPatch{
		"anthropic": {APIKey: strPtr("sk-ant-test")},
	}}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if v, ok, _ := secrets.Get("provider.anthropic.api_key"); !ok || v != "sk-ant-test" {
		t.Fatalf("secret store has (%q, %v), want the key", v, ok)
	}
	body, _ := os.ReadFile(filepath.Join(dir, "settings.json"))
	if strings.Contains(string(body), "sk-ant-test") {
		t.Fatalf("api key written to settings.json: %s", body)
	}
	got, _ := s.Get(ctx)
	if !got.Providers["anthropic"].APIKeySet || got.Providers["anthropic"].APIKey != "" {
		t.Errorf("settings.get must show presence only: %+v", got.Providers["anthropic"])
	}
	if cfg := s.ProviderConfigFor("anthropic"); cfg.APIKey != "sk-ant-test" {
		t.Errorf("ProviderConfigFor did not read the secret: %+v", cfg)
	}
	// An empty key deletes.
	if _, err := s.Set(ctx, Patch{Providers: map[string]ProviderPatch{
		"anthropic": {APIKey: strPtr("")},
	}}); err != nil {
		t.Fatalf("Set clear: %v", err)
	}
	if _, ok, _ := secrets.Get("provider.anthropic.api_key"); ok {
		t.Error("empty api_key patch did not delete the secret")
	}
}

func TestSet_rejectsProvidersOutsideTheWhitelist(t *testing.T) {
	s := newStoreOnTemp(t)
	ctx := context.Background()
	if _, err := s.Set(ctx, Patch{Provider: strPtr("claude-code-cli")}); err == nil {
		t.Error("provider=claude-code-cli was accepted")
	}
	if _, err := s.Set(ctx, Patch{Providers: map[string]ProviderPatch{
		"openrouter": {Model: strPtr("x")},
	}}); err == nil {
		t.Error("providers[openrouter] was accepted")
	}
}

func TestActiveProvider_fallsBackToCodexButLeavesDiskAlone(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINETTA_HOME", dir)
	path := filepath.Join(dir, "settings.json")
	seed := `{"language":"ko","provider":"claude-code-cli",` +
		`"providers":{"claude-code-cli":{"model":"opus"}}}`
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := s.ActiveProvider(); got != "openai-codex" {
		t.Errorf("ActiveProvider = %q, want openai-codex fallback", got)
	}
	if _, err := s.Set(context.Background(), Patch{Theme: strPtr("dark")}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	raw, _ := os.ReadFile(path)
	var onDisk Config
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatal(err)
	}
	if onDisk.Provider != "claude-code-cli" || onDisk.Providers["claude-code-cli"].Model != "opus" {
		t.Errorf("retired provider was rewritten on an unrelated save: %s", raw)
	}
}

func TestHasProviderConsent_isPerProvider(t *testing.T) {
	s := newStoreOnTemp(t)
	ctx := context.Background()
	if _, err := s.Set(ctx, Patch{Providers: map[string]ProviderPatch{
		"anthropic": {ConsentedAt: int64Ptr(1700000000000)},
	}}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if !s.HasProviderConsent("anthropic") {
		t.Error("anthropic consent not recorded")
	}
	if s.HasProviderConsent("openai") {
		t.Error("consent leaked across providers")
	}
	if _, err := s.Set(ctx, Patch{Providers: map[string]ProviderPatch{
		"anthropic": {ConsentedAt: int64Ptr(0)},
	}}); err != nil {
		t.Fatalf("Set revoke: %v", err)
	}
	if s.HasProviderConsent("anthropic") {
		t.Error("consented_at=0 did not revoke")
	}
}
