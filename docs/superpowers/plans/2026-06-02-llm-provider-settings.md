# LLM Provider Settings Expansion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the user pick among 5 LLM providers, fetch each provider's live model list to choose a model, set per-provider API keys, and configure the Claude Code CLI path.

**Architecture:** Add a small export to the user-owned `tars` library (consumed via go.mod `replace` during dev) to expose its model-list fetcher. In the Linetta engine, expand the settings model to per-provider config, route model/api-key/cli-path through the provider factory, and add a `providers.list_models` RPC. The desktop Settings UI gains provider buttons and a model combobox.

**Tech Stack:** Go (engine), tars/pkg/llm, JSONRPC over stdio, React + TypeScript + Vite (desktop), Vitest.

---

## File Structure

**tars** (`/Users/changheonshin/workspace/myworks/tars`):
- Modify: `pkg/llm/exports.go` — re-export `ModelFetcher` / `NewModelFetcher`.

**linetta engine**:
- Modify: `engine/go.mod` — `replace` → local tars (dev only).
- Modify: `engine/internal/settings/settings.go` — `ProviderConfig`, `Providers` map, provider whitelist, `Resolve*` accessors, patch merge.
- Modify: `engine/internal/ai/client.go` — `ResolvedProvider`, new `ClientFactory`/`DefaultClientFactory`.
- Modify: `engine/internal/ai/runner.go` — `ProviderSource.Resolve()`, call site.
- Modify: `engine/internal/companion/companion.go`, `companion/runner.go` — same interface/factory.
- Modify: `engine/internal/summarizer/summarizer.go` — call sites.
- Create: `engine/internal/modelcatalog/catalog.go` — thin wrapper over tars `ModelFetcher` (injectable for tests).
- Create: `engine/internal/rpc/handlers/models.go` — `providers.list_models` handler.
- Modify: `engine/cmd/linetta-engine/main.go` — wire factory/handler.
- Tests alongside each.

**linetta desktop**:
- Modify: `apps/desktop/src/lib/types.ts`, `apps/desktop/src/lib/rpc.ts`, `apps/desktop/src/routes/Settings.tsx` (+ test).

---

## Task 1: Export model fetcher from tars

**Files:**
- Modify: `/Users/changheonshin/workspace/myworks/tars/pkg/llm/exports.go`

- [ ] **Step 1: Add the re-exports** to `pkg/llm/exports.go` (after the existing `NewProvider` export):

```go
type ModelFetcher = internal.ModelFetcher

func NewModelFetcher() ModelFetcher { return internal.NewModelFetcher() }
```

- [ ] **Step 2: Build tars to verify it compiles**

Run: `cd /Users/changheonshin/workspace/myworks/tars && go build ./...`
Expected: no output (success).

- [ ] **Step 3: Commit (in tars repo)**

```bash
cd /Users/changheonshin/workspace/myworks/tars
git add pkg/llm/exports.go
git commit -m "feat(llm): export ModelFetcher for embedding consumers"
```

## Task 2: Point linetta engine at local tars via replace

**Files:**
- Modify: `engine/go.mod`

- [ ] **Step 1: Add replace directive** at the end of `engine/go.mod`:

```
replace github.com/devlikebear/tars => /Users/changheonshin/workspace/myworks/tars
```

- [ ] **Step 2: Tidy + verify the export is visible**

Run: `cd engine && go build ./... && go doc github.com/devlikebear/tars/pkg/llm NewModelFetcher`
Expected: build succeeds; `go doc` prints the `NewModelFetcher` signature.

- [ ] **Step 3: Commit**

```bash
git add engine/go.mod engine/go.sum
git commit -m "build(engine): use local tars via replace for model fetcher export"
```

> NOTE: the `replace` line is removed in the final task once tars is tagged and the version bumped.

## Task 3: Settings data model — per-provider config

**Files:**
- Modify: `engine/internal/settings/settings.go`
- Test: `engine/internal/settings/settings_test.go`

- [ ] **Step 1: Write failing tests** in `settings_test.go`:

