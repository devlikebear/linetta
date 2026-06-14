# 컴패니언 작가 액션 팔레트 — 개발계획서

_작성일: 2026-06-14_
_속한 로드맵: [`roadmap.md`](./roadmap.md), [`phase-3-stats-platform.md`](./phase-3-stats-platform.md)의 AI 퇴고 후속 UX_
_예상 소요: 1.5~2일_
_후속 안정화: [`companion-scene-edit-reliability.md`](./companion-scene-edit-reliability.md)_

## Overview

맞춤법 검사나 자동완성은 글쓰기 중간에 계속 개입하기 쉽다. 이번 슬라이스는 Linetta의 방향에 더 잘 맞게,
컴패니언을 "채팅창"에서 **작가가 바로 실행할 수 있는 액션 팔레트**로 끌어올린다.

이미 선택 영역 기반 `AI 퇴고` 흐름은 `Workspace.tsx` → `CompanionPanel.tsx` → `engine/internal/companion/`
경로에 들어와 있다. 이 계획은 새 AI 백엔드를 붙이지 않고, 기존 컴패니언·proposal·apply ops 흐름을
다듬어 다음을 달성한다.

- 빈 컴패니언 화면에서 한국 웹소설 집필에 맞는 작업을 바로 고를 수 있다.
- 선택 영역 퇴고는 "맞춤법 검사"처럼 보이되, 문체와 고유명사를 보존하는 AI 퇴고로 동작한다.
- `set_scene_text` 제안이 UI에서 원시 op 이름으로 노출되지 않고, 작가가 이해할 수 있는 적용 카드로 보인다.
- 사용자가 승인하기 전까지 본문은 바뀌지 않는다.

## 왜 이 방향인가

- 외부 맞춤법 검사기 연동은 기존 로드맵의 Out of Scope다. 외부 서비스 의존, 약관, 네트워크 실패가 생긴다.
- 실시간 자동완성은 소설 문체를 방해할 수 있고, 현재 Tiptap/컴패니언 구조에서는 별도 상호작용 설계가 크다.
- 컴패니언 고도화는 이미 존재하는 컨텍스트 선택, proposal card, `linetta_apply_ops`, 선택 영역 메뉴를 재사용한다.
- 웹소설가에게 필요한 것은 "항상 켜진 교정기"보다 "막혔을 때 바로 맡길 수 있는 작업"에 가깝다.

## 완료 조건

- [ ] 컴패니언 빈 상태에 작가 액션 프리셋이 표시되고, 클릭하면 해당 작업 프롬프트가 입력창에 채워진다.
- [ ] 프리셋은 현재 씬, 아웃라인, 설정, 플롯 비트 같은 기존 컨텍스트 선택과 자연스럽게 함께 쓸 수 있다.
- [ ] 선택 영역 `AI 퇴고` 결과의 proposal card가 `set_scene_text`를 사람이 읽을 수 있는 라벨로 표시한다.
- [ ] AI 퇴고 프롬프트는 의미·문체·고유명사·대사 톤 보존과 변경 목록 제시를 계속 요구한다.
- [ ] 모든 신규 사용자 노출 문자열은 `ko/en/ja` i18n에 들어간다.
- [ ] `make test`와 실제 Tauri 앱 수동 시나리오가 통과한다.

## 기술 스택 / 환경

- 프론트엔드: Tauri 2 + React 18 + Vite + TypeScript, Tiptap, lucide-react
- 컴패니언 UI: `apps/desktop/src/components/companion/`
- 워크스페이스 선택 메뉴: `apps/desktop/src/routes/Workspace.tsx`
- i18n: `apps/desktop/src/lib/i18n.tsx`
- 엔진 컴패니언: `engine/internal/companion/`
- 전체 검증: `make test`

## 현재 코드베이스 분석

- `apps/desktop/src/routes/Workspace.tsx`
  - 선택 텍스트 컨텍스트 메뉴에서 `runSelectionCompanionRewrite`, `runSelectionCompanionProofread`를 호출한다.
  - `companionRewriteRequest`에 `kind: "rewrite" | "proofread"`를 실어 `CompanionPanel`로 전달한다.

