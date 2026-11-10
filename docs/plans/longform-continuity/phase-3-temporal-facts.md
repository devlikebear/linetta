# Phase 3: 시점 사실과 정산 — 작업지시서

_위에서부터 순차 실행하고 Checkpoint에서 작가 확인을 받는다._

_작성일: 2026-11-02_
_속한 로드맵: [`roadmap.md`](./roadmap.md)_
_예상 소요: 3일_
_상태: 구현 완료. 실제 모델 호출 확인은 작가 동의를 기다린다_

## 페이즈 목표

원장의 사실이 어느 회차부터 어느 회차까지 유효한지 알게 된다. 브리프는 그 회차 시점의 현재 상태를 계산해서 보여 주고, 같은 주어·속성에 어긋나는 사실을 쓰면 엔진이 그 자리에서 충돌로 알려 준다. 회차를 쓴 뒤에는 작가가 켠 경우 정산 패스가 사실을 원장에 남긴다.

## 전제 조건

- [x] Phase 1 완료 및 작가 승인. Phase 2와 같은 파일을 고치므로 Phase 2 뒤에 시작한다
- [x] `main`에서 `feat/longform-temporal-facts` 브랜치 생성
- [x] 마이그레이션 다음 번호 확인

## 포함 기능

1. 독립 검수 실호출 스파이크
2. 원장 사실 확장
3. 시점 조회와 현재 상태 섹션
4. 충돌 감지
5. 정산 패스

## 이 페이즈에서 하지 않는 것

- 설정 변경의 앞 회차 전파 → Phase 4
- 아크별 정리와 복선 범위 → Phase 4
- 기존 원장 자유글의 자동 변환 → 기존 행은 그대로 둔다. 정리는 Phase 4의 재구축 칩으로 한다
- 그래프 DB, 임베딩 → Out of Scope
- 독립 검수와 정산의 기본 켜기 → 측정 뒤 작가가 정한다

## 작업 체크리스트

### 작업 그룹 A: 스파이크

- [x] **T3.A.1** — 독립 검수 실호출 (2026-11-09 합성 작품 1회 실측, 아래 스파이크 기록)
  - 파일: 코드 변경 없음. 결과는 이 문서의 스파이크 기록에 적는다.
  - 내용:
    - #272는 실제 모델 호출을 확인하지 못했다. DB 복사본과 `LINETTA_HOME` 임시 폴더에서 `agent_episode_review_enabled`를 켜고 회차 하나를 쓴다.
    - 호출 수, 소요 시간, 기록된 관찰 수, 5분 제한과 `episodeReviewCallsPerScene = 6`에 걸리는지 기록한다.
    - 쓴 패스가 작가의 턴을 막는지, 세션 잠금이 어떻게 풀리는지 본다.
  - 판단: 패스가 끝나지 않으면 호출 예산과 제한 시간을 먼저 고친다. T3.D.1은 이 결과를 따른다.
  - 검증: 스파이크 기록 작성

### 작업 그룹 B: 원장 사실 확장

- [x] **T3.B.1** — 마이그레이션
  - 파일: `engine/internal/store/migrations/00NN_ledger_facts.sql`
  - 내용:
    - `story_ledger`에 `attribute TEXT NOT NULL DEFAULT ''`, `valid_to_node_id TEXT REFERENCES nodes(id) ON DELETE SET NULL`, `superseded_by TEXT NOT NULL DEFAULT ''`, `strength TEXT NOT NULL DEFAULT 'hard'`, `value_num REAL`을 더한다.
    - 유효 시작은 기존 `planted_node_id`를 쓴다. 새 열을 만들지 않는다.
    - `(project_id, subject, attribute)` 인덱스를 더한다.
    - `strength`는 서술자가 단정한 `hard`, 대사나 소문으로 나온 `soft`다.
  - 검증: `CGO_ENABLED=0 go test ./internal/store/...`

- [x] **T3.B.2** — 저장소와 주어 정규화
  - 파일: `engine/internal/ledger/ledger.go`, `repo.go`, 테스트
  - 내용:
    - `Entry`와 `Patch`에 새 열을 맞춘다. 사실 종류(`decision`, `number`, `timeline`, `knowledge`)에만 허용한다.
    - 저장할 때 `Subject`를 세계관 요소의 정식 이름으로 맞춘다. 별칭 조회는 작은 인터페이스로 받는다. InkOS의 문자열 일치 약점을 피하기 위해서다.
    - 속성은 자유 문자열이지만 도구 설명에 기본어를 준다: 위치, 소지금, 경지, 부상, 소속, 호칭, 작중 날짜, 앎.
  - 검증: 별칭으로 저장해도 같은 주어가 되는 테스트

