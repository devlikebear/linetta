# Phase 2: 작가 경계 — 작업지시서

_위에서부터 순차 실행하고 Checkpoint에서 작가 확인을 받는다._

_작성일: 2026-11-02_
_속한 로드맵: [`roadmap.md`](./roadmap.md)_
_예상 소요: 1일_
_상태: 구현 완료. 화면 수동 확인은 남아 있다_

## 페이즈 목표

작가가 고른 작품 규칙이 어느 회차 브리프에든 들어가고, 에이전트는 그 규칙을 바꾸지 못한다. 작가는 앞 대화를 지우지 않고 새 대화를 시작할 수 있고, 경계는 앱을 재시작해도 남는다.

## 전제 조건

- [x] Phase 1 완료 및 작가 승인
- [x] `main`에서 `feat/longform-writer-boundaries` 브랜치 생성
- [x] 마이그레이션 번호 확인. 작성 시점의 다음 번호는 `0042`다

## 포함 기능

1. 원장 항목 고정
2. 브리프의 작품 규칙 섹션
3. 대화 경계
4. 경계 전 정리 칩

## 이 페이즈에서 하지 않는 것

- 대화 한도 기본값 변경 → Out of Scope. 100,000자를 유지한다
- 대화 자동 경계 → Phase 4의 아크 결산에서 작가에게 제안만 한다
- 모델이 쓰는 인계문 → Out of Scope
- 기존 `agent.clear`를 화면에 노출하기 → 영구 삭제이므로 이 기능을 대신하지 않는다
- 별도 작품 헌법 문서 → 고정한 원장 항목으로 충분하다

## 작업 체크리스트

### 작업 그룹 A: 원장 고정

- [x] **T2.A.1** — 마이그레이션
  - 파일: `engine/internal/store/migrations/0042_writer_boundaries.sql`
  - 내용:
    - `ALTER TABLE story_ledger ADD COLUMN pinned INTEGER NOT NULL DEFAULT 0;`
    - `agent_history_marks(project_id TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE, start_message_id TEXT NOT NULL, set_at INTEGER NOT NULL)`
  - 참조: `0035_story_spine.sql`, `0037_agent_memory_archive.sql`
  - 검증: `CGO_ENABLED=0 go test ./internal/store/...`

- [x] **T2.A.2** — 저장소
  - 파일: `engine/internal/ledger/ledger.go`, `repo.go`, `ledger_test.go`
  - 내용:
    - `Entry`에 `Pinned bool`을 더하고 `entryColumns`, 스캔, 저장을 맞춘다.
    - `SetPinned(ctx, now, id, pinned)`를 더한다. 고정은 이 함수로만 바꾼다. `Patch`에 넣지 않는다.
    - `kind`가 정해진 사실일 때만 고정할 수 있게 한다. 복선은 고정하지 않는다.
  - 검증: `CGO_ENABLED=0 go test ./internal/ledger/...`

- [x] **T2.A.3** — 에이전트는 고정 항목을 못 바꾼다
  - 파일: `engine/internal/mcphost/tools_ledger.go`, 관련 테스트
  - 내용:
    - `editLedger`에서 고정 항목의 수정과 삭제를 `toolErr`로 거절한다. 문구는 작가에게 고정 해제를 부탁하라고 안내한다.
    - `editLedgerInput`에 고정 입력을 추가하지 않는다.
  - 참조: 로드맵의 "작가 소유" 결정, story-assist의 초안 승인 원칙
  - 검증: 내장 에이전트 경로와 외부 경로 모두 거절됨을 테스트로 확인

- [x] **T2.A.4** — RPC와 화면
  - 파일: `engine/internal/rpc/handlers/ledger.go`, 등록 위치, `apps/desktop/src-tauri/src/lib.rs`, `apps/desktop/src/lib/rpc.ts`, `apps/desktop/src/components/CanonPanel.tsx`, `apps/desktop/src/lib/i18n.tsx`
  - 내용:
    - `ledger.set_pinned` RPC를 더하고 허용 목록과 `lib/rpcAllowlist.test.ts`를 맞춘다.
    - 원장 패널의 정해진 사실에 고정 토글을 둔다. 고정 항목을 위로 모은다.
    - 고정 분량이 예산을 넘으면 패널에 안내한다. 세 언어를 모두 쓴다.
  - 검증: vitest, `tsc --noEmit`

- [x] **T2.A.5** — 브리프 섹션
  - 파일: `engine/internal/ledger/brief.go`, `engine/internal/storycontext/types.go`, `render.go`
  - 내용:
    - `LedgerBrief`에 `Pinned []string`을 더한다. `briefPinnedRunes = 3000`이고 다른 원장 예산과 따로 센다.
    - 현재 씬 뒤에 심은 항목은 고정돼도 내보내지 않는다. 미리 읽기 금지 규칙을 지킨다.
    - 렌더 위치는 작가 메모 뒤, 원장 앞이다. 제목은 `## 작품 규칙`이다. 세 언어를 쓴다.
    - 예산을 넘으면 `Omitted`로 센다.
  - 의존: T2.A.2
  - 검증: `CGO_ENABLED=0 go test ./internal/ledger/... ./internal/storycontext/...`

### 작업 그룹 B: 대화 경계

