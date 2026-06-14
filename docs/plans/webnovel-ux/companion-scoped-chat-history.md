# 컴패니언 씬 스코프 채팅 히스토리 — 개선 기획서

_작성일: 2026-06-14_
_속한 로드맵: [`roadmap.md`](./roadmap.md), [`companion-writing-actions.md`](./companion-writing-actions.md)의 후속 안정화_
_관련 문서: [`companion-scene-edit-reliability.md`](./companion-scene-edit-reliability.md)_
_예상 소요: 2~3일_
_구현 상태: 2026-06-14 구현 완료_

## Overview

컴패니언 대화는 앱을 재시작해도 유지되어야 하고, 현재 보고 있는 씬을 편집할 때는 그 씬의 맥락을 잃지 않아야 한다.
하지만 현재 구현은 TARS worker transcript를 프로젝트당 1개만 쓰고, 프론트도 `projectId` 기준 메시지 배열만 들고 있다.
그래서 앱을 껐다 켠 뒤에는 어느 메시지가 어느 씬에서 오간 대화였는지 복원할 수 없고, 장면을 옮겨 다니며 작업하면
이전 씬의 대화가 현재 씬 요청을 흐릴 수 있다.

이 계획의 목표는 컴패니언을 **작품 전체의 집필 동료**로 유지하면서도, 사용자가 현재 씬을 보고 있을 때는
**현재 씬에 묶인 대화와 작업 상태**를 안정적으로 보여주는 것이다.

## 결론

완전한 씬별 세션 분리는 권장하지 않는다.

- 장편 집필에서는 이전 장면에서 정한 작가 취향, 작품 규칙, 인물 해석이 다음 장면에서도 필요하다.
- TARS `EnsureWorker(projectID)` 구조는 이미 프로젝트 단위 worker를 전제로 한다.
- 씬마다 별도 worker를 만들면 대화는 깔끔해지지만, 작품 전체 연속성이 약해지고 세션 관리 비용이 커진다.

권장안은 다음과 같다.

1. **모델 replay용 TARS transcript는 프로젝트당 1개 유지**
2. **Linetta 소유의 companion history 테이블을 추가**
3. 각 메시지에 `node_id`, `run_id`, `scope`, `intent`, `status`를 기록
4. UI는 기본으로 `현재 씬` 대화를 보여주고, 필요하면 `작품 전체`로 전환
5. LLM에 넣는 이전 대화는 "현재 씬 최근 대화 + 작품 전체 요약/기억"으로 재구성

즉 세션을 물리적으로 씬별로 쪼개는 대신, **프로젝트 세션 위에 씬 스코프 레이어를 올린다.**

## 현재 코드베이스 분석

- `engine/internal/companion/companion.go`
  - `NewService()`는 `session.NewStore(sessionsDir)`를 만들고, `History(ctx, projectID)`는
    `s.sessions.EnsureWorker(projectID)` 뒤 `session.ReadMessages(s.sessions.TranscriptPath(sess.ID))`를 그대로 반환한다.
  - `CompactHistory()`, `ClearHistory()`, `DeleteProjectData()`도 같은 worker transcript 파일을 대상으로 동작한다.
  - 결과적으로 저장 단위는 프로젝트 worker 1개다.

- `engine/internal/companion/runner.go`
  - `start()`에서 사용자 메시지를 TARS transcript에 먼저 append한다.
  - 이후 `session.LoadHistory(path, historyTokenBudget)`로 같은 transcript를 읽어 LLM message list에 붙인다.
  - `run()`이 끝나면 assistant 메시지를 같은 transcript에 append하고 `companion.done` 이벤트를 보낸다.
  - 현재 변경분 기준으로 scene write intent와 `set_scene_text` 적용 검증은 강화되어 있지만, transcript에는 씬 메타데이터가 없다.

- `engine/internal/rpc/handlers/companion.go`
  - `companion.history` params는 `project_id`만 받는다.
  - 응답 DTO는 `role`, `content`, `timestamp`만 내려준다.
  - `node_id`, `run_id`, `scope`, `status`가 없어서 프론트가 대화를 씬별로 필터링할 수 없다.

- `apps/desktop/src/lib/rpc.ts`
  - `companion.history(projectId)`는 `project_id`만 호출한다.
  - `companion.compact(projectId)`, `companion.clear(projectId)`도 프로젝트 단위뿐이다.

