# Linetta 컴패니언 — Phase 3: 메모리 (키워드 회상 + 제안 기반 쓰기) 설계

> 작성일: 2026-05-30
> 구현 레포: **Linetta** (engine + FE). tars `pkg/memory` 키워드 프리미티브 사용(임베딩 없음). 사용자 목표("남은 페이즈 모두 개발 완료") 하에 결정 자체 확정.

## 상위 맥락

컴패니언 = 집필 동료. Phase 1(백엔드)·Phase 2(FE 채팅+제안 적용) 완료. **Phase 3(본 spec, 마지막)**: 컴패니언이 작품 설정·작가 취향 같은 사실을 **장기 메모리에 축적**(쓰기)하고, 대화 시 관련 기억을 **회상해 컨텍스트에 주입**한다. 제공자 잠금(claude-code-cli/openai-codex)과 무관한 **키워드 메모리**(tars `pkg/memory`의 `experiences.jsonl`, 임베딩 없음).

## 확정된 Phase 3 결정

1. **저장:** tars `pkg/memory` 키워드 프리미티브(`AppendExperience`/`SearchExperiences`, 순수 stdlib, 임베더·네트워크·CGO 없음). 프로젝트별 루트 `<home>/companion/mem/{projectID}` → `memory/experiences.jsonl`. 기존 backup 트리(`<home>` 하위)에 포함.
2. **쓰기 = 제안 기반:** 플롯과 동일한 propose→검토→적용. proposal 스키마에 `remember {text, category?}` op 추가. 컴패니언이 "기억할 것"을 제안 → 카드에서 작가 승인 → FE `applyProposal`이 `companion.remember` RPC 호출 → engine이 `AppendExperience`. (자동 저장 안 함 — 작가 통제.)
3. **회상:** 매 턴 `gatherContext`가 사용자 입력 텍스트를 질의로 `SearchExperiences`(상위 N=5) → `## 기억` 섹션으로 컨텍스트 주입.
4. **시스템 프롬프트:** 컴패니언이 (a) 장기 기억을 활용하고 (b) 기억할 가치가 있는 사실은 `remember` op로 제안하도록 안내.

## 배경 / 현황 (조사 결과)

- tars `pkg/memory` (v0.33.0): `AppendExperience(root string, exp Experience) error`(EnsureWorkspace 자동 호출, Summary 필수, Category 빈값→"fact", Timestamp 영값→now), `SearchExperiences(root string, opts SearchOptions) ([]Experience, error)`(대소문자 무시 substring on Summary/Tags/SourceSession + Category 정확일치, 최근순 정렬, 파일 없으면 `[]`), `Experience{Timestamp time.Time; Category, Summary string; Tags []string; SourceSession string; Importance int; Auto bool}`, `SearchOptions{Query, Category string; Limit int}`. 키워드 경로는 stdlib만, 임베더/네트워크/CGO 없음. 파일: `<root>/memory/experiences.jsonl`.
- companion `Service`(Phase 1): `NewService(sessionsDir, ...)` — `sessionsDir = <home>/companion`. 메모리 베이스 = `filepath.Join(sessionsDir, "mem")`, 프로젝트 루트 = `filepath.Join(memBase, projectID)`.
- companion `proposal.go`(Phase 1): `Op` 구조체 + `ParseProposal` + `knownOps`. `gatherContext`(companion.go) → `PromptData`. `buildContext`/`buildSystem`(prompt.go). FE `applyProposal.ts`/`ProposalOp`(Phase 2).

## 아키텍처 / 컴포넌트

**Engine:**
- `engine/internal/companion/memory.go` (신설) — 키워드 메모리 헬퍼:
  - `memRoot(memBase, projectID string) string`
  - `(*Service).Remember(projectID, text, category string) error` → `memory.AppendExperience(memRoot, Experience{Summary:text, Category:category})`
  - `(*Service).Recall(projectID, query string, limit int) []string` → `memory.SearchExperiences(...)` → summary 리스트(에러/빈→nil)
- `engine/internal/companion/companion.go` — `Service`에 `memBase string` 필드 추가(`filepath.Join(sessionsDir,"mem")`); `gatherContext` 시그니처에 `query string` 추가 → `PromptData.Memories = Recall(projectID, query, 5)`.
- `engine/internal/companion/prompt.go` — `PromptData`에 `Memories []string`; `buildContext`에 `## 기억` 섹션(있을 때만); `buildSystem`에 메모리 활용 + `remember` op 안내 추가.
- `engine/internal/companion/proposal.go` — `Op`에 `Text`(json `text`), `Category`(json `category`) 추가; `knownOps`에 `remember`; 검증: `remember`는 `text` 필수.
- `engine/internal/companion/runner.go` — `gatherContext(ctx, projectID, nodeID, text)` 호출(사용자 텍스트를 회상 질의로).
- `engine/internal/rpc/handlers/companion.go` — `CompanionRemember(svc) rpc.Handler`(params `{project_id, text, category?}`; 빈 project_id/text→InvalidParams; `svc.Remember`→`{ok:true}`).
- `engine/cmd/linetta-engine/main.go` — `s.Handle("companion.remember", handlers.CompanionRemember(companionSvc))`.

