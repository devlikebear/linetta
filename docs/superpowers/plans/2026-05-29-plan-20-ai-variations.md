# Plan 20 — AI 변형 비교 (Ghost Variations) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** AIPromptBar 에 `변형 ×3` opt-in chip 을 추가해 같은 prompt 로 3개 변형을 병렬 생성하고, ghost text 안에서 ◀▶ 로 전환·비교하며 Tab 으로 하나를 선택해 commit 한다.

**Architecture:** `GhostExtension` 의 state 를 `{ variations: GhostVariation[], currentIdx, mode }` 로 일반화 (단일 모드 = `variations.length === 1` 의 특수 케이스로 backward compat). `useGhostText` 에 `startVariations(args, n)` 메서드 추가 — N 개 `ai.run` RPC 병렬 호출 + runId → variationIdx 매핑. AIPromptBar 의 `변형` chip 상태에 따라 Workspace 가 `start` vs `startVariations` 분기. Engine 변경 없음.

**Tech Stack:** TypeScript / React 18, Tiptap 2 + ProseMirror, Tauri JSONRPC.

---

## 파일 구조

```
apps/desktop/src/
  components/editor/
    GhostExtension.ts    # state 일반화, 5개 새 명령, ArrowLeft/Right, widget 분기
    GhostExtension.css   # +.ai-ghost-indicator, +.ai-ghost-error
  lib/editor/
    useGhostText.ts      # +startVariations, +activeRunIdsRef, +runIdToVariationRef
                         # 이벤트 핸들러에 variation 매핑 분기, cancel/drop/accept 가 모든 runId cancel
  components/ai/
    AIPromptBar.tsx      # +variationsOn state, +`변형 ×3` chip, onRun 시그니처 확장
    AIPromptBar.css      # +.ai-prompt-bar-preset-chip.active (없으면)
  routes/
    Workspace.tsx        # onRun 분기 — variationsOn ? startVariations : start
```

Engine: 변경 없음.

---

## Task 1: GhostExtension — state 일반화 + 5개 명령

**Files:**
- Modify: `apps/desktop/src/components/editor/GhostExtension.ts` (전체 재작성)

이 task는 GhostExtension 의 state schema 를 단일 텍스트에서 variations 배열로 바꾼다. **단일 모드 (N=1) 는 backward compat — Plan 18 동작 보존**. ArrowLeft/Right 키바인딩 + CSS 는 Task 2.

### Step 1: 전체 파일 교체

`apps/desktop/src/components/editor/GhostExtension.ts` 의 전체 내용을 다음으로 교체:

```ts
import { Extension, type RawCommands } from "@tiptap/core";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import { Decoration, DecorationSet } from "@tiptap/pm/view";
import "./GhostExtension.css";

declare module "@tiptap/core" {
  interface Commands<ReturnType> {
    ghost: {
      /**
       * Backward-compat single-variation init. Equivalent to
       * setGhostVariations(1, mode) + setGhostVariationText(0, text).
       */
      setGhostText: (text: string, mode?: GhostMode) => ReturnType;
      /** Plan 20: init N empty variation slots. currentIdx resets to 0. */
      setGhostVariations: (count: number, mode: GhostMode) => ReturnType;
      /** Plan 20: replace text on a specific variation. */
      setGhostVariationText: (idx: number, text: string) => ReturnType;
      /** Plan 20: mark a variation done (with optional error message). */
      setGhostVariationDone: (idx: number, error?: string) => ReturnType;
      /** Plan 20: switch the currently visible variation. Wraps modulo N. No-op when N===1. */
      switchGhostVariation: (direction: -1 | 1) => ReturnType;
      /** Accept the currently visible variation — insert into (or replace) the document. */
      acceptGhostText: () => ReturnType;
      /** Drop the ghost — clear decoration without inserting. */
      dropGhostText: () => ReturnType;
    };
  }
}

export type GhostMode =
  | { kind: "insert"; pos: number }
  | { kind: "replace"; from: number; to: number };

export interface GhostVariation {
  text: string;
  done: boolean;
  /** Optional per-variation error message; if set, also treated as done. */
  error?: string;
}

export interface GhostState {
  mode: GhostMode;
  variations: GhostVariation[];
  currentIdx: number;
}

export const ghostPluginKey = new PluginKey<GhostState | null>("linetta-ghost");

type GhostMeta =
  | { kind: "set"; mode: GhostMode; text: string }
  | { kind: "setVariations"; mode: GhostMode; count: number }
  | { kind: "setVariationText"; idx: number; text: string }
  | { kind: "setVariationDone"; idx: number; error?: string }
  | { kind: "switchVariation"; direction: -1 | 1 }
  | { kind: "drop" }
  | { kind: "done" };

export const GhostExtension = Extension.create({
  name: "linettaGhost",

  addProseMirrorPlugins() {
    return [
      new Plugin<GhostState | null>({
        key: ghostPluginKey,
        state: {
          init: () => null,
          apply(tr, prev) {
            const meta = tr.getMeta(ghostPluginKey) as GhostMeta | undefined;

            if (meta?.kind === "set") {
              // Backward-compat single-variation init.
              return {
                mode: meta.mode,
                variations: [{ text: meta.text, done: false }],
                currentIdx: 0,
              };
            }
            if (meta?.kind === "setVariations") {
              const variations: GhostVariation[] = [];
              for (let i = 0; i < meta.count; i++) {
                variations.push({ text: "", done: false });
              }
              return { mode: meta.mode, variations, currentIdx: 0 };
            }
            if (meta?.kind === "setVariationText" && prev) {
              if (meta.idx < 0 || meta.idx >= prev.variations.length) return prev;
              const next = prev.variations.slice();
              next[meta.idx] = { ...next[meta.idx], text: meta.text };
              return { ...prev, variations: next };
            }
            if (meta?.kind === "setVariationDone" && prev) {
              if (meta.idx < 0 || meta.idx >= prev.variations.length) return prev;
              const next = prev.variations.slice();
              next[meta.idx] = { ...next[meta.idx], done: true, error: meta.error };
              return { ...prev, variations: next };
            }
            if (meta?.kind === "switchVariation" && prev) {
              const n = prev.variations.length;
              if (n <= 1) return prev;
              const nextIdx = ((prev.currentIdx + meta.direction) % n + n) % n;
              return { ...prev, currentIdx: nextIdx };
            }
            if (meta?.kind === "drop") {
              return null;
            }
            if (meta?.kind === "done" && prev) {
              // Legacy single-mode "done" — marks current (only) variation done.
              if (prev.variations.length === 0) return prev;
              const next = prev.variations.slice();
              next[prev.currentIdx] = { ...next[prev.currentIdx], done: true };
              return { ...prev, variations: next };
            }
            // Plan 18 design 2.7: auto-drop on doc edit.
            if (prev && tr.docChanged) {
              return null;
            }
            return prev;
          },
        },
        props: {
          decorations(state) {
            const ghost = this.getState(state);
            if (!ghost) return DecorationSet.empty;
            const pos = ghost.mode.kind === "insert" ? ghost.mode.pos : ghost.mode.to;
            const widget = Decoration.widget(
              pos,
              () => {
                const wrap = document.createElement("span");
                wrap.className = "ai-ghost-wrap";

                const current = ghost.variations[ghost.currentIdx];
                const textSpan = document.createElement("span");
                textSpan.className = "ai-ghost" + (current.done ? " done" : "");
                if (current.error) {
                  textSpan.className += " ai-ghost-error";
                  textSpan.textContent = `(오류: ${current.error})`;
                } else {
                  textSpan.textContent = current.text;
                }
                wrap.appendChild(textSpan);

                if (ghost.variations.length > 1) {
                  const indicator = document.createElement("div");
                  indicator.className = "ai-ghost-indicator";
                  indicator.textContent = `[${ghost.currentIdx + 1}/${ghost.variations.length}] ◀ ▶  Tab 수락`;
                  wrap.appendChild(indicator);
                }
                return wrap;
              },
              { side: 1 },
            );
            return DecorationSet.create(state.doc, [widget]);
          },
        },
      }),
    ];
  },

  addKeyboardShortcuts() {
    return {
      Tab: ({ editor }) => {
        const ghost = ghostPluginKey.getState(editor.state);
        if (!ghost) return false;
        return editor.commands.acceptGhostText();
      },
      Escape: ({ editor }) => {
        const ghost = ghostPluginKey.getState(editor.state);
        if (!ghost) return false;
        return editor.commands.dropGhostText();
      },
    };
  },

  addCommands() {
    return {
      setGhostText:
        (text: string, mode?: GhostMode) =>
        ({ tr, state, dispatch }) => {
          const effectiveMode: GhostMode =
            mode ?? { kind: "insert", pos: state.selection.head };
          if (dispatch) {
            dispatch(
              tr.setMeta(ghostPluginKey, {
                kind: "set",
                mode: effectiveMode,
                text,
              } satisfies GhostMeta),
            );
          }
          return true;
        },
      setGhostVariations:
        (count: number, mode: GhostMode) =>
        ({ tr, dispatch }) => {
          if (count < 1) return false;
          if (dispatch) {
            dispatch(
              tr.setMeta(ghostPluginKey, {
                kind: "setVariations",
                mode,
                count,
              } satisfies GhostMeta),
            );
          }
          return true;
        },
      setGhostVariationText:
        (idx: number, text: string) =>
        ({ tr, dispatch }) => {
          if (dispatch) {
            dispatch(
              tr.setMeta(ghostPluginKey, {
                kind: "setVariationText",
                idx,
                text,
              } satisfies GhostMeta),
            );
          }
          return true;
        },
      setGhostVariationDone:
        (idx: number, error?: string) =>
        ({ tr, dispatch }) => {
          if (dispatch) {
            dispatch(
              tr.setMeta(ghostPluginKey, {
                kind: "setVariationDone",
                idx,
                error,
              } satisfies GhostMeta),
            );
          }
          return true;
        },
      switchGhostVariation:
        (direction: -1 | 1) =>
        ({ tr, dispatch }) => {
          if (dispatch) {
            dispatch(
              tr.setMeta(ghostPluginKey, {
                kind: "switchVariation",
                direction,
              } satisfies GhostMeta),
            );
          }
          return true;
        },
      acceptGhostText:
        () =>
        ({ tr, state, dispatch }) => {
          const ghost = ghostPluginKey.getState(state);
          if (!ghost) return false;
          const current = ghost.variations[ghost.currentIdx];
          if (current.error) return false; // accept on error variation = no-op
          if (dispatch) {
            // Force plain-text commit (no mark inheritance from surrounding
            // selection). schema.text(text) constructs a text node with no marks;
            // replaceWith replaces the target range with that node.
            const node = state.schema.text(current.text);
            let nextTr;
            if (ghost.mode.kind === "insert") {
              nextTr = tr.replaceWith(ghost.mode.pos, ghost.mode.pos, node);
            } else {
              nextTr = tr.replaceWith(ghost.mode.from, ghost.mode.to, node);
            }
            nextTr.setMeta(ghostPluginKey, { kind: "drop" } satisfies GhostMeta);
            dispatch(nextTr);
          }
          return true;
        },
      dropGhostText:
        () =>
        ({ tr, state, dispatch }) => {
          const ghost = ghostPluginKey.getState(state);
          if (!ghost) return false;
          if (dispatch) {
            dispatch(tr.setMeta(ghostPluginKey, { kind: "drop" } satisfies GhostMeta));
          }
          return true;
        },
    } as Partial<RawCommands>;
  },
});

/** Read-only: is a ghost currently active? Used by useGhostText / AIPromptBar. */
export function hasActiveGhost(editor: { state?: any; view?: { state: any } } | null | undefined): boolean {
  if (!editor) return false;
  const state = editor.state ?? editor.view?.state;
  if (!state) return false;
  return ghostPluginKey.getState(state) !== null;
}
```

