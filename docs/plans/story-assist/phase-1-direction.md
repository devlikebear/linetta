# Phase 1 — 작가 방향: 현재 초점과 씬 의도

_작성일: 2026-10-24_
_상위 로드맵: [`roadmap.md`](./roadmap.md)_
_근거: [`inkos-analysis.md`](./inkos-analysis.md) §2_
_예상 소요: 1.5~2일_

## Goal

작가가 두 가지를 적어 둘 수 있다.
- 작품당 하나의 **현재 초점**: "앞으로 몇 회차 동안 무엇에 집중할지"
- 씬마다의 **씬 의도**: 목표 한 줄, 지킬 것, 피할 것

내장 에이전트와 MCP 클라이언트는 둘 다 `linetta_get_story_context` 브리프 **맨 위**의 "작가 방향" 섹션에서 이것을 읽는다.
초점을 정한 뒤 세 회차가 더 쓰이면, 앱과 브리프가 "아직 유효한가요?"라고 표시한다.

이 페이즈에는 LLM 호출이 없다. 저장, 브리프, 화면만 만든다.

## 설계 결정 (InkOS와 다르게 가는 부분)

- **장기 의도(author_intent) 문서는 새로 만들지 않는다.**
  - 리네타에는 이미 시놉시스(`projects.synopsis`)와 플롯 개요(`projects.outline`)가 있다. 둘 다 작가가 소유하는 장기 방향이다.
  - 문서를 하나 더 두면 InkOS처럼 같은 방향이 프롬프트에 세 번 들어간다.
- **작품 메모(work notes)에 섞지 않는다.**
  - 작품 메모는 에이전트가 `linetta_edit_memory`로 기록하는 문서다.
  - 작가의 지시를 에이전트가 쓸 수 있는 문서에 두면, 에이전트가 정리하다 지울 수 있다.
- **구조화된 지킬 것·피할 것을 유지한다.**
  - InkOS는 must-keep / must-avoid가 자유 Markdown으로 무너진 뒤로 지켰는지 확인할 방법을 잃었다.
  - Phase 2 연속성 점검이 "피할 것 위반"을 기준으로 쓴다.
- **배치를 위로 올린다.**
  - InkOS `writer.ts:463-466`의 교훈이다. 방향을 근거 목록 속에 묻으면 모델이 선택 사항으로 읽는다.
  - 그래서 브리프 사용자 메시지의 첫 섹션으로 둔다.
- **비어 있으면 넣지 않는다.**
  - 템플릿 문구나 빈 칸은 섹션 자체를 생략한다.
- **에이전트는 쓸 수 없다.**
  - MCP 쓰기 도구가 없다. 에이전트의 초안 제안은 로드맵의 Next로 미룬다.

## Non-goals

- 에이전트의 초점·씬 의도 초안 제안 → Next
- Markdown 내보내기·가져오기(`mdmeta`) 포함 → Next
- 회차(에피소드 컨테이너) 단위 의도. MVP는 씬 단위만 둔다.

## Touch points

- `engine/internal/store/migrations/0024_writer_direction.sql` 신규
- `engine/internal/direction/` 신규 패키지
- `engine/internal/episode/episode.go` (`LastWritten`)
- `engine/internal/storycontext/types.go`, `builder.go`, `render.go`
- `engine/internal/mcphost/tools_read.go` (`getStoryContextInput`, `sectionReport`)
- `engine/internal/rpc/handlers/direction.go` 신규, `engine/internal/rpc` (reason 코드)
- `engine/internal/engineapp/engineapp.go`
- `engine/internal/restore/restore.go`
- `engine/internal/agent/prompt.go`
- `apps/desktop/src-tauri/src/lib.rs` (`RENDERER_ENGINE_METHODS`)
- `apps/desktop/src/lib/rpc.ts`, `lib/types.ts`, `lib/i18n.tsx`
- `apps/desktop/src/components/DirectionSection.tsx` 신규, `components/ContextPanel.tsx`
- `docs/privacy-policy.md`, `README.md`

## 작업 체크리스트

### A. 저장

