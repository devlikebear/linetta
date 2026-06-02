# LLM Provider Settings Expansion — Design

Date: 2026-06-02
Status: Approved (ready for implementation plan)

## Goal

Extend Linetta's LLM provider configuration so the user can:

1. Choose among more providers: `claude-code-cli`, `openai-codex`, `anthropic`, `openai`, `gemini-native`.
2. Pick a model **per provider from a live, fetched-from-the-provider list** (a combobox), instead of relying on tars' hardcoded defaults. This fixes the `openai-codex` problem where tars' default model (`gpt-5.3-codex`) is one the user's account does not support.
3. Configure per-provider credentials (API key) where needed.
4. Set an explicit Claude Code CLI binary path for `claude-code-cli` when `claude` is not discoverable on `PATH`.

Out of scope: OpenRouter gateway provider, AWS Bedrock. (Bedrock would require a new tars provider with SigV4 auth; tracked separately.)

## Background (current state)

- Provider config today is a single flat `Config.Provider` string in `engine/internal/settings/settings.go`. Allowed values are whitelisted in `validProviders()`; default is `claude-code-cli`. Constants: `ProviderClaudeCodeCLI = "claude-code-cli"`, `ProviderOpenAICodex = "openai-codex"`.
- Providers are constructed through `ai.DefaultClientFactory(provider, workDir)` → `llm.NewProvider(llm.ProviderOptions{Provider, WorkDir})`. Wired at three call sites: `ai/runner.go`, `companion/runner.go`, `summarizer/summarizer.go`, each reading the live provider via a `ProviderSource.Provider() string` interface.
- No model, API key, or CLI path is passed today; tars uses its internal defaults.
- tars (`github.com/devlikebear/tars`, owned by the user) exposes `llm.NewProvider(ProviderOptions{...})`. `ProviderOptions` already carries `Model`, `APIKey`, `BaseURL`, `WorkDir`, etc.
- tars reads the env var `CLAUDE_CODE_CLI_PATH` first in `FindClaudeCodeCLIPath()`, then falls back to `claude` on `PATH`. `NewClaudeCodeCLIClient(workDir, model)` has no path parameter, so the env var is the only injection point.
- tars has an internal model lister (`internal/llm/model_lister.go`): `FetchModels(ctx, ProviderOptions) ([]string, error)`, supporting `openai`, `kimi`, `anthropic`, `gemini`/`gemini-native`, `openai-codex` (each needs the provider credential). It is **not** exported from `pkg/llm`.
- Frontend provider UI lives in `apps/desktop/src/routes/Settings.tsx` (provider buttons; separate "LLM 도구" web-search section). Settings flow via `settings.get` / `settings.set` RPC; types in `apps/desktop/src/lib/types.ts`, calls in `apps/desktop/src/lib/rpc.ts`.

## tars changes (consumed via go.mod `replace`)

Small, low-risk export so Linetta can reuse the existing model lister.

- In `pkg/llm/exports.go`, re-export the model fetcher:
  - `type ModelFetcher = internal.ModelFetcher`
  - `func NewModelFetcher() ModelFetcher { return internal.NewModelFetcher() }`
  - Re-export any types the public method signature requires (the method is `FetchModels(ctx, ProviderOptions) ([]string, error)`; `ProviderOptions` is already exported).
- Workflow: develop against a **local clone of tars** referenced from `engine/go.mod` via a `replace github.com/devlikebear/tars => <local path>` directive. When the feature is complete, the tars change is tagged as a new release and the `replace` is removed in favor of a version bump. The `replace` directive must NOT be committed to `main` as the final state.

## Linetta data model

`engine/internal/settings/settings.go`:

```go
type ProviderConfig struct {
    Model   string `json:"model"`    // selected model id; empty => provider/tars default
    APIKey  string `json:"api_key"`  // anthropic, openai, gemini-native, codex listing
    CliPath string `json:"cli_path"` // claude-code-cli binary path override (optional)
}

type Config struct {
    Provider  string                    `json:"provider"`  // active provider id
    Providers map[string]ProviderConfig `json:"providers"` // per-provider settings
    // ... existing fields unchanged ...
}
```

