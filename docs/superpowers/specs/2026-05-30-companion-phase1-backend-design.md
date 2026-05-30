# Linetta 컴패니언 — Phase 1: 백엔드 (세션·스트리밍·제안 프로토콜) 설계

> 작성일: 2026-05-30
> 구현 레포: **Linetta** (`/Users/changheonshin/workspace/myworks/linetta`), engine 측. tars `pkg/session`·`pkg/memory`·`pkg/llm`를 import.
> 선행: Phase 0 완료 — tars `v0.33.0`로 `pkg/session` 공개됨(릴리스 확인 완료).

## 상위 맥락 (컴패니언 전체)

Linetta 안에서 AI가 "집필 컴패니언"으로 동작: 지속 대화 세션 + 메모리 + 자연어로 플롯/스레드/비트/개요를 함께 작성·수정. 확정된 상위 결정: 집필 컴패니언 / tars 파일 스토어를 Linetta 백업 트리에 / **제안 → 검토 → 적용** / 키워드 메모리.

단계: **Phase 0**(tars pkg/session export, 완료) → **Phase 1(본 spec, 백엔드)** → Phase 2(FE 채팅 패널 + 제안 카드) → Phase 3(메모리 쓰기/회상).

## Phase 1 목표 (한 문장)

컴패니언 대화를 영속(tars `pkg/session`)하고, 프로젝트 컨텍스트를 주입해 LLM 응답을 스트리밍하며, 응답에 담긴 **구조화 플롯 변경 제안(JSON)을 파싱해 FE로 전달**하는 엔진 백엔드를 만든다. **변경 적용은 하지 않는다**(Phase 2).

## 확정된 Phase 1 결정 (브레인스토밍)

1. **제안 인코딩:** 제공자 무관 — 모델이 응답 텍스트에 펜스드 ` ```linetta-proposal ` JSON 블록을 출력. (claude-code-cli는 네이티브 tool-calling 불가, openai-codex만 가능 → tool-calling 대신 JSON-in-text. 둘 다 평문 스트리밍은 정상.)
2. **op 범위(플롯 코어):** `create_thread`, `update_thread`, `add_beat`, `update_beat`, `delete_beat`, `set_outline`. (관계·엔티티는 후속.)
3. **경계:** Phase 1은 파싱·반환까지. 적용·ref 해소·기존 RPC 호출은 Phase 2. 메모리는 Phase 3. **FE 없음**(엔진 전용).

## 배경 / 현황 (조사 결과)

- Linetta engine은 `engine/internal/ai/runner.go`에서 `client.Chat(ctx, msgs, llm.ChatOptions{OnDelta: …})`로 스트리밍(현재 `Tools` 미사용). 클라이언트 팩토리 `ai.DefaultClientFactory(provider, workDir)` → `llm.NewProvider`. 제공자 설정은 `engine/internal/settings`(`claude-code-cli`/`openai-codex`).
- 제공자 tool-calling: **openai-codex 완전 지원, claude-code-cli 미지원**(Tools 무시, `Message.ToolCalls` 항상 nil). → JSON-in-text 채택 근거.
- tars `pkg/session`(v0.33.0): `NewStore(dir)`, `(*Store).EnsureWorker(projectID)`, `TranscriptPath(id)`, `AppendMessage(path,msg)`, `ReadMessages(path)`, `LoadHistory(path,maxTokens)`, `Message{ID,Role,Content,Timestamp,…}`.
- Plan 24 `engine/internal/plot`: `plot.NewBuilder(nodes,beats,threads)` + `(*Builder).Build(ctx,nodeID) (Spine,…)` — 컨텍스트 플롯 스파인 재사용.
- 기존 repos: `project`(outline 포함), `thread`(`ListByProject(projectID, includeClosed)`), `entity`, `relationship`(`ListByProject`), `node`.
- 스트리밍 알림은 `rpc.Notifier`(ai.Runner가 사용)로 발행. 델타 dedup 헬퍼는 ai 패키지(Plan 18).

## 아키텍처 / 컴포넌트

새 패키지 `engine/internal/companion/`:

- **`companion.go`** — `Service` 조립: 세션 스토어(`session.NewStore(companionDir)`), 컨텍스트 빌더(plot.Builder + project/thread/entity/relationship repos), LLM 클라이언트 팩토리(주입 가능; 기본 `ai.DefaultClientFactory`), 세팅(provider) 접근.
- **`prompt.go`** — 시스템/유저 프롬프트 조립: 페르소나 + JSON 제안 포맷 지시 + 프로젝트 컨텍스트.
- **`proposal.go`** — 제안 타입 + 파서/검증(스트림 full_text → 타입드 `Proposal`).
- **`runner.go`** — `Runner`: run 생명주기, 취소(context), 델타 dedup, Notifier로 알림 발행.

`companionDir` = Linetta 홈 백업 트리 하위 `companion/`(`paths` 패키지로 홈 경로 획득; 기존 backup/gitsync 트리에 포함). 세션은 프로젝트별 `EnsureWorker(project_id)`, 트랜스크립트 `companion/sessions/{id}.jsonl`.

### 데이터 흐름 (`companion.send`)
```
{project_id, node_id?, text}
 → store.EnsureWorker(project_id) → sess ; path = store.TranscriptPath(sess.ID)
 → session.AppendMessage(path, Message{Role:"user", Content:text, Timestamp:now})
 → ctx 메시지 조립:
     system = buildSystem()                         // 페르소나 + 제안 포맷 규칙
     context = buildContext(개요, 플롯 스파인, thread 목록(id+name), 엔티티·관계)
     history = session.LoadHistory(path, budget)     // 토큰 예산 내 최근 턴
     messages = [system, context(user), ...history, {user, text}]
 → client.Chat(ctx, messages, llm.ChatOptions{ OnDelta })
     OnDelta → dedup → notify "companion.delta" {run_id, text}
 → done:
     session.AppendMessage(path, Message{Role:"assistant", Content:full_text, Timestamp:now})
     prop, ok := proposal.Parse(full_text)
     if blockPresent { notify "companion.proposal" {run_id, valid:ok, summary, ops | error} }
     notify "companion.done" {run_id, full_text}
 → 에러 → notify "companion.error" {run_id, message}
 → cancel(run_id) → context 취소 → notify "companion.cancelled" {run_id} (부분 assistant 미적재)
