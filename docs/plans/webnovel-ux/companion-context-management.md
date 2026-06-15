# 컴패니언 레퍼런스 컨텍스트 & 토큰 관리 — 개발계획서

_작성일: 2026-06-15_
_속한 로드맵: [`roadmap.md`](./roadmap.md)의 Phase 3 AI 퇴고/컴패니언 고도화 후속_
_관련 문서: [`companion-writing-actions.md`](./companion-writing-actions.md), [`companion-scene-edit-reliability.md`](./companion-scene-edit-reliability.md), [`companion-scoped-chat-history.md`](./companion-scoped-chat-history.md)_
_예상 소요: 4~6일_

## Overview

컴패니언이 씬 본문을 실제로 바꾸는 신뢰성은 `set_scene_text`, intent contract, scene/project scoped history로 많이 보강되었다. 다음 병목은 **컴패니언이 어떤 자료를 보고 답하는지 사용자가 통제하기 어렵다**는 점이다.

작가는 "이 문체를 참고해줘", "이 PDF 설정집을 보고 써줘", "방금 클립보드 내용을 근거로 써줘"처럼 외부 자료를 넣고 싶다. 동시에 긴 원고와 긴 대화가 쌓이면 토큰 한계 때문에 모델이 중요한 맥락을 잃거나, 사용자가 왜 응답이 흐려졌는지 알 수 없다.

이 계획의 목표는 컴패니언에 **레퍼런스 자료 추가**, **토큰 예산 표시**, **요약/컴팩션 관리**를 붙여서, 작가가 현재 요청에 들어가는 맥락을 눈으로 확인하고 조절할 수 있게 만드는 것이다.

## 해결하려는 문제

- 현재 컨텍스트 패널은 `현재 씬`, `작품 개요`, `플롯`, `세계관 요소`, `관계`, `팩트`, `기억` 같은 내부 섹션만 다룬다.
- 파일/클립보드/긴 텍스트/PDF를 "이번 요청의 참고자료"로 넣는 구조가 없다.
- context preview는 항목 개수만 보여주고, 토큰 또는 문자 예산을 알려주지 않는다.
- `companion.compact`는 있지만 사용자가 무엇이 얼마나 압축됐는지 알기 어렵다.
- 긴 대화/레퍼런스가 쌓이면 모델이 질문만 하거나 이전 흐름을 잇는 식으로 빠질 위험이 커진다.

## 제품 원칙

- **컨텍스트는 숨은 마법이 아니라 작가가 보는 재료다.**
  - 이번 요청에 어떤 자료가 들어가는지, 왜 들어가는지, 얼마나 큰지 보여준다.

- **레퍼런스에는 목적이 있어야 한다.**
  - 같은 텍스트라도 `문체 참고`, `내용 근거`, `설정 자료`, `금지/주의사항`은 모델에게 다르게 전달되어야 한다.

- **긴 자료는 원문 보관 + 요약 주입으로 다룬다.**
  - 원문은 프로젝트 데이터로 남기되, 프롬프트에는 목적별 요약 또는 발췌를 우선 넣는다.

- **토큰 관리는 경고가 아니라 조절 도구다.**
  - 사용자가 직접 섹션을 끄거나, 레퍼런스를 요약하거나, 씬 대화를 compact할 수 있어야 한다.

- **씬 본문 작성/편집 신뢰성 계약을 깨지 않는다.**
  - 작성/편집 intent에서는 컨텍스트가 길어도 `set_scene_text` 성공 조건이 우선이다.

## 완료 조건

- [x] 컴패니언 패널에서 마크다운/텍스트/클립보드 레퍼런스를 추가할 수 있다.
- [x] 각 레퍼런스에 `문체 참고`, `내용 참고`, `설정/세계관`, `금지/주의` 목적을 지정할 수 있다.
- [x] context panel에서 내부 컨텍스트와 레퍼런스의 예상 토큰/문자 비용을 볼 수 있다.
- [x] 긴 레퍼런스는 원문 저장 후 요약본을 프롬프트에 넣을 수 있다.
- [x] 씬/작품 대화 compact 결과가 UI에 명확히 보이고, 프롬프트 replay에도 사용된다.
- [x] context preview, send, history, compact 관련 테스트가 추가된다.
- [x] `make test`, `git diff --check`, 실제 Tauri 수동 시나리오가 통과한다.