핵심 변경 요약:
- `GhostState` 가 `{mode, variations, currentIdx}` 로 일반화.
- 5개 새 명령: `setGhostVariations`, `setGhostVariationText`, `setGhostVariationDone`, `switchGhostVariation` + 기존 `setGhostText`/`acceptGhostText`/`dropGhostText` 유지.
- `acceptGhostText` 가 `variations[currentIdx]` 사용. error variation 이면 no-op.
- Widget DOM: `.ai-ghost-wrap` 안에 text span + (N>1 일 때) indicator div. error 면 회색 `(오류: ...)`.
- `done` legacy meta 는 backward compat 유지 (변형 모드는 setVariationDone 사용).

### Step 2: 타입체크

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop && npx tsc --noEmit
```

기대: 에러 없음.

### Step 3: 커밋

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/src/components/editor/GhostExtension.ts
git commit -m "feat(editor): GhostExtension state generalized to variations array"
```

---

## Task 2: GhostExtension — ArrowLeft/Right 키바인딩 + CSS

**Files:**
- Modify: `apps/desktop/src/components/editor/GhostExtension.ts` (`addKeyboardShortcuts` 확장)
- Modify: `apps/desktop/src/components/editor/GhostExtension.css` (인디케이터 + 에러 스타일)

### Step 1: addKeyboardShortcuts 확장

`apps/desktop/src/components/editor/GhostExtension.ts` 의 `addKeyboardShortcuts()` 메서드를 다음으로 교체:

