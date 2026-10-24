# Phase 2 — 연속성 점검: 근거가 검증된 관찰 기록

_작성일: 2026-10-24_
_상위 로드맵: [`roadmap.md`](./roadmap.md)_
_근거: [`inkos-analysis.md`](./inkos-analysis.md) §1_
_선행 조건: Phase 1 완료 (씬 의도의 "피할 것"을 점검 기준으로 씀)_
_예상 소요: 2~3일_

## Goal

작가가 에이전트에게 "이 씬 연속성 점검"을 부탁하는 흐름은 다음과 같다.

1. 에이전트가 번호 붙은 문단으로 씬을 읽는다.
2. 브리프(Story World, 팩트, 작가 방향)와 앞 회차를 대조한다.
3. 어긋나는 대목을 `linetta_report_continuity`로 기록한다.

엔진은 기록하기 전에 확인한다. 인용이 그 문단에 실제로 있는지, 참조한 인물·팩트·비트 id가 이 작품에 있는지 본다. 틀리면 도구 오류로 돌려보내 에이전트가 고치게 한다.

작가는 Story World의 "연속성 점검"에서 관찰을 본다. 문단으로 이동하거나 무시·해결 표시를 할 수 있다. **원고는 한 글자도 바뀌지 않는다.**

## 이미 있는 것과의 경계

| | 맥락 편집 일관성 재검사 (`contextualedit.CheckAfterChange`) | 이 페이즈 |
|---|---|---|
| 판단 | 결정적 (남은 옛 이름, 낡은 메타데이터) | 에이전트의 의미 판단 |
| 시점 | 이름·설정을 바꾼 직후 | 작가가 부탁할 때 |
| 결과 | 바로 보여주는 보고서 | 저장되는 관찰 (상태와 낡음 판정이 있음) |

둘은 대체 관계가 아니다. 이 페이즈는 맥락 편집 코드를 고치지 않는다.

## InkOS에서 가져오는 것 / 다르게 하는 것

- **가져옴: 인용 검증과 재시도.**
  - InkOS는 모델이 줄 번호를 고르고 호스트가 원문을 잘라 인용을 만든다.
  - 리네타는 기존 문단 번호 관례(`style/scenes.go` `sceneOf`)를 쓴다. 모델은 문단 번호와 인용을 함께 보내고, 엔진은 정규화한 인용이 그 문단 범위의 부분 문자열인지 확인한다.
  - 줄 번호보다 문단 번호가 작가에게 익숙하다. 문체 점검도 이미 "문단"으로 보고한다.
- **가져옴: 빈 결과도 유효한 결과.**
  - 문제가 없으면 관찰 0개로 기록한다. "점검했고 문제 없음"이 남는다.
- **가져옴: 취향은 관찰이 아니다.**
  - "Separate objective contradiction from taste preference." 원칙을 도구 설명과 에이전트 지침에 명시한다.
- **추가: 심각도, 분류, 수정 제안, 참조 id.**
  - InkOS 스키마에 없는 필드다.
  - 참조 id는 엔진이 검증하므로 화면에서 눌러 해당 인물이나 팩트를 열 수 있다.
- **추가: 낡음.**
  - 기록 시점의 `content_version`을 저장한다. 본문이 바뀌면 "본문이 바뀜 · 다시 점검"으로 표시한다.
- **안 함: 자동 수정과 수정 범위 게이트.**
  - InkOS의 revision-scope 장치는 AI가 원고를 고칠 때만 의미가 있다.

## Non-goals

- 에디터 여백에 관찰 표시 → Next (MVP는 목록과 씬 열기)
- 작품 전체 일괄 점검 작업 큐. 에이전트가 한 턴에 여러 씬을 점검하는 것은 도구 호출 한도(`agent_max_tool_calls`) 안에서만 한다.
- 떡밥 장부, 시점별 사실 → Later
- 같은 관찰이 다시 올라올 때 "이전에 무시함" 매칭 → Next

## Touch points

