# Phase 3: 통계·플랫폼·퇴고 — 작업지시서

_이 페이즈의 목표를 달성하기 위한 구체적 작업 목록. Claude Code가 위에서부터 순차 실행하되, 각 체크포인트에서 사용자 확인을 받는다._

_작성일: 2026-06-10_
_속한 로드맵: [`roadmap.md`](./roadmap.md)_
_예상 소요: 3~4일_

## 페이즈 목표

연재를 **지속하게 만드는** 장치를 더한다: 일별 집필량 히트맵과 완결 예상으로 동기를
시각화하고, 플랫폼별 텍스트 규칙이 적용된 복사로 발행 직전 수작업을 없애고,
선택 영역 AI 퇴고 원클릭으로 마무리 품질을 올린다.

## 전제 조건

- [ ] Phase 1 완료 (특히 `writing_stats` 테이블과 `export.nodeText` RPC — 이 페이즈의 토대)
- [ ] Phase 2 완료 권장 (다크 모드에서 통계 화면 대비 확인을 함께 하기 위함). 미완료 시 사용자와 진행 순서 협의
- [ ] AI 퇴고 작업(그룹 C)은 LLM provider가 설정된 환경에서 수동 검증 필요

## 포함 기능

1. **집필 통계 대시보드** — 일별 히트맵(최근 12주), 7일 평균, 완결 예상
2. **플랫폼 복사 프로필** — 문피아/네이버시리즈/조아라 텍스트 규칙
3. **AI 퇴고 원클릭** — 선택 영역 맞춤법·문장 다듬기 프리셋

## 이 페이즈에서 하지 않는 것

- 플랫폼 자동 업로드/로그인 → Out of Scope
- 연재 캘린더·발행 예약 알림 → Out of Scope
- 외부 맞춤법 검사기 연동 → Out of Scope (AI 퇴고로 대체)
- 통계의 목표 설정/스트릭 게이미피케이션 → 차기 검토 (이번엔 표시까지만)

## 작업 체크리스트

### 작업 그룹 A: 집필 통계 대시보드

- [ ] **T3.A.1** — 엔진 통계 조회 RPC 확장
  - 파일: `engine/internal/stats/`(Phase 1에서 생성), `engine/internal/rpc/handlers/stats.go`
  - 내용:
    - `stats.range(project_id, from_day, to_day)` → `[{ day, chars_added }]`
    - `stats.summary(project_id)` → `{ today, week_avg /* 최근 7일 평균 */, total_days /* 집필일 수 */ }`
  - 참조: Phase 1의 `stats.today` 핸들러 패턴
  - 검증: Go 테스트 — 빈 범위 / 연속 집필 / 공백일 포함 3케이스, `make test-go`

- [ ] **T3.A.2** — 통계 섹션 UI (컨텍스트 패널)
  - 파일: 신규 `apps/desktop/src/components/StatsSection.tsx`(+`StatsSection.test.tsx`, `StatsSection.css`), `apps/desktop/src/components/ContextPanel.tsx`, `apps/desktop/src/lib/rpc.ts`
  - 내용:
    - 컨텍스트 패널 하단(PlotPanel 아래)에 "집필 기록" 섹션: 최근 12주 일별 히트맵(7×12 그리드, 셀 농도 4단계 — 기존 `--t-teal` 농도 변형), 그 아래 "7일 평균 {n}자 · 집필 {d}일"
    - 히트맵 셀 hover 시 title 툴팁으로 날짜와 글자수
    - 데이터 없는 신규 프로젝트면 섹션 자체에 빈 상태 문구 한 줄
  - i18n: `stats.*` ko/en/ja
  - 의존: T3.A.1
  - 검증: Vitest(셀 농도 매핑 단위 테스트) + 수동

- [ ] **T3.A.3** — 완결 예상 (웹소설 프리셋 한정)
  - 파일: `apps/desktop/src/components/StatsSection.tsx`
  - 내용:
    - `length_target`이 series이고 7일 평균 > 0일 때: `남은 분량 추정 = max(0, 목표 회차분 - 현재 작품 글자수)` 대신 **단순화** — "현재 속도로 주당 약 {k}화" (= 7일 평균 × 7 ÷ episode_char_target)만 표시. 완결 시점 예측은 목표 총 회차 입력이 없으므로 하지 않는다
  - 검증: 단위 테스트 — 평균 0 / 평균 5,000 / 목표 변경 케이스

### 작업 그룹 B: 플랫폼 복사 프로필

- [ ] **T3.B.1** — 텍스트 변환 프로필 정의
  - 파일: 신규 `apps/desktop/src/lib/platformProfiles.ts`(+`platformProfiles.test.ts`)
  - 내용:
    - `transform(text: string, profile: PlatformProfileId): string` 순수 함수. 프로필:
      - `plain` (기본): Phase 1 출력 그대로
      - `munpia`: 문단 사이 빈 줄 1개 유지, 연속 마침표 3개+ → `…`, 전각 물결 정규화
      - `series`(네이버시리즈)·`joara`: 문단 사이 빈 줄 1개, `…` 정규화, 줄 끝 공백 제거
    - 규칙은 보수적으로 — 본문 의미를 바꾸는 치환(따옴표 종류 변경 등)은 하지 않는다
  - 검증: 프로필별 단위 테스트 각 3케이스 이상, Vitest 통과