## 현재 코드베이스 분석

### 엔진

- `engine/internal/companion/companion.go`
  - `SendOptions`는 `Context`, `OutlineStructure`, `Intent`, `Scope`를 받는다.
  - `gatherContext()`가 프로젝트 개요, 아웃라인 노드, 씬 발췌, 플롯, 엔티티, 관계, 팩트, 기억을 모은다.
  - 현재 레퍼런스 자료나 토큰 예산 모델은 없다.

- `engine/internal/companion/prompt.go`
  - `PromptData`를 `buildContext()`로 렌더링한다.
  - `applyContextSelection()`은 기존 `ai.ContextSelection` bool map으로 섹션을 켜고 끈다.
  - `previewFromPromptData()`는 section preview와 item count를 만들지만, 문자 수/토큰 수/예산 상태는 없다.

- `engine/internal/companion/history.go`
  - Linetta 소유 `companion_messages` 테이블을 사용한다.
  - scene/project scope, status, intent, compacted 상태가 있다.
  - `CompactHistoryView()`는 현재 scope 메시지를 하나의 요약 assistant 메시지로 대체한다.

- `engine/internal/companion/runner.go`
  - TARS transcript와 Linetta history를 병행 기록한다.
  - scene edit intent는 `set_scene_text` 적용 검증이 실패하면 error로 끝난다.
  - reasoning delta, thinking status, proposal, choices, applied events를 이미 보낸다.

- `engine/internal/rpc/handlers/companion.go`
  - `companion.preview_context`, `companion.history`, `companion.compact`, `companion.clear`가 있다.
  - 신규 기능은 여기에 `companion.references.*` 또는 `companion.context_budget` 계열 RPC를 추가하는 방식이 자연스럽다.

### 프론트엔드

- `apps/desktop/src/components/companion/CompanionPanel.tsx`
  - `historyScope` scene/project tab이 있다.
  - 이미지 첨부는 있지만 텍스트/PDF/클립보드 레퍼런스 첨부는 없다.
  - 하단 toolbar에 `ctx` 버튼이 있고, `AIContextChecklistList`로 기존 컨텍스트 섹션을 토글한다.
  - reasoning은 streaming 중 `<details>`로 보이지만 완료 메시지에는 별도 reasoning 보관 UI가 없다.

- `apps/desktop/src/components/ai/AIContextChecklist.tsx`
  - 섹션별 체크박스, item count, preview toggle을 제공한다.
  - 토큰/문자 비용, 요약 상태, 레퍼런스 목적을 표현할 필드는 없다.

- `apps/desktop/src/hooks/useCompanion.ts`
  - store key가 `projectId:scope:nodeId` 형태로 scene/project scope를 지원한다.
  - `compact()`와 `clear()`는 이미 hook에서 노출된다.

- `apps/desktop/src/lib/types.ts`, `apps/desktop/src/lib/rpc.ts`
  - `ContextPreviewResponse`, `ContextPreviewSectionResponse`, `AIContextPreview` 타입이 있다.
  - 신규 필드는 wire snake_case -> FE camelCase mapping을 함께 갱신해야 한다.

## MVP 범위

### 포함

- 텍스트/마크다운/클립보드 레퍼런스 추가
- 레퍼런스 목적 지정
- 레퍼런스 원문 저장과 요약 저장
- context preview의 예상 토큰/문자 비용 표시
- scene/project 대화 compact 상태를 더 명확히 표시
- 레퍼런스를 companion prompt에 목적별 섹션으로 주입

### 제외

- PDF 파싱 MVP 포함
  - PDF는 파일 chooser와 저장 모델은 고려하되, 실제 텍스트 추출은 후속으로 둔다.
- 임베딩 기반 semantic retrieval
  - 지금은 목적별 요약/발췌와 수동 선택으로 충분하다.
- 자동 토큰 최적화 전체
  - MVP에서는 사용자가 끄기/요약하기를 선택한다.
- 외부 클라우드 파일 연동
- 모델별 정확한 tokenizer 구현
  - provider별 정확 token count 대신 `rune/4` 또는 `chars/3` 기반 추정치로 시작한다.

## 설계 제안

