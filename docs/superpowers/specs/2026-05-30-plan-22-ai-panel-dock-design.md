# Plan 22 — AI 패널 도킹 + 타깃 하이라이트 Design Spec

## 목적

Plan 21 의 중앙 모달 + 백드롭은 작업 대상(삽입 위치/대체 문단)과 전후 맥락을 가린다. 사용자가 "어디에 삽입/대체하는지, 전후가 어떤지" 살필 수 없다. 해결: 중앙 모달을 **우측 도킹 사이드 패널**로 바꿔 에디터를 항상 보이게 하고, 작업 대상을 **에디터 안에서 시각적으로 하이라이트**한다.

## Goals

1. AI UI 를 중앙 오버레이에서 **우측 도킹 패널**로 전환 (기존 EntitySheet/ThreadSheet 의 `ws-body` 우측 컬럼 레이아웃 재활용). 에디터는 좌측에 좁아진 채 **완전히 보이고 스크롤 가능**.
2. 백드롭 제거 — 에디터 가독성 확보.
3. 작업 대상 **타깃 하이라이트** (에디터 decoration):
   - 대체(replace): 선택 범위 색상 배경
   - 삽입(insert): 삽입 지점 깜박이는 세로바 마커
   - 전체교체(replaceAll): 씬 전체 옅은 틴트
4. 에디터는 `setEditable(false)` 유지 (실수 입력 방지) — 단 보이고 스크롤 가능. frozen 타깃 기준이라 클릭해도 하이라이트 고정.
5. Plan 21 의 3 모드 / commitGenerated / useAIGeneration / 변형 ×3 / ctx 체크리스트(인라인) / 가독성 솔리드 카드 모두 유지.

## Non-Goals

- 드래그 이동 (도킹으로 불필요).
- 패널 폭 사용자 조절.
- Engine 변경.
- useAIGeneration / commitGenerated / AIContextChecklist 로직 변경 (재활용만).

---

## 1. 레이아웃 전환

### 1.1 ws-body 우측 슬롯

현재 `ws-body` (App.css:447) 는 grid `1fr 220px` (기본 ctx-panel), `with-sheet` 면 `1fr 360px` (EntitySheet/ThreadSheet). AI 패널은 더 넓게 — 새 변형 `with-ai-panel` = `1fr 420px`.

우측 컬럼 렌더 우선순위 (Workspace JSX):
1. `aiPanel` 열림 → **AIPanel** (`with-ai-panel`)
2. else `entitySheetId` → EntitySheet (`with-sheet`)
3. else `threadSheetId` → ThreadSheet (`with-sheet`)
4. else → ContextPanel (기본)

AI 패널 열림 중엔 에디터가 잠겨(setEditable false) 멘션 더블클릭 등이 안 먹으므로 EntitySheet/ThreadSheet 가 새로 열릴 일 없음 — 충돌 없음.

### 1.2 에디터 가시성

- 에디터(`ws-editor`)는 좌측 `1fr`, `overflow-y: auto` — AI 패널 열려도 스크롤 가능.
- 사용자가 위아래 스크롤해 타깃 하이라이트 주변 전후 맥락 확인.
- `setEditable(false)` 는 입력만 막고 스크롤·읽기는 허용.

---

## 2. AITargetExtension (신규 경량 익스텐션)

`apps/desktop/src/components/editor/AITargetExtension.ts` — decoration 전용. GhostExtension 보다 단순 (스트리밍/변형 없음, 텍스트 commit 없음 — 순수 시각 하이라이트).

### 2.1 상태 & 명령

```ts
export type AITargetMode = "replace" | "insert" | "replaceAll";

interface AITargetState {
  mode: AITargetMode;
  from: number;
  to: number;
}

// Commands:
//   setAITarget(mode, from, to): show highlight
//   clearAITarget(): remove highlight
```

PluginKey 로 state 보유. null = 하이라이트 없음.

### 2.2 decoration 렌더

```ts
decorations(state) {
  const t = this.getState(state); // AITargetState | null
  if (!t) return DecorationSet.empty;
  if (t.mode === "insert") {
    // 삽입 지점 깜박이는 세로바 widget
    return DecorationSet.create(state.doc, [
      Decoration.widget(t.from, () => {
        const el = document.createElement("span");
        el.className = "ai-target-caret";
        return el;
      }, { side: 0 }),
    ]);
  }
  // replace / replaceAll: inline 배경 하이라이트
  const cls = t.mode === "replaceAll" ? "ai-target-all" : "ai-target-replace";
  // replaceAll 의 from/to 는 문서 전체 범위 (Workspace 가 0..doc.content.size 로 전달)
  const from = Math.max(1, t.from);
  const to = Math.min(state.doc.content.size, t.to);
  if (to <= from) return DecorationSet.empty;
  return DecorationSet.create(state.doc, [
    Decoration.inline(from, to, { class: cls }),
  ]);
}
```

