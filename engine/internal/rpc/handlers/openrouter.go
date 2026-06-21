package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/openrouter"
	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/settings"
)

type openRouterKeyInfoClient func(context.Context, string) (openrouter.KeyInfo, error)

type openRouterOAuth interface {
	Start(context.Context) (openrouter.OAuthStart, error)
	Finish(context.Context, string) (string, error)
}

type openRouterKeyInfoResult struct {
	OK                 bool     `json:"ok"`
	Provider           string   `json:"provider"`
	Label              string   `json:"label,omitempty"`
	Limit              *float64 `json:"limit,omitempty"`
	LimitReset         *string  `json:"limit_reset,omitempty"`
	LimitRemaining     *float64 `json:"limit_remaining,omitempty"`
	IncludeBYOKInLimit bool     `json:"include_byok_in_limit"`
	Usage              float64  `json:"usage"`
	UsageDaily         float64  `json:"usage_daily"`
	UsageWeekly        float64  `json:"usage_weekly"`
	UsageMonthly       float64  `json:"usage_monthly"`
	BYOKUsage          float64  `json:"byok_usage"`
	BYOKUsageDaily     float64  `json:"byok_usage_daily"`
	BYOKUsageWeekly    float64  `json:"byok_usage_weekly"`
	BYOKUsageMonthly   float64  `json:"byok_usage_monthly"`
	IsFreeTier         bool     `json:"is_free_tier"`
}

type openRouterOAuthFinishParams struct {
	RequestID string `json:"request_id"`
}

type openRouterOAuthFinishResult struct {
	OK       bool   `json:"ok"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Message  string `json:"message"`
}

func OpenRouterOAuthStart(oauth openRouterOAuth) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		if oauth == nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: "OpenRouter OAuth is not configured"}
		}
		start, err := oauth.Start(ctx)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		return json.Marshal(start)
	}
}

func OpenRouterOAuthFinish(store *settings.Store, oauth openRouterOAuth) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		if oauth == nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: "OpenRouter OAuth is not configured"}
		}
		var p openRouterOAuthFinishParams
		if len(params) > 0 {
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, &rpc.MethodError{Code: rpc.CodeInvalidParams, Message: err.Error()}
			}
		}
		key, err := oauth.Finish(ctx, p.RequestID)
		if err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: err.Error()}
		}
		provider := settings.ProviderOpenRouter
		if _, err := store.Set(ctx, settings.Patch{
			Provider: &provider,
			Providers: map[string]settings.ProviderConfig{
				provider: {APIKey: key},
			},
		}); err != nil {
			return nil, &rpc.MethodError{Code: rpc.CodeInternalError, Message: fmt.Sprintf("OpenRouter 키를 저장하지 못했습니다: %v", err)}
		}
		return json.Marshal(openRouterOAuthFinishResult{
			OK:       true,
			Provider: provider,
			Model:    settings.DefaultOpenRouterModel,
			Message:  "OpenRouter 연결이 완료되었습니다.",
		})
	}
}

func OpenRouterKeyInfo(store *settings.Store, client openRouterKeyInfoClient) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		cfg := store.ProviderConfigFor(settings.ProviderOpenRouter)
		if strings.TrimSpace(cfg.APIKey) == "" {
			return nil, &rpc.MethodError{
				Code:    rpc.CodeInternalError,
				Message: "OpenRouter API 키가 저장되어 있지 않습니다. 키를 저장한 뒤 다시 확인하세요.",
			}
		}
		if client == nil {
			client = openrouter.FetchKeyInfo
		}
		info, err := client(ctx, cfg.APIKey)
		if err != nil {
			return nil, &rpc.MethodError{
				Code:    rpc.CodeInternalError,
				Message: fmt.Sprintf("OpenRouter 한도 상태를 확인하지 못했습니다: %v", err),
			}
		}
		return json.Marshal(openRouterKeyInfoResult{
			OK:                 true,
			Provider:           settings.ProviderOpenRouter,
			Label:              info.Label,
			Limit:              info.Limit,
			LimitReset:         info.LimitReset,
			LimitRemaining:     info.LimitRemaining,
			IncludeBYOKInLimit: info.IncludeBYOKInLimit,
			Usage:              info.Usage,
			UsageDaily:         info.UsageDaily,
			UsageWeekly:        info.UsageWeekly,
			UsageMonthly:       info.UsageMonthly,
			BYOKUsage:          info.BYOKUsage,
			BYOKUsageDaily:     info.BYOKUsageDaily,
			BYOKUsageWeekly:    info.BYOKUsageWeekly,
			BYOKUsageMonthly:   info.BYOKUsageMonthly,
			IsFreeTier:         info.IsFreeTier,
		})
	}
}
