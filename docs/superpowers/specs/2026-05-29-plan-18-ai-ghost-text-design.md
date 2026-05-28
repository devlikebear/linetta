# Plan 18 — AI Mode 통합 (Cmd+I Ghost Text) Design Spec

## 목적

현재 AI는 별도 모드(`mode === "ai"`)로 에디터를 통째로 가린다. 이를 제거하고, 에디터 안에서 직접 AI 생성을 호출하는 **인라인 ghost-text** UX로 바꾼다. 동시에 AI 컨텍스트에 **작품 설정(장르/분량/시점)** 을 추가하고, 사이드 패널의 컨텍스트 체크리스트를 실제 컨텍스트 페이로드와 일치시킨다.

기존 명령 팔레트는 `Cmd+K` 에서 **`Cmd+P`** 로 이동 (VSCode Quick Open 관례). 비워진 자리는 사용하지 않고, AI bar는 **`Cmd+I`** 에 할당 (VSCode Copilot Inline Chat 관례). 두 단축키 모두 VSCode 사용자에게 익숙한 형태.

## Goals

1. AI 호출 진입점이 에디터 안에서 일어난다 — Cmd+I 누르면 커서 근처 floating prompt bar.
2. 생성 결과는 doc 변경 없이 회색 ghost text(ProseMirror widget decoration)로 인라인 스트리밍.
3. Tab → 수락(doc commit), Esc → 거절(decoration drop), Cmd+Enter → 재실행.
4. 선택 영역이 있으면 preset 의미에 따라 교체 / 이어붙이기.
5. AI 컨텍스트에 `Project.Genres`, `Project.LengthTarget`, `Project.DefaultPOV` 가 들어가 프롬프트의 최상단에 1줄 요약으로 렌더링.
6. 사이드 패널 컨텍스트 체크리스트가 실제 페이로드와 일치 (Plan 16 hierarchical + Plan 18 작품 설정 반영).
7. 기존 별도 AI 모드(`AIMode.tsx`, `AIContextPanel.tsx`) 및 `mode` state 제거.

## Non-Goals

- Multi-turn 대화. 현재 single-shot 모델 유지.
- Composer/multi-file edit. 한 씬 단위 작업만.
- Selection 시각화 diff view (회색 strikethrough + 옆 ghost). 단순 "선택 영역은 그대로, 다음 줄에 ghost" 채택.
- 코드 블록/마크다운 미리보기 렌더링. ghost는 plain text widget으로만.

---

## 1. 아키텍처

### 1.1 데이터 흐름

```
User: Cmd+I
   │
   ▼
AIPromptBar 표시 (Workspace 상태로 토글, 커서 위치 anchor)
   │
   ▼
User: prompt 입력 / preset 클릭 → onRun
   │
   ▼
useAIRun (기존 훅) → engine ai.run RPC
   │            (context = Plan 16 + Plan 18 확장 페이로드)
   ▼
engine: ai.delta 노티 스트림
   │
   ▼
useGhostText hook이 delta 누적 → GhostExtension의 DecorationSet 갱신
   │
   ▼
ProseMirror: cursor 위치에 회색 widget decoration 렌더 (doc 변경 없음)
   │
   ├── User: Tab → GhostExtension.acceptCommand → doc에 텍스트 insert + decoration drop
   ├── User: Esc → GhostExtension.dropCommand → decoration drop만
   └── User: Cmd+Enter → onRegenerate → 같은 prompt + context 재실행 (이전 decoration drop)
```

### 1.2 엔진 측 변경

`engine/internal/ai/ai.go` 에 `ProjectMeta` 타입 추가:

```go
// ProjectMeta carries the project-level configuration the user set when
// creating the project (장르, 예상 분량, 기본 시점). Renders as a single line
// at the very top of the user message so the LLM understands the project's
// fundamental constraints.
type ProjectMeta struct {
    Genres       []string `json:"genres"`
    LengthTarget string   `json:"length_target"`
    DefaultPOV   string   `json:"default_pov"`
}
```

`Context` 구조에 필드 추가:

```go
type Context struct {
    // ... 기존 필드 ...
    Project ProjectMeta `json:"project"`
    // ... 기존 필드 ...
}
```

`engine/internal/ai/context.go` `Build` 메서드에서 이미 `proj`를 로드 중이므로 매핑 한 줄:

