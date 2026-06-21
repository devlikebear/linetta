# AI Onboarding Productization Implementation Plan

> **For agentic workers:** Implement this plan task-by-task. Follow the repo's
> test-first habit: write or update failing tests before implementation, then
> run the listed verification commands before each checkpoint.

**Goal:** Make Linetta's AI setup usable by non-technical webnovel writers. A
writer who hits a missing API key/auth error in the companion should be guided
to a safe connection path, understand cost risk, and retry the same prompt
without seeing provider jargon.

**Background:** See
`docs/superpowers/specs/2026-06-21-ai-onboarding-productization-design.md` for
research and product policy.

**Current architecture:** React/Tauri desktop UI talks to the embedded Go engine
through JSON-RPC. Settings already has an AI wizard, provider config, model
refresh, and `providers.test`. Provider secrets are stored through the engine
secret store. Companion errors currently become ordinary errored assistant
messages in `apps/desktop/src/hooks/useCompanion.ts`.

## Phase 1 — Companion Rescue Card

### Task 1: Classify AI setup failures in the desktop layer

Files:

- Modify: `apps/desktop/src/hooks/useCompanion.ts`
- Modify: `apps/desktop/src/lib/types.ts`
- Test: `apps/desktop/src/hooks/useCompanion.events.test.tsx` or nearest
  existing companion hook test.

Implementation:

- Add `AISetupIssue` type with at least:
  - `missing_key`
  - `auth_required`
  - `model_unavailable`
  - `rate_or_spend_limit`
  - `unknown_provider_error`
- Add a small pure classifier function that maps raw error text to an issue.
  Cover known strings:
  - `api key is required`
  - `auth mode api-key`
  - `401`, `403`, `unauthorized`, `forbidden`
  - `model not found`, `invalid model`
  - `rate limit`, `quota`, `insufficient credits`, `spend limit`
- Preserve the original raw message for advanced details, but do not use it as
  the primary writer-facing text.

Test first:

- Missing-key error becomes `missing_key`.
- Auth/401 error becomes `auth_required`.
- Rate/quota/credits error becomes `rate_or_spend_limit`.
- Unknown errors remain raw enough for support.

Verification:

- `pnpm --dir apps/desktop test -- useCompanion.events.test.tsx --run`

### Task 2: Render a product rescue card instead of raw error bubbles

Files:

- Modify: `apps/desktop/src/components/companion/CompanionPanel.tsx`
- Modify: `apps/desktop/src/components/companion/CompanionPanel.css`
- Modify: `apps/desktop/src/lib/i18n.tsx`
- Test: `apps/desktop/src/components/companion/CompanionPanel.test.tsx`

Implementation:

- Create `CompanionAISetupCard` inside the companion component folder.
- Show it when a bot message is `errored` and has an `AISetupIssue`.
- Actions:
  - `가장 쉬운 방법으로 연결`
  - `구독으로 연결`
  - `API 키 직접 입력`
  - `방금 질문 다시 보내기` when `retryText` exists and setup status is ready.
- Add a details disclosure for the raw error.
- Keep the existing retry behavior for non-setup errors.

Test first:

- Given `missing_key`, the card shows `AI 연결이 필요해요` and does not show raw
  `api key is required` outside details.
- The retry button calls the existing send path with the preserved user prompt.
- Non-setup errors still render as the existing errored bubble.

Verification:

- `pnpm --dir apps/desktop test -- CompanionPanel.test.tsx --run`

---

### Checkpoint: Phase 1

Implementation check:

- [ ] Missing credential/auth/quota/model failures render as a rescue card.
- [ ] Raw provider errors are hidden behind details.
- [ ] The user's blocked prompt is preserved for retry.

Commands:

- [ ] `pnpm --dir apps/desktop test -- useCompanion.events.test.tsx CompanionPanel.test.tsx --run`
- [ ] `git diff --check`

Manual:

- [ ] Use a temp `LINETTA_HOME` with an API-key provider selected but no key.
- [ ] Open a scene, send a companion prompt, confirm the rescue card appears.

Ask the user to confirm the new blocked-state UX before moving to Phase 2.

---

## Phase 2 — Reusable AI Start Modal

### Task 3: Extract Settings wizard into a reusable setup surface

Files:

- Create: `apps/desktop/src/components/ai/AISetupStart.tsx`
- Create: `apps/desktop/src/components/ai/AISetupStart.css`
- Modify: `apps/desktop/src/routes/Settings.tsx`
- Modify: `apps/desktop/src/lib/i18n.tsx`
- Test: `apps/desktop/src/components/ai/AISetupStart.test.tsx`

Implementation:

- Move the beginner choice data currently embedded in `Settings.tsx` into a
  reusable component.
- Support `variant="settings" | "modal"` so Settings can keep its current page
  shape while Companion can open a compact sheet.
- Choices:
  - `가장 쉬운 시작`
  - `구독 활용`
  - `직접 설정`
- The component receives callbacks:
  - `onSelectOpenRouter`
  - `onSelectSubscription`
  - `onSelectDirectKey`
  - `onTestProvider`
