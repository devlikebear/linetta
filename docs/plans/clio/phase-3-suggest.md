# Phase 3: 제안 — 작업지시서

_작성일: 2026-11-04_
_속한 로드맵: [Clio 로드맵](./roadmap.md)_
_예상 소요: 3일_
_상태: 대기 (게이트 있음)_

## 페이즈 목표

짚은 곳(또는 작가가 고른 구간)에서 "제안 받기"를 누르면, 그 구간만 바꾼 내 말투 후보가 1~3개 나온다. 작가가 하나를 받으면 받기 전 본문이 버전으로 남는다. 선택 기능으로 메모를 주면 내 말투 초안을 받는다. 실험의 B 방식이며, 게이트 결과에 따라 뺄 수 있다.

## 전제 조건

- [ ] Phase 2 Checkpoint 통과, 작가 승인
- [ ] **게이트**: run-12 `blind.html`을 작가가 직접 매기고, "결과 보기" 표(글별 '나 같음' 평균, 원문으로 고른 횟수)를 `docs/plans/clio/eval-check.md`에 옮겼다. 다음 둘 중 하나라도 맞으면 작업 그룹 C(메모로 초안)를 한다. 둘 다 아니면 하지 않는다
  - B를 원문으로 고른 횟수가 12묶음 중 3회 이상이다. 네 글 중 하나를 고르니 무작위가 3회다
  - B의 '나 같음' 평균이 원문보다 0.5점 넘게 낮지 않다
- [ ] 브랜치: `feat/clio-phase-3`
- [ ] Clio 마이그레이션 다음 번호: `0003`

## 포함 기능

1. 프로바이더 설정(리네타 `provider`, `settings` 재사용)
2. 구간 제안과 받기/버리기
3. (게이트 통과 시) 메모로 초안
4. 버전과 되돌리기

## 이 페이즈에서 하지 않는 것

- 글 전체 자동 고쳐 쓰기, 측정값에 맞춘 반복 고쳐 쓰기. run-12의 C처럼 평균으로 깎인다
- 에이전트 루프, 도구 호출, MCP. 한 번 묻고 한 번 받는다(`Ask`)
- Codex OAuth 로그인 화면. Clio는 `anthropic`, `openai`, `claude-code` 세 프로바이더만 보여 준다. `codexauth`는 리네타에서 쓰면 된다

## 작업 체크리스트

### 작업 그룹 A: 프로바이더

- [ ] **T3.A.1** — Clio 앱에 프로바이더 연결
  - 파일: `engine/internal/clio/app/app.go`, `app/providers.go`, `engine/internal/clio/handlers/providers.go`(필요할 때만)
  - 내용: `settings.NewStore(<home>, secrets)`와 `provider.NewSource(settingsStore, codexHome)`를 만들고, `engineapp/providers.go`가 등록하는 `settings.get`, `settings.set`, `providers.list`, `providers.list_models`, `providers.test` 핸들러를 같은 팩토리로 등록한다. 핸들러를 복사하지 않고 그대로 쓴다. Clio 쪽에서 프로바이더 목록을 세 개로 거르는 것은 프런트엔드에서 한다
  - 검증: `scripts/validate-story-core-deps.sh` 통과

- [ ] **T3.A.2** — 설정 화면
  - 파일: `apps/clio/src/routes/Settings.tsx`, `Settings.css`, `Settings.test.tsx`, `lib/providers.ts`
  - 내용: 프로바이더 선택, API 키(키체인 저장), 모델 선택, "연결 확인"(`providers.test`), AI 데이터 전송 동의(`AIDataSharingConsentedAt`, 리네타와 같은 동의 문구 구조). 리네타 `components/settings/ProviderSection.tsx`의 흐름을 따르되, 앱 간 import는 하지 않고 필요한 부분만 옮긴다

### 작업 그룹 B: 구간 제안