- New provider whitelist: `claude-code-cli`, `openai-codex`, `anthropic`, `openai`, `gemini-native`.
- Backward compatibility: settings.json without `providers` loads as an empty map; missing entries resolve to zero-value `ProviderConfig`. The existing `provider` field is preserved; default stays `claude-code-cli`.
- `settings.set` patch handling: the `providers` map is merged per-key (a patch that sets `providers["openai"]` must not wipe `providers["anthropic"]`). API keys follow the same plaintext-in-settings.json storage as the existing `web_search_api_key`.

## Provider construction (factory refactor)

Replace the `(provider, workDir)` factory signature with a resolved-config struct.

```go
// shared (ai package)
type ResolvedProvider struct {
    Provider string
    Model    string
    APIKey   string
    CliPath  string
    WorkDir  string
}
type ClientFactory func(p ResolvedProvider) (llm.Client, error)
```

- `ProviderSource` interface changes from `Provider() string` to `Resolve() ResolvedProvider`, implemented by the settings adapter: it reads the active provider + its `ProviderConfig` and returns the resolved struct.
- `DefaultClientFactory`:
  - For `claude-code-cli`: if `CliPath != ""`, `os.Setenv("CLAUDE_CODE_CLI_PATH", CliPath)` before constructing (only injection point tars offers). Then `llm.NewProvider(ProviderOptions{Provider, Model, WorkDir})`.
  - For others: `llm.NewProvider(ProviderOptions{Provider, Model, APIKey, WorkDir})`.
- Update the three call sites (`ai/runner.go`, `companion/runner.go`, `summarizer/summarizer.go`) to use `Resolve()` + the new factory. Behavior stays "read live settings on each run."

## Model listing RPC

New RPC `providers.list_models`:

- Request: `{ provider: string }` (defaults to the active provider when omitted). The handler builds `ProviderOptions` from the stored `ProviderConfig` (provider + api key) and calls tars `NewModelFetcher().FetchModels(ctx, opts)`. The handler supplies the provider's default base URL internally where the fetcher requires one (e.g. `openai`); there is no user-facing base-URL field.
- Response: `{ models: string[] }`.
- `claude-code-cli` is not supported by `FetchModels`; the handler returns an empty list (UI falls back to free-text) rather than an error.
- Errors (missing key, network, unsupported) are returned as a normal RPC error and surfaced inline in the UI without blocking manual entry.
- Frontend call added to `rpc.ts` (e.g. `providers.listModels`).

## Frontend (Settings.tsx)

- Provider selection extended to 5 buttons: Claude Code CLI, OpenAI Codex, Anthropic, OpenAI, Gemini.
- Fields shown depend on the active provider:
  - **Model** (all providers): a combobox — a dropdown populated from `providers.list_models` plus free-text entry, with a "모델 새로고침" action that (re)fetches. Free-text always allowed so brand-new models work immediately and `claude-code-cli` (no list) still works.
  - **API key** (anthropic, openai, gemini-native): password input. Codex shows an API-key field used for model listing.
  - **CLI path** (claude-code-cli): text input; placeholder explains it overrides `PATH` lookup of `claude`.
- `types.ts`: add `ProviderConfig` and `providers: Record<string, ProviderConfig>` to the `Settings` type; extend `ProviderID` union.
- Persist via `settings.set` with a partial `providers` patch for the active provider.

## Error handling

- Model fetch failures: inline message near the model field; manual entry remains available.
- Missing CLI binary (claude-code-cli): unchanged tars error message surfaces through the existing engine-diagnostic path; the new CLI-path field is the remedy.
- Invalid/empty API key for an API provider: surfaces on first generation attempt as today; model-list fetch will also report it.

## Testing

Go (`engine`):
- Settings: backward-compat load (no `providers`), per-key patch merge, provider whitelist validation, defaults.
- Factory: builds correct `ProviderOptions` per provider; `claude-code-cli` + `CliPath` sets `CLAUDE_CODE_CLI_PATH`; api key passed for api providers.
- `ProviderSource.Resolve()` merges active provider + config.
- `providers.list_models` handler: maps config → `ProviderOptions`, returns empty for `claude-code-cli`, propagates fetch errors. (tars `FetchModels` mocked via an injected fetcher interface so tests don't hit the network.)

Frontend (Vitest):
- Settings UI renders provider-specific fields per selected provider.
- Model combobox: populates from a mocked `providers.list_models`, allows free-text, sends the correct `settings.set` patch.

Gate: `make test` (Go tests, Vitest, Vite build, `cargo check`).
