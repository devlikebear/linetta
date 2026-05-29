# Plan 21 — AI 모달 재설계 (Ghost → Modal) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 인라인 ghost-text AI UX 를 중앙 모달로 대체한다. 모달이 입력→생성→비교→선택 전체를 소유하고, 그동안 에디터는 잠긴다(`setEditable(false)` + 백드롭). 모드는 결과 배치(대체/삽입/전체교체)로 재정의하고, 결과는 모달 안 솔리드 카드로 또렷이 표시한다.

**Architecture:** GhostExtension(decoration plugin) / useGhostText / AIPromptBar 를 삭제하고, `commitGenerated` 순수 헬퍼 + `useAIGeneration` 훅(React-state 변형 보유) + `AIModal` 컴포넌트(focus-trap 오버레이)로 대체한다. Cmd+I 가 selection 을 캡처하고 에디터를 잠근 뒤 모달을 연다. 수락 시 선택된 변형 텍스트를 모드별로 plain-text commit. 엔진 변경 없음.

**Tech Stack:** TypeScript / React 18, Tiptap 2 + ProseMirror, Tauri JSONRPC.

---

## 파일 구조

**삭제:**
- `apps/desktop/src/components/editor/GhostExtension.ts`
- `apps/desktop/src/components/editor/GhostExtension.css`
- `apps/desktop/src/lib/editor/useGhostText.ts`
- `apps/desktop/src/components/ai/AIPromptBar.tsx`
- `apps/desktop/src/components/ai/AIPromptBar.css`

**신규:**
- `apps/desktop/src/lib/editor/commitGenerated.ts` — 모드별 plain-text commit 헬퍼 (+ 단위 테스트)
- `apps/desktop/src/lib/editor/commitGenerated.test.ts` — textToParagraphs 단위 테스트 (vitest 없으므로 순수 함수만 분리해 node 실행 또는 타입체크로 검증; 아래 Task 1 참조)
- `apps/desktop/src/lib/editor/useAIGeneration.ts` — variations React-state 보유 훅
- `apps/desktop/src/components/ai/AIModal.tsx` — 중앙 모달
- `apps/desktop/src/components/ai/AIModal.css`

**수정:**
- `apps/desktop/src/routes/Workspace.tsx` — Cmd+I → selection 캡처 + setEditable(false) + 모달, 수락 → commit + setEditable(true), GhostExtension/useGhostText/AIPromptBar 제거
- `apps/desktop/src/components/ai/AIContextChecklist.tsx` — 유지 (변경 없음)

엔진: 변경 없음.

**FE 테스트 인프라:** vitest/jest 미설치 (Plan 17~20 동일). 순수 함수(`textToParagraphs`)는 타입체크 + 인라인 assertion 스크립트로, 나머지는 타입체크 + 수동 스모크.

---

## Task 1: commitGenerated 헬퍼 (모드별 plain-text commit)

**Files:**
- Create: `apps/desktop/src/lib/editor/commitGenerated.ts`

이 task는 생성 텍스트를 에디터에 plain-text 로 넣는 순수 로직 + commit 함수를 만든다. `textToParagraphs` 는 순수 함수라 별도 검증 가능.

### Step 1: 파일 작성

`apps/desktop/src/lib/editor/commitGenerated.ts` 생성:

```ts
import type { Editor } from "@tiptap/react";

export type CommitMode = "replace" | "insert" | "replaceAll";

export interface CommitTarget {
  /** Frozen selection captured at Cmd+I time. */
  from: number;
  to: number;
}

/** A ProseMirror paragraph node holding plain inline content (text + hardBreaks). */
interface ParagraphNode {
  type: "paragraph";
  content?: InlineNode[];
}
type InlineNode = { type: "text"; text: string } | { type: "hardBreak" };

/**
 * Convert plain text into an array of ProseMirror paragraph nodes.
 * - Blank lines (\n\n+) separate paragraphs.
 * - Single newlines within a paragraph become hardBreak nodes.
 * - No marks are applied (plain text commit).
 * An empty paragraph (no content) is represented as { type: "paragraph" }.
 */
export function textToParagraphs(text: string): ParagraphNode[] {
  const normalized = text.replace(/\r\n/g, "\n").replace(/\r/g, "\n");
  const blocks = normalized.split(/\n{2,}/);
  const paragraphs: ParagraphNode[] = [];
  for (const block of blocks) {
    const lines = block.split("\n");
    const content: InlineNode[] = [];
    lines.forEach((line, i) => {
      if (i > 0) content.push({ type: "hardBreak" });
      if (line.length > 0) content.push({ type: "text", text: line });
    });
    paragraphs.push(content.length > 0 ? { type: "paragraph", content } : { type: "paragraph" });
  }
  if (paragraphs.length === 0) paragraphs.push({ type: "paragraph" });
  return paragraphs;
}

/**
 * Commit generated text into the editor as plain text (no mark inheritance).
 * - replaceAll: replace the entire document with the paragraphs (single undo step).
 * - replace: replace [target.from, target.to] with the paragraphs.
 * - insert: insert the paragraphs at target.from.
 */
export function commitGenerated(
  editor: Editor,
  mode: CommitMode,
  target: CommitTarget,
  text: string,
): void {
  const paragraphs = textToParagraphs(text);
  if (mode === "replaceAll") {
    editor.chain().setContent({ type: "doc", content: paragraphs }).run();
    return;
  }
  if (mode === "replace") {
    editor.chain().insertContentAt({ from: target.from, to: target.to }, paragraphs).run();
    return;
  }
  // insert
  editor.chain().insertContentAt(target.from, paragraphs).run();
}
```

### Step 2: 순수 함수 검증 (인라인 스크립트)