```go
func TestProvidersBackwardCompatLoadAndResolve(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINETTA_HOME", dir)
	// Legacy file with no `providers` key.
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"provider":"anthropic"}`), 0o600)
	s, err := New()
	if err != nil { t.Fatal(err) }
	rp := s.Resolve()
	if rp.Provider != "anthropic" { t.Fatalf("provider=%q", rp.Provider) }
	if rp.Model != "" || rp.APIKey != "" { t.Fatalf("expected empty model/key, got %+v", rp) }
}

func TestSetProviderConfigMergePerKey(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINETTA_HOME", dir)
	s, _ := New()
	ctx := context.Background()
	pc1 := ProviderConfig{Model: "claude-3", APIKey: "k1"}
	if _, err := s.Set(ctx, Patch{Providers: map[string]ProviderConfig{"anthropic": pc1}}); err != nil { t.Fatal(err) }
	pc2 := ProviderConfig{Model: "gpt-4o", APIKey: "k2"}
	if _, err := s.Set(ctx, Patch{Providers: map[string]ProviderConfig{"openai": pc2}}); err != nil { t.Fatal(err) }
	cfg, _ := s.Get(ctx)
	if cfg.Providers["anthropic"].Model != "claude-3" { t.Fatalf("anthropic clobbered: %+v", cfg.Providers) }
	if cfg.Providers["openai"].Model != "gpt-4o" { t.Fatalf("openai missing: %+v", cfg.Providers) }
}

func TestSetRejectsUnknownProvider(t *testing.T) {
	dir := t.TempDir(); t.Setenv("LINETTA_HOME", dir)
	s, _ := New()
	bad := "bedrock"
	if _, err := s.Set(context.Background(), Patch{Provider: &bad}); err == nil {
		t.Fatal("expected error for unknown provider")
	}
}
```

- [ ] **Step 2: Run to confirm failure**

Run: `cd engine && go test ./internal/settings/ -run TestProviders -v`
Expected: compile error (`ProviderConfig` / `Resolve` / `Patch.Providers` undefined).

- [ ] **Step 3: Implement.** In `settings.go`:

Add provider constants + whitelist:
```go
const (
	ProviderClaudeCodeCLI = "claude-code-cli"
	ProviderOpenAICodex   = "openai-codex"
	ProviderAnthropic     = "anthropic"
	ProviderOpenAI        = "openai"
	ProviderGeminiNative  = "gemini-native"
)

func validProviders() []string {
	return []string{ProviderClaudeCodeCLI, ProviderOpenAICodex, ProviderAnthropic, ProviderOpenAI, ProviderGeminiNative}
}
```

Add the per-provider struct + Config/Patch fields:
```go
type ProviderConfig struct {
	Model   string `json:"model,omitempty"`
	APIKey  string `json:"api_key,omitempty"`
	CliPath string `json:"cli_path,omitempty"`
}
```
In `Config` add: `Providers map[string]ProviderConfig `json:"providers,omitempty"``
In `Patch` add: `Providers map[string]ProviderConfig `json:"providers,omitempty"``

In `load()`, after the existing field copies, add:
```go
if len(disk.Providers) > 0 {
	s.cfg.Providers = disk.Providers
}
```

In `defaults()` leave `Providers` nil (lazily created on Set).

In `Set()`, before building `persistable`, merge per-key:
```go
if p.Providers != nil {
	if next.Providers == nil {
		next.Providers = map[string]ProviderConfig{}
	}
	for k, v := range p.Providers {
		if !slices.Contains(validProviders(), k) {
			return Config{}, fmt.Errorf("settings: unknown provider %q", k)
		}
		next.Providers[k] = v
	}
}
```
Add `Providers: next.Providers,` to the `persistable` literal.

Add the resolver accessor (returns active provider + its config; ResolvedProvider defined in Task 4 lives in ai, so settings returns its own light struct to avoid import cycle):
```go
// ProviderSettings is the resolved active-provider view consumed by ai.
type ProviderSettings struct {
	Provider string
	Model    string
	APIKey   string
	CliPath  string
}

