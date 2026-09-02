# BYOK 프로바이더 설정 복원 (#91) — 구현 계획

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> 커밋은 기능 단위로, `feat/fix/chore/test` 메시지로. 동작이 바뀌는 작업은 실패하는 테스트를 먼저 쓴다. 각 작업 종료 시 해당 패키지 테스트를 통과시키고, 마지막 작업에서 `make test`를 통과시킨다.
>
> **설계 문서:** `docs/superpowers/specs/2026-09-02-builtin-agent-byok-design.md` (5.1·5.2·5.4·6절, 4.2절의 의존성 게이트). **이슈:** #91 (에픽 #90).

**Goal:** 1.0이 남겨둔 프로바이더 설정을 patch 표면에 되돌리고, 프로바이더 4개 화이트리스트와 프로바이더별 동의를 추가하고, 설정을 tars `llm.Client`로 해석하는 `internal/provider`와 `providers.*` RPC 3개를 만들고, 의존성 게이트를 없애지 않고 좁힌다.

**Architecture:** `settings`가 프로바이더 설정과 키(SecretStore)와 동의를 들고 있고, 새 패키지 `internal/provider`만 그것을 tars `pkg/llm`으로 번역한다. RPC 핸들러는 `pkg/llm`을 링크하지 않도록 인터페이스만 보고, 실패는 `rpc.ReasonError`로 이유 코드만 전달한다. 에이전트 루프(#93)는 이 패키지의 `Source.Client`를 그대로 쓴다.

**Tech Stack:** Go 1.26 엔진, tars v0.34.3 `pkg/llm`, Tauri 2 / Rust 셸(`RENDERER_ENGINE_METHODS`), React 18 + TypeScript + Vitest.

## Global Constraints

- 엔진 모듈은 `github.com/devlikebear/linetta/engine`. 빌드 태그 `mas`와 `mobile`은 독립이며 둘 다 계속 빌드되어야 한다: `make test`, `make test-mobile-engine`, `cd engine && go build -tags mas ./...`, `cd engine && GOOS=windows GOARCH=amd64 go build ./...`.
- 프로바이더 화이트리스트는 정확히 넷: `openai-codex`, `anthropic`, `gemini-native`, `openai`. `claude-code-cli`·`openrouter`는 patch로 쓸 수 없지만 디스크에 남은 값은 **읽을 때 버리지 않고** 다음 저장에서도 살아남는다.
- API 키는 `settings.json`에 절대 쓰이지 않는다. `SecretStore`의 `provider.<id>.api_key`에만 산다.
- 동의는 프로바이더별(`providers[<id>].consented_at`). 동의 없는 프로바이더로는 `providers.test`조차 아무것도 보내지 않는다.
- `tars/pkg/llm`은 `internal/provider`(와 후속 #93의 `internal/agent`)에서만 import한다. `storycontext`, `storyops`, `mcphost`, `rpc/handlers`는 `go list -deps`로 무유입을 검증한다. `pkg/agentloop`·`pkg/session`은 엔진 전역 금지.
- 프로세스 환경 `TARS_OPENAI_CODEX_REFRESH_TOKEN_STORAGE=file`을 고정한다(샌드박스에서 `security` CLI 호출 불가).
- UI가 부르는 새 엔진 메서드는 `apps/desktop/src-tauri/src/lib.rs`의 `RENDERER_ENGINE_METHODS`에 **정렬 순서를 지켜** 추가하고(이진 탐색), 같은 커밋에서 `apps/desktop/src/lib/rpc.ts` 래퍼를 붙인다. `rpcAllowlist.test.ts`는 양쪽이 정확히 일치할 것을 요구한다.
- 이유 코드(신규): `provider_not_configured`, `provider_consent_required`, `provider_auth_failed`, `provider_rate_limited`, `provider_unreachable`. 프로바이더 원문 JSON은 렌더러로 내보내지 않는다.
- Codex의 `auth.json` 위치는 `$LINETTA_HOME/codex/auth.json`. 로그인 자체는 #92. 이 계획에서 Codex의 "설정됨"은 그 파일의 존재 여부다.

---

## 파일 구조

| 파일 | 책임 |
| --- | --- |
| `engine/internal/settings/settings.go` (수정) | 화이트리스트 4개, `ProviderConfig`에서 `CliPath`/`ClearAPIKey` 제거·`ConsentedAt` 추가, `ProviderPatch`, `Set`의 프로바이더 patch 병합, OpenRouter 상수 제거 |
| `engine/internal/settings/providers.go` (신규) | `ValidProviders`, `ActiveProvider`, `HasProviderConsent` — 프로바이더 관련 접근자를 한 파일에 |
| `engine/internal/settings/providers_test.go` (신규) | 위 동작 테스트 |
| `engine/internal/rpc/reason.go` (수정) | 이유 코드 5개, `ReasonError`, `MethodErrorFrom` |
| `engine/internal/rpc/reason_test.go` (신규) | `MethodErrorFrom` 테스트 |
| `engine/internal/provider/provider.go` (신규) | `Resolved`, `Options`, `Source`(`Resolve`/`Client`), env 고정 |
| `engine/internal/provider/errors.go` (신규) | `Classify` |
| `engine/internal/provider/catalog.go` (신규) | `Status`, `List`, `ListModels`, `Test` |
| `engine/internal/provider/*_test.go` (신규) | 가짜 팩토리·페처로 네트워크 없이 검증 |
| `engine/internal/rpc/handlers/providers.go` (신규) | `ProviderService` 인터페이스, `providers.list/list_models/test` 핸들러 |
| `engine/internal/rpc/handlers/providers_test.go` (신규) | 가짜 서비스로 핸들러 검증 |
| `engine/internal/rpc/handlers/settings_test.go` (수정) | "프로바이더 필드 무시" 테스트를 "검증·적용"으로 뒤집음 |
| `engine/internal/engineapp/providers.go` (신규) | `provider.Source` → `handlers.ProviderService` 어댑터 |
| `engine/internal/engineapp/engineapp.go` (수정) | `provider.NewSource` 생성, 핸들러 3개 등록 |
| `apps/desktop/src-tauri/src/lib.rs` (수정) | 허용 목록에 `providers.*` 3개 |
| `apps/desktop/src/lib/types.ts`, `rpc.ts` (수정) | `ProviderID` 4개, `ProviderConfig`/`ProviderPatch`/`ProviderStatus`, `providers.*` 래퍼 |
| `apps/desktop/src/routes/Settings.test.tsx`, `Library.test.tsx` (수정) | 픽스처의 옛 프로바이더 id 교체 |
| `scripts/validate-story-core-deps.sh` (수정) | 게이트 좁히기 |

---

### Task 1: settings — 프로바이더 필드 복원, 화이트리스트, 프로바이더별 동의

**Files:**
- Modify: `engine/internal/settings/settings.go`
- Create: `engine/internal/settings/providers.go`
- Create: `engine/internal/settings/providers_test.go`
- Modify: `engine/internal/rpc/handlers/settings_test.go:58-76`

**Interfaces:**
- Consumes: 기존 `Store`, `SecretStore`, `providerAPIKeySecretName(id)`, `ProviderConfigFor(id)`.
- Produces (후속 작업이 쓰는 이름):
  - `settings.ValidProviders() []string`
  - `settings.ProviderOpenAICodex / ProviderAnthropic / ProviderGeminiNative / ProviderOpenAI` 상수
  - `type ProviderPatch struct { Model, BaseURL, APIKey *string; ConsentedAt *int64 }`, `Patch.Provider *string`, `Patch.Providers map[string]ProviderPatch`
  - `ProviderConfig.ConsentedAt int64` (`json:"consented_at"`)
  - `(*Store).ActiveProvider() string`, `(*Store).HasProviderConsent(id string) bool`
  - 기존 `(*Store).ProviderConfigFor(id) ProviderConfig` (APIKey가 시크릿에서 채워진 채 반환)

- [ ] **Step 1: 실패하는 테스트 작성** — `engine/internal/settings/providers_test.go`

```go
package settings

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func int64Ptr(v int64) *int64 { return &v }

func TestValidProviders_isExactlyTheFour(t *testing.T) {
	want := []string{"openai-codex", "anthropic", "gemini-native", "openai"}
	if got := ValidProviders(); !slices.Equal(got, want) {
		t.Fatalf("ValidProviders = %v, want %v", got, want)
	}
}

func TestSet_providerPatch_roundTrips(t *testing.T) {
	s := newStoreOnTemp(t)
	ctx := context.Background()
	if _, err := s.Set(ctx, Patch{
		Provider: strPtr("anthropic"),
		Providers: map[string]ProviderPatch{
			"anthropic": {Model: strPtr("claude-sonnet-4-5"), ConsentedAt: int64Ptr(1700000000000)},
			"openai":    {BaseURL: strPtr("https://openrouter.ai/api/v1")},
		},
	}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	// Reload from disk with a fresh store: what survived is what was persisted.
	s2, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	got, _ := s2.Get(ctx)
	if got.Provider != "anthropic" {
		t.Errorf("provider = %q, want anthropic", got.Provider)
	}
	if got.Providers["anthropic"].Model != "claude-sonnet-4-5" {
		t.Errorf("anthropic model = %q", got.Providers["anthropic"].Model)
	}
	if got.Providers["anthropic"].ConsentedAt != 1700000000000 {
		t.Errorf("anthropic consented_at = %d", got.Providers["anthropic"].ConsentedAt)
	}
	if got.Providers["openai"].BaseURL != "https://openrouter.ai/api/v1" {
		t.Errorf("openai base_url = %q", got.Providers["openai"].BaseURL)
	}
}

func TestSet_providerPatch_mergesPerKeyAndPerField(t *testing.T) {
	s := newStoreOnTemp(t)
	ctx := context.Background()
	if _, err := s.Set(ctx, Patch{Providers: map[string]ProviderPatch{
		"anthropic": {Model: strPtr("claude-sonnet-4-5"), ConsentedAt: int64Ptr(1)},
	}}); err != nil {
		t.Fatalf("Set 1: %v", err)
	}
	// A patch for another provider, and a partial patch for the same one,
	// must not wipe what was there.
	if _, err := s.Set(ctx, Patch{Providers: map[string]ProviderPatch{
		"openai":    {BaseURL: strPtr("http://localhost:11434/v1")},
		"anthropic": {Model: strPtr("claude-opus-4-1")},
	}}); err != nil {
		t.Fatalf("Set 2: %v", err)
	}
	got, _ := s.Get(ctx)
	if got.Providers["anthropic"].Model != "claude-opus-4-1" {
		t.Errorf("anthropic model = %q, want the second patch", got.Providers["anthropic"].Model)
	}
	if got.Providers["anthropic"].ConsentedAt != 1 {
		t.Errorf("anthropic consent was lost by a partial patch: %d", got.Providers["anthropic"].ConsentedAt)
	}
	if got.Providers["openai"].BaseURL != "http://localhost:11434/v1" {
		t.Errorf("openai base_url = %q", got.Providers["openai"].BaseURL)
	}
}

func TestSet_providerAPIKey_livesOnlyInTheSecretStore(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINETTA_HOME", dir)
	secrets := NewMemorySecretStore()
	s, err := NewWithSecretStore(secrets)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	if _, err := s.Set(ctx, Patch{Providers: map[string]ProviderPatch{
		"anthropic": {APIKey: strPtr("sk-ant-test")},
	}}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if v, ok, _ := secrets.Get("provider.anthropic.api_key"); !ok || v != "sk-ant-test" {
		t.Fatalf("secret store has (%q, %v), want the key", v, ok)
	}
	body, _ := os.ReadFile(filepath.Join(dir, "settings.json"))
	if strings.Contains(string(body), "sk-ant-test") {
		t.Fatalf("api key written to settings.json: %s", body)
	}
	got, _ := s.Get(ctx)
	if !got.Providers["anthropic"].APIKeySet || got.Providers["anthropic"].APIKey != "" {
		t.Errorf("settings.get must show presence only: %+v", got.Providers["anthropic"])
	}
	if cfg := s.ProviderConfigFor("anthropic"); cfg.APIKey != "sk-ant-test" {
		t.Errorf("ProviderConfigFor did not read the secret: %+v", cfg)
	}
	// An empty key deletes.
	if _, err := s.Set(ctx, Patch{Providers: map[string]ProviderPatch{
		"anthropic": {APIKey: strPtr("")},
	}}); err != nil {
		t.Fatalf("Set clear: %v", err)
	}
	if _, ok, _ := secrets.Get("provider.anthropic.api_key"); ok {
		t.Error("empty api_key patch did not delete the secret")
	}
}

func TestSet_rejectsProvidersOutsideTheWhitelist(t *testing.T) {
	s := newStoreOnTemp(t)
	ctx := context.Background()
	if _, err := s.Set(ctx, Patch{Provider: strPtr("claude-code-cli")}); err == nil {
		t.Error("provider=claude-code-cli was accepted")
	}
	if _, err := s.Set(ctx, Patch{Providers: map[string]ProviderPatch{
		"openrouter": {Model: strPtr("x")},
	}}); err == nil {
		t.Error("providers[openrouter] was accepted")
	}
}

func TestActiveProvider_fallsBackToCodexButLeavesDiskAlone(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LINETTA_HOME", dir)
	path := filepath.Join(dir, "settings.json")
	seed := `{"language":"ko","provider":"claude-code-cli",` +
		`"providers":{"claude-code-cli":{"model":"opus"}}}`
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := NewWithSecretStore(NewMemorySecretStore())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := s.ActiveProvider(); got != "openai-codex" {
		t.Errorf("ActiveProvider = %q, want openai-codex fallback", got)
	}
	if _, err := s.Set(context.Background(), Patch{Theme: strPtr("dark")}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	raw, _ := os.ReadFile(path)
	var onDisk Config
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatal(err)
	}
	if onDisk.Provider != "claude-code-cli" || onDisk.Providers["claude-code-cli"].Model != "opus" {
		t.Errorf("retired provider was rewritten on an unrelated save: %s", raw)
	}
}

func TestHasProviderConsent_isPerProvider(t *testing.T) {
	s := newStoreOnTemp(t)
	ctx := context.Background()
	if _, err := s.Set(ctx, Patch{Providers: map[string]ProviderPatch{
		"anthropic": {ConsentedAt: int64Ptr(1700000000000)},
	}}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if !s.HasProviderConsent("anthropic") {
		t.Error("anthropic consent not recorded")
	}
	if s.HasProviderConsent("openai") {
		t.Error("consent leaked across providers")
	}
	if _, err := s.Set(ctx, Patch{Providers: map[string]ProviderPatch{
		"anthropic": {ConsentedAt: int64Ptr(0)},
	}}); err != nil {
		t.Fatalf("Set revoke: %v", err)
	}
	if s.HasProviderConsent("anthropic") {
		t.Error("consented_at=0 did not revoke")
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `cd engine && go test ./internal/settings/ -run 'TestValidProviders|TestSet_provider|TestSet_rejectsProviders|TestActiveProvider|TestHasProviderConsent' 2>&1 | head -20`
Expected: 컴파일 실패 — `undefined: ValidProviders`, `undefined: ProviderPatch`, `s.ActiveProvider undefined` 등.

- [ ] **Step 3: `settings.go` 수정**

(a) 상단 상수 블록을 교체한다. `ProviderClaudeCodeCLI`, `ProviderOpenRouter`, `OpenRouterBaseURL`, `DefaultOpenRouterModel`, `OpenRouterFastModel`, `OpenRouterAutoModel`, `OpenRouterDefaultMaxTokens`를 지우고 다음으로 바꾼다. `AIDataSharingConsentVersion`과 `DefaultOpenAICodexModel`은 그대로 둔다.

```go
// Provider ids the built-in agent can use (#90). Each maps 1:1 to a tars
// pkg/llm provider name. Codex authenticates with an OAuth login the app
// performs itself (#92); the other three take an API key. "openai" is the
// OpenAI-compatible family: base_url points it at OpenRouter, Ollama, LM Studio.
const (
	ProviderOpenAICodex  = "openai-codex"
	ProviderAnthropic    = "anthropic"
	ProviderGeminiNative = "gemini-native"
	ProviderOpenAI       = "openai"
)
```

(b) import 블록에 `"strings"`를 추가한다.

(c) `ProviderConfig`를 교체한다.

```go
// ProviderConfig is one provider's stored entry (#90/#91).
//
// APIKey is legacy on-disk input only: a pre-1.0 settings.json may still carry
// one, load() moves it into the SecretStore and it is never written back.
// Patches carry keys through ProviderPatch instead.
type ProviderConfig struct {
	Model       string `json:"model,omitempty"`
	APIKey      string `json:"api_key,omitempty"`
	APIKeySet   bool   `json:"api_key_set,omitempty"` // settings.get presence flag, never persisted
	BaseURL     string `json:"base_url,omitempty"`    // openai only
	ConsentedAt int64  `json:"consented_at,omitempty"` // per-provider data-sharing consent; 0 = none
}

// ProviderPatch is one provider's entry in Patch.Providers. Nil leaves the
// field alone. An empty APIKey deletes the stored key; ConsentedAt 0 revokes.
type ProviderPatch struct {
	Model       *string `json:"model,omitempty"`
	BaseURL     *string `json:"base_url,omitempty"`
	APIKey      *string `json:"api_key,omitempty"`
	ConsentedAt *int64  `json:"consented_at,omitempty"`
}
```

(d) `Config`의 `Provider`/`Providers` 위에 있는 Deprecated 주석 두 문단을 지우고 한 줄로 바꾼다.

```go
	Language string `json:"language"`
	// The built-in agent's provider (#90). Provider is the active id; Providers
	// holds each id's model, base URL and consent. Keys live in the SecretStore.
	Provider  string                    `json:"provider"`
	Providers map[string]ProviderConfig `json:"providers,omitempty"`
```

(e) `Patch`에 두 필드를 추가한다(`Language` 바로 아래).

```go
	Provider                  *string                  `json:"provider,omitempty"`
	Providers                 map[string]ProviderPatch `json:"providers,omitempty"`
```

(f) `Set`에서 `if p.Language != nil { … }` 블록 바로 뒤에 추가한다.

```go
	if p.Provider != nil {
		if !slices.Contains(ValidProviders(), *p.Provider) {
			return Config{}, fmt.Errorf("settings: unknown provider %q", *p.Provider)
		}
		next.Provider = *p.Provider
	}
	if len(p.Providers) > 0 {
		merged := map[string]ProviderConfig{}
		for id, cfg := range next.Providers {
			merged[id] = cfg
		}
		for id, pp := range p.Providers {
			if !slices.Contains(ValidProviders(), id) {
				return Config{}, fmt.Errorf("settings: unknown provider %q", id)
			}
			cfg := merged[id]
			if pp.Model != nil {
				cfg.Model = strings.TrimSpace(*pp.Model)
			}
			if pp.BaseURL != nil {
				cfg.BaseURL = strings.TrimSpace(*pp.BaseURL)
			}
			if pp.ConsentedAt != nil {
				cfg.ConsentedAt = *pp.ConsentedAt
			}
			if pp.APIKey != nil {
				// setSecret deletes on "", so one field both sets and clears.
				if err := s.setSecret(providerAPIKeySecretName(id), strings.TrimSpace(*pp.APIKey)); err != nil {
					return Config{}, err
				}
			}
			merged[id] = normalizeProviderConfig(id, cfg)
		}
		next.Providers = merged
	}
```

(g) `migrateProviderSecrets`에서 `cfg.ClearAPIKey = false` 줄을 지운다. `runtimeProviderConfig`에서 `cfg.ClearAPIKey = false` 줄을 지운다. `sanitizeConfigForMemory`에서 `cfg.ClearAPIKey = false` 줄을 지운다.

(h) `normalizeProviderConfig`를 교체한다(OpenRouter 분기 제거).

```go
func normalizeProviderConfig(provider string, cfg ProviderConfig) ProviderConfig {
	if provider == ProviderOpenAICodex && (cfg.Model == "" || cfg.Model == "gpt-5.3-codex") {
		cfg.Model = DefaultOpenAICodexModel
	}
	return cfg
}
```

- [ ] **Step 4: `engine/internal/settings/providers.go` 생성**

```go
package settings

import "slices"

// ValidProviders returns the provider ids settings.set accepts, in the order
// the settings pane lists them.
func ValidProviders() []string {
	return []string{ProviderOpenAICodex, ProviderAnthropic, ProviderGeminiNative, ProviderOpenAI}
}

// ActiveProvider returns the provider the built-in agent uses. An id outside
// the whitelist — a pre-1.0 "claude-code-cli" or "openrouter" still on disk —
// is not an error: it stays on disk untouched and Codex is used instead.
func (s *Store) ActiveProvider() string {
	s.mu.RLock()
	id := s.cfg.Provider
	s.mu.RUnlock()
	if slices.Contains(ValidProviders(), id) {
		return id
	}
	return ProviderOpenAICodex
}

// HasProviderConsent reports whether the writer agreed to send manuscript
// text to this provider. Consent is per provider: agreeing to OpenAI is not
// agreeing to Anthropic (the v0.9.3 lesson).
func (s *Store) HasProviderConsent(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Providers[id].ConsentedAt > 0
}
```

- [ ] **Step 5: 핸들러 테스트 뒤집기** — `engine/internal/rpc/handlers/settings_test.go`의 `TestSetSettingsHandler_ignoresRetiredProviderFields`(주석 포함)를 다음 둘로 교체한다.

```go
// The provider fields are back on the patch surface (#90/#91). An unknown id
// is rejected outright rather than silently dropped.
func TestSetSettingsHandler_rejectsUnknownProvider(t *testing.T) {
	store := newSettingsFixture(t)
	_, err := SetSettings(store)(context.Background(),
		json.RawMessage(`{"provider":"nope","theme":"dark"}`))
	if err == nil {
		t.Fatal("expected an error for an unknown provider")
	}
}

func TestSetSettingsHandler_appliesProviderPatch(t *testing.T) {
	store := newSettingsFixture(t)
	res, err := SetSettings(store)(context.Background(), json.RawMessage(
		`{"provider":"anthropic","providers":{"anthropic":{"model":"claude-sonnet-4-5","consented_at":1700000000000}}}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got settings.Config
	_ = json.Unmarshal(res, &got)
	if got.Provider != "anthropic" {
		t.Errorf("provider = %q", got.Provider)
	}
	if got.Providers["anthropic"].Model != "claude-sonnet-4-5" || got.Providers["anthropic"].ConsentedAt != 1700000000000 {
		t.Errorf("provider patch not applied: %+v", got.Providers["anthropic"])
	}
}
```

- [ ] **Step 6: 통과 확인**

Run: `cd engine && go build ./... && go test ./internal/settings/ ./internal/rpc/handlers/`
Expected: PASS. 특히 기존 `TestSet_leavesRetiredCompanionSettingsOnDisk`(디스크의 `openrouter` 보존)와 `TestPlaintextSecretsMigrateOutOfSettingsJSON`(레거시 `api_key` 키체인 이관)이 그대로 통과해야 한다. `go build ./...`가 `ProviderClaudeCodeCLI`/`OpenRouter*` 참조로 실패하면 그 참조를 지운다(이 계획 작성 시점에는 `settings.go` 밖에 참조가 없다).

- [ ] **Step 7: 커밋**

```bash
git add engine/internal/settings/settings.go engine/internal/settings/providers.go engine/internal/settings/providers_test.go engine/internal/rpc/handlers/settings_test.go
git commit -m "feat(settings): bring provider settings back with a four-id whitelist and per-provider consent (#91)"
```

---

### Task 2: rpc — 프로바이더 이유 코드와 `ReasonError`

**Files:**
- Modify: `engine/internal/rpc/reason.go`
- Create: `engine/internal/rpc/reason_test.go`

**Interfaces:**
- Produces: `rpc.ReasonProviderNotConfigured`, `rpc.ReasonProviderConsentRequired`, `rpc.ReasonProviderAuthFailed`, `rpc.ReasonProviderRateLimited`, `rpc.ReasonProviderUnreachable`; `type ReasonError struct { Reason string; Err error }` (`Error`, `Unwrap`); `func MethodErrorFrom(err error) *MethodError`.

- [ ] **Step 1: 실패하는 테스트 작성** — `engine/internal/rpc/reason_test.go`

```go
package rpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func reasonIn(t *testing.T, me *MethodError) string {
	t.Helper()
	var data struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(me.Data, &data); err != nil {
		t.Fatalf("data is not a reason payload: %s", me.Data)
	}
	return data.Reason
}

func TestMethodErrorFrom_reasonErrorBecomesInvalidParamsWithReason(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", &ReasonError{Reason: ReasonProviderAuthFailed, Err: errors.New("401")})
	me := MethodErrorFrom(err)
	if me.Code != CodeInvalidParams {
		t.Errorf("code = %d, want %d", me.Code, CodeInvalidParams)
	}
	if got := reasonIn(t, me); got != "provider_auth_failed" {
		t.Errorf("reason = %q", got)
	}
	if me.Message == "" {
		t.Error("message must survive for logs")
	}
}

func TestMethodErrorFrom_plainErrorIsInternal(t *testing.T) {
	me := MethodErrorFrom(errors.New("boom"))
	if me.Code != CodeInternalError || me.Data != nil {
		t.Errorf("got %+v, want internal error without reason data", me)
	}
}

func TestReasonError_unwrapsItsCause(t *testing.T) {
	cause := errors.New("cause")
	err := &ReasonError{Reason: ReasonProviderUnreachable, Err: cause}
	if !errors.Is(err, cause) {
		t.Error("ReasonError must unwrap to its cause")
	}
	if err.Error() != "provider_unreachable: cause" {
		t.Errorf("Error() = %q", err.Error())
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `cd engine && go test ./internal/rpc/ -run 'TestMethodErrorFrom|TestReasonError' 2>&1 | head`
Expected: 컴파일 실패 — `undefined: ReasonError`, `undefined: MethodErrorFrom`.

- [ ] **Step 3: `reason.go` 수정** — 상수 블록에 추가하고, 파일 끝에 타입과 함수를 붙인다. import에 `"errors"`를 추가한다.

```go
	// The built-in agent's provider layer (#90). The first two are states the
	// settings pane fixes; the last three are what the provider said, reduced
	// to something the reader can act on. The provider's raw body stays in
	// the English Message for logs and never becomes UI text.
	ReasonProviderNotConfigured   = "provider_not_configured"
	ReasonProviderConsentRequired = "provider_consent_required"
	ReasonProviderAuthFailed      = "provider_auth_failed"
	ReasonProviderRateLimited     = "provider_rate_limited"
	ReasonProviderUnreachable     = "provider_unreachable"
```

```go
// ReasonError carries a reason code out of a package that does not build
// MethodErrors itself (internal/provider must not know about JSON-RPC).
// Handlers turn it into one with MethodErrorFrom.
type ReasonError struct {
	Reason string
	Err    error
}

func (e *ReasonError) Error() string {
	if e.Err == nil {
		return e.Reason
	}
	return e.Reason + ": " + e.Err.Error()
}

func (e *ReasonError) Unwrap() error { return e.Err }

// MethodErrorFrom maps any error to a MethodError: a ReasonError anywhere in
// the chain becomes an InvalidParams error carrying its reason; anything else
// is an internal error with the message alone.
func MethodErrorFrom(err error) *MethodError {
	if err == nil {
		return &MethodError{Code: CodeInternalError}
	}
	var re *ReasonError
	if errors.As(err, &re) {
		return &MethodError{Code: CodeInvalidParams, Message: err.Error(), Data: ReasonData(re.Reason)}
	}
	return &MethodError{Code: CodeInternalError, Message: err.Error()}
}
```

- [ ] **Step 4: 통과 확인**

Run: `cd engine && go test ./internal/rpc/`
Expected: PASS

- [ ] **Step 5: 커밋**

```bash
git add engine/internal/rpc/reason.go engine/internal/rpc/reason_test.go
git commit -m "feat(rpc): provider reason codes and a ReasonError handlers can map (#91)"
```

---

### Task 3: `internal/provider` — 설정을 tars 클라이언트로 해석

**Files:**
- Create: `engine/internal/provider/provider.go`
- Create: `engine/internal/provider/provider_test.go`

**Interfaces:**
- Consumes: Task 1의 `settings.ValidProviders`, `ActiveProvider`, `ProviderConfigFor`, `ProviderOpenAICodex`; Task 2의 `rpc.ReasonError`와 이유 코드; tars `llm.ProviderOptions`, `llm.Client`, `llm.NewProvider`, `llm.ModelFetcher`, `llm.NewModelFetcher`.
- Produces:
  - `type Resolved struct { ID, Model, APIKey, BaseURL, CodexHome string; ConsentedAt int64 }`, `(Resolved).Configured() bool`, `(Resolved).Consented() bool`
  - `func Options(r Resolved) llm.ProviderOptions`
  - `type ClientFactory func(llm.ProviderOptions) (llm.Client, error)`
  - `func NewSource(st *settings.Store, codexHome string) *Source`, `(*Source).WithFactory`, `(*Source).WithFetcher`
  - `(*Source).Resolve(id string) (Resolved, error)` — `""`는 활성 프로바이더
  - `(*Source).Client(id string) (llm.Client, Resolved, error)` — #93의 에이전트 루프가 턴마다 부른다

- [ ] **Step 1: 실패하는 테스트 작성** — `engine/internal/provider/provider_test.go`

```go
package provider

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
```

- [ ] **Step 2: 실패 확인**

Run: `cd engine && go test ./internal/provider/ 2>&1 | head`
Expected: 패키지가 없어 컴파일 실패.

- [ ] **Step 3: `engine/internal/provider/provider.go` 생성**

```go
// Package provider turns the writer's settings into a tars llm.Client for the
// built-in agent (#90). It is the only package besides internal/agent allowed
// to import tars/pkg/llm — scripts/validate-story-core-deps.sh enforces that,
// so the story core and the MCP tool layer stay model-free for every agent.
package provider

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/settings"
	"github.com/devlikebear/tars/pkg/llm"
)

// codexRefreshStorageEnv is read by tars when it refreshes a Codex token. Its
// macOS default shells out to the `security` CLI, which a sandboxed App Store
// build cannot do, so every build uses the file store: auth.json at 0600 in
// Linetta's data directory, the way the Codex CLI itself keeps it.
const codexRefreshStorageEnv = "TARS_OPENAI_CODEX_REFRESH_TOKEN_STORAGE"

// codexAuthFile is the credential file tars reads under CodexHome.
const codexAuthFile = "auth.json"

// Resolved is one provider's effective configuration: settings plus the key
// the settings store keeps outside settings.json.
type Resolved struct {
	ID          string
	Model       string
	APIKey      string
	BaseURL     string
	CodexHome   string // openai-codex only: the directory holding auth.json
	ConsentedAt int64
}

// Configured reports whether the provider has a credential to call with: an
// API key, or for Codex a completed login (#92 writes the file; here it only
// has to exist).
func (r Resolved) Configured() bool {
	if r.ID == settings.ProviderOpenAICodex {
		_, err := os.Stat(filepath.Join(r.CodexHome, codexAuthFile))
		return err == nil
	}
	return strings.TrimSpace(r.APIKey) != ""
}

// Consented reports whether the writer agreed to send text to this provider.
func (r Resolved) Consented() bool { return r.ConsentedAt > 0 }

// Options is the tars-facing shape of a Resolved provider. Model stays empty
// when unset so tars applies its own default; Linetta keeps no model catalog.
func Options(r Resolved) llm.ProviderOptions {
	opts := llm.ProviderOptions{
		Provider: r.ID,
		Model:    strings.TrimSpace(r.Model),
		BaseURL:  strings.TrimSpace(r.BaseURL),
	}
	if r.ID == settings.ProviderOpenAICodex {
		opts.AuthConfig.CodexHome = r.CodexHome
	} else {
		opts.APIKey = strings.TrimSpace(r.APIKey)
	}
	return opts
}

// ClientFactory builds an llm.Client. Production is llm.NewProvider; tests
// inject one that never dials.
type ClientFactory func(opts llm.ProviderOptions) (llm.Client, error)

// Source resolves providers from the settings store.
type Source struct {
	settings  *settings.Store
	codexHome string
	factory   ClientFactory
	fetcher   llm.ModelFetcher
}

// NewSource wires the store. codexHome is where Codex's auth.json lives —
// $LINETTA_HOME/codex on every platform (#92 adds the ~/.codex fallback).
func NewSource(st *settings.Store, codexHome string) *Source {
	_ = os.Setenv(codexRefreshStorageEnv, "file")
	return &Source{
		settings:  st,
		codexHome: codexHome,
		factory:   llm.NewProvider,
		fetcher:   llm.NewModelFetcher(),
	}
}

// WithFactory replaces the client factory (tests).
func (s *Source) WithFactory(f ClientFactory) *Source {
	s.factory = f
	return s
}

// WithFetcher replaces the model lister (tests).
func (s *Source) WithFetcher(f llm.ModelFetcher) *Source {
	s.fetcher = f
	return s
}

// Resolve returns the effective config for id, or for the active provider
// when id is empty. Read on every call so a settings change applies to the
// next agent turn without a restart.
func (s *Source) Resolve(id string) (Resolved, error) {
	if id == "" {
		id = s.settings.ActiveProvider()
	}
	if !slices.Contains(settings.ValidProviders(), id) {
		return Resolved{}, &rpc.ReasonError{
			Reason: rpc.ReasonProviderNotConfigured,
			Err:    fmt.Errorf("unknown provider %q", id),
		}
	}
	cfg := s.settings.ProviderConfigFor(id)
	return Resolved{
		ID:          id,
		Model:       cfg.Model,
		APIKey:      cfg.APIKey,
		BaseURL:     cfg.BaseURL,
		CodexHome:   s.codexHome,
		ConsentedAt: cfg.ConsentedAt,
	}, nil
}

// Client builds a client for id ("" = active). It refuses before any network
// activity when the provider is not configured or not consented to. The
// consent rule is "not a byte leaves without it", and this is the only path
// that produces something able to send one, so the check lives here.
func (s *Source) Client(id string) (llm.Client, Resolved, error) {
	r, err := s.Resolve(id)
	if err != nil {
		return nil, Resolved{}, err
	}
	if !r.Configured() {
		return nil, r, &rpc.ReasonError{
			Reason: rpc.ReasonProviderNotConfigured,
			Err:    fmt.Errorf("%s: no credential", r.ID),
		}
	}
	if !r.Consented() {
		return nil, r, &rpc.ReasonError{
			Reason: rpc.ReasonProviderConsentRequired,
			Err:    fmt.Errorf("%s: consent required", r.ID),
		}
	}
	c, err := s.factory(Options(r))
	if err != nil {
		return nil, r, Classify(r.ID, err)
	}
	return c, r, nil
}
```

- [ ] **Step 4: `engine/internal/provider/errors.go` 생성** (Task 3의 `Client`가 `Classify`를 부르므로 여기서 함께 만든다. 테스트는 Task 4에서 추가.)

```go
package provider

import (
	"errors"
	"fmt"
	"strings"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/tars/pkg/llm"
)

// Classify reduces a provider failure to a reason code the UI can translate.
// A ReasonError passes through untouched. Everything else becomes one of
// auth_failed / rate_limited / unreachable: HTTP status first, then the few
// phrases tars uses for local credential problems, then the default. The
// English message keeps only the first line, capped, so a provider's JSON
// body does not ride along into logs verbatim (the v0.8.5 lesson).
func Classify(id string, err error) error {
	if err == nil {
		return nil
	}
	var re *rpc.ReasonError
	if errors.As(err, &re) {
		return err
	}
	reason := rpc.ReasonProviderUnreachable
	var pe *llm.ProviderError
	if errors.As(err, &pe) {
		switch pe.StatusCode {
		case 401, 403:
			reason = rpc.ReasonProviderAuthFailed
		case 402, 429:
			reason = rpc.ReasonProviderRateLimited
		}
	}
	if reason == rpc.ReasonProviderUnreachable {
		msg := strings.ToLower(err.Error())
		switch {
		case strings.Contains(msg, "api key is required"),
			strings.Contains(msg, "invalid api key"),
			strings.Contains(msg, "refresh token"),
			strings.Contains(msg, "unauthorized"),
			strings.Contains(msg, "auth.json"):
			reason = rpc.ReasonProviderAuthFailed
		case strings.Contains(msg, "rate limit"),
			strings.Contains(msg, "quota"),
			strings.Contains(msg, "insufficient credit"):
			reason = rpc.ReasonProviderRateLimited
		}
	}
	return &rpc.ReasonError{Reason: reason, Err: fmt.Errorf("%s: %s", id, firstLine(err))}
}

func firstLine(err error) string {
	line := strings.TrimSpace(strings.SplitN(err.Error(), "\n", 2)[0])
	const max = 200
	if len(line) > max {
		return line[:max] + "…"
	}
	return line
}
```

- [ ] **Step 5: 통과 확인**

Run: `cd engine && go test ./internal/provider/`
Expected: PASS

- [ ] **Step 6: 커밋**

```bash
git add engine/internal/provider/provider.go engine/internal/provider/errors.go engine/internal/provider/provider_test.go
git commit -m "feat(provider): resolve settings into a tars client, refusing before consent (#91)"
```

---

### Task 4: `internal/provider` — 목록, 모델 조회, 연결 테스트, 에러 분류

**Files:**
- Create: `engine/internal/provider/catalog.go`
- Create: `engine/internal/provider/catalog_test.go`

**Interfaces:**
- Consumes: Task 3의 `Source`, `Resolve`, `Client`, `Options`, `Classify`.
- Produces: `type Status struct { ID, Auth string; Active, Configured, Consented bool; Model, BaseURL string }` (JSON: `id`, `auth`, `active`, `configured`, `consented`, `model`, `base_url`); `(*Source).List() []Status`; `(*Source).ListModels(ctx, id) ([]string, error)`; `(*Source).Test(ctx, id) error`.

- [ ] **Step 1: 실패하는 테스트 작성** — `engine/internal/provider/catalog_test.go`

```go
package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/tars/pkg/llm"
)

type fakeFetcher struct {
	models []string
	err    error
	seen   llm.ProviderOptions
}

func (f *fakeFetcher) FetchModels(_ context.Context, opts llm.ProviderOptions) ([]string, error) {
	f.seen = opts
	return f.models, f.err
}

func TestList_reportsTheFourInOrderWithState(t *testing.T) {
	src, st, _ := newSource(t)
	configure(t, st, "anthropic", "sk-ant-test", true)
	got := src.List()
	if len(got) != 4 {
		t.Fatalf("List = %d entries, want 4", len(got))
	}
	wantIDs := []string{"openai-codex", "anthropic", "gemini-native", "openai"}
	for i, id := range wantIDs {
		if got[i].ID != id {
			t.Errorf("List[%d].ID = %q, want %q", i, got[i].ID, id)
		}
	}
	if got[0].Auth != "oauth" || got[1].Auth != "api_key" {
		t.Errorf("auth kinds = %q/%q", got[0].Auth, got[1].Auth)
	}
	a := got[1]
	if !a.Active || !a.Configured || !a.Consented {
		t.Errorf("anthropic = %+v, want active+configured+consented", a)
	}
	if o := got[3]; o.Active || o.Configured || o.Consented {
		t.Errorf("openai = %+v, want nothing set", o)
	}
}

func TestListModels_needsACredentialButNotConsent(t *testing.T) {
	src, st, _ := newSource(t)
	f := &fakeFetcher{models: []string{"b-model", "a-model"}}
	src.WithFetcher(f)

	_, err := src.ListModels(context.Background(), "anthropic")
	if reasonOf(t, err) != rpc.ReasonProviderNotConfigured {
		t.Fatalf("without a key: %v", err)
	}

	configure(t, st, "anthropic", "sk-ant-test", false) // key, no consent
	models, err := src.ListModels(context.Background(), "anthropic")
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 2 || models[0] != "a-model" {
		t.Errorf("models = %v, want sorted", models)
	}
	if f.seen.Provider != "anthropic" || f.seen.APIKey != "sk-ant-test" {
		t.Errorf("fetcher saw %+v", f.seen)
	}
}

func TestListModels_classifiesFetchFailures(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"401", &llm.ProviderError{Provider: "anthropic", StatusCode: 401, Message: `{"error":"bad key"}`}, rpc.ReasonProviderAuthFailed},
		{"429", &llm.ProviderError{Provider: "anthropic", StatusCode: 429, Message: "slow down"}, rpc.ReasonProviderRateLimited},
		{"network", errors.New("dial tcp: connection refused"), rpc.ReasonProviderUnreachable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src, st, _ := newSource(t)
			configure(t, st, "anthropic", "sk-ant-test", false)
			src.WithFetcher(&fakeFetcher{err: tc.err})
			_, err := src.ListModels(context.Background(), "anthropic")
			if got := reasonOf(t, err); got != tc.want {
				t.Errorf("reason = %q, want %q (%v)", got, tc.want, err)
			}
			if got := err.Error(); len(got) > 260 {
				t.Errorf("message not capped: %d chars", len(got))
			}
		})
	}
}

