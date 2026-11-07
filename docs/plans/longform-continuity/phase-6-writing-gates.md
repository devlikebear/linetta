# Phase 6: 집필 문턱 — 작업지시서

_위에서부터 순차 실행하고 Checkpoint에서 작가 확인을 받는다._

_작성일: 2026-11-07_
_속한 로드맵: [`roadmap.md`](./roadmap.md)_
_예상 소요: 1.5일_
_상태: 계획 (작가 검토 대기)_

## 페이즈 목표

내장 에이전트는 계획이 없는 회차의 본문을 쓰지 못한다. 앞 회차를 정산하지 않은 채로는 다음 회차를 쓰기 시작하지 않는다. 문턱은 거절 메시지로 동작한다. 에이전트는 메시지를 읽고 한 번 더 호출해 문턱을 넘는다. 외부 MCP 클라이언트에는 거절 대신 경고를 준다.

## 배경

로드맵 근거에 이렇게 적혀 있다. 결정을 기록한 25턴 중 17턴에 원고 검색이 없었고, "프롬프트 규칙만으로는 강제되지 않는다." 그 뒤로 브리프 용량, 고정 규칙, 정산 패스, 영향 검사를 더했지만 **쓰기 전에 무엇이 갖춰져야 하는지**는 여전히 프롬프트로만 말한다. 예외가 하나 있다. `engine/internal/agent/briefgate.go`는 브리프를 읽지 않은 턴의 본문 쓰기를 거절한다. 이 문턱은 실사용에서 생긴 문제를 고쳤다. 고치는 턴이 문체 메모를 건너뛰던 문제다.

