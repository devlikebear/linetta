# Plan 20 — AI 변형 비교 (Ghost Variations) Design Spec

## 목적

현재 Cmd+I → AIPromptBar → 단일 ghost text → Tab 자동 commit 흐름에서, 사용자가 옵트인하면 **같은 prompt + 컨텍스트로 3개 변형을 병렬 생성** 하고 ghost 안에서 ◀▶ 로 전환하며 비교 후 하나를 선택해 commit 한다.

## Goals

1. `AIPromptBar` 에 `변형` 토글 chip 추가. 활성 시 생성 → 3개 변형 병렬 호출.
2. `GhostExtension` 의 state 를 단일 텍스트에서 `variations: GhostVariation[]` 으로 일반화. 단일 모드 = `variations.length === 1` 의 특수 케이스로 backward compat.
3. ghost widget 안에 `[i/N] ◀ ▶ Tab 수락` 인디케이터 (N>1 일 때만 표시).
4. ◀▶ (ArrowLeft/ArrowRight) 키바인딩으로 variation 전환. Tab 으로 현재 보이는 variation accept. Esc 로 모두 cancel + drop. Cmd+Enter 로 모든 variation 재생성.
5. 변형 모드에서 ai-done 은 자동 commit **하지 않는다**. 사용자가 명시적 Tab.
6. 부분 에러 (한 변형 실패) 시 그 variation 만 회색 `(오류: ...)` 표시, 나머지 진행.
7. 단일 모드 회귀 무, Plan 18 동작 보존.

## Non-Goals

- 변형 N 의 사용자 설정 (settings UI). 항상 3 고정. 추후 별도 plan.
- 변형 N개를 동시에 화면에 보여주기. 한 번에 하나만 (◀▶ 로 swap).
- Diff view 또는 word-level 비교 highlight. 텍스트 그대로.
- 변형끼리 다른 옵션 (예: variation A 는 차갑게, B 는 서정으로). 같은 prompt + 같은 옵션, LLM 의 nondeterminism 으로 다양성 확보.
- Engine 측 변경. `ai.run` RPC 가 N번 병렬로 호출됨.

---

## 1. 아키텍처

### 1.1 데이터 흐름

```
User: Cmd+I → AIPromptBar 표시
   │
   ▼
User: 변형 chip 토글 on → prompt 입력 → Enter
   │
   ▼
Workspace onRun(preset, prompt, variationsOn=true) 콜백
   │
   ▼
ghost.startVariations({nodeId, prompt, options, selectionText, replaceRange}, 3)
   │
   ├─ 1. 기존 in-flight runs 모두 ai.cancel
   ├─ 2. activeRunIdsRef.current 비움, runIdToVariationRef.current.clear()
   ├─ 3. editor.commands.dropGhostText() (decoration 초기화)
   ├─ 4. editor.commands.setGhostVariations(3, mode)  // 빈 3 슬롯 + currentIdx=0
   └─ 5. Array(3).map((_, i) => ai.run(...).then(({run_id}) => {
          activeRunIdsRef.current.push(run_id);
          runIdToVariationRef.current.set(run_id, i);
        }))
   │
   ▼ (병렬 N개 ai.run 시작)
   │
engine: ai-delta / ai-done / ai-error / ai-cancelled / ai-reset 노티 스트림
   │  각 이벤트의 p.run_id 로 어느 variation 인지 매핑
   ▼
useGhostText 이벤트 핸들러:
   ai-delta → setGhostVariationText(idx, existing.variations[idx].text + p.text)
   ai-reset → setGhostVariationText(idx, p.text)  // streamDedup reconcile
   ai-done → setGhostVariationText(idx, p.full_text) + setGhostVariationDone(idx)
   ai-error → setGhostVariationDone(idx, p.message)
   ai-cancelled → (idx 매핑 제거, status 영향 X — 다른 variation 영향 X)
   │
   ▼
   사용자: ◀▶ → switchGhostVariation(direction) → currentIdx 갱신 → widget 재렌더
   사용자: Tab → acceptGhostText() → currentIdx variation 의 text 를 doc commit + dropGhostText
   사용자: Esc → dropGhostText() + 모든 activeRunIds cancel
   사용자: Cmd+Enter → 새 startVariations (이전 모두 cancel + drop)
```

### 1.2 단일 모드 (variation chip off)

기존 Plan 18 `start()` 흐름 그대로. `setGhostText(text, mode)` 가 내부적으로 `setGhostVariations(1, mode) + setGhostVariationText(0, text)` 와 등가가 되도록 backward-compat 매핑.

