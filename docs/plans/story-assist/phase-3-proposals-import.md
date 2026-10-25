# Phase 3 — 제안함과 가져온 원고의 설정 추출

_작성일: 2026-10-24_
_상위 로드맵: [`roadmap.md`](./roadmap.md)_
_근거: [`inkos-analysis.md`](./inkos-analysis.md) §4, §1(id 소유·인용 검증)_
_선행 조건: Phase 2 완료. Phase 2의 Review 도구 그룹과 인용 검증 코드를 재사용한다_
_예상 소요: 3~4일_

## Goal

에이전트는 인물, 장소, 관계, 팩트, 스토리라인, 비트 후보를 **제안함**에 올린다. 이 단계에서는 아무것도 적용되지 않는다.
작가는 제안함에서 항목별로 수락·수정·거절한다. 수락한 항목만 기존 `storyops` 적용 경로로 Story World에 들어간다.

새 인물 후보가 기존 인물의 이름이나 별칭과 겹치면 "같은 대상일 수 있음"으로 표시한다.
작가는 그 후보를 "기존 인물에 별칭 추가"로 바꿀 수 있다.

Markdown 원고를 가져온 직후에는 "Story World 채우기"를 제안한다.
분량과 예상 토큰을 먼저 보여주고, 작가가 시작하면 에이전트가 회차 순서대로 읽으며 제안함을 채운다.
도구 호출 한도에 걸리면 "계속"이라고 보내 이어서 진행한다. 진행률은 엔진이 계산한다.

## 왜 새 경로가 필요한가

- `linetta_apply_story_ops`의 `create_entity`, `create_relationship`, `create_fact_card`는 **즉시 적용**된다.
- 도구 설명에도 적혀 있듯 이 op들은 묶음 undo도 안 된다. 원고 40화를 추출하면 수십 개 항목이 검토 없이 들어간다.
- InkOS도 같은 문제를 "되돌릴 수 있음"(리비전 이력)으로만 막았다. 검토 단계는 없었다.
- 리네타는 문체 프로필에서 쓴 초안→승인 길을 설정 연산에도 깐다.

`linetta_apply_story_ops`는 그대로 둔다. 작가가 "이 인물 하나 만들어 줘"처럼 직접 부탁한 소규모 변경까지 매번 제안함을 거치게 하면 번거롭다.
에이전트 지침에서 쓰임을 나눈다:
- 직접 요청받은 소규모 변경은 apply를 쓴다.
- 원고에서 뽑아낸 것이나 여러 항목은 propose를 쓴다.

## Non-goals

- 수락 결과의 묶음 undo → Next. 현재 storyops에서 entity / fact op는 묶음 undo가 안 된다. 그 제약을 이 페이즈에서 바꾸지 않는다.
- `.txt` 평문 원고 분할과 프롤로그 보존 → Later. 가져오기 자체는 기존 Markdown 경로 그대로다.
- 시점별 사실(유효 회차 범위) → Later
- 설정을 "항상 제안함으로" 강제하는 옵션 → Next

## Touch points

- `engine/internal/store/migrations/0026_story_proposals.sql` 신규
- `engine/internal/proposals/` 신규 패키지
- `engine/internal/evidence/` 신규 (Phase 2의 인용 검증을 옮김)
- `engine/internal/storyops/ops.go`, `apply.go` (aliases 필드, 미리 묶인 ref로 적용)
- `engine/internal/mcphost/tools_proposals.go` 신규, `budget.go`
- `engine/internal/agent/prompt.go`
- `engine/internal/rpc/handlers/proposals.go` 신규, `engine/internal/engineapp/engineapp.go`
- `engine/internal/restore/restore.go` (제외 주석)
- `apps/desktop/src-tauri/src/lib.rs`, `src/lib/rpc.ts`, `lib/types.ts`, `lib/agentTools.ts`, `lib/i18n.tsx`
- `apps/desktop/src/components/ProposalInbox.tsx` 신규, `CanonPanel.tsx`, `routes/Library.tsx`, `routes/Workspace.tsx`, `agent/AgentPanel.tsx`

## 데이터 모델

