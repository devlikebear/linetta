# AI Setup Wizard — Beginner Writer Onboarding

Date: 2026-06-03
Status: Implemented in Settings

## Goal

Linetta should help non-technical writers connect AI without first learning
terms like LLM provider, API key, OAuth, model id, or base URL.

The Settings screen now leads with an **AI 연결 마법사** and keeps raw provider
configuration in **고급 AI 설정** below it.

## Product Policy

Supported beginner paths:

| User has | Linetta path | Reason |
|---|---|---|
| ChatGPT subscription | OpenAI Codex login | Official Codex sign-in path exists for ChatGPT accounts |
| OpenAI API account | OpenAI API key | Direct API billing path |
| Claude / Anthropic API account | Anthropic API key | Claude subscription harness is not supported |
| Gemini API account | Google AI Studio API key | Gemini subscription harness is not supported |

Claude Code CLI remains accepted by the engine for existing-user compatibility,
but it is labeled as a legacy/advanced option and is not part of the beginner
wizard.

## UX Requirements

- Show a beginner-first setup section before raw provider settings.
- Explain in plain Korean that Claude/Gemini subscriptions are not connected
  through Linetta; those providers require API keys.
- Give step-by-step setup instructions for each supported path.
- Provide official links for account, API key, billing, and policy references.
- Let the wizard selection persist the corresponding provider immediately.
- Keep model/API key/base URL controls available in advanced settings.
- Provide a **연결 테스트** button that saves unsaved provider fields and sends
  one tiny request so writers can verify the account before opening the editor.
- Store provider API keys and web-search API keys in macOS Keychain. The
  Settings UI receives presence flags only and never echoes stored secrets back
  into password inputs.
- Default fresh installs to `openai-codex` so the first path matches ChatGPT
  subscription users.

## Verification

- Go settings defaults and RPC settings tests cover `openai-codex` as the fresh
  default.
- Settings Vitest covers:
  - beginner wizard rendering
  - official OpenAI Codex guide link
  - Claude API wizard selection persisting `anthropic`
  - connection test persisting unsaved Claude API drafts before calling
    `providers.test`
  - legacy Claude Code CLI staying outside the default path
  - existing API key/model/base URL flows
  - saved API-key state rendering without echoing the secret
- Engine RPC tests cover `providers.test` resolving the requested provider
  config and surfacing provider-factory errors.
- Engine settings tests cover provider/web-search secret migration, redaction,
  and the absence of secret fields in `settings.json`.
- `make test` covers Go tests, Vitest, TypeScript build, Vite build, engine
  binary build, and Tauri `cargo check`.
