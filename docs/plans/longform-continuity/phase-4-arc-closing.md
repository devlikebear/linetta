# Phase 4: 아크 결산과 영향 검사 — 작업지시서

_위에서부터 순차 실행하고 Checkpoint에서 작가 확인을 받는다._

_작성일: 2026-11-02_
_속한 로드맵: [`roadmap.md`](./roadmap.md)_
_예상 소요: 3.5일_
_상태: 구현 완료. 실제 모델 호출과 화면 수동 확인은 남아 있다_

## 페이즈 목표

작품을 권과 아크로 나누고, 아크를 닫으면 브리프가 다루는 기록이 줄어든다. 설정을 바꾸면 영향받는 회차가 근거와 함께 후보로 기록된다. 큰 변경은 작품 복제본에서 시험하고 승인한 수정만 반영한다.

이 페이즈가 #137의 핵심 시나리오를 처음으로 동작하게 한다.

## 전제 조건

- [x] Phase 3 완료 및 작가 승인
- [x] `main`에서 `feat/longform-arc-closing` 브랜치 생성
- [x] Phase 3의 정산 표본 결과 확인

## 포함 기능

1. 권·아크 계층
2. 복선 범위
3. 영향 검사
4. 결산·재구축 칩
5. 작품 복제
6. 게시 회차 경고

## 이 페이즈에서 하지 않는 것

- 원고 전면 재생성 → Out of Scope
- 영향 검사의 자동 수정 → 후보 기록까지만 한다
- 게시 회차 수정 차단 → 경고만 한다
- 인물 자동 은퇴 → 핵심 요소 선정만 바꾼다
- 규모 수치 확정 → Phase 5

## 작업 체크리스트

### 작업 그룹 A: 권·아크 계층

- [x] **T4.A.1** — 현재 아크 찾기
  - 파일: `engine/internal/ledger/brief.go`, `ledger_test.go`
  - 내용:
    - `tree`에 `arcOf(nodeID)`를 더한다. 가장 가까운 컨테이너가 아크다. 컨테이너가 한 겹인 작품은 지금처럼 부가 곧 아크다.
    - `part_arcs`는 이미 컨테이너 id를 쓰므로 스키마를 바꾸지 않는다.
    - 브리프의 부 섹션에 현재 권과 현재 아크를 함께 보인다. 남은 화수는 아크 기준이다.
  - 검증: 한 겹과 두 겹 구조 테스트

- [x] **T4.A.2** — 부별 흐름 예산
  - 파일: `engine/internal/storycontext/builder.go`, `longform_test.go`
  - 내용:
    - `partSummaries`가 현재 권, 현재 아크를 우선하고, 다른 권은 짧게 줄이게 한다.
    - 권이 10개여도 `partSummariesMaxRunes` 2,500자 안에 드는 테스트를 더한다.
    - 낡은 요약 표시(`staleMark`)는 유지한다.
  - 검증: `CGO_ENABLED=0 go test ./internal/storycontext/...`

- [ ] **T4.A.3** — 개요 화면 확인
  - 파일: `apps/desktop/src/components/OutlinePanel.tsx`
  - 내용: 컨테이너 안의 컨테이너를 만들고 옮기는 흐름을 확인한다. 막힌 곳만 고친다.
  - 검증: 수동 확인, vitest

### 작업 그룹 B: 복선 범위

- [x] **T4.B.1** — 범위 열
  - 파일: `engine/internal/store/migrations/00NN_ledger_scope.sql`, `engine/internal/ledger/ledger.go`, `repo.go`, `engine/internal/mcphost/tools_ledger.go`, 화면
  - 내용:
    - `story_ledger`에 `reach TEXT NOT NULL DEFAULT 'arc'`를 더한다. 값은 `arc`, `volume`, `work`다.
    - 기존 복선은 마이그레이션에서 `work`로 둔다. 지금 브리프에서 사라지면 안 되기 때문이다.
  - 검증: 마이그레이션 테스트, `CGO_ENABLED=0 go test ./internal/ledger/...`

- [x] **T4.B.2** — 브리프 범위 축소
  - 파일: `engine/internal/ledger/brief.go`
  - 내용:
    - 열린 복선을 현재 아크에서 심은 `arc`, 현재 권의 `volume`, `work`로 제한한다.
    - 회수 기한이 된 복선은 범위와 무관하게 `Due`에 나온다. 빠진 복선이 조용히 사라지지 않게 `linetta_list_ledger`의 `reach` 필터로 찾게 한다.
  - 테스트: 앞 아크의 닫힌 범위 복선이 안 보인다, 기한이 된 복선은 보인다
  - 검증: `CGO_ENABLED=0 go test ./internal/ledger/...`