func (s *Store) Resolve() ProviderSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	pc := s.cfg.Providers[s.cfg.Provider]
	return ProviderSettings{
		Provider: s.cfg.Provider,
		Model:    pc.Model,
		APIKey:   pc.APIKey,
		CliPath:  pc.CliPath,
	}
}

// ProviderConfigFor returns the stored config for a provider id (zero value if unset).
func (s *Store) ProviderConfigFor(id string) ProviderConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Providers[id]
}
```
Keep the existing `Provider() string` method (still used by tools.go).

- [ ] **Step 4: Run tests**

Run: `cd engine && go test ./internal/settings/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add engine/internal/settings/
git commit -m "feat(settings): per-provider config (model/api key/cli path)"
```

## Task 4: Provider factory takes resolved config

**Files:**
- Modify: `engine/internal/ai/client.go`
- Modify: `engine/internal/ai/runner.go`
- Test: `engine/internal/ai/client_test.go` (create)

Design: `ai.ResolvedProvider` is the factory input. `ai.ProviderSource` exposes `Resolve() ResolvedProvider`. settings.Store implements it by adapting `settings.ProviderSettings` → handled in main.go via a tiny adapter (avoids settings importing ai).

- [ ] **Step 1: Write failing test** `engine/internal/ai/client_test.go`:

```go
package ai

import (
	"os"
	"testing"
)

func TestDefaultClientFactorySetsClaudeCliPath(t *testing.T) {
	os.Unsetenv("CLAUDE_CODE_CLI_PATH")
	// claude-code-cli with a bogus path: NewProvider will fail to find it, but
	// the env var must be set first (that is the behavior under test).
	_, _ = DefaultClientFactory(ResolvedProvider{Provider: "claude-code-cli", CliPath: "/tmp/does-not-exist-claude"})
	if got := os.Getenv("CLAUDE_CODE_CLI_PATH"); got != "/tmp/does-not-exist-claude" {
		t.Fatalf("CLAUDE_CODE_CLI_PATH=%q", got)
	}
}
```

- [ ] **Step 2: Run to confirm failure**

Run: `cd engine && go test ./internal/ai/ -run TestDefaultClientFactory -v`
Expected: compile error (`ResolvedProvider` undefined).

- [ ] **Step 3: Implement** `client.go`:

```go
package ai

import (
	"os"
	"strings"

	"github.com/devlikebear/tars/pkg/llm"
)

// ResolvedProvider is the per-call provider configuration handed to the factory.
type ResolvedProvider struct {
	Provider string
	Model    string
	APIKey   string
	CliPath  string
	WorkDir  string
}

// ClientFactory creates an llm.Client from a resolved provider config.
type ClientFactory func(p ResolvedProvider) (llm.Client, error)