- `apps/desktop/src/components/companion/CompanionPanel.tsx`
  - `SelectionRewriteKind = "rewrite" | "proofread"`가 이미 있다.
  - `selectionRewritePrompt()`가 proofread일 때 맞춤법·띄어쓰기·조사 오류·비문 중심의 프롬프트를 만든다.
  - 빈 상태는 현재 `PROMPT_EXAMPLE_KEYS` 기반 예시 버튼만 보여주고, 클릭 시 입력창에 복사한다.
  - 입력 하단 툴바는 `web_search`, `web_fetch`, `linetta_apply_ops` 텍스트와 context 버튼을 보여준다.

- `engine/internal/companion/prompt.go`
  - `set_scene_text`를 현재 씬 본문 교체 op로 안내한다.
  - 본문 재작성·수정·확장·다듬기 요청은 `set_scene_text`를 쓰도록 지시한다.

- `apps/desktop/src/lib/types.ts`, `apps/desktop/src/lib/companionDisplay.ts`
  - 엔진은 `set_scene_text`를 지원하지만, 프론트 proposal 타입/필터에는 아직 빠져 있다.

- `apps/desktop/src/components/companion/ProposalCard.tsx`
  - `set_scene_text` 라벨 분기가 없어 proposal card에 원시 op명이 노출될 수 있다.

## 하지 않는 것

- 외부 맞춤법 검사기(부산대/다음 등) 연동
- 실시간 자동완성 또는 ghost text
- 새 LLM provider 추가
- 우측 패널 멀티 스택 재설계
- 플랫폼 자동 업로드

## 작업 체크리스트

### 작업 그룹 A: 작가 액션 프리셋 모델

- [ ] **A.1** — 프리셋 데이터 모델 추가
  - 파일: `apps/desktop/src/components/companion/CompanionPanel.tsx`
  - 대안: 데이터가 길어지면 신규 `apps/desktop/src/components/companion/companionActions.ts`
  - 내용:
    - `CompanionActionPreset` 타입을 둔다.
    - 필드 예시: `id`, `icon`, `labelKey`, `descriptionKey`, `promptKey`, `tone`.
    - 클릭 동작은 MVP에서 자동 전송하지 않고, 입력창에 프롬프트를 채운 뒤 focus한다.
    - 기존 `PROMPT_EXAMPLE_KEYS`는 유지하거나 프리셋의 보조 데이터로 흡수한다.
  - 권장 프리셋:
    - `continueScene`: 다음 문장 이어쓰기
    - `tightenDialogue`: 대사 자연스럽게
    - `raiseTension`: 장면 긴장 강화
    - `checkContinuity`: 설정 모순 체크
    - `nextEpisodeHook`: 다음 회차 훅 제안
    - `finishEpisode`: 회차 마감 점검
  - 검증:
    - `CompanionPanel.test.tsx`에서 프리셋 버튼 렌더링과 클릭 시 draft 반영을 확인한다.

- [ ] **A.2** — i18n 키 추가
  - 파일: `apps/desktop/src/lib/i18n.tsx`
  - 내용:
    - `companion.actions.title`
    - `companion.actions.label.*`
    - `companion.actions.description.*`
    - `companion.actions.prompt.*`
    - `ko/en/ja` 메시지 맵을 모두 채운다.
  - 프롬프트 작성 원칙:
    - 한국어 기본 프롬프트는 웹소설 작업 단위가 드러나게 쓴다.
    - 본문 직접 변경이 필요한 프리셋만 `set_scene_text`를 언급한다.
    - 아이디어/점검 프리셋은 목록이나 선택지로 답하게 하고 자동 적용을 요구하지 않는다.

### 작업 그룹 B: 빈 상태를 액션 팔레트로 변경

