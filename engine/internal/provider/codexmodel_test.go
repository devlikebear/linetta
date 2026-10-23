package provider

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/tars/pkg/llm"
)

// countingFetcher answers like the Codex backend: listable slugs in its
// priority order, not sorted.
type countingFetcher struct {
	models []string
	err    error
	calls  int
}

func (f *countingFetcher) FetchModels(context.Context, llm.ProviderOptions) ([]string, error) {
	f.calls++
	return f.models, f.err
}

// codexSource is a Source with a Codex login on disk, consent given, and the
// given model stored; it returns the model each Client call builds with.
func codexSource(t *testing.T, stored *string, f llm.ModelFetcher) (*Source, func() string) {
	t.Helper()
	src, st, codexHome := newSource(t)
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexHome, "auth.json"), []byte(`{"tokens":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	pp := settings.ProviderPatch{ConsentedAt: int64Ptr(1700000000000), Model: stored}
	if _, err := st.Set(context.Background(), settings.Patch{
		Provider:  strPtr(settings.ProviderOpenAICodex),
		Providers: map[string]settings.ProviderPatch{settings.ProviderOpenAICodex: pp},
	}); err != nil {
		t.Fatalf("configure codex: %v", err)
	}
	src.WithFetcher(f)
	var seen string
	src.WithFactory(func(opts llm.ProviderOptions) (llm.Client, error) {
		seen = opts.Model
		return fakeClient{}, nil
	})
	build := func() string {
		t.Helper()
		_, r, err := src.Client(context.Background(), "")
		if err != nil {
			t.Fatalf("Client: %v", err)
		}
		if r.Model != seen {
			t.Errorf("Resolved.Model = %q, but the client was built with %q", r.Model, seen)
		}
		return seen
	}
	return src, build
}

// A fresh install has no Codex model stored, and the account's own list —
// not a slug baked into this build — decides what the first turn uses.
func TestClient_codexWithNoModelUsesTheFirstListedModel(t *testing.T) {
	f := &countingFetcher{models: []string{"gpt-5.6-sol", "gpt-5.6-terra"}}
	_, build := codexSource(t, nil, f)
	if got := build(); got != "gpt-5.6-sol" {
		t.Fatalf("model = %q, want the first listed gpt-5.6-sol", got)
	}
}

// 1.x wrote gpt-5.3-codex-spark for everyone. A Plus login is refused it, so
// a turn moves to what the account lists — without touching what is stored.
func TestClient_codexLegacyDefaultMovesToTheListWhenTheAccountLacksIt(t *testing.T) {
	f := &countingFetcher{models: []string{"gpt-5.6-sol", "gpt-5.6-terra"}}
	src, build := codexSource(t, strPtr("gpt-5.3-codex-spark"), f)
	if got := build(); got != "gpt-5.6-sol" {
		t.Fatalf("model = %q, want gpt-5.6-sol", got)
	}
	if stored := src.settings.ProviderConfigFor(settings.ProviderOpenAICodex).Model; stored != "gpt-5.3-codex-spark" {
		t.Errorf("stored model = %q; the setting must be left as the writer's file has it", stored)
	}
}

// A plan that still lists the legacy default keeps it.
func TestClient_codexLegacyDefaultStaysWhenTheAccountListsIt(t *testing.T) {
	f := &countingFetcher{models: []string{"gpt-5.6-sol", "gpt-5.3-codex-spark"}}
	_, build := codexSource(t, strPtr("gpt-5.3-codex-spark"), f)
	if got := build(); got != "gpt-5.3-codex-spark" {
		t.Fatalf("model = %q, want the still-listed gpt-5.3-codex-spark", got)
	}
}

// A model the writer chose is sent as written, listed or not, and costs no
// list call.
func TestClient_codexChosenModelIsSentAsWritten(t *testing.T) {
	f := &countingFetcher{models: []string{"gpt-5.6-sol"}}
	_, build := codexSource(t, strPtr("gpt-5.6-luna"), f)
	if got := build(); got != "gpt-5.6-luna" {
		t.Fatalf("model = %q, want the chosen gpt-5.6-luna", got)
	}
	if f.calls != 0 {
		t.Errorf("listed models %d times for a chosen model", f.calls)
	}
}

// No list, no guess: the model is left empty for tars' own default rather
// than the turn failing before it starts.
func TestClient_codexListFailureLeavesTheModelToTars(t *testing.T) {
	f := &countingFetcher{err: errors.New("dial tcp: connection refused")}
	_, build := codexSource(t, strPtr("gpt-5.3-codex-spark"), f)
	if got := build(); got != "" {
		t.Fatalf("model = %q, want empty", got)
	}
}

// One list serves many turns, and a list the settings pane already fetched
// counts — in the backend's order, not the sorted one it showed.
func TestClient_codexModelListIsCachedAcrossTurns(t *testing.T) {
	f := &countingFetcher{models: []string{"gpt-5.6-terra", "gpt-5.6-luna"}}
	src, build := codexSource(t, nil, f)
	if _, err := src.ListModels(context.Background(), settings.ProviderOpenAICodex); err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	for range 3 {
		if got := build(); got != "gpt-5.6-terra" {
			t.Fatalf("model = %q, want gpt-5.6-terra", got)
		}
	}
	if f.calls != 1 {
		t.Errorf("listed models %d times, want once", f.calls)
	}
}

// The retired default reads as no choice at all.
func TestProviderConfigFor_dropsTheRetiredCodexDefault(t *testing.T) {
	f := &countingFetcher{models: []string{"gpt-5.6-sol"}}
	src, build := codexSource(t, strPtr("gpt-5.3-codex"), f)
	if got := src.settings.ProviderConfigFor(settings.ProviderOpenAICodex).Model; got != "" {
		t.Errorf("ProviderConfigFor model = %q, want empty", got)
	}
	if got := build(); got != "gpt-5.6-sol" {
		t.Fatalf("model = %q, want gpt-5.6-sol", got)
	}
}
