// Package fact owns Fact Book cards and their source URLs.
package fact

import (
	"encoding/json"
	"strconv"
	"strings"
)

const (
	StatusVerified           = "verified"
	StatusUncertain          = "uncertain"
	StatusIntentionalFiction = "intentional_fiction"
	StatusStale              = "stale"
)

type Card struct {
	ID        string   `json:"id"`
	ProjectID string   `json:"project_id"`
	NodeID    *string  `json:"node_id,omitempty"`
	Claim     string   `json:"claim"`
	Result    string   `json:"result"`
	Status    string   `json:"status"`
	Category  string   `json:"category"`
	Sources   []Source `json:"sources"`
	CreatedAt int64    `json:"created_at"`
	UpdatedAt int64    `json:"updated_at"`
}

type Source struct {
	ID         string `json:"id"`
	CardID     string `json:"card_id"`
	URL        string `json:"url"`
	Title      string `json:"title"`
	Snippet    string `json:"snippet"`
	AccessedAt int64  `json:"accessed_at"`
}

type SourceInput struct {
	URL        string `json:"url"`
	Title      string `json:"title,omitempty"`
	Snippet    string `json:"snippet,omitempty"`
	AccessedAt int64  `json:"accessed_at,omitempty"`
}

func (s *SourceInput) UnmarshalJSON(data []byte) error {
	var in struct {
		URL        string          `json:"url"`
		Title      string          `json:"title,omitempty"`
		Snippet    string          `json:"snippet,omitempty"`
		AccessedAt json.RawMessage `json:"accessed_at,omitempty"`
	}
	if err := json.Unmarshal(data, &in); err != nil {
		return err
	}
	accessedAt, err := parseOptionalAccessedAt(in.AccessedAt)
	if err != nil {
		return err
	}
	*s = SourceInput{
		URL:        in.URL,
		Title:      in.Title,
		Snippet:    in.Snippet,
		AccessedAt: accessedAt,
	}
	return nil
}

func parseOptionalAccessedAt(raw json.RawMessage) (int64, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return 0, nil
	}
	var n int64
	if err := json.Unmarshal(raw, &n); err == nil {
		return n, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		s = strings.TrimSpace(s)
		if s == "" {
			return 0, nil
		}
		parsed, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, nil
		}
		return parsed, nil
	}
	return 0, nil
}

type NewInput struct {
	ProjectID string        `json:"project_id"`
	NodeID    *string       `json:"node_id,omitempty"`
	Claim     string        `json:"claim"`
	Result    string        `json:"result"`
	Status    string        `json:"status"`
	Category  string        `json:"category,omitempty"`
	Sources   []SourceInput `json:"sources"`
}

type UpdateInput struct {
	ID       string  `json:"id"`
	Claim    *string `json:"claim,omitempty"`
	Result   *string `json:"result,omitempty"`
	Status   *string `json:"status,omitempty"`
	Category *string `json:"category,omitempty"`
}

type ListFilter struct {
	ProjectID string  `json:"project_id"`
	NodeID    *string `json:"node_id,omitempty"`
	Limit     int     `json:"limit,omitempty"`
}

func ValidStatus(v string) bool {
	switch v {
	case StatusVerified, StatusUncertain, StatusIntentionalFiction, StatusStale:
		return true
	default:
		return false
	}
}
