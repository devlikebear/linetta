package settings

import (
	"fmt"
	"slices"
	"strings"
)

// ValidImageProviders are the providers that can draw illustrations. They
// reuse the agent providers' credentials — Codex's ChatGPT login, the Gemini
// API key — but are chosen apart from the agent's: a writer can chat with
// Claude and draw with Gemini.
func ValidImageProviders() []string {
	return []string{ProviderOpenAICodex, ProviderGeminiNative}
}

// ImageProviderConfig is one image provider's stored entry. It never holds a
// credential: those stay with Providers[id] and the SecretStore.
//
// ConsentedAt is its own consent, not the agent's. The agent's sentence
// promises what the agent reads with its tools; drawing sends something else
// — the prompt the writer approved and the reference images they chose.
type ImageProviderConfig struct {
	Model       string `json:"model,omitempty"`
	ConsentedAt int64  `json:"consented_at,omitempty"` // 0 = none
}

// ImageProviderPatch is one entry in Patch.ImageProviders. Nil leaves the
// field alone; ConsentedAt 0 revokes.
type ImageProviderPatch struct {
	Model       *string `json:"model,omitempty"`
	ConsentedAt *int64  `json:"consented_at,omitempty"`
}

// applyImagePatch merges the image fields of p into next. It mutates nothing
// that next shares with the live config: the map is rebuilt.
func applyImagePatch(next *Config, p Patch) error {
	if p.ImageProvider != nil {
		id := *p.ImageProvider
		if id != "" && !slices.Contains(ValidImageProviders(), id) {
			return fmt.Errorf("settings: unknown image provider %q", id)
		}
		next.ImageProvider = id
	}
	if len(p.ImageProviders) == 0 {
		return nil
	}
	// Every id first, so a bad one rejects the whole patch before any merge.
	for id := range p.ImageProviders {
		if !slices.Contains(ValidImageProviders(), id) {
			return fmt.Errorf("settings: unknown image provider %q", id)
		}
	}
	merged := make(map[string]ImageProviderConfig, len(next.ImageProviders)+len(p.ImageProviders))
	for id, cfg := range next.ImageProviders {
		merged[id] = cfg
	}
	for id, pp := range p.ImageProviders {
		cfg := merged[id]
		if pp.Model != nil {
			cfg.Model = strings.TrimSpace(*pp.Model)
		}
		if pp.ConsentedAt != nil {
			cfg.ConsentedAt = *pp.ConsentedAt
		}
		merged[id] = cfg
	}
	next.ImageProviders = merged
	return nil
}

// loadImageSettings copies what a settings.json says about image providers,
// dropping ids this build does not offer.
func loadImageSettings(dst *Config, disk Config) {
	if slices.Contains(ValidImageProviders(), disk.ImageProvider) {
		dst.ImageProvider = disk.ImageProvider
	}
	for id, cfg := range disk.ImageProviders {
		if !slices.Contains(ValidImageProviders(), id) {
			continue
		}
		if dst.ImageProviders == nil {
			dst.ImageProviders = map[string]ImageProviderConfig{}
		}
		dst.ImageProviders[id] = cfg
	}
}

// ActiveImageProvider is the provider illustrations are drawn with, "" when
// the writer has not turned image generation on.
func (s *Store) ActiveImageProvider() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.ImageProvider
}

// ImageProviderConfigFor returns an image provider's stored entry (zero value
// if unset).
func (s *Store) ImageProviderConfigFor(id string) ImageProviderConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.ImageProviders[id]
}

// HasImageConsent reports whether the writer agreed to send illustration
// prompts and reference images to this provider.
func (s *Store) HasImageConsent(id string) bool {
	return s.ImageProviderConfigFor(id).ConsentedAt > 0
}