[Oh Story](https://github.com/zenstory-ai/oh-story-claudecode)의 장편 집필 스킬은 같은 문제를 문턱 두 개로 푼다. 세부 개요가 없으면 집필을 막는다. 앞 화의 추적 기록이 커밋되지 않았으면 다음 화 집필을 막는다. 이 페이즈는 같은 생각을 리네타의 기존 구조로 옮긴다. 새 집필 계약은 만들지 않는다.

- **계획**은 이미 있다. `nodes.plan`이고 `linetta_write_summary as_plan=true`로 쓴다(마이그레이션 0035). 본문을 써도 덮이지 않는다.
- **작가의 지시**도 이미 있다. 씬 의도(`scene_intents.goal`)와 고른 갈래의 비트(`PlannedBranch.Beats`)다. 둘 중 하나가 있으면 계획이 있는 것으로 본다.
- **정산**은 Phase 3의 패스다. 기본값은 꺼짐이다. 지금은 패스가 시간 초과로 끝나거나 작가가 본문을 직접 고쳐도 남는 기록이 없다.

## 전제 조건

- [ ] Phase 1~5 병합 확인
- [ ] `main`에서 `feat/longform-writing-gates` 브랜치 생성
- [ ] 마이그레이션 다음 번호 확인 (작성 시점 0046)

## 포함 기능

1. 계획 문턱
2. 정산 기록
3. 밀린 정산 따라잡기
4. 외부 클라이언트 경고와 측정

## 이 페이즈에서 하지 않는 것

- **세부 개요 양식과 구조 검사**: Oh Story는 세부 개요의 표 구조를 검사한다. 리네타는 계획의 내용을 검사하지 않는다. 비어 있는지만 본다. 양식을 만들면 그것이 새 집필 계약이 된다.
- **고치기 문턱**: 이미 본문이 있는 회차의 수정은 막지 않는다. 브리프 문턱이 이미 그 턴을 지킨다.
- **외부 클라이언트 거절**: `briefgate.go`의 원칙을 따른다. 외부 클라이언트의 작업 방식은 그쪽의 것이다.
- **정산 기본 켜기**: Phase 3에서 정한 대로, 실호출 측정 뒤 작가가 정한다.
- **비밀과 앎** → Phase 7

## 작업 체크리스트

### 작업 그룹 A: 계획 문턱

- [ ] **T6.A.1** — 문턱
  - 파일: `engine/internal/agent/plangate.go` (신규), `plangate_test.go`, `loop.go`
  - 내용:
    - `briefGate`와 같은 모양으로 `planGate`를 만든다. `loopState`에 포인터로 두고 `loop.go:660`의 브리프 문턱 바로 뒤에서 확인한다.
    - 대상은 `linetta_write_scene`, 그리고 `linetta_revise_scene` 중 **본문이 빈 회차**를 겨눈 호출이다. 본문이 있는 회차는 통과한다.
    - 계획이 있다는 기준은 `nodes.plan`, `scene_intents.goal`, 그 회차에 걸린 `PlannedBranch` 비트 중 하나가 비어 있지 않은 것이다. 조회는 작은 인터페이스(`PlanLookup`)로 받는다. `agent`가 `store`를 직접 가져오지 않게 한다.
    - 거절 문구: 이 회차에 계획이 없으니 `linetta_write_summary`를 `as_plan=true`로 불러 이 회차가 할 일을 몇 줄 적은 뒤 쓰라고 말한다. 같은 턴에 계획을 쓰면 통과한다.
    - `briefGate`처럼 턴이 문턱을 넘을 수 있을 때만 켠다. `linetta_write_summary`가 도구 목록에 없으면 문턱도 끈다.
  - 테스트: 계획 없는 빈 회차는 거절되고, 계획을 쓴 뒤에는 통과한다. 씬 의도만 있어도 통과한다. 본문이 있는 회차는 거절되지 않는다. 요약 도구가 없으면 거절하지 않는다. CLI 브리지 경로도 같은 문턱을 지난다
  - 검증: `CGO_ENABLED=0 go test ./internal/agent/...`

- [ ] **T6.A.2** — 프롬프트
  - 파일: `engine/internal/agent/prompt.go`, `prompt_test.go`
  - 내용: 새 회차를 쓰기 전에 계획을 먼저 적는다는 한 문장을 집필 지침에 더한다. 거절 메시지가 같은 말을 하므로 프롬프트에서는 짧게 쓴다. 늘어난 글자 수를 PR에 적는다.
  - 검증: `CGO_ENABLED=0 go test ./internal/agent/...`

### 작업 그룹 B: 정산 기록

- [ ] **T6.B.1** — 마이그레이션
  - 파일: `engine/internal/store/migrations/0046_scene_settlement.sql`
  - 내용:
    - `scene_settlements(node_id TEXT PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE, project_id TEXT NOT NULL, content_version INTEGER NOT NULL, outcome TEXT NOT NULL, settled_at INTEGER NOT NULL)`를 만든다.
    - `outcome`은 `done`, `timeout`, `failed`다. 정산된 판본은 `content_version`으로 기억한다. 본문을 고치면 판본이 바뀌므로 다시 정산할 대상이 된다.
    - vault 번들과 작품 복원, 작품 복제가 새 표를 옮기는지 확인한다. 옮기지 않으면 더한다.
  - 검증: `CGO_ENABLED=0 go test ./internal/store/... ./internal/vault/... ./internal/restore/...`

- [ ] **T6.B.2** — 패스 결과 남기기
  - 파일: `engine/internal/agent/episodereview.go`, `pass.go`, `agent.go`, 테스트
  - 내용:
    - 정산 패스가 끝나면 회차마다 결과를 남긴다. 기록은 `Deps`의 작은 함수(`RecordSettlement`)로 받는다.
    - 시간 초과와 호출 예산 소진은 `timeout`, 오류는 `failed`다. 원장에 쓴 것이 없어도 패스가 끝까지 돌았으면 `done`이다.
  - 검증: 가짜 공급자로 세 결과가 모두 기록되는 테스트

- [ ] **T6.B.3** — 대기 목록 조회
  - 파일: `engine/internal/ledger/settlement.go` (신규), 테스트, `engineapp` RPC, `apps/desktop/src-tauri/src/lib.rs`
  - 내용:
    - `Pending(ctx, projectID, beforeNodeID) []string`을 만든다. 이야기 순서에서 `beforeNodeID`보다 앞이고, 본문이 있고, 마지막 `done`의 판본이 현재 판본과 다른 회차를 고른다.
    - **정산을 켠 시점 이전의 회차는 대기로 보지 않는다.** 켜는 순간 수백 화가 밀린 회차가 되면 안 된다. 스위치를 켠 시각을 설정에 남기고 그 뒤에 쓰거나 고친 회차만 센다.
    - 앱에 `ledger.pending_settlement` RPC를 더한다.
  - 검증: `CGO_ENABLED=0 go test ./internal/ledger/...`

### 작업 그룹 C: 밀린 정산 따라잡기

- [ ] **T6.C.1** — 턴 시작 전 정산
  - 파일: `engine/internal/agent/loop.go`, `cli.go`, `settlement.go`, 테스트
  - 내용:
    - Oh Story는 앞 화의 정산이 끝나지 않았으면 다음 화를 막는다. 리네타에서 그렇게 하면 에이전트가 풀 수 없는 거절이 된다. 정산은 에이전트 도구가 아니라 패스이기 때문이다. 그래서 막는 대신 **먼저 따라잡는다.**
    - 정산이 켜져 있고, 이 턴의 대상 회차보다 앞에 대기 회차가 있으면, 본 루프 전에 정산 패스를 돌린다. 오래된 것부터 최대 3회차, 제한 시간은 2분이다.
    - 패스가 남은 대기를 다 풀지 못해도 턴은 진행한다. 남은 회차를 브리프의 `omitted_sections`처럼 알리는 한 줄(`정산 대기: 12화, 13화`)을 첫 사용자 메시지 앞에 붙인다.
    - 대상 회차가 없는 턴(질문, 세계관 편집)에는 돌리지 않는다.
  - 테스트: 정산이 꺼지면 아무것도 돌지 않는다. 대기 회차만 돈다. 3회차를 넘으면 나머지는 알림으로 간다. 패스가 실패해도 턴이 진행된다
  - 검증: `CGO_ENABLED=0 go test ./internal/agent/...`

- [ ] **T6.C.2** — 패널 표시
  - 파일: `apps/desktop/src/components/CanonPanel.tsx`, `apps/desktop/src/lib/i18n.tsx`, 테스트
  - 내용: 정산이 켜져 있을 때 원장 패널 위에 `정산 대기 n화`를 보인다. 누르면 회차 목록이 나온다. 세 언어를 쓴다.
  - 검증: vitest

### 작업 그룹 D: 외부 클라이언트와 측정

- [ ] **T6.D.1** — 쓰기 결과의 경고
  - 파일: `engine/internal/mcphost/tools_write.go`, `tools_revise.go`, 테스트
  - 내용:
    - `writeSceneOutput`에 `Warnings []string`을 더한다. 계획 없는 빈 회차에 썼다는 경고, 앞에 정산 대기 회차가 있다는 경고다.
    - 쓰기를 막지 않는다. 내장 에이전트는 문턱이 먼저 거절하므로 이 경고를 볼 일이 거의 없다.
    - 도구 설명에 경고의 뜻을 한 문장으로 더하고 `budget_test.go`의 상한을 다시 잰다.
  - 검증: `CGO_ENABLED=0 go test ./internal/mcphost/...`

- [ ] **T6.D.2** — 측정
  - 파일: `engine/internal/agent/log.go`, 이 문서의 측정 기록
  - 내용:
    - 활동 기록에 문턱 거절을 남긴다(`plan_gate`, `settlement_catchup`). 원문은 남기지 않고 회차 ID와 결과만 남긴다.
    - 작가 DB 복사본에서 2주 동안 다음을 센다: 새 회차당 계획 문턱 거절 수, 거절 뒤 추가 호출 수, 따라잡기 정산이 돈 횟수와 걸린 시간.
  - 검증: 측정 기록 작성

---

## ✅ Phase 6 Checkpoint

**구현 확인:**
- [ ] 모든 작업 체크박스 완료
- [ ] 정산이 꺼진 작가에게는 턴 앞뒤로 추가 호출이 없음
- [ ] 본문이 있는 회차를 고치는 턴은 계획 문턱에 걸리지 않음
- [ ] 외부 클라이언트의 쓰기는 거절되지 않고 경고만 받음

**자동 검증:**
- [ ] `CGO_ENABLED=0 go test ./...` 통과 (`engine/`)
- [ ] `go test -tags mobile ./...`와 `scripts/validate-story-core-deps.sh` 통과
- [ ] vitest, `tsc --noEmit`, 프런트 빌드 통과
- [ ] `scene_settlements`가 vault 왕복, 작품 복원, 작품 복제 뒤 남음

**수동 확인:**
- [ ] 복사본에서 계획 없는 빈 회차를 쓰라고 하면, 에이전트가 계획을 먼저 쓰고 본문을 쓴다
- [ ] 정산을 켜고 앞 회차 본문을 직접 고친 뒤 다음 회차를 쓰라고 하면, 그 회차가 먼저 정산된다
- [ ] 정산을 켠 날 이전의 회차가 대기로 잡히지 않는다

**완료 처리:** 측정 기록과 함께 작가에게 보고한다. 계획 문턱을 기본으로 켤지는 작가가 정한다. 승인 뒤 Phase 7로 간다.

---

## 작가 결정 필요

1. **계획 문턱의 기본값.** 권장은 켜짐이다. 브리프 문턱도 항상 켜져 있고, 비용은 새 회차당 호출 한 번이다. 계획 없이 바로 쓰는 것을 선호하면 설정 스위치를 더한다.
2. **계획의 최소 길이.** 지금 안은 "비어 있지 않음"이다. 한 줄짜리 계획이 문턱을 무력하게 만드는지는 측정으로 본다.

## 메모 / 주의

- 계획 문턱은 쓰는 사람이 에이전트일 때만의 규칙이다. 작가가 앱에서 직접 쓰는 본문에는 아무 문턱도 없다.
- 따라잡기 정산은 원고를 다시 모델에 보낸다. Phase 3의 설정 설명이 이미 이 사실을 말한다. 따라잡기를 더하면서 설명이 여전히 맞는지 확인한다.
- `Pending`의 이야기 순서는 `ledger.newTree`를 쓴다. 회상 회차처럼 이야기 순서와 작중 시간이 다른 경우는 이 페이즈에서 다루지 않는다.

---
_다음 페이즈: Phase 7 — 비밀과 앎 → [`phase-7-secrets-and-knowledge.md`](./phase-7-secrets-and-knowledge.md)_
