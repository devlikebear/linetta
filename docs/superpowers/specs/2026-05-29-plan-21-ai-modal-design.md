# Plan 21 — AI 모달 재설계 (Ghost → Modal) Design Spec

## 목적

Plan 20 의 인라인 ghost-text 변형 UX 가 실사용에서 세 가지 문제를 드러냈다:

1. **모드 부적절** — `재작성/확장/요약` 프리셋(프롬프트 시드)이 사용자가 원하는 "결과를 어디에 넣을지" 와 안 맞음.
2. **포커스 혼란** — Cmd+I 바가 떠 있는 동안 에디터 커서 이동·선택 변경이 가능해 무엇을 대상으로 하는지 헷갈림.
3. **가독성** — ghost text 가 배경색과 너무 비슷해 내용 식별 불가.

해결: 인라인 ghost decoration 을 **중앙 모달**로 대체한다. 모달이 프롬프트 입력→생성→비교→선택의 전체 흐름을 소유하고, 그동안 에디터는 잠긴다. 결과는 모달 안에서 솔리드 배경·정상 대비로 표시된다. 모드는 "결과 배치"(대체/삽입/전체교체)로 재정의한다.

## Goals

1. `Cmd+I` → 중앙 오버레이 모달 (백드롭 딤). 에디터 `setEditable(false)` 로 잠금 + 백드롭이 클릭 차단.
2. 모드 자동 분기 (모달 열 때 selection 상태로 고정):
   - selection 있음 → **대체** (라벨 표시, 변경 불가)
   - selection 없음 → 라디오 **[삽입 / 전체교체]**, 기본 삽입
3. 변형 ×3 토글 유지 — 생성 후 모달 안 `[i/N] ◀ ▶` 카드로 비교, 수락 버튼으로 선택.
4. 결과 미리보기는 모달 안 솔리드 카드 (가독성 확보). ghost decoration 완전 제거.
5. 수락 시 선택된 텍스트를 plain text 로 에디터에 commit (모드별 배치). 줄바꿈은 문단 분리.
6. Esc/취소/백드롭 클릭 → commit 없이 닫기 + in-flight cancel + `setEditable(true)`.

## Non-Goals

- 변형 N 사용자 설정 (항상 3 고정).
- 변형 카드 동시 표시 (한 번에 하나, ◀▶ 전환).
- diff / word-level 비교 하이라이트.
- 변형별 다른 옵션 (같은 prompt·옵션, LLM nondeterminism).
- Engine 변경 (Plan 16/18/19 컨텍스트 페이로드 그대로).
- 프리셋 프롬프트 시드 (재작성/확장/요약 제거 — 사용자가 직접 프롬프트 입력).

---

## 1. 아키텍처

### 1.1 흐름

```
Cmd+I
  │  1. const sel = { from, to, empty } 캡처 (frozen reference)
  │  2. editor.setEditable(false)  — 커서/선택 변경 차단
  │  3. 모드 결정: sel.empty === false → "replace"; else 기본 "insert"
  │  4. setAiModal({ open: true, sel, mode })
  ▼
AIModal (중앙 오버레이 + 백드롭, focus-trap)
  │  입력: 프롬프트 textarea, 톤·길이 chip, 변형 토글, ctx 칩, 모드 셀렉터
  │  생성: Enter → useAIGeneration.start(args) 또는 startVariations(args, 3)
  ▼
모달 결과 영역 (useAIGeneration 의 React state)
  │  단일: variations[0].text 스트리밍
  │  변형: variations[currentIdx] + [i/3] ◀ ▶
  │  ◀▶ / Tab / 수락 모두 모달 키핸들러 (에디터 안 건드림)
  ▼
수락 (acceptCurrent):
  │  1. commitToEditor(editor, sel, mode, chosenText)
  │  2. cancel 남은 in-flight runs
  │  3. editor.setEditable(true)
  │  4. 모달 닫기
취소/Esc/백드롭:
  │  1. cancel 모든 in-flight runs
  │  2. editor.setEditable(true)
  │  3. 모달 닫기 (commit 없음)
```