- `apps/desktop/src/hooks/useCompanion.ts`
  - 전역 `stores` 맵의 key가 `projectId`다.
  - `ChatMessage`에는 `role`, `content`, `proposal`, `choices`, `errored`, `retryText`만 있다.
  - `loadHistory(projectId)`는 재시작 후 transcript를 다시 읽지만, 어느 메시지가 현재 씬 메시지인지 알 수 없다.
  - `send()`는 `nodeIdRef.current`를 backend로 보내지만, 로컬 user message에는 node id를 저장하지 않는다.

- `apps/desktop/src/components/companion/CompanionPanel.tsx`
  - 메시지를 받은 순서대로 렌더링한다.
  - 현재 씬/작품 전체 전환 UI가 없고, 메시지에 씬 이름 칩도 없다.
  - reasoning은 streaming 중 `<details>`로 보여줄 수 있지만, 완료 메시지에는 reasoning/history scope 정보가 남지 않는다.

- `engine/internal/companion/prompt.go`
  - context에는 `작성된 본문 발췌`, `씬 (비트를 붙일 수 있는 실제 씬 — node_id)`가 포함된다.
  - 현재 씬 본문 변경은 `set_scene_text`를 쓰라고 지시한다.
  - 다만 이전 대화 replay는 프로젝트 transcript 전체 기준이라, UI상 현재 씬 작업이어도 이전 장면 대화가 섞일 수 있다.

- TARS session message 구조
  - `session.Message`는 `ID`, `Role`, `Content`, `Timestamp`, tool 관련 필드만 제공한다.
  - Linetta의 `node_id`나 `intent`를 자연스럽게 넣을 필드가 없다.
  - 따라서 TARS transcript 자체를 확장하려 하기보다 Linetta 소유 저장소를 두는 편이 안전하다.

## 문제 정의

현재 구조의 문제는 "세션을 저장하지 않는다"가 아니라, **저장한 대화에 작품/씬 범위 정보가 없다**는 점이다.

- 앱 재시작 후 history는 복구되지만 프로젝트 전체 transcript로만 복구된다.
- 사용자가 현재 씬에서 이어서 작업하려 해도, 이전 씬 대화가 같은 목록에 섞인다.
- LLM replay도 같은 transcript에서 가져오므로, 모델이 현재 씬보다 이전 대화의 선택지/질문 흐름을 이어갈 수 있다.
- `clear`와 `compact`도 프로젝트 단위라, 현재 씬 대화만 정리하는 UX를 만들 수 없다.
- 메시지와 적용 이벤트가 연결되지 않아 "이 답변이 어느 씬을 바꿨는지"를 나중에 추적하기 어렵다.

## 제품 원칙

- **컴패니언은 작품 단위 인격을 유지한다.**
  - 작가 취향, 세계관 규칙, 장기 플롯은 씬을 넘어 이어진다.

- **작업 화면은 현재 씬 중심이어야 한다.**
  - 사용자가 2장 1씬을 보고 있으면 컴패니언 패널도 기본적으로 2장 1씬 작업 대화를 보여준다.

- **대화 기록은 모델 transcript가 아니라 앱 데이터다.**
  - TARS transcript는 LLM replay 구현 세부사항이다.
  - 사용자에게 보여줄 history, 필터, 검색, 복구는 Linetta가 소유해야 한다.

- **이전 대화 replay는 무조건 전체가 아니라 의도적으로 조립한다.**
  - 현재 씬 편집 요청에는 현재 씬 최근 대화를 우선한다.
  - 작품 전체 맥락은 memory, outline, plot, compact summary로 제공한다.

## 설계 제안

### 1. Companion History 저장소 추가

SQLite migration을 추가한다.

```sql
CREATE TABLE companion_messages (
  id         TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  node_id    TEXT REFERENCES nodes(id) ON DELETE SET NULL,
  run_id     TEXT,
  role       TEXT NOT NULL,
  scope      TEXT NOT NULL DEFAULT 'project',
  intent     TEXT,
  status     TEXT,
  content    TEXT NOT NULL,
  created_at INTEGER NOT NULL
);

CREATE INDEX idx_companion_messages_project_time
  ON companion_messages(project_id, created_at);

CREATE INDEX idx_companion_messages_node_time
  ON companion_messages(project_id, node_id, created_at);

CREATE INDEX idx_companion_messages_run
  ON companion_messages(run_id);
```