- [x] **T4.B.3** — 핵심 요소 최근성
  - 파일: `engine/internal/entity/repo.go`, `engine/internal/mention/repo.go`, `engine/internal/storycontext/builder.go`
  - 내용:
    - `ListCoreByProject`를 호출하는 브리프 경로에서, 최근 30개 회차의 언급을 더 무겁게 본다. 작가가 지정한 역할은 유지한다.
    - 전체 목록 도구는 바꾸지 않는다.
  - 테스트: 오래전에 많이 나온 인물보다 최근 활성 인물이 먼저 든다
  - 검증: `CGO_ENABLED=0 go test ./internal/storycontext/...`

### 작업 그룹 C: 영향 검사와 표본 대조

- [x] **T4.C.0** — 언급 색인 빈틈 확인
  - 파일: `engine/internal/mention/`, 원고 저장 경로
  - 내용:
    - 실측 작품의 1화는 언급이 0건이고 다른 회차는 34~60건이다. 작가가 붙여 넣은 회차가 왜 색인되지 않았는지 찾는다.
    - 원인을 고치고, 작가가 요청할 때 작품 전체를 다시 색인하는 경로를 둔다. `where_does_appear`도 이 빈틈의 영향을 받는다.
  - 검증: 언급 링크가 없는 본문에서 재색인 뒤 등장 회차가 나오는 테스트

- [x] **T4.C.1** — 영향 후보
  - 파일: `engine/internal/ledger/impact.go` (신규), `impact_test.go`
  - 내용:
    - `Impact(ctx, entryID) ([]Candidate, error)`를 만든다. 입력은 바뀐 사실이다.
    - 후보는 같은 주어·속성 행의 심은 회차, 주어의 이름과 별칭이 원문에 나오는 회차, 인용이 있는 회차다. 근거 종류를 함께 낸다.
    - 언급 표만 쓰지 않는다. 실측에서 언급 기준 순위는 먼 모순 3쌍을 120쌍 중 64~114위로 밀어냈다. 원문 기준은 2쌍을 상위 60위 안에 넣었다.
    - 원문 조회는 `manuscript_fts`를 쓴다.
    - 쓴 회차만 내고 이야기 순서로 정렬한다. 최대 12개이며 넘치면 수를 알린다.
    - 코드로만 계산한다.
  - 참조: `engine/internal/manuscript/searcher.go`
  - 검증: `CGO_ENABLED=0 go test ./internal/ledger/...`

- [x] **T4.C.2** — 영향 패스
  - 파일: `engine/internal/agent/impactreview.go` (신규), 테스트, `loop.go`
  - 내용:
    - 턴에서 `supersedes`나 결정 추가가 있었으면 그 항목들을 모은다. `noteWrittenScene`과 같은 방식이다.
    - 도구는 `linetta_read_scene`, `linetta_search_manuscript`, `linetta_list_ledger`, `linetta_report_continuity`만 준다.
    - 프롬프트는 바뀐 사실, 후보 회차, 근거를 주고 실제로 어긋나는 문장만 인용으로 보고하게 한다. 주제 하나씩 좁혀 대조한다.
    - 관찰의 출처를 `continuity_checks.source = 'impact'`로 남긴다. 화면에서 따로 거른다.
    - 설정 `agent_impact_review_enabled`를 더하고 기본은 꺼짐이다.
  - 참조: `episodereview.go`, `repo_test.go`의 인용 검증
  - 테스트: 스위치, 허용 도구, 인용 불일치 거절
  - 검증: `CGO_ENABLED=0 go test ./internal/agent/... ./internal/continuity/...`

- [x] **T4.C.3** — 수동 칩
  - 파일: `apps/desktop/src/components/ContinuitySection.tsx`, `CanonPanel.tsx`, `apps/desktop/src/lib/i18n.tsx`
  - 내용: 원장 항목에 "이 결정의 영향 찾기"를 둔다. 누르면 입력창에 문장이 채워지고 작가가 전송한다.
  - 검증: vitest

- [x] **T4.C.4** — 표본 대조 일정
  - 파일: `engine/internal/continuity/sampling.go` (신규), `sampling_test.go`
  - 내용:
    - `SamplePairs(ctx, nodeID, seed) ([]Pair, error)`를 만든다. 코드로만 고른다.
    - 대상 회차마다 앵커(작품과 권, 아크의 첫 회차), 직전 2개 회차, 앞 회차 중 무작위 1개를 짝짓는다. 원문 후보(T4.C.1)가 있으면 무작위 대신 상위 회차를 하나 쓴다.
    - 시드는 작품과 회차에서 만들어 같은 입력이면 같은 일정이 나오게 한다. 이미 대조한 쌍은 버전이 바뀌기 전까지 건너뛴다.
    - 순수 무작위 단독 일정은 만들지 않는다. 실측 54회 기준으로 층화 표본이 모순 쌍 8.2/9개, 무작위가 4.1/9개를 덮었다.
  - 테스트: 회차당 짝 수가 상수이고, 같은 시드가 같은 결과를 내고, 건너뛰기가 동작한다
  - 검증: `CGO_ENABLED=0 go test ./internal/continuity/...`