- Keep current official links and plain-language policy copy.

Test first:

- Component renders three beginner choices in Korean.
- Selecting direct key calls the direct-key callback.
- Settings still renders its AI wizard after extraction.

Verification:

- `pnpm --dir apps/desktop test -- AISetupStart.test.tsx Settings.test.tsx --run`

### Task 4: Wire the modal from companion rescue state

Files:

- Modify: `apps/desktop/src/components/companion/CompanionPanel.tsx`
- Modify: `apps/desktop/src/lib/rpc.ts` if a provider test helper shape is needed.
- Test: `apps/desktop/src/components/companion/CompanionPanel.test.tsx`

Implementation:

- Add local state for `aiSetupOpen` and selected setup path.
- From rescue-card actions, open `AISetupStart` without leaving the writing
  workspace.
- For existing direct-key providers, allow "설정에서 자세히 열기" as a secondary
  navigation if inline config would be too dense.
- After successful `providers.test`, enable the retry action.

Test first:

- Clicking `가장 쉬운 방법으로 연결` opens the setup modal.
- Successful provider test changes the blocked prompt action to retry-ready.
- Closing the modal returns focus to the companion input.

Verification:

- `pnpm --dir apps/desktop test -- CompanionPanel.test.tsx AISetupStart.test.tsx --run`

---

### Checkpoint: Phase 2

Implementation check:

- [ ] The Settings wizard and companion setup modal share one component/source of
  copy.
- [ ] A blocked writer can start setup without manual Settings navigation.
- [ ] The modal does not require a model ID in the beginner path.

Commands:

- [ ] `pnpm --dir apps/desktop test -- Settings.test.tsx CompanionPanel.test.tsx AISetupStart.test.tsx --run`
- [ ] `pnpm --dir apps/desktop build`
- [ ] `git diff --check`

Manual:

- [ ] Reproduce screenshot scenario and open setup from the companion panel.

Ask the user to confirm whether the copy feels safe enough for non-technical
writers before moving to Phase 3.

---

## Phase 3 — OpenRouter Beginner Provider

### Task 5: Add `openrouter` provider with manual key first

Files:

- Modify: `engine/internal/settings/settings.go`
- Modify: `engine/internal/settings/settings_test.go`
- Modify: `engine/internal/ai/client.go`
- Modify: `engine/internal/modelcatalog/catalog.go`
- Modify: `engine/internal/rpc/handlers/models.go`
- Modify: `apps/desktop/src/lib/types.ts`
- Modify: `apps/desktop/src/routes/Settings.tsx`
- Modify: `apps/desktop/src/lib/i18n.tsx`
- Test: existing provider/settings tests plus Settings tests.

Implementation:

- Add `ProviderOpenRouter = "openrouter"`.
- Resolve it through the OpenAI-compatible API shape:
  - `BaseURL`: `https://openrouter.ai/api/v1`
  - `APIKey`: stored in Keychain through existing provider secret flow.
  - `Model`: default to a curated preset model, not blank if tars needs a
    concrete model.
- Add list-model support through OpenRouter where tars supports it, or add a
  narrow catalog path if needed.
- Add Settings copy that explains OpenRouter credits and key limits.

Test first:

- Settings accepts `openrouter` and rejects unknown providers.
- `Resolve()` returns the OpenRouter base URL/default model.
- `providers.test` passes resolved provider/base URL/API key into the factory.
- Settings renders OpenRouter as the recommended easiest option.

Verification:

- `cd engine && go test ./internal/settings ./internal/modelcatalog ./internal/rpc/handlers`
- `pnpm --dir apps/desktop test -- Settings.test.tsx --run`

### Task 6: Show OpenRouter credit/key state when available

Files:

- Create: `engine/internal/openrouter/keyinfo.go`
- Create: `engine/internal/rpc/handlers/openrouter.go`
- Modify: `engine/internal/engineapp/engineapp.go`
- Modify: `apps/desktop/src/lib/rpc.ts`
- Modify: `apps/desktop/src/lib/types.ts`
- Modify: `apps/desktop/src/components/ai/AISetupStart.tsx`
- Tests alongside.

Implementation:

- Add an RPC such as `openrouter.key_info`.
- Use the stored OpenRouter key to request key/limit state from OpenRouter's key
  info endpoint.
- Surface only safe fields:
  - remaining credits if available
  - credit limit if available
  - rate limit if available
- If the endpoint fails, show a non-blocking "OpenRouter dashboard에서 확인"
  message.

Test first:

- Fake key-info client returns limit/usage and the RPC redacts secrets.
- UI renders `한도 있음` / `남은 크레딧` without exposing the key.

Verification:

- `cd engine && go test ./internal/openrouter ./internal/rpc/handlers`
- `pnpm --dir apps/desktop test -- AISetupStart.test.tsx --run`

---

### Checkpoint: Phase 3

Implementation check:

- [ ] `openrouter` can be selected, tested, and used as a provider.
- [ ] OpenRouter key/credit state is visible when the API allows it.
- [ ] Beginner copy presents OpenRouter as the default safe BYOK path.

