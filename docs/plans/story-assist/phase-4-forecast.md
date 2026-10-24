# Phase 4 — 갈래 예측: 비정사 전개 비교

_작성일: 2026-10-24_
_상위 로드맵: [`roadmap.md`](./roadmap.md)_
_근거: [`inkos-analysis.md`](./inkos-analysis.md) §3_
_선행 조건: Phase 1 (방향, `episode.LastWritten`), Phase 3 (제안함, Review 그룹) 완료_
_예상 소요: 2~3일_

## Goal

작가가 "다음 회차 갈래 3개 보여줘"라고 하면, 에이전트가 브리프를 바탕으로 2~5개의 전개를 만들어 `linetta_save_forecast`로 저장한다.
엔진은 갈래 수, 회차 수, 참조 id를 검증한다. 그리고 그 시점 정사의 **입력 지문**을 함께 저장한다.

작가는 갈래 예측 sheet에서 카드를 나란히 비교한다.
- **고르기는 앱 RPC로 바로 한다.** LLM 턴을 쓰지 않는다.
- 고른 갈래는 브리프의 "작가 방향" 섹션에 **계획(정사 아님)**으로 들어간다. 이후 에이전트가 따른다.
- 개요에 반영하고 싶으면 [개요 반영 제안 요청]을 누른다. 에이전트가 장면과 비트 후보를 제안함(Phase 3)에 올린다.

정사(개요, 본문, Story World, 팩트, 방향)가 바뀌면 예측이 낡았다고 표시한다. 무엇이 바뀌었는지도 함께 보여준다. 예: "인물 2개, 3화 본문"

## InkOS에서 가져오는 것 / 더 잘하는 것

| | InkOS | 리네타 |
|---|---|---|
| 정사 보호 | 별도 디렉터리 + 전후 스냅샷 테스트 | 별도 테이블 + 정사 테이블 전후 해시 비교 테스트 |
| id | 러너가 `branch-1..N` 부여 | 엔진이 `b1..bN` 부여. 인물·팩트 참조는 실제 id로 검증 |
| 범위 | 2~5, 1~10은 도구 설명에만 있음 | **코드에서 강제** |
| 지문 | 파일 내용 SHA-256 하나. **본문은 빠짐** | 묶음별 해시. **씬 `content_version`을 포함**. 낡은 이유를 표시 |
| 낡음 판정 | 조회·선택 때만, 한 번 낡으면 되돌릴 수 없음 | 목록을 열 때마다 다시 계산. 정사가 되돌아가면 다시 유효 |
| 선택 | 채팅 메시지 → LLM 턴 | 직접 RPC |
| 선택 결과 | 아무 코드도 읽지 않는 Markdown | 브리프에 계획으로 들어가고, 개요 반영은 제안함으로 |
| 생성 격리 | 호출 한 번, 프롬프트로만 격리 | 같음 (MVP). 갈래별로 따로 호출하는 방식은 Later |

## Non-goals

- 갈래별 별도 LLM 호출(진짜 격리) → Later
- 엔진이 갈래를 개요 op로 자동 변환하는 것. 개요 구조 판단은 에이전트가 하고, 결과는 제안함을 거친다.
- 예측 이력 비교, 갈래끼리의 diff

## Touch points

- `engine/internal/store/migrations/0027_forecasts.sql` 신규
- `engine/internal/forecast/` 신규 패키지
- `engine/internal/episode/episode.go` (`LastWritten`, Phase 1에서 추가된 것을 사용)
- `engine/internal/proposals/submit.go` (purpose=forecast 허용 op 확장)
- `engine/internal/storycontext/types.go`, `render.go` (`DirectionBrief.PlannedBranch`)
- `engine/internal/mcphost/tools_forecast.go` 신규, `budget.go`
- `engine/internal/agent/prompt.go`
- `engine/internal/rpc/handlers/forecast.go` 신규, `engineapp.go`, `restore/restore.go` (제외 주석)
- `apps/desktop/src-tauri/src/lib.rs`, `src/lib/rpc.ts`, `lib/types.ts`, `lib/agentTools.ts`, `lib/i18n.tsx`
- `apps/desktop/src/components/ForecastSheet.tsx` 신규, `routes/Workspace.tsx`, `agent/AgentPanel.tsx`

## 데이터 모델

