# AI Response Feedback — Design

Date: 2026-06-03
Status: Approved (goal-driven implementation)

## Goal

Make it obvious that the AI is working — surface tool usage and reasoning so the
user thinks "the AI is doing something, I should wait" instead of staring at a
silent or jumpy panel.

- **Companion chat (A):** persistent working state + friendly tool labels + live
  reasoning stream (collapsed by default).
- **Editor ghost-text / AI generation (B):** persistent "generating" indicator
  (no reasoning, no tools — `ai.Runner` uses plain `Chat`).

Out of scope: smoothing the prose stream itself (gemini chunking / React batching).

## Background

- `companion/runner.go` runs a tars `agentloop`. It already emits
  `companion.thinking` on `EventBeforeTool` (`"도구 실행 중: <tool>"`),
  `EventAfterTool` (apply_ops success), and on query rounds.
- tars `agentloop.RunOptions` exposes **`OnReasoningDelta`** (provider-native
  chain-of-thought: anthropic thinking, kimi reasoning_content, openai reasoning
  summary). The companion does not use it today.
- The thinking indicator only appeared when a tool/query fired; pure-prose
  generations had no feedback, tool names were raw, and reasoning was hidden.
- `ai.Runner.run` (ghost-text) calls `client.Chat` directly — no tools, no
  agentloop. It emits `ai.delta/reset/done/error/cancelled`.

## Companion engine (`companion/runner.go`)

- Pass `OnReasoningDelta` to `agentloop.RunOptions` → emit a new
  `companion.reasoning` notification `{run_id, text}`; the frontend accumulates
  it. Reasoning accumulates for the run; the frontend clears it on
  done/error/cancelled.
- Map tool names to friendly Korean labels in the hook:
  `web_search` → "웹 검색 중…", `web_fetch` → "웹 페이지 읽는 중…",
  `linetta_apply_ops` → "작품 설정 반영 중…", others → "도구 실행 중: <name>".
  Implemented as a pure `friendlyToolLabel(name) string` helper (unit-tested).
- No engine-side "생각 중…" placeholder: the baseline working state is a
  frontend fallback so it disappears the moment prose arrives.

New event routing in the Tauri shell (`engine.rs`): `companion.reasoning` →
`companion-reasoning`.

## Companion frontend (`useCompanion` + `CompanionPanel`)

- `useCompanion`: add `reasoning` state; handle `companion-reasoning`
  (append text, guarded by run_id); clear on done/reset(round)/error/cancelled.
- `CompanionPanel` while `status === "streaming"`:
  - **Working indicator** (always): show the current `thinking` label (tool /
    query status); when there is no thinking label and no prose yet, show a
    pulsing "생각 중…" fallback so there is never a silent gap.
  - **Reasoning block** (when `reasoning` non-empty): a dimmed, collapsible
    section headed "추론 중…"; collapsed shows the last line, expands on click.

## Ghost-text / editor AI (`useAIGeneration` + AI surface)

- Show an "AI 생성 중…" indicator while a generation is in flight (status
  running) and before/while text streams — covering the reasoning-model latency
  gap before the first token. Derived from existing run state; no engine change
  required.

## Testing

- Engine: `friendlyToolLabel` mapping; `companion.reasoning` is emitted when the
  loop calls `OnReasoningDelta` (fake client that drives reasoning deltas + an
  ordered notifier).
- Frontend: `useCompanion` accumulates `companion-reasoning` and clears on done;
  `CompanionPanel` renders the working indicator and a collapsible reasoning
  block; ghost-text surface shows the generating indicator while running.
- `make test` green.