`textToParagraphs` 만 따로 검증. 프로젝트에 vitest 가 없으므로 `npx tsx` 로 즉석 검증 (tsx 가 없으면 node + 임시 ts→js 컴파일 대신, 아래 인라인 assert 를 임시 파일로 만들어 `npx tsx` 시도; 실패 시 타입체크만):

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop
cat > /tmp/t2p.test.mjs <<'EOF'
// Manual mirror of textToParagraphs logic for a sanity check.
function textToParagraphs(text) {
  const normalized = text.replace(/\r\n/g, "\n").replace(/\r/g, "\n");
  const blocks = normalized.split(/\n{2,}/);
  const paragraphs = [];
  for (const block of blocks) {
    const lines = block.split("\n");
    const content = [];
    lines.forEach((line, i) => {
      if (i > 0) content.push({ type: "hardBreak" });
      if (line.length > 0) content.push({ type: "text", text: line });
    });
    paragraphs.push(content.length > 0 ? { type: "paragraph", content } : { type: "paragraph" });
  }
  if (paragraphs.length === 0) paragraphs.push({ type: "paragraph" });
  return paragraphs;
}
import assert from "node:assert";
// 1) two paragraphs split on blank line
let r = textToParagraphs("문단1\n\n문단2");
assert.strictEqual(r.length, 2, "two paragraphs");
assert.strictEqual(r[0].content[0].text, "문단1");
// 2) single newline → hardBreak inside one paragraph
r = textToParagraphs("줄1\n줄2");
assert.strictEqual(r.length, 1, "one paragraph");
assert.strictEqual(r[0].content[1].type, "hardBreak");
assert.strictEqual(r[0].content[2].text, "줄2");
// 3) empty input → single empty paragraph
r = textToParagraphs("");
assert.strictEqual(r.length, 1);
assert.deepStrictEqual(r[0], { type: "paragraph" });
console.log("textToParagraphs OK");
EOF
node /tmp/t2p.test.mjs
```

기대: `textToParagraphs OK`. (이 스크립트는 로직 동등성 sanity check — 커밋 안 함, 검증용.)

### Step 3: 타입체크

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop && npx tsc --noEmit
```

기대: 에러 없음. (Editor 타입은 `@tiptap/react` 에서 import; `setContent`/`insertContentAt` 는 Tiptap 표준 명령.)

### Step 4: 커밋

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/src/lib/editor/commitGenerated.ts
git commit -m "feat(editor): commitGenerated — mode-aware plain-text commit helper"
```

## Context

Plan 21 Task 1. Plan 20 ghost-text UX 를 모달로 대체하는 첫 단계 — commit 로직을 순수 헬퍼로 분리. 후속 Task 들이 useAIGeneration / AIModal / Workspace 를 만든다.

3 모드:
- `replace`: selection 범위 교체 (대체)
- `insert`: 커서 위치 삽입 (삽입)
- `replaceAll`: 문서 전체 교체 (전체교체)

plain text 보장 — schema 의 mark 상속 없음. `insertContentAt` 에 paragraph JSON 을 주면 marks 없이 들어감. `setContent` 도 동일.

Tiptap `Editor` 인스턴스는 Workspace 의 `editorRef.current?.editor` 로 접근 (Plan 18 T12 에서 노출됨).

Working directory: `/Users/changheonshin/workspace/myworks/linetta`. Branch: `main`. Never push. Never skip hooks.

## Report Format

- Status DONE | DONE_WITH_CONCERNS | BLOCKED
- textToParagraphs sanity 스크립트 결과
- 타입체크 결과
- 커밋 SHA
- 우려사항

---

## Task 2: useAIGeneration 훅 (variations React-state)

**Files:**
- Create: `apps/desktop/src/lib/editor/useAIGeneration.ts`

useGhostText 를 대체. variations 를 React state 로 보유, 에디터 commit 은 하지 않음 (Workspace 책임).

### Step 1: 파일 작성

`apps/desktop/src/lib/editor/useAIGeneration.ts` 생성:

```ts
import { useCallback, useEffect, useRef, useState } from "react";
import { ai as aiApi } from "../rpc";
import type { AICancelled, AIDelta, AIDone, AIError, AIOptions, AIReset } from "../types";
import { useEngineEvent } from "../../hooks/useEngineEvent";

export interface GenVariation {
  text: string;
  done: boolean;
  error?: string;
}

export type GenStatus =
  | { kind: "idle" }
  | { kind: "running" }
  | { kind: "done" };

export interface GenRunArgs {
  nodeId: string;
  prompt: string;
  options: AIOptions;
  selectionText?: string;
}

/**
 * useAIGeneration runs one or N parallel ai.run calls and accumulates each
 * run's streamed text into React state (no editor decoration). The consumer
 * (AIModal) renders variations; the Workspace commits the chosen one.
 */