- [x] **T3.B.3** — 도구 입력
  - 파일: `engine/internal/mcphost/tools_ledger.go`, `budget_test.go`
  - 내용:
    - `editLedgerInput`에 `attribute`, `strength`, `value_num`, `supersedes`를 더한다.
    - 출력 `editLedgerOutput`에 `conflicts`, `previous`를 더한다.
    - 고정 항목 보호는 그대로 둔다. `supersedes`가 고정 항목을 가리키면 거절한다.
    - 도구 예산 상한을 측정값으로 올리고 이유를 남긴다.
  - 검증: `CGO_ENABLED=0 go test ./internal/mcphost/...`

### 작업 그룹 C: 시점 조회와 충돌

- [x] **T3.C.1** — `AsOf`
  - 파일: `engine/internal/ledger/asof.go` (신규), `asof_test.go`
  - 내용:
    - `AsOf(ctx, projectID, nodeID) ([]Entry, error)`를 만든다. 회차 순서는 `newTree`를 쓴다.
    - 현재 회차 이전에 심었고, 아직 끝나지 않았고, 대체되지 않은 행 중에서 주어·속성별 최신 행을 고른다.
    - 속성이 빈 옛 행은 대상에서 뺀다. 기존 `Settled` 경로로 계속 나온다.
  - 테스트: 앞 회차 시점과 뒤 회차 시점의 값이 다르다, 미래 사실이 안 보인다, 대체된 값이 안 보인다
  - 검증: `CGO_ENABLED=0 go test ./internal/ledger/...`

- [x] **T3.C.2** — 현재 상태 섹션
  - 파일: `engine/internal/ledger/brief.go`, `engine/internal/storycontext/types.go`, `render.go`, 테스트
  - 내용:
    - `LedgerBrief`에 `State []string`을 더한다. 주어별로 묶어 한 줄씩 낸다. 예: `주어: 위치=…, 소지금=…`.
    - 등장 요소와 계획에 나온 주어를 먼저 넣는다. `briefStateRunes = 2500`이고 넘치면 `Omitted`로 센다.
    - `soft` 값에는 표시를 붙인다.
    - 제목은 `## 현재 상태`, 위치는 작품 규칙 뒤다. 세 언어를 쓴다.
    - Phase 1의 상한 테스트가 계속 통과하도록 세계관 예산을 다시 잰다.
  - 검증: `CGO_ENABLED=0 go test ./internal/ledger/... ./internal/storycontext/... ./internal/mcphost/...`

- [x] **T3.C.3** — 충돌 감지
  - 파일: `engine/internal/ledger/conflict.go` (신규), `repo.go`, `tools_ledger.go`, 테스트
  - 내용:
    - 사실을 추가하면 같은 주어·속성의 유효한 행을 찾아 값이 다르면 `conflicts`로 돌려준다. 저장은 막지 않는다.
    - `supersedes`를 주면 이전 행의 `valid_to_node_id`와 `superseded_by`를 트랜잭션으로 채운다.
    - `value_num`이 있으면 이전 값과의 차이를 `previous`로 돌려준다. `작중 날짜`는 이야기 순서로 뒤로 가면 충돌로 낸다.
    - 비교는 코드로만 한다. LLM을 부르지 않는다.
  - 검증: 충돌, 대체, 날짜 역행 테스트

### 작업 그룹 D: 정산 패스

