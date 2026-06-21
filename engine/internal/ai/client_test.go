//go:build !mas

package ai

import (
	"os"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/settings"
)

func TestDefaultClientFactorySetsClaudeCliPath(t *testing.T) {
	os.Unsetenv("CLAUDE_CODE_CLI_PATH")
	// claude-code-cli with a bogus path: NewProvider may fail to find the binary,
	// but the env var must be set first — that is the behavior under test.
	_, _ = DefaultClientFactory(ResolvedProvider{
		Provider: "claude-code-cli",
		CliPath:  "/tmp/does-not-exist-claude",
	})
	if got := os.Getenv("CLAUDE_CODE_CLI_PATH"); got != "/tmp/does-not-exist-claude" {
		t.Fatalf("CLAUDE_CODE_CLI_PATH=%q, want /tmp/does-not-exist-claude", got)
	}
}

func TestProviderOptionsForTarsMapsOpenRouterToOpenAICompatible(t *testing.T) {
	got := providerOptionsForTars(ResolvedProvider{
		Provider: settings.ProviderOpenRouter,
		Model:    settings.DefaultOpenRouterModel,
		APIKey:   "or-test",
		BaseURL:  settings.OpenRouterBaseURL,
	})
	if got.Provider != settings.ProviderOpenAI {
		t.Fatalf("provider=%q, want openai", got.Provider)
	}
	if got.BaseURL != settings.OpenRouterBaseURL || got.Model != settings.DefaultOpenRouterModel || got.APIKey != "or-test" {
		t.Fatalf("options mismatch: %+v", got)
	}
}
