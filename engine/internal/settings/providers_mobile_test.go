//go:build mobile

package settings

import (
	"context"
	"slices"
	"testing"
)

func TestValidProviders_offersOnlyAPIKeyProvidersOnMobile(t *testing.T) {
	want := []string{"anthropic", "gemini-native", "openai"}
	if got := ValidProviders(); !slices.Equal(got, want) {
		t.Fatalf("ValidProviders = %v, want %v", got, want)
	}
	s := newStoreOnTemp(t)
	for _, id := range []string{"claude-code", "openai-codex"} {
		if _, err := s.Set(context.Background(), Patch{Provider: strPtr(id)}); err == nil {
			t.Errorf("provider=%s was accepted on mobile", id)
		}
	}
	if got := s.ActiveProvider(); got != "anthropic" {
		t.Errorf("ActiveProvider = %q on a new mobile library, want anthropic", got)
	}
	if got := ValidImageProviders(); !slices.Equal(got, []string{"gemini-native"}) {
		t.Errorf("ValidImageProviders = %v, want only gemini-native", got)
	}
}