// DefaultClientFactory delegates to tars. For claude-code-cli it injects the
// configured CLI path via the env var tars reads (it has no path parameter).
func DefaultClientFactory(p ResolvedProvider) (llm.Client, error) {
	if p.Provider == "claude-code-cli" && strings.TrimSpace(p.CliPath) != "" {
		_ = os.Setenv("CLAUDE_CODE_CLI_PATH", strings.TrimSpace(p.CliPath))
	}
	return llm.NewProvider(llm.ProviderOptions{
		Provider: p.Provider,
		Model:    p.Model,
		APIKey:   p.APIKey,
		WorkDir:  p.WorkDir,
	})
}
```

In `runner.go`: change the interface and call site:
```go
type ProviderSource interface {
	Resolve() ResolvedProvider
}
```
Replace `provider := r.src.Provider()` with:
```go
rp := r.src.Resolve()
provider := rp.Provider
```
and `client, err := r.factory(provider, r.workDir)` with:
```go
rp.WorkDir = r.workDir
client, err := r.factory(rp)
```

- [ ] **Step 4: Run test**

Run: `cd engine && go test ./internal/ai/ -run TestDefaultClientFactory -v`
Expected: PASS.

- [ ] **Step 5: Commit** (will be green after Task 5/6 fix callers; commit together in Task 6).

## Task 5: Update companion + summarizer to new factory

**Files:**
- Modify: `engine/internal/companion/companion.go`, `engine/internal/companion/runner.go`
- Modify: `engine/internal/summarizer/summarizer.go`
- Modify tests: `companion/companion_test.go`, `companion/summarizer? ` and `summarizer/summarizer_test.go`

- [ ] **Step 1: companion.go** — replace the local factory/source types with ai's:
```go
// ClientFactory and ProviderSource are shared with the ai package.
type ClientFactory = ai.ClientFactory
type ProviderSource = ai.ProviderSource
```
Add `"github.com/devlikebear/linetta/engine/internal/ai"` import. Remove the old `type ClientFactory func(provider, workDir string)...` and `type ProviderSource interface{ Provider() string }`.

- [ ] **Step 2: companion/runner.go:98** — replace:
```go
rp := r.svc.src.Resolve()
rp.WorkDir = r.svc.workDir
client, err := r.svc.factory(rp)
```

- [ ] **Step 3: summarizer.go:146-147 and 221-222** — replace each pair:
```go
rp := s.src.Resolve()
rp.WorkDir = ""
client, err := s.factory(rp)
provider := rp.Provider
```
(keep the existing `provider` usage in the error log line).

- [ ] **Step 4: Fix test doubles.** In `companion/companion_test.go`, `companion/tools_test.go`, `summarizer/summarizer_test.go` replace `func (p fixedProvider) Provider() string { return string(p) }` with:
```go
func (p fixedProvider) Resolve() ai.ResolvedProvider { return ai.ResolvedProvider{Provider: string(p)} }
```
Keep a separate `Provider() string` on the tools_test source if `tools.go` still needs it (it uses a `ConfigSource` with `Provider()` + `WebSearchProvider()` — leave that interface alone; only the ai/companion ProviderSource changes).

- [ ] **Step 5: Run** `cd engine && go test ./internal/companion/ ./internal/summarizer/ ./internal/ai/ -v`
Expected: PASS.

## Task 6: Wire main.go + commit factory refactor

**Files:**
- Modify: `engine/cmd/linetta-engine/main.go`

- [ ] **Step 1:** settings.Store needs to satisfy `ai.ProviderSource` (`Resolve() ai.ResolvedProvider`). Add a tiny adapter in main.go (settings must not import ai):
```go
type providerSource struct{ s *settings.Store }
func (p providerSource) Resolve() ai.ResolvedProvider {
	r := p.s.Resolve()
	return ai.ResolvedProvider{Provider: r.Provider, Model: r.Model, APIKey: r.APIKey, CliPath: r.CliPath}
}
```
Pass `providerSource{set}` where `set` (the *settings.Store) was passed as the ProviderSource to `ai.NewRunner`, `companion.NewService`, and `summarizer.New`.

- [ ] **Step 2: Build + full engine test**

Run: `cd engine && go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add engine/internal/ai/ engine/internal/companion/ engine/internal/summarizer/ engine/cmd/linetta-engine/main.go
git commit -m "refactor(engine): route model/api-key/cli-path through provider factory"
```

## Task 7: providers.list_models RPC

**Files:**
- Create: `engine/internal/modelcatalog/catalog.go`
- Create: `engine/internal/modelcatalog/catalog_test.go`
- Create: `engine/internal/rpc/handlers/models.go`
- Modify: `engine/cmd/linetta-engine/main.go` (register handler)

- [ ] **Step 1: Test** `modelcatalog/catalog_test.go`:

```go
package modelcatalog

import (
	"context"
	"testing"

	"github.com/devlikebear/tars/pkg/llm"
)

type fakeFetcher struct{ models []string; err error }
func (f fakeFetcher) FetchModels(ctx context.Context, opts llm.ProviderOptions) ([]string, error) {
	return f.models, f.err
}

