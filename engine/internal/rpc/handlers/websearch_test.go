package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/settings"
)

func TestWebSearchHandler_usesStoredProviderAndKey(t *testing.T) {
	ctx := context.Background()
	store := newSettingsFixture(t)
	provider := "perplexity"
	apiKey := "pplx-test"
	if _, err := store.Set(ctx, settings.Patch{
		WebSearchProvider: &provider,
		WebSearchAPIKey:   &apiKey,
	}); err != nil {
		t.Fatalf("settings.Set: %v", err)
	}

	var captured webSearchTestRequest
	handler := TestWebSearch(store, func(_ context.Context, req webSearchTestRequest) (string, error) {
		captured = req
		return "검색 결과 1건 응답", nil
	})

	raw, err := handler(ctx, nil)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got testWebSearchResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.OK || got.Provider != provider || got.Message != "검색 결과 1건 응답" {
		t.Fatalf("result = %+v", got)
	}
	if captured.Provider != provider || captured.APIKey != apiKey || captured.Query == "" {
		t.Fatalf("captured request = %+v", captured)
	}
}

func TestWebSearchHandler_requiresStoredKey(t *testing.T) {
	store := newSettingsFixture(t)
	handler := TestWebSearch(store, func(context.Context, webSearchTestRequest) (string, error) {
		t.Fatal("tester should not be called without an API key")
		return "", nil
	})

	_, err := handler(context.Background(), nil)
	var methodErr *rpc.MethodError
	if !errors.As(err, &methodErr) || methodErr.Code != rpc.CodeInternalError {
		t.Fatalf("expected internal rpc error, got %T %v", err, err)
	}
	if !strings.Contains(methodErr.Message, "web_search API 키") {
		t.Fatalf("error message = %q", methodErr.Message)
	}
}

func TestWebSearchHandler_reportsSecretStoreReadError(t *testing.T) {
	t.Setenv("LINETTA_HOME", t.TempDir())
	store, err := settings.NewWithSecretStore(failingSecretStore{err: errors.New("keychain locked")})
	if err != nil {
		t.Fatalf("NewWithSecretStore: %v", err)
	}
	handler := TestWebSearch(store, func(context.Context, webSearchTestRequest) (string, error) {
		t.Fatal("tester should not be called when keychain read fails")
		return "", nil
	})

	_, err = handler(context.Background(), nil)
	var methodErr *rpc.MethodError
	if !errors.As(err, &methodErr) || methodErr.Code != rpc.CodeInternalError {
		t.Fatalf("expected internal rpc error, got %T %v", err, err)
	}
	if !strings.Contains(methodErr.Message, "keychain locked") {
		t.Fatalf("error message = %q", methodErr.Message)
	}
}

func TestWebSearchHandler_reportsTesterError(t *testing.T) {
	ctx := context.Background()
	store := newSettingsFixture(t)
	apiKey := "bsa-test"
	if _, err := store.Set(ctx, settings.Patch{WebSearchAPIKey: &apiKey}); err != nil {
		t.Fatalf("settings.Set: %v", err)
	}
	handler := TestWebSearch(store, func(context.Context, webSearchTestRequest) (string, error) {
		return "", errors.New("brave status 401")
	})

	_, err := handler(ctx, nil)
	var methodErr *rpc.MethodError
	if !errors.As(err, &methodErr) || methodErr.Code != rpc.CodeInternalError {
		t.Fatalf("expected internal rpc error, got %T %v", err, err)
	}
	if !strings.Contains(methodErr.Message, "brave status 401") {
		t.Fatalf("error message = %q", methodErr.Message)
	}
}

func TestDefaultWebSearchTester_extractsToolErrorMessage(t *testing.T) {
	_, err := DefaultWebSearchTester(context.Background(), webSearchTestRequest{
		Provider: "brave",
		APIKey:   "",
		Query:    "Linetta web_search connection test",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got != "web_search brave api key is required" {
		t.Fatalf("error = %q", got)
	}
}

type failingSecretStore struct {
	err error
}

func (f failingSecretStore) Get(string) (string, bool, error) { return "", false, f.err }
func (f failingSecretStore) Set(string, string) error         { return nil }
func (f failingSecretStore) Delete(string) error              { return nil }
