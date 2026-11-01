# Phase 3: 모델별 대화 한도 — 작업지시서

_작성일: 2026-11-01_
_속한 로드맵: [`roadmap.md`](./roadmap.md)_
_예상 소요: 0.5일_

## 페이즈 목표

대화 한도가 고정값 4만 자에서 공급자별 값으로 바뀐다. 구독 기반 큰 모델은 덜 자르고, 사용량 과금 공급자의 비용은 그대로 둔다. 작가는 설정에서 값을 바꿀 수 있다.

## 전제 조건

- [ ] Phase 1 완료 및 승인
- [ ] `main`에서 `feat/agent-history-budget` 브랜치 생성

## 포함 기능

1. CLI 입력 한계 스파이크
2. 턴 단위 대화 한도
3. 설정값과 화면

## 이 페이즈에서 하지 않는 것

- 모델의 컨텍스트 크기 자동 조회 → 지금 코드에는 이 정보가 없다. 필요해지면 따로 계획한다.
- 턴 안의 도구 결과 예산(`toolResultBudget`) 변경 → 범위 밖
- 도구 행의 재생 → Out of Scope

## 작업 체크리스트

### 작업 그룹 A: 스파이크

- [ ] **T3.A.1** — claude-code 입력 한계 확인
  - 파일: 코드 변경 없음. 결과는 이 문서의 스파이크 기록에 적는다.
  - 내용:
    - 대화가 CLI에 어떻게 전달되는지 tars `pkg/llm`의 Claude Code 클라이언트에서 확인한다. 인자로 넘기면 macOS 인자 길이 제한이 걸릴 수 있다.
    - 한글 12만 자(약 36만 바이트) 대화로 실제 턴을 돌린다.
    - 응답 시간, 사용량 표시, CLI 자체 압축 발생 여부를 기록한다.
  - 판단: 실패하거나 응답이 크게 느려지면 기본값을 낮춘다. 전달 방식이 인자라면 먼저 그 경로를 고친다.
  - 검증: 스파이크 기록 작성

### 작업 그룹 B: 엔진

- [ ] **T3.B.1** — 한도를 매개변수로
  - 파일: `engine/internal/agent/prompt.go`, `agent.go`, `loop.go`, `prompt_test.go`
  - 내용:
    - `priorMessages(msgs, start, budget)`, `windowFrom(replayable, start, budget)`로 바꾼다.
    - 여유분은 `budget / 4`로 계산한다. 4만일 때 지금과 같은 1만이다.
    - `Deps.HistoryBudget func() int`를 더한다. `MaxToolCalls`처럼 턴 시작에 한 번 읽어 `loopState`에 둔다. nil이나 1 미만이면 `historyBudget` 기본값을 쓴다.
    - `replayHistory`와 `ReplayStart`가 같은 값을 쓰게 한다. 패널의 경계 표시가 실제 창과 어긋나면 안 된다.
  - 참조: `agent.go:90`의 `MaxToolCalls`
  - 테스트: 기존 창 유지 테스트를 값별로 실행, 한도가 줄면 창이 다시 채워짐
  - 검증: `CGO_ENABLED=0 go test ./internal/agent/...`

- [ ] **T3.B.2** — 불러오는 행 수 조정
  - 파일: `engine/internal/agent/prompt.go`, `loop.go`, `agent.go`
  - 내용:
    - `historyLoadRows`는 2000이고 도구 행도 이 수에 포함된다. 실측 비율로는 12만 자에 약 1000행이 쓰이지만, 도구를 많이 쓰는 작품은 더 든다.
    - 고정값을 올리지 말고 `load`가 도구 행 중 `ask` 없는 행을 SQL에서 뺄 수 있는지 먼저 본다. 안 되면 6000으로 올린다.
  - 주의: 행 제한이 한도보다 먼저 걸리면 창의 첫 메시지가 매 턴 밀려 캐시가 깨진다.
  - 검증: 행 제한 경계 테스트 추가, `CGO_ENABLED=0 go test ./internal/agent/...`

- [ ] **T3.B.3** — 설정값
  - 파일: `engine/internal/settings/settings.go`, `engine/internal/engineapp/agent_enabled.go`, 설정 테스트
  - 내용:
    - `AgentHistoryBudget int`(`agent_history_budget`), 0은 자동, 수동 범위는 2만~20만
    - 자동 규칙: `claude-code`는 스파이크 결과 값(목표 12만), 그 외는 4만
    - `openai-codex`는 모델별 컨텍스트 크기를 확인하지 못해 4만으로 둔다.
  - 참조: `AgentMaxToolCalls`, `validAgentMaxToolCalls`
  - 검증: `CGO_ENABLED=0 go test ./internal/settings/... ./internal/engineapp/...`

### 작업 그룹 C: 화면

- [ ] **T3.C.1** — 설정 항목
  - 파일: `apps/desktop/src/routes/Settings.tsx`, `apps/desktop/src/lib/i18n.tsx`, `types.ts`
  - 내용: 에이전트 도구 호출 수 설정 근처에 "자동 / 직접 입력"을 둔다. 값을 키우면 응답마다 보내는 양이 늘어 API 비용이 커질 수 있다고 안내한다.
  - 검증: `make test-desktop`

---

## ✅ Phase 3 Checkpoint

**구현 확인:**
- [ ] 모든 작업 체크박스 완료
- [ ] claude-code 공급자에서 자동 한도가 적용된다
- [ ] 그 외 공급자의 동작이 바뀌지 않는다

**자동 검증:**
- [ ] `CGO_ENABLED=0 go test ./...` 통과 (`engine/`)
- [ ] `make test-mobile-engine` 통과
- [ ] `make test-desktop` 통과

**수동 확인:**
- [ ] DB 복사본에 같은 시뮬레이션을 돌려 잘린 턴 수를 비교한다. 기준선은 196턴 중 78턴이다.
- [ ] 긴 작품 복사본에서 실제 한 턴을 돌려 응답을 확인한다.

**완료 처리:**
1. 완료 작업, 자동 검증 결과, 남은 수동 확인을 보고한다.
2. PR을 열고 작가 승인 뒤 다음 페이즈로 간다.
3. 실패하면 항목을 보고하고 원인 분석, 수정, 재검증 순으로 진행한다.

---

## 스파이크 기록

_T3.A.1 결과를 여기에 적는다._

## 메모 / 주의

- 긴 대화를 다시 보내면 프롬프트 캐시 미스 한 번의 비용도 커진다. `historyHeadroom`의 이유가 그대로 적용된다.
- 한도를 올려도 대화 검색은 필요하다. 12만 자도 연재가 길어지면 넘는다.

---
_다음 페이즈: Phase 4 — 검색 정리 → [`phase-4-unified-search.md`](./phase-4-unified-search.md)_