- [ ] **1.A.1 마이그레이션**
  - 파일: `engine/internal/store/migrations/0024_writer_direction.sql`
  - 내용:
    ```sql
    -- The writer's own direction for a work (story-assist Phase 1). Kept out
    -- of agent_memory on purpose: work notes are what an agent records, and
    -- nothing an agent can edit may hold an instruction the writer gave it.
    CREATE TABLE project_focus (
      project_id     TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
      body           TEXT NOT NULL,
      anchor_node_id TEXT,              -- episode node the focus was set at
      set_at         INTEGER NOT NULL
    );
    CREATE TABLE scene_intents (
      node_id    TEXT PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
      project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
      goal       TEXT NOT NULL DEFAULT '',
      keep_json  TEXT NOT NULL DEFAULT '[]',
      avoid_json TEXT NOT NULL DEFAULT '[]',
      updated_at INTEGER NOT NULL
    );
    CREATE INDEX scene_intents_project ON scene_intents(project_id);
    ```
  - 검증: `CGO_ENABLED=0 go test ./internal/store/...` (`migrations_test.go`가 순번과 적용을 확인)

- [ ] **1.A.2 `direction` 패키지: 타입과 한도**
  - 파일: `engine/internal/direction/direction.go`
  - 내용:
    - 상수
      - `MaxFocusRunes = 500`
      - `MaxGoalRunes = 200`
      - `MaxListItems = 5`
      - `MaxItemRunes = 120`
      - `FocusStaleAfterEpisodes = 3`
    - 타입
      - `Focus{ProjectID, Body, AnchorNodeID, SetAt}`
      - `FocusState{Focus; Present bool; EpisodesSince int; Stale bool}`
      - `SceneIntent{NodeID, ProjectID, Goal string; Keep, Avoid []string; UpdatedAt}`
    - 오류
      - `ErrFocusTooLong`, `ErrGoalTooLong`, `ErrTooManyItems`, `ErrItemTooLong`, `ErrProjectNotFound`, `ErrNodeNotFound`, `ErrNotScene`
    - **한도를 넘으면 자르지 말고 거부한다.** `agentmemory.ErrOverBudget`, `style.ErrDraftTooLong`과 같은 이유다. 잘린 지시는 뜻이 바뀐다.
    - 목록 정규화: 앞뒤 공백을 지우고, 빈 항목을 버리고, 중복을 없앤다. 대소문자 접기는 `style.foldCase` 방식과 같게 한다.
  - 참조: `engine/internal/style/draft.go`, `engine/internal/agentmemory/agentmemory.go`

- [ ] **1.A.3 Repo**
  - 파일: `engine/internal/direction/repo.go`, `repo_test.go`
  - 함수:
    ```go
    func NewRepo(s *store.Store, nodes *node.Repo, projects *project.Repo) *Repo
    func (r *Repo) GetFocus(ctx context.Context, projectID string) (FocusState, error)
    func (r *Repo) SetFocus(ctx context.Context, now int64, projectID, body string) (FocusState, error)
    func (r *Repo) GetSceneIntent(ctx context.Context, nodeID string) (SceneIntent, bool, error)
    func (r *Repo) SetSceneIntent(ctx context.Context, now int64, in SceneIntent) (SceneIntent, error)
    ```
  - 규칙:
    - `SetFocus`에 빈 body가 오면 행을 지운다. 초점 없음도 유효한 상태다.
    - `SetFocus`는 `anchor_node_id`를 **엔진이 정한다.** 클라이언트가 보내지 않는다.
      - 값은 글이 있는 마지막 회차의 `NodeID`다.
      - 이 계산은 `engine/internal/episode/episode.go`에 공개 함수로 둔다:
        `func LastWritten(ctx context.Context, nr *node.Repo, projectID, preset string) (Episode, bool, error)`
        - `List`가 돌려준 회차 중 LeafIDs 가운데 word_count > 0인 씬이 있는 마지막 회차를 돌려준다.
        - Phase 4 갈래 예측의 기준 회차도 이 함수를 쓴다.
        - 테스트는 `episode_test.go`에 둔다.
    - `GetFocus`가 계산하는 값:
      - `EpisodesSince` = 지금 글이 있는 마지막 회차의 Ordinal − 기준 회차의 Ordinal
      - `Stale` = `EpisodesSince >= FocusStaleAfterEpisodes`
      - 기준 회차가 지워졌으면 `Stale = true`, `EpisodesSince = -1`로 둔다. UI는 이 경우 "기준 회차를 찾을 수 없음"으로 표시한다.
    - `SetSceneIntent`
      - 노드가 leaf(씬)가 아니면 `ErrNotScene`.
      - goal, keep, avoid가 모두 비면 행을 지운다.
  - 테스트 (table-driven):
    - 한도 초과는 거부되고 자르지 않는다.
    - 빈 입력이면 행이 지워진다.
    - 노드를 지우면 씬 의도도 cascade로 지워진다.
    - 낡음 계산: 기준 뒤 0회차, 2회차, 3회차, 기준 회차 삭제
  - 검증: `cd engine && CGO_ENABLED=0 go test ./internal/direction/...`