(정확한 from/to 클램핑은 구현 task 에서. ProseMirror inline decoration 은 1..content.size 범위. replaceAll 은 Workspace 가 `{ from: 1, to: editor.state.doc.content.size }` 로 setAITarget 호출.)

### 2.3 CSS

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
@keyframes ai-target-blink { to { opacity: 0; } }
```

### 2.4 auto-clear

GhostExtension 의 docChanged auto-drop 와 달리, AI 패널 중엔 에디터가 잠겨 doc 이 안 변하므로 docChanged auto-clear 불필요. clearAITarget 는 Workspace 가 명시 호출 (accept/cancel). 단, commit(accept) 시 doc 이 바뀌므로 — commit 전에 clearAITarget 호출 순서 보장 (아래 3.2).

---

## 3. AIPanel (AIModal 대체)

`apps/desktop/src/components/ai/AIModal.tsx` → `AIPanel.tsx` (rename + 레이아웃 변경). 내용(모드 셀렉터/프롬프트/톤·길이·변형 chip/ctx 인라인 체크리스트/결과 카드/◀▶/수락·취소)은 Plan 21 그대로.

### 3.1 컨테이너 변경

- 중앙 오버레이 `.ai-modal-backdrop` + `.ai-modal` 제거.
- 우측 컬럼 패널 `.ai-panel` — `border-left`, 세로 full height, `overflow-y: auto`, 내부 패딩. EntitySheet 의 `.entity-sheet` 스타일 참고.
- 백드롭 없음 → `onMouseDown` 백드롭 닫기 제거. Esc / 취소 버튼만.
- Props 동일하되 `onCancel` 은 백드롭이 아닌 Esc/취소에서만.

### 3.2 commit 순서 (Workspace)

accept 시: **clearAITarget 먼저 → commitGenerated → setEditable(true) → 패널 닫기**. (clear 를 commit 전에 — commit 으로 doc offset 이 바뀌기 전에 하이라이트 제거.)

실제로는 commitGenerated 가 doc 을 바꾸므로 decoration 은 어차피 무효화됨. 안전하게 clearAITarget 를 commit 직전에 dispatch. 또는 commit 트랜잭션과 별개 트랜잭션으로 clear 후 commit. 구현 task 에서 순서 명시.

---

## 4. Workspace 통합

### 4.1 Cmd+I

```ts
const { from, to, empty } = ed.state.selection;
ed.setEditable(false);
const mode = empty ? "insert" : "replace";
ed.commands.setAITarget(mode, from, to);  // 타깃 하이라이트 ON
setAiModal({ mode, canChooseMode: empty, sel: { from, to } });
// previewContext fetch (Plan 19 동일)
```

### 4.2 모드 변경 (canChooseMode 면 삽입/전체교체 라디오)

라디오로 insert ↔ replaceAll 전환 시 타깃 하이라이트도 갱신:

```ts
onModeChange={(m) => {
  setAiModal((s) => (s ? { ...s, mode: m } : s));
  if (!tiptapEditor) return;
  if (m === "replaceAll") {
    tiptapEditor.commands.setAITarget("replaceAll", 1, tiptapEditor.state.doc.content.size);
  } else {
    // insert — 원래 커서 위치 (sel.from)
    tiptapEditor.commands.setAITarget("insert", aiModal!.sel.from, aiModal!.sel.from);
  }
}}
```

(replace 모드는 selection 있을 때만이라 canChooseMode=false — 라디오 안 뜸. setAITarget("replace", from, to) 는 Cmd+I 에서 1회.)

### 4.3 accept / close

```ts
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

// safety: 패널 닫혔는데 잠겨있으면 해제 + 혹시 남은 타깃 제거
useEffect(() => {
  if (aiModal === null && tiptapEditor && !tiptapEditor.isEditable) {
    tiptapEditor.commands.clearAITarget();
    tiptapEditor.setEditable(true);
  }
}, [aiModal, tiptapEditor]);
```

### 4.4 우측 슬롯 렌더 + ws-body class

```tsx
<div className={`ws-body${
  aiModal ? " with-ai-panel" :
  (entitySheetId || threadSheetId) ? " with-sheet" : ""
}`}>
  <div className="ws-editor">
    <TiptapEditor ... extensions={[...mention, NoteMarkerExtension, AITargetExtension]} />
  </div>
  {aiModal && load ? (
    <AIPanel ... />
  ) : entitySheetId ? (
    <EntitySheet ... />
  ) : threadSheetId ? (
    <ThreadSheet ... />
  ) : (
    <ContextPanel ... />
  )}
