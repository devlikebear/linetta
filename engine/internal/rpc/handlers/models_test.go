package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/ai"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/tars/pkg/llm"
)

type providerTestFakeClient struct {
	messages []llm.ChatMessage
}

func (f *providerTestFakeClient) Ask(context.Context, string) (string, error) {
	return "", errors.New("unused")
}

func (f *providerTestFakeClient) Chat(_ context.Context, messages []llm.ChatMessage, _ llm.ChatOptions) (llm.ChatResponse, error) {
	f.messages = messages
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

func TestProviderHandler_returnsFactoryError(t *testing.T) {
	store := newSettingsFixture(t)
	handler := TestProvider(store, func(ai.ResolvedProvider) (llm.Client, error) {
		return nil, errors.New("missing credential")
	})

	if _, err := handler(context.Background(), json.RawMessage(`{"provider":"openai"}`)); err == nil {
		t.Fatal("expected error")
	}
}