### 1. Companion Reference 저장소

신규 SQLite 테이블을 추가한다.

```sql
CREATE TABLE companion_references (
  id          TEXT PRIMARY KEY,
  project_id  TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  node_id     TEXT REFERENCES nodes(id) ON DELETE SET NULL,
  source_type TEXT NOT NULL, -- text | clipboard | markdown | file
  purpose     TEXT NOT NULL, -- style | content | canon | constraint
  title       TEXT NOT NULL,
  content     TEXT NOT NULL,
  summary     TEXT NOT NULL DEFAULT '',
  char_count  INTEGER NOT NULL DEFAULT 0,
  token_estimate INTEGER NOT NULL DEFAULT 0,
  status      TEXT NOT NULL DEFAULT 'active', -- active | summarized | disabled
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL
);

CREATE INDEX idx_companion_references_project
  ON companion_references(project_id, updated_at);

CREATE INDEX idx_companion_references_node
  ON companion_references(project_id, node_id, updated_at);
```

MVP에서는 file path를 저장하지 않고 텍스트 content를 저장한다. 이후 PDF/파일 재동기화가 필요해지면 `source_path`, `source_hash`를 추가한다.

### 2. Reference Purpose Contract

프롬프트에 레퍼런스를 다음처럼 목적별로 렌더링한다.

- `style`: "문체 참고" — 문장 리듬, 어휘, 시점, 거리감만 참고하고 내용은 복사하지 말라고 지시
- `content`: "내용 참고" — 사실/장면/자료 근거로 사용
- `canon`: "설정/세계관" — 작품 내부 진실로 취급
- `constraint`: "금지/주의사항" — 피해야 할 표현, 톤, 전개 제한

`buildContext()`에는 `## 추가 레퍼런스` 섹션을 추가하되, 각 항목은 `purpose`, `title`, `summary or excerpt`를 명확히 붙인다.

### 3. Token Budget Preview

기존 `ContextPreview`를 확장한다.

```go
type PreviewSection struct {
    ID            ContextKey `json:"id"`
    Label         string     `json:"label"`
    Present       bool       `json:"present"`
    Selected      bool       `json:"selected"`
    Count         int        `json:"count"`
    Preview       string     `json:"preview"`
    CharCount     int        `json:"char_count"`
    TokenEstimate int        `json:"token_estimate"`
}

type ContextPreview struct {
    PreviewCounts
    Sections              []PreviewSection `json:"sections"`
    SelectedItemCount     int              `json:"selected_item_count"`
    SelectedCharCount     int              `json:"selected_char_count"`
    SelectedTokenEstimate int              `json:"selected_token_estimate"`
    BudgetTokenEstimate   int              `json:"budget_token_estimate"`
}
```

추정 함수는 공용 유틸로 둔다.

```go
func EstimateTokens(s string) int {
    runes := len([]rune(s))
    if runes == 0 {
        return 0
    }
    return max(1, (runes+2)/3)
}
```

정확한 tokenizer가 아니어도 "현재 요청이 가벼운지/무거운지"를 보여주는 목적에는 충분하다.

### 4. Context Manager UI

현재 `ctx` 버튼으로 열리는 `companion-context-card`를 확장한다.

구성:

- 상단 요약
  - `선택됨 7개 · 약 8.2k tokens`
  - 상태 색상: 정상 / 큼 / 너무 큼
- 내부 컨텍스트 탭
  - 기존 `AIContextChecklistList`
  - 각 row에 `~1.2k` 같은 token badge 추가
- 레퍼런스 탭
  - active reference list
  - purpose chip
  - token badge
  - enable/disable toggle
  - summarize button
- 관리 액션
  - `클립보드 추가`
  - `텍스트 붙여넣기`
  - `파일 추가`
  - `현재 대화 요약`

MVP에서는 별도 modal 대신, 패널 안의 작은 popover/section으로 시작한다. 카드 안에 카드가 중첩되지 않게 `companion-context-card` 내부는 리스트/툴바 중심으로 구성한다.

### 5. Reference Summarization

요약은 두 단계로 간다.

MVP:

- 긴 레퍼런스는 deterministic snippet summary로 시작한다.
- 예: 앞 1,200자 + 제목/목적 + char/token metadata.
- UI에는 `요약 필요` 상태를 보여준다.

