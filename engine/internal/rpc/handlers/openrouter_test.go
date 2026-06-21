package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/openrouter"
	"github.com/devlikebear/linetta/engine/internal/settings"
)

func TestOpenRouterKeyInfoReturnsRedactedCreditState(t *testing.T) {
	ctx := context.Background()
	store := newSettingsFixture(t)
	provider := settings.ProviderOpenRouter
	if _, err := store.Set(ctx, settings.Patch{
		Provider: &provider,
		Providers: map[string]settings.ProviderConfig{
			provider: {APIKey: "or-secret"},
		},
	}); err != nil {
		t.Fatalf("settings.Set: %v", err)
	}

	var gotKey string
	limit := 5.0
	remaining := 4.25
	handler := OpenRouterKeyInfo(store, func(_ context.Context, apiKey string) (openrouter.KeyInfo, error) {
		gotKey = apiKey
		return openrouter.KeyInfo{
			Label:          "Linetta",
			Limit:          &limit,
			LimitRemaining: &remaining,
			UsageMonthly:   0.75,
		}, nil
	})

	raw, err := handler(ctx, nil)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if gotKey != "or-secret" {
		t.Fatalf("apiKey=%q, want stored key", gotKey)
	}
	if strings.Contains(string(raw), "or-secret") {
		t.Fatalf("response leaked key: %s", raw)
	}
	var got openRouterKeyInfoResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if !got.OK || got.Provider != settings.ProviderOpenRouter || got.Label != "Linetta" {
		t.Fatalf("result mismatch: %+v", got)
	}
	if got.Limit == nil || *got.Limit != limit || got.LimitRemaining == nil || *got.LimitRemaining != remaining {
		t.Fatalf("limit mismatch: %+v", got)
	}
}

func TestOpenRouterKeyInfoRequiresStoredKey(t *testing.T) {
	store := newSettingsFixture(t)
	handler := OpenRouterKeyInfo(store, nil)
	if _, err := handler(context.Background(), nil); err == nil {
		t.Fatal("expected missing key error")
	}
}

type fakeOpenRouterOAuth struct {
	start openrouter.OAuthStart
	key   string
}

func (f *fakeOpenRouterOAuth) Start(context.Context) (openrouter.OAuthStart, error) {
	return f.start, nil
}

func (f *fakeOpenRouterOAuth) Finish(context.Context, string) (string, error) {
	return f.key, nil
}

func TestOpenRouterOAuthFinishStoresKeyWithoutReturningIt(t *testing.T) {
	ctx := context.Background()
	store := newSettingsFixture(t)
	oauth := &fakeOpenRouterOAuth{key: "or-oauth-secret"}
	handler := OpenRouterOAuthFinish(store, oauth)

	raw, err := handler(ctx, json.RawMessage(`{"request_id":"req-1"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if strings.Contains(string(raw), "or-oauth-secret") {
		t.Fatalf("response leaked key: %s", raw)
	}
	var got openRouterOAuthFinishResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if !got.OK || got.Provider != settings.ProviderOpenRouter {
		t.Fatalf("result mismatch: %+v", got)
	}
	cfg := store.ProviderConfigFor(settings.ProviderOpenRouter)
	if cfg.APIKey != "or-oauth-secret" || cfg.Model != settings.DefaultOpenRouterModel {
		t.Fatalf("stored config mismatch: %+v", cfg)
	}
}