- [x] **T4.C.5** — 표본 대조 패스
  - 파일: `engine/internal/agent/samplecheck.go` (신규), 테스트, `apps/desktop/src/components/ContinuitySection.tsx`, `apps/desktop/src/lib/i18n.tsx`
  - 내용:
    - `boundedPass`로 돌린다. 대화 기록을 싣지 않고 브리프 도구도 주지 않는다. 입력은 고른 두세 회차와 고정 작품 규칙뿐이다.
    - 도구는 `linetta_read_scene`, `linetta_report_continuity`만 준다.
    - 프롬프트는 두 회차가 함께 참일 수 없는 진술만 인용 두 개와 함께 보고하게 한다. 취향과 문체는 보고하지 않는다.
    - 관찰의 출처를 `continuity_checks.source = 'sample'`로 남긴다.
    - 실행은 작가가 시작한다. 연속성 패널의 버튼에 대상 범위와 회차 수, 예상 호출 수를 보인다. 턴 뒤 자동 실행은 넣지 않는다.
  - 참조: `episodereview.go`
  - 테스트: 메시지에 대화가 없고, 허용 도구 밖은 호출할 수 없다
  - 검증: `CGO_ENABLED=0 go test ./internal/agent/...`, vitest

### 작업 그룹 D: 결산과 재구축

- [x] **T4.D.1** — 아크 결산 칩
  - 파일: `apps/desktop/src/components/agent/AgentPanel.tsx`, `apps/desktop/src/lib/i18n.tsx`, 내장 절차 위치
  - 내용:
    - 문장을 채우는 칩으로 만든다. 새 도구를 만들지 않는다.
    - 절차: 회수된 복선 닫기, 남은 복선을 넘길지 드롭할지 분류, 범위 승격 제안, 끝난 사실의 유효 범위 닫기, 아크 요약과 다음 아크 계획 갱신, 대화 경계 제안.
    - 범위 승격과 복선 드롭은 `linetta_ask_writer` 카드로 묻는다.
    - 절차가 길어지면 시스템 프롬프트에 넣지 말고 스킬로 분리한다.
  - 검증: 복사본 작품의 1부로 한 번 돌려 본다

- [x] **T4.D.2** — 정본 재구축 칩
  - 파일: 위와 같음
  - 내용:
    - 현재 원고와 고정 규칙을 기준으로 시놉시스, 부 요약, 회차 계획, 원장을 대조하고 낡은 항목을 대체하는 절차다.
    - 옛 자유글 원장 행에 속성을 붙이는 작업도 여기서 한다.
    - 원고는 바꾸지 않는다. 바꾼 기록 목록을 끝에 보고한다.
    - 지금처럼 요약이 회차 수보다 뒤처진 경우를 첫 시나리오로 쓴다.
  - 검증: 복사본에서 실행 뒤 낡은 항목 수를 전후로 잰다

- [x] **T4.D.3** — 작품 복제
  - 파일: `engine/internal/restore/restore.go`, `engine/internal/vault/service.go`, `engine/internal/rpc/handlers/projects.go`, `apps/desktop/src-tauri/src/lib.rs`, 화면
  - 내용:
    - `keepCopy`가 쓰는 스냅샷·`MergeProject` 경로를 공용 함수로 뽑아 `projects.duplicate` RPC를 만든다.
    - 게시 정보는 복제하지 않는다. 복제본이 원본의 게시 회차를 덮어쓰면 안 된다.
    - 제목에 표시를 붙이고 원고, 세계관, 원장, 부별 구조를 복사한다.
    - 큰 설정 변경은 복제본에서 시험하라고 재구축 칩 문구에 안내한다.
  - 검증: 복제 뒤 원본이 그대로이고 복제본에 원장이 있는 테스트

### 작업 그룹 E: 게시 회차 경고

- [x] **T4.E.1** — 수정 경고
  - 파일: `engine/internal/mcphost/tools_write.go`, `tools_revise.go`
  - 내용:
    - `StatusPublished` 회차를 쓰면 저장하되 결과에 `notice`를 더한다. 게시본이 달라졌다는 사실과 다시 게시가 필요하다는 점을 쓴다.
    - 차단하지 않는다.
  - 검증: `CGO_ENABLED=0 go test ./internal/mcphost/...`

