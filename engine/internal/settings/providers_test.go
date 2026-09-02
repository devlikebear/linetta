package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
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

// A single patch that mixes a valid id carrying an API key with an
// off-whitelist id must reject the whole call *and* leave the secret store
// untouched for the valid id. Go map iteration order over p.Providers is
// randomized, so this only holds if every id is validated before any
// s.setSecret call runs — looping exercises both iteration orders, and after
// the validate-then-apply fix the invariant holds by construction regardless
// of order, so every iteration should pass.
func TestSet_rejectsMixedPatch_writesNoSecretForTheValidID(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINETTA_HOME", dir)
	secrets := NewMemorySecretStore()
	s, err := NewWithSecretStore(secrets)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		if _, err := s.Set(ctx, Patch{Providers: map[string]ProviderPatch{
			"anthropic": {APIKey: strPtr("sk-leak")},
			"typo-id":   {Model: strPtr("x")},
		}}); err == nil {
			t.Fatal("expected an error for the off-whitelist id")
		}
		if _, ok, _ := secrets.Get("provider.anthropic.api_key"); ok {
			t.Fatalf("iteration %d: api key was written to the secret store despite Set returning an error", i)
		}
	}
}

// A patch that carries a valid api_key alongside a field that fails a *later*
// validation must leave the secret store exactly as it was. The keychain write
// is the one side effect Set cannot roll back, so it has to happen after every
// validation, not in the middle of the merge loop.
func TestSet_rejectsLateValidation_leavesTheSecretStoreUntouched(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINETTA_HOME", dir)
	secrets := NewMemorySecretStore()
	s, err := NewWithSecretStore(secrets)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()

	// Direction 1: a write that must not happen. theme is validated well
	// after the providers block.
	if _, err := s.Set(ctx, Patch{
		Providers: map[string]ProviderPatch{"anthropic": {APIKey: strPtr("sk-ant-never")}},
		Theme:     strPtr("chartreuse"),
	}); err == nil {
		t.Fatal("expected an error for the unknown theme")
	}
	if v, ok, _ := secrets.Get("provider.anthropic.api_key"); ok {
		t.Fatalf("a rejected patch wrote %q into the secret store", v)
	}

	// Direction 2: a delete that must not happen. Store a key first, then send
	// an empty api_key (the delete) with an out-of-range mcp_port.
	if _, err := s.Set(ctx, Patch{Providers: map[string]ProviderPatch{
		"anthropic": {APIKey: strPtr("sk-ant-keep")},
	}}); err != nil {
		t.Fatalf("Set seed: %v", err)
	}
	if _, err := s.Set(ctx, Patch{
		Providers: map[string]ProviderPatch{"anthropic": {APIKey: strPtr("")}},
		MCPPort:   intPtr(80),
	}); err == nil {
		t.Fatal("expected an error for the out-of-range mcp_port")
	}
	if v, ok, _ := secrets.Get("provider.anthropic.api_key"); !ok || v != "sk-ant-keep" {
		t.Fatalf("a rejected patch deleted the stored key: got (%q, %v)", v, ok)
	}
}

// base_url points the request somewhere other than the provider's own
// endpoint. Consent is per provider and names that provider, so only the
// OpenAI-compatible family may carry one; an empty string still clears.
func TestSet_baseURL_isOpenAIOnly(t *testing.T) {
	s := newStoreOnTemp(t)
	ctx := context.Background()
	for _, id := range []string{"anthropic", "gemini-native", "openai-codex"} {
		if _, err := s.Set(ctx, Patch{Providers: map[string]ProviderPatch{
			id: {BaseURL: strPtr("https://evil.example/v1")},
		}}); err == nil {
			t.Errorf("base_url was accepted for %q", id)
		}
		// Clearing must stay allowed for every id.
		if _, err := s.Set(ctx, Patch{Providers: map[string]ProviderPatch{
			id: {BaseURL: strPtr("  ")},
		}}); err != nil {
			t.Errorf("clearing base_url for %q was rejected: %v", id, err)
		}
	}
	if _, err := s.Set(ctx, Patch{Providers: map[string]ProviderPatch{
		"openai": {BaseURL: strPtr("https://openrouter.ai/api/v1")},
	}}); err != nil {
		t.Fatalf("base_url on openai was rejected: %v", err)
	}
	got, _ := s.Get(ctx)
	if got.Providers["openai"].BaseURL != "https://openrouter.ai/api/v1" {
		t.Errorf("openai base_url = %q", got.Providers["openai"].BaseURL)
	}
	if got.Providers["anthropic"].BaseURL != "" {
		t.Errorf("anthropic kept a base_url: %q", got.Providers["anthropic"].BaseURL)
	}
}

// failingGetSecretStore answers Get with an error the way a locked or
// access-denied keychain does, while still reporting the entry as present.
type failingGetSecretStore struct {
	SecretStore
	err error
}

func (f failingGetSecretStore) Get(string) (string, bool, error) { return "", false, f.err }

// A keychain that refuses to hand the key back currently reads downstream as
// "no key". Until #94 can tell the writer the difference, the failure must at
// least be logged rather than silently swallowed.
func TestProviderConfigFor_logsASecretStoreReadFailure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINETTA_HOME", dir)
	s, err := NewWithSecretStore(failingGetSecretStore{
		SecretStore: NewMemorySecretStore(),
		err:         errors.New("keychain is locked"),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	if cfg := s.ProviderConfigFor("anthropic"); cfg.APIKey != "" || cfg.APIKeySet {
		t.Fatalf("a failed read must not fabricate a key: %+v", cfg)
	}
	if !strings.Contains(logs.String(), "keychain is locked") {
		t.Fatalf("secret-store read failure was swallowed without a log line: %q", logs.String())
	}
}