```

## 제안 JSON 스키마

펜스드 블록(정확히 하나):
````
```linetta-proposal
{ "summary": "...", "ops": [ ... ] }
```
````

op 타입(플롯 코어):
```
create_thread { op:"create_thread", ref?:string, name:string, color?:string, summary?:string }
update_thread { op:"update_thread", thread_id:string, name?:string, color?:string, summary?:string }
add_beat      { op:"add_beat", thread_id?:string, thread_ref?:string, node_id:string, label:string, description?:string, intensity?:int }
update_beat   { op:"update_beat", beat_id:string, label?:string, description?:string, intensity?:int }
delete_beat   { op:"delete_beat", beat_id:string }
set_outline   { op:"set_outline", outline:string }
```
- `create_thread.ref`: 같은 제안 내 forward-참조 핸들. `add_beat`는 기존 thread면 `thread_id`, 같은 제안서 만든 thread면 `thread_ref`(정확히 하나).
- Go 타입: `Proposal{Summary string; Ops []Op}` + op별 구조체(또는 `Op{Type string; …}` 디스패치). 파서는 알려진 필드만 사용.

### 파서(`proposal.Parse(text) (Proposal, bool, error)` 형태)
- full_text에서 ` ```linetta-proposal ` … ` ``` ` 블록을 추출. 없으면 `(zero, false, nil)`(블록 없음).
- 블록 있으면 `json.Unmarshal` → 검증:
  - 알려진 op 타입만, op별 필수 필드 존재.
  - `add_beat`: `thread_id`/`thread_ref` 정확히 하나; `thread_ref`는 같은 제안의 `create_thread.ref`와 일치.
  - 블록 ≥2개: 첫 블록만 채택하고 `valid:false` 사유.
  - 실패 시 `(parsed-partial, true(blockPresent), err)`. 호출자는 blockPresent면 `companion.proposal {valid: err==nil, …}` 발행.

## 프롬프트 & 컨텍스트

**시스템(`buildSystem`)** — 한국어 집필 동료 페르소나 + 규칙:
- 자연스럽게 대화·브레인스토밍·질문응답.
- 구체적 플롯 변경 제안 시에만 ` ```linetta-proposal ` 블록을 **정확히 하나** 출력(스키마 준수). 컨텍스트의 thread/씬 **id로 참조**, 새 thread는 `ref`.
- 직접 적용하지 않는다(작가 검토). 단순 대화면 블록 생략.