```ts
addKeyboardShortcuts() {
  return {
    Tab: ({ editor }) => {
      const ghost = ghostPluginKey.getState(editor.state);
      if (!ghost) return false;
      return editor.commands.acceptGhostText();
    },
    Escape: ({ editor }) => {
      const ghost = ghostPluginKey.getState(editor.state);
      if (!ghost) return false;
      return editor.commands.dropGhostText();
    },
    ArrowLeft: ({ editor }) => {
      const ghost = ghostPluginKey.getState(editor.state);
      if (!ghost || ghost.variations.length <= 1) return false;
      return editor.commands.switchGhostVariation(-1);
    },
    ArrowRight: ({ editor }) => {
      const ghost = ghostPluginKey.getState(editor.state);
      if (!ghost || ghost.variations.length <= 1) return false;
      return editor.commands.switchGhostVariation(1);
    },
  };
},
```

### Step 2: CSS 추가

`apps/desktop/src/components/editor/GhostExtension.css` 끝에 추가:

```css
.ai-ghost-wrap {
  display: inline-block;
}

.ai-ghost-indicator {
  display: block;
  margin-top: 0.2rem;
  font-size: 0.75rem;
  opacity: 0.55;
  font-style: normal;
  user-select: none;
  pointer-events: none;
}

.ai-ghost-error {
  color: #e07a7a;
  font-style: normal;
  opacity: 0.7;
}
```

### Step 3: 타입체크

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop && npx tsc --noEmit
```

기대: 에러 없음.

### Step 4: 커밋

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/src/components/editor/GhostExtension.ts apps/desktop/src/components/editor/GhostExtension.css
git commit -m "feat(editor): GhostExtension ArrowLeft/Right + indicator/error styles"
```

---

## Task 3: useGhostText — startVariations + 멀티 run 추적 + 이벤트 분기

**Files:**
- Modify: `apps/desktop/src/lib/editor/useGhostText.ts` (전체 재작성)

이 task는 useGhostText 에 `startVariations` 메서드를 추가하고, 모든 이벤트 핸들러 (delta/reset/done/error/cancelled) 에 "variation 매핑 우선 분기" 를 넣는다. 기존 `start` (단일 모드) 동작 보존.

### Step 1: 전체 파일 교체

`apps/desktop/src/lib/editor/useGhostText.ts` 의 전체 내용을 다음으로 교체:

