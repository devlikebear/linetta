package modelcatalog

import (
	"context"
	"errors"
	"testing"

	"github.com/devlikebear/tars/pkg/llm"
)

type fakeFetcher struct {
	models     []string
	err        error
	gotOptions llm.ProviderOptions
}

func (f *fakeFetcher) FetchModels(ctx context.Context, opts llm.ProviderOptions) ([]string, error) {
	f.gotOptions = opts
	return f.models, f.err
}

func TestListClaudeCliReturnsEmptyWithoutFetching(t *testing.T) {
	f := &fakeFetcher{models: []string{"should-not-appear"}}
	c := New(f)
	got, err := c.List(context.Background(), "claude-code-cli", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty list, got %v", got)
	}
}

func TestListPassesProviderAndKey(t *testing.T) {
	f := &fakeFetcher{models: []string{"a", "b"}}
	c := New(f)
	got, err := c.List(context.Background(), "anthropic", "key-123", "https://api.minimax.io/v1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
	if f.gotOptions.Provider != "anthropic" || f.gotOptions.APIKey != "key-123" || f.gotOptions.BaseURL != "https://api.minimax.io/v1" {
		t.Fatalf("options mismatch: %+v", f.gotOptions)
	}
}

func TestListMapsOpenRouterToOpenAICompatible(t *testing.T) {
	f := &fakeFetcher{models: []string{"openrouter/auto"}}
	c := New(f)
	got, err := c.List(context.Background(), "openrouter", "or-test", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "openrouter/auto" {
		t.Fatalf("got %v", got)
	}
	if f.gotOptions.Provider != "openai" || f.gotOptions.APIKey != "or-test" || f.gotOptions.BaseURL != "https://openrouter.ai/api/v1" {
		t.Fatalf("options mismatch: %+v", f.gotOptions)
	}
}

func TestListPropagatesError(t *testing.T) {
	c := New(&fakeFetcher{err: errors.New("boom")})
	if _, err := c.List(context.Background(), "openai", "k", ""); err == nil {
		t.Fatal("expected error")
	}
}

func TestListOAuthScopeErrorSoftFails(t *testing.T) {
	// ChatGPT OAuth tokens lack api.model.read; the models endpoint returns 403.
	// Mirroring tars, this becomes an empty list (manual entry), not an error.
	for _, status := range []int{401, 403} {
		c := New(&fakeFetcher{err: &llm.ProviderError{Provider: "openai-codex", StatusCode: status, Message: "insufficient permissions"}})
		got, err := c.List(context.Background(), "openai-codex", "", "")
		if err != nil {
			t.Fatalf("status %d: expected soft fail, got error %v", status, err)
		}
		if len(got) != 0 {
			t.Fatalf("status %d: expected empty list, got %v", status, got)
		}
	}
}
