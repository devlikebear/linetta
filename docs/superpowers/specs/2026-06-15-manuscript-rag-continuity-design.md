# 로컬 RAG — 본문 전문 검색 기반 설정 일관성 (방향 A)

_작성일: 2026-06-15_
_상태: 구현 완료 (v0.4.20 이후 main)_
_관련: [`docs/plans/webnovel-ux/companion-writing-actions.md`](../../plans/webnovel-ux/companion-writing-actions.md)의 `checkContinuity` 프리셋_

## 배경 / 왜 이 방향인가

처음 출발점은 "Google NotebookLM을 리네타에 연동할 수 있는가"였다. 결론:

- NotebookLM은 **공개 소비자 API가 없다.** Enterprise API만 존재하며 GCP 조직 계약 + allowlist 기반이라 개인 사이드 프로젝트에 부적합하다.
- 작가 원고를 외부 노트북에 업로드하는 것은 통제권·저작권 측면에서 바람직하지 않다.
- 리네타는 이미 `pkg/llm`으로 Claude/Gemini를 쓰고 SQLite에 본문을 보유 — NotebookLM이라는 블랙박스 대신 **로컬에서 같은 효과(근거 기반 질의·일관성 점검)** 를 직접 구성하는 게 낫다.

따라서 외부 연동이 아니라 **로컬 RAG**로 간다. 1순위 사용 시나리오는 **설정 일관성 체크**다:
"3화에서 주인공 눈 색이 뭐였지?", "이 설정이 앞 회차와 모순되나?" 같은 고유명사·사실 대조.

### 현재 코드베이스에서 비어 있는 단 하나

리네타 컴패니언은 이미 **에이전트형 검색**(`linetta-query` 블록)을 갖고 있다. 노출 도구:
`search_entities`, `get_scene_text`, `list_scenes`, `list_beats`, `recall_memory`.
구조화된 설정 데이터(`fact.Card`, `entity.Entity`, `relationship`)도 이미 있다.

빠진 것은 **본문(prose) 전체를 가로지르는 전문 검색** 하나다. 현재 전역 검색(`engine/internal/search`)은
`nodes.content_doc`에 대한 `LIKE` 부분일치 스캔이고("FTS는 나중에" 주석), 컴패니언이 등록된 설정과
실제 본문 묘사를 대조하려 해도 본문을 의미/키워드로 훑을 수단이 없다. 이 공백을 메우는 것이 이 작업이다.

## 선택한 접근법: FTS5 검색 도구를 `linetta-query`에 추가

