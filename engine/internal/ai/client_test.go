//go:build !mas && !mobile

package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/tars/pkg/llm"
)

func TestDefaultClientFactorySetsClaudeCliPath(t *testing.T) {
	os.Unsetenv("CLAUDE_CODE_CLI_PATH")
	// claude-code-cli with a bogus path: NewProvider may fail to find the binary,
	// but the env var must be set first — that is the behavior under test.
	_, _ = DefaultClientFactory(ResolvedProvider{
		Provider: "claude-code-cli",
		CliPath:  "/tmp/does-not-exist-claude",
	})
	if got := os.Getenv("CLAUDE_CODE_CLI_PATH"); got != "/tmp/does-not-exist-claude" {
		t.Fatalf("CLAUDE_CODE_CLI_PATH=%q, want /tmp/does-not-exist-claude", got)
	}
}

func TestProviderOptionsForTarsMapsOpenRouterToOpenAICompatible(t *testing.T) {
	got := providerOptionsForTars(ResolvedProvider{
		Provider: settings.ProviderOpenRouter,
		Model:    settings.DefaultOpenRouterModel,
		APIKey:   "or-test",
		BaseURL:  settings.OpenRouterBaseURL,
	})
	if got.Provider != settings.ProviderOpenAI {
		t.Fatalf("provider=%q, want openai", got.Provider)
	}
	if got.BaseURL != settings.OpenRouterBaseURL || got.Model != settings.DefaultOpenRouterModel || got.APIKey != "or-test" {
		t.Fatalf("options mismatch: %+v", got)
	}
	if got.MaxTokens != settings.OpenRouterDefaultMaxTokens {
		t.Fatalf("max tokens=%d, want %d", got.MaxTokens, settings.OpenRouterDefaultMaxTokens)
	}
}

func TestProviderOptionsForTarsPreservesExplicitOpenRouterMaxTokens(t *testing.T) {
	got := providerOptionsForTars(ResolvedProvider{
		Provider:  settings.ProviderOpenRouter,
		Model:     settings.DefaultOpenRouterModel,
		APIKey:    "or-test",
		BaseURL:   settings.OpenRouterBaseURL,
		MaxTokens: 64,
	})
	if got.Provider != settings.ProviderOpenAI {
		t.Fatalf("provider=%q, want openai", got.Provider)
	}
	if got.MaxTokens != 64 {
		t.Fatalf("max tokens=%d, want 64", got.MaxTokens)
	}
}

func TestDefaultClientFactoryOpenRouterSendsMaxTokens(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path=%q, want /v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer or-test" {
			t.Fatalf("authorization=%q, want bearer key", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"연결되었습니다"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer server.Close()

	client, err := DefaultClientFactory(ResolvedProvider{
		Provider:  settings.ProviderOpenRouter,
		Model:     settings.DefaultOpenRouterModel,
		APIKey:    "or-test",
		BaseURL:   server.URL + "/v1",
		MaxTokens: 64,
	})
	if err != nil {
		t.Fatalf("DefaultClientFactory: %v", err)
	}

	_, err = client.Chat(context.Background(), []llm.ChatMessage{
		{Role: "user", Content: "ping"},
	}, llm.ChatOptions{})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got := requestBody["max_tokens"]; got != float64(64) {
		t.Fatalf("max_tokens=%v, want 64; body=%+v", got, requestBody)
	}
}