- `engine/internal/store/migrations/0025_continuity.sql` 신규
- `engine/internal/continuity/` 신규 패키지
- `engine/internal/style/scenes.go` (문단 분할 공개)
- `engine/internal/mcphost/tools_read.go` (`readSceneInput.Numbered`), `tools_continuity.go` 신규, `budget.go`, `tools.go`
- `engine/internal/settings/settings.go` (`review_tools_enabled`)
- `engine/internal/agent/prompt.go`
- `engine/internal/rpc/handlers/continuity.go` 신규, `engine/internal/engineapp/engineapp.go`
- `engine/internal/restore/restore.go`
- `apps/desktop/src-tauri/src/lib.rs`, `src/lib/rpc.ts`, `lib/types.ts`, `lib/agentTools.ts`, `lib/i18n.tsx`
- `apps/desktop/src/components/ContinuitySection.tsx` 신규, `CanonPanel.tsx`, `ContextPanel.tsx`, `agent/AgentPanel.tsx`, `routes/Settings.tsx`

## 데이터 모델

```sql
-- 0025_continuity.sql
-- An agent's continuity reading of one scene (story-assist Phase 2). Nothing
-- here is canon and nothing here changes the manuscript: it is a list of
-- places the writer may want to look at, each tied to the exact text and the
-- scene version it was read from.
CREATE TABLE continuity_checks (
  id              TEXT PRIMARY KEY,
  project_id      TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  node_id         TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  content_version INTEGER NOT NULL,
  source          TEXT NOT NULL,      -- "agent" | "external"
  note            TEXT NOT NULL DEFAULT '',
  checked_at      INTEGER NOT NULL
);
CREATE INDEX continuity_checks_node ON continuity_checks(node_id, checked_at);

CREATE TABLE continuity_observations (
  id             TEXT PRIMARY KEY,
  check_id       TEXT NOT NULL REFERENCES continuity_checks(id) ON DELETE CASCADE,
  project_id     TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  node_id        TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  category       TEXT NOT NULL,   -- character|place|item|relationship|timeline|fact|plot|direction|other
  kind           TEXT NOT NULL,   -- contradiction|gap|question
  severity       TEXT NOT NULL,   -- high|medium|low
  summary        TEXT NOT NULL,
  suggestion     TEXT NOT NULL DEFAULT '',
  paragraph_from INTEGER NOT NULL,
  paragraph_to   INTEGER NOT NULL,
  quote          TEXT NOT NULL,
  refs_json      TEXT NOT NULL DEFAULT '[]',  -- [{type:entity|fact|beat|scene, id, paragraph?, quote?}]
  status         TEXT NOT NULL DEFAULT 'open', -- open|dismissed|resolved|replaced
  status_at      INTEGER NOT NULL,
  created_at     INTEGER NOT NULL
);
CREATE INDEX continuity_obs_project ON continuity_observations(project_id, status);
```

- `kind`의 값
  - `contradiction`: 원고가 작품의 기록(Story World, 팩트, 앞 씬)과 맞지 않는다.
  - `gap`: 원고가 기록에 없는 새 사실을 세운다. 등록할지는 작가가 판단한다. Phase 3 이후에는 제안함으로 이어질 수 있다.
  - `question`: 판단이 필요한 모호한 대목이다.
- `category = direction`: 씬 의도의 "피할 것"이나 "지킬 것", 또는 현재 초점과 맞지 않는 대목이다.
- `replaced`: 같은 씬에 새 점검이 기록되면, 이전 점검의 `open` 관찰이 이 상태가 된다.
  - 작가가 `dismissed`나 `resolved`로 표시한 것은 그대로 둔다.

## 작업 체크리스트

### A. 엔진

- [ ] **2.A.1 문단 분할 공개**
  - 파일: `engine/internal/style/scenes.go`, `style_test.go`
  - 내용:
    - `sceneOf`가 쓰는 분할(`export.DocToPlainText` → `"\n\n"`, 비지 않은 문단만, 1부터)을 `func Paragraphs(contentDoc string) []string`로 공개한다.
    - `sceneOf`도 이 함수를 쓰게 한다.
    - **문체 점검과 연속성 점검의 문단 번호가 반드시 같아야 한다.**
  - 테스트: 기존 문체 점검 테스트가 그대로 통과한다. 빈 문단과 mention 노드도 테스트한다.

