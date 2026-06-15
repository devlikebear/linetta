# 맥락 편집 Phase 1 — 현재 씬 찾기/바꾸기 + 작품 본문 검색

_상위 로드맵: [`contextual-editor-roadmap.md`](./contextual-editor-roadmap.md)_
_목표: 현재 씬 안에서 즉시 쓸 수 있는 찾기/바꾸기와, 작품 전체 본문 검색 결과를 UI에 노출하는 첫 수직 슬라이스_
_예상 소요: 1.5~2일_

## Goal

작가는 현재 씬 안에서 단어를 찾아 이동하고 바꿀 수 있으며, 같은 워크벤치에서 작품 전체 본문 검색 결과를 씬별로 확인하고 해당 씬으로 이동할 수 있다.
이 단계에서는 작품 전체 적용은 하지 않는다. 전체 적용은 Phase 2에서 preview/apply로 추가한다.

## Non-goals

- 작품 전체 replace apply
- entity metadata 수정
- LLM/컴패니언 자동 판단
- 의미 검색/임베딩
- regex mode

## Touch points

- `engine/internal/manuscript/searcher.go`
- `engine/internal/rpc/handlers/manuscript.go` 신규
- `engine/cmd/linetta-engine/main.go`
- `apps/desktop/src/lib/types.ts`
- `apps/desktop/src/lib/rpc.ts`
- `apps/desktop/src/components/editor/Tiptap.tsx`
- `apps/desktop/src/components/contextual/ContextualEditPanel.tsx` 신규
- `apps/desktop/src/routes/Workspace.tsx`
- `apps/desktop/src/lib/i18n.tsx`

## UX

`ContextualEditPanel`은 오른쪽 패널로 열린다.

- Header: `맥락 편집`
- Scope segmented control:
  - `현재 씬`
  - `작품 전체`
- Phase 1 tabs:
  - `찾기`
  - `바꾸기`
- 현재 씬 scope:
  - 검색어 입력
  - match count
  - 이전/다음 버튼
  - replacement 입력
  - `현재 항목 바꾸기`
  - `이 씬 전체 바꾸기`
- 작품 전체 scope:
  - 검색어 입력
  - 결과 목록: scene breadcrumb, snippet, updated time
  - 클릭 시 해당 씬으로 이동
  - `작품 전체 바꾸기` 버튼은 disabled 또는 Phase 2 안내 상태

## Engine tasks

- [x] **1.1 `manuscript.Searcher` 결과 확장**
  - 파일: `engine/internal/manuscript/searcher.go`
  - `Hit`에 `Preview`, `UpdatedAt`이 필요하면 추가한다. 기존 `Snippet`을 그대로 써도 된다.
  - `Query(ctx, projectID, q, limit)`는 project scope를 유지한다.
  - 테스트:
    - `engine/internal/manuscript/manuscript_test.go`
    - 한국어 검색, 2자 이름 fallback, breadcrumb 포함.

- [x] **1.2 RPC handler 추가**
  - 파일: `engine/internal/rpc/handlers/manuscript.go` 신규
  - 메서드: `manuscript.search`
  - params:
    ```json
    {"project_id":"...", "query":"...", "limit":20}
    ```
  - response:
    ```json
    [{"node_id":"...", "breadcrumb":"1부 / 1장 / 씬 1", "snippet":"...", "updated_at":123}]
    ```
  - 빈 query는 `CodeInvalidParams`.
  - 테스트:
    - `engine/internal/rpc/handlers/manuscript_test.go`

- [x] **1.3 engine main 연결**
  - 파일: `engine/cmd/linetta-engine/main.go`
  - 기존 Manuscript RAG wiring의 `manuscriptSearcher`를 `s.Handle("manuscript.search", handlers.SearchManuscript(manuscriptSearcher))`에 연결한다.

## Frontend tasks

- [x] **1.4 타입/RPC 추가**
  - 파일: `apps/desktop/src/lib/types.ts`
  - 타입:
    ```ts
    export interface ManuscriptSearchHit {
      node_id: string;
      breadcrumb: string;
      snippet: string;
      updated_at?: number;
    }
    ```
  - 파일: `apps/desktop/src/lib/rpc.ts`
  - API:
    ```ts
    export const manuscript = {
      search: (projectId: string, query: string, limit = 20) =>
        rpcCall<ManuscriptSearchHit[]>("manuscript.search", { project_id: projectId, query, limit }),
    };
    ```