```go
return Context{
    ProjectID:     proj.ID,
    NodeID:        n.ID,
    Project: ProjectMeta{
        Genres:       proj.Genres,
        LengthTarget: proj.LengthTarget,
        DefaultPOV:   proj.DefaultPOV,
    },
    // ... 기존 필드 ...
}
```

`engine/internal/ai/prompts.go` `buildUser` 의 최상단에 `## 작품 설정` 섹션을 추가:

```go
// renderProjectMeta returns "장르: X, Y · 분량: Z · 시점: W" with empty
// pieces omitted. Returns empty string if all three are empty.
func renderProjectMeta(m ProjectMeta) string { /* ... */ }
```

호출:

```go
// 기존 buildUser 시작부에 추가
if meta := renderProjectMeta(c.Project); meta != "" {
    sb.WriteString("## 작품 설정\n")
    sb.WriteString(meta)
    sb.WriteString("\n\n")
}
// 기존 ## 작품 전반 (synopsis) 섹션 그대로 이어짐
```

값 매핑 규칙 (UI 표시 문자열로 변환):

- `LengthTarget`: `"short"` → `"단편"`, `"novella"` → `"중편"`, `"novel"` → `"장편"`. 매핑 외 값은 원문 유지.
- `DefaultPOV`: `"first"` → `"1인칭"`, `"third"` → `"3인칭"`, `"omniscient"` → `"전지적"`. 매핑 외 값은 원문 유지.
- `Genres`: 그대로 `, ` 조인.

빈 값 처리 (한 항목도 없으면 섹션 자체를 생략):

- `len(Genres) == 0` 이면 `장르:` 항목 생략
- `LengthTarget == ""` 이면 `분량:` 항목 생략
- `DefaultPOV == ""` 이면 `시점:` 항목 생략
- 세 항목 모두 비면 섹션 전체 생략 (헤더도 안 찍음)

### 1.3 프론트엔드 측 변경

**삭제:**
- `apps/desktop/src/components/ai/AIMode.tsx`
- `apps/desktop/src/components/ai/AIMode.css`
- `apps/desktop/src/components/ai/AIContextPanel.tsx`
- `Workspace.tsx` 의 `mode` state (`"edit" | "ai"`), `setMode` 호출, AI/EDIT toggle 버튼, `mode === "ai"` 렌더 분기

**유지 (그대로):**
- 엔진 RPC (`ai.run`, `ai.cancel`, 노티 이벤트들)
- AIOptions 타입, tonePresets 정의
- 사용 중인 `useAIRun` 훅이 있다면 그대로

**신규 파일:**

| 파일 | 책임 |
|---|---|
| `apps/desktop/src/lib/editor/GhostExtension.ts` | Tiptap extension. ProseMirror Plugin + DecorationSet 관리. setGhost / acceptGhost / dropGhost 명령 노출. Tab/Esc 키바인딩 — ghost 활성 시에만 작동. |
| `apps/desktop/src/lib/editor/useGhostText.ts` | React hook. ai.delta 이벤트를 받아 GhostExtension에 누적 텍스트 push. 상태: idle / streaming / done / error. |
| `apps/desktop/src/components/ai/AIPromptBar.tsx` | Cmd+I 시 표시되는 floating prompt. preset chips / 톤·길이 chips / textarea / 생성 버튼 / ctx-칩. 커서 위치 absolute positioning, viewport-aware flip. |
| `apps/desktop/src/components/ai/AIPromptBar.css` | 위 스타일 |
| `apps/desktop/src/components/ai/AIContextChecklist.tsx` | ctx-칩 클릭 시 표시되는 popover. honesty checklist. |
| `apps/desktop/src/components/ai/AIContextChecklist.css` | 위 스타일 |

**Workspace.tsx 변경 요약:**
- `mode` state 제거.
- 새 state: `aiPromptOpen: boolean`, `aiPromptAnchor: { top, left } | null`.
- 전역 `Cmd+I` 핸들러 → `aiPromptOpen` 토글.
- 기존 `Cmd+K` palette 핸들러를 **`Cmd+P` 로 이동** — `Workspace.tsx:326-453` 부근의 keymap, `ShortcutsModal.tsx:11` 의 "명령 팔레트 열기" 라벨, `useFirstLeaf.ts:35` 주석, `ThreadView.tsx:57` 의 hint 문구 모두 동시 업데이트.
- TiptapEditor의 `extensions` prop에 `GhostExtension()` 추가 (기존 `MentionExtension`/`NoteMarkerExtension` 패턴과 동일). `useGhostText` 훅이 노출하는 `setGhostText / acceptGhost / dropGhost` 명령을 AIPromptBar 및 ghost 키바인딩과 연결.
- AIPromptBar는 `aiPromptOpen && load.node` 일 때만 렌더.