func TestListClaudeCliReturnsEmpty(t *testing.T) {
	c := New(fakeFetcher{models: []string{"x"}})
	got, err := c.List(context.Background(), "claude-code-cli", "")
	if err != nil { t.Fatal(err) }
	if len(got) != 0 { t.Fatalf("expected empty, got %v", got) }
}

func TestListPassesProviderAndKey(t *testing.T) {
	c := New(fakeFetcher{models: []string{"a", "b"}})
	got, err := c.List(context.Background(), "anthropic", "key")
	if err != nil { t.Fatal(err) }
	if len(got) != 2 { t.Fatalf("got %v", got) }
}
```

- [ ] **Step 2: Run → fail**

Run: `cd engine && go test ./internal/modelcatalog/ -v`
Expected: compile error (`New`/`List` undefined).

- [ ] **Step 3: Implement** `catalog.go`:

```go
// Package modelcatalog lists available models per provider via tars.
package modelcatalog

import (
	"context"
	"strings"

	"github.com/devlikebear/tars/pkg/llm"
)

// Catalog wraps a tars model fetcher (injectable for tests).
type Catalog struct{ fetcher llm.ModelFetcher }

func New(f llm.ModelFetcher) *Catalog { return &Catalog{fetcher: f} }

func Default() *Catalog { return &Catalog{fetcher: llm.NewModelFetcher()} }

// List returns model ids for a provider. claude-code-cli has no list API and
// returns an empty slice (the UI falls back to free-text entry).
func (c *Catalog) List(ctx context.Context, provider, apiKey string) ([]string, error) {
	if strings.TrimSpace(provider) == "claude-code-cli" {
		return []string{}, nil
	}
	return c.fetcher.FetchModels(ctx, llm.ProviderOptions{
		Provider: provider,
		APIKey:   apiKey,
	})
}
```

- [ ] **Step 4: Run → pass**

Run: `cd engine && go test ./internal/modelcatalog/ -v`
Expected: PASS.

- [ ] **Step 5: Handler** `rpc/handlers/models.go` (match the existing handler style in `rpc/handlers/settings.go`):

```go
package handlers

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/modelcatalog"
	"github.com/devlikebear/linetta/engine/internal/settings"
)

type ModelsHandler struct {
	store   *settings.Store
	catalog *modelcatalog.Catalog
}

func NewModelsHandler(store *settings.Store, catalog *modelcatalog.Catalog) *ModelsHandler {
	return &ModelsHandler{store: store, catalog: catalog}
}

type listModelsParams struct {
	Provider string `json:"provider"`
}

type listModelsResult struct {
	Models []string `json:"models"`
}

func (h *ModelsHandler) ListModels(ctx context.Context, raw json.RawMessage) (any, error) {
	var p listModelsParams
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
	}
	provider := p.Provider
	if provider == "" {
		provider = h.store.Provider()
	}
	key := h.store.ProviderConfigFor(provider).APIKey
	models, err := h.catalog.List(ctx, provider, key)
	if err != nil {
		return nil, err
	}
	return listModelsResult{Models: models}, nil
}
```
(Adjust the handler signature/registration to match the codebase's actual RPC dispatch convention — mirror `settings.go`.)

- [ ] **Step 6: Register** in `main.go`: construct `modelcatalog.Default()` and register `providers.list_models` → `ModelsHandler.ListModels` next to the `settings.*` registrations.

- [ ] **Step 7: Build + test + commit**

Run: `cd engine && go build ./... && go test ./...`
```bash
git add engine/internal/modelcatalog/ engine/internal/rpc/handlers/models.go engine/cmd/linetta-engine/main.go
git commit -m "feat(engine): providers.list_models RPC via tars model fetcher"
```

## Task 8: Frontend — types + rpc client

**Files:**
- Modify: `apps/desktop/src/lib/types.ts`
- Modify: `apps/desktop/src/lib/rpc.ts`

- [ ] **Step 1: types.ts** — extend the provider union and Settings:
```ts
export type ProviderID =
  | "claude-code-cli"
  | "openai-codex"
  | "anthropic"
  | "openai"
  | "gemini-native";