- [x] **1.5 Tiptap current-scene find API**
  - 파일: `apps/desktop/src/components/editor/Tiptap.tsx`
  - `TiptapHandle`에 다음을 추가한다.
    - `findText(query: string): { count: number; activeIndex: number }`
    - `nextMatch(): void`
    - `prevMatch(): void`
    - `replaceActiveMatch(replacement: string): object | null`
    - `replaceAllMatches(query: string, replacement: string): object | null`
  - 구현 메모:
    - MVP는 plain text node 중심으로 처리한다.
    - mention atom 내부 label은 이 단계에서 바꾸지 않는다.
    - replace 후 `onChange`가 호출되어 autosave 경로를 타야 한다.
    - match highlight는 ProseMirror decorations extension으로 추가하거나, Phase 1 MVP에서는 selection 이동만 먼저 허용한다.
  - 테스트:
    - `apps/desktop/src/components/editor/Tiptap.test.tsx`
    - 검색 count, next/prev selection 이동, current replace, scene replace all.

- [x] **1.6 `ContextualEditPanel` 추가**
  - 파일: `apps/desktop/src/components/contextual/ContextualEditPanel.tsx`
  - 파일: `apps/desktop/src/components/contextual/ContextualEditPanel.css`
  - props:
    ```ts
    interface Props {
      open: boolean;
      projectId: string;
      currentNodeId: string;
      editorRef: React.RefObject<TiptapHandle>;
      onNavigateNode: (nodeId: string) => void;
      onClose: () => void;
    }
    ```
  - 현재 씬 mode는 `editorRef.current` API를 사용한다.
  - 작품 전체 mode는 `manuscript.search`를 debounce 호출한다.
  - 디자인:
    - 기존 `CompanionPanel`/`SearchModal`의 조용한 다크 패널 톤을 따른다.
    - nested card 금지. 결과 row는 리스트형으로.
    - 버튼에는 lucide icon 사용.
  - 테스트:
    - `ContextualEditPanel.test.tsx`
    - current scene 검색 input -> editor API 호출
    - whole work 검색 input -> RPC 호출
    - 결과 클릭 -> `onNavigateNode`

- [x] **1.7 Workspace 연결**
  - 파일: `apps/desktop/src/routes/Workspace.tsx`
  - state: `const [contextualEditOpen, setContextualEditOpen] = useState(false)`
  - Command Palette에 `맥락 편집 열기` 추가.
  - 단축키:
    - 기존 Cmd+F는 앱 전역 검색 유지.
    - 새 단축키는 MVP에서 지정하지 않거나 `Cmd+Shift+F`로 시작한다.
  - 오른쪽 패널 공간에서 `CompanionPanel`/`FactBookPanel`과 충돌하지 않게 하나를 열면 다른 편집 보조 패널은 닫는다.

- [x] **1.8 i18n 추가**
  - 파일: `apps/desktop/src/lib/i18n.tsx`
  - ko/en/ja:
    - `contextual.title`
    - `contextual.scope.scene`
    - `contextual.scope.project`
    - `contextual.find.placeholder`
    - `contextual.replace.placeholder`
    - `contextual.replace.current`
    - `contextual.replace.sceneAll`
    - `contextual.projectReplace.disabled`
    - `workspace.command.contextualEdit`

## Checkpoint

**자동 검증**

- [x] `cd engine && go test ./internal/manuscript ./internal/rpc/handlers ./cmd/linetta-engine`
- [x] `pnpm --dir apps/desktop test -- ContextualEditPanel.test.tsx Tiptap.test.tsx --run`
- [x] `pnpm --dir apps/desktop exec tsc --noEmit`
- [x] `git diff --check`

**수동 검증**

- [ ] 현재 씬에서 `민호` 검색 시 match count가 보인다.
- [ ] 이전/다음 이동이 실제 editor selection을 움직인다.
- [ ] 현재 항목 바꾸기 후 autosave 상태가 갱신된다.
- [ ] 작품 전체 검색에서 다른 씬 결과가 보이고 클릭 시 이동한다.

**이 체크포인트를 통과하면 사용자에게 확인 요청 후 Phase 2로 진행한다.**