후속:

- LLM summarizer를 사용해 purpose별 summary를 생성한다.
- style reference는 문체 특징 요약, canon reference는 설정 bullet 요약처럼 다르게 만든다.

이렇게 나누는 이유는 MVP에서 또 다른 LLM run lifecycle과 비용 관리까지 얹으면 범위가 커지기 때문이다.

## 페이즈 구성

### Phase 1: 토큰 가시화 — 기존 컨텍스트의 비용을 보이게 한다

**목표**: 레퍼런스 추가 전이라도, 현재 컴패니언 요청에 어떤 내부 컨텍스트가 얼마나 들어가는지 볼 수 있다.

**포함 기능**

- context preview에 char/token estimate 추가
- frontend type/rpc mapping 확장
- `AIContextChecklistList`에 token badge 표시
- `ctx` chip에 선택 item 수 대신 token estimate 중심 표시

**예상 소요**: 1~1.5일

### Phase 2: 레퍼런스 추가 — 텍스트/마크다운/클립보드 자료를 넣는다

**목표**: 작가가 현재 작품 또는 현재 씬에 참고자료를 붙이고, 목적을 지정해 컴패니언에 주입할 수 있다.

**포함 기능**

- `companion_references` migration/repo
- `companion.references.list/create/update/delete` RPC
- 클립보드/텍스트/마크다운 추가 UI
- reference purpose chip
- prompt context에 active references 렌더링

**예상 소요**: 2~3일

### Phase 3: 컴팩션 운영 — 긴 대화와 긴 자료를 접어서 유지한다

**목표**: 긴 씬 대화와 긴 레퍼런스를 요약해 프롬프트 예산을 줄이고, 사용자가 무엇이 접혔는지 알 수 있다.

**포함 기능**

- `companion.compact` 결과를 UI에서 명확히 표시
- reference summarize/restore 상태
- context budget warning
- 실패/과다 예산 시 사용자가 바로 끌 수 있는 액션

**예상 소요**: 1~1.5일

## 작업 체크리스트

### Phase 1: 토큰 가시화

- [x] **1.1** — token estimate 유틸 추가
  - 파일: `engine/internal/ai/ai.go` 또는 신규 `engine/internal/ai/tokens.go`
  - 내용:
    - `EstimateTokens(text string) int`
    - `EstimateChars(text string) int`
  - 테스트: `engine/internal/ai/context_test.go` 또는 신규 `tokens_test.go`

- [x] **1.2** — context preview wire field 확장
  - 파일: `engine/internal/ai/ai.go`
  - 파일: `engine/internal/companion/prompt.go`
  - 내용:
    - `PreviewSection`에 `char_count`, `token_estimate` 추가
    - `ContextPreview`에 selected total 추가
    - `previewFromPromptData()`에서 section preview 원문 기준으로 산출
  - 검증:
    - `engine/internal/companion/companion_test.go`의 preview 관련 테스트 갱신

- [x] **1.3** — frontend type/rpc mapping 갱신
  - 파일: `apps/desktop/src/lib/types.ts`
  - 파일: `apps/desktop/src/lib/rpc.ts`
  - 내용:
    - `ContextPreviewSectionResponse`, `AIContextSection`, `AIContextPreview`에 char/token field 추가
    - `mapContextPreviewResponse()`에서 camelCase로 변환
  - 검증:
    - TypeScript build 통과

- [x] **1.4** — context checklist UI에 token badge 표시
  - 파일: `apps/desktop/src/components/ai/AIContextChecklist.tsx`
  - 파일: `apps/desktop/src/components/ai/AIContextChecklist.css`
  - 파일: `apps/desktop/src/components/companion/CompanionPanel.tsx`
  - 내용:
    - section row count 옆에 `~1.2k` token badge 추가
    - `ctx` chip은 `ctx ~8k`처럼 selected total 중심으로 표시
    - token field가 없으면 기존 count UI로 fallback
  - 검증:
    - `CompanionPanel.test.tsx`, `ContextPanel.test.tsx` 영향 확인

### Phase 2: 레퍼런스 추가

