# Phase 1: 브리프 용량 — 작업지시서

_위에서부터 순차 실행하고 Checkpoint에서 작가 확인을 받는다._

_작성일: 2026-11-02_
_속한 로드맵: [`roadmap.md`](./roadmap.md)_
_예상 소요: 1.5일_
_상태: 구현 완료. 실제 모델 턴 확인은 Phase 3 스파이크에서 함께 한다_

## 페이즈 목표

어떤 회차를 작업하든 브리프가 도구 결과 상한 안에 온전히 들어오고, 원장의 정해진 사실이 다시 브리프에 나오며, 목록 도구가 잘리지 않고 끝까지 읽힌다. 스키마는 바꾸지 않는다.

## 전제 조건

- [x] 작가 승인
- [x] `main`에서 `fix/longform-brief-capacity` 브랜치 생성
- [x] 구현 직전에 설치 앱의 `library.db*`와 `skills/`를 임시 폴더로 복사한다. `settings.json`은 복사하지 않는다

## 포함 기능

1. 실제 크기 회귀 테스트
2. 브리프 재배분
3. 원장 예산 분리
4. 목록 도구 페이지

## 이 페이즈에서 하지 않는 것

- 원장 스키마 변경 → Phase 2, 3
- 대화 재생 변경 → Phase 2의 선택 기능만
- `agent.maxToolResultChars` 상향 → 상한을 올리면 앞선 턴의 예산이 같이 무너진다. 줄이는 쪽으로 푼다
- 외부 MCP 클라이언트의 브리프 변경 → 현재 씬 본문은 내장 에이전트에서만 뺀다

## 작업 체크리스트

### 작업 그룹 A: 실패하는 테스트부터

- [x] **T1.A.1** — 실제 크기 fixture
  - 파일: `engine/internal/storycontext/longform_test.go`
  - 내용:
    - `seedRealisticWork(t)`를 더한다. 세계관 요소 56개(요약 약 230자, 속성 약 130자), 회차 본문 약 3,900자, 자료 12장, 작가 메모 약 1,200자로 맞춘다.
    - 원장은 가짜 `LedgerSource`로 넣는다. 복선 약 8,700자, 정해진 사실 21건이다.
    - 값은 16화 작품의 평균 크기를 흉내 낸다. 실제 문장은 쓰지 않는다.
  - 참조: 기존 `seedLongWork`, `TestLongWorkBriefFitsAToolResultAndLeadsWithTheStyle`
  - 검증: 수정 전 상태에서 상한 테스트가 실패하는 것을 확인하고 PR 본문에 적는다

- [x] **T1.A.2** — 도구 결과 기준 상한 테스트
  - 파일: `engine/internal/mcphost/tools_read_test.go`
  - 내용:
    - `SourceAgent`로 `linetta_get_story_context`를 부른 결과 텍스트의 룬 수를 잰다. JSON 이스케이프까지 포함해야 한다.
    - 쓴 회차와 안 쓴 회차를 모두 본다. 상한 24,000자, 여유 1,000자로 `≤ 23,000`을 건다.
    - `mcphost`는 `agent`를 가져오지 않으므로 상수를 기존 테스트처럼 테스트 안에 둔다.
  - 의존: T1.A.1
  - 검증: 수정 전 실패 확인

### 작업 그룹 B: 브리프 재배분

- [x] **T1.B.1** — 내장 에이전트 브리프에서 현재 씬 본문 제외
  - 파일: `engine/internal/mcphost/tools_read.go` (`getStoryContext`)
  - 내용:
    - 기존 `if d.Source == SourceAgent` 블록에서 `c.SceneText`도 비운다. 같은 이유의 기존 주석에 한 줄을 더한다. 에이전트는 수정 전에 `linetta_read_scene`으로 본문과 버전을 읽는다.
    - `ScenePlan`은 남긴다. `sectionReport`는 그 뒤에 돌도록 순서를 지킨다.
    - 외부 클라이언트의 브리프는 그대로다.
  - 테스트: 내장 에이전트에는 본문이 없고 외부에는 있다
  - 검증: `CGO_ENABLED=0 go test ./internal/mcphost/...`

- [x] **T1.B.2** — 세계관 카드 총량 예산
  - 파일: `engine/internal/storycontext/builder.go` (`loadEntityBriefs`), `longform_test.go`
  - 내용:
    - `entityBriefMaxRunes = 7000`을 더한다. 이번 회차에 언급된 요소, 핵심 요소 순서로 채운다.
    - 예산을 넘으면 `Recent` 줄부터 빼고, 그래도 넘는 요소는 카드를 뺀다. 빠진 요소는 기존 `ElementsNotInBrief`가 알려 준다.
    - 렌더 순서는 프롬프트 캐시를 위해 지금처럼 고정한다.
  - 참조: `entityRecentMaxRunes`, `trimToBudget`
  - 테스트: 언급된 요소는 예산을 넘어도 핵심 요소보다 먼저 남는다
  - 검증: `CGO_ENABLED=0 go test ./internal/storycontext/...`