`ai-done` 시 자동 commit + bar 닫기 (Plan 18 fixup 동작) 가 **variations.length === 1 일 때만** 발화. 변형 모드는 명시적 Tab.

---

## 2. GhostExtension 변경

### 2.1 새 타입

`apps/desktop/src/components/editor/GhostExtension.ts`:

```ts
export interface GhostVariation {
  text: string;
  done: boolean;
  /** Optional per-variation error message; if set, also treated as done. */
  error?: string;
}

export interface GhostState {
  mode: GhostMode;  // 기존: { kind: "insert" | "replace"; ... }
  variations: GhostVariation[];
  currentIdx: number;
}
```

### 2.2 명령 (Tiptap `Commands` 인터페이스 확장)

| 명령 | 시그니처 | 동작 |
|---|---|---|
| `setGhostText` | `(text: string, mode?: GhostMode) => ReturnType` | 기존 — 단일 variation init. Backward compat. 내부 구현: state 가 `{variations: [{text, done: false}], currentIdx: 0}` 로 set. |
| `setGhostVariations` | `(count: number, mode: GhostMode) => ReturnType` | N 변형 슬롯 init. 모두 `{text: "", done: false}`. currentIdx=0. |
| `setGhostVariationText` | `(idx: number, text: string) => ReturnType` | 특정 variation 텍스트 갱신. idx out of range 면 no-op + return false. |
| `setGhostVariationDone` | `(idx: number, error?: string) => ReturnType` | 변형 완료 마킹. error 옵션 — `(오류: ...)` 표시 + done=true. |
| `switchGhostVariation` | `(direction: -1 \| 1) => ReturnType` | currentIdx 이동 (wrap modulo variations.length). variations.length === 1 이면 no-op. |
| `acceptGhostText` | `() => ReturnType` | 기존 — `variations[currentIdx].text` 를 doc commit (plain text 강제, schema.text(text) + replaceWith). error 가 있는 variation 이면 no-op + return false. |
| `dropGhostText` | `() => ReturnType` | 기존 — state null. (모든 in-flight cancel 은 useGhostText 쪽 책임.) |

### 2.3 Plugin state apply (meta 종류)

기존 `set` / `drop` / `done` 에 추가:

| meta kind | payload | apply 동작 |
|---|---|---|
| `set` (기존) | `{mode, text}` | 단일 모드 init: `{variations: [{text, done: false}], currentIdx: 0, mode}` |
| `setVariations` (NEW) | `{count, mode}` | N 슬롯 init: `{variations: Array(count).fill({text:"",done:false}), currentIdx: 0, mode}` |
| `setVariationText` (NEW) | `{idx, text}` | `variations[idx].text = text` (immutable copy) |
| `setVariationDone` (NEW) | `{idx, error?}` | `variations[idx] = {...variations[idx], done: true, error}` |
| `switchVariation` (NEW) | `{direction}` | `currentIdx = (currentIdx + direction + N) % N`. N=1 이면 unchanged. |
| `drop` (기존) | — | state = null |
| `done` (기존) | — | 더 이상 안 씀 (Plan 18 fixup 으로 unused). 안전상 그대로 두되 dead branch. |
| 그 외 `tr.docChanged` (기존) | — | state = null (auto-drop) |

### 2.4 Widget 렌더

`decorations(state)` 가 빌드하는 widget DOM:

```html
<span class="ai-ghost"><!-- variations[currentIdx].text or "(오류: ...)" --></span>
<!-- variations.length > 1 일 때만 다음 줄 -->
<div class="ai-ghost-indicator">
  [1/3] ◀ ▶  Tab 수락
</div>
```

- variation 의 `error` 가 set 이면 `text` 대신 `(오류: ${error})` 회색 표시. 색 `.ai-ghost-error`.
- variation 의 `done === true` 이면 깜박이 ▌ 멈춤 (현재 단일 모드 `.ai-ghost.done::after` 패턴 재사용).
- variations.length === 1 이면 indicator 줄 자체 렌더 안 함 (단일 모드 UX 보존).

ProseMirror Decoration.widget 의 side: 1 유지.

### 2.5 키바인딩

`addKeyboardShortcuts`:

```ts
{
  Tab: ghost-active ? acceptGhostText : false,
  Escape: ghost-active ? dropGhostText : false,
  ArrowLeft: ghost-active && N>1 ? switchGhostVariation(-1) : false,
  ArrowRight: ghost-active && N>1 ? switchGhostVariation(1) : false,
}
```

`false` 반환 시 다음 핸들러로 전파 (에디터 기본 동작 보존).

### 2.6 CSS