export function useAIGeneration() {
  const [variations, setVariations] = useState<GenVariation[]>([]);
  const [currentIdx, setCurrentIdx] = useState(0);
  const [status, setStatus] = useState<GenStatus>({ kind: "idle" });

  const activeRunIdsRef = useRef<string[]>([]);
  const runIdToVariationRef = useRef<Map<string, number>>(new Map());

  const cancelAllInFlight = useCallback(() => {
    for (const id of activeRunIdsRef.current) {
      aiApi.cancel(id).catch(() => {});
    }
    activeRunIdsRef.current = [];
    runIdToVariationRef.current.clear();
  }, []);

  // Recompute status from the variations array: done when every slot is done.
  const recomputeStatus = useCallback((vs: GenVariation[]) => {
    if (vs.length === 0) {
      setStatus({ kind: "idle" });
      return;
    }
    setStatus(vs.every((v) => v.done) ? { kind: "done" } : { kind: "running" });
  }, []);

  const launch = useCallback(
    ({ nodeId, prompt, options, selectionText = "" }: GenRunArgs, n: number) => {
      cancelAllInFlight();
      const slots: GenVariation[] = Array.from({ length: n }, () => ({ text: "", done: false }));
      setVariations(slots);
      setCurrentIdx(0);
      setStatus({ kind: "running" });
      for (let i = 0; i < n; i++) {
        const idx = i;
        aiApi
          .run(nodeId, prompt, options, selectionText)
          .then(({ run_id }) => {
            activeRunIdsRef.current.push(run_id);
            runIdToVariationRef.current.set(run_id, idx);
          })
          .catch((e) => {
            setVariations((prev) => {
              const next = prev.slice();
              if (next[idx]) next[idx] = { ...next[idx], done: true, error: String(e) };
              recomputeStatus(next);
              return next;
            });
          });
      }
    },
    [cancelAllInFlight, recomputeStatus],
  );

  const start = useCallback((args: GenRunArgs) => launch(args, 1), [launch]);
  const startVariations = useCallback(
    (args: GenRunArgs, n: number) => launch(args, n),
    [launch],
  );

  const switchVariation = useCallback((direction: -1 | 1) => {
    setCurrentIdx((idx) => {
      setVariations((vs) => {
        // no mutation; just read length via closure below
        return vs;
      });
      return idx; // replaced below by functional form using variations length
    });
  }, []);

  // switchVariation needs variations length; implement with a ref mirror.
  const variationsRef = useRef(variations);
  variationsRef.current = variations;
  const switchVariationFixed = useCallback((direction: -1 | 1) => {
    const n = variationsRef.current.length;
    if (n <= 1) return;
    setCurrentIdx((idx) => ((idx + direction) % n + n) % n);
  }, []);

  // cancel: stop all in-flight runs and clear state. Used both for explicit
  // cancel (Esc/취소/백드롭) and after accept (to kill the losing variations).
  const cancel = useCallback(() => {
    cancelAllInFlight();
    setVariations([]);
    setCurrentIdx(0);
    setStatus({ kind: "idle" });
  }, [cancelAllInFlight]);

  useEngineEvent<AIDelta>("ai-delta", (p) => {
    const idx = runIdToVariationRef.current.get(p.run_id);
    if (idx === undefined) return;
    setVariations((prev) => {
      const next = prev.slice();
      if (next[idx]) next[idx] = { ...next[idx], text: next[idx].text + p.text };
      return next;
    });
  });

  useEngineEvent<AIReset>("ai-reset", (p) => {
    const idx = runIdToVariationRef.current.get(p.run_id);
    if (idx === undefined) return;
    setVariations((prev) => {
      const next = prev.slice();
      if (next[idx]) next[idx] = { ...next[idx], text: p.text };
      return next;
    });
  });

  useEngineEvent<AIDone>("ai-done", (p) => {
    const idx = runIdToVariationRef.current.get(p.run_id);
    if (idx === undefined) return;
    activeRunIdsRef.current = activeRunIdsRef.current.filter((id) => id !== p.run_id);
    setVariations((prev) => {
      const next = prev.slice();
      // full_text is the authoritative backstop for any deltas dropped by the
      // early-delta race (delta arriving before run_id → idx mapping is set).
      if (next[idx]) next[idx] = { ...next[idx], text: p.full_text, done: true };
      recomputeStatus(next);
      return next;
    });
  });

  useEngineEvent<AIError>("ai-error", (p) => {
    const idx = runIdToVariationRef.current.get(p.run_id);
    if (idx === undefined) return;
    activeRunIdsRef.current = activeRunIdsRef.current.filter((id) => id !== p.run_id);
    setVariations((prev) => {
      const next = prev.slice();
      if (next[idx]) next[idx] = { ...next[idx], done: true, error: p.message };
      recomputeStatus(next);
      return next;
    });
  });

  useEngineEvent<AICancelled>("ai-cancelled", (p) => {
    const idx = runIdToVariationRef.current.get(p.run_id);
    if (idx === undefined) return;
    runIdToVariationRef.current.delete(p.run_id);
    activeRunIdsRef.current = activeRunIdsRef.current.filter((id) => id !== p.run_id);
  });

  // Cleanup on unmount.
  useEffect(() => {
    return () => {
      cancelAllInFlight();
    };
  }, [cancelAllInFlight]);

  return {
    variations,
    currentIdx,
    status,
    start,
    startVariations,
    switchVariation: switchVariationFixed,
    cancel,
  };
}
```

**구현 정리 노트 (implementer 가 정돈할 것):** 위 `switchVariation` (broken) 는 제거하고 `switchVariationFixed` 만 남겨 `switchVariation` 이름으로 export. 즉 반환 객체의 `switchVariation: switchVariationFixed`. 깨진 첫 `switchVariation` useCallback 블록은 삭제하고 `variationsRef` + `switchVariationFixed` 만 유지. (이 노트대로 정리해서 단일 깔끔한 구현으로.)

### Step 2: 타입체크

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop && npx tsc --noEmit
```

기대: 에러 없음. (`AIDelta`/`AIReset`/`AIDone`/`AIError`/`AICancelled` 타입은 `lib/types.ts` 에 존재 — Plan 18.)