- [ ] **T3.B.2** — 복사 메뉴에 프로필 적용
  - 파일: `apps/desktop/src/components/OutlinePanel.tsx`, `apps/desktop/src/routes/Workspace.tsx`, `engine/internal/settings/settings.go`, `apps/desktop/src/routes/Settings.tsx`
  - 내용:
    - 설정에 `copy_profile: PlatformProfileId`(기본 plain) 추가, Settings "에디터" 섹션(Phase 2)에 선택 UI
    - Phase 1의 "본문 복사"가 복사 직전 `transform(text, settings.copy_profile)` 적용. toast에 프로필명 표기: `"{n}자 복사됨 (문피아)"`
    - 커맨드 팔레트 항목명은 그대로 두고 동작만 프로필 적용 (메뉴 분기 늘리지 않기)
  - 의존: T3.B.1
  - 검증: 수동 — 프로필 변경 후 복사 결과가 규칙대로 변환되는지 외부 에디터에서 확인

### 작업 그룹 C: AI 퇴고 원클릭

- [ ] **T3.C.1** — 선택 메뉴에 "AI 퇴고" 항목
  - 파일: `apps/desktop/src/routes/Workspace.tsx`(`selectionMenu`), `apps/desktop/src/components/companion/CompanionPanel.tsx`
  - 내용:
    - 에디터 선택 컨텍스트 메뉴(현재 2항목: 사실 확인 / 컴패니언에게 다듬기 요청)에 세 번째 항목 "AI 퇴고(맞춤법·문장)" 추가
    - 동작은 기존 `runSelectionCompanionRewrite` 흐름을 재사용하되, `companionRewriteRequest`에 `kind: "proofread"`를 추가해 컴패니언이 **퇴고 전용 지시문**으로 보내도록
  - 참조: `selectionRewriteRequest` 처리 경로 (`CompanionPanel.tsx`의 수신부)
  - 검증: Vitest — proofread kind가 전송 페이로드에 반영되는지

- [ ] **T3.C.2** — 엔진 퇴고 프롬프트
  - 파일: `engine/internal/companion/prompt.go`(+`prompt_test.go`)
  - 내용:
    - 퇴고 요청 시 시스템 지시: "원문 의미·문체·고유명사·대사 톤 유지. 맞춤법·띄어쓰기·조사 오류·비문만 교정. 변경 목록을 함께 제시" — 기존 프롬프트 구성 함수에 분기 추가
    - 응답은 기존 rewrite 제안 카드(ProposalCard) 흐름으로 수신·적용
  - 참조: `engine/internal/companion/prompt.go`의 기존 rewrite 지시문 패턴 (현재 working tree에서 수정 중인 파일 — **충돌 주의, 시작 전 사용자에게 현 상태 확인**)
  - 의존: T3.C.1
  - 검증: `make test-go` + 수동 — 오타 포함 문단 선택 → AI 퇴고 → 의미 보존된 교정 제안 수신·적용

---

## ✅ Phase 3 Checkpoint

**구현 확인:**
- [ ] 모든 작업 체크박스 완료
- [ ] 통계 섹션이 웹소설/일반 프로젝트 모두에서 렌더 (완결 예상 줄만 웹소설 한정)
- [ ] 복사 프로필이 설정에 저장되고 모든 복사 경로(컨텍스트 메뉴·팔레트)에 일관 적용

**자동 검증:**
- [ ] `make test` 통과

**수동 확인:**
- [ ] 며칠 치 `writing_stats`가 있는 프로젝트에서 히트맵 농도·7일 평균 표시
- [ ] 프로필을 "문피아"로 → 회차 복사 → `...`가 `…`로 변환됨
- [ ] 오타 있는 문단 선택 → "AI 퇴고" → 고유명사·대사 톤이 보존된 교정 제안 → 적용 시 본문 반영
- [ ] 다크 모드에서 히트맵 대비 확인 (Phase 2 완료 시)

**이 체크포인트를 통과하면 전체 로드맵 완료 — roadmap.md의 "최종 완료 체크리스트"로 이동.**
실패 시: 실패 항목 보고 → 원인 파악 → 수정 → 재검증.

---

## 참고 자료

- 로드맵: [`roadmap.md`](./roadmap.md)
- AI 퇴고 UX 후속 계획: [`companion-writing-actions.md`](./companion-writing-actions.md)
- Phase 1 산출물: `engine/internal/stats/`, `export.nodeText`, 복사 UI
- 컴패니언 rewrite 경로: `apps/desktop/src/routes/Workspace.tsx`의 `runSelectionCompanionRewrite` → `CompanionPanel.tsx` → `engine/internal/companion/`

## 메모 / 주의

- `engine/internal/companion/*`는 현재 다른 작업으로 수정 중일 수 있다(working tree). T3.C 시작 전 반드시 `git status` 확인 후 사용자와 조율.
- 플랫폼 규칙은 각 플랫폼 에디터 사양이 비공개라 **보수적 최소 규칙**으로 시작하고, 사용자 피드백으로 보강하는 전제. 규칙 추가가 쉽도록 `platformProfiles.ts`를 데이터 주도(룰 배열)로 설계할 것.
- 히트맵은 외부 차트 라이브러리를 쓰지 않는다 — 7×12 div 그리드면 충분하고 기존 미니멀 톤과 맞다.

---
_이전 페이즈: Phase 2 — 구조·에디터 환경 → [`phase-2-structure-editor.md`](./phase-2-structure-editor.md)_