```sql
-- 0027_forecasts.sql
-- Possible continuations an agent drafted for the writer to compare
-- (story-assist Phase 4). Never canon: nothing that builds the outline, the
-- manuscript or Story World reads this table. The writer's chosen branch
-- reaches an agent only as a labelled plan in the brief's direction section.
CREATE TABLE forecasts (
  id                TEXT PRIMARY KEY,
  project_id        TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  divergence        TEXT NOT NULL,      -- the question the writer asked
  base_node_id      TEXT,               -- last written episode at save time
  horizon           INTEGER NOT NULL,   -- episodes ahead, 1..10
  branches_json     TEXT NOT NULL,
  fingerprint       TEXT NOT NULL,      -- sha256 over input_hashes
  input_hashes_json TEXT NOT NULL,      -- {group: hash, episodes: {node_id: hash}}
  source            TEXT NOT NULL,
  selected_branch   TEXT,               -- b1..bN
  selected_at       INTEGER,
  created_at        INTEGER NOT NULL
);
CREATE INDEX forecasts_project ON forecasts(project_id, created_at);
```

갈래 하나(`branches_json`의 원소). InkOS `ForecastBranchSchema`를 리네타 구조에 맞춘 형태다.

```go
type Branch struct {
    ID          string           `json:"id"`           // engine-assigned b1..bN
    Title       string           `json:"title"`        // ≤ 80
    Premise     string           `json:"premise"`      // ≤ 600
    Beats       []BranchBeat     `json:"beats"`        // 1..10, episode_offset 1..horizon
    Decisions   []CharDecision   `json:"decisions"`    // ≤ 6, entity_id validated
    Changes     ProjectedChanges `json:"changes"`      // characters/relationships/world/threads, ≤ 5 each
    Risks       []Risk           `json:"risks"`        // ≤ 5, kind + optional entity_id/fact_id/node_id validated
    Unknowns    []string         `json:"unknowns"`     // ≤ 5
    DirectionFit string          `json:"direction_fit"`// ≤ 300 — how it serves the writer's focus and intent
}
```

`Risk.kind`: `continuity` | `causality` | `character` | `direction` | `pacing` | `other`

## 작업 체크리스트

### A. 엔진

- [ ] **4.A.1 입력 지문**
  - 파일: `engine/internal/forecast/fingerprint.go`, `fingerprint_test.go`
  - 함수:
    ```go
    // Inputs hashes exactly what a forecast was drawn from, group by group, so a
    // stale forecast can say what changed. No timestamps: the same canon gives the
    // same hashes on any machine. The forecasts table itself is never an input.
    func Inputs(ctx context.Context, deps Deps, projectID string) (InputHashes, error)
    func Diff(old, now InputHashes) []StaleReason // {group, count, episode_labels?}
    ```
  - 묶음(group)과 각 묶음의 내용. 각 묶음은 id 정렬 뒤 SHA-256으로 해시한다.
    - `project`: synopsis, outline(플롯 개요), style_notes
    - `outline`: 모든 node의 id, parent, ordinal, kind, label, title, summary
    - `episodes`: 회차별 해시 맵 `{episode node_id: hash(leaf id + content_version …)}`
      - **본문 수정을 잡는다.** InkOS에는 없는 부분이다.
    - `entities`: id, kind, name, aliases, role, summary, attributes
    - `relationships`, `facts`(claim, result, status), `plot`(thread, beat)
    - `direction`: Phase 1의 초점과 씬 의도
  - 테스트:
    - 같은 데이터면 같은 해시가 나온다(순서와 무관).
    - 본문을 저장하면 `episodes`의 해당 회차만 바뀐다.
    - 인물을 수정하면 `entities`가 바뀐다.
    - forecasts 테이블에 써도 해시가 그대로다.
    - 정사를 원래대로 되돌리면 해시도 원래대로 돌아온다.

- [ ] **4.A.2 검증과 저장**
  - 파일: `engine/internal/forecast/forecast.go`, `validate.go`, `repo.go`, `*_test.go`
  - 검증 규칙
    - 갈래 2~5개, horizon 1~10
    - 각 필드 길이 한도 (위 타입 주석)
    - `Beat.episode_offset`은 1..horizon
    - decision과 risk의 entity_id, fact_id, node_id가 이 작품에 있는지 확인한다.
    - 문제는 모두 모아서 돌려준다 (Phase 2·3과 같은 방식).
  - 저장 시 엔진이 하는 일
    - 갈래에 `b1..bN`을 붙인다.
    - `base_node_id = episode.LastWritten(...)`
    - `Inputs`와 `fingerprint`를 계산한다.
  - Repo:
    - `Save`
    - `List(projectID)`: 각 예측에 현재 지문과 비교한 `stale`과 `reasons`를 붙인다. **목록을 열 때마다 다시 계산한다.**
    - `Select(id, branchID)`, `ClearSelection(projectID)`, `Delete(id)`
    - `Selected(projectID)`: 가장 최근에 선택된 예측 1개
  - 선택은 작품당 하나만 둔다. 새로 고르면 이전 선택을 지운다.