- [ ] **2.A.2 `continuity` 패키지: 타입과 검증**
  - 파일: `engine/internal/continuity/continuity.go`, `validate.go`, `validate_test.go`
  - 타입:
    - `Observation`, `Ref{Type, ID string; Paragraph int; Quote string}`, `Check`, `ReportInput`, `Report`
    - enum 검증 함수: `ValidCategory`, `ValidKind`, `ValidSeverity`, `ValidStatus` (`fact.ValidStatus` 관례)
  - 한도:
    - 보고 1회에 관찰 20개
    - summary 300자, suggestion 300자, quote 300자
    - ref는 관찰 1개당 5개
  - 검증 함수:
    ```go
    // Validate checks every observation against the scene it cites and the work's
    // own record. It returns ALL problems at once, each phrased so a model can fix
    // its next call — the tool layer hands them back verbatim.
    func Validate(in ReportInput, paragraphs []string, lookup RefLookup) []Problem
    ```
  - 규칙:
    - `paragraph_from`과 `paragraph_to`는 1 이상, `from ≤ to ≤ len(paragraphs)`이어야 한다.
      - 틀리면 `"observation 3: paragraph 41 is out of range; the scene has 37 paragraphs"`.
    - `quote` 비교는 양쪽을 NFC 정규화하고 연속 공백을 하나로 접은 뒤 한다. `from..to` 문단을 `"\n"`으로 이은 텍스트의 부분 문자열이어야 한다.
      - 틀리면 `"observation 3: quote not found in paragraphs 12–13; copy it exactly from the numbered text or drop the observation"`.
    - `refs[].type == entity / fact / beat`: `RefLookup`으로 이 작품에 있는지 확인한다.
      - 모르는 id면 어떤 도구로 목록을 볼 수 있는지 알려준다(`linetta_list_characters`, `linetta_get_fact_cards`, `linetta_get_plot`).
    - `refs[].type == scene`: 같은 작품의 leaf인지 확인한다. `quote`가 있으면 그 씬의 문단에서도 같은 규칙으로 확인한다.
      - 앞 회차 인용도 지어낼 수 없게 하기 위해서다.
    - `kind == contradiction`이면 ref가 하나 이상 있어야 한다. "무엇과 모순인지"를 가리켜야 한다.
  - 테스트 (table-driven):
    - 정상
    - 문단 범위 밖
    - 공백만 다른 인용은 통과
    - 지어낸 인용은 거부
    - 다른 작품의 entity id는 거부
    - 근거 없는 contradiction은 거부
    - 오류 여러 개를 한 번에 반환

- [ ] **2.A.3 Repo**
  - 파일: `engine/internal/continuity/repo.go`, `repo_test.go`
  - 함수:
    ```go
    func (r *Repo) Record(ctx context.Context, now int64, in ReportInput, source string) (Check, error) // 한 트랜잭션
    func (r *Repo) List(ctx context.Context, f ListFilter) ([]CheckWithObservations, error)           // project_id, node_id?, statuses
    func (r *Repo) SetStatus(ctx context.Context, now int64, observationID, status string) (Observation, error)
    func (r *Repo) OpenCounts(ctx context.Context, projectID string) (map[string]int, error)         // node_id → 열린 관찰 수
    ```
  - 규칙:
    - `Record`는 같은 node의 이전 `open` 관찰을 `replaced`로 바꾸고 새 check와 관찰을 넣는다. 한 트랜잭션에서 처리한다.
    - `List`의 각 check에 `stale = (node.content_version != check.content_version)`을 계산해 붙인다.
    - `SetStatus`는 `open`, `dismissed`, `resolved` 사이만 옮긴다. `replaced`로는 옮길 수 없다.
  - 테스트:
    - 교체 규칙
    - dismissed는 교체되지 않음
    - 낡음 계산
    - node를 지우면 cascade로 지워짐

### B. MCP

