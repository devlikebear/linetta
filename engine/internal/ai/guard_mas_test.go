//go:build mas

package ai

import "testing"

func TestGuardProviderMAS(t *testing.T) {
	for _, p := range []string{"claude-code-cli", "openai-codex"} {
		if err := guardProvider(p); err == nil {
			t.Fatalf("expected %q to be rejected in mas build", p)
		}
	}
	for _, p := range []string{"anthropic", "openai", "gemini-native"} {
		if err := guardProvider(p); err != nil {
			t.Fatalf("provider %q should be allowed: %v", p, err)
		}
	}
}