권장 `scope` 값:

- `scene`: 특정 `node_id`에 묶인 대화
- `project`: 작품 전체 구조, 개요, 캐릭터, 플롯 대화
- `global`: 향후 앱 사용법/설정 등 작품 밖 대화가 필요할 때

권장 `status` 값:

- `streaming`
- `done`
- `applied`
- `failed`
- `cancelled`
- `compacted`

`companion` 패키지에는 `HistoryRepo`를 추가하고, `NewService()`는 `WithHistory(repo)` 식으로 주입받거나 생성자 인자를 확장한다.
메인 엔진에서는 `store.DB()`로 repo를 만든 뒤 companion service에 연결한다.

### 2. TARS transcript와 Linetta history 역할 분리

현행 TARS transcript는 당장 제거하지 않는다.

- TARS transcript
  - 기존 provider/agentloop와 호환되는 LLM replay용 로그
  - 프로젝트 worker 1개 유지
  - 기존 사용자 history를 잃지 않기 위한 fallback

- Linetta companion history
  - UI 표시의 source of truth
  - 씬별 필터와 작품 전체 필터 지원
  - run/apply/error 상태 추적
  - 향후 검색, 메시지 삭제, 씬별 compact의 기반

마이그레이션 전략:

- 새 테이블이 비어 있고 기존 TARS transcript가 있으면, 최초 `companion.history` 호출 시 legacy import를 수행한다.
- legacy import된 메시지는 `scope='project'`, `node_id=NULL`, `status='done'`으로 저장한다.
- 이후 새 turn부터는 사용자 메시지와 assistant 메시지를 Linetta history에 먼저 기록하고, TARS transcript에도 병행 기록한다.

### 3. RPC history API 확장

`companion.history` params를 확장한다.

```ts
type CompanionHistoryScope = "scene" | "project";

interface CompanionHistoryParams {
  project_id: string;
  node_id?: string;
  scope?: CompanionHistoryScope;
  limit?: number;
}
```

동작:

- `scope="scene"`이면 `project_id + node_id` 메시지만 반환한다.
- `scope="project"`이면 프로젝트 전체 메시지를 반환하되, 각 메시지에 `node_id`와 scene label을 포함한다.
- `node_id`가 없으면 `project` scope로 fallback한다.
- 기본 limit은 최근 100개 정도로 둔다.

응답 DTO:

```ts
interface CompanionMessage {
  id: string;
  project_id: string;
  node_id?: string | null;
  node_label?: string;
  run_id?: string;
  role: string;
  content: string;
  timestamp: number;
  scope: "scene" | "project" | "global";
  intent?: string;
  status?: string;
}
```

`companion.clear`와 `companion.compact`도 같은 scope params를 받게 한다.

- `clear(scene)`: 현재 씬 대화만 숨기거나 삭제
- `clear(project)`: 기존처럼 전체 삭제
- `compact(scene)`: 현재 씬 대화 요약 1개로 압축
- `compact(project)`: 작품 전체 대화 요약

MVP에서는 `clear/compact`는 프로젝트 단위를 유지하고, history 조회만 scope를 먼저 적용해도 된다. 다만 API는 이후 확장을 막지 않게 설계한다.

### 4. Frontend store key를 scope-aware로 변경

현재 `stores: Map<string, CompanionSessionStore>`는 `projectId`만 key로 쓴다.
이를 다음 중 하나로 바꾼다.

권장:

```ts
type CompanionStoreKey = `${projectId}:${scope}:${nodeIdOrAll}`;
```

- 현재 씬 보기: `p1:scene:n1`
- 작품 전체 보기: `p1:project:all`

`useCompanion()` signature는 다음 정보를 받도록 확장한다.

```ts
useCompanion(projectId, nodeIdRef, {
  scope,
  onApplied,
  contextSelection,
  outlineStructure,
})
```

또는 기존 인자를 유지하면서 내부에서 `nodeIdRef.current`와 패널 상태의 `scope`를 결합한다.

주의점:

- streaming run은 scope와 node_id를 runProjects에 함께 기록해야 한다.
- `companion-delta`, `companion-done`, `companion-error` 이벤트 payload에 `node_id`가 들어와야 정확한 store를 찾을 수 있다.
- 이벤트 payload 확장이 늦어지면, send 시점에 `runScopes.set(run_id, {projectId, nodeId, scope})`를 두고 매핑한다.

