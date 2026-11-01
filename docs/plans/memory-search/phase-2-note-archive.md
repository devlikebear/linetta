# Phase 2: 작업 노트 보관 — 작업지시서

_작성일: 2026-11-01_
_속한 로드맵: [`roadmap.md`](./roadmap.md)_
_예상 소요: 1.5일_

## 페이즈 목표

작업 노트가 차면 끝난 결정을 지우지 않고 보관 영역으로 옮긴다. 보관한 줄은 프롬프트에 매번 실리지 않고, 필요할 때 검색으로 되찾는다. 작가는 보관 내용을 보고 지울 수 있다.

## 전제 조건

- [ ] Phase 1 완료 및 승인
- [ ] `main`에서 `feat/agent-note-archive` 브랜치 생성
- [ ] 다음 마이그레이션 번호 확인. Phase 1 뒤 기준 `0037`이다.

## 포함 기능

1. `agent_memory_archive` 테이블과 FTS
2. `linetta_edit_memory`의 `archive` 동작
3. `linetta_search_history` 결과에 보관 메모 포함
4. 설정의 보관 메모 목록
5. `experiences.jsonl` 경로 정리

## 이 페이즈에서 하지 않는 것

- `writer_profile` 보관 → Out of Scope
- 자동 보관 → Out of Scope. 무엇이 끝난 결정인지는 에이전트나 작가가 판단한다.
- 작업 노트 예산 증액 → 하지 않는다. 매 턴 프롬프트에 실리는 크기가 늘어난다.

## 작업 체크리스트

### 작업 그룹 A: 저장

- [ ] **T2.A.1** — 보관 테이블 마이그레이션
  - 파일: `engine/internal/store/migrations/0037_agent_memory_archive.sql`
  - 내용:
    - `agent_memory_archive(id TEXT PRIMARY KEY, project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE, body TEXT NOT NULL, source_message_id TEXT, archived_at INTEGER NOT NULL)`
    - `source_message_id`에는 외래 키를 걸지 않는다. 대화는 기기 로컬이고 보관 메모는 작품과 함께 이동한다.
    - `agent_memory_archive_fts`는 Phase 1과 같은 방식의 trigram 색인과 삽입, 삭제 트리거
  - 검증: `CGO_ENABLED=0 go test ./internal/store/...`

- [ ] **T2.A.2** — 보관 동작
  - 파일: `engine/internal/agentmemory/edit.go`, `agentmemory.go`, `archive.go` (신규)
  - 내용:
    - `ActionArchive = "archive"`를 더한다. `find`로 한 줄을 고르는 규칙은 `remove`와 같다.
    - `Repo.Edit` 안의 같은 트랜잭션에서 줄 제거와 보관 행 삽입을 함께 한다.
    - `work_notes`에서만 허용한다. `writer_profile`이면 오류를 낸다.
    - `Repo.ListArchived`, `Repo.DeleteArchived`, `Repo.RestoreArchived`를 더한다. 복원은 예산을 넘으면 `ErrOverBudget`을 돌려준다.
    - `remove`는 그대로 진짜 삭제다. 틀린 내용까지 보관하면 나중에 다시 살아난다.
    - `ErrOverBudget` 안내 문구에 보관 방법을 넣는다.
  - 테스트: 예산 감소, 원자성, 없는 줄, 모호한 줄, `writer_profile` 거절, 복원 시 예산 초과
  - 검증: `CGO_ENABLED=0 go test ./internal/agentmemory/...`

- [ ] **T2.A.3** — vault와 복원
  - 파일: `engine/internal/vaultbundle/tables.go`, `engine/internal/restore/restore.go`, 각 테스트
  - 내용:
    - `agent_memory_archive`를 `dependents`에 `byProject`로 넣는다. 작업 노트와 함께 이동해야 한다.
    - FTS 테이블 6개는 `excluded`에 넣는다.
    - `restore.go`의 병합 복원에 `copyRows`를 더한다. 삽입 트리거로 색인이 함께 채워지는지 테스트한다.
    - `seedWork`에도 행을 넣는다.
  - 검증: `CGO_ENABLED=0 go test ./internal/vaultbundle/... ./internal/restore/...`

### 작업 그룹 B: 도구와 프롬프트

- [ ] **T2.B.1** — `linetta_edit_memory` 확장
  - 파일: `engine/internal/mcphost/tools_write.go`, `tools_memory_test.go`
  - 내용: 동작 목록에 `archive`, 입력에 선택 필드 `source_message_id`를 더한다. 출력은 기존처럼 결과 본문을 돌려준다.
  - 검증: `CGO_ENABLED=0 go test ./internal/mcphost/...`

- [ ] **T2.B.2** — 검색 결과 병합
  - 파일: `engine/internal/mcphost/tools_history.go`, `engine/internal/agentmemory/archive.go`
  - 내용:
    - `Repo.SearchArchived`를 더하고 `ftsquery`를 쓴다.
    - `linetta_search_history` 결과에 `source: "note_archive" | "conversation"`을 넣고 보관 메모를 앞에 둔다.
    - 보관 메모에 `source_message_id`가 있으면 그대로 돌려줘 `linetta_read_history`로 이어지게 한다.
  - 검증: 위 명령 통과

