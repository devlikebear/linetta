# Phase 3: Episode Workbench와 Tessera 협업 실행

## 목표

사람이 에피소드 소재/주제/상황/얼개를 제공하면 Tessera 에이전트 팀이 이를 확장하고, 초안/비평/자료조사/일관성 검토 산출물을 만든다. 이 phase가 끝나면 Linetta의 핵심 협업 경험이 처음으로 동작한다.

## 협업 구조

```text
Human Blueprint
  -> Muse
  -> Plot Architect
  -> Canon Keeper
  -> Researcher
  -> Writer
  -> Critic
  -> Editor
  -> Human Review
```

역할:

- `Muse`: 영감, 장면 가능성, 갈등 씨앗 제안
- `Plot Architect`: 사람이 준 얼개를 장면 단위 구조로 강화
- `Canon Keeper`: 기존 Canon memory와 충돌 가능성 탐지
- `Researcher`: 필요한 자료/팩트체크 항목 정리
- `Writer`: 초안 생성
- `Critic`: 재미, 긴장감, 클리셰, 캐릭터 동기 비평
- `Editor`: 문장, 리듬, 장면 전환 개선

## 범위

- Episode blueprint 저장
- Tessera episode run orchestration
- SSE run event stream
- 산출물 저장
- SwiftUI Episode Workbench

## 데이터 모델

```text
episode_blueprints
  id
  work_id
  episode_id
  premise
  theme
  situation
  must_include
  must_avoid
  structure_notes
  created_at
  updated_at

artifacts
  id
  work_id
  episode_id
  run_id
  kind                 -- muse_notes, outline, research, draft, critique, edit
  title
  body
  created_at

agent_runs
  id
  work_id
  episode_id
  status
  tessera_run_id
  created_at
  closed_at

agent_run_events
  id
  run_id
  seq
  event_json
  created_at
```

## 작업 목록

### 1. Episode blueprint 추가

- [ ] `internal/work`에 blueprint 모델 추가
- [ ] API 추가
  - `GET /api/works/{workID}/episodes/{episodeID}/blueprint`
  - `PUT /api/works/{workID}/episodes/{episodeID}/blueprint`
- [ ] 테스트 추가
  - blueprint 저장/수정
  - 작품/에피소드 소유권 검증

검증:

```sh
go test ./internal/work/... ./internal/server/...
```

### 2. Tessera episode run 패키지 추가

- [ ] `internal/agent` 패키지 추가
  - `type EpisodeRunInput struct`
  - `type EpisodeRunResult struct`
  - `type Runner struct`
- [ ] `Runner.RunEpisode(ctx, input)` 구현
  - 현재 `internal/novel`의 deterministic authoring 흐름을 재사용하되, work memory와 blueprint를 입력으로 받는다.
  - Tessera `run.EventSink`를 받아 이벤트를 저장/전송한다.
- [ ] 에이전트 산출물 key 표준화
  - `muse-notes`
  - `plot-outline`
  - `canon-review`
  - `research-notes`
  - `draft`
  - `critique`
  - `edited-draft`
- [ ] 테스트 추가
  - run closure normal
  - artifacts 생성
  - event sink에 `task.succeeded` 이벤트 기록

검증:

```sh
go test ./internal/agent/...
```

### 3. Run API와 SSE 추가

- [ ] API 추가
  - `POST /api/works/{workID}/episodes/{episodeID}/runs`
  - `GET /api/runs/{runID}`
  - `GET /api/runs/{runID}/events`
  - `GET /api/runs/{runID}/events/stream`
  - `GET /api/runs/{runID}/artifacts`
- [ ] SSE stream은 `text/event-stream`으로 Tessera event JSON을 전송한다.
- [ ] run은 처음에는 synchronous로 시작해도 되지만, UI가 blocking되지 않도록 goroutine + run status 저장 구조로 만든다.
- [ ] 테스트 추가
  - run 생성
  - events 목록 반환
  - artifacts 반환

검증:

```sh
go test ./internal/server/... ./internal/agent/...
```

### 4. SwiftUI Episode Workbench

- [ ] `Views/EpisodeWorkbenchView.swift` 추가
- [ ] 중앙 화면 구성
  - episode title
  - premise/theme/situation 입력
  - must include / must avoid
  - structure notes
  - run button
- [ ] 오른쪽 AI 협업 패널 추가
  - agent list
  - 현재 run status
  - artifacts list
- [ ] 아래쪽 또는 별도 탭에 Run Timeline 추가
  - queued/started/succeeded/failed
  - role/stage 표시
  - 실패 시 error 표시
- [ ] artifact를 선택하면 preview를 보여준다.

수동 확인:

- [ ] 에피소드 얼개를 입력하고 실행할 수 있다.
- [ ] run event가 UI에 표시된다.
- [ ] draft/critique/edit artifact를 볼 수 있다.

---

### Checkpoint: Phase 3 완료 확인

**구현 확인:**
- [ ] 사람의 에피소드 얼개가 저장된다.
- [ ] Tessera 에이전트 run이 실행된다.
- [ ] 이벤트와 산출물이 에피소드에 연결된다.
- [ ] SwiftUI에서 협업 실행 흐름을 볼 수 있다.

**실행 확인:**
- [ ] `go test ./...` 통과
- [ ] `xcodebuild ... test` 통과

**사용자 확인:**
- [ ] "AI가 뮤즈/비평가/편집자로 협업한다"는 경험이 느껴지는지 확인받는다.

이 체크포인트를 통과하면 사용자에게 확인 요청 후 Phase 4로 진행한다.
