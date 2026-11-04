//go:build mobile

package settings

// ValidProviders returns the provider ids settings.set accepts, in the order
// the settings pane lists them. Mobile connects with an API key only: no
// subprocess for the Claude Code CLI, and no ChatGPT sign-in, whose OAuth
// callback needs a local listener the phone cannot keep open while Safari is
// in front.
func ValidProviders() []string {
	return []string{ProviderAnthropic, ProviderGeminiNative, ProviderOpenAI}
}

// DefaultProvider is what a new library starts on, and what an id outside
// ValidProviders falls back to — including a Codex choice carried over in a
// library restored from a desktop backup.
const DefaultProvider = ProviderAnthropic