```sql
-- 0026_story_proposals.sql
-- Story-structure changes an agent suggested and the writer has not applied
-- (story-assist Phase 3). An item becomes part of the work only through the
-- app's proposals.accept RPC, which runs it through storyops like any other
-- change; no MCP tool accepts one.
CREATE TABLE story_proposals (
  id         TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  purpose    TEXT NOT NULL,          -- general | import_extraction | forecast (Phase 4)
  source     TEXT NOT NULL,          -- agent | external
  summary    TEXT NOT NULL DEFAULT '',
  ref_bindings_json TEXT NOT NULL DEFAULT '{}', -- ref → created id, filled as items are accepted
  created_at INTEGER NOT NULL
);
CREATE TABLE story_proposal_items (
  id             TEXT PRIMARY KEY,
  proposal_id    TEXT NOT NULL REFERENCES story_proposals(id) ON DELETE CASCADE,
  project_id     TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  ordinal        INTEGER NOT NULL,
  op_json        TEXT NOT NULL,              -- one storyops.Op
  depends_on_json TEXT NOT NULL DEFAULT '[]', -- item ids whose ref this op uses
  evidence_json  TEXT NOT NULL DEFAULT '[]', -- [{node_id, paragraph, quote}] verified at propose time
  from_node_ids_json TEXT NOT NULL DEFAULT '[]', -- scenes it was drawn from (import progress)
  duplicates_json TEXT NOT NULL DEFAULT '[]', -- [{entity_id, name, kind, matched}]
  status         TEXT NOT NULL DEFAULT 'pending', -- pending|accepted|rejected|failed
  result_json    TEXT NOT NULL DEFAULT '{}',      -- created id or failure message
  status_at      INTEGER NOT NULL
);
CREATE INDEX story_proposal_items_project ON story_proposal_items(project_id, status);
```

## 작업 체크리스트

### A. 공용 기반

- [x] **3.A.1 인용 검증을 `evidence` 패키지로 옮김**
  - 파일: `engine/internal/evidence/evidence.go`, `evidence_test.go`. 이에 맞춰 `engine/internal/continuity/validate.go`도 고친다.
  - 내용:
    - Phase 2의 문단 범위 검사와 정규화 부분 문자열 검사를 옮긴다.
      `func VerifyQuote(paragraphs []string, from, to int, quote string) error`
    - 오류 문구는 그대로 유지한다. Phase 2 테스트가 계속 통과해야 한다.
  - 이유: 패키지 두 개가 같은 검사를 따로 구현하면 인용 규칙이 갈라진다. 검사하는 길은 하나만 둔다.

- [x] **3.A.2 storyops: 별칭 필드와 미리 묶인 ref**
  - 파일: `engine/internal/storyops/ops.go`, `apply.go`, `apply_test.go`, `ops_schema_test.go`
  - 내용:
    - `Op.Aliases []string` (`json:"aliases,omitempty"`)
      - `create_entity`: 별칭을 지정한다.
      - `update_entity`: 별칭을 **추가만** 한다. 기존 별칭은 지우지 않는다. 맥락 편집 Phase 3 규칙("aliases는 자동 삭제하지 않는다")과 같다.
    - `ApplyOps`의 내부 ref 맵(thread / entity / node)을 밖에서 미리 채울 수 있게 한다.
      ```go
      type Refs struct{ Threads, Entities, Nodes map[string]string }
      func (s *Service) ApplyOpsWithRefs(ctx context.Context, projectID, nodeID string, p Proposal, seed Refs, now func() int64) (ApplyOpsResult, Refs)
      ```
      - 기존 `ApplyOps`는 빈 `Refs`로 이것을 부르는 얇은 래퍼가 된다. 동작은 바뀌지 않는다.
    - op 스키마 설명(`jsonschema` 태그)에 aliases를 반영한다. `ops_schema_test.go`가 스키마를 고정한다.
  - 테스트:
    - aliases 생성과 추가
    - 기존 별칭 보존
    - seed ref로 관계 생성
    - 기존 apply 테스트 전부 통과

### B. 제안함 엔진