### B. 브리프

- [ ] **1.B.1 ContextKey 사슬 추가**
  - 파일: `engine/internal/storycontext/types.go`, `builder.go`
  - 내용:
    - `ContextKeyDirection ContextKey = "direction"`를 추가한다.
    - `types.go`의 `ContextKeySkills` 주석이 경고하듯, 키만 있고 스위치가 없으면 결함이다. 그래서 사슬 전체를 함께 추가한다.
      - `ContextSelection.Direction *bool`
      - `Enabled`의 case
      - `ApplyContextSelection`(`builder.go:333`)
      - `getStoryContextInput.IncludeDirection`(`mcphost/tools_read.go`)
      - `sectionReport`
    - `Context`에 필드를 추가한다.
      ```go
      // Direction is the writer's own instruction: the work's current focus and
      // this scene's intent. Only the app writes it (no MCP tool does), and it
      // renders first in the user message because direction buried among the
      // evidence is read as optional.
      Direction DirectionBrief `json:"direction,omitempty"`
      ```
      ```go
      type DirectionBrief struct {
          Focus              string   `json:"focus,omitempty"`
          FocusStale         bool     `json:"focus_stale,omitempty"`
          FocusEpisodesSince int      `json:"focus_episodes_since,omitempty"`
          SceneGoal          string   `json:"scene_goal,omitempty"`
          Keep               []string `json:"keep,omitempty"`
          Avoid              []string `json:"avoid,omitempty"`
      }
      ```
    - 주입:
      - `DirectionSource` 인터페이스(`FocusFor(projectID)`, `SceneIntentFor(nodeID)`)를 둔다.
      - `func (b *ContextBuilder) WithDirectionSource(s DirectionSource) *ContextBuilder`를 추가한다. 기존 `WithFactSource` 등과 같은 방식이다.
      - storycontext가 direction 패키지를 import하지 않도록 인터페이스로만 받는다.
    - 실패 처리: 소스 오류는 다른 소스와 같이 빈 섹션으로 둔다. 브리프 생성을 중단하지 않는다(`builder.go:248` 주석 관례).
  - 프론트:
    - `lib/types.ts` `AIContextKey`에 `"direction"`을 추가한다.
    - AI 맥락 체크리스트가 키 목록을 갖고 있으면 그 목록에도 추가한다.

- [ ] **1.B.2 렌더링**
  - 파일: `engine/internal/storycontext/render.go`, `render_test.go`
  - 내용:
    - `buildUser`의 **첫 섹션**("## 작품 설정"보다 앞)에 둔다.
    - 제목 (`langPick`):
      - ko: `## 작가 방향 (반드시 따름)`
      - en: `## Writer's Direction (binding)`
      - ja: `## 作家の方向性（必ず従う）`
    - 본문 줄:
      - `지금의 초점: …`
        - 낡았으면 같은 줄 끝에 `(N회차 전에 정한 초점 — 따르기 전에 작가에게 아직 유효한지 확인)`을 붙인다.
      - `이 씬의 목표: …`
      - `지킬 것:` 글머리표 목록
      - `피할 것:` 글머리표 목록
    - 모든 필드가 비면 섹션 제목까지 생략한다.
  - 테스트:
    - 비면 섹션이 없다.
    - 섹션이 다른 모든 섹션보다 앞에 온다.
    - 낡음 문구가 붙는다.
    - ko / en / ja 제목
    - `ContextSelection.Direction=false`면 빠진다.
  - 검증: `cd engine && CGO_ENABLED=0 go test ./internal/storycontext/...`