- [ ] **B.1** — `CompanionEmpty` UI 개편
  - 파일: `apps/desktop/src/components/companion/CompanionPanel.tsx`
  - 파일: `apps/desktop/src/components/companion/CompanionPanel.css`
  - 내용:
    - 현재 "프롬프트 예시" 영역을 "작가 액션" 카드/리스트로 바꾼다.
    - 2열 또는 세로 리스트 중 패널 폭에서 안정적인 쪽을 선택한다.
    - 각 액션은 아이콘, 짧은 제목, 한 줄 설명만 표시한다.
    - 카드 안에 긴 사용법 설명을 넣지 않는다.
    - 클릭하면 입력창에 프롬프트가 채워지고, 사용자가 직접 전송한다.
  - UX 기준:
    - 글쓰기 도구 화면이므로 과한 장식보다 빠른 스캔을 우선한다.
    - 버튼/카드 높이는 고정 또는 최소 높이를 둬서 번역 문자열로 레이아웃이 흔들리지 않게 한다.
    - 컴패니언의 기존 헤더 버튼과 입력 툴바 스타일을 유지한다.
  - 검증:
    - `CompanionPanel.test.tsx`의 기존 empty state 테스트를 새 카피에 맞게 갱신한다.
    - 긴 일본어/영어 라벨에서도 버튼 텍스트가 부모를 넘지 않는지 수동 확인한다.

- [ ] **B.2** — 입력 툴바 문맥성 개선
  - 파일: `apps/desktop/src/components/companion/CompanionPanel.tsx`
  - 파일: `apps/desktop/src/components/companion/CompanionPanel.css`
  - 내용:
    - `web_search`, `web_fetch`, `linetta_apply_ops` 원시 텍스트를 그대로 노출할지 재검토한다.
    - MVP에서는 기능 제거 없이 더 사람 친화적인 라벨로 바꾼다.
      - 예: 자료 검색, URL 읽기, 승인 후 반영
    - 컨텍스트 버튼은 유지하되 현재 선택 개수와 로딩 상태가 더 명확히 보이게 한다.
  - i18n:
    - `companion.tool.webSearch`
    - `companion.tool.webFetch`
    - `companion.tool.applyOps`
    - 기존 help 카드 문자열과 중복되면 재사용한다.
  - 검증:
    - 기존 context preview toggle 테스트가 계속 통과해야 한다.

### 작업 그룹 C: AI 퇴고 적용 카드 정리

- [ ] **C.1** — `set_scene_text` proposal 타입 허용
  - 파일: `apps/desktop/src/lib/types.ts`
  - 파일: `apps/desktop/src/lib/companionDisplay.ts`
  - 내용:
    - `ProposalOpType`에 `"set_scene_text"`를 추가한다.
    - `PROPOSAL_OP_TYPES`에도 `"set_scene_text"`를 추가한다.
    - inline `linetta_apply_ops` 파싱 결과가 퇴고 제안을 버리지 않게 한다.
  - 검증:
    - `apps/desktop/src/lib/applyProposal.test.ts` 또는 `companionDisplay` 테스트에 `set_scene_text` op 파싱 케이스를 추가한다.

- [ ] **C.2** — `set_scene_text` 라벨 추가
  - 파일: `apps/desktop/src/components/companion/ProposalCard.tsx`
  - 파일: `apps/desktop/src/lib/i18n.tsx`
  - 내용:
    - `opLabel()`에 `case "set_scene_text"`를 추가한다.
    - 라벨 예시:
      - ko: `현재 씬 본문 교체`
      - en: `Replace current scene text`
      - ja: `現在のシーン本文を置換`
    - 실패 목록에서도 같은 라벨이 보이게 한다.
  - 검증:
    - proposal card 렌더 테스트를 추가해 원시 `set_scene_text`가 화면에 노출되지 않는지 확인한다.

- [ ] **C.3** — 퇴고 프롬프트 회귀 테스트 보강
  - 파일: `apps/desktop/src/components/companion/CompanionPanel.test.tsx`
  - 파일: `engine/internal/companion/prompt_test.go`
  - 내용:
    - 프론트 테스트는 proofread 요청에 `맞춤법`, `고유명사`, `변경 목록`, `set_scene_text`가 들어가는지 유지한다.
    - 엔진 테스트는 시스템 프롬프트가 `set_scene_text`와 현재 씬 본문 교체 지시를 계속 포함하는지 확인한다.
    - 엔진에 proofread 전용 분기가 실제로 없으면 새 분기를 만들지 않는다. 현재는 프론트가 proofread 요청문을 구체화하는 구조로 충분하다.