### 1.2 commitToEditor (모드별 배치)

`apps/desktop/src/lib/editor/commitGenerated.ts` (NEW) — 순수 helper:

```ts
import type { Editor } from "@tiptap/react";

export type CommitMode = "replace" | "insert" | "replaceAll";

export interface CommitTarget {
  /** Frozen selection captured at Cmd+I time. */
  from: number;
  to: number;
}

/**
 * Commit generated text into the editor as plain text (no mark inheritance).
 * Multi-paragraph text (split on blank lines) becomes multiple paragraphs.
 */
export function commitGenerated(
  editor: Editor,
  mode: CommitMode,
  target: CommitTarget,
  text: string,
): void {
  const content = textToParagraphs(text);
  if (mode === "replaceAll") {
    editor.chain().setContent(content).run();
    return;
  }
  if (mode === "replace") {
    editor.chain().insertContentAt({ from: target.from, to: target.to }, content).run();
    return;
  }
  // insert
  editor.chain().insertContentAt(target.from, content).run();
}

/** Split text on blank lines into paragraph nodes; single newlines become hardBreaks. */
function textToParagraphs(text: string): { type: "doc" | "paragraph"; content?: unknown[] }[] | string {
  // Implementation detail in plan: produce ProseMirror-JSON paragraph nodes.
  // ...
  return text; // placeholder — plan specifies exact impl
}
```

(정확한 `textToParagraphs` 구현은 plan task 에서 — 빈 줄 기준 문단 분리, 단일 줄바꿈은 hardBreak. plain text 보장.)

### 1.3 에디터 잠금

- Cmd+I 시 `editor.setEditable(false)`. 모달 닫을 때 (수락/취소 모두) `editor.setEditable(true)`.
- 백드롭(반투명 오버레이)이 에디터 영역 클릭을 가로채 selection 변경 차단.
- frozen `sel` 은 모달 state 에 보관 — commit 시 사용. 에디터가 잠겨있어 from/to 가 유효.
- 안전장치: 모달이 unmount 될 때 effect cleanup 에서 `setEditable(true)` 보장 (수락/취소 누락 대비).

---

## 2. AIModal 컴포넌트

`apps/desktop/src/components/ai/AIModal.tsx` (NEW, AIPromptBar 대체).

### 2.1 Props

```ts
interface Props {
  open: boolean;
  mode: CommitMode;          // 모달 열 때 결정됨
  canChooseMode: boolean;    // selection 없을 때 true → 삽입/전체교체 라디오 노출
  options: AIOptions;
  contextItemCount: number;
  /** generation state from useAIGeneration */
  variations: GenVariation[];
  currentIdx: number;
  status: GenStatus;
  onModeChange: (m: CommitMode) => void;   // 라디오 변경 (canChooseMode일 때만)
  onOptionsChange: (o: AIOptions) => void;
  onRun: (prompt: string, variationsOn: boolean) => void;
  onRegenerate: () => void;
  onSwitch: (direction: -1 | 1) => void;
  onAccept: () => void;
  onCancel: () => void;       // 취소/Esc/백드롭
  onContextClick: () => void;
}
```

### 2.2 레이아웃

```
┌─ backdrop (반투명) ───────────────────────────────┐
│        ┌─ AIModal ────────────────────────────┐   │
│        │ 모드: [대체]  또는  ( ) 삽입  ( ) 전체교체 │   │
│        │ ┌──────────────────────────────────┐  │   │
│        │ │ 프롬프트를 입력하세요…             │  │   │
│        │ └──────────────────────────────────┘  │   │
│        │ 톤 ▾  길이 ▾  [변형 ×3]   ⓘ ctx: 7개   │   │
│        │ ─────────────────────────────────────  │   │
│        │ (생성 후 결과 영역:)                     │   │
│        │ ┌──────────────────────────────────┐  │   │
│        │ │ 생성된 텍스트 (솔리드 배경, 정상 대비)│  │   │
│        │ │ ...                                │  │   │
│        │ └──────────────────────────────────┘  │   │
│        │ [1/3] ◀ ▶          [취소] [수락 ⌘↵]   │   │
│        └────────────────────────────────────────┘   │
└──────────────────────────────────────────────────┘
```