Commands:

- [ ] `make test-go`
- [ ] `pnpm --dir apps/desktop test -- Settings.test.tsx AISetupStart.test.tsx CompanionPanel.test.tsx --run`
- [ ] `pnpm --dir apps/desktop build`
- [ ] `git diff --check`

Manual:

- [ ] Use a real low-limit OpenRouter key in a temp profile.
- [ ] Send a short companion request and verify it succeeds.
- [ ] Remove/expire the key and verify the rescue card returns.

Ask the user to confirm whether manual OpenRouter key setup is acceptable for
the next free release, or whether OAuth must be done before launch.

---

## Phase 4 — OpenRouter OAuth PKCE

### Task 7: Implement desktop OAuth connect

Files:

- Create: `engine/internal/openrouter/oauth.go`
- Modify: `engine/internal/rpc/handlers/openrouter.go`
- Modify: `engine/internal/engineapp/engineapp.go`
- Modify: `apps/desktop/src/lib/rpc.ts`
- Modify: `apps/desktop/src/components/ai/AISetupStart.tsx`
- Test alongside.

Implementation:

- Add `openrouter.oauth_start` RPC:
  - Generate PKCE verifier/challenge and state.
  - Start a temporary localhost callback listener or return a URL plus state if
    Tauri side handles callback.
  - Open the browser from the desktop side.
- Add `openrouter.oauth_finish` RPC:
  - Exchange auth code for API key.
  - Store the key via existing provider secret flow.
  - Set active provider to `openrouter`.
  - Immediately run `providers.test`.
- Protect against state mismatch and listener timeout.

Test first:

- PKCE challenge generation is deterministic under injected randomness.
- State mismatch is rejected.
- Exchanged key is stored through the settings secret flow.

Verification:

- `cd engine && go test ./internal/openrouter ./internal/rpc/handlers`
- `pnpm --dir apps/desktop test -- AISetupStart.test.tsx --run`

### Task 8: Polish OAuth and fallback UX

Files:

- Modify: `apps/desktop/src/components/ai/AISetupStart.tsx`
- Modify: `apps/desktop/src/lib/i18n.tsx`
- Test: `AISetupStart.test.tsx`

Implementation:

- Primary button: `OpenRouter로 연결`.
- Fallback link: `키를 직접 붙여넣기`.
- Explain credit limit before opening the browser.
- After OAuth success, show connection success and enable retry.
- If OAuth is cancelled/times out, keep the modal open and offer manual key.

Verification:

- `pnpm --dir apps/desktop test -- AISetupStart.test.tsx --run`
- `pnpm --dir apps/desktop build`
- `git diff --check`

---

### Checkpoint: Phase 4

Implementation check:

- [ ] A user can connect OpenRouter without copying an API key.
- [ ] Cancellation/timeouts are recoverable.
- [ ] The setup flow still works offline enough to direct the user to manual key
  entry.

Commands:

- [ ] `make test`
- [ ] `git diff --check`

Manual:

- [ ] Fresh temp profile -> companion prompt -> rescue card -> OpenRouter OAuth
  -> connection test -> retry same prompt.
- [ ] Confirm the callback listener shuts down after success/failure.

Ask the user to confirm launch readiness before release work.

---

## Phase 5 — Cost Confidence And Release QA

### Task 9: Add local usage and estimated-cost display

Files:

- Modify: `engine/internal/store` AI run query layer if needed.
- Create/modify RPC handler for AI usage summary.
- Modify: `apps/desktop/src/components/ai/AISetupStart.tsx`
- Modify: `apps/desktop/src/routes/Settings.tsx`
- Tests alongside.

Implementation:

- Show local Linetta AI calls today / this month by provider.
- For providers with available pricing metadata, show coarse estimates only.
- For subscription CLI providers, show usage-policy copy instead of dollars.
- Keep the estimate hidden for unknown pricing.

Verification:

- `make test-go`
- `pnpm --dir apps/desktop test -- AISetupStart.test.tsx Settings.test.tsx --run`

### Task 10: Full product smoke

Commands:

- [ ] `make test`
- [ ] `git diff --check`
- [ ] `LINETTA_HOME=/tmp/linetta-ai-onboarding ./scripts/dev.sh`

Manual scenarios:

- [ ] Fresh profile, no provider key: companion shows rescue card.
- [ ] OpenRouter manual or OAuth connection succeeds.
- [ ] Original blocked prompt can be retried.
- [ ] Direct OpenAI/Anthropic/Gemini key users can still use advanced settings.
- [ ] `openai-codex` default remains usable for ChatGPT-plan/Codex-login users.
- [ ] Keychain prompts do not recur unexpectedly during normal setup checks.
- [ ] Mobile/MAS capability filters still hide unavailable CLI providers.

---

### Checkpoint: Phase 5

If all checks pass, the feature is code-complete. For sale/free distribution,
continue with the normal release checklist: version bump, changelog, desktop
build, installed-app smoke, GitHub release, and distribution/tap verification.