- [x] **T3.D.1** — 패스
  - 파일: `engine/internal/agent/settlement.go` (신규), `settlement_test.go`, `loop.go`, `agent.go`
  - 내용:
    - `episodereview.go` 구조를 따른다. `noteWrittenScene`이 모은 회차를 쓰고, `boundedPass`로 돌린다.
    - 도구는 `linetta_read_scene`, `linetta_list_ledger`, `linetta_edit_ledger`, `linetta_report_continuity`만 준다. 원고와 세계관을 바꾸는 도구는 주지 않는다.
    - 프롬프트는 본문에 근거가 있는 변경분만 인용과 함께 기록하고, `conflicts`가 오면 `linetta_report_continuity`로 넘기라고 지시한다.
    - 최대 3회차, 호출 예산과 제한 시간은 스파이크 결과로 정한다.
    - `maybeEpisodeReview`와 같은 세 지점에서 부르되 정산이 먼저 끝난 뒤 검수가 돌게 한다.
  - 참조: [InkOS의 Settler](https://github.com/Narcooo/inkos)
  - 테스트: 스위치가 꺼지면 안 돌고, 허용 도구 밖은 호출할 수 없다
  - 검증: `CGO_ENABLED=0 go test ./internal/agent/...`

- [x] **T3.D.2** — 설정
  - 파일: `engine/internal/settings/settings.go`, `engine/internal/engineapp/`, `apps/desktop/src/routes/Settings.tsx`, `apps/desktop/src/lib/i18n.tsx`
  - 내용:
    - `agent_settlement_enabled`를 더한다. 기본은 꺼짐이다.
    - 화면 위치는 독립 검수 스위치 옆이다. 원고를 다시 보낸다는 설명을 쓴다.
  - 참조: `AgentEpisodeReviewEnabled`의 전체 경로
  - 검증: `CGO_ENABLED=0 go test ./internal/settings/...`, vitest

- [x] **T3.D.3** — 프롬프트와 패널
  - 파일: `engine/internal/agent/prompt.go`, `apps/desktop/src/components/CanonPanel.tsx`, `apps/desktop/src/lib/i18n.tsx`
  - 내용:
    - 원장 지침에 속성, 대체, 충돌 처리를 한 문장으로 더한다. 프롬프트가 늘어나는 양을 적는다.
    - 패널의 사실에 속성, 유효 범위, `soft`, 대체 이력을 보인다.
  - 검증: `CGO_ENABLED=0 go test ./internal/agent/...`, vitest

---

## ✅ Phase 3 Checkpoint

**구현 확인:**
- [x] 모든 작업 체크박스 완료
- [x] 스위치가 꺼진 작가에게는 턴 뒤에 추가 호출이 없음
- [x] 정산 패스가 원고를 바꿀 수 없음

**자동 검증:**
- [x] `CGO_ENABLED=0 go test ./...` 통과 (`engine/`)
- [x] `go test -tags mobile ./...` 통과
- [x] vitest, `tsc --noEmit`, 프런트 빌드 통과
- [x] 새 열이 vault 왕복과 작품 복원 뒤 남는 테스트 통과

**수동 확인:**
- [x] 복사본에서 회차를 앞뒤로 옮겨 가며 `## 현재 상태`의 값이 달라지는지 확인한다
- [x] 위치나 날짜가 어긋나는 사실을 넣어 `conflicts`가 오는지 확인한다
- [x] 정산을 켜고 회차를 하나 써서, 기록된 사실의 인용이 본문과 맞는지 표본으로 확인한다 (합성 작품 1회 실측: 기록 4건 모두 인용이 본문과 글자 그대로 맞음)
- [x] 패스가 끝나는 시간과 호출 수가 작가의 다음 턴을 막지 않는지 확인한다 (패스가 도는 중에 보낸 다음 턴이 3.4초에 끝남)

**완료 처리:** 결과와 측정표를 작가에게 보고하고, 정산과 독립 검수의 기본값은 작가가 정한다. 승인 뒤 Phase 4로 간다.

---

## 스파이크 기록

2026-11-02. 실제 모델 호출은 하지 못했다.

- 임시 폴더에서 실호출을 준비했지만, AI 데이터 공유 동의를 코드가 대신 넣는 단계에서 막혔다. 동의는 작가 본인이 앱에서 해야 하므로 우회하지 않았다. 호출 수, 소요 시간, 관찰 품질은 **미측정**이다.
- 대신 코드를 읽다가 더 큰 문제를 찾았다. `agent_episode_review_enabled`를 켜도 Claude Code 공급자에서는 아무 일도 일어나지 않았다. `boundedPass`는 모델이 돌려주는 도구 호출을 실행하는데, Claude Code는 자기 루프를 돌려 호출을 돌려주지 않는다. `loopCLI`는 패스를 부르지도 않았다. #272의 독립 검수는 이 공급자에서 한 번도 돈 적이 없다.
- `runPass`를 더해 공급자별로 나눴다. Claude Code에는 허용 도구만 든 전용 브리지(`cliPass`)를 연다. 가짜 CLI로 순서, 허용 도구, 호출 예산, 꺼짐 동작을 테스트했다. 실제 CLI로 도는지는 위와 같은 이유로 미확인이다.
- 제한 시간은 정산과 검수가 함께 쓰도록 10분으로 늘렸다. 호출 예산은 정산이 회차당 14회, 검수가 6회다. 실측 뒤 조정한다.

### 2026-11-09 합성 작품 1회 실측

**합성 작품 1회 실측이다. 작가 원고와 2주 실사용 측정을 대신하지 않는다.** 실제 원고는 쓰지 않았다.

- 조건: 임시 `LINETTA_HOME`에 엔진 RPC로 만든 12화짜리 합성 작품(인물 5, 질문 3, 앎 4줄, 위치·소지금 사실, 일부러 넣은 모순 4개). Claude Code CLI 공급자, 모델 `claude-sonnet-5-5`, 작가 구독. 엔진은 `CGO_ENABLED=0` 빌드를 `--stdio`로 띄워 내장 에이전트에 `agent.run`으로 턴을 보냈다. 동의와 정산·검수 스위치는 그 임시 홈에서만 켰다. 자기 개선 패스는 호출을 가르려고 껐다.
- **CLI 경로에서 패스가 실제로 돈다.** `cliPass`의 전용 브리지로 정산·검수·따라잡기가 모두 끝까지 돌았다(결과 `done`, 시간 초과 0, 실패 0).
- 호출 수와 시간 (패스의 도구 호출은 `mcp_activity`에서 `run_id`가 빈 행으로 셌다):

| 패스 | 회차 | 호출 수 | 걸린 시간 | 결과 |
|---|---|---|---|---|
| 턴 뒤 정산 | 8화 | 4 | 턴 끝 뒤 17.8초 | done, 결정 1건 기록 |
| 턴 뒤 정산 | 9화 | 6 (그중 참조 실패 2) | 턴 끝 뒤 약 35초 | done, 보고 1건 |
| 독립 검수 | 9화 | 6 / 예산 6 (그중 참조 실패 2) | 정산 뒤 약 42초, 턴 끝 뒤 약 77초 | 보고 3건 |
| 턴 뒤 정산 | 11화 (수정 뒤) | 4 | 턴 끝 뒤 24.3초 | done |
| 독립 검수 | 11화 (수정 뒤) | 4 / 예산 6 | 턴 끝 뒤 약 56초 | 보고 2건 |

- **10분 제한에 걸리지 않았다.** 턴 뒤 패스는 정산과 검수를 합쳐 가장 길게 약 77초였다. 정산 예산(회차당 14)도 걸리지 않았다. 검수 예산 6은 9화에서 꼭 다 썼고, 그중 2회는 아래 참조 실패였다.
- **다음 턴을 막지 않는다.** 9화 턴이 끝난 직후, 패스가 도는 중에 보낸 질문 턴이 3.4초에 끝났다.
- **인용 정확도:** 원장에 기록된 4건(정산 1, 쓰기 턴 3)의 인용이 모두 본문과 글자 그대로 맞았다. 엔진이 인용을 본문과 대조하므로 틀린 인용은 저장되지 않는다. 거절된 `linetta_edit_ledger`는 0건이었다.
- **기록 수가 적다.** 정산 패스가 사실을 새로 남긴 것은 6번의 회차 정산에서 1건이다. 모순이 있는 회차에서는 프롬프트대로 기록 대신 보고했다. "변한 것만" 지시가 지켜졌다고 볼 수도, 놓쳤다고 볼 수도 있다. 표본이 작아서 판단하지 않는다.
- **찾은 결함 (이 측정의 PR에서 고침):**
  - 패스가 `linetta_report_continuity`의 `refs`에 원장 항목 id를 entity·fact로 넣어 거절됐다. 수정 전 패스 호출 29회 중 7회였다. 오류 문구에 원장 항목은 참조가 아니라는 안내를 더하고, 정산 프롬프트에 한 줄을 더했다. 수정 뒤 패스 호출 24회 중 0회.
  - 검수 패스가 한국어 회차의 보고를 영어로 썼다. 모든 패스 프롬프트 끝에 "회차의 언어로 쓴다"를 더했다. 수정 뒤 한국어로 나왔다.
- **놓친 것:** 에이전트가 7·8화에 "삼 년"을 넣어 1·3화의 "석 달"과 어긋났다. 정산과 9화 검수는 이를 보고하지 않았다. 쓰기 턴의 에이전트가 답글로 작가에게 먼저 알렸다.
- 원자료와 스크립트는 저장소에 넣지 않았다. 재는 방법은 Phase 6 측정 기록에 적었다.

## 메모 / 주의

- 정산이 사실을 너무 많이 만들면 원장이 다시 선형으로 늘어난다. 변하는 값과 이후 회차가 기대는 사실만 남기라고 프롬프트에 쓴다. 표본에서 화당 건수를 잰다.
- 앱의 원장 추가 RPC는 주어와 본문만 저장하고 있었다. 복사본 검증에서 드러나 속성, 시작 회차, 유효 범위, 값을 저장하게 고쳤다.
- 주어와 속성이 어긋나면 충돌이 안 잡힌다. 탐지 누락은 Phase 5에서 재고, 속성 기본어를 넓히는 쪽을 먼저 본다.
- `value_num` 계산은 차이만 보여 준다. 돈의 증감 사유까지 코드가 판단하지는 않는다.

---
_다음 페이즈: Phase 4 — 아크 결산과 영향 검사 → [`phase-4-arc-closing.md`](./phase-4-arc-closing.md)_