- 변형 OFF → `[i/N]◀▶` 줄 숨김, 수락/취소만.
- 생성 전 → 결과 영역 + 수락 버튼 숨김 (또는 비활성). 생성 버튼만.
- 생성 중 → 결과 영역 스트리밍, 수락 버튼 활성 (스트리밍 중에도 수락 가능 → 남은 cancel). 생성 버튼 → 취소 버튼.
- 모드가 `replace` 이고 canChooseMode=false → "모드: 대체" 라벨만.
- 부분 에러 변형 → 카드에 `(오류: ...)` 회색. 그 variation 수락 시 no-op (수락 버튼 비활성 또는 무시).

### 2.3 가독성 (complaint #3 해결)

결과 카드 텍스트:
```css
.ai-modal-result {
  color: var(--text, #e8e8ea);     /* 정상 대비 (ghost 의 0.45 opacity 아님) */
  background: rgba(255, 255, 255, 0.04);
  white-space: pre-wrap;
  line-height: 1.6;
  max-height: 40vh;
  overflow-y: auto;
}
```

스트리밍 커서는 끝에 `▌` 깜박임 (현재 카드가 still streaming 일 때만).

### 2.4 focus-trap

- 모달 열릴 때 textarea autofocus.
- Tab 이 모달 내부 요소들 사이만 순환 (간단 구현: 모달 컨테이너에 `onKeyDown` 으로 Tab 시 결과 단계면 수락, 입력 단계면 기본). 완전한 focus-trap 라이브러리는 YAGNI — 백드롭 + setEditable(false) 로 충분.
- Esc → onCancel.

---

## 3. useAIGeneration 훅

`apps/desktop/src/lib/editor/useAIGeneration.ts` (NEW, useGhostText 대체).

variations 를 **React state** 로 보유 (PM decoration 아님). 에디터 commit 은 하지 않음 (Workspace 가 commitGenerated 호출).

### 3.1 타입

```ts
export interface GenVariation {
  text: string;
  done: boolean;
  error?: string;
}

export type GenStatus =
  | { kind: "idle" }
  | { kind: "running" }
  | { kind: "done" };

interface GenRunArgs {
  nodeId: string;
  prompt: string;
  options: AIOptions;
  selectionText?: string;
}
```

### 3.2 반환

```ts
{
  variations: GenVariation[];
  currentIdx: number;
  status: GenStatus;
  start: (args: GenRunArgs) => void;          // 단일 (1 variation)
  startVariations: (args: GenRunArgs, n: number) => void;
  switchVariation: (direction: -1 | 1) => void;
  cancel: () => void;                          // 모든 in-flight cancel + state reset
  reset: () => void;                           // state 비움 (수락 후)
}
```

### 3.3 내부

- `variationsRef` + `setVariations` (React state) — 렌더 트리거.
- `runIdToVariationRef: Map<string, number>`, `activeRunIdsRef: string[]`.
- start: 1개 ai.run, variations=[{text:"",done:false}].
- startVariations: n개 빈 슬롯, n개 ai.run 병렬, runId→idx 매핑.
- 이벤트 핸들러 (ai-delta/reset/done/error/cancelled): runId→idx 매핑으로 해당 variation 의 text/done/error 갱신 (React setState, immutable).
  - **단일/변형 모두 동일 경로** — 단일은 N=1 의 특수 케이스. (Plan 20 의 single/variation 이중 분기 불필요 — 자동 commit 이 사라졌으므로.)
  - ai-done: 해당 variation done=true. 자동 commit 없음 (모달이 수락 대기).
  - ai-error: 해당 variation error 설정.
  - 모든 variation done/error 시 status="done".
