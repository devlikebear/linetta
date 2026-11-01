# Phase 3: 모델별 대화 한도 — 작업지시서

_작성일: 2026-11-01_
_속한 로드맵: [`roadmap.md`](./roadmap.md)_
_예상 소요: 0.5일_
_상태: 구현 완료_

## 페이즈 목표

대화 한도가 고정값 4만 자에서 공급자별 값으로 바뀐다. 구독 기반 큰 모델은 덜 자르고, 사용량 과금 공급자의 비용은 그대로 둔다. 작가는 설정에서 값을 바꿀 수 있다.

## 전제 조건

- [x] Phase 1 완료. 작가가 Phase 4까지의 진행을 지시했다
- [x] `feat/agent-history-budget` 브랜치 생성. Phase 2 위에 쌓았다

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

- [x] **T3.A.1** — claude-code 입력 한계 확인
  - 파일: 코드 변경 없음. 결과는 이 문서의 스파이크 기록에 적는다.
  - 내용:
    - 대화가 CLI에 어떻게 전달되는지 tars `pkg/llm`의 Claude Code 클라이언트에서 확인한다. 인자로 넘기면 macOS 인자 길이 제한이 걸릴 수 있다.
    - 한글 12만 자(약 36만 바이트) 대화로 실제 턴을 돌린다.
    - 응답 시간, 사용량 표시, CLI 자체 압축 발생 여부를 기록한다.
  - 판단: 실패하거나 응답이 크게 느려지면 기본값을 낮춘다. 전달 방식이 인자라면 먼저 그 경로를 고친다.
  - 검증: 스파이크 기록 작성

### 작업 그룹 B: 엔진

- [x] **T3.B.1** — 한도를 매개변수로
  - 파일: `engine/internal/agent/prompt.go`, `agent.go`, `loop.go`, `prompt_test.go`
  - 내용:
    - `priorMessages(msgs, start, budget)`, `windowFrom(replayable, start, budget)`로 바꾼다.
    - 여유분은 `budget / 4`로 계산한다. 4만일 때 지금과 같은 1만이다.
    - `Deps.HistoryBudget func() int`를 더한다. `MaxToolCalls`처럼 턴 시작에 한 번 읽어 `loopState`에 둔다. nil이나 1 미만이면 `historyBudget` 기본값을 쓴다.
    - `replayHistory`와 `ReplayStart`가 같은 값을 쓰게 한다. 패널의 경계 표시가 실제 창과 어긋나면 안 된다.
  - 참조: `agent.go:90`의 `MaxToolCalls`
  - 테스트: 기존 창 유지 테스트를 값별로 실행, 한도가 줄면 창이 다시 채워짐
  - 검증: `CGO_ENABLED=0 go test ./internal/agent/...`

- [x] **T3.B.2** — 불러오는 행 수 조정
  - 파일: `engine/internal/agent/prompt.go`, `loop.go`, `agent.go`
  - 내용:
    - `historyLoadRows`는 2000이고 도구 행도 이 수에 포함된다. 실측 비율로는 12만 자에 약 1000행이 쓰이지만, 도구를 많이 쓰는 작품은 더 든다.
    - `historyRowFilter`는 `Clear`와 공유하므로 조건을 더하지 않는다. `loadRowsFor(budget)`로 한도에 비례해 늘린다(최소 2000, 한도의 1/25).
  - 주의: 행 제한이 한도보다 먼저 걸리면 창의 첫 메시지가 매 턴 밀려 캐시가 깨진다.
  - 검증: 행 제한 경계 테스트 추가, `CGO_ENABLED=0 go test ./internal/agent/...`