- [x] **T2.B.1** — 저장과 재생
  - 파일: `engine/internal/companion/history.go`, `engine/internal/agent/agent.go`, `prompt.go`, 관련 테스트
  - 내용:
    - `HistoryRepo`에 `Mark(ctx, projectID)`, `MarkOf(ctx, projectID)`를 더한다.
    - `replayHistory`와 `ReplayStart`가 표시 이전의 메시지를 재생 대상에서 뺀다. 두 함수가 같은 결과를 내야 패널의 경계가 실제와 맞는다.
    - 표시를 그을 때 `historyStarts`의 해당 항목을 지운다.
    - 표시가 가리키는 메시지가 없으면 표시가 없는 것처럼 동작한다.
    - `historyCut`가 참이 되어 기존 `historyCutNote`가 나가게 한다. 검색 도구 안내를 재사용한다.
    - `Clear`와 작품 삭제에서 표시도 지운다.
  - 참조: `priorMessages`, `windowFrom`, `historyCut`
  - 테스트: 표시 뒤 재생에 앞 메시지가 없다, 재시작 뒤 유지, 표시 없을 때 지금과 같음, 대화 지우기 뒤 표시 제거
  - 검증: `CGO_ENABLED=0 go test ./internal/agent/... ./internal/companion/...`

- [x] **T2.B.2** — 검색은 그대로
  - 파일: `engine/internal/mcphost` 대화 검색 테스트
  - 내용: 경계 이전 메시지가 `linetta_search_history`로 찾아지는 테스트를 더한다. 색인과 원본은 건드리지 않는다.
  - 검증: `CGO_ENABLED=0 go test ./internal/mcphost/...`

- [x] **T2.B.3** — RPC와 패널
  - 파일: `engine/internal/rpc/handlers/agent.go`, `apps/desktop/src-tauri/src/lib.rs`, `apps/desktop/src/lib/rpc.ts`, `apps/desktop/src/components/agent/AgentPanel.tsx`, `apps/desktop/src/lib/i18n.tsx`
  - 내용:
    - `agent.start_fresh` RPC를 더한다. 진행 중인 턴이 있으면 거절한다.
    - 패널 메뉴에 "새 대화 시작"을 둔다. 앞 대화는 패널에 그대로 보이고 기존 `historyCut` 안내 줄이 경계를 보여 준다.
    - 확인 문구에 지워지지 않는다는 점과, 대화에서만 정한 내용은 먼저 원장에 옮기라는 점을 쓴다.
  - 검증: vitest, `tsc --noEmit`

- [x] **T2.B.4** — 경계 전 정리 칩
  - 파일: `apps/desktop/src/components/agent/AgentPanel.tsx`, `apps/desktop/src/lib/i18n.tsx`
  - 내용:
    - 확인 창에 칩 하나를 둔다. 누르면 "이번 대화에서 정했지만 원장에 없는 결정을 찾아 옮겨 줘" 문장이 입력창에 채워진다. 작가가 전송한다.
    - 기존 시작 칩과 같은 방식이다. 새 도구를 만들지 않는다.
  - 검증: vitest

---

## ✅ Phase 2 Checkpoint

**구현 확인:**
- [x] 모든 작업 체크박스 완료
- [x] 대화 한도 기본값과 `agent.clear` 동작이 그대로임

**자동 검증:**
- [x] `CGO_ENABLED=0 go test ./...` 통과 (`engine/`)
- [x] `go test -tags mobile ./...` 통과
- [x] vitest, `tsc --noEmit`, 프런트 빌드 통과
- [x] 원장 `pinned`가 vault 왕복과 `restore.MergeProject` 뒤 남는 테스트 통과

**수동 확인:**
- [ ] 복사본 앱에서 표기 규칙 항목을 고정하고, 먼 회차의 브리프에 `## 작품 규칙`으로 들어오는지 확인한다
- [ ] 내장 에이전트에게 고정 항목 수정을 요청해 거절되는지 확인한다
- [ ] 새 대화 시작 뒤 턴을 돌리고, 앱을 재시작해 경계가 유지되는지 확인한다
- [ ] 경계 이전의 결정을 에이전트가 대화 검색으로 찾는지 확인한다

**완료 처리:** 결과를 작가에게 보고하고, 승인 뒤 Phase 3으로 간다.

---

## 검증 기록

2026-11-02. 단위 테스트로 고정 항목 거절, 경계 재시작 유지, 경계 이전 검색을 확인했다. 실제 DB 복사본에서 `ledger.set_pinned`로 표기 규칙을 고정하고 17화 브리프에 `## 작품 규칙`이 들어오는 것을 확인했다. 앱 화면을 직접 눌러 보는 수동 확인은 하지 못했다.

## 메모 / 주의

- 경계를 긋는 턴에서는 재생이 바뀌어 프롬프트 캐시가 한 번 깨진다. 작가가 고른 시점이라 괜찮다.
- `agent_history_marks`는 대화 기록과 같이 기기 로컬이다. `vaultbundle/tables.go`에 넣지 않는다.
- memory-search의 스파이크대로 창 시작점은 메모리에만 있어 재시작하면 달라진다. 경계는 DB에 두므로 이 문제가 없다.

---
_다음 페이즈: Phase 3 — 시점 사실과 정산 → [`phase-3-temporal-facts.md`](./phase-3-temporal-facts.md)_