- [x] **3.B.1 `proposals` 패키지: 접수 검증**
  - 파일: `engine/internal/proposals/proposals.go`, `submit.go`, `submit_test.go`
  - 허용 op
    - `create_entity`, `update_entity`, `create_relationship`, `create_fact_card`, `create_thread`, `update_thread`, `add_beat`, `update_beat`
    - 나머지는 거부하고 "use linetta_apply_story_ops / linetta_write_scene"으로 안내한다.
    - `set_scene_text`, 삭제, `remember`, 개요 구조 op는 받지 않는다. Phase 4가 `create_scene`과 `create_outline_node`를 purpose=forecast에 한해 추가한다.
  - 접수 함수:
    ```go
    func (s *Service) Submit(ctx context.Context, now int64, in SubmitInput, source string) (SubmitResult, []Problem, error)
    ```
  - 검사:
    1. `storyops.ValidateProposal`로 op 형태를 확인한다.
    2. 참조 id(entity_id, thread_id, beat_id, node_id)가 이 작품에 있는지 확인한다.
    3. 같은 묶음 안의 `*_ref` 사용을 분석해 `depends_on`을 계산한다. 정의되지 않은 ref는 문제로 보고한다.
    4. `evidence`가 있으면 `evidence.VerifyQuote`로 확인한다.
    5. `from_node_ids`는 이 작품의 leaf여야 한다.
  - 문제가 하나라도 있으면 아무것도 저장하지 않는다. 문제를 모두 돌려준다 (Phase 2와 같은 재시도 방식).
  - 한도: 묶음 1개에 항목 60개, 항목 1개에 근거 3개

- [x] **3.B.2 중복 후보 판정**
  - 파일: `engine/internal/proposals/dedupe.go`, `dedupe_test.go`
  - 정규화: NFKC → 대소문자 접기 → 공백·가운뎃점·하이픈 제거
  - 비교 대상:
    - (a) 같은 작품의 기존 entity name과 aliases
    - (b) 같은 작품에서 아직 `pending`인 다른 제안 항목의 create_entity name과 aliases
  - 일치 규칙: 정규화한 문자열이 같으면 `matched: "exact"`. 종류(kind)가 달라도 표시는 하되 "종류가 다름"을 적는다.
  - 판정은 **표시만** 한다. 엔진이 자동으로 합치지 않는다. InkOS arbiter가 "의미상 동일성을 추론하지 않는다"고 한 것과 같은 선택이다. 판단은 작가가 한다.
  - 테스트:
    - "김민호"와 "김 민호"
    - "Ha-rin"과 "harin"
    - 별칭 일치
    - pending끼리의 일치
    - 종류가 다른 경우

- [x] **3.B.3 수락, 거절, 수정**
  - 파일: `engine/internal/proposals/accept.go`, `accept_test.go`
  - 함수:
    ```go
    // Accept applies the chosen items, plus nothing else. An item whose
    // dependency is neither already accepted nor in this call is refused with
    // the dependency named — never applied with a dangling ref.
    func (s *Service) Accept(ctx context.Context, now int64, projectID string, itemIDs []string, edits map[string]storyops.Op) (AcceptResult, error)
    func (s *Service) Reject(ctx context.Context, now int64, projectID string, itemIDs []string) error
    ```
  - 규칙:
    - `edits`는 작가가 고친 op다. 같은 허용 목록과 검증을 다시 거친다.
      - "기존 인물에 별칭으로" 전환도 여기서 처리한다. op를 `update_entity{entity_id, aliases:[…]}`로 바꿔 보내면 된다.
    - 적용 순서는 제안의 `ordinal` 순서다. `ApplyOpsWithRefs`에 `ref_bindings_json`을 seed로 넘긴다. 결과로 얻은 새 ref 묶음은 다시 저장한다.
    - 항목별 결과를 `accepted`(생성 id 기록) 또는 `failed`(오류 문구 기록)로 남긴다.
      - 하나가 실패해도 나머지는 진행한다. storyops의 비구조 op 동작과 같다.
    - 거절한 항목에 의존하는 pending 항목은 거절하지 않는다. 그대로 두고, 화면에 "의존 항목이 거절됨"이라고 표시한다.
    - 적용 뒤에는 앱이 이미 듣고 있는 갱신 이벤트를 발행한다. entity나 fact가 바뀌면 Story World와 FactBook이 다시 불러와야 한다.
      - `mcp.changed`는 출처가 MCP라는 뜻이므로 쓰지 않는다. 기존 entity 변경 RPC가 쓰는 이벤트를 찾아 같은 것을 쓴다.
  - 테스트:
    - 의존 없는 수락
    - 의존을 함께 수락
    - 의존이 빠져 거부됨
    - 이전에 수락된 ref 사용
    - 편집 후 수락
    - 별칭 전환
    - 부분 실패