- [ ] **4.A.3 정사 보호 테스트**
  - 파일: `engine/internal/forecast/canon_test.go`
  - 내용: InkOS `forecast-fixture.ts`의 리네타판이다.
    - 정사 테이블과 제안함 테이블을 행 단위 해시로 덤프한다: projects, nodes, entities, relationships, fact_cards, threads, beats, project_focus, scene_intents, story_proposal_items.
    - `Save → List → Select → ClearSelection → Delete`를 실행한 전후로 덤프가 같은지 확인한다.

- [ ] **4.A.4 브리프에 고른 갈래**
  - 파일: `engine/internal/storycontext/types.go`, `render.go`, `render_test.go`. 소스 주입은 `DirectionSource`를 확장한다.
  - 내용:
    - `DirectionBrief.PlannedBranch *PlannedBranchBrief{Title, Premise string; Beats []string; Stale bool; StaleReasons []string}`
    - 렌더링 위치: "작가 방향" 섹션 안, 씬 의도 뒤
      - 소제목: `작가가 고른 다음 전개 (계획 — 정사 아님, 쓰면서 달라질 수 있음)`
      - 낡았으면 `정사가 바뀐 뒤의 계획: (사유). 따르기 전에 작가에게 확인`을 붙인다.
    - 새 ContextKey를 만들지 않는다. 작가가 고른 방향이므로 direction 스위치를 같이 쓴다.
  - 테스트:
    - 선택이 없으면 없음
    - 낡음 문구
    - direction이 꺼지면 함께 빠짐

- [ ] **4.A.5 제안함 확장: purpose=forecast**
  - 파일: `engine/internal/proposals/submit.go`, `accept.go`, 테스트
  - 내용:
    - purpose=forecast일 때만 `create_outline_node`, `create_scene`(본문 없음), `add_beat`를 허용한다.
    - 입력에 `forecast_id`가 반드시 있어야 하고, 그 예측에 선택된 갈래가 있어야 한다.
    - 수락할 때 구조 op(`create_outline_node`, `create_scene`)는 **한 번의 `ApplyOpsWithRefs` 호출로** 묶는다.
      - 그래야 storyops의 "구조 묶음은 전부 아니면 전무 + undo batch"가 유지된다.
      - 결과의 `undo_batch_id`를 화면에 넘겨 되돌리기 버튼을 단다.

### B. MCP와 에이전트

- [ ] **4.B.1 `linetta_save_forecast` (쓰기 도구, Review 그룹)**
  - 파일: `engine/internal/mcphost/tools_forecast.go`, `tools_forecast_test.go`, `budget.go` (`ReviewToolNames`에 추가)
  - 입력: `{project_id, divergence, horizon, branches[]}`. 갈래에 id 필드는 없다.
  - 출력: `{forecast_id, branch_ids, base_episode_label, status: "waiting_for_writer"}`
  - 설명문:
    > Save 2–5 possible continuations from the latest written episode for the writer to compare. Not canon: the outline, manuscript and Story World do not change, and the writer picks a branch (or none) in the app. Draw every branch from the brief; put conflicts with the work's record under risks, citing the entity or fact.
  - 읽기 도구는 따로 두지 않는다. 고른 갈래는 브리프로 전달된다. 다른 갈래를 다시 볼 필요가 생기면 Next에서 판단한다.

- [ ] **4.B.2 에이전트 지침과 시작 칩**
  - `agent/prompt.go`: Review 그룹이 켜져 있을 때만 넣는다.
    > Asked where the story could go next, read the brief and save the options with linetta_save_forecast instead of only describing them; the writer compares and picks in the app. A chosen branch appears in the brief's direction section as a plan, not canon. Asked to put a chosen branch into the outline, propose it with linetta_propose_story_ops purpose=forecast.
  - `AgentPanel.tsx` starter 칩:
    - `forecastNext`: "다음 회차 전개 갈래 3개를 만들어 비교할 수 있게 저장해 줘"
    - [개요 반영 제안 요청] 버튼이 채우는 문구: "고른 갈래 '{title}'을 개요에 반영하는 제안을 만들어 줘(purpose=forecast, forecast_id={id}). 적용은 내가 할게."

### C. RPC와 화면

- [ ] **4.C.1 RPC**
  - 파일: `engine/internal/rpc/handlers/forecast.go`, `forecast_test.go`
  - 메서드:
    - `forecast.list {project_id}` → 예측 목록(stale, reasons, 기준 회차 라벨, 갈래)
      - ref 표시 이름(인물 이름, 팩트 claim)도 붙인다.
    - `forecast.select {forecast_id, branch_id}`
    - `forecast.clear_selection {project_id}`
    - `forecast.delete {forecast_id}`
  - 등록: `engineapp.go`, `lib.rs` `RENDERER_ENGINE_METHODS`
  - 낡은 예측도 고를 수 있다. 다만 RPC 응답과 화면 양쪽에 경고를 보인다. **정책을 한 곳(엔진)에 둔다.** InkOS처럼 코어는 허용하고 UI만 막는 식으로 갈라지지 않게 한다.