- switchVariation: currentIdx wrap.
- cancel: activeRunIds 모두 ai.cancel + state reset + status idle.
- reset: variations=[], currentIdx=0, status idle (수락 후 정리).
- useEngineEvent 의 StrictMode 가드는 기존 hook 재사용 (Plan 18 fixup).
- **early-delta race**: Plan 20 와 동일 — ai-done 의 full_text 가 backstop. 주석 유지.

---

## 4. Workspace 통합

`apps/desktop/src/routes/Workspace.tsx`:

### 4.1 state

```ts
const gen = useAIGeneration(tiptapEditor);
const [aiModal, setAiModal] = useState<{
  open: boolean;
  mode: CommitMode;
  canChooseMode: boolean;
  sel: { from: number; to: number };
} | null>(null);
```

기존 `aiPromptAnchor`, `contextCounts` 관련은 모달 state 로 흡수. ctx counts 는 유지 (Plan 19 previewContext).

### 4.2 Cmd+I 핸들러

```ts
if ((e.metaKey || e.ctrlKey) && e.key === "i") {
  e.preventDefault();
  if (aiModalRef.current) { closeAIModal(); return; }   // 토글 닫기
  const editor = editorRef.current?.editor;
  if (!editor || !loadRef.current) return;
  const { from, to, empty } = editor.state.selection;
  editor.setEditable(false);
  setAiModal({
    open: true,
    mode: empty ? "insert" : "replace",
    canChooseMode: empty,
    sel: { from, to },
  });
  // ctx preview fetch (Plan 19) — 동일
  const reqId = ++previewReqIdRef.current;
  aiApi.previewContext(loadRef.current.node.id).then(...).catch(...);
}
```

### 4.3 onRun / onAccept / onCancel

```ts
const onRun = (prompt, variationsOn) => {
  if (!aiModal || !load) return;
  const selectionText = aiModal.mode === "replace"
    ? tiptapEditor!.state.doc.textBetween(aiModal.sel.from, aiModal.sel.to, "\n")
    : "";
  const args = { nodeId: load.node.id, prompt, options: aiOptions, selectionText };
  if (variationsOn) gen.startVariations(args, 3);
  else gen.start(args);
};

const onAccept = () => {
  if (!aiModal || !tiptapEditor) return;
  const v = gen.variations[gen.currentIdx];
  if (!v || v.error) return;
  commitGenerated(tiptapEditor, aiModal.mode, aiModal.sel, v.text);
  gen.cancel();        // 남은 in-flight cancel
  gen.reset();
  tiptapEditor.setEditable(true);
  setAiModal(null);
  setContextCounts(null);
};

const closeAIModal = () => {
  gen.cancel();
  if (tiptapEditor) tiptapEditor.setEditable(true);
  setAiModal(null);
  setContextCounts(null);
  previewReqIdRef.current++;
};
```

### 4.4 모달 마운트

```tsx
{aiModal?.open && load && (
  <AIModal
    open
    mode={aiModal.mode}
    canChooseMode={aiModal.canChooseMode}
    options={aiOptions}
    contextItemCount={totalContextItems(contextCounts ?? FALLBACK_COUNTS)}
    variations={gen.variations}
    currentIdx={gen.currentIdx}
    status={gen.status}
    onModeChange={(m) => setAiModal((s) => s ? { ...s, mode: m } : s)}
    onOptionsChange={setAiOptions}
    onRun={onRun}
    onRegenerate={() => onRun(lastPromptRef.current, gen.variations.length > 1)}
    onSwitch={gen.switchVariation}
    onAccept={onAccept}
    onCancel={closeAIModal}
    onContextClick={() => setAiCtxChecklistOpen((v) => !v)}
  />
)}
```

(`onRegenerate` 의 lastPrompt 보관은 모달 내부 state 로 두는 게 단순 — 모달이 prompt 를 들고 있으므로 모달 안에서 재생성 처리. Workspace 의 onRegenerate 는 모달이 직접 onRun 재호출로 대체 가능. 구현 시 단순화.)

---

## 5. 제거 / 삭제

