// Package openrouter contains narrow integrations for OpenRouter-specific
// account metadata that is not covered by the OpenAI-compatible chat API.
package openrouter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/devlikebear/linetta/engine/internal/settings"
)

type KeyInfo struct {
	Label              string   `json:"label"`
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

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string, httpClient *http.Client) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = settings.OpenRouterBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &Client{baseURL: baseURL, httpClient: httpClient}
}

func FetchKeyInfo(ctx context.Context, apiKey string) (KeyInfo, error) {
	return NewClient(settings.OpenRouterBaseURL, nil).KeyInfo(ctx, apiKey)
}

func (c *Client) KeyInfo(ctx context.Context, apiKey string) (KeyInfo, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return KeyInfo{}, errors.New("openrouter api key is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/key", nil)
	if err != nil {
		return KeyInfo{}, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return KeyInfo{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = res.Status
		}
		return KeyInfo{}, fmt.Errorf("openrouter key info failed: %s", msg)
	}
	var payload struct {
		Data KeyInfo `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return KeyInfo{}, err
	}
	return payload.Data, nil
}