### Step 3: 커밋

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/src/lib/editor/useAIGeneration.ts
git commit -m "feat(editor): useAIGeneration — React-state variations, no editor decoration"
```

## Context

Plan 21 Task 2. useGhostText 를 대체하는 훅 — 핵심 차이: PM decoration 대신 React state 로 variations 보유, 에디터 commit 안 함. 단일 모드 = N=1 의 특수 케이스 (Plan 20 의 single/variation 이중 분기 제거 — 자동 commit 이 사라졌으므로 통일).

이벤트 핸들러는 runId→idx 매핑으로 해당 variation 만 갱신. early-delta race 는 ai-done 의 full_text 가 backstop (Plan 20 와 동일, 주석 유지). useEngineEvent 의 StrictMode 가드 (Plan 18 fixup) 재사용.

`useGhostText.ts` 는 아직 삭제하지 말 것 (Task 6 에서 Workspace 전환과 함께 삭제). 이 task 는 새 훅 추가만.

Working directory: `/Users/changheonshin/workspace/myworks/linetta`. Branch: `main`. Never push. Never skip hooks.

## Report Format

- Status DONE | DONE_WITH_CONCERNS | BLOCKED
- switchVariation 정리 여부 (깨진 블록 제거 확인)
- 타입체크 결과
- 커밋 SHA
- 우려사항

---

## Task 3: AIModal 컴포넌트 + CSS

**Files:**
- Create: `apps/desktop/src/components/ai/AIModal.tsx`
- Create: `apps/desktop/src/components/ai/AIModal.css`

### Step 1: CSS 작성

`apps/desktop/src/components/ai/AIModal.css` 생성:

```css
.ai-modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 100;
}