- [x] **T3.B.3** — 설정값
  - 파일: `engine/internal/settings/settings.go`, `engine/internal/engineapp/agent_enabled.go`, 설정 테스트
  - 내용:
    - `AgentHistoryBudget int`(`agent_history_budget`), 0은 자동, 수동 범위는 2만~20만
    - 자동 규칙: `claude-code`는 스파이크 결과 값(10만), 그 외는 4만
    - `openai-codex`는 모델별 컨텍스트 크기를 확인하지 못해 4만으로 둔다.
  - 참조: `AgentMaxToolCalls`, `validAgentMaxToolCalls`
  - 검증: `CGO_ENABLED=0 go test ./internal/settings/... ./internal/engineapp/...`

### 작업 그룹 C: 화면

- [x] **T3.C.1** — 설정 항목
  - 파일: `apps/desktop/src/routes/Settings.tsx`, `apps/desktop/src/lib/i18n.tsx`, `types.ts`
  - 내용: 에이전트 도구 호출 수 설정 근처에 "자동 / 직접 입력"을 둔다. 값을 키우면 응답마다 보내는 양이 늘어 API 비용이 커질 수 있다고 안내한다.
  - 검증: `make test-desktop`

---

## ✅ Phase 3 Checkpoint

**구현 확인:**
- [x] 모든 작업 체크박스 완료
- [x] claude-code 공급자에서 자동 한도가 적용된다
- [x] 그 외 공급자의 동작이 바뀌지 않는다

**자동 검증:**
- [x] `CGO_ENABLED=0 go test ./...` 통과 (`engine/`)
- [x] `make test-mobile-engine` 통과
- [x] `make test-desktop` 통과

**수동 확인:**
- [x] DB 복사본에 같은 시뮬레이션을 돌려 잘린 턴 수를 비교한다. 기준선은 196턴 중 78턴이다.
- [x] 긴 작품 복사본에서 실제 한 턴을 돌려 응답을 확인한다.

**완료 처리:**
1. 완료 작업, 자동 검증 결과, 남은 수동 확인을 보고한다.
2. PR을 열고 작가 승인 뒤 다음 페이즈로 간다.
3. 실패하면 항목을 보고하고 원인 분석, 수정, 재검증 순으로 진행한다.

---

## 스파이크 기록

2026-11-01, Claude Code 2.1.280.

- 리네타는 권한 핸들러를 쓰므로 대화는 인자가 아니라 stream-json stdin으로 간다(`provider/claudecode.go`, tars `runControlOnce`). macOS, Linux, Windows의 인자 길이 제한에 걸리지 않는다. `--system-prompt`만 인자로 간다.
- 한글 129,652자(356,522바이트)를 같은 stdin 경로로 보냈다. 7.8초에 응답했고 마지막 줄을 읽었으며 CLI 압축은 없었다. 시험은 `haiku` 모델로 했다.
- 입력은 약 13.9만 토큰이었다. 한글 1자당 약 1토큰이다.
- 12만 자는 200k 창의 대부분을 쓰고 턴의 도구 결과가 들어갈 자리가 좁아서, 기본값을 10만 자로 낮췄다. 여유분을 더하면 12.5만 자까지 창이 유지된다.
- DB 복사본 시뮬레이션: 앱을 재시작하지 않는다는 가정에서 잘린 턴이 196턴 중 78턴에서 0턴으로 줄었다.
- 창의 시작점은 메모리에만 있다. 재시작하면 창이 한도(10만 자)로 다시 채워지므로, 11.9만 자인 가장 긴 작품은 앞쪽 약 2만 자가 빠진다. 복사본 실제 턴에서도 에이전트가 맨 처음은 시야에 없다고 답했다. 이 부분은 대화 검색으로 찾는다.

## 메모 / 주의

- 긴 대화를 다시 보내면 프롬프트 캐시 미스 한 번의 비용도 커진다. `historyHeadroom`의 이유가 그대로 적용된다.
- 한도를 올려도 대화 검색은 필요하다. 12만 자도 연재가 길어지면 넘는다.

---
_다음 페이즈: Phase 4 — 검색 정리 → [`phase-4-unified-search.md`](./phase-4-unified-search.md)_
