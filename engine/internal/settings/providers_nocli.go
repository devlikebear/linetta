//go:build mas || mobile

package settings

// ValidProviders returns the provider ids settings.set accepts, in the order
// the settings pane lists them. The App Store sandbox and the mobile runtimes
// forbid spawning a subprocess, so the Claude Code CLI is not one of them: a
// settings file that names it falls back to Codex like any unknown id.
func ValidProviders() []string {
	return []string{ProviderOpenAICodex, ProviderAnthropic, ProviderGeminiNative, ProviderOpenAI}
}