---

## 2. UX 상세

### 2.1 키바인딩

| 키 | 컨텍스트 | 동작 |
|---|---|---|
| `Cmd+I` | 어디서나 (editor focus) | AIPromptBar 토글 |
| `Cmd+P` | 어디서나 | 명령 팔레트 (Cmd+K에서 이동) |
| `Esc` | AIPromptBar 열림, ghost 없음 | bar 닫기 |
| `Esc` | ghost 활성 | ghost drop + bar 닫기 |
| `Enter` | AIPromptBar textarea focus | 생성 시작 (prompt 비어있으면 흔들기 애니메이션) |
| `Cmd+Enter` | AIPromptBar textarea focus | 같은 prompt 즉시 재실행 (textarea에 줄바꿈 X) |
| `Cmd+Enter` | ghost done 상태 | 재생성 (이전 ghost drop) |
| `Tab` | ghost done 또는 streaming 상태 | ghost 수락 (doc commit) |

### 2.2 AIPromptBar layout

```
┌──────────────────────────────────────┐
│ [재작성] [확장] [요약]   톤 ▾  길이 ▾ │
│ ┌──────────────────────────────────┐ │
│ │ 프롬프트를 입력하세요…             │ │
│ └──────────────────────────────────┘ │
│ ⓘ ctx: 12개          [생성 ⌘↵]      │
└──────────────────────────────────────┘
```

- preset chip 클릭: textarea를 시드 prompt로 채움 + 즉시 생성. 예) "확장" → "이 장면을 더 감각적으로 확장해줘". 기존 AIMode가 쓰던 시드 텍스트 그대로 차용.
- 톤 / 길이: `tonePresets` / `short_form` 옵션을 풀다운으로. 현재 선택값이 chip 라벨로 표시.
- `ⓘ ctx: 12개`: 페이로드에 들어간 총 항목 수(인근 씬 N + 다른 장 M + ... + 작품 설정 1 + ...). 클릭 시 `AIContextChecklist` popover.
- `[생성 ⌘↵]`: 클릭 또는 Cmd+Enter로 발화.
- 생성 도중: 버튼이 `[취소 Esc]` 로 변환, 같은 자리에서 cancel.

### 2.3 Positioning

- 커서 위치를 ProseMirror `view.coordsAtPos(view.state.selection.head)` 로 픽셀 좌표 획득.
- bar 너비 ~480px, 커서 아래 4px 떨어진 곳에 표시. 좌측 정렬은 커서 x ± 마진.
- viewport 하단 200px 안이면 flip — 커서 위로 표시.
- 우측 overflow 시 좌측으로 shift.

### 2.4 Selection × Preset 시멘틱

| Preset | 선택 영역 없음 | 선택 영역 있음 |
|---|---|---|
| 자유 prompt (preset 미선택) | ghost를 커서 위치에 inline append. Tab → 그 위치에 insert. | ghost를 선택 영역 끝 다음 줄에 새 문단으로 표시. Tab → 선택 영역 다음에 **삽입** (선택은 보존). |
| `확장` | 자유 prompt와 동일. | 자유 prompt와 동일 (이어붙이기 의미). |
| `재작성` | 동작 안 함 — bar 안에 "선택 영역이 필요합니다" 한 줄 hint. preset chip은 disabled 상태. | ghost를 선택 영역 다음 줄에 표시. Tab → 선택 영역을 ghost로 **교체**. |
| `요약` | 동작 안 함 (재작성과 동일 hint). | 재작성과 동일 — 교체. |

엔진 LLM 호출 시 선택 영역 텍스트는 system prompt 의 `## 작가의 지시` 섹션 위에 새 `## 선택 영역` 섹션으로 전달 (기존 SceneText 와 별도). 이는 Plan 9 prompt 구조와 호환 — `SelectionText` 필드를 `Context` 에 추가.

```go
type Context struct {
    // ... 기존 ...
    SelectionText string `json:"selection_text"`
}
```

비어있으면 섹션 생략.

### 2.5 Ghost decoration 렌더