| 파일 | 처리 |
|---|---|
| `apps/desktop/src/components/editor/GhostExtension.ts` | **삭제** |
| `apps/desktop/src/components/editor/GhostExtension.css` | **삭제** |
| `apps/desktop/src/lib/editor/useGhostText.ts` | **삭제** (useAIGeneration 으로 대체) |
| `apps/desktop/src/components/ai/AIPromptBar.tsx` | **삭제** (AIModal 로 대체) |
| `apps/desktop/src/components/ai/AIPromptBar.css` | **삭제** |
| `Workspace.tsx` | GhostExtension extensions 배열에서 제거, useGhostText→useAIGeneration, AIPromptBar→AIModal |
| `apps/desktop/src/components/ai/AIContextChecklist.tsx` | **유지** |

엔진: 변경 없음.

---

## 6. 에러 처리

| 상황 | 처리 |
|---|---|
| 빈 프롬프트 생성 시도 | textarea 흔들기, 호출 안 함 |
| 한 변형 RPC start 실패 | 그 variation error 설정, 나머지 진행 |
| ai-error 이벤트 | 해당 variation `(오류: ...)` 표시 |
| 모든 변형 실패 | 모두 에러 카드. 재생성 또는 취소 |
| error variation 에서 수락 | no-op (수락 버튼 비활성) |
| 모달 unmount 시 setEditable 누락 | effect cleanup 에서 `setEditable(true)` 보장 |
| previewContext 실패 | 토스트 (Plan 19 동일), ctx 칩 FALLBACK |

---

## 7. 테스트 전략

엔진 변경 없음 — FE 타입체크 + 수동 스모크.

### 수동 스모크

**모드:**
1. selection 있음 + Cmd+I → 모드 "대체" 라벨. 프롬프트 + 생성 → 수락 → 선택 영역이 생성문으로 교체 (plain text).
2. selection 없음 + Cmd+I → [삽입/전체교체] 라디오, 기본 삽입. 생성 → 수락 → 커서 위치 삽입.
3. selection 없음 + 전체교체 선택 → 생성 → 수락 → 씬 본문 전체 교체.

**잠금 (complaint #2):**
4. 모달 열린 상태에서 에디터 클릭/드래그 시도 → 백드롭이 막음, 커서/선택 안 변함.
5. 모달 열린 상태에서 타이핑 시도 → setEditable(false) 라 입력 안 됨.

**가독성 (complaint #3):**
6. 생성된 텍스트가 솔리드 배경에 정상 대비로 또렷이 보임.

**변형:**
7. 변형 ×3 ON → 생성 → 모달 안 [1/3] ◀▶ 전환. 각 카드 스트리밍/완료.
8. 변형 중 수락 → 선택된 카드 commit + 나머지 cancel (engine 로그).

**닫기:**
9. Esc / 취소 / 백드롭 클릭 → commit 없이 닫힘 + in-flight cancel + 에디터 다시 편집 가능 (setEditable true).

**회귀:**
10. Cmd+P 팔레트 정상. ctx 칩 → checklist 실제 카운트 (Plan 19).
11. 모달 안 닫고 다른 씬 못 감 (에디터 잠김) — 정상.

통과 시 `git tag plan-21-ai-modal-done`.

---

## 8. 위험 / 미해결

- **setEditable(false) 부작용**: 잠긴 동안 자동저장/포커스 관련 다른 effect 가 영향받는지 확인. 스모크 #4-5, #11.
- **commitGenerated 문단 분리**: `textToParagraphs` 가 빈 줄→문단, 단일 줄바꿈→hardBreak 정확히. plain text 보장 (mark 없음). plan task 에서 명시.
- **전체교체 setContent**: 현재 씬 전체를 교체 — undo 1회로 복구 가능해야 (Tiptap chain 단일 트랜잭션). 스모크 #3 에서 Cmd+Z 확인.
- **모달 열린 채 노드 전환 차단**: 사이드바 클릭이 모달 백드롭에 막히는지, 아니면 별도 가드 필요한지 확인.
- **Plan 20 인프라 제거**: GhostExtension/useGhostText/AIPromptBar 삭제 — 잔여 import 없도록 grep.