`apps/desktop/src/components/editor/GhostExtension.css` 에 추가:

```css
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

기존 `.ai-ghost` 스타일은 변경 없음.

---

## 3. useGhostText 변경

### 3.1 새 refs

```ts
const activeRunIdsRef = useRef<string[]>([]);  // 현재 in-flight runId 들
const runIdToVariationRef = useRef<Map<string, number>>(new Map());
```

`runIdRef` (기존 단일 run 추적) 는 그대로 유지 — `start()` 단일 모드에서 사용.

### 3.2 새 메서드 `startVariations`

```ts
const startVariations = useCallback(async (
  args: RunArgs,
  n: number,
) => {
  if (!editor) return;
  // 1. 기존 in-flight cancel
  for (const id of activeRunIdsRef.current) {
    aiApi.cancel(id).catch(() => {});
  }
  if (runIdRef.current) {  // 단일 모드 run 이 살아있다면도 cancel
    aiApi.cancel(runIdRef.current).catch(() => {});
    runIdRef.current = null;
  }
  activeRunIdsRef.current = [];
  runIdToVariationRef.current.clear();
  editor.commands.dropGhostText();

  // 2. N 슬롯 init
  const mode = args.replaceRange
    ? { kind: "replace" as const, from: args.replaceRange.from, to: args.replaceRange.to }
    : { kind: "insert" as const, pos: editor.state.selection.head };
  editor.commands.setGhostVariations(n, mode);

  // 3. N개 병렬 호출
  setStatus({ kind: "running", runId: "(variations)", text: "" });
  for (let i = 0; i < n; i++) {
    aiApi.run(args.nodeId, args.prompt, args.options, args.selectionText ?? "")
      .then(({ run_id }) => {
        activeRunIdsRef.current.push(run_id);
        runIdToVariationRef.current.set(run_id, i);
      })
      .catch((e) => {
        editor.commands.setGhostVariationDone(i, String(e));
      });
  }
}, [editor]);
```

### 3.3 기존 이벤트 핸들러 갱신

```ts
useEngineEvent<AIDelta>("ai-delta", (p) => {
  if (!editor) return;
  // 1. 변형 모드 매핑 우선 확인
  const idx = runIdToVariationRef.current.get(p.run_id);
  if (idx !== undefined) {
    const existing = ghostPluginKey.getState(editor.state);
    const currentText = existing?.variations[idx]?.text ?? "";
    editor.commands.setGhostVariationText(idx, currentText + p.text);
    return;
  }
  // 2. 단일 모드 (기존 Plan 18 흐름)
  if (p.run_id !== runIdRef.current) return;
  // ... 기존 단일 모드 핸들러 ...
});
```

ai-reset / ai-done / ai-error / ai-cancelled 도 동일 패턴 — 먼저 variation map 확인 후 매핑되면 variation 명령, 매핑 없으면 단일 모드 흐름.

`ai-done` 의 단일 모드 분기 안 자동 commit + bar 닫기 (Plan 18 fixup) 는 그대로. 변형 모드 분기는 `setGhostVariationDone(idx)` 만 하고 자동 commit 안 함.

### 3.4 `cancel` / `drop` 갱신

```ts
const cancel = useCallback(async () => {
  for (const id of activeRunIdsRef.current) {
    aiApi.cancel(id).catch(() => {});
  }
  if (runIdRef.current) {
    aiApi.cancel(runIdRef.current).catch(() => {});
  }
  activeRunIdsRef.current = [];
  runIdToVariationRef.current.clear();
  runIdRef.current = null;
  if (editor) editor.commands.dropGhostText();
  setStatus({ kind: "idle" });
}, [editor]);

