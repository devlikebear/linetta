package openrouter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientKeyInfoFetchesCreditsAndLimits(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		if r.URL.Path != "/key" {
			t.Fatalf("path=%q, want /key", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"label":"Linetta","limit":10,"limit_reset":"monthly","limit_remaining":7.5,"include_byok_in_limit":true,"usage":2.5,"usage_daily":0.1,"usage_weekly":0.5,"usage_monthly":1.2,"byok_usage":0,"byok_usage_daily":0,"byok_usage_weekly":0,"byok_usage_monthly":0,"is_free_tier":false}}`))
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, srv.Client()).KeyInfo(context.Background(), "or-test")
	if err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer or-test" {
		t.Fatalf("Authorization=%q", auth)
	}
	if got.Label != "Linetta" || got.Limit == nil || *got.Limit != 10 || got.LimitRemaining == nil || *got.LimitRemaining != 7.5 {
		t.Fatalf("key info mismatch: %+v", got)
	}
	if got.UsageMonthly != 1.2 || !got.IncludeBYOKInLimit || got.IsFreeTier {
		t.Fatalf("usage fields mismatch: %+v", got)
	}
}

func TestClientKeyInfoRequiresAPIKey(t *testing.T) {
	_, err := NewClient("http://example.invalid", nil).KeyInfo(context.Background(), " ")
	if err == nil {
		t.Fatal("expected missing key error")
	}
}

func TestClientModelsFetchesModelIDs(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		if r.URL.Path != "/models" {
			t.Fatalf("path=%q, want /models", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"anthropic/claude-sonnet-4","name":"Claude Sonnet 4"},{"id":"openai/gpt-4o","name":"GPT-4o"},{"id":"","name":"broken"}]}`))
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, srv.Client()).Models(context.Background(), "or-test")
	if err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer or-test" {
		t.Fatalf("Authorization=%q", auth)
	}
	if len(got) != 2 || got[0].ID != "anthropic/claude-sonnet-4" || got[1].ID != "openai/gpt-4o" {
		t.Fatalf("models mismatch: %+v", got)
	}
}
