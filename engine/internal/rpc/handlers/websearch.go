package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/settings"
	tarstools "github.com/devlikebear/tars/pkg/tools"
)

type webSearchTestRequest struct {
	Provider string
	APIKey   string
	Query    string
}

type webSearchTester func(context.Context, webSearchTestRequest) (string, error)

type testWebSearchResult struct {
	OK       bool   `json:"ok"`
	Provider string `json:"provider"`
	Message  string `json:"message"`
}

// TestWebSearch verifies the saved web_search provider and API key without
// running a Companion turn. It preserves secret-store read errors so Settings
// can explain whether Keychain access is the failing layer.
func TestWebSearch(store *settings.Store, tester webSearchTester) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		provider := store.WebSearchProvider()
		apiKey, ok, err := store.WebSearchAPIKeyStatus()
		if err != nil {
			return nil, &rpc.MethodError{
				Code:    rpc.CodeInternalError,
				Message: fmt.Sprintf("web_search API 키를 Keychain에서 읽지 못했습니다: %v", err),
			}
		}
		if !ok || strings.TrimSpace(apiKey) == "" {
			return nil, &rpc.MethodError{
				Code:    rpc.CodeInternalError,
				Message: "web_search API 키가 저장되어 있지 않습니다. 설정에서 API 키를 저장한 뒤 다시 테스트하세요.",
			}
		}
		if tester == nil {
			tester = DefaultWebSearchTester
		}
		testCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		msg, err := tester(testCtx, webSearchTestRequest{
			Provider: provider,
			APIKey:   apiKey,
			Query:    "Linetta web_search connection test",
		})
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		msg = strings.TrimSpace(msg)
		if msg == "" {
			msg = "응답을 받았습니다."
		}
		return json.Marshal(testWebSearchResult{OK: true, Provider: provider, Message: msg})
	}
}

func DefaultWebSearchTester(ctx context.Context, req webSearchTestRequest) (string, error) {
	provider := strings.TrimSpace(strings.ToLower(req.Provider))
	if provider == "" {
		provider = "brave"
	}
	opts := tarstools.WebSearchOptions{
		Enabled:  true,
		Provider: provider,
		CacheTTL: 0,
	}
	switch provider {
	case "brave":
		opts.BraveAPIKey = req.APIKey
	case "perplexity":
		opts.PerplexityAPIKey = req.APIKey
	default:
		return "", fmt.Errorf("provider must be one of: brave|perplexity")
	}
	tool := tarstools.NewWebSearchToolWithOptions(opts)
	payload, err := json.Marshal(map[string]any{
		"query":    strings.TrimSpace(req.Query),
		"count":    1,
		"provider": provider,
	})
	if err != nil {
		return "", err
	}
	result, err := tool.Execute(ctx, json.RawMessage(payload))
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(result.Text())
	if result.IsError {
		if text == "" {
			text = "web_search connection test failed"
		}
		return "", errors.New(extractWebSearchMessage(text))
	}
	return summarizeWebSearchTestResult(text)
}

func extractWebSearchMessage(text string) string {
	var payload struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err == nil && strings.TrimSpace(payload.Message) != "" {
		return strings.TrimSpace(payload.Message)
	}
	return text
}

func summarizeWebSearchTestResult(text string) (string, error) {
	var payload struct {
		Count   int    `json:"count"`
		Message string `json:"message"`
		Results []struct {
			Title string `json:"title"`
			URL   string `json:"url"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return "응답을 받았습니다.", nil
	}
	if payload.Message != "" && payload.Count == 0 && len(payload.Results) == 0 {
		return strings.TrimSpace(payload.Message), nil
	}
	if payload.Count > 0 {
		return fmt.Sprintf("검색 결과 %d건 응답", payload.Count), nil
	}
	if len(payload.Results) > 0 {
		return fmt.Sprintf("검색 결과 %d건 응답", len(payload.Results)), nil
	}
	return "응답을 받았습니다.", nil
}