```ts
import { useCallback, useEffect, useRef, useState } from "react";
import type { Editor } from "@tiptap/react";
import { ai as aiApi } from "../rpc";
import type { AICancelled, AIDelta, AIDone, AIError, AIOptions, AIReset } from "../types";
import { useEngineEvent } from "../../hooks/useEngineEvent";
import { ghostPluginKey, type GhostMode } from "../../components/editor/GhostExtension";

export type GhostStatus =
  | { kind: "idle" }
  | { kind: "running"; runId: string; text: string }
  | { kind: "done"; text: string }
  | { kind: "error"; message: string };

interface RunArgs {
  nodeId: string;
  prompt: string;
  options: AIOptions;
  selectionText?: string;
  /** When provided, ghost is committed by replacing this range instead of inserting at the head. */
  replaceRange?: { from: number; to: number };
}

/**
 * useGhostText wires ai.run RPC + ai-delta/done/error/reset/cancelled
 * notifications to a Tiptap editor's GhostExtension commands.
 *
 * Single-mode: start() — one run, auto-commit on done.
 * Variation-mode: startVariations(args, n) — N parallel runs, user picks via ◀▶+Tab.
 */
export function useGhostText(editor: Editor | null) {
  const [status, setStatus] = useState<GhostStatus>({ kind: "idle" });
  // Single-mode active run.
  const runIdRef = useRef<string | null>(null);
  const accumulatedRef = useRef<string>("");
  // Variation-mode active runs.
  const activeRunIdsRef = useRef<string[]>([]);
  const runIdToVariationRef = useRef<Map<string, number>>(new Map());

  // Helper: cancel every in-flight run (single + variations).
  const cancelAllInFlight = useCallback(() => {
    for (const id of activeRunIdsRef.current) {
      aiApi.cancel(id).catch(() => {});
    }
    if (runIdRef.current) {
      aiApi.cancel(runIdRef.current).catch(() => {});
    }
    activeRunIdsRef.current = [];
    runIdToVariationRef.current.clear();
    runIdRef.current = null;
  }, []);

  const start = useCallback(
    async ({ nodeId, prompt, options, selectionText = "", replaceRange }: RunArgs) => {
      if (!editor) return;
      cancelAllInFlight();
      editor.commands.dropGhostText();
      accumulatedRef.current = "";
      try {
        const { run_id } = await aiApi.run(nodeId, prompt, options, selectionText);
        runIdRef.current = run_id;
        setStatus({ kind: "running", runId: run_id, text: "" });
        const mode: GhostMode = replaceRange
          ? { kind: "replace", from: replaceRange.from, to: replaceRange.to }
          : { kind: "insert", pos: editor.state.selection.head };
        editor.commands.setGhostText("", mode);
      } catch (e) {
        setStatus({ kind: "error", message: String(e) });
      }
    },
    [editor, cancelAllInFlight],
  );

  const startVariations = useCallback(
    async (
      { nodeId, prompt, options, selectionText = "", replaceRange }: RunArgs,
      n: number,
    ) => {
      if (!editor) return;
      cancelAllInFlight();
      editor.commands.dropGhostText();

      const mode: GhostMode = replaceRange
        ? { kind: "replace", from: replaceRange.from, to: replaceRange.to }
        : { kind: "insert", pos: editor.state.selection.head };
      editor.commands.setGhostVariations(n, mode);
      setStatus({ kind: "running", runId: "(variations)", text: "" });

      for (let i = 0; i < n; i++) {
        const idx = i;
        aiApi
          .run(nodeId, prompt, options, selectionText)
          .then(({ run_id }) => {
            activeRunIdsRef.current.push(run_id);
            runIdToVariationRef.current.set(run_id, idx);
          })
          .catch((e) => {
            editor.commands.setGhostVariationDone(idx, String(e));
          });
      }
    },
    [editor, cancelAllInFlight],
  );

  const cancel = useCallback(async () => {
    cancelAllInFlight();
    if (editor) editor.commands.dropGhostText();
    setStatus({ kind: "idle" });
  }, [editor, cancelAllInFlight]);

  const accept = useCallback(() => {
    if (!editor) return;
    // Cancel any remaining in-flight runs (token saving) before commit.
    cancelAllInFlight();
    editor.commands.acceptGhostText();
    accumulatedRef.current = "";
    setStatus({ kind: "idle" });
  }, [editor, cancelAllInFlight]);

  const drop = useCallback(() => {
    if (!editor) return;
    cancelAllInFlight();
    editor.commands.dropGhostText();
    accumulatedRef.current = "";
    setStatus({ kind: "idle" });
  }, [editor, cancelAllInFlight]);

  useEngineEvent<AIDelta>("ai-delta", (p) => {
    if (!editor) return;
    // Variation-mode first.
    const vIdx = runIdToVariationRef.current.get(p.run_id);
    if (vIdx !== undefined) {
      const existing = ghostPluginKey.getState(editor.state);
      const current = existing?.variations[vIdx]?.text ?? "";
      editor.commands.setGhostVariationText(vIdx, current + p.text);
      return;
    }
    // Single-mode fallback.
    if (p.run_id !== runIdRef.current) return;
    accumulatedRef.current += p.text;
    const existing = ghostPluginKey.getState(editor.state);
    editor.commands.setGhostText(accumulatedRef.current, existing?.mode);
    setStatus({ kind: "running", runId: p.run_id, text: accumulatedRef.current });
  });

  useEngineEvent<AIReset>("ai-reset", (p) => {
    if (!editor) return;
    const vIdx = runIdToVariationRef.current.get(p.run_id);
    if (vIdx !== undefined) {
      editor.commands.setGhostVariationText(vIdx, p.text);
      return;
    }
    if (p.run_id !== runIdRef.current) return;
    accumulatedRef.current = p.text;
    const existing = ghostPluginKey.getState(editor.state);
    editor.commands.setGhostText(p.text, existing?.mode);
    setStatus({ kind: "running", runId: p.run_id, text: p.text });
  });

  useEngineEvent<AIDone>("ai-done", (p) => {
    if (!editor) return;
    const vIdx = runIdToVariationRef.current.get(p.run_id);
    if (vIdx !== undefined) {
      // Variation-mode: mark this variation done, do NOT auto-commit.
      editor.commands.setGhostVariationText(vIdx, p.full_text);
      editor.commands.setGhostVariationDone(vIdx);
      // Remove from active runs (already done).
      activeRunIdsRef.current = activeRunIdsRef.current.filter((id) => id !== p.run_id);
      return;
    }
    // Single-mode: auto-commit (Plan 18 fixup).
    if (p.run_id !== runIdRef.current) return;
    const existing = ghostPluginKey.getState(editor.state);
    editor.commands.setGhostText(p.full_text, existing?.mode);
    editor.commands.acceptGhostText();
    runIdRef.current = null;
    accumulatedRef.current = "";
    setStatus({ kind: "done", text: p.full_text });
  });

  useEngineEvent<AIError>("ai-error", (p) => {
    if (!editor) return;
    const vIdx = runIdToVariationRef.current.get(p.run_id);
    if (vIdx !== undefined) {
      editor.commands.setGhostVariationDone(vIdx, p.message);
      activeRunIdsRef.current = activeRunIdsRef.current.filter((id) => id !== p.run_id);
      return;
    }
    if (p.run_id !== runIdRef.current) return;
    editor.commands.dropGhostText();
    runIdRef.current = null;
    setStatus({ kind: "error", message: p.message });
  });

  useEngineEvent<AICancelled>("ai-cancelled", (p) => {
    if (!editor) return;
    const vIdx = runIdToVariationRef.current.get(p.run_id);
    if (vIdx !== undefined) {
      // Just clean up the mapping; do NOT touch ghost decoration (might be other variations alive or already accepted).
      runIdToVariationRef.current.delete(p.run_id);
      activeRunIdsRef.current = activeRunIdsRef.current.filter((id) => id !== p.run_id);
      return;
    }
    if (p.run_id !== runIdRef.current) return;
    runIdRef.current = null;
    editor.commands.dropGhostText();
    setStatus({ kind: "idle" });
  });

  // Cleanup on editor change/unmount — cancel any in-flight runs.
  useEffect(() => {
    return () => {
      cancelAllInFlight();
    };
  }, [editor, cancelAllInFlight]);

  return { status, start, startVariations, cancel, accept, drop };
}
```

