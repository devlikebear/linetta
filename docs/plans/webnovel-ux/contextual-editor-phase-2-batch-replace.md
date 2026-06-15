# 맥락 편집 Phase 2 — 작품 전체 바꾸기 Preview/Apply

_상위 로드맵: [`contextual-editor-roadmap.md`](./contextual-editor-roadmap.md)_
_선행 조건: [`contextual-editor-phase-1-find-search.md`](./contextual-editor-phase-1-find-search.md) 완료_
_목표: 작품 전체 검색 결과를 변경 후보로 변환하고, 장면별 diff를 확인한 뒤 선택 적용한다._
_예상 소요: 2~3일_

## Goal

작가는 작품 전체에서 `old -> new` 변경 후보를 만들고, 씬별로 어떤 문장이 바뀌는지 확인한 뒤, 선택한 후보만 적용할 수 있다.
적용 전 각 변경 대상 씬에는 snapshot을 남겨야 하며, 실패한 씬은 나머지 적용을 막지 않고 결과에 표시한다.

## Non-goals

- 의미 기반 자동 치환
- LLM batch rewrite
- regex replace
- cross-project replace
- 새 undo 시스템

## Touch points

- `engine/internal/manuscriptedit/` 신규
- `engine/internal/rpc/handlers/manuscript_edit.go` 신규
- `engine/internal/snapshot`
- `engine/internal/node`
- `apps/desktop/src/components/contextual/ContextualEditPanel.tsx`
- `apps/desktop/src/components/contextual/BatchReplaceReview.tsx` 신규
- `apps/desktop/src/lib/rpc.ts`
- `apps/desktop/src/lib/types.ts`
- `apps/desktop/src/routes/Workspace.tsx`

## Engine model

새 패키지 `engine/internal/manuscriptedit`를 둔다. `engine/internal/manuscript`는 검색 전용으로 유지한다.

### Types

```go
type ReplacePlanRequest struct {
    ProjectID   string
    Query       string
    Replacement string
    NodeIDs     []string // optional; empty means hits from search
    MatchCase   bool
    WholeWord   bool // MVP에서는 false만 지원해도 됨
}

type ReplaceCandidate struct {
    ID          string
    NodeID      string
    Breadcrumb  string
    Before      string
    After       string
    Occurrences int
    Selected    bool
}

type ReplacePlan struct {
    ProjectID   string
    Query       string
    Replacement string
    Candidates  []ReplaceCandidate
}

type ApplyReplaceRequest struct {
    ProjectID    string
    Query        string
    Replacement  string
    CandidateIDs []string
}

type ApplyReplaceResult struct {
    Applied  int
    Skipped  int
    Failures []ApplyFailure
}
```

### Implementation notes

- Tiptap JSON을 문자열 전체 replace하지 않는다.
- `text` node만 순회해 replacement를 적용한다.
- mention atom label은 Phase 2에서 변경하지 않는다. Entity rename은 Phase 3.
- 변경 전 `snapshots.Create(nodeID, currentDoc, snapshot.ReasonManual, now)`를 호출한다.
- 적용 후 `nodes.UpdateContent`를 호출한다. 이 경로가 manuscript index를 갱신한다.
- 한 node에 여러 후보가 있으면 node 단위로 한 번에 적용한다.
- candidate id는 deterministic하게 `nodeID:index`로 만들 수 있다.
- node content_version이 preview 이후 바뀐 경우:
  - apply 직전 재계산한다.
  - mismatch면 해당 node는 failure 처리하고 사용자가 preview를 다시 만들게 한다.

## Engine tasks

- [x] **2.1 `manuscriptedit` 패키지 추가**
  - 파일: `engine/internal/manuscriptedit/replace.go`
  - 함수:
    - `PlanReplace(ctx, projectID, query, replacement string, nodeIDs []string) (ReplacePlan, error)`
    - `ApplyReplace(ctx, plan ReplacePlan, candidateIDs []string, now int64) (ApplyReplaceResult, error)`
  - 의존성:
    - `*node.Repo`
    - `*snapshot.Repo`
    - `*manuscript.Searcher`
  - 테스트:
    - 한 씬 여러 occurrence.
    - 여러 씬 중 일부만 apply.
    - text node는 바뀌고 mention atom은 유지.
    - content_version mismatch는 failure.
    - snapshot 생성 확인.