- [ ] **T2.B.3** — 프롬프트 안내
  - 파일: `engine/internal/agent/prompt.go`, `prompt_test.go`
  - 내용: 작업 노트 안내에 "원고나 원장에 이미 반영된 끝난 결정은 archive, 틀린 내용은 remove"를 더한다. 메모리 도구 그룹이 꺼지면 안내도 뺀다.
  - 검증: `CGO_ENABLED=0 go test ./internal/agent/...`

### 작업 그룹 C: 설정 화면

- [ ] **T2.C.1** — RPC
  - 파일: `engine/internal/rpc/handlers/memory.go`, `engine/internal/engineapp/engineapp.go`, `apps/desktop/src/lib/rpc.ts`, `types.ts`
  - 내용: `memory.archive.list`, `memory.archive.delete`, `memory.archive.restore`
  - 검증: `CGO_ENABLED=0 go test ./internal/rpc/...`

- [ ] **T2.C.2** — 보관 메모 목록
  - 파일: `apps/desktop/src/components/settings/MemorySection.tsx`, `MemorySection.test.tsx`, `apps/desktop/src/lib/i18n.tsx`
  - 내용: 작품 노트 아래에 접힌 목록을 둔다. 줄마다 되돌리기와 지우기를 둔다. 예산 초과로 되돌리기가 실패하면 이유를 보여 준다.
  - 검증: `make test-desktop`

### 작업 그룹 D: 키워드 기억 정리

- [ ] **T2.D.1** — `remember`를 보관으로 전환
  - 파일: `engine/internal/storyops/apply.go`, `engine/internal/engineapp/engineapp.go`, `engine/internal/companion/memory.go`
  - 내용:
    - `storyops`의 메모리 인터페이스(`Remember(projectID, text, category string) error`) 구현을 보관 저장소로 연결한다. `category`는 쓰지 않는다.
    - `Screen`을 통과한 내용만 저장한다.
  - 검증: `CGO_ENABLED=0 go test ./internal/storyops/...`

- [ ] **T2.D.2** — 기존 기록 가져오기
  - 파일: `engine/internal/companion/legacymemory.go`, `legacymemory_test.go`
  - 내용:
    - 시작할 때 작품별 `experiences.jsonl`을 읽어 보관 메모로 가져온다.
    - ID는 작품 ID와 본문의 해시로 만들어 여러 번 돌아도 중복되지 않게 한다.
    - 파일은 지우지 않는다. 내보내기가 `MemoryRoot`를 가리킨다.
    - 없는 작품의 기록은 건너뛰고 로그만 남긴다.
  - 검증: `CGO_ENABLED=0 go test ./internal/companion/...`

- [ ] **T2.D.3** — 최신 5개 주입 제거
  - 파일: `engine/internal/companion/context_sources.go`, `engine/internal/storycontext/builder.go`, `render.go`, `engine/internal/mcphost/tools_read.go`
  - 내용:
    - `ContextMemories`와 `Context.Memories` 경로를 제거한다. 관련도 없이 최신 5개를 넣던 경로다.
    - `ContextKeyMemories`는 큐레이션 메모리가 함께 쓰므로 유지한다.
    - `IncludedSections` 보고가 실제 내용과 맞는지 확인한다.
  - 검증: `CGO_ENABLED=0 go test ./internal/storycontext/... ./internal/mcphost/...`

---

## ✅ Phase 2 Checkpoint

**구현 확인:**
- [ ] 모든 작업 체크박스 완료
- [ ] 보관 후 노트 사용량이 줄고, 검색에서 해당 줄이 나온다
- [ ] 설정에서 보관 메모를 보고 지울 수 있다

**자동 검증:**
- [ ] `CGO_ENABLED=0 go test ./...` 통과 (`engine/`)
- [ ] `make test-mobile-engine` 통과
- [ ] `make test-desktop` 통과

**수동 확인:**
- [ ] 2158자 노트를 쓰는 작품 복사본에서 에이전트에게 끝난 결정의 정리를 맡겨 결과 확인
- [ ] 백업, 복원 뒤 보관 메모와 검색이 유지되는지 확인

**완료 처리:**
1. 완료 작업, 자동 검증 결과, 남은 수동 확인을 보고한다.
2. PR을 열고 작가 승인 뒤 다음 페이즈로 간다.
3. 실패하면 항목을 보고하고 원인 분석, 수정, 재검증 순으로 진행한다.

---

## 메모 / 주의

- 보관 메모는 언제 썼는지가 중요하다. 결과에 `archived_at`을 함께 돌려줘 오래된 결정을 구분하게 한다.
- `tars` 의존성의 `pkg/memory`는 가져오기에 쓰므로 이 페이즈에서 제거하지 않는다.
- 가져오기는 다른 사용자의 기기에서도 돈다. 이 개발 기기에는 `experiences.jsonl`이 없어 실데이터로 시험하지 못한다. 테스트 픽스처로 검증한다.

---
_다음 페이즈: Phase 3 — 모델별 대화 한도 → [`phase-3-history-budget.md`](./phase-3-history-budget.md)_