- [x] **3.B.4 원고 추출 진행률과 예상 분량**
  - 파일: `engine/internal/proposals/progress.go`, `progress_test.go`
  - 함수:
    ```go
    func (s *Service) ExtractionProgress(ctx context.Context, projectID string) (Progress, error)
    // Progress{TotalEpisodes, CoveredEpisodes int; NextEpisode *episode.Episode; PendingItems int}
    func (s *Service) ExtractionEstimate(ctx context.Context, projectID string) (Estimate, error)
    // Estimate{Episodes, Chars, InputTokens int}
    ```
  - 계산:
    - "다룬 회차"란, purpose=import_extraction인 항목의 `from_node_ids`가 그 회차의 LeafIDs와 하나라도 겹치는 회차다.
      - 회차 목록은 `episode.List`에서 얻는다.
    - `NextEpisode`는 다루지 않은 회차 가운데 첫 회차다.
    - 예상 입력 토큰
      - 원고 전체를 `storycontext.EstimateTokens`로 센 값 × 1.5. 브리프와 도구 왕복 몫이다.
      - 이 값은 어림이고, 화면에 "어림값"이라고 쓴다.
  - **InkOS에 없는 것**: 비용 미리보기, 그리고 엔진이 계산하는 "어디서부터 이어 할지"

### C. MCP와 에이전트

- [x] **3.C.1 `linetta_propose_story_ops` (쓰기 도구, Review 그룹)**
  - 파일: `engine/internal/mcphost/tools_proposals.go` 신규, `tools_proposals_test.go`, `budget.go` (`ReviewToolNames`에 추가)
  - 입력:
    - `project_id`, `summary`
    - `purpose`: `general` | `import_extraction`
    - `items[]`: `{op, evidence?[{node_id, paragraph, quote}], from_node_ids?[]}`
  - 출력:
    - `{proposal_id, items, possible_duplicates, status: "waiting_for_writer"}`
    - purpose=import_extraction이면 `progress: {covered, total, next_episode_label}`도 넣는다.
  - 설명문:
    > Suggest world elements (characters, places, items, concepts, relationships, fact cards, storylines, beats) for the writer to accept one by one. Nothing is applied. Use it for anything drawn from the manuscript or for more than a few items; reuse existing entity ids from linetta_list_characters instead of proposing duplicates. Cite evidence from linetta_read_scene numbered=true where you can.
  - 성공하면 `notifyChanged(projectID, "linetta_propose_story_ops", nil, "")`

- [x] **3.C.2 에이전트 지침**
  - 파일: `engine/internal/agent/prompt.go`, `prompt_test.go`. Review 그룹이 켜져 있을 때만 넣는다.
    > Changes you are asked for directly and that are small go through linetta_apply_story_ops. Anything drawn from the manuscript, or more than a few world elements at once, goes through linetta_propose_story_ops: the writer accepts it item by item.
    > When asked to fill Story World from the manuscript, work episode by episode in outline order, starting where the tool's progress says. Check what already exists first, propose with purpose=import_extraction and from_node_ids, and stop before your tool budget runs out to tell the writer how far you got and to say "continue".

### D. RPC와 화면

- [x] **3.D.1 RPC**
  - 파일: `engine/internal/rpc/handlers/proposals.go`, `proposals_test.go`
  - 메서드:
    - `proposals.list {project_id, statuses?}` → 묶음과 항목
      - 각 항목에 사람이 읽을 요약(`"새 인물: 하린 — 검술 사범"`)을 붙인다.
      - 근거의 씬 breadcrumb, 중복 후보, 의존 상태(`blocked_by_rejected`)도 붙인다.
    - `proposals.accept {project_id, item_ids, edits?}`
    - `proposals.reject {project_id, item_ids}`
    - `proposals.pending_count {project_id}`
    - `proposals.extraction_estimate {project_id}`
    - `proposals.extraction_progress {project_id}`
  - 등록: `engineapp.go`, `lib.rs` `RENDERER_ENGINE_METHODS`

