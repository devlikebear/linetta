//go:build mas || mobile

package settings

import (
	"context"
	"slices"
	"testing"
)

func TestValidProviders_leavesTheCLIOutOfSandboxedBuilds(t *testing.T) {
	want := []string{"openai-codex", "anthropic", "gemini-native", "openai"}
	if got := ValidProviders(); !slices.Equal(got, want) {
		t.Fatalf("ValidProviders = %v, want %v", got, want)
	}
	s := newStoreOnTemp(t)
	if _, err := s.Set(context.Background(), Patch{Provider: strPtr("claude-code")}); err == nil {
		t.Error("provider=claude-code was accepted in a build that cannot spawn it")
	}
}
