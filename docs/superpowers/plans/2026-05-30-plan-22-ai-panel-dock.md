# Plan 22 — AI 패널 도킹 + 타깃 하이라이트 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 중앙 모달 AI UI 를 우측 도킹 사이드 패널로 전환해 에디터를 항상 보이게 하고, 작업 대상(삽입 지점/대체 범위/전체 씬)을 에디터 안에서 시각적으로 하이라이트한다.

**Architecture:** 새 경량 `AITargetExtension` 이 ProseMirror decoration 으로 타깃을 하이라이트한다. `AIModal` 을 `AIPanel` 로 바꿔 중앙 오버레이 대신 `ws-body` 우측 컬럼(EntitySheet/ThreadSheet 패턴 재활용)에 렌더한다. Workspace 가 Cmd+I 에서 selection 캡처 + setEditable(false) + setAITarget, 수락/취소에서 clearAITarget + setEditable(true). useAIGeneration / commitGenerated / AIContextChecklist 는 재활용(무변경). 엔진 무변경.

**Tech Stack:** TypeScript / React 18, Tiptap 2 + ProseMirror, Tauri JSONRPC.

---

## 파일 구조

**신규:**
- `apps/desktop/src/components/editor/AITargetExtension.ts` — 타깃 하이라이트 decoration extension
- `apps/desktop/src/components/editor/AITargetExtension.css`

**rename + 수정:**
- `apps/desktop/src/components/ai/AIModal.tsx` → `AIPanel.tsx` (컨테이너 중앙→우측 도킹, 백드롭 제거)
- `apps/desktop/src/components/ai/AIModal.css` → `AIPanel.css`

**수정:**
- `apps/desktop/src/routes/Workspace.tsx` — 우측 슬롯 렌더, AITargetExtension 추가, setAITarget/clearAITarget 연결, ws-body class
- `apps/desktop/src/App.css` — `.ws-body.with-ai-panel` 추가

**무변경:** `useAIGeneration.ts`, `commitGenerated.ts`, `AIContextChecklist.tsx`. 엔진 전체.

**FE 테스트 인프라:** vitest 미설치 — 타입체크 + 수동 스모크.

---

## Task 1: AITargetExtension (타깃 하이라이트 decoration)

**Files:**
- Create: `apps/desktop/src/components/editor/AITargetExtension.ts`
- Create: `apps/desktop/src/components/editor/AITargetExtension.css`

이 task 는 standalone 신규 익스텐션. Workspace 에 아직 안 붙이므로(미사용) 컴파일만 통과하면 됨.

### Step 1: AITargetExtension.ts 작성

Create `apps/desktop/src/components/editor/AITargetExtension.ts`:

```ts
import { Extension, type RawCommands } from "@tiptap/core";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import { Decoration, DecorationSet } from "@tiptap/pm/view";
import "./AITargetExtension.css";

declare module "@tiptap/core" {
  interface Commands<ReturnType> {
    aiTarget: {
      /** Show the target highlight for the AI panel. */
      setAITarget: (mode: AITargetMode, from: number, to: number) => ReturnType;
      /** Remove the target highlight. */
      clearAITarget: () => ReturnType;
    };
  }
}

export type AITargetMode = "replace" | "insert" | "replaceAll";

export interface AITargetState {
  mode: AITargetMode;
  from: number;
  to: number;
}

export const aiTargetPluginKey = new PluginKey<AITargetState | null>("linetta-ai-target");

type AITargetMeta =
  | { kind: "set"; mode: AITargetMode; from: number; to: number }
  | { kind: "clear" };

export const AITargetExtension = Extension.create({
  name: "linettaAITarget",

  addProseMirrorPlugins() {
    return [
      new Plugin<AITargetState | null>({
        key: aiTargetPluginKey,
        state: {
          init: () => null,
          apply(tr, prev) {
            const meta = tr.getMeta(aiTargetPluginKey) as AITargetMeta | undefined;
            if (meta?.kind === "set") {
              return { mode: meta.mode, from: meta.from, to: meta.to };
            }
            if (meta?.kind === "clear") {
              return null;
            }
            // Map stored positions through doc changes (defensive; editor is
            // normally locked while a target is active so docChanged is rare).
            if (prev && tr.docChanged) {
              return {
                ...prev,
                from: tr.mapping.map(prev.from),
                to: tr.mapping.map(prev.to),
              };
            }
            return prev;
          },
        },
        props: {
          decorations(state) {
            const t = this.getState(state);
            if (!t) return DecorationSet.empty;
            const size = state.doc.content.size;
            if (t.mode === "insert") {
              const pos = Math.min(Math.max(0, t.from), size);
              return DecorationSet.create(state.doc, [
                Decoration.widget(
                  pos,
                  () => {
                    const el = document.createElement("span");
                    el.className = "ai-target-caret";
                    return el;
                  },
                  { side: 0 },
                ),
              ]);
            }
            const cls = t.mode === "replaceAll" ? "ai-target-all" : "ai-target-replace";
            const from = Math.max(1, Math.min(t.from, size));
            const to = Math.max(1, Math.min(t.to, size));
            if (to <= from) return DecorationSet.empty;
            return DecorationSet.create(state.doc, [
              Decoration.inline(from, to, { class: cls }),
            ]);
          },
        },
      }),
    ];
  },

  addCommands() {
    return {
      setAITarget:
        (mode: AITargetMode, from: number, to: number) =>
        ({ tr, dispatch }) => {
          if (dispatch) {
            dispatch(
              tr.setMeta(aiTargetPluginKey, { kind: "set", mode, from, to } satisfies AITargetMeta),
            );
          }
          return true;
        },
      clearAITarget:
        () =>
        ({ tr, state, dispatch }) => {
          if (aiTargetPluginKey.getState(state) === null) return false;
          if (dispatch) {
            dispatch(tr.setMeta(aiTargetPluginKey, { kind: "clear" } satisfies AITargetMeta));
          }
          return true;
        },
    } as Partial<RawCommands>;
  },
});
```

