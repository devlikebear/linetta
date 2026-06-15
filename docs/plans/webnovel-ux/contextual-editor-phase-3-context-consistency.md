# 맥락 편집 Phase 3 — 맥락 변경 마법사 + 일관성 재검사

_상위 로드맵: [`contextual-editor-roadmap.md`](./contextual-editor-roadmap.md)_
_선행 조건: [`contextual-editor-phase-1-find-search.md`](./contextual-editor-phase-1-find-search.md), [`contextual-editor-phase-2-batch-replace.md`](./contextual-editor-phase-2-batch-replace.md) 완료_
_목표: 캐릭터·장소·아이템·설정 변경을 작품 전체 작업으로 만들고, 변경 전후 일관성을 확인한다._
_예상 소요: 2~3일_

## Goal

작가는 "캐릭터 이름 변경", "장소명 변경", "주요 소품 이름 변경", "설정 변경"을 하나의 마법사에서 시작할 수 있다.
Linetta는 기존 entity/fact/relationship과 본문 RAG 검색을 함께 사용해 변경 후보를 만들고,
metadata 업데이트와 본문 후보 적용을 분리해서 보여준다. 적용 후에는 남은 표현과 잠재 모순을 다시 보고한다.

## Non-goals

- 임베딩 기반 의미 검색
- LLM이 여러 씬을 자동 재작성
- 관계/팩트의 복잡한 schema migration
- 자동 conflict 해결
- 외부 지식 검증

## Touch points

- `engine/internal/entity`
- `engine/internal/fact`
- `engine/internal/mention`
- `engine/internal/manuscript`
- `engine/internal/manuscriptedit`
- `engine/internal/rpc/handlers/contextual_edit.go` 신규
- `apps/desktop/src/components/contextual/ContextChangeWizard.tsx` 신규
- `apps/desktop/src/components/EntitySheet.tsx`
- `apps/desktop/src/components/FactBookPanel.tsx`
- `apps/desktop/src/components/companion/CompanionPanel.tsx`

## Concept

Phase 2의 batch replace는 문자열 중심이다. Phase 3은 "대상" 중심이다.

예:

- 캐릭터 `민호`
  - canonical name: `민호`
  - aliases: `민호 형`, `김민호`, `그 남자`
  - 새 이름: `민준`
  - alias 정책:
    - `김민호` -> `김민준`
    - `민호 형` -> `민준 형`
    - `그 남자`는 그대로
- 장소 `백천`
  - 새 이름: `백연`
  - 본문, entity.name, relationship notes, fact result 후보 검토
- 아이템 `푸른 시계`
  - 새 이름과 설정 변경
  - 본문 묘사 중 이름만 바꿀 후보와 설정 문장 재검토 후보 분리

## UX

### Context Change Wizard

진입:

- `ContextualEditPanel`의 `맥락 변경` tab
- `EntitySheet`의 `작품 전체 변경...`
- `FactBookPanel`의 `이 설정 영향 확인`
- 컴패니언 액션 프리셋 `설정 변경 영향 확인`

Step:

1. 대상 선택
   - free text
   - existing entity
   - fact card
   - selected text
2. 변경 유형 선택
   - 이름 변경
   - 별칭 추가/정리
   - 장소/조직명 변경
   - 아이템/스킬/마법명 변경
   - 설정 문장 변경
3. 변경 내용 입력
   - old terms
   - new terms
   - aliases
   - "직접 치환하지 말고 검토만" toggle
4. 후보 생성
   - metadata updates
   - manuscript replace candidates
   - "검토 필요" candidates
5. 적용
   - metadata update ops
   - selected manuscript candidates
6. 일관성 재검사
   - remaining old terms
   - possible alias misses
   - conflicting fact/entity mentions

## Engine tasks

구현 메모: MVP에서는 resolver/planner/apply/consistency를 `engine/internal/contextualedit/contextualedit.go`에 통합 구현했다.

- [x] **3.1 Context target resolver**
  - 파일: `engine/internal/contextualedit/resolver.go` 신규
  - 입력:
    - `project_id`
    - `entity_id?`
    - `fact_id?`
    - `selected_text?`
    - `query?`
  - 출력:
    - canonical name
    - aliases
    - kind (`character|place|item|concept|fact|free_text`)
    - related entity/fact/relationship ids
  - 기존 repos:
    - `entity.Repo`
    - `fact.Repo`
    - `relationship.Repo`

- [x] **3.2 Context change planner**
  - 파일: `engine/internal/contextualedit/planner.go`
  - 함수:
    ```go
    PlanContextChange(ctx, input ChangeInput) (ChangePlan, error)
    ```
  - 출력:
    - metadata candidate list
    - manuscript replace plan from Phase 2
    - review-only hits
    - warnings
  - 규칙:
    - character/place/item/concept의 `name` 변경은 metadata candidate로 표시한다.
    - aliases는 자동 삭제하지 않는다. 새 alias 추가 제안까지만.
    - fact card의 `claim/result` 변경은 자동 적용하지 않고 review candidate로 둔다.
    - relationship notes는 MVP에서 자동 변경하지 않는다. 검색 결과로 노출한다.

- [x] **3.3 Context change apply**
  - 파일: `engine/internal/contextualedit/apply.go`
  - metadata apply:
    - `entities.Update`
    - aliases 처리 방식이 기존 Repo에 없으면 Phase 3에서는 name/summary/attributes만 적용하고 alias 변경은 out-of-scope로 남긴다.
  - manuscript apply:
    - Phase 2 `manuscriptedit.ApplyReplace`
  - 결과:
    - metadata applied
    - manuscript applied
    - failures