- [x] **T1.B.3** — 자료집 예산
  - 파일: `engine/internal/storycontext/builder.go`
  - 내용:
    - `factBriefMaxRunes = 1500`을 더한다. 회차에 붙은 카드를 먼저 넣고, 작품 설정 카드(`intentional_fiction`), 나머지 순서로 채운다.
    - 잘린 카드 수는 렌더 끝에 한 줄로 알린다. `linetta_get_fact_cards`로 볼 수 있다고 쓴다.
    - `ContextFacts`의 조회 조건은 건드리지 않는다.
  - 테스트: 예산 안에 들고 회차 카드가 먼저 남는다
  - 검증: `CGO_ENABLED=0 go test ./internal/storycontext/...`

- [x] **T1.B.4** — 초과 안전장치
  - 파일: `engine/internal/storycontext/render.go`, `engine/internal/mcphost/tools_read.go`
  - 내용:
    - 작가 메모가 길거나 작품이 커서 그래도 넘치면 자료집, 관계, 세계관 뒤쪽 순서로 덜어 낸다.
    - 덜어 낸 섹션은 `getStoryContextOutput`의 새 필드 `omitted_sections`로 알린다. 조용히 잘리는 일을 없앤다.
    - 문체, 작가 방향, 원장, 현재 씬 계획은 덜어 내지 않는다.
  - 테스트: 강제로 큰 입력에서 상한 안에 들고 `omitted_sections`가 채워진다
  - 검증: `CGO_ENABLED=0 go test ./internal/storycontext/... ./internal/mcphost/...`

### 작업 그룹 C: 원장 예산 분리

- [x] **T1.C.1** — 복선과 사실의 예산을 나눈다
  - 파일: `engine/internal/ledger/brief.go`, `ledger_test.go`
  - 내용:
    - `briefMaxRunes`를 `briefOwedRunes = 2000`, `briefSettledRunes = 1600`으로 나눈다. 한쪽이 남으면 다른 쪽이 쓴다.
    - 회수할 복선이 먼저라는 기존 순서는 그대로 둔다. `TestBriefNeverReadsAheadAndPutsDueFirst`와 `TestBriefStaysInBudgetAndSaysWhatItLeftOut`는 수정하지 않고 통과해야 한다.
    - 새 테스트 `TestBriefKeepsSettledWhenOwedOverflows`: 복선이 예산을 넘쳐도 `Settled`가 비지 않는다.
  - 검증: `CGO_ENABLED=0 go test ./internal/ledger/...`

