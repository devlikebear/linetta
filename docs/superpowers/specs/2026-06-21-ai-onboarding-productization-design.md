# AI Onboarding Productization — Beginner Writers Can Start AI

Date: 2026-06-21
Status: Proposed (supersedes the earlier OpenRouter-led draft of the same date)

## Goal

Linetta's buyers are aspiring and working webnovel writers, not AI-tool
operators. Many do not know what Claude / Codex / Gemini are, and "bring your
own key" (BYOK) alone makes them give up before they ever generate a line.

A beginner must be able to go from a blocked state — the screenshot where the
companion answers a writing request with the raw error
`api key is required for auth mode api-key` — to a working AI connection in a
few clicks, without (a) understanding model IDs, (b) fearing surprise bills, or
(c) reading English error text.

This spec keeps Linetta's real differentiator intact: a writer can reuse a flat
monthly subscription (ChatGPT Plus via Codex, Claude Pro/Max via Claude Code)
and write with **no per-token cost anxiety**, while still owning their own
credentials (BYOK).

## Research Summary

Refreshed 2026-06-21. Pricing and model names are volatile; implementation
should fetch live data where possible and keep local defaults conservative.

Where the two novel-writing extremes sit, and what each teaches us:

| Product / pattern | What they do | Takeaway for Linetta |
|---|---|---|
| **Sudowrite** | No API key. Bundled AI with monthly credits. Beginner-loved; the only common complaint is "I wish I had more credits." | Simplicity sells, but it requires operating/paying an AI backend. Out of scope for a free release. The lesson to steal is **"no cost fear,"** not the credit economy. |
| **Novelcrafter** | BYOK (OpenAI / Anthropic / OpenRouter / local). Reviews are literally titled "powerful but frustrating to set up"; the recurring complaint is "why am I doing config work at all?" | This is the trap Linetta must avoid: flexibility that strands non-technical writers in provider configuration. |
| **OpenRouter** | One key for many models; optional per-key spend limit; model catalog, `auto` router, and OAuth PKCE app connect. | Three beginner-friendliness techniques worth copying in spirit: (1) hide provider choice, (2) ship a sane explicit default model instead of `auto`, (3) make spend caps visible to kill cost fear. Keep OpenRouter itself as an **advanced** paid option, not the hero. |
| **OpenAI Codex / Claude Code CLI** | Official ChatGPT-plan Codex login and Claude Pro/Max login. Cache local auth; no per-request key paste. | This is Linetta's real edge: subscription reuse = no per-token cost. But login happens **only through the official CLI** (writes `~/.codex/auth.json`, `~/.claude/oauth.json`). Linetta must not build its own OAuth (ToS), so the friction is CLI install + one terminal login — which the wizard must hide as much as possible. |
| **Google AI Studio (Gemini)** | API key issued with one button, **no credit card**, usable free tier. | Lowest-friction "paste one key, start free" path for a non-technical writer. This becomes the default of the key branch. |

Sources:

- Sudowrite vs Novelcrafter comparison: https://ilampadmanabhan.medium.com/sudowrite-vs-novelcrafter-bdc3f33ba95f
- Novelcrafter "frustrating to set up" review: https://ilampadmanabhan.medium.com/novelcrafter-review-64d391c629a2
- Novelcrafter BYOK FAQ: https://www.novelcrafter.com/help/faq/ai-and-prompting/byok
- OpenRouter quickstart: https://openrouter.ai/docs/quickstart
- OpenRouter auth / API keys: https://openrouter.ai/docs/api/reference/authentication
- OpenRouter OAuth PKCE: https://openrouter.ai/docs/guides/overview/auth/oauth
- OpenAI Codex with ChatGPT plan: https://help.openai.com/en/articles/11369540-using-codex-with-your-chatgpt-plan
- OpenAI Codex auth: https://developers.openai.com/codex/auth
- Claude Code authentication: https://code.claude.com/docs/en/authentication
- Claude Code Pro/Max plan support: https://support.claude.com/en/articles/11145838-use-claude-code-with-your-pro-or-max-plan
- Google AI Studio API key: https://ai.google.dev/gemini-api/docs/api-key
- MiniMax pricing (advanced low-cost preset): https://platform.minimax.io/docs/pricing/overview

## Design Decisions (from brainstorming, 2026-06-21)

1. **Hero path = subscription OAuth**, but realized as a **two-branch** first
   screen because subscription login irreducibly needs CLI install + terminal
   login, which can be a bigger wall than pasting a key. Branch by user level:
   "이미 구독 중" vs "키로 빠르게 시작".
2. **Both proactive and reactive onboarding.** A first-run nudge invites
   connection; a rescue card catches the blocked state if they skip it.
3. **Model selection is auto-defaulted** per provider and hidden behind
   "고급 설정." Beginners never see a model ID.
