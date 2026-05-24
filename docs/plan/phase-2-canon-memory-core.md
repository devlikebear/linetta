# Phase 2: Canon Memory Core

## 목표

작품 전체를 지탱하는 Canon memory를 만든다. 이 phase가 끝나면 작가는 작품별로 캐릭터, 세계관, 연표, 복선, 스타일, 자료를 저장하고 다시 확인할 수 있다.

## 핵심 원칙

- Canon memory는 작품의 뼈대다.
- AI는 Canon을 직접 수정할 수 없다.
- 모든 Canon 변경은 `Decision` 또는 `ChangeProposal` 기록을 남긴다.
- 장편 연재에서 중요한 것은 "많이 기억하는 것"보다 "확정된 사실과 미확정 아이디어를 분리하는 것"이다.

## 범위

- Canon item 공통 모델
- 캐릭터/세계관/연표/복선/스타일/자료 CRUD
- Canon decision log
- SwiftUI Canon Memory 화면

## 데이터 모델

```text
canon_items
  id
  work_id
  kind                  -- character, world_fact, timeline_event, plot_thread, style_rule, source
  title
  body
  status                -- draft, canon, archived
  importance            -- low, medium, high
  created_at
  updated_at

canon_decisions
  id
  work_id
  canon_item_id
  decision_type         -- create, update, archive, approve
  reason
  actor                 -- human, system
  created_at

memory_links
  id
  work_id
  from_item_id
  to_item_id
  relation              -- affects, contradicts, references, resolves
```

필요하면 `characters`, `timeline_events` 같은 특화 table을 별도로 둘 수 있지만, MVP는 `canon_items.kind` 기반으로 단순하게 시작한다.

## 작업 목록

### 1. Memory repository 추가

- [ ] `internal/memory` 패키지 추가
  - `type Item struct`
  - `type Kind string`
  - `type Status string`
  - `type CreateItemInput struct`
  - `type UpdateItemInput struct`
  - `type Repository struct`
- [ ] 함수 구현
  - `CreateItem`
  - `UpdateItem`
  - `ArchiveItem`
  - `ListItems`
  - `GetItem`
  - `RecordDecision`
- [ ] 테스트 추가
  - 작품별 memory isolation
  - `draft -> canon -> archived` 상태 전이
  - 변경마다 decision 기록 생성

검증:

```sh
go test ./internal/memory/...
```

### 2. Canon API 추가

- [ ] `internal/server`에 API 추가
  - `GET /api/works/{workID}/memory`
  - `POST /api/works/{workID}/memory`
  - `GET /api/works/{workID}/memory/{itemID}`
  - `PATCH /api/works/{workID}/memory/{itemID}`
  - `POST /api/works/{workID}/memory/{itemID}/archive`
  - `GET /api/works/{workID}/memory/decisions`
- [ ] query 지원
  - `?kind=character`
  - `?status=canon`
- [ ] 테스트 추가
  - item 생성/수정/목록
  - 다른 작품 memory 접근 차단
  - decision log 반환

검증:

```sh
go test ./internal/server/...
```

### 3. SwiftUI Canon Memory 화면

- [ ] `Views/CanonMemoryView.swift` 추가
- [ ] 작품 작업실에 Canon Memory 탭 추가
- [ ] segmented control 또는 sidebar로 kind 전환
  - Characters
  - World
  - Timeline
  - Plot Threads
  - Style
  - Sources
- [ ] item 목록 + 상세 편집 pane 구성
- [ ] 저장 시 API 호출
- [ ] 빈 상태에서는 "첫 캐릭터/세계관 설정 추가" 액션 제공

수동 확인:

- [ ] 새 작품에서 캐릭터를 추가할 수 있다.
- [ ] 세계관 규칙을 저장하고 다시 열 수 있다.
- [ ] 연표 항목을 추가해도 다른 작품에는 보이지 않는다.

### 4. Memory 검색 최소 기능

- [ ] `GET /api/works/{workID}/memory/search?q=...` 추가
- [ ] MVP 검색은 SQL `LIKE` 기반으로 충분하다.
- [ ] SwiftUI에서 현재 작품 memory 검색 필드 추가
- [ ] 검색 결과에서 item 상세로 이동한다.

검증:

```sh
go test ./internal/memory/... ./internal/server/...
```

---

### Checkpoint: Phase 2 완료 확인

**구현 확인:**
- [ ] 작품별 Canon memory가 저장된다.
- [ ] Canon item 상태와 decision log가 남는다.
- [ ] SwiftUI에서 memory를 종류별로 보고 수정할 수 있다.

**실행 확인:**
- [ ] `go test ./...` 통과
- [ ] `xcodebuild ... test` 통과

**사용자 확인:**
- [ ] "이 정도면 장편 작품의 설정 뼈대를 관리할 수 있다"는 감각이 드는지 확인받는다.

이 체크포인트를 통과하면 사용자에게 확인 요청 후 Phase 3으로 진행한다.