### Step 2: CSS 작성

Create `apps/desktop/src/components/editor/AITargetExtension.css`:

```css
.ai-target-replace {
  background: rgba(120, 170, 255, 0.22);
  border-radius: 2px;
}

.ai-target-all {
  background: rgba(120, 170, 255, 0.10);
}

.ai-target-caret {
  display: inline-block;
  width: 2px;
  height: 1.1em;
  margin: 0 -1px;
  background: rgba(120, 170, 255, 0.9);
  vertical-align: text-bottom;
  animation: ai-target-blink 1s steps(2) infinite;
}

@keyframes ai-target-blink {
  to { opacity: 0; }
}
```

### Step 3: 타입체크

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop && npx tsc --noEmit
```

기대: 에러 없음. (`@tiptap/core`, `@tiptap/pm/state`, `@tiptap/pm/view` import 는 기존 익스텐션 — 예전 GhostExtension/FocusExtension — 과 동일 경로. `@tiptap/pm` 은 직접 의존.)

### Step 4: 커밋

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/src/components/editor/AITargetExtension.ts apps/desktop/src/components/editor/AITargetExtension.css
git commit -m "feat(editor): AITargetExtension — highlight AI target (replace/insert/replaceAll)"
```

## Context

Plan 22 Task 1. 우측 도킹 AI 패널이 작업 대상을 가리지 않으면서, 대상을 에디터 안에서 보여주기 위한 하이라이트 익스텐션. 3 모드:
- replace: 선택 범위 inline 배경 (`.ai-target-replace`)
- insert: 커서 위치 widget caret (`.ai-target-caret`)
- replaceAll: 문서 전체 옅은 틴트 (`.ai-target-all`)

GhostExtension 은 Plan 21 에서 삭제됨 — 이건 더 단순 (decoration 만, 텍스트 commit/스트리밍 없음). FocusExtension (Plan 12) 이 유사한 decoration 패턴 — import 경로 참고 (`@tiptap/pm/state`, `@tiptap/pm/view`).

이 task 는 익스텐션 정의만 — Workspace 연결은 Task 3.

Working directory: `/Users/changheonshin/workspace/myworks/linetta`. Branch: `main`. Never push. Never skip hooks.

## Report Format

- Status DONE | DONE_WITH_CONCERNS | BLOCKED
- 타입체크 결과 (Tiptap import 경로 이슈 있었는지)
- 커밋 SHA
- 우려사항

---

## Task 2: AIModal → AIPanel (우측 도킹 레이아웃)

**Files:**
- Rename + modify: `apps/desktop/src/components/ai/AIModal.tsx` → `AIPanel.tsx`
- Rename + modify: `apps/desktop/src/components/ai/AIModal.css` → `AIPanel.css`
- Modify: `apps/desktop/src/App.css` (+ `.with-ai-panel`)
- Modify: `apps/desktop/src/routes/Workspace.tsx` (import 변경 + JSX 를 ws-body 우측 슬롯으로 이동)

