package settings

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImageSettings_offByDefault(t *testing.T) {
	s := newStoreOnTemp(t)
	got, _ := s.Get(context.Background())
	if got.ImageProvider != "" || len(got.ImageProviders) != 0 {
		t.Fatalf("defaults = %q %v, want image generation off", got.ImageProvider, got.ImageProviders)
	}
	if s.ActiveImageProvider() != "" || s.HasImageConsent(ProviderGeminiNative) {
		t.Fatal("accessors report image generation on by default")
	}
}

func TestImageSettings_roundTripThroughDisk(t *testing.T) {
	s := newStoreOnTemp(t)
	ctx := context.Background()
	if _, err := s.Set(ctx, Patch{
		ImageProvider: strPtr(ProviderGeminiNative),
		ImageProviders: map[string]ImageProviderPatch{
			ProviderGeminiNative: {Model: strPtr(" gemini-3.1-flash-image "), ConsentedAt: int64Ptr(1700000000000)},
		},
	}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	s2, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if s2.ActiveImageProvider() != ProviderGeminiNative {
		t.Fatalf("active = %q", s2.ActiveImageProvider())
	}
	cfg := s2.ImageProviderConfigFor(ProviderGeminiNative)
	if cfg.Model != "gemini-3.1-flash-image" || !s2.HasImageConsent(ProviderGeminiNative) {
		t.Fatalf("gemini image config = %+v", cfg)
	}
	// Consent is per image provider and apart from the agent's.
	if s2.HasImageConsent(ProviderOpenAICodex) || s2.HasProviderConsent(ProviderGeminiNative) {
		t.Fatal("image consent leaked to another provider or to the agent")
	}
}

func TestImageSettings_patchMergesPerProviderAndPerField(t *testing.T) {
	s := newStoreOnTemp(t)
	ctx := context.Background()
	mustSet := func(p Patch) {
		t.Helper()
		if _, err := s.Set(ctx, p); err != nil {
			t.Fatalf("Set: %v", err)
		}
	}
	mustSet(Patch{ImageProviders: map[string]ImageProviderPatch{
		ProviderOpenAICodex:  {Model: strPtr("gpt-5.6-luna"), ConsentedAt: int64Ptr(1)},
		ProviderGeminiNative: {Model: strPtr("gemini-3.1-flash-image")},
	}})
	mustSet(Patch{ImageProviders: map[string]ImageProviderPatch{ProviderOpenAICodex: {ConsentedAt: int64Ptr(0)}}})

	if cfg := s.ImageProviderConfigFor(ProviderOpenAICodex); cfg.Model != "gpt-5.6-luna" || cfg.ConsentedAt != 0 {
		t.Fatalf("codex = %+v, want the model kept and consent revoked", cfg)
	}
	if cfg := s.ImageProviderConfigFor(ProviderGeminiNative); cfg.Model != "gemini-3.1-flash-image" {
		t.Fatalf("gemini = %+v, want it untouched", cfg)
	}

	mustSet(Patch{ImageProvider: strPtr(ProviderOpenAICodex)})
	mustSet(Patch{ImageProvider: strPtr("")})
	if s.ActiveImageProvider() != "" {
		t.Fatalf("active = %q, want off", s.ActiveImageProvider())
	}
}

func TestImageSettings_rejectsProvidersThatCannotDraw(t *testing.T) {
	s := newStoreOnTemp(t)
	ctx := context.Background()
	for name, p := range map[string]Patch{
		"active":       {ImageProvider: strPtr(ProviderAnthropic)},
		"entry":        {ImageProviders: map[string]ImageProviderPatch{ProviderOpenAI: {Model: strPtr("dall-e")}}},
		"mixed":        {ImageProviders: map[string]ImageProviderPatch{ProviderGeminiNative: {Model: strPtr("m")}, "nope": {}}},
		"unknown name": {ImageProvider: strPtr("midjourney")},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := s.Set(ctx, p); err == nil {
				t.Fatal("want an error")
			}
			if s.ActiveImageProvider() != "" || len(s.ImageProviderConfigFor(ProviderGeminiNative).Model) != 0 {
				t.Fatal("a rejected patch changed the image settings")
			}
		})
	}
}

func TestImageSettings_loadDropsIdsThisBuildCannotDraw(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINETTA_HOME", dir)
	body := `{"image_provider":"anthropic","image_providers":{"anthropic":{"model":"x"},"gemini-native":{"model":"gemini-3.1-flash-image","consented_at":5}}}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if s.ActiveImageProvider() != "" {
		t.Fatalf("active = %q, want off for an id that cannot draw", s.ActiveImageProvider())
	}
	got, _ := s.Get(context.Background())
	if _, ok := got.ImageProviders[ProviderAnthropic]; ok || got.ImageProviders[ProviderGeminiNative].ConsentedAt != 5 {
		t.Fatalf("image providers = %v", got.ImageProviders)
	}
}

func TestImageSettings_neverWriteACredential(t *testing.T) {
	s := newStoreOnTemp(t)
	ctx := context.Background()
	if _, err := s.Set(ctx, Patch{
		Providers:      map[string]ProviderPatch{ProviderGeminiNative: {APIKey: strPtr("AIza-secret-key")}},
		ImageProvider:  strPtr(ProviderGeminiNative),
		ImageProviders: map[string]ImageProviderPatch{ProviderGeminiNative: {Model: strPtr("gemini-3.1-flash-image")}},
	}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(os.Getenv("LINETTA_HOME"), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "AIza-secret-key") {
		t.Fatal("settings.json holds the api key")
	}
	if !strings.Contains(string(raw), `"image_provider": "gemini-native"`) {
		t.Fatalf("settings.json lacks the image provider:\n%s", raw)
	}
}
