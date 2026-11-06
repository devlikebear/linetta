# Phase 4: 내보내기와 되먹임 — 작업지시서

_작성일: 2026-11-04_
_속한 로드맵: [Clio 로드맵](./roadmap.md)_
_예상 소요: 2일_
_상태: 대기_

## 페이즈 목표

다 쓴 초안을 X나 Threads 작성창으로 넘기거나 복사한다. 게시했다고 표시한 초안은 내 글(`source = written`)이 되어 다음 프로필 계산에 들어간다. 2주 동안 실제로 써 보고 "계속 쓸지"를 판단할 수 있게 사용 통계와 기록 양식을 남긴다. CI에 Clio 검사를 붙인다.

## 전제 조건

- [ ] Phase 2 Checkpoint 통과(Phase 3은 없어도 된다. 없으면 제안 통계 항목은 비워 둔다)
- [ ] 브랜치: `feat/clio-phase-4`
- [ ] Clio 마이그레이션 다음 번호: Phase 3을 했으면 `0004`, 안 했으면 `0003`

## 포함 기능

1. 복사와 intent 링크
2. 게시 표시와 글 목록 반영
3. 프로필 갱신
4. 사용 통계와 도그푸딩 기록 양식
5. CI 작업

## 이 페이즈에서 하지 않는 것

- X/Threads API 게시, 게시 여부 자동 확인. 게시는 작가가 직접 누르고, 앱은 작가의 표시를 믿는다
- 게시 후 수정 반영. 게시 표시 뒤 X에서 고친 내용은 다시 가져오기로만 들어온다
- 배포 빌드(서명, dmg, 자동 업데이트)

## 작업 체크리스트

### 작업 그룹 A: 내보내기

- [ ] **T4.A.1** — 공유 링크
  - 파일: `apps/clio/src/lib/share.ts`, `share.test.ts`, `components/PublishBar.tsx`, `PublishBar.css`
  - 내용: 리네타 `apps/desktop/src/lib/share.ts`의 `shareAction`에서 X(`https://x.com/intent/post?text=`)와 Threads(`https://www.threads.com/intent/post?text=`) 부분만 옮긴다. 링크는 Tauri `open_external_url`로 연다(허용 호스트는 Phase 1에서 `x.com`, `www.threads.com`). 버튼은 "복사", "X에서 열기", "Threads에서 열기"다. X 글자 수가 280을 넘으면 "X에서 열기"를 막고 이유를 보여 준다
  - 블로그 맥락은 복사만 둔다

- [ ] **T4.A.2** — 게시 표시
  - 파일: `engine/internal/clio/draft/publish.go`, `publish_test.go`, `engine/internal/rpc/handlers/clio_drafts.go`
  - 내용: `drafts.mark_published`(`{draft_id, content_version}`)는 한 트랜잭션에서 두 가지를 한다. `drafts.published_at`을 기록하고, 같은 본문으로 `posts`에 `source='written'`, 초안의 platform과 kind로 넣는다. 같은 본문이 이미 있으면(다시 가져온 경우) 새로 넣지 않는다. `drafts.unmark_published`는 written 글을 지운다. 단일 연결이니 트랜잭션 안에서 다른 쿼리를 열지 않는다
  - 테스트: 표시 → 글 수 +1, 같은 본문 두 번 → +1, 표시 해제 → 원래대로, `content_version` 충돌

### 작업 그룹 B: 되먹임

- [ ] **T4.B.1** — 프로필 갱신
  - 파일: `engine/internal/clio/app/cache.go`, `cache_test.go`
  - 내용: 프로필과 Corpus 캐시를 `posts`의 변경 카운터(가져오기, 게시 표시, 해제)로 무효화한다. `voice.profile` 응답에 `n`, `written_n`, `computed_at`을 더한다. 내 말투 화면에 "가져온 글 150 · Clio로 쓴 글 3"을 표시한다

- [ ] **T4.B.2** — 사용 통계
  - 파일: `engine/internal/clio/migrations/000N_publish_findings.sql`(N은 전제 조건의 다음 번호), `engine/internal/clio/stats/stats.go`, `stats_test.go`, `engine/internal/rpc/handlers/clio_stats.go`, `apps/clio/src/routes/Stats.tsx`
  - 내용: `stats.summary`(`{since}`)는 다음을 집계한다
    - 만든 초안 수와 게시 표시 수
    - 규칙별 지적 수. `check.run` 결과를 저장하지 않으니, 게시 표시 순간의 지적을 `publish_findings(draft_id, rule, severity)`에 남긴다
    - (Phase 3을 했다면) 규칙별 제안 받음과 버림 수(`suggest_events`)
    - 게시 직전에 남아 있던 강한 지적 수. 작가가 무시한 지적이라는 신호다
  - 글 본문은 통계에 넣지 않는다

### 작업 그룹 C: 도그푸딩 기록과 CI

- [ ] **T4.C.1** — 기록 양식
  - 파일: `docs/plans/clio/dogfood.md`
  - 내용: 2주(시작일과 끝일) 동안 주 1회 적는 표다. 칸은 `stats.summary` 수치, "가장 쓸모 있던 지적", "가장 틀린 지적", "Clio를 안 쓰고 X에서 바로 쓴 글 수와 이유"다. 끝에 결정 칸을 둔다(계속 / 검사기만 / 접기). 계속이면 배포 로드맵을 따로 쓴다

- [ ] **T4.C.2** — CI
  - 파일: `.github/workflows/ci.yml`, `Makefile`
  - 내용: ubuntu에서만 도는 `clio` 작업을 추가한다. `make test-clio`를 돌리고, Go, pnpm, Node, Rust 설정은 기존 `ci` 작업의 단계를 그대로 쓴다. Windows 행렬에는 넣지 않는다. Go 테스트는 기존 `test-go`의 `go test ./...`가 이미 `internal/clio`를 돌리니, 이 작업은 프런트엔드와 크레이트를 맡는다. `apps/clio/**`가 바뀌지 않았으면 이 작업을 건너뛰도록 `paths` 필터를 둔다
  - 검증: PR에서 `clio` 작업이 초록색이다. 리네타 `ci` 작업의 시간이 늘지 않는다

---

### ✅ Checkpoint: Phase 4 완료 확인

**구현 확인:**
- [ ] `stats.summary`의 응답에 글 본문이 없다
- [ ] `dogfood.md`에 시작일이 적혀 있다

**실행 확인:**
- [ ] `cd engine && CGO_ENABLED=0 go test ./...` 통과, `make test-clio` 통과
- [ ] PR CI에서 `clio` 작업과 기존 작업이 모두 통과한다

**수동 확인:**
- [ ] 초안 → "X에서 열기" → 브라우저의 X 작성창에 본문이 채워진다. 게시는 작가가 직접 한다
- [ ] "게시함" 표시 → 내 말투 화면의 "Clio로 쓴 글"이 1 늘고, 프로필의 `computed_at`이 바뀐다
- [ ] 표시 해제 → 원래 숫자로 돌아온다
- [ ] 300자 한글 초안에서 "X에서 열기"가 막히고 이유가 보인다

**이 체크포인트를 통과하면 사용자에게 확인 요청 후 2주 도그푸딩을 시작한다.**
만약 실패하면: 실패 항목 보고 → 사용자와 함께 원인 파악 → 수정 → 재검증.