- [x] **2.2 RPC 추가**
  - 파일: `engine/internal/rpc/handlers/manuscript_edit.go`
  - 메서드:
    - `manuscript.replace_preview`
    - `manuscript.replace_apply`
  - error:
    - 빈 query/replacement는 invalid params.
    - 후보 없음은 빈 candidates로 성공.
  - 테스트:
    - invalid params.
    - preview returns candidates.
    - apply returns applied count and failures.

- [x] **2.3 main wiring**
  - 파일: `engine/cmd/linetta-engine/main.go`
  - `manuscriptedit.NewService(nodes, snaps, manuscriptSearcher)` 형태로 연결한다.

## Frontend tasks

- [x] **2.4 타입/RPC 추가**
  - 파일: `apps/desktop/src/lib/types.ts`
  - 타입:
    - `ReplaceCandidate`
    - `ReplacePlan`
    - `ApplyReplaceResult`
  - 파일: `apps/desktop/src/lib/rpc.ts`
  - API:
    - `manuscript.replacePreview(...)`
    - `manuscript.replaceApply(...)`

- [x] **2.5 Batch review UI 추가**
  - 파일: `apps/desktop/src/components/contextual/BatchReplaceReview.tsx`
  - 파일: `apps/desktop/src/components/contextual/BatchReplaceReview.css`
  - 기능:
    - candidate list grouped by scene.
    - checkbox per candidate.
    - select all / deselect all.
    - before/after inline diff.
    - occurrence count.
    - apply button.
    - result summary.
  - UI 기준:
    - 카드 안 카드 금지. scene group은 section band, candidate는 row.
    - 긴 본문은 2~4줄 clamp.
    - diff는 삭제/추가 색상을 과하게 쓰지 않는다.

- [x] **2.6 ContextualEditPanel에 project replace 활성화**
  - 파일: `apps/desktop/src/components/contextual/ContextualEditPanel.tsx`
  - `작품 전체` + `바꾸기` mode에서:
    - 검색어와 replacement 입력.
    - `미리보기` 버튼.
    - candidate review 표시.
    - apply 완료 시 현재 열려 있는 node가 바뀌었으면 Workspace reload.

- [x] **2.7 Workspace refresh**
  - 파일: `apps/desktop/src/routes/Workspace.tsx`
  - batch apply 후:
    - 현재 node가 변경된 경우 `nodes.get(currentNodeId)`로 reload.
    - tree의 word_count/updated_at이 바뀌면 `nodes.listTree(projectId)` refresh.
    - 저장 상태는 `saved` 또는 별도 toast로 표시.

- [x] **2.8 i18n**
  - 파일: `apps/desktop/src/lib/i18n.tsx`
  - keys:
    - `contextual.preview`
    - `contextual.applySelected`
    - `contextual.candidates.count`
    - `contextual.candidates.empty`
    - `contextual.apply.applied`
    - `contextual.apply.failed`
    - `contextual.snapshotNotice`

## Safety requirements

- Apply 버튼은 preview 생성 전 비활성화.
- 선택 후보가 0개면 apply 비활성화.
- 적용 전 snapshot이 실패한 node는 변경하지 않는다.
- 일부 실패해도 성공한 node는 결과에 표시한다.
- 사용자가 `replace all`을 눌러도 "모든 후보 선택 + 적용"이지 무조건 silent replace가 아니다.

## Checkpoint

**자동 검증**

- [x] `cd engine && go test ./internal/manuscriptedit ./internal/rpc/handlers ./internal/node ./internal/snapshot`
- [x] `pnpm --dir apps/desktop test -- ContextualEditPanel.test.tsx BatchReplaceReview.test.tsx --run`
- [x] `pnpm --dir apps/desktop exec tsc --noEmit`
- [ ] `make test`
- [ ] `git diff --check`

**수동 검증**

- [ ] 작품 전체 `민호 -> 민준` preview가 씬별 후보를 만든다.
- [ ] 일부 후보만 선택해 적용하면 선택된 씬만 바뀐다.
- [ ] 현재 열려 있는 씬이 적용 대상이면 editor 내용이 새로 보인다.
- [ ] VersionSheet에서 적용 전 snapshot이 보인다.
- [ ] content_version mismatch 상황에서 해당 씬만 실패로 표시된다.

**이 체크포인트를 통과하면 사용자에게 확인 요청 후 Phase 3로 진행한다.**
