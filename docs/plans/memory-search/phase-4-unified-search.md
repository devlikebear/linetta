# Phase 4: 검색 정리 — 작업지시서

_작성일: 2026-11-01_
_속한 로드맵: [`roadmap.md`](./roadmap.md)_
_예상 소요: 1일_

## 페이즈 목표

앱 전체 검색이 원고 JSON 원문 대신 `manuscript_fts` 색인을 쓴다. Phase 1 측정이 뒷받침하면 에이전트가 원고 밖 자료까지 한 번에 찾는 도구를 더한다.

## 전제 조건

- [ ] Phase 1 완료 및 승인
- [ ] `main`에서 `feat/app-search-fts` 브랜치 생성
- [ ] 로드맵의 측정 쿼리 결과 확인. 작업 그룹 C의 진행 여부를 정한다.

## 포함 기능

1. 앱 전체 검색의 색인 전환
2. 색인이 없는 작품의 재구축
3. (조건부) `linetta_search_work`

## 이 페이즈에서 하지 않는 것

- 앱 검색에 대화 내용 포함 → 하지 않는다. 앱 검색은 작품 자료를 찾는 기능이다.
- `linetta_search_manuscript` 제거 → 하지 않는다. 외부 MCP 클라이언트가 쓴다.
- 임베딩 → Out of Scope

## 작업 체크리스트

### 작업 그룹 A: 색인 준비

- [ ] **T4.A.1** — 누락 색인 재구축
  - 파일: `engine/internal/manuscript/indexer.go`, `manuscript_test.go`
  - 내용:
    - `rebuildIfEmpty`는 작품별 검색 때만 돈다. 앱 전체 검색은 색인이 없는 작품을 놓친다.
    - `Indexer.RebuildMissing(ctx)`를 더한다. 원고가 있는데 `manuscript_fts` 행이 없는 작품만 다시 만든다.
    - 앱 시작 후 백그라운드에서 한 번 돈다. 시작을 막지 않는다.
  - 참조: `Rebuild`, `RebuildAll`, `ProjectRowCount`
  - 검증: `CGO_ENABLED=0 go test ./internal/manuscript/...`

### 작업 그룹 B: 앱 검색 전환

- [ ] **T4.B.1** — 본문 검색 교체
  - 파일: `engine/internal/search/search.go`, `search_test.go`, `engine/internal/rpc/handlers/search_test.go`
  - 내용:
    - `lower(content_doc) LIKE`를 제거한다. JSON의 키 이름까지 걸리고 작품이 커질수록 느려진다.
    - 제목, 라벨, 작품명 LIKE는 유지한다. 본문은 `manuscript_fts`로 찾는다.
    - 3글자 이상은 trigram, 3글자 미만이 있으면 `manuscript_fts.plain`을 LIKE로 찾는다. `ftsquery`를 쓴다.
    - 순위는 라벨, 제목, 작품명 일치를 먼저 두고 그다음 본문이다. 기존 `rank` 순서를 유지한다.
    - 미리보기는 색인 본문에서 만든다. `docToPlainText`를 더는 쓰지 않으면 제거한다.
  - 테스트: `paragraph`로 검색해도 본문에 없으면 안 걸림, 2글자 이름, 대소문자, 보관 작품 제외, 빈 제목 미리보기(#45)
  - 검증: `CGO_ENABLED=0 go test ./internal/search/... ./internal/rpc/...`

### 작업 그룹 C: 통합 검색 (조건부)

_Phase 1 측정에서 에이전트가 검색을 실제로 쓰고, 원고 밖 자료를 여러 도구로 뒤지는 패턴이 보일 때만 진행한다. 아니면 건너뛰고 이 사실을 PR에 적는다._

- [ ] **T4.C.1** — `linetta_search_work`
  - 파일: `engine/internal/mcphost/tools_read.go`, `tools_read_test.go`
  - 내용:
    - 입력: `project_id`, `query`, `sources`(선택: `entities`, `ledger`, `facts`, `continuity`, `plans`), `limit`
    - 인물 요약, 원장, 자료 카드, 연속성 관찰, 회차 계획을 LIKE로 찾아 출처별로 돌려준다. 규모가 작아 새 색인은 만들지 않는다.
    - 원고는 포함하지 않는다. `linetta_search_manuscript`가 있다.
    - 외부 클라이언트에도 열리므로 `ReadToolNames`에 추가하고, 도구 설명 길이는 도구 예산 테스트에 맞춘다.
  - 검증: `CGO_ENABLED=0 go test ./internal/mcphost/...`

---

## ✅ Phase 4 Checkpoint

**구현 확인:**
- [ ] 모든 작업 체크박스 완료 (작업 그룹 C를 건너뛰면 사유 기록)
- [ ] 앱 검색이 본문 JSON을 더는 읽지 않는다

**자동 검증:**
- [ ] `CGO_ENABLED=0 go test ./...` 통과 (`engine/`)
- [ ] `make test-mobile-engine` 통과
- [ ] `make test-desktop` 통과

**수동 확인:**
- [ ] DB 복사본에서 인물명, 2글자 이름, 작품에 없는 단어 `paragraph`로 검색해 결과 확인
- [ ] 색인을 비운 복사본을 띄워 재구축 뒤 검색되는지 확인

**완료 처리:**
1. 완료 작업, 자동 검증 결과, 남은 수동 확인을 보고한다.
2. PR을 열고 작가 승인을 받는다.
3. 로드맵의 최종 완료 체크리스트를 확인한다.

---

## 메모 / 주의

- 앱 검색은 모든 작품을 가로지른다. `manuscript_fts`의 `project_id`는 UNINDEXED이므로 조인으로 보관 작품을 뺀다.
- 원고 저장 경로가 색인을 항상 갱신하는지 다시 확인한다. 어긋나면 앱 검색 결과가 오래된 내용을 보여 준다.

---
_마지막 페이즈. 완료 후 로드맵의 최종 완료 체크리스트를 확인한다._