**컨텍스트(`buildContext`, user 메시지)** — 섹션:
- `## 작품 개요`(project.Outline)
- `## 플롯`(plot.Builder 스파인 — node_id 있으면 현재 씬 중심, 없으면 첫 leaf 기준)
- `## 스토리라인`(thread.ListByProject open 목록: `- [id] name — summary`)
- `## 등장 인물·장소·관계`(엔티티 name+id, 관계 from↔to)
- 히스토리는 `LoadHistory(path, historyTokenBudget)`로 prior 메시지 배열, 마지막에 새 user 텍스트. `historyTokenBudget`은 상수(기본 6000; 플랜에서 확정·조정 가능).

상한: 컨텍스트 조각 전부 bounded(플롯 스파인/개요/목록), 히스토리는 `historyTokenBudget` 토큰 예산. 메모리 회상 주입은 Phase 3.

## RPC 표면

- `companion.send` `{project_id:string, node_id?:string, text:string}` → `{run_id:string}` (스트리밍은 알림).
- `companion.history` `{project_id:string}` → `{messages:[{role,content,timestamp}]}` (`ReadMessages` 매핑; 없으면 빈 배열).
- `companion.cancel` `{run_id:string}` → `{ok:true}`.

알림: `companion.delta`{run_id,text} / `companion.done`{run_id,full_text} / `companion.error`{run_id,message} / `companion.cancelled`{run_id} / `companion.proposal`{run_id,valid,summary?,ops?,error?}.

main.go: `companion.NewService(...)` 조립(companionDir, repos, plot.Builder, clientFactory, notifier, settings) + 3개 핸들러 등록.

## 에러 처리

- 빈 `project_id`/`text` → InvalidParams. `node_id` 잘못돼도 플롯 스파인 graceful(빈/부분) → 대화 진행.
- `EnsureWorker` 실패 → InternalError(RPC). run 시작 후 LLM 실패 → `companion.error` 알림.
- 제안 블록 0개 → 제안 이벤트 없음. 검증 실패/≥2개 → `companion.proposal {valid:false, error}`, run 정상 종료.
- 취소 → 부분 assistant 미적재, `companion.cancelled`.
- 트랜스크립트 append 실패 → best-effort 로깅, done은 발행(턴 유실 방지).
- 빠른 연속 send/StrictMode → run_id 키로 격리(ai.Runner 패턴).

## 테스트 전략 (엔진 Go TDD)

- `proposal.go`: 블록 추출/유효 ops 파싱/블록 없음→none/잘못된 JSON→err/미지 op→err/add_beat thread_id XOR thread_ref/thread_ref↔create_thread.ref 해소/블록 ≥2개→첫 개+valid:false.
- `prompt.go`: buildSystem에 제안-포맷 지시 포함; buildContext가 개요/플롯/스토리라인(id 포함)/엔티티 섹션 렌더; 히스토리 주입 순서.
- `companion` 서비스/runner: **fake LLM 클라이언트**(제안 블록 든 full_text) + fake Notifier + 임시 companionDir → `companion.delta`/`companion.done`/`companion.proposal` 발행 검증, 트랜스크립트에 user+assistant 적재, 취소 시 cancelled+부분 미적재.
- 핸들러: `companion.send` 빈 파라미터→InvalidParams + run_id 반환; `companion.history` 메시지 매핑; `companion.cancel`.

검증: `go test ./...` 전 통과 + 엔진 빌드(tars v0.33.0). FE 없음(Phase 2). 실제 제공자 수동 스모크는 Phase 2(FE 채팅 패널 후).

## 성공 기준

1. `engine/go.mod`가 tars `v0.33.0` 핀, 빌드 통과.
2. `companion.send`가 세션을 영속하고 응답을 `companion.delta`/`done`으로 스트리밍.
3. 응답의 `linetta-proposal` 블록이 타입드 Proposal로 파싱돼 `companion.proposal`로 발행(유효/무효 구분).
4. `companion.history`가 트랜스크립트를 반환.
5. 컨텍스트(개요·플롯·스토리라인 id·엔티티)가 프롬프트에 주입.
6. `go test ./...` 전 통과, 엔진 빌드 ok.

## 범위 밖 (Phase 1 아님)

- FE: 채팅 패널, rpc 클라이언트(companion 네임스페이스), 제안 검토 카드, 적용 버튼 — Phase 2.
- 제안 적용(ref 해소 + 기존 RPC 호출) — Phase 2.
- 메모리 쓰기/회상 주입 — Phase 3.
- 관계·엔티티 op — 후속.
