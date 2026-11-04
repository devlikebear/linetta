//go:build !mas && !mobile

package settings

// ValidProviders returns the provider ids settings.set accepts, in the order
// the settings pane lists them. This build may spawn a subprocess, so the
// Claude Code CLI is among them.
func ValidProviders() []string {
	return []string{ProviderOpenAICodex, ProviderAnthropic, ProviderGeminiNative, ProviderOpenAI, ProviderClaudeCode}
}

// DefaultProvider is what a new library starts on, and what an id outside
// ValidProviders falls back to.
const DefaultProvider = ProviderOpenAICodex