- [ ] **4.C.2 `ForecastSheet`**
  - 파일: `apps/desktop/src/components/ForecastSheet.tsx`, `.css`, `.test.tsx`
  - 화면:
    - 예측 목록(최신순). 각 예측의 머리:
      - divergence
      - "N화 이후 · 갈래 K개 · 약 H회차"
      - `정사 아님` 배지
      - 낡았으면 주황 배너: "정사가 바뀜: 인물 2개, 3화 본문"
    - 갈래 카드: 가로 스크롤 그리드. 좁은 폭에서는 세로로 쌓는다.
      - 제목, 전제
      - 회차별 비트 타임라인
      - 위험 배지 (종류별 라벨, 설명은 툴팁, 참조 칩은 누르면 인물이나 팩트 열기)
      - 접힌 상세: 인물 결정, 예상 변화, 불확실한 점, 방향 부합 설명
      - [이 갈래 고르기] / 고른 상태면 [선택 해제]
      - [개요 반영 제안 요청]: 고른 갈래에만 표시
    - [삭제]: 예측 단위
    - 실시간 갱신: `mcp.changed`의 tool이 `linetta_save_forecast`일 때
  - 진입: Workspace 커맨드 팔레트 "갈래 예측 열기", 개요 패널 머리의 버튼
  - 테스트:
    - 카드 렌더
    - 고르기와 해제 (RPC 호출 확인, LLM 경로 없음)
    - 낡음 배너와 사유
    - 개요 반영 요청 시 패널 입력이 채워짐

- [ ] **4.C.3 복원 정책, 다국어, 문서**
  - `restore.go`: 예측은 작품 단위 복원에서 뺀다. 비정사이고, 원래 서재의 id를 참조하기 때문이다. 이 이유를 주석으로 남긴다.
  - `lib/i18n.tsx`:
    - `forecast.*` (제목, 정사 아님, 낡음과 사유 묶음 이름, 위험 종류, 고르기, 해제, 삭제, 개요 반영)
    - starter 칩, 도구 라벨
  - `lib/agentTools.ts`: `WRITE_TOOL_NAMES`와 라벨
  - `README.md` 새 절 "Where the story could go":
    - 비정사
    - 고른 갈래가 브리프에 들어가는 방식
    - 낡음 판정
    - 개요 반영은 제안함으로

---

## ✅ Phase 4 Checkpoint

**자동 검증**
- [ ] `cd engine && CGO_ENABLED=0 go test ./internal/forecast/... ./internal/proposals/... ./internal/storycontext/... ./internal/mcphost/... ./internal/rpc/handlers/... ./internal/agent/... ./internal/episode/...`
- [ ] `make test-mobile-engine`
- [ ] `pnpm --dir apps/desktop test -- ForecastSheet.test.tsx AgentPanel.test.tsx agentToolParity.test.ts rpcAllowlist.test.ts i18n.catalog.test.ts --run`
- [ ] `pnpm --dir apps/desktop exec tsc --noEmit`
- [ ] `make test`

**수동 검증** (격리된 `LINETTA_HOME`, 프로바이더 하나 연결)
- [ ] starter 칩 `forecastNext`를 실행한다. 갈래 3개가 sheet에 나란히 보인다. 개요와 Story World에는 변화가 없다.
- [ ] 한 갈래를 고른다. 활동 로그에 새 도구 호출이 생기지 않아야 한다(직접 RPC). 다음 턴 브리프의 "작가 방향"에 고른 갈래가 계획으로 들어간다.
- [ ] 마지막 회차 본문을 한 문장 고치고 sheet를 다시 연다. "정사가 바뀜: N화 본문" 배너가 보인다. 문장을 되돌리면 배너가 사라진다.
- [ ] [개요 반영 제안 요청] → 보내기 → 제안함에 장면 후보가 쌓인다 → 적용하면 개요에 생기고, 되돌리기 버튼으로 한 번에 되돌릴 수 있다.
- [ ] 갈래 6개를 저장하려는 시도(도구를 직접 호출하는 테스트로 대신해도 된다)가 오류로 돌아온다.

**완료 처리**
1. 결과를 보고한다. 확인하지 못한 항목은 그렇다고 적는다.
2. `feat/story-assist-phase-4-forecast` 브랜치를 푸시하고 PR을 연다. 머지는 사용자 승인 뒤에 한다.
3. 로드맵 상태를 갱신한다(`_상태:_` 줄, 페이즈 표).