func TestTest_refusesWithoutConsentAndNeverDials(t *testing.T) {
	src, st, _ := newSource(t)
	configure(t, st, "anthropic", "sk-ant-test", false)
	src.WithFactory(func(llm.ProviderOptions) (llm.Client, error) {
		t.Fatal("no client may be built before consent")
		return nil, nil
	})
	err := src.Test(context.Background(), "anthropic")
	if reasonOf(t, err) != rpc.ReasonProviderConsentRequired {
		t.Errorf("reason = %v", err)
	}
}

func TestTest_asksOnceAndClassifiesTheAnswer(t *testing.T) {
	src, st, _ := newSource(t)
	configure(t, st, "anthropic", "sk-ant-test", true)
	calls := 0
	src.WithFactory(func(llm.ProviderOptions) (llm.Client, error) {
		return fakeClient{ask: func(string) (string, error) {
			calls++
			return "OK", nil
		}}, nil
	})
	if err := src.Test(context.Background(), "anthropic"); err != nil {
		t.Fatalf("Test: %v", err)
	}
	if calls != 1 {
		t.Errorf("Ask called %d times, want 1", calls)
	}

	src.WithFactory(func(llm.ProviderOptions) (llm.Client, error) {
		return fakeClient{ask: func(string) (string, error) {
			return "", &llm.ProviderError{Provider: "anthropic", StatusCode: 401, Message: "nope"}
		}}, nil
	})
	err := src.Test(context.Background(), "anthropic")
	if reasonOf(t, err) != rpc.ReasonProviderAuthFailed {
		t.Errorf("reason = %v", err)
	}
}

