# Phase 4: 일관성 검수와 Canon Diff 승인

## 목표

AI 산출물이 기존 Canon과 충돌하는지 검수하고, 새 설정/관계/복선/연표 변화는 사람이 승인해야만 Canon memory에 반영되도록 한다. 이 phase가 Linetta를 일반 AI 글쓰기 도구와 구분하는 핵심이다.

## 핵심 원칙

- Canon Keeper는 제안자이지 결정권자가 아니다.
- 출판 가능한 장편을 만들려면 "좋은 문장"보다 "기억의 통제"가 먼저다.
- 모든 Canon 변경은 diff로 보여준다.
- 충돌은 무조건 막는 것이 아니라, 작가가 의도한 변화인지 확인하게 한다.

## 범위

- Canon conflict report
- Canon change proposal
- Memory Diff UI
- 승인/거절/보류 workflow
- episode draft revision loop

## 데이터 모델

```text
canon_change_proposals
  id
  work_id
  episode_id
  run_id
  target_item_id
  change_type           -- create, update, archive, link
  kind
  title
  before_body
  after_body
  reason
  confidence
  status                -- pending, approved, rejected, deferred
  created_at
  decided_at

continuity_issues
  id
  work_id
  episode_id
  run_id
  severity              -- info, warning, blocker
  title
  body
  related_item_ids
  status                -- open, accepted, resolved, ignored
  created_at
  updated_at
```

## 작업 목록

### 1. Canon proposal repository

- [ ] `internal/memory`에 proposal 모델 추가
- [ ] 함수 구현
  - `CreateProposal`
  - `ListProposals`
  - `ApproveProposal`
  - `RejectProposal`
  - `DeferProposal`
- [ ] 승인 시 동작
  - `create`: 새 canon item 생성
  - `update`: 기존 canon item 수정
  - `archive`: 기존 canon item archive
  - `link`: memory link 생성
- [ ] 모든 승인/거절은 `canon_decisions`에 기록한다.
- [ ] 테스트 추가
  - pending proposal 승인 시 Canon 반영
  - rejected proposal은 Canon 미반영
  - 승인 decision 기록

검증:

```sh
go test ./internal/memory/...
```

### 2. Continuity issue 모델과 API

- [ ] `internal/memory` 또는 `internal/review`에 issue 모델 추가
- [ ] API 추가
  - `GET /api/works/{workID}/episodes/{episodeID}/continuity`
  - `PATCH /api/continuity/{issueID}`
  - `GET /api/works/{workID}/proposals`
  - `POST /api/proposals/{proposalID}/approve`
  - `POST /api/proposals/{proposalID}/reject`
  - `POST /api/proposals/{proposalID}/defer`
- [ ] 테스트 추가
  - issue 목록
  - proposal 승인/거절
  - 다른 작품 proposal 접근 차단

검증:

```sh
go test ./internal/server/... ./internal/memory/...
```

### 3. Agent run에 검수 산출물 연결

- [ ] `internal/agent`에서 Canon Keeper 산출물을 structured output으로 만든다.
- [ ] deterministic MVP에서는 규칙 기반으로 최소 proposal을 생성한다.
  - draft에 새 인물명이 있으면 character draft proposal 후보
  - "처음", "과거", "몇 년 전" 같은 표현이 있으면 timeline 후보
  - 기존 canon item title과 다른 설명이 나오면 warning issue 후보
- [ ] 나중에 LLM 기반 structured extraction으로 교체 가능한 인터페이스로 둔다.
- [ ] 테스트 추가
  - draft 산출 후 proposal 생성
  - continuity issue 생성

검증:

```sh
go test ./internal/agent/... ./internal/memory/...
```

### 4. SwiftUI Memory Diff View

- [ ] `Views/MemoryDiffView.swift` 추가
- [ ] Episode Workbench 안에 "Canon Changes" 탭 추가
- [ ] proposal 목록
  - create/update/archive badge
  - before/after diff
  - reason
  - approve/reject/defer buttons
- [ ] Continuity issue 목록
  - severity
  - 관련 canon item 링크
  - accepted/ignored/resolved 처리
- [ ] 승인하면 Canon Memory 화면에 즉시 반영된다.

수동 확인:

- [ ] AI가 제안한 새 설정이 pending으로 표시된다.
- [ ] 승인 전에는 Canon에 반영되지 않는다.
- [ ] 승인 후 Canon Memory에 추가된다.
- [ ] 거절하면 Canon에 반영되지 않는다.

### 5. Revision loop

- [ ] Episode Workbench에 "Revise with selected issues" 액션 추가
- [ ] 사람이 continuity issue를 선택해 재수정 run을 시작할 수 있다.
- [ ] 수정 run은 기존 draft와 issue 목록을 입력으로 받는다.
- [ ] 새 artifact는 이전 artifact를 덮어쓰지 않고 version으로 저장한다.

검증:

```sh
go test ./...
```

---

### Checkpoint: Phase 4 완료 확인

**구현 확인:**
- [ ] AI가 Canon 변경을 직접 반영하지 않는다.
- [ ] Memory Diff에서 승인/거절/보류가 가능하다.
- [ ] Continuity issue가 에피소드와 연결된다.
- [ ] 선택한 issue로 revision run을 만들 수 있다.

**실행 확인:**
- [ ] `go test ./...` 통과
- [ ] `xcodebuild ... test` 통과

**사용자 확인:**
- [ ] "장편 연재 중 설정 붕괴를 막아주는 느낌"이 드는지 확인받는다.

이 체크포인트를 통과하면 사용자에게 확인 요청 후 Phase 5로 진행한다.
