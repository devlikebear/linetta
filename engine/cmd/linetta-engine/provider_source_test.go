package main

import (
	"context"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/settings"
)

func TestProviderSourceDelegatesWebSearchSettings(t *testing.T) {
	t.Setenv("LINETTA_HOME", t.TempDir())
	store, err := settings.NewWithSecretStore(settings.NewMemorySecretStore())
	if err != nil {
		t.Fatalf("NewWithSecretStore: %v", err)
	}
	provider := "perplexity"
	apiKey := "pplx-test"
	if _, err := store.Set(context.Background(), settings.Patch{
		WebSearchProvider: &provider,
		WebSearchAPIKey:   &apiKey,
	}); err != nil {
		t.Fatalf("settings.Set: %v", err)
	}

	src := providerSource{store: store}
	if got := src.WebSearchProvider(); got != provider {
		t.Fatalf("WebSearchProvider() = %q, want %q", got, provider)
	}
	if got := src.WebSearchAPIKey(); got != apiKey {
		t.Fatalf("WebSearchAPIKey() = %q, want saved key", got)
	}
}