</div>
```

AIContextChecklist 인라인은 AIPanel 안 (Plan 21 fix `0ed540b` 그대로 — `showChecklist` + `checklistCounts` prop).

### 4.5 App.css

```css
.ws-body.with-ai-panel {
  grid-template-columns: 1fr 420px;
}
```

---

## 5. 에러 처리

| 상황 | 처리 |
|---|---|
| 빈 프롬프트 | 흔들기 (Plan 21 동일) |
| 변형 부분 에러 | 카드 `(오류:...)` (Plan 21 동일) |
| 패널 unmount 시 잠금/타깃 누락 | safety effect 에서 clearAITarget + setEditable(true) |
| 노드 전환 중 패널 열림 | Plan 21 하드닝 — load.node.id 변경 시 closeAIModal (clearAITarget 포함) |
| replaceAll 빈 문서 (content.size ≤ 1) | decoration to≤from 이면 빈 set (하이라이트 없음) — 정상 |

---

## 6. 제거 / 신규

| 파일 | 처리 |
|---|---|
| `AIModal.tsx` → `AIPanel.tsx` | rename + 레이아웃 (중앙→우측 도킹, 백드롭 제거) |
| `AIModal.css` → `AIPanel.css` | 컨테이너 스타일 변경 (.ai-panel) |
| `AITargetExtension.ts` + `.css` | **신규** |
| `Workspace.tsx` | 우측 슬롯 렌더, setAITarget/clearAITarget, ws-body class, extensions |
| `App.css` | `.with-ai-panel` 추가 |
| `useAIGeneration.ts` / `commitGenerated.ts` / `AIContextChecklist.tsx` | 변경 없음 |

엔진: 변경 없음.

---

## 7. 테스트 전략

엔진 변경 없음 — FE 타입체크 + 수동 스모크.

### 수동 스모크

**도킹 + 가시성:**
1. Cmd+I → 우측 패널 열림, 에디터 좌측에 좁아진 채 보임. 위아래 스크롤 → 타깃 주변 전후 맥락 확인 가능.

**타깃 하이라이트:**
2. 선택 영역 + Cmd+I → 선택 범위가 파란 배경 하이라이트 (대체 타깃). 패널 "모드: 대체".
3. 선택 없이 Cmd+I → 커서 위치에 깜박이는 세로바 (삽입 타깃). 라디오 삽입.
4. 라디오 전체교체 선택 → 씬 전체 옅은 틴트로 바뀜. 다시 삽입 → 세로바로 복귀.

**commit:**
5. 대체 생성 → 수락 → 하이라이트 사라지고 선택 영역 교체.
6. 삽입 생성 → 수락 → 세로바 위치에 삽입.
7. 전체교체 생성 → 수락 → 씬 전체 교체. Cmd+Z 1회 복구.

**잠금:**
8. 패널 열린 중 본문 타이핑 시도 → 입력 안 됨(잠김), 스크롤은 됨.
9. 취소/Esc → 하이라이트 제거 + 편집 가능 복구.

**변형 / ctx:**
10. 변형 ×3 → 패널 안 ◀▶, 수락 → 선택 카드 commit + 나머지 cancel.
11. ctx 칩 → 패널 안 인라인 체크리스트 (Plan 21 fix). 실제 카운트.

**회귀:**
12. Cmd+P/Cmd+R 패널 중 차단. 노드 전환 시 패널 닫힘 + 잠금 해제. EntitySheet/ThreadSheet 정상 (패널 안 열렸을 때).

통과 시 `git tag plan-22-ai-panel-dock-done`.

---

## 8. 위험 / 미해결

- **inline decoration 범위**: ProseMirror inline decoration 은 텍스트 범위 1..content.size. replace 의 from/to 가 노드 경계 걸칠 때 클램핑 필요 (구현 task 에서 Math.max/min).
- **setEditable(false) 중 스크롤**: ProseMirror non-editable 도 스크롤 OK 확인 (스모크 #1).
- **모드 라디오 전환 시 타깃 갱신**: insert↔replaceAll 전환마다 setAITarget 재호출 — aiModal.sel.from 보존 필요 (insert 복귀 시 원래 커서).
- **commit 전 clearAITarget 순서**: clear 먼저 dispatch 후 commit. 둘 다 트랜잭션이라 순서 보장.
- **우측 패널 폭 420px** 가 작은 화면에서 에디터를 너무 좁게 — 첫 버전 고정, 추후 반응형.