export interface ProviderConfig {
  model?: string;
  api_key?: string;
  cli_path?: string;
}
```
Add to `Settings`: `providers?: Record<string, ProviderConfig>;`
Add to `SettingsPatch`: `providers?: Record<string, ProviderConfig>;`

- [ ] **Step 2: rpc.ts** — add:
```ts
export const providers = {
  listModels: (provider: ProviderID) =>
    rpcCall<{ models: string[] }>("providers.list_models", { provider }),
};
```
(Import `ProviderID` if needed.)

- [ ] **Step 3: Build check**

Run: `cd apps/desktop && pnpm exec tsc -b`
Expected: no type errors.

- [ ] **Step 4: Commit**

```bash
git add apps/desktop/src/lib/types.ts apps/desktop/src/lib/rpc.ts
git commit -m "feat(desktop): provider config types + list_models client"
```

## Task 9: Frontend — Settings UI

**Files:**
- Modify: `apps/desktop/src/routes/Settings.tsx`
- Test: `apps/desktop/src/routes/Settings.test.tsx` (create or extend existing Settings test)

- [ ] **Step 1: Read the current `Settings.tsx`** provider section (around lines 98-124) to match its component patterns and styling.

- [ ] **Step 2: Write a failing Vitest** asserting: selecting "Anthropic" shows an API-key field and a model combobox; entering a model and saving sends a `settings.set` patch with `providers["anthropic"].model`. Mock `rpc.settings` and `rpc.providers.listModels`.

- [ ] **Step 3: Implement** — extend provider buttons to 5 (claude-code-cli, openai-codex, anthropic, openai, gemini-native). For the active provider render:
  - Model combobox: an `<input list=...>` (datalist) populated from `providers.listModels(active)` with a "새로고침" button; free-text allowed. claude-code-cli shows free-text only (list returns empty).
  - API key password input for anthropic/openai/gemini-native/openai-codex.
  - CLI path text input for claude-code-cli (placeholder: `PATH의 claude를 못 찾을 때 실행 파일 경로`).
  Persist via `rpc.settings.set({ providers: { [active]: { model, api_key, cli_path } } })`.

- [ ] **Step 4: Run** `cd apps/desktop && pnpm test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/desktop/src/routes/Settings.tsx apps/desktop/src/routes/Settings.test.tsx
git commit -m "feat(desktop): provider settings UI with model combobox + cli path"
```

## Task 10: Full gate + finalize tars dependency

**Files:**
- Modify: `engine/go.mod` (remove replace), bump tars version.

- [ ] **Step 1: Run the full gate** with replace still active:

Run: `make test`
Expected: PASS (Go tests, Vitest, Vite build, cargo check).

- [ ] **Step 2: Tag tars release.** In `/Users/changheonshin/workspace/myworks/tars`: bump `VERSION.txt`, commit, `git tag v0.33.1`, push branch + tag (per user confirmation at this step).

- [ ] **Step 3: Switch linetta off replace:**
```bash
cd engine
# remove the replace line, then:
go get github.com/devlikebear/tars@v0.33.1
go mod tidy
go build ./...
```

- [ ] **Step 4: Re-run gate**

Run: `make test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add engine/go.mod engine/go.sum
git commit -m "build(engine): consume tars v0.33.1 (model fetcher export)"
```

> If tagging/pushing tars is deferred, keep the `replace` directive on the feature branch and note it in the PR; do not merge the replace to main.

---

## Self-Review Notes

- Spec coverage: providers (T3), per-provider model/key/cli-path (T3/T4), claude cli path env (T4), model listing RPC (T7), tars export (T1), frontend combobox + fields (T8/T9), backward compat (T3), tests throughout. ✓
- Risk: the factory signature change touches ai/companion/summarizer + their test doubles (T4-T6) — sequenced so the engine only needs to compile green at T6.
- Open detail resolved at implementation time: exact RPC dispatch/registration convention (mirror `rpc/handlers/settings.go` + its registration in `main.go`); exact Settings.tsx component primitives.
