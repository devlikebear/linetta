package settings

import "slices"

// ValidProviders returns the provider ids settings.set accepts, in the order
// the settings pane lists them.
func ValidProviders() []string {
	return []string{ProviderOpenAICodex, ProviderAnthropic, ProviderGeminiNative, ProviderOpenAI}
}

// ActiveProvider returns the provider the built-in agent uses. An id outside
// the whitelist — a pre-1.0 "claude-code-cli" or "openrouter" still on disk —
// is not an error: it stays on disk untouched and Codex is used instead.
func (s *Store) ActiveProvider() string {
	s.mu.RLock()
	id := s.cfg.Provider
	s.mu.RUnlock()
	if slices.Contains(ValidProviders(), id) {
		return id
	}
	return ProviderOpenAICodex
}

// HasProviderConsent reports whether the writer agreed to send manuscript
// text to this provider. Consent is per provider: agreeing to OpenAI is not
// agreeing to Anthropic (the v0.9.3 lesson).
func (s *Store) HasProviderConsent(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Providers[id].ConsentedAt > 0
}