### 5. CompanionPanel UI: 현재 씬 / 작품 전체

컴패니언 헤더 아래 또는 메시지 영역 상단에 작은 segmented control을 둔다.

- `현재 씬`
- `작품 전체`

기본값:

- editor가 leaf node를 열고 있으면 `현재 씬`
- leaf가 아니거나 node가 없으면 `작품 전체`

현재 씬 탭:

- 현재 node_id에 묶인 메시지만 표시
- 빈 상태는 "이 씬에서 바로 할 수 있는 작업" 액션 팔레트를 보여준다.
- 사용자가 메시지를 보내면 기본 `scope='scene'`, `node_id=current`로 저장한다.

작품 전체 탭:

- 프로젝트 전체 메시지를 표시
- 다른 씬 메시지에는 작은 scene chip을 붙인다.
- 캐릭터, 플롯, 개요, 설정 대화가 자연스럽게 남는다.

디자인 원칙:

- 패널이 좁으므로 탭은 텍스트 2개만 쓰고, 긴 설명은 넣지 않는다.
- 메시지 bubble 안에는 scene chip을 넣지 말고, `msg-who` 라인이나 bubble 위 meta line에 표시한다.
- 현재 씬 탭에서 다른 씬의 메시지를 억지로 흐리게 보여주지 않는다. 완전히 숨기는 편이 덜 헷갈린다.

### 6. LLM history replay 재구성

현재는 `session.LoadHistory(path, historyTokenBudget)`로 프로젝트 transcript를 그대로 replay한다.
씬 스코프가 생기면 replay도 다음 순서로 바꾼다.

scene intent 또는 현재 씬 질문:

1. system
2. 현재 project context (`buildContext`)
3. 현재 씬 최근 대화 N개
4. 작품 전체 compact summary 또는 최근 project-scope 메시지 일부
5. 이번 user turn

project intent 또는 작품 전체 질문:

1. system
2. 현재 project context
3. project-scope 최근 대화 N개
4. 필요 시 current scene 최근 대화 일부
5. 이번 user turn

구현 방법:

- 초기 MVP는 TARS `LoadHistory`를 계속 쓰되, UI history만 scope-aware로 바꾼다.
- 다음 단계에서 `HistoryRepo.LoadForPrompt(projectID, nodeID, intent, budget)`를 만들고, runner가 TARS transcript 대신 이 결과로 `msgs`를 조립한다.
- TARS transcript append는 호환/백업 목적으로 유지한다.

이 단계가 중요한 이유:

- UI만 씬별로 보여도, LLM replay가 전체 transcript면 모델은 여전히 이전 씬의 질문 흐름을 이어갈 수 있다.
- 특히 "1번", "적용해줘", "그렇게 해줘" 같은 짧은 follow-up은 현재 씬 최근 대화와 붙어야 정확해진다.

### 7. 이벤트 payload 확장

다음 이벤트 payload에 `node_id`, `scope`, `intent`를 추가한다.

- `companion.delta`
- `companion.reset`
- `companion.thinking`
- `companion.reasoning`
- `companion.proposal`
- `companion.choices`
- `companion.applied`
- `companion.done`
- `companion.error`
- `companion.cancelled`

MVP 최소값:

- `done/error/cancelled/applied`에는 반드시 포함
- streaming delta 계열은 `run_id` 기반 매핑으로 처리 가능

### 8. Reasoning 표시 정책

모델의 비공개 chain-of-thought를 노출하려는 기능으로 만들면 안 된다.
대신 사용자에게 보여줄 것은 앱이 안전하게 요약한 **작업 흐름**이다.

권장 정책:

- streaming 중 `reasoning` 이벤트가 오면 현재처럼 접을 수 있는 영역에 표시한다.
- 완료 후에는 raw reasoning을 transcript에 저장하지 않는다.
- scene write 성공 메시지는 앱이 만든 작업 흐름을 보여준다.
  - 예: "현재 씬 맥락 확인 → 본문 작성 → set_scene_text 적용 → readback 검증"
- 실패 시에는 진단 복사용 metadata를 제공한다.

즉 사용자가 "추론 과정이 생략된 것 같다"고 느끼는 문제는 raw reasoning 노출보다,
**앱이 지금 무슨 작업을 했는지 단계별 상태를 보여주는 UX**로 해결한다.