- [ ] **1.B.3 MCP 브리프 입력과 보고**
  - 파일: `engine/internal/mcphost/tools_read.go`, `tools_read_test.go`
  - 내용:
    - `IncludeDirection *bool`. 설명: `"set false to leave out the writer's direction (current focus and this scene's intent); on by default"`
    - `sectionReport`가 direction의 포함 여부와 비어 있음을 보고한다.
  - 테스트: read_only 모드에서도 방향이 브리프에 나온다. 읽기 도구 경로이기 때문이다.

- [ ] **1.B.4 engineapp 배선**
  - 파일: `engine/internal/engineapp/engineapp.go`
  - 내용:
    - `direction.NewRepo(...)`를 만든다.
    - 브리프 빌더들에 `WithDirectionSource`를 단다.
      - `mcpContextBuilder`(`engineapp.go:255`) 외에도 `storycontext.NewContextBuilder`를 부르는 곳을 모두 찾아서 단다.

### C. RPC와 복원

- [ ] **1.C.1 RPC 핸들러**
  - 파일: `engine/internal/rpc/handlers/direction.go`, `direction_test.go`
  - 메서드:
    - `direction.get_focus {project_id}` → `FocusState`
    - `direction.set_focus {project_id, body}` → `FocusState`
    - `direction.get_scene_intent {node_id}` → `{present, intent}`
    - `direction.set_scene_intent {node_id, goal, keep, avoid}` → `SceneIntent`
  - 오류 매핑: `style.go`의 `styleError`처럼 표로 매핑하고, `rpc.Reason…` 코드를 새로 추가한다.
  - 등록: `engineapp.go`에서 `s.Handle(...)`
  - 허용 목록: `apps/desktop/src-tauri/src/lib.rs` `RENDERER_ENGINE_METHODS`에 4개를 추가한다.
  - 검증:
    - `cd engine && CGO_ENABLED=0 go test ./internal/rpc/handlers/ -run Direction`
    - `pnpm --dir apps/desktop test -- rpcAllowlist.test.ts --run`

- [ ] **1.C.2 작품 단위 복원**
  - 파일: `engine/internal/restore/restore.go`, 복원 테스트
  - 내용:
    - `project_focus`: project_id를 새 id로 바꾸고, `anchor_node_id`는 node id 매핑으로 바꾼다. 매핑에 없으면 NULL로 둔다.
    - `scene_intents`: node_id와 project_id를 재매핑한다.
    - 이 함수 머리 주석(`restore.go:181`)의 테이블 목록에도 추가한다.
  - 테스트: 백업에서 복원한 작품에 초점과 씬 의도가 따라온다.

### D. 에이전트와 화면

- [ ] **1.D.1 에이전트 지침 한 줄**
  - 파일: `engine/internal/agent/prompt.go`, `prompt_test.go`
  - 내용: "How you work" 목록에 다음 줄을 추가한다.
    > The story brief's Writer's Direction section is the writer's own instruction. Follow it over your own judgment; if a request conflicts with it, say so before acting. You cannot edit it — if it looks out of date, tell the writer.
  - 테스트: 프롬프트에 이 줄이 있다. 그룹 스위치와는 관계없다.

