package provider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/tars/pkg/llm"
)

func strPtr(v string) *string { return &v }
func int64Ptr(v int64) *int64 { return &v }

// fakeClient never dials. Ask is scripted per test; Chat is unused here.
type fakeClient struct {
	ask func(prompt string) (string, error)
}

func (f fakeClient) Ask(_ context.Context, prompt string) (string, error) { return f.ask(prompt) }
func (f fakeClient) Chat(_ context.Context, _ []llm.ChatMessage, _ llm.ChatOptions) (llm.ChatResponse, error) {
	return llm.ChatResponse{}, nil
}

func newSource(t *testing.T) (*Source, *settings.Store, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LINETTA_HOME", home)
	st, err := settings.NewWithSecretStore(settings.NewMemorySecretStore())
	if err != nil {
		t.Fatalf("settings: %v", err)
	}
	codexHome := filepath.Join(home, "codex")
	return NewSource(st, codexHome), st, codexHome
}

func reasonOf(t *testing.T, err error) string {
	t.Helper()
	var re *rpc.ReasonError
	if !errors.As(err, &re) {
		t.Fatalf("want a ReasonError, got %v", err)
	}
	return re.Reason
}

func configure(t *testing.T, st *settings.Store, id string, key string, consented bool) {
	t.Helper()
	pp := settings.ProviderPatch{APIKey: strPtr(key)}
	if consented {
		pp.ConsentedAt = int64Ptr(1700000000000)
	}
	if _, err := st.Set(context.Background(), settings.Patch{
		Provider:  strPtr(id),
		Providers: map[string]settings.ProviderPatch{id: pp},
	}); err != nil {
		t.Fatalf("configure %s: %v", id, err)
	}
}

func TestNewSource_forcesTheFileRefreshStoreForCodex(t *testing.T) {
	t.Setenv("TARS_OPENAI_CODEX_REFRESH_TOKEN_STORAGE", "")
	newSource(t)
	if got := os.Getenv("TARS_OPENAI_CODEX_REFRESH_TOKEN_STORAGE"); got != "file" {
		t.Fatalf("env = %q, want file (the sandbox cannot run the security CLI)", got)
	}
}

func TestResolve_emptyMeansTheActiveProvider(t *testing.T) {
	src, _, codexHome := newSource(t)
	r, err := src.Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if r.ID != settings.ProviderOpenAICodex {
		t.Errorf("default active provider = %q, want openai-codex", r.ID)
	}
	if r.CodexHome != codexHome {
		t.Errorf("CodexHome = %q, want %q", r.CodexHome, codexHome)
	}
}

func TestResolve_readsTheKeyFromTheSecretStore(t *testing.T) {
	src, st, _ := newSource(t)
	configure(t, st, "anthropic", "sk-ant-test", false)
	r, err := src.Resolve("anthropic")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if r.APIKey != "sk-ant-test" {
		t.Errorf("APIKey = %q", r.APIKey)
	}
}

func TestResolve_rejectsUnknownIds(t *testing.T) {
	src, _, _ := newSource(t)
	_, err := src.Resolve("claude-code-cli")
	if reasonOf(t, err) != rpc.ReasonProviderNotConfigured {
		t.Errorf("reason = %v", err)
	}
}