- ProseMirror `DecorationSet` 의 `Decoration.widget(pos, () => HTMLElement)` 사용.
- widget 내용: `<span class="ai-ghost">{accumulatedText}</span>`. 멀티라인이면 `\n` 을 `<br>` 로 변환 (또는 단순 `white-space: pre-wrap`).
- 스타일:
  ```css
  .ai-ghost {
    color: rgba(232, 232, 234, 0.45);
    white-space: pre-wrap;
    font-style: italic;
  }
  .ai-ghost::after {
    content: "▌";
    opacity: 0.7;
    animation: blink 1s steps(2) infinite;
  }
  .ai-ghost.done::after { content: ""; }
  ```
- 스트리밍 delta가 들어올 때마다 `Plugin.spec.state.apply` 로 새 DecorationSet 생성 + `view.dispatch(view.state.tr.setMeta(pluginKey, { text }))`. 매번 React re-render 없이 ProseMirror 갱신.

### 2.6 컨텍스트 honesty checklist

`AIContextChecklist` popover 항목 (Plan 16 + Plan 18 반영):

```
✓ 현재 씬 본문
✓ 인근 씬 요약 (직전 2개 + 직후 1개)        — Plan 16
✓ 같은 장 다른 씬, 형제 장, 형제 부 요약   — Plan 16
✓ 작품 시놉시스                            — Plan 16
✓ 관련 과거 씬 (멘션 기반 topology RAG)   — Plan 16
✓ 등장 인물·장소 (멘션 기준)
✓ 활성 스토리라인 / 작가 주석 / 작가 메모
✓ 작품 설정 (장르/분량/시점)               — Plan 18 NEW
✓ 작가 style notes                        (있을 때만)
```

각 항목 옆에 작은 회색 caption — 실제 페이로드에서 채워진 카운트 (예: "인근 씬 요약 · 3개"). 비어있는 섹션은 회색 처리 + "—" 표시 (체크 X). 사용자가 컨텍스트의 "투명도"를 갖도록.

`ctx: N개` 칩의 N 계산:
- nearby + same_chapter + other_chapter + other_part 의 합
- + (synopsis 있으면 +1)
- + len(related_scenes)
- + len(entities)
- + len(active_threads)
- + len(notes)
- + (project meta 채워진 항목 수)
- + (style_notes 있으면 +1)

### 2.7 에러 처리

| 상황 | 처리 |
|---|---|
| prompt 비어있는데 Enter | textarea 흔들기 (1s shake), 호출 안 함. |
| 엔진 에러 (RPC 실패 또는 ai.error 노티) | bar 안 작은 빨간 텍스트 "오류: {message}". 생성 버튼 다시 활성. |
| 생성 도중 사용자가 doc 편집 | ghost decoration 즉시 drop. toast: "편집으로 인해 AI 결과를 폐기했습니다". `streamDedup` 의 "예측 불가능 상태" 와 동일 정책. |
| 생성 도중 사용자가 Cmd+I 한 번 더 | bar 닫기 → 생성은 백그라운드 계속. 결과 도착 시 — bar가 닫혀있으면 ghost만 표시 (별도 토스트 X). 다시 Cmd+I 누르면 bar 재표시. |
| 노드 변경 (사이드바에서 다른 씬 클릭) | 활성 ghost 즉시 drop. 생성 in-flight면 `ai.cancel` 발화. |

### 2.8 모바일/터치 환경

대상 OOS — Tauri 데스크탑 전용. 다루지 않음.

---

## 3. 테스트 전략

### 3.1 엔진 단위 테스트

- `engine/internal/ai/ai_test.go` (또는 새 파일): `ProjectMeta` 매핑 보존 (Build → Context.Project 채워짐).
- `engine/internal/ai/prompts_test.go`: 새 `## 작품 설정` 섹션
  - 모든 필드 채워진 케이스 — 헤더 + 한 줄 `장르: X, Y · 분량: 단편 · 시점: 1인칭`
  - 일부 필드 빈 케이스 — 항목 생략
  - 모두 빈 케이스 — 섹션 자체 생략
- `engine/internal/ai/context_test.go`: `SelectionText` 가 채워진 입력 → 프롬프트 렌더에 `## 선택 영역` 등장
- `engine/internal/ai/prompts_test.go`: `SelectionText` 빈 케이스 → 섹션 미등장

### 3.2 프론트엔드 단위 테스트