대안 비교는 [부록 A](#부록-a-대안-비교)에 둔다. 요약:

- **A. FTS5(trigram) lexical 검색 도구 — 선택.** 오프라인·무료·순수 Go·Windows 빌드 안전, 단일 커넥션 친화. 기존 에이전트형 검색에 도구 1개 추가라 표면적 최소. 고유명사·사실 대조에 lexical이 정확.
- B. Gemini 임베딩 + Go 브루트포스 코사인 — 의미 검색은 강하나 네트워크·Gemini 종속·재임베딩 비용. 1순위(정확 대조)엔 과투자.
- C. 하이브리드 — 가장 강력하나 구현·유지비 최대. YAGNI 위반.

### 실현가능성 확정 (구현 전 검증 완료)

- `modernc.org/sqlite` v1.50.1에서 `CREATE VIRTUAL TABLE ... USING fts5(..., tokenize='trigram')` 가 **컴파일·실행됨.** C 확장 불필요 → Windows 크로스컴파일 유지.
- Tiptap JSON → 평문 추출기가 이미 존재 (`search.docToPlainText`, `ai/context.go`, 컴패니언 `plainTextFromDoc`, `snapshot.plaintextFromDoc`).
- 마이그레이션은 번호 SQL 파일(`store/migrations/`, 구현 당시 최신 `0014_companion_references.sql`).
- `companion/query.go`의 도구 디스패치는 단순 `switch`라 `case` 추가가 idiomatic.

## 아키텍처

```
노드 저장(content_doc) ──▶ Indexer.Upsert ──▶ manuscript_fts (FTS5 trigram, 앱 관리형)
                                                      │
컴패니언 LLM ── linetta-query{search_manuscript} ──▶ Searcher.Query ──▶ 랭킹된 스니펫
                                                      │
        모델이 [node_id]로 get_scene_text 호출 → entity/fact와 대조 → 모순 목록(본문 자동 변경 없음)
```

새 패키지 **`engine/internal/manuscript`** — 단일 책임: 본문 prose의 FTS 인덱스 관리 + 검색.
기존 `engine/internal/search`(데스크톱 UI용 전역 LIKE 검색)와 목적이 달라 분리한다. 두 패키지는
서로를 import하지 않는다.

## 데이터 모델

마이그레이션 `store/migrations/0015_manuscript_fts.sql`:

```sql
CREATE VIRTUAL TABLE manuscript_fts USING fts5(
  plain,                       -- 본문 평문 (인덱싱 대상)
  node_id    UNINDEXED,
  project_id UNINDEXED,
  tokenize = 'trigram'
);
```

설계 근거:

- **앱 관리형 인덱스(트리거 아님).** `content_doc`는 Tiptap JSON이라 SQL 트리거가 평문을 추출할 수 없다.
  Go에서 `docToPlainText`로 변환한 뒤 upsert한다.
- **trigram 토크나이저.** 한국어 고유명사 부분일치에 강하고 별도 형태소 사전이 필요 없다.
- `node_id`/`project_id`는 `UNINDEXED` 메타 컬럼 — 필터·역참조용이며 전문 검색 대상이 아니다.

## 컴포넌트

### `Indexer` (`engine/internal/manuscript/indexer.go`)

- `Upsert(ctx, projectID, nodeID, contentDoc string) error`
  노드 저장/오토세이브 커밋 경로에서 호출. `DELETE FROM manuscript_fts WHERE node_id=?` 후
  평문 `INSERT`. 두 statement 모두 단일 실행 — 단일 커넥션 안전.
  leaf 노드(씬)만 색인한다(폴더/그룹 노드 제외).
- `Delete(ctx, nodeID string) error`
  노드 삭제 시 해당 행 제거.
- `Rebuild(ctx, projectID string) error`
  백필/복구용. **단일 커넥션 제약 준수**: `(node_id, content_doc)`를 전부 메모리로 읽어
  Rows를 닫은 **뒤**, 트랜잭션으로 평문 추출·재삽입한다. Rows를 열어둔 채 다른 쿼리를 실행하지 않는다.
  먼저 `DELETE FROM manuscript_fts WHERE project_id=?`로 기존 행을 비우고 재구축.

### `Searcher` (`engine/internal/manuscript/searcher.go`)

- `Query(ctx, projectID, q string, limit int) ([]Hit, error)`
  ```sql
  SELECT node_id, snippet(manuscript_fts, 0, '…', '…', '…', 12), bm25(manuscript_fts)
    FROM manuscript_fts
   WHERE manuscript_fts MATCH ? AND project_id = ?
   ORDER BY bm25(manuscript_fts)
   LIMIT ?;
  ```
  `Hit{NodeID, Breadcrumb, Snippet}`. 브레드크럼은 `node.BreadcrumbLabel`로 보강(기존 `list_scenes`와 동일 방식).
  `limit` 기본 5, 상한 20.
- `buildTrigramMatch(q string) (string, bool)` — trigram MATCH 쿼리 빌더.
  - FTS5 특수 연산자(`"`, `*`, `(`, `)`, `:`, `^`, `AND`/`OR`/`NOT` 등)를 이스케이프/제거.
  - 공백으로 분리한 각 term을 `"..."`로 인용해 AND 결합.
  - **3자 미만 term은 trigram이 색인하지 못한다.** 모든 term이 3자 미만이면 `(_, false)`를 반환해
    호출부가 **LIKE 폴백 경로**(`plain LIKE '%q%'`)로 전환한다. 짧은 한국어 이름("수아") 대응.

### 평문 추출 재사용

기존에 동일 로직이 4곳에 중복돼 있다(`search`, `ai`, `companion`, `snapshot`). 이 작업에서는
**대규모 리팩터링을 하지 않는다**(YAGNI / 최소 스코프). `manuscript` 패키지는 가장 가까운 구현
(`search.docToPlainText`)을 재사용하거나 동등한 로컬 헬퍼를 둔다. 중복 통합은 별도 정리 과제로 남긴다.

## 컴패니언 통합

### `engine/internal/companion/query.go`

도구 디스패치 `switch`에 추가:

```go
case "search_manuscript":
    q := strings.TrimSpace(qry.Args["query"])
    if q == "" {
        return "(오류: query 필요)"
    }
    limit := parseLimitArg(qry.Args["limit"], 5, 20)
    hits, err := s.manuscript.Query(ctx, projectID, q, limit)
    if err != nil {
        return "(오류: " + err.Error() + ")"
    }
    if len(hits) == 0 {
        return "(검색 결과 없음)"
    }
    // "- [node_id] 브레드크럼: …스니펫…" 형식 (기존 list_scenes 톤과 동일)
```

반환 포맷은 모델이 곧바로 `get_scene_text(node_id)`로 전문을 받을 수 있도록 `node_id`를 노출한다.

### `engine/internal/companion/prompt.go`

도구 안내에 한 줄 추가:

> `search_manuscript(query, limit?)` — 본문 전체에서 고유명사·설정 묘사가 나오는 대목을 찾는다.
> 결과의 node_id로 `get_scene_text`를 호출해 전문을 확인하라. 패러프레이즈에 약하므로
> 동의어가 의심되면 여러 표현으로 검색하라.

### 설정 일관성 흐름 (에이전트형, 별도 파이프라인 없음)

1. 모델이 `search_entities` / fact로 **등록된 설정**을 확인한다.
2. `search_manuscript`로 **본문 속 실제 묘사**를 검색한다.
3. 후보 `node_id`에 대해 `get_scene_text`로 전문을 받는다.
4. 등록 설정과 본문 묘사를 대조해 **모순 목록**을 제시한다. 본문을 자동 변경하지 않는다.

`companion-writing-actions.md`의 `checkContinuity` 프리셋이 구현되면 그 프롬프트가 위 절차를
안내하도록 연결한다. 단, 이 검색 도구는 **프리셋과 독립적으로** 어떤 프롬프트에서도 사용 가능하다
(프리셋 구현은 이 설계의 선행 조건이 아니다).

## 백필 전략

- 마이그레이션 `0015`는 **스키마만** 만든다. 기존 프로젝트의 인덱스는 비어 있다.
- **지연 재구축(lazy rebuild):** `search_manuscript` 실행 시 해당 `project_id` 행이 0이면 먼저
  `Indexer.Rebuild(projectID)`를 1회 수행한 뒤 검색한다. 이후에는 저장 훅이 증분으로 유지한다.
- 데이터 이전 마이그레이션이 필요 없고, 단일 커넥션 제약(메모리 선적재) 안에서 안전하다.

## 인덱싱 훅 (저장 경로 연결)

노드 `content_doc`가 영속화되는 경로(노드 업데이트/오토세이브 커밋)에서 `Indexer.Upsert`를,
삭제 경로에서 `Indexer.Delete`를 호출한다. 정확한 호출 지점은 구현 시 노드 저장 RPC/repo 경로를
따라 확정한다(`project/repo.go`의 `content_doc` 영속화 지점 기준). 인덱싱은 저장 트랜잭션과
**느슨하게 결합**한다 — 인덱싱 실패가 본문 저장을 막아서는 안 된다(아래 오류 처리).

## 오류 처리 / 비기능 요구

- **인덱싱은 best-effort.** Upsert/Delete 실패는 본문 저장을 막지 않고 로깅만 한다. 검색 품질은
  떨어질 수 있어도 본문 데이터는 불변. 다음 저장 또는 `Rebuild`에서 자연 복구된다.
- 빈 쿼리 → `(오류: query 필요)`. 무결과 → `(검색 결과 없음)`. (기존 도구 톤 일치)
- **프로젝트 범위 한정.** 크로스 프로젝트 검색은 하지 않는다(`project_id` 필터 필수).
- 단일 SQLite 커넥션 제약 준수: 검색=단일 SELECT, 증분=단일 statement, Rebuild=메모리 선적재 후 트랜잭션.

## 테스트

- `manuscript` 패키지
  - `Upsert` → `Query` 라운드트립 (한국어 본문, 고유명사 검색).
  - `Delete` 후 검색 결과에서 제외.
  - `Rebuild` — Rows를 먼저 닫고 재삽입하는지(단일 커넥션에서 데드락 없이 완료).
  - `buildTrigramMatch` — 한국어 다중 term AND, 공백 처리, FTS 연산자 이스케이프, **3자 미만 → LIKE 폴백**.
- `companion/query_test.go` — `search_manuscript` 디스패치와 결과 포맷(`[node_id]` 포함, 무결과 문자열).
- `companion/prompt_test.go` — 시스템 프롬프트에 `search_manuscript` 안내 포함.
- 전체: `make test`.

## 하지 않는 것 (YAGNI)

- **임베딩/의미 검색.** 의미 기반 유사장면이 1순위로 올라오면 `manuscript` 패키지에 벡터 Searcher를
  추가해 확장(방향 B). 지금 스키마/인터페이스는 그 확장을 막지 않게만 둔다.
- **키 입력마다 실시간 인덱싱.** 저장/오토세이브 커밋 시점에만 색인한다.
- **새 UI 패널.** 기존 컴패니언·proposal·context 선택 UI를 재사용한다. 프론트엔드 변경 없음.
- **크로스 프로젝트 검색, 새 LLM provider, 평문 추출기 통합 리팩터링.**

## 알려진 한계

- **패러프레이즈에 약함**("붉은 눈" ↔ "진홍빛 동공"). lexical 검색의 본질적 한계 — 프롬프트로
  동의어 재검색을 유도해 완화한다. 근본 해결은 향후 임베딩(방향 B).
- **2자 이하 고유명사는 trigram 미색인** → LIKE 폴백 + `search_entities` 보완.

## 완료 조건

- [x] `0015_manuscript_fts.sql` 마이그레이션이 적용되고 `make test`가 통과한다.
- [x] `manuscript.Indexer` (Upsert/Delete/Rebuild)와 `Searcher.Query`가 동작하고 단위 테스트를 통과한다.
- [x] 노드 저장/삭제 경로에 인덱싱 훅이 연결되고, 인덱싱 실패가 본문 저장을 막지 않는다.
- [x] 컴패니언이 `search_manuscript` 도구로 본문을 검색하고, 결과 node_id로 `get_scene_text`를 이어 호출할 수 있다.
- [x] 기존 프로젝트에서 첫 검색 시 지연 재구축이 단일 커넥션에서 데드락 없이 완료된다.
- [x] 시스템 프롬프트에 `search_manuscript` 안내가 포함된다.

## 부록 A: 대안 비교

| 접근 | 의미 검색 | 오프라인 | 비용 | Windows/순수Go | 구현 표면 |
|------|-----------|----------|------|----------------|-----------|
| **A. FTS5 trigram (선택)** | 약 | ✅ | 무료 | ✅ | 도구 1개 + 마이그레이션 |
| B. Gemini 임베딩 + 코사인 | 강 | ❌ | 임베딩 API | ✅(브루트포스) | 청킹·임베딩·벡터저장·재임베딩 |
| C. 하이브리드 (RRF) | 강 | 부분 | 임베딩 API | ✅ | A+B 전부 |

1순위가 고유명사·사실 대조이므로 A가 정확도·비용·오프라인·빌드 제약에서 우월하다.
임베딩은 의미 검색이 1순위가 될 때 별도 작업으로 추가한다.