func TestOptions_mapsEachProviderToTars(t *testing.T) {
	cases := []struct {
		name string
		in   Resolved
		want llm.ProviderOptions
	}{
		{"anthropic", Resolved{ID: "anthropic", APIKey: " sk ", Model: "claude-sonnet-4-5"},
			llm.ProviderOptions{Provider: "anthropic", APIKey: "sk", Model: "claude-sonnet-4-5"}},
		{"gemini", Resolved{ID: "gemini-native", APIKey: "g"},
			llm.ProviderOptions{Provider: "gemini-native", APIKey: "g"}},
		{"openai-compatible", Resolved{ID: "openai", APIKey: "o", BaseURL: "https://openrouter.ai/api/v1"},
			llm.ProviderOptions{Provider: "openai", APIKey: "o", BaseURL: "https://openrouter.ai/api/v1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Options(tc.in)
			if got.Provider != tc.want.Provider || got.APIKey != tc.want.APIKey ||
				got.BaseURL != tc.want.BaseURL || got.Model != tc.want.Model {
				t.Errorf("Options = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestOptions_codexCarriesItsHome(t *testing.T) {
	got := Options(Resolved{ID: "openai-codex", CodexHome: "/x/codex"})
	if got.Provider != "openai-codex" || got.AuthConfig.CodexHome != "/x/codex" {
		t.Errorf("Options = %+v", got)
	}
	if got.APIKey != "" {
		t.Error("codex must not carry an api key")
	}
}

func TestConfigured_codexMeansTheAuthFileExists(t *testing.T) {
	_, _, codexHome := newSource(t)
	r := Resolved{ID: "openai-codex", CodexHome: codexHome}
	if r.Configured() {
		t.Fatal("configured before any login")
	}
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexHome, "auth.json"), []byte(`{"tokens":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if !r.Configured() {
		t.Fatal("auth.json present but not configured")
	}
}

func TestClient_refusesWithoutACredentialAndNeverBuilds(t *testing.T) {
	src, st, _ := newSource(t)
	configure(t, st, "anthropic", "", true)
	src.WithFactory(func(llm.ProviderOptions) (llm.Client, error) {
		t.Fatal("factory must not run without a credential")
		return nil, nil
	})
	_, _, err := src.Client("anthropic")
	if reasonOf(t, err) != rpc.ReasonProviderNotConfigured {
		t.Errorf("reason = %v", err)
	}
}

func TestClient_refusesWithoutConsentAndNeverBuilds(t *testing.T) {
	src, st, _ := newSource(t)
	configure(t, st, "anthropic", "sk-ant-test", false)
	src.WithFactory(func(llm.ProviderOptions) (llm.Client, error) {
		t.Fatal("factory must not run without consent")
		return nil, nil
	})
	_, _, err := src.Client("anthropic")
	if reasonOf(t, err) != rpc.ReasonProviderConsentRequired {
		t.Errorf("reason = %v", err)
	}
}

func TestClient_buildsWhenConfiguredAndConsented(t *testing.T) {
	src, st, _ := newSource(t)
	configure(t, st, "anthropic", "sk-ant-test", true)
	var seen llm.ProviderOptions
	src.WithFactory(func(opts llm.ProviderOptions) (llm.Client, error) {
		seen = opts
		return fakeClient{ask: func(string) (string, error) { return "OK", nil }}, nil
	})
	c, r, err := src.Client("")
	if err != nil {
		t.Fatalf("Client: %v", err)
	}
	if c == nil || r.ID != "anthropic" {
		t.Errorf("client=%v resolved=%+v", c, r)
	}
	if seen.Provider != "anthropic" || seen.APIKey != "sk-ant-test" {
		t.Errorf("factory saw %+v", seen)
	}
}

func TestClient_classifiesFactoryFailures(t *testing.T) {
	src, st, _ := newSource(t)
	configure(t, st, "anthropic", "sk-ant-test", true)
	src.WithFactory(func(llm.ProviderOptions) (llm.Client, error) {
		return nil, errors.New("api key is required for auth mode api-key")
	})
	_, _, err := src.Client("anthropic")
	if reasonOf(t, err) != rpc.ReasonProviderAuthFailed {
		t.Errorf("reason = %v", err)
	}
}

// An empty CodexHome makes filepath.Join collapse to a bare "auth.json",
// which os.Stat resolves against the process's working directory. A stray
// file there would then read as a completed Codex login.
func TestConfigured_codexWithNoHomeDoesNotStatTheWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(`{"tokens":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	r := Resolved{ID: "openai-codex", CodexHome: ""}
	if r.Configured() {
		t.Fatal("an empty CodexHome picked up ./auth.json as a login")
	}
}

// Resolved is passed across packages and will go further with the agent loop
// (#93). Printing it with any of the usual verbs must not spill the key.
func TestResolved_stringRedactsTheAPIKey(t *testing.T) {
	r := Resolved{ID: "anthropic", Model: "claude-sonnet-4-5", APIKey: "sk-ant-supersecret"}
	for _, format := range []string{"%v", "%s", "%+v", "%q"} {
		got := fmt.Sprintf(format, r)
		if strings.Contains(got, "sk-ant-supersecret") {
			t.Errorf("%s printed the api key: %s", format, got)
		}
		if !strings.Contains(got, "anthropic") {
			t.Errorf("%s dropped the provider id: %s", format, got)
		}
	}
	if got := fmt.Sprintf("%v", Resolved{ID: "openai"}); strings.Contains(got, "APIKey:set") {
		t.Errorf("an unset key reported as set: %s", got)
	}
}