4. **Key-branch default = Google AI Studio free key** (truly $0 start, no card).
5. **Subscription branch automates `codex login`**: the app spawns the official
   CLI login and polls for the credential file. No in-app OAuth (ToS-safe).
6. **No Linetta-hosted credits, no in-app OAuth, no model-list UI surgery.**

## Non-goals

- Self-hosted credit economy (Sudowrite-style). Needs a paid backend, billing,
  and abuse controls; conflicts with a free BYOK release.
- In-app OAuth login for ChatGPT/Claude subscriptions. Extracting a subscription
  token into a custom client violates provider ToS. Login stays in the official
  CLI; Linetta reads the resulting credential file.
- Task-preset model manifest / curated model picker. Replaced by a single sane
  default per provider plus an advanced override.
- Scraping provider dashboards or auto-creating provider accounts.
- Hiding that direct API billing is separate from consumer subscriptions.

## Architecture: three touchpoints, one shared component

```
                    ┌─────────────────────────────┐
   [proactive]──────▶│   AIConnectWizard (new)      │◀──────[reactive]
  first-run tour      │  - two-branch first screen   │   companion raw error
  "connect AI" step   │  - auto-detect/poll status   │   → rescue card + CTA
                      │  - auto-default model        │
                      └─────────────────────────────┘
                                 │
              reuses existing settings / providers RPC
        (providers.test, providers.detect_cli, providers.list_models,
         settings.set) + new codex-login handler
```

`AIConnectWizard` is the single beginner surface. All three touchpoints open it;
nothing reimplements connection logic. It can render as a modal/sheet and is
reused by the Settings AI wizard as well.

Existing anchors to reuse:

- `engine/internal/settings/settings.go` — per-provider config, Keychain-backed
  secrets, fresh default `openai-codex`.
- `engine/internal/clidetect/clidetect.go` — currently detects `claude` only;
  extend to `codex`.
- `engine/internal/rpc/handlers/models.go` — `providers.list_models`,
  `providers.detect_cli`, `providers.test`.
- `apps/desktop/src/routes/Settings.tsx` — provider drafts, model refresh,
  connection test, per-provider setup guides.
- `apps/desktop/src/components/companion/CompanionPanel.tsx` — error bubble
  (`m.errored`) and retry plumbing; the surface to convert into a rescue card.
- `apps/desktop/src/components/onboarding/OnboardingTour.tsx` — spotlight tour to
  host the proactive nudge step.

## UX Requirements

### 1. Wizard first screen — two-branch

One question: "AI를 어떻게 연결할까요?" Two large cards, both leading with the
cost message:

- **이미 구독 중이에요** — ChatGPT Plus·Claude 정액 구독 그대로 사용 → 추가 비용
  0원. Fine print: "한 번만 설치가 필요해요."
- **키로 빠르게 시작** — 무료로 바로 시작 (Google 무료 키) → 설치 없이 복붙 한 번.

A single line below: "잘 모르겠어요 → 키로 빠르게 시작" routes the undecided to
the least-blocking (free) branch to defeat choice paralysis.

### 2. Subscription branch — three-state connector that hides the terminal

On entry, auto-detect current state and render accordingly:

| State | Detection | Screen |
|---|---|---|
| **A. Connected** | `~/.codex/auth.json` (or `~/.claude/oauth.json`) present and valid | ✅ "연결 완료! 바로 쓰세요." Done. |
| **B. CLI present, not logged in** | codex/claude CLI detected, no auth file | **[ChatGPT로 로그인]** → app runs `codex login` → browser opens → app polls for the auth file → auto ✅. |
| **C. CLI missing** | detection fails | "한 번만 설치하면 됩니다." OS-specific install command (one line) + [복사] + [터미널 열기] + [다시 확인] poll. Persistent escape hatch: "대신 키로 시작." |

New work:

- Extend `clidetect` to detect `codex` (same login-shell + known-paths pattern as
  `claude`); generalize `providers.detect_cli` to take a provider/CLI argument.
- New desktop-only RPC to **spawn `codex login` and poll** for the credential
  file with a bounded timeout and a cancel path. Surface progress states
  (실행 중 / 브라우저에서 로그인 대기 / 완료 / 실패) to the wizard.
- Reuse the same three-state frame for Claude subscription (`claude-code-cli`)
  via `claude` login, so both subscriptions share one UI.
- Mobile builds cannot spawn a CLI: states B/C show "데스크톱에서 설정하세요"
  guidance instead of the login button.

Exact `codex login` / `claude` invocation, flags, and auth-file validity checks
are confirmed against the installed CLI version during implementation.

### 3. Key branch — "free, paste once" (default: Google AI Studio free key)

- Default recommendation: **Google AI Studio free key** (`gemini-native`
  provider). No credit card; free tier = genuine $0 start.
- Flow: [Google AI Studio 키 발급 페이지 열기] → paste key → auto connection test
  (`providers.test`) → ✅.