func TestClassify_passesReasonErrorsThroughAndNilStaysNil(t *testing.T) {
	in := &rpc.ReasonError{Reason: rpc.ReasonProviderConsentRequired}
	if out := Classify("anthropic", in); out != in {
		t.Errorf("Classify rewrapped a ReasonError: %v", out)
	}
	if Classify("anthropic", nil) != nil {
		t.Error("Classify(nil) must be nil")
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `cd engine && go test ./internal/provider/ 2>&1 | head`
Expected: 컴파일 실패 — `src.List undefined`, `src.ListModels undefined`, `src.Test undefined`.

- [ ] **Step 3: `engine/internal/provider/catalog.go` 생성**

```go
package provider

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/devlikebear/linetta/engine/internal/rpc"
	"github.com/devlikebear/linetta/engine/internal/settings"
)

// Status is what the settings pane shows per provider. It never carries a
// secret: Configured says a key or login exists, not what it is.
type Status struct {
	ID         string `json:"id"`
	Auth       string `json:"auth"` // "oauth" | "api_key"
	Active     bool   `json:"active"`
	Configured bool   `json:"configured"`
	Consented  bool   `json:"consented"`
	Model      string `json:"model,omitempty"`
	BaseURL    string `json:"base_url,omitempty"`
}

// testTimeout bounds providers.test; a hung endpoint must not hang the pane.
const testTimeout = 30 * time.Second

// testPrompt is the one line providers.test sends. No manuscript text — but
// it still goes through Client, so it still needs consent.
const testPrompt = "Reply with the single word OK."

func authKind(id string) string {
	if id == settings.ProviderOpenAICodex {
		return "oauth"
	}
	return "api_key"
}

// List describes every provider this build offers, in whitelist order.
func (s *Source) List() []Status {
	active := s.settings.ActiveProvider()
	out := make([]Status, 0, len(settings.ValidProviders()))
	for _, id := range settings.ValidProviders() {
		r, err := s.Resolve(id)
		if err != nil {
			continue
		}
		out = append(out, Status{
			ID:         id,
			Auth:       authKind(id),
			Active:     id == active,
			Configured: r.Configured(),
			Consented:  r.Consented(),
			Model:      r.Model,
			BaseURL:    r.BaseURL,
		})
	}
	return out
}

// ListModels asks the provider for its model ids. It needs a credential but
// not consent: no manuscript text is involved, and the pane lists models
// before the writer has read the consent sentence.
func (s *Source) ListModels(ctx context.Context, id string) ([]string, error) {
	r, err := s.Resolve(id)
	if err != nil {
		return nil, err
	}
	if !r.Configured() {
		return nil, &rpc.ReasonError{
			Reason: rpc.ReasonProviderNotConfigured,
			Err:    fmt.Errorf("%s: no credential", r.ID),
		}
	}
	models, err := s.fetcher.FetchModels(ctx, Options(r))
	if err != nil {
		return nil, Classify(r.ID, err)
	}
	sort.Strings(models)
	return models, nil
}

// Test sends one short prompt through a real client. It runs through Client
// on purpose: even the connection test sends nothing before consent.
func (s *Source) Test(ctx context.Context, id string) error {
	c, r, err := s.Client(id)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, testTimeout)
	defer cancel()
	if _, err := c.Ask(ctx, testPrompt); err != nil {
		return Classify(r.ID, err)
	}
	return nil
}
```

- [ ] **Step 4: 통과 확인**

Run: `cd engine && go test ./internal/provider/ && go vet ./internal/provider/`
Expected: PASS

- [ ] **Step 5: 커밋**

```bash
git add engine/internal/provider/catalog.go engine/internal/provider/catalog_test.go
git commit -m "feat(provider): provider list, live model list, and a consent-gated connection test (#91)"
```

---

### Task 5: `providers.*` RPC — 핸들러, 엔진 배선, 렌더러 허용 목록, 프론트 래퍼

**Files:**
- Create: `engine/internal/rpc/handlers/providers.go`
- Create: `engine/internal/rpc/handlers/providers_test.go`
- Create: `engine/internal/engineapp/providers.go`
- Modify: `engine/internal/engineapp/engineapp.go` (MCP 배선 뒤, `s.Handle("mcp.activity", …)` 뒤)
- Modify: `apps/desktop/src-tauri/src/lib.rs:94` (`"projects.update",` 뒤)
- Modify: `apps/desktop/src/lib/types.ts:488-506, 517-518, 550-551`
- Modify: `apps/desktop/src/lib/rpc.ts` (`export const mcp` 뒤)
- Modify: `apps/desktop/src/routes/Settings.test.tsx:164,178`, `apps/desktop/src/routes/Library.test.tsx` (7곳)

**Interfaces:**
- Consumes: Task 4의 `provider.Source.List/ListModels/Test`; Task 2의 `rpc.MethodErrorFrom`.
- Produces:
  - handlers: `type ProviderService interface { List(ctx) (json.RawMessage, error); ListModels(ctx, id string) ([]string, error); Test(ctx, id string) error }`, `ProvidersList/ProvidersListModels/ProvidersTest(svc) rpc.Handler`
  - RPC: `providers.list` → `Status[]`; `providers.list_models {provider}` → `{models: string[]}`; `providers.test {provider}` → `{ok: true}`
  - 프론트: `providers.list()`, `providers.listModels(id)`, `providers.test(id)`; 타입 `ProviderID`, `ProviderConfig`, `ProviderPatch`, `ProviderStatus` — #94의 `ProviderSection`이 쓴다

- [ ] **Step 1: 실패하는 핸들러 테스트 작성** — `engine/internal/rpc/handlers/providers_test.go`

```go
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/devlikebear/linetta/engine/internal/rpc"
)

type fakeProviderService struct {
	list   json.RawMessage
	models []string
	err    error
	seenID string
}

func (f *fakeProviderService) List(context.Context) (json.RawMessage, error) { return f.list, f.err }
func (f *fakeProviderService) ListModels(_ context.Context, id string) ([]string, error) {
	f.seenID = id
	return f.models, f.err
}
func (f *fakeProviderService) Test(_ context.Context, id string) error {
	f.seenID = id
	return f.err
}

func TestProvidersList_returnsTheServicePayload(t *testing.T) {
	svc := &fakeProviderService{list: json.RawMessage(`[{"id":"anthropic","auth":"api_key"}]`)}
	res, err := ProvidersList(svc)(context.Background(), nil)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if string(res) != `[{"id":"anthropic","auth":"api_key"}]` {
		t.Errorf("payload = %s", res)
	}
}

func TestProvidersListModels_passesTheIdAndWrapsModels(t *testing.T) {
	svc := &fakeProviderService{models: []string{"a", "b"}}
	res, err := ProvidersListModels(svc)(context.Background(), json.RawMessage(`{"provider":"openai"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if svc.seenID != "openai" {
		t.Errorf("provider id = %q", svc.seenID)
	}
	var out struct {
		Models []string `json:"models"`
	}
	_ = json.Unmarshal(res, &out)
	if len(out.Models) != 2 {
		t.Errorf("models = %v", out.Models)
	}
}

func TestProvidersListModels_reasonErrorKeepsItsCode(t *testing.T) {
	svc := &fakeProviderService{err: &rpc.ReasonError{Reason: rpc.ReasonProviderAuthFailed, Err: errors.New("401")}}
	_, err := ProvidersListModels(svc)(context.Background(), json.RawMessage(`{"provider":"anthropic"}`))
	var me *rpc.MethodError
	if !errors.As(err, &me) {
		t.Fatalf("want MethodError, got %v", err)
	}
	if me.Code != rpc.CodeInvalidParams {
		t.Errorf("code = %d", me.Code)
	}
	var data struct {
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(me.Data, &data)
	if data.Reason != "provider_auth_failed" {
		t.Errorf("reason = %q", data.Reason)
	}
}

func TestProvidersTest_okPayload(t *testing.T) {
	svc := &fakeProviderService{}
	res, err := ProvidersTest(svc)(context.Background(), json.RawMessage(`{"provider":"gemini-native"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if svc.seenID != "gemini-native" || string(res) != `{"ok":true}` {
		t.Errorf("id=%q payload=%s", svc.seenID, res)
	}
}
```

- [ ] **Step 2: 실패 확인**

Run: `cd engine && go test ./internal/rpc/handlers/ -run 'TestProviders' 2>&1 | head`
Expected: 컴파일 실패 — `undefined: ProvidersList` 등.

- [ ] **Step 3: `engine/internal/rpc/handlers/providers.go` 생성**

```go
package handlers

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/rpc"
)

// ProviderService is the slice of internal/provider the RPC layer needs.
// Declared as an interface with plain types because handlers must not link
// tars/pkg/llm (scripts/validate-story-core-deps.sh) and the provider
// package does. List hands back JSON the way MCPController.Status does.
type ProviderService interface {
	List(ctx context.Context) (json.RawMessage, error)
	ListModels(ctx context.Context, id string) ([]string, error)
	Test(ctx context.Context, id string) error
}

type providerParams struct {
	Provider string `json:"provider,omitempty"` // empty means the active provider
}

func decodeProviderParams(params json.RawMessage) providerParams {
	var p providerParams
	if len(params) > 0 {
		_ = json.Unmarshal(params, &p)
	}
	return p
}

// ProvidersList returns a handler for providers.list: every provider this
// build offers and where each one stands (configured, consented, active).
func ProvidersList(svc ProviderService) rpc.Handler {
	return func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
		out, err := svc.List(ctx)
		if err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		return out, nil
	}
}

// ProvidersListModels returns a handler for providers.list_models. A failure
// is an RPC error; the pane falls back to free-text entry.
func ProvidersListModels(svc ProviderService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		p := decodeProviderParams(params)
		models, err := svc.ListModels(ctx, p.Provider)
		if err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		if models == nil {
			models = []string{}
		}
		return json.Marshal(struct {
			Models []string `json:"models"`
		}{models})
	}
}

// ProvidersTest returns a handler for providers.test: one short prompt,
// consent-gated on the provider side.
func ProvidersTest(svc ProviderService) rpc.Handler {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		p := decodeProviderParams(params)
		if err := svc.Test(ctx, p.Provider); err != nil {
			return nil, rpc.MethodErrorFrom(err)
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
}
```

- [ ] **Step 4: 핸들러 테스트 통과 확인**

Run: `cd engine && go test ./internal/rpc/handlers/`
Expected: PASS

- [ ] **Step 5: 엔진 배선** — `engine/internal/engineapp/providers.go` 생성

```go
package engineapp

import (
	"context"
	"encoding/json"

	"github.com/devlikebear/linetta/engine/internal/provider"
)

// providerService adapts *provider.Source to handlers.ProviderService. The
// handlers see JSON and strings only, so they never link tars/pkg/llm.
type providerService struct {
	src *provider.Source
}

func (p providerService) List(context.Context) (json.RawMessage, error) {
	return json.Marshal(p.src.List())
}

func (p providerService) ListModels(ctx context.Context, id string) ([]string, error) {
	return p.src.ListModels(ctx, id)
}

func (p providerService) Test(ctx context.Context, id string) error {
	return p.src.Test(ctx, id)
}
```

`engine/internal/engineapp/engineapp.go`: import에 `"github.com/devlikebear/linetta/engine/internal/provider"`를 추가하고, `a.closers = append(a.closers, stopMCP)` 바로 뒤에 추가한다.

```go
	// The built-in agent's provider layer (#90). Resolves the writer's
	// provider settings into a client on demand; nothing here dials until an
	// agent run or a connection test asks it to. Codex's auth.json lives under
	// the data directory so the App Store build can reach it.
	providerSrc := provider.NewSource(settingsStore, filepath.Join(home, "codex"))
	providers := providerService{src: providerSrc}
```

그리고 `s.Handle("mcp.activity", handlers.MCPActivity(mcpCtrl))` 바로 뒤에 등록한다.

```go
	s.Handle("providers.list", handlers.ProvidersList(providers))
	s.Handle("providers.list_models", handlers.ProvidersListModels(providers))
	s.Handle("providers.test", handlers.ProvidersTest(providers))
```

- [ ] **Step 6: 엔진 빌드·테스트**

Run: `cd engine && go build ./... && go test ./internal/engineapp/ ./internal/rpc/...`
Expected: PASS. (`engineapp_test.go`의 스모크가 등록된 메서드 수를 세고 있다면 3을 더한다 — 실패 메시지가 알려준다.)

- [ ] **Step 7: 렌더러 허용 목록** — `apps/desktop/src-tauri/src/lib.rs`의 `"projects.update",` 바로 뒤에 넣는다. 이진 탐색이므로 위치가 곧 정확성이다: `projects.*` < `providers.*` < `relationships.*`.

```rust
    "providers.list",
    "providers.list_models",
    "providers.test",
```

- [ ] **Step 8: 프론트 타입** — `apps/desktop/src/lib/types.ts`

`ProviderID`와 `ProviderConfig`(488-506행)를 교체한다.

```ts
/** The four providers the built-in agent can use (#90). Codex logs in with
 *  OAuth inside the app; the other three take an API key. "openai" is the
 *  OpenAI-compatible family — base_url points it at OpenRouter, Ollama, LM
 *  Studio. */
export type ProviderID = "openai-codex" | "anthropic" | "gemini-native" | "openai";
export type WebSearchProvider = "brave" | "perplexity";

export type AppLanguage = "ko" | "en" | "ja";

/** One provider's stored entry as settings.get shows it. The key itself never
 *  arrives — api_key_set says whether one is stored. */
export interface ProviderConfig {
  model?: string;
  api_key_set?: boolean;
  base_url?: string;
  /** Per-provider data-sharing consent, epoch ms; absent or 0 means none. */
  consented_at?: number;
}

/** One provider's entry in a settings.set patch. An empty api_key deletes
 *  the stored key; consented_at 0 revokes. */
export interface ProviderPatch {
  model?: string;
  base_url?: string;
  api_key?: string;
  consented_at?: number;
}

/** providers.list — where each provider stands. Never carries a secret. */
export interface ProviderStatus {
  id: ProviderID;
  auth: "oauth" | "api_key";
  active: boolean;
  configured: boolean;
  consented: boolean;
  model?: string;
  base_url?: string;
}
```

`SettingsPatch`의 `providers?: Record<string, ProviderConfig>;`(551행)를 `providers?: Record<string, ProviderPatch>;`로 바꾼다. `Settings.providers`(518행)는 그대로 `Record<string, ProviderConfig>`.

- [ ] **Step 9: 프론트 래퍼** — `apps/desktop/src/lib/rpc.ts`

import 목록에 `ProviderStatus`를 추가하고, `export const mcp = { … };` 바로 뒤에 넣는다.

```ts
export const providers = {
  list: () => rpcCall<ProviderStatus[]>("providers.list"),
  listModels: (provider: string) =>
    rpcCall<{ models: string[] }>("providers.list_models", { provider }),
  test: (provider: string) => rpcCall<{ ok: true }>("providers.test", { provider }),
};
```

- [ ] **Step 10: 테스트 픽스처의 옛 프로바이더 id 교체** — `ProviderID`가 좁아지면 `tsc -b`가 픽스처에서 실패한다.

```bash
cd apps/desktop && sed -i '' 's/provider: "openrouter"/provider: "openai-codex"/g' src/routes/Settings.test.tsx && sed -i '' 's/provider: "claude-code-cli"/provider: "openai-codex"/g' src/routes/Library.test.tsx && grep -rn '"claude-code-cli"\|"openrouter"' src | grep -v lib/types.ts
```

Expected: 마지막 grep이 아무것도 출력하지 않는다.

- [ ] **Step 11: 프론트·셸 검증**

Run: `cd apps/desktop && pnpm lint && pnpm test && pnpm build && cd src-tauri && cargo check && cargo test`
Expected: 전부 PASS. `rpcAllowlist.test.ts`의 세 케이스(호출 ⊆ 허용, 허용 ⊆ 호출, 정렬)가 통과해야 한다.

- [ ] **Step 12: 커밋**

```bash
git add engine/internal/rpc/handlers/providers.go engine/internal/rpc/handlers/providers_test.go engine/internal/engineapp/providers.go engine/internal/engineapp/engineapp.go apps/desktop/src-tauri/src/lib.rs apps/desktop/src/lib/types.ts apps/desktop/src/lib/rpc.ts apps/desktop/src/routes/Settings.test.tsx apps/desktop/src/routes/Library.test.tsx
git commit -m "feat: providers.list / list_models / test over RPC, wired through to the renderer (#91)"
```

---

### Task 6: 의존성 게이트 좁히기

**Files:**
- Modify: `scripts/validate-story-core-deps.sh`

**Interfaces:**
- Consumes: Task 3·4의 `internal/provider`(`pkg/llm` import), Task 5의 핸들러(`pkg/llm` 무유입).
- Produces: `make test-go`가 새 규칙으로 돈다. #93의 `internal/agent`는 이 스크립트의 `allowed` 목록에 이미 들어 있다.

- [ ] **Step 1: 현재 게이트가 실패함을 확인** (Task 3 이후 `provider`가 `pkg/llm`을 링크하므로 옛 규칙은 깨진다)

Run: `bash scripts/validate-story-core-deps.sh; echo "exit=$?"`
Expected: `github.com/devlikebear/tars/pkg/llm` 출력 후 `exit=1`.

- [ ] **Step 2: 스크립트 교체**

```bash
#!/usr/bin/env bash
# Linetta's story core does not talk to a language model. The MCP-first pivot
# (#47) moved AI collaboration behind the MCP boundary; the built-in agent
# (#90) brought a model client back — into two packages only, as one more
# client of the same MCP tools.
#
# This gate keeps both facts true:
#   1. tars' agent loop and session code never link into the engine at all.
#      The built-in agent's loop is written here (internal/agent) so that
#      cancellation, the activity log and the writer's limits stay in hand.
#   2. tars/pkg/llm is imported only by internal/provider and internal/agent,
#      and never reaches the story core, the MCP tool layer or the RPC
#      handlers — they are what every agent, external or built-in, runs on.
#
# tars' pkg/tools (fact-book URL capture) and pkg/memory (remembered facts) are
# deliberately still linked — neither carries a model.
set -euo pipefail
cd "$(dirname "$0")/../engine"

banned='github.com/devlikebear/tars/pkg/(agentloop|session)'
if go list -deps ./... | grep -E "$banned"; then
  echo "error: the engine must not link tars agentloop/session code" >&2
  echo "       the built-in agent's loop lives in internal/agent" >&2
  exit 1
fi

llm='github.com/devlikebear/tars/pkg/llm'
for pkg in ./internal/storycontext ./internal/storyops ./internal/mcphost ./internal/rpc/handlers; do
  if go list -deps "$pkg" | grep -qx "$llm"; then
    echo "error: $pkg must not link $llm — it is shared by every agent" >&2
    exit 1
  fi
done

allowed='internal/provider|internal/agent'
importers=$(go list -f '{{.ImportPath}}: {{join .Imports " "}}' ./... \
  | grep -F "$llm" | cut -d: -f1 | grep -Ev "/($allowed)$" || true)
if [ -n "$importers" ]; then
  echo "error: only internal/provider and internal/agent may import $llm; found:" >&2
  echo "$importers" >&2
  exit 1
fi

echo "engine deps OK: pkg/llm only in provider/agent; no agentloop/session; story core clean"
```

- [ ] **Step 3: 통과 확인**

Run: `bash scripts/validate-story-core-deps.sh`
Expected: `engine deps OK: …`

- [ ] **Step 4: 게이트가 실제로 잡는지 음성 확인** — 핸들러에 금지 import를 잠깐 넣었다 뺀다.

```bash
printf 'package handlers\n\nimport _ "github.com/devlikebear/tars/pkg/llm"\n' > engine/internal/rpc/handlers/zz_gate_probe.go
bash scripts/validate-story-core-deps.sh; echo "exit=$?"
rm engine/internal/rpc/handlers/zz_gate_probe.go
bash scripts/validate-story-core-deps.sh
```

Expected: 첫 실행은 `error: ./internal/rpc/handlers must not link …`와 `exit=1`, 삭제 후 두 번째 실행은 OK. `git status`에 프로브 파일이 남아 있지 않아야 한다.

- [ ] **Step 5: 커밋**

```bash
git add scripts/validate-story-core-deps.sh
git commit -m "chore(engine): narrow the tars dependency gate to allow pkg/llm in provider/agent only (#91)"
```

---

### Task 7: 전체 검증 계약

**Files:** 없음 (검증만). 실패하면 해당 작업으로 돌아가 고치고 그 작업의 커밋에 `fixup`을 얹는다.

- [ ] **Step 1: 데스크톱·엔진·셸 전체**

Run: `make test`
Expected: `test-go`(엔진 테스트 + 게이트), `test-desktop`(lint·vitest·build), `test-tauri`(cargo check·test) 전부 PASS.

- [ ] **Step 2: 모바일 태그**

Run: `make test-mobile-engine`
Expected: PASS. `internal/provider`는 태그 없이 컴파일되고 모바일에서도 링크된다(호출부는 데스크톱뿐).

- [ ] **Step 3: MAS 태그와 Windows 크로스컴파일**

Run: `cd engine && go build -tags mas ./... && GOOS=windows GOARCH=amd64 go build ./...`
Expected: 둘 다 출력 없이 성공. Windows가 `pkg/llm` 안에서 실패하면 tars 쪽 문제다 — 이 계획의 범위 밖이므로 실패 로그를 #91에 남기고 멈춘다.

- [ ] **Step 4: 실물 확인 (선택, 키가 있을 때)** — 앱을 띄우지 않고 엔진 CLI로 확인한다.

```bash
cd engine && LINETTA_HOME=/tmp/linetta-91 go run ./cmd/linetta-engine --stdio <<'EOF'
{"jsonrpc":"2.0","id":1,"method":"providers.list"}
EOF
```

Expected: 프로바이더 4개, 전부 `configured:false`, `openai-codex`만 `active:true`. (`--stdio`가 JSON-RPC를 stdin/stdout으로 서비스한다 — `engine/cmd/linetta-engine/main.go:20`.)

- [ ] **Step 5: 이슈 갱신** — #91의 체크리스트를 채우고, 스펙 14절의 결정과 다르게 한 것이 있으면 적는다. 이 계획에서 스펙과 달라진 점 하나: patch의 키 삭제가 `clear_api_key` 플래그가 아니라 **빈 `api_key`** 로 표현된다(필드 하나가 설정과 삭제를 모두 맡는다). `ProviderConfig.ClearAPIKey`/`CliPath`는 제거됐고, 디스크의 `cli_path` 값은 다음 저장에서 사라진다(쓰는 프로바이더가 없다).

---

## 자기 검토 기록

- **스펙 커버리지:** 5.1 목록 4개(Task 1·4), 5.2 데이터 모델·화이트리스트·관용 로드·키체인(Task 1), 5.4 프로바이더별 동의와 `provider_consent_required`(Task 1·3), 6절 `providers.*` 3개와 이유 코드 5개(Task 2·5), 4.2 게이트 좁히기(Task 6), 5.3의 `TARS_OPENAI_CODEX_REFRESH_TOKEN_STORAGE=file`(Task 3), `RENDERER_ENGINE_METHODS`·`rpc.ts` 동시 추가(Task 5). Codex 로그인(5.3 나머지)은 #92, 에이전트 루프는 #93, 설정 UI는 #94로 이 계획 밖.
- **이름 일관성:** `settings.ProviderPatch{Model,BaseURL,APIKey,ConsentedAt}`·`ValidProviders`·`ActiveProvider`·`HasProviderConsent`(Task 1) → `provider.Source.Resolve/Client`(Task 3) → `List/ListModels/Test`(Task 4) → `handlers.ProviderService`(Task 5) → `engineapp.providerService`(Task 5). `rpc.ReasonError`/`MethodErrorFrom`(Task 2)를 Task 3·4·5가 같은 이름으로 쓴다.
- **플레이스홀더:** 없음. 유일한 조건부 지시는 Task 1 Step 6과 Task 5 Step 6의 "실패 메시지가 알려주면 그 참조를 고친다"이며, 계획 작성 시점의 grep 결과(참조 없음)를 함께 적었다.
