package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/ai"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/tars/pkg/llm"
)

type providerTestFakeClient struct {
	messages []llm.ChatMessage
	err      error
}

func (f *providerTestFakeClient) Ask(context.Context, string) (string, error) {
	return "", errors.New("unused")
}

func (f *providerTestFakeClient) Chat(_ context.Context, messages []llm.ChatMessage, _ llm.ChatOptions) (llm.ChatResponse, error) {
	f.messages = messages
	if f.err != nil {
		return llm.ChatResponse{}, f.err
	}
	return llm.ChatResponse{
		Message: llm.ChatMessage{Role: "assistant", Content: "연결되었습니다"},
	}, nil
}

func TestProviderHandler_usesRequestedProviderConfig(t *testing.T) {
	ctx := context.Background()
	store := newSettingsFixture(t)
	provider := settings.ProviderAnthropic
	apiKey := "sk-ant-test"
	model := "claude-sonnet-4-6"
	baseURL := "https://api.anthropic.com"
	if _, err := store.Set(ctx, settings.Patch{
		Provider: &provider,
		Providers: map[string]settings.ProviderConfig{
			provider: {APIKey: apiKey, Model: model, BaseURL: baseURL},
		},
	}); err != nil {
		t.Fatalf("settings.Set: %v", err)
	}

	client := &providerTestFakeClient{}
	var captured ai.ResolvedProvider
	handler := TestProvider(store, func(p ai.ResolvedProvider) (llm.Client, error) {
		captured = p
		return client, nil
	})

	raw, err := handler(ctx, json.RawMessage(`{"provider":"anthropic"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got testProviderResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.OK || got.Provider != provider || got.Model != model || got.Message != "연결되었습니다" {
		t.Fatalf("result = %+v", got)
	}
	if captured.Provider != provider || captured.APIKey != apiKey || captured.Model != model || captured.BaseURL != baseURL {
		t.Fatalf("captured provider = %+v", captured)
	}
	if len(client.messages) != 2 || client.messages[1].Content == "" {
		t.Fatalf("test prompt not sent: %+v", client.messages)
	}
}

func TestProviderHandler_usesOpenRouterConfig(t *testing.T) {
	ctx := context.Background()
	store := newSettingsFixture(t)
	provider := settings.ProviderOpenRouter
	apiKey := "or-test"
	if _, err := store.Set(ctx, settings.Patch{
		Provider: &provider,
		Providers: map[string]settings.ProviderConfig{
			provider: {APIKey: apiKey},
		},
	}); err != nil {
		t.Fatalf("settings.Set: %v", err)
	}

	var captured ai.ResolvedProvider
	client := &providerTestFakeClient{}
	handler := TestProvider(store, func(p ai.ResolvedProvider) (llm.Client, error) {
		captured = p
		return client, nil
	})

	raw, err := handler(ctx, json.RawMessage(`{"provider":"openrouter"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got testProviderResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Provider != provider || got.Model != settings.DefaultOpenRouterModel {
		t.Fatalf("result = %+v", got)
	}
	if captured.Provider != provider || captured.APIKey != apiKey || captured.Model != settings.DefaultOpenRouterModel || captured.BaseURL != settings.OpenRouterBaseURL {
		t.Fatalf("captured provider = %+v", captured)
	}
	if captured.MaxTokens != 64 {
		t.Fatalf("max tokens = %d, want 64", captured.MaxTokens)
	}
	if len(client.messages) != 2 || client.messages[1].Content == "" {
		t.Fatalf("test prompt not sent: %+v", client.messages)
	}
}

func TestProviderHandler_usesOpenAICodexDefaultModel(t *testing.T) {
	store := newSettingsFixture(t)
	var captured ai.ResolvedProvider
	handler := TestProvider(store, func(p ai.ResolvedProvider) (llm.Client, error) {
		captured = p
		return &providerTestFakeClient{}, nil
	})

	raw, err := handler(context.Background(), json.RawMessage(`{"provider":"openai-codex"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got testProviderResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if captured.Provider != settings.ProviderOpenAICodex || captured.Model != settings.DefaultOpenAICodexModel {
		t.Fatalf("captured provider = %+v", captured)
	}
	if got.Model != settings.DefaultOpenAICodexModel {
		t.Fatalf("result model=%q, want %q", got.Model, settings.DefaultOpenAICodexModel)
	}
}

func TestProviderHandler_propagatesConsentOnlyForActiveProvider(t *testing.T) {
	ctx := context.Background()
	store := newSettingsFixture(t)
	version := settings.AIDataSharingConsentVersion
	consentedAt := int64(1_720_000_000_000)
	if _, err := store.Set(ctx, settings.Patch{
		AIDataSharingConsentVersion: &version,
		AIDataSharingConsentedAt:    &consentedAt,
	}); err != nil {
		t.Fatalf("grant consent: %v", err)
	}

	var captured []ai.ResolvedProvider
	handler := TestProvider(store, func(p ai.ResolvedProvider) (llm.Client, error) {
		captured = append(captured, p)
		return &providerTestFakeClient{}, nil
	})

	for _, provider := range []string{settings.ProviderOpenAICodex, settings.ProviderAnthropic} {
		params, err := json.Marshal(testProviderParams{Provider: provider})
		if err != nil {
			t.Fatalf("marshal params: %v", err)
		}
		if _, err := handler(ctx, params); err != nil {
			t.Fatalf("test provider %q: %v", provider, err)
		}
	}

	if len(captured) != 2 {
		t.Fatalf("captured providers = %d, want 2", len(captured))
	}
	if !captured[0].DataSharingConsent {
		t.Fatal("active provider should receive stored data-sharing consent")
	}
	if captured[1].DataSharingConsent {
		t.Fatal("non-active provider must not reuse another provider's data-sharing consent")
	}
}

func TestProviderHandler_returnsFactoryError(t *testing.T) {
	store := newSettingsFixture(t)
	handler := TestProvider(store, func(ai.ResolvedProvider) (llm.Client, error) {
		return nil, errors.New("missing credential")
	})

	if _, err := handler(context.Background(), json.RawMessage(`{"provider":"openai"}`)); err == nil {
		t.Fatal("expected error")
	}
}

func TestProviderHandler_sanitizesOpenRouterCreditLimitError(t *testing.T) {
	ctx := context.Background()
	store := newSettingsFixture(t)
	provider := settings.ProviderOpenRouter
	if _, err := store.Set(ctx, settings.Patch{
		Provider: &provider,
		Providers: map[string]settings.ProviderConfig{
			provider: {APIKey: "or-test"},
		},
	}); err != nil {
		t.Fatalf("settings.Set: %v", err)
	}

	rawErr := errors.New(`openai status 402: {"error":{"message":"This request requires more credits, or fewer max_tokens. You requested up to 65536 tokens, but can only afford 2487. To increase, visit https://openrouter.ai/workspaces/default/keys/3b58c93c9d00cdc7b06afea22e6f1a66b956dd70fc03affef8a099605cb7dffb and adjust the key's total limit","code":402},"user_id":"user_3FRaYxdFVnEeX3jxWwN7Kn5pVbE"}`)
	handler := TestProvider(store, func(ai.ResolvedProvider) (llm.Client, error) {
		return &providerTestFakeClient{err: rawErr}, nil
	})

	_, err := handler(ctx, json.RawMessage(`{"provider":"openrouter"}`))
	if err == nil {
		t.Fatal("expected error")
	}
	var methodErr *rpc.MethodError
	if !errors.As(err, &methodErr) {
		t.Fatalf("error type=%T, want MethodError", err)
	}
	if !strings.Contains(methodErr.Message, "OpenRouter 크레딧 또는 키 한도가 부족합니다") {
		t.Fatalf("message=%q, want friendly OpenRouter credit guidance", methodErr.Message)
	}
	if !strings.Contains(methodErr.Message, "64토큰") {
		t.Fatalf("message=%q, want connection-test token limit hint", methodErr.Message)
	}
	for _, leaked := range []string{"workspaces/default/keys", "3b58c93c9d00cdc7", "user_3FRaY"} {
		if strings.Contains(methodErr.Message, leaked) {
			t.Fatalf("message leaked %q: %s", leaked, methodErr.Message)
		}
	}
}
