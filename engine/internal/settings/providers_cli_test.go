//go:build !mas && !mobile

package settings

import (
	"context"
	"slices"
	"testing"
)

func TestValidProviders_includesClaudeCodeWhereASubprocessIsAllowed(t *testing.T) {
	want := []string{"openai-codex", "anthropic", "gemini-native", "openai", "claude-code"}
	if got := ValidProviders(); !slices.Equal(got, want) {
		t.Fatalf("ValidProviders = %v, want %v", got, want)
	}
}

func TestSet_claudeCodeIsSelectableButTakesNoKey(t *testing.T) {
	s := newStoreOnTemp(t)
	ctx := context.Background()
	if _, err := s.Set(ctx, Patch{
		Provider:  strPtr("claude-code"),
		Providers: map[string]ProviderPatch{"claude-code": {Model: strPtr("opus"), ConsentedAt: int64Ptr(1700000000000)}},
	}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := s.ActiveProvider(); got != "claude-code" {
		t.Errorf("ActiveProvider = %q", got)
	}
	if _, err := s.Set(ctx, Patch{Providers: map[string]ProviderPatch{
		"claude-code": {APIKey: strPtr("sk-ant-should-not-be-stored")},
	}}); err == nil {
		t.Error("an api_key for claude-code was accepted")
	}
	// Clearing is harmless and stays allowed, so a generic "clear key" path
	// does not have to know which providers have one.
	if _, err := s.Set(ctx, Patch{Providers: map[string]ProviderPatch{
		"claude-code": {APIKey: strPtr("")},
	}}); err != nil {
		t.Errorf("clearing claude-code's (absent) key: %v", err)
	}
}