const drop = useCallback(() => {
  if (!editor) return;
  for (const id of activeRunIdsRef.current) {
    aiApi.cancel(id).catch(() => {});
  }
  if (runIdRef.current) {
    aiApi.cancel(runIdRef.current).catch(() => {});
  }
  editor.commands.dropGhostText();
  activeRunIdsRef.current = [];
  runIdToVariationRef.current.clear();
  runIdRef.current = null;
  setStatus({ kind: "idle" });
}, [editor]);
```

(현재 `drop()` 도 Plan 18 fixup 으로 cancel 호출 — 거기에 array 처리 추가.)

### 3.5 새 메서드 `accept`

기존 `accept()` 가 단일 ghost commit + setStatus idle. 변경 없이 그대로 — `acceptGhostText()` 명령이 내부적으로 currentIdx 사용.

다만 변형 모드에서 accept 후 남은 in-flight runs cancel 필요:

```ts
const accept = useCallback(() => {
  if (!editor) return;
  // accept 직전에 남은 in-flight cancel (다른 variations 의 토큰 절약)
  for (const id of activeRunIdsRef.current) {
    aiApi.cancel(id).catch(() => {});
  }
  if (runIdRef.current) {
    aiApi.cancel(runIdRef.current).catch(() => {});
  }
  editor.commands.acceptGhostText();
  activeRunIdsRef.current = [];
  runIdToVariationRef.current.clear();
  runIdRef.current = null;
  setStatus({ kind: "idle" });
}, [editor]);
```

---

## 4. AIPromptBar 변경

### 4.1 토글 chip

`apps/desktop/src/components/ai/AIPromptBar.tsx`:

- 새 props: 없음. variationsOn 상태는 컴포넌트 내부 state.

```tsx
const [variationsOn, setVariationsOn] = useState(false);
```

톤·길이 chip row 안에 추가:

```tsx
<button
  type="button"
  className={`ai-prompt-bar-preset-chip${variationsOn ? " active" : ""}`}
  onClick={() => setVariationsOn(v => !v)}
  aria-pressed={variationsOn}
  title="3개 변형 병렬 생성"
>
  변형 ×3