- [x] **3.D.2 `ProposalInbox`**
  - 파일: `apps/desktop/src/components/ProposalInbox.tsx`, `.css`, `.test.tsx`
  - 형태: Workspace의 오른쪽 sheet. `VersionSheet`처럼 연다.
  - 화면:
    - 머리: "제안함 · 대기 N", 묶음 필터, 진행률(추출 묶음이 있으면 `회차 6/40`)
    - 종류별 묶음: 인물 / 장소 / 사물 / 개념 / 관계 / 팩트 / 스토리라인 / 비트
    - 항목 행:
      - 체크박스
      - 요약
      - 근거 인용과 [씬 열기]
      - 중복 경고: `"같은 대상일 수 있음: 하린(인물)"`와 [기존 하린에 별칭으로] 버튼
      - [편집]: 이름, 요약, 속성, 관계 라벨을 그 자리에서 고친다
      - 의존 안내: `"'하린' 생성이 먼저 필요"`
    - 하단: [선택 항목 적용] [선택 항목 거절]
      - 적용 결과는 토스트로 알린다: 적용 N, 실패 M. 실패 사유는 항목에 표시한다.
    - 실시간 갱신: `mcp.changed`의 tool이 `linetta_propose_story_ops`일 때
  - 테스트:
    - 목록과 요약
    - 선택 적용
    - 의존 항목 자동 선택 안내
    - 별칭 전환
    - 편집 후 적용
    - 실패 표시
    - 이벤트 갱신

- [x] **3.D.3 진입점**
  - `components/CanonPanel.tsx` 머리에 "제안함 N" 배지를 단다. N이 0이면 숨긴다.
  - `routes/Workspace.tsx`: 커맨드 팔레트 항목 "제안함 열기"를 추가한다.
  - `routes/Library.tsx`의 Markdown 가져오기가 성공한 직후, 프로바이더가 연결되어 있으면 "Story World 채우기" 다이얼로그를 띄운다.
    - 다이얼로그 내용: 회차 수, 글자 수, 어림 입력 토큰(`proposals.extraction_estimate`), 사용할 프로바이더와 모델
      - 문구 예: "아무것도 자동으로 적용되지 않습니다. 후보는 제안함에 쌓입니다."
    - [시작]을 누르면 그 작품의 Workspace로 이동한다. 에이전트 패널을 열고 입력창에 추출 문구를 채운다. **보내기는 작가가 누른다.** starter 칩과 같은 방식이다.
    - [나중에]를 누르면 닫는다. 같은 문구는 starter 칩 `fillStoryWorld`로도 언제든 쓸 수 있다.
  - `components/agent/AgentPanel.tsx`: starter 칩 `fillStoryWorld`를 추가한다. Review 그룹이 꺼져 있으면 숨긴다.
  - `lib/agentTools.ts`: `WRITE_TOOL_NAMES`와 라벨

- [x] **3.D.4 복원 정책**
  - 파일: `engine/internal/restore/restore.go`
  - 내용: 제안함은 **작품 단위 복원 대상에서 뺀다.** 함수 머리 주석에 그 이유를 쓴다.
    - pending op는 원래 서재의 entity id를 가리킨다. 복원으로 id가 바뀌면 뜻을 잃는다.
    - 제안은 언제든 다시 만들 수 있다.

- [x] **3.D.5 다국어와 문서**
  - `lib/i18n.tsx`:
    - `proposals.*` (제목, 종류, 중복, 의존, 적용과 거절, 결과 토스트)
    - `importFill.*` (다이얼로그)
    - starter 칩, 도구 라벨
  - `README.md`: 새 절 "Suggestions inbox"를 쓴다.
    - 무엇이 즉시 적용되고(apply) 무엇이 제안함으로 가는지(propose)
    - 원고 가져오기 뒤 Story World 채우기: 비용 어림, 이어 하기
  - `docs/privacy-policy.md`: 추출은 연결된 프로바이더로 원고를 보낸다. 시작 전에 분량을 보여준다.

---

## ✅ Phase 3 Checkpoint