- [x] **T1.C.2** — 정해진 사실을 관련도로 고른다
  - 파일: `engine/internal/ledger/brief.go`
  - 내용:
    - 지금은 최신순이다. 현재 씬의 계획과 요약, 직전 회차 요약에 `Subject`가 나오는 항목을 앞에 두고, 같은 그룹 안에서는 최신순으로 둔다.
    - `ContextLedger`의 시그니처와 `LedgerSource` 인터페이스는 그대로 둔다. 이미 가진 `node.Repo` 목록의 값을 쓴다.
    - 임베딩이나 형태소 분석을 넣지 않는다.
  - 참조: [SillyTavern World Info](https://docs.sillytavern.app/usage/worldinfo)의 키워드 활성화
  - 테스트: 계획에 나온 주어의 오래된 항목이 무관한 최신 항목보다 먼저 들어온다
  - 검증: `CGO_ENABLED=0 go test ./internal/ledger/...`

### 작업 그룹 D: 목록 도구 페이지

- [x] **T1.D.1** — 공용 페이지 도우미
  - 파일: `engine/internal/mcphost/paging.go` (신규), `paging_test.go`
  - 내용:
    - `pageRows[T](rows []T, cursor string, maxRunes int) (page []T, next string, total int)`를 만든다. 행을 JSON으로 쟀을 때 `maxRunes` 안에서 끊는다.
    - 커서는 불투명한 문자열이다. 기본 `maxRunes`는 20,000이다.
    - 전체가 한 페이지에 들면 `next`는 비고 결과는 지금과 같다.
  - 검증: `CGO_ENABLED=0 go test ./internal/mcphost/...`

- [x] **T1.D.2** — 원장 목록
  - 파일: `engine/internal/mcphost/tools_ledger.go`
  - 내용:
    - `listLedgerInput`에 `cursor`, `query`를 더한다. `query`는 주어와 본문의 부분 문자열이다.
    - 출력에 `next_cursor`, `total`을 더한다. 도구 안에서는 최신순으로 정렬해 최근 결정이 먼저 보이게 한다.
    - `ledger.Repo.List`의 오래된 순은 화면이 쓰므로 바꾸지 않는다.
  - 검증: 57건 규모 테스트에서 페이지를 따라가면 모든 항목이 정확히 한 번 나온다

- [x] **T1.D.3** — 세계관 목록과 개요
  - 파일: `engine/internal/mcphost/tools_read.go`
  - 내용:
    - `linetta_list_characters`에 `cursor`, `query`를 더한다.
    - `linetta_get_outline`에 `cursor`를 더한다. 짧은 작품의 결과는 지금과 같다.
    - 도구 설명은 짧게 유지한다. `budget_test.go`의 상한이 움직이면 측정값과 이유를 주석에 남긴다.
  - 검증: 합성 300행 개요를 페이지로 끝까지 읽는다

- [x] **T1.D.4** — 프롬프트 한 줄
  - 파일: `engine/internal/agent/prompt.go`
  - 내용: 목록 결과에 `next_cursor`가 있으면 필요한 만큼 이어 읽으라고 원장 지침에 한 문장만 더한다.
  - 검증: `CGO_ENABLED=0 go test ./internal/agent/...`

---

## ✅ Phase 1 Checkpoint

**구현 확인:**
- [x] 모든 작업 체크박스 완료
- [x] 회수 우선 순서와 문체 우선 배치가 유지됨
- [x] 외부 클라이언트의 브리프 본문이 그대로임

**자동 검증:**
- [x] `CGO_ENABLED=0 go test ./...` 통과 (`engine/`)
- [x] `go test -tags mobile ./internal/ledger/... ./internal/storycontext/...` 통과
- [x] `scripts/validate-story-core-deps.sh` 통과

**수동 확인:**
- [x] 임시 `go test -overlay`로 실제 DB 복사본의 1~25화 브리프를 뽑아, 모든 회차의 도구 결과가 23,000자 이내이고 `Settled`가 1건 이상임을 확인한다
- [x] 같은 복사본에서 원장과 세계관 목록을 페이지로 끝까지 읽어 빠지는 항목이 없음을 확인한다
- [ ] 실제 내장 에이전트 턴에서 회차 수정을 한 번 돌려 `linetta_read_scene` 호출 뒤 저장되는 것을 확인한다

**완료 처리:** 결과와 측정표를 작가에게 보고하고, 승인 뒤 Phase 2로 간다. 실패하면 항목을 보고하고 원인을 함께 파악한 뒤 수정하고 다시 잰다.

---

## 측정 기록

2026-11-02, 실제 DB 복사본의 25개 회차.

| 항목 | 이전 | 이후 |
|---|---|---|
| 도구 결과 최대 크기 | 쓴 회차 약 30,000자 이상 | 22,574자 |
| 상한 초과 회차 | 쓴 회차 전부 | 0 |
| `Settled`가 빈 회차 | 전부 | 0 (회차당 3~6건) |
| 원장 목록 | 45,500자, 잘림 | 3페이지, 57건 전부 |
| 세계관 목록 | 30,000자, 잘림 | 2페이지, 56개 전부 |

처음 한도 23,000자에서는 `elements_not_in_brief`와 섹션 보고까지 합쳐 2개 회차가 넘었다. 전체 결과 기준으로 재서 `agentBriefMaxRunes`를 20,500자로 낮췄다. 9개 회차에서 자료집이 `omitted_sections`로 빠진다.

## 메모 / 주의

- 세계관 7,000자, 자료집 1,500자, 원장 3,600자는 실측 재배분의 출발값이다. 합계는 약 21,800자로 추정된다. 수동 확인에서 다시 재서 조정한다.
- 브리프 앞부분의 순서를 흔들면 프롬프트 캐시가 깨진다. 문체보다 앞에 새 섹션을 넣지 않는다.
- `linetta_list_ledger`를 최신순으로 바꾸면 기존 스킬이나 작업 습관이 오래된 순을 기대할 수 있다. 출력 항목의 `created_at`으로 구분할 수 있으니 별도 호환 처리는 하지 않는다.

---
_다음 페이즈: Phase 2 — 작가 경계 → [`phase-2-writer-boundaries.md`](./phase-2-writer-boundaries.md)_