- [ ] **1.D.2 `DirectionSection` 컴포넌트**
  - 파일: `apps/desktop/src/components/DirectionSection.tsx`, `.css`, `.test.tsx`
  - props: `{ projectId: string; nodeId?: string }`
  - 화면:
    - "지금의 초점"
      - 여러 줄 입력, 글자 수 `n/500`
      - 저장 버튼
      - 낡음 배지: "3회차 전에 정함 · 아직 유효한가요?" 그리고 [그대로 유지] 버튼
        - [그대로 유지]는 같은 본문으로 `set_focus`를 다시 불러 기준 회차를 새로 잡는다.
    - "이 씬의 의도" (`nodeId`가 있을 때만)
      - 목표 한 줄
      - 지킬 것·피할 것: 한 줄에 한 항목, 최대 5개
      - 저장 버튼
    - 한도를 넘으면 저장 버튼을 막고 이유를 보여준다. 엔진 오류 문구는 `rpcErrorMessage`로 보여준다.
  - 참조: `components/StyleSection.tsx` (상태, `busy`, `run` 패턴)
  - 테스트:
    - 불러오기
    - 저장
    - 한도 초과
    - 낡음 배지와 [그대로 유지]
    - 씬이 없을 때 씬 의도 영역 숨김

- [ ] **1.D.3 `ContextPanel`에 배치**
  - 파일: `apps/desktop/src/components/ContextPanel.tsx`
  - 내용: 현재 씬의 사이드 패널 상단에 `DirectionSection`을 접을 수 있게 둔다. 접힘 상태는 localStorage에 저장하고, 읽기·쓰기 실패는 무시한다.

- [ ] **1.D.4 다국어**
  - 파일: `apps/desktop/src/lib/i18n.tsx`
  - 키:
    - `direction.focus.title`, `direction.focus.placeholder`, `direction.focus.stale`, `direction.focus.keep`, `direction.focus.anchorMissing`
    - `direction.scene.title`, `direction.scene.goal`, `direction.scene.keep`, `direction.scene.avoid`
    - `direction.limit.focus`, `direction.limit.items`, `direction.limit.item`
  - 검증: `pnpm --dir apps/desktop test -- i18n.catalog.test.ts --run`

- [ ] **1.D.5 문서**
  - `README.md`: "What the agent remembers" 앞에 짧은 절 "Your direction"을 둔다.
    - 초점과 씬 의도는 작가만 쓴다.
    - 브리프 맨 위에 들어간다.
    - 세 회차가 지나면 확인을 요청한다.
    - MCP 클라이언트(read_only 포함)도 읽는다.
  - `docs/privacy-policy.md`: 방향 문서는 로컬에 저장되며, 프로바이더를 연결한 경우 브리프의 일부로 전송된다.

---

## ✅ Phase 1 Checkpoint

**자동 검증**
- [ ] `cd engine && CGO_ENABLED=0 go test ./internal/direction/... ./internal/storycontext/... ./internal/mcphost/... ./internal/rpc/handlers/... ./internal/restore/... ./internal/agent/...`
- [ ] `make test-mobile-engine`
- [ ] `pnpm --dir apps/desktop test -- DirectionSection.test.tsx ContextPanel.test.tsx rpcAllowlist.test.ts i18n.catalog.test.ts --run`
- [ ] `pnpm --dir apps/desktop exec tsc --noEmit`
- [ ] `make test`
- [ ] `git diff --check`

**수동 검증** (`LINETTA_HOME=/tmp/linetta-story-assist ./scripts/dev.sh`)
- [ ] 씬을 열고 초점 "둘의 오해를 아직 풀지 않는다"와 피할 것 "고백"을 저장한다.
- [ ] 에이전트 패널에서 이 씬의 다음 장면을 요청한다. 활동 로그의 `linetta_get_story_context` 결과 첫 섹션이 "작가 방향"인지 확인한다.
- [ ] 세 회차 분량을 더 쓴 뒤 다시 열면 낡음 배지가 보이고, [그대로 유지]를 누르면 배지가 사라진다.
- [ ] 초점과 씬 의도를 모두 비우면 브리프에서 섹션이 사라진다.
- [ ] Settings → Backup에서 그 작품을 복원하면 초점과 씬 의도가 따라온다.

**완료 처리**
1. 위 항목이 모두 통과하면 사용자에게 보고한다: 완료 작업 요약, 테스트 결과, 수동 확인 결과(확인하지 못한 항목은 그렇다고 적는다).
2. 브랜치 `feat/story-assist-phase-1-direction`을 푸시하고 PR을 연다. 머지는 사용자 승인 뒤에 한다.
3. 실패 항목이 있으면 보고한다. 원인을 사용자와 함께 파악하고, 수정한 뒤 다시 검증한다.