**FE:**
- `apps/desktop/src/lib/types.ts` — `ProposalOpType += "remember"`; `ProposalOp`에 `text?`, `category?` 추가.
- `apps/desktop/src/lib/rpc.ts` — `companion.remember(projectId, text, category?)` → `{ok:true}`.
- `apps/desktop/src/lib/applyProposal.ts` — `case "remember"`: `companion.remember(projectId, op.text ?? "", op.category)`; 빈 text → 실패 기록.
- `apps/desktop/src/components/companion/ProposalCard.tsx` — `opLabel`에 `remember: 기억하기: {text}`.

## 데이터 흐름

```
[회상] send(text) → runner → gatherContext(..., query=text)
   → Recall(projectID, text, 5) → ["작가는 단문 선호", ...]
   → buildContext: ## 기억\n- 작가는 단문 선호\n...   (컨텍스트 주입)
[쓰기] AI 응답에 remember op 포함된 proposal
   → companion.proposal → 카드 "기억하기: X"
   → 작가 [적용] → applyProposal → companion.remember(projectId, X, cat?)
   → engine AppendExperience → experiences.jsonl
   → 이후 턴부터 Recall로 회상됨
```

## 제안 op 확장

```
remember { op:"remember", text:string, category?:string }
```
- engine `Op`: `Text string json:"text,omitempty"`, `Category string json:"category,omitempty"`(기존 create_thread의 summary/name과 별개).
- 검증: `remember`는 `strings.TrimSpace(Text) != ""` 필수, 아니면 op 검증 실패.
- FE `ProposalOp`: `text?: string; category?: string` 추가, `ProposalOpType`에 `"remember"`.

## 프롬프트 변경

- `buildSystem` 추가 문장: "이전 대화에서 알게 된 작품 설정·작가 취향은 아래 '기억'에 주어집니다. 기억할 가치가 있는 새 사실(작가 취향, 세계관 규칙 등)은 `remember` op로 제안하세요(작가가 승인하면 저장됩니다)." + op 목록에 `remember{text,category?}` 추가.
- `buildContext` `## 기억` 섹션: `Memories`가 있으면 `## 기억\n- {summary}\n...`.

## 에러 처리

- `Recall`: SearchExperiences 에러/빈 → `nil`(회상 섹션 생략). 파일 없음은 정상(빈).
- `Remember`: 빈 text → AppendExperience가 "summary required" 에러 → 핸들러 InvalidParams. (FE는 빈 text op를 실패로 기록.)
- `memRoot`: 디렉터리는 AppendExperience가 EnsureWorkspace로 생성. Recall은 파일 없으면 빈.
- 회상은 best-effort: 실패해도 대화 진행(기존 gatherContext의 best-effort 패턴과 동일).

## 테스트 전략

Engine(Go TDD):
- `memory.go`: `Remember`로 적재 후 `Recall(query)`가 substring 매칭 회상, 무관 query→빈, 빈 text Remember→에러. 임시 memBase(t.TempDir).
- `proposal.go`: `remember` op 파싱·검증(text 필수, 누락→에러), 기존 plot op 회귀 없음.
- `prompt.go`: `## 기억` 렌더(Memories 있을 때만), buildSystem에 remember 안내 포함.
- `companion` service: gatherContext가 query로 Recall 결과를 PromptData.Memories에 채움(fake 없이 실제 memBase 적재 후).
- handler: `companion.remember` 정상/빈 파라미터.

FE: `tsc --noEmit` 클린(types/rpc/applyProposal/card). 수동 스모크는 사용자.

검증: `go test ./...` 전 통과 + 엔진 빌드 + `tsc --noEmit`.

## 성공 기준

1. 컴패니언이 `remember` op를 제안하고, 적용 시 `companion.remember`로 `experiences.jsonl`에 사실이 저장된다.
2. 다음 턴부터 관련 기억이 `## 기억`으로 컨텍스트에 회상 주입된다.
3. 임베딩/네트워크 의존 없음(키워드만), 제공자 잠금과 무관.
4. `go test ./...` 통과, 엔진 빌드 ok, `tsc --noEmit` 클린.

## 범위 밖

- 시맨틱(임베딩) 회상 — 후속(설정에 Gemini 키 넣으면 토글).
- 자동 기억 추출(승인 없이 저장) — 의도적으로 제외(작가 통제).
- 메모리 편집/삭제 UI, MEMORY.md 노트 — 후속.