</button>
```

### 4.2 onRun 시그니처 확장

기존:

```ts
onRun: (preset: PresetID, prompt: string) => void;
```

변경:

```ts
onRun: (preset: PresetID, prompt: string, variationsOn: boolean) => void;
```

`submit(preset)` 안 onRun 호출에 `variationsOn` 전달.

### 4.3 CSS

기존 `.ai-prompt-bar-preset-chip.active` 스타일이 있다면 그대로 적용. 없으면 추가 (다른 chip 의 active 패턴 차용):

```css
.ai-prompt-bar-preset-chip.active {
  background: rgba(255, 255, 255, 0.18);
  border-color: rgba(255, 255, 255, 0.25);
}
```

---

## 5. Workspace 변경

`apps/desktop/src/routes/Workspace.tsx`:

AIPromptBar 의 onRun 핸들러 분기:

```tsx
onRun={(preset, promptText, variationsOn) => {
  const isReplacePreset = preset === "rewrite" || preset === "compact";
  const hasSel = !!tiptapEditor && !tiptapEditor.state.selection.empty;
  const selectionText = hasSel ? tiptapEditor!.state.doc.textBetween(...) : "";
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

---

## 6. 에러 처리

| 상황 | 처리 |
|---|---|
| 한 변형 RPC start 실패 | 그 variation 즉시 `setGhostVariationDone(i, error)`. 나머지 진행. |
| 한 변형 ai-error 이벤트 | 그 variation `setGhostVariationDone(idx, p.message)`. 회색 `(오류: ...)` 표시. |
| 모든 변형 실패 | 모두 error 표시. 사용자 Esc 또는 Cmd+Enter 재시도. |
| 사용자가 error variation 에서 Tab | `acceptGhostText` 가 `variation.error` 체크 후 false. 키 흘려보내. UX 노트: 다음 정상 variation 으로 자동 ◀▶ 도 가능하나 first pass 에선 단순 no-op. |
| doc 변경 중 변형 모드 | 기존 auto-drop (tr.docChanged → state null) 그대로. 모든 variations 사라짐. useGhostText 의 다음 delta 가 매핑 없으면 무시. RPC cancel 은 별도 — drop 명령은 commands 경유여야만 cancel. 위험: doc 변경 만으로는 RPC 가 살아있을 수 있음. → 별도 fix 필요? **첫 PR 에선 documented limitation 으로 두고 후속.** (기존 단일 모드도 같은 문제 있음 — Plan 18 의 design 2.7 의도.) |

---

## 7. 테스트 전략

FE 테스트 인프라 부재 — 수동 스모크.

### 7.1 회귀 (Plan 18 단일 모드 보존)

1. 변형 chip off + Cmd+I + 자유 prompt + Enter → 단일 ghost streaming → ai-done 자동 commit + bar 닫기. **Plan 18 시나리오 그대로.**
2. 선택 영역 + 변형 chip off + `재작성` preset + Tab → 선택 영역 plain replace. Plan 18 동작 확인.
3. 변형 chip off + Esc 중도 → ghost drop + RPC cancel (Plan 18 fixup).

### 7.2 새 시나리오 (변형 모드)

4. 변형 chip on + 자유 prompt + Enter → 3개 variation 병렬 stream 시작. ghost 안 `[1/3] ◀ ▶ Tab 수락` 인디케이터.
5. variation 1 stream 중 ▶ → `[2/3]` 로 전환 + variation 2 누적 텍스트 표시 (그 시점 스냅샷).
6. ◀ 로 wrap (variations[0]) → variation 1 으로 복귀.
7. 모든 variation done 후 Tab → 현재 보고 있는 variation 의 text 가 doc 에 plain commit. bar 닫힘.
8. variation 1 done, 2/3 still streaming 상태에서 Tab on variation 1 → variation 1 commit + 2/3 cancel RPC.
9. 선택 영역 + chip on + `재작성` → 3 variations 가 모두 같은 영역 replace 후보. Tab → 현재 variation 으로 replace.

### 7.3 에러

10. 임시 throw 또는 invalid nodeID 시뮬 → 한 variation 만 `(오류: ...)` 표시. 나머지 진행.
11. 모든 variation 실패 → 인디케이터 보면서 ◀▶ 로 셋 모두 오류 확인.

### 7.4 Cancel

12. 3개 streaming 중 Esc → engine 로그에서 3 cancel RPC. ghost drop.
13. Cmd+Enter (변형 done 후) → 이전 3개 drop + (in-flight 있으면) cancel + 새 3개 시작.

### 7.5 토큰 사용량

14. 같은 prompt 단일 vs 변형 모드 → ai_runs 행이 1 vs 3 추가. 토큰 카운터 약 3배.

---

## 8. 위험 / 미해결

- **GhostExtension state 일반화 회귀 위험**: 단일 모드 (N=1) 가 Plan 18 동작과 동일해야 함. 7.1 회귀 시나리오가 핵심 게이트.
- **doc 변경 시 RPC leak**: 사용자가 streaming 중 본문 편집 → ghost auto-drop, 그러나 in-flight RPC 는 cancel 안 됨 (직접 cancel 명령이 없음). Plan 18 도 같은 한계. 후속 PR 에서 plugin state apply 안에서 auto-drop 시 useGhostText 에 알림 (custom event 또는 effect) → cancel. 첫 PR 범위 외.
- **Provider 동시 호출 안전성**: codex CLI / claude-code-cli 가 동시 3 호출 처리 가능한지 첫 스모크에서 확인. 만약 불안정하면 engine 측에 in-flight queue 또는 sequential 폴백.
- **인디케이터 키바인딩 충돌**: ArrowLeft/ArrowRight 가 ghost 활성 + N>1 일 때만 가로채기. ghost 비활성 시 에디터 커서 이동 정상. 첫 스모크 #5 에서 확인.
- **부분 에러 시 자동 skip**: 사용자가 ◀▶ 로 error variation 마주치면 약간 어색. 후속 UX 개선 — error 는 skip 하거나 인디케이터에서 dimmed 표시. 첫 PR 은 단순 표시.
- **`setStatus` 의미 약화**: 단일 모드는 status.kind 가 정확히 ai 진행 상태 반영. 변형 모드는 N 중 어느 하나라도 진행 중이면 `running`, 모두 done 이면 `idle` (acceptGhostText 후) — `done` 상태로 잠시 전이는 의도적으로 안 거침 (단일 모드의 auto-accept 트리거를 회피).

---

## 9. 파일 변경 요약

```
apps/desktop/src/
  components/editor/
    GhostExtension.ts    # state 일반화, 5개 새 명령, ArrowLeft/Right 키바인딩, widget DOM 분기
    GhostExtension.css   # +.ai-ghost-indicator, +.ai-ghost-error
  lib/editor/
    useGhostText.ts      # +startVariations, +activeRunIdsRef, +runIdToVariationRef,
                         # 이벤트 핸들러에 variation 매핑 분기, cancel/drop/accept 가 array cancel
  components/ai/
    AIPromptBar.tsx      # +variationsOn state, +`변형 ×3` chip, onRun 시그니처 확장
    AIPromptBar.css      # +.ai-prompt-bar-preset-chip.active (없으면)
  routes/
    Workspace.tsx        # onRun 분기 — variationsOn ? startVariations : start
```

Engine: 변경 없음.

---

## 10. 마이그레이션 / 호환성

- 기존 단일 모드 흐름은 모두 backward compat. `setGhostText(text, mode)` → 내부적으로 `setGhostVariations(1, mode) + setGhostVariationText(0, text)` 와 등가.
- `ai_runs` 테이블 schema 변경 없음. 변형 모드는 3개 row 가 동시에 생성될 뿐.
- 사용자가 chip 토글 안 하면 Plan 18 그대로 — 토큰 사용량 / latency / UX 무변화.
- chip on 한 상태는 컴포넌트 상태로 휘발 (settings 영구 저장 안 함). 매 Cmd+I 마다 reset = false.