**자동 검증**
- [x] `cd engine && CGO_ENABLED=0 go test ./internal/evidence/... ./internal/continuity/... ./internal/storyops/... ./internal/proposals/... ./internal/mcphost/... ./internal/rpc/handlers/... ./internal/agent/...`
- [x] `make test-mobile-engine`
- [x] `pnpm --dir apps/desktop test -- ProposalInbox.test.tsx CanonPanel.test.tsx AgentPanel.test.tsx agentToolParity.test.ts rpcAllowlist.test.ts i18n.catalog.test.ts --run`
- [x] `pnpm --dir apps/desktop exec tsc --noEmit`
- [x] `make test` — `test-go`의 cgo `-race` 전체 스위트는 이 기기에서 로그인 키체인 프롬프트로 멈추므로 (알려진 제약, linetta-local-test-environment) 대신 `cd engine && CGO_ENABLED=0 go test ./...`로 돌렸다(통과). `bash scripts/validate-story-core-deps.sh`, `make test-desktop`, `make test-tauri`는 그대로 돌렸다(모두 통과). `-race` 전체 스위트는 CI에 맡긴다.

**수동 검증** (격리된 `LINETTA_HOME`, 프로바이더 하나 연결) — 실제 프로바이더 연결과 UI 클릭이 필요해 이번 턴에서는 직접 확인하지 못했다. 각 항목 옆에 같은 동작을 덮는 자동 테스트를 적는다.
- [ ] 회차 5개짜리 Markdown 원고를 가져온다. "Story World 채우기" 다이얼로그에 회차 수, 글자 수, 어림 토큰이 보인다.
  - 자동 테스트: `Library.importFill.test.tsx`("shows the dialog with the estimate when a provider is connected" 등), `proposals.TestExtractionEstimate_sizesTheWholeManuscript`
- [ ] [시작] → 패널에 문구가 채워진다 → 보내기. 다음을 확인한다.
  - 제안함에 후보가 쌓인다.
  - Story World에는 **아직 아무것도 생기지 않았다.**
  - 자동 테스트: `AgentPanel.test.tsx`("pre-fills the composer from initialDraft without sending" — 보내지 않음), `Library.importFill.test.tsx`("'시작' sends the writer to Workspace with the extraction prompt queued, not sent"), `proposals.TestSubmit_acceptsAllowedOpsAndStoresPending`(status가 pending으로만 남음 — storyops 미적용)
- [ ] 미리 만들어 둔 인물 "김민호"와 후보 "김 민호"가 중복으로 표시된다. [별칭으로]를 누른 뒤 적용하면 김민호의 별칭이 늘어난다.
  - 자동 테스트: `proposals.TestSubmit_flagsDuplicateAgainstExistingEntity`, `proposals.TestAccept_aliasConversionEdit`, `ProposalInbox.test.tsx`("converts a duplicate candidate into an alias of the existing entity")
- [ ] 관계 후보만 선택하고 적용하면 "인물 생성이 먼저 필요" 안내가 나온다. 인물을 함께 선택하면 둘 다 적용된다.
  - 자동 테스트: `proposals.TestAccept_refusedWhenDependencyMissing`, `proposals.TestAccept_withDependencyAcceptedTogether`, `ProposalInbox.test.tsx`("auto-selects an unmet dependency and notes it")
- [ ] 도구 호출 한도로 턴이 멈춘 뒤 "계속"을 보내면, 진행률에 표시된 다음 회차부터 이어진다.
  - 자동 테스트: `proposals.TestExtractionProgress_tracksCoverageAndNextEpisode`(다음 회차 계산만 검증; 턴이 실제로 멈췄다가 이어지는 것은 모델 행동이라 끝까지는 덮지 못한다), `agent/prompt_test.go`의 `TestSystemPrompt_statesProposeVsApplyWhenReviewOn`(그 지침 문구가 프롬프트에 들어가는 것까지 보증)
- [ ] 직접 "인물 하나 만들어 줘"라고 부탁하면 제안함을 거치지 않고 바로 만들어진다(apply 경로).
  - `linetta_apply_story_ops` 자체는 이번 Phase에서 바뀌지 않았고 기존 `storyops` 전체 테스트가 그대로 통과한다. 다만 어느 경로를 쓸지는 `agent/prompt.go`의 지침을 읽은 모델이 고르는 것이라 그 선택 자체는 코드로 강제하지 않으며, 이 항목도 직접 확인하지 못했다.

**완료 처리**
1. 결과를 보고한다. 확인하지 못한 항목은 그렇다고 적는다.
2. `feat/story-assist-phase-3-proposals` 브랜치를 푸시하고 PR을 연다. 머지는 사용자 승인 뒤에 한다.