## 작업 체크리스트

### 작업 그룹 A: history 저장소

- [x] **A.1** — migration 추가
  - 파일: `engine/internal/store/migrations/0013_companion_messages.sql`
  - 내용:
    - `companion_messages` 테이블
    - project/time, node/time, run index
  - 검증:
    - migration idempotency test
    - project 삭제 시 companion messages cascade 삭제

- [x] **A.2** — `HistoryRepo` 추가
  - 파일: 신규 `engine/internal/companion/history.go`
  - 내용:
    - `Append(ctx, MessageRecord)`
    - `List(ctx, Query)`
    - `MarkStatus(ctx, id/runID, status)`
    - `Clear(ctx, Query)`
    - `ImportLegacy(ctx, projectID, []session.Message)`
  - 검증:
    - scene scope list가 해당 node만 반환
    - project scope list가 node metadata를 포함
    - legacy import가 중복 실행되지 않음

- [x] **A.3** — service wiring
  - 파일: `engine/cmd/linetta-engine/main.go`, `engine/internal/companion/companion.go`
  - 내용:
    - `store.DB()` 기반 repo 생성
    - companion service에 주입
    - 테스트 helper 갱신

### 작업 그룹 B: RPC와 DTO

- [x] **B.1** — history params 확장
  - 파일: `engine/internal/rpc/handlers/companion.go`
  - 파일: `apps/desktop/src/lib/rpc.ts`
  - 파일: `apps/desktop/src/lib/types.ts`
  - 내용:
    - `scope`, `node_id`, `limit`
    - 응답에 `id`, `node_id`, `node_label`, `run_id`, `scope`, `intent`, `status`
  - 검증:
    - handler test에서 scene/project scope 응답 확인

- [x] **B.2** — send/run metadata 기록
  - 파일: `engine/internal/companion/runner.go`
  - 내용:
    - user message 기록 시 node_id/scope/intent/run_id 저장
    - assistant done/error/cancelled도 같은 run_id로 저장
    - TARS transcript append는 유지
  - 검증:
    - send 후 history(scene)에 user/assistant 둘 다 복원
    - 앱 재시작 시 같은 씬 대화가 복원

- [x] **B.3** — event payload metadata 추가
  - 파일: `engine/internal/companion/runner.go`, `apps/desktop/src/lib/types.ts`, `apps/desktop/src/hooks/useCompanion.ts`
  - 내용:
    - done/error/applied/cancelled에 node_id/scope/intent 포함
    - streaming event는 run mapping으로 보완
  - 검증:
    - 동시에 다른 panel scope가 있어도 이벤트가 올바른 store로 감

### 작업 그룹 C: frontend scope UX

- [x] **C.1** — `useCompanion` store key 확장
  - 파일: `apps/desktop/src/hooks/useCompanion.ts`
  - 내용:
    - `projectId` 단일 key에서 `projectId + scope + nodeId` key로 변경
    - history load도 scope별로 수행
    - runProjects/runScopes 매핑 추가
  - 검증:
    - 씬 A에서 메시지 전송 후 씬 B로 이동하면 씬 A 메시지가 보이지 않음
    - 작품 전체 탭에서는 씬 A 메시지가 보임
    - 앱 재시작 후 동일하게 복원

- [x] **C.2** — CompanionPanel segmented control
  - 파일: `apps/desktop/src/components/companion/CompanionPanel.tsx`
  - 파일: `apps/desktop/src/components/companion/CompanionPanel.css`
  - 파일: `apps/desktop/src/lib/i18n.tsx`
  - 내용:
    - `현재 씬`, `작품 전체` 탭
    - scene chip 표시
    - 현재 씬이 없을 때 project scope fallback
  - 검증:
    - `CompanionPanel.test.tsx`에서 탭 전환과 empty state 확인
    - 긴 scene label이 패널 폭을 넘지 않음

- [x] **C.3** — clear/compact UX 정책 정리
  - MVP:
    - clear/compact는 현재 선택 scope에 적용한다.
    - destructive action은 기존 버튼을 유지하되 tooltip/aria label을 scope-aware로 바꾼다.
  - 검증:
    - 현재 씬 clear가 작품 전체 기록까지 삭제하지 않음

### 작업 그룹 D: prompt replay 개선