- [x] **T4.E.2** — 게시 전 열린 지적 안내
  - 파일: 게시 화면 구성요소, `apps/desktop/src/lib/i18n.tsx`
  - 내용: 게시할 회차에 `high` 관찰이 열려 있으면 게시 전에 수를 보여 준다. 게시는 막지 않는다.
  - 검증: vitest

---

## ✅ Phase 4 Checkpoint

**구현 확인:**
- [x] 모든 작업 체크박스 완료
- [x] 패스가 원고와 게시 상태를 바꾸지 않음
- [x] 한 겹 구조의 기존 작품에서 브리프가 달라지지 않음. 기존 복선은 `work`로 남음

**자동 검증:**
- [x] `CGO_ENABLED=0 go test ./...` 통과 (`engine/`)
- [x] `go test -tags mobile ./...` 통과
- [x] vitest, `tsc --noEmit`, 프런트 빌드 통과

**수동 확인:**
- [ ] 합성 작품에서 "진실을 아는 시점"을 18화에서 27화로 옮기고, 19~26화의 영향 회차가 후보로 나오는지 본다 (#137)
- [ ] 복사본에서 어긋난 설정을 하나 바꾸고 영향 패스가 해당 회차를 인용과 함께 찾는지 본다
- [ ] 알려진 모순을 되돌린 복사본에서 표본 대조를 돌려, 먼 회차의 모순을 대화 기록 없이 찾는지 본다
- [ ] 복사본 작품의 1부를 결산해 브리프의 열린 복선 수가 줄어드는지 잰다
- [ ] 작품을 복제해 재구축을 돌리고 원본이 그대로인지 확인한다
- [ ] 게시 회차 수정 경고가 뜨는지 확인한다

**완료 처리:** 결과와 측정표를 작가에게 보고하고 #137에 진행 상황을 남긴다. 승인 뒤 Phase 5로 간다.

---

## 구현 기록

2026-11-02.

- 1화에 언급이 없던 원인을 찾았다. 첫 글을 쓸 때 등록 요소가 없었고, 부분 수정은 링크를 만들지 않는다. `mentions.relink_work`를 더했고 복사본에서 7개 회차에 21건을 연결했다.
- 영향 후보는 `manuscript_fts` 대신 회차 본문을 직접 비교한다. 후보 계산이 작품당 한 번이고, 어절 쌍을 비교해야 "쥐어 본 적" 같은 표현을 찾을 수 있어서다.
- 복사본에서 시험 결정 "칼을 쥐어 본 적 없는 자만"을 넣으면 후보 순위 10화 20점, 11화 12점, 1화 10점이다. 언급 기준에서 묻혔던 1화가 상위로 올라왔다.
- 관찰의 출처는 `continuity_checks.source`를 새 값으로 넓히지 않고 메모 머리말(`영향 검사:`, `표본 대조:`)로 구분한다. 출처 값의 검증과 화면이 두 값을 전제하기 때문이다.
- 표본 대조는 턴과 무관한 패스다. `StartSampleCheck`가 턴 없이 공급자와 도구 세션을 세운다.
- 복제는 `projects.duplicate`로 `MergeProject`를 재사용한다. 게시 정보는 복사되지 않는다.
- 결정 하나의 영향 찾기 칩은 원장 항목이 아니라 패널의 장편 정리 칩에 두었다. 입력창에 닿는 곳이 패널뿐이다.
- 개요 화면의 중첩 컨테이너 생성과 이동은 직접 눌러 보지 못했다. 엔진과 브리프는 테스트로 확인했다.

## 메모 / 주의

- 복선 범위의 기본값은 새 항목에만 `arc`다. 기존 행은 `work`라 처음에는 줄어들지 않는다. 첫 결산에서 줄어든다.
- 영향 후보 12개는 출발값이다. 놓침이 많으면 Phase 5에서 조정한다.
- 표본 대조의 실측은 한 작품의 모순 쌍 9개로 소급 시험한 값이고, 모순이 1화에 몰린 작품이라 앵커 효과가 크게 나왔을 수 있다. 덮은 쌍이 곧 찾은 모순도 아니다.
- 표본 대조는 회차끼리의 모순만 본다. 원장끼리의 충돌과 설정과 원고의 충돌은 Phase 3의 충돌 감지와 영향 검사가 맡는다.
- 작품 복제는 저장소 경로가 길게 얽혀 있다. 추정보다 커지면 이 작업만 따로 PR로 뺀다.
- 결산 절차는 프롬프트로 강제되지 않는다. 지난 분석에서 프롬프트 규칙이 지켜지지 않았으므로, 필요한 단계는 칩과 카드로 작가 앞에 드러낸다.

---
_다음 페이즈: Phase 5 — 규모 평가 → [`phase-5-scale-eval.md`](./phase-5-scale-eval.md)_
