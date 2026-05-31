# Linetta 컴패니언 — Phase 6: 온디맨드 읽기 루프 설계

> 작성일: 2026-05-31
> 구현 레포: Linetta (engine + FE + Rust 브리지 1줄). 컴패니언 "완성"의 마지막 조각 — AI가 직접 조회 도구를 호출해 자료를 모은 뒤 답/제안하게 한다.

## 상위 맥락

지금 컴패니언은 1회 LLM 호출 → 스트리밍 → 제안 파싱. 사전 주입(개요/플롯/씬 id/스토리라인/엔티티/관계/기억)으로 컨텍스트를 주지만 **AI가 능동적으로 더 깊이 조회**(특정 씬 본문 읽기, 이름으로 엔티티 검색, 특정 스토리라인 비트 보기 등)할 수는 없다. **Phase 6**: 제공자 무관 **JSON 쿼리 루프**를 추가해 "읽기 자율"을 완성한다 — AI가 `linetta-query` 블록으로 조회 → 엔진이 실행해 결과를 대화에 주입 → AI 계속, 최종 답/제안 도출.

## 확정된 결정

1. **제공자 무관 루프:** `claude-code-cli`가 네이티브 tool-loop 불가하므로, 텍스트 내 `linetta-query` 펜스드 블록 + 엔진 재프롬프트 루프로 구현(읽기 자율 결정 시 이미 합의한 방식). 두 제공자 동작.
2. **읽기 전용:** 루프 도구는 모두 비파괴 조회. 쓰기는 여전히 제안→검토(기존). 
3. **상한:** 최대 라운드 N=3(상수). 초과 시 마지막 답으로 마감(무한 루프/토큰 폭주 방지).
4. **스트리밍 보존:** 기존 reset + 프로즈-strip 메커니즘을 재사용 — 쿼리 라운드의 텍스트는 표시에서 숨기고(`linetta-query` 블록 strip), 라운드가 쿼리로 판명되면 `companion.reset("")`로 부분 프로즈를 비우고 `companion.thinking` 상태를 띄운 뒤 다음 라운드. 최종(쿼리 없는) 라운드만 일반 프로즈+제안으로 마감.

## 읽기 도구 (MVP, 5종)

```
search_entities { query }            → 이름/요약 substring 매칭 엔티티 [id, kind, name, role]
get_scene_text  { node_id }          → 해당 씬 본문 평문(상한 trim)
list_scenes                          → 프로젝트 전체 leaf 씬 [node_id, breadcrumb]
list_beats      { node_id | thread_id } → 해당 씬/스토리라인 비트 [id, thread, label, desc]
recall_memory   { query }            → 키워드 메모리 검색 결과 summaries
```
엔진 매핑: `mention/entities.Search`, `node.Get`(+평문화), `node.ListByProject`(leaf+breadcrumb), `beat.ListByNode/ListByThread`, `memory.SearchExperiences`. 모두 기존 repo/함수.

## 쿼리 블록 포맷

````
```linetta-query
{ "queries": [ { "tool": "get_scene_text", "args": { "node_id": "..." } }, { "tool": "search_entities", "args": { "query": "하나" } } ] }
```
````
- 한 라운드에 여러 조회 허용(배치). 미지 tool/필수 arg 누락 → 그 조회는 에러 메시지로 결과에 포함(전체 실패 아님).

## 아키텍처 / 컴포넌트

**Engine `companion/query.go` (신설):**
- `Query{Tool string; Args map[string]string}` + `QueryRequest{Queries []Query}` + `ParseQuery(full) (QueryRequest, bool, error)`(linetta-query 블록 추출; proposal 파서와 동일 펜스 추출 재사용).
- `(*Service).runQueries(ctx, projectID string, qs []Query) string` — 각 조회 실행, 결과를 사람이 읽는 텍스트 블록으로 조립("## 조회 결과\n### get_scene_text(...)\n..."). 알 수 없는 tool/누락 arg는 "(오류: ...)" 라인.
- 평문화 헬퍼: 씬 본문 Tiptap JSON→평문(ai 패키지 docToPlainText 재사용 불가 시 companion에 작은 복제 또는 node 패키지에 공개 헬퍼). **결정:** `node` 패키지에 `PlainText(doc *string) string` 공개 헬퍼를 추가(ai/context.go의 docToPlainText 로직 추출·공유) — ai도 그걸 쓰도록 정리(작은 리팩터). 또는 companion 자체 최소 평문화. MVP는 companion 자체 최소 평문화(텍스트 노드만 추출)로 시작, 추출 리팩터는 후속.