- `apps/desktop/src/lib/editor/GhostExtension.test.ts` (Tiptap test util)
  - `setGhost(text)` 호출 후 DecorationSet 에 widget 1개
  - `acceptGhost` → doc 변경 (텍스트 추가) + decoration drop
  - `dropGhost` → doc 변경 없음 + decoration drop
  - `dispatch(tr)` (사용자 doc 편집) → decoration auto-drop
- `apps/desktop/src/components/ai/AIPromptBar.test.tsx`
  - 빈 prompt + Enter → onRun 호출 안 됨, 흔들기 클래스 부여
  - preset chip 클릭 → textarea 값 채워짐 + onRun 호출
  - 톤 풀다운 선택 → options 업데이트
  - ctx 칩 클릭 → checklist popover 표시

### 3.3 수동 스모크

플랜 8에서 진행할 스모크 시나리오:

1. Cmd+I → bar 표시 → "이 장면을 더 감각적으로 확장해줘" 입력 → Enter → ghost 스트리밍 → Tab → doc에 commit.
2. 선택 영역 잡고 Cmd+I → "재작성" preset → ghost가 선택 다음 줄에 → Tab → 선택 영역 교체.
3. 생성 도중 Esc → ghost 사라짐, doc 안 변함.
4. 생성 도중 doc 편집 → 토스트 + ghost drop.
5. 작품 설정 채워진 작품에서 AI 호출 → `ai_runs.prompt` 또는 stderr 로그에서 `## 작품 설정` 섹션 확인.
6. 작품 설정 비어있는 작품에서 호출 → 섹션 미등장 확인.
7. ctx 칩 클릭 → checklist popover 가 honest 한지 확인 (Plan 16의 hierarchical 모든 layer 가 항목으로 등장).
8. Cmd+P → 명령 팔레트 정상 열림 (이동된 단축키). Cmd+I → AI bar 열림 (충돌 없음).

### 3.4 회귀

- Plan 16의 hierarchical context 기능: AI 호출 정상 + ai_runs.context_json 의 `hierarchical` 채워짐
- Plan 9 의 톤 preset / 길이 옵션: 동일 동작
- streamDedup / 재시도 처리 (Plan 11 버그픽스): GhostExtension 안에서도 유지

---

## 4. 파일 구조 요약

```
engine/
  internal/
    ai/
      ai.go              # +ProjectMeta type, +Context.Project, +Context.SelectionText
      context.go         # +Project 매핑, +SelectionText pass-through
      prompts.go         # +renderProjectMeta + ## 작품 설정 섹션, +## 선택 영역 섹션
      prompts_test.go    # +cases
      context_test.go    # +cases for ProjectMeta and SelectionText

apps/desktop/src/
  routes/
    Workspace.tsx        # -mode state, -AI/EDIT 토글, +Cmd+I 핸들러, +AIPromptBar 마운트
  components/
    ai/
      AIMode.tsx         # DELETE
      AIMode.css         # DELETE
      AIContextPanel.tsx # DELETE
      AIPromptBar.tsx    # NEW
      AIPromptBar.css    # NEW
      AIContextChecklist.tsx # NEW
      AIContextChecklist.css # NEW
  lib/
    editor/
      GhostExtension.ts  # NEW (Tiptap extension)
      useGhostText.ts    # NEW (React hook)
```

---

## 5. 마이그레이션 / 호환성

- 진행 중인 AI 호출(active stream)이 있는 상태에서 앱 재시작 → 기존 동작과 동일하게 그냥 idle 상태로 복귀. RPC 자체 변경 없음.
- `ai_runs` 테이블 schema 변경 없음. context_json 에 `project`, `selection_text` 신규 필드만 추가 (구 행 호환).
- 사용자가 작품 설정을 비워둔 채로 만든 기존 프로젝트도 정상 동작 (빈 값 → 섹션 생략).

## 6. 위험 / 미해결 결정 사항 (구현 중 결정)

- ghost text가 멀티 단락 일 때 widget 가 visual flow를 자연스럽게 보여줄지 — 첫 PR 에서 plain widget으로 가고, 어색하면 paragraph-level widget으로 분해 (PR 2).
- viewport flip 디테일 (커서가 화면 가장자리 가까울 때) — 구현 시 케이스 확인.
- AIPromptBar 가 모달 backdrop 을 갖지 않으므로 outside click 처리 — 일단 outside click = bar 닫기로 진행. 입력 보존은 다음 Cmd+I 누름 때 복구.
