// Package llm provides a small, dependency-free client for OpenAI's Chat
// Completions endpoint. We use it for one-shot tasks like episode blueprint
// suggestions where pulling in the full tessera task graph is overkill.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"
)

// Client is a minimal Chat Completions caller. Zero-value is unusable; build
// via NewFromEnv.
type Client struct {
	APIKey    string
	BaseURL   string // default https://api.openai.com/v1
	Model     string // default gpt-4o-mini
	HTTP      *http.Client
}

// NewFromEnv constructs a Client using OPENAI_API_KEY (required), with
// LINETTA_LLM_BASE_URL and LINETTA_LLM_MODEL as optional overrides. Returns
// nil + ErrNoAPIKey if the key is absent — callers should fall back to a
// deterministic path in that case.
func NewFromEnv() (*Client, error) {
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		return nil, ErrNoAPIKey
	}
	base := os.Getenv("LINETTA_LLM_BASE_URL")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	model := os.Getenv("LINETTA_LLM_MODEL")
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &Client{
		APIKey:  key,
		BaseURL: base,
		Model:   model,
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}, nil
}

// ErrNoAPIKey signals that the caller should use a non-LLM fallback path.
var ErrNoAPIKey = errors.New("llm: OPENAI_API_KEY not set")

// Message is a single role+content turn in the chat history.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is what we send. We hard-code response_format to JSON so callers
// can decode directly; the caller is responsible for prompting the model to
// emit valid JSON in the requested shape.
type ChatRequest struct {
	Model          string    `json:"model"`
	Messages       []Message `json:"messages"`
	Temperature    float64   `json:"temperature,omitempty"`
	ResponseFormat *struct {
		Type string `json:"type"`
	} `json:"response_format,omitempty"`
}

// ChatJSON sends messages and decodes the model's reply (assumed JSON) into out.
// Returns an error if the API call fails, the response is non-2xx, or the body
// is not valid JSON of the expected shape.
func (c *Client) ChatJSON(ctx context.Context, messages []Message, temperature float64, out any) error {
	if c == nil || c.APIKey == "" {
		return ErrNoAPIKey
	}
	req := ChatRequest{
		Model:       c.Model,
		Messages:    messages,
		Temperature: temperature,
		ResponseFormat: &struct {
			Type string `json:"type"`
		}{Type: "json_object"},
	}
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("llm.ChatJSON: marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("llm.ChatJSON: build request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return fmt.Errorf("llm.ChatJSON: http: %w", err)
	}
	defer resp.Body.Close()

	var raw struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return fmt.Errorf("llm.ChatJSON: decode response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if raw.Error != nil {
			return fmt.Errorf("llm.ChatJSON: %s (%s)", raw.Error.Message, raw.Error.Type)
		}
		return fmt.Errorf("llm.ChatJSON: HTTP %d", resp.StatusCode)
	}
	if len(raw.Choices) == 0 {
		return errors.New("llm.ChatJSON: no choices in response")
	}
	if err := json.Unmarshal([]byte(raw.Choices[0].Message.Content), out); err != nil {
		return fmt.Errorf("llm.ChatJSON: decode model JSON: %w (body=%q)", err, raw.Choices[0].Message.Content)
	}
	return nil
}
