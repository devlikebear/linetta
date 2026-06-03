package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/devlikebear/linetta/engine/internal/ai"
	"github.com/devlikebear/linetta/engine/internal/clidetect"
	"github.com/devlikebear/linetta/engine/internal/modelcatalog"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/tars/pkg/llm"
)

type listModelsParams struct {
	Provider string `json:"provider"`
}

type listModelsResult struct {
	Models []string `json:"models"`
}

// ListModels returns a handler for providers.list_models. It resolves the API
// key for the requested provider from settings and asks the catalog for the
// live model list. With no provider in the request, the active provider is used.
func ListModels(store *settings.Store, catalog *modelcatalog.Catalog) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p listModelsParams
		if len(params) > 0 {
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
			}
		}
		provider := p.Provider
		if provider == "" {
			provider = store.Provider()
		}
		cfg := store.ProviderConfigFor(provider)
		models, err := catalog.List(ctx, provider, cfg.APIKey, cfg.BaseURL)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(listModelsResult{Models: models})
	}
}

type detectCLIResult struct {
	Path string `json:"path"`
}

// DetectCLI returns a handler for providers.detect_cli. It locates the Claude
// Code CLI executable (PATH, login shell, then known install dirs) and returns
// the resolved path, or an empty string when not found.
func DetectCLI() rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		return json.Marshal(detectCLIResult{Path: clidetect.Detect(ctx)})
	}
}

type testProviderParams struct {
	Provider string `json:"provider"`
}

type testProviderResult struct {
	OK       bool   `json:"ok"`
	Provider string `json:"provider"`
	Model    string `json:"model,omitempty"`
	Message  string `json:"message"`
}

// TestProvider sends a tiny, context-free prompt through the selected provider.
// It is intentionally separate from ai.run so Settings can verify credentials
// before the writer creates or opens a scene.
func TestProvider(store *settings.Store, factory ai.ClientFactory) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var p testProviderParams
		if len(params) > 0 {
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
			}
		}
		resolved := store.Resolve()
		if strings.TrimSpace(p.Provider) != "" {
			cfg := store.ProviderConfigFor(p.Provider)
			resolved = settings.ProviderSettings{
				Provider: p.Provider,
				Model:    cfg.Model,
				APIKey:   cfg.APIKey,
				BaseURL:  cfg.BaseURL,
				CliPath:  cfg.CliPath,
			}
		}
		rp := ai.ResolvedProvider{
			Provider: resolved.Provider,
			Model:    resolved.Model,
			APIKey:   resolved.APIKey,
			BaseURL:  resolved.BaseURL,
			CliPath:  resolved.CliPath,
		}
		client, err := factory(rp)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		testCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		resp, err := client.Chat(testCtx, []llm.ChatMessage{
			{Role: "system", Content: "당신은 Linetta의 AI 연결 테스트입니다. 아주 짧게 한국어로만 답하세요."},
			{Role: "user", Content: "연결 테스트입니다. '연결되었습니다'라고만 답하세요."},
		}, llm.ChatOptions{})
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		msg := strings.TrimSpace(resp.Message.Content)
		if msg == "" {
			msg = "응답을 받았습니다."
		}
		return json.Marshal(testProviderResult{
			OK:       true,
			Provider: rp.Provider,
			Model:    rp.Model,
			Message:  msg,
		})
	}
}