.ai-modal {
  background: var(--surface, #1d1d1f);
  color: var(--text, #e8e8ea);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 12px;
  width: min(640px, 92vw);
  max-height: 84vh;
  display: flex;
  flex-direction: column;
  gap: 0.7rem;
  padding: 1rem 1.1rem;
  box-shadow: 0 24px 64px rgba(0, 0, 0, 0.6);
  font-size: 0.9rem;
}

.ai-modal-modes {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  font-size: 0.82rem;
}

.ai-modal-mode-label {
  background: rgba(255, 255, 255, 0.1);
  border-radius: 6px;
  padding: 0.2rem 0.6rem;
}

.ai-modal-mode-radio {
  display: flex;
  align-items: center;
  gap: 0.3rem;
  cursor: pointer;
}

.ai-modal-textarea {
  width: 100%;
  min-height: 3rem;
  max-height: 10rem;
  resize: vertical;
  background: rgba(0, 0, 0, 0.25);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 8px;
  padding: 0.5rem 0.6rem;
  color: inherit;
  font-family: inherit;
  font-size: 0.92rem;
}

.ai-modal-textarea.shake {
  animation: ai-modal-shake 0.35s;
}

@keyframes ai-modal-shake {
  10%, 90% { transform: translateX(-1px); }
  20%, 80% { transform: translateX(2px); }
  30%, 50%, 70% { transform: translateX(-3px); }
  40%, 60% { transform: translateX(3px); }
}

.ai-modal-chiprow {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  flex-wrap: wrap;
}

.ai-modal-chip {
  background: rgba(255, 255, 255, 0.06);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 999px;
  padding: 0.2rem 0.65rem;
  font-size: 0.8rem;
  color: inherit;
  cursor: pointer;
}

.ai-modal-chip.active {
  background: rgba(255, 255, 255, 0.18);
  border-color: rgba(255, 255, 255, 0.25);
}

.ai-modal-ctx {
  margin-left: auto;
  opacity: 0.75;
  background: none;
  border: none;
  color: inherit;
  cursor: pointer;
  font-size: 0.78rem;
}

.ai-modal-result {
  color: var(--text, #e8e8ea);
  background: rgba(255, 255, 255, 0.04);
  border: 1px solid rgba(255, 255, 255, 0.06);
  border-radius: 8px;
  padding: 0.6rem 0.7rem;
  white-space: pre-wrap;
  line-height: 1.6;
  max-height: 40vh;
  overflow-y: auto;
}

.ai-modal-result-cursor {
  opacity: 0.7;
  animation: ai-modal-blink 1s steps(2) infinite;
}

@keyframes ai-modal-blink {
  to { opacity: 0; }
}

.ai-modal-error {
  color: #e07a7a;
  opacity: 0.85;
}

.ai-modal-footer {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.ai-modal-nav {
  display: flex;
  align-items: center;
  gap: 0.3rem;
  font-size: 0.8rem;
  opacity: 0.8;
}

.ai-modal-actions {
  margin-left: auto;
  display: flex;
  gap: 0.5rem;
}

.ai-modal-btn {
  padding: 0.3rem 0.85rem;
  font-size: 0.82rem;
  background: rgba(255, 255, 255, 0.1);
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 6px;
  color: inherit;
  cursor: pointer;
}

.ai-modal-btn.primary {
  background: rgba(120, 170, 255, 0.25);
  border-color: rgba(120, 170, 255, 0.4);
}

.ai-modal-btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
```

### Step 2: 컴포넌트 작성

`apps/desktop/src/components/ai/AIModal.tsx` 생성:

```tsx
import { useEffect, useRef, useState } from "react";
import type { AIOptions } from "../../lib/types";
import type { CommitMode } from "../../lib/editor/commitGenerated";
import type { GenStatus, GenVariation } from "../../lib/editor/useAIGeneration";
import { TONE_PRESETS } from "../../lib/tonePresets";
import "./AIModal.css";

interface Props {
  mode: CommitMode;
  canChooseMode: boolean; // true when no selection → show 삽입/전체교체 radio
  options: AIOptions;
  contextItemCount: number;
  variations: GenVariation[];
  currentIdx: number;
  status: GenStatus;
  onModeChange: (m: CommitMode) => void;
  onOptionsChange: (o: AIOptions) => void;
  onRun: (prompt: string, variationsOn: boolean) => void;
  onSwitch: (direction: -1 | 1) => void;
  onAccept: () => void;
  onCancel: () => void;
  onContextClick: () => void;
}

const MODE_LABEL: Record<CommitMode, string> = {
  replace: "대체",
  insert: "삽입",
  replaceAll: "전체교체",
};

export function AIModal(props: Props) {
  const [prompt, setPrompt] = useState("");
  const [variationsOn, setVariationsOn] = useState(false);
  const [shake, setShake] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    textareaRef.current?.focus();
  }, []);

  const hasResult = props.variations.length > 0;
  const isRunning = props.status.kind === "running";
  const current = props.variations[props.currentIdx];
  const acceptable = !!current && !current.error;

  const run = () => {
    const text = prompt.trim();
    if (!text) {
      setShake(true);
      setTimeout(() => setShake(false), 350);
      textareaRef.current?.focus();
      return;
    }
    props.onRun(text, variationsOn);
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Escape") {
      e.preventDefault();
      props.onCancel();
      return;
    }
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      run();
      return;
    }
    if (hasResult && props.variations.length > 1) {
      if (e.key === "ArrowLeft") {
        e.preventDefault();
        props.onSwitch(-1);
      } else if (e.key === "ArrowRight") {
        e.preventDefault();
        props.onSwitch(1);
      }
    }
  };

  const onTextareaKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey && !e.metaKey && !e.ctrlKey) {
      e.preventDefault();
      run();
    }
  };

  return (
    <div className="ai-modal-backdrop" onMouseDown={props.onCancel}>
      <div
        className="ai-modal"
        onMouseDown={(e) => e.stopPropagation()}
        onKeyDown={onKeyDown}
      >
        <div className="ai-modal-modes">
          {props.canChooseMode ? (
            <>
              <label className="ai-modal-mode-radio">
                <input
                  type="radio"
                  name="ai-mode"
                  checked={props.mode === "insert"}
                  onChange={() => props.onModeChange("insert")}
                />
                삽입
              </label>
              <label className="ai-modal-mode-radio">
                <input
                  type="radio"
                  name="ai-mode"
                  checked={props.mode === "replaceAll"}
                  onChange={() => props.onModeChange("replaceAll")}
                />
                전체교체
              </label>
            </>
          ) : (
            <span className="ai-modal-mode-label">모드: {MODE_LABEL[props.mode]}</span>
          )}
        </div>

        <textarea
          ref={textareaRef}
          className={`ai-modal-textarea${shake ? " shake" : ""}`}
          placeholder="프롬프트를 입력하세요…"
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          onKeyDown={onTextareaKeyDown}
          rows={3}
        />

        <div className="ai-modal-chiprow">
          <select
            className="ai-modal-chip"
            value={props.options.tone}
            onChange={(e) => props.onOptionsChange({ ...props.options, tone: e.target.value as AIOptions["tone"] })}
          >
            {TONE_PRESETS.map((t) => (
              <option key={t.id} value={t.id}>톤: {t.label}</option>
            ))}
          </select>
          <button
            type="button"
            className="ai-modal-chip"
            onClick={() => props.onOptionsChange({ ...props.options, short_form: !props.options.short_form })}
            aria-pressed={props.options.short_form}
          >
            {props.options.short_form ? "길이: 한 문단" : "길이: 자유"}
          </button>
          <button
            type="button"
            className={`ai-modal-chip${variationsOn ? " active" : ""}`}
            onClick={() => setVariationsOn((v) => !v)}
            aria-pressed={variationsOn}
            title="3개 변형 병렬 생성 (토큰 3배)"
          >
            변형 ×3
          </button>
          <button type="button" className="ai-modal-ctx" onClick={props.onContextClick}>
            ⓘ ctx: {props.contextItemCount}개
          </button>
        </div>

        {hasResult && (
          <div className="ai-modal-result">
            {current?.error ? (
              <span className="ai-modal-error">(오류: {current.error})</span>
            ) : (
              <>
                {current?.text}
                {isRunning && !current?.done && <span className="ai-modal-result-cursor">▌</span>}
              </>
            )}
          </div>
        )}

        <div className="ai-modal-footer">
          {hasResult && props.variations.length > 1 && (
            <div className="ai-modal-nav">
              <button type="button" className="ai-modal-chip" onClick={() => props.onSwitch(-1)}>◀</button>
              <span>{props.currentIdx + 1}/{props.variations.length}</span>
              <button type="button" className="ai-modal-chip" onClick={() => props.onSwitch(1)}>▶</button>
            </div>
          )}
          <div className="ai-modal-actions">
            <button type="button" className="ai-modal-btn" onClick={props.onCancel}>
              취소
            </button>
            {!hasResult ? (
              <button type="button" className="ai-modal-btn primary" onClick={run}>
                생성 ⌘↵
              </button>
            ) : (
              <>
                <button type="button" className="ai-modal-btn" onClick={run} title="다시 생성">
                  다시
                </button>
                <button
                  type="button"
                  className="ai-modal-btn primary"
                  onClick={props.onAccept}
                  disabled={!acceptable}
                >
                  수락 Tab
                </button>
              </>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
```

추가로 Tab 수락: `onKeyDown` (모달 컨테이너) 에 Tab 처리 추가 — `hasResult && acceptable` 일 때 Tab 으로 수락. 위 `onKeyDown` 에 다음 분기를 ArrowLeft/Right 위에 추가:

```tsx
    if (hasResult && e.key === "Tab") {
      e.preventDefault();
      if (acceptable) props.onAccept();
      return;
    }
```

(이 블록을 `onKeyDown` 함수 안, Escape/Cmd+Enter 처리 다음에 삽입.)

### Step 3: 타입체크

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop && npx tsc --noEmit
```

기대: 에러 없음. (`CommitMode` from Task 1, `GenStatus`/`GenVariation` from Task 2, `TONE_PRESETS`/`AIOptions` 기존.)

### Step 4: 커밋

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/src/components/ai/AIModal.tsx apps/desktop/src/components/ai/AIModal.css
git commit -m "feat(ai): AIModal — centered modal with mode selector + variation cards"
```

## Context

Plan 21 Task 3. AIPromptBar 를 대체하는 중앙 모달. focus-trap 은 백드롭 + setEditable(false)(Workspace 책임) 로 충분 — 모달 자체는 textarea autofocus + Esc/Tab/◀▶ 키핸들러만.

결과 카드는 솔리드 배경 + 정상 대비 (`.ai-modal-result` — ghost 의 0.45 opacity 아님). complaint #3 해결.

모드 셀렉터: canChooseMode (selection 없음) 면 삽입/전체교체 라디오, 아니면 "모드: 대체" 라벨.

`tonePresets` 의 `TONE_PRESETS` 와 `AIOptions` 는 기존 (Plan 11/18). `useAIGeneration` 의 타입은 Task 2.

Working directory: `/Users/changheonshin/workspace/myworks/linetta`. Branch: `main`. Never push. Never skip hooks.

## Report Format

- Status DONE | DONE_WITH_CONCERNS | BLOCKED
- Tab 수락 분기 추가 위치
- 타입체크 결과
- 커밋 SHA
- 우려사항

---

## Task 4: Workspace 통합 (모달 + 잠금 + commit) & 옛 파일 삭제

**Files:**
- Modify: `apps/desktop/src/routes/Workspace.tsx`
- Delete: `apps/desktop/src/components/editor/GhostExtension.ts`, `GhostExtension.css`, `apps/desktop/src/lib/editor/useGhostText.ts`, `apps/desktop/src/components/ai/AIPromptBar.tsx`, `AIPromptBar.css`

이 task는 큰 통합 작업. 단계적으로.

### Step 1: import 교체

`apps/desktop/src/routes/Workspace.tsx` 상단 import 에서 제거:

```tsx
import { GhostExtension } from "../components/editor/GhostExtension";
import { useGhostText } from "../lib/editor/useGhostText";
import { AIPromptBar } from "../components/ai/AIPromptBar";
```

추가:

```tsx
import { useAIGeneration } from "../lib/editor/useAIGeneration";
import { AIModal } from "../components/ai/AIModal";
import { commitGenerated, type CommitMode } from "../lib/editor/commitGenerated";
```

(`AIContextChecklist`, `totalContextItems` import 는 유지.)

### Step 2: state 교체

기존 (라인 86-89 부근):

```tsx
const [aiPromptAnchor, setAiPromptAnchor] = useState<{ top: number; left: number } | null>(null);
const aiPromptAnchorRef = useRef(aiPromptAnchor);
useEffect(() => { aiPromptAnchorRef.current = aiPromptAnchor; }, [aiPromptAnchor]);
const closeAIBarRef = useRef<(() => void) | null>(null);
```

를 다음으로 교체:

```tsx
const [aiModal, setAiModal] = useState<{
  mode: CommitMode;
  canChooseMode: boolean;
  sel: { from: number; to: number };
} | null>(null);
const aiModalOpenRef = useRef(false);
useEffect(() => { aiModalOpenRef.current = aiModal !== null; }, [aiModal]);
const closeAIModalRef = useRef<(() => void) | null>(null);
```

`contextCounts`, `previewReqIdRef`, `loadRef`, `aiCtxChecklistOpen` 등은 유지.

### Step 3: useGhostText → useAIGeneration

기존 (라인 420-421):

```tsx
const tiptapEditor = editorRef.current?.editor ?? null;
const ghost = useGhostText(tiptapEditor);
```

를:

```tsx
const tiptapEditor = editorRef.current?.editor ?? null;
const gen = useAIGeneration();
```

### Step 4: closeAIModal + commit + done effect

기존 `closeAIBar` useCallback (라인 427-433) 과 done effect (435-441) 를 다음으로 교체:

```tsx
const closeAIModal = useCallback(() => {
  gen.cancel();
  if (tiptapEditor) tiptapEditor.setEditable(true);
  setAiModal(null);
  setContextCounts(null);
  setAiCtxChecklistOpen(false);
  previewReqIdRef.current++;
}, [gen, tiptapEditor]);
useEffect(() => { closeAIModalRef.current = closeAIModal; }, [closeAIModal]);

const acceptAIModal = useCallback(() => {
  if (!aiModal || !tiptapEditor) return;
  const v = gen.variations[gen.currentIdx];
  if (!v || v.error) return;
  commitGenerated(tiptapEditor, aiModal.mode, aiModal.sel, v.text);
  gen.cancel();
  tiptapEditor.setEditable(true);
  setAiModal(null);
  setContextCounts(null);
  setAiCtxChecklistOpen(false);
  previewReqIdRef.current++;
}, [aiModal, gen, tiptapEditor]);

// Safety: if the modal unmounts for any reason, re-enable editing.
useEffect(() => {
  if (aiModal === null && tiptapEditor && !tiptapEditor.isEditable) {
    tiptapEditor.setEditable(true);
  }
}, [aiModal, tiptapEditor]);
```

(Plan 20 의 `ghost.status.kind === "done"` 자동 close effect 는 제거 — 모달은 자동 commit 안 함, 사용자가 수락/취소.)

### Step 5: Cmd+I 핸들러 교체

라인 334-365 의 `else if (e.key.toLowerCase() === "i")` 블록을 다음으로 교체:

```tsx
      } else if (e.key.toLowerCase() === "i") {
        e.preventDefault();
        if (aiModalOpenRef.current) {
          closeAIModalRef.current?.();
          return;
        }
        const ed = editorRef.current?.editor;
        const currentLoad = loadRef.current;
        if (!ed || !currentLoad) return;
        const { from, to, empty } = ed.state.selection;
        ed.setEditable(false);
        setAiModal({
          mode: empty ? "insert" : "replace",
          canChooseMode: empty,
          sel: { from, to },
        });
        const reqId = ++previewReqIdRef.current;
        aiApi.previewContext(currentLoad.node.id)
          .then((counts) => {
            if (reqId !== previewReqIdRef.current) return;
            setContextCounts(counts);
          })
          .catch((err) => {
            if (reqId !== previewReqIdRef.current) return;
            showToast(`컨텍스트 정보를 가져오지 못했습니다: ${err}`);
          });
      }
```

### Step 6: extensions 에서 GhostExtension 제거

라인 762-766 의 extensions 배열에서 `GhostExtension,` 줄 삭제:

```tsx
extensions={[
  ...(mentionExtension ? [mentionExtension] : []),
  NoteMarkerExtension,
]}
```

### Step 7: AIPromptBar → AIModal 마운트

라인 811-861 의 `{aiPromptAnchor && (<AIPromptBar ... />)}` 블록 전체를 다음으로 교체:

```tsx
      {aiModal && load && (
        <AIModal
          mode={aiModal.mode}
          canChooseMode={aiModal.canChooseMode}
          options={aiOptions}
          contextItemCount={totalContextItems(contextCounts ?? FALLBACK_COUNTS)}
          variations={gen.variations}
          currentIdx={gen.currentIdx}
          status={gen.status}
          onModeChange={(m) => setAiModal((s) => (s ? { ...s, mode: m } : s))}
          onOptionsChange={setAiOptions}
          onRun={(promptText, variationsOn) => {
            const selectionText =
              aiModal.mode === "replace"
                ? tiptapEditor!.state.doc.textBetween(aiModal.sel.from, aiModal.sel.to, "\n")
                : "";
            const args = {
              nodeId: load.node.id,
              prompt: promptText,
              options: aiOptions,
              selectionText,
            };
            if (variationsOn) gen.startVariations(args, 3);
            else gen.start(args);
          }}
          onSwitch={gen.switchVariation}
          onAccept={acceptAIModal}
          onCancel={closeAIModal}
          onContextClick={() => setAiCtxChecklistOpen((v) => !v)}
        />
      )}
      {aiCtxChecklistOpen && aiModal && (
        <AIContextChecklist
          anchor={{ top: 120, left: window.innerWidth / 2 - 160 }}
          counts={contextCounts ?? FALLBACK_COUNTS}
          onClose={() => setAiCtxChecklistOpen(false)}
        />
      )}
```

(`tiptapEditor!` non-null: aiModal 이 열려있으면 editor 존재. 안전상 `tiptapEditor?.` 로 바꾸고 빈 문자열 fallback 해도 됨 — 단순화 위해 `aiModal.mode === "replace"` 일 때만 textBetween 호출.)

### Step 8: 옛 파일 삭제

```bash
cd /Users/changheonshin/workspace/myworks/linetta
rm apps/desktop/src/components/editor/GhostExtension.ts
rm apps/desktop/src/components/editor/GhostExtension.css
rm apps/desktop/src/lib/editor/useGhostText.ts
rm apps/desktop/src/components/ai/AIPromptBar.tsx
rm apps/desktop/src/components/ai/AIPromptBar.css
```

### Step 9: 잔여 참조 검색

```bash
grep -rn "GhostExtension\|useGhostText\|AIPromptBar\|ghostPluginKey\|aiPromptAnchor\|closeAIBar\b" /Users/changheonshin/workspace/myworks/linetta/apps/desktop/src --include="*.ts" --include="*.tsx"
```

기대: 결과 0개. 남아있으면 제거.

### Step 10: 타입체크 + 엔진 빌드

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop && npx tsc --noEmit
cd /Users/changheonshin/workspace/myworks/linetta/engine && go test ./...
cd /Users/changheonshin/workspace/myworks/linetta && ./scripts/build-engine.sh
```

기대: 타입체크 clean (잔여 import 0), 엔진 테스트 PASS, 빌드 성공.

### Step 11: 커밋

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/src/routes/Workspace.tsx
git add -u apps/desktop/src/components/editor/ apps/desktop/src/lib/editor/ apps/desktop/src/components/ai/
git commit -m "feat(workspace): replace ghost-text with AI modal; lock editor while open"
```

## Context

Plan 21 Task 4 — 통합 finale + 옛 인프라 삭제. 이전 task:
- T1 `commitGenerated` 헬퍼
- T2 `useAIGeneration` 훅
- T3 `AIModal` 컴포넌트

이번 task 가 Workspace 를 모달 흐름으로 전환하고 GhostExtension/useGhostText/AIPromptBar 삭제.

핵심: Cmd+I → selection 캡처 + setEditable(false). 수락 → commitGenerated + setEditable(true). 취소/Esc/백드롭 → cancel + setEditable(true). unmount safety effect 로 setEditable(true) 보장.

`tiptapEditor.isEditable` 는 Tiptap Editor 표준 속성. `setEditable(bool)` 도 표준.

Working directory: `/Users/changheonshin/workspace/myworks/linetta`. Branch: `main`. Never push. Never skip hooks.

## Before You Begin

큰 통합이라 다음 발생 시 BLOCKED 보고:
1. Cmd+I 핸들러의 closure 가 stale state 잡는 문제 (aiModalOpenRef/loadRef/closeAIModalRef 로 우회 — Plan 19/20 패턴)
2. tiptapEditor 가 처음 null 인 타이밍 (모달은 editor 존재 후에만 열림)
3. 타입체크 에러 폭증 (10개 이상)

## Report Format

- Status DONE | DONE_WITH_CONCERNS | BLOCKED
- 잔여 참조 grep 결과 (0이어야 함)
- 타입체크 / 엔진 테스트 / 빌드 결과
- 커밋 SHA
- 우려사항

---

## 통합 검증 (Task 4 직후 수동 스모크)

```bash
rm -rf /tmp/linetta-plan21 && LINETTA_HOME=/tmp/linetta-plan21 ./scripts/dev.sh
```

**모드:**
1. 본문 일부 선택 + Cmd+I → 모달에 "모드: 대체" 라벨. 프롬프트 + 생성 → 수락 → 선택 영역이 생성문으로 plain 교체.
2. 선택 없이 Cmd+I → [삽입/전체교체] 라디오 (기본 삽입). 생성 → 수락 → 커서 위치 삽입.
3. 선택 없이 + 전체교체 라디오 → 생성 → 수락 → 씬 본문 전체 교체. Cmd+Z 로 1회 복구 확인.

**잠금 (complaint #2):**
4. 모달 열린 상태에서 에디터 클릭/드래그 → 백드롭이 막음, 커서/선택 안 변함.
5. 모달 열린 상태에서 본문 타이핑 시도 → setEditable(false) 라 입력 안 됨.
6. 취소/Esc 후 → 에디터 다시 편집 가능 (setEditable true 복구).

**가독성 (complaint #3):**
7. 생성된 텍스트가 솔리드 배경에 또렷이 보임 (이전 ghost 처럼 흐리지 않음).

**변형:**
8. 변형 ×3 ON → 생성 → 모달 안 [1/3] ◀▶ 전환, 각 카드 스트리밍/완료. 수락 → 선택 카드 commit + 나머지 cancel (engine 로그).

**닫기/회귀:**
9. Cmd+I 재토글 → 모달 닫힘 + 편집 가능.
10. Cmd+P 팔레트 정상. ctx 칩 → checklist 실제 카운트 (Plan 19).
11. ZEN 모드 / 다른 씬 전환 — 모달 안 열린 상태에서 정상 (잠금은 모달 열렸을 때만).

통과 시:

```bash
git tag plan-21-ai-modal-done
```

---

## Self-Review

**1. Spec 커버리지:**

| Spec 요구 | Task |
|---|---|
| commitGenerated 모드별 (replace/insert/replaceAll) | Task 1 |
| textToParagraphs (문단 분리 + hardBreak + plain) | Task 1 |
| useAIGeneration React-state variations | Task 2 |
| 이벤트 핸들러 runId→idx + full_text backstop | Task 2 |
| 단일=N=1 통일 (이중 분기 제거) | Task 2 |
| AIModal 중앙 오버레이 + 백드롭 | Task 3 |
| 모드 셀렉터 (대체 라벨 / 삽입·전체교체 라디오) | Task 3 |
| 결과 솔리드 카드 (가독성) | Task 3 (CSS) |
| 변형 ◀▶ 카드 + Tab 수락 | Task 3 |
| Cmd+I selection 캡처 + setEditable(false) | Task 4 |
| 수락 commitGenerated + setEditable(true) | Task 4 |
| 취소/Esc/백드롭 cancel + setEditable(true) | Task 4 |
| GhostExtension/useGhostText/AIPromptBar 삭제 | Task 4 |
| AIContextChecklist 유지 | Task 4 (마운트 유지) |
| unmount safety setEditable(true) | Task 4 |
| 수동 스모크 11 시나리오 | Task 4 직후 |

모든 spec 요구 매핑.

**2. Placeholder scan:** "TBD"/"TODO" 없음. Task 2 의 switchVariation 정리 노트는 implementer 지시 (placeholder 아님 — 명확한 정리 지침). Task 1 의 spec 본문에 있던 `// ...` placeholder 는 이 plan 에서 완전한 textToParagraphs 구현으로 대체됨.

**3. Type 일관성:**
- `CommitMode` ("replace"|"insert"|"replaceAll") — Task 1 정의, Task 3 import, Task 4 import. 일치.
- `commitGenerated(editor, mode, target, text)` — Task 1 시그니처, Task 4 호출. 일치.
- `GenVariation`/`GenStatus`/`GenRunArgs` — Task 2 정의, Task 3 import (GenVariation/GenStatus), Task 4 사용. 일치.
- `useAIGeneration()` 반환 `{ variations, currentIdx, status, start, startVariations, switchVariation, cancel }` — Task 2 정의, Task 4 사용 (gen.variations, gen.start, gen.startVariations, gen.switchVariation, gen.cancel). 일치. (reset 제거 — cancel 이 in-flight cancel + state clear 를 모두 수행하므로 수락/취소 양쪽에서 cancel 사용.)
- AIModal Props — Task 3 정의, Task 4 마운트 시 모든 prop 전달. 일치 (mode, canChooseMode, options, contextItemCount, variations, currentIdx, status, onModeChange, onOptionsChange, onRun, onSwitch, onAccept, onCancel, onContextClick).
- `onRun(prompt, variationsOn)` — Task 3 (2-arg), Task 4 콜백 (2-arg). 일치. (Plan 20 의 3-arg preset 제거됨.)

체크 통과.

**4. 위험 영역:**
- Task 4 의 Cmd+I closure stale state — aiModalOpenRef/loadRef/closeAIModalRef 패턴 (Plan 19/20 검증됨).
- 모달 열린 채 사이드바 노드 전환: 백드롭이 사이드바 클릭을 막는지 확인 (백드롭 z-index 100). 막힌다면 OK; 안 막히면 노드 전환 시 closeAIModal 호출 필요 — 스모크 #11 에서 확인.
- `commitGenerated` 의 `insertContentAt` 가 frozen `sel.from` 사용 — 에디터 잠금 중 doc 불변이라 offset 유효. setEditable(false) 가 이를 보장.