- [ ] **2.B.1 `linetta_read_scene`에 `numbered` 옵션**
  - 파일: `engine/internal/mcphost/tools_read.go`, `tools_read_test.go`
  - 내용:
    - `Numbered bool` 필드. 설명: `"plain text with each paragraph prefixed [n], counted the way linetta_analyze_style and linetta_report_continuity count them"`
    - `numbered`와 `format=markdown`을 같이 주면 도구 오류를 낸다.
    - 출력의 `Text`는 `"[1] …\n\n[2] …"` 형식이다. `ContentVersion`은 이미 출력에 있다.
  - **새 도구를 만들지 않는 이유**: 읽기 도구를 하나 더 두면 상시 비용이 늘어난다. 기존 도구의 옵션으로 충분하다.

- [ ] **2.B.2 `linetta_report_continuity` (쓰기 도구, Review 그룹)**
  - 파일: `engine/internal/mcphost/tools_continuity.go` 신규, `tools_continuity_test.go`
  - 입력:
    - `project_id?`, `node_id`
    - `scene_version`: 읽을 때의 `content_version`
    - `observations[]`, `note?`
  - 동작:
    1. `requireNodeInProject`로 대상을 확인한다.
    2. `scene_version`이 현재 버전과 다르면 다음 오류를 낸다.
       `"the scene changed since you read it (version N, now M); read it again with numbered=true"`
    3. `continuity.Validate`가 낸 문제를 모두 담아 `toolErr`로 돌려보낸다. 모델이 한 번에 고치게 하기 위해서다.
    4. `Record`. source는 `d.sourceOrExternal()`이다.
    5. `notifyChanged(projectID, "linetta_report_continuity", []string{nodeID}, "")`
  - 출력: `{check_id, recorded, status: "waiting_for_writer"}`
  - 설명문 (상시 비용이므로 짧게):
    > Record what you found checking one scene against the work's own record (Story World, fact cards, earlier scenes, the writer's direction). Cite paragraphs from linetta_read_scene numbered=true and copy quotes exactly; every contradiction must point at what it contradicts. Taste is not a finding. An empty list records a clean check. The writer reviews these; nothing in the manuscript changes.
  - 등록: `registerWriteTools`에서 full 모드일 때만 등록하고, Review 그룹이 꺼져 있으면 빼는 경로를 탄다(`excludedByGroups`).

- [ ] **2.B.3 Review 도구 그룹**
  - 파일: `engine/internal/mcphost/budget.go`, `budget_test.go`, `engine/internal/settings/settings.go`
  - 내용:
    - `ToolGroups.Review bool`
    - `ReviewToolNames = []string{"linetta_report_continuity"}`. Phase 3과 4가 이 목록에 도구를 더한다.
    - `AllToolGroups()`에 `Review: true`를 넣는다.
    - `excludedByGroups`에 분기를 더한다.
    - 설정 `review_tools_enabled`(기본값 true), `Store.ReviewToolsEnabled()`, `ToolGroupsFrom`에 연결한다.
    - Tool budget 보고(`budget.go:165` 이하)에 그룹별 비용을 추가한다.
  - 에이전트 쪽: `agent/prompt.go`의 `toolGroups`와 `resolveToolGroups`에 `review`를 추가한다. 세션 캐시 키에 같은 값이 들어가는지 확인한다(`agent.Service.session` 주석 참고).
  - 테스트:
    - `TestToolGroupNamesAreAllRealTools` 통과
    - Review를 끄면 tools/list에서 빠짐
    - read_only에서는 원래 없음

### C. RPC, 복원, 에이전트

- [ ] **2.C.1 RPC**
  - 파일: `engine/internal/rpc/handlers/continuity.go`, `continuity_test.go`
  - 메서드:
    - `continuity.list {project_id, node_id?, statuses?}` → checks와 관찰
      - 각 관찰에 breadcrumb(씬 경로)과 ref 표시 이름(인물 이름, 팩트 claim)을 붙인다. 화면이 따로 조회하지 않게 하기 위해서다.
    - `continuity.set_status {observation_id, status}`
    - `continuity.open_counts {project_id}`
  - 등록: `engineapp.go`, `lib.rs` `RENDERER_ENGINE_METHODS`