- [x] **2.1** — reference migration/repo 추가
  - 파일: 신규 `engine/internal/store/migrations/0014_companion_references.sql`
  - 파일: 신규 `engine/internal/companion/references.go`
  - 파일: 신규 `engine/internal/companion/references_test.go`
  - 내용:
    - CRUD repo 구현
    - purpose/status normalize
    - char_count/token_estimate 저장
  - 검증:
    - Go repo tests

- [x] **2.2** — companion service에 reference repo 연결
  - 파일: `engine/internal/companion/companion.go`
  - 파일: 엔진 wiring 파일에서 `NewHistoryRepo`를 연결한 위치
  - 내용:
    - `WithReferences(repo)` 추가
    - `gatherContext()`가 project/node active references를 load
    - `PromptData.References` 추가
  - 검증:
    - `PreviewContext`에서 reference section이 보이는지 테스트

- [x] **2.3** — references RPC 추가
  - 파일: `engine/internal/rpc/handlers/companion.go`
  - 파일: RPC registration 파일
  - 파일: `apps/desktop/src/lib/rpc.ts`
  - 파일: `apps/desktop/src/lib/types.ts`
  - 메서드:
    - `companion.references.list`
    - `companion.references.create`
    - `companion.references.update`
    - `companion.references.delete`
  - 검증:
    - `engine/internal/rpc/handlers/companion_test.go`

- [x] **2.4** — prompt 렌더링에 reference purpose 반영
  - 파일: `engine/internal/companion/prompt.go`
  - 내용:
    - `## 추가 레퍼런스` 섹션 추가
    - purpose별 지시 문구 포함
    - 긴 원문은 summary가 있으면 summary 우선, 없으면 bounded excerpt 사용
  - 테스트:
    - style reference가 "문체만 참고" 지시를 포함하는지
    - constraint reference가 금지/주의 섹션으로 렌더링되는지

- [x] **2.5** — CompanionPanel 레퍼런스 UI 추가
  - 파일: `apps/desktop/src/components/companion/CompanionPanel.tsx`
  - 파일: `apps/desktop/src/components/companion/CompanionPanel.css`
  - 권장: 복잡해지면 신규 `apps/desktop/src/components/companion/ReferenceManager.tsx`
  - 내용:
    - context card 안에 references area 추가
    - `클립보드 추가`, `텍스트 붙여넣기`, `마크다운 파일 추가`
    - purpose segmented control
    - active/disabled toggle
  - 검증:
    - `CompanionPanel.test.tsx`에서 reference create/list/toggle mock 테스트

### Phase 3: 컴팩션 운영

- [x] **3.1** — compacted message 표시 개선
  - 파일: `apps/desktop/src/hooks/useCompanion.ts`
  - 파일: `apps/desktop/src/components/companion/CompanionPanel.tsx`
  - 내용:
    - `status === "compacted"` 메시지를 일반 assistant bubble이 아니라 요약 기록으로 표시
    - scene/project compact 버튼 tooltip에 현재 scope 명시
  - 검증:
    - scene scope compact 시 현재 씬 메시지만 요약되는지 확인

- [x] **3.2** — reference summary 상태 추가
  - 파일: `engine/internal/companion/references.go`
  - 파일: `apps/desktop/src/components/companion/ReferenceManager.tsx`
  - 내용:
    - MVP deterministic summary: 긴 텍스트 앞부분 + purpose/title metadata
    - `status=summarized`면 prompt에는 summary 우선
    - UI에서 `원문 사용` / `요약 사용` 전환
  - 검증:
    - summary가 있는 reference는 prompt preview token estimate가 감소해야 한다.

- [x] **3.3** — budget warning과 quick actions
  - 파일: `apps/desktop/src/components/companion/CompanionPanel.tsx`
  - 파일: `apps/desktop/src/components/companion/CompanionPanel.css`
  - 내용:
    - selected token estimate가 임계값을 넘으면 warning row 표시
    - quick action: `현재 씬만 남기기`, `레퍼런스 요약`, `대화 요약`
  - 권장 임계값:
    - normal: `< 12k`
    - large: `12k~24k`
    - too large: `> 24k`
  - 검증:
    - 큰 reference mock에서 warning UI 표시

## UX 플로우

### 레퍼런스 추가

