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

type Model struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	ContextLength int        `json:"context_length,omitempty"`
	Pricing       PricingMap `json:"pricing,omitempty"`
}

// PricingMap holds the per-token price strings OpenRouter reports for a model.
//
// The object is not a flat string map: OpenRouter adds keys over time, and some
// carry structured values rather than strings (for example "overrides", an
// array of time-window price tables). Decoding straight into map[string]string
// fails on those, and because the models endpoint returns one array, a single
// unknown value shape discards the entire catalogue.
//
// Keep the string entries and skip everything else, so an unrecognised value
// costs at most that one key.
type PricingMap map[string]string

func (p *PricingMap) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	out := make(PricingMap, len(raw))
	for key, value := range raw {
		var s string
		if err := json.Unmarshal(value, &s); err != nil {
			continue // structured or unexpected value: not a price string
		}
		out[key] = s
	}
	*p = out
	return nil
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

func FetchModels(ctx context.Context, apiKey string) ([]Model, error) {
	return NewClient(settings.OpenRouterBaseURL, nil).Models(ctx, apiKey)
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

func (c *Client) Models(ctx context.Context, apiKey string) ([]Model, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, errors.New("openrouter api key is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = res.Status
		}
		return nil, fmt.Errorf("openrouter models failed: %s", msg)
	}
	var payload struct {
		Data []Model `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, err
	}
	models := make([]Model, 0, len(payload.Data))
	for _, model := range payload.Data {
		model.ID = strings.TrimSpace(model.ID)
		if model.ID == "" {
			continue
		}
		models = append(models, model)
	}
	return models, nil
}