- Collapsed "유료로 더 좋은 모델 쓰고 싶어요" reveals OpenAI / Anthropic /
  OpenRouter / MiniMax (low-cost). For per-token providers, show one-line
  guidance on setting a spend cap (OpenRouter-style) to neutralize cost fear.

### 4. Reactive rescue — companion raw-error replacement

The screenshot bug: `api key is required for auth mode api-key` renders as an
errored bubble (`CompanionPanel.tsx`, `m.errored`).

Fix: classify AI-call failures. **Auth / missing-credential / unconfigured**
errors render a friendly card instead of a raw bubble:

```
┌─────────────────────────────────────────┐
│ ⚙️ AI가 아직 연결되지 않았어요              │
│ 글쓰기 AI를 연결하면 바로 도와드릴 수 있어요.│
│        [ AI 연결하기 ]   [ 자세히 ]        │
└─────────────────────────────────────────┘
```

- **[AI 연결하기]** opens `AIConnectWizard` as a modal. On success, the **blocked
  prompt is auto-retried** with fresh editor context (reuse existing retry path).
- The engine wraps raw provider errors in a **classification enum**
  (auth / network / quota / other) sent to the frontend. The original English
  text is tucked under "자세히." Auth/unconfigured → rescue card; others keep the
  existing friendly errored message.

### 5. Proactive nudge — first-run tour step

Add a step to the existing `OnboardingTour`, shown **only when not connected**:

- Final step spotlights the companion button: "여기서 AI가 도와줘요. 먼저 한 번만
  연결할까요?" with **[연결하기]** opening `AIConnectWizard`.
- Skippable (not a gate) — the rescue card is the safety net.
- Auto-skipped when a credential/key already exists.

### 6. Model auto-default + cost messaging (common)

- Connecting a provider sets a single verified default model automatically
  (codex already defaults to `gpt-5.3-codex-spark`; hardcode one sane default per
  provider for gemini-native / openai / anthropic). The wizard never shows a
  model field.
- Model change lives only in Settings "고급." 
- Cost copy, applied consistently:
  - Subscription: "이미 내는 정액 요금 안에서 추가 비용 없이."
  - Free key: "무료 등급으로 시작, 한도 초과해도 자동 정지."
  - Paid key: "지출 상한을 걸어두면 안전해요."

## Scope

Implementation building blocks and rough size:

| Item | Location | Size |
|---|---|---|
| `AIConnectWizard` component (two-branch, status polling) | desktop frontend (new) | M |
| codex (and claude) CLI detection | `engine/internal/clidetect` | S |
| `codex login` spawn + auth-file poll RPC | engine handler (new) + frontend | M |
| `providers.detect_cli` generalized to take a CLI/provider arg | engine handler | S |
| Error classifier (engine enum → frontend mapping) | `engine/internal/ai` + frontend | S–M |
| Companion rescue card + auto-retry wiring | `CompanionPanel` / companion hook | S |
| Onboarding tour "connect AI" step (conditional) | tour caller | S |
| Per-provider default model + i18n copy (ko/en/ja) | settings + `i18n.tsx` | S |

Suggested sequencing (single release is feasible, but if split):

- First: error classifier + rescue card + key branch (Gemini free) + auto-default
  model. This alone unblocks the screenshot scenario for a non-technical writer.
- Then: subscription branch with `codex login` automation + CLI detection.
- Then: Claude subscription reuse on the same three-state frame; proactive tour
  step polish.

## Risks / open items

- `codex login` is an interactive flow that opens a browser and runs a local
  callback server. Spawning it from the app needs care: process lifetime,
  timeout, user cancel, and detecting completion purely by polling the auth file.
  Validate behavior against the installed CLI before committing the UX copy.
- macOS GUI apps inherit a minimal PATH; CLI detection must consult the login
  shell (already handled for `claude` in `clidetect`).
- Gemini free-tier rate limits and data-use terms differ from paid; copy should
  set expectations without overpromising.
- Sandboxed (MAS) and mobile builds cannot spawn a CLI; subscription branch
  degrades to guidance there.

## Acceptance Criteria

- Missing-credential / auth / unconfigured errors never appear as raw English
  bubbles in the companion panel.
- From the blocked companion state, a fresh user reaches a successful connection
  test without manually navigating to Settings, and the original prompt is
  retried after success.
- The beginner never types a model ID; connecting any provider yields a working
  default model.
- The key branch lets a user start for free via a Google AI Studio key.
- The subscription branch, on desktop with the CLI installed, completes login
  from inside the app (button → `codex login` → poll → connected) without the
  user typing a terminal command.
- Cost framing is present and plain-Korean for each path (subscription vs free
  key vs paid key).
- Existing advanced provider settings remain available and keep storing secrets
  in the Keychain (presence flags only to the UI).
- Live Tauri QA verifies the screenshot scenario end to end: empty scene →
  companion prompt → missing credential → rescue card → connect → retry.