1. 사용자가 컴패니언 `ctx` 버튼을 연다.
2. `레퍼런스` 영역에서 `클립보드 추가` 또는 `텍스트 붙여넣기`를 누른다.
3. 제목과 목적을 고른다.
4. 저장하면 context list에 `문체 참고 · 약 1.8k` 같은 칩이 보인다.
5. 다음 컴패니언 요청부터 해당 레퍼런스가 prompt context에 포함된다.

### 토큰 조절

1. `ctx ~18k`처럼 예산이 커진 상태가 보인다.
2. 사용자가 context card를 열어 큰 섹션을 확인한다.
3. 레퍼런스를 끄거나 요약 사용으로 바꾼다.
4. `ctx ~8k`로 줄어든 것을 보고 전송한다.

### 씬 작성 요청

1. 사용자가 `이 씬 작성해줘`라고 요청한다.
2. scene scope history + 현재 씬 + active references가 들어간다.
3. intent contract가 `scene_write`로 분류된다.
4. 모델은 `set_scene_text`를 호출해야 하고, 성공 메시지는 앱이 적용 검증 후 표시한다.

## 검증 계획

### 자동 검증

- `go test ./engine/internal/companion/...`
- `go test ./engine/internal/rpc/handlers/...`
- `pnpm --dir apps/desktop test -- CompanionPanel.test.tsx --run`
- `pnpm --dir apps/desktop exec tsc --noEmit`
- `make test`
- `git diff --check`

### 실제 Tauri 수동 시나리오

- `LINETTA_HOME=/tmp/linetta-context-plan ./scripts/dev.sh`
- 기존 작품을 열고 컴패니언 `ctx` 패널을 연다.
- 현재 씬/플롯/세계관 섹션에 token badge가 보이는지 확인한다.
- 클립보드 텍스트를 `문체 참고` 레퍼런스로 추가한다.
- `이 문체를 참고해서 현재 씬 이어써줘`를 보낸다.
- 응답이 질문만 하지 않고 scene write intent로 실제 본문에 적용되는지 확인한다.
- 긴 레퍼런스를 요약 상태로 바꿨을 때 token estimate가 줄어드는지 확인한다.
- scene compact와 project compact가 서로 다른 scope에 적용되는지 확인한다.

## Checkpoint

### Checkpoint 1: 토큰 가시화

- [x] context preview 응답에 section별 `char_count`, `token_estimate`가 포함된다.
- [x] 컴패니언 `ctx` chip과 checklist row에서 token estimate가 보인다.
- [x] 기존 context selection toggle이 회귀하지 않는다.

### Checkpoint 2: 레퍼런스 주입

- [x] 텍스트/클립보드/마크다운 레퍼런스를 추가할 수 있다.
- [x] 레퍼런스 purpose가 저장되고 UI에 보인다.
- [x] active reference가 `buildContext()`에 목적별로 렌더링된다.
- [x] disabled reference는 prompt에 들어가지 않는다.

### Checkpoint 3: 컴팩션 운영

- [x] 대화 compact 결과가 요약 기록으로 표시된다.
- [x] 긴 reference를 요약 사용으로 바꾸면 token estimate가 줄어든다.
- [x] 예산 warning과 quick action이 동작한다.

### Final Checkpoint

- [x] `make test` 통과
- [x] `git diff --check` 통과
- [x] 실제 Tauri 수동 시나리오 통과
- [x] Out of Scope 항목이 들어가지 않았는지 확인
- [ ] 필요하면 이 문서를 구현 상태와 릴리즈 버전으로 갱신

## Claude Code 실행 메모

- 작업 시작 전 `git status --short --branch`를 확인한다.
- 이 기능은 UX와 prompt contract가 함께 움직이므로, phase마다 실제 Tauri 앱 확인을 한다.
- `AIContextChecklistList`는 AI 패널과 컴패니언이 함께 쓰므로 토큰 badge 추가 시 AI 패널 회귀를 같이 본다.
- `ContextPreviewResponse` wire field를 추가할 때 기존 필드는 optional/fallback으로 유지한다.
- 레퍼런스는 처음부터 PDF까지 욕심내지 않는다. 텍스트/클립보드/마크다운으로 먼저 end-to-end를 만든다.
- scene edit reliability의 성공 메시지 정책을 유지한다. 모델이 "참고했습니다"라고 말해도 실제 적용은 `set_scene_text` 검증이 기준이다.