이 task 는 중앙 오버레이를 우측 도킹 컬럼으로 바꾼다. 타깃 하이라이트 연결은 Task 3 (이 task 는 레이아웃만 — 패널 열리고 닫히고 생성/수락 동작).

### Step 1: 파일 rename

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git mv apps/desktop/src/components/ai/AIModal.tsx apps/desktop/src/components/ai/AIPanel.tsx
git mv apps/desktop/src/components/ai/AIModal.css apps/desktop/src/components/ai/AIPanel.css
```

### Step 2: AIPanel.tsx 내부 갱신

`apps/desktop/src/components/ai/AIPanel.tsx` 에서:

1. export 이름 `AIModal` → `AIPanel`:
```tsx
export function AIPanel(props: Props) {
```

2. import css 경로:
```tsx
import "./AIPanel.css";
```

3. 최상위 컨테이너 — 중앙 오버레이 백드롭 제거. 현재:
```tsx
  return (
    <div className="ai-modal-backdrop" onMouseDown={props.onCancel}>
      <div
        className="ai-modal"
        onMouseDown={(e) => e.stopPropagation()}
        onKeyDown={onKeyDown}
      >
        ... 내용 ...
      </div>
    </div>
  );
```
을 다음으로 (백드롭 div 제거, 단일 패널 aside):
```tsx
  return (
    <aside className="ai-panel" onKeyDown={onKeyDown}>
      ... 내용 (그대로) ...
    </aside>
  );
```
(내부 `.ai-modal-*` className 들은 그대로 두되 — Step 3 에서 css 클래스명을 `.ai-panel-*` 로 일괄 변경하거나, 간단히 css 만 `.ai-modal` → `.ai-panel` 컨테이너 규칙 추가. 최소 변경 위해: 컨테이너만 `.ai-panel` 로 바꾸고 내부 자식 클래스 `.ai-modal-*` 는 유지. CSS 에서 `.ai-modal-backdrop`/`.ai-modal` 규칙을 `.ai-panel` 규칙으로 교체, 내부 `.ai-modal-*` 규칙은 그대로 둠.)

### Step 3: AIPanel.css 컨테이너 규칙 교체

`apps/desktop/src/components/ai/AIPanel.css` 에서 `.ai-modal-backdrop` 와 `.ai-modal` 규칙을 다음으로 교체 (나머지 `.ai-modal-*` 내부 규칙은 그대로 유지):

```css
.ai-panel {
  border-left: 1px solid rgba(255, 255, 255, 0.1);
  background: var(--surface, #1d1d1f);
  color: var(--text, #e8e8ea);
  height: 100%;
  display: flex;
  flex-direction: column;
  gap: 0.7rem;
  padding: 1rem 1.1rem;
  overflow-y: auto;
  font-size: 0.9rem;
}
```

(기존 `.ai-modal-backdrop { position: fixed; inset: 0; ... }` 와 `.ai-modal { ... width: min(640px,...); box-shadow; ... }` 두 블록을 위 `.ai-panel` 하나로 대체. `.ai-modal-modes`, `.ai-modal-textarea`, `.ai-modal-chip`, `.ai-modal-result`, `.ai-modal-footer`, `.ai-modal-btn` 등 내부 규칙은 손대지 않음 — tsx 의 자식 className 이 그대로이므로.)

### Step 4: Workspace import + JSX 이동

`apps/desktop/src/routes/Workspace.tsx`:

1. import 변경:
```tsx
import { AIPanel } from "../components/ai/AIPanel";
```
(기존 `import { AIModal } from "../components/ai/AIModal";` 교체.)

2. ws-body className 에 `with-ai-panel` 추가 — 현재:
```tsx
<div className={`ws-body${(entitySheetId || threadSheetId) ? " with-sheet" : ""}`}>
```
을:
```tsx
<div className={`ws-body${
  aiModal ? " with-ai-panel" : (entitySheetId || threadSheetId) ? " with-sheet" : ""
}`}>
```

3. 우측 슬롯 렌더에 AIPanel 을 최우선 분기로 추가. 현재 우측 슬롯은:
```tsx
        {entitySheetId ? (
          <EntitySheet ... />
        ) : threadSheetId ? (
          <ThreadSheet ... />
        ) : (
          <ContextPanel ... />
        )}
```
을 다음으로 (aiModal 분기를 맨 앞에):
```tsx
        {aiModal && load ? (
          <AIPanel
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
            showChecklist={aiCtxChecklistOpen}
            checklistCounts={contextCounts ?? FALLBACK_COUNTS}
          />
        ) : entitySheetId ? (
          <EntitySheet ... />        // 기존 그대로
        ) : threadSheetId ? (
          <ThreadSheet ... />        // 기존 그대로
        ) : (
          <ContextPanel ... />       // 기존 그대로
        )}
```

4. 기존의 ws-body **밖** 에 있던 `{aiModal && load && (<AIModal ... />)}` 블록 (라인 825-857 부근) 을 **완전 삭제** (이제 ws-body 안으로 옮겼으므로). 중복 렌더 방지.

### Step 5: 타입체크

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop && npx tsc --noEmit
```

기대: 에러 없음. 잔여 `AIModal` 참조 0 확인:
```bash
grep -rn "AIModal" /Users/changheonshin/workspace/myworks/linetta/apps/desktop/src --include="*.ts" --include="*.tsx"
```
기대: 0 (모두 AIPanel 로). 남으면 교체.

### Step 6: App.css 추가

`apps/desktop/src/App.css` 의 `.ws-body.with-sheet` 규칙 옆에 추가:

```css
.ws-body.with-ai-panel {
  grid-template-columns: 1fr 420px;
}
```

### Step 7: 커밋

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/src/components/ai/AIPanel.tsx apps/desktop/src/components/ai/AIPanel.css apps/desktop/src/routes/Workspace.tsx apps/desktop/src/App.css
git commit -m "feat(ai): dock AI panel to right column (AIModal → AIPanel), drop backdrop"
```

## Context

Plan 22 Task 2. 중앙 오버레이 + 백드롭 → 우측 도킹 컬럼. 기존 EntitySheet/ThreadSheet 가 쓰는 `ws-body` grid 우측 컬럼 패턴 재활용. 에디터는 좌측 `1fr` 에 좁아진 채 보이고 스크롤 가능.

AIPanel 내부 내용(모드 셀렉터, 프롬프트, chip, 결과 카드, ◀▶, 수락/취소, 인라인 ctx 체크리스트)은 Plan 21 그대로 — 컨테이너만 백드롭→aside 로. 백드롭 제거로 백드롭-클릭 닫기 사라짐 (Esc/취소 버튼만, 키핸들러는 `onKeyDown` 에 이미 있음).

타깃 하이라이트(setAITarget/clearAITarget) 연결은 Task 3 — 이 task 는 레이아웃 전환만.

`acceptAIModal`, `closeAIModal`, `aiModal` state, `gen`, `aiOptions`, `contextCounts`, `aiCtxChecklistOpen` 모두 기존 (Plan 21). 변수명 유지.

Working directory: `/Users/changheonshin/workspace/myworks/linetta`. Branch: `main`. Never push. Never skip hooks.

## Report Format

- Status DONE | DONE_WITH_CONCERNS | BLOCKED
- 잔여 AIModal grep 결과 (0이어야)
- 타입체크 결과
- 커밋 SHA
- 우려사항

---

## Task 3: Workspace — 타깃 하이라이트 연결

**Files:**
- Modify: `apps/desktop/src/routes/Workspace.tsx`

AITargetExtension 을 에디터에 붙이고, Cmd+I / 모드변경 / accept / close 에서 setAITarget / clearAITarget 호출.

### Step 1: import + extensions 배열

`apps/desktop/src/routes/Workspace.tsx` 상단에 추가:
```tsx
import { AITargetExtension } from "../components/editor/AITargetExtension";
```

TiptapEditor 의 `extensions` 배열에 추가:
```tsx
extensions={[
  ...(mentionExtension ? [mentionExtension] : []),
  NoteMarkerExtension,
  AITargetExtension,
]}
```

### Step 2: Cmd+I 에서 setAITarget

Cmd+I 핸들러 (전역 keydown, `else if (e.key.toLowerCase() === "i")` 블록) 에서 `ed.setEditable(false)` 직후, `setAiModal(...)` 직전에 추가:

```tsx
        const { from, to, empty } = ed.state.selection;
        ed.setEditable(false);
        const mode = empty ? "insert" : "replace";
        ed.commands.setAITarget(mode, from, to);
        setAiModal({
          mode,
          canChooseMode: empty,
          sel: { from, to },
        });
```

(기존 코드는 `mode: empty ? "insert" : "replace"` 를 setAiModal 안에서 인라인 계산했을 수 있음 — `mode` 지역변수로 빼서 setAITarget 와 setAiModal 둘 다에 사용.)

### Step 3: 모드 변경 시 타깃 갱신

우측 슬롯 AIPanel 의 `onModeChange` 를 다음으로 교체 (Task 2 에서는 단순 setAiModal 만 했음):

```tsx
            onModeChange={(m) => {
              setAiModal((s) => (s ? { ...s, mode: m } : s));
              if (!tiptapEditor || !aiModal) return;
              if (m === "replaceAll") {
                tiptapEditor.commands.setAITarget("replaceAll", 1, tiptapEditor.state.doc.content.size);
              } else if (m === "insert") {
                tiptapEditor.commands.setAITarget("insert", aiModal.sel.from, aiModal.sel.from);
              } else {
                tiptapEditor.commands.setAITarget("replace", aiModal.sel.from, aiModal.sel.to);
              }
            }}
```

(canChooseMode=true 일 때만 라디오로 insert↔replaceAll 전환. replace 는 selection 있을 때 고정이라 라디오 안 뜸 — 하지만 방어적으로 세 분기 모두 둠.)

### Step 4: accept / close / safety 에 clearAITarget

`acceptAIModal` (라인 438 부근) — `commitGenerated` **직전**에 clearAITarget:
```tsx
  const acceptAIModal = useCallback(() => {
    if (!aiModal || !tiptapEditor) return;
    const v = gen.variations[gen.currentIdx];
    if (!v || v.error) return;
    tiptapEditor.commands.clearAITarget();
    commitGenerated(tiptapEditor, aiModal.mode, aiModal.sel, v.text);
    gen.cancel();
    tiptapEditor.setEditable(true);
    setAiModal(null);
    setContextCounts(null);
    setAiCtxChecklistOpen(false);
    previewReqIdRef.current++;
  }, [aiModal, gen, tiptapEditor]);
```

`closeAIModal` (라인 420 부근):
```tsx
  const closeAIModal = useCallback(() => {
    gen.cancel();
    if (tiptapEditor) {
      tiptapEditor.commands.clearAITarget();
      tiptapEditor.setEditable(true);
    }
    setAiModal(null);
    setContextCounts(null);
    setAiCtxChecklistOpen(false);
    previewReqIdRef.current++;
  }, [gen, tiptapEditor]);
```

safety effect (라인 452 부근):
```tsx
  useEffect(() => {
    if (aiModal === null && tiptapEditor && !tiptapEditor.isEditable) {
      tiptapEditor.commands.clearAITarget();
      tiptapEditor.setEditable(true);
    }
  }, [aiModal, tiptapEditor]);
```

### Step 5: 타입체크 + 엔진 빌드

```bash
cd /Users/changheonshin/workspace/myworks/linetta/apps/desktop && npx tsc --noEmit
cd /Users/changheonshin/workspace/myworks/linetta/engine && go test ./...
```

기대: 타입체크 clean, 엔진 테스트 PASS (엔진 무변경 — 회귀 확인).

### Step 6: 커밋

```bash
cd /Users/changheonshin/workspace/myworks/linetta
git add apps/desktop/src/routes/Workspace.tsx
git commit -m "feat(workspace): wire AI target highlight (setAITarget on open/mode, clear on accept/close)"
```

## Context

Plan 22 Task 3 (최종 구현). Task 1 의 AITargetExtension 을 에디터에 붙이고, Task 2 의 도킹 패널과 연동해 타깃 하이라이트를 켜고 끈다.

- Cmd+I: selection 캡처 + setEditable(false) + setAITarget(mode, from, to).
- 모드 라디오 전환 (canChooseMode): insert↔replaceAll 에 맞춰 setAITarget 재호출. insert 는 aiModal.sel.from (원래 커서).
- accept: clearAITarget → commitGenerated (clear 먼저, commit 으로 doc 바뀌기 전).
- close/safety: clearAITarget + setEditable(true).

`AITargetMode` 는 `commitGenerated` 의 `CommitMode` 와 동일 union ("replace"|"insert"|"replaceAll") — aiModal.mode 그대로 전달 가능.

`tiptapEditor.commands.setAITarget` / `clearAITarget` 는 Task 1 의 익스텐션이 declare module 로 타입 등록.

Working directory: `/Users/changheonshin/workspace/myworks/linetta`. Branch: `main`. Never push. Never skip hooks.

## Report Format

- Status DONE | DONE_WITH_CONCERNS | BLOCKED
- 타입체크 / 엔진 테스트 결과
- 커밋 SHA
- 우려사항

---

## 통합 검증 (Task 3 직후 수동 스모크)

```bash
rm -rf /tmp/linetta-plan22 && LINETTA_HOME=/tmp/linetta-plan22 ./scripts/dev.sh
```

1. **도킹 + 가시성**: Cmd+I → 우측 패널, 에디터 좌측 보임. 위아래 스크롤 → 타깃 전후 맥락 확인.
2. **대체 타깃**: 선택 + Cmd+I → 선택 범위 파란 하이라이트. "모드: 대체".
3. **삽입 타깃**: 선택 없이 Cmd+I → 커서에 깜박이는 세로바. 라디오 삽입.
4. **전체교체 타깃**: 라디오 전체교체 → 씬 전체 옅은 틴트. 삽입 복귀 → 세로바.
5. **commit**: 대체/삽입/전체교체 각각 생성→수락 → 하이라이트 사라지고 올바른 위치에 반영. 전체교체 Cmd+Z 1회 복구.
6. **잠금**: 패널 중 타이핑 안 됨, 스크롤 됨. 취소/Esc → 하이라이트 제거 + 편집 가능.
7. **변형 / ctx**: 변형 ×3 ◀▶, ctx 칩 인라인 체크리스트.
8. **회귀**: Cmd+P/R 차단, 노드 전환 시 패널 닫힘+하이라이트 제거, EntitySheet/ThreadSheet 정상(패널 안 열렸을 때).

통과 시:
```bash
git tag plan-22-ai-panel-dock-done
```

---

## Self-Review

**1. Spec 커버리지:**

| Spec 요구 | Task |
|---|---|
| AITargetExtension (replace/insert/replaceAll decoration) | Task 1 |
| setAITarget / clearAITarget 명령 | Task 1 |
| 타깃 CSS (replace 배경/insert caret/all 틴트) | Task 1 |
| 중앙 모달 → 우측 도킹 (AIPanel) | Task 2 |
| 백드롭 제거 | Task 2 |
| ws-body with-ai-panel grid | Task 2 (App.css) |
| 우측 슬롯 렌더 우선순위 (aiPanel > sheet > ctx) | Task 2 |
| extensions 에 AITargetExtension | Task 3 |
| Cmd+I setAITarget + setEditable(false) | Task 3 |
| 모드 라디오 전환 시 타깃 갱신 | Task 3 |
| accept clearAITarget→commit | Task 3 |
| close/safety clearAITarget + setEditable(true) | Task 3 |
| 에디터 보임·스크롤 (setEditable false 중) | Task 2 레이아웃 + 스모크 #1,6 |
| 3 모드 / commitGenerated / useAIGeneration / 변형 / ctx 인라인 재활용 | 무변경 (Plan 21) |
| 수동 스모크 8 시나리오 | Task 3 직후 |

모든 spec 요구 매핑.

**2. Placeholder scan:** Task 2 의 `<EntitySheet ... />` 등 `...` 는 "기존 그대로" 주석과 함께 — 기존 JSX 보존 지시 (placeholder 아님). 나머지 코드 블록 완전. "TBD"/"TODO" 없음.

**3. Type 일관성:**
- `AITargetMode` ("replace"|"insert"|"replaceAll") — Task 1 정의. `commitGenerated` 의 `CommitMode` 와 동일 union (Plan 21) → aiModal.mode 직접 전달.
- `setAITarget(mode, from, to)` / `clearAITarget()` — Task 1 declare module, Task 3 호출. 일치.
- `aiTargetPluginKey` — Task 1 내부. Task 3 은 commands 만 사용.
- `AIPanel` props — Plan 21 AIModal Props 와 동일 (Task 2 는 rename 만, prop 시그니처 무변경). Task 2 의 마운트가 모든 prop 전달.
- `.ai-panel` (Task 2 컨테이너) — `.ai-modal-*` 내부 클래스는 tsx 유지 → css 내부 규칙 유지. 일관.

체크 통과.

**4. 위험:**
- Task 2 의 rename(git mv) + 내부 export 변경 + Workspace import 변경이 한 task — 중간에 깨진 상태 없도록 Step 1→4 순서 준수. typecheck 는 Step 5 에서.
- Task 2 에서 ws-body 밖 기존 AIModal 마운트 블록 삭제 누락 시 중복 렌더 — Step 4-4 명시.
- inline decoration from/to 클램핑 (Task 1 Math.max/min) — replaceAll content.size, 빈 문서 to≤from 가드.