- [x] **D.1** — `HistoryRepo.LoadForPrompt`
  - 파일: `engine/internal/companion/history.go`
  - 파일: `engine/internal/companion/runner.go`
  - 내용:
    - scene intent는 현재 씬 최근 대화를 우선
    - project intent는 project scope 최근 대화 우선
    - compact summary가 있으면 예산 안에서 포함
  - 검증:
    - "1번", "적용해줘" 같은 follow-up이 현재 씬 대화와 붙어 모델에 전달됨
    - 다른 씬의 직전 선택지에 끌려가지 않음

- [x] **D.2** — compact summary 분리
  - 파일: `engine/internal/companion/companion.go`
  - 내용:
    - scene compact summary와 project compact summary 구분
    - legacy `CompactHistory`와 호환 정책 결정
  - 검증:
    - scene compact가 다른 씬 대화를 요약하지 않음

### 작업 그룹 E: 검증

- [x] **E.1** — Go tests
  - 명령:
    - `go test ./engine/internal/companion ./engine/internal/rpc/handlers ./engine/internal/store`
  - 케이스:
    - migration
    - HistoryRepo append/list/import
    - RPC scene/project history
    - runner가 node_id/scope를 저장

- [x] **E.2** — Frontend tests
  - 명령:
    - `cd apps/desktop && pnpm test -- useCompanion.events.test.tsx CompanionPanel.test.tsx --run`
  - 케이스:
    - scope별 history load
    - event routing
    - 탭 전환 UI

- [x] **E.3** — 전체 게이트와 수동 검증
  - 명령:
    - `make test`
    - `git diff --check`
    - Tauri 앱 smoke
  - 수동 시나리오:
    - 씬 A에서 "이 씬 작성해줘" 실행
    - 씬 B로 이동해 컴패니언 패널 확인: 씬 A 대화가 현재 씬 탭에 보이지 않음
    - 작품 전체 탭으로 전환: 씬 A 대화가 scene chip과 함께 보임
    - 앱 종료 후 재실행: 씬 A/B별 기록이 그대로 복원
    - 씬 B에서 "1번 적용해줘" 같은 follow-up이 씬 A 선택지를 따라가지 않음

## 구현 순서

1. `git status --short --branch`로 시작 상태를 확인한다.
2. A.1~A.3으로 Linetta-owned history 저장소를 만든다.
3. B.1로 history RPC를 scope-aware로 확장한다.
4. C.1~C.2로 현재 씬/작품 전체 UI를 붙인다.
5. B.2~B.3으로 신규 turn부터 metadata가 저장되게 한다.
6. D.1로 LLM prompt replay를 scope-aware로 바꾼다.
7. C.3/D.2는 clear/compact UX를 마무리하며 적용한다.
8. E 전체 검증 후 Tauri smoke까지 확인한다.

## 완료 조건

- [x] 앱 재시작 후 컴패니언 대화가 복원된다.
- [x] 현재 씬 탭에서는 현재 node의 대화만 보인다.
- [x] 작품 전체 탭에서는 모든 대화가 scene chip과 함께 보인다.
- [x] 새 컴패니언 메시지는 `node_id`, `scope`, `run_id`, `intent`, `status`를 저장한다.
- [x] 현재 씬 편집 follow-up은 다른 씬의 직전 대화를 이어받지 않는다.
- [x] `clear/compact`는 최소한 사용자가 선택한 scope를 오해하지 않게 표시된다.
- [x] `make test`, `git diff --check`, Tauri smoke가 통과한다.

## 리스크와 대응

- **legacy transcript import 중복**
  - project별 import marker를 두거나, legacy message timestamp/content hash로 중복을 막는다.

- **동시 streaming event routing**
  - send 성공 전에는 pending run id를 scope store에 묶고, run id가 오면 `runScopes`를 갱신한다.

- **project scope가 너무 시끄러워짐**
  - 기본은 현재 씬 탭으로 두고, project scope에는 scene chip과 검색/필터를 후속으로 붙인다.

- **LLM replay 전환으로 과거 맥락 손실**
  - TARS transcript fallback을 한 릴리즈 유지한다.
  - prompt에 current scene recent history와 project summary를 함께 넣어 균형을 맞춘다.

- **raw reasoning 노출 기대**
  - 비공개 추론 원문 저장/노출 대신 앱이 검증 가능한 작업 흐름과 적용 상태를 표시한다.