- [ ] **2.C.2 작품 단위 복원**
  - 파일: `engine/internal/restore/restore.go`
  - 내용:
    - `continuity_checks`와 `continuity_observations`를 node와 project 재매핑과 함께 복사한다.
    - `refs_json` 안의 scene id도 재매핑한다. entity, fact, beat id는 복원 때 새 id가 되므로, 매핑 표가 있으면 그것으로 바꾸고 없으면 그 ref를 버린다.
    - 상태(`open` / `dismissed` / `resolved` / `replaced`)와 `content_version`은 그대로 옮긴다. 복원된 씬의 버전이 같으면 낡음 판정도 그대로 유지된다.
    - 함수 머리 주석(`restore.go:181`)의 테이블 목록에 두 테이블을 추가한다.
  - **정책 확정 (2026-10-24, 사용자 결정): 관찰은 작품 단위 복원에 포함한다.**
    - 작가가 무시하거나 해결로 표시한 판단도 작업 기록이다. 복원했는데 그 기록이 사라지면, 같은 관찰을 다시 검토해야 한다.
    - 제안함(Phase 3)과 갈래 예측(Phase 4)을 복원에서 빼는 것과는 사정이 다르다. 관찰은 씬과 원고 인용에 묶여 있어 재매핑 뒤에도 뜻이 유지된다.
  - 테스트: 백업에서 복원한 작품에 관찰과 상태가 따라온다. 매핑에 없는 entity ref는 버려지고, 나머지 ref는 남는다.

- [ ] **2.C.3 에이전트 지침과 시작 칩**
  - `engine/internal/agent/prompt.go`: Review 그룹이 켜져 있을 때만 다음 줄을 넣는다.
    > To check a scene's continuity, read it with linetta_read_scene numbered=true, get the brief, look earlier scenes up with linetta_where_does_appear / linetta_search_manuscript, and record what you find with linetta_report_continuity. Report contradictions with the work's own record, not taste; an empty result is fine. Do not fix the prose unless the writer asks.
  - `apps/desktop/src/components/agent/AgentPanel.tsx`: starter 칩 `checkContinuity`를 추가한다.
    - 문구 (ko): "지금 씬을 Story World와 앞 회차에 대조해 연속성을 점검하고 결과를 기록해 줘. 원고는 고치지 마."
    - Review 그룹이 꺼져 있으면 칩을 숨긴다.

### D. 화면

- [ ] **2.D.1 `ContinuitySection`**
  - 파일: `apps/desktop/src/components/ContinuitySection.tsx`, `.css`, `.test.tsx`
  - props: `{ projectId; nodeId?; onOpenNode?: (nodeId) => void; onOpenEntity?; onOpenFact? }`
  - 화면:
    - 범위 토글 `현재 씬 / 작품 전체`. `StyleSection`의 Scope 패턴을 쓴다.
    - 상태 필터 `열림 / 무시 / 해결`
    - 씬별로 묶인 행:
      - 심각도 점
      - 분류 라벨
      - 요약
      - `¶12–13 "인용…"`
      - ref 칩 (누르면 EntitySheet나 FactBook으로 이동)
      - 제안
      - [씬 열기] [무시] [해결됨]
    - check가 낡았으면 묶음 머리에 "본문이 바뀜 · 다시 점검" 배지를 단다.
    - 관찰 0개인 최근 점검은 "N분 전 점검 · 문제 없음"으로 보여준다.
    - 실시간 갱신: `useEngineEvent`에서 `mcp.changed`의 tool이 `linetta_report_continuity`이면 다시 불러온다. `StyleSection.tsx:63-67` 패턴이다.
  - 테스트:
    - 목록
    - 상태 변경
    - 낡음 배지
    - 빈 점검 표시
    - 이벤트 수신 시 다시 불러오기

