//go:build mobile

package ai

import "testing"

func TestMobileBlocksCLIAndCodex(t *testing.T) {
	got := UnavailableProviders()
	want := map[string]bool{"claude-code-cli": true, "openai-codex": true}
	if len(got) != len(want) {
		t.Fatalf("UnavailableProviders() = %v, want keys %v", got, want)
	}
	for _, p := range got {
		if !want[p] {
			t.Fatalf("unexpected provider %q in %v", p, got)
		}
	}
	if err := guardProvider("claude-code-cli"); err == nil {
		t.Fatal("guardProvider(claude-code-cli) = nil, want error on mobile build")
	}
	if err := guardProvider("openai-codex"); err == nil {
		t.Fatal("guardProvider(openai-codex) = nil, want error on mobile build")
	}
}
