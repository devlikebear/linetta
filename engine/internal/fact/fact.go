// Package fact owns Fact Book cards and their source URLs.
package fact

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
