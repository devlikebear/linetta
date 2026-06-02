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
	got, err := c.List(context.Background(), "claude-code-cli", "")
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
	got, err := c.List(context.Background(), "anthropic", "key-123")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
	if f.gotOptions.Provider != "anthropic" || f.gotOptions.APIKey != "key-123" {
		t.Fatalf("options mismatch: %+v", f.gotOptions)
	}
}

func TestListPropagatesError(t *testing.T) {
	c := New(&fakeFetcher{err: errors.New("boom")})
	if _, err := c.List(context.Background(), "openai", "k"); err == nil {
		t.Fatal("expected error")
	}
}