핵심 변경:
- 새 refs: `activeRunIdsRef`, `runIdToVariationRef`.
- 새 helper: `cancelAllInFlight` — 단일 + 변형 모두 cancel + ref reset.
- 새 메서드: `startVariations(args, n)`.
- `cancel` / `accept` / `drop` 모두 `cancelAllInFlight` 사용.
- 이벤트 핸들러 (delta/reset/done/error/cancelled) 모두 **variation 매핑 우선 분기** 후 단일 모드 fallback.
- 변형 모드 ai-done: 자동 commit X, `setGhostVariationDone` 만.
- 변형 모드 ai-error: 그 variation 만 error 마킹, 나머지 진행.

### Step 2: 타입체크

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop && npx tsc --noEmit
```

기대: 에러 없음.

### Step 3: 커밋

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/src/lib/editor/useGhostText.ts
git commit -m "feat(editor): useGhostText — startVariations + multi-run event branching"
```

---

## Task 4: AIPromptBar — 변형 chip + onRun 시그니처 확장

**Files:**
- Modify: `apps/desktop/src/components/ai/AIPromptBar.tsx`
- Modify: `apps/desktop/src/components/ai/AIPromptBar.css`

### Step 1: AIPromptBar 컴포넌트 수정

`apps/desktop/src/components/ai/AIPromptBar.tsx` 의 `Props` interface 와 component 본문, `submit` 함수를 다음으로 갱신.

`Props` interface 의 `onRun` 시그니처 변경:

```ts
interface Props {
  anchor: { top: number; left: number } | null;
  hasSelection: boolean;
  busy: boolean;
  options: AIOptions;
  contextItemCount: number;
  errorMessage?: string;
  onOptionsChange: (o: AIOptions) => void;
  onRun: (preset: PresetID, prompt: string, variationsOn: boolean) => void;
  onCancel: () => void;
  onClose: () => void;
  onContextClick: () => void;
}
```

컴포넌트 본문 안에 새 state 추가 (`prompt`/`shake` state 옆):

```tsx
const [variationsOn, setVariationsOn] = useState(false);
```

`submit` 함수의 onRun 호출에 `variationsOn` 전달:

```tsx
const submit = (preset: PresetID) => {
  const seed = preset ? PRESET_SEED[preset] : "";
  const text = preset ? seed : prompt.trim();
  if (!text) {
    setShake(true);
    setTimeout(() => setShake(false), 350);
    textareaRef.current?.focus();
    return;
  }
  if (preset && !prompt) setPrompt(seed);
  onRun(preset, text, variationsOn);
};
```

톤·길이 chip row 안에 `LengthChip` 다음에 새 chip 추가:

```tsx
<button
  type="button"
  className={`ai-prompt-bar-preset-chip${variationsOn ? " active" : ""}`}
  onClick={() => setVariationsOn((v) => !v)}
  aria-pressed={variationsOn}
  title="3개 변형 병렬 생성 (토큰 3배)"
>
  변형 ×3
</button>
```

(`<LengthChip>` 이 있는 줄 직후. 기존 JSX 구조 유지.)

### Step 2: CSS active 상태 추가

`apps/desktop/src/components/ai/AIPromptBar.css` 끝에 다음 추가 (기존에 `.active` 가 없는 경우):

```css
.ai-prompt-bar-preset-chip.active {
  background: rgba(255, 255, 255, 0.18);
  border-color: rgba(255, 255, 255, 0.25);
}
```

(이미 active 스타일이 있다면 step skip 후 step 3 으로.)

### Step 3: 타입체크

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop && npx tsc --noEmit
```

기대: 에러 발생 — Workspace.tsx 의 `onRun={(preset, promptText) => ...}` 가 2-인자 콜백이라 새 시그니처와 맞지 않음. **Task 5 에서 Workspace.tsx 갱신 후 통과 예정.**

만약 다른 unrelated 에러가 보이면 보고. 그렇지 않으면 의도된 실패로 진행.

### Step 4: 커밋

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/src/components/ai/AIPromptBar.tsx apps/desktop/src/components/ai/AIPromptBar.css
git commit -m "feat(ai): AIPromptBar — '변형 ×3' opt-in chip"
```

---

## Task 5: Workspace — onRun 분기 + 수동 스모크 + tag

**Files:**
- Modify: `apps/desktop/src/routes/Workspace.tsx`

### Step 1: onRun 콜백 갱신

`apps/desktop/src/routes/Workspace.tsx` 안의 `<AIPromptBar ... onRun={...} />` 를 찾아 콜백을 다음으로 교체:

```tsx
onRun={(preset, promptText, variationsOn) => {
  const isReplacePreset = preset === "rewrite" || preset === "compact";
  const hasSel = !!tiptapEditor && !tiptapEditor.state.selection.empty;
  const selectionText = hasSel
    ? tiptapEditor!.state.doc.textBetween(
        tiptapEditor!.state.selection.from,
        tiptapEditor!.state.selection.to,
        "\n",
      )
    : "";
  const replaceRange = isReplacePreset && hasSel
    ? { from: tiptapEditor!.state.selection.from, to: tiptapEditor!.state.selection.to }
    : undefined;
  const args = {
    nodeId: load.node.id,
    prompt: promptText,
    options: aiOptions,
    selectionText,
    replaceRange,
  };
  if (variationsOn) {
    ghost.startVariations(args, 3);
  } else {
    ghost.start(args);
  }
}}
```

(기존 onRun 의 selection/replaceRange 추출 로직은 그대로 보존. 마지막 한 분기만 추가.)

### Step 2: 타입체크 + 엔진 빌드 회귀

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop && npx tsc --noEmit
cd /Users/changheonshin/workspace/myworks/linetta/engine && go test ./...
```

기대: 타입체크 clean. Engine 테스트 모두 PASS (이 plan 은 engine 변경 없음).

### Step 3: 커밋

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/src/routes/Workspace.tsx
git commit -m "feat(workspace): onRun branches to startVariations when chip on"
```

### Step 4: 엔진 빌드 (회귀 확인)

```bash
cd /Users/changheonshin/workspace/myworks/linetta && ./scripts/build-engine.sh
```

기대: 빌드 성공.

### Step 5: 수동 스모크 시나리오 (controller 가 직접 실행)

```bash
rm -rf /tmp/linetta-plan20 && LINETTA_HOME=/tmp/linetta-plan20 ./scripts/dev.sh
```

**Plan 18 회귀 (단일 모드):**

1. 작품/씬 생성 후 본문 입력. `Cmd+I` → 변형 chip OFF 상태 확인. 자유 prompt + Enter → 단일 ghost streaming → ai-done 자동 commit + bar 닫기. **Plan 18 동작 그대로**.
2. 선택 영역 + chip OFF + `재작성` → 선택 영역 plain text replace.
3. chip OFF + 스트리밍 중 Esc → ghost drop + RPC cancel.

**변형 모드 신규:**