**Engine `companion/runner.go` (루프화):**
- `run`을 라운드 루프로: round 0..N. 각 라운드 `client.Chat`(스트리밍, dedup). 라운드 종료 시 full text에서:
  - `ParseQuery` present → `companion.reset("")` 발행(부분 프로즈 클리어) + `companion.thinking {round, text:"조회: …"}` 발행 → `runQueries` 결과를 messages에 user 메시지로 append → 다음 라운드. (트랜스크립트에는 조회 라운드는 적재하지 않거나 간략 적재 — MVP: 적재 안 함, 최종만 적재.)
  - 쿼리 없음(또는 round == N) → 최종: assistant 메시지 적재 + `ParseProposal` → `companion.proposal` + `companion.done`.
- delta는 매 라운드 스트리밍하되 FE가 `linetta-query`/`linetta-proposal` 블록을 표시에서 숨김(streamProse 확장). 쿼리 라운드 종료 시 reset으로 클리어.

**Engine 알림:** 신규 `companion.thinking {run_id, text}`.

**Rust 브리지:** `engine.rs` allowlist에 `companion.thinking` → `companion-thinking` 추가.

**FE:**
- `lib/types.ts`: `CompanionThinking{run_id, text}`.
- `hooks/useCompanion.ts`: `companion-thinking` 구독 → 일시 상태(예: `thinking` 문자열) 표시; reset은 이미 streaming 클리어. 최종 done까지 thinking 누적/표시.
- `components/companion/CompanionPanel.tsx`: streamProse가 `linetta-query`도 cut(현재 linetta-proposal만). 스트리밍 중 `thinking` 상태를 "🔎 {text}"로 표시.
- (rpc 추가 없음 — 조회는 엔진 내부 루프, FE는 이벤트만.)

## 데이터 흐름

```
send(text) → round0 Chat
  델타(프로즈만 표시) … 종료 → full0에 linetta-query?
    yes → reset("") + thinking("씬 본문 조회 중") → runQueries → "## 조회 결과 …" user 메시지 append → round1 Chat
    no  → done(full0) + proposal
  (round == 3 이면 강제 최종)
```

## 에러 처리

- 라운드 상한(3) 도달 시 마지막 라운드를 최종으로 처리(쿼리 블록 있어도 더 실행 안 함; "(추가 조회 생략)" 가능).
- ParseQuery 실패(블록 있으나 JSON 오류) → 그 라운드를 최종으로 간주(무한 루프 방지) + 프로즈 그대로 마감.
- 개별 조회 실패(미지 tool/arg/없는 id) → 결과 텍스트에 오류 라인, 루프는 계속.
- 취소(context) → 현재 라운드 Chat 취소 → `companion.cancelled`(기존).
- 스트리밍: 쿼리 라운드의 부분 프로즈는 reset으로 정리; 최종만 사용자에게 답으로 남음.

## 테스트 전략

Engine(Go TDD):
- `query.go`: ParseQuery(블록 추출·없음·JSON 오류), runQueries(각 tool 실행 결과 포맷, 미지 tool→오류 라인, fake repos/임시 store).
- runner 루프: fake client가 1라운드는 linetta-query, 2라운드는 일반 답을 반환하도록 스크립트 → companion.thinking + 최종 done이 발행되고, 트랜스크립트에 최종 assistant만 적재, 라운드 상한 동작. (기존 fakeClient를 라운드별 응답 큐로 확장.)
- 기존 단일-턴 동작(쿼리 없는 응답)은 round0에서 바로 done — 회귀 없음.

FE: `tsc --noEmit` 클린. 수동 스모크: "3장에 뭐가 적혀 있는지 보고 이어서 제안해줘" → (조회 중 표시) → 답+제안.

검증: `go test ./...` + 엔진 빌드 + tsc + cargo check(브리지).

## 성공 기준

1. AI가 `linetta-query`로 조회를 능동 호출하고, 엔진이 실행해 결과를 다음 라운드에 주입하며, 최대 3라운드 내 최종 답/제안을 낸다.
2. 5개 읽기 도구가 동작(엔티티 검색·씬 본문·씬 목록·비트·기억).
3. 스트리밍 중 쿼리/제안 블록은 숨고, 조회 라운드는 `companion.thinking` 상태로 보이며 최종만 답으로 남는다.
4. 쿼리 없는 일반 대화는 기존처럼 1라운드로 동작(회귀 없음).
5. `go test ./...` 통과, 빌드 ok, tsc 클린.

## 범위 밖
- 쓰기 도구의 루프 내 자동 실행(쓰기는 계속 제안→검토).
- 조회 라운드 트랜스크립트 영속(최종만 적재).
- 시맨틱 검색, 외부 도구/MCP.