- [x] **3.4 Consistency check**
  - 파일: `engine/internal/contextualedit/consistency.go`
  - 함수:
    ```go
    CheckAfterChange(ctx, projectID string, terms []string, changedEntityIDs []string) (ConsistencyReport, error)
    ```
  - checks:
    - old terms still present.
    - new terms absent in expected scenes.
    - entity/fact summary still contains old term.
    - manuscript search has conflicting nearby snippets.
  - MVP는 deterministic report다. LLM 해석은 별도 "컴패니언에게 검토 요청" 버튼으로 넘긴다.

- [x] **3.5 RPC handlers**
  - 파일: `engine/internal/rpc/handlers/contextual_edit.go`
  - 메서드:
    - `contextual.resolve_target`
    - `contextual.plan_change`
    - `contextual.apply_change`
    - `contextual.check_consistency`
  - 테스트:
    - `engine/internal/rpc/handlers/contextual_edit_test.go`

## Frontend tasks

- [x] **3.6 ContextChangeWizard 추가**
  - 파일: `apps/desktop/src/components/contextual/ContextChangeWizard.tsx`
  - 파일: `apps/desktop/src/components/contextual/ContextChangeWizard.css`
  - props:
    ```ts
    interface Props {
      projectId: string;
      currentNodeId?: string;
      initialEntityId?: string;
      initialText?: string;
      onApplied: () => void;
    }
    ```
  - states:
    - target selection
    - change input
    - plan loading
    - plan review
    - applying
    - consistency report

- [x] **3.7 EntitySheet 연결**
  - 파일: `apps/desktop/src/components/EntitySheet.tsx`
  - `작품 전체 변경...` button 추가.
  - 클릭 시 Workspace에 `contextualEditOpen` + `initialEntityId` 전달.
  - 기존 entity edit form과 충돌하지 않게 wizard는 별도 panel state로 둔다.

- [x] **3.8 FactBookPanel 연결**
  - 파일: `apps/desktop/src/components/FactBookPanel.tsx`
  - fact card menu에 `영향 확인` 추가.
  - 자동 수정이 아니라 consistency report를 먼저 보여준다.

- [x] **3.9 Companion action 연결**
  - 파일: `apps/desktop/src/components/companion/CompanionPanel.tsx`
  - 액션 프리셋이 있다면:
    - `작품 전체 이름 변경`
    - `설정 변경 영향 확인`
  - MVP에서 컴패니언은 wizard를 직접 열 수 없으면 prompt만 채운다.
  - 프롬프트는 `search_manuscript` 사용을 유도하되, 실제 적용은 ContextualEditPanel로 하도록 안내한다.

- [x] **3.10 i18n**
  - 파일: `apps/desktop/src/lib/i18n.tsx`
  - keys:
    - `contextual.change.title`
    - `contextual.change.target`
    - `contextual.change.type.rename`
    - `contextual.change.type.setting`
    - `contextual.change.oldTerms`
    - `contextual.change.newTerms`
    - `contextual.change.metadata`
    - `contextual.change.manuscript`
    - `contextual.consistency.title`
    - `contextual.consistency.remainingOldTerms`
    - `contextual.consistency.reviewNeeded`

## Consistency report shape

```ts
interface ConsistencyIssue {
  severity: "info" | "warning" | "risk";
  kind: "remaining_old_term" | "metadata_stale" | "possible_conflict" | "review_needed";
  node_id?: string;
  breadcrumb?: string;
  snippet?: string;
  message: string;
}

interface ConsistencyReport {
  ok: boolean;
  issues: ConsistencyIssue[];
}
```

## Safety requirements

- entity/fact metadata 변경과 manuscript 변경은 서로 다른 checkbox group으로 표시한다.
- "검토 필요" 후보는 기본 unchecked.
- fact claim/result는 자동 변경하지 않는다.
- relationship notes는 자동 변경하지 않는다.
- consistency report가 warning을 내도 이미 적용된 변경을 자동 rollback하지 않는다. 사용자가 snapshot으로 복구한다.

## Checkpoint

**자동 검증**

- [x] `cd engine && go test ./internal/contextualedit ./internal/rpc/handlers ./internal/manuscriptedit`
- [x] `pnpm --dir apps/desktop test -- ContextChangeWizard.test.tsx ContextualEditPanel.test.tsx EntitySheet.test.tsx FactBookPanel.test.tsx --run`
- [x] `pnpm --dir apps/desktop exec tsc --noEmit`
- [x] `make test`
- [x] `git diff --check`

**수동 검증**

- [ ] EntitySheet에서 캐릭터 `민호`의 `작품 전체 변경...`을 연다.
- [ ] 새 이름 `민준`을 입력하면 본문 후보와 entity metadata 후보가 분리되어 보인다.
- [ ] 일부 본문 후보와 metadata 변경을 적용한다.
- [ ] 일관성 report가 남은 `민호` 표현을 찾아낸다.
- [ ] FactBookPanel의 설정 변경 영향 확인은 자동 수정 없이 report만 보여준다.

**Final**

- [x] `LINETTA_HOME=/tmp/linetta-contextual-editor ./scripts/dev.sh`
- [ ] 실제 Tauri 앱에서 현재 씬 편집, 작품 전체 preview/apply, 맥락 변경 wizard, consistency report를 한 흐름으로 확인한다.
- [x] 사용자가 확인하면 릴리즈 전 `make validate-distribution`을 추가로 실행한다.