4. `Cmd+I` → `변형 ×3` chip 클릭 (active 시각화 확인) → 자유 prompt + Enter → ghost 안 인디케이터 `[1/3] ◀ ▶  Tab 수락` 표시. 3개 variation 병렬 stream 시작.
5. ▶ 키로 variation 2 로 → `[2/3]` 표시 + variation 2 의 현재 누적 텍스트. ◀ 로 다시 variation 1.
6. 모든 variation done 후 Tab → 현재 보고 있는 variation 의 text 가 plain 으로 doc 에 commit + bar 닫힘.
7. variation 1 done, 2/3 still streaming 상태에서 Tab → variation 1 commit + 2/3 cancel RPC (engine 로그 확인).
8. 선택 영역 + chip ON + `재작성` preset → 3 variations 모두 선택 영역 replace 후보. Tab → 현재 variation 으로 replace.

**에러 처리:**

9. 임의로 engine 끄고 변형 모드 생성 시도 → 3 variations 모두 error 표시. ◀▶ 로 셋 다 `(오류: ...)` 회색 확인.

**Cancel:**

10. 3 streaming 중 Esc → engine 로그에서 3 cancel RPC. ghost drop.

**Cmd+P 회귀:**

11. `Cmd+P` → 명령 팔레트 정상 (Plan 18). ArrowLeft/Right 가 ghost 비활성일 때 에디터 커서 이동 정상 (Plan 20 키바인딩 가드 검증).

**Plan 19 회귀:**

12. ctx 칩 클릭 → AIContextChecklist popover 실제 카운트 정상 표시.

### Step 6: tag

스모크 통과 시:

```bash
git tag plan-20-ai-variations-done
```

---

## Self-Review

**1. Spec 커버리지:**

| Spec 요구 | Task |
|---|---|
| GhostState 일반화 (variations 배열) | Task 1 |
| 5개 새 명령 | Task 1 |
| widget 안 indicator + error 표시 | Task 1 (DOM) + Task 2 (CSS) |
| ArrowLeft/Right 키바인딩 (N>1 가드) | Task 2 |
| useGhostText startVariations | Task 3 |
| runId→variationIdx 매핑 | Task 3 |
| 이벤트 핸들러 variation 분기 | Task 3 |
| 변형 모드 ai-done 자동 commit 없음 | Task 3 |
| 부분 에러 처리 (variation 별) | Task 3 |
| cancel/drop 모든 runId cancel | Task 3 |
| accept 시 남은 in-flight cancel | Task 3 |
| AIPromptBar `변형 ×3` chip | Task 4 |
| onRun 시그니처 확장 | Task 4 (declare) + Task 5 (wire) |
| Workspace 분기 | Task 5 |
| 단일 모드 회귀 보존 | Task 1 (backward compat) + 스모크 #1-3 |
| 수동 스모크 12 시나리오 | Task 5 Step 5 |

모든 spec 요구 매핑.

**2. Placeholder scan:**
- 모든 task 가 실제 코드/명령/기대 출력 포함.
- Task 3 Step 1 의 cancelAllInFlight 헬퍼는 helper 정의 후 사용 — placeholder 아님.
- Task 4 의 "이미 active 스타일이 있다면 step skip" 은 conditional, placeholder 아님.

**3. Type 일관성:**
- `GhostVariation` (Task 1) → `setGhostVariationText` (Task 1) → `runIdToVariationRef` 사용 (Task 3) — 모두 일치.
- `GhostMode` 타입 (Task 1) → `setGhostVariations(count, mode)` (Task 1, 3, 5) — 일치.
- `PresetID` (Plan 18 AIPromptBar) → Workspace onRun 콜백 (Task 5) — 일치.
- 새 `onRun(preset, prompt, variationsOn)` 시그니처: Task 4 (선언) → Task 5 (호출처) — 일치.
- `cancelAllInFlight` 명칭: Task 3 내부에서만 사용 — 정의-호출 일관.
- `setGhostVariationDone` 시그니처 `(idx, error?)`: Task 1 (정의), Task 3 (호출 with error 또는 without) — 모두 일치.

체크 통과.

**4. 위험 영역 / 명시적 한계:**
- Task 1 의 state 일반화로 Plan 18 단일 모드 회귀 위험 — 스모크 #1-3 이 핵심 게이트.
- Task 3 의 `accept` 가 남은 in-flight cancel — 사용자가 Tab 누르면 즉시 다른 variations 죽음. 의도된 토큰 절약. 만약 사용자가 후회하면 Cmd+Z 로 commit 자체는 되돌릴 수 있지만 cancel 된 variations 는 못 살림. 알려진 트레이드오프.
- doc 변경 시 RPC leak 한계 — Plan 18 부터 이어지는 documented limitation. Plan 20 에서 새로 도입된 문제 아님.
