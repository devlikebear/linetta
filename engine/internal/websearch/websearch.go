// Package websearch asks a self-hosted RustXNG server for search results.
//
// Linetta neither installs nor starts that server: the writer runs it and
// gives Settings its address. Only snippets are requested. Page text
// (include_raw_content) stays off, so a search never has the server fetch a
// page on an agent's behalf.
package websearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// DefaultResults and MaxResults bound one search, so a model cannot ask
	// for the 200 results the server allows.
	DefaultResults = 5
	MaxResults     = 10

	requestTimeout  = 15 * time.Second
	maxSnippetRunes = 500
	maxBodyBytes    = 2 << 20
)

// Result is one hit, reduced to what a model needs.
type Result struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

// Client talks to one server. The zero value uses a default HTTP client.
type Client struct {
	HTTP *http.Client
}

// ValidateBaseURL reports whether base is an http(s) address with a host.
func ValidateBaseURL(base string) error {
	u, err := url.Parse(base)
	if err != nil {
		return fmt.Errorf("websearch: invalid address: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("websearch: address must be an http or https URL")
	}
	return nil
}

// Search returns up to limit results for query from the server at base.
func (c Client) Search(ctx context.Context, base, query string, limit int) ([]Result, error) {
	if err := ValidateBaseURL(base); err != nil {
		return nil, err
	}
	if limit < 1 {
		limit = DefaultResults
	}
	if limit > MaxResults {
		limit = MaxResults
	}
	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("websearch: invalid address: %w", err)
	}
	u.Path = strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/search") + "/search"
	q := u.Query()
	q.Set("q", query)
	q.Set("format", "json")
	q.Set("max_results", strconv.Itoa(limit))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("websearch: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: requestTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("websearch: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("websearch: server answered %d", resp.StatusCode)
	}
	var payload struct {
		Results []Result `json:"results"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodyBytes)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("websearch: decode response: %w", err)
	}
	results := make([]Result, 0, limit)
	for _, r := range payload.Results {
		if len(results) == limit {
			break
		}
		results = append(results, Result{
			Title:   strings.TrimSpace(r.Title),
			URL:     strings.TrimSpace(r.URL),
			Content: clip(strings.TrimSpace(r.Content)),
		})
	}
	return results, nil
}

func clip(s string) string {
	if utf8.RuneCountInString(s) <= maxSnippetRunes {
		return s
	}
	return string([]rune(s)[:maxSnippetRunes])
}
