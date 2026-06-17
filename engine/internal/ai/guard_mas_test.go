//go:build mas

package ai

import "testing"

func TestGuardProviderMAS(t *testing.T) {
	if err := guardProvider("claude-code-cli"); err == nil {
		t.Fatal("expected claude-code-cli to be rejected in mas build")
	}
	for _, p := range []string{"anthropic", "openai", "gemini-native", "openai-codex"} {
		if err := guardProvider(p); err != nil {
			t.Fatalf("provider %q should be allowed: %v", p, err)
		}
	}
}