### 작업 그룹 D: 수동 검증과 문서 반영

- [ ] **D.1** — 자동 검증
  - 명령:
    - `cd apps/desktop && pnpm test -- CompanionPanel.test.tsx applyProposal.test.ts --run`
    - `cd apps/desktop && pnpm build`
    - `make test`
    - `git diff --check`

- [ ] **D.2** — 실제 Tauri 앱 수동 시나리오
  - 명령:
    - `make tauri-dev`
  - 시나리오:
    - 컴패니언 열기 → 빈 상태에서 `다음 문장 이어쓰기` 선택 → 입력창에 프롬프트가 채워지는지 확인
    - 컨텍스트 버튼 열기 → 현재 씬/개요/플롯 선택 상태가 유지되는지 확인
    - 오타가 있는 한국어 문장 선택 → 우클릭 → `AI 퇴고` → 전송
    - 응답이 변경 목록을 포함하는지 확인
    - proposal card가 `현재 씬 본문 교체`처럼 표시되는지 확인
    - 적용 버튼 클릭 → 현재 씬 본문이 바뀌는지 확인
    - 거절 버튼 클릭 시 본문이 바뀌지 않는지 확인

- [ ] **D.3** — 로드맵 연결
  - 파일: `docs/plans/webnovel-ux/phase-3-stats-platform.md`
  - 내용:
    - AI 퇴고 UX 보강분으로 이 문서를 링크한다.
    - Phase 3 체크포인트의 `AI 퇴고` 항목이 이 계획의 완료 조건과 충돌하지 않는지 확인한다.

## 구현 순서

1. `git status --short --branch`로 작업 시작 상태를 확인한다.
2. C그룹을 먼저 처리한다. 퇴고 proposal이 버려지거나 원시 op로 보이는 문제를 먼저 없앤다.
3. A그룹으로 프리셋 데이터 모델과 i18n 키를 추가한다.
4. B그룹으로 빈 상태와 입력 툴바를 정리한다.
5. 테스트를 업데이트하고 `make test`까지 통과시킨다.
6. 실제 Tauri 앱에서 한국어 원고 선택 → AI 퇴고 → 적용까지 확인한다.
7. 필요하면 Phase 3 문서에 링크를 추가한다.

## 체크포인트

### Checkpoint 1: 퇴고 적용 카드

- [ ] `set_scene_text` proposal이 프론트에서 파싱된다.
- [ ] proposal card에 원시 `set_scene_text` 문자열이 보이지 않는다.
- [ ] 선택 영역 `AI 퇴고` 테스트가 통과한다.

### Checkpoint 2: 작가 액션 팔레트

- [ ] 빈 컴패니언 화면에서 5~6개의 작가 액션이 보인다.
- [ ] 액션 클릭 시 draft가 채워지고 자동 전송되지 않는다.
- [ ] context preview toggle과 첨부 기능이 회귀하지 않는다.

### Final Checkpoint

- [ ] `make test` 통과
- [ ] `git diff --check` 통과
- [ ] Tauri 수동 시나리오 통과
- [ ] Out of Scope 항목이 들어가지 않았는지 확인

## Claude Code 실행 메모

- 이 작업은 기능 추가지만, 외부 연동이나 새 엔진 기능이 아니다. 커밋 메시지는 `feat: improve companion writing actions` 정도가 적절하다.
- 기존 `AI 생성` 모드와 선택 영역 `AI 퇴고`를 합치려고 하지 않는다. 이번 범위는 컴패니언 기본 화면과 proposal 표시 품질이다.
- 사용자가 원문 변경을 승인하기 전까지 본문을 바꾸지 않는다.
- `i18n.tsx`는 `MessageKey` union과 3개 언어 메시지 맵이 맞아야 빌드가 통과한다.
- 작업 중 `engine/internal/companion/*`에 다른 변경이 보이면 먼저 `git status`로 범위를 보고 사용자에게 확인한다.