- [ ] **T3.B.1** — 제안 패키지
  - 파일: `engine/internal/clio/suggest/suggest.go`, `prompt.go`, `similar.go`, `suggest_test.go`
  - 내용:
    ```go
    type Asker interface{ Ask(ctx context.Context, prompt string) (string, error) }
    type Request struct {
        Body          string
        Start, End    int            // rune 오프셋
        Platform, Kind string
        Finding       *check.Finding // 짚은 곳에서 왔으면
    }
    type Candidate struct{ Text string; Findings []check.Finding }
    func Suggest(ctx context.Context, a Asker, req Request, prof voice.ContextProfile, corpus []post.Post) ([]Candidate, error)
    ```
    - `Asker`는 `tars/pkg/llm.Client`가 구조적으로 만족한다. `suggest`는 tars를 import하지 않는다(의존성 게이트)
    - **말투는 초안을 따른다.** 구간 밖 본문에서 존댓말과 반말 중 우세한 쪽을 정하고, 프롬프트에 "이 글은 존댓말로 쓰고 있다"처럼 못 박는다. 프로필의 다수 말투로 바꾸지 않는다. run-12에서 B가 답글의 존댓말을 반말로 바꾼 실패를 막기 위해서다
    - 비슷한 글(`similar.go`): `eval-voice.py`의 `similar()`를 옮긴다(문자 bigram Jaccard 상위 5편 + 무작위 3편). 다만 같은 맥락이고 같은 말투인 글에서만 고른다
    - 프롬프트는 1인칭으로 쓴다("내가 쓰던 글이다. 표시한 부분만 내 말투로 바꿔 줘"). "작성자 대신, 구분되지 않게" 같은 3인칭 사칭형은 거절된다(PR #319). 출력은 후보 1~3개를 줄 단위 JSON 배열로 받는다. 구간 밖 본문은 보내되 바꾸지 말라고 적는다
    - 후보 검증: 후보를 넣은 전체 본문에 `check.Run`을 다시 돌린다. 강한 지적이 새로 생긴 후보는 버린다. 한글이 없는 답은 오류로 낸다. 후보가 0개면 `rpc.NotFound(ReasonNoSuggestion, ...)`
  - 테스트: 가짜 `Asker`로 검증한다
    - 존댓말 초안이면 프롬프트에 존댓말 지시가 들어간다
    - 강한 지적이 생기는 후보는 버린다
    - JSON이 깨지면 줄 단위로 복구한다
    - 비슷한 글은 맥락과 말투가 다르면 고르지 않는다

- [ ] **T3.B.2** — 받기와 버전
  - 파일: `engine/internal/clio/migrations/0003_draft_versions.sql`, `engine/internal/clio/draft/versions.go`, `versions_test.go`, `engine/internal/clio/handlers/suggest.go`(+ `_test.go`)
  - 내용:
    - 테이블: `draft_versions(id, draft_id, body, reason TEXT CHECK (reason IN ('suggestion','manual')), created_at)`
    - RPC: `suggest.request`(`Request` → 후보), `suggest.accept`(`{draft_id, content_version, start, end, text}`). 받기 전 본문을 버전으로 남긴 뒤 구간을 바꾸고, `content_version`이 틀리면 `CodeContentConflict`를 낸다. 함께 `drafts.versions`, `drafts.restore`
    - 받은 제안과 버린 제안은 Phase 4 통계에 쓰니 `suggest_events(draft_id, rule, accepted INTEGER, created_at)`에 기록한다(같은 마이그레이션)

- [ ] **T3.B.3** — 편집기 연결
  - 파일: `apps/clio/src/components/SuggestPopover.tsx`, `SuggestPopover.css`, `SuggestPopover.test.tsx`, `routes/Write.tsx`
  - 내용: 지적을 누르거나 구간을 고른 뒤 "제안 받기"를 누르면 팝오버가 열린다. 후보마다 바뀐 부분을 표시하고, 받기와 버리기 버튼을 둔다. 대기 중에는 취소할 수 있다. 프로바이더가 설정되지 않았으면 설정 화면으로 안내한다. 초안 목록에 "버전" 보기를 두고, 되돌리기를 할 수 있게 한다

### 작업 그룹 C: 메모로 초안 (게이트 통과 시에만)

- [ ] **T3.C.1** — 초안 생성
  - 파일: `engine/internal/clio/suggest/draft.go`, `draft_test.go`, `clio/handlers/suggest.go`
  - 내용: `suggest.draft`(`{note, platform, kind, register}` → 본문). 실험의 B 방식(프로필 수치 + 비슷한 글 + 메모)이다. 다만 `register`는 작가가 고른다(`존댓말`, `반말`). 결과는 새 초안으로 저장하고 바로 `check.run`을 보여 준다. 자동 고쳐 쓰기는 없다
  - UI: 새 초안 버튼 옆에 "메모로 시작"을 둔다. 메모, 맥락, 말투를 입력한다

---

### ✅ Checkpoint: Phase 3 완료 확인

**구현 확인:**
- [ ] `go list -deps ./internal/clio/suggest | grep tars`가 비어 있다
- [ ] 게이트 결과가 `eval-check.md`에 있고, 작업 그룹 C를 했는지 안 했는지 적혀 있다

**실행 확인:**
- [ ] `cd engine && CGO_ENABLED=0 go test ./...` 통과, `make test-clio` 통과, `scripts/validate-story-core-deps.sh` 통과

**수동 확인 (설정한 프로바이더로):**
- [ ] X 답글 맥락에서 "좋네요. 결론적으로 다양한 인사이트를 얻으실 수 있을 것 같아요"를 쓰고 AI 표현 지적에서 제안을 받는다. 후보가 존댓말을 유지하고, 지적된 구간 밖은 그대로다
- [ ] 받은 뒤 버전 목록에 받기 전 본문이 있고, 되돌리면 원래대로 돌아온다
- [ ] 프로바이더를 지우면 "제안 받기"가 설정 안내로 바뀌고, 검사기는 계속 동작한다
- [ ] 다른 창에서 같은 초안을 고친 뒤 받으면 충돌 토스트가 뜨고 본문이 덮어써지지 않는다

**이 체크포인트를 통과하면 사용자에게 확인 요청 후 다음으로 진행.**
만약 실패하면: 실패 항목 보고 → 사용자와 함께 원인 파악 → 수정 → 재검증.
