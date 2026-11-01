# 에이전트 메모리와 맥락 검색 — 로드맵

_이 문서는 전체 계획의 총괄본이다. 구현 전에 이 로드맵을 먼저 읽고, 해당 페이즈 작업지시서로 이동한다. 페이즈 전환은 작가(저장소 소유자)의 명시적 승인 후에만 한다._

_작성일: 2026-11-01_
_상태: 구현 완료, 병합 대기. 남은 것은 실제 모델 수동 확인과 Phase 4 조건부 도구의 측정_
_예상 전체 소요: 4~5일_
_페이즈 수: 4개_

## Overview

긴 작품에서 에이전트 대화는 앞에서부터 잘린다. 잘린 내용은 패널에는 남아 있지만 에이전트는 다시 볼 수 없다. 지금 잘림 안내는 "작가에게 물어보라"고만 한다.

[Obelisk](https://github.com/tommy0103/obelisk)의 구조를 참고해 이 문제를 푼다. 원본 대화를 남기고, 필요할 때 검색하고, 잘린 지점의 위치를 알려 준다. 요약으로 작가의 말을 대신하지 않는다는 [prompt.go](../../../engine/internal/agent/prompt.go)의 기존 원칙도 유지한다.

Obelisk 코드는 가져오지 않는다. Node 런타임, 셸로 실행하는 쿼리, 코딩 에이전트 로그 전용 수집기가 리네타 구조와 맞지 않고, 외부 AGPL 코드를 넣으면 리네타의 별도 상용 라이선스 여지가 막힐 수 있다. 설계만 참고해 Go로 직접 만든다.

## 가치 제안

> 연재가 길어져도 에이전트는 작가와 나눈 결정을 스스로 찾아보고, 작업 노트가 차도 오래된 결정을 잃지 않는다.

## 근거 (2026-11-01 실측)

설치된 앱의 DB 복사본으로 측정했다. 작품 6개, 196턴, 사용자 한 명의 약 3주치 기록이다.

- 196턴 중 78턴(40%)에서 앞 대화가 잘렸다. 모두 긴 작품 2개에서 나왔다. 대화 창의 시작점이 실제로 움직인 건 10번이다.
- 두 작품의 대화 중 52~62%가 현재 창 밖에 있다.
- "앞 대화가 안 보인다"는 응답은 0건이고, 잘린 내용을 작가가 직접 가리킨 뚜렷한 사례도 찾지 못했다.
- 잘린 구간의 작가 지시 표본 15개 중 13개는 작업 노트, 문체, 원장, 인물 중 한 곳에서 찾았다. 표본 확인은 단어 존재만 본 것이다.
- 작업 노트는 2158/2200자, 1964/2200자로 거의 찼다. 뒤쪽 작품은 100화 계획에 원고가 8개 정도다.
- Obelisk의 대화 인덱스와 같은 기본 unicode61 토크나이저로 재현하면 단어 일치 재현율이 인물명 65%, 개념어 54%다. 리네타의 trigram+LIKE 조합은 부분 문자열 기준 100%다.

따라서 눈앞의 피해보다 작품이 길어질 때의 병목을 먼저 막는 작업이다. Phase 1 뒤에 실제로 쓰이는지 재서 다음 범위를 조정한다.

## 결정 사항

- **대화 원본이 진실이고 색인은 파생 데이터다.** 색인은 기기 로컬이고 vault 번들에 싣지 않는다. `companion_messages`가 이미 그렇다.
- **색인 대상은 내장 에이전트의 사용자·어시스턴트 메시지다.** `intent='agent'`만 넣어 1.0 companion의 기록이 섞이지 않게 한다. 도구 행은 160자 잘린 요약이라 넣지 않는다.
- **대화 검색 도구는 내장 에이전트 전용이다.** 대화에는 작가가 붙여 넣은 글과 사적인 메모가 있다. 외부 MCP 클라이언트에는 내주지 않는다.
- **현재 상태가 과거 대화보다 우선한다.** 도구 설명과 시스템 프롬프트에 명시한다. 실측에서도 30화 계획이 나중에 10화로 바뀐 사례가 있었다.
- **검색 결과는 데이터이지 지시가 아니다.** 붙여 넣은 원고의 문장이 지시처럼 읽혀도 따르지 않게 도구 설명에 적는다.
- **자동 요약은 넣지 않는다.** 보관은 에이전트가 줄 단위로 명시적으로 옮기는 동작이다.

## 완료 조건 (전체)

- [x] 잘린 대화에 있던 결정을 에이전트가 도구로 찾아 답한다.
- [x] 대화 지우기와 작품 삭제 뒤 색인에 해당 내용이 남지 않는다.
- [x] 외부 MCP 클라이언트는 대화 검색 도구를 볼 수 없다.
- [x] 작업 노트의 줄을 지우지 않고 보관하고, 검색으로 되찾을 수 있다.
- [x] 보관 메모를 작가가 설정에서 보고 지울 수 있다.
- [x] 대화 한도가 공급자에 맞게 정해지고 작가가 바꿀 수 있다.
- [x] 앱 전체 검색이 원고 JSON의 키 이름에 걸리지 않는다.

## 기술 스택 / 환경

- **엔진**: Go, `modernc.org/sqlite`, FTS5 trigram (`0015_manuscript_fts.sql` 선례)
- **앱**: Tauri 2, React 18, TypeScript (`apps/desktop`)
- **도구 표면**: `engine/internal/mcphost`, 내장 에이전트는 같은 도구를 인메모리로 쓴다
- **로컬 검증**: `CGO_ENABLED=0 go test ./...` (`engine/`에서 실행). cgo 빌드는 macOS 키체인을 건드려 멈출 수 있다. `-race`는 CI에 맡긴다.
- **모바일 빌드**: `make test-mobile-engine`. 마이그레이션은 모바일에도 적용된다.

## Out of Scope

- **Obelisk 도입과 코드 복사**: 위 Overview 참고
- **턴 중간 컨텍스트 교체(rollover)**: 현재 설정인 claude-code 공급자는 턴 안의 루프를 CLI가 돌린다. 공급자 공통으로 적용할 수 없다.
- **모델이 쓰는 인계문**: 작업 노트와 원장이 이미 그 역할을 한다. Phase 1 측정 뒤 턴 경계에서만 다시 검토한다.
- **임베딩·벡터 검색**: 의미 검색의 약점은 먼저 검색어를 바꿔 여러 번 찾는 방식으로 메운다.
- **도구 결과 원문 저장**: 읽기 결과는 현재 원고에서, 쓰기는 스냅샷에서 복원할 수 있다.
- **대화 기록의 기기 간 동기화**: `companion_messages`는 기기 로컬로 둔다.
- **`writer_profile` 보관**: 작품 단위 `work_notes`만 다룬다.

## 페이즈 구성

각 페이즈는 끝나면 동작하는 무언가가 나오는 수직 슬라이스다.

### Phase 1: 대화 검색 — 잘린 대화를 에이전트가 찾는다

- **목표**: 에이전트가 잘린 대화를 검색하고 앞뒤를 읽는다. 잘림 안내가 경계 위치를 알려 준다.
- **포함 기능**: 대화 FTS, 검색·읽기 도구, 잘림 위치 표시
- **예상 소요**: 1.5일
- **작업지시서**: [`phase-1-history-search.md`](./phase-1-history-search.md)
- **Checkpoint 요약**: 실제 DB 복사본에서 잘린 결정을 찾고, 대화 지우기 뒤 색인이 비며, 외부 서버에 도구가 없다.

### Phase 2: 작업 노트 보관 — 노트가 차도 결정을 잃지 않는다

- **목표**: 노트 줄을 보관 영역으로 옮기고 검색으로 되찾는다. 쓰이지 않던 키워드 기억을 이 경로로 합친다.
- **포함 기능**: 보관 테이블, `archive` 동작, 설정 UI, `experiences.jsonl` 경로 정리
- **예상 소요**: 1.5일
- **작업지시서**: [`phase-2-note-archive.md`](./phase-2-note-archive.md)
- **Checkpoint 요약**: 보관한 줄이 예산에서 빠지고 검색되며, vault 왕복과 복원 뒤에도 남는다.

### Phase 3: 모델별 대화 한도 — 큰 모델은 덜 자른다

- **목표**: 대화 한도를 공급자별로 정하고 작가가 조정한다.
- **포함 기능**: 한도 설정, 턴 단위 읽기, 설정 UI
- **예상 소요**: 0.5일
- **작업지시서**: [`phase-3-history-budget.md`](./phase-3-history-budget.md)
- **Checkpoint 요약**: 스파이크로 CLI 입력 경로를 확인하고, 실측 시뮬레이션에서 잘린 턴이 78턴에서 0턴으로 줄었다(재시작 없는 경우).

### Phase 4: 검색 정리 — 앱 검색을 색인으로 옮긴다

- **목표**: 앱 전체 검색을 `manuscript_fts`로 옮긴다. 측정 결과가 뒷받침하면 에이전트 통합 검색을 더한다.
- **포함 기능**: 앱 검색 전환, 조건부 통합 도구
- **예상 소요**: 1일
- **작업지시서**: [`phase-4-unified-search.md`](./phase-4-unified-search.md)
- **Checkpoint 요약**: `paragraph` 같은 JSON 키로 원고가 걸리지 않는다.

## 페이즈 간 의존성

```
Phase 1 (대화 검색)
  ├─→ Phase 2 (작업 노트 보관)
  └─→ Phase 4 (검색 정리, 공용 쿼리 패키지 재사용)

Phase 3 (대화 한도)은 독립
```

**병렬 가능성**: Phase 3은 Phase 1과 파일이 일부 겹치므로(`agent/prompt.go`) 동시에 진행하지 않고 순서대로 한다. 순서는 1 → 2 → 3 → 4를 기본으로 한다.

## 측정

Phase 1 배포 후 2주간 쓰고 아래 쿼리로 본다. 데이터는 기기 안에만 두고 외부로 보내지 않는다.

```sql
SELECT json_extract(content, '$.name') AS tool, COUNT(*) AS calls, SUM(json_extract(content, '$.ok')) AS ok
  FROM companion_messages
 WHERE role = 'tool' AND intent = 'agent'
   AND json_extract(content, '$.name') IN ('linetta_search_history', 'linetta_read_history')
 GROUP BY 1;
```

호출이 드물면 Phase 4의 통합 도구는 하지 않는다. 쓸 만한 결과가 안 나오는 호출이 많으면 도구 설명과 결과 형식부터 고친다. Obelisk의 [이슈 #16](https://github.com/tommy0103/obelisk/issues/16)도 첫 검색의 낭비를 보고한다.

## 페이즈 간 전환 규칙

각 페이즈는 다음을 만족해야 완료로 본다.

1. 작업지시서의 모든 체크박스 완료
2. Checkpoint의 모든 검증 통과
3. 작가가 명시적으로 다음 진행을 승인

Checkpoint가 실패하면 원인을 보고하고, 함께 파악해 수정한 뒤 다시 검증한다.

페이즈마다 `main`에서 새 브랜치를 따서 PR을 연다. 병합은 작가가 승인한 뒤에만 한다.

## 최종 완료 체크리스트

- [x] 모든 페이즈 Checkpoint 통과
- [x] `CGO_ENABLED=0 go test ./...` 통과 (`engine/`)
- [x] `make test-mobile-engine`과 같은 `go test -tags mobile ./...` 통과
- [x] vitest, `tsc --noEmit`, 프런트 빌드 통과
- [x] 긴 작품 복사본으로 잘린 결정 찾기 시나리오 확인
- [x] 대화 지우기, 작품 삭제 뒤 색인 삭제 확인
- [x] Out of Scope 항목 미구현 확인
- [x] `docs/privacy-policy.md`와 README 반영. `CHANGELOG.md`는 릴리스 커밋에서 쓴다

## 참고 자료

- Obelisk 컨텍스트 교체 명세: `docs/dsh-context-window-extra-plugin-spec.md` (Obelisk 저장소)
- Obelisk 제한 조회 설계: `docs/adr/0017-bounded-messages-and-default-retrieval-surface.md` (Obelisk 저장소)
- 대화 창 규칙: [`engine/internal/agent/prompt.go`](../../../engine/internal/agent/prompt.go)
- 작업 노트: [`engine/internal/agentmemory/agentmemory.go`](../../../engine/internal/agentmemory/agentmemory.go)
- 원고 검색: [`engine/internal/manuscript/searcher.go`](../../../engine/internal/manuscript/searcher.go)