- [ ] **2.D.2 배치**
  - `components/CanonPanel.tsx`(Story World): `StyleSection` 아래에 `ContinuitySection`을 둔다.
  - `components/ContextPanel.tsx`: 현재 씬의 열린 관찰 수 배지를 단다. 누르면 Story World의 해당 섹션을 연다(현재 씬 범위).
  - `lib/agentTools.ts`: `WRITE_TOOL_NAMES`에 `linetta_report_continuity`와 라벨 키를 추가한다. `agentToolParity.test.ts`가 Go 목록과 대조한다.
  - `routes/Settings.tsx` Tool budget: "점검·제안" 스위치를 추가한다. 끄면 무엇이 빠지는지 한 줄로 설명한다.

- [ ] **2.D.3 다국어**
  - 파일: `apps/desktop/src/lib/i18n.tsx`
  - 키:
    - `continuity.title`, `continuity.scope.*`, `continuity.status.*`
    - `continuity.category.*`, `continuity.kind.*`, `continuity.severity.*`
    - `continuity.stale`, `continuity.clean`
    - `continuity.action.open`, `continuity.action.dismiss`, `continuity.action.resolve`
    - `agentPanel.starters.checkContinuity.*`
    - 도구 라벨, Tool budget 그룹 문구

- [ ] **2.D.4 문서**
  - `README.md`: 새 절 "Continuity checks"를 쓴다. 담을 내용:
    - 무엇을 기록하는가
    - 엔진이 인용을 검증한다
    - 원고는 바뀌지 않는다
    - 본문이 바뀌면 낡음 표시
    - Tool budget 스위치
  - `docs/privacy-policy.md`: 관찰은 `library.db`에 로컬 저장된다는 점

---

## ✅ Phase 2 Checkpoint

**자동 검증**
- [ ] `cd engine && CGO_ENABLED=0 go test ./internal/continuity/... ./internal/style/... ./internal/mcphost/... ./internal/settings/... ./internal/rpc/handlers/... ./internal/restore/... ./internal/agent/...`
- [ ] `make test-mobile-engine`
- [ ] `pnpm --dir apps/desktop test -- ContinuitySection.test.tsx CanonPanel.test.tsx ContextPanel.test.tsx AgentPanel.test.tsx agentToolParity.test.ts rpcAllowlist.test.ts i18n.catalog.test.ts --run`
- [ ] `pnpm --dir apps/desktop exec tsc --noEmit`
- [ ] `make test`

**수동 검증** (격리된 `LINETTA_HOME`, 프로바이더 하나 연결)
- [ ] 인물 "하린"의 속성에 "왼손잡이"를 넣는다. 씬 본문에는 "하린은 오른손으로 검을 뽑았다"를 쓴다.
- [ ] starter 칩 "연속성 점검"을 실행한다. 다음을 확인한다.
  - 관찰 1건이 Story World에 뜬다.
  - 인용이 본문 문장과 같다.
  - "하린" ref 칩을 누르면 EntitySheet가 열린다.
- [ ] 활동 로그에서 `linetta_report_continuity`가 실패 후 재시도된 기록이 있으면 오류 문구를 읽어 본다. 모델이 고칠 수 있는 문장이어야 한다.
- [ ] 본문을 한 글자 고치면 "본문이 바뀜" 배지가 뜬다.
- [ ] [무시]한 관찰은 다시 점검해도 무시 상태로 남는다. 열린 관찰은 "교체됨"으로 숨겨진다.
- [ ] Settings → Tool budget에서 "점검·제안"을 끄면 다음 메시지부터 도구와 칩이 사라진다.
- [ ] Settings → Backup에서 그 작품을 복원하면 관찰과 상태(무시·해결)가 따라오고, 인물 ref 칩이 복원된 인물을 연다.
- [ ] 위 과정 전후로 씬 본문의 `content_version`을 활동 로그나 VersionSheet에서 확인한다. 점검 도구가 본문을 바꾸지 않았어야 한다.

**완료 처리**
1. 결과를 보고한다. 확인하지 못한 항목은 그렇다고 적는다.
2. `feat/story-assist-phase-2-continuity` 브랜치를 푸시하고 PR을 연다. 머지는 사용자 승인 뒤에 한다.
